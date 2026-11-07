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
	out := M{"system": v.Text(input["system"], commander["system"]), "resourceKeys": []string{"gold", "silver", "agronomictreatment", "bertrandite", "indite", "bauxite"}, "cargoCapacity": 128, "credits": 1e7, "radiusLy": 100, "maxLegLy": 100, "maxAgeHours": 24, "carrierMaxAgeHours": 6, "minutesPerLeg": 15, "operatingCostPerLeg": 0, "demandRatio": 4, "maxArrivalLs": 10000, "minPad": "L", "includeCarriers": false, "includePlanetary": true, "roundTrip": false}
	if n(commander, "cargoCapacity") > 0 {
		out["cargoCapacity"] = commander["cargoCapacity"]
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
	}{{"cargoCapacity", 1, 25000}, {"credits", 0, 1e15}, {"radiusLy", 5, 5000}, {"maxLegLy", 1, 5000}, {"maxAgeHours", 1, 720}, {"carrierMaxAgeHours", .25, 168}, {"minutesPerLeg", 1, 1440}, {"operatingCostPerLeg", 0, 1e12}, {"demandRatio", 1, 20}, {"maxArrivalLs", 0, 1e7}} {
		if x, ok := v.Number(input[r.k]); ok {
			out[r.k] = v.Clamp(x, r.min, r.max)
		}
	}
	for _, k := range []string{"includeCarriers", "includePlanetary", "roundTrip"} {
		if b, ok := input[k].(bool); ok {
			out[k] = b
		}
	}
	switch input["minPad"] {
	case "S", "M", "L", "any":
		out["minPad"] = input["minPad"]
	}
	out["cargoCapacity"] = math.Floor(n(out, "cargoCapacity"))
	return out
}
func PairLeg(buy, sell M, key string, options M, budgets ...float64) M {
	if buy["id"] == sell["id"] || v.Bool(buy["own"]) || v.Bool(sell["own"]) || !v.Bool(options["includePlanetary"]) && (v.Bool(buy["planetary"]) || v.Bool(sell["planetary"])) || !v.Bool(options["includeCarriers"]) && (v.Bool(buy["carrier"]) || v.Bool(sell["carrier"])) {
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
	return M{"key": key, "name": source["name"], "buy": buy, "sell": sell, "quantity": quantity, "buyPrice": source["price"], "sellPrice": target["price"], "supply": source["supply"], "demand": target["demand"], "purchaseCost": purchase, "revenue": revenue, "grossMargin": margin, "netEstimate": net, "profitPerTonne": n(target, "price") - n(source, "price"), "distanceLy": distance, "minutes": options["minutesPerLeg"], "profitPerHour": net * 60 / n(options, "minutesPerLeg")}
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
	for _, outward := range legs {
		buy, sell := v.Obj(outward["buy"]), v.Obj(outward["sell"])
		if !v.Bool(options["roundTrip"]) {
			rows = append(rows, M{"id": v.Str(buy["id"]) + "|" + v.Str(sell["id"]) + "|" + v.Str(outward["key"]), "legs": []M{outward}, "netEstimate": outward["netEstimate"], "grossMargin": outward["grossMargin"], "minutes": outward["minutes"], "profitPerHour": outward["profitPerHour"]})
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
					net := n(outward, "netEstimate") + n(inward, "netEstimate")
					minutes := n(options, "minutesPerLeg") * 2
					rows = append(rows, M{"id": v.Str(buy["id"]) + "|" + v.Str(sell["id"]) + "|" + v.Str(outward["key"]) + "|" + key, "legs": []M{outward, inward}, "netEstimate": net, "grossMargin": n(outward, "grossMargin") + n(inward, "grossMargin"), "minutes": minutes, "profitPerHour": net * 60 / minutes})
				}
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if n(rows[i], "profitPerHour") != n(rows[j], "profitPerHour") {
			return n(rows[i], "profitPerHour") > n(rows[j], "profitPerHour")
		}
		return n(rows[i], "netEstimate") > n(rows[j], "netEstimate")
	})
	if len(rows) > 60 {
		rows = rows[:60]
	}
	return rows
}
func (s *Searcher) FindRoutes(ctx context.Context, input, state M) (M, error) {
	options := RouteOptions(input, v.Obj(state["commander"]))
	keys := v.Arr(options["resourceKeys"])
	if v.Str(options["system"]) == "" || len(keys) == 0 {
		return nil, errors.New("Укажите исходную систему и товары.")
	}
	out := M{"running": false, "options": options, "results": []M{}, "checked": 0, "total": len(keys) * 2, "updatedAt": v.ISO(s.Now()), "coverageLimited": false, "assumptions": "Спрос ограничивает объём с выбранным запасом. Доход в час использует введённое время рейса. Доставка к первой станции не включена. Котировки могут измениться."}
	markets := map[string]M{}
	errs := []string{}
	for _, rawKey := range keys {
		key := v.Str(rawKey)
		markets[key] = M{}
		for _, mode := range []string{"buy", "sell"} {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			request := v.Copy(options)
			request["mode"] = mode
			request["resourceKey"] = key
			request["quantity"] = options["cargoCapacity"]
			request["useCargo"] = false
			result, err := s.Search(ctx, request, state)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			markets[key][mode] = result["results"]
			if v.Bool(result["coverageLimited"]) {
				out["coverageLimited"] = true
			}
			if v.Str(result["error"]) != "" {
				errs = append(errs, v.Str(result["error"]))
			}
			plus(out, "checked", 1)
			out["results"] = BuildRoutes(markets, options)
			if s.OnUpdate != nil {
				s.OnUpdate(v.Copy(out))
			}
		}
	}
	out["status"] = "Торговые рейсы рассчитаны по доступным котировкам."
	if len(v.Rows(out["results"])) == 0 {
		out["status"] = "Прибыльных рейсов с этими ограничениями не найдено."
	}
	if len(errs) > 0 {
		out["error"] = strings.Join(errs, " · ")
	}
	out["updatedAt"] = v.ISO(time.Now())
	return out, nil
}
