package engineering

import (
	"sort"
	"strconv"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type shipPlanGoal struct {
	row, current M
	query        roadmapQuery
	engineers    []M
}

// CurrentShipPlan projects saved slot targets against the latest matching
// loadout. Inventory is compared once after summing all pending recipes.
func (p *Planner) CurrentShipPlan(loadout M, targets []M, observations M) M {
	p.mu.RLock()
	defer p.mu.RUnlock()

	bySlot := map[string]currentShipModule{}
	if len(targets) > 0 {
		for _, installed := range p.currentShipModules(loadout, observations) {
			bySlot[v.Str(installed.row["slot"])] = installed
		}
	}
	loadoutKnown := v.Arr(loadout["Modules"]) != nil
	goals, blocked := []M{}, []M{}
	var pending []shipPlanGoal
	summary := M{"total": 0, "pending": 0, "complete": 0, "unavailable": 0, "partial": 0, "estimated": 0}
	seenSlots := map[string]bool{}
	for _, target := range targets {
		slot := v.Str(target["slot"])
		if slot == "" || seenSlots[slot] {
			continue
		}
		seenSlots[slot] = true
		goal := shipPlanTarget(target, bySlot[slot], loadoutKnown)
		goals = append(goals, goal.row)
		switch goal.row["status"] {
		case "pending":
			pending = append(pending, goal)
		case "unavailable":
			blocked = append(blocked, goal.row)
		}
		status := v.Str(goal.row["status"])
		summary[status] = v.Int(summary[status]) + 1
	}

	budget := map[string]int{}
	unknown := map[string]bool{}
	estimated := map[string]bool{}
	stops := map[string]*shipPlanStop{}
	assigned := map[string]bool{}
	coverage := shipPlanCoverage(pending)
	for _, goal := range pending {
		engineer := shipPlanWorkshop(goal.engineers, goal.query.grade, assigned, coverage, observations)
		key := v.Str(engineer["key"])
		assigned[key] = true
		recipe := p.currentShipRecipe(goal.current, goal.query, engineer, observations)
		goal.row["engineer"], goal.row["recipe"] = engineer, recipe
		if v.Bool(recipe["totalsPartial"]) {
			summary["partial"] = v.Int(summary["partial"]) + 1
		}
		if v.Bool(recipe["totalsEstimated"]) {
			summary["estimated"] = v.Int(summary["estimated"]) + 1
		}
		accumulateIngredients(budget, v.Rows(recipe["totals"]), 1)
		accumulateIngredients(budget, v.Rows(recipe["currencies"]), 1)
		for _, step := range v.Rows(recipe["grades"]) {
			if step["status"] == "unknown" {
				for _, ingredient := range v.Rows(step["perRoll"]) {
					unknown[v.Str(ingredient["key"])] = true
				}
			}
			if step["status"] == "estimated" {
				for _, ingredient := range v.Rows(step["materials"]) {
					estimated[v.Str(ingredient["key"])] = true
				}
			}
		}
		if recipe["experimentalStatus"] == "unknown" {
			for _, ingredient := range v.Rows(effects[goal.query.effect]["materials"]) {
				unknown[v.Str(ingredient["key"])] = true
			}
		}

		stop := p.shipPlanVisit(stops, engineer, observations)
		stop.tasks = append(stop.tasks, goal.row)
		stop.grade = max(stop.grade, goal.query.grade)
	}
	supplies := p.shipPlanUnlockSupplies(stops, budget, observations)
	ordered := p.orderShipPlanStops(stops, observations)
	for key := range unknown {
		// Include ingredients with only unknown quantities without implying zero.
		budget[key] += 0
	}
	materialRows, currencyRows := []M{}, []M{}
	for _, row := range p.ingredientRows(ingredientTotals(budget), 1, false, observations) {
		key := v.Str(row["key"])
		if estimated[key] {
			row["estimated"] = true
		}
		if unknown[key] {
			row["partial"] = true
			if v.Int(row["count"]) == 0 {
				row["count"], row["missing"] = nil, nil
			}
		}
		if row["category"] == "currency" {
			currencyRows = append(currencyRows, row)
		} else {
			materialRows = append(materialRows, row)
		}
	}
	summary["total"] = len(goals)
	return M{"goals": goals, "summary": summary, "materials": materialRows, "currencies": currencyRows, "unlockSupplies": supplies,
		"totalsPartial":   v.Int(summary["partial"]) > 0 || len(blocked) > 0,
		"totalsEstimated": v.Int(summary["estimated"]) > 0,
		"itinerary":       M{"stops": ordered, "blocked": blocked}}
}

func shipPlanTarget(target M, installed currentShipModule, loadoutKnown bool) shipPlanGoal {
	row := M{"slot": target["slot"], "item": target["item"], "grade": target["grade"], "blueprintKey": target["blueprintKey"],
		"experimentalKey": target["experimentalKey"], "status": "unavailable"}
	goal := shipPlanGoal{row: row, current: installed.row}
	symbol := symbolText(target["item"])
	module := shipModule(symbol)
	row["name"], row["english"], row["moduleClass"] = modules[module]["name"], roadmapData.modules[module]["english"], shipModuleClass(symbol)
	bp, effect := blueprints[v.Str(target["blueprintKey"])], effects[v.Str(target["experimentalKey"])]
	row["blueprintName"], row["blueprintEnglish"] = bp["name"], bp["english"]
	row["experimentalName"], row["experimentalEnglish"] = effect["name"], effect["english"]
	if !loadoutKnown {
		row["reason"] = "loadout-unavailable"
		return goal
	}
	if symbol == "" || installed.row["symbol"] != symbol {
		row["reason"] = "module-changed"
		return goal
	}
	row["name"], row["english"] = installed.row["name"], installed.row["english"]
	q, err := parseRoadmapQuery(target)
	goal.query = q
	choice := shipBlueprintChoice(installed.choices, q.blueprint)
	if err != nil || q.grade <= 0 || q.grade > v.Int(choice["maxGrade"]) ||
		len(v.Rows(v.At(bp, "grades", strconv.Itoa(q.grade)))) == 0 ||
		q.effect != "" && (!contains(modules[module]["experimentals"], q.effect) || v.Bool(roadmapData.effectExclusions[symbol][q.effect])) {
		row["reason"] = "goal-incompatible"
		return goal
	}
	currentGrade := v.Int(installed.row["grade"])
	if installed.row["blueprintKey"] == q.blueprint &&
		(currentGrade > q.grade || currentGrade == q.grade && v.Num(installed.row["quality"]) >= 1) &&
		(q.effect == "" || installed.row["experimentalKey"] == q.effect) {
		row["status"] = "complete"
		return goal
	}

	allBarred := false
	for _, engineer := range v.Rows(choice["engineers"]) {
		if v.Int(engineer["maxGrade"]) < q.grade || v.Str(engineer["system"]) == "" || v.Str(engineer["base"]) == "" {
			continue
		}
		if engineer["status"] == "barred" {
			allBarred = true
			continue
		}
		goal.engineers = append(goal.engineers, engineer)
	}
	switch {
	case len(goal.engineers) != 0:
		row["status"] = "pending"
	case allBarred:
		row["reason"] = "access-barred"
	default:
		row["reason"] = "workshop-unavailable"
	}
	return goal
}

func shipPlanCoverage(goals []shipPlanGoal) map[string]int {
	coverage := map[string]int{}
	for _, goal := range goals {
		bestAccess := shipPlanWorkshopAccess(goal.engineers[0], goal.query.grade)
		for _, engineer := range goal.engineers[1:] {
			bestAccess = max(bestAccess, shipPlanWorkshopAccess(engineer, goal.query.grade))
		}
		for _, engineer := range goal.engineers {
			if shipPlanWorkshopAccess(engineer, goal.query.grade) == bestAccess {
				coverage[v.Str(engineer["key"])]++
			}
		}
	}
	return coverage
}

func shipPlanWorkshopAccess(engineer M, grade int) int {
	if v.Int(engineer["accessibleGrade"]) >= grade {
		if v.Int(engineer["rank"]) == 5 {
			return 201
		}
		return 200
	}
	return recipeAccessScore(engineer)
}

func shipPlanWorkshop(engineers []M, grade int, assigned map[string]bool, coverage map[string]int, observations M) M {
	ranked := append([]M(nil), engineers...)
	origin := observations["position"]
	if len(v.Arr(origin)) != 3 {
		origin = shipPlanCoordinates(v.Str(observations["system"]), observations)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		aAccess, bAccess := shipPlanWorkshopAccess(a, grade), shipPlanWorkshopAccess(b, grade)
		if aAccess != bAccess {
			return aAccess > bAccess
		}
		aKey, bKey := v.Str(a["key"]), v.Str(b["key"])
		if assigned[aKey] != assigned[bKey] {
			return assigned[aKey]
		}
		if coverage[aKey] != coverage[bKey] {
			return coverage[aKey] > coverage[bKey]
		}
		distance := func(engineer M) any {
			system := v.Str(engineer["system"])
			return shipPlanDistance(origin, shipPlanCoordinates(system, observations), observations["system"], system)
		}
		da, oka := v.Number(distance(a))
		db, okb := v.Number(distance(b))
		if oka != okb {
			return oka
		}
		if oka && da != db {
			return da < db
		}
		return shipWorkshopScore(a) > shipWorkshopScore(b)
	})
	return ranked[0]
}
