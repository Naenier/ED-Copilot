package app

import (
	v "github.com/Naenier/ED-Copilot/internal/value"
	"path/filepath"
	"testing"
)

func TestSavedRouteAPIPersistsCompleteQuotesAndRemovesFavourite(t *testing.T) {
	engine, journalFile, dir := fixtureEngine(t, nil)
	route := M{"id": "one", "legs": []M{{"key": "gold", "name": "Золото", "quantity": 10, "buyPrice": 100, "sellPrice": 200, "buy": M{"system": "Synthetic A", "station": "Test buy", "pad": "L", "updatedAt": "2026-11-08T12:00:00Z", "supply": 100}, "sell": M{"system": "Synthetic B", "station": "Test sell", "pad": "M", "demand": 1000}}}, "netEstimate": 1000}
	result, err := engine.Do("/api/trading/saved-routes", M{"action": "save-route", "route": route})
	if err != nil {
		t.Fatal(err)
	}
	id := v.Str(result["id"])
	if id == "" {
		t.Fatal("No saved identifier")
	}
	if got := v.Rows(engine.SnapshotForUI("deals")["savedTradeRoutes"]); len(got) != 1 || v.At(v.Rows(got[0]["legs"])[0], "sell", "station") != "Test sell" {
		t.Fatal("Scoped view lost quotes", got)
	}
	engine.Close()
	anonymous, err := New(Options{DataDir: dir, JournalDir: filepath.Join(dir, "journals"), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	if rows := v.Rows(anonymous.SnapshotForUI("deals")["savedTradeRoutes"]); len(rows) != 0 {
		t.Fatal("Private route appeared without journal identity", rows)
	}
	if err := anonymous.CloseWithSave(); err != nil {
		t.Fatal(err)
	}
	restored, err := New(Options{DataDir: dir, JournalDir: filepath.Dir(journalFile), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if rows := v.Rows(restored.SnapshotForUI("deals")["savedTradeRoutes"]); len(rows) != 1 || rows[0]["id"] != id {
		t.Fatal("Saved route lost on restart", rows)
	}
	if _, err := restored.Do("/api/trading/saved-routes", M{"action": "delete-route", "id": id}); err != nil {
		t.Fatal(err)
	}
	if len(v.Rows(restored.SnapshotForUI("deals")["savedTradeRoutes"])) != 0 {
		t.Fatal("Deleted route still visible")
	}
}
