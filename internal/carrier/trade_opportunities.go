package carrier

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/data"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	tradePeakSystem          = "HIP 58832"
	tradePeakPort            = "Rackham's Peak"
	tradePeakMarket          = "128968675"
	tradePeakScenario        = "booze-cruise"
	TradeOpportunityScenario = "profitable"
	netOpportunityMinimum    = 400_000_000
	// Allow time to load and sell each shuttle's cargo. The route has separate
	// flight time. Both are estimates and can be edited in the imported plan.
	tradeShuttleMinutes = 10
)

type tradeOpportunityInputs struct {
	options, state, origin M
	system                 string
	services               float64
	capacity, budget       float64
	estimatedCarrier       bool
	manualFuel             []float64
}

type tradeHop struct {
	from, to string
	distance float64
}

func tradeServices(trade, carrier M) (float64, bool) {
	if tons := v.Num(v.At(trade, "options", "servicesTons")); tons > 0 {
		return tons, false
	}
	if v.At(trade, "options", "carrierType") == "fleet" && v.Bool(carrier["owned"]) {
		if crew, known := v.Number(v.At(carrier, "space", "crewTons")); known {
			return crew + v.Num(v.At(carrier, "space", "shipPacksTons")) + v.Num(v.At(carrier, "space", "modulePacksTons")), false
		}
	}
	return 0, true
}

// TradeOpportunityContext is also the cache key. Moving the carrier or
// changing service space or the plan's constraints changes it.
func TradeOpportunityContext(trade, state M, request M) M {
	options := v.Obj(trade["options"])
	owned := v.Obj(state["carrier"])
	commander := v.Obj(state["commander"])
	location := v.Obj(owned["location"])
	system := tradeText(v.First(request["system"], location["system"], commander["system"]), 150)
	if options["carrierType"] == "squadron" && v.Str(request["system"]) == "" {
		system = tradeText(commander["system"], 150)
	}
	services, estimated := tradeServices(trade, owned)
	out := M{
		"scenario": TradeOpportunityScenario, "system": system, "carrierType": options["carrierType"],
		"commanderSystem": commander["system"], "shipPad": options["haulerPad"],
		"shipModel":    v.First(commander["shipModel"], commander["shipType"], commander["ship"]),
		"shipCapacity": commander["cargoCapacity"],
		"credits":      v.First(request["credits"], v.At(state, "pilot", "credits")),
		"servicesTons": services, "estimatedServices": estimated,
	}
	for _, key := range []string{"tritiumPrice", "weeklyUpkeep", "durationHours", "extraCosts", "haulerCapacity",
		"unloadHaulerPad", "unloadHaulerCapacity", "maxQuoteAgeHours", "demandRatio", "holidayConfirmed"} {
		out[key] = options[key]
	}
	if options["carrierType"] == "squadron" {
		out["manualFuelTons"] = squadronTradeFuel(trade)
	}
	return out
}

func squadronTradeFuel(trade M) any {
	legs := v.Rows(trade["legs"])
	if len(legs) != 2 || v.At(trade, "options", "preset") != tradePeakScenario {
		return nil
	}
	out, outKnown := v.Number(legs[0]["tritiumTons"])
	back, backKnown := v.Number(legs[1]["tritiumTons"])
	if !outKnown || !backKnown || out <= 0 || back <= 0 {
		return nil
	}
	return []float64{out, back}
}

func observedHauler(commander M) (string, float64) {
	if v.Str(commander["vehicle"]) != "ship" && v.Str(commander["vehicle"]) != "" {
		return "", 0
	}
	model := v.Key(v.First(commander["shipModel"], commander["shipType"], commander["ship"]))
	if model == "" {
		return "", 0
	}
	for _, item := range data.Array("encyclopedia-ships.json") {
		for _, field := range []string{"name", "key", "symbol"} {
			if model == v.Key(item[field]) {
				capacity := v.Num(commander["cargoCapacity"])
				if capacity <= 0 && v.Key(item["name"]) == "pantherclippermkii" {
					capacity = 1300
				}
				return v.Str(item["pad"]), capacity
			}
		}
	}
	return "", 0
}

