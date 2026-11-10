package engineering

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/journal"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (p *Planner) roadmapRecipe(q roadmapQuery, ids []string, equipment []M, observations M) (M, error) {
	if q.module == "" || q.blueprint == "" && q.effect == "" {
		return nil, nil
	}
	module, bp := modules[q.module], roadmapData.blueprint(q.blueprint)
	personal := roadmapData.modules[q.module]["kind"] == "odyssey"
	if q.effect != "" && !contains(module["experimentals"], q.effect) {
		return nil, fmt.Errorf("Экспериментальный эффект несовместим с модулем")
	}
	if q.blueprint != "" && (personal && bp["moduleKey"] != q.module || !personal && !contains(module["blueprints"], q.blueprint)) {
		return nil, fmt.Errorf("Выберите совместимые модуль и чертёж")
	}
	grade := q.grade
	if grade == 0 && q.blueprint != "" {
		for _, key := range ids {
			for _, cap := range roadmapData.capabilities[key] {
				if q.capabilityMatches(cap) {
					grade = max(grade, cap.maxGrade)
				}
			}
		}
	}
	candidates := []string{}
	var best M
	for _, key := range ids {
		check := q
		check.grade = grade
		if !engineerHasCapability(key, check) {
			continue
		}
		candidates = append(candidates, key)
		state := p.roadmapEngineerState(key)
		if best == nil || recipeAccessScore(state) > recipeAccessScore(best) {
			best = state
		}
	}
	if best == nil {
		return nil, nil
	}
	var current M
	for _, item := range equipment {
		if item["key"] == q.equipment {
			current = item
			break
		}
	}
	recipe := M{"moduleKey": q.module, "blueprintKey": q.blueprint, "experimentalKey": q.effect, "kind": best["kind"], "grade": grade,
		"engineerKeys": candidates, "engineer": best, "current": current, "compatible": true,
		"confidence": "exact", "conditions": []string{}, "grades": []M{}, "experimentalMaterials": []M{}, "totals": []M{}}
	if q.equipment != "" && current == nil {
		recipe["conditions"] = []string{"equipment-unavailable"}
	}
	if personal && journal.GameMode(v.Str(v.At(observations, "game", "version"))) == "legacy" {
		recipe["compatible"], recipe["totals"] = false, nil
		recipe["conditions"] = append(recipe["conditions"].([]string), "live-only")
		return recipe, nil
	}
	if personal {
		p.odysseyRecipe(recipe, bp, current, observations)
	} else {
		p.shipRecipe(recipe, bp, current, observations)
	}
	return recipe, nil
}

func recipeAccessScore(engineer M) int {
	rank := v.Int(engineer["rank"])
	switch engineer["status"] {
	case "unlocked":
		return 100 + rank
	case "invited", "acquainted":
		return 50
	case "known":
		return 20
	case "barred":
		return 0
	default:
		return 10
	}
}

