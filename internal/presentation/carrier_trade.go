package presentation

import (
	"fmt"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func CarrierTradePage(mode, query string, state M) Page {
	c := v.Obj(state["carrierTrade"])
	s := v.Obj(c["summary"])
	page := Page{Title: "Торговля флотоносцем", Description: "Посчитайте весь рейс владельца: рынки, тритий, прыжки, содержание и работу перевозчиков.", Modes: []string{"Рейс", "Расходы", "Проверки"}, Summary: "Плановая прибыль: " + Number(s["profit"]) + " кр. · Неизвестные цены и расходы не считаются нулевыми."}
	switch mode {
	case "Проверки":
		page.Columns = []string{"Проверка", "Результат", "Пояснение"}
		for _, r := range v.Rows(c["issues"]) {
			if contains(query, r["title"], r["detail"]) {
				r = v.Copy(r)
				r["id"] = fmt.Sprint(r["code"], "-", len(page.Rows))
				page.Rows = append(page.Rows, row(r, []string{v.Str(r["title"]), map[string]string{"error": "Нужно исправить", "warn": "Нужно проверить", "info": "Информация"}[v.Str(r["level"])], v.Str(r["detail"])}, v.Str(r["detail"])))
			}
		}
	case "Расходы":
		page.Columns = []string{"Показатель", "Значение"}
		for _, pair := range [][2]string{{"grossMargin", "Маржа товаров до расходов"}, {"haulingFees", "Работа других пилотов"}, {"fuelCost", "Израсходованный тритий"}, {"jumpCost", "Обслуживание прыжков"}, {"allocatedUpkeep", "Содержание за время рейса"}, {"extraCosts", "Прочие расходы"}, {"profit", "Результат рейса"}, {"workingCapital", "Капитал в рейсе"}, {"breakEvenSellPrice", "Безубыточная цена продажи туда, кр./т"}, {"paybackTrips", "Рейсов до окупаемости"}} {
			if contains(query, pair[1]) {
				page.Rows = append(page.Rows, row(M{"id": pair[0]}, []string{pair[1], Number(s[pair[0]])}, strings.Join(func() []string {
					out := []string{}
					for _, note := range v.Arr(c["notes"]) {
						out = append(out, v.Str(note))
					}
					return out
				}(), "\r\n")))
			}
		}
	default:
		page.Columns = []string{"Плечо", "Товар", "Объём, т", "Маржа после перевозчиков, кр.", "Погрузка / продажа"}
		for _, leg := range v.Rows(c["legs"]) {
			if contains(query, leg["name"], leg["commodityName"], leg["sourceSystem"], leg["destinationSystem"]) {
				page.Rows = append(page.Rows, row(leg, []string{v.Str(leg["name"]), v.Str(leg["commodityName"]), Number(leg["effectiveQuantity"]), Number(leg["contribution"]), v.Text(leg["sourceSystem"], "Не выбрано") + " → " + v.Text(leg["destinationSystem"], "Не выбрано")}, "Тритий: "+Number(leg["tritiumTons"])+" т\r\nПрыжков: "+Number(leg["jumps"])+"\r\nПерелёт: "+Number(leg["travelHours"])+" ч"))
			}
		}
	}
	return page
}

func CarrierTradeActions(mode string, state, selected M) []Action {
	c := v.Obj(state["carrierTrade"])
	o := v.Obj(c["options"])
	post := func(id, title string, body M, fields ...Field) Action {
		return Action{ID: id, Title: title, Path: "/api/carrier/trade", Payload: body, Fields: fields}
	}
	options := post("carrier-trade.options", "Параметры флотоносца и рейса", M{"action": "configure"}, selectField("carrierType", "Тип флотоносца", o["carrierType"], ChoicePairs("fleet", "Личный флотоносец", "squadron", "Флотоносец эскадрильи")), boolField("returnTrip", "Включить обратный перелёт", o["returnTrip"]), numberField("servicesTons", "Службы и комплекты, т", o["servicesTons"], 0, 100000), numberField("otherCargoTons", "Другие грузы, т", o["otherCargoTons"], 0, 1000000), numberField("tankTons", "Тритий в баке перед вылетом, т", o["tankTons"], 0, 1000), numberField("cargoTritiumTons", "Тритий в трюме (пусто — рассчитать), т", o["cargoTritiumTons"], 0, 100000), numberField("arrivalReserveTons", "Резерв трития после рейса, т", o["arrivalReserveTons"], 0, 100000), numberField("tritiumPrice", "Оценка трития, кр./т", o["tritiumPrice"], 0, 1e9), numberField("weeklyUpkeep", "Содержание без прыжков, кр./нед.", o["weeklyUpkeep"], 0, 1e12), numberField("durationHours", "Полное время рейса (пусто — сумма этапов), ч", o["durationHours"], 0, 100000), numberField("extraCosts", "Прочие расходы, кр.", o["extraCosts"], 0, 1e15), numberField("carrierInvestment", "Сумма для окупаемости, кр.", o["carrierInvestment"], 0, 1e15), selectField("haulerPad", "Площадка корабля-перевозчика", o["haulerPad"], ChoicePairs("", "Неизвестно", "S", "S", "M", "M", "L", "L")), numberField("haulerCapacity", "Трюм корабля-перевозчика, т", o["haulerCapacity"], 1, 100000), boolField("holidayConfirmed", "Public Holiday подтверждён в игре", o["holidayConfirmed"]))
	options.Transform = func(body M) (M, error) { delete(body, "action"); return M{"action": "configure", "options": body}, nil }
	actions := []Action{options}
	for _, leg := range v.Rows(c["legs"]) {
		id := v.Str(leg["id"])
		fields := []Field{{Key: "commodityKey", Label: "Товар", Kind: "combo", Default: leg["commodityKey"], Choices: goodsChoices(state), Required: true}, numberField("quantity", "Объём, т", leg["quantity"], 0, 1000000), textField("sourceSystem", "Исходная система", leg["sourceSystem"]), textField("destinationSystem", "Система продажи", leg["destinationSystem"]), numberField("buyPrice", "Цена закупки, кр./т", leg["buyPrice"], 0, 1e9), numberField("sellPrice", "Цена продажи, кр./т", leg["sellPrice"], 0, 1e9), selectField("loadingMode", "Погрузка", leg["loadingMode"], ChoicePairs("self", "Своими силами", "haulers", "Другими пилотами")), selectField("unloadingMode", "Разгрузка", leg["unloadingMode"], ChoicePairs("self", "Своими силами", "haulers", "Другими пилотами")), numberField("loadRewardPerTon", "Оплата погрузки, кр./т", leg["loadRewardPerTon"], 0, 1e9), numberField("unloadRewardPerTon", "Оплата разгрузки, кр./т", leg["unloadRewardPerTon"], 0, 1e9), numberField("loadingHours", "Погрузка, ч", leg["loadingHours"], 0, 100000), numberField("unloadingHours", "Разгрузка, ч", leg["unloadingHours"], 0, 100000), numberField("jumps", "Прыжков", leg["jumps"], 0, 50000), numberField("tritiumTons", "Расход трития, т", leg["tritiumTons"], 0, 1000000), numberField("travelHours", "Перелёт и дозаправки, ч", leg["travelHours"], 0, 100000)}
		action := post("carrier-trade."+id, "Плечо «"+v.Str(leg["name"])+"»", M{"action": "configure"}, fields...)
		action.Transform = func(body M) (M, error) {
			delete(body, "action")
			return M{"action": "configure", "legs": M{id: body}}, nil
		}
		actions = append(actions, action)
	}
	legDefault := "outbound"
	if selected["id"] == "return" {
		legDefault = "return"
	}
	actions = append(actions, post("carrier-trade.import-carrier", "Взять конфигурацию флотоносца", M{"action": "import-carrier"}), post("carrier-trade.import-route", "Взять рассчитанный маршрут", M{"action": "import-route"}, selectField("leg", "Плечо", legDefault, ChoicePairs("outbound", "Туда", "return", "Обратно"))), post("carrier-trade.booze", "Шаблон Booze Cruise", M{"action": "preset", "preset": "booze-cruise"}, textField("sourceSystem", "Исходная система в Баббле", v.At(state, "commander", "system"))), Action{ID: "carrier-trade.source", Title: "Руководство PTN", Kind: "url", Path: "https://pilotstradenetwork.com/guides/fleet-carrier-trading/"})
	for index := range actions {
		if actions[index].ID == "carrier-trade.import-route" && o["carrierType"] == "squadron" {
			actions[index].Disabled = true
			actions[index].Title = "Импорт Spansh доступен для личного флотоносца"
		}
	}
	reset := post("carrier-trade.reset", "Сбросить план рейса", M{"action": "reset"})
	reset.Confirm = "Сбросить параметры торгового рейса? Другие планы приложения останутся."
	return append(actions, reset)
}
