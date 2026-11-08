package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/profile"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestSeedDemoShowcaseIncludesNormalizedCarrierRoute(t *testing.T) {
	engine, _, dataDir := fixtureEngine(t, nil)
	if len(v.Rows(v.At(engine.Route.Snapshot(), "route", "stops"))) != 0 {
		t.Fatal("ordinary synthetic engine unexpectedly starts with a demo route")
	}
	if err := engine.SeedDemoShowcase(); err != nil {
		t.Fatal(err)
	}
	state := v.Obj(engine.SnapshotForUI("route")["carrierRoute"])
	if state["status"] != "ready" || v.Bool(state["running"]) || v.Str(state["error"]) != "" {
		t.Fatal("showcase route is not ready", state["status"], state["error"])
	}
	stops := v.Rows(v.At(state, "route", "stops"))
	if len(stops) != 3 || !v.Bool(stops[0]["visited"]) || v.Bool(stops[1]["visited"]) || v.Str(stops[1]["name"]) != "Учебная промежуточная система" {
		t.Fatal("showcase has no meaningful next jump", stops)
	}
	for _, check := range []struct {
		path []string
		want float64
	}{
		{[]string{"summary", "totalJumps"}, 2}, {[]string{"summary", "distanceLy"}, 800},
		{[]string{"summary", "fuelTons"}, 200}, {[]string{"summary", "fuelCost"}, 10000000},
		{[]string{"summary", "startingTritiumTons"}, 1500}, {[]string{"summary", "arrivalTritiumTons"}, 1300},
		{[]string{"summary", "reserveShortfallTons"}, 0}, {[]string{"summary", "totalMinutes"}, 37},
		{[]string{"summary", "timeRange", "maxMinutes"}, 57}, {[]string{"route", "mass", "startingTotalMassTons"}, 32500},
	} {
		if got := v.Num(v.At(state, check.path...)); got != check.want {
			t.Fatalf("%v = %v, want %v", check.path, got, check.want)
		}
	}
	departure := v.Time(v.At(state, "summary", "departureTime"))
	arrival := v.Time(v.At(state, "summary", "estimatedArrival"))
	if departure.IsZero() || arrival.Sub(departure) != 37*time.Minute {
		t.Fatal("showcase ETA was not derived from its jump plan", departure, arrival)
	}
	if state["source"] != "Демонстрационные данные" || len(v.Arr(state["warnings"])) == 0 {
		t.Fatal("synthetic route is not clearly labelled")
	}
	saved, err := profile.Read(filepath.Join(dataDir, "settings-go-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Num(v.At(saved, "carrierRouteGo", "state", "summary", "fuelTons")) != 200 {
		t.Fatal("normalized showcase route was not saved")
	}
}
