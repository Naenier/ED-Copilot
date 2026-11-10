package engineering

import (
	"encoding/json"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func roadmapForTest(t *testing.T, planner *Planner, query, observations M) M {
	t.Helper()
	result, err := planner.RoadmapWithContext(query, observations)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func findRoadmapRow(rows any, key string) M {
	for _, row := range v.Rows(rows) {
		if row["key"] == key {
			return row
		}
	}
	return nil
}

func TestRoadmapOrdersDependenciesAndReportsActualUnlockEvidence(t *testing.T) {
	p := New(nil)
	all := roadmapForTest(t, p, nil, nil)
	if len(v.Rows(all["engineers"])) != 38 || v.At(all, "summary", "unlocked") != 0 {
		t.Fatal("fresh roadmap must include all engineers without invented access", all["summary"])
	}
	b, err := json.Marshal(all)
	if err != nil || len(b) > 850000 {
		t.Fatalf("unfiltered roadmap should remain bounded, got %d bytes, error %v", len(b), err)
	}
	q := M{"engineerIDs": []string{"300220"}}
	rows := v.Rows(roadmapForTest(t, p, q, nil)["engineers"])
	if len(rows) != 3 || rows[0]["key"] != "300160" || rows[1]["key"] != "300200" || rows[2]["key"] != "300220" ||
		rows[0]["dependency"] != true || rows[2]["dependency"] != false {
		t.Fatal("Palin unlock path must include Martuuk, Qwent, then Palin")
	}
	for _, event := range []M{
		{"event": "EngineerProgress", "EngineerID": 300200, "Progress": "Unlocked", "Rank": 3, "RankProgress": 57},
		{"event": "Statistics", "Exploration": M{"Greatest_Distance_From_Start": 5800}},
		{"event": "Materials", "Manufactured": []M{{"Name": "unknownenergysource", "Count": 12}}},
		{"event": "EngineerContribution", "EngineerID": 300220, "Type": "Materials", "Material": "unknownenergysource", "Quantity": 1, "TotalQuantity": 5},
	} {
		p.Apply(event, nil)
	}
	palin := findRoadmapRow(roadmapForTest(t, p, q, nil)["engineers"], "300220")
	referral := findRoadmapRow(palin["steps"], "referral-300200")
	travel := findRoadmapRow(palin["steps"], "travel")
	donation := findRoadmapRow(palin["steps"], "donate")
	item := findRoadmapRow(donation["items"], "unknownenergysource")
	if referral["status"] != "available" || v.Num(referral["currentProgress"]) != 57 || travel["status"] != "available" ||
		v.Int(travel["current"]) != 5800 || v.Int(item["donated"]) != 5 || v.Int(item["missing"]) != 9 {
		t.Fatal(referral, travel, donation)
	}
	if e := p.Update(M{"action": "engineer", "key": "300220", "unlocked": true}); e != nil {
		t.Fatal(e)
	}
	palin = findRoadmapRow(roadmapForTest(t, p, q, nil)["engineers"], "300220")
	if palin["manualUsed"] != true || palin["rank"] != nil {
		t.Fatal("manual fallback must not invent rank", palin)
	}
	p.Apply(M{"event": "EngineerProgress", "EngineerID": 300220, "Progress": "Invited"}, nil)
	palin = findRoadmapRow(roadmapForTest(t, p, q, nil)["engineers"], "300220")
	if palin["status"] != "invited" || palin["manualUsed"] != false {
		t.Fatal("manual flag must not override observed progress", palin)
	}
}

func TestShipRecipeUsesSelectedEquipmentAndKeepsEstimateHonest(t *testing.T) {
	p := New(nil)
	p.Apply(M{"event": "EngineerProgress", "EngineerID": 300100, "Progress": "Unlocked", "Rank": 5}, nil)
	p.Apply(M{"event": "Loadout", "ShipID": 1, "Modules": []M{
		{"Slot": "FrameShiftDrive", "Item": "$int_hyperdrive_size5_class5_name;", "Engineering": M{
			"BlueprintName": "FSD_LongRange", "Level": 2, "Quality": 1, "ExperimentalEffect": "special_fsd_cooled",
		}},
	}}, nil)
	query := M{"engineerIDs": []string{"300100"}, "moduleKey": "fsd", "blueprintKey": "FSD_LongRange", "grade": 3, "experimentalKey": "special_fsd_cooled", "equipmentKey": "ship:1:FrameShiftDrive"}
	live := M{"game": M{"version": "4.4.2", "live": true}}
	got := v.Obj(roadmapForTest(t, p, query, live)["recipe"])
	grades := v.Rows(got["grades"])
	if len(grades) != 3 || grades[0]["status"] != "done" || grades[1]["status"] != "done" ||
		v.Int(grades[2]["rolls"]) != 3 || got["confidence"] != "exact" || got["experimentalStatus"] != "done" {
		t.Fatal("known previous grades and effect must not be purchased again", got)
	}
	if total := findRoadmapRow(got["totals"], "chromium"); total != nil && total["owned"] != nil {
		t.Fatal("inventory must not assume zero before material snapshot", total)
	}
	legacy := M{"game": M{"version": "3.8.0", "live": false}}
	older := v.Obj(roadmapForTest(t, p, query, legacy)["recipe"])
	if older["confidence"] != "unknown" || older["totals"] != nil || v.Rows(older["grades"])[2]["rolls"] != nil {
		t.Fatal("legacy must not assume fixed Live rolls", older)
	}
	p.Apply(M{"event": "EngineerCraft", "Slot": "FrameShiftDrive", "Module": "int_hyperdrive_size5_class5", "BlueprintName": "FSD_LongRange", "Level": 3, "Quality": .8}, nil)
	got = v.Obj(roadmapForTest(t, p, query, live)["recipe"])
	if got["confidence"] != "estimate" || v.Rows(got["grades"])[2]["status"] != "unknown" {
		t.Fatal("partial grade must not imply known remaining attempts", got)
	}
	p.Apply(M{"event": "ShipyardSwap", "ShipID": 2}, nil)
	p.Apply(M{"event": "Loadout", "ShipID": 2, "Modules": []M{{"Slot": "FrameShiftDrive", "Item": "int_hyperdrive_size5_class5"}}}, nil)
	got = v.Obj(roadmapForTest(t, p, query, live)["recipe"])
	if got["confidence"] != "estimate" || v.Obj(got["current"])["grade"] != nil {
		t.Fatal("old ship module cannot be shown as current on another ship", got)
	}
}

func TestOdysseyRoadmapNeedsKnownSlotAndKeepsReferralDependencies(t *testing.T) {
	p := New(nil)
	rows := v.Rows(roadmapForTest(t, p, M{"engineerIDs": []string{"odyssey-yi-shen"}}, nil)["engineers"])
	if len(rows) != 4 || rows[3]["key"] != "odyssey-yi-shen" {
		t.Fatal("Yi Shen must follow all three Colonia referrals")
	}
	id := json.Number("18446744073709551615")
	p.Apply(M{"event": "SuitLoadout", "SuitID": id, "SuitName": "utilitysuit_class1", "SuitMods": []string{}}, nil)
	p.Apply(M{"event": "EngineerProgress", "EngineerID": roadmapData.engineers["odyssey-jude-navarro"]["journalId"], "Progress": "Unlocked"}, nil)
	obs := M{"shipLocker": M{"components": M{"known": true, "entries": []M{{"symbol": "microthrusters", "count": 5}}}}}
	q := M{"engineerIDs": []string{"odyssey-jude-navarro"}, "moduleKey": "suit", "blueprintKey": "suit_increasedmeleedamage", "equipmentKey": "suit:" + string(id)}
	result := v.Obj(roadmapForTest(t, p, q, obs)["recipe"])
	if result["needsSlot"] != true || v.Int(v.At(result, "equipmentUpgrade", "grade")) != 2 ||
		v.At(result, "engineer", "status") != "unlocked" {
		t.Fatal("G1 suit needs separate G2 upgrade and observed access", result)
	}
	item := findRoadmapRow(result["totals"], "microthrusters")
	if v.Int(item["count"]) != 8 || v.Int(item["owned"]) != 5 || v.Int(item["missing"]) != 3 {
		t.Fatal("Odyssey stock must use the known locker group", item)
	}
	p.Apply(M{"event": "SwitchSuitLoadout", "SuitID": id, "SuitName": "utilitysuit_class2", "SuitMods": []string{"suit_increasedmeleedamage"}}, nil)
	result = v.Obj(roadmapForTest(t, p, q, obs)["recipe"])
	if v.Rows(result["grades"])[0]["status"] != "done" || len(v.Rows(result["totals"])) != 0 {
		t.Fatal("already installed Odyssey mod must not be added to material total", result)
	}
}

func TestStoredModuleObservationIsRemovedOnRetrieval(t *testing.T) {
	p := New(nil)
	p.Apply(M{"event": "StoredModules", "Items": []M{
		{"Name": "$int_hyperdrive_size5_class5_name;", "StorageSlot": 7, "EngineerModifications": "FSD_LongRange", "Level": 5, "Quality": 1},
	}}, nil)
	q := M{"moduleKey": "fsd", "blueprintKey": "FSD_LongRange", "equipmentKey": "stored:7"}
	got := v.Obj(roadmapForTest(t, p, q, nil)["recipe"])
	if v.At(got, "current", "stored") != true || v.Rows(got["grades"])[4]["status"] != "done" {
		t.Fatal("stored module engineering lost", got)
	}
	p.Apply(M{"event": "ModuleRetrieve", "StorageSlot": 7}, nil)
	got = v.Obj(roadmapForTest(t, p, q, nil)["recipe"])
	if !contains(got["conditions"], "equipment-unavailable") {
		t.Fatal("retired storage slot still counted", got)
	}
	if _, err := p.Roadmap(M{"engineerIDs": []string{"unknown"}}); err == nil {
		t.Fatal("invalid filter accepted")
	}
}

func TestOperationsCurrencyAndJournalBlueprintAlias(t *testing.T) {
	p := New(nil)
	p.Apply(M{"event": "Loadout", "Modules": []M{{"Slot": "SmallHardpoint1", "Item": "Hpt_MultiCannon_Fixed_Small",
		"Engineering": M{"BlueprintName": "Weapon_Overcharged", "Level": 1, "Quality": 1}}}}, nil)
	query := M{"moduleKey": "mc", "blueprintKey": "MC_Overcharged", "equipmentKey": "ship:SmallHardpoint1", "grade": 1}
	recipe := v.Obj(roadmapForTest(t, p, query, nil)["recipe"])
	if v.At(recipe, "current", "blueprintKey") != "MC_Overcharged" || v.Rows(recipe["grades"])[0]["status"] != "done" {
		t.Fatal("must resolve overloaded blueprint in the module's own catalogue", recipe)
	}
	operations := M{"moduleKey": "fs", "blueprintKey": "FuelScoop_Efficiency", "grade": 1}
	recipe = v.Obj(roadmapForTest(t, p, operations, nil)["recipe"])
	currency := findRoadmapRow(recipe["currencies"], "mercenarycoin")
	if v.Int(currency["count"]) != 10 || currency["owned"] != nil || findRoadmapRow(recipe["totals"], "mercenarycoin") != nil {
		t.Fatal("Mercenary Coins must remain separate and carry unknown stock", recipe)
	}
	recipe = v.Obj(roadmapForTest(t, p, operations, M{"game": M{"version": "3.8.0", "live": false}})["recipe"])
	if recipe["compatible"] != false || !contains(recipe["conditions"], "live-only") {
		t.Fatal("Operations blueprint cannot be applied in Legacy", recipe)
	}
}
