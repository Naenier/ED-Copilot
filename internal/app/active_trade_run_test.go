package app

import (
	"path/filepath"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestActiveTradeRunSelectsAuthoritativeQuoteAndReplaysProgress(t *testing.T) {
	engine, file, data := fixtureEngine(t, nil)
	owner := engine.planWorkspace.key
	a := M{"system": "Synthetic Origin", "station": "Synthetic Station", "marketId": "42"}
	b := M{"system": "Trade Destination", "station": "Trade Port", "marketId": "45"}
	route := M{"id": "trade-run-test", "legs": []M{
		{"buy": a, "sell": b, "key": "indite", "name": "Индит", "quantity": 10, "buyPrice": 10, "sellPrice": 30},
		{"buy": b, "sell": a, "key": "bauxite", "name": "Боксит", "quantity": 10, "buyPrice": 5, "sellPrice": 15},
	}}
	engine.mu.Lock()
	engine.network["tradeRoutes"] = M{"results": []M{route}}
	engine.mu.Unlock()
	body := M{"action": "start", "routeId": route["id"], "expectedPlanWorkspace": owner}
	if _, err := engine.Do("/api/trading/active-route", v.Merge(v.Copy(body), M{"expectedPlanWorkspace": "someone-else"})); err == nil {
		t.Fatal("activation for the wrong owner succeeded")
	}
	if _, err := engine.Do("/api/trading/active-route", v.Merge(v.Copy(body), M{"routeId": "missing", "route": route})); err == nil {
		t.Fatal("an unlisted renderer-supplied route was accepted")
	}
	start := time.Now().UTC().Truncate(time.Second)
	if _, err := engine.Do("/api/trading/active-route", body); err != nil {
		t.Fatal(err)
	}
	if copy := engine.PendingClipboard(); copy["kind"] != "trade-route" || copy["text"] != "Trade Destination" {
		t.Fatalf("incorrect initial clipboard request: %v", copy)
	}
	engine.Apply(M{"event": "Undocked", "timestamp": v.ISO(start.Add(2 * time.Second)), "StationName": "Synthetic Station", "MarketID": 42}, nil)
	engine.Apply(M{"event": "FSDJump", "timestamp": v.ISO(start.Add(3 * time.Second)), "StarSystem": "Trade Destination", "SystemAddress": "999"}, nil)
	if copy := engine.PendingClipboard(); copy["text"] != "Synthetic Origin" {
		t.Fatalf("incorrect reverse clipboard request: %v", copy)
	}
	view := engine.SnapshotForUI("deals")
	if v.At(view, "tradeRun", "target", "station") != "Trade Port" || v.At(view, "tradeRun", "profitPerHour") != nil {
		t.Fatalf("route advanced before docking: %v", view["tradeRun"])
	}
	engine.Apply(M{"event": "Docked", "timestamp": v.ISO(start.Add(15 * time.Second)), "StarSystem": "Trade Destination",
		"SystemAddress": "999", "StationName": "Trade Port", "MarketID": 45}, nil)
	if view = engine.SnapshotForUI("deals"); v.At(view, "tradeRun", "atStation", "sell", "key") != "indite" ||
		v.At(view, "tradeRun", "atStation", "buy", "key") != "bauxite" {
		t.Fatalf("missing destination market actions: %v", view["tradeRun"])
	}
	engine.Close()
	writeEvents(t, file, true, []M{
		{"event": "Undocked", "timestamp": v.ISO(start.Add(18 * time.Second)), "StationName": "Trade Port", "MarketID": 45},
		{"event": "FSDJump", "timestamp": v.ISO(start.Add(53 * time.Second)), "StarSystem": "Synthetic Origin", "SystemAddress": "18446744073709551615"},
		{"event": "Docked", "timestamp": v.ISO(start.Add(62 * time.Second)), "StarSystem": "Synthetic Origin", "StationName": "Synthetic Station", "MarketID": 42},
	})
	reopened, err := New(Options{DataDir: data, JournalDir: filepath.Dir(file), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	view = reopened.SnapshotForUI("deals")
	if v.Num(v.At(view, "tradeRun", "completedRounds")) != 1 || v.Num(v.At(view, "tradeRun", "lastRoundSeconds")) != 60 ||
		v.Num(v.At(view, "tradeRun", "profitPerHour")) != 18000 || reopened.PendingClipboard() != nil {
		t.Fatalf("previously persisted route was not resumed silently: %v", view["tradeRun"])
	}
	if _, err := reopened.Do("/api/trading/active-route", M{"action": "stop", "expectedPlanWorkspace": owner}); err != nil {
		t.Fatal(err)
	}
	if len(v.Obj(reopened.SnapshotForUI("deals")["tradeRun"])) != 0 || reopened.PendingClipboard() != nil {
		t.Fatal("stopped route remained active")
	}
}
