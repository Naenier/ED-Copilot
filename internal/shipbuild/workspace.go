package shipbuild

import (
	"errors"
	"reflect"
	"sort"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/ships"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const baselineID = "baseline"

func selectedBuild(saved M, id string) M {
	if id == "" {
		id = v.Str(saved["selectedId"])
	}
	if id == baselineID {
		if b, ok := saved["baseline"].(M); ok {
			return b
		}
	}
	for _, row := range v.Rows(saved["targets"]) {
		if row["id"] == id {
			return row
		}
	}
	return nil
}

// View shows the saved baseline and plans, plus a read-only preview when
// supplied. Ownership is checked by the application before this call.
func View(saved, observedLoadout, request M) (M, error) {
	view := M{"targets": []M{}, "selectedId": saved["selectedId"]}
	observed := journalBuild(observedLoadout, nil, "journal")
	if observed != nil {
		view["observed"] = buildView(observed)
	}
	baseline := selectedBuild(saved, baselineID)
	if baseline != nil {
		view["baseline"] = buildView(baseline)
	}
	target := selectedBuild(saved, "")
	targets := []M{}
	for _, row := range v.Rows(saved["targets"]) {
		targets = append(targets, M{"id": row["id"], "name": row["name"], "format": row["format"], "ship": row["ship"]})
	}
	view["targets"] = targets
	if raw := v.Str(request["text"]); raw != "" {
		parsed, err := Parse(raw)
		if err != nil {
			return nil, err
		}
		preview := make([]M, 0, len(parsed))
		for _, build := range parsed {
			preview = append(preview, buildView(build))
		}
		view["preview"] = preview
	}
	if target != nil {
		view["target"] = buildView(target)
		if baseline == nil || v.Bool(request["liveBaseline"]) {
			baseline = observed
		}
		if baseline != nil && target["id"] != baselineID {
			comparison := compareBuilds(baseline, target, request["storedModules"])
			view["comparison"], view["upgradeGoals"] = comparison, comparison["upgradeGoals"]
		}
	}
	return view, nil
}

// Update validates and returns a detached persistent workspace and its view.
func Update(saved, observedLoadout, request M) (M, M, error) {
	next := v.Copy(saved)
	if next == nil {
		next = M{}
	}
	targets := v.Rows(next["targets"])
	action := v.Str(request["action"])
	switch action {
	case "baseline", "copy":
		var build M
		if action == "baseline" {
			build = journalBuild(observedLoadout, nil, "journal")
			if build == nil {
				return nil, nil, errors.New("В журнале пока нет оснащения текущего корабля.")
			}
			build["id"] = baselineID
			next["baseline"] = build
			break
		}
		original := selectedBuild(next, v.Str(request["id"]))
		if original == nil {
			return nil, nil, errors.New("Сборка не найдена.")
		}
		if len(targets) >= maxBuilds {
			return nil, nil, errors.New("Достигнут предел сохранённых сборок.")
		}
		build = v.Copy(original)
		build["id"] = v.ID()
		if title := label(request["name"], 160); title != "" {
			build["name"] = title
		}
		targets, next["selectedId"] = append(targets, build), build["id"]
	case "import":
		if len(targets) >= maxBuilds {
			return nil, nil, errors.New("Достигнут предел сохранённых сборок.")
		}
		parsed, err := Parse(v.Str(request["text"]))
		if err != nil {
			return nil, nil, err
		}
		index := v.Int(request["index"])
		if index < 0 || index >= len(parsed) {
			return nil, nil, errors.New("Выберите сборку из импорта.")
		}
		build := parsed[index]
		if title := label(request["name"], 160); title != "" {
			build["name"] = title
		}
		build["id"] = v.ID()
		targets, next["selectedId"] = append(targets, build), build["id"]
	case "select", "remove", "replace":
		id := v.Str(request["id"])
		target := selectedBuild(next, id)
		if id == "" || id == baselineID || target == nil {
			return nil, nil, errors.New("Сборка не найдена.")
		}
		switch action {
		case "select":
			next["selectedId"] = id
		case "remove":
			for i, row := range targets {
				if row["id"] == id {
					targets = append(targets[:i], targets[i+1:]...)
					break
				}
			}
			if next["selectedId"] == id {
				next["selectedId"] = ""
				if len(targets) > 0 {
					next["selectedId"] = targets[0]["id"]
				}
			}
		case "replace":
			if err := replace(target, request); err != nil {
				return nil, nil, err
			}
		}
	default:
		return nil, nil, errors.New("Неизвестное действие со сборкой.")
	}
	next["targets"] = targets
	view, err := View(next, observedLoadout, M{"storedModules": request["storedModules"]})
	return next, view, err
}

func replace(target, request M) error {
	if target["format"] == "coriolis" {
		return errors.New("Для замены модуля импортируйте сборку Coriolis в формате SLEF.")
	}
	slot := strings.ToLower(label(request["slot"], 120))
	item := label(request["item"], 120)
	if slot == "" || !symbol.MatchString(item) {
		return errors.New("Укажите слот и игровой идентификатор модуля.")
	}
	payload := v.Obj(target["payload"])
	for _, row := range v.Rows(payload["Modules"]) {
		if strings.ToLower(v.Str(row["Slot"])) != slot {
			continue
		}
		changed := !strings.EqualFold(item, v.Str(row["Item"]))
		if changed {
			for key := range row {
				if key != "Slot" && key != "On" && key != "Priority" {
					delete(row, key)
				}
			}
			row["Item"] = item
		}
		if on, ok := request["on"].(bool); ok {
			row["On"] = on
		}
		if raw, supplied := request["priority"]; supplied {
			value, known := v.Number(raw)
			if !known || value < 0 || value > 4 || value != float64(int(value)) {
				return errors.New("Приоритет питания должен быть от 1 до 5.")
			}
			row["Priority"] = int(value)
		}
		for _, key := range []string{"PowerCapacity", "PowerDraw", "PowerRetracted", "PowerDeployed"} {
			delete(payload, key)
		}
		if changed {
			for _, key := range []string{"UnladenMass", "LadenMass", "CargoCapacity", "MaxJumpRange", "UnladenJumpRange", "LadenJumpRange", "ModulesValue", "Rebuy"} {
				delete(payload, key)
			}
		}
		// A link to the imported fit would now open a different build.
		delete(v.Obj(target["header"]), "appURL")
		return nil
	}
	return errors.New("Слот не найден в сборке.")
}

// ApplyObservedCraft updates the matching currently installed module. A
// crafting event cannot establish the new ship-wide numerical performance.
func ApplyObservedCraft(loadout, event M) M {
	slot, item := v.Str(event["Slot"]), journalItemSymbol(event["Module"])
	if slot == "" || item == "" || v.Bool(event["IsPreview"]) ||
		event["ShipID"] != nil && v.Str(event["ShipID"]) != v.Str(loadout["ShipID"]) ||
		v.Time(event["timestamp"]).Before(v.Time(loadout["timestamp"])) {
		return loadout
	}
	for index, row := range v.Rows(loadout["Modules"]) {
		if !strings.EqualFold(slot, v.Str(row["Slot"])) || item != journalItemSymbol(row["Item"]) ||
			v.Time(event["timestamp"]).Before(v.Time(row["EngineeringObservedAt"])) {
			continue
		}
		next := v.Copy(loadout)
		module := v.Rows(next["Modules"])[index]
		engineering := v.Obj(module["Engineering"])
		if blueprint := v.Str(event["BlueprintName"]); blueprint != "" && !strings.EqualFold(blueprint, v.Str(engineering["BlueprintName"])) {
			engineering = M{}
		}
		if event["Level"] != nil && v.Int(event["Level"]) != v.Int(engineering["Level"]) {
			delete(engineering, "Quality")
		}
		for _, field := range []string{"BlueprintName", "BlueprintID", "Level", "Quality", "Engineer", "EngineerID", "ExperimentalEffect", "ExperimentalEffect_Localised"} {
			if value := event[field]; value != nil {
				engineering[field] = v.Clone(value)
			}
		}
		if effect := event["ApplyExperimentalEffect"]; effect != nil {
			engineering["ExperimentalEffect"] = v.Clone(effect)
		}
		if modifiers := event["Modifiers"]; modifiers != nil {
			engineering["Modifiers"] = v.Clone(modifiers)
		} else {
			delete(engineering, "Modifiers")
		}
		module["Engineering"] = engineering
		if event["timestamp"] != nil {
			module["EngineeringObservedAt"] = event["timestamp"]
		}
		for _, field := range []string{"UnladenMass", "LadenMass", "MaxJumpRange", "UnladenJumpRange", "LadenJumpRange", "PowerCapacity", "PowerDraw", "PowerRetracted", "PowerDeployed"} {
			delete(next, field)
		}
		return next
	}
	return loadout
}

func journalItemSymbol(raw any) string {
	item := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v.Str(raw))), "$")
	return strings.TrimSuffix(strings.TrimSuffix(item, ";"), "_name")
}

