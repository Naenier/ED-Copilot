package trading

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	stationRouteNearLy   = 250
	stationRoutePageSize = 32
	stationRoutePages    = 6
	stationRouteLimit    = 60
)

var stationRoutePowers = []string{
	"A. Lavigny-Duval", "Aisling Duval", "Archon Delaine", "Denton Patreus", "Edmund Mahon", "Felicia Winters",
	"Jerome Archer", "Li Yong-Rui", "Nakato Kaine", "Pranav Antal", "Yuri Grom", "Zemina Torval",
}

func canonicalRoutePower(raw any) string {
	key := v.Key(raw)
	if key == "arlavignyduval" || key == "arissalavignyduval" {
		key = "alavignyduval"
	}
	if key == "none" {
		return "none"
	}
	for _, power := range stationRoutePowers {
		if key == v.Key(power) {
			return power
		}
	}
	return ""
}

func canonicalRouteState(raw any) string {
	switch v.Key(raw) {
	case "none", "unoccupied":
		return "Unoccupied"
	case "exploited":
		return "Exploited"
	case "fortified":
		return "Fortified"
	case "stronghold":
		return "Stronghold"
	case "contested":
		return "Contested"
	default:
		return ""
	}
}

// StationRouteOptions is the search form's contract. Distance from the
// reference system is a search bound, separate from the selected distance
// between trading ports.
func StationRouteOptions(input, commander M) M {
	out := M{
		"system": strings.TrimSpace(v.Text(input["system"], commander["system"])), "cargoCapacity": nil,
		"maxDistance": 20.0, "maxAgeHours": 24.0, "minPad": v.Text(ShipPad(commander), "L"),
		"maxArrivalLs": 1000.0, "allowSurface": false, "minSupply": 10000.0, "minDemand": 10000.0,
		"roundTrip": true, "orderBy": "profit", "automatic": v.Bool(input["automatic"]),
		"sourcePower": canonicalRoutePower(input["sourcePower"]), "targetPower": canonicalRoutePower(input["targetPower"]),
		"sourcePowerState": canonicalRouteState(input["sourcePowerState"]), "targetPowerState": canonicalRouteState(input["targetPowerState"]),
	}
	// Internally match the provider's abbreviated name, while retaining the
	// canonical full spelling in the application-facing selector.
	for _, field := range []string{"sourcePower", "targetPower"} {
		if out[field] == "A. Lavigny-Duval" {
			out[field] = "Arissa Lavigny-Duval"
		}
	}
	if value, ok := v.Number(commander["cargoCapacity"]); ok {
		out["cargoCapacity"] = math.Floor(v.Clamp(value, 0, 25000))
	}
	for _, bound := range []struct {
		field    string
		min, max float64
	}{
		{"cargoCapacity", 0, 25000}, {"maxDistance", .01, 5000}, {"maxAgeHours", 1, 720},
		{"maxArrivalLs", 0, 1e7}, {"minSupply", 0, 1e9}, {"minDemand", 0, 1e9},
	} {
		if value, ok := v.Number(input[bound.field]); ok {
			value = v.Clamp(value, bound.min, bound.max)
			if bound.field == "cargoCapacity" || bound.field == "minSupply" || bound.field == "minDemand" {
				value = math.Floor(value)
			}
			out[bound.field] = value
		}
	}
	for _, field := range []string{"roundTrip", "allowSurface"} {
		if value, ok := input[field].(bool); ok {
			out[field] = value
		}
	}
	switch input["minPad"] {
	case "S", "M", "L":
		out["minPad"] = input["minPad"]
	}
	switch input["orderBy"] {
	case "profit", "profit-per-ton", "distance", "margin":
		out["orderBy"] = input["orderBy"]
	}
	return out
}

type stationRouteMarket struct {
	port        M
	coordinates []float64
	id          string
	buys, sells map[string]M
}

type stationRouteLeg struct {
	buy, sell      M
	quantity, gain float64
}

func (l stationRouteLeg) unitGain() float64 {
	return n(l.sell, "price") - n(l.buy, "price")
}

func (l stationRouteLeg) margin() float64 {
	if l.quantity == 0 {
		return 0
	}
	return l.unitGain() * 100 / n(l.buy, "price")
}

type stationRouteMatch struct {
	source, target *stationRouteMarket
	out, back      stationRouteLeg
	distance       float64
}

func (m stationRouteMatch) id() string {
	return m.source.id + "|" + m.target.id + "|" + v.Str(m.out.buy["key"]) + "|" + v.Str(m.back.buy["key"])
}

