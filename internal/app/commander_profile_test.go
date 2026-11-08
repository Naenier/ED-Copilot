package app

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"reflect"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func commanderEvent(kind, at string, fields M) M {
	return v.Merge(M{"event": kind, "timestamp": at}, fields)
}

func TestCommanderScopedSnapshotHasRequiredLightweightDomainsOnly(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	state := e.SnapshotForUI("commander")
	for _, key := range []string{"commanderProfile", "ranks", "carrier", "coloniesSelectors", "engineeringInventory"} {
		if state[key] == nil {
			t.Fatal("commander page missing", key)
		}
	}
	for _, path := range [][]string{{"world"}, {"mining", "world"}, {"engineering", "catalog"}, {"engineeringInventory", "catalog"}, {"colonization", "activeProject"}, {"learning", "courses"}} {
		if v.At(state, path...) != nil {
			t.Fatal("commander snapshot computed unrelated detail", path)
		}
	}
	if v.At(state, "commanderProfile", "commanderFID") != "FSYNTHETIC" || v.At(state, "commanderProfile", "fleet", "currentShip", "shipId") != "1" {
		t.Fatal("initial owner/ship not replayed", state["commanderProfile"])
	}
	full := e.Snapshot()
	if !reflect.DeepEqual(full["commanderProfile"], state["commanderProfile"]) || !reflect.DeepEqual(full["coloniesSelectors"], state["coloniesSelectors"]) {
		t.Fatal("full/scoped commander contracts differ")
	}
	for _, tab := range []string{"overview", "settings", "ranks", "politics", "hud", "sync"} {
		other := e.SnapshotForUI(tab)
		for _, key := range []string{"commanderProfile", "engineeringInventory", "coloniesSelectors"} {
			if other[key] != nil {
				t.Fatalf("%s leaked to %s", key, tab)
			}
		}
	}
}

