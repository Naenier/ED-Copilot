package engineering

import (
	"fmt"
	"sort"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (p *Planner) Roadmap(query M) (M, error) {
	return p.RoadmapWithContext(query, nil)
}

// RoadmapWithContext is read-only. The catalogue is shared and indexed once,
// but each response contains only detached rows for the current selection.
func (p *Planner) RoadmapWithContext(query, observations M) (M, error) {
	q, err := parseRoadmapQuery(query)
	if err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	ids := []string{}
	for _, key := range roadmapData.engineerOrder {
		if len(q.engineers) > 0 && !q.engineers[key] {
			continue
		}
		if q.module != "" || q.blueprint != "" || q.effect != "" || q.grade != 0 {
			if !engineerHasCapability(key, q) {
				continue
			}
		}
		ids = append(ids, key)
	}

	selected := map[string]bool{}
	for _, key := range ids {
		selected[key] = true
	}
	ordered := p.roadmapOrder(ids)
	rows := make([]M, 0, len(ordered))
	summary := M{"total": len(ordered), "targets": len(ids), "dependencies": len(ordered) - len(ids), "unlocked": 0, "maxRank": 0}
	for _, key := range ordered {
		engineer := roadmapData.engineers[key]
		state := p.roadmapEngineerState(key)
		state["dependency"] = !selected[key]
		if state["status"] == "unlocked" {
			summary["unlocked"] = v.Int(summary["unlocked"]) + 1
		}
		if state["status"] == "unlocked" && v.Int(state["rank"]) == 5 {
			summary["maxRank"] = v.Int(summary["maxRank"]) + 1
		}
		caps := []M{}
		for _, cap := range roadmapData.capabilities[key] {
			if selected[key] && !q.capabilityMatches(cap) {
				continue
			}
			module, blueprint := roadmapData.modules[cap.module], roadmapData.blueprint(cap.blueprint)
			var accessible any
			switch {
			case state["status"] == "unlocked" && state["kind"] == "odyssey":
				accessible = 1
			case state["status"] == "unlocked" && state["rank"] != nil:
				accessible = min(v.Int(state["rank"]), cap.maxGrade)
			case state["status"] != "unlocked" && state["status"] != "unknown":
				accessible = 0
			}
			caps = append(caps, M{"moduleKey": cap.module, "moduleName": module["name"], "moduleEnglish": module["english"], "blueprintKey": cap.blueprint, "blueprintName": blueprint["name"], "blueprintEnglish": blueprint["english"], "maxGrade": cap.maxGrade, "accessibleGrade": accessible, "damageType": blueprint["damageType"]})
		}
		steps := []M{}
		for _, requirement := range v.Rows(engineer["requirements"]) {
			steps = append(steps, p.roadmapRequirement(key, state, requirement, observations))
		}
		if state["kind"] == "ship" {
			steps = append(steps, p.roadmapRankStep(key, state))
		}
		state["capabilities"], state["steps"] = caps, steps
		state["reputation"] = v.Clone(engineer["reputation"])
		rows = append(rows, state)
	}
	equipment := p.roadmapEquipment(q.module)
	recipe, err := p.roadmapRecipe(q, ids, equipment, observations)
	if err != nil {
		return nil, err
	}
	result := M{
		"filters":   p.roadmapFilters(q, equipment),
		"engineers": rows, "recipe": recipe, "summary": summary, "verifiedAt": roadmapData.verifiedAt,
		"limitations": M{"equipmentScope": "journal-observed", "unknownIsLocked": false},
	}
	if q.module != "" {
		result["specialRecipes"] = p.specialShipVariants(q.module, observations)
	}
	return result, nil
}

func engineerHasCapability(key string, q roadmapQuery) bool {
	for _, capability := range roadmapData.capabilities[key] {
		if q.capabilityMatches(capability) {
			return true
		}
	}
	return false
}

func (p *Planner) roadmapEngineerState(key string) M {
	engineer, progress := roadmapData.engineers[key], p.progress[key]
	status := strings.ToLower(v.Text(progress["status"], "unknown"))
	manualUsed := false
	if unlocked, manual := p.manual[key].(bool); manual && status == "unknown" {
		manualUsed = true
		status = "locked"
		if unlocked {
			status = "unlocked"
		}
	}
	rank := progress["rank"]
	percent := progress["progress"]
	if status != "unlocked" {
		rank, percent = nil, nil
	}
	return M{"key": key, "name": engineer["name"], "english": engineer["english"], "kind": engineer["kind"], "system": engineer["system"], "base": engineer["base"], "status": status, "rank": rank, "progress": percent, "updatedAt": progress["timestamp"], "manualUnlocked": p.manual[key], "manualUsed": manualUsed}
}

func (p *Planner) roadmapOrder(ids []string) []string {
	order := make([]string, 0, len(ids))
	visiting := map[string]bool{}
	var visit func(string)
	visit = func(key string) {
		if visiting[key] || roadmapData.engineers[key] == nil {
			return
		}
		visiting[key] = true
		status := v.Str(p.roadmapEngineerState(key)["status"])
		if !knownEngineer(status) || roadmapData.engineers[key]["kind"] == "odyssey" && status == "known" {
			for _, step := range v.Rows(roadmapData.engineers[key]["requirements"]) {
				visit(v.Str(step["engineerKey"]))
			}
		}
		order = append(order, key)
	}
	for _, key := range ids {
		visit(key)
	}
	return order
}

func knownEngineer(status string) bool {
	switch status {
	case "known", "invited", "acquainted", "unlocked", "barred":
		return true
	default:
		return false
	}
}

func (p *Planner) roadmapRequirement(key string, state, requirement, observations M) M {
	step := v.Copy(requirement)
	kind, status := v.Str(step["kind"]), v.Str(state["status"])
	step["status"] = "unknown"
	dependency := v.Str(step["engineerKey"])
	if dependency != "" {
		parent := p.roadmapEngineerState(dependency)
		step["engineerName"], step["engineerEnglish"], step["engineerSystem"], step["engineerBase"] = parent["name"], parent["english"], parent["system"], parent["base"]
		step["current"], step["currentProgress"] = parent["rank"], parent["progress"]
		step["target"] = step["rank"]
		if parent["status"] == "unlocked" && step["rank"] == nil {
			step["status"] = "available"
		}
		if current, ok := v.Number(parent["rank"]); ok {
			rank, progress := v.Num(step["rank"]), v.Num(step["progress"])
			if current > rank || current == rank && v.Num(parent["progress"]) >= progress {
				step["status"] = "available"
			} else if current < rank || parent["progress"] != nil {
				step["status"] = "pending"
			}
		}
	}
	if metric := v.Str(step["metric"]); metric != "" {
		if current, ok := v.Number(p.roadmap.metrics[metric]); ok {
			step["current"] = current
			step["status"] = "pending"
			if current >= v.Num(step["target"]) {
				step["status"] = "available"
			}
		}
	}
	if kind == "discovery" && dependency == "" && step["metric"] == nil {
		step["status"] = "available"
	}
	if rows := v.Rows(step["items"]); len(rows) > 0 {
		p.decorateHandIn(step, v.Text(step["engineerKey"], key), rows, observations)
	}
	switch {
	case status == "unlocked", kind == "discovery" && knownEngineer(status),
		kind == "referral" && knownEngineer(status) && (state["kind"] == "ship" || status != "known"),
		(kind == "invitation" || kind == "meeting") && (status == "invited" || status == "acquainted"):
		step["status"] = "done"
	case status == "barred":
		step["status"] = "blocked"
	case (kind == "unlock" || kind == "donation") && status == "known":
		step["status"] = "pending"
	case (kind == "unlock" || kind == "donation") && (status == "invited" || status == "acquainted") && step["status"] == "unknown":
		step["status"] = "pending"
	}
	return step
}

func (p *Planner) decorateHandIn(step M, engineer string, items []M, observations M) {
	combined := v.Bool(step["combined"])
	ownedTotal, target, donatedTotal, known := 0, v.Int(step["target"]), 0, true
	rows := make([]M, 0, len(items))
	for _, item := range items {
		row := v.Copy(item)
		key, category, count := v.Str(row["key"]), v.Str(row["category"]), v.Int(row["count"])
		if id := roadmapMaterialEntry(key, category); id != "" {
			row["entryId"] = id
		}
		donated, observed := p.roadmap.contributions[engineer][contributionKey(key, category)]
		if observed {
			row["donated"] = donated
		}
		donatedTotal += donated
		if owned, valid := p.roadmapStock(key, category, observations); valid {
			row["owned"] = owned
			ownedTotal += owned
			if !combined {
				row["missing"] = max(0, count-donated-owned)
			}
		} else {
			known = false
		}
		rows = append(rows, row)
	}
	step["items"] = rows
	if combined && target > 0 {
		if donatedTotal > 0 {
			step["current"] = donatedTotal
		}
		if known || ownedTotal+donatedTotal >= target {
			step["owned"] = ownedTotal
			step["missing"] = max(0, target-donatedTotal-ownedTotal)
		}
	} else if len(rows) == 1 && (step["kind"] == "unlock" || step["kind"] == "donation") {
		step["current"], step["target"] = rows[0]["donated"], rows[0]["count"]
	}
}

func contributionKey(key, category string) string {
	switch category {
	case "credits", "bond", "bounty":
		return category
	case "vouchers":
		if key == "bountyvouchers" {
			return "bounty"
		}
	}
	if canonical := MaterialKey(key); canonical != "" && category != "commodity" {
		return canonical
	}
	return v.Key(key)
}

func (p *Planner) roadmapRankStep(key string, state M) M {
	step := M{"key": "max-rank", "kind": "rank", "status": "unknown", "current": state["rank"], "target": 5,
		"text":    "Поднимите репутацию до G5, выполняя модификации лично в мастерской.",
		"english": "Reach Grade 5 reputation by crafting in person at the workshop."}
	switch {
	case state["status"] == "unlocked" && v.Int(state["rank"]) == 5:
		step["status"] = "done"
	case state["status"] == "unlocked" && state["rank"] != nil:
		step["status"] = "available"
	case state["status"] == "barred":
		step["status"] = "blocked"
	case knownEngineer(v.Str(state["status"])) && state["status"] != "unlocked":
		step["status"] = "pending"
	}
	if step["status"] == "done" {
		return step
	}
	best, unitCost := roadmapCapability{}, 1<<30
	for _, cap := range roadmapData.capabilities[key] {
		recipe := v.Rows(v.Obj(blueprints[cap.blueprint]["grades"])["1"])
		if len(recipe) == 0 {
			continue
		}
		cost := 0
		for _, item := range recipe {
			cost += v.Int(item["count"])
		}
		if cap.maxGrade > best.maxGrade || cap.maxGrade == best.maxGrade && cost < unitCost {
			best, unitCost = cap, cost
		}
	}
	if best.module != "" {
		m, b := roadmapData.modules[best.module], roadmapData.blueprint(best.blueprint)
		step["practice"] = M{"moduleKey": best.module, "blueprintKey": best.blueprint, "moduleName": m["name"], "moduleEnglish": m["english"], "blueprintName": b["name"], "blueprintEnglish": b["english"], "maxGrade": best.maxGrade}
	}
	return step
}

func (p *Planner) roadmapEquipment(module string) []M {
	rows := []M{}
	if module == "" {
		return rows
	}
	for _, source := range []map[string]M{p.roadmap.ship, p.roadmap.stored, p.roadmap.personal} {
		for _, row := range source {
			if row["moduleKey"] == module {
				rows = append(rows, v.Copy(row))
			}
		}
	}
	for _, model := range roadmapData.personalEquipment {
		if model["moduleKey"] != module {
			continue
		}
		key := v.Str(model["key"])
		rows = append(rows, M{"key": "catalog:" + key, "moduleKey": module, "name": model["name"], "english": model["english"],
			"symbol": v.Text(v.At(model, "grades", "1", "symbol"), model["itemSymbol"]), "grade": 1, "mods": []string{}, "planned": true})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i]["planned"] != rows[j]["planned"] {
			return !v.Bool(rows[i]["planned"])
		}
		return v.Str(rows[i]["key"]) < v.Str(rows[j]["key"])
	})
	return rows
}

