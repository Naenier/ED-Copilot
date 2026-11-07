package app

import (
	"bytes"
	"encoding/json"
	"github.com/Naenier/ED-Copilot/internal/profile"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testEvent(kind string, fields M) M {
	return v.Merge(M{"event": kind, "timestamp": "2026-11-07T12:00:00Z"}, fields)
}
func fixtureEngine(t *testing.T, saved M) (*Engine, string, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "journals")
	data := filepath.Join(root, "profile")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(data, 0700); err != nil {
		t.Fatal(err)
	}
	if saved != nil {
		if err := profile.Write(filepath.Join(data, "settings.json"), saved); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(dir, "Journal.2026-11-07T120000.01.log")
	writeEvents(t, file, false, []M{testEvent("Fileheader", M{"gameversion": "4.4.2", "build": "synthetic"}), testEvent("Commander", M{"Name": "Synthetic Commander", "FID": "FSYNTHETIC"}), testEvent("LoadGame", M{"Commander": "Synthetic Commander", "FID": "FSYNTHETIC", "Ship": "panthermkii", "ShipID": 1, "ShipName": "Мой корабль", "ShipIdent": "TEST-01", "Credits": 1000000, "Loan": 0, "Horizons": true, "Odyssey": true}), testEvent("Loadout", M{"Ship": "PantherMkII", "ShipID": 1, "ShipName": "Мой корабль", "ShipIdent": "TEST-01", "CargoCapacity": 100, "Modules": []M{{"Item": "hpt_mininglaser_fixed_small"}, {"Item": "int_refinery_size2_class2"}}}), testEvent("Location", M{"StarSystem": "Synthetic Origin", "SystemAddress": json.Number("18446744073709551615"), "StarPos": []float64{0, 0, 0}, "Docked": true, "StationName": "Synthetic Station", "MarketID": 42}), testEvent("Cargo", M{"Vessel": "Ship", "Inventory": []M{{"Name": "Gold", "Count": 10}, {"Name": "Steel", "Count": 30, "Stolen": 5}}}), testEvent("Scan", M{"BodyID": 1, "BodyName": "Synthetic Origin 1", "Landable": true, "SurfaceGravity": 9.80665, "ReserveLevel": "PristineResources", "Rings": []M{{"Name": "Synthetic Origin 1 A Ring", "RingClass": "eRingClass_Metallic"}}}), testEvent("SAASignalsFound", M{"BodyName": "Synthetic Origin 1 A Ring", "Signals": []M{{"Type": "Platinum", "Count": 1}}})})
	engine, err := New(Options{DataDir: data, JournalDir: dir, DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	return engine, file, data
}
func writeEvents(t *testing.T, file string, appendMode bool, events []M) {
	t.Helper()
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if appendMode {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	f, err := os.OpenFile(file, flags, 0600)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		b, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(append(b, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}
func doOK(t *testing.T, e *Engine, path string, body M) M {
	t.Helper()
	out, err := e.Do(path, body)
	if err != nil {
		t.Fatalf("%s %v: %v", path, body, err)
	}
	if !v.Bool(out["ok"]) {
		t.Fatalf("command %s failed: %v", path, out)
	}
	return out
}
func expectNumber(t *testing.T, raw any, want float64) {
	t.Helper()
	if v.Num(raw) != want {
		t.Fatalf("got %v want %g", raw, want)
	}
}
func TestNativeEngineBootstrapLegacyProfileAndShipIdentity(t *testing.T) {
	legacy := M{"settings": M{"minTier": "high", "activity": "colonization", "hideUnfocused": false}, "navigation": M{"tabs": []string{"settings", "overview", "colonization"}}, "objectives": []M{{"id": "old-goal", "title": "Saved task", "kind": "other", "status": "active"}}, "manualPrices": M{"gold": M{"price": 123, "timestamp": "2026-11-07T11:00:00Z"}}, "mining": M{"options": M{"resourceKey": "gold", "method": "laser"}}, "carrier": M{"planning": M{"reserveTritium": 200}}}
	engine, _, dir := fixtureEngine(t, legacy)
	legacyFile, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	protected := []byte("Synthetic protected data; must remain unchanged by offline migration")
	for _, name := range []string{"frontier-tokens.enc", "integration-state.bin"} {
		if err = os.WriteFile(filepath.Join(dir, name), protected, 0600); err != nil {
			t.Fatal(err)
		}
	}
	state := engine.Snapshot()
	if v.At(state, "app", "runtime") != "Go · native Windows" || v.At(state, "commander", "ship") != "Panther Clipper Mk II" {
		t.Fatal(state["commander"])
	}
	if v.At(state, "commander", "shipName") != "Мой корабль" || v.At(state, "commander", "shipIdent") != "TEST-01" {
		t.Fatal("model and custom name mixed", state["commander"])
	}
	if v.At(state, "commander", "systemAddress") != json.Number("18446744073709551615") {
		t.Fatal("ID64 rounded", state["commander"])
	}
	if v.At(state, "settings", "minTier") != "high" || v.At(state, "settings", "hideUnfocused") != nil {
		t.Fatal("settings migration failed", state["settings"])
	}
	expectNumber(t, v.At(state, "commander", "cargoTons"), 40)
	if len(v.Rows(state["targets"])) != 1 {
		t.Fatal("journal ring scan missing", state["targets"])
	}
	if len(v.Rows(state["objectives"])) != 1 {
		t.Fatal("saved goal lost")
	}
	doOK(t, engine, "/api/desktop", M{"displayMode": "borderless"})
	doOK(t, engine, "/api/settings", M{"method": "core", "overlayEnabled": false})
	engine.Close()
	after, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil || !bytes.Equal(after, legacyFile) {
		t.Fatal("legacy profile was overwritten", err)
	}
	for _, name := range []string{"frontier-tokens.enc", "integration-state.bin"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(b, protected) {
			t.Fatal("protected credentials changed", name, err)
		}
	}
	restored, err := New(Options{DataDir: dir, JournalDir: filepath.Join(filepath.Dir(dir), "journals"), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if v.At(restored.Snapshot(), "settings", "method") != "core" || v.At(restored.Snapshot(), "desktop", "displayMode") != "borderless" {
		t.Fatal("Go settings did not survive restart")
	}
}
func TestEngineLocalActionsTradePlanDestinationAndExports(t *testing.T) {
	engine, _, _ := fixtureEngine(t, nil)
	doOK(t, engine, "/api/market", M{"prices": []M{{"key": "gold", "price": 270000}}})
	state := engine.Snapshot()
	var gold M
	for _, r := range v.Rows(state["goods"]) {
		if r["key"] == "gold" {
			gold = r
		}
	}
	expectNumber(t, gold["manualPrice"], 270000)
	doOK(t, engine, "/api/market", M{"prices": []M{{"key": "gold", "price": nil}}})
	doOK(t, engine, "/api/trading/plan", M{"key": "gold", "quantity": 10, "buyPrice": 100, "sellPrice": 150, "source": "A", "destination": "B"})
	expectNumber(t, v.At(engine.Snapshot(), "tradePlan", "grossMargin"), 500)
	quote := M{"id": "market:99", "station": "Destination", "system": "Target", "updatedAt": "2026-11-07T12:00:00Z", "quotes": []M{{"key": "gold", "price": 150, "quantity": 10}}}
	doOK(t, engine, "/api/trading/destination", M{"destination": quote})
	if v.At(engine.Snapshot(), "trading", "destination", "station") != "Destination" {
		t.Fatal("selected destination invisible")
	}
	doOK(t, engine, "/api/objectives", M{"action": "add", "title": "Synthetic objective", "kind": "trade", "system": "Test"})
	goals := v.Rows(engine.Snapshot()["objectives"])
	if len(goals) != 1 {
		t.Fatal(goals)
	}
	id := goals[0]["id"]
	for _, action := range []string{"complete", "reopen"} {
		doOK(t, engine, "/api/objectives", M{"action": action, "id": id})
	}
	doOK(t, engine, "/api/objectives", M{"action": "update", "id": id, "title": "Updated objective"})
	if v.Rows(engine.Snapshot()["objectives"])[0]["title"] != "Updated objective" {
		t.Fatal("goal update lost")
	}
	target := v.Rows(engine.Snapshot()["targets"])[0]
	doOK(t, engine, "/api/bookmarks", M{"action": "add", "targetId": target["id"]})
	doOK(t, engine, "/api/bookmarks", M{"action": "add", "targetId": target["id"]})
	bookmarks := v.Rows(engine.Snapshot()["bookmarks"])
	if len(bookmarks) != 1 {
		t.Fatal("duplicate bookmark")
	}
	doOK(t, engine, "/api/bookmarks", M{"action": "remove", "id": bookmarks[0]["id"]})
	doOK(t, engine, "/api/navigation", M{"tabs": []string{"colonization", "statistics"}})
	tabs := v.Arr(v.At(engine.Snapshot(), "navigation", "tabs"))
	if len(tabs) < 4 {
		t.Fatal("required tabs missing", tabs)
	}
	preview, err := engine.Do("/api/theme/preview", M{"settings": M{"mode": "preset", "preset": "orange"}})
	if err != nil || v.At(preview, "theme", "settings", "preset") != "orange" {
		t.Fatal(preview, err)
	}
	doOK(t, engine, "/api/theme", M{"settings": M{"mode": "preset", "preset": "orange"}})
	for _, kind := range []string{"statistics", "trading", "engineering"} {
		csv, err := engine.ExportCSV(kind)
		if err != nil || !strings.HasPrefix(csv, "\ufeff") {
			t.Fatal(kind, csv, err)
		}
	}
	if b, err := engine.ExportPlan("objectives"); err != nil || !bytes.Contains(b, []byte("Updated objective")) {
		t.Fatal(string(b), err)
	}
	doOK(t, engine, "/api/objectives", M{"action": "remove", "id": id})
	for _, tc := range []struct {
		path string
		body M
	}{{"/api/settings", M{"activity": "invalid"}}, {"/api/desktop", M{"displayMode": "invalid"}}, {"/api/market", M{"prices": []M{{"key": "gold", "price": -1}}}}, {"/api/trading/plan", M{"key": "gold", "quantity": 0, "buyPrice": 0, "sellPrice": 0}}, {"/api/frontier/login", nil}, {"/api/search", M{"system": "Sol"}}, {"/api/trading/routes", nil}, {"/api/colonization/search", nil}, {"/api/carrier/route", nil}, {"/api/integrations/sync", nil}, {"/unknown", nil}} {
		if _, err := engine.Do(tc.path, tc.body); err == nil {
			t.Fatal("invalid/disabled command accepted", tc.path)
		}
	}
}
func createEngineColony(t *testing.T, engine *Engine, name string, capacity int, market int) M {
	t.Helper()
	doOK(t, engine, "/api/colonization", M{"action": "create-project", "targetSystem": name, "status": "building"})
	p := v.Obj(v.At(engine.Snapshot(), "colonization", "activeProject"))
	doOK(t, engine, "/api/colonization", M{"action": "configure", "projectId": p["id"], "cargoCapacity": capacity})
	for i, typ := range []string{"coriolis", "hub-refinery"} {
		doOK(t, engine, "/api/colonization", M{"action": "add-building", "projectId": p["id"], "typeId": typ, "name": name + " stage " + typ})
		p = v.Obj(v.At(engine.Snapshot(), "colonization", "activeProject"))
		buildings := v.Rows(p["buildings"])
		building := buildings[len(buildings)-1]
		commodity := "steel"
		if i == 1 {
			commodity = "aluminium"
		}
		doOK(t, engine, "/colonization/resource-row", M{"projectId": p["id"], "buildingId": building["id"], "commodity": commodity, "required": capacity, "sourceNote": "Synthetic requirement"})
		if i == 0 {
			doOK(t, engine, "/api/colonization", M{"action": "link-depot", "projectId": p["id"], "buildingId": building["id"], "marketId": market})
		}
	}
	return v.Obj(v.At(engine.Snapshot(), "colonization", "activeProject"))
}
func TestEngineMultipleColonyQueuesAndJournalSupplies(t *testing.T) {
	engine, file, dir := fixtureEngine(t, nil)
	alpha := createEngineColony(t, engine, "Alpha", 100, 101)
	beta := createEngineColony(t, engine, "Beta", 200, 202)
	ab := v.Rows(alpha["buildings"])
	bb := v.Rows(beta["buildings"])
	doOK(t, engine, "/api/colonization", M{"action": "reorder-supply", "projectId": alpha["id"], "ids": []any{ab[1]["id"], ab[0]["id"]}})
	doOK(t, engine, "/api/colonization", M{"action": "move-building", "projectId": beta["id"], "id": bb[1]["id"], "direction": "up"})
	doOK(t, engine, "/colonization/manual-delivery", M{"projectId": alpha["id"], "buildingId": ab[1]["id"], "commodity": "aluminium", "count": 20})
	writeEvents(t, file, true, []M{testEvent("FSDJump", M{"StarSystem": "Alpha", "SystemAddress": 1001, "StarPos": []float64{1, 0, 0}, "timestamp": "2026-11-07T12:01:00Z"}), testEvent("ColonisationConstructionDepot", M{"MarketID": 101, "ResourcesRequired": []M{{"Name": "$steel_name;", "RequiredAmount": 100, "ProvidedAmount": 10}}, "timestamp": "2026-11-07T12:01:01Z"}), testEvent("ColonisationContribution", M{"MarketID": 101, "Contributions": []M{{"Name": "$steel_name;", "Amount": 10}}, "timestamp": "2026-11-07T12:01:02Z"})})
	engine.Reader.Poll()
	state := engine.Snapshot()
	find := func(state M, id any) M {
		for _, p := range v.Rows(v.At(state, "colonization", "projects")) {
			if p["id"] == id {
				return p
			}
		}
		return nil
	}
	a, b := find(state, alpha["id"]), find(state, beta["id"])
	expectNumber(t, a["totalMissing"], 160)
	expectNumber(t, b["totalMissing"], 400)
	if len(v.Rows(b["journalDeliveries"])) != 0 {
		t.Fatal("contribution leaked to Beta")
	}
	doOK(t, engine, "/api/colonization", M{"action": "select-project", "id": alpha["id"]})
	state = engine.Snapshot()
	if v.At(state, "colonization", "activeProject", "nextTask", "buildingId") != ab[1]["id"] {
		t.Fatal("supply priority not used by overlay")
	}
	expectNumber(t, v.At(state, "colonization", "cargoCapacity"), 100)
	if v.At(state, "commander", "cargoForSync") != nil {
		t.Fatal("pre-delivery authoritative cargo snapshot retained")
	}
	cargo := v.Rows(v.At(state, "commander", "cargo"))
	for _, r := range cargo {
		if r["key"] == "steel" {
			expectNumber(t, r["count"], 20)
		}
	}
	if csv, err := engine.ExportCSV("colonization"); err != nil || !strings.Contains(csv, "Alpha") {
		t.Fatal(csv, err)
	}
	if b, err := engine.ExportPlan("colonization"); err != nil || !bytes.Contains(b, []byte("Alpha")) {
		t.Fatal(string(b), err)
	}
	engine.Reader.Rescan()
	engine.Reader.Poll()
	expectNumber(t, find(engine.Snapshot(), alpha["id"])["totalMissing"], 160)
	engine.Close()
	restored, err := New(Options{DataDir: dir, JournalDir: filepath.Dir(file), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	state = restored.Snapshot()
	if v.At(state, "colonization", "selectedProjectId") != alpha["id"] || len(v.Rows(v.At(state, "colonization", "projects"))) != 2 {
		t.Fatal("colony selection/restart isolation lost")
	}
	expectNumber(t, find(state, alpha["id"])["totalMissing"], 160)
}
func TestEngineConcurrentJournalPollingAndSnapshots(t *testing.T) {
	engine, file, _ := fixtureEngine(t, nil)
	events := []M{}
	for i := 0; i < 150; i++ {
		events = append(events, testEvent("ProspectedAsteroid", M{"Materials": []M{{"Name": "Platinum", "Proportion": 30}}, "Remaining": 100}))
		events = append(events, testEvent("MiningRefined", M{"Type": "Platinum"}))
	}
	writeEvents(t, file, true, events)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); engine.Reader.Poll() }()
	go func() {
		defer wg.Done()
		for i := 0; i < 40; i++ {
			engine.Snapshot()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			if err := engine.Save(); err != nil {
				t.Error(err)
			}
		}
	}()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("poll/snapshot/save lock-order deadlock")
	}
	expectNumber(t, v.At(engine.Snapshot(), "stats", "total", "refinedTons"), 150)
}
