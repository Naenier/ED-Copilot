package app

import (
	"slices"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// CAPI in this app uses Frontier's Live host. Its roster can identify stored
// ships but often omits their modules. Journal observations always take
// precedence for an individual loadout.
func (e *Engine) observeFrontierFleet(result, profile, commander M) {
	owner := e.planWorkspace
	if owner.key == "" || owner.branch != "live" || e.planReplay ||
		v.Str(v.At(result, "metadata", "source")) != "Frontier CAPI" ||
		v.Str(commander["name"]) == "" || !strings.EqualFold(v.Str(v.At(profile, "commander", "name")), v.Str(commander["name"])) {
		return
	}
	if fid := v.Str(v.At(profile, "commander", "fid")); fid != "" && fid != v.Str(commander["fid"]) {
		return
	}
	observed := v.Time(v.At(result, "metadata", "observedAt", "profile"))
	if observed.IsZero() {
		return
	}
	at := v.Time(v.At(result, "metadata", "requestStartedAt", "profile"))
	if at.IsZero() {
		at = observed
	}
	f := &e.section7.fleetFits
	if f.owner != owner.key {
		f.reset(owner.key)
	}
	f.capiList(profile["ships"], at, observed)
	active := v.Obj(profile["ship"])
	if id := engineeringShipID(active["id"]); id != "" && id == engineeringShipID(commander["shipId"]) {
		f.capiShip(active, id, at, observed)
		if f.current == "" {
			f.current = id
		}
	}
}

func (f *fleetFitObservations) capiList(raw any, at, observed time.Time) {
	if at.Before(f.listAt) || at.Equal(f.listAt) {
		return
	}
	complete := true
	switch rows := raw.(type) {
	case M:
		keys := make([]string, 0, len(rows))
		for id := range rows {
			keys = append(keys, id)
		}
		slices.Sort(keys)
		for _, key := range keys[:min(len(keys), fleetFitLimit)] {
			id, row := engineeringShipID(key), v.Obj(rows[key])
			if id == "" || row["id"] != nil && engineeringShipID(row["id"]) != id || !f.capiShip(row, id, at, observed) {
				complete = false
			}
		}
		complete = complete && len(keys) <= fleetFitLimit
	case []any, []M:
		items := v.Arr(raw)
		for _, item := range items[:min(len(items), fleetFitLimit)] {
			if item == nil {
				continue
			}
			row := v.Obj(item)
			if !f.capiShip(row, engineeringShipID(row["id"]), at, observed) {
				complete = false
			}
		}
		complete = complete && len(items) <= fleetFitLimit
	default:
		return
	}
	f.setRosterDate(at, complete)
}

func (f *fleetFitObservations) capiShip(raw M, id string, at, observed time.Time) bool {
	identity := fleetIdentity(id, v.Str(raw["name"]), raw["shipName"], raw["locName"], raw["shipID"])
	if identity == nil {
		return false
	}
	identity["system"], identity["station"] = v.Str(v.At(raw, "starsystem", "name")), v.Str(v.At(raw, "station", "name"))
	ship := f.see(identity, at)
	if ship == nil || ship.source == "journal" && ship.loadout != nil {
		return true
	}
	if loadout := capiFleetLoadout(raw, identity, observed); loadout != nil {
		ship.setLoadout(loadout, at, "capi")
	}
	return true
}

// Convert only explicit Companion API values into the journal loadout format.
// Some responses carry a recipe level without its progress or an exact
// numerical roll. The fitter would assume a full roll if we omitted it.
func capiFleetLoadout(raw, identity M, observed time.Time) M {
	bySlot := v.Obj(raw["modules"])
	if len(bySlot) < 8 || len(bySlot) > 120 {
		return nil
	}
	slots := make([]string, 0, len(bySlot))
	for slot := range bySlot {
		slots = append(slots, slot)
	}
	slices.Sort(slots)
	modules := make([]M, 0, len(bySlot))
	for _, slot := range slots {
		rawModule := v.Obj(bySlot[slot])
		module := v.Obj(rawModule["module"])
		item, validItem := module["name"].(string)
		if !validItem || item == "" {
			return nil
		}
		row := M{"Slot": slot, "Item": item}
		if enabled, ok := module["on"].(bool); ok {
			row["On"] = enabled
		}
		if priority, valid := v.Number(module["priority"]); valid {
			row["Priority"] = priority
		}
		engineer := v.Obj(rawModule["engineer"])
		special := v.Obj(rawModule["specialModifications"])
		if len(v.Arr(rawModule["specialModifications"])) > 0 {
			return nil
		}
		if len(engineer) > 0 || len(special) > 0 || len(v.Obj(rawModule["WorkInProgress_modifications"])) > 0 {
			level, levelOK := v.Number(engineer["recipeLevel"])
			quality, qualityOK := v.Number(engineer["recipeQuality"])
			blueprint := v.Str(engineer["recipeName"])
			if blueprint == "" || !levelOK || level < 1 || level > 5 || float64(int(level)) != level ||
				!qualityOK || quality < 0 || quality > 1 || len(special) > 1 {
				return nil
			}
			engineering := M{"BlueprintName": blueprint, "Level": level, "Quality": quality}
			for effect := range special {
				engineering["ExperimentalEffect"] = effect
			}
			row["Engineering"] = engineering
		}
		modules = append(modules, row)
	}
	if !fullFleetLoadout(modules) {
		return nil
	}
	return M{"event": "Loadout", "timestamp": v.ISO(observed), "Ship": identity["shipType"], "ShipID": identity["shipId"],
		"ShipName": identity["shipName"], "ShipIdent": identity["shipIdent"], "Modules": modules}
}
