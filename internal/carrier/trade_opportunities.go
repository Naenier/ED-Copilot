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
	tradePeakSystem   = "HIP 58832"
	tradePeakPort     = "Rackham's Peak"
	tradePeakMarket   = "128968675"
	tradePeakScenario = "booze-cruise"
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
		"scenario": tradePeakScenario, "system": system, "carrierType": options["carrierType"],
		"commanderSystem": commander["system"], "shipPad": options["haulerPad"],
		"shipModel":    v.First(commander["shipModel"], commander["shipType"], commander["ship"]),
		"shipCapacity": commander["cargoCapacity"],
		"credits":      v.First(request["credits"], v.At(state, "pilot", "credits")),
		"servicesTons": services, "estimatedServices": estimated,
	}
	for _, key := range []string{"tritiumPrice", "weeklyUpkeep", "durationHours", "extraCosts", "haulerCapacity", "maxQuoteAgeHours", "demandRatio", "holidayConfirmed"} {
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
	plan.options["preset"] = tradePeakScenario
	if v.At(trade, "options", "preset") != tradePeakScenario {
		plan.options["holidayConfirmed"] = false
	}
	observedPad, observedCapacity := observedHauler(v.Obj(state["commander"]))
	shipPad := v.Text(plan.options["haulerPad"], observedPad)
	shipCapacity := v.Num(plan.options["haulerCapacity"])
	if shipCapacity == 0 && (shipPad == "" || shipPad == observedPad) {
		shipCapacity = observedCapacity
	}
	if shipPad == "L" || shipPad == "" {
		shipPad, shipCapacity = "M", 280
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
	start := float64(0)
	for range 24 {
		empty := tradeFuelConsumed(reposition, plan.services, start)
		out := empty + tradeFuelConsumed(loaded, plan.services+cargo, start-empty)
		in := tradeFuelConsumed(back, plan.services, start-out)
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
	low, high := 0.0, math.Floor(math.Min(limit, plan.capacity-plan.services))
	kind, _ := calculatorCatalog(v.Str(plan.options["carrierType"]))
	jumpCost := float64(len(reposition)+len(loaded)+len(back)) * float64(kind.JumpCost)
	var bestBuy, bestFuelMarket M
	var bestFuel tradeFuelPlan
	for low < high {
		mid := math.Floor((low + high + 1) / 2)
		buy := buyTradeCargo(buyOffers, mid)
		fuel := plan.fuelForCargo(mid, reposition, loaded, back)
		fuelMarket := fuelStation(fuelMarkets, fuel.acquired)
		fuelPrice := v.First(fuelMarket["price"], plan.options["tritiumPrice"])
		hours := v.Num(plan.options["durationHours"])
		if plan.options["durationHours"] == nil {
			hours = timeForTradeHops(reposition) + timeForTradeHops(loaded) + timeForTradeHops(back) +
				tradeHandlingHours(buy, mid, fuel.acquired, v.Num(plan.options["haulerCapacity"]))
		}
		upkeep := math.Ceil(hours/168) * v.Num(plan.options["weeklyUpkeep"])
		affordable := v.Num(buy["purchaseTotal"])+fuel.acquired*v.Num(fuelPrice)+jumpCost+upkeep+v.Num(plan.options["extraCosts"]) <= plan.budget
		if buy != nil && fuel.valid && affordable && (fuel.acquired == 0 || fuelPrice != nil) {
			low = mid
			bestBuy, bestFuelMarket, bestFuel = buy, fuelMarket, fuel
		} else {
			high = mid - 1
		}
	}
	if low < 1 {
		return nil
	}
	fuelPrice := v.First(plan.options["tritiumPrice"], bestFuelMarket["price"])
	return plan.makeCandidate(key, bestBuy, sell, low, supply, reposition, loaded, back, bestFuel, fuelPrice, bestFuelMarket, now)
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
	options["arrivalReserveTons"] = fuel.reserve
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
	outbound["tritiumTons"], backLeg["tritiumTons"] = fuel.outbound, fuel.inward
	fuelPurchaseCost := tradeTimes(v.First(fuelMarket["price"], fuelPrice), fuel.acquired)
	outbound["route"] = M{"sourceSystem": source, "destinationSystem": destination, "carrierType": options["carrierType"],
		"quantity": quantity, "servicesTons": options["servicesTons"], "otherCargoTons": options["otherCargoTons"],
		"positioningSource": plan.system, "reposition": tradeHopRows(reposition), "loaded": tradeHopRows(loaded),
		"fuelPurchaseTons": fuel.acquired, "fuelPurchaseCost": fuelPurchaseCost, "fuelMarket": fuelMarket}
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
		// Join the corridor straight from the supplier when reachable.
		loaded := append(reverseTradeHops(reposition), corridor...)
		if len(reposition) > 0 {
			if direct, ok := directTradePeakCorridor(raw); ok && len(direct) <= len(loaded) {
				loaded = direct
			}
		}
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

// FindTradeOpportunities combines nearby suppliers with the route to Rackham's Peak.
func FindTradeOpportunities(ctx context.Context, client trading.HTTPDoer, trade, state, request M, update func(M)) (M, error) {
	now := time.Now()
	request = v.Merge(M{}, request, M{"scenario": tradePeakScenario})
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
	keys := []string{"wine", "liquor"}
	age := v.Num(plan.options["maxQuoteAgeHours"])
	pad := v.Str(plan.options["haulerPad"])
	minVolume := math.Max(1, math.Floor((plan.capacity-plan.services)/20))
	out["stage"] = "markets"
	if update != nil {
		update(v.Copy(out))
	}
	type query struct {
		key     string
		minimum float64
	}
	queries := []query{{"tritium", 1}}
	for _, key := range keys {
		queries = append(queries, query{key, minVolume})
	}
	type marketReply struct {
		query query
		data  M
		err   error
	}
	responses := make(chan marketReply, len(queries))
	for _, q := range queries {
		go func() {
			result, requestErr := marketInput(searcher, ctx, plan.state, plan.system, q.key, "buy", pad, 100, age, q.minimum)
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
	go func() {
		hops, routeErr := tradePeakCorridor(ctx, route, plan.origin)
		peak <- peakReply{hops, routeErr}
	}()
	go func() {
		quotes, quoteErr := peakTradeQuotes(ctx, searcher, now, age)
		peakMarket <- quoteReply{quotes, quoteErr}
	}()
	markets := map[string][]M{}
	for range queries {
		reply := <-responses
		if reply.err != nil || v.Str(reply.data["error"]) != "" {
			out["partial"] = true
		}
		if v.Bool(reply.data["coverageLimited"]) {
			out["limited"] = true
		}
		markets[reply.query.key] = v.Rows(reply.data["results"])
	}
	if err := ctx.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return out, err
	}
	fuelMarkets := []M{}
	for _, raw := range markets["tritium"] {
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
		rows = append(rows, plan.peakCandidates(key, markets[key], sale.quotes[key], corridor.hops, fuelMarkets, now)...)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if v.Num(rows[i]["quantity"]) != v.Num(rows[j]["quantity"]) {
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
		if v.Num(rows[i]["purchaseCost"]) != v.Num(rows[j]["purchaseCost"]) {
			return v.Num(rows[i]["purchaseCost"]) < v.Num(rows[j]["purchaseCost"])
		}
		return v.Str(rows[i]["id"]) < v.Str(rows[j]["id"])
	})
	if len(rows) > 8 {
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
