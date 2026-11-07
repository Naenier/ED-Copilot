package carrier

import (
	"context"
	"encoding/json"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func closeNumber(t *testing.T, raw any, want float64) {
	t.Helper()
	if math.Abs(v.Num(raw)-want) > 1e-7 {
		t.Fatalf("got %v want %g", raw, want)
	}
}
func ce(kind string, extra M) M {
	return v.Merge(M{"event": kind, "CarrierID": "42", "timestamp": "2026-11-07T12:00:00Z"}, extra)
}
func TestTrackerOwnershipFinanceStockAndFreshness(t *testing.T) {
	tracker := New(M{"planning": M{"weeklyUpkeep": 100, "reserveTritium": 200}})
	tracker.Now = func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }
	tracker.Apply(ce("CarrierStats", M{"CarrierType": "SquadronCarrier", "FuelLevel": 999}), M{})
	if tracker.OwnerID != "" {
		t.Fatal("squadron carrier became personal carrier")
	}
	tracker.Apply(ce("CarrierStats", M{"Name": "Test carrier", "Callsign": "AAA-111", "FuelLevel": 800, "SpaceUsage": M{"TotalCapacity": 25000, "Cargo": 1000, "FreeSpace": 24000}, "Finance": M{"CarrierBalance": 1000, "ReserveBalance": 100, "AvailableBalance": 900}, "Crew": []M{{"CrewRole": "repair", "Activated": true, "Enabled": false}}}), M{})
	state := tracker.Snapshot(nil)
	if !v.Bool(state["owned"]) || state["id"] != "42" {
		t.Fatal(state)
	}
	if v.At(state, "fuel", "reserveTons") != nil {
		t.Fatal("unknown warehouse tritium treated as zero")
	}
	closeNumber(t, v.At(state, "finance", "runwayWeeks"), 10)
	if tracker.SetStockBaseline(M{"carrierId": "wrong", "items": []M{}}) == nil {
		t.Fatal("foreign baseline accepted")
	}
	if err := tracker.SetStockBaseline(M{"items": []M{{"key": "tritium", "count": 100}, {"key": "gold", "count": 20}}, "timestamp": "2026-11-07T11:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	tracker.Apply(ce("Docked", M{"StationType": "FleetCarrier", "MarketID": "42", "StationName": "Test"}), M{})
	tracker.Apply(ce("CargoTransfer", M{"Transfers": []M{{"Type": "tritium", "Count": 30, "Direction": "tocarrier"}}}), M{})
	state = tracker.Snapshot(nil)
	closeNumber(t, v.At(state, "fuel", "reserveTons"), 130)
	if !v.Bool(v.At(state, "warehouse", "complete")) {
		t.Fatal("baseline warehouse is incomplete")
	}
	tracker.Apply(ce("CarrierJumpRequest", M{"SystemName": "Target", "DepartureTime": "2026-11-07T12:15:00Z", "BodyID": 3}), M{})
	tracker.Apply(ce("CarrierLocation", M{"StarSystem": "Target", "BodyID": 3, "timestamp": "2026-11-07T12:10:00Z"}), M{})
	if v.At(tracker.Snapshot(nil), "jump", "pending") == nil {
		t.Fatal("arrival confirmed before departure")
	}
	tracker.Apply(ce("CarrierLocation", M{"StarSystem": "Target", "BodyID": 3, "timestamp": "2026-11-07T12:16:00Z"}), M{})
	state = tracker.Snapshot(nil)
	if v.At(state, "jump", "pending") != nil || !v.Bool(v.At(state, "fuel", "needsRefresh")) {
		t.Fatal("arrival/fuel invalidation failed")
	}
	tracker.Apply(ce("CarrierDepositFuel", M{"Total": 900, "Amount": 100}), M{})
	if v.Bool(v.At(tracker.Snapshot(nil), "fuel", "needsRefresh")) {
		t.Fatal("fuel snapshot not refreshed")
	}
	restored := New(tracker.ExportSettings())
	if restored.StockBaseline["carrierId"] != "42" || v.Num(restored.Planning["weeklyUpkeep"]) != 100 {
		t.Fatal("configuration lost")
	}
}
func TestTrackerMissingDepartureCannotConfirmArrival(t *testing.T) {
	tracker := New(M{})
	tracker.Apply(ce("CarrierStats", M{}), M{})
	tracker.Apply(ce("CarrierJumpRequest", M{"SystemName": "Target"}), M{})
	tracker.Apply(ce("CarrierLocation", M{"StarSystem": "Target"}), M{})
	if v.At(tracker.Snapshot(nil), "jump", "pending") == nil {
		t.Fatal("unknown departure date must not confirm arrival")
	}
}
func routeFixture() M {
	return M{"status": "ok", "job": "12345678-1234-1234-1234-123456789012", "result": M{"capacity_used": 0, "fuel_loaded": 500, "tritium_stored": 0, "jumps": []M{{"name": "A", "id64": "18446744073709551613", "x": 0, "y": 0, "z": 0, "distance": 0, "fuel_used": 0, "fuel_in_tank": 500, "tritium_in_market": 0, "restock_amount": 0}, {"name": "B", "id64": "18446744073709551614", "x": 400, "y": 0, "z": 0, "distance": 400, "fuel_used": 100, "fuel_in_tank": 400, "tritium_in_market": 0, "restock_amount": 0}, {"name": "C", "id64": "18446744073709551615", "x": 800, "y": 0, "z": 0, "distance": 400, "fuel_used": 100, "fuel_in_tank": 300, "tritium_in_market": 0, "restock_amount": 0}}}}
}
func routeOptions(t *testing.T) M {
	t.Helper()
	options, err := NormalizeRouteOptions(M{"sourceSystem": "A", "destinationSystem": "C", "tankTons": 500, "calculateStartingFuel": false, "departureTime": "2026-11-07T12:00:00Z"}, M{})
	if err != nil {
		t.Fatal(err)
	}
	return options
}
func TestProviderFuelTimeValidationAndCSV(t *testing.T) {
	options := routeOptions(t)
	result, err := NormalizeProviderRoute(routeFixture(), options, M{"createdAt": "2026-11-07T12:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	summary := v.Obj(result["summary"])
	closeNumber(t, summary["fuelTons"], 200)
	closeNumber(t, summary["travelMinutes"], 37)
	if summary["estimatedArrival"] != "2026-11-07T12:37:00Z" || summary["readyAgain"] != "2026-11-07T12:42:00Z" {
		t.Fatal(summary)
	}
	stops := v.Rows(v.At(result, "route", "stops"))
	if stops[2]["id64"] != "18446744073709551615" {
		t.Fatal("ID64 lost precision")
	}
	bad := routeFixture()
	v.Rows(v.At(bad, "result", "jumps"))[1]["fuel_in_tank"] = 401
	if _, err := NormalizeProviderRoute(bad, options, M{}); err == nil {
		t.Fatal("unbalanced fuel accepted")
	}
	bad = routeFixture()
	v.Rows(v.At(bad, "result", "jumps"))[1]["x"] = 600
	if _, err := NormalizeProviderRoute(bad, options, M{}); err == nil {
		t.Fatal("overlong jump accepted")
	}
	bad = routeFixture()
	v.Rows(v.At(bad, "result", "jumps"))[1]["fuel_used"] = 600
	v.Rows(v.At(bad, "result", "jumps"))[1]["fuel_in_tank"] = 0
	v.Rows(v.At(bad, "result", "jumps"))[1]["restock_amount"] = 100
	if _, err := NormalizeProviderRoute(bad, options, M{}); err == nil {
		t.Fatal("insufficient tank fuel accepted")
	}
	stops[1]["name"] = "=HYPERLINK(bad)"
	csv, err := RouteCSV(result)
	if err != nil || !strings.Contains(csv, "'=HYPERLINK") {
		t.Fatal("spreadsheet formula not escaped", csv, err)
	}
}
func TestRoutePayloadAndOptionsValidation(t *testing.T) {
	options := routeOptions(t)
	systems := []M{{"id64": "1", "name": "A"}, {"id64": "2", "name": "C"}}
	payload := BuildRoutePayload(options, systems, nil)
	if payload["calculate_starting_fuel"] != 0 {
		t.Fatal(payload)
	}
	closeNumber(t, payload["fuel_loaded"], 500)
	form := PayloadForm(payload)
	if form.Get("source") != "1" || form.Get("destinations") != "2" {
		t.Fatal(form)
	}
	for _, bad := range []M{{"sourceSystem": "A", "destinationSystem": "A"}, {"sourceSystem": "A", "destinationSystem": "C", "cargoTritiumTons": 10, "usedCapacityTons": 0}, {"sourceSystem": "A", "destinationSystem": "C", "tankTons": 1001}, {"sourceSystem": "A", "destinationSystem": "C", "refuelSystems": []string{"Other"}}, {"sourceSystem": "A", "destinationSystem": "C", "usedCapacityTons": 1.5}} {
		if _, err := NormalizeRouteOptions(bad, M{}); err == nil {
			t.Fatal("invalid options accepted", bad)
		}
	}
}
func TestRouteJournalProgressAndPersistence(t *testing.T) {
	options := routeOptions(t)
	result, err := NormalizeProviderRoute(routeFixture(), options, M{"createdAt": "2026-11-07T12:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	planner := NewRoutePlanner(result)
	if planner.ObserveCarrier(M{"owned": true, "location": M{"system": "C", "systemAddress": "18446744073709551615", "updatedAt": "2026-11-07T13:00:00Z"}}) {
		t.Fatal("skipped next point accepted")
	}
	if planner.ObserveCarrier(M{"owned": true, "location": M{"system": "B", "updatedAt": "2026-11-07T11:00:00Z"}}) {
		t.Fatal("old replay confirmed fresh route")
	}
	if !planner.ObserveCarrier(M{"owned": true, "location": M{"system": "B", "systemAddress": "18446744073709551614", "updatedAt": "2026-11-07T13:00:00Z"}}) {
		t.Fatal("current own-carrier location not confirmed")
	}
	closeNumber(t, v.At(planner.Snapshot(), "summary", "completedJumps"), 1)
	restored := NewRoutePlanner(planner.ExportSettings())
	closeNumber(t, v.At(restored.Snapshot(), "summary", "completedJumps"), 1)
	if err := restored.MarkVisited(2, "manual", "2026-11-07T13:30:00Z"); err != nil {
		t.Fatal(err)
	}
	closeNumber(t, v.At(restored.Snapshot(), "summary", "remainingJumps"), 0)
}

func TestUnfinishedRouteKeepsOptionsAcrossLegacyAndGoRestart(t *testing.T) {
	options := M{"sourceSystem": "Sol", "destinationSystem": "Colonia", "waypoints": []string{"A"}, "tankTons": 400, "usedCapacityTons": 1500, "calculateStartingFuel": false, "reserveTritiumTons": 200}
	legacy := M{"version": 1, "state": M{"running": true, "status": "queued", "error": "interrupted", "options": options, "route": nil, "progress": M{"stage": "polling", "label": "Waiting"}}}
	p := NewRoutePlanner(legacy)
	for _, state := range []M{p.Snapshot(), NewRoutePlanner(p.ExportSettings()).Snapshot()} {
		if v.Bool(state["running"]) || state["status"] != "idle" || state["error"] != nil || v.At(state, "progress", "stage") != "idle" {
			t.Fatal("unfinished route resumed a network job", state)
		}
		for key, want := range options {
			a, _ := json.Marshal(v.Obj(state["options"])[key])
			b, _ := json.Marshal(want)
			if string(a) != string(b) {
				t.Fatal("unfinished route option lost", key, state["options"])
			}
		}
		closeNumber(t, v.At(state, "options", "jumpMinutes"), 15)
	}
}

type routeDoer func(*http.Request) (*http.Response, error)

func (f routeDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }
func routeResponse(payload M) *http.Response {
	b, _ := json.Marshal(payload)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}}
}
func TestSpanshJobPollingAndCancellation(t *testing.T) {
	planner := NewRoutePlanner(nil)
	planner.PollInterval = time.Millisecond
	planner.Timeout = time.Second
	polls := 0
	planner.Client = routeDoer(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/search/systems":
			name := r.URL.Query().Get("q")
			id, x := "18446744073709551613", 0
			if name == "C" {
				id = "18446744073709551615"
				x = 800
			}
			return routeResponse(M{"results": []M{{"name": name, "id64": id, "x": x, "y": 0, "z": 0}}}), nil
		case "/api/fleetcarrier/route":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("fuel_loaded") != "500" {
				t.Fatal(r.Form)
			}
			return routeResponse(M{"status": "queued", "job": "12345678-1234-1234-1234-123456789012"}), nil
		default:
			polls++
			return routeResponse(routeFixture()), nil
		}
	})
	state, err := planner.Search(context.Background(), routeOptions(t), M{})
	if err != nil || polls != 1 || state["status"] != "ready" {
		t.Fatal(state, err)
	}
	blocked := NewRoutePlanner(nil)
	started := make(chan struct{})
	blocked.Client = routeDoer(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	done := make(chan error, 1)
	go func() { _, e := blocked.Search(context.Background(), routeOptions(t), M{}); done <- e }()
	<-started
	blocked.Cancel()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("cancelled request succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled request blocked")
	}
}

func TestTrackerAllManagementEvents(t *testing.T) {
	tracker := New(nil)
	tracker.Now = func() time.Time { return time.Date(2026, 11, 7, 12, 4, 0, 0, time.UTC) }
	tracker.Apply(ce("CarrierBuy", M{"Name": "Original", "Callsign": "ABC-123", "Location": "Origin", "Price": 5000000000}), nil)
	tracker.Apply(ce("CarrierFinance", M{"CarrierBalance": 1000, "TaxRate": 5, "TaxRate_repair": 10}), nil)
	tracker.Apply(ce("CarrierBankTransfer", M{"Withdraw": 100, "CarrierBalance": 900}), nil)
	tracker.Apply(ce("CarrierNameChanged", M{"Name": "Renamed", "Callsign": "DEF-456"}), nil)
	tracker.Apply(ce("CarrierDockingPermission", M{"DockingAccess": "friends", "AllowNotorious": true}), nil)
	for _, operation := range []string{"activate", "pause", "resume", "deactivate", "replace"} {
		tracker.Apply(ce("CarrierCrewServices", M{"CrewRole": "repair", "Operation": operation, "CrewName": "Test crew"}), nil)
	}
	tracker.Apply(ce("CarrierTradeOrder", M{"Commodity": "Gold", "SaleOrder": 100, "Price": 500}), nil)
	tracker.Apply(ce("Docked", M{"MarketID": "42", "StationType": "FleetCarrier"}), nil)
	tracker.Apply(ce("CargoTransfer", M{"Transfers": []M{{"Type": "Gold", "Count": 25, "Direction": "toship"}}}), nil)
	tracker.Apply(ce("MarketSell", M{"MarketID": "42", "Type": "Gold", "Count": 5}), nil)
	tracker.Apply(ce("MarketBuy", M{"MarketID": "42", "Type": "Gold", "Count": 10}), nil)
	state := tracker.Snapshot(nil)
	if state["name"] != "Renamed" || state["dockingAccessLabel"] != "Друзья" || !v.Bool(state["allowNotorious"]) {
		t.Fatal(state)
	}
	closeNumber(t, v.At(state, "finance", "balance"), 900)
	closeNumber(t, v.At(state, "finance", "taxRates", "repair"), 10)
	items := v.Rows(v.At(state, "warehouse", "items"))
	if len(items) != 1 {
		t.Fatal(items)
	}
	closeNumber(t, items[0]["count"], 70)
	tracker.Apply(ce("CarrierTradeOrder", M{"Commodity": "Gold", "CancelTrade": true}), nil)
	if len(v.Rows(tracker.Snapshot(nil)["orders"])) != 0 {
		t.Fatal("cancelled order retained")
	}
	tracker.Apply(ce("CarrierTradeOrder", M{"Commodity": "Silver", "PurchaseOrder": 10, "BlackMarket": true}), nil)
	tracker.Apply(ce("CarrierDecommission", M{"ScrapTime": 1794096000}), nil)
	if !v.Bool(tracker.Snapshot(nil)["pendingDecommission"]) {
		t.Fatal("decommission not tracked")
	}
	tracker.Apply(ce("CarrierCancelDecommission", nil), nil)
	for _, kind := range []string{"CarrierShipPack", "CarrierModulePack"} {
		tracker.Apply(ce(kind, M{"Cost": 100, "PackTheme": "Test"}), nil)
		tracker.Apply(ce(kind, M{"Refund": 50}), nil)
	}
	tracker.Apply(ce("CarrierJump", M{"MarketID": "42", "StarSystem": "Destination", "StarPos": []float64{1, 2, 3}, "timestamp": "2026-11-07T12:00:00Z"}), nil)
	state = tracker.Snapshot(nil)
	if v.At(state, "jump", "phase") != "cooldown" || v.Bool(state["pendingDecommission"]) {
		t.Fatal(state)
	}
	if err := tracker.Update(M{"action": "planning", "targetTankTons": 600, "weeklyUpkeep": 400}); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Update(M{"action": "stock", "items": []M{{"key": "Gold", "count": 10}}, "timestamp": "2026-11-07T12:05:00Z"}); err != nil {
		t.Fatal(err)
	}
	if tracker.Update(M{"action": "invalid"}) == nil {
		t.Fatal("unknown action accepted")
	}
	tracker.Apply(ce("Undocked", nil), nil)
	if tracker.IsOwnDockedMarket(nil) {
		t.Fatal("undock retained own market")
	}
	if tracker.SetStockBaseline(M{"items": []M{{"key": "Gold", "count": -1}}}) == nil {
		t.Fatal("negative baseline allowed")
	}
	if tracker.SetStockBaseline(M{"items": []M{}, "timestamp": "invalid"}) == nil {
		t.Fatal("invalid baseline date allowed")
	}
}
func TestAdaptiveReserveUsesRecalculatedMass(t *testing.T) {
	planner := NewRoutePlanner(nil)
	jobs := 0
	planner.OnUpdate = func(M) {}
	planner.Client = routeDoer(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			name := r.URL.Query().Get("q")
			x, id := 0, "1"
			if name == "C" {
				x = 400
				id = "2"
			}
			return routeResponse(M{"results": []M{{"name": name, "id64": id, "x": x, "y": 0, "z": 0}}}), nil
		}
		jobs++
		_ = r.ParseForm()
		loaded := 200.0
		if r.Form.Get("calculate_starting_fuel") == "0" {
			loaded, _ = strconv.ParseFloat(r.Form.Get("fuel_loaded"), 64)
		}
		burn := 200 + math.Ceil(math.Max(0, loaded-200)/100)
		arrival := loaded - burn
		return routeResponse(M{"status": "ok", "job": "test", "result": M{"fuel_loaded": loaded, "capacity_used": 0, "jumps": []M{{"name": "A", "id64": "1", "x": 0, "y": 0, "z": 0, "distance": 0, "fuel_used": 0, "fuel_in_tank": loaded, "tritium_in_market": 0, "restock_amount": 0}, {"name": "C", "id64": "2", "x": 400, "y": 0, "z": 0, "distance": 400, "fuel_used": burn, "fuel_in_tank": arrival, "tritium_in_market": 0, "restock_amount": 0}}}}), nil
	})
	state, err := planner.Search(context.Background(), M{"sourceSystem": "A", "destinationSystem": "C", "calculateStartingFuel": true, "reserveTritiumTons": 100}, M{})
	if err != nil {
		t.Fatal(err)
	}
	if jobs < 3 {
		t.Fatal("reserve not iteratively verified against extra fuel mass", jobs)
	}
	closeNumber(t, v.At(state, "summary", "arrivalTritiumTons"), 100)
	closeNumber(t, v.At(state, "summary", "restockTons"), 0)
	closeNumber(t, v.At(state, "summary", "minimumStartingTritiumTons"), 200)
	if planner.SelectStop(1) != nil || planner.SelectStop(99) == nil {
		t.Fatal("stop selection validation")
	}
	if csv, err := planner.ExportCSV(); err != nil || !strings.Contains(csv, "Система") {
		t.Fatal(csv, err)
	}
	if planner.MarkVisited(-1, "manual", "bad") == nil {
		t.Fatal("invalid progress accepted")
	}
}
func TestRouteRestockTimeAndWarnings(t *testing.T) {
	options := routeOptions(t)
	options["refuelMethod"] = "mine"
	options["miningTonsPerHour"] = 60
	options["refuelMinutes"] = 10
	options["maxQueueMinutes"] = 3
	options["tritiumPrice"] = 100
	options["reserveTritiumTons"] = 1000
	fixture := routeFixture()
	jumps := v.Rows(v.At(fixture, "result", "jumps"))
	jumps[1]["restock_amount"] = 100
	jumps[1]["fuel_in_tank"] = 500
	jumps[1]["must_restock"] = true
	jumps[2]["fuel_in_tank"] = 400
	result, err := NormalizeProviderRoute(fixture, options, M{})
	if err != nil {
		t.Fatal(err)
	}
	closeNumber(t, v.At(result, "summary", "miningMinutes"), 100)
	closeNumber(t, v.At(result, "summary", "refuelMinutes"), 10)
	closeNumber(t, v.At(result, "summary", "totalMinutes"), 147)
	closeNumber(t, v.At(result, "summary", "timeRange", "maxMinutes"), 153)
	closeNumber(t, v.At(result, "summary", "fuelCost"), 20000)
	if len(v.Arr(result["warnings"])) < 3 {
		t.Fatal("missing fuel/restock/ice warnings", result["warnings"])
	}
	if _, err := NormalizeProviderRoute(M{"status": "queued"}, options, nil); err == nil {
		t.Fatal("incomplete route accepted")
	}
	if _, err := RouteCSV(M{}); err == nil {
		t.Fatal("empty route exported")
	}
}