func newTradeOpportunityInputs(trade, state, request M) (tradeOpportunityInputs, error) {
	key := TradeOpportunityContext(trade, state, request)
	options := v.Copy(v.Obj(trade["options"]))
	kind, ok := calculatorCatalog(v.Str(options["carrierType"]))
	plan := tradeOpportunityInputs{options: options, state: state, system: v.Str(key["system"]),
		capacity: float64(kind.Capacity), services: v.Num(key["servicesTons"]), estimatedCarrier: v.Bool(key["estimatedServices"])}
	if !ok || plan.system == "" {
		return plan, errors.New("Укажите систему отправления флотоносца.")
	}
	budget, known := v.Number(key["credits"])
	if !known || budget <= 0 {
		return plan, errors.New("Нужен известный баланс или бюджет на закупку.")
	}
	plan.budget = budget
	plan.manualFuel, _ = key["manualFuelTons"].([]float64)
	if kind.Key == "squadron" && len(plan.manualFuel) == 0 {
		return plan, errors.New("Для флотоносца эскадрильи укажите расход трития туда и обратно в плане рейса.")
	}
	plan.options["servicesTons"] = plan.services
	plan.options["otherCargoTons"] = 0
	plan.options["returnTrip"] = true
	plan.options["plannedFullLoad"] = true
	plan.options["preset"] = "custom"
	plan.options["holidayConfirmed"] = false
	observedPad, observedCapacity := observedHauler(v.Obj(state["commander"]))
	shipPad := v.Text(plan.options["haulerPad"], observedPad)
	shipCapacity := v.Num(plan.options["haulerCapacity"])
	if shipCapacity == 0 && (shipPad == "" || shipPad == observedPad) {
		shipCapacity = observedCapacity
	}
	if shipPad == "" {
		shipPad = "L"
	}
	if shipCapacity <= 0 {
		shipCapacity = 280
	}
	plan.options["haulerPad"], plan.options["haulerCapacity"] = shipPad, shipCapacity
	plan.options["unloadHaulerPad"] = v.Text(plan.options["unloadHaulerPad"], shipPad)
	if v.Num(plan.options["unloadHaulerCapacity"]) <= 0 {
		plan.options["unloadHaulerCapacity"] = shipCapacity
	}
	return plan, nil
}

func (plan tradeOpportunityInputs) forPeak(trade M) tradeOpportunityInputs {
	plan.options = v.Copy(plan.options)
	plan.options["preset"] = tradePeakScenario
	plan.options["holidayConfirmed"] = v.At(trade, "options", "preset") == tradePeakScenario &&
		v.Bool(v.At(trade, "options", "holidayConfirmed"))
	if plan.options["unloadHaulerPad"] == "L" || v.At(trade, "options", "unloadHaulerPad") == nil ||
		v.Str(v.At(trade, "options", "unloadHaulerPad")) == "" {
		plan.options["unloadHaulerPad"] = "M"
	}
	if v.Num(v.At(trade, "options", "unloadHaulerCapacity")) <= 0 || v.At(trade, "options", "unloadHaulerPad") == "L" {
		plan.options["unloadHaulerCapacity"] = 400
	}
	return plan
}

func opportunitySearchState(state, origin M) M {
	copy := v.Copy(state)
	copy["world"] = []M{{"name": origin["system"], "coordinates": origin["coordinates"], "address": origin["systemAddress"]}}
	return copy
}

func knownOpportunityOrigin(state M, system string) M {
	if known := knownTradePeakOrigin(system); known != nil {
		return known
	}
	for _, candidate := range []M{v.Obj(v.At(state, "carrier", "location")), v.Obj(state["commander"])} {
		if strings.EqualFold(v.Str(candidate["system"]), system) && len(trading.Coords(candidate["coordinates"])) == 3 {
			return candidate
		}
	}
	return nil
}

