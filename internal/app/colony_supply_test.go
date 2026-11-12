package app

import (
	"testing"

	"github.com/Naenier/ED-Copilot/internal/profile"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestColonySupplyPortfolioUsesActiveCommanderCargo(t *testing.T) {
	engine, _, profileDir := fixtureEngine(t, nil)
	view := func() M { return engine.SnapshotForUI("colonization") }
	var projectIDs []string
	var buildingIDs []any
	for _, name := range []string{"Supply Alpha", "Supply Beta"} {
		doOK(t, engine, "/api/colonization", M{"action": "create-project", "targetSystem": name})
		id := v.Str(v.At(view(), "colonization", "activeProject", "id"))
		projectIDs = append(projectIDs, id)
		doOK(t, engine, "/api/colonization", M{"action": "add-building", "projectId": id, "typeId": "custom"})
		buildings := v.Rows(v.At(view(), "colonization", "activeProject", "buildings"))
		buildingIDs = append(buildingIDs, buildings[0]["id"])
		doOK(t, engine, "/api/colonization", M{
			"action": "set-resource-plan", "projectId": id, "buildingId": buildings[0]["id"],
			"resources": []M{{"commodity": "steel", "required": 100}}, "sourceNote": "Synthetic test",
		})
	}
	before := view()
	request := M{"projectIds": projectIDs, "expectedPlanWorkspace": v.At(before, "planWorkspace", "key")}
	result, err := engine.Do("/api/colonization/portfolio", request)
	if err != nil {
		t.Fatal(err)
	}
	supplies := v.Rows(result["supplies"])
	if len(supplies) != 1 || v.Num(supplies[0]["missing"]) != 200 || v.Num(supplies[0]["cargoCovered"]) != 25 {
		t.Fatalf("combined supply should allocate the ship's non-stolen cargo once: %v", result)
	}
	if after := view(); v.At(after, "colonization", "activeProject", "id") != projectIDs[1] {
		t.Fatal("a supply estimate changed the active colony")
	}
	workspace := request["expectedPlanWorkspace"]
	reservation, err := engine.Do("/api/colonization/supply-reservation", M{
		"action": "reserve", "projectId": projectIDs[0], "buildingId": buildingIDs[0],
		"commodity": "steel", "source": "ship", "count": 20, "expectedPlanWorkspace": workspace,
	})
	if err != nil || v.Num(reservation["remaining"]) != 20 {
		t.Fatalf("could not reserve journal-observed cargo: %v %v", reservation, err)
	}
	saved, err := profile.LoadSettings(profileDir)
	if err != nil || len(v.Rows(v.At(saved, "colonization", "supplyReservations"))) != 1 {
		t.Fatalf("successful reserve did not persist in the profile: %v", err)
	}
	second, err := engine.Do("/api/colonization/portfolio", M{"projectIds": []string{projectIDs[1]}, "expectedPlanWorkspace": workspace})
	rows := v.Rows(second["supplies"])
	if err != nil || len(rows) != 1 || v.Num(rows[0]["cargoCovered"]) != 5 || v.Num(rows[0]["reservedElsewhere"]) != 20 {
		t.Fatalf("cargo assigned to another colony remained available: %v %v", second, err)
	}
	market, err := engine.Do("/api/colonization/supply-market", M{"commodity": "steel", "quantity": 95, "expectedPlanWorkspace": workspace})
	if err != nil || market["navigate"] != "sale" || market["mode"] != "Купить" || v.At(market, "prefill", "system") != "Synthetic Origin" || v.Num(v.At(market, "prefill", "quantity")) != 95 {
		t.Fatalf("wrong procurement handoff: %v %v", market, err)
	}
	request["expectedPlanWorkspace"] = "another-commander"
	if _, err := engine.Do("/api/colonization/portfolio", request); err == nil {
		t.Fatal("a different commander's request was accepted")
	}
}
