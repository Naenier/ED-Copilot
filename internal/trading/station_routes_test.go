package trading

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestStationRouteOptionsMatchesForm(t *testing.T) {
	bad := M{"credits": 1, "resourceKeys": []string{"gold"}, "radiusLy": 5000, "minNetProfit": 999999,
		"demandRatio": 40, "includeCarriers": true, "minutesPerLeg": 5, "maxLegLy": 6000, "marketType": "all"}
	station := M{"system": "Brestla", "cargoCapacity": 1216, "cargoTons": 1000, "ship": "Panther Clipper Mk II"}
	options := StationRouteOptions(bad, station)
	for key := range bad {
		if _, exists := options[key]; exists {
			t.Fatalf("obsolete filter %q persisted: %v", key, options)
		}
	}
	if options["system"] != "Brestla" || options["cargoCapacity"] != float64(1216) || options["minPad"] != "L" ||
		options["allowSurface"] != false || options["roundTrip"] != true || options["sourcePower"] != "" {
		t.Fatalf("incorrect search form defaults: %v", options)
	}
}

func TestStationRoutesFilterRolesAndReportMarketFields(t *testing.T) {
	now := time.Date(2026, 11, 14, 18, 0, 0, 0, time.UTC)
	station := func(id string, x int, at time.Time, power, politicalState, kind string, ls int, pad string, goods ...M) M {
		return M{
			"market_id": id, "name": id + " Port", "system_name": id, "type": kind, "system_x": x, "system_y": 0, "system_z": 0,
			"distance_to_arrival": ls, "market_updated_at": v.ISO(at), "system_controlling_power": power,
			"system_power_state": politicalState, "market": goods, "large_pads": v.Nullable(1, pad == "L"), "small_pads": 1,
		}
	}
	goldBuy := M{"commodity": "Gold", "category": "Metals", "buy_price": 1000, "supply": 150}
	silverSell := M{"commodity": "Silver", "category": "Metals", "sell_price": 1800, "demand": 150}
	goldSell := M{"commodity": "Gold", "category": "Metals", "sell_price": 4000, "demand": 0}
	silverBuy := M{"commodity": "Silver", "category": "Metals", "buy_price": 1200, "supply": 80}
	a := station("Alpha", 2, now.Add(-time.Hour), "A. Lavigny-Duval", "Fortified", "Orbis Starport", 90, "L", goldBuy, silverSell)
	b := station("Beta", 17, now.Add(-2*time.Hour), "Zemina Torval", "Stronghold", "Dodec Starport", 330, "L", goldSell, silverBuy)
	wrongPower := station("Wrong Power", 5, now, "Zemina Torval", "Fortified", "Orbis Starport", 90, "L", goldBuy)
	wrongState := station("Wrong State", 5, now, "A. Lavigny-Duval", "Exploited", "Orbis Starport", 90, "L", goldBuy)
	far := station("Far", 25, now, "Zemina Torval", "Stronghold", "Orbis Starport", 90, "L", goldSell)
	stale := station("Stale", 10, now.Add(-25*time.Hour), "Zemina Torval", "Stronghold", "Orbis Starport", 90, "L", goldSell)
	farArrival := station("Far Arrival", 10, now, "Zemina Torval", "Stronghold", "Orbis Starport", 1500, "L", goldSell)
	surface := station("Surface", 11, now, "Zemina Torval", "Stronghold", "Planetary Port", 90, "L", goldSell)
	small := station("Small", 11, now, "Zemina Torval", "Stronghold", "Outpost", 90, "S", goldSell)
	finite := station("Finite", 11, now, "Zemina Torval", "Stronghold", "Outpost", 90, "L", v.Merge(v.Copy(goldSell), M{"demand": 10}))
	calls := 0
	search := NewSearcher(doerFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		input := M{}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		filters := v.Obj(input["filters"])
		if request.URL.Path != "/api/stations/search" || v.At(filters, "is_planetary", "value") != false ||
			v.At(filters, "has_large_pad", "value") != true || len(v.Arr(v.At(filters, "market_updated_at", "value"))) != 2 {
			t.Fatalf("missing provider filter: %s %v", request.URL, filters)
		}
		powers := v.Arr(v.At(filters, "system_controlling_power", "value"))
		states := v.Arr(v.At(filters, "system_power_state", "value"))
		if calls == 1 && len(powers) == 1 && powers[0] == "A. Lavigny-Duval" && len(states) == 1 && states[0] == "Fortified" {
			return response(M{"count": 3, "results": []M{a, wrongPower, wrongState}}), nil
		}
		if calls == 2 && len(powers) == 1 && powers[0] == "Zemina Torval" && len(states) == 1 && states[0] == "Stronghold" {
			rows := []M{b, far, stale, farArrival, surface, small, finite}
			return response(M{"count": len(rows), "results": rows}), nil
		}
		t.Fatalf("incorrect political role query: %v", filters)
		return nil, nil
	}))
	search.Now = func() time.Time { return now }
	options := M{"system": "Brestla", "cargoCapacity": 100, "minSupply": 70, "minDemand": 100,
		"sourcePower": "Arissa Lavigny-Duval", "sourcePowerState": "Fortified", "targetPower": "Zemina Torval", "targetPowerState": "Stronghold"}
	out, err := search.FindStationRoutes(context.Background(), options, M{"commander": M{"system": "Brestla", "coordinates": []float64{0, 0, 0}}})
	rows := v.Rows(out["results"])
	if err != nil || len(rows) != 1 || calls != 2 {
		t.Fatalf("wrong route set: %v, calls=%d, err=%v", out, calls, err)
	}
	route := rows[0]
	legs := v.Rows(route["legs"])
	if len(legs) != 2 || route["roundTrip"] != true || route["updatedAt"] != v.ISO(now.Add(-2*time.Hour)) ||
		route["profitPerHour"] != nil || legs[0]["demandUnlimited"] != true ||
		v.At(legs[0], "buy", "controllingPower") != "Arissa Lavigny-Duval" || v.At(legs[0], "sell", "powerplayState") != "Stronghold" {
		t.Fatalf("wrong loop or market metadata: %v", route)
	}
	checkN(t, route["netEstimate"], 348000)
	checkN(t, route["marginPerTon"], 3600)
	checkN(t, route["legDistanceLy"], 15)
	checkN(t, route["originDistanceLy"], 2)
	checkN(t, legs[0]["marginPercent"], 300)
	checkN(t, legs[1]["quantity"], 80)
}

