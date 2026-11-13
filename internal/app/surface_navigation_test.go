package app

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestSurfaceNavigationReadsStatusAndTracksPlanetAcrossVehicles(t *testing.T) {
	e, file, _ := fixtureEngine(t, nil)
	body := "Synthetic Origin 1"
	start := v.Time("2026-11-07T12:00:00Z")
	eventAt := func(kind string, second int, fields M) M {
		return v.Merge(M{"event": kind, "timestamp": v.ISO(start.Add(time.Duration(second) * time.Second))}, fields)
	}
	statusPath := filepath.Join(filepath.Dir(file), "Status.json")
	writeStatus := func(second int, fields M) {
		t.Helper()
		data, err := json.Marshal(eventAt("Status", second, fields))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(statusPath, data, 0600); err != nil {
			t.Fatal(err)
		}
		e.Reader.Poll()
	}
	e.Apply(eventAt("ApproachBody", 1, M{"Body": body, "StarSystem": "Synthetic Origin"}), nil)
	owner := v.At(e.SnapshotForUI("surface-navigation"), "planWorkspace", "key")
	set, err := e.Do("/api/surface-navigation", M{"action": "set", "coordinates": "0 / 1", "expectedPlanWorkspace": owner})
	if err != nil || v.At(set, "surfaceNavigation", "status") != "waiting-position" ||
		v.At(set, "surfaceNavigation", "target", "bodyName") != body {
		t.Fatalf("expected a target bound to the approaching planet: %v, %v", set, err)
	}
	if _, err = e.Do("/api/surface-navigation", M{"action": "set", "coordinates": "91 / 1"}); err == nil {
		t.Fatal("invalid latitude accepted")
	}
	if _, err = e.Do("/api/surface-navigation", M{"action": "clear", "expectedPlanWorkspace": "another-pilot"}); err == nil {
		t.Fatal("write for a different commander accepted")
	}
	coords := func(flags, heading, longitude int) M {
		return M{"Flags": flags, "BodyName": body, "Latitude": 0, "Longitude": longitude, "Heading": heading, "PlanetRadius": 1e6}
	}
	writeStatus(2, coords(statusHasSurfaceCoordinates|1<<4, 80, 0))
	flight := e.SnapshotForHUD()
	guidance := v.Obj(v.At(flight, "surfaceNavigation", "guidance"))
	if v.At(flight, "surfaceNavigation", "status") != "active" || guidance["bearing"] != 90.0 ||
		guidance["relativeHeading"] != 10.0 || math.Abs(v.Num(guidance["distanceM"])-math.Pi/180*1e6) > .001 {
		t.Fatalf("unexpected orbital course: %v", flight["surfaceNavigation"])
	}
	hud := presentation.BuildHUD(flight, start)
	if len(hud.Sections) == 0 || hud.Sections[0].Kind != "surface-navigation" {
		t.Fatal("the flight overlay did not project the coordinate course")
	}
	e.Apply(eventAt("LaunchSRV", 3, nil), nil)
	writeStatus(4, coords(statusHasSurfaceCoordinates|1<<26, 100, 0))
	if v.At(e.SurfaceNavigationSnapshot(), "guidance", "relativeHeading") != -10.0 {
		t.Fatal("SRV heading did not update")
	}
	e.Apply(eventAt("Disembark", 5, nil), nil)
	foot := coords(statusHasSurfaceCoordinates, 0, 1)
	foot["Flags2"] = 1
	delete(foot, "PlanetRadius")
	writeStatus(6, foot)
	if nav := e.SurfaceNavigationSnapshot(); nav["status"] != "arrived" || v.At(nav, "guidance", "distanceM") != 0.0 {
		t.Fatalf("foot arrival did not use current coordinates: %v", nav)
	}
	e.Apply(eventAt("LeaveBody", 7, M{"Body": body, "StarSystem": "Synthetic Origin"}), nil)
	writeStatus(7, coords(statusHasSurfaceCoordinates, 0, 0))
	if nav := e.SurfaceNavigationSnapshot(); nav["status"] != "waiting-position" || nav["current"] != nil {
		t.Fatalf("left-body status reused: %v", nav)
	}
	e.Apply(eventAt("ApproachBody", 8, M{"Body": "Synthetic Origin 2", "StarSystem": "Synthetic Origin"}), nil)
	other := coords(statusHasSurfaceCoordinates, 0, 0)
	other["BodyName"] = "Synthetic Origin 2"
	writeStatus(9, other)
	if nav := e.SurfaceNavigationSnapshot(); nav["status"] != "wrong-body" || nav["guidance"] != nil {
		t.Fatalf("course computed on another planet: %v", nav)
	}
	writeStatus(10, M{"Flags": 0, "Latitude": 0, "Longitude": 0})
	if nav := e.SurfaceNavigationSnapshot(); nav["current"] != nil || nav["guidance"] != nil {
		t.Fatalf("position survived missing Has Lat Long: %v", nav)
	}
	e.Apply(eventAt("Commander", 11, M{"FID": "FANOTHERPILOT", "Name": "Other pilot"}), nil)
	if nav := e.SurfaceNavigationSnapshot(); nav["status"] != "no-target" || nav["current"] != nil {
		t.Fatalf("first commander's position or target leaked: %v", nav)
	}
}

func TestSurfaceNavigationPersistsPendingTargetAndWorksWithoutRadius(t *testing.T) {
	e, file, data := fixtureEngine(t, nil)
	if _, err := e.Do("/api/surface-navigation", M{"action": "set", "latitude": 44.4825, "longitude": -61.4854}); err != nil {
		t.Fatal(err)
	}
	e.Close()
	reopened, err := New(Options{DataDir: data, JournalDir: filepath.Dir(file), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if nav := reopened.SurfaceNavigationSnapshot(); nav["status"] != "waiting-position" ||
		v.At(nav, "target", "latitude") != 44.4825 || v.At(nav, "target", "bodyName") != nil {
		t.Fatalf("pending target failed to survive reload: %v", nav)
	}
	reopened.Apply(testEvent("Status", M{"Flags": statusHasSurfaceCoordinates, "BodyName": "Synthetic Origin 2",
		"Latitude": 44.4, "Longitude": -61.4}), M{"side": true})
	nav := reopened.SurfaceNavigationSnapshot()
	if nav["status"] != "active" || v.At(nav, "target", "bodyName") != "Synthetic Origin 2" ||
		v.At(nav, "guidance", "bearing") == nil || v.At(nav, "guidance", "distanceM") != nil {
		t.Fatalf("expected valid course with unknown radius: %v", nav)
	}
	clear, err := reopened.Do("/api/surface-navigation", M{"action": "clear"})
	if err != nil || v.At(clear, "surfaceNavigation", "status") != "no-target" {
		t.Fatalf("clear failed: %v, %v", clear, err)
	}
}
