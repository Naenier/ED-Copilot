package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestGalacticAveragePricesAreSeparatePersistentAndNeverReplacePositiveWithZero(t *testing.T) {
	e, file, dir := fixtureEngine(t, M{"manualPrices": M{"steel": M{"price": 999999}}})
	market := func(stamp string, mean any) {
		e.Apply(M{"event": "Market", "timestamp": stamp, "Items": []M{{"Name": "$steel_name;", "SellPrice": 7777, "BuyPrice": 8888, "MeanPrice": mean}}}, M{"side": true})
	}
	market("2026-11-07T13:00:00Z", 4179)
	for _, tc := range []struct {
		at    string
		price any
	}{{"2026-11-07T12:00:00Z", 200}, {"2026-11-07T14:00:00Z", 0}, {"2026-11-07T14:01:00Z", nil}, {"invalid", 500}, {"2026-11-07T14:03:00Z", -1}, {"2026-11-07T14:04:00Z", 1e12}} {
		market(tc.at, tc.price)
	}
	if v.Num(v.At(e.galacticPrices, "steel", "price")) != 4179 || v.At(e.galacticPrices, "steel", "observedAt") != "2026-11-07T13:00:00Z" {
		t.Fatal(e.galacticPrices)
	}
	if err := e.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(Options{DataDir: dir, JournalDir: filepath.Dir(file), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if v.Num(v.At(reopened.galacticPrices, "steel", "price")) != 4179 {
		t.Fatal("lost between sessions", reopened.galacticPrices)
	}
	reopened.Reader.Rescan()
	reopened.Reader.Poll()
	if v.Num(v.At(reopened.galacticPrices, "steel", "price")) != 4179 {
		t.Fatal("replay erased average")
	}
}

func TestMarketSidecarSuppliesColonyCostsWithoutAddingCostsToHUD(t *testing.T) {
	e, file, _ := fixtureEngine(t, nil)
	sidecar := M{"event": "Market", "timestamp": "2026-11-07T13:00:00Z", "MarketID": 42, "StarSystem": "Synthetic Origin", "StationName": "Synthetic Station", "Items": []M{{"Name": "$steel_name;", "SellPrice": 15, "MeanPrice": 4179}}}
	raw, _ := json.Marshal(sidecar)
	if err := os.WriteFile(filepath.Join(filepath.Dir(file), "Market.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	writeEvents(t, file, true, []M{testEvent("Market", M{"MarketID": 42})})
	e.Reader.Poll()
	doOK(t, e, "/api/colonization", M{"action": "create-project", "targetSystem": "Synthetic Budget"})
	project := v.Obj(v.At(e.Snapshot(), "colonization", "activeProject"))
	doOK(t, e, "/api/colonization", M{"action": "add-building", "projectId": project["id"], "typeId": "coriolis"})
	project = v.Obj(v.At(e.Snapshot(), "colonization", "activeProject"))
	building := v.Rows(project["buildings"])[0]
	doOK(t, e, "/api/colonization", M{"action": "set-resource-plan", "projectId": project["id"], "buildingId": building["id"], "resources": []M{{"commodity": "steel", "required": 100}}, "sourceNote": "Synthetic budget"})
	state := e.SnapshotForUI("colonization")
	if v.Num(v.At(state, "colonization", "activeProject", "costSummary", "required", "value")) != 417900 {
		t.Fatal(v.At(state, "colonization", "activeProject", "costSummary"))
	}
	doOK(t, e, "/api/settings", M{"activity": "colonization"})
	hud := e.SnapshotForHUD()
	if v.At(hud, "colonization", "activeProject", "costSummary") != nil || v.Rows(v.At(hud, "colonization", "activeProject", "supplies"))[0]["galacticAveragePrice"] != nil {
		t.Fatal("desktop costs added to HUD")
	}
}

func TestGalacticAveragesRejectBetaLegacyAndUntrustedSavedPrices(t *testing.T) {
	e, _, _ := fixtureEngine(t, M{"galacticPrices": M{"steel": M{"price": 99999, "observedAt": "2026-11-07T13:00:00Z", "source": "manual"}, "gold": M{"price": 1, "observedAt": "invalid", "source": "Market.json"}}})
	if len(e.galacticPrices) != 0 {
		t.Fatal("restored invalid provenance", e.galacticPrices)
	}
	for _, version := range []string{"3.8.0", "4.4.2 Beta"} {
		e.Apply(testEvent("Fileheader", M{"gameversion": version}), M{})
		e.Apply(M{"event": "Market", "timestamp": "2026-11-07T15:00:00Z", "Items": []M{{"Name": "steel", "MeanPrice": 99999}}}, M{"side": true})
	}
	if len(e.galacticPrices) != 0 {
		t.Fatal("test/legacy galaxy contaminated live reference", e.galacticPrices)
	}
	e.Apply(testEvent("Fileheader", M{"gameversion": "4.4.2"}), M{})
	e.Apply(M{"event": "Market", "timestamp": "2026-11-07T16:00:00Z", "Source": "Frontier CAPI", "Items": []M{{"Name": "steel", "MeanPrice": 4179}}}, M{"side": true})
	if v.Num(v.At(e.galacticPrices, "steel", "price")) != 4179 || v.At(e.galacticPrices, "steel", "source") != "Frontier CAPI" {
		t.Fatal(e.galacticPrices)
	}
}
