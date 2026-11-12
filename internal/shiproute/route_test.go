package shiproute

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type testTransport func(*http.Request) (*http.Response, error)

func (fn testTransport) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestExactPlannerKeepsFuelAndExactAddresses(t *testing.T) {
	const origin = "18446744073709551611"
	const end = "18446744073709551612"
	const job = "00000000-0000-0000-0000-000000000001"
	calls := 0
	planner := NewPlanner(&http.Client{Transport: testTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		var response string
		switch req.URL.Path {
		case "/api/search/systems":
			switch req.URL.Query().Get("q") {
			case "Sol":
				response = `{"results":[{"name":"Sol X","id64":"1"},{"name":"Sol","id64":` + origin + `}]}`
			case "Lave":
				response = `{"results":[{"name":"Lave","id64":"` + end + `"}]}`
			}
		case "/api/generic/route":
			if req.Method != http.MethodPost || req.ParseForm() != nil ||
				req.Form.Get("source") != origin || req.Form.Get("destination") != end ||
				req.Form.Get("ship_build") != "{}" || req.Form.Get("max_fuel_per_jump") != "5" {
				t.Errorf("unexpected exact request: %v", req.Form)
			}
			response = `{"status":"queued","job":"` + job + `"}`
		case "/api/results/" + job:
			response = `{"status":"running"}`
			if calls == 5 {
				response = `{"status":"ok","result":{"jumps":[
				  {"name":"Sol","id64":"` + origin + `","fuel_in_tank":32,"is_scoopable":true},
				  {"name":"Refuel","id64":"2","distance_jumped":20,"fuel_in_tank":32,"fuel_used":3.4,"is_scoopable":true,"must_refuel":true},
				  {"name":"Lave","id64":"` + end + `","distance_jumped":10,"fuel_in_tank":30,"fuel_used":2}
				]}}`
			}
		default:
			t.Errorf("unexpected request: %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
	})})
	planner.poll = time.Millisecond
	opts := Options{Origin: "Sol", Destination: "Lave", Mode: "exact", JumpRangeLY: 50, Efficiency: 60,
		ShipBuild: &FlightParameters{FuelPower: 2.45, FuelMultiplier: .012, OptimalMass: 1050, BaseMass: 300, TankSize: 32, MaxFuelPerJump: 5}}
	route, err := planner.Plan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	refuel := route.Stops[1]
	if calls != 5 || len(route.Stops) != 3 || route.Stops[0].SystemAddress != origin || route.Stops[2].SystemAddress != end ||
		!refuel.Refuel || refuel.Jumps != 1 || refuel.FuelUsed == nil || *refuel.FuelUsed != 3.4 || StarKind(refuel.StarClass) != "scoopable" ||
		StarKind(route.Stops[2].StarClass) != "unknown" || route.MaxFuelPerJump != 5 {
		t.Fatalf("wrong exact route: %+v", route)
	}
	route.Next = 1
	state := route.snapshot(M{"pilot": M{"fuel": M{"mainTons": 3.0, "mainCapacityTons": 32}}})
	if next := v.Obj(state["nextStop"]); next["system"] != "Refuel" || next["fuelCaution"] != true {
		t.Fatalf("live fuel should warn before first jump: %+v", next)
	}
}

func TestNeutronAnchorsPreserveEstimatedLegs(t *testing.T) {
	response := M{"result": M{"system_jumps": []M{
		{"system": "Sol"},
		{"system": "Neutron A", "neutron_star": true, "jumps": 3, "distance_jumped": 110},
		{"system": "Lave", "jumps": 1, "distance_jumped": 180},
	}}}
	route, err := parseRoute(response, Options{Origin: "Sol", Destination: "Lave", Mode: "neutron", JumpRangeLY: 50})
	if err != nil {
		t.Fatal(err)
	}
	if route.Stops[1].Jumps != 3 || route.Stops[1].Boost != "neutron" || route.Stops[2].Boost != "" {
		t.Fatalf("wrong anchors: %+v", route.Stops)
	}
	state := route.snapshot(nil)
	if rows := v.Rows(state["stops"]); rows[2]["rangeCaution"] == true {
		t.Fatal("jump after a neutron departure should not be flagged as out of range")
	}
}

func TestJourneyScopesReplayAndStationStops(t *testing.T) {
	s := New(nil)
	ctx := M{"commander": M{"system": "Sol", "systemAddress": "1"}}
	err := s.Update("A", M{"action": "import", "name": "Supply",
		"autoCopy": true, "waypoints": []M{{"system": "Lave", "systemAddress": "2"}, {"system": "Diso", "station": "Shifnalport", "action": "Collect materials"}, {"system": "Leesti"}}}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	r := s.active("A")
	r.CreatedAt, r.ObservedFrom = "2026-11-13T12:00:01.500Z", "2026-11-13T12:00:01.500Z"
	if r.Next != 1 {
		t.Fatal("the origin should be marked as visited")
	}
	jump := M{"event": "FSDJump", "timestamp": "2026-11-13T12:00:01Z", "StarSystem": "Lave", "SystemAddress": "2"}
	if copied := s.Observe("B", jump, nil); copied != "" || r.Next != 1 {
		t.Fatal("another commander affected progress")
	}
	if copied := s.Observe("A", jump, M{"replay": true}); copied != "" || r.Next != 1 {
		t.Fatal("same-second replay from before the route must be ignored")
	}
	if copied := s.Observe("A", jump, nil); copied != "Diso" || r.Next != 2 {
		t.Fatalf("same-second live arrival did not advance/copy: %q %d", copied, r.Next)
	}
	dock := M{"event": "Docked", "timestamp": "2026-11-13T12:00:03Z", "StarSystem": "Diso", "StationName": "Somewhere Else"}
	s.Observe("A", dock, nil)
	if r.Next != 2 {
		t.Fatal("wrong station advanced the route")
	}
	dock["StationName"] = "Shifnalport"
	if copied := s.Observe("A", dock, M{"replay": true}); copied != "" || r.Next != 3 {
		t.Fatalf("replay should catch up without copying: %q %d", copied, r.Next)
	}
	restored := New(s.Export())
	if restored.active("A").Next != 3 || restored.active("B") != nil || restored.active("A").Stops[2].Action != "Collect materials" {
		t.Fatal("persisted routes lost progress or station actions")
	}
}
