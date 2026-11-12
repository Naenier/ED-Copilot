package presentation

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const colonySupplyLog = "Журнал снабжения"

// ColonyActions describes native controls and maps them to the domain contract.
func ColonyActions(mode string, state M, selected M) []Action {
	if mode == "Снабжение" || mode == "supplies" {
		mode = colonySupplyLog
	}
	c := v.Obj(state["colonization"])
	project := v.Obj(c["activeProject"])
	projectID := v.Str(project["id"])
	buildings := v.Rows(project["buildings"])
	catalog := v.Rows(c["catalog"])
	if len(catalog) == 0 {
		catalog = v.Rows(v.Obj(c["catalog"])["types"])
	}
	projects := v.Rows(c["projects"])
	search := v.Obj(c["search"])
	commander := v.Obj(state["commander"])
	scope, carrierChoices := "", []Choice{}
	for _, warehouse := range v.Rows(c["carrierWarehouses"]) {
		if warehouse["kind"] == "squadron" && v.Str(warehouse["id"]) != "" && v.Str(commander["fid"]) != "" {
			scope = v.Str(commander["fid"]) + "|" + v.Str(warehouse["id"])
			carrierChoices = []Choice{{Value: scope, Label: v.Text(warehouse["callsign"], "Флотоносец эскадрильи")}}
		}
	}
	actions := []Action{{ID: "colony.carrier-stock", Title: "Учёт склада флотоносца", Path: "/api/colonization", Payload: M{"action": "configure"}, Fields: []Field{boolField("includeCarrierWarehouse", "Учитывать склад флотоносца", c["includeCarrierWarehouse"]), {Key: "squadronWarehouseAccessConfirmed", Label: "Доступен груз эскадры", Kind: "bool", Default: v.Bool(c["squadronWarehouseAccessConfirmed"]), Help: "Frontier CAPI показывает общий склад, но не подтверждает право вывоза. Оставьте выключенным, если груз вам недоступен."}, {Key: "squadronWarehouseAccessScope", Label: "Флотоносец эскадрильи", Kind: "select", Default: scope, Choices: carrierChoices}}}}
	post := func(id, title, command string, payload M, fields ...Field) Action {
		return Action{ID: id, Title: title, Path: "/api/colonization", Payload: v.Merge(M{"action": command, "projectId": projectID}, payload), Fields: fields}
	}
	if len(projects) > 0 {
		choices := []Choice{}
		for _, p := range projects {
			label := v.Text(p["name"], p["targetSystem"])
			if label != v.Str(p["targetSystem"]) {
				label += " · " + v.Str(p["targetSystem"])
			}
			choices = append(choices, Choice{v.Str(p["id"]), label})
		}
		actions = append(actions, post("colony.select", "Выбрать колонию", "select-project", nil, Field{Key: "id", Label: "Колония", Kind: "select", Choices: choices, Default: projectID, Required: true, Help: "Строительство, снабжение и оверлей переключаются на выбранную колонию."}))
	}
	originDefault := v.Text(c["originSystem"], v.At(c, "config", "originSystem"), commander["system"])
	factionDefault := v.Text(c["faction"], v.At(c, "config", "faction"))
	factions := colonyNamedChoices(v.Rows(search["factions"]), "name", "name", true)
	actions = append(actions, post("colony.create", "Добавить колонию вручную", "create-project", nil, Field{Key: "targetSystem", Label: "Целевая система", Kind: "text", Required: true, Help: "Точное название системы из игры. Это создаёт личный план и не покупает заявку."}, Field{Key: "name", Label: "Название колонии в вашем списке", Kind: "text"}, Field{Key: "originSystem", Label: "Исходная система", Kind: "text", Default: originDefault}, Field{Key: "faction", Label: "Малая фракция", Kind: "combo", Choices: factions, Default: factionDefault}))
	if mode == "Выбор системы" || mode == "" || mode == "selection" {
		actions = append(actions, Action{ID: "colony.search", Title: "Найти системы для колонизации", Path: "/api/colonization/search", Fields: []Field{{Key: "originSystem", Label: "Исходная обитаемая система", Kind: "text", Default: originDefault, Required: true, Help: "Поиск покажет соседние известные базе системы в разрешённом радиусе."}, {Key: "faction", Label: "Фракция исходной станции", Kind: "combo", Choices: factions, Default: factionDefault, Help: "Важно, какая фракция контролирует исходную станцию, а не только систему."}}}, Action{ID: "colony.cancel-search", Title: "Остановить поиск", Path: "/api/colonization/cancel", Disabled: !v.Bool(search["running"])})
		if system := v.Text(selected["system"], selected["name"]); system != "" {
			actions = append(actions, Action{ID: "colony.choose-candidate", Title: "Выбрать систему для новой колонии", Path: "/api/colonization", Disabled: v.Str(selected["availability"]) == "unavailable", Payload: M{"action": "create-project", "candidateId": v.Str(selected["id"]), "targetSystem": system}, Fields: []Field{{Key: "name", Label: "Название колонии в вашем списке", Kind: "text", Default: system, Help: "Создаётся только личный план. Доступность заявки проверьте в игре."}}}, Action{ID: "colony.copy-candidate", Title: "Скопировать выбранную систему", Kind: "copy", Path: system})
		}
	}
	if projectID == "" {
		return actions
	}
	statusChoices := []Choice{{"planned", "В плане"}, {"claimed", "Заявка подана"}, {"building", "Строительство"}, {"complete", "Завершена"}, {"cancelled", "Отменена"}}
	economyChoices := colonyNamedChoices(v.Rows(c["economies"]), "id", "name", true)
	edit := post("colony.edit", "Параметры выбранной колонии", "update-project", M{"id": projectID}, Field{Key: "name", Label: "Название колонии", Kind: "text", Default: project["name"]}, Field{Key: "targetSystem", Label: "Целевая система", Kind: "text", Default: project["targetSystem"], Required: true}, Field{Key: "originSystem", Label: "Исходная система", Kind: "text", Default: project["originSystem"]}, Field{Key: "faction", Label: "Фракция", Kind: "combo", Choices: factions, Default: project["faction"]}, Field{Key: "status", Label: "Личный статус плана", Kind: "select", Choices: statusChoices, Default: v.Text(project["manualStatus"], project["status"], "planned"), Help: "Наблюдения строительного склада имеют приоритет перед личной отметкой."}, Field{Key: "goalEconomy", Label: "Направление экономики", Kind: "select", Choices: economyChoices, Default: project["goalEconomy"]}, colonyInteger("pointsTier2", "Доступные очки T2", v.At(project, "points", "tier2"), false, -100000, 100000), colonyInteger("pointsTier3", "Доступные очки T3", v.At(project, "points", "tier3"), false, -100000, 100000), colonyInteger("externalPortCount", "Начатых дополнительных портов T2/T3 вне этого плана", project["externalPortCount"], false, 0, 1000), Field{Key: "claimedAt", Label: "Дата покупки заявки", Kind: "datetime", Default: colonyDateDefault(project["claimedAt"]), Help: "ДД.ММ.ГГГГ ЧЧ:ММ. Пусто — дата неизвестна."}, Field{Key: "deadline", Label: "Свой срок завершения первичного порта", Kind: "datetime", Default: func() string {
		if v.Str(project["deadlineSource"]) == "manual" {
			return colonyDateDefault(project["deadline"])
		}
		return ""
	}(), Help: "Пусто — расчёт по дате заявки. Формат ДД.ММ.ГГГГ ЧЧ:ММ."}, Field{Key: "notes", Label: "Заметки о колонии", Kind: "multiline", Default: project["notes"]})
	edit.Transform = func(body M) (M, error) {
		points := M{}
		for _, key := range []string{"pointsTier2", "pointsTier3"} {
			n, e := colonyOptionalInteger(body[key], -100000, 100000)
			if e != nil {
				return nil, e
			}
			points[map[string]string{"pointsTier2": "tier2", "pointsTier3": "tier3"}[key]] = n
			delete(body, key)
		}
		body["points"] = points
		for _, key := range []string{"claimedAt", "deadline"} {
			date, e := colonyParseDate(body[key])
			if e != nil {
				label := map[string]string{"claimedAt": "Дата покупки заявки", "deadline": "Срок завершения"}[key]
				return nil, fmt.Errorf("%s: %w", label, e)
			}
			body[key] = date
		}
		return body, nil
	}
	actions = append(actions, edit, Action{ID: "colony.capacity", Title: "Вместимость перевозки для этой колонии", Path: "/api/colonization", Payload: M{"action": "configure", "projectId": projectID}, Fields: []Field{colonyInteger("cargoCapacity", "Тонн груза за рейс", project["cargoCapacity"], false, 1, 100000)}, Transform: func(body M) (M, error) {
		n, e := colonyOptionalInteger(body["cargoCapacity"], 1, 100000)
		if e != nil {
			return nil, e
		}
		body["cargoCapacity"] = n
		return body, nil
	}}, Action{ID: "colony.export-csv", Title: "Экспорт снабжения колонии в CSV", Kind: "export", Path: "/api/colonization/resources.csv", Payload: M{"projectId": projectID}}, Action{ID: "colony.export-json", Title: "Экспорт плана колонии в JSON", Kind: "export", Path: "/api/colonization/plan.json", Payload: M{"projectId": projectID}}, Action{ID: "colony.copy-system", Title: "Скопировать систему колонии", Kind: "copy", Path: v.Str(project["targetSystem"])}, Action{ID: "colony.delete", Title: "Удалить личный план колонии", Path: "/api/colonization", Payload: M{"action": "remove-project", "id": projectID}, Confirm: "Удалить личный план «" + v.Text(project["name"], project["targetSystem"]) + "»? Это удалит его очередь и личные отметки в ED Copilot; колония в игре останется."})
	buildChoices := colonyBuildingChoices(buildings)
	selectedBuild := colonySelectedBuilding(buildings, selected)
	defaultBuilding := v.Str(selectedBuild["id"])
	if defaultBuilding == "" && len(buildings) > 0 {
		defaultBuilding = v.Str(buildings[0]["id"])
	}
	if mode == "Строительство" || mode == "construction" {
		typeChoices := colonyNamedChoices(catalog, "id", "name", false)
		primaryExists := false
		for _, b := range buildings {
			primaryExists = primaryExists || v.Bool(b["isPrimary"])
		}
		add := post("colony.add-building", "Добавить объект строительства", "add-building", nil, Field{Key: "typeId", Label: "Тип объекта", Kind: "select", Choices: typeChoices, Required: true, Default: func() string {
			if len(typeChoices) > 0 {
				return typeChoices[0].Value
			}
			return ""
		}()}, Field{Key: "name", Label: "Имя объекта в плане", Kind: "text"}, Field{Key: "body", Label: "Небесное тело", Kind: "text"}, Field{Key: "isPrimary", Label: "Первичный порт системы", Kind: "bool", Default: !primaryExists, Help: "Не каждый тип подходит для первичного порта."}, Field{Key: "notes", Label: "Заметки", Kind: "multiline"})
		add.Disabled = len(typeChoices) == 0
		add.Transform = func(body M) (M, error) {
			if v.Bool(body["isPrimary"]) {
				allowed := false
				for _, typ := range catalog {
					if v.Str(typ["id"]) == v.Str(body["typeId"]) {
						allowed = v.Bool(typ["canBePrimary"])
						break
					}
				}
				if !allowed {
					return nil, errors.New("Выбранный тип объекта не может быть первичным портом.")
				}
			}
			return body, nil
		}
		actions = append(actions, add)
	}
	if len(selectedBuild) > 0 && (mode == "Строительство" || mode == "construction" || mode == "Очередь снабжения") {
		buildingID := v.Str(selectedBuild["id"])
		bs := []Choice{{"planned", "В плане"}, {"building", "Строится"}, {"complete", "Построено"}, {"cancelled", "Отменено"}}
		fields := []Field{{Key: "name", Label: "Имя объекта", Kind: "text", Default: selectedBuild["name"]}, {Key: "body", Label: "Небесное тело", Kind: "text", Default: selectedBuild["body"]}, {Key: "status", Label: "Личный статус", Kind: "select", Choices: bs, Default: v.Text(selectedBuild["manualStatus"], selectedBuild["status"], "planned")}, {Key: "tierOverride", Label: "Уровень по экрану архитектора", Kind: "select", Choices: []Choice{{"", "По каталогу"}, {"1", "T1"}, {"2", "T2"}, {"3", "T3"}}, Default: colonyOptionalText(selectedBuild["tierOverride"])}, colonyInteger("pointCostOverride", "Стоимость начала в очках — свой вариант", selectedBuild["pointCostOverride"], false, 0, 100000), colonyInteger("pointRewardOverride", "Очки за завершение — свой вариант", selectedBuild["pointRewardOverride"], false, 0, 100000), {Key: "notes", Label: "Заметки", Kind: "multiline", Default: selectedBuild["notes"]}}
		typeChoices := colonyNamedChoices(catalog, "id", "name", false)
		if len(typeChoices) > 0 {
			fields = append([]Field{{Key: "typeId", Label: "Тип объекта", Kind: "select", Choices: typeChoices, Required: true, Default: v.Text(selectedBuild["catalogTypeId"], selectedBuild["typeId"])}}, fields...)
		}
		editBuild := post("colony.edit-building", "Изменить выбранный объект", "update-building", M{"id": buildingID}, fields...)
		editBuild.Transform = func(body M) (M, error) {
			for _, key := range []string{"tierOverride", "pointCostOverride", "pointRewardOverride"} {
				min, max := float64(0), float64(100000)
				if key == "tierOverride" {
					min, max = 1, 3
				}
				n, e := colonyOptionalInteger(body[key], min, max)
				if e != nil {
					return nil, e
				}
				body[key] = n
			}
			return body, nil
		}
		actions = append(actions, editBuild)
		command, upTitle, downTitle := "move-building", "Передвинуть стройку выше", "Передвинуть стройку ниже"
		order := buildings
		if mode == "Очередь снабжения" {
			command, upTitle, downTitle = "move-supply-building", "Повысить приоритет поставок", "Понизить приоритет поставок"
			order = v.Rows(project["supplyQueue"])
		}
		index := -1
		for i, b := range order {
			if v.Str(b["id"]) == buildingID {
				index = i
			}
		}
		up := post("colony.move-up", upTitle, command, M{"id": buildingID, "direction": "up"})
		up.Disabled = index <= 0
		down := post("colony.move-down", downTitle, command, M{"id": buildingID, "direction": "down"})
		down.Disabled = index < 0 || index >= len(order)-1
		actions = append(actions, up, down)
		if mode != "Очередь снабжения" {
			supply := v.Rows(project["supplyQueue"])
			for index, stage := range supply {
				if v.Str(stage["id"]) != buildingID {
					continue
				}
				priorityUp := post("colony.supply-up", "Повысить приоритет поставок", "move-supply-building", M{"id": buildingID, "direction": "up"})
				priorityDown := post("colony.supply-down", "Понизить приоритет поставок", "move-supply-building", M{"id": buildingID, "direction": "down"})
				priorityUp.Disabled = index == 0
				priorityDown.Disabled = index >= len(supply)-1
				actions = append(actions, priorityUp, priorityDown)
				break
			}
		}
		dependencies := []Choice{}
		existing := []Choice{}
		for _, b := range buildings {
			if v.Str(b["id"]) == buildingID {
				continue
			}
			choice := Choice{v.Str(b["id"]), v.Text(b["name"], b["id"])}
			dependencies = append(dependencies, choice)
			if colonyContains(v.Arr(selectedBuild["dependsOn"]), choice.Value) {
				existing = append(existing, choice)
			}
		}
		for _, remove := range []bool{false, true} {
			choices, title, id := dependencies, "Добавить предшествующий объект", "colony.dependency-add"
			if remove {
				choices, title, id = existing, "Убрать зависимость от объекта", "colony.dependency-remove"
			}
			if len(choices) == 0 {
				continue
			}
			base := append([]any(nil), v.Arr(selectedBuild["dependsOn"])...)
			removing := remove
			dependency := post(id, title, "update-building", M{"id": buildingID}, Field{Key: "dependencyId", Label: "Предшествующий объект", Kind: "select", Choices: choices, Default: choices[0].Value, Required: true})
			dependency.Transform = func(body M) (M, error) {
				id := v.Str(body["dependencyId"])
				if id == "" || id == buildingID {
					return nil, errors.New("Выберите другой объект этой колонии.")
				}
				deps := []any{}
				for _, old := range base {
					if v.Str(old) != id {
						deps = append(deps, old)
					}
				}
				if !removing {
					deps = append(deps, id)
				}
				body["dependsOn"] = deps
				delete(body, "dependencyId")
				return body, nil
			}
			actions = append(actions, dependency)
		}
		depots := []Choice{{"", "Убрать связь с площадкой"}}
		for _, d := range v.Rows(c["depots"]) {
			if system := v.Str(d["system"]); system != "" && !strings.EqualFold(system, v.Str(project["targetSystem"])) {
				continue
			}
			if v.Bool(d["linked"]) && v.Str(d["marketId"]) != v.Str(selectedBuild["marketId"]) {
				continue
			}
			depots = append(depots, Choice{v.Str(d["marketId"]), v.Text(d["stationName"], d["station"], d["name"], d["marketId"])})
		}
		if current := v.Str(selectedBuild["marketId"]); current != "" {
			found := false
			for _, choice := range depots {
				found = found || choice.Value == current
			}
			if !found {
				depots = append(depots, Choice{current, "Сохранённая площадка · " + current + " (ожидание журнала)"})
			}
		}
		actions = append(actions, post("colony.link-depot", "Связать со стройплощадкой из журнала", "link-depot", M{"buildingId": buildingID}, Field{Key: "marketId", Label: "Строительная площадка", Kind: "select", Choices: depots, Default: v.Str(selectedBuild["marketId"]), Help: "Здесь только площадки этой системы. Требования обновятся по журналу."}), Action{ID: "colony.delete-building", Title: "Убрать выбранный объект из плана", Path: "/api/colonization", Payload: M{"action": "remove-building", "projectId": projectID, "id": buildingID}, Confirm: "Удалить этап «" + v.Text(selectedBuild["name"], selectedBuild["typeId"]) + "» и его личные отметки? Это не отменяет строительство в игре."})
	}
	if len(buildings) > 0 && (mode == colonySupplyLog || mode == "Бюджет" || mode == "Строительство" || mode == "construction" || mode == "Очередь снабжения" || mode == "Поставки") {
		goods := colonyGoods(state, project)
		commodity := v.Text(selected["commodity"], selected["key"])
		required := selected["required"]
		if len(selectedBuild) > 0 {
			for _, resource := range v.Rows(selectedBuild["requirements"]) {
				if v.Str(resource["commodity"]) == commodity {
					required = resource["required"]
					break
				}
			}
			for _, resource := range v.Rows(selectedBuild["resourcePlan"]) {
				if v.Str(resource["commodity"]) == commodity {
					required = resource["required"]
					break
				}
			}
			if rs := v.Rows(selectedBuild["resourcePlan"]); len(rs) > 0 && commodity == "" {
				commodity = v.Str(rs[0]["commodity"])
				required = rs[0]["required"]
			}
		}
		sourceDefault := selectedBuild["sourceNote"]
		resource := Action{ID: "colony.resource-upsert", Title: "Плановая потребность в товаре", Path: "/colonization/resource-row", Payload: M{"projectId": projectID}, Fields: []Field{{Key: "buildingId", Label: "Объект этой колонии", Kind: "select", Choices: buildChoices, Default: defaultBuilding, Required: true}, {Key: "commodity", Label: "Товар", Kind: "combo", Choices: goods, Default: commodity, Required: true}, colonyInteger("required", "Нужно всего, т", required, true, 1, 1e9), {Key: "sourceNote", Label: "Основание плановой оценки", Kind: "text", Default: sourceDefault, Required: true, Help: "Это ручная оценка. Точный список игрового склада имеет приоритет."}}, Transform: colonyResourceBody}
		actions = append(actions, resource)
		if mode == colonySupplyLog || mode == "Бюджет" {
			manualBuildings := []Choice{}
			manualGoods := []Choice{}
			seen := map[string]bool{}
			for _, b := range buildings {
				if len(v.Rows(b["resourcePlan"])) == 0 {
					continue
				}
				manualBuildings = append(manualBuildings, Choice{v.Str(b["id"]), v.Str(b["name"])})
				for _, r := range v.Rows(b["resourcePlan"]) {
					key := v.Str(r["commodity"])
					if !seen[key] {
						manualGoods = append(manualGoods, Choice{key, v.Text(r["name"], key)})
						seen[key] = true
					}
				}
			}
			if len(manualBuildings) > 0 {
				actions = append(actions, Action{ID: "colony.resource-remove", Title: "Убрать товар из ручной сметы", Path: "/colonization/resource-row", Payload: M{"projectId": projectID, "remove": true}, Confirm: "Удалить строку только из ручной оценки? Подтверждённые требования игры сохранятся.", Fields: []Field{{Key: "buildingId", Label: "Объект", Kind: "select", Choices: manualBuildings, Default: manualBuildings[0].Value, Required: true}, {Key: "commodity", Label: "Товар из ручной сметы", Kind: "select", Choices: manualGoods, Default: func() string {
					if seen[commodity] {
						return commodity
					}
					return manualGoods[0].Value
				}(), Required: true}}})
			}
			if commodity != "" {
				need, exact := v.Number(selected["toBuy"])
				if !exact {
					var bounded bool
					need, bounded = v.Number(selected["toBuyAtMost"])
					if !bounded {
						need = v.Num(selected["missing"])
					}
				}
				quantity := math.Max(1, math.Ceil(need))
				actions = append(actions, Action{ID: "colony.purchase", Title: "Найти, где купить выбранный товар", Kind: "purchase", Path: "sale", Disabled: exact && need <= 0, Payload: M{"resourceKey": commodity, "quantity": quantity, "system": v.Text(commander["system"], project["originSystem"])}, Fields: []Field{colonyInteger("quantity", "Найти объём, т", quantity, true, 1, 1e9), {Key: "system", Label: "Искать рядом с системой", Kind: "text", Default: v.Text(commander["system"], project["originSystem"]), Required: true}}})
			}
		}
		if mode == "Поставки" || mode == colonySupplyLog || mode == "Строительство" || mode == "construction" {
			delivery := post("colony.delivery-add", "Отметить личную поставку", "record-delivery", nil, Field{Key: "buildingId", Label: "Объект этой колонии", Kind: "select", Choices: buildChoices, Default: defaultBuilding, Required: true}, Field{Key: "commodity", Label: "Товар", Kind: "combo", Choices: goods, Default: commodity, Required: true}, colonyInteger("count", "Передано, т", nil, true, 1, 1e9), Field{Key: "notes", Label: "Примечание", Kind: "multiline", Help: "Личная отметка не прибавляется повторно к принятому объёму игрового склада."})
			delivery.Transform = func(body M) (M, error) {
				n, e := colonyOptionalInteger(body["count"], 1, 1e9)
				if e != nil || n == nil {
					return nil, errors.New("Укажите положительное целое количество тонн.")
				}
				body["count"] = n
				body["commodity"] = v.Key(body["commodity"])
				if v.Str(body["commodity"]) == "" {
					return nil, errors.New("Выберите товар.")
				}
				return body, nil
			}
			actions = append(actions, delivery)
		}
	}
	if (mode == "Поставки" || mode == colonySupplyLog) && v.Str(selected["source"]) == "manual" && v.Str(selected["id"]) != "" {
		actions = append(actions, Action{ID: "colony.delivery-remove", Title: "Удалить выбранную личную отметку", Path: "/api/colonization", Payload: M{"action": "remove-delivery", "projectId": projectID, "id": selected["id"]}, Confirm: "Удалить личную отметку доставки? События журнала игры останутся."})
	}
	return actions
}

