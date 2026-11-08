package commander

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func event(kind string, second int, fields M) M {
	return v.Merge(M{"event": kind, "timestamp": fmt.Sprintf("2026-11-08T12:%02d:%02dZ", second/60, second%60)}, fields)
}
func scope(fid string) M {
	return M{"commander": M{"fid": fid, "name": "Synthetic Pilot", "system": "Synthetic System", "systemAddress": json.Number("18446744073709551614")}}
}
func start(t *Tracker) {
	t.Apply(event("Commander", 0, M{"FID": "FTEST", "Name": "Synthetic Pilot"}), scope("FTEST"))
}

func TestUnknownAndKnownZeroRemainDifferent(t *testing.T) {
	tracker := New()
	state := tracker.Snapshot()
	if v.At(state, "finance", "credits") != nil || v.At(state, "finance", "known") != false || v.At(state, "fleet", "count") != nil || v.At(state, "inventory", "materials", "raw", "count") != nil {
		t.Fatal("unknown observations became zero", state)
	}
	start(tracker)
	tracker.Apply(event("LoadGame", 1, M{"Credits": 0, "Loan": 0, "Ship": "panthermkii", "ShipID": 0}), scope("FTEST"))
	tracker.Apply(event("Materials", 2, M{"Raw": []M{}, "Manufactured": []M{{"Name": "iron", "Count": 0}}}), scope("FTEST"))
	tracker.Apply(event("Reputation", 3, M{"Federation": 0, "Empire": -99.5}), scope("FTEST"))
	tracker.Apply(event("Statistics", 4, M{"Bank_Account": M{"Current_Wealth": 0, "Spent_On_Ships": 12}, "Combat": M{"Bounties_Claimed": 0}}), scope("FTEST"))
	state = tracker.Snapshot()
	for _, path := range [][]string{{"finance", "credits"}, {"finance", "loan"}, {"finance", "netWorth"}, {"inventory", "materials", "raw", "count"}, {"reputation", "federation"}} {
		if v.At(state, path...) == nil || v.Num(v.At(state, path...)) != 0 {
			t.Fatalf("observed zero missing at %v", path)
		}
	}
	if v.At(state, "inventory", "materials", "encoded", "known") != false || v.At(state, "reputation", "alliance") != nil {
		t.Fatal("missing group gained a value")
	}
	if v.At(state, "fleet", "currentShip", "ship") != "Panther Clipper Mk II" || v.At(state, "fleet", "currentShip", "shipId") != "0" {
		t.Fatal("ship display or zero ID lost", state["fleet"])
	}
	if v.At(state, "fleet", "known") != false || v.At(state, "fleet", "complete") != false {
		t.Fatal("one current ship was presented as a fleet")
	}
}

func TestIdentitySwitchClearsEveryDomainAndRejectsStaleOwners(t *testing.T) {
	tracker := New()
	start(tracker)
	tracker.Apply(event("LoadGame", 5, M{"Credits": 1200, "Ship": "python", "ShipID": 4}), scope("FTEST"))
	tracker.Apply(event("Statistics", 6, M{"Combat": M{"Bounties_Claimed": 10}}), scope("FTEST"))
	tracker.Apply(event("ShipLocker", 7, M{"Items": []M{{"Name": "healthpack", "Count": 3}}}), scope("FTEST"))
	if !tracker.Apply(event("Commander", 10, M{"FID": "FOTHER"}), scope("FOTHER")) {
		t.Fatal("new owner not accepted")
	}
	state := tracker.Snapshot()
	if state["commanderFID"] != "FOTHER" || v.At(state, "finance", "credits") != nil || v.At(state, "fleet", "currentShip", "known") != false || v.At(state, "statistics", "known") != false || v.At(state, "inventory", "shipLocker", "items", "known") != false {
		t.Fatal("previous owner's data survived", state)
	}
	before := tracker.Snapshot()
	tracker.Apply(event("Commander", 2, M{"FID": "FTEST"}), scope("FTEST"))
	tracker.Apply(event("LoadGame", 20, M{"FID": "FTEST", "Credits": 99}), scope("FOTHER"))
	tracker.Apply(event("Status", 20, M{"Balance": 99}), scope("FTEST"))
	if !reflect.DeepEqual(before, tracker.Snapshot()) {
		t.Fatal("stale or mismatching owner changed state")
	}
	tracker.Reset()
	if tracker.Snapshot()["commanderFID"] != "" || v.At(tracker.Snapshot(), "finance", "known") != false {
		t.Fatal("reset retained observations")
	}
}

