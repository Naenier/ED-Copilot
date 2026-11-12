package engineering

import (
	"errors"
	"math"
	"sort"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const itineraryLimit = 240

// UpdateItinerary edits preferences only. The current journal and catalogue
// are always used to rebuild feasibility and the next stop.
func UpdateItinerary(options, request M) (M, error) {
	next := v.Copy(options)
	id := strings.TrimSpace(v.Str(request["id"]))
	switch request["action"] {
	case "source":
		if request["source"] != "plan" && request["source"] != "build" {
			return nil, errors.New("Выберите источник маршрута инженеров.")
		}
		next["source"] = request["source"]
	case "reset":
		delete(next, "order")
		delete(next, "deferred")
	case "defer":
		flag, ok := request["deferred"].(bool)
		if !ok || !validStopID(id) {
			return nil, errors.New("Остановка не найдена.")
		}
		rows := preferenceIDs(next["deferred"])
		rows = removeStop(rows, id)
		if flag && len(rows) < itineraryLimit {
			rows = append(rows, id)
		}
		next["deferred"] = rows
	case "move":
		ids := preferenceIDs(request["visibleIds"])
		delta, known := v.Number(request["direction"])
		if !validStopID(id) || !known || delta != -1 && delta != 1 {
			return nil, errors.New("Выберите направление перемещения остановки.")
		}
		direction := int(delta)
		position := -1
		for index, current := range ids {
			if current == id {
				position = index
				break
			}
		}
		if position < 0 || position+direction < 0 || position+direction >= len(ids) {
			return nil, errors.New("Остановка не найдена.")
		}
		ids[position], ids[position+direction] = ids[position+direction], ids[position]
		seen := map[string]bool{}
		for _, row := range ids {
			seen[row] = true
		}
		for _, row := range preferenceIDs(next["order"]) {
			if !seen[row] && len(ids) < itineraryLimit {
				ids = append(ids, row)
			}
		}
		next["order"] = ids
	default:
		return nil, errors.New("Неизвестное действие с поездкой к инженерам.")
	}
	return next, nil
}

func validStopID(id string) bool {
	return len(id) > 0 && len(id) <= 200 && !strings.ContainsAny(id, "\r\n\x00")
}

func preferenceIDs(value any) []string {
	ids, seen := []string{}, map[string]bool{}
	for _, raw := range v.Arr(value) {
		id := v.Str(raw)
		if validStopID(id) && !seen[id] && len(ids) < itineraryLimit {
			ids, seen[id] = append(ids, id), true
		}
	}
	return ids
}

func removeStop(rows []string, id string) []string {
	for index, row := range rows {
		if row == id {
			return append(rows[:index], rows[index+1:]...)
		}
	}
	return rows
}

// Itinerary suggests only stops the current evidence makes feasible. Counts
// that are missing or estimated stay in blocked tasks with their reason.
func (p *Planner) Itinerary(options, observations M) M {
	snapshot := p.Snapshot(nil)
	p.mu.RLock()
	defer p.mu.RUnlock()
	source := v.Text(options["source"], "plan")
	items := v.Rows(snapshot["items"])
	otherNeeds := map[string]int{}
	if source == "build" {
		for _, row := range items {
			accumulateIngredients(otherNeeds, v.Rows(row["materials"]), 1)
		}
		items = v.Rows(observations["buildItems"])
	} else {
		items = p.itineraryPlanItems(items)
	}
	needs, reserve := map[string]int{}, map[string]int{}
	for _, row := range items {
		if !v.Bool(row["complete"]) {
			accumulateIngredients(needs, v.Rows(row["materials"]), 1)
		}
	}
	for key, count := range needs {
		reserve[key] = count
	}
	for key, count := range otherNeeds {
		reserve[key] += count
	}
	available, known := map[string]int{}, map[string]bool{}
	for _, row := range v.Rows(snapshot["materials"]) {
		key := v.Str(row["key"])
		if owned, ok := v.Number(row["owned"]); ok {
			known[key], available[key] = true, max(0, int(owned)-otherNeeds[key])
		}
	}
	stops, blocked := []M{}, []M{}
	byStop := map[string]M{}
	add := func(stop, task M) {
		id := v.Str(stop["id"])
		if row := byStop[id]; row != nil {
			tasks := v.Rows(row["tasks"])
			for _, previous := range tasks {
				if task["id"] != nil && previous["id"] == task["id"] {
					return
				}
			}
			row["tasks"] = append(tasks, task)
			return
		}
		stop["tasks"] = []M{task}
		byStop[id], stops = stop, append(stops, stop)
	}
	for _, item := range items {
		if v.Bool(item["complete"]) {
			continue
		}
		task := itineraryTask(item)
		engineer, requirement := p.itineraryEngineer(item, observations)
		if engineer == nil || item["confidence"] == "unknown" || item["confidence"] == "estimate" {
			task["reason"] = v.Text(item["reason"], "recipe-unconfirmed")
			if engineer != nil {
				task["engineer"], task["requirement"] = engineer, requirement
			}
			blocked = append(blocked, task)
			continue
		}
		if requirement != nil {
			if requirement["status"] == "ready-unlock" {
				add(workshopStop(engineer, "unlock"), M{"id": "unlock:" + v.Str(engineer["key"]), "requirement": requirement})
				requirement["status"] = "available"
			}
			task["reason"], task["engineer"], task["requirement"] = "access-required", engineer, requirement
			blocked = append(blocked, task)
			continue
		}
		var deficits []M
		for _, ingredient := range v.Rows(item["materials"]) {
			key, quantity := v.Str(ingredient["key"]), v.Int(ingredient["count"])
			if !known[key] || available[key] < quantity {
				row := v.Copy(ingredient)
				row["missing"] = nil
				if known[key] {
					row["missing"] = quantity - available[key]
				}
				deficits = append(deficits, row)
			}
		}
		if len(deficits) > 0 {
			task["reason"], task["deficits"], task["engineer"] = "materials-needed", deficits, engineer
			blocked = append(blocked, task)
			continue
		}
		for _, ingredient := range v.Rows(item["materials"]) {
			available[v.Str(ingredient["key"])] -= v.Int(ingredient["count"])
		}
		add(workshopStop(engineer, "upgrade"), task)
	}
	for _, trade := range itineraryTrades(v.Rows(snapshot["materials"]), needs, reserve) {
		category := v.Str(trade["category"])
		if station := itineraryTrader(category, observations); station != nil {
			add(station, trade)
		}
	}
	deferred := preferenceIDs(options["deferred"])
	excluded, order := map[string]bool{}, map[string]int{}
	for _, id := range deferred {
		excluded[id] = true
	}
	for position, id := range preferenceIDs(options["order"]) {
		order[id] = position + 1
	}
	sort.SliceStable(stops, func(i, j int) bool {
		a, b := order[v.Str(stops[i]["id"])], order[v.Str(stops[j]["id"])]
		if a > 0 && b > 0 {
			return a < b
		}
		if a > 0 || b > 0 {
			return a > 0
		}
		if (stops[i]["kind"] == "trader") != (stops[j]["kind"] == "trader") {
			return stops[i]["kind"] == "trader"
		}
		if x, ok := v.Number(stops[i]["distance"]); ok {
			if y, ok := v.Number(stops[j]["distance"]); ok {
				return x < y
			}
		}
		return false
	})
	active, later := []M{}, []M{}
	for _, stop := range stops {
		if excluded[v.Str(stop["id"])] {
			later = append(later, stop)
		} else {
			active = append(active, stop)
		}
	}
	for _, task := range blocked {
		task["deferred"] = excluded[v.Str(task["id"])]
	}
	result := M{"source": source, "stops": active, "deferred": later, "blocked": blocked, "goalCount": len(items),
		"buildAvailable": observations["buildAvailable"] == true, "buildName": observations["buildName"],
		"customized": len(order) > 0 || len(deferred) > 0}
	if len(active) > 0 {
		result["currentStop"] = active[0]
	}
	return result
}

func itineraryTask(item M) M {
	id := v.Text(item["id"], item["slot"])
	return M{"id": "goal:" + id, "item": item["item"], "module": item["moduleName"], "moduleEnglish": item["moduleEnglish"],
		"blueprint": item["blueprintName"], "blueprintEnglish": item["blueprintEnglish"], "experimental": item["experimentalName"],
		"experimentalEnglish": item["experimentalEnglish"], "grade": item["maxGrade"], "quantity": item["quantity"]}
}

func workshopStop(engineer M, action string) M {
	return M{"id": "engineer:" + v.Str(engineer["key"]), "kind": "engineer", "action": action,
		"name": engineer["name"], "english": engineer["english"], "system": engineer["system"], "station": engineer["base"],
		"entryId": "engineer/" + v.Str(engineer["key"])}
}

func (p *Planner) itineraryPlanItems(items []M) []M {
	goalCounts := map[string]int{}
	for _, row := range items {
		goalCounts[v.Str(row["moduleKey"])] += v.Int(row["quantity"])
	}
	for _, row := range items {
		module, bp := v.Str(row["moduleKey"]), v.Str(row["blueprintKey"])
		row["confidence"], row["moduleEnglish"] = "exact", modules[module]["english"]
		row["blueprintEnglish"], row["experimentalEnglish"] = blueprints[bp]["english"], effects[v.Str(row["experimentalKey"])]["english"]
		if goalCounts[module] != 1 {
			continue
		}
		var installed []M
		for _, current := range p.roadmap.ship {
			if current["moduleKey"] == module {
				installed = append(installed, current)
			}
		}
		if len(installed) != 1 || installed[0]["blueprintKey"] != bp {
			continue
		}
		current := installed[0]
		rolls := v.Arr(row["rolls"])
		for index := range rolls {
			if v.Int(current["grade"]) > index+1 || v.Int(current["grade"]) == index+1 && v.Num(current["quality"]) >= 1 {
				rolls[index] = 0
			}
		}
		if current["experimentalKey"] != nil && current["experimentalKey"] == row["experimentalKey"] {
			row["experimentalKey"], row["experimentalName"], row["experimentalEnglish"] = "", nil, nil
		}
		row["materials"] = ItemMaterials(row)
		row["complete"] = len(v.Rows(row["materials"])) == 0
	}
	return items
}

func (p *Planner) itineraryEngineer(item, observations M) (M, M) {
	query := roadmapQuery{module: v.Str(item["moduleKey"]), blueprint: v.Str(item["blueprintKey"]),
		effect: v.Str(item["experimentalKey"]), grade: v.Int(item["maxGrade"])}
	if query.module == "" || query.blueprint == "" && query.effect == "" {
		return nil, nil
	}
	var ready, best M
	for _, key := range roadmapData.engineerOrder {
		if !engineerHasCapability(key, query) {
			continue
		}
		state := p.roadmapEngineerState(key)
		if v.Str(state["system"]) == "" || v.Str(state["base"]) == "" {
			continue
		}
		if state["status"] == "unlocked" && (query.grade == 0 || v.Int(state["rank"]) >= query.grade) {
			if v.Key(state["system"]) == v.Key(observations["system"]) {
				return state, nil
			}
			if ready == nil || recipeAccessScore(state) > recipeAccessScore(ready) {
				ready = state
			}
		}
		if best == nil || recipeAccessScore(state) > recipeAccessScore(best) {
			best = state
		}
	}
	if ready != nil {
		return ready, nil
	}
	if best == nil {
		return nil, nil
	}
	key := v.Str(best["key"])
	if best["status"] == "unlocked" {
		rank := p.roadmapRankStep(key, best)
		if query.grade > 0 {
			rank["target"] = query.grade
		}
		return best, rank
	}
	for _, raw := range v.Rows(roadmapData.engineers[key]["requirements"]) {
		step := p.roadmapRequirement(key, best, raw, observations)
		if step["status"] == "done" {
			continue
		}
		if (best["status"] == "invited" || best["status"] == "acquainted") && readyHandIn(step) {
			step["status"] = "ready-unlock"
		}
		return best, step
	}
	return best, M{"status": "unknown", "kind": "unlock"}
}

func readyHandIn(step M) bool {
	if step["kind"] != "unlock" && step["kind"] != "donation" || step["status"] == "blocked" {
		return false
	}
	if missing, known := v.Number(step["missing"]); known {
		return missing == 0
	}
	rows := v.Rows(step["items"])
	if len(rows) == 0 {
		return step["status"] == "available"
	}
	for _, item := range rows {
		if missing, known := v.Number(item["missing"]); !known || missing > 0 {
			return false
		}
	}
	return true
}

func itineraryTrades(inventory []M, needs, reserved map[string]int) []M {
	stock, known := map[string]int{}, map[string]bool{}
	for _, row := range inventory {
		if amount, ok := v.Number(row["owned"]); ok {
			key := v.Str(row["key"])
			stock[key], known[key] = int(amount), true
		}
	}
	trades := []M{}
	for _, target := range inventory {
		key := v.Str(target["key"])
		remaining := reserved[key] - stock[key]
		if !known[key] || needs[key] == 0 || remaining <= 0 {
			continue
		}
		var best M
		for _, donor := range inventory {
			donorKey := v.Str(donor["key"])
			excess := stock[donorKey] - reserved[donorKey]
			if !known[donorKey] || excess <= 0 {
				continue
			}
			offer := TradeOffer(donor, target, remaining, excess, stock[key], reserved[donorKey])
			if offer == nil || v.Int(offer["maxGain"]) <= 0 {
				continue
			}
			batches := min(v.Int(offer["batches"]), v.Int(offer["maxGain"])/v.Int(offer["gain"]))
			offer["requiredFrom"], offer["received"] = batches*v.Int(offer["cost"]), batches*v.Int(offer["gain"])
			offer["toEnglish"] = target["english"]
			score := func(row M) float64 {
				if v.Int(row["received"]) >= remaining {
					return float64(v.Int(row["requiredFrom"])) / float64(v.Int(row["received"]))
				}
				return 1e6 - float64(v.Int(row["received"]))
			}
			if best == nil || score(offer) < score(best) {
				best = offer
			}
		}
		if best != nil {
			stock[v.Str(best["fromKey"])] -= v.Int(best["requiredFrom"])
			stock[key] += v.Int(best["received"])
			trades = append(trades, best)
		}
	}
	return trades
}

var observedMaterialTraders = []M{
	{"category": "raw", "system": "82 Eridani", "station": "Bresnik Port", "coords": []float64{10.40625, -16.3125, -3.75}},
	{"category": "manufactured", "system": "LHS 380", "station": "Qureshi Orbital", "coords": []float64{5.28125, 6.75, 10.875}},
	{"category": "encoded", "system": "Bhritzameno", "station": "Feynman Terminal", "coords": []float64{-11.3125, -1.03125, 15.34375}},
	{"category": "raw", "system": "Kojeara", "station": "TolaGarf's Junkyard", "coords": []float64{-9513.09375, -908.84375, 19814.28125}},
	{"category": "manufactured", "system": "Coeus", "station": "Foster Terminal", "coords": []float64{-9531.71875, -885.3125, 19839.21875}},
	{"category": "manufactured", "system": "Eoch Flyuae QK-E d12-2118", "station": "Venatores Hub", "coords": []float64{-8633.78125, -700.8125, 17942.1875}},
	{"category": "encoded", "system": "Ratraii", "station": "Colonia Dream", "coords": []float64{-9530.96875, -918.03125, 19803.8125}},
}

func itineraryTrader(category string, observations M) M {
	from := v.Arr(v.First(observations["position"], observations["coords"]))
	var best M
	var bestDistance any
	for _, row := range append(v.Rows(observations["traders"]), observedMaterialTraders...) {
		if row["category"] != category || v.Str(row["system"]) == "" || v.Str(row["station"]) == "" {
			continue
		}
		var distance any
		coords := v.Arr(row["coords"])
		if v.Key(row["system"]) == v.Key(observations["system"]) {
			distance = float64(0)
		} else if len(from) == 3 && len(coords) == 3 {
			squares, known := float64(0), true
			for i := range coords {
				a, aOK := v.Number(from[i])
				b, bOK := v.Number(coords[i])
				known = known && aOK && bOK
				squares += (a - b) * (a - b)
			}
			if known {
				distance = math.Sqrt(squares)
			}
		}
		if best == nil || distance != nil && (bestDistance == nil || v.Num(distance) < v.Num(bestDistance)) {
			best, bestDistance = row, distance
		}
	}
	if best == nil {
		return nil
	}
	return M{"id": "trader:" + category, "kind": "trader", "action": "trade", "category": category, "system": best["system"],
		"station": best["station"], "distance": bestDistance}
}