func (p *Planner) shipRecipe(recipe, bp, current, observations M) {
	engineer := v.Obj(recipe["engineer"])
	game := v.Obj(observations["game"])
	fixed := fixedRollsVersion(v.Str(game["version"]))
	if live, explicit := game["live"].(bool); explicit && !live {
		fixed = false
	}
	conditions, _ := recipe["conditions"].([]string)
	if v.Bool(bp["liveOnly"]) && journal.GameMode(v.Str(game["version"])) == "legacy" {
		recipe["compatible"], recipe["totals"] = false, nil
		recipe["conditions"] = append(conditions, "live-only")
		return
	}
	if key := v.Str(recipe["experimentalKey"]); key != "" && v.Bool(roadmapData.effectExclusions[v.Str(current["symbol"])][key]) {
		recipe["compatible"], recipe["totals"] = false, nil
		recipe["conditions"] = append(conditions, "experimental-incompatible")
		return
	}
	if engineer["status"] != "unlocked" {
		conditions = append(conditions, "unlock-engineer")
	} else if engineer["rank"] != nil && v.Int(engineer["rank"]) < v.Int(recipe["grade"]) {
		conditions = append(conditions, "rank-engineer")
	}
	if current != nil && v.Str(current["blueprintKey"]) != "" && current["blueprintKey"] != bp["key"] && bp["key"] != nil {
		conditions = append(conditions, "replaces-blueprint")
	}
	if current != nil && current["unmappedBlueprint"] != nil {
		conditions = append(conditions, "unmapped-blueprint")
	}
	sameBlueprint := bp["key"] != nil && current != nil && current["blueprintKey"] == bp["key"]
	knownGrade := v.Int(current["grade"])
	grades, totals := []M{}, map[string]int{}
	for g := 1; g <= v.Int(recipe["grade"]); g++ {
		raw := v.Rows(v.Obj(bp["grades"])[strconv.Itoa(g)])
		if len(raw) == 0 {
			continue
		}
		step := M{"grade": g, "status": "pending", "perRoll": p.ingredientRows(raw, 1, false, observations), "materials": []M{}}
		if sameBlueprint && (knownGrade > g || knownGrade == g && v.Num(current["quality"]) >= 1) {
			step["status"], step["rolls"] = "done", 0
			grades = append(grades, step)
			continue
		}
		if sameBlueprint && knownGrade == g || current != nil && current["unmappedBlueprint"] != nil || len(conditions) != 0 && conditions[0] == "equipment-unavailable" {
			step["status"] = "unknown"
		}
		if version := v.Str(game["version"]); version != "" && !fixed {
			// Legacy and old observations cannot use the live fixed-roll rule.
			recipe["confidence"] = "unknown"
			grades = append(grades, step)
			continue
		}
		step["rolls"] = g
		step["basis"] = "g5"
		if !fixed || v.Int(engineer["rank"]) != 5 || step["status"] == "unknown" {
			recipe["confidence"] = "estimate"
		}
		step["materials"] = p.ingredientRows(raw, g, false, observations)
		accumulateIngredients(totals, raw, g)
		grades = append(grades, step)
	}
	recipe["grades"] = grades
	if key := v.Str(recipe["experimentalKey"]); key != "" {
		if bp["key"] == nil && v.Int(current["grade"]) == 0 {
			conditions = append(conditions, "requires-blueprint")
		}
		raw := v.Rows(effects[key]["materials"])
		recipe["experimentalStatus"] = "pending"
		switch {
		case current != nil && current["experimentalKey"] == key && (bp["key"] == nil || sameBlueprint):
			recipe["experimentalStatus"] = "done"
		case current != nil && !v.Bool(current["effectKnown"]) && (bp["key"] == nil || sameBlueprint):
			recipe["experimentalStatus"] = "unknown"
			if recipe["confidence"] == "exact" {
				recipe["confidence"] = "estimate"
			}
			fallthrough
		default:
			recipe["experimentalMaterials"] = p.ingredientRows(raw, 1, false, observations)
			accumulateIngredients(totals, raw, 1)
		}
	}
	if recipe["confidence"] == "unknown" {
		recipe["totals"] = nil
	} else {
		materials, currencies := []M{}, []M{}
		for _, row := range p.ingredientRows(ingredientTotals(totals), 1, false, observations) {
			if row["category"] == "currency" {
				currencies = append(currencies, row)
			} else {
				materials = append(materials, row)
			}
		}
		recipe["totals"], recipe["currencies"] = materials, currencies
	}
	recipe["conditions"] = conditions
}

func fixedRollsVersion(version string) bool {
	if journal.GameMode(version) != "live" {
		return false
	}
	parts := strings.Split(strings.TrimSpace(version), ".")
	want := []int{4, 0, 0, 1808}
	for index, minimum := range want {
		if index >= len(parts) {
			return false
		}
		value, err := strconv.Atoi(parts[index])
		if err != nil {
			return false
		}
		if value != minimum {
			return value > minimum
		}
	}
	return true
}

