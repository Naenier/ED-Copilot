package colonization

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/value"
)

func TestOwnedDiscoveredInitialCompletionDeactivatesOnlyItsClaimDeadline(t *testing.T) {
	now := testNow.Add(40 * 24 * time.Hour)
	p := New(nil, WithClock(func() time.Time { return now }))
	claim := value.ISO(testNow)
	depot := func(system, address, market, at string, complete bool) {
		discoveryEvent(p, "ColonisationConstructionDepot", at, M{"MarketID": market, "ConstructionComplete": complete, "ResourcesRequired": []M{{"Name": "$steel_name;", "RequiredAmount": 100, "ProvidedAmount": 20}}}, M{"system": system, "systemAddress": address, "station": "Orbital Construction Site: Synthetic " + market})
	}
	discoveryClaim(p, "Synthetic Established Deadline", "41001", claim)
	discoveryEvent(p, "ColonisationBeaconDeployed", value.ISO(testNow.Add(time.Hour)), nil, M{"system": "Synthetic Established Deadline", "systemAddress": "41001"})
	depot("Synthetic Established Deadline", "41001", "41101", value.ISO(testNow.Add(2*time.Hour)), false)
	initial := active(p)
	if initial["primaryComplete"] != false || initial["deadlineActive"] != true || initial["effectiveDeadline"] != initial["deadline"] || value.Obj(initial["nextTask"])["deadline"] != initial["deadline"] || !strings.Contains(strings.Join(stringsOf(initial["warnings"]), " "), "срок первичного порта истёк") {
		t.Fatal("unfinished primary lost its active/expired deadline")
	}
	originalDeadline := initial["deadline"]
	depot("Synthetic Established Deadline", "41001", "41101", value.ISO(testNow.Add(24*time.Hour)), true)
	depot("Synthetic Established Deadline", "41001", "41102", value.ISO(testNow.Add(25*time.Hour)), false)
	discoveryClaim(p, "Synthetic Unfinished Deadline", "42001", claim)
	depot("Synthetic Unfinished Deadline", "42001", "42101", value.ISO(testNow.Add(2*time.Hour)), false)
	for _, planner := range []*Planner{p, New(p.ExportSettings(), WithClock(func() time.Time { return now }))} {
		projects := rows(planner.Snapshot(nil)["projects"])
		if len(projects) != 2 {
			t.Fatal("independent claim fixtures were merged")
		}
		established, unfinished := projects[0], projects[1]
		if established["status"] != "building" || established["primaryComplete"] != true || established["deadlineActive"] != false || established["effectiveDeadline"] != nil || established["deadline"] != originalDeadline || value.Obj(established["nextTask"])["deadline"] != nil || len(rows(established["supplyQueue"])) == 0 {
			t.Fatalf("completed initial phase retained a live deadline or closed later construction: status=%v primary=%v active=%v effective=%v raw=%v original=%v taskDeadline=%v supplyCount=%d", established["status"], established["primaryComplete"], established["deadlineActive"], established["effectiveDeadline"], established["deadline"], originalDeadline, value.Obj(established["nextTask"])["deadline"], len(rows(established["operationalSupplies"])))
		}
		if planner == p && len(rows(established["operationalSupplies"])) == 0 {
			t.Fatal("live subsequent depot requirements were dropped")
		}
		if strings.Contains(strings.Join(stringsOf(established["warnings"]), " "), "срок первичного порта истёк") {
			t.Fatal("historical primary deadline still generated an expired warning")
		}
		if unfinished["primaryComplete"] != false || unfinished["deadlineActive"] != true || unfinished["effectiveDeadline"] == nil {
			t.Fatal("established colony suppressed another colony's actual primary deadline")
		}
	}
}