func (m stationRouteMatch) profit() float64 { return m.out.gain + m.back.gain }

func (m stationRouteMatch) rank(order any) float64 {
	switch order {
	case "distance":
		return -m.distance
	case "profit-per-ton":
		return m.out.unitGain() + m.back.unitGain()
	case "margin":
		return m.out.margin() + m.back.margin()
	default:
		return m.profit()
	}
}

func stationRouteTravelOptions(options M, radius float64) M {
	return M{
		"radiusLy": radius, "minPad": options["minPad"], "maxArrivalLs": options["maxArrivalLs"],
		"maxAgeHours": options["maxAgeHours"], "marketType": "stations", "includeCarriers": false,
		"includePlanetary": options["allowSurface"],
	}
}

func stationRouteLimitMatch(port M, power, state string) bool {
	controller := canonicalRoutePower(port["controllingPower"])
	controlState := canonicalRouteState(port["powerplayState"])
	if power == "none" {
		// A missing Power on an observed system is distinct from having no
		// political data for it at all.
		if !v.Bool(port["powerKnown"]) || controller != "" && controller != "none" {
			return false
		}
	} else if power != "" && controller != power {
		return false
	}
	return state == "" || controlState == state
}

func stationRouteMarketFrom(raw, options, travel M, now time.Time, origin []float64, goods map[string]M) *stationRouteMarket {
	port := marketStation(raw, travel, now, origin)
	if len(port) == 0 || v.Str(port["system"]) == "" || v.Str(port["station"]) == "" {
		return nil
	}
	if port["distanceLs"] == nil && n(travel, "maxArrivalLs") > 0 {
		return nil
	}
	controller := canonicalRoutePower(raw["system_controlling_power"])
	if controller == "A. Lavigny-Duval" {
		controller = "Arissa Lavigny-Duval"
	}
	port["controllingPower"], port["powerplayState"] = controller, canonicalRouteState(raw["system_power_state"])
	_, controllerKnown := raw["system_controlling_power"]
	port["powerKnown"] = controllerKnown || port["powerplayState"] != ""
	market := &stationRouteMarket{port: port, coordinates: Coords(port["coordinates"]), id: v.Str(port["id"]), buys: map[string]M{}, sells: map[string]M{}}
	if len(market.coordinates) != 3 {
		return nil
	}
	for _, rawQuote := range v.Rows(raw["market"]) {
		sourceName := v.First(rawQuote["commodity"], rawQuote["symbol"], rawQuote["name"])
		key := v.Key(sourceName)
		if key == "" {
			continue
		}
		good := goods[key]
		if good == nil {
			good = Good(sourceName, nil)
			goods[key] = good
		}
		key = v.Str(good["key"])
		if good["kind"] == "raw" || key == "" || key == "drones" {
			continue
		}
		quote := M{"key": key, "name": good["name"], "english": good["english"], "category": rawQuote["category"],
			"updatedAt": port["updatedAt"], "ageHours": port["ageHours"]}
		supply, supplyKnown := v.Number(rawQuote["supply"])
		price, priceKnown := v.Number(rawQuote["buy_price"])
		if supplyKnown && supply >= math.Max(1, n(options, "minSupply")) && priceKnown && price > 0 {
			market.buys[key] = v.Merge(v.Copy(quote), M{"price": price, "supply": supply})
		}
		demand, demandKnown := v.Number(rawQuote["demand"])
		price, priceKnown = v.Number(rawQuote["sell_price"])
		// An ordinary market may report a valid sell quote and zero for
		// unlimited demand. Fleet carriers (not searched here) use zero for
		// a depleted buy order.
		if demandKnown && (demand == 0 || demand >= math.Max(1, n(options, "minDemand"))) && priceKnown && price > 0 {
			market.sells[key] = v.Merge(v.Copy(quote), M{"price": price, "demand": demand, "demandUnlimited": demand == 0})
		}
	}
	return market
}

