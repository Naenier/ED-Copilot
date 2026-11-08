package colonization

import (
	"github.com/Naenier/ED-Copilot/internal/value"
	"testing"
	"time"
)

func TestCarrierLateServerSnapshotCannotEraseJournalDeposit(t *testing.T) {
	p, _, ctx := stockFixture(t)
	p.ApplyCAPI(stockResult(capiStock("42", 30), nil, testNow), value.Obj(ctx["commander"]))
	moved := testNow.Add(time.Minute)
	p.now = func() time.Time { return moved }
	ctx["carrierId"] = "42" // Real CargoTransfer records do not carry an ID.
	p.Apply(M{"event": "CargoTransfer", "timestamp": value.ISO(moved), "Transfers": []M{{"Type": "steel", "Count": 70, "Direction": "tocarrier"}}}, ctx)
	if got := num(steelSupply(t, p, ctx)["carrierAvailable"]); got != 100 {
		t.Fatal("deposit was not applied immediately", got)
	}
	staleAt := testNow.Add(2 * time.Minute)
	p.now = func() time.Time { return staleAt }
	old := stockResult(capiStock("42", 30), nil, staleAt)
	value.Obj(old["metadata"])["requestStartedAt"] = M{"carrier": value.ISO(staleAt.Add(-time.Second))}
	p.ApplyCAPI(old, value.Obj(ctx["commander"]))
	check := func(planner *Planner) {
		t.Helper()
		warehouse := rows(planner.Snapshot(ctx)["carrierWarehouses"])[0]
		if num(steelSupply(t, planner, ctx)["carrierAvailable"]) != 100 || value.Time(warehouse["receivedAt"]) != testNow {
			t.Fatal("server receive time rolled back or refreshed journal projection", warehouse)
		}
	}
	check(p)
	restored := New(p.ExportSettings(), WithClock(p.now))
	restored.ApplyCAPI(old, value.Obj(ctx["commander"]))
	check(restored)
	freshAt := testNow.Add(3 * time.Minute)
	restored.now = func() time.Time { return freshAt }
	confirmed := stockResult(capiStock("42", 100), nil, freshAt)
	value.Obj(confirmed["metadata"])["requestStartedAt"] = M{"carrier": value.ISO(freshAt.Add(-time.Second))}
	restored.ApplyCAPI(confirmed, value.Obj(ctx["commander"]))
	warehouse := rows(restored.Snapshot(ctx)["carrierWarehouses"])[0]
	if !value.Time(warehouse["receivedAt"]).Equal(freshAt) || num(steelSupply(t, restored, ctx)["carrierAvailable"]) != 100 || len(value.Obj(value.At(restored.ExportSettings(), "carrierInventoryCache", "warehouses", "fleet", "pendingMovements"))) != 0 {
		t.Fatal("matching API snapshot did not confirm, or deposit was counted twice", warehouse)
	}
}

func TestCarrierPendingMovementsKeepWithdrawalAndSameSecondTradesDistinct(t *testing.T) {
	p, _, ctx := stockFixture(t)
	p.ApplyCAPI(stockResult(capiStock("42", 50), nil, testNow), value.Obj(ctx["commander"]))
	moved := testNow.Add(time.Minute)
	p.now = func() time.Time { return moved }
	for _, event := range []M{
		{"event": "CargoTransfer", "Transfers": []M{{"Type": "steel", "Count": 10, "Direction": "toship"}}},
		{"event": "MarketSell", "Type": "steel", "Count": 5},
		{"event": "MarketBuy", "Type": "steel", "Count": 3},
	} {
		event["timestamp"], event["MarketID"] = value.ISO(moved), "42"
		p.Apply(event, ctx)
	}
	later := moved.Add(time.Second)
	p.now = func() time.Time { return later }
	p.ApplyCAPI(stockResult(capiStock("42", 50), nil, later), value.Obj(ctx["commander"]))
	if num(steelSupply(t, p, ctx)["carrierAvailable"]) != 42 {
		t.Fatal("same-second movement order lost", steelSupply(t, p, ctx))
	}
	p.now = func() time.Time { return testNow.Add(16 * time.Minute) }
	if r := steelSupply(t, p, ctx); r["carrierAvailable"] != nil || r["toBuy"] != nil {
		t.Fatal("unconfirmed API response prolonged old stock freshness", r)
	}
}

