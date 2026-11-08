package colonization

import (
	"reflect"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/value"
)

func discoveryEvent(p *Planner, kind, at string, fields, commander M) {
	p.Apply(merge(M{"event": kind, "timestamp": at}, fields), M{"commander": commander})
}

func discoveryClaim(p *Planner, name, address, at string) {
	discoveryEvent(p, "ColonisationSystemClaim", at, M{"StarSystem": name, "SystemAddress": address}, M{"system": "Synthetic Origin"})
}

func discoveryDepot(p *Planner, system, address, market, at string, complete bool) {
	discoveryEvent(p, "ColonisationConstructionDepot", at, M{"MarketID": market, "ConstructionComplete": complete, "ConstructionFailed": false, "ResourcesRequired": []M{{"Name": "$steel_name;", "RequiredAmount": 100, "ProvidedAmount": 20}}}, M{"system": system, "systemAddress": address, "station": "Synthetic Construction Site"})
}

func TestOwnClaimAutomaticallyPopulatesSelectorHUDAndIndependentSupply(t *testing.T) {
	p := New(nil)
	discoveryEvent(p, "LoadGame", "2026-11-01T12:00:00Z", M{"FID": "FSYNTHETIC"}, nil)
	discoveryClaim(p, "Synthetic Colony A", "12345", "2026-11-01T12:01:00Z")
	discoveryEvent(p, "ColonisationBeaconDeployed", "2026-11-01T12:02:00Z", nil, M{"system": "Synthetic Colony A", "systemAddress": "12345"})
	discoveryDepot(p, "Synthetic Colony A", "12345", "201", "2026-11-01T12:03:00Z", false)
	selector, full := p.SelectorSnapshot(), p.Snapshot(nil)
	projects := rows(selector["projects"])
	if len(projects) != 1 || selector["selectedProjectId"] != projects[0]["id"] || value.Obj(full["activeProject"])["id"] != projects[0]["id"] {
		t.Fatal("owned claim did not produce one consistently selected colony")
	}
	project := value.Obj(full["activeProject"])
	buildings := rows(project["buildings"])
	if project["targetSystem"] != "Synthetic Colony A" || project["originSystem"] != "Synthetic Origin" || len(buildings) != 1 || buildings[0]["marketId"] != "201" || buildings[0]["typeId"] != "custom" || buildings[0]["tier"] != nil {
		t.Fatal("observed construction was lost or assigned an invented type/tier", project)
	}
	if num(project["totalMissing"]) != 80 || str(buildings[0]["billSource"]) != "journal" || num(rows(project["supplies"])[0]["missing"]) != 80 {
		t.Fatal("owned depot's authoritative bill did not reach the supply queue")
	}
	if projects[0]["buildings"] != nil || projects[0]["supplies"] != nil {
		t.Fatal("global selector expanded detailed supply data")
	}
	firstID, firstBuildingID := projects[0]["id"], buildings[0]["id"]
	p.Reset()
	discoveryEvent(p, "LoadGame", "2026-11-01T12:00:00Z", M{"FID": "FSYNTHETIC"}, nil)
	discoveryClaim(p, "Synthetic Colony A", "12345", "2026-11-01T12:01:00Z")
	discoveryDepot(p, "Synthetic Colony A", "12345", "201", "2026-11-01T12:03:00Z", false)
	if len(rows(p.SelectorSnapshot()["projects"])) != 1 || active(p)["id"] != firstID || len(rows(active(p)["buildings"])) != 1 || rows(active(p)["buildings"])[0]["id"] != firstBuildingID {
		t.Fatal("replay duplicated a discovered colony or construction stage")
	}
}