func buildView(build M) M {
	payload := v.Obj(build["payload"])
	provider := ""
	var slots []M
	stats := M{}
	if build["format"] == "coriolis" {
		provider = externalLink(v.At(payload, "references", "coriolis"), "coriolis")
		for _, reference := range v.Rows(payload["references"]) {
			if provider == "" {
				provider = externalLink(reference["url"], "coriolis")
			}
		}
		slots = coriolisSlots(v.Obj(payload["components"]))
		known := v.Obj(payload["stats"])
		for key, field := range map[string]string{"unladenMass": "unladenMass", "ladenMass": "ladenMass", "cargo": "cargoCapacity",
			"unladenRange": "unladenRange", "ladenRange": "ladenRange", "powerAvailable": "powerAvailable",
			"powerRetracted": "powerRetracted", "powerDeployed": "powerDeployed", "cost": "totalCost"} {
			stats[key] = nonnegative(known[field])
		}
	} else {
		provider = externalLink(v.At(build, "header", "appURL"), "edsy")
		if provider == "" {
			provider = externalLink(v.At(build, "header", "appURL"), "coriolis")
		}
		slots = journalSlots(build)
		for key, field := range map[string]string{"unladenMass": "UnladenMass", "ladenMass": "LadenMass", "cargo": "CargoCapacity",
			"unladenRange": "UnladenJumpRange", "ladenRange": "LadenJumpRange", "maximumRange": "MaxJumpRange",
			"powerAvailable": "PowerCapacity", "powerRetracted": "PowerRetracted", "powerDeployed": "PowerDeployed"} {
			stats[key] = nonnegative(payload[field])
		}
		hull, modules := nonnegative(payload["HullValue"]), nonnegative(payload["ModulesValue"])
		if hull != nil && modules != nil {
			stats["cost"] = v.Num(hull) + v.Num(modules)
		}
	}
	return M{"id": build["id"], "name": build["name"], "ship": ships.DisplayShip(v.Str(build["ship"])),
		"format": build["format"], "observedAt": payload["timestamp"], "slots": slots, "stats": stats, "providerURL": provider}
}