func TestDuplicateAndOlderSnapshotsAreIdempotent(t *testing.T) {
	tracker := New()
	start(tracker)
	for _, e := range []M{
		event("Status", 10, M{"Balance": 42}),
		event("Materials", 10, M{"Raw": []M{{"Name": "iron", "Count": 5}, {"Name": "iron", "Count": 5}}}),
		event("Reputation", 10, M{"Alliance": -2}),
		event("Statistics", 10, M{"Exploration": M{"Total_Hyperspace_Distance": 19.5}}),
		event("StoredModules", 10, M{"Items": []M{{"StorageSlot": 1, "Name": "int_fsd_size5_class5"}}}),
	} {
		tracker.Apply(e, scope("FTEST"))
		before := tracker.Snapshot()
		tracker.Apply(e, scope("FTEST"))
		if !reflect.DeepEqual(before, tracker.Snapshot()) {
			t.Fatal("duplicate changed observation", e)
		}
	}
	before := tracker.Snapshot()
	tracker.Apply(event("Status", 9, M{"Balance": 900}), scope("FTEST"))
	tracker.Apply(event("Materials", 9, M{"Raw": []M{}}), scope("FTEST"))
	tracker.Apply(event("Reputation", 9, M{"Alliance": 100}), scope("FTEST"))
	tracker.Apply(event("Statistics", 9, M{"Exploration": M{"Total_Hyperspace_Distance": 0}}), scope("FTEST"))
	tracker.Apply(event("StoredModules", 9, M{"Items": []M{}}), scope("FTEST"))
	if !reflect.DeepEqual(before, tracker.Snapshot()) {
		t.Fatal("older snapshot replaced a newer field")
	}
	if v.Num(v.At(before, "inventory", "materials", "raw", "count")) != 5 {
		t.Fatal("duplicate inventory rows doubled the count")
	}
}

func TestStoredShipsAreIndependentOfStationOffersAndRetainExactIDs(t *testing.T) {
	tracker := New()
	start(tracker)
	tracker.Apply(event("Shipyard", 1, M{"PriceList": []M{{"id": 1, "ShipType": "anaconda", "ShipPrice": 100}}}), scope("FTEST"))
	if v.At(tracker.Snapshot(), "fleet", "known") != false {
		t.Fatal("station offers became player ships")
	}
	stored := event("StoredShips", 2, M{"StationName": "Synthetic Port", "StarSystem": "Synthetic System", "MarketID": json.Number("18446744073709551614"), "ShipsHere": []M{{"ShipID": json.Number("18446744073709551615"), "ShipType": "python", "Name": "My ship", "Value": 0}, {"ShipID": 0, "ShipType": "empire_eagle"}}, "ShipsRemote": []M{{"ShipID": json.Number("18446744073709551615"), "ShipType": "python"}}})
	tracker.Apply(stored, scope("FTEST"))
	state := tracker.Snapshot()
	rows := v.Rows(v.At(state, "fleet", "entries"))
	if len(rows) != 2 || rows[1]["shipId"] != "18446744073709551615" || rows[1]["ship"] != "Python" || rows[1]["shipName"] != "My ship" || rows[1]["marketId"] != "18446744073709551614" {
		t.Fatal("fleet ID, name or duplicate handling failed", rows)
	}
	if v.At(state, "fleet", "complete") != false || v.At(state, "fleet", "storedListComplete") != true {
		t.Fatal("stored list overstated full fleet completeness")
	}
	tracker.Apply(event("StoredShips", 1, M{"ShipsHere": []M{}, "ShipsRemote": []M{}}), scope("FTEST"))
	if !reflect.DeepEqual(state, tracker.Snapshot()) {
		t.Fatal("older fleet replaced latest observation")
	}
}

func TestChangingCurrentShipClearsPreviousRebuyAndRejectsVehicles(t *testing.T) {
	tracker := New()
	start(tracker)
	tracker.Apply(event("Loadout", 1, M{"Ship": "python", "ShipID": 1, "HullValue": 10, "ModulesValue": 20, "Rebuy": 2}), scope("FTEST"))
	if v.Num(v.At(tracker.Snapshot(), "finance", "shipValue")) != 30 {
		t.Fatal("hull plus modules not calculated")
	}
	tracker.Apply(event("LoadGame", 2, M{"Ship": "testbuggy", "ShipID": 7}), scope("FTEST"))
	if v.At(tracker.Snapshot(), "fleet", "currentShip", "shipId") != "1" {
		t.Fatal("SRV became a ship")
	}
	tracker.Apply(event("ShipyardSwap", 3, M{"ShipType": "anaconda", "ShipID": 2}), scope("FTEST"))
	state := tracker.Snapshot()
	if v.At(state, "fleet", "currentShip", "shipId") != "2" || v.At(state, "finance", "rebuy") != nil || v.At(state, "finance", "shipValue") != nil {
		t.Fatal("previous ship finance leaked to new ship", state)
	}
}

