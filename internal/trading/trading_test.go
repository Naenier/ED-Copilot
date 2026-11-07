package trading

import (
	"context"
	"encoding/json"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"
)

func checkN(t *testing.T, raw any, want float64) {
	t.Helper()
	if math.Abs(v.Num(raw)-want) > 1e-9 {
		t.Fatalf("got %v, want %v", raw, want)
	}
}
func event(kind string, values M) M {
	return v.Merge(M{"event": kind, "timestamp": "2026-11-07T12:00:00Z"}, values)
}
func TestLedgerFIFOUnknownAndOwnCarrier(t *testing.T) {
	s := New(M{})
	cmd := M{"vehicle": "ship"}
	c := M{"commander": cmd, "carrierId": "42"}
	s.Apply(event("MarketBuy", M{"Type": "Gold", "Count": 10, "TotalCost": 1000}), c)
	s.Apply(event("MarketBuy", M{"Type": "Gold", "Count": 5, "TotalCost": 1000}), c)
	result := s.Apply(event("MarketSell", M{"Type": "Gold", "Count": 12, "TotalSale": 3600}), c)
	checkN(t, result["minedRevenue"], 0)
	snap := s.Snapshot(cmd)
	checkN(t, v.At(snap, "totals", "knownCost"), 1400)
	checkN(t, v.At(snap, "totals", "grossMargin"), 2200)
	inventory := v.Rows(snap["inventory"])
	if len(inventory) != 1 {
		t.Fatal(inventory)
	}
	checkN(t, inventory[0]["count"], 3)
	checkN(t, inventory[0]["unitCost"], 200)
	s.Apply(event("CargoTransfer", M{"Transfers": []M{{"Type": "Gold", "Count": 3, "Direction": "tocarrier"}}}), c)
	if len(v.Rows(s.Snapshot(cmd)["inventory"])) != 0 {
		t.Fatal("cargo transfer retained ship inventory")
	}
	s.Apply(event("MarketBuy", M{"Type": "Gold", "Count": 2, "TotalCost": 1}), M{"commander": cmd, "carrierId": "42", "ownMarket": true})
	s.Apply(event("MarketSell", M{"Type": "Gold", "Count": 2, "TotalSale": 600}), c)
	checkN(t, s.Totals["knownCost"], 1800)
	checkN(t, s.Totals["revenue"], 4200)
	checkN(t, s.Totals["purchaseCost"], 2000)
	checkN(t, s.Totals["internalTons"], 5)
	s.Apply(event("CollectCargo", M{"Type": "Gold", "Count": 2}), c)
	s.Apply(event("MarketSell", M{"Type": "Gold", "Count": 2, "TotalSale": 600}), c)
	checkN(t, s.Totals["unknownCostTons"], 2)
}
func TestLedgerMinedColonizationSRVAndReplayBoundaries(t *testing.T) {
	s := New(M{})
	ship := M{"commander": M{"vehicle": "ship"}}
	s.Apply(event("MiningRefined", M{"Type": "Platinum", "Count": 10}), ship)
	s.Apply(event("ColonisationContribution", M{"Contributions": []M{{"Name": "$platinum_name;", "Amount": 4}}}), ship)
	s.Apply(event("Cargo", M{"Vessel": "SRV", "Inventory": []M{{"Name": "Gold", "Count": 2}}}), M{"commander": M{"vehicle": "srv"}})
	checkN(t, v.Rows(s.Snapshot(M{"vehicle": "ship"})["inventory"])[0]["count"], 6)
	checkN(t, v.Rows(s.Snapshot(M{"vehicle": "srv"})["inventory"])[0]["count"], 2)
	r := s.Apply(event("MarketSell", M{"Type": "Platinum", "Count": 3, "TotalSale": 3000}), ship)
	checkN(t, r["minedRevenue"], 3000)
	checkN(t, s.Totals["grossMargin"], 3000)
	s.Apply(event("MarketBuy", M{"Type": "Gold", "Count": 1, "TotalCost": 100}), M{"commander": M{"vehicle": "ship"}, "countInPeriod": false})
	checkN(t, s.Totals["boughtTons"], 0)
	if Name("$aluminium_name;", nil) != "Алюминий" {
		t.Fatal("construction commodity untranslated")
	}
	s.Apply(event("Died", M{}), ship)
	if len(v.Rows(s.Snapshot(M{"vehicle": "ship"})["inventory"])) != 0 {
		t.Fatal("death did not clear cargo")
	}
}
func TestBasketAndMarketPriceDirection(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	state := M{"commander": M{"cargo": []M{{"key": "gold", "count": 10}, {"key": "drones", "count": 4}}}}
	options := NormalizeOptions(M{"mode": "sell", "quantity": 50, "system": "Sol"})
	basket, _, e := BuildBasket(state, options)
	if e != nil || len(basket) != 1 {
		t.Fatalf("basket=%v,err=%v", basket, e)
	}
	station := M{"market_id": json.Number("18446744073709551615"), "name": "Test station", "system_name": "Sol", "type": "Coriolis Starport", "has_large_pad": true, "market_updated_at": v.ISO(now.Add(-time.Hour)), "system_x": 0, "system_y": 0, "system_z": 0, "distance_to_arrival": 30, "market": []M{{"commodity": "Gold", "sell_price": 300, "buy_price": 500, "demand": 20, "supply": 100}}}
	r := StationResult(station, basket, options, now, []float64{0, 0, 0})
	if r == nil {
		t.Fatal("valid station removed")
	}
	checkN(t, r["estimatedTotal"], 3000)
	if !v.Bool(r["bulkTaxRisk"]) {
		t.Fatal("bulk demand risk missed")
	}
	if r["marketId"] != "18446744073709551615" {
		t.Fatal("ID64 rounded")
	}
	options = NormalizeOptions(M{"mode": "buy", "resourceKey": "gold", "quantity": 5, "useCargo": false})
	basket, _, _ = BuildBasket(state, options)
	r = StationResult(station, basket, options, now, []float64{0, 0, 0})
	checkN(t, r["estimatedTotal"], 2500)
	for _, change := range []M{{"market_updated_at": v.ISO(now.Add(-25 * time.Hour))}, {"market_updated_at": v.ISO(now.Add(11 * time.Minute))}, {"has_large_pad": false}, {"system_x": 10000}, {"distance_to_arrival": 20000}} {
		copy := v.Merge(v.Copy(station), change)
		if StationResult(copy, basket, options, now, []float64{0, 0, 0}) != nil {
			t.Fatal("invalid/stale station accepted", change)
		}
	}
	carrier := v.Merge(v.Copy(station), M{"type": "Fleet Carrier", "market_updated_at": v.ISO(now.Add(-7 * time.Hour))})
	if StationResult(carrier, basket, options, now, []float64{0, 0, 0}) != nil {
		t.Fatal("carrier freshness gate missed")
	}
}
func TestRouteFundingDemandAndRoundTrip(t *testing.T) {
	options := RouteOptions(M{"cargoCapacity": 10, "credits": 1000, "operatingCostPerLeg": 0, "roundTrip": true, "demandRatio": 4}, M{})
	market := func(id string, price, supply, demand float64, key string) M {
		return M{"id": id, "coordinates": []float64{0, 0, 0}, "quotes": []M{{"key": key, "name": key, "price": price, "supply": supply, "demand": demand}}}
	}
	buyGold, sellGold := market("A", 100, 100, 0, "gold"), market("B", 200, 0, 40, "gold")
	buySilver, sellSilver := market("B", 150, 100, 0, "silver"), market("A", 200, 0, 40, "silver")
	routes := BuildRoutes(map[string]M{"gold": {"buy": []M{buyGold}, "sell": []M{sellGold}}, "silver": {"buy": []M{buySilver}, "sell": []M{sellSilver}}}, options)
	if len(routes) == 0 {
		t.Fatal("round trip missing")
	}
	legs := v.Rows(routes[0]["legs"])
	checkN(t, legs[0]["quantity"], 10)
	checkN(t, legs[1]["quantity"], 10)
	checkN(t, routes[0]["netEstimate"], 1500)
	options["operatingCostPerLeg"] = 100
	leg := PairLeg(buyGold, sellGold, "gold", options)
	checkN(t, leg["quantity"], 9)
	same := v.Copy(sellGold)
	same["id"] = "A"
	if PairLeg(buyGold, same, "gold", options) != nil {
		t.Fatal("same station round-trip accepted")
	}
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
func response(body any) *http.Response {
	b, _ := json.Marshal(body)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}}
}
func TestMarketSearchTransportAndPartialCoverage(t *testing.T) {
	calls := 0
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	client := doerFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if !strings.HasSuffix(r.URL.Path, "/stations/search") {
			t.Fatal("unexpected endpoint", r.URL)
		}
		var request M
		_ = json.NewDecoder(r.Body).Decode(&request)
		if v.At(request, "filters", "has_large_pad", "value") != true {
			t.Fatal("large pad filter missing")
		}
		rows := []M{}
		for i := 0; i < 60; i++ {
			rows = append(rows, M{"market_id": i + 1, "name": "Market", "system_name": "Sol", "type": "Coriolis", "has_large_pad": true, "market_updated_at": v.ISO(now), "system_x": 0, "system_y": 0, "system_z": 0, "market": []M{{"commodity": "Gold", "sell_price": 100, "demand": 100}}})
		}
		return response(M{"count": 200, "results": rows}), nil
	})
	search := NewSearcher(client)
	search.Now = func() time.Time { return now }
	out, e := search.Search(context.Background(), M{"mode": "sell", "resourceKey": "gold", "quantity": 1, "useCargo": false, "system": "Sol"}, M{"commander": M{"system": "Sol", "coordinates": []float64{0, 0, 0}}})
	if e != nil || calls != 2 {
		t.Fatalf("calls=%d,error=%v", calls, e)
	}
	if !v.Bool(out["coverageLimited"]) || len(v.Rows(out["results"])) != 60 {
		t.Fatal("limited coverage not shown", out)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := FetchJSON(cancelled, doerFunc(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() }), "GET", "https://example.invalid", nil); err == nil {
		t.Fatal("cancellation ignored")
	}
}
func TestTradePlanAndDestinationPersistence(t *testing.T) {
	s := New(M{})
	if s.Update(M{"action": "plan", "key": "platinum", "quantity": 100, "buyPrice": 1, "sellPrice": 5}) != nil {
		t.Fatal("plan rejected")
	}
	checkN(t, s.Plan["grossMargin"], 400)
	if s.Update(M{"action": "plan", "key": "gold", "quantity": -1, "buyPrice": 0, "sellPrice": 100}) == nil {
		t.Fatal("negative quantity accepted")
	}
	quote := M{"station": "Test", "system": "Sol", "quotes": []M{{"key": "gold", "price": 5}}}
	if s.Update(M{"action": "destination", "destination": quote}) != nil {
		t.Fatal("destination rejected")
	}
	restored := New(s.ExportSettings())
	if restored.Destination["station"] != "Test" || restored.Plan["key"] != "platinum" {
		t.Fatal("persisted plan lost")
	}
}

