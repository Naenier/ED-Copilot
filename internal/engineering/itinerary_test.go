package engineering

import (
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestEngineeringItineraryKeepsAccessAndDeferralSeparateFromObservedCraft(t *testing.T) {
	p := New(nil)
	input := M{"action": "add", "moduleKey": "fsd", "blueprintKey": "FSD_LongRange", "rolls": []int{1}}
	if err := p.Update(input); err != nil {
		t.Fatal(err)
	}
	plan := p.Itinerary(nil, nil)
	if len(v.Rows(plan["stops"])) != 0 || len(v.Rows(plan["blocked"])) != 1 {
		t.Fatal(plan)
	}
	p.Apply(M{"event": "EngineerProgress", "EngineerID": 300100, "Progress": "Unlocked", "Rank": 5}, nil)
	for _, row := range v.Rows(v.Rows(p.Snapshot(nil)["items"])[0]["materials"]) {
		key := v.Str(row["key"])
		p.stock[key] = v.Int(row["count"])
		p.known[v.Str(materials[key]["category"])] = true
	}
	plan = p.Itinerary(nil, nil)
	next := v.Obj(plan["currentStop"])
	if next["system"] != "Deciat" || next["action"] != "upgrade" || len(v.Rows(plan["blocked"])) != 0 {
		t.Fatal(plan)
	}
	options, err := UpdateItinerary(nil, M{"action": "defer", "id": next["id"], "deferred": true})
	if err != nil || len(v.Rows(p.Itinerary(options, nil)["deferred"])) != 1 || p.Itinerary(options, nil)["currentStop"] != nil {
		t.Fatal(options, err)
	}
	p.Apply(M{"event": "Loadout", "ShipID": 10, "Modules": []M{{"Slot": "FrameShiftDrive", "Item": "int_hyperdrive_size5_class5"}}}, nil)
	p.Apply(M{"event": "EngineerCraft", "Slot": "FrameShiftDrive", "Module": "int_hyperdrive_size5_class5", "BlueprintName": "FSD_LongRange", "Level": 1, "Quality": 1}, nil)
	plan = p.Itinerary(options, nil)
	if len(v.Rows(plan["stops"])) != 0 || len(v.Rows(plan["deferred"])) != 0 || len(v.Rows(plan["blocked"])) != 0 {
		t.Fatal(plan)
	}
}

func TestItineraryTradesUseOnlyUnreservedSameCategoryStock(t *testing.T) {
	rows := []M{
		{"key": "need", "category": "raw", "family": "one", "tradeable": true, "grade": 1, "capacity": 300, "owned": 0},
		{"key": "donor", "category": "raw", "family": "one", "tradeable": true, "grade": 2, "capacity": 150, "owned": 10},
		{"key": "encoded", "category": "encoded", "family": "one", "tradeable": true, "grade": 2, "capacity": 150, "owned": 99},
	}
	offers := itineraryTrades(rows, map[string]int{"need": 6}, map[string]int{"need": 6, "donor": 9})
	if len(offers) != 1 || offers[0]["fromKey"] != "donor" || v.Int(offers[0]["requiredFrom"]) != 1 || v.Int(offers[0]["received"]) != 3 {
		t.Fatal(offers)
	}
}

func TestBuildNeedsCreditsJournalProgressAndKeepsUnknownModules(t *testing.T) {
	p := New(nil)
	p.Apply(M{"event": "EngineerProgress", "EngineerID": 300100, "Progress": "Unlocked", "Rank": 5}, nil)
	item := "int_hyperdrive_size5_class5"
	baseline := M{"Item": item, "Engineering": M{"BlueprintName": "FSD_LongRange", "Level": 2, "Quality": 1, "ExperimentalEffect": "special_fsd_cooled"}}
	target := M{"BlueprintName": "FSD_LongRange", "Level": 3, "Quality": 1, "ExperimentalEffect": "special_fsd_cooled"}
	goals := []M{{"slot": "FrameShiftDrive", "item": item, "target": target, "baseline": baseline}}
	context := M{"game": M{"version": "4.4.2", "live": true}}
	result := p.BuildNeeds(goals, context)
	got := v.Rows(result["items"])[0]
	if got["confidence"] != "exact" || len(v.Rows(got["materials"])) == 0 {
		t.Fatal(result)
	}
	goals[0]["baseline"] = M{"Engineering": target}
	result = p.BuildNeeds(goals, context)
	if v.Rows(result["items"])[0]["complete"] != true || len(v.Rows(result["materials"])) != 0 {
		t.Fatal(result)
	}
	goals[0]["item"] = "int_future_module"
	result = p.BuildNeeds(goals, context)
	if v.Rows(result["items"])[0]["reason"] != "unknown-recipe" || result["complete"] != false {
		t.Fatal(result)
	}
}
