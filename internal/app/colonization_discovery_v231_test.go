package app

import (
	"testing"

	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestJournalOwnedColonyDiscoveryPopulatesEveryActivitySelectorAndHUD(t *testing.T) {
	engine, file, data := fixtureEngine(t, nil)
	writeEvents(t, file, true, []M{
		{"event": "ColonisationSystemClaim", "timestamp": "2026-11-07T12:01:00Z", "StarSystem": "Synthetic Claimed System", "SystemAddress": "424242"},
		{"event": "FSDJump", "timestamp": "2026-11-07T12:02:00Z", "StarSystem": "Synthetic Claimed System", "SystemAddress": "424242", "StarPos": []float64{5, 0, 0}},
		{"event": "ColonisationBeaconDeployed", "timestamp": "2026-11-07T12:03:00Z"},
		{"event": "Docked", "timestamp": "2026-11-07T12:04:00Z", "StarSystem": "Synthetic Claimed System", "StationName": "Synthetic Construction Site", "StationType": "PlanetaryConstructionDepot", "MarketID": "2020"},
		{"event": "ColonisationConstructionDepot", "timestamp": "2026-11-07T12:05:00Z", "MarketID": "2020", "ConstructionProgress": 0.2, "ConstructionComplete": false, "ConstructionFailed": false, "ResourcesRequired": []M{{"Name": "$steel_name;", "RequiredAmount": 100, "ProvidedAmount": 20}}},
		{"event": "ColonisationContribution", "timestamp": "2026-11-07T12:06:00Z", "MarketID": "2020", "Contributions": []M{{"Name": "$steel_name;", "Amount": 5}}},
	})
	engine.Reader.Poll()
	doOK(t, engine, "/api/settings", M{"activity": "colonization"})
	full := v.Obj(engine.SnapshotForUI("colonization")["colonization"])
	project := v.Obj(full["activeProject"])
	id := v.Str(project["id"])
	if id == "" || len(v.Rows(full["projects"])) != 1 || full["selectedProjectId"] != id || project["originSystem"] != "Synthetic Origin" || v.Num(project["totalMissing"]) != 75 {
		t.Fatal("journal claim did not become a selected colony with authoritative remaining supplies")
	}
	for _, tab := range p.Tabs {
		selector := v.Obj(engine.SnapshotForUI(tab.ID)["colonization"])
		if selector["selectedProjectId"] != id || len(v.Rows(selector["projects"])) != 1 || v.Rows(selector["projects"])[0]["id"] != id {
			t.Fatalf("activity selector lost detected colony on %s", tab.ID)
		}
	}
	hud := v.Obj(engine.SnapshotForHUD()["colonization"])
	if hud["selectedProjectId"] != id || v.Obj(hud["activeProject"])["id"] != id || len(v.Rows(v.Obj(hud["activeProject"])["operationalSupplies"])) != 1 {
		t.Fatal("HUD does not use the same detected colony and independent supply queue")
	}
	if err := engine.Save(); err != nil {
		t.Fatal(err)
	}
	restored, err := New(Options{DataDir: data, JournalDir: engine.opts.JournalDir, DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restored.Close)
	restored.Reader.Poll()
	after := v.Obj(restored.SnapshotForHUD()["colonization"])
	if after["selectedProjectId"] != id || v.Obj(after["activeProject"])["id"] != id || len(v.Rows(after["projects"])) != 1 || len(v.Rows(v.Obj(after["activeProject"])["buildings"])) != 1 || v.Num(v.Obj(after["activeProject"])["totalMissing"]) != 75 {
		t.Fatal("restart/replay duplicated the colony or reset its selected supply queue")
	}
}