func bestStationRouteLeg(buy, sell *stationRouteMarket, cargo float64, order string) stationRouteLeg {
	var best stationRouteLeg
	bestScore := -1.0
	for key, source := range buy.buys {
		target := sell.sells[key]
		margin := n(target, "price") - n(source, "price")
		if margin <= 0 {
			continue
		}
		quantity := math.Min(cargo, n(source, "supply"))
		if !v.Bool(target["demandUnlimited"]) {
			quantity = math.Min(quantity, n(target, "demand"))
		}
		quantity = math.Floor(quantity)
		leg := stationRouteLeg{buy: source, sell: target, quantity: quantity, gain: quantity * margin}
		score := leg.gain
		switch order {
		case "profit-per-ton":
			score = margin
		case "margin":
			score = leg.margin()
		}
		if score > bestScore || score == bestScore && (leg.gain > best.gain || leg.gain == best.gain && key < v.Str(best.buy["key"])) {
			best, bestScore = leg, score
		}
	}
	return best
}

func stationRouteLegDTO(leg stationRouteLeg, buy, sell M, distance float64) M {
	outbound := routeMarket(buy, leg.buy, leg.quantity)
	inbound := routeMarket(sell, leg.sell, leg.quantity)
	inbound["demandUnlimited"] = leg.sell["demandUnlimited"]
	purchase, revenue := leg.quantity*n(leg.buy, "price"), leg.quantity*n(leg.sell, "price")
	return M{
		"key": leg.buy["key"], "name": leg.buy["name"], "english": leg.buy["english"], "category": leg.buy["category"],
		"buy": outbound, "sell": inbound, "quantity": leg.quantity, "buyPrice": leg.buy["price"], "sellPrice": leg.sell["price"],
		"supply": leg.buy["supply"], "demand": leg.sell["demand"], "demandUnlimited": leg.sell["demandUnlimited"],
		"purchaseCost": purchase, "revenue": revenue, "netEstimate": leg.gain, "grossMargin": leg.gain,
		"marginPercent": leg.margin(), "profitPerTonne": leg.unitGain(), "distanceLy": distance,
		"approachDistanceLy": buy["distanceLy"],
	}
}

func stationRouteResult(match stationRouteMatch, options M) M {
	legs := []M{stationRouteLegDTO(match.out, match.source.port, match.target.port, match.distance)}
	totalDistance := match.distance
	if match.back.quantity > 0 {
		legs = append(legs, stationRouteLegDTO(match.back, match.target.port, match.source.port, match.distance))
		totalDistance *= 2
	}
	updated := v.Time(match.source.port["updatedAt"])
	if at := v.Time(match.target.port["updatedAt"]); at.Before(updated) {
		updated = at
	}
	result := M{
		"id": match.id(), "legs": legs, "from": match.source.port, "to": match.target.port,
		"roundTrip": len(legs) == 2, "netEstimate": match.profit(), "grossMargin": match.profit(),
		"marginPerTon": match.out.unitGain() + match.back.unitGain(), "distanceLy": totalDistance, "legDistanceLy": match.distance,
		"approachDistanceLy": match.source.port["distanceLy"], "originDistanceLy": match.source.port["distanceLy"],
		"updatedAt": v.ISO(updated), "options": v.Copy(options),
	}
	AddRoutePowerBonus(result)
	return result
}

func buildStationRoutes(ctx context.Context, sources, targets []*stationRouteMarket, options M) ([]M, error) {
	rows := make([]stationRouteMatch, 0, stationRouteLimit*2)
	capacity, maximum, order := n(options, "cargoCapacity"), n(options, "maxDistance"), v.Str(options["orderBy"])
	compare := func(a, b stationRouteMatch) bool {
		if a.rank(order) != b.rank(order) {
			return a.rank(order) > b.rank(order)
		}
		if a.profit() != b.profit() {
			return a.profit() > b.profit()
		}
		return a.id() < b.id()
	}
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, target := range targets {
			if source.id == target.id {
				continue
			}
			distance := Distance(source.coordinates, target.coordinates)
			if distance > maximum {
				continue
			}
			out := bestStationRouteLeg(source, target, capacity, order)
			if out.quantity <= 0 {
				continue
			}
			match := stationRouteMatch{source: source, target: target, out: out, distance: distance}
			if v.Bool(options["roundTrip"]) {
				match.back = bestStationRouteLeg(target, source, capacity, order)
			}
			rows = append(rows, match)
			if len(rows) == stationRouteLimit*2 {
				sort.Slice(rows, func(i, j int) bool { return compare(rows[i], rows[j]) })
				rows = rows[:stationRouteLimit]
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return compare(rows[i], rows[j]) })
	results := make([]M, 0, min(len(rows), stationRouteLimit))
	for _, row := range rows[:min(len(rows), stationRouteLimit)] {
		results = append(results, stationRouteResult(row, options))
	}
	return results, nil
}
