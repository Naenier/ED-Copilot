package app

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/shipbuild"
	"github.com/Naenier/ED-Copilot/internal/ships"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const fleetFitLimit = 512

type fleetFitShip struct {
	identity   M
	seenAt     time.Time
	retiredAt  time.Time
	modifiedAt time.Time
	source     string
	loadout    M
	loadoutAt  time.Time
}

type fleetFitObservations struct {
	owner      string
	ships      map[string]*fleetFitShip
	current    string
	listAt     time.Time
	completeAt time.Time
	truncated  bool
}

func (f *fleetFitObservations) reset(owner string) {
	*f = fleetFitObservations{owner: owner, ships: make(map[string]*fleetFitShip)}
}

func (f *fleetFitObservations) record(id string, at time.Time) *fleetFitShip {
	if ship := f.ships[id]; ship != nil {
		return ship
	}
	if len(f.ships) >= fleetFitLimit {
		f.truncated = true
		oldest := ""
		for key, ship := range f.ships {
			if key != f.current && (oldest == "" || ship.seenAt.Before(f.ships[oldest].seenAt)) {
				oldest = key
			}
		}
		if oldest == "" || at.Before(f.ships[oldest].seenAt) {
			return nil
		}
		delete(f.ships, oldest)
	}
	ship := &fleetFitShip{seenAt: at}
	f.ships[id] = ship
	return ship
}

func fleetIdentity(id, symbol string, rawName, localName, ident any) M {
	if id == "" || symbol == "" || ships.VehicleKind(symbol) != "ship" {
		return nil
	}
	return ships.Identity(symbol, v.Str(localName), id, v.Str(rawName), v.Str(ident))
}

func (f *fleetFitObservations) see(identity M, at time.Time) *fleetFitShip {
	if identity == nil {
		return nil
	}
	ship := f.record(v.Str(identity["shipId"]), at)
	if ship == nil || at.Before(ship.seenAt) || !ship.retiredAt.IsZero() {
		return nil
	}
	for key, old := range ship.identity {
		if v.Str(identity[key]) == "" {
			identity[key] = old
		}
	}
	if ship.identity["ship"] != nil && !strings.EqualFold(v.Str(ship.identity["ship"]), v.Str(identity["ship"])) {
		ship.loadout, ship.loadoutAt, ship.source, ship.modifiedAt = nil, time.Time{}, "", at
	}
	ship.identity, ship.seenAt = identity, at
	return ship
}

func (s *fleetFitShip) setLoadout(loadout M, at time.Time, source string) {
	if at.Before(s.modifiedAt) || at.Before(s.loadoutAt) || at.Equal(s.loadoutAt) && s.source == "journal" && source == "capi" {
		return
	}
	s.loadout, s.loadoutAt, s.source = loadout, at, source
}

func (f *fleetFitObservations) retire(id string, at time.Time) {
	if id == "" {
		return
	}
	if ship := f.record(id, at); ship != nil && !at.Before(ship.modifiedAt) && !at.Before(ship.retiredAt) &&
		(ship.source != "journal" || !at.Before(ship.loadoutAt)) {
		*ship = fleetFitShip{seenAt: at, retiredAt: at}
		if f.current == id {
			f.current = ""
		}
	}
}

