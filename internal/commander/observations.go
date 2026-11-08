package commander

import (
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/ships"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func arrayRows(x any) ([]M, bool) {
	a := v.Arr(x)
	if a == nil {
		return nil, false
	}
	rows := make([]M, 0, len(a))
	for index, raw := range a {
		if index > maxModules {
			break
		}
		row, ok := raw.(M)
		if !ok || row == nil {
			return nil, false
		}
		rows = append(rows, row)
	}
	return rows, true
}

func shipRow(raw M, here bool, event M) M {
	id := exactID(v.First(raw["ShipID"], raw["id"]))
	symbol := clean(v.First(raw["ShipType"], raw["name"]))
	if id == "" || symbol == "" || ships.VehicleKind(symbol) != "ship" {
		return nil
	}
	row := ships.Identity(symbol, clean(v.First(raw["ShipType_Localised"], raw["locName"])), id, clean(v.First(raw["Name"], raw["shipName"])), clean(v.First(raw["ShipIdent"], raw["shipID"])))
	row["id"], row["shipId"], row["here"] = id, id, here
	row["value"], _ = numeric(v.First(raw["Value"], v.At(raw, "value", "total")), 0, 1e18)
	row["inTransit"] = nil
	if b, ok := raw["InTransit"].(bool); ok {
		row["inTransit"] = b
	}
	row["hot"] = nil
	if b, ok := raw["Hot"].(bool); ok {
		row["hot"] = b
	}
	if here {
		row["system"], row["station"] = clean(event["StarSystem"]), clean(event["StationName"])
		row["marketId"] = exactID(event["MarketID"])
	} else {
		row["system"], row["station"] = clean(raw["StarSystem"]), clean(raw["StationName"])
		row["marketId"] = exactID(raw["ShipMarketID"])
	}
	row["systemAddress"] = exactID(raw["SystemAddress"])
	row["transferPrice"], _ = numeric(raw["TransferPrice"], 0, 1e18)
	row["transferTimeSeconds"], _ = numeric(raw["TransferTime"], 0, 1e9)
	return row
}

func (t *Tracker) storedShips(event M, at string) bool {
	if !t.fresh("fleet", at, "journal") {
		return false
	}
	here, okHere := arrayRows(event["ShipsHere"])
	remote, okRemote := arrayRows(event["ShipsRemote"])
	if !okHere && !okRemote {
		return false
	}
	rows := []M{}
	seen := map[string]bool{}
	truncated := false
	for _, group := range []struct {
		rows []M
		here bool
	}{{here, true}, {remote, false}} {
		for _, raw := range group.rows {
			row := shipRow(raw, group.here, event)
			if row == nil {
				truncated = true
				continue
			}
			id := v.Str(row["id"])
			if seen[id] {
				continue
			}
			seen[id] = true
			if len(rows) >= maxShips {
				truncated = true
				break
			}
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return v.Str(rows[i]["id"]) < v.Str(rows[j]["id"]) })
	prev := v.Obj(t.state["fleet"])
	next := M{"entries": rows, "known": true, "count": len(rows), "updatedAt": at, "source": "journal", "scope": "storedShips", "complete": false, "storedListComplete": okHere && okRemote && !truncated, "truncated": truncated, "currentShip": prev["currentShip"]}
	if truncated {
		next["count"] = nil
	}
	t.state["fleet"] = next
	t.mark("fleet", at, "journal", next)
	return !reflect.DeepEqual(prev, next)
}

