package engineering

import (
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestCurrentShipPlanCombinesTwoSlotsAndUsesStockOnce(t *testing.T) {
	p := New(nil)
	p.Apply(M{"event": "EngineerProgress", "EngineerID": 300180, "Progress": "Unlocked", "Rank": 5}, nil)
	p.Apply(M{"event": "Materials", "Raw": []M{{"Name": "sulphur", "Count": 1}}, "Manufactured": []M{}, "Encoded": []M{}}, nil)
	loadout := M{"Modules": []M{
		{"Slot": "SmallHardpoint1", "Item": "$hpt_pulselaser_fixed_small_name;"},
		{"Slot": "SmallHardpoint2", "Item": "hpt_pulselaser_fixed_small"},
	}}
	targets := []M{
		{"slot": "SmallHardpoint1", "item": "hpt_pulselaser_fixed_small", "blueprintKey": "Weapon_Efficient", "grade": 1},
		{"slot": "SmallHardpoint2", "item": "hpt_pulselaser_fixed_small", "blueprintKey": "Weapon_Efficient", "grade": 1},
	}
	obs := M{"game": M{"version": "4.4.2", "live": true}, "system": "Wyrd", "position": workshopCoordinates("Wyrd")}
	result := p.CurrentShipPlan(loadout, targets, obs)
	sulphur := findRoadmapRow(result["materials"], "sulphur")
	stops := v.Rows(v.At(result, "itinerary", "stops"))
	if v.Int(sulphur["count"]) != 2 || v.Int(sulphur["owned"]) != 1 || v.Int(sulphur["missing"]) != 1 ||
		result["totalsPartial"] != false || len(stops) != 1 || stops[0]["id"] != "engineer:300180" ||
		len(v.Rows(stops[0]["tasks"])) != 2 || stops[0]["status"] != "ready" || v.Num(stops[0]["distance"]) != 0 {
		t.Fatal(result)
	}
	loadout["Modules"] = []M{
		{"Slot": "SmallHardpoint1", "Item": "hpt_pulselaser_fixed_small", "Engineering": M{"BlueprintName": "Weapon_Efficient", "Level": 1, "Quality": 1}},
		{"Slot": "SmallHardpoint2", "Item": "hpt_beamlaser_fixed_small"},
	}
	result = p.CurrentShipPlan(loadout, targets, obs)
	goals := v.Rows(result["goals"])
	if goals[0]["status"] != "complete" || goals[1]["reason"] != "module-changed" ||
		len(v.Rows(result["materials"])) != 0 || len(v.Rows(v.At(result, "itinerary", "stops"))) != 0 ||
		len(v.Rows(v.At(result, "itinerary", "blocked"))) != 1 {
		t.Fatal(result)
	}
}

func TestCurrentShipPlanShowsMixedQuantitiesWithoutInventingDeficits(t *testing.T) {
	p := New(nil)
	p.Apply(M{"event": "EngineerProgress", "EngineerID": 300100, "Progress": "Unlocked", "Rank": 5}, nil)
	p.Apply(M{"event": "Materials", "Raw": []M{}, "Manufactured": []M{{"Name": "chemicalprocessors", "Count": 1}}}, nil)
	loadout := M{"Modules": []M{
		{"Slot": "FrameShiftDrive", "Item": "int_hyperdrive_size5_class5", "Engineering": M{"BlueprintName": "FSD_LongRange", "Level": 3, "Quality": .71}},
		{"Slot": "MainEngines", "Item": "int_engine_size5_class5"},
	}}
	targets := []M{
		{"slot": "FrameShiftDrive", "item": "int_hyperdrive_size5_class5", "blueprintKey": "FSD_LongRange", "grade": 4},
		{"slot": "MainEngines", "item": "int_engine_size5_class5", "blueprintKey": "Engine_Dirty", "grade": 3},
	}
	obs := M{"game": M{"version": "4.4.2", "live": true}, "system": "Sol", "position": []float64{0, 0, 0}}
	result := p.CurrentShipPlan(loadout, targets, obs)
	unknown := findRoadmapRow(result["materials"], "chemicalprocessors")
	known := findRoadmapRow(result["materials"], "chemicaldistillery")
	stockUnknown := findRoadmapRow(result["materials"], "legacyfirmware")
	stops := v.Rows(v.At(result, "itinerary", "stops"))
	if result["totalsPartial"] != true || v.Int(v.At(result, "summary", "partial")) != 1 ||
		unknown["partial"] != true || unknown["count"] != nil || unknown["missing"] != nil || v.Int(unknown["owned"]) != 1 ||
		v.Int(known["count"]) != 4 || v.Int(known["missing"]) != 4 ||
		v.Int(stockUnknown["count"]) != 6 || stockUnknown["missing"] != nil ||
		len(stops) != 1 || stops[0]["id"] != "engineer:300100" || v.Num(stops[0]["distance"]) < 100 {
		t.Fatal(result)
	}
}

func TestCurrentShipPlanVisitsPrerequisitesFirstAndCombinesHandIns(t *testing.T) {
	p := New(nil)
	p.Apply(M{"event": "EngineerContribution", "EngineerID": 300000, "Type": "Commodity", "Commodity": "lavianbrandy", "TotalQuantity": 10}, nil)
	p.Apply(M{"event": "Cargo", "Vessel": "Ship", "Inventory": []M{{"Name": "lavianbrandy", "Count": 20}}}, nil)
	loadout := M{"Modules": []M{
		{"Slot": "TinyHardpoint1", "Item": "hpt_shieldbooster_size0_class5"},
		{"Slot": "TinyHardpoint2", "Item": "hpt_shieldbooster_size0_class5"},
	}}
	targets := []M{
		{"slot": "TinyHardpoint1", "item": "hpt_shieldbooster_size0_class5", "blueprintKey": "ShieldBooster_HeavyDuty", "grade": 5},
		{"slot": "TinyHardpoint2", "item": "hpt_shieldbooster_size0_class5", "blueprintKey": "ShieldBooster_Resistive", "grade": 5},
	}
	obs := M{"game": M{"version": "4.4.2", "live": true}, "system": "Leesti", "position": workshopCoordinates("Leesti")}
	result := p.CurrentShipPlan(loadout, targets, obs)
	stops := v.Rows(v.At(result, "itinerary", "stops"))
	if len(stops) != 3 || stops[0]["id"] != "engineer:300260" || stops[1]["id"] != "engineer:300210" ||
		stops[2]["id"] != "engineer:300000" || len(v.Rows(stops[2]["tasks"])) != 2 ||
		len(v.Rows(stops[0]["tasks"])) != 0 || len(v.Rows(stops[0]["steps"])) == 0 {
		t.Fatal(stops)
	}
	brandy := findRoadmapRow(result["unlockSupplies"], "lavianbrandy")
	if v.Int(brandy["count"]) != 40 || v.Int(brandy["owned"]) != 20 || v.Int(brandy["missing"]) != 20 {
		t.Fatal(result["unlockSupplies"])
	}
}

func TestCurrentShipPlanEstimatesAnObservedEightyPercentDSS(t *testing.T) {
	p := New(nil)
	p.Apply(M{"event": "EngineerProgress", "EngineerID": 300250, "Progress": "Unlocked", "Rank": 5}, nil)
	p.Apply(M{"event": "Materials", "Raw": []M{}, "Manufactured": []M{}, "Encoded": []M{}}, nil)
	item := "int_detailedsurfacescanner_tiny"
	loadout := M{"Modules": []M{{"Slot": "Slot01_Size1", "Item": item, "Engineering": M{
		"BlueprintName": "Sensor_Expanded", "Level": 5, "Quality": .8,
	}}}}
	target := M{"slot": "Slot01_Size1", "item": item, "blueprintKey": "Sensor_Expanded", "grade": 5}
	obs := M{"game": M{"version": "4.4.2", "live": true}}
	response, err := p.CurrentShipEngineering(loadout, target, obs)
	if err != nil {
		t.Fatal(err)
	}
	recipe := v.Obj(v.At(response, "detail", "recipe"))
	grades := v.Rows(recipe["grades"])
	if recipe["confidence"] != "estimate" || recipe["totalsPartial"] != false || recipe["totalsEstimated"] != true ||
		len(grades) != 5 || grades[4]["status"] != "estimated" || v.Int(grades[4]["rolls"]) != 1 {
		t.Fatal(recipe)
	}
	result := p.CurrentShipPlan(loadout, []M{target}, obs)
	if result["totalsEstimated"] != true || result["totalsPartial"] != false || v.Int(v.At(result, "summary", "estimated")) != 1 {
		t.Fatal(result)
	}
	materials := v.Rows(result["materials"])
	if len(materials) == 0 {
		t.Fatal("empty estimated materials")
	}
	for _, ingredient := range materials {
		if ingredient["estimated"] != true || v.Int(ingredient["count"]) != 1 || v.Int(ingredient["missing"]) != 1 {
			t.Fatal(ingredient)
		}
	}
}
