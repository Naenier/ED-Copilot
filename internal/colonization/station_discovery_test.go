package colonization

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/value"
)

func observeStation(p *Planner, name, typ, market, address, at string, services []string) {
	discoveryEvent(p, "Docked", at, M{"StationName": name, "StationType": typ, "MarketID": market, "SystemAddress": address, "StarSystem": "Synthetic Station Colony", "StationServices": services}, nil)
}

func TestOwnPermanentStationIsDiscoveredWithoutDepotOrARXAssumptions(t *testing.T) {
	p := New(nil)
	discoveryClaim(p, "Synthetic Station Colony", "12345", "2026-11-01T12:00:00Z")
	observeStation(p, "$EXT_PANEL_ColonisationShip; SYN-001", "SurfaceStation", "201", "12345", "2026-11-01T12:01:00Z", []string{"colonisationcontribution"})
	discoveryDepot(p, "Synthetic Station Colony", "12345", "201", "2026-11-01T12:02:00Z", true)
	discoveryEvent(p, "CompleteConstruction", "2026-11-01T12:03:00Z", nil, M{"system": "Synthetic Station Colony", "systemAddress": "12345"})
	if len(rows(active(p)["buildings"])) != 0 || len(rows(p.Snapshot(nil)["depots"])) != 0 || len(p.depots) != 1 || PrimaryPortComplete(active(p)) {
		t.Fatal("supply ship or unlocated CompleteConstruction became a built facility")
	}
	if claimSourceEligible(p.stations["201"]) {
		t.Fatal("supply vessel remained an eligible origin for a new colony claim")
	}
	observeStation(p, "Synthetic Permanent Port", "SurfaceStation", "202", "12345", "2026-11-01T12:04:00Z", []string{"shipyard", "outfitting", "registeringcolonisation"})
	observeStation(p, "Planetary Construction Site: Synthetic Future", "PlanetaryConstructionDepot", "203", "12345", "2026-11-01T12:05:00Z", []string{"colonisationcontribution"})
	discoveryDepot(p, "Synthetic Station Colony", "12345", "203", "2026-11-01T12:06:00Z", false)
	project := active(p)
	buildings := rows(project["buildings"])
	if len(buildings) != 2 || buildings[0]["marketId"] != "202" || buildings[0]["status"] != "complete" || buildings[0]["journalSource"] != "station" || buildings[0]["journalStationType"] != "SurfaceStation" || buildings[0]["typeId"] != "custom" || buildings[0]["tier"] != nil || len(rows(buildings[0]["requirements"])) != 0 {
		t.Fatal("observed actual station lost, or a type/ARX bill/points was invented")
	}
	if project["status"] != "building" || num(project["totalMissing"]) != 80 || len(rows(project["operationalSupplies"])) != 1 || len(rows(p.Snapshot(nil)["depots"])) != 1 || !PrimaryPortComplete(project) {
		t.Fatalf("station/supply state: status=%v missing=%v operations=%d depots=%d primary=%v", project["status"], project["totalMissing"], len(rows(project["operationalSupplies"])), len(rows(p.Snapshot(nil)["depots"])), PrimaryPortComplete(project))
	}
	ids := []string{str(buildings[0]["id"]), str(buildings[1]["id"])}
	restored := New(p.ExportSettings())
	observeStation(restored, "Synthetic Permanent Port", "SurfaceStation", "202", "12345", "2026-11-01T12:04:00Z", []string{"shipyard"})
	discoveryClaim(restored, "Synthetic Station Colony", "12345", "2026-11-01T12:00:00Z")
	after := rows(active(restored)["buildings"])
	if len(after) != 2 || after[0]["id"] != ids[0] || after[1]["id"] != ids[1] || !PrimaryPortComplete(active(restored)) {
		t.Fatal("restart/out-of-order replay duplicated station identity or lost establishment")
	}
	if err := p.Update(M{"action": "add-building", "projectId": project["id"], "name": "Wrong ship link", "typeId": "custom", "marketId": "201"}); err == nil {
		t.Fatal("supply ship remained a selectable/attachable construction depot")
	}
}

