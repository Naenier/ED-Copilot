package app

import (
	v "github.com/Naenier/ED-Copilot/internal/value"
	"testing"
)

func TestCommanderCargoKnowledgeRequiresACompleteInventoryRatherThanAnEventDate(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	e.Apply(M{"event": "Cargo", "timestamp": "2099-01-01T00:00:00Z", "Vessel": "Ship", "Inventory": []M{}}, nil)
	for _, state := range []M{e.Snapshot(), e.SnapshotForUI("commander")} {
		if !v.Bool(v.At(state, "commander", "cargoKnown")) || v.Num(v.At(state, "commander", "cargoTons")) != 0 {
			t.Fatal("observed empty cargo must remain known zero")
		}
	}
	e.Apply(M{"event": "MarketBuy", "timestamp": "2099-01-01T00:00:01Z", "Type": "gold", "Count": 2, "BuyPrice": 100, "TotalCost": 200}, nil)
	state := e.SnapshotForUI("commander")
	if v.At(state, "commander", "cargoUpdatedAt") == "" || v.Bool(v.At(state, "commander", "cargoKnown")) {
		t.Fatal("a delta date must not imply a complete snapshot")
	}
	e.Apply(M{"event": "Cargo", "timestamp": "2099-01-01T00:00:02Z", "Vessel": "Ship", "Inventory": []M{{"Name": "gold", "Count": 2}}}, nil)
	if !v.Bool(v.At(e.SnapshotForUI("commander"), "commander", "cargoKnown")) {
		t.Fatal("a refreshed snapshot must restore known state")
	}
}