func TestSelectedMarketAgeDistanceAndCargoProjection(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	s := New(M{})
	cmd := M{"vehicle": "ship", "coordinates": []float64{0, 0, 0}}
	s.Apply(event("Cargo", M{"Vessel": "Ship", "Inventory": []M{{"Name": "gold", "Count": 10}}}), M{"commander": cmd})
	station := M{"station": "Test", "system": "Sol", "carrier": true, "coordinates": []float64{3, 4, 0}, "distanceLy": 99, "updatedAt": v.ISO(now.Add(-7 * time.Hour)), "source": "Spansh", "sourceUrl": "https://spansh.co.uk/station/1", "quotes": []M{{"key": "gold", "price": 5, "updatedAt": v.ISO(now.Add(-2 * time.Hour))}}}
	if err := s.Update(M{"action": "destination", "destination": station, "commander": cmd}); err != nil {
		t.Fatal(err)
	}
	project := func() M { return v.Obj(s.Snapshot(M{"commander": cmd, "now": v.ISO(now)})["destination"]) }
	first := project()
	checkN(t, first["ageHours"], 7)
	checkN(t, first["distanceLy"], 5)
	checkN(t, v.Rows(first["quotes"])[0]["ageHours"], 2)
	if !v.Bool(first["expired"]) || v.Bool(first["cargoChanged"]) || first["source"] != "Spansh" || first["sourceUrl"] != station["sourceUrl"] {
		t.Fatal(first)
	}
	checkN(t, s.Destination["distanceLy"], 99)
	if s.Destination["ageHours"] != nil {
		t.Fatal("projection rewrote saved market observation")
	}
	cmd["coordinates"] = []float64{3, 4, 0}
	s.Apply(event("EjectCargo", M{"Type": "Gold", "Count": 1}), M{"commander": cmd})
	second := project()
	checkN(t, second["distanceLy"], 0)
	if !v.Bool(second["cargoChanged"]) {
		t.Fatal("changed cargo not shown", second)
	}
	third := v.Obj(s.Snapshot(M{"commander": cmd, "marketOptions": M{"carrierMaxAgeHours": 8}, "now": v.ISO(now)})["destination"])
	if v.Bool(third["expired"]) {
		t.Fatal("selected freshness threshold ignored", third)
	}
	for _, date := range []any{nil, "invalid", v.ISO(now.Add(time.Hour))} {
		if !v.Bool(QuoteState(M{"updatedAt": date}, M{}, now)["expired"]) {
			t.Fatal("unverifiable timestamp considered fresh", date)
		}
	}
	restored := New(s.ExportSettings())
	if restored.Destination["cargoManifest"] != "gold:10" {
		t.Fatal("selection manifest not persisted", restored.Destination)
	}
}

