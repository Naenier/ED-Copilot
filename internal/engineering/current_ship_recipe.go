package engineering

import (
	"errors"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (p *Planner) currentShipDetail(current M, choices []M, selected M, q roadmapQuery, input, observations M) (M, error) {
	if q.blueprint != "" {
		selected = shipBlueprintChoice(choices, q.blueprint)
		if selected == nil {
			return nil, errors.New("Чертёж несовместим с выбранным модулем")
		}
	}
	module, symbol := v.Str(current["moduleKey"]), v.Str(current["symbol"])
	experimentals := []M{}
	var validEffect bool
	if _, chosen := input["experimentalKey"]; !chosen && selected["key"] == current["blueprintKey"] {
		q.effect = v.Str(current["experimentalKey"])
	}
	for _, raw := range v.Arr(modules[module]["experimentals"]) {
		key := v.Str(raw)
		if v.Bool(roadmapData.effectExclusions[symbol][key]) {
			continue
		}
		validEffect = validEffect || q.effect == key
		row := roadLabel(effects[key])
		row["materials"] = p.ingredientRows(v.Rows(effects[key]["materials"]), 1, false, observations)
		experimentals = append(experimentals, row)
	}
	if q.effect != "" && !validEffect {
		if _, chosen := input["experimentalKey"]; chosen {
			return nil, errors.New("Экспериментальный эффект несовместим с выбранным модулем")
		}
		q.effect = ""
	}
	if q.grade == 0 {
		q.grade = v.Int(selected["maxGrade"])
	}
	if q.grade > v.Int(selected["maxGrade"]) {
		return nil, errors.New("У выбранного чертежа нет такой ступени улучшения")
	}
	detail := M{"slot": current["slot"], "blueprints": choices, "blueprintKey": v.Str(selected["key"]),
		"grade": q.grade, "experimentalKey": q.effect, "experimentals": experimentals, "unlockSteps": []M{}}
	if selected == nil {
		return detail, nil
	}
	if _, exists := v.Obj(blueprints[v.Str(selected["key"])]["grades"])[v.Str(q.grade)]; !exists {
		return nil, errors.New("У выбранного чертежа нет такой ступени улучшения")
	}
	var best, available M
	keys := []string{}
	for _, engineer := range v.Rows(selected["engineers"]) {
		if available == nil && engineer["status"] == "unlocked" {
			available = engineer
		}
		if v.Int(engineer["maxGrade"]) < q.grade {
			continue
		}
		keys = append(keys, v.Str(engineer["key"]))
		if best == nil {
			best = engineer
		}
	}
	if best == nil {
		return detail, nil
	}
	key := v.Str(best["key"])
	detail["engineer"], detail["availableEngineer"] = best, available
	detail["unlockSteps"] = p.currentShipUnlockSteps(key, q.grade, observations)
	recipe := M{"moduleKey": module, "blueprintKey": selected["key"], "experimentalKey": q.effect,
		"kind": "ship", "grade": q.grade, "engineerKeys": keys, "engineer": best, "compatible": true,
		"confidence": "exact", "conditions": []string{}, "grades": []M{}, "experimentalMaterials": []M{}, "totals": []M{}}
	p.shipRecipe(recipe, blueprints[v.Str(selected["key"])], current, observations)
	p.qualifyCurrentShipBudget(recipe, current, observations)
	detail["recipe"] = recipe
	return detail, nil
}

func (p *Planner) currentShipUnlockSteps(key string, grade int, observations M) []M {
	rows := []M{}
	for _, id := range p.roadmapOrder([]string{key}) {
		engineer := p.roadmapEngineerState(id)
		steps := []M{}
		for _, requirement := range v.Rows(roadmapData.engineers[id]["requirements"]) {
			step := p.roadmapRequirement(id, engineer, requirement, observations)
			if step["status"] != "done" {
				steps = append(steps, step)
			}
		}
		if id == key && v.Int(engineer["rank"]) < grade {
			step := p.roadmapRankStep(id, engineer)
			step["target"] = grade
			if grade != 5 {
				step["text"] = "Поднимите репутацию до G" + v.Str(grade) + ", выполняя модификации лично в мастерской."
				step["english"] = "Reach Grade " + v.Str(grade) + " reputation by crafting in person at the workshop."
			}
			steps = append(steps, step)
		}
		if len(steps) > 0 {
			engineer["steps"] = steps
			rows = append(rows, engineer)
		}
	}
	return rows
}

// Frontier's 18.08 fixed roll count is verified at rank 5. A percentage in
// Loadout is progress through the current grade, not the number of rolls spent.
func (p *Planner) qualifyCurrentShipBudget(recipe, current, observations M) {
	if !v.Bool(recipe["compatible"]) {
		return
	}
	game := v.Obj(observations["game"])
	live, liveKnown := game["live"].(bool)
	fixed := fixedRollsVersion(v.Str(game["version"])) && (!liveKnown || live)
	engineer := v.Obj(recipe["engineer"])
	verifiedRank := engineer["status"] == "unlocked" && v.Int(engineer["rank"]) == 5
	totals, missing := map[string]int{}, false
	for _, step := range v.Rows(recipe["grades"]) {
		if step["status"] == "done" {
			continue
		}
		if !fixed || !verifiedRank || step["status"] == "unknown" || current["unmappedBlueprint"] != nil {
			step["status"], step["materials"] = "unknown", []M{}
			delete(step, "rolls")
			delete(step, "basis")
			missing = true
			continue
		}
		accumulateIngredients(totals, v.Rows(step["materials"]), 1)
	}
	if recipe["experimentalStatus"] == "unknown" {
		recipe["experimentalMaterials"] = []M{}
		missing = true
	} else {
		accumulateIngredients(totals, v.Rows(recipe["experimentalMaterials"]), 1)
	}
	materials, currencies := []M{}, []M{}
	for _, row := range p.ingredientRows(ingredientTotals(totals), 1, false, observations) {
		if row["category"] == "currency" {
			currencies = append(currencies, row)
		} else {
			materials = append(materials, row)
		}
	}
	recipe["totals"], recipe["currencies"], recipe["totalsPartial"] = materials, currencies, missing
	if missing {
		recipe["confidence"] = "unknown"
		if len(totals) > 0 {
			recipe["confidence"] = "estimate"
		}
	}
}
