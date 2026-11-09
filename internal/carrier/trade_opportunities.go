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
	tradePeakSystem = "HIP 58832"
	tradePeakPort   = "Rackham's Peak"
	tradePeakMarket = "128968675"
	// Allow time to load and sell each shuttle's cargo. The route has separate
	// flight time. Both are estimates and can be edited in the imported plan.
	tradeShuttleMinutes = 10
)

type tradeOpportunityInputs struct {
	options, state, origin M
	scenario, system       string
	baseCargo, tank, hold  float64
	capacity, budget       float64
	estimatedCarrier       bool
}

type tradeHop struct {
	from, to string
	distance float64
}

func tradeStockFresh(raw any, now time.Time) bool {
	at := v.Time(raw)
	return !at.IsZero() && !at.After(now.Add(10*time.Minute)) && now.Sub(at) <= 24*time.Hour
}

// TradeOpportunityContext is also the cache key. Moving the carrier or
// changing its available space, fuel or the plan's constraints changes it.
func TradeOpportunityContext(trade, state M, request M) M {
	now := time.Now()
	options := v.Obj(trade["options"])
	owned := v.Obj(state["carrier"])
	commander := v.Obj(state["commander"])
	scenario := v.Text(request["scenario"], "custom")
	if scenario != "booze-cruise" {
		scenario = "custom"
	}
	location := v.Obj(owned["location"])
	system := tradeText(v.First(request["system"], location["system"], commander["system"]), 150)
	if options["carrierType"] == "squadron" && v.Str(request["system"]) == "" {
		system = tradeText(commander["system"], 150)
	}
	out := M{
		"scenario": scenario, "system": system, "carrierType": options["carrierType"],
		"commanderSystem": commander["system"], "shipPad": options["haulerPad"],
		"shipModel":    v.First(commander["shipModel"], commander["shipType"], commander["ship"]),
		"shipCapacity": commander["cargoCapacity"],
		"credits":      v.First(request["credits"], v.At(state, "pilot", "credits")),
		"stock": M{"owned": owned["owned"], "capacity": v.At(owned, "space", "capacityTons"),
			"free": v.At(owned, "space", "freeTons"), "tank": v.At(owned, "fuel", "tankTons"),
			"hold": v.At(owned, "fuel", "reserveTons"), "needsRefresh": v.At(owned, "fuel", "needsRefresh"),
			"spaceFresh": tradeStockFresh(v.At(owned, "space", "updatedAt"), now), "estimated": v.At(owned, "space", "estimated"),
			"warehouseFresh": tradeStockFresh(v.At(owned, "warehouse", "updatedAt"), now),
			"tankFresh":      tradeStockFresh(v.At(owned, "fuel", "updatedAt"), now)},
	}
	for _, key := range []string{"servicesTons", "otherCargoTons", "tankTons", "cargoTritiumTons", "arrivalReserveTons",
		"tritiumPrice", "weeklyUpkeep", "durationHours", "extraCosts", "haulerCapacity", "maxQuoteAgeHours", "demandRatio", "holidayConfirmed"} {
		out[key] = options[key]
	}
	return out
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
				return v.Str(item["pad"]), v.Num(commander["cargoCapacity"])
			}
		}
	}
	return "", 0
}