func (f *fleetFitObservations) journal(event M) {
	kind := v.Str(event["event"])
	switch kind {
	case "StoredShips", "LoadGame", "Loadout", "ShipyardSwap", "ShipyardNew", "ShipyardBuy", "ShipyardSell", "SetUserShipName",
		"EngineerCraft", "EngineerLegacyConvert", "ModuleBuy", "ModuleSell", "ModuleStore", "ModuleRetrieve", "ModuleSwap", "MassModuleStore":
	default:
		return
	}
	at := v.Time(event["timestamp"])
	if at.IsZero() {
		return
	}
	switch kind {
	case "StoredShips":
		f.journalList(event, at)
		return
	case "ShipyardSwap", "ShipyardBuy", "ShipyardSell":
		f.retire(engineeringShipID(event["SellShipID"]), at)
		if id := engineeringShipID(event["StoreShipID"]); id != "" {
			f.see(fleetIdentity(id, v.Str(event["StoreOldShip"]), nil, nil, nil), at)
		}
		if kind != "ShipyardSwap" {
			return
		}
	}
	if oneOf(kind, "LoadGame", "Loadout", "ShipyardSwap", "ShipyardNew") {
		id := observedEngineeringShipID(event)
		identity := fleetIdentity(id, v.Text(event["Ship"], event["ShipType"]), event["ShipName"], v.First(event["Ship_Localised"], event["ShipType_Localised"]), event["ShipIdent"])
		if kind == "ShipyardNew" {
			// Reuse of an id is only valid if the journal explicitly reports
			// acquisition of a new hull after the previous hull was sold.
			if previous := f.ships[id]; previous != nil && !at.Before(previous.seenAt) {
				delete(f.ships, id)
			}
		}
		if ship := f.see(identity, at); ship != nil {
			f.current = id
			if kind == "ShipyardNew" {
				ship.modifiedAt = at
			}
			if kind == "Loadout" && fullFleetLoadout(event["Modules"]) {
				payload := M{"event": "Loadout", "timestamp": event["timestamp"], "Ship": identity["shipType"], "ShipID": id,
					"ShipName": identity["shipName"], "ShipIdent": identity["shipIdent"], "Modules": v.Clone(event["Modules"])}
				for _, key := range []string{"FuelCapacity", "FuelLevel", "CargoCapacity", "MaxJumpRange", "UnladenMass"} {
					if value := event[key]; value != nil {
						payload[key] = v.Clone(value)
					}
				}
				ship.setLoadout(payload, at, "journal")
			}
		}
		return
	}
	id := engineeringShipID(event["ShipID"])
	if event["ShipID"] == nil {
		id = f.current
	}
	ship := f.ships[id]
	if ship == nil || at.Before(ship.loadoutAt) || !ship.retiredAt.IsZero() {
		return
	}
	switch kind {
	case "SetUserShipName":
		for destination, fields := range map[string][]string{
			"shipName": {"UserShipName"}, "shipIdent": {"UserShipId", "UserShipID"},
		} {
			for _, field := range fields {
				if name, exists := event[field]; exists {
					ship.identity[destination] = v.Str(name)
					break
				}
			}
		}
	case "EngineerCraft", "EngineerLegacyConvert":
		if ship.loadout == nil {
			return
		}
		crafted := shipbuild.ApplyObservedCraft(ship.loadout, event)
		if slot := v.Str(event["Slot"]); slot != "" {
			for _, module := range v.Rows(crafted["Modules"]) {
				if strings.EqualFold(v.Str(module["Slot"]), slot) && module["EngineeringObservedAt"] == event["timestamp"] {
					ship.setLoadout(crafted, at, "journal")
					break
				}
			}
		}
	default:
		ship.loadout, ship.modifiedAt, ship.source = nil, at, ""
	}
}

func (f *fleetFitObservations) journalList(event M, at time.Time) {
	if at.Before(f.listAt) {
		return
	}
	here, remote := v.Arr(event["ShipsHere"]), v.Arr(event["ShipsRemote"])
	if here == nil && remote == nil {
		return
	}
	complete := here != nil && remote != nil && len(here)+len(remote) <= fleetFitLimit
	for _, group := range []struct {
		ships []any
		here  bool
	}{{here, true}, {remote, false}} {
		for _, raw := range group.ships[:min(len(group.ships), fleetFitLimit)] {
			row := v.Obj(raw)
			identity := fleetIdentity(engineeringShipID(row["ShipID"]), v.Str(row["ShipType"]), row["Name"], row["ShipType_Localised"], row["ShipIdent"])
			if identity == nil {
				complete = false
				continue
			}
			identity["system"], identity["station"] = v.Str(row["StarSystem"]), v.Str(row["StationName"])
			if group.here {
				identity["system"], identity["station"] = v.Str(event["StarSystem"]), v.Str(event["StationName"])
			}
			f.see(identity, at)
		}
	}
	f.setRosterDate(at, complete)
}

