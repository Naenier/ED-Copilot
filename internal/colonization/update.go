package colonization

import (
	"errors"
	"github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"strings"
)

// Update applies an action atomically; rejected actions cannot partially change another colony.
func (p *Planner) Update(body M) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	oldProjects, oldConfig, oldSelected := p.projects, p.config, p.selected
	p.projects = cloneRows(p.projects)
	p.config = copyM(p.config)
	if err := p.updateLocked(body); err != nil {
		p.projects, p.config, p.selected = oldProjects, oldConfig, oldSelected
		return err
	}
	return nil
}
func (p *Planner) updateLocked(body M) error {
	action := str(body["action"])
	switch action {
	case "configure":
		for _, key := range []string{"originSystem", "faction"} {
			if v, ok := body[key]; ok {
				p.config[key] = text(v, 200)
			}
		}
		if raw, ok := body["cargoCapacity"]; ok {
			var capacity any
			if raw != nil && str(raw) != "" {
				n, valid := value.Number(raw)
				if !valid || n < 1 || n > 100000 || math.Trunc(n) != n {
					return errors.New("Укажите целую вместимость от 1 до 100 000 тонн.")
				}
				capacity = n
			}
			if id := str(body["projectId"]); id != "" {
				project := p.projectLocked(id)
				if project == nil {
					return errors.New("Колония не найдена.")
				}
				project["cargoCapacity"] = capacity
			} else {
				p.config["cargoCapacity"] = capacity
			}
		}
		return nil
	case "create-project":
		if len(p.projects) >= 100 {
			return errors.New("Достигнут предел 100 колоний.")
		}
		var candidate M
		for _, r := range rows(p.search["results"]) {
			if str(r["id"]) == str(body["candidateId"]) {
				candidate = r
				break
			}
		}
		target := text(firstText(candidate["system"], body["targetSystem"]), 200)
		if target == "" {
			return errors.New("Выберите целевую систему.")
		}
		if str(candidate["availability"]) == "unavailable" {
			return errors.New("По последним данным система не подходит для новой колонии.")
		}
		address := marketID(value.First(candidate["id64"], body["systemAddress"]))
		if e := p.uniqueColony(target, address, nil); e != nil {
			return e
		}
		data := merge(copyM(body), M{"id": value.ID(), "targetSystem": target, "originSystem": firstText(value.Obj(p.search["origin"])["name"], p.config["originSystem"]), "faction": firstText(body["faction"], p.config["faction"]), "coordinates": value.First(candidate["coordinates"], body["coordinates"]), "systemAddress": address, "buildings": []M{}, "deliveries": []M{}, "supplyOrder": []string{}})
		project := cleanProject(data)
		p.projects = append(p.projects, project)
		p.selected = str(project["id"])
		return nil
	case "select-project":
		project := p.projectLocked(str(body["id"]))
		if project == nil {
			return errors.New("Колония не найдена.")
		}
		p.selected = str(project["id"])
		return nil
	case "remove-project":
		project := p.projectLocked(str(body["id"]))
		if project == nil {
			return errors.New("Колония не найдена.")
		}
		filtered := []M{}
		for _, r := range p.projects {
			if str(r["id"]) != str(project["id"]) {
				filtered = append(filtered, r)
			}
		}
		p.projects = filtered
		if p.selected == str(project["id"]) {
			p.selected = ""
			if len(filtered) > 0 {
				p.selected = str(filtered[0]["id"])
			}
		}
		return nil
	}
	project := p.projectLocked(firstText(body["projectId"], func() string {
		if action == "update-project" {
			return str(body["id"])
		}
		return ""
	}()))
	if project == nil {
		return errors.New("Колония не найдена.")
	}
	switch action {
	case "update-project":
		data := merge(copyM(project), body, M{"id": project["id"], "buildings": project["buildings"], "deliveries": project["deliveries"]})
		next := cleanProject(data)
		if str(next["targetSystem"]) == "" {
			return errors.New("Укажите целевую систему.")
		}
		if e := p.uniqueColony(str(next["targetSystem"]), str(next["systemAddress"]), project); e != nil {
			return e
		}
		for _, b := range rows(project["buildings"]) {
			if depot := p.depots[str(b["marketId"])]; depot != nil && str(depot["system"]) != "" && !equal(depot["system"], next["targetSystem"]) {
				return errors.New("Связанная стройплощадка находится в другой системе.")
			}
		}
		for i, r := range p.projects {
			if str(r["id"]) == str(project["id"]) {
				p.projects[i] = next
				break
			}
		}
		return nil
	case "record-delivery":
		b := building(project, str(body["buildingId"]))
		if b == nil {
			return errors.New("Объект строительства не найден в выбранной колонии.")
		}
		key := value.Key(body["commodity"])
		count, ok := value.Number(body["count"])
		if key == "" || len(key) > 100 || !ok || count < 1 || count > 1e9 || math.Trunc(count) != count {
			return errors.New("Укажите товар и положительное целое количество.")
		}
		ds := rows(project["deliveries"])
		if len(ds) >= 5000 {
			return errors.New("Достигнут предел ручных записей в колонии.")
		}
		project["deliveries"] = append(ds, M{"id": value.ID(), "buildingId": b["id"], "commodity": key, "count": count, "timestamp": value.ISO(p.now()), "notes": text(body["notes"], 1000), "source": "manual"})
		return nil
	case "remove-delivery":
		found := false
		out := []M{}
		for _, r := range rows(project["deliveries"]) {
			if str(r["id"]) == str(body["id"]) {
				found = true
			} else {
				out = append(out, r)
			}
		}
		if !found {
			return errors.New("Запись доставки не найдена в выбранной колонии.")
		}
		project["deliveries"] = out
		return nil
	case "add-building":
		bs := rows(project["buildings"])
		if len(bs) >= 200 {
			return errors.New("Достигнут предел 200 объектов в колонии.")
		}
		if typesByID[str(body["typeId"])] == nil {
			return errors.New("Выберите тип объекта из каталога.")
		}
		next := cleanBuilding(merge(copyM(body), M{"id": value.ID(), "resourcePlan": []M{}}))
		if e := p.uniqueDepot(str(next["marketId"]), nil); e != nil {
			return e
		}
		if value.Bool(next["isPrimary"]) {
			for _, r := range bs {
				if value.Bool(r["isPrimary"]) {
					return errors.New("Первичный порт в колонии уже указан.")
				}
			}
		}
		if depot := p.depots[str(next["marketId"])]; depot != nil && str(depot["system"]) != "" && !equal(depot["system"], project["targetSystem"]) {
			return errors.New("Депо находится в другой системе.")
		}
		project["buildings"] = append(bs, next)
		if e := validateDependencies(project); e != nil {
			return e
		}
		if ids := stringsOf(project["supplyOrder"]); len(ids) > 0 {
			project["supplyOrder"] = append(ids, str(next["id"]))
		}
		return nil
	case "reorder-buildings", "reorder-supply":
		ids := stringsOf(body["ids"])
		bs := rows(project["buildings"])
		if len(ids) != len(bs) {
			return errors.New("Очередь должна содержать каждый объект колонии ровно один раз.")
		}
		seen := map[string]bool{}
		order := []M{}
		for _, id := range ids {
			b := building(project, id)
			if b == nil || seen[id] {
				return errors.New("Очередь содержит повтор или объект другой колонии.")
			}
			seen[id] = true
			order = append(order, b)
		}
		if action == "reorder-buildings" {
			project["buildings"] = order
		} else {
			project["supplyOrder"] = ids
		}
		return nil
	}
	b := building(project, firstText(body["buildingId"], body["id"]))
	if b == nil {
		return errors.New("Объект строительства не найден в выбранной колонии.")
	}
	switch action {
	case "remove-building":
		bs := []M{}
		for _, r := range rows(project["buildings"]) {
			if str(r["id"]) == str(b["id"]) {
				continue
			}
			deps := []string{}
			for _, id := range stringsOf(r["dependsOn"]) {
				if id != str(b["id"]) {
					deps = append(deps, id)
				}
			}
			r["dependsOn"] = deps
			bs = append(bs, r)
		}
		project["buildings"] = bs
		deliveries := []M{}
		for _, r := range rows(project["deliveries"]) {
			if str(r["buildingId"]) != str(b["id"]) {
				deliveries = append(deliveries, r)
			}
		}
		project["deliveries"] = deliveries
		ids := []string{}
		for _, id := range stringsOf(project["supplyOrder"]) {
			if id != str(b["id"]) {
				ids = append(ids, id)
			}
		}
		project["supplyOrder"] = ids
	case "move-building", "move-supply-building":
		direction := str(body["direction"])
		if direction != "up" && direction != "down" {
			return errors.New("Неизвестное направление перемещения.")
		}
		bs := rows(project["buildings"])
		if action == "move-supply-building" {
			bs = supplyBuildings(project, bs)
		}
		index := -1
		for i, r := range bs {
			if str(r["id"]) == str(b["id"]) {
				index = i
				break
			}
		}
		dest := index + 1
		if direction == "up" {
			dest = index - 1
		}
		if index >= 0 && dest >= 0 && dest < len(bs) {
			bs[index], bs[dest] = bs[dest], bs[index]
		}
		if action == "move-building" {
			project["buildings"] = bs
		} else {
			ids := []string{}
			for _, r := range bs {
				ids = append(ids, str(r["id"]))
			}
			project["supplyOrder"] = ids
		}
	case "set-resource-plan":
		rs, e := resourceRows(body["resources"])
		if e != nil {
			return e
		}
		source := text(body["sourceNote"], 1000)
		if len(rs) > 0 && source == "" {
			return errors.New("Укажите источник ручной оценки потребности.")
		}
		b["resourcePlan"], b["sourceNote"] = rs, source
	case "link-depot":
		id := marketID(body["marketId"])
		if strings.TrimSpace(str(body["marketId"])) != "" && id == "" {
			return errors.New("MarketID должен содержать только цифры.")
		}
		if e := p.uniqueDepot(id, b); e != nil {
			return e
		}
		if d := p.depots[id]; d != nil && str(d["system"]) != "" && !equal(d["system"], project["targetSystem"]) {
			return errors.New("Депо находится в другой системе.")
		}
		b["marketId"] = id
	case "update-building":
		next := cleanBuilding(merge(copyM(b), body, M{"id": b["id"], "typeId": b["typeId"]}))
		if e := p.uniqueDepot(str(next["marketId"]), b); e != nil {
			return e
		}
		if value.Bool(next["isPrimary"]) {
			for _, r := range rows(project["buildings"]) {
				if str(r["id"]) != str(b["id"]) && value.Bool(r["isPrimary"]) {
					return errors.New("Первичный порт в колонии уже указан.")
				}
			}
		}
		if d := p.depots[str(next["marketId"])]; d != nil && str(d["system"]) != "" && !equal(d["system"], project["targetSystem"]) {
			return errors.New("Депо находится в другой системе.")
		}
		bs := rows(project["buildings"])
		for i, r := range bs {
			if str(r["id"]) == str(b["id"]) {
				bs[i] = next
			}
		}
		project["buildings"] = bs
		return validateDependencies(project)
	default:
		return errors.New("Неизвестное действие планировщика колонизации.")
	}
	return nil
}