func newTradeOpportunityInputs(trade, state, request M) (tradeOpportunityInputs, error) {
	key := TradeOpportunityContext(trade, state, request)
	options := v.Copy(v.Obj(trade["options"]))
	kind, ok := calculatorCatalog(v.Str(options["carrierType"]))
	plan := tradeOpportunityInputs{options: options, state: state, scenario: v.Str(key["scenario"]), system: v.Str(key["system"]),
		capacity: float64(kind.Capacity), baseCargo: v.Num(options["servicesTons"]) + v.Num(options["otherCargoTons"]),
		tank: v.Num(options["tankTons"]), hold: v.Num(options["cargoTritiumTons"]), estimatedCarrier: true}
	if !ok || plan.system == "" {
		return plan, errors.New("Укажите систему отправления флотоносца.")
	}
	budget, known := v.Number(key["credits"])
	if !known || budget <= 0 {
		return plan, errors.New("Нужен известный баланс или бюджет на закупку.")
	}
	plan.budget = budget
	stock := v.Obj(key["stock"])
	if kind.Key == "fleet" && v.Bool(stock["owned"]) {
		if free, freeKnown := v.Number(stock["free"]); freeKnown && free >= 0 && free <= plan.capacity {
			occupied := plan.capacity - free
			if hold, holdKnown := v.Number(stock["hold"]); holdKnown && hold >= 0 && hold <= occupied && v.Bool(stock["warehouseFresh"]) {
				plan.hold = hold
			} else {
				plan.hold = 0
			}
			plan.baseCargo = occupied - plan.hold
			plan.estimatedCarrier = !v.Bool(stock["spaceFresh"]) || !v.Bool(stock["warehouseFresh"]) || v.Bool(stock["estimated"])
		}
		if tank, tankKnown := v.Number(stock["tank"]); tankKnown && !v.Bool(stock["needsRefresh"]) && v.Bool(stock["tankFresh"]) {
			plan.tank = tank
		} else {
			plan.tank = 0
			plan.estimatedCarrier = true
		}
	}
	plan.options["servicesTons"] = math.Min(plan.baseCargo, v.Num(plan.options["servicesTons"]))
	plan.options["otherCargoTons"] = math.Max(0, plan.baseCargo-v.Num(plan.options["servicesTons"]))
	plan.options["tankTons"] = plan.tank
	plan.options["returnTrip"] = true
	plan.options["preset"] = plan.scenario
	if v.At(trade, "options", "preset") != plan.scenario {
		plan.options["holidayConfirmed"] = false
	}
	observedPad, observedCapacity := observedHauler(v.Obj(state["commander"]))
	shipPad := v.Text(plan.options["haulerPad"], observedPad)
	shipCapacity := v.Num(plan.options["haulerCapacity"])
	if shipCapacity == 0 && (shipPad == "" || shipPad == observedPad) {
		shipCapacity = observedCapacity
	}
	if plan.scenario == "booze-cruise" && (shipPad == "L" || shipPad == "") {
		shipPad, shipCapacity = "M", 280
	}
	if shipPad == "" {
		shipPad = "L" // only large-pad stations when the ship type is unknown
	}
	if shipCapacity <= 0 {
		shipCapacity = 280
	}
	plan.options["haulerPad"], plan.options["haulerCapacity"] = shipPad, shipCapacity
	return plan, nil
}

func opportunitySearchState(state, origin M) M {
	copy := v.Copy(state)
	copy["world"] = []M{{"name": origin["system"], "coordinates": origin["coordinates"], "address": origin["systemAddress"]}}
	return copy
}

