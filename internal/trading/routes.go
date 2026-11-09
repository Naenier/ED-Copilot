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
	out := M{"system": v.Text(input["system"], commander["system"]), "resourceKeys": []string{"gold", "silver", "agronomictreatment", "bertrandite", "indite", "bauxite"}, "cargoCapacity": nil, "credits": nil, "radiusLy": 100, "maxLegLy": 100, "maxAgeHours": 24, "carrierMaxAgeHours": 6, "minutesPerLeg": 15, "operatingCostPerLeg": 0, "demandRatio": 4, "minSupply": 1, "minDemand": 1, "maxArrivalLs": 10000, "minPad": "L", "marketType": "stations", "orderBy": "profitPerHour", "includeCarriers": false, "includePlanetary": true, "roundTrip": false}
	if capacity, known := v.Number(commander["cargoCapacity"]); known {
		out["cargoCapacity"] = math.Max(0, capacity-n(commander, "cargoTons"))
	}
	if input["resourceKeys"] != nil || input["resourceKey"] != nil {
		keys := v.Arr(input["resourceKeys"])
		if len(keys) == 0 {
			keys = []any{input["resourceKey"]}
		}
		seen := map[string]bool{}
		result := []string{}
		for _, raw := range keys {
			k := v.Key(raw)
			if k != "" && !seen[k] {
				seen[k] = true
				result = append(result, k)
			}
		}
		if len(result) > 6 {
			result = result[:6]
		}
		out["resourceKeys"] = result
	}
	for _, r := range []struct {
		k        string
		min, max float64
	}{{"cargoCapacity", 0, 25000}, {"credits", 0, 1e15}, {"radiusLy", 5, 5000}, {"maxLegLy", 1, 5000}, {"maxAgeHours", 1, 720}, {"carrierMaxAgeHours", .25, 168}, {"minutesPerLeg", 1, 1440}, {"operatingCostPerLeg", 0, 1e12}, {"demandRatio", 1, 20}, {"minSupply", 1, 1e9}, {"minDemand", 1, 1e9}, {"maxArrivalLs", 0, 1e7}} {
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

func PairLeg(buy, sell M, key string, options M, budgets ...float64) M {
	if buy["id"] == sell["id"] || !routeStationAllowed(buy, options) || !routeStationAllowed(sell, options) {
		return nil
	}
	distance := Distance(buy["coordinates"], sell["coordinates"])
	if math.IsNaN(distance) || distance > n(options, "maxLegLy") {
		return nil
	}
	var source, target M
	for _, q := range v.Rows(buy["quotes"]) {
		if q["key"] == key {
			source = q
		}
	}
	for _, q := range v.Rows(sell["quotes"]) {
		if q["key"] == key {
			target = q
		}
	}
	if n(source, "price") <= 0 || n(target, "price") <= n(source, "price") {
		return nil
	}
	if n(source, "supply") < n(options, "minSupply") || n(target, "demand") < n(options, "minDemand") {
		return nil
	}
	safeDemand := n(target, "demand")
	if !v.Bool(sell["carrier"]) {
		safeDemand = math.Floor(safeDemand / n(options, "demandRatio"))
	}
	budget := n(options, "credits")
	if len(budgets) > 0 {
		budget = budgets[0]
	}
	quantity := math.Floor(math.Min(math.Min(n(options, "cargoCapacity"), n(source, "supply")), math.Min(safeDemand, (budget-n(options, "operatingCostPerLeg"))/n(source, "price"))))
	if quantity <= 0 {
		return nil
	}
	purchase, revenue := quantity*n(source, "price"), quantity*n(target, "price")
	margin := revenue - purchase
	net := margin - n(options, "operatingCostPerLeg")
	if net <= 0 {
		return nil
	}
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
func routeLess(a, b, options M) bool {
	key, descending := "profitPerHour", true
	switch options["orderBy"] {
	case "profit":
		key = "netEstimate"
	case "distance":
		key = "distanceLy"
		descending = false
	case "purchaseCost":
		key = "minimumCapital"
		descending = false
	}
	if n(a, key) != n(b, key) {
		if descending {
			return n(a, key) > n(b, key)
		}
		return n(a, key) < n(b, key)
	}
	if n(a, "netEstimate") != n(b, "netEstimate") {
		return n(a, "netEstimate") > n(b, "netEstimate")
	}
	return v.Str(a["id"]) < v.Str(b["id"])
}
func BuildRoutes(markets map[string]M, options M) []M {
	legs := []M{}
	for key, sides := range markets {
		for _, buy := range v.Rows(sides["buy"]) {
			for _, sell := range v.Rows(sides["sell"]) {
				if leg := PairLeg(buy, sell, key, options); leg != nil {
					legs = append(legs, leg)
				}
			}
		}
	}
	rows := []M{}
	add := func(row M) {
		rows = append(rows, row)
		if len(rows) > 120 {
			sort.Slice(rows, func(i, j int) bool { return routeLess(rows[i], rows[j], options) })
			rows = rows[:60]
		}
	}
	for _, outward := range legs {
		buy, sell := v.Obj(outward["buy"]), v.Obj(outward["sell"])
		if !v.Bool(options["roundTrip"]) {
			add(routeSummary(outward, nil, options))
			continue
		}
		for key, sides := range markets {
			for _, returnBuy := range v.Rows(sides["buy"]) {
				if returnBuy["id"] != sell["id"] {
					continue
				}
				for _, returnSell := range v.Rows(sides["sell"]) {
					if returnSell["id"] != buy["id"] {
						continue
					}
					inward := PairLeg(returnBuy, returnSell, key, options, n(options, "credits")+n(outward, "netEstimate"))
					if inward == nil {
						continue
					}
					add(routeSummary(outward, inward, options))
				}
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return routeLess(rows[i], rows[j], options) })
	if len(rows) > 60 {
		rows = rows[:60]
	}
	return rows
}
func (s *Searcher) FindRoutes(ctx context.Context, input, state M) (M, error) {
	input = v.Copy(input)
	if input["credits"] == nil {
		input["credits"] = v.At(state, "pilot", "credits")
	}
	input["now"] = v.ISO(s.Now())
	options := RouteOptions(input, v.Obj(state["commander"]))
	keys := v.Arr(options["resourceKeys"])
	if v.Str(options["system"]) == "" || len(keys) == 0 {
		return nil, errors.New("Укажите исходную систему и товары.")
	}
	if options["cargoCapacity"] == nil || n(options, "cargoCapacity") < 1 || options["credits"] == nil {
		return nil, errors.New("Укажите свободный трюм и известный бюджет на закупку.")
	}
	if v.Bool(options["automatic"]) {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
	}
	out := M{"running": false, "options": options, "results": []M{}, "checked": 0, "total": len(keys) * 2, "updatedAt": v.ISO(s.Now()), "coverageLimited": false, "assumptions": "Спрос ограничивает объём с выбранным запасом. Доход в час использует введённое время рейса. Доставка к первой станции не включена. Котировки могут измениться."}
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