func nonnegative(value any) any {
	if n, ok := v.Number(value); ok && n >= 0 {
		return n
	}
	return nil
}

func journalSlots(build M) []M {
	slots := []M{}
	for _, raw := range v.Rows(v.At(build, "payload", "Modules")) {
		slots = append(slots, M{"slot": raw["Slot"], "item": raw["Item"], "name": v.Text(raw["Item_Localised"], raw["Item"]),
			"on": raw["On"], "priority": raw["Priority"], "engineering": raw["Engineering"], "raw": raw,
			"engineeringKnown": build["format"] == "journal" || raw["Engineering"] != nil})
	}
	return slots
}

func coriolisSlots(components M) []M {
	slots := []M{}
	put := func(slot string, item any) {
		raw := v.Obj(item)
		if item == nil {
			slots = append(slots, M{"slot": slot, "empty": true})
			return
		}
		itemSymbol := v.Text(raw["symbol"], raw["fdname"], raw["item"])
		title := v.Text(raw["name"], raw["group"], raw["id"])
		if len(raw) == 0 {
			title = v.Str(item)
		}
		if title == "" {
			title = slot
		}
		if raw["class"] != nil && raw["rating"] != nil {
			title = v.Str(raw["class"]) + v.Str(raw["rating"]) + " · " + title
		}
		slots = append(slots, M{"slot": slot, "item": itemSymbol, "name": title, "on": raw["enabled"], "priority": raw["priority"],
			"engineering": M{"blueprint": raw["blueprint"], "modifications": raw["modifications"]}, "raw": item})
	}
	core := v.Obj(components["standard"])
	keys := make([]string, 0, len(core))
	for key := range core {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		put(key, core[key])
	}
	for _, group := range []string{"hardpoints", "utility", "internal", "military"} {
		for i, item := range v.Arr(components[group]) {
			put(group+" "+v.Str(i+1), item)
		}
	}
	groups := []string{}
	for key, value := range components {
		if key != "standard" && key != "hardpoints" && key != "utility" && key != "internal" && key != "military" && v.Arr(value) != nil {
			groups = append(groups, key)
		}
	}
	sort.Strings(groups)
	for _, group := range groups {
		for i, item := range v.Arr(components[group]) {
			put(group+" "+v.Str(i+1), item)
		}
	}
	return slots
}