func (f *fleetFitObservations) setRosterDate(at time.Time, complete bool) {
	f.listAt = at
	if complete {
		f.completeAt = at
	}
}

// A partial record cannot be interpreted as a factory fit. The fitter gets
// every installed item from a full snapshot and never invents missing core slots.
func fullFleetLoadout(raw any) bool {
	modules := v.Arr(raw)
	if len(modules) < 8 || len(modules) > 120 {
		return false
	}
	slots := make(map[string]bool, len(modules))
	for _, value := range modules {
		module := v.Obj(value)
		slot, validSlot := module["Slot"].(string)
		item, validItem := module["Item"].(string)
		slot = strings.ToLower(slot)
		if !validSlot || !validItem || slot == "" || item == "" || slots[slot] {
			return false
		}
		slots[slot] = true
	}
	for _, required := range []string{"armour", "powerplant", "mainengines", "frameshiftdrive", "lifesupport", "powerdistributor", "radar", "fueltank"} {
		if !slots[required] {
			return false
		}
	}
	return true
}

func (e *Engine) fleetFitsCommand(path string, body M) (M, error) {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.RLock()
	owner := e.planWorkspace.key
	currentPosition := M{"shipID": engineeringShipID(e.commander["shipId"]), "vehicle": e.commander["vehicle"],
		"system": e.commander["system"], "station": e.commander["station"]}
	e.mu.RUnlock()
	if err := validatePlanWorkspace(body, owner); err != nil {
		return nil, err
	}
	f := &e.section7.fleetFits
	pending := e.journalObservationPending(e.windowConnection(nil))
	if path == "/api/ship-fits/fleet/loadout" {
		id, ok := body["shipId"].(string)
		if !ok || engineeringShipID(id) != id || id == "" {
			return nil, errors.New("Выберите корабль из флота.")
		}
		ship := f.ships[id]
		if pending || owner == "" || owner != f.owner || !f.visible(id, ship) || ship.loadout == nil {
			return nil, errors.New("Сборка этого корабля недоступна. Переключитесь на него в игре, чтобы журнал записал оснащение.")
		}
		loadout := v.Copy(ship.loadout)
		loadout["ShipName"], loadout["ShipIdent"] = ship.identity["shipName"], ship.identity["shipIdent"]
		return M{"loadout": loadout}, nil
	}
	rows := []M{}
	if pending || owner == "" || owner != f.owner {
		return M{"rows": rows, "known": false, "complete": false, "pending": pending}, nil
	}
	for id, ship := range f.ships {
		if !f.visible(id, ship) {
			continue
		}
		row := v.Copy(ship.identity)
		row["current"], row["available"] = id == f.current, ship.loadout != nil
		if row["current"] == true && currentPosition["shipID"] == id && currentPosition["vehicle"] == "ship" {
			row["system"], row["station"] = v.Str(currentPosition["system"]), v.Str(currentPosition["station"])
		}
		if ship.loadout != nil {
			row["observedAt"], row["source"] = v.ISO(ship.loadoutAt), ship.source
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a["current"] != b["current"] {
			return a["current"] == true
		}
		if v.Str(a["ship"]) != v.Str(b["ship"]) {
			return v.Str(a["ship"]) < v.Str(b["ship"])
		}
		if v.Str(a["shipName"]) != v.Str(b["shipName"]) {
			return v.Str(a["shipName"]) < v.Str(b["shipName"])
		}
		return v.Str(a["shipId"]) < v.Str(b["shipId"])
	})
	return M{"rows": rows, "known": !f.listAt.IsZero() || len(rows) > 0, "complete": !f.completeAt.IsZero() && f.completeAt.Equal(f.listAt) && !f.truncated}, nil
}

func (f *fleetFitObservations) visible(id string, ship *fleetFitShip) bool {
	return ship != nil && ship.identity != nil && ship.retiredAt.IsZero() &&
		(id == f.current || !ship.seenAt.Before(f.completeAt))
}
