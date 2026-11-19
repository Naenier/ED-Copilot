package app

import (
	"encoding/json"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/shipfits"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestShipFitsLibrarySurvivesRestartAndBulkImportIsAtomic(t *testing.T) {
	opts := Options{DataDir: t.TempDir(), JournalDir: t.TempDir(), DisableNetwork: true, ManualPolling: true}
	open := func() *Engine {
		t.Helper()
		e, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(e.Close)
		return e
	}
	e := open()
	draft := M{"name": "  Разведчик  ", "ship": "python", "build": M{"version": 1, "shipId": "python", "code": "example-code", "cargo": 16}}
	view := researchResult(t, e, "/api/ship-fits", M{})
	if len(view["fits"].([]shipfits.Record)) != 0 || len(v.Obj(view["observed"])) != 0 {
		t.Fatal("new profile did not start with an empty library")
	}

	fit := researchResult(t, e, "/api/ship-fits/save", draft)["fit"].(shipfits.Record)
	if fit.ID == "" || fit.Name != "Разведчик" || fit.UpdatedAt.IsZero() {
		t.Fatalf("save did not return the normalized build: %#v", fit)
	}
	draft["id"], draft["name"] = fit.ID, "Экспедиция"
	researchResult(t, e, "/api/ship-fits/save", draft)
	invalid := M{"name": "Чужой корпус", "ship": "anaconda", "build": draft["build"]}
	if _, err := e.Do("/api/ship-fits/import", M{"fits": []M{draft, invalid}}); err == nil {
		t.Fatal("mismatched ship identifier accepted on import")
	}
	fits := researchResult(t, e, "/api/ship-fits/export", nil)["fits"].([]shipfits.Record)
	if len(fits) != 1 || fits[0].ID != fit.ID {
		t.Fatal("failed import changed the library")
	}

	var imported []any
	raw, err := json.Marshal(fits)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &imported); err != nil {
		t.Fatal(err)
	}
	result := researchResult(t, e, "/api/ship-fits/import", M{"fits": imported})["fits"].([]shipfits.Record)
	if len(result) != 1 || result[0].ID == fit.ID || result[0].Build != nil {
		t.Fatal("import overwrote the saved build or returned the full payload")
	}

	reopened := open()
	fit = researchResult(t, reopened, "/api/ship-fits/get", M{"id": fit.ID})["fit"].(shipfits.Record)
	var body M
	if err := json.Unmarshal(fit.Build, &body); err != nil || fit.Name != "Экспедиция" || body["name"] != fit.Name || v.Num(body["cargo"]) != 16 {
		t.Fatalf("saved build did not round-trip: %#v, %v", body, err)
	}
	researchResult(t, reopened, "/api/ship-fits/delete", M{"id": fit.ID})
	fits = researchResult(t, reopened, "/api/ship-fits", nil)["fits"].([]shipfits.Record)
	if len(fits) != 1 || fits[0].ID != result[0].ID || fits[0].Build != nil {
		t.Fatal("delete affected the imported build or list included full payloads")
	}
}

func TestShipFitsCurrentLoadoutTracksCommander(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	owner := v.Str(v.At(e.SnapshotForUI("section-seven"), "planWorkspace", "key"))
	e.Apply(testEvent("Loadout", M{
		"Ship": "Python", "ShipID": 1,
		"Modules": []M{{"Slot": "PowerPlant", "Item": "int_powerplant_size7_class5", "On": true}},
	}), nil)
	view := researchResult(t, e, "/api/ship-fits", M{"expectedPlanWorkspace": owner})
	if v.At(view, "observed", "Ship") != "Python" {
		t.Fatal("current ship loadout was not available")
	}
	e.Apply(testEvent("Commander", M{"FID": "FTESTFITOTHER", "Name": "Other Pilot"}), nil)
	if _, err := e.Do("/api/ship-fits", M{"expectedPlanWorkspace": owner}); err == nil {
		t.Fatal("stale loadout request accepted")
	}
	if view = researchResult(t, e, "/api/ship-fits", M{}); len(v.Obj(view["observed"])) != 0 {
		t.Fatal("previous commander's ship was available for import")
	}
}

