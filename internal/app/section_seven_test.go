package app

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func researchResult(t *testing.T, e *Engine, path string, body M) M {
	t.Helper()
	result, err := e.Do(path, body)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return result
}

func TestResearchWorkspacesAndShipRoutePersistWithoutClipboardReplay(t *testing.T) {
	e, file, dir := fixtureEngine(t, nil)
	owner := v.Str(v.At(e.SnapshotForUI("section-seven"), "planWorkspace", "key"))
	e.Apply(testEvent("Loadout", M{
		"Ship": "Python", "ShipID": 1,
		"Modules": []M{{"Slot": "PowerPlant", "Item": "int_powerplant_size7_class5", "On": true}},
	}), nil)
	build := researchResult(t, e, "/api/ship-builds/update", M{"action": "baseline", "expectedPlanWorkspace": owner})
	if v.At(build, "baseline", "id") != "baseline" {
		t.Fatalf("journal loadout did not become a baseline: %v", build)
	}
	expeditions := researchResult(t, e, "/api/exploration/update", M{
		"action": "save-expedition", "name": "Test expedition", "expectedPlanWorkspace": owner,
	})
	if len(v.Rows(expeditions["expeditions"])) != 1 {
		t.Fatalf("expedition was not saved: %v", expeditions)
	}
	const first = "9007199254740997"
	const second = "9007199254740998"
	const third = "9007199254740999"
	route := researchResult(t, e, "/api/ship-route", M{
		"action": "import", "name": "Test route", "autoCopy": true, "expectedPlanWorkspace": owner,
		"waypoints": []M{{"system": "Target One", "systemAddress": first}, {"system": "Target Two", "systemAddress": second}, {"system": "Target Three", "systemAddress": third}},
	})
	if v.At(route, "route", "nextStop", "system") != "Target One" || e.PendingClipboard() != nil {
		t.Fatalf("import changed the clipboard or picked a wrong waypoint: %v", route)
	}
	jump := func(name, address string, at time.Time) M {
		return testEvent("FSDJump", M{"StarSystem": name, "SystemAddress": json.Number(address), "timestamp": v.ISO(at)})
	}
	at := time.Now().Add(time.Second)
	e.Apply(jump("Target One", second, at), nil)
	if e.PendingClipboard() != nil {
		t.Fatal("a same-name system with the wrong ID advanced the route")
	}
	e.Apply(jump("Target One", first, at), nil)
	pending := e.PendingClipboard()
	if pending["text"] != "Target Two" || pending["kind"] != "ship-route" {
		t.Fatalf("confirmed live waypoint did not copy its successor: %v", pending)
	}
	e.AcknowledgeClipboard(v.Str(pending["id"]))
	if v.At(e.SnapshotForHUD(), "shipRoute", "system") != "Target Two" {
		t.Fatal("overlay did not receive the current route target")
	}
	e.Apply(testEvent("Commander", M{"FID": "FSECTIONSEVENOTHER", "Name": "Other Pilot"}), nil)
	other := researchResult(t, e, "/api/ship-route", M{})
	otherBuild := researchResult(t, e, "/api/ship-builds", M{})
	otherExp := researchResult(t, e, "/api/exploration/query", M{})
	if other["route"] != nil || otherBuild["baseline"] != nil || len(v.Rows(otherExp["expeditions"])) != 0 {
		t.Fatal("workspace data leaked across commanders")
	}
	if _, err := e.Do("/api/exploration/update", M{"action": "save-expedition", "name": "Wrong owner", "expectedPlanWorkspace": owner}); err == nil {
		t.Fatal("stale action from another profile was accepted")
	}
	e.Close()
	writeEvents(t, file, true, []M{jump("Target Two", second, at.Add(time.Second))})
	restarted, err := New(Options{DataDir: dir, JournalDir: filepath.Dir(file), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if resumed := researchResult(t, restarted, "/api/ship-route", M{}); v.At(resumed, "route", "nextStop", "system") != "Target Three" || restarted.PendingClipboard() != nil {
		t.Fatalf("journal replay must catch up route without touching the clipboard: %v", resumed)
	}
}

func TestObjectiveNoticeHistoryStaysWithinCommanderAndUsesHUD(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	researchResult(t, e, "/api/notifications/update", M{"preferences": M{"enabled": true}})
	doOK(t, e, "/api/objectives", M{"action": "add", "title": "Synthetic delivery"})
	goal := v.Rows(e.SnapshotForUI("targets")["objectives"])[0]
	for range 2 {
		doOK(t, e, "/api/objectives", M{"action": "complete", "id": goal["id"]})
	}
	history := v.Rows(v.Obj(v.Clone(researchResult(t, e, "/api/notifications", nil)))["events"])
	if len(history) != 1 || history[0]["englishTitle"] != "Objective completed" ||
		v.At(e.SnapshotForHUD(), "notificationHUD", "detail") != goal["title"] {
		t.Fatalf("invalid notice projection: %v", history)
	}
	e.Apply(testEvent("Commander", M{"FID": "FSECTIONSEVENOTHER", "Name": "Other Pilot"}), nil)
	if v.At(e.SnapshotForHUD(), "notificationHUD", "detail") != nil ||
		len(v.Rows(v.Obj(v.Clone(researchResult(t, e, "/api/notifications", nil)))["events"])) != 0 {
		t.Fatal("notice from previous commander is still visible")
	}
}

func TestItineraryReactsToEngineeringPlanEditWithoutNewJournalEvent(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	read := func() int {
		return v.Int(researchResult(t, e, "/api/engineering/itinerary", nil)["goalCount"])
	}
	if got := read(); got != 0 {
		t.Fatalf("empty plan contained %d goals", got)
	}
	doOK(t, e, "/api/engineering", M{"action": "add", "moduleKey": "am", "blueprintKey": "AFM_Shielded", "quantity": 1, "rolls": []int{1, 0, 0, 0, 0}})
	if got := read(); got != 1 {
		t.Fatalf("added goal did not invalidate itinerary: %d", got)
	}
	doOK(t, e, "/api/engineering", M{"action": "clear"})
	if got := read(); got != 0 {
		t.Fatalf("cleared goal remained in itinerary: %d", got)
	}
}
