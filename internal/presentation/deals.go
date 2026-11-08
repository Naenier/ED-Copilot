package presentation

import (
	"fmt"
	"math"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func DealsPage(mode, query string, state M) Page {
	p := Page{Title: "Рейсы", Description: "Системы, станции и экономика каждого плеча торгового маршрута.", Modes: []string{"Поиск", "Сохранённые"}, Columns: []string{"Система закупки → продажи", "Станции", "Товары / тоннаж", "Прибыль, кр.", "Капитал, кр.", "За час, кр."}}
	routes := v.Obj(state["tradeRoutes"])
	p.Summary = v.Text(routes["status"], "Укажите исходную систему, свободный трюм, бюджет и товары для сравнения публичных котировок.")
	rows := v.Rows(routes["results"])
	if mode == "Сохранённые" {
		rows = v.Rows(state["savedTradeRoutes"])
	}
	for _, route := range rows {
		if count := len(v.Rows(route["legs"])); count < 1 || count > 2 {
			continue
		}
		systems, stations, goods, detail := []string{}, []string{}, []string{}, []string{}
		for i, leg := range v.Rows(route["legs"]) {
			buy, sell := v.Obj(leg["buy"]), v.Obj(leg["sell"])
			systems = append(systems, v.Str(buy["system"])+" → "+v.Str(sell["system"]))
			stations = append(stations, v.Str(buy["station"])+" → "+v.Str(sell["station"]))
			goods = append(goods, v.Text(leg["name"], leg["key"])+" · "+Number(leg["quantity"])+" т")
			detail = append(detail, fmt.Sprintf("Плечо %d: %s → %s", i+1, systems[len(systems)-1], stations[len(stations)-1]), fields(leg, "name", "Товар", "quantity", "Груз, т", "buyPrice", "Закупка, кр./т", "sellPrice", "Продажа, кр./т", "supply", "Запас, т", "demand", "Спрос, т", "safeDemand", "Спрос с запасом, т", "purchaseCost", "Закупка всего, кр.", "grossMargin", "Маржа, кр.", "operatingCost", "Расходы плеча, кр.", "netEstimate", "Результат, кр.", "distanceLy", "Расстояние, св. л."))
			for _, station := range []M{buy, sell} {
				detail = append(detail, fields(station, "station", "Станция", "system", "Система", "type", "Тип", "pad", "Площадка", "distanceLs", "От звезды, св. с.", "observedAt", "Наблюдение", "ageHours", "Возраст, ч"))
			}
		}
		if !contains(query, strings.Join(systems, " "), strings.Join(stations, " "), strings.Join(goods, " ")) {
			continue
		}
		detail = append(detail, "Доход в час использует введённое время плеча. Подлёт к первой станции не включён. Котировки не гарантируют цену при прибытии.")
		p.Rows = append(p.Rows, row(route, []string{strings.Join(systems, " / "), strings.Join(stations, " / "), strings.Join(goods, " / "), Number(route["netEstimate"]), Number(route["minimumCapital"]), Number(route["profitPerHour"])}, strings.Join(detail, "\r\n\r\n")))
	}
	return p
}

func DealsActions(state, selected M) []Action {
	copySystem := func(text any) Action { return Action{Kind: "copy", Path: v.Str(text), Disabled: v.Str(text) == ""} }
	cmd := v.Obj(state["commander"])
	o := v.Obj(v.At(state, "tradeRoutes", "options"))
	value := func(key string, fallback any) any { return v.First(o[key], fallback) }
	var capacity any
	if n, known := v.Number(cmd["cargoCapacity"]); known {
		capacity = math.Max(0, n-v.Num(cmd["cargoTons"]))
	}
	fields := []Field{textField("system", "Исходная система", value("system", cmd["system"])), numberField("cargoCapacity", "Свободный трюм, т", value("cargoCapacity", capacity), 1, 25000), numberField("credits", "Средства на закупку, кр.", value("credits", v.At(state, "pilot", "credits")), 0, 1e15), numberField("radiusLy", "Область поиска, св. л.", value("radiusLy", 100), 5, 5000), numberField("maxLegLy", "Максимальная длина плеча, св. л.", value("maxLegLy", 100), 1, 5000), numberField("minutesPerLeg", "Полное время одного плеча, мин", value("minutesPerLeg", 15), 1, 1440), numberField("operatingCostPerLeg", "Расходы на плечо, кр.", value("operatingCostPerLeg", 0), 0, 1e12), numberField("maxAgeHours", "Возраст цен станций, ч", value("maxAgeHours", 24), 1, 720), numberField("carrierMaxAgeHours", "Возраст цен флотоносцев, ч", value("carrierMaxAgeHours", 6), .25, 168), numberField("maxArrivalLs", "Максимум от звезды, св. с.", value("maxArrivalLs", 10000), 0, 1e7), numberField("minSupply", "Минимальный запас, т", value("minSupply", 1), 1, 1e9), numberField("minDemand", "Минимальный спрос, т", value("minDemand", 1), 1, 1e9), numberField("demandRatio", "Запас спроса к грузу", value("demandRatio", 4), 1, 20), selectField("minPad", "Площадка вашего корабля", value("minPad", "L"), ChoicePairs("L", "L · большая", "M", "M · средняя", "S", "S · малая")), selectField("marketType", "Тип рынков", value("marketType", "stations"), ChoicePairs("stations", "Станции", "all", "Станции и флотоносцы", "carriers", "Флотоносцы")), boolField("includePlanetary", "Включать планетарные порты", value("includePlanetary", true)), boolField("roundTrip", "Обратный груз A → B → A", value("roundTrip", false)), selectField("orderBy", "Сортировка", value("orderBy", "profitPerHour"), ChoicePairs("profitPerHour", "Прибыль за час", "profit", "Прибыль за рейс", "distance", "Расстояние", "purchaseCost", "Начальный капитал"))}
	for i := 0; i < 3; i++ {
		fields[i].Required = true
	}
	keys := v.Arr(o["resourceKeys"])
	if len(keys) == 0 {
		keys = []any{"gold", "silver", "agronomictreatment", "bertrandite", "indite", "bauxite"}
	}
	for i := 0; i < 6; i++ {
		var key any = ""
		if i < len(keys) {
			key = keys[i]
		}
		fields = append(fields, selectField(fmt.Sprint("resource", i+1), fmt.Sprint("Товар ", i+1), key, append([]Choice{{"", "Не искать"}}, goodsChoices(state)...)))
	}
	search := Action{ID: "deals.search", Title: "Найти выгодные рейсы", Path: "/api/trading/routes", Fields: fields}
	search.Transform = func(body M) (M, error) {
		keys := []string{}
		for i := 1; i <= 6; i++ {
			key := fmt.Sprint("resource", i)
			if text := v.Str(body[key]); text != "" {
				keys = append(keys, text)
			}
			delete(body, key)
		}
		body["resourceKeys"] = keys
		return body, nil
	}
	actions := []Action{search, {ID: "deals.stop", Title: "Остановить поиск", Path: "/api/trading/cancel"}}
	if len(v.Obj(state["tradePlan"])) > 0 {
		actions = append(actions, Action{ID: "deals.clear-plan", Title: "Убрать план плеча", Path: "/api/trading/plan/clear", Confirm: "Убрать текущий план плеча? Сохранённые рейсы останутся."})
	}
	if len(v.Rows(selected["legs"])) > 0 {
		actions = append(actions, Action{ID: "deals.save-route", Title: "Сохранить весь рейс", Path: "/api/trading/saved-routes", Payload: M{"action": "save-route", "route": selected}})
	}
	if v.Bool(selected["quoteSnapshot"]) {
		actions = append(actions, Action{ID: "deals.delete-route", Title: "Удалить сохранённый рейс", Path: "/api/trading/saved-routes", Payload: M{"action": "delete-route", "id": selected["id"]}, Confirm: "Удалить рейс из сохранённых?"})
	}
	for i, leg := range v.Rows(selected["legs"]) {
		buy, sell := v.Obj(leg["buy"]), v.Obj(leg["sell"])
		sell = v.Merge(v.Copy(sell), M{"selectionMode": "sell", "quoteOptions": v.First(selected["options"], o)})
		suffix := fmt.Sprint(i)
		title := fmt.Sprintf("Плечо %d: ", i+1)
		a, b := copySystem(buy["system"]), copySystem(sell["system"])
		a.ID, a.Title = "deals.copy-buy."+suffix, title+"копировать систему закупки"
		b.ID, b.Title = "deals.copy-sell."+suffix, title+"копировать систему продажи"
		actions = append(actions, a, b, Action{ID: "deals.copy-station-buy." + suffix, Title: title + "копировать станцию закупки", Kind: "copy", Path: v.Str(buy["station"])}, Action{ID: "deals.copy-station-sell." + suffix, Title: title + "копировать станцию продажи", Kind: "copy", Path: v.Str(sell["station"])}, Action{ID: "deals.destination." + suffix, Title: title + "назначить покупателя", Path: "/api/trading/destination", Payload: M{"destination": sell}, Disabled: len(v.Rows(sell["quotes"])) == 0}, Action{ID: "deals.plan." + suffix, Title: title + "сохранить план", Path: "/api/trading/plan", Payload: M{"key": leg["key"], "quantity": leg["quantity"], "buyPrice": leg["buyPrice"], "sellPrice": leg["sellPrice"], "source": buy["system"], "destination": sell["system"], "buy": buy, "sell": sell}})
		actions = append(actions, Action{ID: "deals.purchase." + suffix, Title: title + "проверить закупку", Kind: "purchase", Path: "sale", Payload: M{"resourceKey": leg["key"], "quantity": leg["quantity"], "system": buy["system"]}, Fields: []Field{numberField("quantity", "Закупить, т", leg["quantity"], 1, 25000), textField("system", "Искать рядом с системой", buy["system"])}})
	}
	return actions
}