func TestCarrierPendingProjectionSurvivesLegacyCacheAndMalformedResponse(t *testing.T) {
	p, _, ctx := stockFixture(t)
	p.ApplyCAPI(stockResult(capiStock("42", 30), nil, testNow), value.Obj(ctx["commander"]))
	moved := testNow.Add(time.Minute)
	p.now = func() time.Time { return moved }
	p.Apply(M{"event": "CargoTransfer", "timestamp": value.ISO(moved), "CarrierID": "42", "Transfers": []M{{"Type": "steel", "Count": 5, "Direction": "tocarrier"}}}, M{"file": "synthetic.log", "offset": 42})
	legacy := p.ExportSettings()
	delete(value.Obj(value.At(legacy, "carrierInventoryCache", "warehouses", "fleet")), "pendingMovements")
	restored := New(legacy, WithClock(func() time.Time { return moved.Add(time.Minute) }))
	bad := capiStock("42", 30)
	delete(bad, "cargo")
	restored.ApplyCAPI(stockResult(bad, nil, moved.Add(time.Minute)), value.Obj(ctx["commander"]))
	if r := steelSupply(t, restored, ctx); num(r["carrierAvailable"]) != 35 {
		t.Fatal("legacy corrected quantities were replaced by incomplete response", r)
	}
	warehouse := rows(restored.Snapshot(ctx)["carrierWarehouses"])[0]
	if !value.Bool(warehouse["awaitingConfirmation"]) || warehouse["reason"] != "Склад скорректирован по журналу. Frontier пока не подтвердил последние перемещения." {
		t.Fatal("projection provenance missing", warehouse)
	}
}

func TestCarrierWithdrawalBeyondKnownStockInvalidatesBaseline(t *testing.T) {
	p, _, ctx := stockFixture(t)
	p.ApplyCAPI(stockResult(capiStock("42", 3), nil, testNow), value.Obj(ctx["commander"]))
	moved := testNow.Add(time.Minute)
	p.now = func() time.Time { return moved }
	p.Apply(M{"event": "CargoTransfer", "timestamp": value.ISO(moved), "CarrierID": "42", "Transfers": []M{{"Type": "steel", "Count": 5, "Direction": "toship"}}}, ctx)
	if r := steelSupply(t, p, ctx); r["carrierAvailable"] != nil || r["toBuy"] != nil {
		t.Fatal("inconsistent baseline became confirmed zero", r)
	}
}

func TestCarrierNetZeroJournalMovementsCanBeConfirmed(t *testing.T) {
	p, _, ctx := stockFixture(t)
	p.ApplyCAPI(stockResult(capiStock("42", 30), nil, testNow), value.Obj(ctx["commander"]))
	moved := testNow.Add(time.Minute)
	p.now = func() time.Time { return moved }
	for _, direction := range []string{"tocarrier", "toship"} {
		p.Apply(M{"event": "CargoTransfer", "timestamp": value.ISO(moved), "CarrierID": "42", "Transfers": []M{{"Type": "steel", "Count": 5, "Direction": direction}}}, ctx)
	}
	later := moved.Add(time.Second)
	p.now = func() time.Time { return later }
	p.ApplyCAPI(stockResult(capiStock("42", 30), nil, later), value.Obj(ctx["commander"]))
	warehouse := rows(p.Snapshot(ctx)["carrierWarehouses"])[0]
	if value.Bool(warehouse["awaitingConfirmation"]) || !value.Time(warehouse["receivedAt"]).Equal(later) || num(steelSupply(t, p, ctx)["carrierAvailable"]) != 30 {
		t.Fatal("matching net-zero result did not confirm cleanly", warehouse)
	}
}