func TestObservedStationEnrichesMatchingManualPortWithoutReplacingPlan(t *testing.T) {
	p := New(nil)
	action(t, p, M{"action": "create-project", "targetSystem": "Synthetic Station Colony", "systemAddress": "12345", "name": "Synthetic Custom Project"})
	projectID := active(p)["id"]
	action(t, p, M{"action": "add-building", "projectId": projectID, "name": "My manual port", "typeId": "custom", "marketId": "202", "notes": "Keep manual notes"})
	before := rows(rows(p.ExportSettings()["projects"])[0]["buildings"])[0]
	discoveryClaim(p, "Synthetic Station Colony", "12345", "2026-11-01T12:00:00Z")
	observeStation(p, "Synthetic Actual Station", "SurfaceStation", "202", "12345", "2026-11-01T12:01:00Z", []string{"shipyard"})
	for _, planner := range []*Planner{p, New(p.ExportSettings())} {
		project := active(planner)
		buildings := rows(project["buildings"])
		if len(buildings) != 1 || buildings[0]["id"] != before["id"] || buildings[0]["name"] != before["name"] || buildings[0]["typeId"] != before["typeId"] || buildings[0]["notes"] != before["notes"] || buildings[0]["journalDiscovered"] != false || buildings[0]["journalSource"] != "station" || buildings[0]["status"] != "complete" || !PrimaryPortComplete(project) {
			t.Fatal("manual port lost its identity or actual station completion evidence")
		}
		stored := rows(rows(planner.ExportSettings()["projects"])[0]["buildings"])[0]
		if stored["status"] != before["status"] {
			t.Fatal("observed status overwrote an explicit manual status")
		}
	}
}

func TestStationDiscoveryRequiresOwnClaimExactAddressAndActualVisit(t *testing.T) {
	p := New(nil)
	observeStation(p, "Synthetic Old Port", "Orbis", "200", "12345", "2026-10-31T12:00:00Z", nil)
	discoveryClaim(p, "Synthetic Station Colony", "12345", "2026-11-01T12:00:00Z")
	for _, tc := range []struct{ typ, name, address string }{
		{"SurfaceStation", "Synthetic Wrong Address", "99999"}, {"FleetCarrier", "Synthetic Carrier", "12345"},
		{"MegaShip", "Synthetic Megaship", "12345"}, {"OrbitalConstructionSite", "Orbital Construction Site: Synthetic", "12345"},
		{"PlanetaryConstructionDepot", "Planetary Construction Site: Synthetic", "12345"}, {"", "Synthetic Unknown Type", "12345"},
	} {
		observeStation(p, tc.name, tc.typ, "201", tc.address, "2026-11-01T12:01:00Z", nil)
	}
	discoveryEvent(p, "Location", "2026-11-01T12:02:00Z", M{"StationName": "Synthetic Nearby Port", "StationType": "Orbis", "MarketID": "202", "SystemAddress": "12345", "StarSystem": "Synthetic Station Colony", "Docked": false}, nil)
	if len(rows(active(p)["buildings"])) != 0 {
		t.Fatal("old/foreign/carrier/depot/undocked observation became a finished personal station")
	}
}

func TestShipMigrationRemovesOnlySupplyVesselAndItsWorkingQueueLinks(t *testing.T) {
	saved := M{"selectedProjectId": "synthetic-project", "projects": []M{{"id": "synthetic-project", "targetSystem": "Synthetic Station Colony", "buildings": []M{
		{"id": "ship", "typeId": "custom", "name": "$EXT_PANEL_ColonisationShip; SYN-001", "marketId": "201", "status": "complete", "journalDiscovered": true},
		{"id": "manual", "typeId": "coriolis", "name": "Synthetic Manual Plan", "notes": "Keep this", "dependsOn": []string{"ship"}},
	}, "supplyOrder": []string{"ship", "manual"}, "deliveries": []M{{"id": "delivery", "buildingId": "manual", "commodity": "steel", "count": 5}}}}}
	p := New(saved)
	project := rows(p.ExportSettings()["projects"])[0]
	if p.SelectorSnapshot()["selectedProjectId"] != "synthetic-project" || len(rows(project["buildings"])) != 1 || rows(project["buildings"])[0]["id"] != "manual" || !reflect.DeepEqual(stringsOf(project["supplyOrder"]), []string{"manual"}) || len(stringsOf(rows(project["buildings"])[0]["dependsOn"])) != 0 || rows(project["buildings"])[0]["notes"] != "Keep this" || len(rows(project["deliveries"])) != 1 {
		t.Fatal("ship migration retained phantom work or changed an unrelated manual plan")
	}
	for _, b := range rows(active(p)["buildings"]) {
		if strings.Contains(str(b["name"]), "ColonisationShip") || IsColonisationShip(b) {
			t.Fatal("supply vessel remained in a working building table")
		}
	}
	if value.Num(active(p)["totalMissing"]) != 0 {
		t.Fatal("removed ship still contributes to building supply metrics")
	}
}