func tradeDirectHop(from, to M) ([]tradeHop, bool) {
	if strings.EqualFold(v.Str(from["system"]), v.Str(to["system"])) {
		return nil, true
	}
	distance := trading.Distance(from["coordinates"], to["coordinates"])
	if math.IsNaN(distance) || distance > 500 {
		return nil, false
	}
	return []tradeHop{{v.Str(from["system"]), v.Str(to["system"]), distance}}, true
}

func reverseTradeHops(hops []tradeHop) []tradeHop {
	out := make([]tradeHop, len(hops))
	for index, hop := range hops {
		out[len(hops)-index-1] = tradeHop{hop.to, hop.from, hop.distance}
	}
	return out
}

type tradeFuelPlan struct {
	tank, hold, acquired, reserve float64
	outbound, inward              float64
	valid                         bool
}

func tradeFuelConsumed(hops []tradeHop, usedCargo, remaining float64, pickups map[string]float64) float64 {
	total := float64(0)
	picked := map[string]bool{}
	for _, hop := range hops {
		if len(pickups) != 0 {
			system := v.Key(hop.from)
			if !picked[system] {
				usedCargo += pickups[system]
				picked[system] = true
			}
		}
		// Matches Spansh's per-hop rounding for the 25,000 t Drake-class dry
		// mass. Tank and cargo tritium both contribute to mass. Fuel already
		// burned is removed before the next jump.
		cost := math.Round(5 + hop.distance*(25000+usedCargo+math.Max(0, remaining-total))/200000)
		total += cost
	}
	return total
}

func (plan tradeOpportunityInputs) fuelForCargo(cargo float64, reposition, loaded, back []tradeHop, offers []M) tradeFuelPlan {
	provision := func(out, in float64) tradeFuelPlan {
		reserve := math.Ceil((out + in) / 10)
		start := out + in + reserve
		tank := math.Min(TankCapacity, start)
		hold := start - tank
		return tradeFuelPlan{tank: tank, hold: hold, acquired: start, reserve: reserve, outbound: out, inward: in,
			valid: plan.services+hold+cargo <= plan.capacity}
	}
	if len(plan.manualFuel) == 2 {
		return provision(plan.manualFuel[0], plan.manualFuel[1])
	}
	pickups := map[string]float64{}
	remaining := cargo
	lastSystem := ""
	for _, offer := range offers {
		lastSystem = v.Key(offer["system"])
		tons := math.Min(remaining, math.Floor(v.Num(offer["supply"])))
		pickups[lastSystem] += tons
		remaining -= tons
	}
	// Physical capacity is independent of market stock. The search rejects
	// insufficient stock after calculating the full load.
	pickups[lastSystem] += remaining
	start := float64(0)
	for range 24 {
		empty := tradeFuelConsumed(reposition, plan.services, start, nil)
		out := empty + tradeFuelConsumed(loaded, plan.services, start-empty, pickups)
		in := tradeFuelConsumed(back, plan.services, start-out, nil)
		fuel := provision(out, in)
		if fuel.acquired == start {
			return fuel
		}
		start = fuel.acquired
	}
	return tradeFuelPlan{}
}

func marketInput(s trading.Searcher, ctx context.Context, state M, system, key, side, pad string, radius, age, min float64) (M, error) {
	return s.Search(ctx, M{"system": system, "resourceKey": key, "mode": side, "minPad": pad,
		"useCargo": false, "includeCarriers": false, "radiusLy": radius, "maxAgeHours": age,
		"minSupply": min, "minDemand": min, "maxArrivalLs": 10000}, state)
}

func pickTradeQuote(market M, key string, now time.Time, age float64) M {
	quote, _, err := tradeMarket(market, key)
	if err != nil || v.Num(quote["price"]) <= 0 {
		return nil
	}
	at := v.Time(quote["updatedAt"])
	if at.IsZero() || at.After(now.Add(10*time.Minute)) || now.Sub(at).Hours() > age {
		return nil
	}
	return quote
}

