package carrier

import (
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestCarrierJumpFuelRespondsToJournalAndDoesNotDebitWarehouse(t *testing.T) {
	tr := New(nil)
	apply := func(kind, at string, fields M) {
		tr.Apply(v.Merge(M{"event": kind, "timestamp": at, "CarrierID": "42"}, fields), nil)
	}
	apply("CarrierStats", "2026-11-12T20:00:00Z", M{"Callsign": "SYN-001", "FuelLevel": 742,
		"SpaceUsage": M{"TotalCapacity": 25000, "FreeSpace": 18013, "Cargo": 2079}})
	apply("CarrierLocation", "2026-11-12T20:00:01Z", M{"StarSystem": "A", "SystemAddress": "1", "StarPos": []float64{0, 0, 0}})
	if err := tr.SetStockBaseline(M{"items": []M{{"key": "tritium", "count": 2079}}, "timestamp": "2026-11-12T20:00:02Z", "complete": true}); err != nil {
		t.Fatal(err)
	}
	apply("CarrierJumpRequest", "2026-11-12T20:00:03Z", M{"SystemName": "B", "SystemAddress": "2", "DepartureTime": "2026-11-12T20:15:03Z"})
	apply("CarrierLocation", "2026-11-12T20:14:03Z", M{"StarSystem": "B", "SystemAddress": "2"})
	arrival := M{"StarSystem": "B", "SystemAddress": "2", "MarketID": "42", "Docked": true, "StarPos": []float64{499.54, 0, 0}}
	apply("CarrierJump", "2026-11-12T20:15:05Z", arrival)
	apply("CarrierLocation", "2026-11-12T20:15:06Z", M{"StarSystem": "B", "SystemAddress": "2"})
	apply("CarrierJump", "2026-11-12T20:15:07Z", arrival)
	fuel := v.Obj(tr.Snapshot(nil)["fuel"])
	if v.Num(fuel["tankTons"]) != 655 || v.Num(fuel["reserveTons"]) != 2079 || !v.Bool(fuel["tankEstimated"]) || v.Bool(fuel["needsRefresh"]) {
		t.Fatal("jump should update the tank immediately and only once", fuel)
	}
	apply("CargoTransfer", "2026-11-12T20:15:08Z", M{"Transfers": []M{{"Type": "tritium", "Count": 140, "Direction": "toship"}}})
	apply("CarrierDepositFuel", "2026-11-12T20:15:09Z", M{"Total": 796, "Amount": 140})
	fuel = v.Obj(tr.Snapshot(nil)["fuel"])
	if v.Num(fuel["tankTons"]) != 796 || v.Num(fuel["reserveTons"]) != 1939 || v.Bool(fuel["tankEstimated"]) {
		t.Fatal("the journal must reconcile stock transfer and donation separately", fuel)
	}
}

func TestCarrierLateArrivalCoordinatesAndFrontierTankConfirmation(t *testing.T) {
	tr, result, cmd := capiStockFixture()
	tr.Apply(M{"event": "CarrierStats", "timestamp": "2026-11-09T00:00:01Z", "CarrierID": tr.OwnerID, "FuelLevel": 500,
		"SpaceUsage": M{"TotalCapacity": 25000, "FreeSpace": 20000}}, nil)
	tr.Apply(M{"event": "CarrierLocation", "timestamp": "2026-11-09T00:00:02Z", "CarrierID": tr.OwnerID, "StarSystem": "A", "StarPos": []float64{0, 0, 0}}, nil)
	tr.Apply(M{"event": "CarrierJumpRequest", "timestamp": "2026-11-09T00:00:03Z", "CarrierID": tr.OwnerID, "SystemName": "B", "DepartureTime": "2026-11-09T00:15:03Z"}, nil)
	tr.Apply(M{"event": "CarrierLocation", "timestamp": "2026-11-09T00:15:03Z", "CarrierID": tr.OwnerID, "StarSystem": "B"}, nil)
	if !v.Bool(v.At(tr.Snapshot(nil), "fuel", "needsRefresh")) {
		t.Fatal("a location without distance was treated as a fuel observation")
	}
	tr.Apply(M{"event": "CarrierJump", "timestamp": "2026-11-09T00:15:04Z", "MarketID": tr.OwnerID, "StarSystem": "B", "StarPos": []float64{400, 0, 0}}, nil)
	if got := v.Num(v.At(tr.Snapshot(nil), "fuel", "tankTons")); got != 434 {
		t.Fatalf("arrival coordinates did not resolve the pending burn: %v", got)
	}
	metadata := v.Obj(result["metadata"])
	metadata["requestStartedAt"] = M{"carrier": "2026-11-09T00:15:10Z"}
	metadata["observedAt"] = M{"carrier": "2026-11-09T00:15:11Z"}
	record := v.Obj(v.At(result, "data", "carrier"))
	record["currentStarSystem"], record["fuel"] = "A", 500
	tr.ApplyCAPI(result, cmd)
	if got := v.Num(v.At(tr.Snapshot(nil), "fuel", "tankTons")); got != 434 {
		t.Fatalf("pre-jump CAPI snapshot was applied: %v", got)
	}
	record["currentStarSystem"], record["fuel"] = "B", 435
	tr.ApplyCAPI(result, cmd)
	fuel := v.Obj(tr.Snapshot(nil)["fuel"])
	if v.Num(fuel["tankTons"]) != 435 || v.Bool(fuel["tankEstimated"]) {
		t.Fatal("confirmed tank did not replace the estimate", fuel)
	}
}
