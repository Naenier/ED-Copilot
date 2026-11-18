package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/journal"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestOverviewProjectsOnlyLandingCards(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	e.Apply(testEvent("Cargo", M{"Vessel": "Ship", "Inventory": []M{{"Name": "gold", "Count": 15}}}), nil)
	e.Apply(testEvent("CarrierStats", M{"CarrierID": 42, "Callsign": "XXX-YYY", "Name": "Test Carrier"}), nil)
	doOK(t, e, "/api/settings", M{"activity": "colonization"})
	doOK(t, e, "/api/colonization", M{"action": "create-project", "targetSystem": "Synthetic Colony", "originSystem": "Synthetic Origin"})

	landing := e.SnapshotForOverview()
	full := e.SnapshotForUI("overview")
	for _, path := range [][]string{
		{"commander", "cargoTons"}, {"pilot", "credits"}, {"pilot", "fuel"}, {"carrier", "name"}, {"carrier", "callsign"}, {"carrier", "location", "system"},
		{"objectives"}, {"riches"}, {"settings", "activity"}, {"theme", "palette"}, {"planWorkspace"},
	} {
		got, want := v.At(landing, path...), v.At(full, path...)
		if !reflect.DeepEqual(v.Clone(got), v.Clone(want)) {
			t.Errorf("overview card value differs for %v: %v != %v", path, got, want)
		}
	}
	if v.At(landing, "commander", "cargoTons") != float64(15) || v.At(landing, "carrier", "owned") != true {
		t.Fatal("fixture did not exercise cargo and carrier cards")
	}
	if len(v.Rows(v.At(landing, "coloniesSelectors", "projects"))) != 1 {
		t.Fatal("initial overlay colony selector is missing")
	}
	for _, key := range []string{"tradeLedger", "resources", "goods", "colonization", "engineering", "learning", "frontier", "ranks"} {
		if _, ok := landing[key]; ok {
			t.Errorf("landing snapshot eagerly loads %s", key)
		}
	}
	if len(v.Rows(v.At(full, "commander", "cargo"))) != 1 {
		t.Fatal("cargo table projection was removed")
	}
}

func TestNativeRefreshSkipsHUDOnlyWhenItCannotBeVisible(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	doOK(t, e, "/api/settings", M{"overlayEnabled": false, "activity": "colonization"})
	doOK(t, e, "/api/colonization", M{"action": "create-project", "targetSystem": "Synthetic Colony", "originSystem": "Synthetic Origin"})
	state := e.SnapshotForWindow()
	if state["desktop"] == nil || state["settings"] == nil || state["resources"] != nil || state["colonization"] != nil {
		t.Fatalf("wrong native initial projection: %v", state)
	}

	state = e.SnapshotForNativeRefresh()
	if state["resources"] != nil || len(v.Rows(v.At(state, "colonization", "projects"))) != 1 {
		t.Fatal("overlay off should preserve the selector without projecting the HUD")
	}
	doOK(t, e, "/api/settings", M{"overlayEnabled": true})
	e.mu.Lock()
	e.gameRunning = false
	e.mu.Unlock()
	if e.SnapshotForNativeRefresh()["resources"] != nil {
		t.Fatal("HUD data computed while the game is stopped")
	}
	e.mu.Lock()
	e.gameRunning = true
	e.mu.Unlock()
	if e.SnapshotForNativeRefresh()["resources"] == nil {
		t.Fatal("HUD data missing while the game and overlay are enabled")
	}
	e.Reader = journal.New(e.Reader.Dir, nil, nil)
	state = e.SnapshotForNativeRefresh()
	if !v.Bool(v.At(state, "connection", "scanning")) || state["resources"] != nil {
		t.Fatal("HUD data computed before the journal was ready")
	}
}

func TestOverviewPublishesCompletedJournalReplay(t *testing.T) {
	e, file, _ := fixtureEngine(t, nil)
	doOK(t, e, "/api/settings", M{"overlayEnabled": true})
	e.Apply(testEvent("FSDJump", M{"StarSystem": "Published System"}), nil)
	e.Apply(testEvent("FSDTarget", M{"Name": "Published Destination", "RemainingJumpsInRoute": 1}), nil)
	writeEvents(t, file, false, []M{
		testEvent("Fileheader", M{"gameversion": "4.4.2"}),
		testEvent("LoadGame", M{"Commander": "Synthetic Commander", "FID": "FSYNTHETIC", "Ship": "Python", "ShipID": 1}),
		testEvent("Location", M{"StarSystem": "Historical System"}),
		testEvent("FSDTarget", M{"Name": "Historical Destination"}),
		testEvent("FSDJump", M{"StarSystem": "Latest System"}),
	})
	navRoute := []byte(`{"timestamp":"2026-11-07T12:00:01Z","event":"NavRoute","Route":[{"StarSystem":"Latest System"},{"StarSystem":"Latest Destination"}]}`)
	if err := os.WriteFile(filepath.Join(filepath.Dir(file), "NavRoute.json"), navRoute, 0600); err != nil {
		t.Fatal(err)
	}

	var checked int
	var replayRevision float64
	apply := e.Reader.Apply
	e.Reader.Apply = func(event, meta M) {
		apply(event, meta)
		if kind := v.Str(event["event"]); kind != "FSDTarget" && kind != "NavRoute" || !v.Bool(meta["replay"]) {
			return
		}
		checked++
		for _, view := range []M{e.SnapshotForOverview(), e.SnapshotForWindow(), e.SnapshotForNativeRefresh(), e.SnapshotForUI("overview")} {
			if !v.Bool(v.At(view, "connection", "scanning")) || v.At(view, "commander", "system") != "Published System" {
				t.Fatalf("replay exposed an incomplete location: %v, %v", view["commander"], view["connection"])
			}
		}
		view := e.SnapshotForUI("overview")
		if v.At(view, "pilot", "navTarget", "system") != "Published Destination" {
			t.Fatalf("replay exposed an incomplete destination: %v", v.At(view, "pilot", "navTarget"))
		}
		if e.SnapshotForHUD()["resources"] != nil {
			t.Fatal("overlay received incomplete journal data")
		}
		replayRevision = v.Num(view["revision"])
	}
	e.Reader.Rescan()
	e.Reader.Poll()
	view := e.SnapshotForUI("overview")
	if checked != 2 || v.Bool(v.At(view, "connection", "scanning")) || v.Num(view["revision"]) <= replayRevision {
		t.Fatalf("completed replay was not published: checked %d, connection %v, revision %v after %v", checked, view["connection"], view["revision"], replayRevision)
	}
	if v.At(view, "pilot", "navTarget", "system") != "Latest Destination" {
		t.Fatalf("finished overview missed the latest side-file destination: %v", v.At(view, "pilot", "navTarget"))
	}
	for _, latest := range []M{view, e.SnapshotForOverview(), e.SnapshotForWindow(), e.SnapshotForNativeRefresh()} {
		if v.At(latest, "commander", "system") != "Latest System" {
			t.Fatalf("finished replay missed the latest location: %v", latest["commander"])
		}
	}
	e.Apply(testEvent("FSDJump", M{"StarSystem": "Live System"}), nil)
	if v.At(e.SnapshotForOverview(), "commander", "system") != "Live System" || v.At(e.SnapshotForNativeRefresh(), "commander", "system") != "Live System" {
		t.Fatal("live location stopped updating after replay")
	}
}
