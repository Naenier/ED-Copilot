// Package presentation maps domain observations to native controls and Russian prose.
package presentation

import (
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type M = v.M
type Tab struct {
	ID, Title string
	Required  bool
}

var Tabs = []Tab{{"overview", "Обзор", true}, {"targets", "Цели", false}, {"sale", "Торговля", false}, {"encyclopedia", "Энциклопедия", false}, {"statistics", "Статистика", false}, {"settings", "Настройки", true}, {"deals", "Рейсы", false}, {"carrier", "Флотоносец", false}, {"route", "Маршрут флотоносца", false}, {"mining", "Заход добычи", false}, {"engineering", "Инженерия", false}, {"colonization", "Колонизация", false}, {"account", "Frontier", false}, {"integrations", "Синхронизация", false}, {"help", "Руководство", false}}
var DefaultTabs = []string{"overview", "targets", "sale", "encyclopedia", "statistics", "settings"}
var Activities = []Choice{{"flight", "Полёт и навигация"}, {"asteroid", "Добыча в кольцах"}, {"surface", "Планетарная добыча"}, {"raw", "Сбор сырья"}, {"trade", "Торговля"}, {"carrier", "Флотоносец"}, {"engineering", "Материалы для инженерии"}, {"colonization", "Колонизация и снабжение"}}

type Choice struct{ Value, Label string }
type Row struct {
	ID      string
	Data    M
	Cells   []string
	Details string
}
type Page struct {
	Title, Description, Summary string
	Modes                       []string
	Columns                     []string
	Rows                        []Row
}

func Title(id string) string {
	for _, t := range Tabs {
		if t.ID == id {
			return t.Title
		}
	}
	return "ED Copilot"
}
func NormalizeTabs(raw any) []string {
	valid := map[string]bool{}
	for _, t := range Tabs {
		valid[t.ID] = true
	}
	out := []string{}
	seen := map[string]bool{}
	for _, x := range v.Arr(raw) {
		s := v.Str(x)
		if valid[s] && !seen[s] {
			out = append(out, s)
			seen[s] = true
		}
	}
	if raw == nil {
		return append([]string(nil), DefaultTabs...)
	}
	if !seen["overview"] {
		out = append([]string{"overview"}, out...)
	}
	if !seen["settings"] {
		out = append(out, "settings")
	}
	return out
}
func Number(raw any) string {
	n, ok := v.Number(raw)
	if !ok {
		return "—"
	}
	s := strconv.FormatFloat(n, 'f', 0, 64)
	if math.Abs(n-math.Trunc(n)) > 0.00001 {
		s = strconv.FormatFloat(n, 'f', 2, 64)
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	parts := strings.Split(s, ".")
	whole := parts[0]
	negative := strings.HasPrefix(whole, "-")
	whole = strings.TrimPrefix(whole, "-")
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + " " + whole[i:]
	}
	if negative {
		whole = "-" + whole
	}
	if len(parts) > 1 {
		return whole + "," + parts[1]
	}
	return whole
}
func Date(raw any) string {
	t := v.Time(raw)
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("02.01.2006 15:04")
}

var labels = map[string]string{"planned": "В плане", "claimed": "Заявка подана", "building": "Строится", "complete": "Завершено", "completed": "Выполнено", "cancelled": "Отменено", "failed": "Провалено", "active": "Активно", "ready": "Готово к сдаче", "unknown": "Неизвестно", "confirmed": "Подтверждено", "potential": "Требует разведки", "candidate": "Проверить в игре", "unavailable": "Не подходит", "unlocked": "Доступен", "locked": "Закрыт", "invited": "Приглашение", "raw": "Сырьё", "manufactured": "Промышленные материалы", "encoded": "Данные", "buy": "Покупка", "sell": "Продажа", "transfer": "Перевод", "cost": "Расход", "journal": "Журнал игры", "manual": "Личная отметка", "spansh": "Spansh", "ship": "Корабль", "srv": "ТРП / Nomad", "foot": "Пешком", "medium": "Средняя", "high": "Высокая", "low": "Низкая", "laser": "Лазер", "core": "Ядра", "subsurface": "Подповерхностная", "surface": "Поверхность", "surface-planet": "Планетарная", "flight": "Полёт", "trade": "Торговля", "carrier": "Флотоносец", "engineering": "Инженерия", "colonization": "Колонизация", "normal": "Обычный", "travel": "Перелёт", "exploration": "Исследования", "combat": "Бой", "mission": "Миссия", "odyssey": "Пешие задачи", "other": "Другое"}

var detailLabels = map[string]string{"contribution": "новые поставки", "plan": "План", "blue": "Синяя", "orange": "Оранжевая", "green": "Зелёная", "purple": "Фиолетовая", "preset": "Готовая палитра", "custom": "Свои цвета", "matrix": "Матрица RGB", "game": "Палитра игры", "missing": "Не хватает", "incomplete": "Неполный комплект", "not-applicable": "Не применяется", "present": "Есть", "not-required": "Не требуется", "ok": "Готово", "disabled": "Отключено", "empty": "Нет запаса", "full": "Заполнено"}

func Label(raw any) string {
	s := v.Str(raw)
	if strings.Contains(s, "+") {
		parts := []string{}
		for _, part := range strings.Split(s, "+") {
			parts = append(parts, Label(part))
		}
		return strings.Join(parts, " + ")
	}
	if x := labels[s]; x != "" {
		return x
	}
	if x := detailLabels[s]; x != "" {
		return x
	}
	if x := map[string]string{"clean": "Нет розыска", "wanted": "В розыске", "hostile": "Враждебный", "speeding": "Превышение скорости"}[strings.ToLower(s)]; x != "" {
		return x
	}
	if s == "" {
		return "—"
	}
	return s
}
func Format(raw any) string {
	if raw == nil {
		return "—"
	}
	switch x := raw.(type) {
	case bool:
		if x {
			return "Да"
		}
		return "Нет"
	case string:
		if x == "" {
			return "—"
		}
		if !v.Time(x).IsZero() {
			return Date(x)
		}
		return Label(x)
	}
	if _, ok := v.Number(raw); ok {
		return Number(raw)
	}
	return "—"
}
func Ship(c M) string {
	model := v.Text(c["ship"], "Корабль не определён")
	if name := v.Str(c["shipName"]); name != "" {
		model += " «" + name + "»"
	}
	if ident := v.Str(c["shipIdent"]); ident != "" {
		model += " · " + ident
	}
	return model
}
func fields(m M, pairs ...string) string {
	out := []string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		raw := v.At(m, strings.Split(pairs[i], ".")...)
		if raw != nil && v.Str(raw) != "" {
			out = append(out, pairs[i+1]+": "+Format(raw))
		}
	}
	return strings.Join(out, "\r\n")
}
func row(raw M, cells []string, details string) Row {
	for i, cell := range cells {
		if strings.TrimSpace(cell) == "" {
			cells[i] = "—"
		}
	}
	id := v.Text(raw["id"], raw["key"], raw["commodity"], raw["marketId"])
	if id == "" && v.Str(raw["timestamp"]) != "" {
		id = strings.Join([]string{v.Str(raw["timestamp"]), v.Str(raw["event"]), v.Str(raw["kind"]), v.Str(raw["label"]), v.Str(raw["detail"])}, "|")
	}
	if id == "" && v.Str(raw["fromKey"]) != "" {
		id = v.Str(raw["fromKey"]) + "→" + v.Str(raw["toKey"])
	}
	if id == "" {
		id = v.Text(raw["date"], raw["name"], raw["system"], raw["title"])
	}
	return Row{ID: id, Data: raw, Cells: cells, Details: details}
}
func rowsByPath(s M, path string) []M   { return v.Rows(v.At(s, strings.Split(path, ".")...)) }
func sum(m M, key, label string) string { return label + ": " + Number(m[key]) }
func contains(query string, values ...any) bool {
	fold := func(s string) string { return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "ё", "е") }
	q := fold(query)
	if q == "" {
		return true
	}
	for _, x := range values {
		if strings.Contains(fold(v.Str(x)), q) {
			return true
		}
	}
	return false
}
func Build(tab, mode, query string, s M) Page {
	p := Page{Title: Title(tab), Columns: []string{"Параметр", "Значение"}}
	cmd := v.Obj(s["commander"])
	switch tab {
	case "overview":
		p.Description = "Положение, корабль и следующие действия для ваших занятий."
		p.Modes = []string{"Планы", "Состояние", "Корабль и груз", "Миссии", "Журнал"}
		p.Summary = v.Text(cmd["system"], "Ожидание журналов") + "  ·  " + Ship(cmd) + "  ·  " + Number(v.At(s, "pilot", "credits")) + " кр."
		switch mode {
		case "Состояние":
			p.Rows = append(p.Rows, row(M{"key": "ship"}, []string{"Корабль", Ship(cmd)}, "Модель, личное имя и идентификатор корабля."))
			for _, pair := range [][2]string{{"commander.system", "Система"}, {"commander.body", "Тело"}, {"commander.station", "Станция"}, {"commander.vehicle", "Положение пилота"}, {"commander.docked", "Пристыкован"}, {"commander.cargoTons", "Груз, т"}, {"commander.cargoCapacity", "Вместимость трюма, т"}, {"pilot.credits", "Баланс, кр."}, {"pilot.creditsAt", "Время баланса"}, {"pilot.creditsSource", "Источник баланса"}, {"pilot.fuel.mainTons", "Основное топливо, т"}, {"pilot.fuel.mainCapacityTons", "Вместимость основного бака, т"}, {"pilot.fuel.updatedAt", "Время снимка топлива"}, {"pilot.session.since", "Начало сеанса"}, {"pilot.session.jumps", "Прыжков за сеанс"}, {"pilot.session.distanceLy", "Пройдено за сеанс, св. л."}, {"pilot.legalState", "Правовой статус"}, {"pilot.navTarget.system", "Следующий прыжок"}, {"pilot.navTarget.remainingJumps", "Прыжков до цели"}} {
				p.Rows = append(p.Rows, row(M{"key": pair[0]}, []string{pair[1], Format(v.At(s, strings.Split(pair[0], ".")...))}, "Последние известные наблюдения журнала и текущего снимка игры. Проверяйте время обновления."))
			}
		case "Корабль и груз":
			p.Columns = []string{"Товар", "На борту, т"}
			for _, r := range v.Rows(cmd["cargo"]) {
				p.Rows = append(p.Rows, row(r, []string{v.Str(r["name"]), Number(r["count"])}, "Груз по последнему наблюдению игры.\r\n"+fields(cmd, "cargoUpdatedAt", "Время снимка")))
			}
		case "Миссии":
			p.Columns = []string{"Поручение", "Система", "Назначение", "Срок"}
			for _, r := range rowsByPath(s, "pilot.missions") {
				p.Rows = append(p.Rows, row(r, []string{v.Str(r["name"]), v.Str(r["destinationSystem"]), v.Str(r["destinationStation"]), Date(r["expiry"])}, fields(r, "status", "Статус", "faction", "Фракция", "reward", "Награда", "cargo", "Груз", "count", "Количество")))
			}
		case "Журнал":
			p.Columns = []string{"Время", "Действие", "Система", "Подробности"}
			for _, r := range rowsByPath(s, "pilot.recent") {
				p.Rows = append(p.Rows, row(r, []string{Date(r["timestamp"]), v.Str(r["label"]), v.Str(r["system"]), v.Str(r["detail"])}, fields(r, "station", "Станция", "detail", "Подробности")))
			}
		default:
			p.Columns = []string{"Задача", "Занятие", "Система", "Статус"}
			for _, r := range rowsByPath(s, "objectives") {
				p.Rows = append(p.Rows, row(r, []string{v.Str(r["title"]), Label(r["kind"]), v.Str(r["system"]), Label(r["status"])}, fields(r, "station", "Станция", "body", "Тело", "notes", "Заметки")))
			}
			for _, r := range rowsByPath(s, "colonization.projects") {
				x := v.Copy(r)
				x["navigate"] = "colonization"
				p.Rows = append(p.Rows, row(x, []string{v.Text(r["name"], r["targetSystem"]), "Колонизация", v.Str(r["targetSystem"]), Label(r["status"])}, "Построек: "+Number(len(v.Rows(r["buildings"])))+"\r\nПо известным сметам осталось: "+Number(r["totalMissing"])+" т"))
			}
		}
	case "targets":
		p.Description = "Личные цели, курс корабля и выбранные места добычи."
		p.Modes = []string{"Личные цели", "Места добычи", "Сохранённые места", "Навигация", "Инженеры"}
		switch mode {
		case "Сохранённые места":
			p.Columns = []string{"Система", "Тело / кольцо", "Занятие", "Сохранено"}
			for _, saved := range v.Rows(s["bookmarks"]) {
				target := v.Obj(saved["target"])
				for _, candidate := range v.Rows(s["targets"]) {
					if candidate["id"] == saved["targetId"] {
						target = candidate
						break
					}
				}
				p.Rows = append(p.Rows, row(saved, []string{v.Str(saved["system"]), v.Text(saved["ring"], saved["body"], saved["label"]), Label(saved["kind"]), Date(saved["addedAt"])}, detailsTarget(target)+"\r\nСохранённое место. При необходимости обновите сведения перед перелётом."))
			}
		case "Места добычи":
			p.Columns = []string{"Система / тело", "Место", "Качество", "Дальность, св. л.", "Основание"}
			for _, r := range rowsByPath(s, "targets") {
				if !contains(query, r["system"], r["body"], r["ring"]) {
					continue
				}
				p.Rows = append(p.Rows, row(r, []string{v.Str(r["system"]) + " · " + v.Str(r["body"]), v.Text(r["ring"], r["body"]), Label(r["tier"]), Number(r["distanceLy"]), Label(r["confidence"])}, detailsTarget(r)))
			}
		case "Навигация":
			p.Columns = []string{"Система", "Класс звезды", "Пройдено"}
			for _, r := range rowsByPath(s, "pilot.navRoute") {
				p.Rows = append(p.Rows, row(r, []string{v.Str(r["system"]), v.Str(r["starClass"]), Format(r["visited"])}, "Маршрут корабля прочитан из карты галактики игры."))
			}
		case "Инженеры":
			sub := Build("engineering", "Инженеры", query, s)
			p.Columns, p.Rows, p.Summary = sub.Columns, sub.Rows, sub.Summary
		default:
			p.Columns = []string{"Цель", "Занятие", "Приоритет", "Система", "Статус"}
			for _, r := range rowsByPath(s, "objectives") {
				if !contains(query, r["title"], r["system"], r["notes"]) {
					continue
				}
				p.Rows = append(p.Rows, row(r, []string{v.Str(r["title"]), Label(r["kind"]), Label(r["priority"]), v.Str(r["system"]), Label(r["status"])}, fields(r, "station", "Станция", "body", "Тело", "notes", "Заметки")))
			}
		}
	case "sale":
		p.Description = "Сравните рынки по цене, запасу, спросу, расстоянию и дате наблюдения."
		p.Modes = []string{"Продать", "Купить", "Журнал сделок"}
		trade := v.Obj(s["trading"])
		p.Summary = v.Text(trade["status"], trade["error"])
		if mode == "Журнал сделок" {
			p.Columns = []string{"Время", "Операция", "Товар", "Тонны", "Сумма, кр.", "Маржа, кр."}
			for _, r := range rowsByPath(s, "tradeLedger.recent") {
				p.Rows = append(p.Rows, row(r, []string{Date(r["timestamp"]), Label(r["kind"]), v.Str(r["name"]), Number(r["count"]), Number(r["value"]), Number(r["margin"])}, fields(r, "system", "Система", "station", "Станция", "internal", "Внутренний перевод", "unknownTons", "Без известной себестоимости")))
			}
			break
		}
		availableKey, availableTitle := "demand", "Спрос, т"
		if mode == "Купить" {
			availableKey, availableTitle = "supply", "Запас, т"
		}
		p.Columns = []string{"Станция", "Система", "Дальность, св. л.", "Цена, кр./т", availableTitle, "Наблюдение"}
		for _, r := range v.Rows(trade["results"]) {
			if !contains(query, r["station"], r["system"]) {
				continue
			}
			quotes := v.Rows(r["quotes"])
			q := M{}
			if len(quotes) > 0 {
				q = quotes[0]
			}
			observed := Date(r["updatedAt"])
			if v.Bool(r["expired"]) {
				observed = "Устарела · " + observed
			}
			p.Rows = append(p.Rows, row(r, []string{v.Str(r["station"]), v.Str(r["system"]), Number(r["distanceLy"]), Number(q["price"]), Number(q[availableKey]), observed}, marketDetails(r)))
		}
	case "deals":
		p.Description = "Прибыльные перевозки с учётом спроса, запаса, грузоподъёмности и возраста котировки."
		p.Columns = []string{"Откуда", "Куда", "Товар", "Груз, т", "Маржа, кр.", "На тонну, кр."}
		for _, r := range rowsByPath(s, "tradeRoutes.results") {
			legs := v.Rows(r["legs"])
			if len(legs) == 0 {
				legs = []M{r}
			}
			from, to, goods, tons, margins := []string{}, []string{}, []string{}, []string{}, []string{}
			details := ""
			for i, leg := range legs {
				a := v.Text(leg["sourceStation"], v.At(leg, "source", "station"), v.At(leg, "buy", "station"))
				b := v.Text(leg["destinationStation"], v.At(leg, "destination", "station"), v.At(leg, "sell", "station"))
				from, to = append(from, a), append(to, b)
				goods = append(goods, v.Text(leg["name"], leg["commodityName"]))
				tons = append(tons, Number(v.First(leg["quantity"], leg["tons"])))
				margins = append(margins, Number(v.First(leg["profitPerTonne"], leg["profitPerTon"], leg["marginPerTon"])))
				details += fmt.Sprintf("Рейс %d: %s → %s\r\n", i+1, a, b) + fields(leg, "name", "Товар", "quantity", "Груз, т", "buyPrice", "Покупка, кр./т", "sellPrice", "Продажа, кр./т", "supply", "Запас, т", "demand", "Спрос, т", "distanceLy", "Дальность, св. л.", "netEstimate", "Оценка прибыли, кр.") + "\r\n\r\n"
			}
			p.Rows = append(p.Rows, row(r, []string{strings.Join(from, " / "), strings.Join(to, " / "), strings.Join(goods, " / "), strings.Join(tons, " / "), Number(v.First(r["grossMargin"], r["profit"])), strings.Join(margins, " / ")}, details))
		}
	case "carrier":
		p.Description = "Личный флотоносец: состояние, склад, приказы и журнал операций."
		p.Modes = []string{"Состояние", "Склад", "Торговые приказы", "История", "Финансы", "Службы"}
		c := v.Obj(s["carrier"])
		p.Summary = v.Text(c["name"], "Данные флотоносца ещё не прочитаны") + " · " + v.Str(c["callsign"]) + " · " + v.Str(v.At(c, "location", "system"))
		switch mode {
		case "Финансы":
			for _, pair := range [][2]string{{"finance.balance", "Баланс, кр."}, {"finance.reserveBalance", "Финансовый резерв, кр."}, {"finance.availableBalance", "Доступно, кр."}, {"finance.reservePercent", "Доля резерва, %"}, {"finance.weeklyUpkeep", "Содержание в неделю, кр."}, {"finance.runwayWeeks", "Запас содержания, недель"}, {"finance.updatedAt", "Время финансового снимка"}} {
				p.Rows = append(p.Rows, row(M{"key": pair[0]}, []string{pair[1], Format(v.At(c, strings.Split(pair[0], ".")...))}, "Запас содержания рассчитан по заданному недельному расходу. Фактические платежи и баланс проверяйте в игре."))
			}
		case "Службы":
			p.Columns = []string{"Служба", "Установлена", "Активна", "Сотрудник", "Наблюдение"}
			for _, service := range v.Rows(c["services"]) {
				p.Rows = append(p.Rows, row(service, []string{v.Text(service["name"], service["key"]), Format(service["installed"]), Format(service["enabled"]), Format(service["crewName"]), Date(service["updatedAt"])}, "Службы устанавливаются и приостанавливаются через управление флотоносцем в игре."))
			}
		case "Склад":
			p.Columns = []string{"Товар", "Известный остаток, т", "Источник", "Наблюдение"}
			for _, r := range v.Rows(v.At(c, "warehouse", "items")) {
				p.Rows = append(p.Rows, row(r, []string{v.Text(r["name"], r["key"]), Number(r["count"]), Label(v.First(v.At(c, "warehouse", "sourceLabel"), v.At(c, "warehouse", "source"))), Date(v.At(c, "warehouse", "updatedAt"))}, "Отсутствие товара в неполном снимке не означает нулевой остаток."))
			}
		case "Торговые приказы":
			p.Columns = []string{"Товар", "Приказ", "Цена, кр./т", "Объём, т"}
			for _, r := range v.Rows(c["orders"]) {
				p.Rows = append(p.Rows, row(r, []string{v.Str(r["name"]), Label(v.First(r["side"], r["direction"])), Number(r["price"]), Number(v.First(r["quantity"], r["stock"]))}, fields(r, "timestamp", "Время", "key", "Товар")))
			}
		case "История":
			p.Columns = []string{"Время", "Операция", "Изменение счёта, кр.", "Баланс, кр."}
			for _, r := range v.Rows(c["transactions"]) {
				p.Rows = append(p.Rows, row(r, []string{Date(r["timestamp"]), v.Text(r["label"], Label(r["kind"])), Number(r["amount"]), Number(r["balance"])}, fields(r, "direction", "Направление", "source", "Источник")))
			}
		default:
			for _, pair := range [][3]string{{"fuel.tankTons", "Тритий в баке", "т"}, {"fuel.reserveTons", "Известный резерв трития", "т"}, {"fuel.needsRefresh", "Нужен свежий снимок топлива", ""}, {"space.capacityTons", "Общая вместимость", "т"}, {"space.cargoTons", "Товарный груз", "т"}, {"space.reservedTons", "Зарезервировано", "т"}, {"space.crewTons", "Службы и экипаж", "т"}, {"space.shipPacksTons", "Комплекты кораблей", "т"}, {"space.modulePacksTons", "Комплекты модулей", "т"}, {"space.freeTons", "Свободный склад", "т"}, {"finance.balance", "Баланс флотоносца", "кр."}, {"finance.weeklyUpkeep", "Содержание в неделю", "кр."}, {"jump.pending.system", "Назначенный прыжок", ""}, {"jump.pending.departureTime", "Отправление", ""}, {"dockingAccessLabel", "Доступ к стыковке", ""}, {"allowNotorious", "Доступ при дурной славе", ""}, {"pendingDecommission", "Назначено списание флотоносца", ""}, {"scrapTime", "Время списания", ""}, {"statsAt", "Параметры получены", ""}} {
				advisories := []string{"Проверяйте время и источник снимка в игре."}
				for _, advisory := range v.Rows(c["advisories"]) {
					advisories = append(advisories, v.Str(advisory["text"]))
				}
				p.Rows = append(p.Rows, row(M{"key": pair[0]}, []string{pair[1], Format(v.At(c, strings.Split(pair[0], ".")...)) + " " + pair[2]}, strings.Join(advisories, "\r\n")))
			}
		}
	case "route":
		p.Description = "Маршрут только для флотоносца: прыжки, тритий, резерв, дозаправки и время."
		p.Modes = []string{"Остановки", "Итоги"}
		r := v.Obj(s["carrierRoute"])
		p.Summary = fields(v.Obj(r["summary"]), "remainingJumps", "Осталось прыжков", "fuelTons", "Расход трития, т", "totalMinutes", "Время, мин")
		if mode == "Итоги" {
			p.Columns = []string{"Показатель", "Значение"}
			for _, pair := range [][2]string{{"totalJumps", "Всего прыжков"}, {"completedJumps", "Пройдено прыжков"}, {"remainingJumps", "Осталось прыжков"}, {"distanceLy", "Путь, св. л."}, {"directDistanceLy", "По прямой, св. л."}, {"fuelTons", "Расход трития, т"}, {"minimumStartingTritiumTons", "Минимум трития на старте, т"}, {"startingTritiumTons", "Всего трития на старте, т"}, {"startingTankTons", "На старте в баке, т"}, {"startingCargoTritiumTons", "На старте на складе, т"}, {"additionalTritiumNeededTons", "Дополнительно получить трития, т"}, {"restockTons", "Пополнить в пути, т"}, {"restockStops", "Остановок пополнения"}, {"acquireTritiumTons", "Всего получить трития, т"}, {"arrivalTritiumTons", "Всего трития при прибытии, т"}, {"arrivalTankTons", "При прибытии в баке, т"}, {"arrivalCargoTritiumTons", "При прибытии на складе, т"}, {"reserveTritiumTons", "Выбранный резерв, т"}, {"reserveShortfallTons", "Нехватка до резерва, т"}, {"travelMinutes", "Перелёт, мин"}, {"miningMinutes", "Добыча трития, мин"}, {"refuelMinutes", "Получение и доставка трития, мин"}, {"totalMinutes", "Всего времени, мин"}, {"timeRange.maxMinutes", "Время с введённой очередью, мин"}, {"departureTime", "Начало маршрута"}, {"estimatedArrival", "Расчётное прибытие"}, {"estimatedArrivalLatest", "Прибытие с введённой очередью"}, {"estimatedComplete", "Завершение всех операций"}, {"readyAgain", "Готовность к следующему прыжку"}, {"fuelCost", "Стоимость израсходованного трития, кр."}, {"acquireCost", "Стоимость получаемого трития, кр."}} {
				p.Rows = append(p.Rows, row(M{"key": pair[0]}, []string{pair[1], Format(v.At(v.Obj(r["summary"]), strings.Split(pair[0], ".")...))}, joinStrings(r["warnings"])+"\r\n"+joinStrings(r["assumptions"])))
			}
			break
		}
		p.Columns = []string{"№", "Система", "Дистанция, св. л.", "Расход, т", "В баке, т", "Дозаправка, т", "Пройдено"}
		for _, stop := range v.Rows(v.At(r, "route", "stops")) {
			p.Rows = append(p.Rows, row(stop, []string{Number(stop["index"]), v.Text(stop["name"], stop["system"]), Number(v.First(stop["distanceLy"], stop["distance"])), Number(stop["fuelUsedTons"]), Number(v.First(stop["tankTons"], stop["tankAfterTons"], stop["fuelRemainingTons"])), Number(stop["restockTons"]), Format(stop["visited"])}, fields(stop, "cargoTritiumTons", "Тритий в грузе, т", "elapsedMinutes", "Время от начала, мин", "refuelMinutes", "Дозаправка, мин", "estimatedArrival", "Прибытие", "estimatedDeparture", "Отправление", "estimatedReady", "Готовность")+"\r\n"+joinStrings(r["warnings"])+"\r\n"+joinStrings(r["assumptions"])))
		}
	case "colonization":
		p.Description = "Отдельные колонии, порядок строительства и независимые очереди снабжения."
		p.Modes = []string{"Выбор системы", "Строительство", "Очередь снабжения", "Снабжение", "Поставки"}
		c := v.Obj(s["colonization"])
		project := v.Obj(c["activeProject"])
		p.Summary = v.Text(project["name"], project["targetSystem"], "Создайте или выберите колонию") + " · " + Label(project["status"])
		switch mode {
		case "Строительство", "Очередь снабжения":
			p.Columns = []string{"№", "Объект", "Тело", "Уровень", "Статус", "Осталось, т"}
			buildings := v.Rows(project["buildings"])
			if mode == "Очередь снабжения" {
				buildings = v.Rows(project["supplyQueue"])
			}
			for i, r := range buildings {
				missing := 0.0
				for _, item := range v.Rows(r["requirements"]) {
					missing += v.Num(item["missing"])
				}
				remaining := "Неизвестно"
				if len(v.Rows(r["requirements"])) > 0 {
					remaining = Number(missing)
				}
				p.Rows = append(p.Rows, row(r, []string{Number(i + 1), v.Str(r["name"]), v.Str(r["body"]), Number(r["tier"]), Label(r["status"]), remaining}, buildDetails(r, project)))
			}
			if len(buildings) == 0 && len(project) > 0 && mode == "Строительство" {
				for i, advice := range v.Rows(project["buildAdvice"]) {
					p.Rows = append(p.Rows, row(M{"id": fmt.Sprintf("colony-advice-%d", i), "kind": "colony-advice"}, []string{"—", v.Str(advice["title"]), "—", "—", "Рекомендация", "—"}, v.Str(advice["text"])))
				}
			}
		case "Снабжение":
			p.Columns = []string{"Товар", "Нужно, т", "Сдано, т", "Осталось, т", "На борту, т", "На флотоносце, т", "Основание"}
			for _, r := range v.Rows(project["supplies"]) {
				p.Rows = append(p.Rows, row(r, []string{v.Text(r["name"], r["commodity"]), Number(r["required"]), Number(r["delivered"]), Number(r["missing"]), Number(r["cargoAvailable"]), Number(r["carrierAvailable"]), Label(r["source"])}, "Остаток по строительному складу уже включает принятые поставки. Груз на борту ещё не доставлен.\r\n"+fields(r, "updatedAt", "Наблюдение", "sourceNote", "Источник оценки")))
			}
		case "Поставки":
			p.Columns = []string{"Время", "Товар", "Объём, т", "Основание", "Объект"}
			for _, r := range append(v.Rows(project["manualDeliveries"]), v.Rows(project["journalDeliveries"])...) {
				resources := v.Rows(r["resources"])
				if len(resources) == 0 {
					resources = []M{r}
				}
				for _, q := range resources {
					p.Rows = append(p.Rows, row(r, []string{Date(r["timestamp"]), v.Text(q["name"], q["commodity"]), Number(v.First(q["count"], q["amount"])), Label(r["source"]), v.Text(r["buildingId"], r["marketId"])}, v.Str(r["notes"])))
				}
			}
		default:
			p.Columns = []string{"Система", "Дальность, св. л.", "Проверка", "Наблюдение"}
			search := v.Obj(c["search"])
			p.Summary = v.Text(search["error"], v.At(search, "completeness", "reason"), p.Summary)
			for _, r := range v.Rows(search["results"]) {
				p.Rows = append(p.Rows, row(r, []string{v.Text(r["system"], r["name"]), Number(r["distanceLy"]), Label(r["availability"]), Date(v.First(r["updatedAt"], r["checkedAt"]))}, joinStrings(r["reasons"])+"\r\nДоступность заявки окончательно подтверждается в игре."))
			}
		}
	case "engineering":
		p.Description = "План улучшений, материалы, обмен у торговцев и доступ к инженерам."
		p.Modes = []string{"План", "Материалы", "Материалы плана", "Сырьё", "Промышленные материалы", "Данные", "Сырьё плана", "Промышленные материалы плана", "Данные плана", "Обмен", "Инженеры"}
		e := v.Obj(s["engineering"])
		p.Summary = "Нужно материалов: " + Number(v.At(e, "shopping", "neededUnits")) + " · Не хватает: " + Number(v.At(e, "shopping", "missingUnits"))
		switch mode {
		case "Материалы", "Материалы плана", "Сырьё", "Промышленные материалы", "Данные", "Сырьё плана", "Промышленные материалы плана", "Данные плана":
			p.Columns = []string{"Материал", "Категория", "Уровень", "Есть", "Нужно", "Не хватает"}
			category, planOnly, _ := materialView(mode)
			for _, r := range v.Rows(e["materials"]) {
				if category != "" && r["category"] != category || planOnly && v.Num(r["needed"]) <= 0 {
					continue
				}
				p.Rows = append(p.Rows, row(r, []string{v.Str(r["name"]), Label(r["category"]), "G" + Number(r["grade"]), Number(r["owned"]), Number(r["needed"]), Number(r["missing"])}, fields(r, "familyName", "Группа обмена", "capacity", "Вместимость", "updatedAt", "Снимок")))
			}
		case "Обмен":
			p.Columns = []string{"Отдать", "Количество", "Получить", "Количество", "Хватает", "Помещается"}
			for _, r := range v.Rows(v.At(e, "trader", "offers")) {
				p.Rows = append(p.Rows, row(r, []string{v.Str(r["fromName"]), Number(r["requiredFrom"]), v.Str(r["toName"]), Number(r["received"]), Format(r["affordable"]), Format(r["storageFits"])}, fields(r, "available", "Доступно без резерва", "protectedCount", "Защищено планом", "maxGain", "Максимально можно получить")))
			}
		case "Инженеры":
			p.Columns = []string{"Инженер", "Система", "База", "Доступ", "Ранг"}
			for _, r := range v.Rows(e["engineers"]) {
				if !contains(query, r["name"], r["english"], r["system"]) {
					continue
				}
				p.Rows = append(p.Rows, row(r, []string{v.Str(r["name"]), v.Str(r["system"]), v.Str(r["base"]), Label(r["status"]), Number(r["rank"])}, fields(r, "english", "Название", "url", "Руководство")+"\r\n"+capabilityDetails(r)))
			}
		default:
			p.Columns = []string{"Модуль", "Чертёж", "Ступень", "Количество", "Экспериментальный эффект"}
			for _, r := range v.Rows(e["items"]) {
				p.Rows = append(p.Rows, row(r, []string{v.Str(r["moduleName"]), v.Str(r["blueprintName"]), "G" + Number(r["maxGrade"]), Number(r["quantity"]), v.Str(r["experimentalName"])}, recipeDetails(r["materials"])+"\r\n"+v.Str(r["label"])))
			}
		}
	case "encyclopedia":
		return encyclopedia(mode, query, s)
	case "help":
		p.Description = "Инструкции по работе с ED Copilot и игровыми инструментами."
		p.Columns = []string{"Раздел", "Тема", "Кратко"}
		for _, r := range data.Array("help.json") {
			if contains(query, r["title"], r["summary"], r["body"], r["keywords"]) {
				p.Rows = append(p.Rows, row(r, []string{v.Str(r["group"]), v.Str(r["title"]), v.Str(r["summary"])}, v.Str(r["body"])))
			}
		}
	case "statistics":
		p.Description = "Результаты за выбранный период. Доставки колониям учитываются отдельно от продаж."
		p.Modes = []string{"Ресурсы", "Итоги", "Дни", "Торговля", "Колонии"}
		p.Columns = []string{"Ресурс", "Произведено, т", "Сырьё, ед.", "Продано, т", "Выручка, кр."}
		for _, r := range rowsByPath(s, "stats.byResource") {
			p.Rows = append(p.Rows, row(r, []string{v.Str(r["name"]), Number(r["refinedTons"]), Number(r["materialUnits"]), Number(r["soldTons"]), Number(r["revenue"])}, "Фактические события журнала игры."))
		}
		if mode == "Итоги" {
			p.Columns = []string{"Показатель", "Значение"}
			p.Rows = nil
			for _, pair := range [][2]string{{"since", "Начало периода"}, {"total.refinedTons", "Произведено, т"}, {"total.materialUnits", "Собрано сырья, ед."}, {"total.soldTons", "Продано, т"}, {"total.revenue", "Выручка, кр."}, {"total.miningRevenue", "Выручка от добытого, кр."}, {"total.prospected", "Разведано астероидов"}, {"total.accepted", "Подходящих астероидов"}, {"total.rejected", "Пропущено астероидов"}, {"total.asteroidsCracked", "Взорвано ядер"}, {"elapsedSeconds", "Учтено времени, с"}, {"revenuePerHour", "Выручка в час, кр."}} {
				p.Rows = append(p.Rows, row(M{"key": pair[0]}, []string{pair[1], Format(v.At(v.Obj(s["stats"]), strings.Split(pair[0], ".")...))}, "Учёт по поддерживаемым событиям журнала за выбранный период. Выручка не равна прибыли; известная себестоимость и маржа показаны в торговом журнале."))
			}
		}
		if mode == "Дни" {
			p.Columns = []string{"Дата", "Произведено, т", "Сырьё", "Разведано астероидов", "Выручка, кр."}
			p.Rows = nil
			for _, r := range rowsByPath(s, "stats.sessions") {
				p.Rows = append(p.Rows, row(r, []string{v.Str(r["date"]), Number(r["refinedTons"]), Number(r["materialUnits"]), Number(r["prospected"]), Number(r["revenue"])}, ""))
			}
		}
		if mode == "Торговля" {
			sub := Build("sale", "Журнал сделок", query, s)
			p.Columns, p.Rows, p.Summary = sub.Columns, sub.Rows, sub.Summary
		}
		if mode == "Колонии" {
			sub := Build("colonization", "Поставки", query, s)
			p.Columns, p.Rows, p.Summary = sub.Columns, sub.Rows, sub.Summary
		}
	case "mining":
		p.Description = "Текущий заход добычи, оборудование и план дохода."
		p.Modes = []string{"Текущий заход", "История", "Снаряжение", "План"}
		m := v.Obj(s["mining"])
		p.Summary = "Добыто: " + Number(v.At(m, "current", "refinedTons")) + " т · Темп: " + Number(v.At(m, "current", "tonsPerHour")) + " т/ч"
		if mode == "История" {
			p.Columns = []string{"Начало", "Место", "Добыто, т", "Темп, т/ч"}
			for _, r := range v.Rows(m["history"]) {
				p.Rows = append(p.Rows, row(r, []string{Date(r["startedAt"]), v.Str(r["system"]), Number(r["refinedTons"]), Number(r["tonsPerHour"])}, fields(r, "body", "Тело", "method", "Способ")))
			}
		} else if mode == "Снаряжение" {
			p.Columns = []string{"Проверка", "Нужно", "Статус", "Найдено / доступно"}
			for _, check := range v.Rows(v.At(m, "preflight", "checks")) {
				p.Rows = append(p.Rows, row(check, []string{v.Text(check["label"], check["name"]), Format(check["required"]), Label(check["status"]), Number(v.First(check["usableCount"], check["count"], check["freeTons"]))}, v.Text(check["message"], check["text"], v.At(m, "preflight", "help"))))
			}
		} else if mode == "План" {
			p.Columns = []string{"Показатель", "Значение"}
			for _, pair := range [][2]string{{"targetTons", "Целевой груз, т"}, {"aboardTons", "Уже на борту, т"}, {"remainingTons", "Ещё добыть / купить, т"}, {"sellPrice", "Ориентир продажи, кр./т"}, {"purchasePrice", "Закупка, кр./т"}, {"unitMargin", "Маржа за тонну, кр."}, {"revenue", "Выручка, кр."}, {"operatingCost", "Расходы, кр."}, {"profit", "Плановая прибыль, кр."}, {"trips", "Рейсов"}, {"miningMinutes", "Добыча, мин"}, {"travelMinutes", "Перевозка, мин"}, {"totalMinutes", "Итого времени, мин"}, {"estimatedProfitPerHour", "Прибыль в час, кр."}} {
				plan := v.Obj(m["plan"])
				p.Rows = append(p.Rows, row(M{"key": pair[0]}, []string{pair[1], Format(plan[pair[0]])}, fields(plan, "priceSource", "Источник цены", "priceAt", "Цена от", "rateSource", "Основание темпа")+"\r\n"+joinStrings(plan["warnings"])))
			}
		} else {
			for _, pair := range [][2]string{{"current.refinedTons", "Произведено, т"}, {"current.tonsPerHour", "Темп, т/ч"}, {"current.prospectScans", "Разведано астероидов"}, {"current.asteroidsCracked", "Взорвано ядер"}, {"cargo.limpets", "Дроны в трюме"}, {"plan.profit", "Плановая прибыль"}, {"plan.totalMinutes", "Плановое время, мин"}} {
				p.Rows = append(p.Rows, row(M{"key": pair[0]}, []string{pair[1], Format(v.At(m, strings.Split(pair[0], ".")...))}, ""))
			}
			for _, r := range v.Rows(v.At(m, "preflight", "checks")) {
				p.Rows = append(p.Rows, row(r, []string{v.Text(r["label"], r["name"]), v.Text(r["message"], r["text"])}, v.Str(r["text"])))
			}
		}
	case "settings":
		p.Description = "Окно, палитра, папка журналов и необязательные ориентиры цен."
		for _, r := range []M{{"name": "Папка журналов", "value": v.At(s, "connection", "journalDir")}, {"name": "Режим оверлея", "value": Label(v.At(s, "settings", "activity"))}, {"name": "Палитра", "value": v.At(s, "theme", "settings", "preset")}, {"name": "Данные приложения", "value": v.At(s, "app", "profileDir")}} {
			p.Rows = append(p.Rows, row(r, []string{v.Str(r["name"]), Format(r["value"])}, "Изменения сохраняются в личном профиле Windows."))
		}
	case "account":
		p.Description = "Вход через официальный сайт Frontier. Пароль остаётся в браузере."
		a := v.Obj(s["frontier"])
		p.Summary = v.Text(a["lastError"], a["error"])
		p.Rows = append(p.Rows, row(M{"key": "connected"}, []string{"Вход сохранён", Format(a["connected"])}, ""))
		for _, pair := range [][2]string{{"summary.profile.commander", "Командир"}, {"summary.profile.credits", "Кредиты"}, {"summary.profile.system", "Система"}, {"summary.profile.ship", "Модель корабля"}, {"summary.profile.shipName", "Личное имя корабля"}, {"lastSync", "Последнее обновление"}} {
			p.Rows = append(p.Rows, row(M{"key": pair[0]}, []string{pair[1], Format(v.At(a, strings.Split(pair[0], ".")...))}, "Данные Companion API могут запаздывать относительно игры."))
		}
	case "integrations":
		p.Description = "Сохраните личный API-ключ Inara и включите отправку событий."
		a := v.Obj(s["integrations"])
		p.Summary = v.Text(a["lastError"], a["blockedReason"])
		for _, pair := range [][2]string{{"apiKeySet", "Ключ сохранён"}, {"enabled", "Отправлять события"}, {"queued", "В очереди"}, {"sent", "Подтверждено событий"}, {"lastSuccess", "Последняя успешная отправка"}, {"nextRetryAt", "Следующая попытка"}} {
			p.Rows = append(p.Rows, row(M{"key": pair[0]}, []string{pair[1], Format(a[pair[0]])}, "Ключ и очередь хранятся зашифрованными. Старые журналы при перечитывании не отправляются заново."))
		}
	}
	if query != "" && tab != "help" && tab != "encyclopedia" {
		filtered := p.Rows[:0]
		for _, r := range p.Rows {
			if contains(query, strings.Join(r.Cells, " "), r.Details) {
				filtered = append(filtered, r)
			}
		}
		p.Rows = filtered
	}
	return p
}
func joinStrings(raw any) string {
	out := []string{}
	for _, x := range v.Arr(raw) {
		out = append(out, v.Str(x))
	}
	return strings.Join(out, "\r\n")
}
func recipeDetails(raw any) string {
	out := []string{}
	for _, r := range v.Rows(raw) {
		out = append(out, v.Text(r["name"], materialName(r["key"]), r["key"])+": "+Number(v.First(r["count"], r["required"])))
	}
	return strings.Join(out, "\r\n")
}
func detailsTarget(r M) string {
	return fields(r, "recommendation", "Рекомендация", "reserve", "Запасы", "source", "Источник", "updatedAt", "Наблюдение") + "\r\n" + joinStrings(r["reasons"]) + "\r\n" + recipeDetails(v.First(r["hotspots"], r["resources"], r["materials"]))
}
func marketDetails(r M) string {
	out := fields(r, "system", "Система", "station", "Станция", "distanceLs", "От звезды, св. с.", "pad", "Площадка", "estimatedTotal", "Сумма сделки, кр.", "coveredTons", "Учтено груза, т", "missingTons", "Без котировки, т", "updatedAt", "Наблюдение", "source", "Источник")
	if v.Bool(r["expired"]) {
		out += "\r\nКотировка устарела: повторите поиск перед закупкой или продажей."
	}
	for _, warning := range v.Arr(r["warnings"]) {
		text := v.Text(v.Obj(warning)["text"], warning)
		if text != "" {
			out += "\r\n" + text
		}
	}
	for _, q := range v.Rows(r["quotes"]) {
		out += "\r\n\r\n" + v.Str(q["name"]) + "\r\n" + fields(q, "price", "Цена за тонну", "demand", "Спрос, т", "supply", "Запас, т", "quantity", "В расчёте, т", "subtotal", "Сумма по товару, кр.", "updatedAt", "Котировка товара от")
	}
	return out
}
func buildDetails(r, project M) string {
	out := fields(r, "status", "Статус", "body", "Тело", "pointCost", "Очки начала", "pointReward", "Очки завершения", "billSource", "Основание сметы") + "\r\n" + joinStrings(r["warnings"])
	for _, q := range v.Rows(r["requirements"]) {
		out += "\r\n" + v.Str(q["name"]) + ": нужно " + Number(q["required"]) + ", принято " + Number(q["delivered"]) + ", осталось " + Number(q["missing"]) + " т"
	}
	for _, a := range v.Rows(project["buildAdvice"]) {
		out += "\r\n\r\n" + v.Str(a["title"]) + "\r\n" + v.Str(a["text"])
	}
	return out
}
func capabilityDetails(r M) string {
	out := []string{}
	for _, c := range v.Rows(r["capabilities"]) {
		out = append(out, v.Str(c["moduleName"])+" · "+v.Str(c["blueprintName"])+" · G"+Number(c["maxGrade"]))
	}
	return strings.Join(out, "\r\n")
}
func encyclopedia(mode, query string, s M) Page {
	p := Page{Title: "Энциклопедия", Description: "Справочник по всем основным занятиям Elite Dangerous с поиском и источниками.", Modes: []string{"Статьи", "Корабли", "Товары", "Материалы", "Инженеры", "Чертежи", "Модули", "Эффекты"}, Columns: []string{"Название", "Раздел", "Кратко"}}
	switch mode {
	case "Корабли":
		p.Columns = []string{"Модель", "Производитель", "Площадка", "Масса корпуса, т"}
		for _, r := range data.Array("encyclopedia-ships.json") {
			if contains(query, r["name"], r["english"], r["manufacturer"]) {
				p.Rows = append(p.Rows, row(r, []string{v.Str(r["name"]), v.Str(r["manufacturer"]), v.Str(r["pad"]), Number(r["hullMass"])}, shipReferenceDetails(r)))
			}
		}
	case "Товары":
		for _, r := range v.Rows(s["goods"]) {
			if contains(query, r["name"], r["english"], r["description"]) {
				p.Rows = append(p.Rows, row(referenceData(r, v.Rows(r["sources"])), []string{v.Str(r["name"]), Label(r["tier"]), v.Str(r["description"])}, v.Str(r["description"])+"\r\n"+joinStrings(r["tips"])+"\r\n"+referenceSources(v.Rows(r["sources"]))))
			}
		}
	case "Материалы", "Инженеры", "Чертежи", "Модули", "Эффекты":
		key := map[string]string{"Материалы": "materials", "Инженеры": "engineers", "Чертежи": "blueprints", "Модули": "modules", "Эффекты": "experimentals"}[mode]
		catalog := data.Object("engineering-data.json")
		for _, r := range v.Rows(catalog[key]) {
			if contains(query, r["name"], r["english"]) {
				p.Rows = append(p.Rows, row(referenceData(r, v.Rows(catalog["sources"])), []string{v.Str(r["name"]), Label(r["category"]), v.Text(r["description"], r["system"])}, engineeringReferenceDetails(key, r, catalog)))
			}
		}
	default:
		codex := data.Object("encyclopedia.json")
		sections := map[string]string{}
		for _, r := range v.Rows(codex["sections"]) {
			sections[v.Str(r["id"])] = v.Text(r["title"], r["name"])
		}
		for _, r := range v.Rows(codex["articles"]) {
			if contains(query, r["title"], r["summary"], r["keywords"], joinStrings(v.First(r["paragraphs"], r["body"]))) {
				refs := []M{}
				for _, id := range v.Arr(r["sourceIds"]) {
					if ref := v.Obj(v.Obj(codex["sources"])[v.Str(id)]); len(ref) > 0 {
						refs = append(refs, ref)
					}
				}
				p.Rows = append(p.Rows, row(referenceData(r, refs), []string{v.Str(r["title"]), sections[v.Str(r["section"])], v.Str(r["summary"])}, v.Str(r["summary"])+"\r\n\r\n"+joinStrings(v.First(r["paragraphs"], r["body"]))+"\r\n\r\n"+joinStrings(r["tips"])+"\r\n\r\n"+referenceSources(refs)))
			}
		}
	}
	sort.SliceStable(p.Rows, func(i, j int) bool { return p.Rows[i].Cells[0] < p.Rows[j].Cells[0] })
	return p
}
func HelpFor(tab string) string {
	preferred := map[string]string{"overview": "overview", "targets": "targets", "sale": "trade", "deals": "deals", "carrier": "carrier", "route": "carrier-route", "engineering": "engineering", "colonization": "colonization", "encyclopedia": "encyclopedia", "statistics": "statistics", "mining": "mining-analysis", "settings": "settings", "account": "frontier", "integrations": "integrations", "help": "start"}
	for _, r := range data.Array("help.json") {
		if v.Str(r["id"]) == preferred[tab] {
			return v.Str(r["body"])
		}
	}
	for _, r := range data.Array("help.json") {
		if r["tab"] == tab {
			return v.Str(r["body"])
		}
	}
	return "Выберите раздел, используйте поиск и кнопки действий. Сохранённые планы остаются в личном профиле Windows."
}
func Age(at any, now time.Time) string {
	t := v.Time(at)
	if t.IsZero() {
		return "Возраст неизвестен"
	}
	h := now.Sub(t).Hours()
	if h < 1 {
		return fmt.Sprintf("%d мин назад", int(math.Max(0, h*60)))
	}
	return fmt.Sprintf("%d ч назад", int(h))
}