func TestStationRouteProgramSupportsSurfaceAndKeepsSingleLeg(t *testing.T) {
	now := time.Date(2026, 11, 14, 18, 0, 0, 0, time.UTC)
	src := M{"market_id": "buy", "name": "Ground", "system_name": "Alpha", "type": "Planetary Port", "has_large_pad": true,
		"system_x": 0, "system_y": 0, "system_z": 0, "distance_to_arrival": 10, "market_updated_at": v.ISO(now),
		"market": []M{{"commodity": "Silver", "buy_price": 10, "supply": 20}}}
	dst := v.Merge(v.Copy(src), M{"market_id": "sell", "name": "Orbit", "system_name": "Beta", "type": "Coriolis", "system_x": 10,
		"market": []M{{"commodity": "Silver", "sell_price": 100, "demand": 2}}})
	options := StationRouteOptions(M{"cargoCapacity": 10, "allowSurface": true, "minSupply": 1, "minDemand": 1, "roundTrip": true}, nil)
	travel := stationRouteTravelOptions(options, 100)
	goods := map[string]M{}
	parse := func(raw M) *stationRouteMarket {
		return stationRouteMarketFrom(raw, options, travel, now, []float64{0, 0, 0}, goods)
	}
	a, b := parse(src), parse(dst)
	if a == nil || b == nil || !v.Bool(a.port["planetary"]) || stationRouteLimitMatch(b.port, "none", "") {
		t.Fatalf("expected both ports with unknown Power data: %v, %v", a, b)
	}
	rows, err := buildStationRoutes(context.Background(), []*stationRouteMarket{a}, []*stationRouteMarket{b}, options)
	if err != nil || len(rows) != 1 || rows[0]["roundTrip"] != false || len(v.Rows(rows[0]["legs"])) != 1 {
		t.Fatalf("round-trip toggle hid profitable outbound: %v, %v", rows, err)
	}
	checkN(t, v.Rows(rows[0]["legs"])[0]["quantity"], 2)
}
