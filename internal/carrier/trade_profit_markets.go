package carrier

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	bulkTradeRadius   = 250
	bulkTradePage     = 100
	bulkTradePages    = 2
	mixedTradeSystems = 12
	mixedTradePaths   = 12
	mixedTradeBuyers  = 12
)

type bulkTradeOffer struct {
	market          M
	price, quantity float64
}

type bulkTradeMarkets struct {
	buys, sells []bulkTradeOffer
}

type tradeSupplySystem struct {
	market M
	offers []M
}

type tradeSupplyPath struct {
	markets, buyOffers []M
	supply             float64
}

// Rank pickup chains by gross margin before passing the shortlist to the
// planner. The quotes remain in loading order so each hop uses its own mass.
func tradeSupplyPaths(suppliers []tradeSupplySystem, sale M, minimumCargo, cargoCeiling float64) []tradeSupplyPath {
	if minimumCargo < 1 || cargoCeiling < minimumCargo {
		return nil
	}
	salePrice := v.Num(sale["price"])
	saleID := v.Str(sale["marketId"])
	type supply struct {
		market M
		offers []M
		tons   float64
		gross  float64
	}
	var groups []supply
	for _, system := range suppliers {
		var group supply
		group.market = system.market
		for _, offer := range system.offers {
			if v.Str(offer["marketId"]) == saleID && saleID != "" || v.Num(offer["price"]) >= salePrice {
				continue
			}
			tons := v.Num(offer["supply"])
			if tons <= 0 {
				continue
			}
			group.gross += math.Min(tons, math.Max(0, cargoCeiling-group.tons)) * (salePrice - v.Num(offer["price"]))
			group.tons += tons
			group.offers = append(group.offers, offer)
		}
		if group.tons > 0 {
			groups = append(groups, group)
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].gross != groups[j].gross {
			return groups[i].gross > groups[j].gross
		}
		return v.Str(groups[i].market["system"]) < v.Str(groups[j].market["system"])
	})
	if len(groups) > mixedTradeSystems {
		groups = groups[:mixedTradeSystems]
	}

	type scoredPath struct {
		order  [3]int
		size   int
		supply float64
		gross  float64
	}
	var shortlisted []scoredPath
	var search func(path scoredPath, visited uint16)
	search = func(path scoredPath, visited uint16) {
		if path.size >= 2 && path.supply >= minimumCargo && path.gross >= netOpportunityMinimum {
			shortlisted = append(shortlisted, path)
		}
		if path.size == len(path.order) || path.supply >= cargoCeiling {
			return
		}
		for index, group := range groups {
			bit := uint16(1 << index)
			if visited&bit != 0 {
				continue
			}
			next := path
			next.order[next.size] = index
			next.size++
			remaining := cargoCeiling - next.supply
			for _, offer := range group.offers {
				tons := math.Min(remaining, v.Num(offer["supply"]))
				next.gross += tons * (salePrice - v.Num(offer["price"]))
				remaining -= tons
				if remaining <= 0 {
					break
				}
			}
			next.supply += group.tons
			search(next, visited|bit)
		}
	}
	search(scoredPath{}, 0)
	sort.SliceStable(shortlisted, func(i, j int) bool {
		if shortlisted[i].gross != shortlisted[j].gross {
			return shortlisted[i].gross > shortlisted[j].gross
		}
		return shortlisted[i].size < shortlisted[j].size
	})
	if len(shortlisted) > mixedTradePaths {
		shortlisted = shortlisted[:mixedTradePaths]
	}
	paths := make([]tradeSupplyPath, 0, len(shortlisted))
	for _, ranked := range shortlisted {
		path := tradeSupplyPath{supply: ranked.supply}
		for _, index := range ranked.order[:ranked.size] {
			group := groups[index]
			path.markets = append(path.markets, group.market)
			path.buyOffers = append(path.buyOffers, group.offers...)
		}
		paths = append(paths, path)
	}
	return paths
}

func (offer bulkTradeOffer) quote(key, availability string) M {
	quote, _, _ := tradeMarket(v.Merge(M{}, offer.market, M{
		"commodityKey": key, "price": offer.price, availability: offer.quantity,
	}), key)
	return quote
}

