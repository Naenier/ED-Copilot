package presentation

import (
	"fmt"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func calculatorStateName(state string, mandatory bool) string {
	if mandatory {
		return "Встроена · работает"
	}
	switch state {
	case "active":
		return "Работает"
	case "suspended":
		return "Приостановлена"
	default:
		return "Не установлена"
	}
}

// CarrierCalculatorPage is a view contract shared by the native presentation
// and the redesigned shell. The calculator itself remains independent of UI.
func CarrierCalculatorPage(mode, query string, state M) Page {
	c := v.Obj(state["carrierCalculator"])
	s := v.Obj(c["summary"])
	p := Page{Title: "Калькулятор флотоносца", Description: "Сравните покупку, службы, вместимость и долгосрочный бюджет личного флотоносца и флотоносца эскадрильи.", Modes: []string{"Сводка", "Службы", "Сравнение"}, Summary: v.Text(c["name"], "Личный флотоносец") + " · Планирование расходов, без изменений в игре"}
	if len(c) == 0 {
		p.Columns = []string{"Показатель", "Значение"}
		return p
	}
	credit := func(value any) string {
		if value == nil {
			return "—"
		}
		return Number(value) + " кр."
	}
	switch mode {
	case "Службы":
		p.Columns = []string{"Служба", "Состояние", "Установка, кр.", "Содержание, кр./нед.", "Занято, т"}
		for _, r := range v.Rows(c["services"]) {
			description := "Установка: " + credit(r["installationPrice"]) + "\r\nРаботает: " + credit(r["activeWeeklyUpkeep"]) + " в неделю\r\nПриостановлена: " + credit(r["suspendedWeeklyUpkeep"]) + " в неделю\r\nУстановленный объём: " + Number(r["installedCapacityTons"]) + " т\r\nСнятие освобождает объём; приостановка его сохраняет."
			if v.Bool(r["mandatory"]) {
				description = "Эта верфь встроена в стоимость флотоносца эскадрильи. Всегда работает и занимает 3 000 т. Дополнительной платы за установку и содержание нет."
			}
			p.Rows = append(p.Rows, row(r, []string{v.Str(r["name"]), calculatorStateName(v.Str(r["state"]), v.Bool(r["mandatory"])), Number(r["buyPrice"]), Number(r["weeklyUpkeep"]), Number(r["capacityTons"])}, description))
		}
	case "Сравнение":
		p.Columns = []string{"Тип", "Покупка, кр.", "Содержание, кр./нед.", "Свободно, т", "С топливом, кр./нед."}
		for _, r := range v.Rows(c["comparison"]) {
			r = v.Copy(r)
			r["id"] = r["carrierType"]
			p.Rows = append(p.Rows, row(r, []string{v.Str(r["name"]), Number(r["totalBuyPrice"]), Number(r["weeklyUpkeep"]), Number(r["availableCargoTons"]), Number(r["weeklyCostWithFuel"])}, "Каждый тип хранит собственный набор служб, график прыжков и бюджет.\r\nПрыжки за неделю: "+Number(r["jumpsPerWeek"])+"\r\nСтоимость корпуса: "+credit(r["hullBuyPrice"])+"\r\nСлужбы занимают: "+Number(r["servicesCapacityTons"])+" т\r\nДругие грузы: "+Number(r["cargoTons"])+" т"))
		}
	default:
		p.Columns = []string{"Показатель", "Значение", "Пояснение"}
		add := func(id, label, value, note string) {
			p.Rows = append(p.Rows, row(M{"id": id, "kind": "calculator-summary"}, []string{label, value, note}, note))
		}
		add("buy", "Покупка корпуса и служб", credit(s["totalBuyPrice"]), "Разовый платёж. Приостановленные службы также установлены и оплачены.")
		add("space", "Свободная вместимость", Number(s["availableCargoTons"])+" т", "После установленных служб и введённого объёма других грузов.")
		if v.Num(s["overCapacityTons"]) > 0 {
			add("over-capacity", "Не помещается", Number(s["overCapacityTons"])+" т", "Уменьшите грузы или снимите дополнительные службы; приостановка не освобождает место.")
		}
		add("core-services", "Основные и дополнительные службы", credit(s["weeklyServicesAndCore"])+" / нед.", "Без обслуживания прыжков и покупки трития.")
		add("jumps", "Обслуживание прыжков", credit(s["weeklyJumpCost"])+" / нед.", Number(s["jumpsPerWeek"])+" прыжков × "+credit(s["jumpCostPerJump"])+"; топливо считается отдельно.")
		add("week", "Недельное содержание", credit(s["weeklyUpkeep"]), "Корпус, установленные службы и обслуживание запланированных прыжков. Без трития.")
		add("month", "Содержание за 30 дней", credit(s["monthlyUpkeep"]), "Точная пропорция 30/7 недель. Календарный месяц может содержать четыре или пять недельных списаний.")
		add("year", "Содержание за 365 дней", credit(s["yearlyUpkeep"]), "Точная пропорция 365/7 недель, без стоимости трития.")
		add("fuel-amount", "Тритий на неделю", Number(s["weeklyTritiumTons"])+" т", "Средний расход на прыжок вводится отдельно. Для точного маршрута используйте планировщик маршрута флотоносца.")
		add("fuel-cost", "Покупка топлива на неделю", credit(s["weeklyFuelCost"]), "Количество прыжков × средний расход × ваша цена трития. Прочерк означает, что расход или цена неизвестны.")
		add("week-fuel", "Недельные расходы с топливом", credit(s["weeklyCostWithFuel"]), "Содержание и покупка рассчитанного трития, без доходов от торговли и комиссий.")
		add("reserve", "Минимальный резерв банка", credit(s["minimumBankReserve"]), Number(s["reserveWeeks"])+" недель содержания без трития.")
		add("reserve-full", "Резерв банка с топливом", credit(s["recommendedBankReserve"]), "Полный резерв для выбранного срока, если расход и цена топлива известны.")
		add("required-minimum", "Покупка и минимальный резерв", credit(s["minimumRequiredBudget"]), "Нижняя оценка стартового бюджета: покупка и содержание на выбранный срок, без топлива.")
		add("required", "Покупка и полный резерв", credit(s["requiredBudget"]), "Покупка плюс резерв содержания и топлива. Не включает пакеты кораблей/модулей и товарные запасы.")
		add("bank", "Баланс банка", credit(s["bankBalance"]), "Сумма, уже отложенная на содержание; не прибавляется к вашему бюджету покупки второй раз.")
		add("bank-runway", "Банка хватит без топлива", Number(s["bankWholeWeeksWithoutFuel"])+" полных нед.", "При текущем наборе служб и графике прыжков, без новых доходов.")
		add("bank-runway-full", "Банка хватит с топливом", Number(s["bankWholeWeeksWithFuel"])+" полных нед.", "Если тритий оплачивается из того же планового бюджета.")
		add("gap", "Не хватает до полного бюджета", credit(s["budgetGap"]), "Ваш бюджет покупки сравнивается с покупкой и полным резервом. Прочерк — недостаточно исходных данных.")
		notes := []string{}
		for _, raw := range v.Arr(c["notes"]) {
			notes = append(notes, v.Str(raw))
		}
		add("assumptions", "Как считается", "Параметры и допущения", strings.Join(notes, "\r\n\r\n"))
	}
	filtered := []Row{}
	for _, r := range p.Rows {
		if contains(query, strings.Join(r.Cells, " "), r.Details) {
			filtered = append(filtered, r)
		}
	}
	p.Rows = filtered
	return p
}

func CarrierCalculatorActions(mode string, state, selected M) []Action {
	c := v.Obj(state["carrierCalculator"])
	options := v.Obj(c["options"])
	kind := v.Text(c["selectedType"], "fleet")
	post := func(id, title string, payload M, fields ...Field) Action {
		return Action{ID: id, Title: title, Path: "/api/carrier/calculator", Payload: v.Merge(M{"carrierType": kind}, payload), Fields: fields}
	}
	integer := func(key, label string, value any, max float64, required bool) Field {
		return Field{Key: key, Label: label, Kind: "integer", Default: value, Required: required, Min: 0, Max: max}
	}
	configure := post("carrier-calculator.configure", "Параметры расчёта", M{"action": "configure"},
		integer("jumpsPerWeek", "Планируемых прыжков за неделю", v.First(options["jumpsPerWeek"], 0), 10000, true),
		integer("reserveWeeks", "Резерв содержания, недель", v.First(options["reserveWeeks"], 12), 520, true),
		integer("bankBalance", "Баланс банка, кр. · пусто, если неизвестен", options["bankBalance"], 1e15, false),
		integer("purchaseBudget", "Ваш бюджет покупки и резерва, кр. · необязательно", options["purchaseBudget"], 1e15, false),
		numberField("cargoTons", "Другие грузы, тритий в трюме и пакеты, т", v.First(options["cargoTons"], 0), 0, 1000000),
		numberField("tritiumPerJump", "Средний расход трития на прыжок, т · необязательно", options["tritiumPerJump"], 0, 1000),
		integer("tritiumPrice", "Цена покупки трития, кр./т · необязательно", options["tritiumPrice"], 1e9, false),
	)
	configure.Fields[4].Required = true
	configure.Fields[5].Help = "Это ваша средняя оценка. Расход зависит от дальности и загрузки; точный перелёт рассчитывается в «Маршруте флотоносца»."
	configure.Fields[6].Help = "Обслуживание прыжков не включает топливо. Пустая цена не считается нулевой."
	types := []Choice{{"fleet", "Личный флотоносец"}, {"squadron", "Флотоносец эскадрильи"}}
	actions := []Action{configure, post("carrier-calculator.type", "Выбрать тип флотоносца", M{"action": "select"}, Field{Key: "carrierType", Label: "Тип флотоносца", Kind: "select", Default: kind, Required: true, Choices: types})}
	if mode == "Службы" {
		serviceKey := v.Str(selected["key"])
		choices := []Choice{}
		for _, service := range v.Rows(c["services"]) {
			if !v.Bool(service["mandatory"]) {
				choices = append(choices, Choice{v.Str(service["key"]), v.Str(service["name"])})
			}
		}
		service := post("carrier-calculator.service", "Изменить состояние службы", M{"action": "service"}, Field{Key: "serviceKey", Label: "Служба", Kind: "select", Choices: choices, Default: serviceKey, Required: true}, Field{Key: "state", Label: "Состояние", Kind: "select", Default: v.Text(selected["state"], "off"), Required: true, Choices: []Choice{{"off", "Не установлена"}, {"active", "Работает"}, {"suspended", "Приостановлена"}}})
		service.Disabled = v.Bool(selected["mandatory"])
		actions = append(actions, service)
	}
	allActive, suspended, none := M{}, M{}, M{}
	for _, service := range v.Rows(c["services"]) {
		if !v.Bool(service["mandatory"]) {
			key := v.Str(service["key"])
			allActive[key] = "active"
			if service["state"] != "off" {
				suspended[key] = "suspended"
			}
			none[key] = "off"
		}
	}
	actions = append(actions, post("carrier-calculator.activate-all", "Установить все доступные службы", M{"action": "configure", "services": allActive}), post("carrier-calculator.suspend-all", "Приостановить дополнительные службы", M{"action": "configure", "services": suspended}), post("carrier-calculator.remove-all", "Снять дополнительные службы", M{"action": "configure", "services": none}))
	reset := post("carrier-calculator.reset", "Сбросить этот план", M{"action": "reset"})
	reset.Confirm = "Сбросить службы и параметры только выбранного типа флотоносца?"
	actions = append(actions, reset, Action{ID: "carrier-calculator.route", Title: "Открыть маршрут флотоносца", Kind: "navigate", Path: "route"}, Action{ID: "carrier-calculator.source", Title: "Открыть калькулятор PTN", Kind: "url", Path: v.Text(v.At(c, "source", "url"), "https://pilotstradenetwork.com/tools/carrier-calculator/")})
	if mode == "Сравнение" && v.Str(selected["carrierType"]) != "" {
		actions = append([]Action{post("carrier-calculator.select-comparison", fmt.Sprintf("Открыть: %s", v.Str(selected["name"])), M{"action": "select", "carrierType": selected["carrierType"]})}, actions...)
	}
	return actions
}