func (p *Planner) roadmapFilters(q roadmapQuery, equipment []M) M {
	erows, mods, bps, exs := []M{}, []M{}, []M{}, []M{}
	for _, key := range roadmapData.engineerOrder {
		erows = append(erows, p.roadmapEngineerState(key))
	}
	bpMax, effectKeys := map[string]int{}, map[string]bool{}
	bpModules := map[string][]string{}
	for _, key := range roadmapData.moduleOrder {
		mods = append(mods, v.Copy(roadmapData.modules[key]))
		for _, raw := range v.Arr(modules[key]["blueprints"]) {
			id := v.Str(raw)
			bpModules[id] = append(bpModules[id], key)
			if q.module == "" || key == q.module {
				for grade := range v.Obj(blueprints[id]["grades"]) {
					bpMax[id] = max(bpMax[id], v.Int(grade))
				}
			}
		}
		if q.module == "" || key == q.module {
			for _, raw := range v.Arr(modules[key]["experimentals"]) {
				effectKeys[v.Str(raw)] = true
			}
		}
	}
	for key, row := range roadmapData.personalRecipes {
		bpModules[key] = []string{v.Str(row["moduleKey"])}
		if q.module == "" || row["moduleKey"] == q.module {
			bpMax[key] = 1
		}
	}
	for key, grade := range bpMax {
		bp := roadmapData.blueprint(key)
		if bp == nil || grade == 0 {
			continue
		}
		row := roadLabel(bp)
		row["moduleKeys"] = bpModules[key]
		row["maxGrade"] = grade
		row["damageType"] = bp["damageType"]
		row["kind"] = "ship"
		if roadmapData.personalRecipes[key] != nil {
			row["kind"] = "odyssey"
		}
		bps = append(bps, row)
	}
	for key := range effectKeys {
		if row := effects[key]; row != nil {
			exs = append(exs, roadLabel(row))
		}
	}
	for _, rows := range [][]M{bps, exs} {
		sort.Slice(rows, func(i, j int) bool {
			a, b := rows[i], rows[j]
			return fmt.Sprint(a["english"], a["key"]) < fmt.Sprint(b["english"], b["key"])
		})
	}
	return M{"engineers": erows, "modules": mods, "blueprints": bps, "experimentals": exs, "equipment": equipment}
}
