package trading

import (
	"context"
	"errors"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"sort"
	"strings"
	"time"
)

func RouteOptions(input, commander M) M {
	out := M{"system": v.Text(input["system"], commander["system"]), "resourceKeys": []string{}, "resourceFilter": false, "cargoCapacity": nil, "credits": nil, "radiusLy": 100, "maxLegLy": 100, "maxAgeHours": 24, "carrierMaxAgeHours": 6, "minutesPerLeg": 15, "operatingCostPerLeg": 0, "minNetProfit": 0, "demandRatio": 4, "minSupply": 1, "minDemand": 1, "maxArrivalLs": 10000, "minPad": "L", "marketType": "stations", "orderBy": "profitPerHour", "includeCarriers": false, "includePlanetary": true, "roundTrip": false}
	if capacity, known := v.Number(commander["cargoCapacity"]); known {
		out["cargoCapacity"] = math.Max(0, capacity-n(commander, "cargoTons"))
	}
	filter, explicitFilter := input["resourceFilter"].(bool)
	if filter || !explicitFilter && (input["resourceKeys"] != nil || input["resourceKey"] != nil) {
		keys := v.Arr(input["resourceKeys"])
		if len(keys) == 0 {
			keys = []any{input["resourceKey"]}
		}
		seen := map[string]bool{}
		result := []string{}
		for _, raw := range keys {
			k := v.Str(Good(raw, nil)["key"])
			if k != "" && !seen[k] {
				seen[k] = true
				result = append(result, k)
			}
		}
		out["resourceKeys"] = result
		out["resourceFilter"] = filter || len(result) > 0
	}
	for _, r := range []struct {
		k        string
		min, max float64
	}{{"cargoCapacity", 0, 25000}, {"credits", 0, 1e15}, {"radiusLy", 5, 5000}, {"maxLegLy", 1, 5000}, {"maxAgeHours", 1, 720}, {"carrierMaxAgeHours", .25, 168}, {"minutesPerLeg", 1, 1440}, {"operatingCostPerLeg", 0, 1e12}, {"minNetProfit", 0, 1e15}, {"demandRatio", 1, 20}, {"minSupply", 1, 1e9}, {"minDemand", 1, 1e9}, {"maxArrivalLs", 0, 1e7}} {
		if x, ok := v.Number(input[r.k]); ok {
			out[r.k] = v.Clamp(x, r.min, r.max)
		}
	}
	for _, k := range []string{"includeCarriers", "includePlanetary", "roundTrip", "automatic"} {
		if b, ok := input[k].(bool); ok {
			out[k] = b
		}
	}
	switch input["minPad"] {
	case "S", "M", "L", "any":
		out["minPad"] = input["minPad"]
	}
	if ship := v.Str(input["shipPad"]); input["minPad"] == nil && (ship == "S" || ship == "M" || ship == "L") {
		out["minPad"] = ship
	}
	if input["minPad"] == nil && input["shipPad"] == nil {
		if pad := ShipPad(commander); pad != "" {
			out["minPad"] = pad
		}
	}
	out["shipPad"] = out["minPad"]
	if v.Bool(out["includeCarriers"]) {
		out["marketType"] = "all"
	}
	switch input["marketType"] {
	case "stations", "carriers", "all":
		out["marketType"] = input["marketType"]
		out["includeCarriers"] = input["marketType"] != "stations"
	}
	switch input["orderBy"] {
	case "profit", "profitPerHour", "distance", "purchaseCost":
		out["orderBy"] = input["orderBy"]
	}
	if at := v.Time(input["now"]); !at.IsZero() {
		out["now"] = v.ISO(at)
	}
	if out["cargoCapacity"] != nil {
		out["cargoCapacity"] = math.Floor(n(out, "cargoCapacity"))
	}
	return out
}