// The station endpoint returns the entire market even without a commodity
// filter. Sample several distance bands to cover both the departure area and
// the outer part of the search, without a separate request for each good.
func findBulkTradeMarkets(ctx context.Context, searcher trading.Searcher, origin M, pad string, ageHours float64, now time.Time) (map[string]bulkTradeMarkets, bool, error) {
	coords := trading.Coords(origin["coordinates"])
	if len(coords) != 3 {
		return nil, false, errors.New("Координаты системы отправления неизвестны.")
	}
	type bandResult struct {
		stations []M
		limited  bool
		err      error
	}
	bands := [][2]int{{0, 80}, {80, 160}, {160, bulkTradeRadius}}
	results := make(chan bandResult, len(bands))
	options := trading.NormalizeOptions(M{
		"minPad": pad, "includeCarriers": false, "radiusLy": bulkTradeRadius,
		"maxAgeHours": ageHours, "maxArrivalLs": 10000,
	})
	since := now.Add(-time.Duration(v.Num(options["maxAgeHours"]) * float64(time.Hour)))
	for _, band := range bands {
		go func() {
			filters := M{
				"has_market": M{"value": true}, "distance": M{"min": band[0], "max": band[1]},
				"market_updated_at":   M{"value": []string{v.ISO(since), v.ISO(now.Add(10 * time.Minute))}, "comparison": "<=>"},
				"type":                M{"value": []string{"Drake-Class Carrier", "Squadron Carrier"}, "logic": "not"},
				"distance_to_arrival": M{"value": []any{0, options["maxArrivalLs"]}, "comparison": "<=>"},
			}
			if pad == "L" {
				filters["has_large_pad"] = M{"value": true}
			}
			out := bandResult{}
			for request, page := 0, 0; request < bulkTradePages; request++ {
				payload := M{
					"filters": filters, "sort": []M{{"distance": M{"direction": "asc"}}},
					"size": bulkTradePage, "page": page,
					"reference_coords": M{"x": coords[0], "y": coords[1], "z": coords[2]},
				}
				response, err := trading.FetchJSON(ctx, searcher.Client, http.MethodPost, searcher.BaseURL+"/stations/search", payload)
				if err != nil {
					out.err = err
					break
				}
				raw := v.Obj(response)
				if _, present := raw["results"]; !present {
					out.err = errors.New("В ответе Spansh нет списка рынков.")
					break
				}
				rows := v.Rows(raw["results"])
				out.stations = append(out.stations, rows...)
				count, known := v.Number(raw["count"])
				out.limited = count > float64(len(out.stations)) || !known && len(rows) >= bulkTradePage
				if !out.limited || len(rows) < bulkTradePage {
					break
				}
				// Reach beyond the nearest stations in a dense distance band.
				page = max(1, int(count)/bulkTradePage/2)
			}
			results <- out
		}()
	}

	markets := map[string]bulkTradeMarkets{}
	goods := map[string]M{}
	seen := map[string]bool{}
	var firstErr error
	limited := false
	for range bands {
		band := <-results
		limited = limited || band.limited || band.err != nil
		if firstErr == nil {
			firstErr = band.err
		}
		for _, station := range band.stations {
			indexBulkTradeStation(markets, seen, goods, station, options, coords, now)
		}
	}
	return markets, limited, firstErr
}

