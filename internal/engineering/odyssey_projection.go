package engineering

import (
	"sort"
	"strconv"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type odysseyStepKey struct {
	item, modification string
	grade              int
}

type odysseyGoal struct {
	model M
	grade int
	mods  map[string]bool
}

type odysseyKitWork struct {
	items    []M
	pending  map[odysseyStepKey][]M
	goals    map[string]odysseyGoal
	complete bool
}

// OdysseyLoadouts returns the selected kit, its journal progress and the
// combined material reservation for the active commander's kits.
func (p *Planner) OdysseyLoadouts(observations M) M {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.odysseyProjection(observations, true)
}

// OdysseyHUD omits editor catalogues, owned equipment and per-item steps.
func (p *Planner) OdysseyHUD(observations M) M {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.odysseyProjection(observations, false)
}

func (p *Planner) odysseyProjection(observations M, desktop bool) M {
	owner := odysseyWorkspaceKey(observations)
	ws := p.odysseyPlans[owner]
	result := M{"workspace": owner, "selectedId": "", "activeLoadout": nil, "materials": []M{}, "portfolioMaterials": []M{}}
	if desktop {
		result["equipment"], result["modifications"] = p.odysseyCatalogues()
		result["ownedEquipment"] = p.odysseyOwnedEquipment(owner, ws)
		result["loadouts"] = []M{}
	}
	if ws == nil {
		return result
	}
	result["selectedId"] = ws.selected
	if desktop {
		result["loadouts"] = v.Clone(ws.loadouts)
	}
	var active M
	for _, kit := range ws.loadouts {
		if kit["id"] == ws.selected {
			active = kit
			break
		}
	}
	if active == nil {
		return result
	}

	selected := p.odysseyWork(owner, ws, active, desktop)
	activeView := M{"id": active["id"], "name": active["name"]}
	if desktop {
		activeView["reserve"], activeView["items"] = active["reserve"], selected.items
	}
	result["activeLoadout"] = activeView
	works := []odysseyKitWork{selected}
	for _, kit := range ws.loadouts {
		if kit["id"] != ws.selected && v.Bool(kit["reserve"]) {
			works = append(works, p.odysseyWork(owner, ws, kit, false))
		}
	}
	conflicts := odysseyConflictingItems(works)
	if desktop {
		for _, item := range selected.items {
			if conflicts[v.Str(item["equipmentKey"])] {
				item["conditions"] = append(item["conditions"].([]string), "incompatible-goals")
			}
		}
	}
	activeDemand, sharedDemand := map[string]int{}, map[string]int{}
	seen := map[odysseyStepKey]bool{}
	complete := len(conflicts) == 0
	for index, work := range works {
		complete = complete && work.complete
		for key, ingredients := range work.pending {
			if index == 0 {
				accumulateIngredients(activeDemand, ingredients, 1)
			}
			if !seen[key] && (index == 0 || key.modification == "" || !conflicts[key.item]) {
				accumulateIngredients(sharedDemand, ingredients, 1)
				seen[key] = true
			}
		}
	}
	result["activeComplete"], result["portfolioComplete"] = selected.complete, complete
	stock := readOdysseyStock(observations)
	result["materials"] = odysseyMaterialProjection(activeDemand, activeDemand, sharedDemand, stock)
	if desktop {
		result["portfolioMaterials"] = odysseyMaterialProjection(sharedDemand, activeDemand, sharedDemand, stock)
	} else if pickups := odysseyPickupMaterials(observations["recentPickupKeys"], sharedDemand); len(pickups) > 0 {
		result["portfolioMaterials"] = odysseyMaterialProjection(pickups, activeDemand, sharedDemand, stock)
	}
	return result
}

func (p *Planner) odysseyWork(owner string, ws *odysseyWorkspace, kit M, detailed bool) odysseyKitWork {
	work := odysseyKitWork{items: []M{}, pending: map[odysseyStepKey][]M{}, goals: map[string]odysseyGoal{}, complete: true}
	for index, item := range v.Rows(kit["items"]) {
		bound := v.Str(item["equipmentKey"])
		identity := bound
		if identity == "" {
			identity = v.Str(kit["id"]) + ":" + strconv.Itoa(index)
		}
		model := odysseyEquipmentByKey[v.Str(item["itemKey"])]
		observed := p.odysseyEquipmentFor(owner, ws, bound)
		target, start := v.Int(item["targetGrade"]), v.Int(item["startingGrade"])
		currentGrade := start
		var installed any = []string{}
		conditions := []string{}
		if bound != "" {
			currentGrade, installed = v.Int(observed["grade"]), observed["mods"]
			if observed == nil || odysseyItemKey(observed["symbol"]) != model["key"] {
				currentGrade, installed = 0, nil
				conditions = append(conditions, "equipment-unavailable")
			}
		}
		if currentGrade == 0 {
			conditions = append(conditions, "grade-unknown")
		}
		desired := v.Arr(item["modKeys"])
		if installed == nil && len(desired) > 0 {
			conditions = append(conditions, "mods-unknown")
		}
		goal := odysseyGoal{model: model, grade: max(target, currentGrade), mods: map[string]bool{}}
		for _, mod := range v.Arr(installed) {
			goal.mods[v.Str(mod)] = true
		}
		for _, mod := range desired {
			recipe := roadmapData.personalRecipes[v.Str(mod)]
			goal.mods[v.Str(recipe["journalSymbol"])] = true
		}
		blocked := len(goal.mods) > odysseySlots(model, goal.grade)
		if blocked {
			conditions = append(conditions, "no-free-slot")
		}
		if bound != "" {
			work.goals[bound] = goal
		}

		steps := []M{}
		for grade := min(start+1, max(2, currentGrade+1)); grade <= target; grade++ {
			recipe := odysseyGradeRecipes[v.Str(model["itemSymbol"])+"-"+strconv.Itoa(grade)]
			status := "pending"
			switch {
			case currentGrade == 0:
				status = "unknown"
			case grade <= currentGrade:
				status = "done"
			case recipe == nil:
				status = "blocked"
				conditions = append(conditions, "missing-recipe")
			}
			ingredients := v.Rows(recipe["materials"])
			if status == "pending" {
				work.pending[odysseyStepKey{item: identity, grade: grade}] = ingredients
			}
			if detailed {
				step := M{"kind": "grade", "grade": grade, "status": status, "credits": recipe["credits"]}
				step["materials"] = p.odysseyStepMaterials(status, ingredients)
				steps = append(steps, step)
			}
		}
		for _, raw := range desired {
			key := v.Str(raw)
			recipe := roadmapData.personalRecipes[key]
			journalKey := v.Str(recipe["journalSymbol"])
			status := "pending"
			switch {
			case contains(installed, journalKey):
				status = "done"
			case currentGrade == 0 || installed == nil:
				status = "unknown"
			case blocked:
				status = "blocked"
			}
			ingredients := v.Rows(recipe["materials"])
			if status == "pending" {
				work.pending[odysseyStepKey{item: identity, modification: key}] = ingredients
			}
			if detailed {
				step := M{"kind": "modification", "modKey": key, "name": recipe["name"], "english": recipe["english"], "status": status, "credits": recipe["credits"]}
				step["engineers"], step["materials"] = p.odysseyEngineers(recipe), p.odysseyStepMaterials(status, ingredients)
				steps = append(steps, step)
			}
		}
		work.complete = work.complete && len(conditions) == 0
		if detailed {
			work.items = append(work.items, v.Merge(v.Copy(item), M{
				"key": identity, "name": model["name"], "english": model["english"], "moduleKey": model["moduleKey"],
				"currentGrade": v.Nullable(float64(currentGrade), currentGrade > 0), "installedMods": v.Clone(installed),
				"observedAt": observed["observedAt"], "steps": steps, "conditions": conditions,
			}))
		}
	}
	return work
}

func odysseyConflictingItems(works []odysseyKitWork) map[string]bool {
	all := map[string]odysseyGoal{}
	for _, work := range works {
		for key, goal := range work.goals {
			combined := all[key]
			if combined.mods == nil {
				combined = odysseyGoal{model: goal.model, mods: map[string]bool{}}
			}
			combined.grade = max(combined.grade, goal.grade)
			for mod := range goal.mods {
				combined.mods[mod] = true
			}
			all[key] = combined
		}
	}
	conflicts := map[string]bool{}
	for key, goal := range all {
		if len(goal.mods) > odysseySlots(goal.model, goal.grade) {
			conflicts[key] = true
		}
	}
	return conflicts
}

func (p *Planner) odysseyStepMaterials(status string, ingredients []M) []M {
	if status != "pending" {
		return []M{}
	}
	return p.ingredientRows(ingredients, 1, true, nil)
}

func (p *Planner) odysseyEngineers(recipe M) []M {
	rows := []M{}
	for _, raw := range v.Arr(recipe["engineerKeys"]) {
		key := v.Str(raw)
		e := roadmapData.engineers[key]
		rows = append(rows, M{"key": key, "name": e["name"], "english": e["english"], "entryId": "engineer/" + key, "status": p.roadmapEngineerState(key)["status"]})
	}
	return rows
}

func (p *Planner) odysseyCatalogues() ([]M, []M) {
	equipment := []M{}
	for _, raw := range roadmapData.personalEquipment {
		row := v.Copy(raw)
		row["slot"] = odysseyWeaponSlot(row)
		if row["moduleKey"] == "suit" {
			row["primarySlots"] = odysseyPrimarySlots(v.Str(row["key"]))
		}
		equipment = append(equipment, row)
	}
	sort.Slice(equipment, func(i, j int) bool { return v.Str(equipment[i]["english"]) < v.Str(equipment[j]["english"]) })
	mods := []M{}
	for _, key := range sortedOdysseyKeys(roadmapData.personalRecipes) {
		recipe := roadmapData.personalRecipes[key]
		mods = append(mods, M{"key": key, "name": recipe["name"], "english": recipe["english"], "moduleKey": recipe["moduleKey"],
			"journalSymbol": recipe["journalSymbol"], "damageType": recipe["damageType"], "engineers": p.odysseyEngineers(recipe)})
	}
	sort.SliceStable(mods, func(i, j int) bool { return v.Str(mods[i]["english"]) < v.Str(mods[j]["english"]) })
	return equipment, mods
}

func (p *Planner) odysseyOwnedEquipment(owner string, ws *odysseyWorkspace) []M {
	rows := []M{}
	if owner == "" {
		return rows
	}
	if ws == nil {
		ws = &odysseyWorkspace{}
	}
	keys := M{}
	if p.odysseyObservationOwner == owner {
		for key := range p.roadmap.personal {
			keys[key] = true
		}
	}
	for key := range ws.progress {
		keys[key] = true
	}
	for _, key := range sortedOdysseyKeys(keys) {
		observed := p.odysseyEquipmentFor(owner, ws, key)
		model := odysseyEquipmentByKey[odysseyItemKey(observed["symbol"])]
		if model != nil {
			rows = append(rows, M{"key": key, "itemKey": model["key"], "name": model["name"], "english": model["english"], "grade": observed["grade"],
				"mods": v.Clone(observed["mods"]), "observedAt": observed["observedAt"]})
		}
	}
	return rows
}