func peakTradeQuotes(ctx context.Context, searcher trading.Searcher, now time.Time, age float64) (map[string]M, error) {
	result, err := trading.FetchJSON(ctx, searcher.Client, http.MethodGet, searcher.BaseURL+"/station/"+tradePeakMarket, nil)
	if err != nil {
		return nil, err
	}
	record := v.Obj(v.At(v.Obj(result), "record"))
	if v.Str(record["market_id"]) != tradePeakMarket ||
		!tradeRackham(M{"system": record["system_name"], "station": record["name"]}) {
		return nil, errors.New("Не удалось подтвердить рынок Rackham's Peak.")
	}
	quotes := map[string]M{}
	timestamp := v.Str(record["market_updated_at"])
	for _, entry := range v.Rows(record["market"]) {
		resource := trading.Good(v.Key(v.First(entry["symbol"], entry["commodity"], entry["name"])), nil)
		key := v.Str(resource["key"])
		if key == "" {
			continue
		}
		resource["quantity"] = 1
		quote := trading.Quote(entry, resource, "sell", timestamp, now)
		if quote == nil {
			continue
		}
		picked := pickTradeQuote(M{"station": record["name"], "system": record["system_name"], "pad": "M",
			"marketId": tradePeakMarket, "source": "Spansh", "sourceUrl": "https://spansh.co.uk/station/" + tradePeakMarket,
			"updatedAt": timestamp, "distanceLs": record["distance_to_arrival"], "quotes": []M{quote}}, key, now, age)
		if picked != nil {
			quotes[key] = picked
		}
	}
	return quotes, nil
}

func timeForTradeHops(count int) float64 {
	options := DefaultRouteOptions()
	return (float64(count)*(v.Num(options["jumpMinutes"])+v.Num(options["overheadMinutes"])) +
		float64(max(0, count-1))*v.Num(options["cooldownMinutes"])) / 60
}

func tradeHopRows(groups ...[]tradeHop) []M {
	var rows []M
	for _, group := range groups {
		for _, hop := range group {
			rows = append(rows, M{"from": hop.from, "to": hop.to, "distanceLy": hop.distance})
		}
	}
	return rows
}

// buyTradeCargo fills a quantity from offers ordered by the pickup route.
// The selected offers retain their station, date and quantity.
func buyTradeCargo(offers []M, quantity float64) M {
	if len(offers) == 0 || quantity < 1 {
		return nil
	}
	buy := v.Copy(offers[0])
	remaining, cost := quantity, 0.0
	orders := []M{}
	for _, offer := range offers {
		tons := math.Min(remaining, math.Floor(v.Num(offer["supply"])))
		if tons < 1 {
			continue
		}
		cost += tons * v.Num(offer["price"])
		remaining -= tons
		orders = append(orders, v.Merge(v.Copy(offer), M{"quantity": tons}))
		if v.Time(offer["updatedAt"]).Before(v.Time(buy["updatedAt"])) {
			buy["updatedAt"] = offer["updatedAt"]
		}
		if remaining == 0 {
			break
		}
	}
	if remaining > 0 {
		return nil
	}
	buy["price"], buy["purchaseTotal"] = cost/quantity, cost
	if len(orders) > 1 {
		buy["orders"], buy["supply"] = orders, quantity
	}
	return buy
}

func tradePadFits(available, required string) bool {
	if required == "" {
		return true
	}
	return strings.Index("SML", available) >= strings.Index("SML", required) && len(available) == 1
}

func fuelStation(fuelMarkets []M, quantity float64, pad string) M {
	for _, quote := range fuelMarkets {
		if v.Num(quote["supply"]) >= quantity && tradePadFits(v.Str(quote["pad"]), pad) {
			return quote
		}
	}
	return nil
}