func indexBulkTradeStation(markets map[string]bulkTradeMarkets, seen map[string]bool, goods map[string]M, station, options M, coords []float64, now time.Time) {
	id := v.Text(station["market_id"], station["id"])
	if id == "" || seen[id] || v.Str(station["system_name"]) == "" || v.Str(station["name"]) == "" {
		return
	}
	observations := v.Rows(station["market"])
	indexed := map[string]bool{}
	var market M
	for _, raw := range observations {
		name := v.Text(raw["symbol"], raw["commodity"], raw["name"])
		good, cached := goods[name]
		if !cached {
			good = trading.Good(name, nil)
			goods[name] = good
		}
		key := v.Str(good["key"])
		if key == "" || good["kind"] == "raw" || indexed[key] {
			continue
		}
		buy, sell := v.Num(raw["buy_price"]), v.Num(raw["sell_price"])
		supply, demand := math.Floor(v.Num(raw["supply"])), math.Floor(v.Num(raw["demand"]))
		canBuy, canSell := buy > 0 && buy <= 1e9 && supply > 0, sell > 0 && sell <= 1e9 && demand > 0
		if !canBuy && !canSell {
			continue
		}
		if market == nil {
			mode := "sell"
			if canBuy {
				mode = "buy"
			}
			resource := v.Merge(M{}, good, M{"quantity": 1})
			market = trading.StationResult(station, []M{resource}, v.Merge(M{}, options, M{"mode": mode}), now, coords)
			if market == nil || len(trading.Coords(market["coordinates"])) != 3 {
				return
			}
			delete(market, "quotes")
			seen[id] = true
		}
		indexed[key] = true
		book := markets[key]
		if canBuy {
			book.buys = append(book.buys, bulkTradeOffer{market, buy, supply})
		}
		if canSell {
			book.sells = append(book.sells, bulkTradeOffer{market, sell, demand})
		}
		markets[key] = book
	}
}

func (plan tradeOpportunityInputs) localCandidates(markets map[string]bulkTradeMarkets, fuelMarkets []M, now time.Time) []M {
	cargoCeiling := math.Floor(plan.capacity - plan.services)
	if cargoCeiling < 1 {
		return nil
	}
	loadingPad := v.Str(plan.options["haulerPad"])
	unloadingPad := v.Text(plan.options["unloadHaulerPad"], loadingPad)
	padRanks := map[string]int{"S": 1, "M": 2, "L": 3}
	var rows []M
	for key, book := range markets {
		if len(book.buys) == 0 || len(book.sells) == 0 {
			continue
		}
		cheapest := math.Inf(1)
		for _, buy := range book.buys {
			if padRanks[v.Str(buy.market["pad"])] >= padRanks[loadingPad] {
				cheapest = math.Min(cheapest, buy.price)
			}
		}
		var sales []bulkTradeOffer
		for _, sale := range book.sells {
			if padRanks[v.Str(sale.market["pad"])] >= padRanks[unloadingPad] &&
				(sale.price-cheapest)*math.Min(sale.quantity, cargoCeiling) >= netOpportunityMinimum {
				sales = append(sales, sale)
			}
		}
		if len(sales) == 0 {
			continue
		}
		suppliers := map[string][]bulkTradeOffer{}
		for _, buy := range book.buys {
			if padRanks[v.Str(buy.market["pad"])] < padRanks[loadingPad] {
				continue
			}
			system := strings.ToLower(v.Str(buy.market["system"]))
			suppliers[system] = append(suppliers[system], buy)
		}
		var mixedSuppliers []tradeSupplySystem
		for _, system := range suppliers {
			sort.Slice(system, func(i, j int) bool {
				if system[i].price != system[j].price {
					return system[i].price < system[j].price
				}
				return v.Str(system[i].market["marketId"]) < v.Str(system[j].market["marketId"])
			})
			reposition, reachable := tradeDirectHop(plan.origin, system[0].market)
			if !reachable {
				continue
			}
			group := tradeSupplySystem{market: system[0].market}
			for _, supplier := range system {
				if quote := supplier.quote(key, "supply"); quote != nil {
					group.offers = append(group.offers, quote)
				}
			}
			mixedSuppliers = append(mixedSuppliers, group)
			for _, sale := range sales {
				saleCeiling := math.Min(sale.quantity, cargoCeiling)
				if (sale.price-system[0].price)*saleCeiling < netOpportunityMinimum {
					continue
				}
				loaded, outward := tradeDirectHop(system[0].market, sale.market)
				back, inward := tradeDirectHop(sale.market, plan.origin)
				if !outward || !inward {
					continue
				}
				rows = append(rows, plan.localSupplyCandidates(key, system, sale, saleCeiling, reposition, loaded, back, fuelMarkets, now)...)
			}
		}
		rows = append(rows, plan.localMixedSupplyCandidates(key, mixedSuppliers, sales, cargoCeiling, fuelMarkets, now)...)
	}
	return rows
}