func TestCarrierPendingLeaseExpiresAndAcceptsExternalAbsoluteStock(t *testing.T) {
	p, _, ctx := stockFixture(t)
	p.ApplyCAPI(stockResult(capiStock("42", 30), nil, testNow), value.Obj(ctx["commander"]))
	moved := testNow.Add(time.Minute)
	p.now = func() time.Time { return moved }
	p.Apply(M{"event": "CargoTransfer", "timestamp": value.ISO(moved), "CarrierID": "42", "Transfers": []M{{"Type": "steel", "Count": 70, "Direction": "tocarrier"}}}, ctx)
	// A different commander may have delivered ten more tonnes. Defend the
	// local projection only within the bounded lease after the last movement.
	beforeExpiry := testNow.Add(14 * time.Minute)
	p.now = func() time.Time { return beforeExpiry }
	p.ApplyCAPI(stockResult(capiStock("42", 110), nil, beforeExpiry), value.Obj(ctx["commander"]))
	if r := steelSupply(t, p, ctx); num(r["carrierAvailable"]) != 100 {
		t.Fatal("pending projection overwritten before lease expiry", r)
	}
	afterExpiry := testNow.Add(16 * time.Minute)
	p.now = func() time.Time { return afterExpiry }
	if r := steelSupply(t, p, ctx); r["carrierAvailable"] != nil {
		t.Fatal("rejected response extended original freshness", r)
	}
	fresh := stockResult(capiStock("42", 110), nil, afterExpiry)
	value.Obj(fresh["metadata"])["requestStartedAt"] = M{"carrier": value.ISO(afterExpiry.Add(-time.Second))}
	p.ApplyCAPI(fresh, value.Obj(ctx["commander"]))
	warehouse := rows(p.Snapshot(ctx)["carrierWarehouses"])[0]
	if r := steelSupply(t, p, ctx); num(r["carrierAvailable"]) != 110 || value.Bool(warehouse["awaitingConfirmation"]) || !value.Time(warehouse["receivedAt"]).Equal(afterExpiry) {
		t.Fatal("external absolute stock did not recover after bounded lease, or delta added twice", r, warehouse)
	}
	if len(value.Obj(value.At(p.ExportSettings(), "carrierInventoryCache", "warehouses", "fleet", "pendingMovements"))) != 0 {
		t.Fatal("old pending movements survived fresh absolute observation")
	}
}

func TestCarrierLateTransferLeaseProtectsPollBoundaryWithoutRefreshingBaseline(t *testing.T) {
	for _, test := range []struct {
		name      string
		confirmAt time.Duration
		amount    float64
	}{{"matching observation confirms before guard expiry", 16 * time.Minute, 100}, {"external absolute observation recovers at guard expiry", 29 * time.Minute, 110}} {
		t.Run(test.name, func(t *testing.T) {
			p, _, ctx := stockFixture(t)
			p.ApplyCAPI(stockResult(capiStock("42", 30), nil, testNow), value.Obj(ctx["commander"]))
			moved := testNow.Add(14 * time.Minute)
			p.now = func() time.Time { return moved }
			p.Apply(M{"event": "CargoTransfer", "timestamp": value.ISO(moved), "CarrierID": "42", "Transfers": []M{{"Type": "steel", "Count": 70, "Direction": "tocarrier"}}}, ctx)
			for _, age := range []time.Duration{15 * time.Minute, 16 * time.Minute} {
				at := testNow.Add(age)
				p.now = func() time.Time { return at }
				stale := stockResult(capiStock("42", 30), nil, at)
				value.Obj(stale["metadata"])["requestStartedAt"] = M{"carrier": value.ISO(at.Add(-time.Second))}
				p.ApplyCAPI(stale, value.Obj(ctx["commander"]))
				cached := value.Obj(value.At(p.ExportSettings(), "carrierInventoryCache", "warehouses", "fleet"))
				if num(value.At(cached, "items", "steel")) != 100 || !value.Time(cached["receivedAt"]).Equal(testNow) {
					t.Fatal("poll near original expiry erased recent transfer or refreshed old baseline", cached)
				}
			}
			warehouse := rows(p.Snapshot(ctx)["carrierWarehouses"])[0]
			if value.Bool(warehouse["known"]) || steelSupply(t, p, ctx)["carrierAvailable"] != nil {
				t.Fatal("guard extended warehouse freshness", warehouse)
			}
			at := testNow.Add(test.confirmAt)
			p.now = func() time.Time { return at }
			fresh := stockResult(capiStock("42", test.amount), nil, at)
			value.Obj(fresh["metadata"])["requestStartedAt"] = M{"carrier": value.ISO(at.Add(-time.Second))}
			p.ApplyCAPI(fresh, value.Obj(ctx["commander"]))
			warehouse = rows(p.Snapshot(ctx)["carrierWarehouses"])[0]
			if !value.Bool(warehouse["known"]) || value.Bool(warehouse["awaitingConfirmation"]) || num(steelSupply(t, p, ctx)["carrierAvailable"]) != test.amount || !value.Time(warehouse["receivedAt"]).Equal(at) {
				t.Fatal("absolute confirmation did not recover cleanly", warehouse)
			}
		})
	}
}
