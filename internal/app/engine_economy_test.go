package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type economyTransport struct {
	mu    sync.Mutex
	calls map[string]int
	stamp string
}

type economyRoundTripper func(*http.Request) (*http.Response, error)

func (f economyRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEngineSearchProgressRemainsRunningUntilFinalResponse(t *testing.T) {
	for _, kind := range []string{"mining", "market"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := fixtureEngine(t, nil)
			base := &economyTransport{calls: map[string]int{}, stamp: v.ISO(time.Now())}
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			stationCalls := 0
			e.opts.HTTPClient = &http.Client{Transport: economyRoundTripper(func(r *http.Request) (*http.Response, error) {
				block := kind == "mining" && r.URL.Path == "/api/bodies/search"
				if kind == "market" && r.URL.Path == "/api/stations/search" {
					stationCalls++
					block = stationCalls == 2
				}
				if block {
					close(entered)
					select {
					case <-release:
					case <-r.Context().Done():
						return nil, r.Context().Err()
					}
				}
				return base.RoundTrip(r)
			})}
			e.opts.DisableNetwork = false
			path, name, body := "/api/search", "search", M{"system": "Synthetic Origin", "resourceKey": "platinum"}
			if kind == "market" {
				path, name, body = "/api/trading/search", "trading", M{"mode": "sell", "useCargo": true}
			}
			doOK(t, e, path, body)
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("search never reached delayed response")
			}
			progress := v.Obj(e.Snapshot()[name])
			if !v.Bool(progress["running"]) {
				t.Fatal("intermediate progress incorrectly signaled job completion", progress)
			}
			releaseOnce.Do(func() { close(release) })
			finished := waitEngineJob(t, e, name)
			if kind == "mining" && finished["finishedAt"] == nil {
				t.Fatal("job stopped before terminal result", finished)
			}
		})
	}
}

func (f *economyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[r.URL.Path]++
	var payload any
	switch r.URL.Path {
	case "/api/dump/18446744073709551615":
		payload = M{"system": M{"name": "Synthetic Origin", "id64": json.Number("18446744073709551615"), "coords": M{"x": 0, "y": 0, "z": 0}, "bodies": []M{{"bodyId": 2, "name": "Synthetic Origin 2", "isLandable": true, "reserveLevel": "PristineResources", "distanceToArrival": 100, "materials": M{"Polonium": .2}, "rings": []M{{"name": "Synthetic Origin 2 A Ring", "type": "Metallic", "signals": M{"Platinum": 1}}}}}}}
	case "/api/bodies/search":
		payload = M{"count": 0, "results": []M{}}
	case "/api-v1/sphere-systems":
		payload = []M{}
	case "/api/stations/search":
		payload = M{"count": 2, "results": []M{
			{"market_id": 99, "name": "Synthetic Buyer", "system_name": "Destination", "system_id64": json.Number("18446744073709551614"), "type": "Coriolis", "has_large_pad": true, "market_updated_at": f.stamp, "system_x": 3, "system_y": 4, "system_z": 0, "distance_to_arrival": 100, "market": []M{{"commodity": "Gold", "sell_price": 150, "buy_price": 100, "demand": 100, "supply": 100}}},
			{"market_id": 100, "name": "Stale Buyer", "system_name": "Destination", "type": "FleetCarrier", "has_large_pad": true, "market_updated_at": v.ISO(v.Time(f.stamp).Add(-48 * time.Hour)), "system_x": 1, "system_y": 0, "system_z": 0, "distance_to_arrival": 10, "market": []M{{"commodity": "Gold", "sell_price": 999999, "demand": 100}}},
		}}
	case "/api/search/systems":
		name, id, x := r.URL.Query().Get("q"), "18446744073709551613", 0
		if name == "C" {
			id, x = "18446744073709551615", 800
		}
		payload = M{"results": []M{{"name": name, "id64": id, "x": x, "y": 0, "z": 0}}}
	case "/api/fleetcarrier/route":
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
		if r.Form.Get("fuel_loaded") != "500" || r.Form.Get("source") != "18446744073709551613" || r.Form.Get("destinations") != "18446744073709551615" {
			return nil, fmt.Errorf("invalid route payload: %v", r.Form)
		}
		payload = M{"status": "queued", "job": "12345678-1234-1234-1234-123456789012"}
	case "/api/results/12345678-1234-1234-1234-123456789012":
		payload = M{"status": "ok", "job": "12345678-1234-1234-1234-123456789012", "result": M{"capacity_used": 0, "fuel_loaded": 500, "tritium_stored": 0, "jumps": []M{
			{"name": "A", "id64": "18446744073709551613", "x": 0, "y": 0, "z": 0, "distance": 0, "fuel_used": 0, "fuel_in_tank": 500, "tritium_in_market": 0, "restock_amount": 0},
			{"name": "B", "id64": "18446744073709551614", "x": 400, "y": 0, "z": 0, "distance": 400, "fuel_used": 100, "fuel_in_tank": 400, "tritium_in_market": 0, "restock_amount": 0},
			{"name": "C", "id64": "18446744073709551615", "x": 800, "y": 0, "z": 0, "distance": 400, "fuel_used": 100, "fuel_in_tank": 300, "tritium_in_market": 0, "restock_amount": 0},
		}}}
	default:
		return nil, fmt.Errorf("network smoke rejects unconfigured URL %s", r.URL)
	}
	raw, _ := json.Marshal(payload)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw)), Request: r}, nil
}