// routeMarket keeps the selected commodity and its original observation. Its
// quantity describes this leg, rather than the larger basket used for search.
func routeMarket(raw, quote M, quantity float64) M {
	out := M{}
	for _, key := range []string{"id", "marketId", "station", "system", "systemAddress", "type", "pad", "carrier", "planetary", "distanceLy", "distanceLs", "updatedAt", "sourceUpdatedAt", "ageHours", "source", "sourceUrl", "own"} {
		out[key] = raw[key]
	}
	out["coordinates"] = v.Clone(raw["coordinates"])
	out["warnings"] = v.Clone(raw["warnings"])
	q := v.Copy(quote)
	q["quantity"], q["requestedQuantity"], q["subtotal"] = quantity, quantity, quantity*n(quote, "price")
	q["bulkTaxRisk"] = false // the paired volume has already been capped by safe demand.
	out["quotes"] = []M{q}
	out["requestedTons"], out["coveredTons"], out["missingTons"], out["coverageComplete"], out["estimatedTotal"] = quantity, quantity, 0, true, q["subtotal"]
	out["demandSafe"], out["bulkTaxRisk"] = true, false
	out["price"], out["supply"], out["demand"] = quote["price"], quote["supply"], quote["demand"]
	out["observedAt"] = v.First(quote["updatedAt"], raw["sourceUpdatedAt"], raw["updatedAt"])
	return out
}

func routeStationAllowed(station, options M) bool {
	if v.Bool(station["own"]) || !v.Bool(options["includePlanetary"]) && v.Bool(station["planetary"]) || !v.Bool(options["includeCarriers"]) && v.Bool(station["carrier"]) || options["marketType"] == "carriers" && !v.Bool(station["carrier"]) {
		return false
	}
	rank := map[string]int{"S": 1, "M": 2, "L": 3}
	// Unknown pads remain usable for legacy/manual scenarios, but are labelled
	// unverified. Network search already excludes them for a required ship pad.
	if rank[v.Str(station["pad"])] > 0 && rank[v.Str(station["pad"])] < rank[v.Str(options["minPad"])] {
		return false
	}
	if arrival, known := v.Number(station["distanceLs"]); known && n(options, "maxArrivalLs") > 0 && arrival > n(options, "maxArrivalLs") {
		return false
	}
	if distance, known := v.Number(station["distanceLy"]); known && distance > n(options, "radiusLy") {
		return false
	}
	limit := n(options, "maxAgeHours")
	if v.Bool(station["carrier"]) {
		limit = n(options, "carrierMaxAgeHours")
	}
	if age, known := v.Number(station["ageHours"]); known && age > limit {
		return false
	}
	if now := v.Time(options["now"]); !now.IsZero() {
		if at := v.Time(station["updatedAt"]); !at.IsZero() && (at.After(now.Add(10*time.Minute)) || now.Sub(at).Hours() > limit) {
			return false
		}
	}
	return true
}

type routeEndpoint struct {
	market M
	quote  M
	coords []float64
	id     string
}

func prepareRouteEndpoint(market M, key string, options M) routeEndpoint {
	if !routeStationAllowed(market, options) {
		return routeEndpoint{}
	}
	coords, ok := market["coordinates"].([]float64)
	if !ok {
		coords = Coords(market["coordinates"])
	}
	quote := routeQuote(market, key)
	if len(coords) != 3 || quote == nil {
		return routeEndpoint{}
	}
	return routeEndpoint{market: market, quote: quote, coords: coords, id: v.Str(market["id"])}
}

type routeLeg struct {
	buy, sell                                                      routeEndpoint
	key                                                            string
	quantity, safeDemand, distance, budget, purchase, revenue, net float64
}

func evaluateLeg(buy, sell routeEndpoint, key string, budget float64, options M) (routeLeg, bool) {
	if buy.market == nil || sell.market == nil || buy.id == sell.id {
		return routeLeg{}, false
	}
	source, target := buy.quote, sell.quote
	sourcePrice, targetPrice := n(source, "price"), n(target, "price")
	if sourcePrice <= 0 || targetPrice <= sourcePrice || n(source, "supply") < n(options, "minSupply") || n(target, "demand") < n(options, "minDemand") {
		return routeLeg{}, false
	}
	dx, dy, dz := buy.coords[0]-sell.coords[0], buy.coords[1]-sell.coords[1], buy.coords[2]-sell.coords[2]
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if math.IsNaN(distance) || distance > n(options, "maxLegLy") {
		return routeLeg{}, false
	}
	safeDemand := n(target, "demand")
	if !v.Bool(sell.market["carrier"]) {
		safeDemand = math.Floor(safeDemand / n(options, "demandRatio"))
	}
	cost := n(options, "operatingCostPerLeg")
	quantity := math.Floor(math.Min(math.Min(n(options, "cargoCapacity"), n(source, "supply")), math.Min(safeDemand, (budget-cost)/sourcePrice)))
	if quantity <= 0 {
		return routeLeg{}, false
	}
	purchase, revenue := quantity*sourcePrice, quantity*targetPrice
	net := revenue - purchase - cost
	if net <= 0 {
		return routeLeg{}, false
	}
	return routeLeg{buy, sell, key, quantity, safeDemand, distance, budget, purchase, revenue, net}, true
}