func (t *Tracker) modules(event M, at string) bool {
	if !t.fresh("storedModules", at, "journal") {
		return false
	}
	rawRows, ok := arrayRows(event["Items"])
	if !ok {
		return false
	}
	rows := []M{}
	seen := map[string]bool{}
	truncated := false
	for index, raw := range rawRows {
		symbol := clean(raw["Name"])
		if symbol == "" {
			symbol = clean(raw["Item"])
		}
		if symbol == "" {
			truncated = true
			continue
		}
		slot := exactID(raw["StorageSlot"])
		key := slot
		if key == "" {
			key = "unkeyed-" + strconv.Itoa(index)
			truncated = true
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		if len(rows) >= maxModules {
			truncated = true
			break
		}
		row := M{"id": key, "storageSlot": slot, "symbol": symbol, "name": v.Text(clean(raw["Name_Localised"]), clean(raw["Item_Localised"]), label(symbol)), "system": clean(raw["StarSystem"]), "marketId": exactID(raw["MarketID"]), "systemAddress": exactID(raw["SystemAddress"]), "station": clean(raw["StationName"]), "engineerModifications": clean(raw["EngineerModifications"])}
		for f, k := range map[string]string{"BuyPrice": "buyPrice", "TransferCost": "transferCost", "TransferTime": "transferTimeSeconds", "Level": "level", "Quality": "quality"} {
			row[k], _ = numeric(raw[f], 0, 1e18)
		}
		row["hot"] = nil
		if b, ok := raw["Hot"].(bool); ok {
			row["hot"] = b
		}
		row["inTransit"] = nil
		if b, ok := raw["InTransit"].(bool); ok {
			row["inTransit"] = b
		}
		rows = append(rows, row)
	}
	next := M{"known": true, "entries": rows, "count": len(rows), "truncated": truncated, "updatedAt": at, "source": "journal"}
	if truncated {
		next["count"] = nil
	}
	prev := t.state["storedModules"]
	t.state["storedModules"] = next
	t.mark("storedModules", at, "journal", next)
	return !reflect.DeepEqual(prev, next)
}

func inventoryRows(rawRows []M) ([]M, any, bool) {
	rows := []M{}
	indexes := map[string]int{}
	total := 0
	truncated := false
	for _, raw := range rawRows {
		symbol := clean(raw["Name"])
		count, ok := numeric(raw["Count"], 0, 1e9)
		if symbol == "" || !ok || v.Num(count) != float64(v.Int(count)) {
			truncated = true
			continue
		}
		owner, mission := exactID(raw["OwnerID"]), exactID(raw["MissionID"])
		key := v.Key(symbol) + "|" + owner + "|" + mission
		if index, exists := indexes[key]; exists {
			if v.Num(rows[index]["count"]) != v.Num(count) {
				truncated = true
			}
			continue
		}
		if len(rows) >= maxInventoryRows {
			truncated = true
			break
		}
		indexes[key] = len(rows)
		total += v.Int(count)
		rows = append(rows, M{"key": key, "symbol": symbol, "name": v.Text(clean(raw["Name_Localised"]), label(symbol)), "count": count, "ownerId": owner, "missionId": mission})
	}
	sort.Slice(rows, func(i, j int) bool { return v.Str(rows[i]["key"]) < v.Str(rows[j]["key"]) })
	if truncated {
		return rows, nil, true
	}
	return rows, total, false
}

func (t *Tracker) inventory(event M, container string, fields map[string]string, at string) bool {
	groups := v.Obj(v.At(t.state, "inventory", container))
	changed := false
	for field, key := range fields {
		stampKey := "inventory." + container + "." + key
		if !t.fresh(stampKey, at, "journal") {
			continue
		}
		rawRows, ok := arrayRows(event[field])
		if !ok {
			continue
		}
		rows, total, truncated := inventoryRows(rawRows)
		next := M{"known": true, "entries": rows, "count": total, "truncated": truncated, "updatedAt": at, "source": "journal"}
		changed = !reflect.DeepEqual(groups[key], next) || changed
		groups[key] = next
		t.mark(stampKey, at, "journal", next)
	}
	return changed
}

func (t *Tracker) statistics(event M, at string) bool {
	if !t.fresh("statistics", at, "journal") {
		return false
	}
	sections := []M{}
	truncated := false
	for _, name := range sortedKeys(event) {
		if name == "event" || name == "timestamp" {
			continue
		}
		raw, ok := event[name].(M)
		if !ok {
			continue
		}
		if len(sections) >= maxStatisticsSections {
			truncated = true
			break
		}
		entries := []M{}
		for _, key := range sortedKeys(raw) {
			value, ok := numeric(raw[key], -1e18, 1e18)
			if !ok {
				continue
			}
			if len(entries) >= maxStatisticsFields {
				truncated = true
				break
			}
			entries = append(entries, M{"id": key, "label": label(key), "value": value})
		}
		if len(entries) > 0 {
			sections = append(sections, M{"id": name, "label": label(name), "entries": entries})
		}
	}
	if len(sections) == 0 {
		return false
	}
	next := M{"known": true, "updatedAt": at, "source": "journal", "sections": sections, "truncated": truncated}
	prev := t.state["statistics"]
	t.state["statistics"] = next
	t.mark("statistics", at, "journal", next)
	changed := !reflect.DeepEqual(prev, next)
	return t.field("finance", "netWorth", v.At(event, "Bank_Account", "Current_Wealth"), at, "journal", 0, 1e18) || changed
}

// ApplyCAPI accepts only an already identity-verified profile of the active FID.
// The profile's ship is the current ship; lastStarport shipyard offers are never
// an owned fleet. Optional profile.ships observations remain explicitly partial.
func (t *Tracker) ApplyCAPI(profile, context M) bool {
	if context["identityVerified"] != true {
		return false
	}
	owner := clean(v.At(context, "commander", "fid"))
	expected := clean(v.At(context, "commander", "name"))
	name := clean(v.At(profile, "commander", "name"))
	at := timestamp(context["observedAt"])
	if owner == "" || expected == "" || name == "" || !strings.EqualFold(name, expected) || at == "" {
		return false
	}
	if fid := clean(v.At(profile, "commander", "fid")); fid != "" && fid != owner {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.owner != "" && t.owner != owner {
		return false
	}
	if t.owner == "" {
		t.clear(owner)
		t.ownerAt = at
	}
	changed := false
	for field, key := range map[string]string{"credits": "credits", "debt": "loan"} {
		changed = t.field("finance", key, v.At(profile, "commander", field), at, "capi", 0, 1e18) || changed
	}
	ship := v.Obj(profile["ship"])
	if len(ship) > 0 {
		changed = t.currentShip(M{"event": "Loadout", "timestamp": at, "Ship": ship["name"], "Ship_Localised": ship["locName"], "ShipID": ship["id"], "ShipName": ship["shipName"], "ShipIdent": ship["shipID"]}, at, "capi", context) || changed
	}
	if !t.fresh("fleet", at, "capi") {
		return changed
	}
	var rawRows []M
	inputTruncated := false
	switch raw := profile["ships"].(type) {
	case M:
		for _, key := range sortedKeys(raw) {
			row, ok := raw[key].(M)
			if !ok {
				inputTruncated = true
				continue
			}
			copy := v.Copy(row)
			if copy["id"] == nil {
				copy["id"] = key
			}
			rawRows = append(rawRows, copy)
			if len(rawRows) > maxShips {
				break
			}
		}
	case []M:
		rawRows = raw
	case []any:
		var ok bool
		rawRows, ok = arrayRows(raw)
		if !ok {
			return changed
		}
	default:
		return changed
	}
	rows := []M{}
	seen := map[string]bool{}
	truncated := inputTruncated
	for _, raw := range rawRows {
		row := shipRow(raw, false, nil)
		if row == nil {
			truncated = true
			continue
		}
		id := v.Str(row["id"])
		if seen[id] {
			continue
		}
		seen[id] = true
		if len(rows) >= maxShips {
			truncated = true
			break
		}
		rows = append(rows, row)
	}
	prev := v.Obj(t.state["fleet"])
	next := M{"known": true, "updatedAt": at, "source": "capi", "entries": rows, "count": len(rows), "truncated": truncated, "complete": false, "scope": "profileShips", "currentShip": prev["currentShip"]}
	if truncated {
		next["count"] = nil
	}
	t.state["fleet"] = next
	t.mark("fleet", at, "capi", next)
	return !reflect.DeepEqual(prev, next) || changed
}