func TestInventoryContainersDoNotMergeAndEmptyNotificationsDoNotErase(t *testing.T) {
	tracker := New()
	start(tracker)
	tracker.Apply(event("Backpack", 1, M{"Items": []M{{"Name": "healthpack", "Count": 2, "MissionID": json.Number("18446744073709551614")}}, "Data": []M{}}), scope("FTEST"))
	tracker.Apply(event("ShipLocker", 2, M{"Items": []M{{"Name": "healthpack", "Count": 10}}, "Components": []M{}}), scope("FTEST"))
	tracker.Apply(event("ShipLocker", 3, nil), scope("FTEST"))
	state := tracker.Snapshot()
	if v.Num(v.At(state, "inventory", "backpack", "items", "count")) != 2 || v.Num(v.At(state, "inventory", "shipLocker", "items", "count")) != 10 || v.At(state, "inventory", "backpack", "components", "known") != false {
		t.Fatal("containers or unknown groups merged", state["inventory"])
	}
	if v.Rows(v.At(state, "inventory", "backpack", "items", "entries"))[0]["missionId"] != "18446744073709551614" {
		t.Fatal("inventory mission ID was rounded")
	}
	tracker.Apply(event("ShipLocker", 4, M{"Items": []M{}}), scope("FTEST"))
	if v.At(tracker.Snapshot(), "inventory", "shipLocker", "items", "count") == nil || v.Num(v.At(tracker.Snapshot(), "inventory", "shipLocker", "items", "count")) != 0 {
		t.Fatal("explicit empty inventory is not known zero")
	}
}

func TestCAPIRequiresMatchingVerifiedIdentityAndCannotSwitchOwner(t *testing.T) {
	tracker := New()
	start(tracker)
	profile := M{"commander": M{"name": "Synthetic Pilot", "credits": 0, "debt": 12}, "ship": M{"id": json.Number("18446744073709551615"), "name": "panthermkii", "shipName": "Synthetic Hauler"}}
	ctx := v.Merge(scope("FTEST"), M{"observedAt": "2026-11-08T12:00:10Z"})
	before := tracker.Snapshot()
	if tracker.ApplyCAPI(profile, ctx) || !reflect.DeepEqual(before, tracker.Snapshot()) {
		t.Fatal("unverified CAPI accepted")
	}
	ctx["identityVerified"] = true
	if !tracker.ApplyCAPI(profile, ctx) {
		t.Fatal("verified current profile ignored")
	}
	state := tracker.Snapshot()
	if v.At(state, "finance", "credits") == nil || v.At(state, "fleet", "currentShip", "shipId") != "18446744073709551615" || v.At(state, "fleet", "known") != false {
		t.Fatal("CAPI identity or partial fleet lost", state)
	}
	other := v.Merge(scope("FOTHER"), M{"identityVerified": true, "observedAt": "2026-11-08T12:00:20Z"})
	if tracker.ApplyCAPI(profile, other) || !reflect.DeepEqual(state, tracker.Snapshot()) {
		t.Fatal("CAPI switched owner")
	}
	bad := v.Copy(profile)
	v.Obj(bad["commander"])["name"] = "Another Pilot"
	if tracker.ApplyCAPI(bad, ctx) {
		t.Fatal("mismatched CAPI name accepted")
	}
	tracker.Apply(event("Status", 10, M{"Balance": 7}), scope("FTEST"))
	tracker.ApplyCAPI(profile, ctx)
	if v.Num(v.At(tracker.Snapshot(), "finance", "credits")) != 7 {
		t.Fatal("same-time CAPI overrode journal")
	}
}

func TestResetGameBlocksEarlierObservationsAndFreshOwnerWorks(t *testing.T) {
	tracker := New()
	start(tracker)
	tracker.Apply(event("Status", 10, M{"Balance": 900}), scope("FTEST"))
	tracker.Apply(event("ClearSavedGame", 20, M{"FID": "FTEST"}), scope("FTEST"))
	tracker.Apply(event("Status", 15, M{"Balance": 100}), scope("FTEST"))
	if v.At(tracker.Snapshot(), "finance", "credits") != nil {
		t.Fatal("pre-reset observation returned")
	}
	tracker.Apply(event("LoadGame", 21, M{"Credits": 0, "Loan": 0}), scope("FTEST"))
	if v.At(tracker.Snapshot(), "finance", "credits") == nil {
		t.Fatal("new game zero credits lost")
	}
}

