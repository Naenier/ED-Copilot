package commander

import (
	"github.com/Naenier/ED-Copilot/internal/ships"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// Presence belongs to an exact owned ship, independently of the commander's
// location. Embark/Location describe taxis, multicrew, SRVs and walking too.
// In particular a Loadout or CAPI ship record does not prove being aboard it.
func (t *Tracker) observeShipPresence(event, context M, at string) bool {
	if !t.fresh("shipPresence", at, "journal") {
		return false
	}
	if previous := t.stamps["shipLocation"].at; previous != "" && v.Time(at).Before(v.Time(previous)) {
		return false
	}
	kind := v.Str(event["event"])
	currentID := v.Str(v.At(t.state, "fleet", "currentShip", "shipId"))
	presence, id := t.shipPresence, t.aboardShipID
	set := false
	switch kind {
	case "LoadGame":
		if raw := clean(event["Ship"]); raw != "" {
			presence, id, set = ships.VehicleKind(raw), "", true
			if presence == "ship" {
				id = exactID(event["ShipID"])
				if id == "" || id != currentID {
					presence, id = "unknown", ""
				}
			}
		}
	case "Disembark", "DropShipDeploy":
		presence, id, set = "foot", "", true
	case "LaunchSRV":
		presence, id, set = "srv", "", true
	case "JoinACrew", "CrewJoin":
		presence, id, set = "multicrew", "", true
	case "QuitACrew", "CrewQuit":
		presence, id, set = "unknown", "", true
	case "Embark":
		presence, id, set = "unknown", "", true
		if !v.Bool(event["SRV"]) && !v.Bool(event["Taxi"]) && !v.Bool(event["Multicrew"]) {
			if boarded := exactID(event["ID"]); boarded != "" && boarded == currentID {
				presence, id = "ship", boarded
			}
		}
		if v.Bool(event["SRV"]) {
			presence = "srv"
		}
	case "DockSRV", "DockFighter":
		if currentID != "" && v.At(context, "commander", "vehicle") == "ship" && exactID(v.At(context, "commander", "shipId")) == currentID {
			presence, id, set = "ship", currentID, true
		}
	case "LaunchFighter":
		if v.Bool(event["PlayerControlled"]) {
			presence, id, set = "fighter", "", true
		}
	case "Location":
		// Odyssey supplies all four flags. Only an explicit own-ship state
		// with the matching context ID can re-establish presence on startup.
		allFalse := true
		for _, key := range []string{"Taxi", "Multicrew", "InSRV", "OnFoot"} {
			if flag, ok := event[key].(bool); !ok || flag {
				allFalse = false
			}
		}
		vehicle := v.Str(v.At(context, "commander", "vehicle"))
		if allFalse && currentID != "" && exactID(v.At(context, "commander", "shipId")) == currentID && (vehicle == "ship" || vehicle == "unknown" || vehicle == "") {
			presence, id, set = "ship", currentID, true
		}
	}
	for flag, mode := range map[string]string{"Taxi": "taxi", "Multicrew": "multicrew", "InSRV": "srv", "OnFoot": "foot"} {
		if v.Bool(event[flag]) {
			presence, id, set = mode, "", true
		}
	}
	if !set {
		return false
	}
	changed := t.shipPresence != presence || t.aboardShipID != id
	t.shipPresence, t.aboardShipID = presence, id
	t.stamps["shipPresence"] = stamp{at, "journal"}
	if t.latest == "" || v.Time(at).After(v.Time(t.latest)) {
		t.latest = at
	}
	return changed
}

func (t *Tracker) observeShipLocation(event, context M, at string) bool {
	kind := v.Str(event["event"])
	if kind != "Location" && kind != "Docked" && kind != "FSDJump" && kind != "Undocked" && kind != "CarrierJump" {
		return false
	}
	if !t.fresh("shipLocation", at, "journal") || !t.fresh("shipPresence", at, "journal") {
		return false
	}
	ship := v.Obj(v.At(t.state, "fleet", "currentShip"))
	id := v.Str(ship["shipId"])
	if id == "" || t.shipPresence != "ship" || t.aboardShipID != id {
		return false
	}
	for _, flag := range []string{"Taxi", "Multicrew", "InSRV", "OnFoot"} {
		if v.Bool(event[flag]) {
			return false
		}
	}
	if v.Bool(v.At(context, "game", "multicrew")) {
		return false
	}
	if vehicle := v.Str(v.At(context, "commander", "vehicle")); vehicle != "" && vehicle != "ship" {
		return false
	}
	if scoped, exists := v.Obj(context["commander"])["shipId"]; exists && scoped != nil && exactID(scoped) != id {
		return false
	}
	if explicit, exists := event["ShipID"]; exists && explicit != nil && exactID(explicit) != id {
		return false
	}
	if previous := t.stamps["currentShip"].at; previous != "" && v.Time(at).Before(v.Time(previous)) {
		return false
	}
	before := v.Copy(ship)
	system := clean(event["StarSystem"])
	if system != "" {
		if system != v.Str(ship["system"]) {
			ship["systemAddress"] = nil
		}
		ship["system"] = system
	}
	if address := exactID(event["SystemAddress"]); address != "" {
		ship["systemAddress"] = address
	}
	switch kind {
	case "Docked":
		ship["station"], ship["docked"] = clean(event["StationName"]), true
		ship["marketId"] = nil
		if market := exactID(event["MarketID"]); market != "" {
			ship["marketId"] = market
		}
	case "Undocked", "FSDJump":
		ship["station"], ship["marketId"], ship["docked"] = "", nil, false
	case "Location", "CarrierJump":
		ship["station"], ship["marketId"], ship["docked"] = "", nil, nil
		if docked, known := event["Docked"].(bool); known {
			ship["docked"] = docked
			if docked {
				ship["station"] = clean(event["StationName"])
				if market := exactID(event["MarketID"]); market != "" {
					ship["marketId"] = market
				}
			}
		}
	}
	ship["locationKnown"] = v.Str(ship["system"]) != "" || v.Str(ship["station"]) != ""
	ship["locationUpdatedAt"], ship["locationSource"] = at, "journal"
	t.stamps["shipLocation"] = stamp{at, "journal"}
	t.stamps["shipPresence"] = stamp{at, "journal"}
	if t.latest == "" || v.Time(at).After(v.Time(t.latest)) {
		t.latest = at
	}
	return !sameObservation(before, ship)
}