func PairLeg(buy, sell M, key string, options M, budgets ...float64) M {
	budget := n(options, "credits")
	if len(budgets) > 0 {
		budget = budgets[0]
	}
	leg, ok := evaluateLeg(prepareRouteEndpoint(buy, key, options), prepareRouteEndpoint(sell, key, options), key, budget, options)
	if !ok {
		return nil
	}
	return leg.dto(options)
}

func (l routeLeg) dto(options M) M {
	buy, sell, source, target := l.buy.market, l.sell.market, l.buy.quote, l.sell.quote
	key, quantity, safeDemand, distance, budget := l.key, l.quantity, l.safeDemand, l.distance, l.budget
	purchase, revenue, net := l.purchase, l.revenue, l.net
	margin := revenue - purchase

	limits := M{"cargo": n(options, "cargoCapacity"), "supply": math.Floor(n(source, "supply")), "safeDemand": safeDemand, "capital": math.Max(0, math.Floor((budget-n(options, "operatingCostPerLeg"))/n(source, "price")))}
	limiting := []string{}
	for _, field := range []string{"cargo", "supply", "safeDemand", "capital"} {
		if n(limits, field) == quantity {
			limiting = append(limiting, field)
		}
	}
	warnings := []string{}
	for _, station := range []M{buy, sell} {
		if v.Str(station["pad"]) == "" {
			warnings = append(warnings, "Размер площадки не подтверждён.")
		}
		if station["distanceLs"] == nil {
			warnings = append(warnings, "Расстояние от звезды неизвестно.")
		}
		if v.Str(station["updatedAt"]) == "" {
			warnings = append(warnings, "Время наблюдения цены неизвестно.")
		}
		if v.Bool(station["carrier"]) {
			warnings = append(warnings, "Флотоносец мог переместиться или изменить заказ.")
		}
	}
	var approach any
	if value, known := v.Number(buy["distanceLy"]); known {
		approach = value
	}
	return M{"key": key, "name": v.Text(source["name"], Name(key, nil)), "buy": routeMarket(buy, source, quantity), "sell": routeMarket(sell, target, quantity), "quantity": quantity, "buyPrice": source["price"], "sellPrice": target["price"], "supply": source["supply"], "demand": target["demand"], "safeDemand": safeDemand, "purchaseCost": purchase, "revenue": revenue, "grossMargin": margin, "operatingCost": options["operatingCostPerLeg"], "netEstimate": net, "profitPerTonne": n(target, "price") - n(source, "price"), "netPerTonne": net / quantity, "marginPercent": (n(target, "price")/n(source, "price") - 1) * 100, "distanceLy": distance, "approachDistanceLy": approach, "minutes": options["minutesPerLeg"], "profitPerHour": net * 60 / n(options, "minutesPerLeg"), "timeBasis": "manual", "budgetBefore": budget, "budgetAfter": budget + net, "limits": limits, "limitingFactors": limiting, "warnings": warnings}
}