func TestBoundsInvalidValuesAndDetachedSnapshots(t *testing.T) {
	tracker := New()
	start(tracker)
	rows := []M{}
	for i := 0; i < maxShips+12; i++ {
		rows = append(rows, M{"ShipID": i, "ShipType": "python"})
	}
	tracker.Apply(event("StoredShips", 1, M{"ShipsHere": rows, "ShipsRemote": []M{}}), scope("FTEST"))
	tracker.Apply(event("LoadGame", 2, M{"Credits": -3, "Loan": "0"}), scope("FTEST"))
	tracker.Apply(event("Materials", 3, M{"Raw": []M{{"Name": "iron", "Count": 1.5}}}), scope("FTEST"))
	tracker.Apply(event("Reputation", 4, M{"Federation": 101, "Alliance": "0"}), scope("FTEST"))
	state := tracker.Snapshot()
	if len(v.Rows(v.At(state, "fleet", "entries"))) != maxShips || v.At(state, "fleet", "truncated") != true || v.At(state, "fleet", "count") != nil {
		t.Fatal("fleet bound or completeness failed")
	}
	if v.At(state, "finance", "credits") != nil || v.At(state, "finance", "loan") != nil || v.At(state, "reputation", "alliance") != nil || v.At(state, "inventory", "materials", "raw", "count") != nil {
		t.Fatal("invalid values became trustworthy numbers")
	}
	v.Obj(state["finance"])["credits"] = 999
	v.Rows(v.At(state, "fleet", "entries"))[0]["ship"] = "changed"
	if v.At(tracker.Snapshot(), "finance", "credits") != nil || v.Rows(v.At(tracker.Snapshot(), "fleet", "entries"))[0]["ship"] == "changed" {
		t.Fatal("snapshot shares mutable state")
	}
	for _, raw := range []any{json.Number("18446744073709551615"), uint64(18446744073709551615), "18446744073709551615"} {
		if exactID(raw) != "18446744073709551615" {
			t.Fatal("valid exact ID rejected", raw)
		}
	}
	for _, raw := range []any{float64(18446744073709551615), -1, "1e5", json.Number("1.5")} {
		if exactID(raw) != "" {
			t.Fatal("unsafe ID accepted", raw)
		}
	}
}

func TestConcurrentSnapshotsAndObservations(t *testing.T) {
	tracker := New()
	start(tracker)
	var group sync.WaitGroup
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func(n int) {
			defer group.Done()
			for j := 0; j < 10; j++ {
				tracker.Apply(event("Status", n+1, M{"Balance": n}), scope("FTEST"))
				tracker.Snapshot()
			}
		}(i)
	}
	group.Wait()
	if v.Num(v.At(tracker.Snapshot(), "finance", "credits")) != 11 {
		t.Fatal("latest concurrent balance not retained")
	}
}

func TestChangesInvalidateTotalsUntilNewCompleteSnapshot(t *testing.T) {
	tracker := New()
	start(tracker)
	tracker.Apply(event("Backpack", 1, M{"Items": []M{{"Name": "healthpack", "Count": 4}}}), scope("FTEST"))
	tracker.Apply(event("BackpackChange", 3, M{"Removed": []M{{"Name": "healthpack", "Count": 1}}}), scope("FTEST"))
	group := v.Obj(v.At(tracker.Snapshot(), "inventory", "backpack", "items"))
	if group["known"] != false || group["count"] != nil || group["needsSnapshot"] != true || v.Num(group["lastKnownCount"]) != 4 || len(v.Rows(group["entries"])) != 1 {
		t.Fatal("mutation silently kept old current totals", group)
	}
	tracker.Apply(event("Backpack", 2, M{"Items": []M{{"Name": "healthpack", "Count": 99}}}), scope("FTEST"))
	if v.At(tracker.Snapshot(), "inventory", "backpack", "items", "known") != false {
		t.Fatal("older full list undid later invalidation")
	}
	tracker.Apply(event("Backpack", 4, M{"Items": []M{{"Name": "healthpack", "Count": 3}}}), scope("FTEST"))
	group = v.Obj(v.At(tracker.Snapshot(), "inventory", "backpack", "items"))
	if group["known"] != true || v.Num(group["count"]) != 3 || group["needsSnapshot"] != nil {
		t.Fatal("new complete list did not restore known count", group)
	}
}
