package colonization

import (
	"github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"sort"
	"strings"
	"time"
)

func (p *Planner) Snapshot(context M) M {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.snapshotLocked(context)
}
func (p *Planner) snapshotLocked(context M) M {
	cargo, cargoKnown := AvailableCargo(value.Obj(context["commander"]))
	warehouses, warehouse := p.carrierWarehouses(context)
	projects := []M{}
	var active M
	for _, project := range p.projects {
		if !p.projectVisible(project) {
			continue
		}
		row := p.projectSnapshot(project, cargo, cargoKnown, warehouse)
		if context["includeDevelopment"] == true {
			row["development"] = p.developmentProjection(row, rows(row["buildings"]), str(row["id"]) == p.selected)
		}
		projects = append(projects, row)
		if str(row["id"]) == p.selected {
			active = row
		}
	}
	search := copyM(p.search)
	search["originFactions"] = search["factions"]
	depots := []M{}
	for _, id := range sortedKeys(p.depots) {
		row := copyM(p.depots[id])
		if IsColonisationShip(row) {
			continue
		}
		row["stationName"] = row["station"]
		linked := false
		for _, project := range p.projects {
			for _, b := range rows(project["buildings"]) {
				if str(b["marketId"]) == id {
					linked = true
				}
			}
		}
		row["linked"] = linked
		depots = append(depots, row)
	}
	claims := []M{}
	for _, id := range sortedKeys(p.claims) {
		claims = append(claims, copyM(p.claims[id]))
	}
	supplies := []M{}
	var activeValue any
	capacity := p.config["cargoCapacity"]
	if active != nil {
		activeValue = active
		supplies = rows(active["supplies"])
		capacity = active["cargoCapacity"]
	}
	return merge(copyM(p.config), M{"config": copyM(p.config), "cargoCapacity": capacity, "selectedProjectId": p.selected, "projects": projects, "activeProject": activeValue, "catalog": Catalog(), "economies": cloneRows(economies), "rules": Rules(), "sources": cloneRows(sources), "search": search, "aggregateSupplies": supplies, "depots": depots, "claims": claims, "cargoKnown": cargoKnown, "carrierWarehouses": warehouses, "carrierWarehouseComplete": !warehouse.excluded && warehouse.complete, "journalContributionCount": len(p.contributions), "limitations": []string{"Список кандидатов отражает только наблюдения Spansh; заявки и разрешения проверяются в игре.", "Точная потребность загружается из журнала при посещении строительного депо. Ручные планы помечены как оценки.", "Баланс очков задаётся по экрану архитектора. Экономические направления не являются прогнозом объёмов рынка."}})
}
func (p *Planner) statusOf(b M) string {
	if d := p.depots[str(b["marketId"])]; d != nil {
		if journalStatus := str(b["journalStatus"]); journalStatus != "" && value.Time(b["journalUpdatedAt"]).After(value.Time(d["timestamp"])) {
			return journalStatus
		}
		if value.Bool(d["failed"]) {
			return "failed"
		}
		if value.Bool(d["complete"]) {
			return "complete"
		}
		return "building"
	}
	if journalStatus := str(b["journalStatus"]); journalStatus != "" {
		return journalStatus
	}
	return str(b["status"])
}
func (p *Planner) projectSnapshot(project M, cargo map[string]float64, cargoKnown bool, warehouse *stock) M {
	points := copyM(value.Obj(project["points"]))
	ports := int(num(project["externalPortCount"]))
	bs := rows(project["buildings"])
	for _, b := range bs {
		t := buildingCatalogType(b)
		tier := firstAny(b["tierOverride"], t["tier"])
		if str(t["kind"]) == "port" && num(tier) >= 2 && !value.Bool(b["isPrimary"]) && has([]string{"building", "complete"}, p.statusOf(b)) {
			ports++
		}
	}
	buildings := []M{}
	warnings := []string{}
	for index, b := range bs {
		t := buildingCatalogType(b)
		tier := firstAny(b["tierOverride"], t["tier"])
		status := p.statusOf(b)
		d := p.depots[str(b["marketId"])]
		own := p.buildingContributions(b)
		resources := buildingRequirements(project, b, d, own)
		for _, row := range resources {
			key := str(row["commodity"])
			row["cargoAvailable"] = nullableCount(cargo, key, cargoKnown)
			row["carrierAvailable"] = warehouseCount(warehouse, key)
		}
		ownWarnings := []string{}
		if status == "failed" {
			ownWarnings = append(ownWarnings, "Журнал сообщает о провале строительства.")
		}
		if len(resources) == 0 && status != "complete" && status != "cancelled" {
			ownWarnings = append(ownWarnings, "Потребность в товарах неизвестна. Посетите депо или добавьте ручную оценку с источником.")
		}
		for _, r := range resources {
			if !value.Bool(r["authoritative"]) {
				ownWarnings = append(ownWarnings, "План ресурсов — ручная оценка; он будет заменён требованиями связанного депо.")
				break
			}
		}
		if str(b["marketId"]) != "" && d == nil {
			ownWarnings = append(ownWarnings, "Депо связано, но его требования ещё не прочитаны из журнала.")
		}
		if tier == nil {
			ownWarnings = append(ownWarnings, "Уровень и строительные очки этого варианта требуют ручного подтверждения.")
		}
		for _, id := range stringsOf(b["dependsOn"]) {
			di := -1
			var dep M
			for i, r := range bs {
				if str(r["id"]) == id {
					di, dep = i, r
					break
				}
			}
			if dep == nil {
				ownWarnings = append(ownWarnings, "Зависимый объект отсутствует.")
			} else if di > index && !has([]string{"building", "complete"}, p.statusOf(dep)) {
				ownWarnings = append(ownWarnings, "Сначала нужен объект «"+str(dep["name"])+"», стоящий позже в очереди.")
			} else if has([]string{"cancelled", "failed"}, p.statusOf(dep)) {
				ownWarnings = append(ownWarnings, "Зависимый объект «"+str(dep["name"])+"» отменён или строительство провалено.")
			}
		}
		cost := b["pointCostOverride"]
		if cost == nil {
			switch {
			case value.Bool(b["isPrimary"]):
				cost = float64(0)
			case str(t["kind"]) == "port" && num(tier) == 2:
				cost = 3 + math.Max(0, float64(ports-1))*2
			case str(t["kind"]) == "port" && num(tier) == 3:
				cost = 6 * math.Max(1, float64(ports))
			default:
				cost = t["pointCost"]
			}
		}
		reward := firstAny(b["pointRewardOverride"], t["pointReward"])
		before := copyM(points)
		if status == "planned" {
			costTier := num(firstAny(t["costTier"], tier))
			if costTier == 2 || costTier == 3 {
				key := "tier2"
				if costTier == 3 {
					key = "tier3"
				}
				if cost != nil && num(cost) == 0 {
				} else if cost == nil || points[key] == nil {
					ownWarnings = append(ownWarnings, "Баланс очков T"+formatNumber(costTier)+" неизвестен. Введите текущий баланс по экрану архитектора.")
				} else {
					if num(points[key]) < num(cost) {
						ownWarnings = append(ownWarnings, "Не хватает "+formatNumber(num(cost)-num(points[key]))+" очк. T"+formatNumber(costTier)+" для начала строительства.")
					}
					points[key] = num(points[key]) - num(cost)
				}
			}
			if str(t["kind"]) == "port" && num(tier) >= 2 && !value.Bool(b["isPrimary"]) {
				ports++
			}
		}
		if status == "planned" || status == "building" {
			key := ""
			if num(t["rewardTier"]) == 3 {
				key = "tier3"
			} else if num(tier) == 1 {
				key = "tier2"
			} else if num(tier) == 2 {
				key = "tier3"
			}
			if key != "" && points[key] != nil && reward != nil {
				points[key] = num(points[key]) + num(reward)
			}
		}
		ownDelivered := float64(0)
		for _, c := range own {
			for _, r := range rows(c["resources"]) {
				ownDelivered += num(r["count"])
			}
		}
		pointSource := t["source"]
		if b["pointCostOverride"] != nil || b["pointRewardOverride"] != nil {
			pointSource = "manual"
		}
		billSource := "unknown"
		if d != nil {
			billSource = "journal"
		} else if len(resources) > 0 {
			billSource = "manual"
		}
		buildings = append(buildings, merge(copyM(b), M{"name": displayBuildingName(b), "displayName": displayBuildingName(b), "catalogTypeId": t["id"], "manualStatus": b["status"], "tier": tier, "kind": t["kind"], "location": t["location"], "economy": t["economy"], "status": status, "effectiveStatus": status, "pointCost": cost, "pointReward": reward, "pointsBefore": before, "pointsAfter": copyM(points), "pointSource": pointSource, "requirements": resources, "authoritative": d != nil, "billSource": billSource, "warnings": ownWarnings, "prerequisites": value.Clone(t["prerequisites"]), "progress": d["constructionProgress"], "depotUpdatedAt": d["timestamp"], "ownDelivered": ownDelivered}))
	}
	if points0 := value.Obj(project["points"]); points0["tier2"] == nil || points0["tier3"] == nil {
		warnings = append(warnings, "Укажите текущие доступные очки T2/T3. Уже построенные объекты включены в этот баланс и повторно очки не начисляют.")
	}
	concurrent, unknownBills := 0, 0
	allDone, anyComplete := len(buildings) > 0, false
	for _, b := range buildings {
		status := str(b["status"])
		if status == "building" {
			concurrent++
		}
		if !has([]string{"complete", "cancelled", "failed"}, status) && len(rows(b["requirements"])) == 0 {
			unknownBills++
		}
		if status == "complete" {
			anyComplete = true
		}
		if status != "complete" && status != "cancelled" {
			allDone = false
		}
	}
	if concurrent > 5 {
		warnings = append(warnings, "В плане более пяти одновременных строек. Проверьте фактическое ограничение у архитектора.")
	}
	var claim M
	for _, r := range p.claims {
		if str(project["systemAddress"]) != "" && str(r["systemAddress"]) == str(project["systemAddress"]) || str(r["system"]) != "" && equal(r["system"], project["targetSystem"]) {
			claim = r
			break
		}
	}
	claimedAt := firstAny(project["claimedAt"], claim["claimedAt"])
	primaryProject := merge(M{}, project, M{"buildings": buildings, "claimedAt": claimedAt, "claimObservation": claim})
	primaryComplete := PrimaryPortComplete(primaryProject)
	deadline := project["deadline"]
	if deadline == nil && claimedAt != nil {
		deadline = value.ISO(value.Time(claimedAt).Add(28 * 24 * time.Hour))
	}
	capacity := firstAny(project["cargoCapacity"], p.config["cargoCapacity"])
	supplies := aggregateResources(buildings, cargo, cargoKnown, warehouse, capacity)
	queue := supplyBuildings(project, buildings)
	status := str(project["status"])
	if status == "planned" && claim != nil && str(claim["status"]) != "released" {
		status = "claimed"
		if str(claim["status"]) == "beacon" {
			status = "building"
		}
	}
	if status == "planned" || status == "claimed" {
		for _, b := range buildings {
			if has([]string{"building", "complete"}, str(b["status"])) {
				status = "building"
				break
			}
		}
	}
	if !has([]string{"complete", "cancelled"}, status) && anyComplete && allDone {
		status = "complete"
	}
	if str(claim["status"]) == "released" {
		warnings = append(warnings, "Журнал содержит отказ от заявки. Проверьте действительность колонии в игре.")
	}
	deadlineActive := deadline != nil && !primaryComplete && !has([]string{"complete", "cancelled"}, status)
	var effectiveDeadline any
	if deadlineActive {
		effectiveDeadline = deadline
		if value.Time(deadline).Before(p.now()) {
			warnings = append(warnings, "Расчётный срок первичного порта истёк. Актуальный таймер проверьте в игре.")
		}
	}
	totalRequired, totalMissing := float64(0), float64(0)
	for _, r := range supplies {
		totalRequired += num(r["required"])
		totalMissing += num(r["missing"])
	}
	journalDeliveries := []M{}
	for _, c := range p.contributions {
		for _, b := range bs {
			if str(b["marketId"]) != "" && str(c["marketId"]) == str(b["marketId"]) {
				journalDeliveries = append(journalDeliveries, copyM(c))
				break
			}
		}
	}
	deadlineSource := ""
	if project["deadline"] != nil {
		deadlineSource = "manual"
	} else if claimedAt != nil {
		deadlineSource = "claim-estimate"
	}
	var beacon any
	if claimedAt != nil {
		beacon = value.ISO(value.Time(claimedAt).Add(24 * time.Hour))
	}
	operational := supplies
	if has([]string{"complete", "cancelled"}, status) {
		operational = []M{}
	}
	var taskValue any
	if task := nextTask(merge(copyM(project), M{"status": status}), queue, effectiveDeadline); task != nil {
		taskValue = task
	}
	var claimValue any
	if claim != nil {
		claimValue = copyM(claim)
	}
	return merge(copyM(project), M{"status": status, "manualStatus": project["status"], "cargoCapacity": capacity, "buildings": buildings, "supplyQueue": queue, "supplies": supplies, "aggregateSupplies": supplies, "operationalSupplies": operational, "claimedAt": claimedAt, "deadline": deadline, "primaryComplete": primaryComplete, "deadlineActive": deadlineActive, "effectiveDeadline": effectiveDeadline, "deadlineSource": deadlineSource, "beaconDeadline": beacon, "claimObservation": claimValue, "pointsProjected": points, "warnings": warnings, "concurrent": concurrent, "unknownBills": unknownBills, "totalRequired": totalRequired, "totalMissing": totalMissing, "manualDeliveries": value.Clone(project["deliveries"]), "journalDeliveries": journalDeliveries, "economyPlan": economyPlan(str(project["goalEconomy"]), buildings), "buildAdvice": buildAdvice(primaryProject, buildings), "nextTask": taskValue})
}