func (plan tradeOpportunityInputs) localMixedSupplyCandidates(key string, suppliers []tradeSupplySystem, sales []bulkTradeOffer, cargoCeiling float64, fuelMarkets []M, now time.Time) []M {
	if len(suppliers) < 2 {
		return nil
	}
	minimumCargo := cargoCeiling - plan.fuelForCargo(0, nil, nil, nil, nil).hold
	// Even the longest local pickup chain fits in the tank for a personal
	// carrier. Only manual squadron fuel can displace local freight.
	sort.Slice(sales, func(i, j int) bool {
		if sales[i].price != sales[j].price {
			return sales[i].price > sales[j].price
		}
		return v.Str(sales[i].market["marketId"]) < v.Str(sales[j].market["marketId"])
	})
	var rows []M
	checked := 0
	for _, sale := range sales {
		if sale.quantity < minimumCargo {
			continue
		}
		back, reachable := tradeDirectHop(sale.market, plan.origin)
		sell := sale.quote(key, "demand")
		if !reachable || sell == nil {
			continue
		}
		if checked >= mixedTradeBuyers {
			break
		}
		checked++
		ceiling := math.Min(sale.quantity, cargoCeiling)
		for _, path := range tradeSupplyPaths(suppliers, sell, minimumCargo, ceiling) {
			reposition, reachable := tradeDirectHop(plan.origin, path.markets[0])
			if !reachable {
				continue
			}
			loaded, reachable := localTradePickupHops(path, sale.market)
			if !reachable {
				continue
			}
			row := plan.candidate(key, path.buyOffers, sell, path.supply, math.Min(path.supply, ceiling), reposition, loaded, back, fuelMarkets, now)
			if row != nil && len(tradeOrderSystems(v.Obj(row["buy"]))) == len(path.markets) {
				rows = append(rows, row)
			}
		}
	}
	return rows
}

func localTradePickupHops(path tradeSupplyPath, sale M) ([]tradeHop, bool) {
	var hops []tradeHop
	from := path.markets[0]
	for _, market := range path.markets[1:] {
		leg, reachable := tradeDirectHop(from, market)
		if !reachable {
			return nil, false
		}
		hops = append(hops, leg...)
		from = market
	}
	last, reachable := tradeDirectHop(from, sale)
	return append(hops, last...), reachable
}

func tradeOrderSystems(buy M) map[string]bool {
	systems := map[string]bool{}
	for _, order := range v.Rows(buy["orders"]) {
		systems[strings.ToLower(v.Str(order["system"]))] = true
	}
	return systems
}

func (plan tradeOpportunityInputs) localSupplyCandidates(key string, suppliers []bulkTradeOffer, sale bulkTradeOffer, ceiling float64, reposition, loaded, back []tradeHop, fuelMarkets []M, now time.Time) []M {
	sell := sale.quote(key, "demand")
	if sell == nil {
		return nil
	}
	minimumCargo := math.Floor(plan.capacity - plan.services - plan.fuelForCargo(0, reposition, loaded, back, nil).hold)
	if ceiling < minimumCargo {
		return nil
	}
	var rows, combined []M
	var combinedSupply float64
	for _, supplier := range suppliers {
		if supplier.market["marketId"] == sale.market["marketId"] || supplier.price >= sale.price {
			continue
		}
		buy := supplier.quote(key, "supply")
		if buy == nil {
			continue
		}
		combined = append(combined, buy)
		combinedSupply += supplier.quantity
		if supplier.quantity < minimumCargo || (sale.price-supplier.price)*ceiling < netOpportunityMinimum {
			continue
		}
		row := plan.candidate(key, []M{buy}, sell, supplier.quantity, math.Min(supplier.quantity, ceiling), reposition, loaded, back, fuelMarkets, now)
		if row != nil {
			rows = append(rows, row)
		}
	}
	if len(combined) > 1 && combinedSupply >= minimumCargo {
		row := plan.candidate(key, combined, sell, combinedSupply, math.Min(combinedSupply, ceiling), reposition, loaded, back, fuelMarkets, now)
		if row != nil && len(v.Rows(v.At(row, "buy", "orders"))) > 1 {
			rows = append(rows, row)
		}
	}
	return rows
}
