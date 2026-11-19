package app

import (
	"encoding/json"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func testedFleetModules() []M {
	return []M{
		{"Slot": "Armour", "Item": "python_armour_grade1", "On": true, "Priority": 0},
		{"Slot": "PowerPlant", "Item": "int_powerplant_size7_class5", "On": true, "Priority": 0},
		{"Slot": "MainEngines", "Item": "int_engine_size6_class5", "On": true, "Priority": 0},
		{"Slot": "FrameShiftDrive", "Item": "int_hyperdrive_size5_class5", "On": true, "Priority": 0},
		{"Slot": "LifeSupport", "Item": "int_lifesupport_size4_class5", "On": true, "Priority": 0},
		{"Slot": "PowerDistributor", "Item": "int_powerdistributor_size7_class5", "On": true, "Priority": 0},
		{"Slot": "Radar", "Item": "int_sensors_size6_class5", "On": true, "Priority": 0},
		{"Slot": "FuelTank", "Item": "int_fueltank_size5_class3", "On": true, "Priority": 0},
		{"Slot": "CargoHatch", "Item": "modularcargobaydoor", "On": false, "Priority": 3},
	}
}

func fleetRows(result M) map[string]M {
	rows := map[string]M{}
	for _, row := range v.Rows(result["rows"]) {
		rows[v.Str(row["shipId"])] = row
	}
	return rows
}

func TestShipFitFleetReplaysStoredLoadoutsAndKeepsExactShipID(t *testing.T) {
	e, journalFile, _ := fixtureEngine(t, nil)
	owner := e.planWorkspace.key
	const exactID = "18446744073709551615"
	at := time.Date(2026, 11, 7, 12, 1, 0, 0, time.UTC)
	event := func(kind string, body M) M {
		at = at.Add(time.Second)
		return v.Merge(M{"event": kind, "timestamp": v.ISO(at)}, body)
	}
	modules := testedFleetModules()
	modules[3]["Engineering"] = M{"BlueprintName": "FSD_LongRange", "Level": 4, "Quality": .72, "ExperimentalEffect": "special_fsd_heavy"}
	writeEvents(t, journalFile, true, []M{
		event("Loadout", M{"Ship": "Python", "ShipID": json.Number(exactID), "ShipName": "Observed Runner", "Modules": modules}),
		event("ShipyardSwap", M{"ShipType": "PantherMkII", "ShipID": 1, "StoreOldShip": "Python", "StoreShipID": json.Number(exactID)}),
		event("StoredShips", M{"ShipsHere": []M{}, "ShipsRemote": []M{
			{"ShipID": json.Number(exactID), "ShipType": "Python", "Name": "Observed Runner", "StarSystem": "Synthetic A"},
			{"ShipID": 8, "ShipType": "Anaconda", "Name": "Hull Only"},
		}}),
	})
	e.Reader.Rescan()
	e.Reader.Poll()
	out := researchResult(t, e, "/api/ship-fits/fleet", M{"expectedPlanWorkspace": owner})
	rows := fleetRows(out)
	if out["complete"] != true || len(rows) != 3 || rows[exactID]["available"] != true ||
		rows[exactID]["current"] != false || rows[exactID]["source"] != "journal" ||
		rows[exactID]["system"] != "Synthetic A" || rows["8"]["available"] != false || rows["1"]["current"] != true {
		t.Fatalf("wrong fleet projection: %v", out)
	}
	fit := researchResult(t, e, "/api/ship-fits/fleet/loadout", M{"expectedPlanWorkspace": owner, "shipId": exactID})
	if v.At(fit, "loadout", "ShipID") != exactID || len(v.Rows(v.At(fit, "loadout", "Modules"))) != len(modules) {
		t.Fatalf("incomplete fleet import: %v", fit)
	}
	fsd := v.Rows(v.At(fit, "loadout", "Modules"))[3]
	if v.Num(v.At(fsd, "Engineering", "Quality")) != .72 || v.At(fsd, "Engineering", "ExperimentalEffect") != "special_fsd_heavy" {
		t.Fatalf("lost journal engineering: %v", fsd)
	}
	if _, err := e.Do("/api/ship-fits/fleet/loadout", M{"expectedPlanWorkspace": owner, "shipId": "8"}); err == nil {
		t.Fatal("metadata-only ship was exposed as an outfitted fit")
	}
	e.Apply(event("ModuleStore", M{"ShipID": json.Number(exactID), "Slot": "FrameShiftDrive"}), nil)
	if _, err := e.Do("/api/ship-fits/fleet/loadout", M{"shipId": exactID}); err == nil {
		t.Fatal("the pre-outfitting build survived a newer known mutation")
	}
	e.Apply(event("ShipyardSell", M{"SellShipID": json.Number(exactID)}), nil)
	if fleetRows(researchResult(t, e, "/api/ship-fits/fleet", nil))[exactID] != nil {
		t.Fatal("sold ship remains in the fleet")
	}
	e.Apply(event("Commander", M{"FID": "FSYNTHOTHER", "Name": "Synthetic Other"}), nil)
	if _, err := e.Do("/api/ship-fits/fleet", M{"expectedPlanWorkspace": owner}); err == nil {
		t.Fatal("stale fleet projection request was not rejected")
	}
	if out := researchResult(t, e, "/api/ship-fits/fleet", nil); out["known"] != false || len(v.Rows(out["rows"])) > 0 {
		t.Fatalf("previous commander fleet leaked: %v", out)
	}
}

func TestShipFitFleetUsesVerifiedLiveCAPIWithoutInventingEngineering(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	res := func(fid string, started, arrived time.Time) M {
		fixture := M{}
		for _, item := range testedFleetModules() {
			fixture[v.Str(item["Slot"])] = M{"module": M{"name": item["Item"], "on": item["On"], "priority": item["Priority"]}}
		}
		incomplete := v.Copy(fixture)
		v.Obj(incomplete["FrameShiftDrive"])["engineer"] = M{"recipeName": "FSD_LongRange", "recipeLevel": 5}
		return M{
			"status": M{"connected": true, "fid": fid},
			"data": M{"profile": M{"commander": M{"name": "Synthetic Commander"}, "ships": M{
				"2": M{"name": "Python", "shipName": "Complete CAPI", "modules": fixture},
				"3": M{"name": "Python", "shipName": "Incomplete CAPI", "modules": incomplete},
				"4": M{"name": "Python", "shipName": "Roster Only", "starsystem": M{"name": "Synthetic B"}},
			}}},
			"metadata": M{"source": "Frontier CAPI", "observedAt": M{"profile": v.ISO(arrived)}, "requestStartedAt": M{"profile": v.ISO(started)}},
		}
	}
	at := time.Date(2026, 11, 7, 12, 1, 0, 0, time.UTC)
	e.applyVerifiedProfileCAPI(res("FNOTOURSCAPI", at, at.Add(time.Second)))
	if fleetRows(researchResult(t, e, "/api/ship-fits/fleet", nil))["2"] != nil {
		t.Fatal("unverified account influenced the fleet")
	}
	e.applyVerifiedProfileCAPI(res("FSYNTHETIC", at, at.Add(time.Second)))
	rows := fleetRows(researchResult(t, e, "/api/ship-fits/fleet", nil))
	if len(rows) != 4 || rows["2"]["source"] != "capi" || rows["2"]["available"] != true ||
		rows["3"]["available"] != false || rows["4"]["available"] != false || rows["4"]["system"] != "Synthetic B" {
		t.Fatalf("Companion API completeness not respected: %v", rows)
	}
	fit := researchResult(t, e, "/api/ship-fits/fleet/loadout", M{"shipId": "2"})
	var found bool
	for _, item := range v.Rows(v.At(fit, "loadout", "Modules")) {
		if item["Slot"] == "CargoHatch" {
			found = item["On"] == false && v.Int(item["Priority"]) == 3
		}
	}
	if !found {
		t.Fatalf("lost enabled status or module priority: %v", fit)
	}
	e.Apply(M{"event": "ModuleStore", "timestamp": v.ISO(at.Add(3 * time.Second)), "ShipID": 2}, nil)
	e.applyVerifiedProfileCAPI(res("FSYNTHETIC", at.Add(2*time.Second), at.Add(9*time.Second)))
	if fleetRows(researchResult(t, e, "/api/ship-fits/fleet", nil))["2"]["available"] != false {
		t.Fatal("a delayed pre-change CAPI response restored the obsolete assembly")
	}
	e.Apply(testEvent("Fileheader", M{"gameversion": "3.8.2", "part": 1}), nil)
	e.Apply(testEvent("Commander", M{"FID": "FSYNTHETIC", "Name": "Synthetic Commander"}), nil)
	e.applyVerifiedProfileCAPI(res("FSYNTHETIC", at, at.Add(time.Second)))
	if fleetRows(researchResult(t, e, "/api/ship-fits/fleet", nil))["2"] != nil {
		t.Fatal("a Live CAPI update appeared in a Legacy fleet")
	}
}

func TestShipFitFleetRecognizesCraftAndRenameWithoutStaleFlightStats(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	at := time.Date(2026, 11, 7, 12, 2, 0, 0, time.UTC)
	e.Apply(M{"event": "Loadout", "timestamp": v.ISO(at), "Ship": "Python", "ShipID": 2,
		"ShipName": "First name", "Modules": testedFleetModules(), "MaxJumpRange": 30}, nil)
	build := func() M {
		t.Helper()
		result := researchResult(t, e, "/api/ship-fits/fleet/loadout", M{"shipId": "2"})
		return v.Obj(result["loadout"])
	}
	craft := M{"event": "EngineerCraft", "ShipID": 2, "timestamp": v.ISO(at.Add(time.Second)),
		"Slot": "FrameShiftDrive", "Module": "int_hyperdrive_size5_class5", "BlueprintName": "FSD_LongRange",
		"Level": 3, "Quality": .72, "IsPreview": true}
	e.Apply(craft, nil)
	if got := build(); v.Num(got["MaxJumpRange"]) != 30 || v.Rows(got["Modules"])[3]["Engineering"] != nil {
		t.Fatalf("preview altered the recorded ship: %v", got)
	}

	e.Apply(v.Merge(v.Copy(craft), M{"IsPreview": false, "timestamp": v.ISO(at.Add(2 * time.Second))}), nil)
	e.Apply(M{"event": "SetUserShipName", "ShipID": 2, "timestamp": v.ISO(at.Add(3 * time.Second)),
		"UserShipName": "Second name", "UserShipId": "RUNNER"}, nil)
	got := build()
	if got["ShipName"] != "Second name" || got["ShipIdent"] != "RUNNER" || got["MaxJumpRange"] != nil ||
		v.Num(v.At(v.Rows(got["Modules"])[3], "Engineering", "Quality")) != .72 {
		t.Fatalf("craft or rename did not reach the import projection: %v", got)
	}
}