func (p *Planner) odysseyRecipe(recipe, bp, current, observations M) {
	conditions, _ := recipe["conditions"].([]string)
	if v.At(recipe, "engineer", "status") != "unlocked" {
		conditions = append(conditions, "unlock-engineer")
	}
	recipe["description"], recipe["descriptionEnglish"] = bp["description"], bp["descriptionEnglish"]
	recipe["modifiers"], recipe["damageType"] = v.Clone(bp["modifiers"]), bp["damageType"]
	required, ok := bp["damageType"].(string)
	if ok && current != nil {
		symbol := v.Str(current["symbol"])
		switch {
		case strings.Contains(symbol, "_"+required+"_"):
		case strings.Contains(symbol, "_laser_") || strings.Contains(symbol, "_kinetic_") || strings.Contains(symbol, "_plasma_"):
			recipe["compatible"], recipe["conditions"], recipe["totals"] = false, append(conditions, "weapon-incompatible"), nil
			return
		default:
			conditions = append(conditions, "weapon-type-unknown")
		}
	}
	raw := v.Rows(bp["materials"])
	row := M{"grade": 1, "rolls": 1, "status": "pending", "materials": p.ingredientRows(raw, 1, true, observations)}
	totals := map[string]int{}
	mods := v.Arr(current["mods"])
	if current != nil && contains(mods, v.Str(bp["journalSymbol"])) && (!ok || !contains(conditions, "weapon-type-unknown")) {
		row["status"], row["rolls"], row["materials"] = "done", 0, []M{}
	} else {
		accumulateIngredients(totals, raw, 1)
		recipe["credits"] = bp["credits"]
		row["credits"] = bp["credits"]
	}
	if current != nil && row["status"] != "done" {
		p.odysseySlots(recipe, current, observations, &conditions, totals)
	}
	if len(conditions) > 0 && conditions[0] == "equipment-unavailable" {
		recipe["confidence"] = "estimate"
	}
	recipe["grades"] = []M{row}
	if recipe["compatible"] == false {
		recipe["totals"] = nil
	} else {
		recipe["totals"] = p.ingredientRows(ingredientTotals(totals), 1, true, observations)
	}
	recipe["conditions"] = conditions
}

func (p *Planner) odysseySlots(recipe, current, observations M, conditions *[]string, totals map[string]int) {
	grade, known := v.Number(current["grade"])
	mods := v.Arr(current["mods"])
	item := strings.SplitN(v.Str(current["symbol"]), "_class", 2)[0]
	upgradeRows := []M{}
	for _, raw := range roadmapData.personalGrades {
		if raw["itemSymbol"] != item || v.Num(raw["grade"]) <= grade {
			continue
		}
		row := M{"grade": raw["grade"], "credits": raw["credits"], "materials": p.ingredientRows(v.Rows(raw["materials"]), 1, true, observations)}
		upgradeRows = append(upgradeRows, row)
	}
	sort.Slice(upgradeRows, func(i, j int) bool { return v.Int(upgradeRows[i]["grade"]) < v.Int(upgradeRows[j]["grade"]) })
	recipe["equipmentGrades"] = upgradeRows
	if !known || mods == nil && grade > 1 {
		*conditions = append(*conditions, "slots-unknown")
		return
	}
	if len(mods) < int(grade)-1 {
		return
	}
	recipe["needsSlot"] = true
	if grade >= 5 {
		recipe["compatible"] = false
		*conditions = append(*conditions, "no-free-slot")
		return
	}
	*conditions = append(*conditions, "upgrade-equipment")
	if len(upgradeRows) > 0 && v.Num(upgradeRows[0]["grade"]) == grade+1 {
		upgrade := upgradeRows[0]
		recipe["equipmentUpgrade"] = upgrade
		if cost, known := v.Number(upgrade["credits"]); known {
			recipe["credits"] = v.Num(recipe["credits"]) + cost
		}
		for _, m := range v.Rows(upgrade["materials"]) {
			totals[v.Str(m["key"])] += v.Int(m["count"])
		}
	}
}