func compareBuilds(baseline, target M, stored any) M {
	before, after := buildView(baseline), buildView(target)
	left, right := v.Obj(before["stats"]), v.Obj(after["stats"])
	stats := M{}
	for _, key := range []string{"unladenMass", "ladenMass", "cargo", "maximumRange", "unladenRange", "ladenRange", "powerAvailable", "powerRetracted", "powerDeployed", "cost"} {
		var delta any
		if left[key] != nil && right[key] != nil {
			delta = v.Num(right[key]) - v.Num(left[key])
		}
		stats[key] = M{"before": left[key], "after": right[key], "delta": delta}
	}
	result := M{"stats": stats, "slots": []M{}, "shopping": []M{}, "upgradeGoals": []M{}, "compatibleLayout": false}
	if ships.DisplayShip(v.Str(baseline["ship"])) != ships.DisplayShip(v.Str(target["ship"])) ||
		(baseline["format"] == "coriolis") != (target["format"] == "coriolis") {
		return result
	}
	result["compatibleLayout"] = true
	result["slots"], result["shopping"], result["upgradeGoals"] = compareSlots(v.Rows(before["slots"]), v.Rows(after["slots"]), stored, target["format"] == "coriolis")
	return result
}

func compareSlots(before, after []M, stored any, coriolis bool) ([]M, []M, []M) {
	left, right := map[string]M{}, map[string]M{}
	stock := map[string][]M{}
	moduleKey := func(value any) string { return strings.ToLower(v.Str(value)) }
	for _, row := range before {
		left[strings.ToLower(v.Str(row["slot"]))] = row
	}
	for _, row := range after {
		right[strings.ToLower(v.Str(row["slot"]))] = row
	}
	for _, row := range before {
		key := strings.ToLower(v.Str(row["slot"]))
		item := moduleKey(row["item"])
		if item != "" && item != moduleKey(right[key]["item"]) {
			stock[item] = append(stock[item], v.Obj(row["raw"]))
		}
	}
	for _, row := range v.Rows(stored) {
		if item := moduleKey(v.First(row["Name"], row["Item"])); item != "" {
			stock[item] = append(stock[item], nil)
		}
	}
	rows, shopping, upgrades := []M{}, []M{}, []M{}
	for _, row := range after {
		slot := v.Str(row["slot"])
		previous := left[strings.ToLower(slot)]
		item, old := moduleKey(row["item"]), moduleKey(previous["item"])
		change := "same"
		baseline := v.Obj(previous["raw"])
		if coriolis {
			if !reflect.DeepEqual(previous["raw"], row["raw"]) {
				change = "changed"
			}
		} else if item != old {
			change = "replace"
			baseline = nil
			if copies := stock[item]; len(copies) > 0 {
				index := 0
				for i, candidate := range copies {
					if candidate != nil && reflect.DeepEqual(candidate["Engineering"], row["engineering"]) {
						index = i
						break
					}
				}
				baseline, stock[item] = copies[index], append(copies[:index], copies[index+1:]...)
				if baseline == nil {
					baseline = M{"Item": row["item"], "StoredUnknownEngineering": true}
				}
			} else if item != "" {
				shopping = append(shopping, M{"slot": slot, "item": row["item"], "name": row["name"]})
			}
		} else if !reflect.DeepEqual(previous["engineering"], row["engineering"]) ||
			!reflect.DeepEqual(previous["on"], row["on"]) || !reflect.DeepEqual(previous["priority"], row["priority"]) {
			change = "changed"
		}
		rows = append(rows, M{"slot": slot, "before": previous, "after": row, "change": change})
		if !coriolis && len(v.Obj(row["engineering"])) > 0 && !reflect.DeepEqual(baseline["Engineering"], row["engineering"]) {
			upgrades = append(upgrades, M{"slot": slot, "item": row["item"], "target": row["engineering"], "baseline": baseline})
		}
	}
	for _, row := range before {
		if right[strings.ToLower(v.Str(row["slot"]))] == nil {
			rows = append(rows, M{"slot": row["slot"], "before": row, "change": "unobserved"})
		}
	}
	return rows, shopping, upgrades
}