func TestCommanderJournalObservationsRebuildWithoutPersistedDuplicateState(t *testing.T) {
	e, file, data := fixtureEngine(t, nil)
	at := "2026-11-08T12:00:00Z"
	writeEvents(t, file, true, []M{
		commanderEvent("Statistics", at, M{"Bank_Account": M{"Current_Wealth": 0}, "Combat": M{"Bounties_Claimed": 14}}),
		commanderEvent("StoredShips", at, M{"ShipsHere": []M{{"ShipID": json.Number("18446744073709551615"), "ShipType": "python", "Name": "Synthetic Stored Ship"}}, "ShipsRemote": []M{}}),
		commanderEvent("StoredModules", at, M{"Items": []M{{"StorageSlot": 7, "Name": "int_hyperdrive_size6_class5", "Name_Localised": "6A Frame Shift Drive"}}}),
		commanderEvent("Materials", at, M{"Raw": []M{{"Name": "iron", "Count": 10}}, "Manufactured": []M{}, "Encoded": []M{}}),
		commanderEvent("Reputation", at, M{"Empire": 0}),
		commanderEvent("ShipLocker", at, M{"Items": []M{{"Name": "healthpack", "Count": 2}}, "Data": []M{}}),
	})
	e.Reader.Poll()
	before := e.SnapshotForUI("commander")["commanderProfile"]
	if err := e.Save(); err != nil {
		t.Fatal(err)
	}
	e.Close()
	reopened, err := New(Options{DataDir: data, JournalDir: filepath.Dir(file), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after := reopened.SnapshotForUI("commander")["commanderProfile"]
	if !reflect.DeepEqual(before, after) {
		t.Fatal("replay changed commander observations", before, after)
	}
	if v.Rows(v.At(v.Obj(after), "fleet", "entries"))[0]["shipId"] != "18446744073709551615" {
		t.Fatal("stored ship ID was rounded during replay")
	}
	reopened.Reader.Rescan()
	reopened.Reader.Poll()
	if !reflect.DeepEqual(after, reopened.SnapshotForUI("commander")["commanderProfile"]) {
		t.Fatal("second replay duplicated observations")
	}
}

func TestCommanderMaterialProjectionUsesExactExistingDeltaLedger(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	e.Apply(commanderEvent("Materials", "2026-11-08T12:00:00Z", M{"Raw": []M{{"Name": "iron", "Count": 10}}, "Manufactured": []M{}, "Encoded": []M{}}), nil)
	e.Apply(commanderEvent("MaterialCollected", "2026-11-08T12:00:01Z", M{"Category": "Raw", "Name": "iron", "Count": 2}), nil)
	state := e.SnapshotForUI("commander")
	for _, group := range []string{"raw", "manufactured", "encoded"} {
		if v.At(state, "commanderProfile", "inventory", "materials", group, "known") != true {
			t.Fatal("exact material group became unknown after collection", group)
		}
	}
	if v.Num(v.At(state, "commanderProfile", "inventory", "materials", "raw", "count")) != 12 {
		t.Fatal("material collection not reflected")
	}
	e.Apply(commanderEvent("MaterialDiscarded", "2026-11-08T12:00:02Z", M{"Category": "Raw", "Name": "iron", "Count": 3}), nil)
	if v.Num(v.At(e.SnapshotForUI("commander"), "commanderProfile", "inventory", "materials", "raw", "count")) != 9 {
		t.Fatal("material expense not reflected")
	}
	e.Apply(commanderEvent("MaterialDiscarded", "2026-11-08T12:00:03Z", M{"Category": "Raw", "Name": "iron", "Count": 100}), nil)
	if v.At(e.SnapshotForUI("commander"), "commanderProfile", "inventory", "materials", "raw", "count") != nil {
		t.Fatal("uncertain negative material ledger became exact zero")
	}
}

func TestCommanderFIDSwitchClearsProjectedInventoryAndActivity(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	e.Apply(commanderEvent("Materials", "2026-11-08T12:00:00Z", M{"Raw": []M{{"Name": "iron", "Count": 8}}}), nil)
	e.Apply(commanderEvent("Commander", "2026-11-08T12:00:01Z", M{"FID": "FSECOND", "Name": "Second Synthetic Commander"}), nil)
	state := e.SnapshotForUI("commander")
	if v.At(state, "commanderProfile", "commanderFID") != "FSECOND" || v.At(state, "commanderProfile", "finance", "credits") != nil || v.At(state, "commanderProfile", "inventory", "materials", "raw", "count") != nil || len(v.Rows(v.At(state, "pilot", "recent"))) != 0 {
		t.Fatal("previous owner leaked through commander projections", state["commanderProfile"])
	}
	if v.At(state, "engineeringInventory", "shopping", "knownCategories", "raw") == true {
		t.Fatal("previous owner's engineering material baseline survived")
	}
}

func TestCommanderCAPIResultMustMatchStillActiveAuthenticatedFID(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	cmd := e.Commander()
	result := M{"status": M{"connected": true, "commanderMismatch": false, "fid": cmd["fid"]}, "data": M{"profile": M{"commander": M{"name": cmd["name"], "credits": 0, "debt": 0}, "ship": M{"id": 1, "name": "panthermkii"}}}, "metadata": M{"observedAt": M{"profile": "2026-11-08T12:00:00Z"}}}
	if !e.applyVerifiedProfileCAPI(result) || v.At(e.SnapshotForUI("commander"), "commanderProfile", "finance", "credits") == nil {
		t.Fatal("verified zero CAPI balance missing")
	}
	e.Apply(commanderEvent("Commander", "2026-11-08T12:00:01Z", M{"FID": "FSECOND", "Name": cmd["name"]}), nil)
	if e.applyVerifiedProfileCAPI(result) || v.At(e.SnapshotForUI("commander"), "commanderProfile", "finance", "credits") != nil {
		t.Fatal("late CAPI crossed FIDs with the same name")
	}
	delete(v.Obj(result["status"]), "fid")
	if e.applyVerifiedProfileCAPI(result) {
		t.Fatal("unbound CAPI status accepted")
	}
}

func TestCommanderReceivesVerifiedCAPIThroughActualEngineCommand(t *testing.T) {
	e, _, _ := accountFixture(t)
	result, err := e.Do("/api/frontier/login", nil)
	if err != nil {
		t.Fatal(err)
	}
	login, _ := url.Parse(v.Str(result["url"]))
	if code := callbackRequest(t, e, url.Values{"state": {login.Query().Get("state")}, "code": {"synthetic-code"}}, "", "/api/frontier/callback", "GET"); code != 200 {
		t.Fatal("fixture login failed", code)
	}
	if _, err = e.Do("/api/frontier/sync", M{"carrier": false}); err != nil {
		t.Fatal(err)
	}
	state := e.SnapshotForUI("commander")
	if v.Num(v.At(state, "commanderProfile", "finance", "credits")) != 987654 || v.At(state, "commanderProfile", "fleet", "currentShip", "shipId") != "7" || v.At(state, "commanderProfile", "fleet", "known") != false {
		t.Fatal("verified profile did not reach commander page", state["commanderProfile"])
	}
}
