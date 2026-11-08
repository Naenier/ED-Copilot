package commander

import (
	"encoding/json"
	"reflect"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func aboard(id any, vehicle string) M {
	return M{"commander": M{"fid": "FTEST", "name": "Synthetic Pilot", "shipId": id, "vehicle": vehicle, "system": "Unrelated commander system", "station": "Unrelated commander station"}, "game": M{"multicrew": false}}
}

func shipLocationTracker() (*Tracker, M) {
	t := New()
	start(t)
	ctx := aboard(1, "ship")
	t.Apply(event("LoadGame", 1, M{"Ship": "python", "ShipID": 1}), ctx)
	t.Apply(event("Loadout", 2, M{"Ship": "python", "ShipID": 1}), ctx)
	t.Apply(event("Location", 3, M{"StarSystem": "Observed origin", "SystemAddress": json.Number("18446744073709551615"), "Docked": true, "StationName": "Observed port", "MarketID": json.Number("18446744073709551614")}), ctx)
	return t, ctx
}

func TestCurrentShipLocationFollowsObservedOwnShipLifecycle(t *testing.T) {
	tracker := New()
	start(tracker)
	ctx := aboard(1, "ship")
	tracker.Apply(event("LoadGame", 1, M{"Ship": "python", "ShipID": 1}), ctx)
	tracker.Apply(event("Loadout", 2, M{"Ship": "python", "ShipID": 1}), ctx)
	if v.At(tracker.Snapshot(), "fleet", "currentShip", "system") != nil {
		t.Fatal("Loadout copied unverified commander position")
	}
	tracker.Apply(event("Location", 3, M{"StarSystem": "Observed origin", "SystemAddress": json.Number("18446744073709551615"), "Docked": true, "StationName": "Observed port", "MarketID": json.Number("18446744073709551614")}), ctx)
	ship := v.Obj(v.At(tracker.Snapshot(), "fleet", "currentShip"))
	if ship["system"] != "Observed origin" || ship["station"] != "Observed port" || ship["systemAddress"] != "18446744073709551615" || ship["marketId"] != "18446744073709551614" || ship["locationKnown"] != true || ship["locationUpdatedAt"] != "2026-11-08T12:00:03Z" {
		t.Fatal("own location not observed accurately", ship)
	}
	tracker.Apply(event("Undocked", 4, M{"StationName": "Observed port"}), ctx)
	ship = v.Obj(v.At(tracker.Snapshot(), "fleet", "currentShip"))
	if ship["station"] != "" || ship["marketId"] != nil || ship["docked"] != false || ship["system"] != "Observed origin" {
		t.Fatal("undocking retained old station", ship)
	}
	tracker.Apply(event("FSDJump", 5, M{"StarSystem": "Observed destination", "SystemAddress": json.Number("18446744073709551613")}), ctx)
	tracker.Apply(event("Docked", 6, M{"StarSystem": "Observed destination", "StationName": "Destination port", "MarketID": 42}), ctx)
	ship = v.Obj(v.At(tracker.Snapshot(), "fleet", "currentShip"))
	if ship["system"] != "Observed destination" || ship["station"] != "Destination port" || ship["marketId"] != "42" || ship["docked"] != true {
		t.Fatal("arrival/docking not observed", ship)
	}
	tracker.Apply(event("Location", 7, M{"StarSystem": "Observed destination", "Docked": false, "StationName": "Not actually docked"}), ctx)
	if v.At(tracker.Snapshot(), "fleet", "currentShip", "station") != "" {
		t.Fatal("undocked Location created a fake station")
	}
}

func TestTaxiFootSRVAndMulticrewNeverMoveOwnedMothership(t *testing.T) {
	for _, test := range []struct {
		name       string
		transition M
		vehicle    string
	}{
		{"walking", event("Disembark", 4, M{"SRV": false, "Taxi": false, "ID": 1}), "foot"},
		{"srv", event("LaunchSRV", 4, M{"ID": 7}), "srv"},
		{"taxi despite ship context", event("Embark", 4, M{"Taxi": true, "SRV": false, "Multicrew": false}), "ship"},
		{"multicrew despite ship context", event("Embark", 4, M{"Taxi": false, "SRV": false, "Multicrew": true}), "ship"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tracker, ctx := shipLocationTracker()
			before := v.Copy(v.Obj(v.At(tracker.Snapshot(), "fleet", "currentShip")))
			tracker.Apply(test.transition, ctx)
			ctx = aboard(1, test.vehicle)
			for _, e := range []M{event("FSDJump", 5, M{"StarSystem": "Other vessel destination", "SystemAddress": 8}), event("Docked", 6, M{"StarSystem": "Other vessel destination", "StationName": "Other vessel port"}), event("Location", 7, M{"StarSystem": "Other vessel destination", "Docked": true, "StationName": "Other vessel port"}), event("Undocked", 8, nil)} {
				tracker.Apply(e, ctx)
			}
			if !reflect.DeepEqual(before, v.Obj(v.At(tracker.Snapshot(), "fleet", "currentShip"))) {
				t.Fatal("commander travel changed mothership position", tracker.Snapshot()["fleet"])
			}
			tracker.Apply(event("Embark", 9, M{"Taxi": false, "Multicrew": false, "SRV": false, "ID": 1}), aboard(1, "ship"))
			tracker.Apply(event("FSDJump", 10, M{"StarSystem": "Confirmed own flight"}), aboard(1, "ship"))
			if v.At(tracker.Snapshot(), "fleet", "currentShip", "system") != "Confirmed own flight" || v.At(tracker.Snapshot(), "fleet", "currentShip", "station") != "" {
				t.Fatal("confirmed own embark did not restore tracking")
			}
		})
	}
}