func routeQuote(station M, key string) M {
	if quotes, ok := station["routeQuotes"].(map[string]M); ok {
		return quotes[key]
	}
	var quote M
	for _, row := range v.Rows(station["quotes"]) {
		if row["key"] == key {
			quote = row
		}
	}
	return quote
}
func routeSummary(outward M, inward M, options M) M {
	legs := []M{outward}
	if inward != nil {
		legs = append(legs, inward)
	}
	net, gross, minutes, distance, cost, totalPurchase, capital := 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0
	warnings := []string{}
	seen := map[string]bool{}
	for _, leg := range legs {
		capital = math.Max(capital, n(leg, "purchaseCost")+n(leg, "operatingCost")-net)
		net += n(leg, "netEstimate")
		gross += n(leg, "grossMargin")
		minutes += n(leg, "minutes")
		distance += n(leg, "distanceLy")
		cost += n(leg, "operatingCost")
		totalPurchase += n(leg, "purchaseCost")
		for _, raw := range v.Arr(leg["warnings"]) {
			text := v.Str(raw)
			if !seen[text] {
				seen[text] = true
				warnings = append(warnings, text)
			}
		}
	}
	id := v.Str(v.At(outward, "buy", "id")) + "|" + v.Str(v.At(outward, "sell", "id")) + "|" + v.Str(outward["key"])
	if inward != nil {
		id += "|" + v.Str(inward["key"])
	}
	return M{"id": id, "legs": legs, "roundTrip": inward != nil, "netEstimate": net, "grossMargin": gross, "operatingCost": cost, "minutes": minutes, "profitPerHour": net * 60 / minutes, "distanceLy": distance, "legDistanceLy": outward["distanceLy"], "approachDistanceLy": outward["approachDistanceLy"], "minimumCapital": capital, "totalPurchaseCost": totalPurchase, "warnings": warnings, "timeBasis": "manual", "options": v.Copy(options)}
}
func BuildRoutes(markets map[string]M, options M) []M {
	roundTrip := v.Bool(options["roundTrip"])
	minimum, budget, cost := n(options, "minNetProfit"), n(options, "credits"), n(options, "operatingCostPerLeg")
	type marketSides struct {
		buy, sell []routeEndpoint
	}
	type returnMarket struct {
		key string
		buy routeEndpoint
	}
	prepared := make(map[string]marketSides, len(markets))
	returnBuys := map[string][]returnMarket{}
	returnSells := map[string]map[string]routeEndpoint{}
	for key, sides := range markets {
		var ready marketSides
		if roundTrip {
			returnSells[key] = map[string]routeEndpoint{}
		}
		for _, market := range v.Rows(sides["buy"]) {
			if buy := prepareRouteEndpoint(market, key, options); buy.market != nil {
				ready.buy = append(ready.buy, buy)
				if roundTrip {
					returnBuys[buy.id] = append(returnBuys[buy.id], returnMarket{key, buy})
				}
			}
		}
		for _, market := range v.Rows(sides["sell"]) {
			if sell := prepareRouteEndpoint(market, key, options); sell.market != nil {
				ready.sell = append(ready.sell, sell)
				if roundTrip {
					returnSells[key][sell.id] = sell
				}
			}
		}
		prepared[key] = ready
	}

	type candidate struct {
		out, back  routeLeg
		net, order float64
		id         string
	}
	rows := make([]candidate, 0, 120)
	var cutoff *candidate
	less := func(a, b candidate) bool {
		if a.order != b.order {
			return a.order > b.order
		}
		if a.net != b.net {
			return a.net > b.net
		}
		return a.id < b.id
	}
	add := func(out, back routeLeg) {
		net := out.net + back.net
		if net < minimum {
			return
		}
		order := net
		switch options["orderBy"] {
		case "distance":
			order = -(out.distance + back.distance)
		case "purchaseCost":
			order = -math.Max(out.purchase+cost, back.purchase+cost-out.net)
		}
		if cutoff != nil && (order < cutoff.order || order == cutoff.order && net < cutoff.net) {
			return
		}
		id := out.buy.id + "|" + out.sell.id + "|" + out.key
		if roundTrip {
			id += "|" + back.key
		}
		row := candidate{out, back, net, order, id}
		if cutoff != nil && !less(row, *cutoff) {
			return
		}
		rows = append(rows, row)
		if len(rows) == 120 {
			sort.Slice(rows, func(i, j int) bool { return less(rows[i], rows[j]) })
			rows = rows[:60]
			cutoff = &rows[59]
		}
	}
	for key, sides := range prepared {
		for _, buy := range sides.buy {
			for _, sell := range sides.sell {
				outward, ok := evaluateLeg(buy, sell, key, budget, options)
				if !ok {
					continue
				}
				if !roundTrip {
					add(outward, routeLeg{})
					continue
				}
				for _, back := range returnBuys[sell.id] {
					returnSell := returnSells[back.key][buy.id]
					inward, ok := evaluateLeg(back.buy, returnSell, back.key, budget+outward.net, options)
					if ok {
						add(outward, inward)
					}
				}
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return less(rows[i], rows[j]) })
	results := make([]M, 0, min(len(rows), 60))
	for _, row := range rows[:min(len(rows), 60)] {
		var inward M
		if roundTrip {
			inward = row.back.dto(options)
		}
		results = append(results, routeSummary(row.out.dto(options), inward, options))
	}
	return results
}

func (s *Searcher) FindRoutes(ctx context.Context, input, state M) (M, error) {
	input = v.Copy(input)
	if input["credits"] == nil {
		input["credits"] = v.At(state, "pilot", "credits")
	}
	input["now"] = v.ISO(s.Now())
	options := RouteOptions(input, v.Obj(state["commander"]))
	keys := v.Arr(options["resourceKeys"])
	if v.Str(options["system"]) == "" {
		return nil, errors.New("Укажите исходную систему поиска.")
	}
	if v.Bool(options["resourceFilter"]) && len(keys) == 0 {
		return nil, errors.New("Выберите хотя бы один товар.")
	}
	if options["cargoCapacity"] == nil || n(options, "cargoCapacity") < 1 || options["credits"] == nil {
		return nil, errors.New("Укажите свободный трюм и известный бюджет на закупку.")
	}
	legacySearch := input["resourceFilter"] == nil && len(keys) > 0 && len(keys) <= 6
	if !legacySearch || v.Bool(options["automatic"]) {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
	}
	out := M{"running": false, "options": options, "results": []M{}, "checked": 0, "total": len(keys) * 2, "updatedAt": v.ISO(s.Now()), "coverageLimited": false, "assumptions": "Спрос ограничивает объём с выбранным запасом. Доход в час использует введённое время рейса. Доставка к первой станции не включена. Котировки могут измениться."}
	if !legacySearch {
		return s.findRegionalRoutes(ctx, state, options, out)
	}
	markets := map[string]M{}
	errs := []string{}
	// Market search publishes station DTOs. Reusing the parent's callback here
	// briefly replaced tradeRoutes.results with raw stations during pagination.
	// Only the route planner may publish into the route stream.
	child := *s
	child.OnUpdate = nil
	publish := func() {
		out["updatedAt"] = v.ISO(s.Now())
		out["progress"] = M{"checked": out["checked"], "total": out["total"], "label": "Сравнение торговых рейсов"}
		if len(errs) > 0 {
			out["error"] = strings.Join(errs, " · ")
		}
		if s.OnUpdate != nil {
			s.OnUpdate(v.Copy(out))
		}
	}
	publish()
	for _, rawKey := range keys {
		key := v.Str(rawKey)
		markets[key] = M{}
		for _, mode := range []string{"buy", "sell"} {
			if err := ctx.Err(); err != nil {
				if v.Bool(options["automatic"]) && errors.Is(err, context.DeadlineExceeded) {
					out["coverageLimited"] = true
					out["status"] = "Поиск остановлен по времени."
					out["error"] = "За отведённое время проверены не все рынки."
					return out, nil
				}
				return out, err
			}
			request := v.Copy(options)
			request["mode"] = mode
			request["resourceKey"] = key
			request["quantity"] = options["cargoCapacity"]
			request["useCargo"] = false
			result, err := child.Search(ctx, request, state)
			plus(out, "checked", 1)
			if err != nil {
				errs = append(errs, err.Error())
			} else {
				markets[key][mode] = result["results"]
				if v.Bool(result["coverageLimited"]) {
					out["coverageLimited"] = true
				}
				if v.Str(result["error"]) != "" {
					errs = append(errs, v.Str(result["error"]))
				}
			}
			out["results"] = BuildRoutes(markets, options)
			publish()
		}
	}
	out["status"] = "Торговые рейсы рассчитаны по доступным котировкам."
	if len(v.Rows(out["results"])) == 0 {
		out["status"] = "Прибыльных рейсов с этими ограничениями не найдено."
	}
	if len(errs) > 0 {
		out["error"] = strings.Join(errs, " · ")
	}
	out["updatedAt"] = v.ISO(s.Now())
	return out, nil
}
