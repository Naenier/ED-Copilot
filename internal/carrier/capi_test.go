package carrier

import (
	v "github.com/Naenier/ED-Copilot/internal/value"
	"testing"
	"time"
)

func capiStockFixture() (*Tracker, M, M) {
	at := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	t := New(nil)
	t.Apply(M{"event": "CarrierStats", "timestamp": v.ISO(at.Add(-time.Hour)), "CarrierID": "90071992547409993", "Callsign": "SYN-001", "Name": "Synthetic"}, nil)
	result := M{"status": M{"connected": true, "fid": "F1"}, "data": M{"carrier": M{"market": M{"id": "90071992547409993"}, "name": M{"callsign": "SYN-001"}, "cargo": []M{{"commodity": "gold", "qty": 5}, {"commodity": "gold", "qty": 2, "stolen": true}, {"commodity": "tritium", "qty": 10, "mission": true}}, "orders": M{"commodities": M{"sales": []M{{"name": "gold", "stock": 1000}}}}}}, "metadata": M{"source": "Frontier CAPI", "observedAt": M{"carrier": v.ISO(at)}, "requestStartedAt": M{"carrier": v.ISO(at.Add(-time.Second))}}}
	return t, result, M{"fid": "F1"}
}

func TestVerifiedCarrierHoldRefreshReplacesOrdersAndBaselineWithoutReplay(t *testing.T) {
	tracker, result, cmd := capiStockFixture()
	if err := tracker.SetStockBaseline(M{"items": []M{{"key": "gold", "count": 100}}, "timestamp": "2026-11-08T20:00:00Z", "complete": true}); err != nil {
		t.Fatal(err)
	}
	if !tracker.ApplyCAPI(result, cmd) {
		t.Fatal("verified hold not imported")
	}
	state := tracker.Snapshot(nil)
	warehouse := v.Obj(state["warehouse"])
	if !v.Bool(warehouse["complete"]) || v.Num(warehouse["knownTons"]) != 17 || warehouse["sourceLabel"] != "Снимок Frontier CAPI и последующие записанные операции" {
		t.Fatal("physical cargo/order/baseline confused", warehouse)
	}
	tracker.moveStock(tracker.Current(), "gold", "Gold", -3, "2026-11-09T00:00:01Z", "toship")
	if v.Num(v.At(tracker.Snapshot(nil), "warehouse", "knownTons")) != 14 {
		t.Fatal("post-observation transfer not applied")
	}
	if tracker.ApplyCAPI(result, cmd) {
		t.Fatal("same snapshot reapplied after transfer")
	}
	restored := New(tracker.ExportSettings())
	restored.Apply(M{"event": "CarrierStats", "timestamp": "2026-11-08T23:00:00Z", "CarrierID": "90071992547409993", "Callsign": "SYN-001"}, nil)
	if v.Num(v.At(restored.Snapshot(nil), "warehouse", "knownTons")) != 17 {
		t.Fatal("confirmed baseline did not persist")
	}
}

func TestCarrierCAPIRejectsOldOrUnverifiedStockAndKeepsUnknownDistinctFromEmpty(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Tracker, M, M)
	}{
		{"other FID", func(_ *Tracker, r, c M) { c["fid"] = "F2" }},
		{"mismatch", func(_ *Tracker, r, c M) { v.Obj(r["status"])["commanderMismatch"] = true }},
		{"other carrier", func(_ *Tracker, r, c M) { v.Obj(v.At(r, "data", "carrier", "market"))["id"] = "2" }},
		{"other callsign", func(_ *Tracker, r, c M) { v.Obj(v.At(r, "data", "carrier", "name"))["callsign"] = "OTHER" }},
		{"missing cargo", func(_ *Tracker, r, c M) { delete(v.Obj(v.At(r, "data", "carrier")), "cargo") }},
		{"invalid quantity", func(_ *Tracker, r, c M) {
			v.Obj(v.At(r, "data", "carrier"))["cargo"] = []M{{"commodity": "gold", "qty": -1}}
		}},
		{"newer manual edit", func(tr *Tracker, r, c M) {
			_ = tr.SetStockBaseline(M{"items": []M{{"key": "gold", "count": 3}}, "timestamp": "2026-11-08T23:59:59.500Z"})
		}},
		{"movement after request", func(tr *Tracker, r, c M) {
			tr.moveStock(tr.Current(), "gold", "Gold", -1, "2026-11-08T23:59:59.500Z", "toship")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tr, r, c := capiStockFixture()
			test.change(tr, r, c)
			before := v.Copy(tr.StockBaseline)
			if tr.ApplyCAPI(r, c) {
				t.Fatal("invalid observation accepted")
			}
			if len(before) != len(tr.StockBaseline) {
				t.Fatal("old stock erased")
			}
		})
	}
	tr, r, c := capiStockFixture()
	v.Obj(v.At(r, "data", "carrier"))["cargo"] = []M{}
	if !tr.ApplyCAPI(r, c) || !v.Bool(v.At(tr.Snapshot(nil), "warehouse", "complete")) || v.Num(v.At(tr.Snapshot(nil), "warehouse", "knownTons")) != 0 {
		t.Fatal("confirmed empty stock not represented")
	}
}