func TestPartialWarehouseDoesNotInventAbsentTritium(t *testing.T) {
	tracker := New(nil)
	tracker.Apply(ce("CarrierStats", M{"FuelLevel": 500}), nil)
	if err := tracker.SetStockBaseline(M{"items": []M{{"key": "gold", "count": 10}}, "complete": false}); err != nil {
		t.Fatal(err)
	}
	state := tracker.Snapshot(nil)
	if v.Bool(v.At(state, "warehouse", "complete")) || v.At(state, "fuel", "reserveTons") != nil {
		t.Fatal("partial manual stock inferred complete warehouse", state["warehouse"], state["fuel"])
	}
	restored := New(tracker.ExportSettings())
	restored.Apply(ce("CarrierStats", M{"FuelLevel": 500}), nil)
	if v.Bool(v.At(restored.Snapshot(nil), "warehouse", "complete")) {
		t.Fatal("partial baseline became complete on restart")
	}
	if err := tracker.SetStockBaseline(M{"items": []M{{"key": "gold", "count": 0}}, "complete": false}); err != nil {
		t.Fatal(err)
	}
	items := v.Rows(v.At(tracker.Snapshot(nil), "warehouse", "items"))
	if len(items) != 1 || v.Num(items[0]["count"]) != 0 {
		t.Fatal("known zero in partial snapshot was lost", items)
	}
	if err := tracker.SetStockBaseline(M{"items": []M{}, "complete": true}); err != nil {
		t.Fatal(err)
	}
	state = tracker.Snapshot(nil)
	if !v.Bool(v.At(state, "warehouse", "complete")) || v.Num(v.At(state, "fuel", "reserveTons")) != 0 {
		t.Fatal("confirmed empty warehouse must have known zero reserve")
	}
}
