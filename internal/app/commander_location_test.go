package app

import (
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestCommanderCurrentShipReceivesReplayedLocationAndOwnFlight(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	ship := v.Obj(v.At(e.SnapshotForUI("commander"), "commanderProfile", "fleet", "currentShip"))
	if ship["system"] != "Synthetic Origin" || ship["station"] != "Synthetic Station" || ship["systemAddress"] != "18446744073709551615" || ship["locationKnown"] != true {
		t.Fatal("replayed Location did not reach current ship", ship)
	}
	e.Apply(commanderEvent("Undocked", "2026-11-08T12:00:00Z", nil), nil)
	e.Apply(commanderEvent("FSDJump", "2026-11-08T12:00:01Z", M{"StarSystem": "Own destination", "SystemAddress": "18446744073709551614"}), nil)
	ship = v.Obj(v.At(e.SnapshotForUI("commander"), "commanderProfile", "fleet", "currentShip"))
	if ship["system"] != "Own destination" || ship["station"] != "" || ship["docked"] != false || ship["marketId"] != nil {
		t.Fatal("own flight retained stale station", ship)
	}
}

func TestCommanderCurrentShipIgnoresTaxiDespiteLegacyCommanderShipVehicle(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	e.Apply(commanderEvent("Disembark", "2026-11-08T12:00:00Z", M{"Taxi": false, "SRV": false, "ID": 1}), nil)
	e.Apply(commanderEvent("Embark", "2026-11-08T12:00:01Z", M{"Taxi": true, "Multicrew": false, "SRV": false}), nil)
	e.Apply(commanderEvent("FSDJump", "2026-11-08T12:00:02Z", M{"StarSystem": "Taxi destination", "Taxi": true}), nil)
	e.Apply(commanderEvent("Docked", "2026-11-08T12:00:03Z", M{"StarSystem": "Taxi destination", "StationName": "Taxi port"}), nil)
	state := e.SnapshotForUI("commander")
	ship := v.Obj(v.At(state, "commanderProfile", "fleet", "currentShip"))
	if v.At(state, "commander", "system") != "Taxi destination" || ship["system"] != "Synthetic Origin" || ship["station"] != "Synthetic Station" {
		t.Fatal("taxi commander movement changed mothership location", ship)
	}
	e.Apply(commanderEvent("Disembark", "2026-11-08T12:00:04Z", M{"Taxi": true, "SRV": false}), nil)
	e.Apply(commanderEvent("Embark", "2026-11-08T12:00:05Z", M{"Taxi": false, "Multicrew": false, "SRV": false, "ID": 1}), nil)
	e.Apply(commanderEvent("Location", "2026-11-08T12:00:06Z", M{"StarSystem": "Confirmed own destination", "Docked": true, "StationName": "Confirmed own port"}), nil)
	ship = v.Obj(v.At(e.SnapshotForUI("commander"), "commanderProfile", "fleet", "currentShip"))
	if ship["system"] != "Confirmed own destination" || ship["station"] != "Confirmed own port" {
		t.Fatal("confirmed own embark did not restore location tracking", ship)
	}
}

func TestCommanderCurrentShipIgnoresSRVUntilDockedBackAboard(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	e.Apply(commanderEvent("LaunchSRV", "2026-11-08T12:00:00Z", M{"ID": 7}), nil)
	e.Apply(commanderEvent("Location", "2026-11-08T12:00:01Z", M{"StarSystem": "SRV location", "InSRV": true, "Docked": false}), nil)
	if v.At(e.SnapshotForUI("commander"), "commanderProfile", "fleet", "currentShip", "system") != "Synthetic Origin" {
		t.Fatal("SRV location replaced mothership observation")
	}
	e.Apply(commanderEvent("DockSRV", "2026-11-08T12:00:02Z", M{"ID": 7}), nil)
	e.Apply(commanderEvent("FSDJump", "2026-11-08T12:00:03Z", M{"StarSystem": "Own destination"}), nil)
	if v.At(e.SnapshotForUI("commander"), "commanderProfile", "fleet", "currentShip", "system") != "Own destination" {
		t.Fatal("DockSRV did not restore matching own ship presence")
	}
}