func (plan tradeOpportunityInputs) candidate(key string, buyOffers []M, sell M, supply, limit float64, reposition, loaded, back []tradeHop, fuelMarkets []M, now time.Time) M {
	if len(buyOffers) == 0 || v.Num(buyOffers[0]["price"]) <= 0 || v.Num(sell["price"]) <= 0 {
		return nil
	}
	// First find the physical capacity for this route. Market volumes and the
	// budget must cover that entire load, so they never shrink the plan.
	low, high := 0.0, math.Floor(plan.capacity-plan.services)
	if plan.fuelForCargo(high, reposition, loaded, back, buyOffers).valid {
		low = high
	}
	for low < high {
		mid := math.Floor((low + high + 1) / 2)
		fuel := plan.fuelForCargo(mid, reposition, loaded, back, buyOffers)
		if fuel.valid {
			low = mid
		} else {
			high = mid - 1
		}
	}
	unloadCapacity := v.Num(v.First(plan.options["unloadHaulerCapacity"], plan.options["haulerCapacity"]))
	saleHaul := math.Min(low, unloadCapacity)
	requiredDemand := low + (v.Num(plan.options["demandRatio"])-1)*saleHaul
	if low < 1 || low > limit || low > supply || requiredDemand > v.Num(sell["demand"]) {
		return nil
	}
	buy := buyTradeCargo(buyOffers, low)
	if buy == nil || low*v.Num(sell["price"])-v.Num(buy["purchaseTotal"]) < netOpportunityMinimum {
		return nil
	}
	fuel := plan.fuelForCargo(low, reposition, loaded, back, buyOffers)
	fuelMarket := fuelStation(fuelMarkets, fuel.acquired, v.Str(plan.options["haulerPad"]))
	fuelPrice := v.First(fuelMarket["price"], plan.options["tritiumPrice"])
	row := plan.makeCandidate(key, buy, sell, low, supply, reposition, loaded, back, fuel, fuelPrice, fuelMarket, now)
	summary := v.Obj(row["summary"])
	profit, known := v.Number(summary["profit"])
	capital, funded := v.Number(summary["workingCapital"])
	if !known || profit < netOpportunityMinimum || !funded || capital > plan.budget ||
		low != math.Floor(v.Num(summary["availableCargoTons"])) || summary["fuelFits"] != true {
		return nil
	}
	return row
}

func tradeLoadingSorties(buy M, cargo, haulerCapacity float64) float64 {
	if orders := v.Rows(buy["orders"]); len(orders) > 1 {
		var total, loadSorties float64
		for _, order := range orders {
			quantity := v.Num(order["quantity"])
			total += quantity
			loadSorties += math.Ceil(quantity / haulerCapacity)
		}
		if total == cargo {
			return loadSorties
		}
	}
	return math.Ceil(cargo / haulerCapacity)
}

