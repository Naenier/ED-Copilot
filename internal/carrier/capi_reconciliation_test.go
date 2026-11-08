package carrier

import (
	v "github.com/Naenier/ED-Copilot/internal/value"
	"testing"
	"time"
)

func TestCarrierServerLagCannotEraseDepositThatPredatesRequest(t *testing.T) {
	tr, initial, cmd := capiStockFixture()
	if !tr.ApplyCAPI(initial, cmd) {
		t.Fatal("initial stock not accepted")
	}
	at := v.Time(v.At(initial, "metadata", "observedAt", "carrier"))
	tr.Apply(M{"event": "Docked", "timestamp": v.ISO(at), "StationType": "FleetCarrier", "MarketID": tr.OwnerID}, nil)
	tr.Apply(M{"event": "CargoTransfer", "timestamp": v.ISO(at.Add(time.Minute)), "Transfers": []M{{"Type": "gold", "Count": 3, "Direction": "tocarrier"}}}, nil)
	later := v.Copy(initial)
	v.Obj(later["metadata"])["observedAt"] = M{"carrier": v.ISO(at.Add(2 * time.Minute))}
	v.Obj(later["metadata"])["requestStartedAt"] = M{"carrier": v.ISO(at.Add(2*time.Minute - time.Second))}
	if tr.ApplyCAPI(later, cmd) {
		t.Fatal("late old server payload replaced journal deposit")
	}
	if got := v.Num(v.At(tr.Snapshot(nil), "warehouse", "knownTons")); got != 20 {
		t.Fatal("immediate physical hold incorrect", got)
	}
	v.Obj(v.At(later, "data", "carrier"))["cargo"] = []M{{"commodity": "gold", "qty": 10}, {"commodity": "tritium", "qty": 10, "mission": true}}
	if !tr.ApplyCAPI(later, cmd) || v.Num(v.At(tr.Snapshot(nil), "warehouse", "knownTons")) != 20 {
		t.Fatal("confirmed stock rejected or deposit counted twice")
	}
}

func TestCarrierPendingFullWithdrawalMissingCommodityAndNetZero(t *testing.T) {
	for _, test := range []struct {
		name     string
		deltas   []float64
		incoming []M
		accept   bool
		tons     float64
	}{
		{"full withdrawal", []float64{-7}, []M{{"commodity": "gold", "qty": 7}, {"commodity": "tritium", "qty": 10}}, false, 10},
		{"missing deposited commodity", []float64{3}, []M{{"commodity": "tritium", "qty": 10}}, false, 20},
		{"net zero confirmed", []float64{3, -3}, []M{{"commodity": "gold", "qty": 7}, {"commodity": "tritium", "qty": 10}}, true, 17},
	} {
		t.Run(test.name, func(t *testing.T) {
			tr, initial, cmd := capiStockFixture()
			if !tr.ApplyCAPI(initial, cmd) {
				t.Fatal("fixture rejected")
			}
			at := v.Time(v.At(initial, "metadata", "observedAt", "carrier"))
			for _, delta := range test.deltas {
				tr.moveStock(tr.Current(), "gold", "Gold", delta, v.ISO(at.Add(time.Minute)), "transfer")
			}
			later := v.Copy(initial)
			v.Obj(later["metadata"])["observedAt"] = M{"carrier": v.ISO(at.Add(2 * time.Minute))}
			v.Obj(later["metadata"])["requestStartedAt"] = M{"carrier": v.ISO(at.Add(2*time.Minute - time.Second))}
			v.Obj(v.At(later, "data", "carrier"))["cargo"] = test.incoming
			if tr.ApplyCAPI(later, cmd) != test.accept {
				t.Fatal("unexpected reconciliation decision")
			}
			if got := v.Num(v.At(tr.Snapshot(nil), "warehouse", "knownTons")); got != test.tons {
				t.Fatal("physical stock lost or double counted", got)
			}
		})
	}
}

func TestCarrierPendingLeaseAcceptsChangedExternalStockAfterExpiry(t *testing.T) {
	tr, initial, cmd := capiStockFixture()
	if !tr.ApplyCAPI(initial, cmd) {
		t.Fatal("fixture rejected")
	}
	at := v.Time(v.At(initial, "metadata", "observedAt", "carrier"))
	tr.moveStock(tr.Current(), "gold", "Gold", 3, v.ISO(at.Add(time.Minute)), "transfer")
	late := v.Copy(initial)
	v.Obj(v.At(late, "data", "carrier"))["cargo"] = []M{{"commodity": "gold", "qty": 15}, {"commodity": "tritium", "qty": 10}}
	for _, test := range []struct {
		age  time.Duration
		want bool
		tons float64
	}{{14 * time.Minute, false, 20}, {16 * time.Minute, true, 25}} {
		v.Obj(late["metadata"])["observedAt"] = M{"carrier": v.ISO(at.Add(test.age))}
		v.Obj(late["metadata"])["requestStartedAt"] = M{"carrier": v.ISO(at.Add(test.age - time.Second))}
		if tr.ApplyCAPI(late, cmd) != test.want || v.Num(v.At(tr.Snapshot(nil), "warehouse", "knownTons")) != test.tons {
			t.Fatal("bounded pending lease did not permit absolute recovery", test, tr.StockBaseline)
		}
	}
}

