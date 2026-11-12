package engineering

import (
	"sort"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// BuildNeeds resolves observed journal identifiers against the engineering
// catalogue. Unknown effects and stored equipment without known engineering
// keep their material count unknown instead of becoming a stock recipe.
func (p *Planner) BuildNeeds(goals []M, observations M) M {
	p.mu.RLock()
	defer p.mu.RUnlock()
	items := []M{}
	totals := map[string]int{}
	complete := true
	for _, goal := range goals {
		row := p.buildNeed(goal, observations)
		items = append(items, row)
		accumulateIngredients(totals, v.Rows(row["materials"]), 1)
		complete = complete && row["confidence"] == "exact"
	}
	return M{"items": items, "materials": p.ingredientRows(ingredientTotals(totals), 1, false, observations), "complete": complete}
}

func (p *Planner) buildNeed(goal, observations M) M {
	slot, item := v.Str(goal["slot"]), v.Str(goal["item"])
	symbol := symbolText(item)
	module := shipModule(symbol)
	raw := v.Obj(goal["target"])
	wanted := M{"moduleKey": module, "symbol": symbol}
	applyShipUpgrade(wanted, raw)
	bp, effect := v.Str(wanted["blueprintKey"]), v.Str(wanted["experimentalKey"])
	grade := v.Int(wanted["grade"])
	result := M{"id": "build:" + slot, "slot": slot, "item": item, "moduleKey": module,
		"moduleName": modules[module]["name"], "moduleEnglish": modules[module]["english"],
		"blueprintKey": bp, "blueprintName": blueprints[bp]["name"], "blueprintEnglish": blueprints[bp]["english"],
		"experimentalKey": effect, "experimentalName": effects[effect]["name"], "experimentalEnglish": effects[effect]["english"],
		"maxGrade": grade, "confidence": "unknown", "materials": []M{}}
	rawEffect := v.Key(v.First(raw["ExperimentalEffect"], raw["ApplyExperimentalEffect"]))
	if module == "" || wanted["unmappedBlueprint"] != nil || rawEffect != "" && effect == "" || bp == "" && effect == "" || bp != "" && grade == 0 {
		result["reason"] = "unknown-recipe"
		return result
	}
	baseline := v.Obj(goal["baseline"])
	if v.Bool(baseline["StoredUnknownEngineering"]) {
		result["reason"] = "unknown-installed-upgrades"
		return result
	}
	current := M{"moduleKey": module, "symbol": symbol, "effectKnown": true}
	applyShipUpgrade(current, v.Obj(baseline["Engineering"]))
	if bp != "" && bp == current["blueprintKey"] && effect == v.Str(current["experimentalKey"]) {
		knownGrade := v.Int(current["grade"])
		quality, qualityKnown := v.Number(wanted["quality"])
		if knownGrade > grade || knownGrade == grade && qualityKnown && v.Num(current["quality"]) >= quality {
			result["complete"], result["confidence"] = true, "exact"
			return result
		}
	}
	q := roadmapQuery{module: module, blueprint: bp, effect: effect, grade: grade}
	candidates := []string{}
	var best M
	for _, key := range roadmapData.engineerOrder {
		if !engineerHasCapability(key, q) {
			continue
		}
		candidates = append(candidates, key)
		if state := p.roadmapEngineerState(key); best == nil || recipeAccessScore(state) > recipeAccessScore(best) {
			best = state
		}
	}
	sort.Strings(candidates)
	result["engineerKeys"] = candidates
	if best == nil {
		result["reason"] = "unknown-recipe"
		return result
	}
	recipe := M{"moduleKey": module, "blueprintKey": bp, "experimentalKey": effect,
		"grade": grade, "engineer": best, "compatible": true, "confidence": "exact", "conditions": []string{}}
	p.shipRecipe(recipe, blueprints[bp], current, observations)
	result["confidence"], result["conditions"] = recipe["confidence"], recipe["conditions"]
	if quality, known := v.Number(wanted["quality"]); bp != "" && (!known || quality < 1) && result["confidence"] == "exact" {
		result["confidence"] = "estimate"
	}
	if !v.Bool(recipe["compatible"]) || recipe["totals"] == nil || contains(recipe["conditions"], "requires-blueprint") {
		result["confidence"], result["reason"] = "unknown", "unknown-recipe"
		return result
	}
	result["materials"] = recipe["totals"]
	result["complete"] = len(v.Rows(result["materials"])) == 0 && result["confidence"] == "exact"
	return result
}
