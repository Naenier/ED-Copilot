package trading

import (
	"context"
	"math"
	"net/http"
	"reflect"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func detailsMarket(id, system, station, key string, price, supply, demand, x float64) M {
	return M{"id": id, "marketId": id, "system": system, "station": station, "type": "Coriolis", "pad": "L", "distanceLs": 420, "distanceLy": 5, "coordinates": []float64{x, 0, 0}, "updatedAt": "2026-11-08T12:00:00Z", "ageHours": 1, "source": "Spansh", "quotes": []M{{"key": key, "name": Name(key, nil), "price": price, "supply": supply, "demand": demand, "updatedAt": "2026-11-08T12:00:00Z", "ageHours": 1}}}
}
func TestDetailedRouteQuantityProvenanceAndIndependentDTO(t *testing.T) {
	o := RouteOptions(M{"cargoCapacity": 100, "credits": 10500, "operatingCostPerLeg": 500, "demandRatio": 4}, M{})
	buy, sell := detailsMarket("A", "Alpha", "Source", "gold", 100, 80, 0, 0), detailsMarket("B", "Beta", "Buyer", "gold", 200, 0, 200, 30)
	before := v.Copy(buy)
	leg := PairLeg(buy, sell, "gold", o)
	for key, want := range map[string]float64{"quantity": 50, "purchaseCost": 5000, "revenue": 10000, "grossMargin": 5000, "operatingCost": 500, "netEstimate": 4500, "marginPercent": 100, "profitPerHour": 18000, "safeDemand": 50, "budgetAfter": 15000} {
		checkN(t, leg[key], want)
	}
	if !reflect.DeepEqual(v.Arr(leg["limitingFactors"]), []any{"safeDemand"}) || v.At(leg, "buy", "station") != "Source" || v.At(leg, "sell", "system") != "Beta" || v.At(leg, "buy", "observedAt") != "2026-11-08T12:00:00Z" {
		t.Fatal("lost detailed route fields", leg)
	}
	checkN(t, v.Rows(v.At(leg, "buy", "quotes"))[0]["quantity"], 50)
	checkN(t, v.At(leg, "limits", "capital"), 100)
	v.Obj(leg["buy"])["station"] = "changed"
	v.Rows(v.At(leg, "buy", "quotes"))[0]["price"] = 999
	if !reflect.DeepEqual(before, v.Copy(buy)) {
		t.Fatal("result DTO aliases input observations")
	}
}
func TestDetailedRouteFiltersAreEnforcedWhenPairingCachedQuotes(t *testing.T) {
	o := RouteOptions(M{"cargoCapacity": 100, "credits": 100000, "minSupply": 50, "minDemand": 100, "shipPad": "L", "now": "2026-11-08T13:00:00Z"}, M{})
	buy, sell := detailsMarket("A", "Alpha", "Source", "gold", 100, 100, 0, 0), detailsMarket("B", "Beta", "Buyer", "gold", 200, 0, 1000, 30)
	for name, change := range map[string]M{"small pad": {"pad": "M"}, "arrival": {"distanceLs": 20000}, "radius": {"distanceLy": 200}, "stale": {"updatedAt": "2026-11-01T00:00:00Z"}, "future": {"updatedAt": "2026-11-09T00:00:00Z"}, "planetary": {"planetary": true}} {
		t.Run(name, func(t *testing.T) {
			options := v.Copy(o)
			if name == "planetary" {
				options["includePlanetary"] = false
			}
			if PairLeg(v.Merge(v.Copy(buy), change), sell, "gold", options) != nil {
				t.Fatal("filter not enforced", name)
			}
		})
	}
	low := v.Copy(buy)
	v.Rows(low["quotes"])[0]["supply"] = 49
	if PairLeg(low, sell, "gold", o) != nil {
		t.Fatal("minimum supply ignored")
	}
	low = v.Copy(sell)
	v.Rows(low["quotes"])[0]["demand"] = 99
	if PairLeg(buy, low, "gold", o) != nil {
		t.Fatal("minimum demand ignored")
	}
	o["marketType"], o["includeCarriers"] = "carriers", true
	if PairLeg(buy, sell, "gold", o) != nil {
		t.Fatal("carrier-only filter returned a station")
	}
}
func TestDetailedRoundTripReinvestsOnlyRealizedNetAndBoundsResults(t *testing.T) {
	o := RouteOptions(M{"cargoCapacity": 10, "credits": 1000, "operatingCostPerLeg": 100, "roundTrip": true, "demandRatio": 1}, M{})
	market := map[string]M{"gold": {"buy": []M{detailsMarket("A", "A", "A", "gold", 100, 100, 0, 0)}, "sell": []M{detailsMarket("B", "B", "B", "gold", 200, 0, 100, 30)}}, "silver": {"buy": []M{detailsMarket("B", "B", "B", "silver", 200, 100, 0, 30)}, "sell": []M{detailsMarket("A", "A", "A", "silver", 250, 0, 100, 0)}}}
	rows := BuildRoutes(market, o)
	if len(rows) != 2 {
		t.Fatal("roundtrip candidates", rows)
	}
	var route M
	for _, candidate := range rows {
		if v.Rows(candidate["legs"])[0]["key"] == "gold" {
			route = candidate
		}
	}
	legs := v.Rows(route["legs"])
	checkN(t, legs[0]["quantity"], 9)
	checkN(t, legs[0]["netEstimate"], 800)
	checkN(t, legs[1]["budgetBefore"], 1800)
	checkN(t, legs[1]["quantity"], 8)
	checkN(t, route["netEstimate"], 1100)
	checkN(t, route["minimumCapital"], 1000)
	checkN(t, route["distanceLy"], 60)
	if route["roundTrip"] != true || route["timeBasis"] != "manual" {
		t.Fatal("route basis lost")
	}
	options := RouteOptions(M{"cargoCapacity": 1, "credits": 10000, "orderBy": "distance", "demandRatio": 1}, M{})
	buys := []M{}
	for i := 1; i <= 90; i++ {
		m := detailsMarket(string(rune(1000+i)), "A", "A", "gold", 100, 100, 0, float64(i))
		m["distanceLy"] = 0
		buys = append(buys, m)
	}
	result := BuildRoutes(map[string]M{"gold": {"buy": buys, "sell": []M{detailsMarket("Z", "B", "B", "gold", 200, 0, 100, 0)}}}, options)
	if len(result) != 60 || n(result[0], "distanceLy") != 1 || n(result[59], "distanceLy") != 60 {
		t.Fatal("top60 distance ordering lost", len(result))
	}
}
func TestRouteSearchNeverInventsShipCapacityOrBalance(t *testing.T) {
	o := RouteOptions(nil, M{})
	if o["credits"] != nil || o["cargoCapacity"] != nil {
		t.Fatal("unknown commander values replaced with defaults", o)
	}
	o = RouteOptions(nil, M{"cargoCapacity": 100, "cargoTons": 40})
	checkN(t, o["cargoCapacity"], 60)
	calls := 0
	s := NewSearcher(doerFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled }))
	_, err := s.FindRoutes(context.Background(), M{"system": "A", "resourceKeys": []string{"gold"}, "cargoCapacity": 100}, M{})
	if err == nil || calls != 0 {
		t.Fatal("unknown balance reached external transport", err, calls)
	}
	s.Now = func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) }
	_, err = s.FindRoutes(context.Background(), M{"system": "A", "resourceKeys": []string{"gold"}}, M{"commander": M{"system": "A", "coordinates": []float64{0, 0, 0}, "cargoCapacity": 100, "cargoTons": 100}, "pilot": M{"credits": 5000}})
	if err == nil || calls != 0 {
		t.Fatal("full hold was invented as free capacity", err, calls)
	}
	if math.IsNaN(n(o, "cargoCapacity")) {
		t.Fatal("invalid normalized capacity")
	}
	previous := RouteOptions(M{"shipPad": "L"}, M{})
	previous["minPad"] = "M"
	if next := RouteOptions(previous, M{}); next["minPad"] != "M" || next["shipPad"] != "M" {
		t.Fatal("stale shipPad alias overrode a changed canonical minimum", next)
	}
}