func waitEngineJob(t *testing.T, e *Engine, name string) M {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state := v.Obj(e.Snapshot()[name])
		if !v.Bool(state["running"]) {
			if v.Str(state["error"]) != "" {
				t.Fatalf("%s failed: %v", name, state)
			}
			return state
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%s did not finish", name)
	return nil
}

func TestEngineEconomyNetworkSmokeAndRouteRestart(t *testing.T) {
	first, file, dir := fixtureEngine(t, nil)
	first.Close()
	transport := &economyTransport{calls: map[string]int{}, stamp: v.ISO(time.Now())}
	engine, err := New(Options{DataDir: dir, JournalDir: filepath.Dir(file), HTTPClient: &http.Client{Transport: transport}, DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	// Enable only the command guard after bootstrap; no OAuth listener or account
	// workers are created, and every HTTP request is handled by this fixture.
	engine.opts.DisableNetwork = false
	engine.Route.PollInterval = time.Millisecond
	doOK(t, engine, "/api/search", M{"system": "Synthetic Origin", "resourceKey": "platinum"})
	search := waitEngineJob(t, engine, "search")
	if len(v.Rows(search["systems"])) != 1 || len(v.Rows(engine.Snapshot()["targets"])) < 2 {
		t.Fatal("remote body did not join journal targets", search, engine.Snapshot()["targets"])
	}
	doOK(t, engine, "/api/trading/search", M{"mode": "sell", "resourceKey": "gold", "useCargo": true, "maxAgeHours": 1})
	market := waitEngineJob(t, engine, "trading")
	rows := v.Rows(market["results"])
	if len(rows) != 1 || rows[0]["station"] != "Synthetic Buyer" {
		t.Fatal("fresh buyer selection", rows)
	}
	expectNumber(t, rows[0]["estimatedTotal"], 1500)
	expectNumber(t, rows[0]["distanceLy"], 5)
	doOK(t, engine, "/api/trading/destination", M{"destination": rows[0]})
	engine.Apply(testEvent("EjectCargo", M{"Type": "Gold", "Count": 1}), M{})
	if !v.Bool(v.At(engine.Snapshot(), "trading", "destination", "cargoChanged")) {
		t.Fatal("selected market was not invalidated by changed cargo")
	}
	doOK(t, engine, "/api/trading/routes", M{"resourceKeys": []string{"gold"}, "credits": 10000, "cargoCapacity": 10})
	routes := waitEngineJob(t, engine, "tradeRoutes")
	if len(v.Rows(routes["results"])) != 0 {
		t.Fatal("same-market cycle yielded artificial profit", routes)
	}
	doOK(t, engine, "/api/carrier/route", M{"sourceSystem": "A", "destinationSystem": "C", "tankTons": 500, "calculateStartingFuel": false, "departureTime": "2026-11-07T12:00:00Z"})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state := engine.Snapshot()
		if !v.Bool(v.At(state, "carrierRoute", "running")) && v.At(state, "carrierRoute", "route") != nil {
			break
		}
		if v.Str(v.At(state, "carrierRoute", "error")) != "" {
			t.Fatal(state["carrierRoute"])
		}
		time.Sleep(time.Millisecond)
	}
	state := engine.Snapshot()
	if v.At(state, "carrierRoute", "status") != "ready" {
		t.Fatal("route job did not finish", state["carrierRoute"])
	}
	expectNumber(t, v.At(state, "carrierRoute", "summary", "fuelTons"), 200)
	expectNumber(t, v.At(state, "carrierRoute", "summary", "travelMinutes"), 37)
	stamp := v.ISO(time.Now().Add(time.Second))
	writeEvents(t, file, true, []M{testEvent("CarrierStats", M{"CarrierID": "42", "Name": "Synthetic Carrier", "FuelLevel": 400, "timestamp": stamp}), testEvent("CarrierLocation", M{"CarrierID": "42", "StarSystem": "B", "SystemAddress": json.Number("18446744073709551614"), "timestamp": stamp})})
	engine.Reader.Poll()
	expectNumber(t, v.At(engine.Snapshot(), "carrierRoute", "summary", "completedJumps"), 1)
	doOK(t, engine, "/api/carrier/route/select", M{"index": 2})
	if csv, err := engine.ExportCSV("route"); err != nil || !strings.Contains(csv, "18446744073709551615") {
		t.Fatal(csv, err)
	}
	engine.Close()
	restored, err := New(Options{DataDir: dir, JournalDir: filepath.Dir(file), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	expectNumber(t, v.At(restored.Snapshot(), "carrierRoute", "summary", "completedJumps"), 1)
	if v.At(restored.Snapshot(), "trading", "destination", "station") != "Synthetic Buyer" {
		t.Fatal("selected market lost at restart")
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	for _, path := range []string{"/api/dump/18446744073709551615", "/api/stations/search", "/api/fleetcarrier/route", "/api/results/12345678-1234-1234-1234-123456789012"} {
		if transport.calls[path] == 0 {
			t.Fatal("mock endpoint not exercised", path)
		}
	}
}