func TestPrimaryCompletionIgnoresOldForeignAndUnconfirmedConstruction(t *testing.T) {
	claimAt := testNow
	base := M{"claimedAt": value.ISO(claimAt), "journalOwnership": M{"source": "journal-claim", "claimedAt": value.ISO(claimAt), "beaconAt": value.ISO(claimAt.Add(time.Hour))}, "buildings": []M{{"name": "Orbital Construction Site: Synthetic Primary", "status": "complete", "effectiveStatus": "complete", "journalDiscovered": true, "journalStatus": "complete", "journalUpdatedAt": value.ISO(claimAt.Add(24 * time.Hour))}}}
	if !PrimaryPortComplete(base) {
		t.Fatal("confirmed own construction did not establish the colony")
	}
	station := copyM(base)
	observed := rows(station["buildings"])[0]
	observed["name"], observed["journalStationName"], observed["journalStationType"], observed["journalSource"] = "Synthetic Actual Port", "Synthetic Actual Port", "SurfaceStation", "station"
	if !PrimaryPortComplete(station) {
		t.Fatal("confirmed permanent station in the current own claim did not establish the colony")
	}
	for name, mutate := range map[string]func(M){
		"old completion":                              func(p M) { rows(p["buildings"])[0]["journalUpdatedAt"] = value.ISO(claimAt.Add(-time.Hour)) },
		"foreign observation":                         func(p M) { p["journalOwnership"] = nil },
		"manual secondary":                            func(p M) { rows(p["buildings"])[0]["journalDiscovered"] = false },
		"no completion observation":                   func(p M) { rows(p["buildings"])[0]["journalStatus"] = "building" },
		"unknown observation date":                    func(p M) { rows(p["buildings"])[0]["journalUpdatedAt"] = nil },
		"generic depot without construction evidence": func(p M) { rows(p["buildings"])[0]["name"] = "Synthetic Unidentified Depot" },
		"colonisation ship is not a building": func(p M) {
			b := rows(p["buildings"])[0]
			b["name"], b["journalStationName"], b["isPrimary"] = "Synthetic Ship", "$EXT_PANEL_ColonisationShip; Synthetic Ship", true
		},
	} {
		t.Run(name, func(t *testing.T) {
			project := copyM(base)
			mutate(project)
			before, _ := json.Marshal(project)
			if PrimaryPortComplete(project) {
				t.Fatal("unconfirmed/old/foreign evidence hid the current claim deadline")
			}
			after, _ := json.Marshal(project)
			if string(before) != string(after) {
				t.Fatal("completion check mutated caller-owned observations")
			}
		})
	}
}

func TestNewClaimDeadlineSurvivesHistoricalCompletedDepotOnRestart(t *testing.T) {
	claimAt := testNow
	saved := M{"selectedProjectId": "synthetic-new-claim", "projects": []M{{"id": "synthetic-new-claim", "name": "Synthetic Reclaimed Colony", "targetSystem": "Synthetic Reclaimed System", "status": "building", "claimedAt": value.ISO(claimAt), "journalOwnership": M{"source": "journal-claim", "status": "beacon", "claimedAt": value.ISO(claimAt), "beaconAt": value.ISO(claimAt.Add(time.Hour))}, "buildings": []M{{"id": "old-initial", "typeId": "custom", "status": "complete", "journalDiscovered": true, "journalStatus": "complete", "journalUpdatedAt": value.ISO(claimAt.Add(-24 * time.Hour))}, {"id": "new-initial", "typeId": "custom", "status": "building", "journalDiscovered": true, "journalStatus": "building", "journalUpdatedAt": value.ISO(claimAt.Add(2 * time.Hour))}}}}}
	p := New(saved, WithClock(func() time.Time { return claimAt.Add(3 * time.Hour) }))
	for _, planner := range []*Planner{p, New(p.ExportSettings())} {
		project := value.Obj(planner.Snapshot(nil)["activeProject"])
		if project["primaryComplete"] != false || project["deadlineActive"] != true || project["effectiveDeadline"] == nil || project["effectiveDeadline"] != project["deadline"] {
			t.Fatal("an old completed depot suppressed the newer claim's deadline")
		}
	}
}