func TestForeignDepotContributionAndBeaconDoNotProveOwnership(t *testing.T) {
	p := New(nil)
	discoveryDepot(p, "Synthetic Foreign", "300", "301", "2026-11-01T12:00:00Z", false)
	discoveryEvent(p, "ColonisationContribution", "2026-11-01T12:00:01Z", M{"MarketID": "301", "Contributions": []M{{"Name": "$steel_name;", "Amount": 5}}}, M{"system": "Synthetic Foreign", "systemAddress": "300"})
	discoveryEvent(p, "ColonisationBeaconDeployed", "2026-11-01T12:00:02Z", nil, M{"system": "Synthetic Foreign", "systemAddress": "300"})
	discoveryEvent(p, "ColonisationSystemClaimRelease", "2026-11-01T12:00:03Z", M{"StarSystem": "Synthetic Foreign", "SystemAddress": "300"}, nil)
	if len(rows(p.SelectorSnapshot()["projects"])) != 0 || p.Snapshot(nil)["activeProject"] != nil {
		t.Fatal("someone else's construction activity became a personal colony")
	}
	discoveryClaim(p, "Synthetic Own", "400", "2026-11-01T12:01:00Z")
	discoveryDepot(p, "Synthetic Own", "401", "402", "2026-11-01T12:02:00Z", false)
	if len(rows(active(p)["buildings"])) != 0 {
		t.Fatal("same name with a conflicting SystemAddress linked a foreign depot")
	}
}

func TestDiscoverClaimPreservesManualProjectIDsQueueAndSelectedOtherColony(t *testing.T) {
	p := New(nil)
	manual := colony(t, p, "Synthetic Manual A", 100, 201)
	manualID := manual["id"]
	bs := rows(manual["buildings"])
	action(t, p, M{"action": "reorder-supply", "projectId": manualID, "ids": []string{str(bs[1]["id"]), str(bs[0]["id"])}})
	action(t, p, M{"action": "update-project", "id": manualID, "originSystem": "Synthetic Chosen Origin", "faction": "Synthetic Chosen Faction", "notes": "Keep my plan"})
	other := colony(t, p, "Synthetic Manual B", 200, 202)
	before := rows(p.ExportSettings()["projects"])[0]
	discoveryClaim(p, "Synthetic Manual A", "12345", "2026-11-01T12:01:00Z")
	after := rows(p.ExportSettings()["projects"])[0]
	for _, key := range []string{"id", "name", "originSystem", "faction", "notes", "buildings", "supplyOrder", "deliveries"} {
		if !reflect.DeepEqual(before[key], after[key]) {
			t.Fatalf("claim overwrote an existing manual field: %s", key)
		}
	}
	if len(rows(p.SelectorSnapshot()["projects"])) != 2 || p.SelectorSnapshot()["selectedProjectId"] != other["id"] {
		t.Fatal("claim duplicated a plan or replaced an explicit selection")
	}
	discoveryDepot(p, "Synthetic Manual A", "12345", "201", "2026-11-01T12:02:00Z", false)
	after = rows(p.ExportSettings()["projects"])[0]
	if len(rows(after["buildings"])) != 2 || !reflect.DeepEqual(before["supplyOrder"], after["supplyOrder"]) || rows(after["buildings"])[0]["id"] != bs[0]["id"] {
		t.Fatal("observed matching depot duplicated or reordered a manual construction")
	}
}

func TestReleasedClaimDoesNotReviveDuringRestartOrSameTimestampReplay(t *testing.T) {
	p := New(nil)
	// Journal events can share a timestamp; lifecycle order must remain terminal.
	const at = "2026-11-01T12:00:00Z"
	discoveryClaim(p, "Synthetic Released", "12345", at)
	discoveryEvent(p, "ColonisationBeaconDeployed", at, nil, M{"system": "Synthetic Released", "systemAddress": "12345"})
	discoveryEvent(p, "ColonisationSystemClaimRelease", at, M{"StarSystem": "Synthetic Released", "SystemAddress": "12345"}, nil)
	id := active(p)["id"]
	restored := New(p.ExportSettings())
	discoveryClaim(restored, "Synthetic Released", "12345", at)
	discoveryEvent(restored, "ColonisationBeaconDeployed", at, nil, M{"system": "Synthetic Released", "systemAddress": "12345"})
	discoveryDepot(restored, "Synthetic Released", "12345", "201", "2026-11-01T12:01:00Z", false)
	if active(restored)["id"] != id || active(restored)["status"] != "cancelled" || len(rows(active(restored)["buildings"])) != 0 || len(rows(active(restored)["operationalSupplies"])) != 0 {
		t.Fatal("released claim revived or collected new construction from old replay")
	}
	discoveryClaim(restored, "Synthetic Released", "12345", "2026-11-02T12:00:00Z")
	if active(restored)["status"] != "claimed" || active(restored)["id"] != id {
		t.Fatal("a genuinely newer personal claim did not reactivate the same independent plan")
	}
}