func knownOpportunityOrigin(state M, system string) M {
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

func tradePeakHops(ctx context.Context, route *RoutePlanner, source string) ([]tradeHop, error) {
	if strings.EqualFold(source, tradePeakSystem) {
		return nil, nil
	}
	result, err := route.Search(ctx, M{"sourceSystem": source, "destinationSystem": tradePeakSystem,
		"usedCapacityTons": 0, "cargoTritiumTons": 0, "reserveTritiumTons": 0, "calculateStartingFuel": true}, nil)
	if err != nil {
		return nil, err
	}
	stops := v.Rows(v.At(result, "route", "stops"))
	hops := make([]tradeHop, 0, len(stops)-1)
	for i := 1; i < len(stops); i++ {
		hops = append(hops, tradeHop{v.Str(stops[i-1]["name"]), v.Str(stops[i]["name"]), v.Num(stops[i]["distanceLy"])})
	}
	return hops, nil
}

type tradeFuelPlan struct {
	tank, hold, acquired float64
	outbound, inward     float64
	valid                bool
}

func tradeFuelConsumed(hops []tradeHop, usedCargo, remaining float64) float64 {
	total := float64(0)
	for _, hop := range hops {
		// Matches Spansh's per-hop rounding for the 25,000 t Drake-class dry
		// mass. Tank and cargo tritium both contribute to mass. Fuel already
		// burned is removed before the next jump.
		cost := math.Round(5 + hop.distance*(25000+usedCargo+math.Max(0, remaining-total))/200000)
		total += cost
	}
	return total
}

func (plan tradeOpportunityInputs) fuelForCargo(cargo float64, reposition, loaded, back []tradeHop) tradeFuelPlan {
	initial := plan.tank + plan.hold
	reserve := v.Num(plan.options["arrivalReserveTons"])
	start := initial
	for range 12 {
		empty := tradeFuelConsumed(reposition, plan.baseCargo, start)
		out := empty + tradeFuelConsumed(loaded, plan.baseCargo+cargo, start-empty)
		in := tradeFuelConsumed(back, plan.baseCargo, start-out)
		required := out + in + reserve
		if start >= required {
			// Fill the depot before loading cargo. Transfers from the hold
			// leave the total tritium mass unchanged.
			acquired := math.Max(0, start-initial)
			tank := math.Min(TankCapacity, start)
			hold := start - tank
			return tradeFuelPlan{tank: tank, hold: hold, acquired: acquired, outbound: out, inward: in,
				valid: plan.baseCargo+hold+cargo <= plan.capacity}
		}
		start = required
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
	for _, key := range []string{"wine", "liquor"} {
		resource := v.Merge(trading.Good(key, nil), M{"quantity": 1})
		for _, entry := range v.Rows(record["market"]) {
			quote := trading.Quote(entry, resource, "sell", timestamp, now)
			if quote == nil {
				continue
			}
			quotes[key] = pickTradeQuote(M{"station": record["name"], "system": record["system_name"], "pad": "M",
				"marketId": tradePeakMarket, "source": "Spansh", "sourceUrl": "https://spansh.co.uk/station/" + tradePeakMarket,
				"updatedAt": timestamp, "distanceLs": record["distance_to_arrival"], "quotes": []M{quote}}, key, now, age)
			break
		}
	}
	return quotes, nil
}

func timeForTradeHops(hops []tradeHop) float64 {
	return float64(len(hops)) * (v.Num(DefaultRouteOptions()["jumpMinutes"]) +
		v.Num(DefaultRouteOptions()["cooldownMinutes"]) + v.Num(DefaultRouteOptions()["overheadMinutes"])) / 60
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

// buyTradeCargo fills a quantity using the cheapest offers in one system.
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

func fuelStation(fuelMarkets []M, quantity float64) M {
	for _, quote := range fuelMarkets {
		if v.Num(quote["supply"]) >= quantity {
			return quote
		}
	}
	return nil
}

func (plan tradeOpportunityInputs) candidate(key string, buyOffers []M, sell M, supply, limit float64, reposition, loaded, back []tradeHop, fuelMarkets []M, now time.Time) M {
	if len(buyOffers) == 0 || v.Num(buyOffers[0]["price"]) <= 0 || limit <= 0 {
		return nil
	}
	low, high := 0.0, math.Floor(math.Min(limit, plan.capacity-plan.baseCargo))
	kind, _ := calculatorCatalog(v.Str(plan.options["carrierType"]))
	squadron := kind.Key == "squadron"
	if squadron {
		// The 25,000 t Drake model does not establish squadron fuel use.
		high = math.Min(high, math.Floor(plan.capacity-plan.baseCargo-plan.hold))
	}
	jumpCost := float64(len(reposition)+len(loaded)+len(back)) * float64(kind.JumpCost)
	fuelFor := func(quantity float64) tradeFuelPlan {
		if squadron {
			return tradeFuelPlan{tank: plan.tank, hold: plan.hold, valid: true}
		}
		return plan.fuelForCargo(quantity, reposition, loaded, back)
	}
	for low < high {
		mid := math.Floor((low + high + 1) / 2)
		buy := buyTradeCargo(buyOffers, mid)
		fuel := fuelFor(mid)
		fuelMarket := fuelStation(fuelMarkets, fuel.acquired)
		hours := v.Num(plan.options["durationHours"])
		if plan.options["durationHours"] == nil {
			hours = timeForTradeHops(reposition) + timeForTradeHops(loaded) + timeForTradeHops(back) +
				tradeHandlingHours(buy, mid, fuel.acquired, v.Num(plan.options["haulerCapacity"]))
		}
		upkeep := hours * v.Num(plan.options["weeklyUpkeep"]) / 168
		affordable := v.Num(buy["purchaseTotal"])+fuel.acquired*v.Num(fuelMarket["price"])+jumpCost+upkeep+v.Num(plan.options["extraCosts"]) <= plan.budget
		if buy != nil && fuel.valid && affordable && (fuel.acquired == 0 || fuelMarket != nil) {
			low = mid
		} else {
			high = mid - 1
		}
	}
	if low < 1 {
		return nil
	}
	fuel := fuelFor(low)
	fuelMarket := fuelStation(fuelMarkets, fuel.acquired)
	fuelPrice := v.First(plan.options["tritiumPrice"], fuelMarket["price"])
	return plan.makeCandidate(key, buyTradeCargo(buyOffers, low), sell, low, supply, reposition, loaded, back, fuel, fuelPrice, fuelMarket, now)
}

func tradeHandlingHours(buy M, cargo, fuel, haulerCapacity float64) float64 {
	unloadSorties := math.Ceil(cargo / haulerCapacity)
	loadSorties := unloadSorties
	if orders := v.Rows(buy["orders"]); len(orders) > 1 {
		loadSorties = 0
		for _, order := range orders {
			loadSorties += math.Ceil(v.Num(order["quantity"]) / haulerCapacity)
		}
	}
	return (loadSorties + unloadSorties + math.Ceil(fuel/haulerCapacity)) * tradeShuttleMinutes / 60
}

func (plan tradeOpportunityInputs) makeCandidate(key string, buy, sell M, quantity, supply float64, reposition, loaded, back []tradeHop, fuel tradeFuelPlan, fuelPrice any, fuelMarket M, now time.Time) M {
	if quantity < 1 {
		return nil
	}
	options := v.Copy(plan.options)
	options["tankTons"], options["cargoTritiumTons"], options["tritiumPrice"] = fuel.tank, fuel.hold, fuelPrice
	_, legs := tradeDefaults()
	source, destination := v.Str(buy["system"]), v.Str(sell["system"])
	outbound := v.Obj(legs["outbound"])
	outbound["commodityKey"], outbound["quantity"] = key, quantity
	outbound["sourceSystem"], outbound["destinationSystem"] = source, destination
	outbound["buyMarket"], outbound["sellMarket"], outbound["buyPrice"], outbound["sellPrice"] = buy, sell, buy["price"], sell["price"]
	haulerCapacity := v.Num(options["haulerCapacity"])
	unloading := math.Ceil(quantity/haulerCapacity) * tradeShuttleMinutes / 60
	outbound["loadingHours"] = tradeHandlingHours(buy, quantity, fuel.acquired, haulerCapacity) - unloading
	outbound["unloadingHours"] = unloading
	outbound["jumps"], outbound["travelHours"] = len(reposition)+len(loaded), timeForTradeHops(reposition)+timeForTradeHops(loaded)
	backLeg := v.Obj(legs["return"])
	backLeg["sourceSystem"], backLeg["destinationSystem"] = destination, plan.system
	backLeg["jumps"], backLeg["travelHours"] = len(back), timeForTradeHops(back)
	if options["carrierType"] == "fleet" {
		outbound["tritiumTons"], backLeg["tritiumTons"] = fuel.outbound, fuel.inward
	}
	outbound["route"] = M{"sourceSystem": source, "destinationSystem": destination, "carrierType": options["carrierType"],
		"quantity": quantity, "servicesTons": options["servicesTons"], "otherCargoTons": options["otherCargoTons"],
		"positioningSource": plan.system, "reposition": tradeHopRows(reposition), "loaded": tradeHopRows(loaded),
		"fuelPurchaseTons": fuel.acquired, "fuelMarket": fuelMarket}
	backLeg["route"] = M{"sourceSystem": destination, "destinationSystem": plan.system, "carrierType": options["carrierType"],
		"quantity": 0, "servicesTons": options["servicesTons"], "otherCargoTons": options["otherCargoTons"], "return": tradeHopRows(back)}
	result := tradeSnapshot(options, legs, now)
	if profit, known := v.Number(v.At(result, "summary", "profit")); known && profit <= 0 && plan.scenario == "custom" {
		return nil
	}
	result["purchaseCost"], result["supply"], result["quantity"] = buy["purchaseTotal"], supply, quantity
	result["commodityKey"], result["commodityName"] = key, tradeCommodity(key)
	result["buy"], result["sell"], result["fuelMarket"] = buy, sell, fuelMarket
	result["fuelPurchaseTons"] = fuel.acquired
	result["fuelPurchaseCost"] = fuel.acquired * v.Num(fuelMarket["price"])
	result["route"] = M{"origin": plan.system, "reposition": tradeHopRows(reposition), "loaded": tradeHopRows(loaded), "return": tradeHopRows(back)}
	result["estimatedCarrier"] = plan.estimatedCarrier
	result["shuttleMinutes"] = tradeShuttleMinutes
	result["id"] = strings.Join([]string{key, v.Str(buy["system"]), v.Str(buy["station"]), v.Str(sell["system"]), v.Str(sell["station"])}, "|")
	if len(v.Rows(buy["orders"])) > 1 {
		result["id"] = v.Str(result["id"]) + "|combined"
	}
	return result
}

func (plan tradeOpportunityInputs) peakCandidates(key string, markets []M, sell M, corridor []tradeHop, fuelMarkets []M, now time.Time) []M {
	if sell == nil {
		sell = M{"system": tradePeakSystem, "station": tradePeakPort, "pad": "M"}
	}
	type systemSupply struct {
		market M
		offers []M
	}
	bySystem := map[string]*systemSupply{}
	for _, raw := range markets {
		buy := pickTradeQuote(raw, key, now, v.Num(plan.options["maxQuoteAgeHours"]))
		if buy == nil {
			continue
		}
		system := v.Key(buy["system"])
		if bySystem[system] == nil {
			bySystem[system] = &systemSupply{market: raw}
		}
		bySystem[system].offers = append(bySystem[system].offers, buy)
	}
	rows := []M{}
	back := reverseTradeHops(corridor)
	add := func(raw M, buyOffers []M, combined bool) {
		reposition, reachable := tradeDirectHop(plan.origin, raw)
		if !reachable {
			return
		}
		// Leave the acquisition system loaded and join the verified corridor
		// at the selected hub. The return is empty and follows those systems.
		loaded := append(reverseTradeHops(reposition), corridor...)
		var supply float64
		for _, quote := range buyOffers {
			supply += v.Num(quote["supply"])
		}
		limit := supply
		if sell["price"] != nil {
			limit = math.Min(limit, v.Num(sell["demand"]))
		}
		row := plan.candidate(key, buyOffers, sell, supply, limit, reposition, loaded, back, fuelMarkets, now)
		if row != nil && (!combined || len(v.Rows(v.At(row, "buy", "orders"))) > 1) {
			rows = append(rows, row)
		}
	}
	for _, supply := range bySystem {
		sort.SliceStable(supply.offers, func(i, j int) bool { return v.Num(supply.offers[i]["price"]) < v.Num(supply.offers[j]["price"]) })
		for _, quote := range supply.offers {
			add(supply.market, []M{quote}, false)
		}
		if len(supply.offers) > 1 {
			add(supply.market, supply.offers, true)
		}
	}
	return rows
}

// FindTradeOpportunities computes a bounded shortlist around the carrier. It
// uses independent market requests and never replaces the commander’s saved
// route or the existing trade plan.
func FindTradeOpportunities(ctx context.Context, client trading.HTTPDoer, trade, state, request M, update func(M)) (M, error) {
	now := time.Now()
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
	keys := []string{"gold", "silver", "agronomictreatment", "beryllium"}
	if plan.scenario == "booze-cruise" {
		keys = []string{"wine", "liquor"}
	}
	age := v.Num(plan.options["maxQuoteAgeHours"])
	pad := v.Str(plan.options["haulerPad"])
	minVolume := math.Max(1, math.Floor((plan.capacity-plan.baseCargo-plan.hold)/20))
	out["stage"] = "markets"
	if update != nil {
		update(v.Copy(out))
	}
	type query struct {
		key, side, system string
		radius, minimum   float64
	}
	queries := []query{{"tritium", "buy", plan.system, 100, 1}}
	for _, key := range keys {
		queries = append(queries, query{key, "buy", plan.system, 100, minVolume})
		if plan.scenario != "booze-cruise" {
			queries = append(queries, query{key, "sell", plan.system, 100, minVolume})
		}
	}
	type marketReply struct {
		query query
		data  M
		err   error
	}
	responses := make(chan marketReply, len(queries))
	throttle := make(chan struct{}, 3)
	for _, q := range queries {
		go func() {
			select {
			case throttle <- struct{}{}:
				defer func() { <-throttle }()
			case <-ctx.Done():
				responses <- marketReply{query: q, err: ctx.Err()}
				return
			}
			result, requestErr := marketInput(searcher, ctx, plan.state, q.system, q.key, q.side, pad, q.radius, age, q.minimum)
			responses <- marketReply{q, result, requestErr}
		}()
	}
	type peakReply struct {
		hops []tradeHop
		err  error
	}
	type quoteReply struct {
		quotes map[string]M
		err    error
	}
	peak := make(chan peakReply, 1)
	peakMarket := make(chan quoteReply, 1)
	if plan.scenario == "booze-cruise" {
		go func() { hops, routeErr := tradePeakHops(ctx, route, plan.system); peak <- peakReply{hops, routeErr} }()
		go func() {
			quotes, quoteErr := peakTradeQuotes(ctx, searcher, now, age)
			peakMarket <- quoteReply{quotes, quoteErr}
		}()
	}
	markets := map[string]M{}
	for range queries {
		reply := <-responses
		if reply.err != nil || v.Str(reply.data["error"]) != "" {
			out["partial"] = true
		}
		if v.Bool(reply.data["coverageLimited"]) {
			out["limited"] = true
		}
		if markets[reply.query.key] == nil {
			markets[reply.query.key] = M{}
		}
		markets[reply.query.key][reply.query.side] = reply.data["results"]
	}
	if err := ctx.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return out, err
	}
	fuelMarkets := []M{}
	for _, raw := range v.Rows(markets["tritium"]["buy"]) {
		quote := pickTradeQuote(raw, "tritium", now, age)
		if quote != nil {
			fuelMarkets = append(fuelMarkets, quote)
		}
	}
	sort.SliceStable(fuelMarkets, func(i, j int) bool { return v.Num(fuelMarkets[i]["price"]) < v.Num(fuelMarkets[j]["price"]) })
	out["stage"] = "routes"
	if update != nil {
		update(v.Copy(out))
	}
	rows := []M{}
	if plan.scenario == "booze-cruise" {
		corridor := <-peak
		if corridor.err != nil {
			out["status"] = "Маршрут до HIP 58832 недоступен."
			return out, corridor.err
		}
		sale := <-peakMarket
		if sale.err != nil {
			out["partial"] = true
		}
		for _, key := range keys {
			rows = append(rows, plan.peakCandidates(key, v.Rows(markets[key]["buy"]), sale.quotes[key], corridor.hops, fuelMarkets, now)...)
		}
	} else {
		opts := trading.RouteOptions(M{"system": plan.system, "radiusLy": 100, "maxLegLy": 200, "maxAgeHours": age,
			"cargoCapacity": plan.capacity, "credits": plan.budget, "orderBy": "profit", "minPad": pad,
			"demandRatio": 1, "roundTrip": false}, v.Obj(state["commander"]))
		opts["cargoCapacity"] = math.Max(0, plan.capacity-plan.baseCargo-plan.hold)
		for _, key := range keys {
			for _, pair := range trading.BuildRoutes(map[string]M{key: markets[key]}, opts) {
				legs := v.Rows(pair["legs"])
				if len(legs) == 0 {
					continue
				}
				leg := legs[0]
				rawBuy, rawSell := v.Obj(leg["buy"]), v.Obj(leg["sell"])
				buy, sell := pickTradeQuote(rawBuy, key, now, age), pickTradeQuote(rawSell, key, now, age)
				if buy == nil || sell == nil {
					continue
				}
				reposition, startOK := tradeDirectHop(plan.origin, rawBuy)
				loaded, loadOK := tradeDirectHop(rawBuy, rawSell)
				back, endOK := tradeDirectHop(rawSell, plan.origin)
				if !startOK || !loadOK || !endOK || v.Num(sell["demand"]) < math.Min(v.Num(plan.options["haulerCapacity"]), v.Num(leg["quantity"]))*v.Num(plan.options["demandRatio"]) {
					continue
				}
				if row := plan.candidate(key, []M{buy}, sell, v.Num(buy["supply"]), v.Num(leg["quantity"]), reposition, loaded, back, fuelMarkets, now); row != nil {
					rows = append(rows, row)
				}
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if plan.scenario == "booze-cruise" && v.Num(rows[i]["quantity"]) != v.Num(rows[j]["quantity"]) {
			return v.Num(rows[i]["quantity"]) > v.Num(rows[j]["quantity"])
		}
		a, aKnown := v.Number(v.At(rows[i], "summary", "profit"))
		b, bKnown := v.Number(v.At(rows[j], "summary", "profit"))
		if aKnown != bKnown {
			return aKnown
		}
		if aKnown && a != b {
			return a > b
		}
		if v.Num(rows[i]["quantity"]) != v.Num(rows[j]["quantity"]) {
			return v.Num(rows[i]["quantity"]) > v.Num(rows[j]["quantity"])
		}
		if v.Num(rows[i]["purchaseCost"]) != v.Num(rows[j]["purchaseCost"]) {
			return v.Num(rows[i]["purchaseCost"]) < v.Num(rows[j]["purchaseCost"])
		}
		return v.Str(rows[i]["id"]) < v.Str(rows[j]["id"])
	})
	if len(rows) > 8 && plan.scenario == "booze-cruise" {
		counts := map[string]int{}
		mixed, overflow := []M{}, []M{}
		for _, row := range rows {
			key := v.Str(row["commodityKey"])
			if counts[key] < 4 {
				mixed = append(mixed, row)
				counts[key]++
			} else {
				overflow = append(overflow, row)
			}
		}
		rows = append(mixed, overflow...)
	}
	if len(rows) > 8 {
		rows = rows[:8]
	}
	out["results"], out["updatedAt"] = rows, v.ISO(time.Now())
	if len(rows) == 0 {
		out["status"] = "Подходящих рейсов по свежим котировкам не найдено."
	}
	return out, nil
}
