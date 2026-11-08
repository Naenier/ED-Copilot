package presentation

import (
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"sort"
	"strings"
)

func choices(rows []M, key, label string) []Choice {
	out := []Choice{}
	for _, r := range rows {
		out = append(out, Choice{v.Str(r[key]), v.Str(r[label])})
	}
	return out
}
func ChoicePairs(values ...string) []Choice {
	r := []Choice{}
	for i := 0; i+1 < len(values); i += 2 {
		r = append(r, Choice{values[i], values[i+1]})
	}
	return r
}
func numberField(key, label string, value any, min, max float64) Field {
	return Field{Key: key, Label: label, Kind: "number", Default: value, Min: min, Max: max}
}
func boolField(key, label string, value any) Field {
	return Field{Key: key, Label: label, Kind: "bool", Default: v.Bool(value)}
}
func textField(key, label string, value any) Field {
	return Field{Key: key, Label: label, Kind: "text", Default: value}
}
func selectField(key, label string, value any, items []Choice) Field {
	return Field{Key: key, Label: label, Kind: "select", Default: value, Choices: items}
}
func goodsChoices(s M) []Choice {
	return resourceChoices(s, false)
}
func resourceChoices(s M, includeRaw bool) []Choice {
	unique := map[string]Choice{}
	for _, r := range append(v.Rows(s["goods"]), v.Rows(s["resources"])...) {
		if includeRaw || r["kind"] != "raw" {
			key := v.Str(r["key"])
			if key != "" {
				unique[key] = Choice{key, v.Str(r["name"])}
			}
		}
	}
	out := []Choice{}
	for _, r := range unique {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}
func coreActions(tab, mode string, state, selected, prefill M) []Action {
	cmd := v.Obj(state["commander"])
	settings := v.Obj(state["settings"])
	hasRow := len(selected) > 0
	newAction := func(id, title, path string, payload M, fields ...Field) Action {
		return Action{ID: id, Title: title, Path: path, Payload: payload, Fields: fields}
	}
	export := func(id, title, path string) Action { return Action{ID: id, Title: title, Path: path, Kind: "export"} }
	copySystem := func(system any) Action {
		return Action{ID: "copy.system", Title: "Копировать систему", Kind: "copy", Path: v.Str(system), Disabled: v.Str(system) == ""}
	}
	switch tab {
	case "exobiology":
		return ExobiologyActions(mode, state, selected)
	case "learning":
		return LearningActions(mode, state, selected)
	case "carrier-calculator":
		return CarrierCalculatorActions(mode, state, selected)
	case "carrier-trade":
		return CarrierTradeActions(mode, state, selected)
	case "colonization":
		return ColonyActions(mode, state, selected)
	case "overview", "targets":
		if tab == "targets" && mode == "Сохранённые места" {
			systems, seen := []string{}, map[string]bool{}
			for _, r := range v.Rows(state["bookmarks"]) {
				if system := v.Str(r["system"]); system != "" && !seen[system] {
					systems, seen[system] = append(systems, system), true
				}
			}
			return []Action{copySystem(selected["system"]), {ID: "bookmark.remove", Title: "Убрать сохранённое место", Path: "/api/bookmarks", Payload: M{"action": "remove", "id": selected["id"]}, Disabled: !hasRow}, {ID: "bookmarks.copy", Title: "Копировать все системы", Kind: "copy", Path: strings.Join(systems, "\r\n"), Disabled: len(systems) == 0}, {ID: "bookmarks.search", Title: "Найти место добычи", Kind: "navigate", Path: "targets", Payload: M{"mode": "Места добычи"}}}
		}
		if tab == "targets" && mode == "Места добычи" {
			return []Action{newAction("mining.search", "Найти места добычи", "/api/search", nil, textField("system", "Исходная система", cmd["system"]), numberField("radiusLy", "Радиус, св. л.", v.First(settings["radiusLy"], 50), 5, 100), selectField("resourceKey", "Ресурс, необязательно", "", append([]Choice{{"", "Любой ценный ресурс"}}, goodsChoices(state)...))), newAction("mining.stop", "Остановить поиск", "/api/search/cancel", nil), {ID: "target.bookmark", Title: "Сохранить место", Path: "/api/bookmarks", Payload: M{"action": "add", "targetId": selected["id"]}, Disabled: !hasRow}, copySystem(selected["system"])}
		}
		if tab == "targets" && mode == "Навигация" {
			return []Action{copySystem(v.First(selected["system"], v.At(state, "pilot", "navTarget", "system"))), {ID: "nav.route.carrier", Title: "Маршрут флотоносца", Kind: "navigate", Path: "route"}}
		}
		if tab == "targets" && mode == "Инженеры" {
			return CoreActions("engineering", "Инженеры", state, selected, prefill)
		}
		goalFields := func(r M) []Field {
			return []Field{{Key: "title", Label: "Что нужно сделать", Kind: "text", Required: true, Default: r["title"]}, selectField("kind", "Занятие", v.First(r["kind"], "travel"), ChoicePairs("travel", "Перелёт", "trade", "Торговля", "mining", "Добыча", "engineering", "Инженерия", "exploration", "Исследования", "combat", "Бой", "mission", "Поручение", "carrier", "Флотоносец", "colonization", "Колонизация", "odyssey", "Пешие задачи", "other", "Другое")), textField("system", "Система", r["system"]), textField("station", "Станция", r["station"]), textField("body", "Тело", r["body"]), selectField("priority", "Приоритет", v.First(r["priority"], "normal"), ChoicePairs("normal", "Обычный", "high", "Высокий")), {Key: "notes", Label: "Заметки", Kind: "multiline", Default: r["notes"]}}
		}
		isGoal := hasRow && selected["title"] != nil && selected["kind"] != nil
		actions := []Action{newAction("goal.add", "Новая цель", "/api/objectives", M{"action": "add"}, goalFields(M{})...), newAction("goal.edit", "Изменить цель", "/api/objectives", M{"action": "update", "id": selected["id"]}, goalFields(selected)...), newAction("goal.complete", "Выполнено", "/api/objectives", M{"action": "complete", "id": selected["id"]}), newAction("goal.reopen", "Вернуть в работу", "/api/objectives", M{"action": "reopen", "id": selected["id"]}), {ID: "goal.delete", Title: "Удалить цель", Path: "/api/objectives", Payload: M{"action": "remove", "id": selected["id"]}, Confirm: "Удалить выбранную личную цель?"}, copySystem(v.First(selected["system"], cmd["system"])), {ID: "overview.colonies", Title: "Колонии", Kind: "navigate", Path: "colonization"}}
		for i := 1; i <= 4; i++ {
			actions[i].Disabled = !isGoal
		}
		return actions
	case "sale":
		buying := mode == "Купить"
		tradeMode := "sell"
		title := "Найти покупателей"
		if buying {
			tradeMode = "buy"
			title = "Найти продавцов"
		}
		inputs := v.Merge(M{"system": cmd["system"], "quantity": 1, "resourceKey": "", "radiusLy": 100, "maxAgeHours": 24, "carrierMaxAgeHours": 6, "maxArrivalLs": 10000, "minDemand": 1, "minSupply": 1, "demandRatio": 4, "minPad": "L", "useCargo": !buying}, prefill)
		if buying && v.Str(inputs["resourceKey"]) == "" {
			inputs["resourceKey"] = "tritium"
		}
		marketGoods := goodsChoices(state)
		if !buying {
			marketGoods = append([]Choice{{"", "Весь выбранный груз"}}, marketGoods...)
		}
		fields := []Field{textField("system", "От какой системы искать", inputs["system"]), selectField("resourceKey", "Товар", inputs["resourceKey"], marketGoods), numberField("quantity", "Количество, т (если выбран товар)", inputs["quantity"], 1, 25000), boolField("useCargo", "Использовать груз текущего корабля / ТРП", inputs["useCargo"]), boolField("useCarrierCargo", "Использовать известный склад личного флотоносца", inputs["useCarrierCargo"]), numberField("radiusLy", "Радиус, св. л.", inputs["radiusLy"], 5, 5000), selectField("minPad", "Посадочная площадка", inputs["minPad"], ChoicePairs("L", "Большая", "M", "Средняя или большая", "S", "Любая корабельная", "any", "Без ограничения")), numberField("maxAgeHours", "Возраст цен станций, ч", inputs["maxAgeHours"], 1, 720), numberField("carrierMaxAgeHours", "Возраст цен флотоносцев, ч", inputs["carrierMaxAgeHours"], 0.25, 168), boolField("includeCarriers", "Включать чужие флотоносцы", inputs["includeCarriers"]), numberField("maxArrivalLs", "Максимум от звезды, св. с.", inputs["maxArrivalLs"], 0, 1e7), numberField("minDemand", "Минимальный спрос, т", inputs["minDemand"], 0, 1e9), numberField("minSupply", "Минимальный запас, т", inputs["minSupply"], 0, 1e9), numberField("demandRatio", "Запас спроса к вашему грузу", inputs["demandRatio"], 1, 20)}
		search := newAction("market.search", title, "/api/trading/search", M{"mode": tradeMode}, fields...)
		visible := []Field{}
		for _, field := range search.Fields {
			if buying && (field.Key == "useCargo" || field.Key == "useCarrierCargo" || field.Key == "minDemand" || field.Key == "demandRatio") || !buying && field.Key == "minSupply" {
				continue
			}
			visible = append(visible, field)
		}
		search.Fields = visible
		selectedMarket := v.Copy(selected)
		selectedMarket["selectionMode"] = tradeMode
		selectedMarket["quoteOptions"] = v.Copy(v.Obj(v.At(state, "trading", "options")))
		destinationLabel := "Везти сюда"
		if buying {
			destinationLabel = "Назначить закупку"
		}
		plan := v.Obj(state["tradePlan"])
		quote := M{}
		if q := v.Rows(selected["quotes"]); len(q) > 0 {
			quote = q[0]
		}
		if quote["key"] != nil && plan["key"] != nil && quote["key"] != plan["key"] {
			plan = M{}
		}
		buyPrice, sellPrice := v.First(plan["buyPrice"], 0), v.First(quote["price"], plan["sellPrice"], 0)
		source, destination := v.First(plan["source"], cmd["system"]), v.First(selected["system"], plan["destination"])
		if buying {
			buyPrice, sellPrice = v.First(quote["price"], plan["buyPrice"], 0), v.First(plan["sellPrice"], 0)
			source, destination = v.First(selected["system"], plan["source"], cmd["system"]), plan["destination"]
		}
		planFields := []Field{selectField("key", "Товар", v.First(quote["key"], plan["key"], "gold"), goodsChoices(state)), numberField("quantity", "Объём, т", v.First(quote["quantity"], plan["quantity"], 1), 1, 25000), numberField("buyPrice", "Закупочная цена, кр./т", buyPrice, 0, 1e9), numberField("sellPrice", "Продажная цена, кр./т", sellPrice, 0, 1e9), textField("source", "Откуда", source), textField("destination", "Куда", destination)}
		return []Action{search, {ID: "market.destination", Title: destinationLabel, Path: "/api/trading/destination", Payload: M{"destination": selectedMarket}, Disabled: len(v.Rows(selected["quotes"])) == 0}, newAction("trade.plan", "План перевозки", "/api/trading/plan", nil, planFields...), newAction("market.cancel", "Остановить поиск", "/api/trading/cancel", nil), newAction("market.clear", "Убрать выбранный рынок", "/api/trading/destination", M{"destination": nil}), copySystem(selected["system"]), export("trade.export", "Экспорт сделок CSV", "/api/trading/export.csv")}
	case "deals":
		return DealsActions(state, selected)
	case "carrier":
		c := v.Obj(state["carrier"])
		planning := v.Obj(c["planning"])
		return []Action{newAction("carrier.planning", "Параметры флотоносца", "/api/carrier/planning", nil, numberField("weeklyUpkeep", "Содержание в неделю, кр.", v.At(c, "finance", "weeklyUpkeep"), 0, 1e12), numberField("reserveTritium", "Резерв трития, т", planning["reserveTritium"], 0, 25000), numberField("targetTankTons", "Целевой запас в баке, т", v.First(planning["targetTankTons"], 1000), 0, 1000)), {ID: "carrier.stock", Title: "Уточнить остаток товара", Path: "/carrier/stock-row", Fields: []Field{selectField("key", "Товар", selected["key"], goodsChoices(state)), numberField("count", "Фактический остаток, т", selected["count"], 0, 25000), {Key: "complete", Label: "Таблица содержит полный список товаров склада", Kind: "bool", Default: v.At(c, "warehouse", "complete"), Help: "Если полный состав неизвестен, оставьте выключенным. Отсутствующий в неполном снимке товар останется неизвестным."}}}, {ID: "carrier.stock.frontier", Title: "Склад из Frontier", Path: "/api/frontier/carrier-stock"}, {ID: "carrier.route", Title: "Маршрут флотоносца", Kind: "navigate", Path: "route"}, {ID: "carrier.tritium", Title: "Купить тритий", Kind: "purchase", Path: "sale", Payload: M{"resourceKey": "tritium", "quantity": 1000, "system": cmd["system"]}}, copySystem(v.At(c, "location", "system"))}
	case "route":
		c := v.Obj(state["carrier"])
		routeState := v.Obj(state["carrierRoute"])
		o := v.Obj(routeState["options"])
		if (routeState["status"] == nil || routeState["status"] == "idle") && v.Str(o["sourceSystem"]) == "" && v.Str(o["destinationSystem"]) == "" {
			o = M{}
		}
		usedCapacity := 0.0
		minimumFuel := o["calculateStartingFuel"]
		if minimumFuel == nil {
			minimumFuel = v.At(c, "fuel", "tankTons") == nil && v.At(c, "fuel", "reserveTons") == nil
		}
		if capacity, ok := v.Number(v.At(c, "space", "capacityTons")); ok {
			if free, known := v.Number(v.At(c, "space", "freeTons")); known {
				usedCapacity = capacity - free
			}
		}
		fields := []Field{textField("sourceSystem", "Откуда", v.First(o["sourceSystem"], v.At(c, "location", "system"), cmd["system"])), {Key: "destinationSystem", Label: "Куда", Kind: "text", Required: true, Default: o["destinationSystem"]}, {Key: "waypoints", Label: "Промежуточные системы — по одной в строке", Kind: "multiline", Default: strings.Join(toStrings(o["waypoints"]), "\r\n")}, {Key: "refuelSystems", Label: "Системы дозаправки — по одной в строке", Kind: "multiline", Default: strings.Join(toStrings(o["refuelSystems"]), "\r\n")}, boolField("calculateStartingFuel", "Рассчитать минимальный стартовый запас трития", minimumFuel), numberField("usedCapacityTons", "Занято вместимости флотоносца, т", v.First(o["usedCapacityTons"], usedCapacity), 0, 25000), numberField("tankTons", "Тритий в баке, т", v.First(o["tankTons"], v.At(c, "fuel", "tankTons"), 1000), 0, 1000), numberField("cargoTritiumTons", "Тритий на складе, т", v.First(o["cargoTritiumTons"], v.At(c, "fuel", "reserveTons"), 0), 0, 25000), numberField("reserveTritiumTons", "Оставить трития по прибытии, т", v.First(o["reserveTritiumTons"], v.At(c, "planning", "reserveTritium"), 0), 0, 26000), {Key: "departureTime", Label: "Начало маршрута", Kind: "datetime", Default: o["departureTime"], Help: "Пусто — начинать сейчас. Время в вашем часовом поясе."}, numberField("jumpMinutes", "Подготовка прыжка, мин", v.First(o["jumpMinutes"], 15), 1, 240), numberField("cooldownMinutes", "Восстановление, мин", v.First(o["cooldownMinutes"], 5), 0, 120), numberField("overheadMinutes", "Операции между прыжками, мин", v.First(o["overheadMinutes"], 1), 0, 240), numberField("maxQueueMinutes", "Возможная дополнительная очередь, мин", v.First(o["maxQueueMinutes"], 0), 0, 240), selectField("refuelMethod", "Пополнение трития", v.First(o["refuelMethod"], "mine"), ChoicePairs("mine", "Добывать", "buy", "Покупать", "manual", "Перегружать")), numberField("miningTonsPerHour", "Темп добычи трития, т/ч", v.First(o["miningTonsPerHour"], 150), 1, 10000), numberField("refuelMinutes", "Время получения и доставки трития на остановке, мин", v.First(o["refuelMinutes"], 0), 0, 10080), numberField("tritiumPrice", "Цена трития, кр./т", v.First(o["tritiumPrice"], 0), 0, 1e8)}
		for i := range fields {
			if fields[i].Key == "calculateStartingFuel" {
				fields[i].Help = "Выключено — учитывать указанные фактические запасы. Включено — рассчитать гипотетический минимальный запас с соответствующей массой флотоносца."
			}
		}
		return []Action{newAction("route.calculate", "Рассчитать маршрут", "/api/carrier/route", nil, fields...), {ID: "route.visited", Title: "Пройдено до этой остановки", Path: "/api/carrier/route/visited", Payload: M{"index": selected["index"]}, Disabled: selected["index"] == nil, Confirm: "Отметить этот этап маршрута пройденным в личном плане?"}, export("route.export", "Экспорт маршрута CSV", "/api/carrier/route.csv"), newAction("route.cancel", "Остановить расчёт", "/api/carrier/route/cancel", nil), copySystem(v.First(selected["name"], selected["system"]))}
	case "engineering":
		e := v.Obj(state["engineering"])
		catalog := v.Obj(e["catalog"])
		if len(catalog) == 0 {
			catalog = data.Object("engineering-data.json")
		}
		if _, _, materials := materialView(mode); materials {
			shopping := []string{}
			for _, r := range v.Rows(e["materials"]) {
				if v.Num(r["needed"]) > 0 {
					shopping = append(shopping, v.Str(r["name"])+": нужно "+Number(r["needed"])+", есть "+Number(r["owned"])+", не хватает "+Number(r["missing"]))
				}
			}
			material := false
			for _, r := range v.Rows(e["materials"]) {
				if r["key"] == selected["key"] && r["tradeable"] != false {
					material = true
					break
				}
			}
			return []Action{{ID: "engineering.material-trade", Title: "Подобрать обмен для материала", Kind: "engineering-trade", Path: "/api/engineering", Payload: M{"action": "configure", "tradeTargetKey": selected["key"], "tradeTargetAmount": max(1, v.Num(selected["missing"]))}, Disabled: !material}, {ID: "engineering.copy-shopping", Title: "Копировать список материалов плана", Kind: "copy", Path: strings.Join(shopping, "\r\n"), Disabled: len(shopping) == 0}, export("engineering.materials", "Список материалов CSV", "/api/engineering/materials.csv"), {ID: "engineering.go-plan", Title: "Список улучшений", Kind: "navigate", Path: "engineering", Payload: M{"mode": "План"}}}
		}
		if mode == "Инженеры" {
			return []Action{{ID: "engineer.mark", Title: "Отметить доступ", Path: "/api/engineering", Payload: M{"action": "engineer", "key": selected["key"]}, Fields: []Field{boolField("unlocked", "Инженер доступен", selected["status"] == "unlocked")}, Disabled: !hasRow}, {ID: "engineer.reset", Title: "Вернуть статус из журнала", Path: "/api/engineering", Payload: M{"action": "engineer", "key": selected["key"], "unlocked": nil}, Disabled: !hasRow}, {ID: "engineer.guide", Title: "Открыть руководство", Kind: "url", Path: v.Str(selected["url"]), Disabled: v.Str(selected["url"]) == ""}, copySystem(selected["system"])}
		}
		if mode == "Обмен" {
			trader := v.Obj(e["trader"])
			return []Action{newAction("engineering.trade", "Рассчитать обмен", "/api/engineering", M{"action": "configure"}, selectField("tradeTargetKey", "Нужный материал", trader["targetKey"], choices(v.Rows(catalog["materials"]), "key", "name")), numberField("tradeTargetAmount", "Нужно получить, ед.", v.First(trader["targetAmount"], 1), 1, 100000), boolField("protectPlan", "Защитить материалы из плана улучшений", v.First(trader["protectPlan"], true)), boolField("showAllTrades", "Показывать варианты даже при нехватке запаса", trader["showAllTrades"]))}
		}
		fields := func(r M) []Field {
			out := []Field{selectField("moduleKey", "Модуль", r["moduleKey"], choices(v.Rows(catalog["modules"]), "key", "name")), selectField("blueprintKey", "Чертёж", r["blueprintKey"], choices(v.Rows(catalog["blueprints"]), "key", "name")), numberField("quantity", "Количество модулей", v.First(r["quantity"], 1), 1, 100), textField("label", "Ваша пометка", r["label"])}
			rolls := v.Arr(r["rolls"])
			for i := 1; i <= 5; i++ {
				value := any(i)
				if i <= len(rolls) {
					value = rolls[i-1]
				}
				out = append(out, Field{Key: fmt.Sprint("roll", i), Label: fmt.Sprintf("Попытки улучшения G%d", i), Kind: "integer", Default: value, Min: 0, Max: 100, Help: "Ноль пропускает уже выполненную ступень. Материалы считаются для указанного числа попыток."})
			}
			out = append(out, selectField("experimentalKey", "Экспериментальный эффект", r["experimentalKey"], append([]Choice{{"", "Без эффекта"}}, choices(v.Rows(catalog["experimentals"]), "key", "name")...)))
			return out
		}
		planAction := func(a Action) Action {
			a.Transform = func(body M) (M, error) {
				rolls := []any{}
				for i := 1; i <= 5; i++ {
					key := fmt.Sprint("roll", i)
					rolls = append(rolls, v.Int(body[key]))
					delete(body, key)
				}
				body["rolls"] = rolls
				return body, nil
			}
			return a
		}
		add := planAction(newAction("engineering.add", "Добавить улучшение", "/api/engineering", M{"action": "add"}, fields(M{})...))
		edit := planAction(newAction("engineering.edit", "Изменить улучшение", "/api/engineering", M{"action": "update", "id": selected["id"]}, fields(selected)...))
		edit.Disabled = selected["moduleKey"] == nil
		return []Action{add, edit, {ID: "engineering.remove", Title: "Удалить улучшение", Path: "/api/engineering", Payload: M{"action": "remove", "id": selected["id"]}, Disabled: selected["moduleKey"] == nil, Confirm: "Убрать улучшение из личного плана?"}, export("engineering.materials", "Список материалов CSV", "/api/engineering/materials.csv"), export("engineering.export", "Сохранить план JSON", "/api/engineering/plan.json"), {ID: "engineering.import", Title: "Загрузить план JSON", Kind: "import-plan", Path: "/api/engineering"}, {ID: "engineering.clear", Title: "Очистить план", Path: "/api/engineering", Payload: M{"action": "clear"}, Confirm: "Удалить все улучшения из личного плана?"}}
	case "mining":
		o := v.Obj(v.At(state, "mining", "options"))
		fields := []Field{selectField("resourceKey", "Ресурс", v.First(o["resourceKey"], "platinum"), resourceChoices(state, true)), selectField("method", "Способ добычи", v.First(o["method"], "laser"), ChoicePairs("laser", "Лазерная", "core", "Ядра", "subsurface", "Подповерхностная", "surface", "Поверхность астероида", "surface-planet", "Планетарная", "raw", "Инженерное сырьё")), selectField("mode", "Занятие", v.First(o["mode"], "mining"), ChoicePairs("mining", "Добыча", "trade", "Закупка и перевозка")), numberField("targetTons", "Целевой объём, т", o["targetTons"], 0, 25000), numberField("targetCredits", "Целевой доход, кр.", o["targetCredits"], 0, 1e15), numberField("sellPrice", "Ожидаемая продажа, кр./т", o["sellPrice"], 0, 1e9), numberField("purchasePrice", "Закупка, кр./т", o["purchasePrice"], 0, 1e9), numberField("tonsPerHour", "Ваш темп, т/ч (пусто — измеренный)", o["tonsPerHour"], 0, 100000), numberField("tripMinutes", "Дорога, мин", v.First(o["tripMinutes"], 20), 0, 10080), numberField("operatingCost", "Прочие расходы, кр.", v.First(o["operatingCost"], 0), 0, 1e12)}
		return []Action{newAction("mining.plan", "Параметры плана", "/api/mining/plan", nil, fields...), {ID: "mining.reset", Title: "Начать новый заход", Path: "/api/mining/reset", Confirm: "Завершить текущий период захода и начать новый?"}, {ID: "mining.targets", Title: "Найти место добычи", Kind: "navigate", Path: "targets", Payload: M{"mode": "Места добычи"}}, export("mining.export", "Экспорт статистики CSV", "/api/export.csv")}
	case "statistics":
		return []Action{export("stats.export", "Экспорт статистики CSV", "/api/export.csv"), export("stats.trade.export", "Экспорт сделок CSV", "/api/trading/export.csv"), {ID: "stats.reset", Title: "Новый период учёта", Path: "/api/session", Payload: M{"action": "reset"}, Confirm: "Начать новый период статистики? История игровых журналов останется на месте."}}
	case "settings":
		appearance := v.Obj(v.At(state, "theme", "settings"))
		colors := v.Obj(appearance["colors"])
		colorFields := []Field{selectField("mode", "Способ оформления", v.First(appearance["mode"], "preset"), ChoicePairs("preset", "Готовая палитра", "custom", "Свои цвета", "matrix", "Матрица HUD", "game", "Цвета из игры")), selectField("preset", "Готовая палитра", v.First(appearance["preset"], "blue"), ChoicePairs("blue", "Синий космос", "orange", "Оранжевый HUD", "green", "Изумруд", "purple", "Фиолетовый")), numberField("overlayOpacity", "Плотность оверлея (0,4–1)", v.First(appearance["overlayOpacity"], 0.82), 0.4, 1)}
		for _, r := range [][2]string{{"accent", "Акцент"}, {"text", "Текст"}, {"muted", "Вторичный текст"}, {"bg", "Фон"}, {"panel", "Панели"}, {"good", "Подтверждение"}, {"warn", "Предупреждение"}, {"danger", "Ошибка"}} {
			colorFields = append(colorFields, Field{Key: r[0], Label: r[1], Kind: "color", Default: colors[r[0]], Help: "Цвет в формате #RRGGBB"})
		}
		matrix := v.Arr(appearance["matrix"])
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				var val any = 0
				if i == j {
					val = 1
				}
				if i < len(matrix) && j < len(v.Arr(matrix[i])) {
					val = v.Arr(matrix[i])[j]
				}
				colorFields = append(colorFields, numberField(fmt.Sprintf("matrix%d%d", i, j), fmt.Sprintf("Матрица HUD: строка %d, столбец %d", i+1, j+1), val, -2, 2))
			}
		}
		colorAction := newAction("settings.palette", "Оформление и цвета", "/api/theme", nil, colorFields...)
		colorAction.Transform = func(body M) (M, error) {
			col := M{}
			for _, k := range []string{"accent", "text", "muted", "bg", "panel", "good", "warn", "danger"} {
				col[k] = body[k]
			}
			matrix := [][]float64{}
			for i := 0; i < 3; i++ {
				row := []float64{}
				for j := 0; j < 3; j++ {
					row = append(row, v.Num(body[fmt.Sprintf("matrix%d%d", i, j)]))
				}
				matrix = append(matrix, row)
			}
			settings := M{"mode": body["mode"], "preset": body["preset"], "matrix": matrix, "overlayOpacity": body["overlayOpacity"], "source": appearance["source"]}
			if body["mode"] == "custom" || body["mode"] == "game" {
				settings["colors"] = col
			}
			return M{"settings": settings}, nil
		}
		return []Action{newAction("settings.filters", "Настройки добычи и оверлея", "/api/settings", nil, selectField("minTier", "Минимальная ценность", settings["minTier"], ChoicePairs("low", "Любая", "medium", "Средняя и выше", "high", "Высокая")), selectField("method", "Способ добычи", settings["method"], ChoicePairs("all", "Любой", "laser", "Лазер", "core", "Ядра", "subsurface", "Подповерхностная", "surface", "Поверхность астероида", "surface-planet", "Планетарная", "raw", "Сырьё")), numberField("radiusLy", "Радиус поиска, св. л.", settings["radiusLy"], 5, 100), numberField("maxArrivalLs", "Максимум от звезды, св. с.", settings["maxArrivalLs"], 100, 1e6), numberField("minLaserPercent", "Минимальное содержание для лазера, %", settings["minLaserPercent"], 1, 100), boolField("hideDepleted", "Скрывать истощённые кольца для лазерной добычи", settings["hideDepleted"]), boolField("overlayEnabled", "Включить оверлей (виден только при активной игре)", settings["overlayEnabled"])), colorAction, newAction("settings.window", "Режим окна", "/api/desktop", nil, selectField("displayMode", "Отображение", v.First(v.At(state, "desktop", "displayMode"), "windowed"), ChoicePairs("windowed", "В окне", "fullscreen", "Полноэкранный", "borderless", "Без рамки"))), {ID: "settings.import.palette", Title: "Скопировать палитру из игры", Path: "/api/theme/import", Kind: "theme-import", Fields: []Field{{Key: "filePath", Label: "Файл палитры, необязательно", Kind: "file", Help: "Пусто — найти применённую тему EDHM или GraphicsConfigurationOverride.xml автоматически. Файлы игры не меняются."}}}, {ID: "settings.journals", Title: "Папка журналов", Kind: "folder", Path: "/api/journal-dir"}, {ID: "settings.prices", Title: "Справочная цена товара", Path: "/api/market", Fields: []Field{selectField("key", "Товар", "", goodsChoices(state)), {Key: "price", Label: "Ваш ориентир, кр./т", Kind: "number", Min: 0, Max: 1e9, Help: "Необязательно: используется для оценки груза. Пусто возвращает последнюю известную цену игры; котировки рынков и история сделок считаются отдельно."}}, Transform: func(body M) (M, error) { return M{"prices": []M{{"key": body["key"], "price": body["price"]}}}, nil }}, newAction("settings.rescan", "Перечитать журналы", "/api/rescan", nil)}
	case "account":
		return []Action{newAction("frontier.login", "Войти через Frontier", "/api/frontier/login", nil), newAction("frontier.sync", "Обновить данные", "/api/frontier/sync", M{"carrier": true}), {ID: "frontier.logout", Title: "Выйти из Frontier", Path: "/api/frontier/logout", Confirm: "Выйти из Frontier в ED Copilot?"}}
	case "integrations":
		return []Action{newAction("inara.settings", "Ключ и отправка в Inara", "/api/integrations/configure", nil, Field{Key: "apiKey", Label: "Личный API-ключ Inara", Kind: "password", Help: "Пустое поле сохраняет прежний ключ. Ключ хранится зашифрованным в вашем профиле Windows."}, boolField("enabled", "Отправлять события в Inara", v.At(state, "integrations", "enabled"))), {ID: "inara.site", Title: "Настройки API на Inara", Kind: "url", Path: "https://inara.cz/elite/cmdr-settings-api/"}}
	case "encyclopedia", "help":
		actions := []Action{{ID: "details.open", Title: "Открыть полностью", Kind: "details", Disabled: !hasRow}}
		if url := v.Str(selected["url"]); url != "" {
			actions = append(actions, Action{ID: "reference.open", Title: "Открыть источник", Kind: "url", Path: url})
		}
		for _, id := range v.Arr(selected["sourceIds"]) {
			s := v.Obj(v.Obj(data.Object("encyclopedia.json")["sources"])[v.Str(id)])
			if url := v.Str(s["url"]); url != "" {
				actions = append(actions, Action{ID: "source." + v.Str(id), Title: v.Text(s["title"], "Источник"), Kind: "url", Path: url})
			}
		}
		return actions
	}
	return []Action{{ID: "help", Title: "Как пользоваться", Kind: "help"}}
}
func toStrings(raw any) []string {
	r := []string{}
	for _, x := range v.Arr(raw) {
		r = append(r, v.Str(x))
	}
	return r
}

// PurchasePrefill applies the same form/transform contract as ordinary actions
// before opening the market search, retaining editable quantities and origins.
func PurchasePrefill(action Action, values M) (M, error) {
	body, err := action.Body(values)
	if err != nil {
		return nil, err
	}
	return v.Merge(v.Copy(body), M{"useCargo": false, "useCarrierCargo": false}), nil
}
