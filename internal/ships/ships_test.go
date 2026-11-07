package ships

import (
	v "github.com/Naenier/ED-Copilot/internal/value"
	"testing"
)

func TestModelNamesAndCustomIdentity(t *testing.T) {
	for raw, want := range map[string]string{"panthermkii": "Panther Clipper Mk II", "PYTHON_NX": "Python Mk II", "AnacondaMkII": "Anaconda Mk II", "MediumTransport01": "Lynx Highliner", "Federation_Dropship_MkII": "Federal Assault Ship", "$ShipName_PantherMkII;": "Panther Clipper Mk II"} {
		if got := DisplayShip(raw); got != want {
			t.Fatalf("%s => %s, want %s", raw, got, want)
		}
	}
	if DisplayShip("FutureHull", "Будущий корабль") != "Будущий корабль" || DisplayShip("FutureHull") != "FutureHull" {
		t.Fatal("unknown hull fallback")
	}
	state := ApplyEvent(nil, v.M{"event": "LoadGame", "Ship": "panthermkii", "ShipID": 42, "ShipName": "Мой корабль", "ShipIdent": "ED-42"})
	if state["ship"] != "Panther Clipper Mk II" || state["shipName"] != "Мой корабль" {
		t.Fatal(state)
	}
	state = ApplyEvent(state, v.M{"event": "SetUserShipName", "Ship": "Lander01", "ShipID": 173, "UserShipName": "NomadName", "UserShipId": "SLV"})
	if state["ship"] != "Panther Clipper Mk II" || state["shipName"] != "Мой корабль" {
		t.Fatal("launchable rename changed mothership")
	}
	state = ApplyEvent(state, v.M{"event": "SetUserShipName", "Ship": "panthermkii", "ShipID": 42, "UserShipName": "Новое имя", "UserShipId": "ED-43"})
	if state["shipName"] != "Новое имя" || state["shipIdent"] != "ED-43" {
		t.Fatal("own rename")
	}
	state = ApplyEvent(state, v.M{"event": "LoadGame", "Ship": "Lander01", "ShipID": 173, "ShipName": "NomadName"})
	if state["ship"] != "Panther Clipper Mk II" || state["vehicleName"] != "Nomad" || state["vehicle"] != "srv" {
		t.Fatal("vehicle identity", state)
	}
	state = ApplyEvent(state, v.M{"event": "Loadout", "Ship": "Python", "ShipID": 99})
	if state["shipName"] != "" || state["ship"] != "Python" {
		t.Fatal("ship swap retained old vanity name")
	}
	for raw, want := range map[string]string{"Buggy": "srv", "Combat_SRV": "srv", "Lander01": "srv", "OnFoot": "foot", "Anaconda": "ship"} {
		if VehicleKind(raw) != want {
			t.Fatal(raw)
		}
	}
	profile := CAPI(v.M{"name": "panthermkii", "id": 42, "shipName": "DESTINY", "shipID": "ED-42"})
	if profile["ship"] != "Panther Clipper Mk II" || profile["shipName"] != "DESTINY" || profile["shipId"] != 42 || profile["shipIdent"] != "ED-42" {
		t.Fatal("CAPI fields conflated", profile)
	}
}