func TestOwnedObservedStationAndSecondaryQueueDoNotRecommendAnotherPrimary(t *testing.T) {
	p := New(nil, WithClock(func() time.Time { return testNow.Add(3 * 24 * time.Hour) }))
	discoveryClaim(p, "Synthetic Observed Port System", "43001", value.ISO(testNow))
	cmd := M{"system": "Synthetic Observed Port System", "systemAddress": "43001"}
	discoveryEvent(p, "ColonisationBeaconDeployed", value.ISO(testNow.Add(time.Hour)), nil, cmd)
	discoveryEvent(p, "Docked", value.ISO(testNow.Add(24*time.Hour)), M{"MarketID": "43101", "StationName": "Synthetic Established Port", "StationType": "SurfaceStation", "StarSystem": "Synthetic Observed Port System", "SystemAddress": "43001", "StationServices": []string{"shipyard", "outfitting", "registeringcolonisation"}}, cmd)
	discoveryEvent(p, "ColonisationConstructionDepot", value.ISO(testNow.Add(25*time.Hour)), M{"MarketID": "43102", "StationName": "Planetary Construction Site: Synthetic Next Facility", "ConstructionComplete": false, "ResourcesRequired": []M{{"Name": "$steel_name;", "RequiredAmount": 100, "ProvidedAmount": 20}}}, cmd)
	for _, planner := range []*Planner{p, New(p.ExportSettings())} {
		project := value.Obj(planner.Snapshot(nil)["activeProject"])
		advice := rows(project["buildAdvice"])
		if project["primaryComplete"] != true || project["deadlineActive"] != false || len(rows(project["supplyQueue"])) == 0 || len(advice) == 0 || str(advice[0]["title"]) == "Сначала выберите первичный порт" || !strings.Contains(str(advice[0]["text"]), "Первичный порт уже построен") {
			t.Fatal("confirmed existing port still received first-port advice or lost its secondary queue")
		}
		if len(stringsOf(advice[0]["typeIds"])) != 0 {
			t.Fatal("observed station without a known variant received an invented primary-port type")
		}
	}
}

func TestManualLinkedPermanentStationRequiresFreshOwnJournalConfirmation(t *testing.T) {
	claimAt := testNow
	project := M{"claimedAt": value.ISO(claimAt), "journalOwnership": M{"source": "journal-claim", "claimedAt": value.ISO(claimAt)}, "buildings": []M{{"name": "Synthetic Manual Port Label", "typeId": "custom", "status": "planned", "effectiveStatus": "complete", "marketId": "44101", "journalDiscovered": false, "journalSource": "station", "journalStationName": "Synthetic Actual Port", "journalStationType": "SurfaceStation", "journalStatus": "complete", "journalUpdatedAt": value.ISO(claimAt.Add(24 * time.Hour))}}}
	before, _ := json.Marshal(project)
	if !PrimaryPortComplete(project) {
		t.Fatal("matching manual port ignored its actual fresh permanent-station observation")
	}
	for name, change := range map[string]func(M){
		"no own claim":                func(p M) { p["journalOwnership"] = nil },
		"old observation":             func(p M) { rows(p["buildings"])[0]["journalUpdatedAt"] = value.ISO(claimAt.Add(-time.Hour)) },
		"generic depot":               func(p M) { rows(p["buildings"])[0]["journalSource"] = "depot" },
		"unknown actual station type": func(p M) { rows(p["buildings"])[0]["journalStationType"] = "" },
		"colonisation ship": func(p M) {
			rows(p["buildings"])[0]["journalStationName"] = "$EXT_PANEL_ColonisationShip; Synthetic Ship"
		},
	} {
		t.Run(name, func(t *testing.T) {
			copy := copyM(project)
			change(copy)
			if PrimaryPortComplete(copy) {
				t.Fatal("unconfirmed manual/depot/ship evidence established a port")
			}
		})
	}
	after, _ := json.Marshal(project)
	if string(before) != string(after) {
		t.Fatal("observed completion changed manual names, types, statuses or journal linkage")
	}
}
