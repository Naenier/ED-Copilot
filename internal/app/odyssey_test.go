package app

import (
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestOdysseyStockKeepsContainersSeparateAndRecentPickupsScoped(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	workspace := v.Str(v.At(e.SnapshotForUI("engineering-odyssey"), "planWorkspace", "key"))
	if workspace == "" {
		t.Fatal("fixture has no commander workspace")
	}
	request := M{"expectedPlanWorkspace": workspace}
	base := time.Now().UTC().Truncate(time.Second)
	event := func(kind string, offset int, details M) {
		e.Apply(v.Merge(M{"event": kind, "timestamp": v.ISO(base.Add(time.Duration(offset) * time.Second))}, details), M{"file": "synthetic"})
	}
	event("Disembark", 0, nil)
	event("BackpackMaterials", 1, M{"Components": []M{{"Name": "graphene", "Count": 2}}})
	event("ShipLocker", 1, M{"Components": []M{{"Name": "graphene", "Count": 7}}})
	context, err := e.odysseyObservations(request)
	if err != nil || v.Int(v.At(context, "backpack", "components", "count")) != 2 || v.Int(v.At(context, "shipLocker", "components", "count")) != 7 {
		t.Fatalf("separate inventory observations were lost: %v %v", context, err)
	}
	event("BackpackChange", 2, M{"Added": []M{{"Name": "$graphene_name;", "Type": "Component", "Count": 1}, {"Name": "graphene", "Type": "Component", "Count": 2}, {"Name": "geneticrepairmeds", "Type": "Item", "Count": 1, "MissionID": "183"}}})
	context, err = e.odysseyObservations(request)
	if err != nil || v.Bool(v.At(context, "backpack", "components", "known")) || !v.Bool(v.At(context, "shipLocker", "components", "known")) {
		t.Fatalf("the old backpack snapshot was reused after a change: %v %v", context, err)
	}
	if len(e.odysseyPickups) != 1 || e.odysseyPickups[0]["key"] != "graphene" || v.Int(e.odysseyPickups[0]["count"]) != 3 {
		t.Fatalf("recent resource pickup was lost or mission cargo was included: %v", e.odysseyPickups)
	}
	event("Backpack", 2, M{"Components": []M{{"Name": "graphene", "Count": 5}}})
	context, err = e.odysseyObservations(request)
	if err != nil || !v.Bool(v.At(context, "backpack", "components", "known")) || v.Int(v.At(context, "backpack", "components", "count")) != 5 {
		t.Fatalf("replacement snapshot did not restore the exact stock: %v %v", context, err)
	}
	event("SuitLoadout", 3, M{"SuitID": "18446744073709551614", "SuitName": "utilitysuit_class1", "SuitMods": []string{}})
	result, err := e.Do("/api/engineering/odyssey/loadout", v.Merge(v.Copy(request), M{
		"action": "save", "name": "Settlement visit", "reserve": true,
		"items": []M{{"itemKey": "utilitysuit", "equipmentKey": "suit:18446744073709551614", "targetGrade": 2}},
	}))
	if err != nil || v.At(result, "activeLoadout", "name") != "Settlement visit" || len(v.Rows(result["materials"])) == 0 {
		t.Fatalf("saved Odyssey loadout did not use observed equipment: %v %v", result["activeLoadout"], err)
	}
	doOK(t, e, "/api/settings", M{"activity": "odyssey"})
	if hud := e.SnapshotForHUD(); v.At(hud, "odyssey", "activeLoadout", "name") != "Settlement visit" || v.At(hud, "odyssey", "equipment") != nil {
		t.Fatalf("Odyssey HUD did not use a scoped loadout projection: %v", hud["odyssey"])
	}
	event("UpgradeSuit", 4, M{"Name": "utilitysuit_class1", "SuitID": "18446744073709551614", "Class": 2})
	result, err = e.OdysseyLoadouts(request)
	items := v.Rows(v.At(result, "activeLoadout", "items"))
	if err != nil || len(items) != 1 || v.Int(items[0]["currentGrade"]) != 2 || len(v.Rows(result["materials"])) != 0 {
		t.Fatalf("journal upgrade did not clear the planned grade cost: %v %v", items, err)
	}
	event("Embark", 5, nil)
	if len(e.odysseyPickups) != 0 {
		t.Fatal("a foot pickup survived boarding")
	}
	if hud := e.SnapshotForHUD(); hud["odyssey"] != nil {
		t.Fatal("Odyssey overlay remained active after boarding")
	}
	if _, err := e.OdysseyLoadouts(M{"expectedPlanWorkspace": "previous-owner"}); err == nil {
		t.Fatal("stale commander request reached Odyssey data")
	}
}