func TestFindRoutesAndReferenceFallback(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	calls := 0
	client := doerFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "GET" {
			return response(M{"name": "Test", "id64": json.Number("18446744073709551615"), "coords": M{"x": 0, "y": 0, "z": 0}}), nil
		}
		var request M
		_ = json.NewDecoder(r.Body).Decode(&request)
		market := v.Rows(v.At(request, "filters", "market"))[0]
		id := 1
		if market["demand"] != nil {
			id = 2
		}
		return response(M{"count": 1, "results": []M{{"market_id": id, "name": "Test", "system_name": "Test", "type": "Coriolis", "has_large_pad": true, "system_x": 0, "system_y": 0, "system_z": 0, "market_updated_at": v.ISO(now), "market": []M{{"commodity": "Gold", "buy_price": 100, "sell_price": 200, "supply": 100, "demand": 100}}}}}), nil
	})
	search := NewSearcher(client)
	search.Now = func() time.Time { return now }
	updates := 0
	search.OnUpdate = func(M) { updates++ }
	out, err := search.FindRoutes(context.Background(), M{"system": "Test", "resourceKeys": []string{"gold"}, "credits": 10000, "cargoCapacity": 10, "minutesPerLeg": 10}, M{})
	if err != nil || len(v.Rows(out["results"])) != 1 || calls != 4 || updates == 0 {
		t.Fatal(out, err, calls, updates)
	}
	reference, err := search.Reference(context.Background(), M{}, M{"system": "Test"})
	if err != nil || reference["systemAddress"] != json.Number("18446744073709551615") {
		t.Fatal("reference ID64 lost", reference, err)
	}
	if len(Resources()) < 30 || Resource("unknown-resource") != nil {
		t.Fatal("catalog fallback")
	}
	if _, _, err := BuildBasket(M{}, NormalizeOptions(M{"resourceKey": "polonium", "mode": "buy"})); err == nil {
		t.Fatal("raw commodity basket accepted")
	}
	if _, _, err := BuildBasket(M{}, NormalizeOptions(nil)); err == nil {
		t.Fatal("empty cargo basket accepted")
	}
	state := M{"carrier": M{"warehouse": M{"items": []M{{"key": "gold", "count": 7}}}}}
	basket, _, err := BuildBasket(state, NormalizeOptions(M{"useCarrierCargo": true}))
	if err != nil || len(basket) != 1 || v.Num(basket[0]["quantity"]) != 7 {
		t.Fatal(basket, err)
	}
}
func TestTransientTransportRetryAndMalformedData(t *testing.T) {
	attempts := 0
	raw, err := FetchJSON(context.Background(), doerFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("busy"))}, nil
		}
		return response(M{"id64": json.Number("18446744073709551615")}), nil
	}), "GET", "https://test.invalid/", nil)
	if err != nil || attempts != 2 || v.Obj(raw)["id64"] != json.Number("18446744073709551615") {
		t.Fatal(raw, err, attempts)
	}
	for _, body := range []string{"<html>failure</html>", "{"} {
		if _, err := FetchJSON(context.Background(), doerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		}), "GET", "https://test.invalid", nil); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
	s := New(nil)
	s.Market(event("Market", M{"Items": []M{{"Name": "Gold"}}}))
	for _, e := range []M{event("CarrierDepositFuel", M{"Amount": 1}), event("EjectCargo", M{"Type": "Gold", "Count": 1}), event("RefuelAll", M{"Cost": 100}), event("BuyAmmo", M{"Cost": 5})} {
		s.Apply(e, M{"commander": M{"vehicle": "ship"}})
	}
	checkN(t, s.Totals["operatingCosts"], 105)
	if s.Update(M{"action": "clear-plan"}) != nil || s.Update(M{"action": "destination", "destination": nil}) != nil || s.Update(M{"action": "unknown"}) == nil {
		t.Fatal("trade actions validation")
	}
}