func TestLocationFlagsContextAndExactIDsMustAgree(t *testing.T) {
	for _, flag := range []string{"Taxi", "Multicrew", "OnFoot", "InSRV"} {
		t.Run(flag, func(t *testing.T) {
			tracker, ctx := shipLocationTracker()
			before := v.Copy(v.Obj(v.At(tracker.Snapshot(), "fleet", "currentShip")))
			tracker.Apply(event("Location", 4, M{flag: true, "StarSystem": "Rejected location", "Docked": true, "StationName": "Rejected port"}), ctx)
			if !reflect.DeepEqual(before, v.Obj(v.At(tracker.Snapshot(), "fleet", "currentShip"))) {
				t.Fatal("foreign mode flag moved own ship")
			}
		})
	}
	for _, test := range []struct {
		name    string
		context M
		fields  M
	}{
		{"foot context", aboard(1, "foot"), M{}},
		{"srv context", aboard(1, "srv"), M{}},
		{"different context ship", aboard(2, "ship"), M{}},
		{"different event ship", aboard(1, "ship"), M{"ShipID": 2}},
		{"unsafe event ship id", aboard(1, "ship"), M{"ShipID": float64(18446744073709551615)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tracker, _ := shipLocationTracker()
			before := v.Copy(v.Obj(v.At(tracker.Snapshot(), "fleet", "currentShip")))
			tracker.Apply(event("Location", 4, v.Merge(M{"StarSystem": "Rejected location", "Docked": true, "StationName": "Rejected port"}, test.fields)), test.context)
			if !reflect.DeepEqual(before, v.Obj(v.At(tracker.Snapshot(), "fleet", "currentShip"))) {
				t.Fatal("mismatched vehicle identity moved own ship")
			}
		})
	}
}

func TestOlderLocationOrPresenceCannotRewindOrPoisonShipLocation(t *testing.T) {
	tracker, ctx := shipLocationTracker()
	tracker.Apply(event("FSDJump", 10, M{"StarSystem": "Newest own system"}), ctx)
	before := tracker.Snapshot()
	tracker.Apply(event("Location", 5, M{"StarSystem": "Old system", "Docked": true, "StationName": "Old port"}), ctx)
	tracker.Apply(event("Embark", 6, M{"Taxi": true}), ctx)
	tracker.Apply(event("Loadout", 7, M{"Ship": "anaconda", "ShipID": 2}), aboard(2, "ship"))
	if !reflect.DeepEqual(before, tracker.Snapshot()) {
		t.Fatal("older location/loadout changed observed position")
	}
	tracker.Apply(event("Docked", 11, M{"StarSystem": "Newest own system", "StationName": "Newest own port"}), ctx)
	if v.At(tracker.Snapshot(), "fleet", "currentShip", "station") != "Newest own port" {
		t.Fatal("old taxi poisoned newer own-ship presence")
	}
}

func TestShipSwitchDropsLocationUntilNewIdentityHasOwnPresenceProof(t *testing.T) {
	tracker, _ := shipLocationTracker()
	ctx := aboard(2, "ship")
	tracker.Apply(event("ShipyardSwap", 4, M{"ShipType": "anaconda", "ShipID": 2}), ctx)
	ship := v.Obj(v.At(tracker.Snapshot(), "fleet", "currentShip"))
	if ship["system"] != nil || ship["station"] != nil || ship["locationKnown"] != nil {
		t.Fatal("new ship inherited previous ship position", ship)
	}
	tracker.Apply(event("Docked", 5, M{"StarSystem": "Unconfirmed system", "StationName": "Unconfirmed port"}), ctx)
	if v.At(tracker.Snapshot(), "fleet", "currentShip", "system") != nil {
		t.Fatal("selected ship treated as physical presence")
	}
	tracker.Apply(event("Location", 6, M{"StarSystem": "New own system", "Docked": true, "StationName": "New own port", "Taxi": false, "Multicrew": false, "OnFoot": false, "InSRV": false}), ctx)
	if v.At(tracker.Snapshot(), "fleet", "currentShip", "system") != "New own system" {
		t.Fatal("explicit matching own-ship Location did not restore presence")
	}
}

func TestCAPILoadoutDoesNotCopyCommanderLocation(t *testing.T) {
	tracker, ctx := shipLocationTracker()
	v.Obj(ctx["commander"])["vehicle"] = "foot"
	ctx["identityVerified"], ctx["observedAt"] = true, "2026-11-08T12:00:08Z"
	profile := M{"commander": M{"name": "Synthetic Pilot", "credits": 10}, "ship": M{"id": 1, "name": "python"}}
	tracker.ApplyCAPI(profile, ctx)
	if v.At(tracker.Snapshot(), "fleet", "currentShip", "system") != "Observed origin" || v.At(tracker.Snapshot(), "fleet", "currentShip", "station") != "Observed port" {
		t.Fatal("CAPI ship identity adopted commander's walking position")
	}
}
