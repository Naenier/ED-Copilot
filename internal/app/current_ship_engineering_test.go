package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/profile"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const currentShipEngineeringPath = "/api/engineering/current-ship"
const currentShipEngineeringPlanPath = currentShipEngineeringPath + "/plan"

func shipPlanGoals(t *testing.T, e *Engine) []M {
	t.Helper()
	return v.Rows(v.At(researchResult(t, e, currentShipEngineeringPath, nil), "plan", "goals"))
}

func observedShipPlanLoadout(id any, at time.Time, items ...M) M {
	return testEvent("Loadout", M{"Ship": "python", "ShipID": id, "Modules": items, "timestamp": v.ISO(at)})
}

func TestCurrentShipEngineeringTargetsStayWithShipAndCommander(t *testing.T) {
	e, file, dir := fixtureEngine(t, nil)
	owner := v.Str(v.At(e.SnapshotForUI("engineering-roadmap"), "planWorkspace", "key"))
	shipID := "9007199254740999"
	at := time.Date(2026, time.November, 7, 12, 0, 1, 0, time.UTC)
	fsd := M{"Slot": "FrameShiftDrive", "Item": "$int_hyperdrive_size5_class5_name;"}
	thrusters := M{"Slot": "MainEngines", "Item": "int_engine_size6_class5"}
	loadout := observedShipPlanLoadout(json.Number(shipID), at, fsd, thrusters)
	e.Apply(loadout, nil)
	writeEvents(t, file, true, []M{loadout})
	fsdGoal := M{"action": "upsert", "slot": "FrameShiftDrive", "item": "INT_HYPERDRIVE_SIZE5_CLASS5",
		"blueprintKey": "FSD_LongRange", "grade": 5, "experimentalKey": "special_fsd_cooled", "expectedPlanWorkspace": owner, "expectedShipID": shipID}
	thrustersGoal := v.Merge(v.Copy(fsdGoal), M{"slot": "MainEngines", "item": "int_engine_size6_class5", "blueprintKey": "Engine_Dirty", "experimentalKey": ""})
	first := researchResult(t, e, currentShipEngineeringPlanPath, fsdGoal)
	if first["shipID"] != shipID || len(v.Rows(v.At(first, "plan", "goals"))) != 1 ||
		v.At(first, "detail", "item") != fsd["Item"] {
		t.Fatal("new goal did not retain exact ship identity or observed module")
	}
	researchResult(t, e, currentShipEngineeringPlanPath, thrustersGoal)
	fsdGoal["grade"] = 4
	researchResult(t, e, currentShipEngineeringPlanPath, fsdGoal)
	view := researchResult(t, e, currentShipEngineeringPath, M{"slot": "FrameShiftDrive"})
	if len(v.Rows(v.At(view, "plan", "goals"))) != 2 || v.Int(v.At(view, "detail", "grade")) != 4 {
		t.Fatal("upsert duplicated the slot or did not restore its chosen target")
	}

	e.Apply(testEvent("ModuleStore", M{"ShipID": json.Number(shipID), "Slot": "FrameShiftDrive", "timestamp": v.ISO(at.Add(time.Second))}), nil)
	view = researchResult(t, e, currentShipEngineeringPath, nil)
	if view["shipID"] != shipID || view["loadoutKnown"] != false || len(v.Rows(v.At(view, "plan", "goals"))) != 2 {
		t.Fatal("outfitting event hid or deleted the ship's saved goals")
	}
	e.Apply(observedShipPlanLoadout(json.Number(shipID), at.Add(2*time.Second), thrusters), nil)
	goals := shipPlanGoals(t, e)
	if len(goals) != 2 || goals[0]["status"] != "unavailable" {
		t.Fatal("missing module was applied to a different installed slot or discarded")
	}
	e.Apply(observedShipPlanLoadout(0, at.Add(3*time.Second), fsd, thrusters), nil)
	view = researchResult(t, e, currentShipEngineeringPath, nil)
	if view["shipID"] != "0" || len(shipPlanGoals(t, e)) != 0 {
		t.Fatal("same-model ship received another ship's plan")
	}
	if _, err := e.Do(currentShipEngineeringPlanPath, fsdGoal); err == nil {
		t.Fatal("stale ship write was accepted")
	}
	newShip := v.Merge(v.Copy(fsdGoal), M{"expectedShipID": "0"})
	researchResult(t, e, currentShipEngineeringPlanPath, newShip)

	e.Apply(testEvent("Commander", M{"FID": "FPLANOTHER", "Name": "Other Pilot", "timestamp": v.ISO(at.Add(4 * time.Second))}), nil)
	e.Apply(observedShipPlanLoadout(0, at.Add(5*time.Second), fsd, thrusters), nil)
	if len(shipPlanGoals(t, e)) != 0 {
		t.Fatal("another commander saw the same ship ID's goals")
	}
	if _, err := e.Do(currentShipEngineeringPlanPath, newShip); err == nil {
		t.Fatal("stale commander write was accepted")
	}

	restarted, err := New(Options{DataDir: dir, JournalDir: filepath.Dir(file), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restarted.Close)
	if len(shipPlanGoals(t, restarted)) != 2 {
		t.Fatal("ship goals were not saved to disk by the update command")
	}
	remove := M{"action": "remove", "slot": "FrameShiftDrive", "expectedPlanWorkspace": owner, "expectedShipID": shipID}
	for range 2 {
		researchResult(t, restarted, currentShipEngineeringPlanPath, remove)
	}
	if goals := shipPlanGoals(t, restarted); len(goals) != 1 || goals[0]["slot"] != "MainEngines" {
		t.Fatal("removing one goal changed other goals")
	}
	researchResult(t, restarted, currentShipEngineeringPlanPath, v.Merge(v.Copy(remove), M{"action": "clear"}))
	if len(shipPlanGoals(t, restarted)) != 0 {
		t.Fatal("clear did not remove all ship goals")
	}
}

func TestCurrentShipEngineeringPlanRejectsInvalidAndFailedWrites(t *testing.T) {
	e, _, dir := fixtureEngine(t, nil)
	owner := v.Str(v.At(e.SnapshotForUI("engineering-roadmap"), "planWorkspace", "key"))
	at := time.Date(2026, time.November, 7, 12, 0, 1, 0, time.UTC)
	e.Apply(observedShipPlanLoadout(12, at, M{"Slot": "FrameShiftDrive", "Item": "int_hyperdrive_size5_class5"}), nil)
	good := M{"action": "upsert", "slot": "FrameShiftDrive", "item": "int_hyperdrive_size5_class5",
		"blueprintKey": "FSD_LongRange", "grade": 4, "experimentalKey": "", "expectedPlanWorkspace": owner, "expectedShipID": "12"}
	for _, fields := range []M{
		{"expectedPlanWorkspace": nil}, {"expectedShipID": nil}, {"item": "int_hyperdrive_size6_class5"},
		{"slot": "MainEngines"}, {"blueprintKey": "Engine_Dirty"}, {"grade": 1.5}, {"experimentalKey": "special_weapon_toughened"},
	} {
		if _, err := e.Do(currentShipEngineeringPlanPath, v.Merge(v.Copy(good), fields)); err == nil {
			t.Fatalf("accepted invalid goal: %v", fields)
		}
	}
	if len(shipPlanGoals(t, e)) != 0 {
		t.Fatal("rejected input changed the plan")
	}
	researchResult(t, e, currentShipEngineeringPlanPath, good)
	file := filepath.Join(dir, profile.SettingsFile)
	if err := os.WriteFile(file, []byte("corrupt test profile"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Do(currentShipEngineeringPlanPath, v.Merge(v.Copy(good), M{"grade": 5})); err == nil {
		t.Fatal("save to an invalid profile unexpectedly succeeded")
	}
	if goals := shipPlanGoals(t, e); len(goals) != 1 || v.Int(goals[0]["grade"]) != 4 {
		t.Fatal("failed persistence changed the live plan")
	}
	researchResult(t, e, currentShipEngineeringPlanPath, good)
}

func TestCurrentShipEngineeringPlanReflectsJournalAndPortableExport(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	owner := v.Str(v.At(e.SnapshotForUI("engineering-roadmap"), "planWorkspace", "key"))
	at := time.Date(2026, time.November, 7, 12, 0, 1, 0, time.UTC)
	e.Apply(observedShipPlanLoadout(11, at, M{"Slot": "FrameShiftDrive", "Item": "int_hyperdrive_size5_class5"}), nil)
	e.Apply(testEvent("EngineerProgress", M{"EngineerID": 300100, "Progress": "Unlocked", "Rank": 5}), nil)
	read := func() M {
		t.Helper()
		return v.Obj(researchResult(t, e, currentShipEngineeringPath, nil)["plan"])
	}
	findMaterial := func(plan M, key string) M {
		for _, item := range v.Rows(plan["materials"]) {
			if item["key"] == key {
				return item
			}
		}
		return nil
	}
	goal := M{"action": "upsert", "slot": "FrameShiftDrive", "item": "int_hyperdrive_size5_class5",
		"blueprintKey": "FSD_LongRange", "grade": 1, "experimentalKey": "", "expectedPlanWorkspace": owner, "expectedShipID": "11"}
	researchResult(t, e, currentShipEngineeringPlanPath, goal)
	plan := read()
	if len(v.Rows(v.At(plan, "itinerary", "stops"))) != 1 {
		t.Fatal("the saved target did not produce a workshop visit")
	}
	if item := findMaterial(plan, "disruptedwakeechoes"); item == nil || item["owned"] != nil {
		t.Fatal("unknown stock was presented as known")
	}
	e.Apply(testEvent("Materials", M{"Encoded": []M{{"Name": "disruptedwakeechoes", "Count": 8}}}), nil)
	if item := findMaterial(read(), "disruptedwakeechoes"); v.Int(item["owned"]) != 8 || v.Int(item["missing"]) != 0 {
		t.Fatalf("updated inventory did not flow into the saved plan: %v", item)
	}
	e.Apply(testEvent("EngineerCraft", M{"Slot": "FrameShiftDrive", "Module": "int_hyperdrive_size5_class5",
		"BlueprintName": "FSD_LongRange", "Level": 1, "Quality": 1, "timestamp": v.ISO(at.Add(time.Second))}), nil)
	plan = read()
	if v.Int(v.At(plan, "summary", "complete")) != 1 || len(v.Rows(plan["materials"])) > 0 || len(v.Rows(v.At(plan, "itinerary", "stops"))) > 0 {
		t.Fatal("completed work was not removed from materials and itinerary")
	}
	archive := filepath.Join(t.TempDir(), "engineering-profile.json")
	if err := e.ExportProfile(archive); err != nil {
		t.Fatal(err)
	}
	imported := filepath.Join(t.TempDir(), "imported")
	if err := profile.ImportPortable(archive, imported); err != nil {
		t.Fatal(err)
	}
	settings, err := profile.LoadSettings(imported)
	if err != nil || len(v.Rows(v.At(settings, "currentShipEngineeringPlans", owner, "11"))) != 1 {
		t.Fatal("portable export did not preserve the saved goal")
	}
}

func TestCurrentShipEngineeringAllowsEditingObsoleteSavedBlueprint(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	at := time.Date(2026, time.November, 7, 12, 0, 1, 0, time.UTC)
	const item = "int_hyperdrive_size5_class5"
	const removedBlueprint = "FSD_RemovedBlueprint"
	e.Apply(observedShipPlanLoadout(11, at, M{"Slot": "FrameShiftDrive", "Item": item}), nil)
	owner := v.Str(v.At(e.SnapshotForUI("engineering-roadmap"), "planWorkspace", "key"))
	e.section7.currentShipEngineeringPlans[owner] = M{"11": []M{{"slot": "FrameShiftDrive", "item": item,
		"blueprintKey": removedBlueprint, "grade": 5, "experimentalKey": ""}}}
	result := researchResult(t, e, currentShipEngineeringPath, M{"slot": "FrameShiftDrive"})
	if detail := v.Obj(result["detail"]); detail["blueprintKey"] == "" || detail["blueprintKey"] == removedBlueprint {
		t.Fatal("an obsolete saved blueprint blocked the available recipes")
	}
	if goals := v.Rows(v.At(result, "plan", "goals")); len(goals) != 1 || goals[0]["status"] != "unavailable" || goals[0]["blueprintKey"] != removedBlueprint {
		t.Fatal("an obsolete saved goal was lost or incorrectly treated as available")
	}
	if _, err := e.Do(currentShipEngineeringPath, M{"slot": "FrameShiftDrive", "blueprintKey": removedBlueprint}); err == nil {
		t.Fatal("an invalid explicit blueprint request was accepted")
	}
}