func TestCarrierFractionalCAPITimeDoesNotReapplyWholeSecondJournalMovement(t *testing.T) {
	tr, initial, cmd := capiStockFixture()
	v.Obj(v.At(initial, "data", "carrier"))["cargo"] = []M{{"commodity": "gold", "qty": 20}}
	if !tr.ApplyCAPI(initial, cmd) {
		t.Fatal("fixture rejected")
	}
	at := v.Time(v.At(initial, "metadata", "observedAt", "carrier"))
	moved := at.Add(time.Second)
	tr.moveStock(tr.Current(), "gold", "Gold", 7, v.ISO(moved), "transfer")
	later := v.Copy(initial)
	v.Obj(v.At(later, "data", "carrier"))["cargo"] = []M{{"commodity": "gold", "qty": 27}}
	v.Obj(later["metadata"])["requestStartedAt"] = M{"carrier": v.ISO(moved.Add(100 * time.Millisecond))}
	v.Obj(later["metadata"])["observedAt"] = M{"carrier": v.ISO(moved.Add(500 * time.Millisecond))}
	if !tr.ApplyCAPI(later, cmd) {
		t.Fatal("confirmed API result rejected")
	}
	if got := v.Num(v.At(tr.Snapshot(nil), "warehouse", "knownTons")); got != 27 {
		t.Fatal("pre-observation whole-second movement applied twice", got)
	}
}

func TestCarrierWholeSecondOrderCannotOverlayNewerFractionalSnapshot(t *testing.T) {
	tr, initial, cmd := capiStockFixture()
	if !tr.ApplyCAPI(initial, cmd) {
		t.Fatal("fixture rejected")
	}
	at := v.Time(v.At(initial, "metadata", "observedAt", "carrier")).Add(time.Second)
	tr.Apply(M{"event": "CarrierTradeOrder", "CarrierID": tr.OwnerID, "timestamp": v.ISO(at), "Commodity": "gold", "SaleOrder": 20, "Price": 1}, nil)
	later := v.Copy(initial)
	v.Obj(v.At(later, "data", "carrier"))["cargo"] = []M{{"commodity": "gold", "qty": 27}}
	v.Obj(later["metadata"])["requestStartedAt"] = M{"carrier": v.ISO(at.Add(100 * time.Millisecond))}
	v.Obj(later["metadata"])["observedAt"] = M{"carrier": v.ISO(at.Add(500 * time.Millisecond))}
	if !tr.ApplyCAPI(later, cmd) {
		t.Fatal("confirmed API result rejected")
	}
	if got := v.Num(v.At(tr.Snapshot(nil), "warehouse", "knownTons")); got != 27 {
		t.Fatal("older order overlaid newer absolute hold", got)
	}
}

func TestCarrierRecentTransferSurvivesOriginalBaselineExpiry(t *testing.T) {
	for _, test := range []struct {
		name      string
		confirmAt time.Duration
		amount    float64
	}{{"matching API confirms early", 16 * time.Minute, 27}, {"external absolute API accepted at latest movement deadline", 29 * time.Minute, 35}} {
		t.Run(test.name, func(t *testing.T) {
			tr, initial, cmd := capiStockFixture()
			v.Obj(v.At(initial, "data", "carrier"))["cargo"] = []M{{"commodity": "gold", "qty": 20}}
			if !tr.ApplyCAPI(initial, cmd) {
				t.Fatal("fixture rejected")
			}
			at := v.Time(v.At(initial, "metadata", "observedAt", "carrier"))
			tr.moveStock(tr.Current(), "gold", "Gold", 7, v.ISO(at.Add(14*time.Minute)), "transfer")
			for _, age := range []time.Duration{15 * time.Minute, 16 * time.Minute} {
				stale := v.Copy(initial)
				v.Obj(stale["metadata"])["requestStartedAt"] = M{"carrier": v.ISO(at.Add(age - time.Second))}
				v.Obj(stale["metadata"])["observedAt"] = M{"carrier": v.ISO(at.Add(age))}
				if tr.ApplyCAPI(stale, cmd) || v.Num(v.At(tr.Snapshot(nil), "warehouse", "knownTons")) != 27 || !v.Time(tr.StockBaseline["timestamp"]).Equal(at) {
					t.Fatal("old poll erased a recent transfer or refreshed original baseline", tr.StockBaseline)
				}
			}
			fresh := v.Copy(initial)
			v.Obj(v.At(fresh, "data", "carrier"))["cargo"] = []M{{"commodity": "gold", "qty": test.amount}}
			v.Obj(fresh["metadata"])["requestStartedAt"] = M{"carrier": v.ISO(at.Add(test.confirmAt - time.Second))}
			v.Obj(fresh["metadata"])["observedAt"] = M{"carrier": v.ISO(at.Add(test.confirmAt))}
			if !tr.ApplyCAPI(fresh, cmd) || v.Num(v.At(tr.Snapshot(nil), "warehouse", "knownTons")) != test.amount {
				t.Fatal("absolute recovery re-added the old transfer or stayed blocked", tr.StockBaseline)
			}
		})
	}
}
