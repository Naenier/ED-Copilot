package presentation

import (
	"fmt"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func DealsPage(mode, query string, state M) Page {
	p := Page{Title: "Рейсы", Modes: []string{"Поиск", "Сохранённые"}, Columns: []string{"Система закупки → продажи", "Станции", "Товары / тоннаж", "Прибыль, кр.", "Длина маршрута, св. л.", "За час, кр."}}
	routes := v.Obj(state["tradeRoutes"])
	p.Summary = v.Str(routes["status"])
	run := v.Obj(state["tradeRun"])
	rows := v.Rows(routes["results"])
	if mode == "Сохранённые" {
		rows = v.Rows(state["savedTradeRoutes"])
		p.Summary = ""
	}
	for _, route := range rows {
		if count := len(v.Rows(route["legs"])); count < 1 || count > 2 {
			continue
		}
		systems, stations, goods, detail := []string{}, []string{}, []string{}, []string{}
		if mode == "Сохранённые" {
			label := "Общий рейс"
			if route["scope"] == "personal" {
				label = "Личный рейс"
			}
			detail = append(detail, label)
		}
		for i, leg := range v.Rows(route["legs"]) {
			buy, sell := v.Obj(leg["buy"]), v.Obj(leg["sell"])
			systems = append(systems, v.Str(buy["system"])+" → "+v.Str(sell["system"]))
			stations = append(stations, v.Str(buy["station"])+" → "+v.Str(sell["station"]))
			goods = append(goods, v.Text(leg["name"], leg["key"])+" · "+Number(leg["quantity"])+" т")
			detail = append(detail, fmt.Sprintf("Плечо %d: %s → %s", i+1, systems[len(systems)-1], stations[len(stations)-1]), fields(leg, "name", "Товар", "quantity", "Груз, т", "buyPrice", "Закупка, кр./т", "sellPrice", "Продажа, кр./т", "supply", "Запас, т", "demand", "Спрос, т", "grossMargin", "Маржа, кр."))
			for _, station := range []M{buy, sell} {
				detail = append(detail, fields(station, "station", "Станция", "system", "Система", "type", "Тип", "pad", "Площадка", "distanceLs", "От звезды, св. с.", "observedAt", "Наблюдение", "ageHours", "Возраст, ч"))
			}
		}
		if !contains(query, strings.Join(systems, " "), strings.Join(stations, " "), strings.Join(goods, " ")) {
			continue
		}
		var hourly any
		if v.Str(run["routeId"]) == v.Str(route["id"]) && v.Int(run["completedRounds"]) > 0 {
			hourly = v.First(run["actualProfitPerHour"], run["profitPerHour"])
		}
		p.Rows = append(p.Rows, row(route, []string{strings.Join(systems, " / "), strings.Join(stations, " / "), strings.Join(goods, " / "), Number(v.First(route["grossMargin"], route["netEstimate"])), Number(v.First(route["legDistanceLy"], route["distanceLy"])), Number(hourly)}, strings.Join(detail, "\r\n\r\n")))
	}
	return p
}

func DealsActions(state, selected M) []Action {
	copySystem := func(text any) Action { return Action{Kind: "copy", Path: v.Str(text), Disabled: v.Str(text) == ""} }
	cmd := v.Obj(state["commander"])
	o := v.Obj(v.At(state, "tradeRoutes", "options"))
	value := func(key string, fallback any) any { return v.First(o[key], fallback) }
	powers := []Choice{{"", "Любая"}}
	for _, power := range []string{"Aisling Duval", "Archon Delaine", "Arissa Lavigny-Duval", "Denton Patreus", "Edmund Mahon", "Felicia Winters", "Jerome Archer", "Li Yong-Rui", "Nakato Kaine", "Pranav Antal", "Yuri Grom", "Zemina Torval"} {
		powers = append(powers, Choice{power, power})
	}
	states := ChoicePairs("", "Любое", "Unoccupied", "Незанятая", "Exploited", "Эксплуатируемая", "Fortified", "Укреплённая", "Stronghold", "Оплот", "Contested", "Оспариваемая")
	fields := []Field{
		textField("system", "Ближайшая система", value("system", cmd["system"])),
		numberField("cargoCapacity", "Вместимость трюма, т", value("cargoCapacity", cmd["cargoCapacity"]), 1, 25000),
		numberField("maxDistance", "Макс. дальность маршрута, св. л.", value("maxDistance", 20), 1, 5000),
		numberField("maxAgeHours", "Макс. возраст цен, ч", value("maxAgeHours", 24), 1, 720),
		selectField("minPad", "Мин. посадочная площадка", value("minPad", v.Text(cmd["shipPad"], "L")), ChoicePairs("L", "Большая", "M", "Средняя", "S", "Малая")),
		numberField("maxArrivalLs", "Макс. расстояние до станции, св. с.", value("maxArrivalLs", 1000), 0, 1e7),
		boolField("allowSurface", "Использовать наземные станции", value("allowSurface", false)),
		numberField("minSupply", "Минимальный запас, т", value("minSupply", 10000), 0, 1e9),
		numberField("minDemand", "Минимальный спрос, т", value("minDemand", 10000), 0, 1e9),
		boolField("roundTrip", "Перевозка туда и обратно", value("roundTrip", true)),
		selectField("sourcePower", "Держава в системе отправления", value("sourcePower", ""), powers),
		selectField("sourcePowerState", "Состояние Powerplay в системе отправления", value("sourcePowerState", ""), states),
		selectField("targetPower", "Держава в системе назначения", value("targetPower", ""), powers),
		selectField("targetPowerState", "Состояние Powerplay в системе назначения", value("targetPowerState", ""), states),
		selectField("orderBy", "Упорядочить по", value("orderBy", "profit"), ChoicePairs("profit", "Прибыль за рейс", "profit-per-ton", "Прибыль за тонну", "margin", "Рентабельность", "distance", "Длина маршрута")),
	}
	fields[0].Required, fields[1].Required = true, true
	search := Action{ID: "deals.search", Title: "Найти выгодные рейсы", Path: "/api/trading/routes", Fields: fields}
	actions := []Action{search, {ID: "deals.stop", Title: "Остановить поиск", Path: "/api/trading/cancel"}}
	owner := v.Str(v.At(state, "planWorkspace", "key"))
	runID, routeID := v.Str(v.At(state, "tradeRun", "routeId")), v.Str(selected["id"])
	if owner != "" && routeID != "" {
		action, title := "start", "Выбрать маршрут"
		if runID == routeID && v.At(state, "tradeRun", "phase") != "completed" {
			action, title = "stop", "Завершить отслеживание"
		}
		actions = append(actions, Action{ID: "deals.track", Title: title, Path: "/api/trading/active-route", Payload: M{"action": action, "routeId": routeID, "expectedPlanWorkspace": owner}})
	}
	if len(v.Rows(selected["legs"])) > 0 && !v.Bool(selected["quoteSnapshot"]) {
		actions = append(actions, Action{ID: "deals.save-route", Title: "Сохранить весь рейс", Path: "/api/trading/saved-routes", Payload: M{"action": "save-route", "route": selected, "expectedPlanWorkspace": owner}})
	}
	if v.Bool(selected["quoteSnapshot"]) {
		actions = append(actions, Action{ID: "deals.delete-route", Title: "Удалить сохранённый рейс", Path: "/api/trading/saved-routes", Payload: M{"action": "delete-route", "id": selected["id"], "expectedPlanWorkspace": owner}, Confirm: "Удалить рейс из сохранённых?"})
		if owner != "" {
			scope, title := "personal", "Сделать личным"
			if selected["scope"] == "personal" {
				scope, title = "shared", "Сделать общим"
			}
			actions = append(actions, Action{ID: "deals.scope", Title: title, Path: "/api/trading/saved-routes", Payload: M{"action": "set-scope", "id": selected["id"], "scope": scope, "expectedPlanWorkspace": owner}})
		}
	}
	for i, leg := range v.Rows(selected["legs"]) {
		buy, sell := v.Obj(leg["buy"]), v.Obj(leg["sell"])
		suffix := fmt.Sprint(i)
		title := fmt.Sprintf("Плечо %d: ", i+1)
		a, b := copySystem(buy["system"]), copySystem(sell["system"])
		a.ID, a.Title = "deals.copy-buy."+suffix, title+"копировать систему закупки"
		b.ID, b.Title = "deals.copy-sell."+suffix, title+"копировать систему продажи"
		actions = append(actions, a, b, Action{ID: "deals.copy-station-buy." + suffix, Title: title + "копировать станцию закупки", Kind: "copy", Path: v.Str(buy["station"])}, Action{ID: "deals.copy-station-sell." + suffix, Title: title + "копировать станцию продажи", Kind: "copy", Path: v.Str(sell["station"])})
	}
	return actions
}