func (p *Planner) buildingContributions(building M) []M {
	var own []M
	market := str(building["marketId"])
	for _, contribution := range p.contributions {
		if str(contribution["marketId"]) == market {
			own = append(own, contribution)
		}
	}
	return own
}

func buildingRequirements(project, building, depot M, contributions []M) []M {
	manual := map[string]float64{}
	for _, row := range rows(project["deliveries"]) {
		if str(row["buildingId"]) == str(building["id"]) {
			manual[str(row["commodity"])] += num(row["count"])
		}
	}
	resources := []M{}
	if depot == nil {
		for _, row := range rows(building["resourcePlan"]) {
			delivered := manual[str(row["commodity"])]
			resources = append(resources, merge(copyM(row), M{"delivered": delivered, "manualDelivered": delivered, "missing": max0(num(row["required"]) - delivered), "source": "manual", "sourceNote": building["sourceNote"], "authoritative": false}))
		}
		return resources
	}

	pending := map[string]float64{}
	for _, contribution := range contributions {
		if num(contribution["sequence"]) <= num(depot["sequence"]) {
			continue
		}
		for _, part := range rows(contribution["resources"]) {
			pending[str(part["commodity"])] += num(part["count"])
		}
	}
	for _, row := range rows(depot["requirements"]) {
		key := str(row["commodity"])
		delivered := num(row["delivered"]) + pending[key]
		source := "journal"
		if pending[key] > 0 {
			source += "+contribution"
		}
		resources = append(resources, merge(copyM(row), M{"observedDelivered": row["delivered"], "pendingOwn": pending[key], "delivered": delivered, "missing": max0(num(row["required"]) - delivered), "manualDelivered": manual[key], "source": source, "updatedAt": depot["timestamp"]}))
	}
	return resources
}