func (plan tradeOpportunityInputs) makeCandidate(key string, buy, sell M, quantity, supply float64, reposition, loaded, back []tradeHop, fuel tradeFuelPlan, fuelPrice any, fuelMarket M, now time.Time) M {
	if quantity < 1 {
		return nil
	}
	options := v.Copy(plan.options)
	options["tankTons"], options["cargoTritiumTons"], options["tritiumPrice"] = fuel.tank, fuel.hold, fuelPrice
	options["arrivalReserveTons"] = fuel.reserve
	_, legs := tradeDefaults()
	source, destination := v.Str(buy["system"]), v.Str(sell["system"])
	outbound := v.Obj(legs["outbound"])
	outbound["commodityKey"], outbound["quantity"] = key, quantity
	outbound["sourceSystem"], outbound["destinationSystem"] = source, destination
	outbound["buyMarket"], outbound["sellMarket"], outbound["buyPrice"], outbound["sellPrice"] = buy, sell, buy["price"], sell["price"]
	haulerCapacity := v.Num(options["haulerCapacity"])
	loadingSorties := tradeLoadingSorties(buy, quantity, haulerCapacity)
	unloadingSorties := math.Ceil(quantity / v.Num(v.First(options["unloadHaulerCapacity"], haulerCapacity)))
	fuelLoadingHours := math.Ceil(fuel.acquired/haulerCapacity) * tradeShuttleMinutes / 60
	outbound["loadingHours"] = loadingSorties * tradeShuttleMinutes / 60
	outbound["unloadingHours"] = unloadingSorties * tradeShuttleMinutes / 60
	outwardJumps := len(reposition) + len(loaded)
	outbound["jumps"], outbound["travelHours"] = outwardJumps, timeForTradeHops(outwardJumps)
	backLeg := v.Obj(legs["return"])
	backLeg["sourceSystem"], backLeg["destinationSystem"] = destination, plan.system
	backLeg["jumps"], backLeg["travelHours"] = len(back), timeForTradeHops(len(back))
	outbound["tritiumTons"], backLeg["tritiumTons"] = fuel.outbound, fuel.inward
	fuelPurchaseCost := tradeTimes(v.First(fuelMarket["price"], fuelPrice), fuel.acquired)
	outbound["route"] = M{"sourceSystem": source, "destinationSystem": destination, "carrierType": options["carrierType"],
		"quantity": quantity, "servicesTons": options["servicesTons"], "otherCargoTons": options["otherCargoTons"],
		"positioningSource": plan.system, "reposition": tradeHopRows(reposition), "loaded": tradeHopRows(loaded),
		"fuelPurchaseTons": fuel.acquired, "fuelPurchaseCost": fuelPurchaseCost, "fuelMarket": fuelMarket,
		"fuelLoadingHours": fuelLoadingHours}
	backLeg["route"] = M{"sourceSystem": destination, "destinationSystem": plan.system, "carrierType": options["carrierType"],
		"quantity": 0, "servicesTons": options["servicesTons"], "otherCargoTons": options["otherCargoTons"], "return": tradeHopRows(back)}
	result := tradeSnapshot(options, legs, now)
	result["purchaseCost"], result["supply"], result["quantity"] = buy["purchaseTotal"], supply, quantity
	result["commodityKey"], result["commodityName"] = key, tradeCommodity(key)
	result["buy"], result["sell"], result["fuelMarket"] = buy, sell, fuelMarket
	result["fuelPurchaseTons"] = fuel.acquired
	result["fuelPurchaseCost"] = fuelPurchaseCost
	result["route"] = M{"origin": plan.system, "reposition": tradeHopRows(reposition), "loaded": tradeHopRows(loaded), "return": tradeHopRows(back)}
	result["estimatedCarrier"] = plan.estimatedCarrier
	result["shuttleMinutes"] = tradeShuttleMinutes
	result["id"] = strings.Join([]string{key, v.Str(buy["system"]), v.Str(buy["station"]), v.Str(sell["system"]), v.Str(sell["station"])}, "|")
	if len(v.Rows(buy["orders"])) > 1 {
		for _, order := range v.Rows(buy["orders"]) {
			result["id"] = v.Str(result["id"]) + "|" + v.Str(order["system"]) + "|" + v.Str(order["station"])
		}
	}
	return result
}