func TestShipFitMaterialsCombinesRecipesBeforeComparingInventory(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	owner := v.Str(v.At(e.SnapshotForUI("section-seven"), "planWorkspace", "key"))
	e.Apply(testEvent("Materials", M{
		"Raw":          []M{{"Name": "nickel", "Count": 4}},
		"Manufactured": []M{{"Name": "conductivecomponents", "Count": 10}},
	}), nil)
	request := M{"expectedPlanWorkspace": owner, "modules": []M{
		{"slot": "SmallHardpoint1", "blueprint": "Weapon_Overcharged", "grade": 2},
		{"slot": "SmallHardpoint2", "blueprint": "Weapon_Overcharged", "grade": 2},
		{"slot": "FrameShiftDrive", "blueprint": "FSD_LongRange", "grade": 3, "quality": .5, "experimental": "special_fsd_heavy"},
	}}
	read := func() (M, map[string]M) {
		t.Helper()
		response := researchResult(t, e, "/api/ship-fits/materials", request)
		groups := v.Rows(response["groups"])
		if len(groups) != 3 || groups[0]["category"] != "raw" || groups[1]["category"] != "manufactured" || groups[2]["category"] != "encoded" {
			t.Fatalf("wrong material groups: %v", groups)
		}
		rows := map[string]M{}
		for _, group := range groups {
			for _, row := range v.Rows(group["rows"]) {
				key := v.Str(row["key"])
				if rows[key] != nil {
					t.Fatalf("material %s appears more than once", key)
				}
				rows[key] = row
			}
		}
		return response, rows
	}
	response, rows := read()
	if len(v.Rows(response["unavailable"])) != 0 || response["estimated"] != true {
		t.Fatalf("normal catalogue ingredients missing: %v", response)
	}
	for key, want := range map[string][3]any{
		"nickel": {6, 4, 2}, "phosphorus": {2, 0, 2},
		"conductivecomponents": {4, 10, 0}, "galvanisingalloys": {3, 0, 3},
		"disruptedwakeechoes": {8, nil, nil}, "wakesolutions": {2, nil, nil},
	} {
		row := rows[key]
		if [3]any{row["required"], row["owned"], row["missing"]} != want {
			t.Errorf("%s cost/inventory differs from %v: %v", key, want, row)
		}
	}
	groups := v.Rows(response["groups"])
	if groups[0]["required"] != 8 || groups[0]["missing"] != 4 || groups[2]["unknown"] != 3 {
		t.Fatalf("wrong group totals: %v", groups)
	}

	e.domainMu.Lock()
	e.planReplay = true
	e.domainMu.Unlock()
	_, rows = read()
	e.domainMu.Lock()
	e.planReplay = false
	e.domainMu.Unlock()
	if rows["nickel"]["required"] != 6 || rows["nickel"]["owned"] != nil || rows["nickel"]["missing"] != nil {
		t.Fatalf("an in-progress replay published a partial inventory: %v", rows["nickel"])
	}

	e.Apply(testEvent("MaterialCollected", M{"Name": "nickel", "Count": 3}), nil)
	_, rows = read()
	if rows["nickel"]["owned"] != 7 || rows["nickel"]["missing"] != 0 {
		t.Fatalf("material collection did not revise the fitter inventory: %v", rows["nickel"])
	}
	request["expectedPlanWorkspace"] = "different-owner"
	if _, err := e.Do("/api/ship-fits/materials", request); err == nil {
		t.Fatal("stale fitter request accepted another commander's inventory")
	}
}

func TestShipFitMaterialsChargesOnlyAddedFactoryExperimental(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	response := researchResult(t, e, "/api/ship-fits/materials", M{"modules": []M{
		{"slot": "Slot1", "blueprint": "CargoRack_IncreasedCapacity", "grade": 5, "engineeringLocked": true},
		{"slot": "MediumHardpoint1", "blueprint": "Weapon_HighCapacity", "grade": 5, "engineeringLocked": true,
			"factoryExperimental": "special_feedback_cascade", "experimental": "special_feedback_cascade"},
		{"slot": "FrameShiftDrive", "blueprint": "FSD_LongRange", "grade": 5, "engineeringLocked": true, "experimental": "special_fsd_heavy"},
		{"slot": "MediumHardpoint2", "experimental": "no-such-effect"},
	}})
	groups := v.Rows(response["groups"])
	if groups[0]["required"] != 0 || groups[1]["required"] != 3 || groups[2]["required"] != 6 || response["estimated"] != false {
		t.Fatalf("factory upgrades were charged to the fit or the added experimental was omitted: %v", response)
	}
	unavailable := v.Rows(response["unavailable"])
	if len(unavailable) != 1 || unavailable[0]["slot"] != "MediumHardpoint2" || unavailable[0]["reason"] != "experimental" {
		t.Fatalf("unsupported experimental was not reported: %v", unavailable)
	}
}

func TestShipFitMaterialsUsesTheDisplayedCraftPercentage(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	for _, example := range []struct {
		quality float64
		rolls   int
	}{{.6667, 2}, {.67, 2}, {.68, 3}} {
		response := researchResult(t, e, "/api/ship-fits/materials", M{"modules": []M{
			{"slot": "FrameShiftDrive", "blueprint": "FSD_LongRange", "grade": 3, "quality": example.quality},
		}})
		count := -1
		for _, group := range v.Rows(response["groups"]) {
			for _, row := range v.Rows(group["rows"]) {
				if row["key"] == "wakesolutions" {
					count = v.Int(row["required"])
				}
			}
		}
		if count != example.rolls {
			t.Errorf("quality %g: got %d Wake Solutions, want %d", example.quality, count, example.rolls)
		}
	}
}