func colonyInteger(key, label string, def any, required bool, min, max float64) Field {
	return Field{Key: key, Label: label, Kind: "integer", Default: def, Required: required, Min: min, Max: max}
}
func colonyNamedChoices(rows []M, key, label string, empty bool) []Choice {
	out := []Choice{}
	if empty {
		out = append(out, Choice{"", "Не задано"})
	}
	for _, r := range rows {
		id := v.Str(r[key])
		if id != "" {
			out = append(out, Choice{id, v.Text(r[label], id)})
		}
	}
	return out
}
func colonyBuildingChoices(buildings []M) []Choice {
	out := []Choice{}
	for _, b := range buildings {
		out = append(out, Choice{v.Str(b["id"]), v.Text(b["name"], b["typeId"])})
	}
	return out
}
func colonySelectedBuilding(buildings []M, selected M) M {
	for _, b := range buildings {
		if v.Str(b["id"]) == v.Str(selected["id"]) || v.Str(b["id"]) == v.Str(selected["buildingId"]) {
			return b
		}
	}
	for _, ref := range v.Rows(selected["buildings"]) {
		for _, building := range buildings {
			if v.Str(building["id"]) == v.Str(ref["id"]) {
				return building
			}
		}
	}
	return M{}
}
func colonyContains(values []any, id string) bool {
	for _, x := range values {
		if v.Str(x) == id {
			return true
		}
	}
	return false
}
func colonyOptionalInteger(raw any, min, max float64) (any, error) {
	if raw == nil || strings.TrimSpace(v.Str(raw)) == "" {
		return nil, nil
	}
	n, ok := v.Number(raw)
	if !ok || math.Trunc(n) != n || n < min || n > max {
		return nil, fmt.Errorf("Укажите целое число от %.0f до %.0f или оставьте поле пустым.", min, max)
	}
	return n, nil
}
func colonyOptionalText(raw any) string {
	if raw == nil {
		return ""
	}
	return v.Str(raw)
}
func colonyDateDefault(raw any) string {
	t := v.Time(raw)
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("02.01.2006 15:04")
}
func colonyParseDate(raw any) (any, error) {
	s := strings.TrimSpace(v.Str(raw))
	if s == "" {
		return nil, nil
	}
	if t := v.Time(s); !t.IsZero() {
		return v.ISO(t), nil
	}
	for _, layout := range []string{"02.01.2006 15:04", "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02"} {
		if t, e := time.ParseInLocation(layout, s, time.Local); e == nil {
			return v.ISO(t), nil
		}
	}
	return nil, errors.New("введите дату в формате ДД.ММ.ГГГГ ЧЧ:ММ")
}
func colonyResourceBody(body M) (M, error) {
	key := v.Key(body["commodity"])
	required, e := colonyOptionalInteger(body["required"], 1, 1e9)
	if e != nil || required == nil {
		return nil, errors.New("Укажите положительную целую потребность в тоннах.")
	}
	if key == "" {
		return nil, errors.New("Выберите товар.")
	}
	if strings.TrimSpace(v.Str(body["sourceNote"])) == "" {
		return nil, errors.New("Укажите основание ручной оценки.")
	}
	body["commodity"], body["required"] = key, required
	return body, nil
}
func colonyGoods(state, project M) []Choice {
	by := map[string]string{}
	for _, list := range []any{state["goods"], state["resources"]} {
		for _, r := range v.Rows(list) {
			if v.Str(r["kind"]) == "raw" {
				continue
			}
			key := v.Text(r["key"], r["commodity"])
			if key != "" {
				by[key] = v.Text(r["name"], key)
			}
		}
	}
	for _, b := range v.Rows(project["buildings"]) {
		for _, key := range []string{"requirements", "resourcePlan"} {
			for _, r := range v.Rows(b[key]) {
				if id := v.Str(r["commodity"]); id != "" {
					by[id] = v.Text(r["name"], id)
				}
			}
		}
	}
	out := []Choice{}
	for id, label := range by {
		out = append(out, Choice{id, label})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}