func (plan tradeOpportunityInputs) peakCandidates(key string, markets []M, sell M, corridor []tradeHop, fuelMarkets []M, now time.Time) []M {
	bySystem := map[string]*tradeSupplySystem{}
	for _, raw := range markets {
		buy := pickTradeQuote(raw, key, now, v.Num(plan.options["maxQuoteAgeHours"]))
		if buy == nil {
			continue
		}
		system := v.Key(buy["system"])
		if bySystem[system] == nil {
			bySystem[system] = &tradeSupplySystem{market: raw}
		}
		bySystem[system].offers = append(bySystem[system].offers, buy)
	}
	rows := []M{}
	back := reverseTradeHops(corridor)
	add := func(markets []M, buyOffers []M, combined bool) {
		reposition, reachable := tradeDirectHop(plan.origin, markets[0])
		if !reachable {
			return
		}
		last := markets[len(markets)-1]
		loaded, reachable := localTradePickupHops(tradeSupplyPath{markets: markets}, last)
		if !reachable {
			return
		}
		toOrigin, reachable := tradeDirectHop(last, plan.origin)
		if !reachable {
			return
		}
		tail := append(toOrigin, corridor...)
		if direct, ok := directTradePeakCorridor(last); ok && len(direct) <= len(tail) {
			tail = direct
		}
		loaded = append(loaded, tail...)
		var supply float64
		for _, quote := range buyOffers {
			supply += v.Num(quote["supply"])
		}
		limit := math.Min(supply, v.Num(sell["demand"]))
		row := plan.candidate(key, buyOffers, sell, supply, limit, reposition, loaded, back, fuelMarkets, now)
		if row != nil && (!combined || len(v.Rows(v.At(row, "buy", "orders"))) > 1) &&
			(len(markets) == 1 || len(tradeOrderSystems(v.Obj(row["buy"]))) == len(markets)) {
			rows = append(rows, row)
		}
	}
	suppliers := make([]tradeSupplySystem, 0, len(bySystem))
	for _, supply := range bySystem {
		sort.SliceStable(supply.offers, func(i, j int) bool { return v.Num(supply.offers[i]["price"]) < v.Num(supply.offers[j]["price"]) })
		for _, quote := range supply.offers {
			add([]M{supply.market}, []M{quote}, false)
		}
		if len(supply.offers) > 1 {
			add([]M{supply.market}, supply.offers, true)
		}
		suppliers = append(suppliers, *supply)
	}
	ceiling := math.Floor(plan.capacity - plan.services)
	minimum := math.Max(1, math.Floor(ceiling*.70))
	if len(plan.manualFuel) == 2 {
		minimum = math.Floor(ceiling - plan.fuelForCargo(0, nil, nil, nil, nil).hold)
	}
	for _, path := range tradeSupplyPaths(suppliers, sell, minimum, ceiling) {
		add(path.markets, path.buyOffers, true)
	}
	return rows
}

type peakMarketResult struct {
	sales            map[string]M
	purchases        map[string][]M
	hops             []tradeHop
	limited, partial bool
}

