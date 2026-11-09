package app

import (
	"fmt"
	"io"
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
	engine.network[carrierTradeOpportunityJob] = v.Copy(M{"options": request, "context": carrier.TradeOpportunityContext(trade, state, request),
		"updatedAt": now, "results": []M{offer}})
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
