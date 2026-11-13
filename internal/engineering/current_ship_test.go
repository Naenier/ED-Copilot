package engineering

import (
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestCurrentShipEngineeringKeepsInstalledGradesAndAccessSeparate(t *testing.T) {
	planner := New(nil)
	for _, name := range []string{"Broo Tarquin", "The Dweller", "Mel Brandon"} {
		planner.Apply(M{"event": "EngineerProgress", "Engineer": name, "Progress": "Known"}, nil)
	}
	planner.Apply(M{"event": "EngineerProgress", "EngineerID": 300100, "Progress": "Unlocked", "Rank": 5}, nil)
	planner.Apply(M{"event": "Materials", "Raw": []M{}, "Manufactured": []M{}, "Encoded": []M{}}, nil)
	loadout := M{"Ship": "panthermkii", "ShipID": 9, "Modules": []M{
		{"Slot": "Armour", "Item": "panthermkii_armour_grade2", "Engineering": M{"BlueprintName": "Armour_HeavyDuty", "Level": 5, "Quality": 1}},
		{"Slot": "FrameShiftDrive", "Item": "$int_hyperdrive_overcharge_size6_class5_name;", "Engineering": M{
			"BlueprintName": "FSD_LongRange", "Level": 3, "Quality": .67, "ExperimentalEffect": "special_fsd_cooled"}},
		{"Slot": "MainEngines", "Item": "int_engine_size6_class5"},
		{"Slot": "SmallHardpoint1", "Item": "hpt_pulselaser_fixed_small"},
		{"Slot": "SmallHardpoint2", "Item": "hpt_pulselaser_fixed_small", "Engineering": M{"BlueprintName": "Weapon_Efficient", "Level": 5}},
		{"Slot": "Slot01_Size5", "Item": "int_cargorack_size5_class1"},
		{"Slot": "Slot02_Size5", "Item": "int_repairer_size5_class5"},
	}}
	observations := M{"game": M{"version": "4.4.2", "live": true}}
	response, err := planner.CurrentShipEngineering(loadout, M{"slot": "FrameShiftDrive"}, observations)
	if err != nil {
		t.Fatal(err)
	}
	if response["loadoutKnown"] != true || response["ship"] != "Panther Clipper Mk II" || len(v.Rows(response["modules"])) != 6 {
		t.Fatal(response)
	}
	summary := v.Obj(response["summary"])
	for key, want := range map[string]int{"complete": 1, "inProgress": 1, "available": 1, "locked": 1, "unknown": 2} {
		if got := v.Int(summary[key]); got != want {
			t.Fatalf("%s: got %d, want %d (%v)", key, got, want, summary)
		}
	}
	module := v.Rows(response["modules"])[1]
	if module["moduleClass"] != "6A" || module["experimentalKey"] != "special_fsd_cooled" || v.Int(module["accessibleGrade"]) != 5 {
		t.Fatal(module)
	}
	detail := v.Obj(response["detail"])
	if detail["blueprintKey"] != "FSD_LongRange" || detail["experimentalKey"] != "special_fsd_cooled" || v.At(detail, "engineer", "key") != "300100" {
		t.Fatal(detail)
	}
	recipe := v.Obj(detail["recipe"])
	grades := v.Rows(recipe["grades"])
	if len(grades) != 5 || grades[0]["status"] != "done" || grades[1]["status"] != "done" || grades[2]["rolls"] != nil ||
		grades[2]["status"] != "unknown" || v.Int(grades[3]["rolls"]) != 4 || v.Int(grades[4]["rolls"]) != 5 ||
		recipe["experimentalStatus"] != "done" || recipe["totalsPartial"] != true || len(v.Rows(recipe["totals"])) == 0 {
		t.Fatal(recipe)
	}
	for _, ingredient := range v.Rows(recipe["totals"]) {
		if ingredient["owned"] == nil || v.Int(ingredient["missing"]) != v.Int(ingredient["count"]) {
			t.Fatal("known zero stock should produce the full deficit", ingredient)
		}
	}
	response, err = planner.CurrentShipEngineering(loadout, M{"slot": "FrameShiftDrive", "grade": 3, "experimentalKey": ""}, observations)
	if err != nil || v.At(response, "detail", "experimentalKey") != "" || v.At(response, "detail", "recipe", "experimentalStatus") != nil {
		t.Fatal(response, err)
	}
}

func TestCurrentShipEngineeringListsDependencyStepsAndUnknownBudget(t *testing.T) {
	planner := New(nil)
	planner.Apply(M{"event": "EngineerProgress", "EngineerID": 300100, "Progress": "Unlocked", "Rank": 5}, nil)
	loadout := M{"Modules": []M{{"Slot": "MainEngines", "Item": "int_engine_size6_class5"}}}
	request := M{"slot": "MainEngines", "blueprintKey": "Engine_Dirty", "grade": 5}
	response, err := planner.CurrentShipEngineering(loadout, request, M{"game": M{"version": "4.4.2", "live": true}})
	if err != nil {
		t.Fatal(err)
	}
	detail := v.Obj(response["detail"])
	if v.At(detail, "availableEngineer", "key") != "300100" || v.Int(v.At(detail, "availableEngineer", "accessibleGrade")) != 3 {
		t.Fatal(detail)
	}
	unlock := v.Rows(detail["unlockSteps"])
	if len(unlock) < 2 || len(v.Rows(unlock[0]["steps"])) == 0 || unlock[len(unlock)-1]["key"] != v.At(detail, "engineer", "key") {
		t.Fatal("expected dependency steps before target engineer", unlock)
	}
	recipe := v.Obj(detail["recipe"])
	firstGrade := v.Rows(recipe["grades"])[0]
	materials := v.Rows(firstGrade["perRoll"])
	if recipe["confidence"] != "unknown" || recipe["totalsPartial"] != true || firstGrade["rolls"] != nil ||
		len(materials) == 0 || materials[0]["missing"] != nil {
		t.Fatal(recipe)
	}
	response, err = planner.CurrentShipEngineering(loadout, M{"slot": "MainEngines", "blueprintKey": "Engine_Dirty", "grade": 3}, M{"game": M{"version": "3.8.0", "live": false}})
	if err != nil || v.At(response, "detail", "recipe", "confidence") != "unknown" {
		t.Fatal(response, err)
	}
	if _, err := planner.CurrentShipEngineering(loadout, M{"slot": "MainEngines", "blueprintKey": "FSD_LongRange"}, nil); err == nil {
		t.Fatal("incompatible blueprint was accepted")
	}
}