func accumulateIngredients(totals map[string]int, rows []M, multiplier int) {
	for _, row := range rows {
		totals[v.Str(row["key"])] += v.Int(row["count"]) * multiplier
	}
}

func ingredientTotals(totals map[string]int) []M {
	out := make([]M, 0, len(totals))
	for key, count := range totals {
		out = append(out, M{"key": key, "count": count})
	}
	return out
}

func (p *Planner) ingredientRows(raw []M, multiplier int, personal bool, observations M) []M {
	rows := make([]M, 0, len(raw))
	for _, material := range raw {
		key := v.Str(material["key"])
		label := materials[key]
		if personal {
			label = roadmapData.personalMaterials[key]
		}
		category := v.Str(label["category"])
		count := v.Int(material["count"]) * multiplier
		row := M{"key": key, "name": v.Text(label["name"], key), "english": v.Text(label["english"], key), "category": category, "count": count}
		if len(label) != 0 {
			row["entryId"] = roadmapMaterialEntry(key, category)
		}
		if owned, known := p.roadmapStock(key, category, observations); known {
			row["owned"], row["missing"] = owned, max(0, count-owned)
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return v.Str(rows[i]["key"]) < v.Str(rows[j]["key"]) })
	return rows
}

func roadmapMaterialEntry(key, category string) string {
	switch category {
	case "raw", "manufactured", "encoded", "currency":
		if canonical := MaterialKey(key); canonical != "" {
			return "material/" + canonical
		}
	case "item", "items", "component", "components", "data", "consumable", "consumables":
		if roadmapData.personalMaterials[key] != nil {
			return "material/odyssey-" + key
		}
	}
	return ""
}

func (p *Planner) roadmapStock(key, category string, observations M) (int, bool) {
	switch category {
	case "raw", "manufactured", "encoded":
		canonical := MaterialKey(key)
		return p.stock[canonical], canonical != "" && p.known[category] && !p.uncertain[canonical]
	case "commodity":
		return p.roadmap.cargo[v.Key(key)], p.roadmap.cargoKnown
	case "item", "items", "component", "components", "data", "consumable", "consumables":
		group := map[string]string{"item": "items", "items": "items", "component": "components", "components": "components", "data": "data", "consumable": "consumables", "consumables": "consumables"}[category]
		inventory := v.Obj(v.At(observations, "shipLocker", group))
		if !v.Bool(inventory["known"]) || v.Bool(inventory["truncated"]) || v.Bool(inventory["uncertain"]) {
			return 0, false
		}
		lookup := v.Key(v.Text(roadmapData.personalMaterials[key]["symbol"], key))
		count := 0
		for _, row := range v.Rows(inventory["entries"]) {
			mission := v.Str(row["missionId"])
			if v.Key(row["symbol"]) == lookup && (mission == "" || mission == "0" || mission == "18446744073709551615") {
				count += v.Int(row["count"])
			}
		}
		return count, true
	}
	return 0, false
}

func (p *Planner) specialShipVariants(module string, observations M) []M {
	rows := []M{}
	for _, raw := range roadmapData.specialShipRecipes {
		variants := []M{}
		for _, variant := range v.Rows(raw["variants"]) {
			if shipModule(symbolText(variant["itemSymbol"])) == module {
				variants = append(variants, v.Copy(variant))
			}
		}
		if len(variants) == 0 {
			continue
		}
		row := roadLabel(raw)
		row["acquisition"], row["variants"], row["liveOnly"] = raw["acquisition"], variants, raw["liveOnly"]
		grades := []M{}
		for grade := 1; grade <= 5; grade++ {
			if recipe := v.Rows(v.Obj(raw["grades"])[strconv.Itoa(grade)]); len(recipe) > 0 {
				grades = append(grades, M{"grade": grade, "perRoll": p.ingredientRows(recipe, 1, false, observations)})
			}
		}
		row["grades"] = grades
		rows = append(rows, row)
	}
	return rows
}