func searchPeakTradeMarkets(ctx context.Context, searcher trading.Searcher, route *RoutePlanner, plan tradeOpportunityInputs, now time.Time) peakMarketResult {
	age := v.Num(plan.options["maxQuoteAgeHours"])
	sales, err := peakTradeQuotes(ctx, searcher, now, age)
	if err != nil {
		return peakMarketResult{partial: true}
	}
	result := peakMarketResult{sales: sales, purchases: map[string][]M{}}
	capacity := plan.capacity - plan.services
	maxRevenue := func(key string) float64 {
		quote := sales[key]
		return math.Min(capacity, v.Num(quote["demand"])) * v.Num(quote["price"])
	}
	var keys []string
	for key := range sales {
		if maxRevenue(key) >= netOpportunityMinimum {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return result
	}
	sort.Slice(keys, func(i, j int) bool {
		if a, b := maxRevenue(keys[i]), maxRevenue(keys[j]); a != b {
			return a > b
		}
		return keys[i] < keys[j]
	})
	const maxPeakCommodities = 6
	if len(keys) > maxPeakCommodities {
		result.limited = true
		keys = keys[:maxPeakCommodities]
	}
	result.hops, err = tradePeakCorridor(ctx, route, plan.origin)
	if err != nil {
		result.partial = true
		return result
	}
	type purchase struct {
		key  string
		data M
		err  error
	}
	purchases := make(chan purchase, len(keys))
	pad := v.Str(plan.options["haulerPad"])
	minimum := math.Max(1, math.Floor(capacity/20))
	for _, key := range keys {
		go func() {
			found, searchErr := marketInput(searcher, ctx, plan.state, plan.system, key, "buy", pad, 100, age, minimum)
			purchases <- purchase{key, found, searchErr}
		}()
	}
	for range keys {
		buy := <-purchases
		result.partial = result.partial || buy.err != nil || v.Str(buy.data["error"]) != ""
		result.limited = result.limited || v.Bool(buy.data["coverageLimited"])
		result.purchases[buy.key] = v.Rows(buy.data["results"])
	}
	return result
}

// FindTradeOpportunities compares regional markets and the Rackham's Peak corridor.
func FindTradeOpportunities(ctx context.Context, client trading.HTTPDoer, trade, state, request M, update func(M)) (M, error) {
	now := time.Now()
	request = v.Merge(M{}, request, M{"scenario": TradeOpportunityScenario})
	plan, err := newTradeOpportunityInputs(trade, state, request)
	out := M{"options": request, "context": TradeOpportunityContext(trade, state, request), "results": []M{}, "updatedAt": v.ISO(now)}
	if err != nil {
		out["status"] = err.Error()
		return out, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 95*time.Second)
	defer cancel()
	searcher := *trading.NewSearcher(client)
	route := NewRoutePlanner(nil)
	if client != nil {
		route.Client = client
	}
	route.Timeout = 65 * time.Second
	plan.origin = knownOpportunityOrigin(state, plan.system)
	if plan.origin == nil {
		resolved, resolveErr := route.Resolve(ctx, plan.system)
		if resolveErr != nil {
			return out, resolveErr
		}
		plan.origin = M{"system": resolved["name"], "coordinates": resolved["coordinates"], "systemAddress": resolved["id64"]}
	}
	plan.state = opportunitySearchState(state, plan.origin)
	age := v.Num(plan.options["maxQuoteAgeHours"])
	pad := v.Str(plan.options["haulerPad"])
	out["stage"] = "markets"
	if update != nil {
		update(v.Copy(out))
	}
	peakPlan := plan.forPeak(trade)
	peak := make(chan peakMarketResult, 1)
	go func() { peak <- searchPeakTradeMarkets(ctx, searcher, route, peakPlan, now) }()
	type regionalResult struct {
		markets map[string]bulkTradeMarkets
		limited bool
		err     error
	}
	regional := make(chan regionalResult, 1)
	go func() {
		if len(plan.manualFuel) != 0 {
			regional <- regionalResult{}
			return
		}
		bulkPad := v.Text(plan.options["unloadHaulerPad"], pad)
		if tradePadFits(bulkPad, pad) {
			bulkPad = pad
		}
		markets, limited, searchErr := findBulkTradeMarkets(ctx, searcher, plan.origin, bulkPad, age, now)
		regional <- regionalResult{markets, limited, searchErr}
	}()
	fuel, fuelErr := marketInput(searcher, ctx, plan.state, plan.system, "tritium", "buy", pad, 100, age, 1)
	fuelMarkets := []M{}
	for _, raw := range v.Rows(fuel["results"]) {
		quote := pickTradeQuote(raw, "tritium", now, age)
		if quote != nil {
			fuelMarkets = append(fuelMarkets, quote)
		}
	}
	sort.SliceStable(fuelMarkets, func(i, j int) bool { return v.Num(fuelMarkets[i]["price"]) < v.Num(fuelMarkets[j]["price"]) })
	local, corridor := <-regional, <-peak
	out["partial"] = local.err != nil || corridor.partial || fuelErr != nil || v.Str(fuel["error"]) != ""
	out["limited"] = local.limited || corridor.limited || v.Bool(fuel["coverageLimited"])
	if err := ctx.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return out, err
	}
	out["stage"] = "routes"
	if update != nil {
		update(v.Copy(out))
	}
	rows := plan.localCandidates(local.markets, fuelMarkets, now)
	for key, purchases := range corridor.purchases {
		rows = append(rows, peakPlan.peakCandidates(key, purchases, corridor.sales[key], corridor.hops, fuelMarkets, now)...)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a := v.Num(v.At(rows[i], "summary", "profit"))
		b := v.Num(v.At(rows[j], "summary", "profit"))
		if a != b {
			return a > b
		}
		if v.Num(rows[i]["purchaseCost"]) != v.Num(rows[j]["purchaseCost"]) {
			return v.Num(rows[i]["purchaseCost"]) < v.Num(rows[j]["purchaseCost"])
		}
		return v.Str(rows[i]["id"]) < v.Str(rows[j]["id"])
	})
	if len(rows) > 8 {
		rows = rows[:8]
	}
	out["results"], out["updatedAt"] = rows, v.ISO(time.Now())
	if len(rows) == 0 {
		out["status"] = "Не найдено рейсов с полным трюмом и чистой прибылью от 400 млн кр."
	}
	return out, nil
}
