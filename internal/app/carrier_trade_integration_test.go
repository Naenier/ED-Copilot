package app

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/carrier"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestCarrierTradeOpportunitiesUseDefaultHTTPClient(t *testing.T) {
	const child = "ED_COPILOT_CARRIER_TRADE_DEFAULT_HTTP_TEST"
	if os.Getenv(child) != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestCarrierTradeOpportunitiesUseDefaultHTTPClient$", "-test.timeout=30s")
		command.Env = append(os.Environ(), child+"=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("carrier trade with production HTTP defaults failed: %v\n%s", err, output)
		}
		return
	}

	// Isolate the process-wide default transport from other background jobs.
	engine, _, _ := fixtureEngine(t, nil)
	engine.opts.DisableNetwork = false
	var requests atomic.Int32
	http.DefaultTransport = economyRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodGet && request.URL.Path == "/api/station/128968675" {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(
				`{"record":{"market_id":"128968675","system_name":"HIP 58832","name":"Rackham's Peak"}}`))}, nil
		}
		if request.Method != http.MethodPost || request.URL.Path != "/api/stations/search" {
			return nil, fmt.Errorf("unexpected market request: %s %s", request.Method, request.URL.Path)
		}
		requests.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"count":0,"results":[]}`)),
		}, nil
	})

	doOK(t, engine, "/api/carrier/trade", M{"action": "opportunities", "system": "Sol"})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		search := v.Obj(v.At(engine.SnapshotForUI("carrier-trade"), "carrierTrade", "opportunities"))
		if v.Bool(search["running"]) {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if requests.Load() == 0 || search["stage"] != "routes" || search["error"] != nil || v.Bool(search["partial"]) {
			t.Fatalf("default client did not complete a healthy market search: requests=%d, stage=%v, error=%v, partial=%v",
				requests.Load(), search["stage"], search["error"], search["partial"])
		}
		return
	}
	t.Fatal("carrier trade opportunities did not finish")
}

func TestCarrierTradeOpportunityHandoffChecksContextAndKeepsLogistics(t *testing.T) {
	engine, _, _ := fixtureEngine(t, nil)
	state := engine.SnapshotForUI("carrier-trade")
	trade := v.Obj(state["carrierTrade"])
	request := M{"scenario": "custom", "system": "Synthetic A", "credits": 15000000.0}
	now := v.ISO(time.Now())
	offer := v.Copy(trade)
	offer["id"], offer["buy"] = "synthetic", M{"price": 1000, "updatedAt": now}
	outbound := v.Rows(offer["legs"])[0]
	outbound["quantity"], outbound["buyPrice"], outbound["sellPrice"], outbound["tritiumTons"] = 300, 1000, 3000, 25
	outbound["buyMarket"] = M{"station": "Synthetic Purchase", "system": "Synthetic A", "orders": []M{
		{"station": "Synthetic Purchase", "quantity": 100}, {"station": "Synthetic Secondary", "quantity": 200}}}
	outbound["route"] = M{"fuelPurchaseTons": 50, "fuelMarket": M{"station": "Synthetic Fuel"}}
	engine.mu.Lock()
	engine.carrierTradeSearch = &carrierTradeSearch{
		request: carrierTradeSearchRequest{system: "Synthetic A", credits: 15000000},
		context: carrier.TradeOpportunityContext(trade, state, request),
		view:    M{"updatedAt": now, "results": []M{offer}},
	}
	engine.mu.Unlock()
	doOK(t, engine, "/api/carrier/trade", M{"action": "apply-opportunity", "id": "synthetic"})
	plan := engine.CarrierTrade.ExportSettings()
	leg := v.Obj(v.At(plan, "legs", "outbound"))
	if v.Num(leg["quantity"]) != 300 || len(v.Rows(v.At(leg, "buyMarket", "orders"))) != 2 ||
		v.At(leg, "route", "fuelMarket", "station") != "Synthetic Fuel" {
		t.Fatal("applying the option lost purchase or replenishment details", leg)
	}
	doOK(t, engine, "/api/carrier/trade", M{"options": M{"tritiumPrice": 49000}})
	if _, err := engine.Do("/api/carrier/trade", M{"action": "apply-opportunity", "id": "synthetic"}); err == nil {
		t.Fatal("stale option replaced a changed plan")
	}
}

func TestCarrierTradeOpportunityRejectsMalformedInput(t *testing.T) {
	for _, input := range []M{
		{"system": 42}, {"system": strings.Repeat("a", 151)}, {"system": "a\nb"},
		{"credits": "15000000"}, {"credits": -1}, {"credits": math.Inf(1)}, {"credits": 1e16},
		{"force": "true"}, {"scenario": "unexpected"},
	} {
		if _, _, err := parseCarrierTradeSearch(input); err == nil {
			t.Errorf("invalid search input accepted: %v", input)
		}
	}
	request, force, err := parseCarrierTradeSearch(M{"system": "  Sagittarius A* ", "credits": 15000000, "force": true})
	if err != nil || !force || request.system != "Sagittarius A*" || request.credits != 15000000 {
		t.Fatal("valid search options were not normalized", request, force, err)
	}
}

func TestCarrierTradeOpportunityRedirectsStayOnHTTPSOrigin(t *testing.T) {
	for _, tc := range []struct {
		location string
		follows  bool
	}{
		{"/api/stations/search", true},
		{"http://spansh.co.uk/api/stations/search", false},
		{"https://example.test/api/stations/search", false},
		{"https://spansh.co.uk:8443/api/stations/search", false},
	} {
		t.Run(tc.location, func(t *testing.T) {
			calls := 0
			client := carrierTradeHTTPClient(&http.Client{Transport: economyRoundTripper(func(request *http.Request) (*http.Response, error) {
				calls++
				header := http.Header{}
				status := http.StatusOK
				if calls == 1 {
					header.Set("Location", tc.location)
					status = http.StatusFound
				}
				return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(""))}, nil
			})})
			response, err := client.Get("https://spansh.co.uk/api/search/systems")
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if (calls == 2) != tc.follows {
				t.Fatalf("redirect made %d HTTP calls, follow expected %t", calls, tc.follows)
			}
		})
	}
}

func TestCarrierTradeOpportunityCancelsOnReplacementAndCommanderChange(t *testing.T) {
	engine, _, _ := fixtureEngine(t, nil)
	requests := make(chan *http.Request, 4)
	engine.opts.DisableNetwork = false
	engine.opts.HTTPClient = &http.Client{Transport: economyRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests <- request
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	start := func(system string) (*http.Request, *carrierTradeSearch) {
		t.Helper()
		doOK(t, engine, "/api/carrier/trade", M{"action": "opportunities", "system": system, "credits": 1000000})
		select {
		case request := <-requests:
			engine.mu.RLock()
			job := engine.carrierTradeSearch
			engine.mu.RUnlock()
			return request, job
		case <-time.After(3 * time.Second):
			t.Fatal("opportunity search did not reach transport")
			return nil, nil
		}
	}
	expectCanceled := func(request *http.Request) {
		t.Helper()
		select {
		case <-request.Context().Done():
		case <-time.After(3 * time.Second):
			t.Fatal("superseded request remained active")
		}
	}
	view := func() M {
		return v.Obj(engine.SnapshotForUI("carrier-trade")["carrierTrade"])
	}
	first, old := start("First Contract System")
	duplicate := doOK(t, engine, "/api/carrier/trade", M{"action": "opportunities", "system": "First Contract System", "credits": 1000000})
	if !v.Bool(duplicate["cached"]) {
		t.Fatal("identical live request started twice", duplicate)
	}
	second, edited := start("Second Contract System")
	expectCanceled(first)
	// Even a producer that failed to propagate cancellation cannot overwrite its successor.
	engine.publishCarrierTradeSearch(context.Background(), old, M{"status": "superseded"}, false, nil)
	if v.At(view(), "opportunities", "options", "system") != "Second Contract System" {
		t.Fatal("old request replaced the new UI snapshot")
	}
	doOK(t, engine, "/api/carrier/trade", M{"options": M{"tritiumPrice": 52000}})
	expectCanceled(second)
	engine.publishCarrierTradeSearch(context.Background(), edited, M{"status": "changed"}, false, nil)
	if view()["opportunities"] != nil {
		t.Fatal("planner edit preserved a search under the old assumptions")
	}
	third, other := start("Third Contract System")
	before := v.Num(view()["opportunityScope"])
	engine.Apply(testEvent("Commander", M{"FID": "OTHER"}), nil)
	expectCanceled(third)
	engine.Apply(testEvent("Commander", M{"FID": "FSYNTHETIC"}), nil)
	engine.publishCarrierTradeSearch(context.Background(), other, M{"status": "previous owner"}, false, nil)
	after := view()
	if after["opportunities"] != nil || v.Num(after["opportunityScope"]) <= before {
		t.Fatal("a commander switch preserved another session's opportunity cache", after["opportunities"])
	}
	fourth, closing := start("Fourth Contract System")
	engine.Close()
	expectCanceled(fourth)
	engine.publishCarrierTradeSearch(context.Background(), closing, M{"status": "closed"}, false, nil)
	if view()["opportunities"] != nil {
		t.Fatal("closed search still visible")
	}
	if _, err := engine.Do("/api/carrier/trade", M{"action": "opportunities"}); err == nil {
		t.Fatal("new search accepted after close")
	}
	if _, err := engine.Do("/api/carrier/trade", M{"action": "apply-opportunity", "id": "synthetic"}); err == nil {
		t.Fatal("handoff accepted after close")
	}
}

func TestCarrierTradeOpportunityViewIsDetachedAndRejectsChangedInputs(t *testing.T) {
	engine, _, _ := fixtureEngine(t, nil)
	state := engine.SnapshotForUI("carrier-trade")
	request := carrierTradeSearchRequest{system: "Sol", credits: 10}
	job := &carrierTradeSearch{request: request, context: carrier.TradeOpportunityContext(v.Obj(state["carrierTrade"]), state, request.options())}
	engine.mu.Lock()
	engine.carrierTradeSearch = job
	engine.mu.Unlock()
	result := M{"results": []M{{"id": "example", "legs": []M{{"id": "outbound"}}, "buy": M{
		"station": "Original", "sourceUrl": "https://example.test/",
		"orders": []M{{"station": "Supplier", "quantity": 1, "providerPayload": "internal"}},
	}}}, "context": M{"fid": "internal"}}
	engine.publishCarrierTradeSearch(context.Background(), job, result, true, nil)
	snapshot := func() M {
		return v.Obj(v.At(engine.SnapshotForUI("carrier-trade"), "carrierTrade", "opportunities"))
	}
	visible := snapshot()
	rows := v.Rows(visible["results"])
	if visible["context"] != nil || len(rows) != 1 {
		t.Fatal("UI should expose only the public operation view", visible)
	}
	orders := v.Rows(v.At(rows[0], "buy", "orders"))
	if rows[0]["legs"] != nil || v.At(rows[0], "buy", "sourceUrl") != nil ||
		len(orders) != 1 || orders[0]["providerPayload"] != nil {
		t.Fatal("private fields escaped through the nested trade result", rows[0])
	}
	v.Obj(rows[0]["buy"])["station"] = "Modified"
	if v.At(v.Rows(snapshot()["results"])[0], "buy", "station") != "Original" {
		t.Fatal("mutating a UI snapshot changed the stored search result")
	}
	doOK(t, engine, "/api/carrier/trade", M{"options": M{"tritiumPrice": 52000}})
	engine.publishCarrierTradeSearch(context.Background(), job, M{"status": "outdated"}, false, nil)
	if len(snapshot()) != 0 {
		t.Fatal("a late result became visible under new planning options")
	}
}

func tradeIntegrationRoute(t *testing.T) M {
	t.Helper()
	options, err := carrier.NormalizeRouteOptions(M{"sourceSystem": "Synthetic A", "destinationSystem": "Synthetic B", "tankTons": 500, "calculateStartingFuel": false, "departureTime": "2026-11-08T12:00:00Z"}, M{})
	if err != nil {
		t.Fatal(err)
	}
	provider := M{"status": "ok", "job": "11111111-2222-3333-4444-555555555555", "result": M{"capacity_used": 0, "fuel_loaded": 500, "tritium_stored": 0, "jumps": []M{
		{"name": "Synthetic A", "id64": "111", "x": 0, "y": 0, "z": 0, "distance": 0, "fuel_used": 0, "fuel_in_tank": 500, "tritium_in_market": 0, "restock_amount": 0},
		{"name": "Synthetic B", "id64": "222", "x": 400, "y": 0, "z": 0, "distance": 400, "fuel_used": 100, "fuel_in_tank": 400, "tritium_in_market": 0, "restock_amount": 0},
	}}}
	result, err := carrier.NormalizeProviderRoute(provider, options, M{"createdAt": "2026-11-08T12:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	result["options"] = options
	return result
}

func TestCarrierTradeAPIImportsPersonalRouteAndRejectsSquadronAtomically(t *testing.T) {
	engine, _, _ := fixtureEngine(t, M{"carrierRouteGo": M{"state": tradeIntegrationRoute(t)}})
	doOK(t, engine, "/api/carrier/trade", M{"action": "import-route", "leg": "outbound"})
	plan := engine.CarrierTrade.ExportSettings()
	leg := v.Obj(v.At(plan, "legs", "outbound"))
	if leg["sourceSystem"] != "Synthetic A" || leg["destinationSystem"] != "Synthetic B" || v.At(leg, "route", "carrierType") != "fleet" || !strings.HasPrefix(v.Str(v.At(leg, "route", "sourceUrl")), "https://spansh.co.uk/fleet-carrier/results/") {
		t.Fatal("route identity or provenance was not imported", leg)
	}
	expectNumber(t, leg["jumps"], 1)
	expectNumber(t, leg["tritiumTons"], 100)
	expectNumber(t, leg["travelHours"], 16.0/60)
	if v.At(plan, "legs", "return", "tritiumTons") != nil {
		t.Fatal("outbound import also replaced the return leg")
	}
	doOK(t, engine, "/api/carrier/trade", M{"options": M{"carrierType": "squadron"}})
	before := engine.CarrierTrade.ExportSettings()
	if _, err := engine.Do("/api/carrier/trade", M{"action": "import-route", "leg": "return"}); err == nil || !strings.Contains(err.Error(), "другого типа") {
		t.Fatal("personal route accepted for squadron carrier", err)
	}
	if !reflect.DeepEqual(before, engine.CarrierTrade.ExportSettings()) {
		t.Fatal("failed squadron route import changed the trade plan")
	}
}

func TestCarrierTradeAPIRefusesMissingAndMismatchedRoutesWithoutChangingPlan(t *testing.T) {
	engine, _, _ := fixtureEngine(t, nil)
	before := engine.CarrierTrade.ExportSettings()
	if _, err := engine.Do("/api/carrier/trade", M{"action": "import-route", "leg": "outbound"}); err == nil {
		t.Fatal("missing route was silently converted to zero fuel")
	}
	if !reflect.DeepEqual(before, engine.CarrierTrade.ExportSettings()) {
		t.Fatal("missing route changed the plan")
	}
	engine.Route = carrier.NewRoutePlanner(M{"state": tradeIntegrationRoute(t)})
	doOK(t, engine, "/api/carrier/trade", M{"legs": M{"return": M{"sourceSystem": "Synthetic B", "destinationSystem": "Synthetic A"}}})
	before = engine.CarrierTrade.ExportSettings()
	if _, err := engine.Do("/api/carrier/trade", M{"action": "import-route", "leg": "return"}); err == nil {
		t.Fatal("outbound route was accepted as a reversed trip")
	}
	if !reflect.DeepEqual(before, engine.CarrierTrade.ExportSettings()) {
		t.Fatal("mismatched route import was not atomic")
	}
}

func TestCarrierTradeAPIImportsSelectedCarrierCostsAndKeepsSquadronFuelIndependent(t *testing.T) {
	engine, journalFile, dataDir := fixtureEngine(t, nil)
	engine.Apply(testEvent("CarrierStats", M{"CarrierID": "42", "Name": "Synthetic Personal Carrier", "FuelLevel": 800}), M{})
	engine.domainMu.Lock()
	err := engine.Carrier.SetStockBaseline(M{"carrierId": "42", "items": []M{{"key": "tritium", "count": 130}}, "timestamp": "2026-11-08T12:00:00Z", "complete": true})
	engine.domainMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	doOK(t, engine, "/api/carrier/calculator", M{"carrierType": "fleet", "jumpsPerWeek": 10, "services": M{"repair": "active", "refuel": "suspended"}})
	doOK(t, engine, "/api/carrier/trade", M{"action": "import-carrier"})
	options := v.Obj(engine.CarrierTrade.ExportSettings()["options"])
	if options["carrierType"] != "fleet" {
		t.Fatal("import ignored the calculator's selected type", options)
	}
	expectNumber(t, options["weeklyUpkeep"], 7250000) // excludes ten jump fees: those belong to the trade's route.
	expectNumber(t, options["servicesTons"], 680)
	expectNumber(t, options["carrierInvestment"], 5090000000)
	expectNumber(t, options["tankTons"], 1000)
	if options["cargoTritiumTons"] != nil {
		t.Fatal("import copied tritium from the live warehouse")
	}

	doOK(t, engine, "/api/carrier/trade", M{"options": M{"carrierType": "squadron", "tankTons": 321, "cargoTritiumTons": 55, "otherCargoTons": 222}, "legs": M{"return": M{"commodityKey": "wine", "quantity": 900}}})
	doOK(t, engine, "/api/carrier/calculator", M{"carrierType": "squadron", "jumpsPerWeek": 15, "services": M{"repair": "active"}})
	doOK(t, engine, "/api/carrier/trade", M{"action": "import-carrier"})
	options = v.Obj(engine.CarrierTrade.ExportSettings()["options"])
	expectNumber(t, options["weeklyUpkeep"], 11500000)
	expectNumber(t, options["servicesTons"], 3180)
	expectNumber(t, options["carrierInvestment"], 25050000000)
	expectNumber(t, options["tankTons"], 321)
	expectNumber(t, options["cargoTritiumTons"], 55)
	expectNumber(t, options["otherCargoTons"], 222)
	if options["carrierType"] != "squadron" || v.At(engine.CarrierTrade.ExportSettings(), "legs", "return", "commodityKey") != "wine" {
		t.Fatal("type or independent trade parameters lost during configuration import")
	}
	snapshot := engine.SnapshotForUI("carrier-trade")
	if v.At(snapshot, "carrierTrade", "context", "calculator", "selectedType") != "squadron" || snapshot["trading"] == nil || v.At(snapshot, "carrierTrade", "context", "route") == nil {
		t.Fatal("scoped trade screen lost integration context")
	}
	before := engine.CarrierTrade.ExportSettings()
	calculatorBefore := engine.Calculator.ExportSettings()
	engine.Close()
	restored, err := New(Options{DataDir: dataDir, JournalDir: filepath.Dir(journalFile), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if !reflect.DeepEqual(before, restored.CarrierTrade.ExportSettings()) || !reflect.DeepEqual(calculatorBefore, restored.Calculator.ExportSettings()) {
		t.Fatal("selected calculator and trade plans did not survive API save/restart")
	}
}

func TestSeedDemoShowcaseRefusesNetworkEnabledEngine(t *testing.T) {
	// No clients or real profile exist here. The guard must run before touching
	// any domain, otherwise this call panics instead of refusing demo data.
	engine := &Engine{opts: Options{DisableNetwork: false}}
	if err := engine.SeedDemoShowcase(); err == nil {
		t.Fatal("demo data may be inserted into a network-enabled user profile")
	}
}