func TestCompletedDiscoveredColonySurvivesMissingDepotAndOldReplay(t *testing.T) {
	p := New(nil)
	discoveryClaim(p, "Synthetic Established", "12345", "2026-11-01T12:00:00Z")
	discoveryDepot(p, "Synthetic Established", "12345", "201", "2026-11-01T12:02:00Z", true)
	id := active(p)["id"]
	if active(p)["status"] != "complete" {
		t.Fatal("completed observed construction did not establish a completed plan")
	}
	restored := New(p.ExportSettings())
	if active(restored)["id"] != id || active(restored)["status"] != "complete" || rows(active(restored)["buildings"])[0]["status"] != "complete" {
		t.Fatal("completed owned colony vanished when the depot was no longer current")
	}
	discoveryClaim(restored, "Synthetic Established", "12345", "2026-11-01T12:00:00Z")
	discoveryDepot(restored, "Synthetic Established", "12345", "201", "2026-11-01T12:01:00Z", false)
	if active(restored)["id"] != id || active(restored)["status"] != "complete" {
		t.Fatal("earlier claim/depot replay downgraded completed construction")
	}
}

func TestDiscoveryKeepsIndependentColoniesAndDoesNotSelectAnotherCommandersColony(t *testing.T) {
	p := New(nil)
	discoveryEvent(p, "LoadGame", "2026-11-01T12:00:00Z", M{"FID": "FSYNTHETIC-A"}, nil)
	discoveryClaim(p, "Synthetic Colony A", "12345", "2026-11-01T12:01:00Z")
	firstID := active(p)["id"]
	discoveryClaim(p, "Synthetic Colony B", "23456", "2026-11-01T12:02:00Z")
	discoveryDepot(p, "Synthetic Colony A", "12345", "201", "2026-11-01T12:03:00Z", false)
	discoveryDepot(p, "Synthetic Colony B", "23456", "202", "2026-11-01T12:04:00Z", false)
	if len(rows(p.SelectorSnapshot()["projects"])) != 2 || active(p)["id"] != firstID || len(rows(active(p)["buildings"])) != 1 || rows(active(p)["buildings"])[0]["marketId"] != "201" {
		t.Fatal("independent colonies or explicit active selection were mixed")
	}
	restored := New(p.ExportSettings())
	discoveryEvent(restored, "LoadGame", "2026-11-02T12:00:00Z", M{"FID": "FSYNTHETIC-B"}, nil)
	if len(rows(restored.SelectorSnapshot()["projects"])) != 0 || restored.SelectorSnapshot()["selectedProjectId"] != "" || restored.Snapshot(nil)["activeProject"] != nil {
		t.Fatal("another commander's journal-derived colony leaked into this selector")
	}
	discoveryEvent(restored, "LoadGame", "2026-11-03T12:00:00Z", M{"FID": "FSYNTHETIC-A"}, nil)
	if len(rows(restored.SelectorSnapshot()["projects"])) != 2 || active(restored)["id"] != firstID {
		t.Fatal("switching back destroyed persisted personal colony plans")
	}
}

func TestCompletedFirstDepotDoesNotCloseSecondaryConstructionSupply(t *testing.T) {
	p := New(nil)
	discoveryClaim(p, "Synthetic Continuing Colony", "12345", "2026-11-01T12:00:00Z")
	discoveryEvent(p, "ColonisationBeaconDeployed", "2026-11-01T12:01:00Z", nil, M{"system": "Synthetic Continuing Colony", "systemAddress": "12345"})
	discoveryDepot(p, "Synthetic Continuing Colony", "12345", "201", "2026-11-01T12:02:00Z", true)
	id := active(p)["id"]
	// Even saving after the first completed depot must not close future phases.
	p = New(p.ExportSettings())
	discoveryDepot(p, "Synthetic Continuing Colony", "12345", "202", "2026-11-02T12:00:00Z", false)
	for _, planner := range []*Planner{p, New(p.ExportSettings())} {
		project := active(planner)
		bs := rows(project["buildings"])
		if project["id"] != id || project["status"] != "building" || len(bs) != 2 || bs[0]["status"] != "complete" || bs[1]["status"] != "building" {
			t.Fatal("first completed phase permanently closed the colony plan")
		}
	}
	if needs := rows(active(p)["operationalSupplies"]); len(needs) != 1 || num(needs[0]["missing"]) != 80 {
		t.Fatal("completed first depot hid the unfinished second depot's supply queue")
	}
}