type stock struct {
	items    map[string]float64
	complete bool
	excluded bool
}

func AvailableCargo(commander M) (map[string]float64, bool) {
	if vehicle := str(commander["vehicle"]); vehicle != "" && vehicle != "ship" {
		return nil, false
	}
	if !isRowArray(commander["cargoForSync"]) {
		return nil, false
	}
	result := map[string]float64{}
	for _, r := range rows(commander["cargoForSync"]) {
		if id := str(r["MissionID"]); id != "" && id != "0" {
			continue
		}
		key := value.Key(r["Name"])
		count := natural(r["Count"], 1e9)
		if key == "" || count == nil {
			continue
		}
		result[key] += max0(num(count) - num(natural(r["Stolen"], 1e9)))
	}
	return result, true
}
func availableWarehouse(carrier M) *stock {
	w := value.Obj(carrier["warehouse"])
	if !value.Bool(carrier["owned"]) || !isRowArray(w["items"]) {
		return nil
	}
	s := &stock{items: map[string]float64{}, complete: value.Bool(w["complete"])}
	for _, r := range rows(w["items"]) {
		key := value.Key(firstText(r["key"], r["commodity"], r["name"]))
		count := natural(r["count"], 1e9)
		if key != "" && count != nil {
			s.items[key] += num(count)
		}
	}
	return s
}
func warehouseCount(s *stock, key string) any {
	if s == nil || s.excluded {
		return nil
	}
	if n, ok := s.items[key]; ok {
		return n
	}
	if s.complete {
		return float64(0)
	}
	return nil
}
func nullableCount(items map[string]float64, key string, known bool) any {
	if !known {
		return nil
	}
	return items[key]
}
func aggregateResources(buildings []M, cargo map[string]float64, cargoKnown bool, warehouse *stock, capacity any) []M {
	by := map[string]M{}
	for _, b := range buildings {
		if has([]string{"complete", "cancelled", "failed"}, str(b["status"])) {
			continue
		}
		for _, r := range rows(b["requirements"]) {
			key := str(r["commodity"])
			row := by[key]
			if row == nil {
				row = M{"commodity": key, "name": r["name"], "required": float64(0), "delivered": float64(0), "missing": float64(0), "manualDelivered": float64(0), "authoritative": true, "sources": []string{}, "buildings": []M{}}
				by[key] = row
			}
			for _, field := range []string{"required", "delivered", "missing", "manualDelivered"} {
				row[field] = num(row[field]) + num(r[field])
			}
			row["authoritative"] = value.Bool(row["authoritative"]) && value.Bool(r["authoritative"])
			ss := stringsOf(row["sources"])
			if !has(ss, str(r["source"])) {
				row["sources"] = append(ss, str(r["source"]))
			}
			row["buildings"] = append(rows(row["buildings"]), M{"id": b["id"], "name": b["name"], "missing": r["missing"]})
		}
	}
	result := []M{}
	for _, key := range sortedKeys(by) {
		row := by[key]
		ship, carrier := nullableCount(cargo, key, cargoKnown), warehouseCount(warehouse, key)
		var toLoad, toBuy, toBuyAtMost, covered, trips any
		if ship != nil {
			toLoad = max0(num(row["missing"]) - num(ship))
			covered = math.Min(num(row["missing"]), num(ship))
			toBuyAtMost = max0(num(toLoad) - num(carrier))
			if warehouse != nil && (warehouse.excluded || warehouse.complete) || num(toLoad) == 0 || carrier != nil && num(carrier) >= num(toLoad) {
				toBuy = toBuyAtMost
			}
		}
		if num(capacity) > 0 {
			trips = math.Ceil(num(row["missing"]) / num(capacity))
		}
		merge(row, M{"source": strings.Join(stringsOf(row["sources"]), "+"), "cargoAvailable": ship, "carrierAvailable": carrier, "toLoad": toLoad, "toBuy": toBuy, "toBuyAtMost": toBuyAtMost, "trips": trips, "coveredByCargo": covered, "carrierComplete": warehouse != nil && !warehouse.excluded && warehouse.complete, "carrierIncluded": warehouse != nil && !warehouse.excluded})
		result = append(result, row)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if num(result[i]["missing"]) != num(result[j]["missing"]) {
			return num(result[i]["missing"]) > num(result[j]["missing"])
		}
		return str(result[i]["name"]) < str(result[j]["name"])
	})
	return result
}
func max0(n float64) float64 { return math.Max(0, n) }
func firstAny(v ...any) any {
	for _, x := range v {
		if x != nil {
			return x
		}
	}
	return nil
}
func stringsLower(s string) string { return strings.ToLower(s) }
func formatNumber(v any) string    { return strconvFormat(num(v)) }
