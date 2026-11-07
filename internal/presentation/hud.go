package presentation

import (
	"math"
	"sort"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// HUDDocument contains only the selected activity. It has no dependency on a
// window system, so focus policy, unknown observations and colony isolation are
// tested without a running game or a real commander profile.
type HUDDocument struct {
	Activity string
	Sections []HUDSection
}
type HUDSection struct {
	Title string
	Lines []HUDLine
}
type HUDLine struct{ Text, Role string }
type HUDRect struct{ X, Y, Width, Height int }

// HUDVisible requires an identified game process. Journal activity alone is
// insufficient: old journal files also exist when another program has focus.
func HUDVisible(enabled bool, running any, foregroundProcess string) bool {
	if !enabled || running == false {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(foregroundProcess))
	name = strings.ReplaceAll(name, `\`, "/")
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	switch name {
	case "elitedangerous.exe", "elitedangerous32.exe", "elitedangerous64.exe":
		return true
	}
	return false
}

// HUDGeometry uses physical pixels and the active game's client area. Ship and
// both sizes of SRV share the lower origin, leaving cockpit notifications free.
func HUDGeometry(view HUDRect, vehicle string, dpi, contentHeight int) HUDRect {
	if dpi < 72 || dpi > 480 {
		dpi = 96
	}
	scale := float64(dpi) / 96
	pad := max(8, int(math.Round(14*scale)))
	width := min(int(math.Round(410*scale)), max(0, view.Width-2*pad))
	top := int(math.Round(270 * float64(view.Height) / 1080))
	if vehicle == "foot" {
		top = pad
	}
	top = min(max(0, top), max(0, view.Height-pad-80))
	height := min(max(0, contentHeight), max(0, view.Height-top-pad))
	return HUDRect{view.X + view.Width - pad - width, view.Y + top, width, height}
}

func BuildHUD(s M, now time.Time) HUDDocument {
	activity := v.Text(v.At(s, "settings", "activity"), "flight")
	doc := HUDDocument{Activity: activity}
	add := func(title string, lines ...HUDLine) { doc.Sections = append(doc.Sections, HUDSection{title, lines}) }
	line := func(text string) HUDLine { return HUDLine{text, "text"} }
	muted := func(text string) HUDLine { return HUDLine{text, "muted"} }
	warn := func(text string) HUDLine { return HUDLine{text, "warn"} }
	cmd := v.Obj(s["commander"])
	context := []HUDLine{line(v.Text(cmd["system"], "Положение ещё не определено"))}
	if body := v.Str(cmd["body"]); body != "" {
		context = append(context, muted(body))
	}
	if station := v.Str(cmd["station"]); station != "" && v.Bool(cmd["docked"]) {
		context = append(context, muted(station))
	}
	context = append(context, muted(Ship(cmd)))
	add("ED COPILOT · "+hudActivity(activity), context...)
	cargo := func() {
		rows := []HUDLine{line("Груз: " + Number(cmd["cargoTons"]) + " / " + Number(cmd["cargoCapacity"]) + " т")}
		if free := v.At(s, "mining", "cargo", "freeTons"); free != nil {
			rows = append(rows, muted("Свободно: "+Number(free)+" т"))
		}
		add("ТРЮМ", rows...)
	}
	switch activity {
	case "asteroid":
		prospect := v.Obj(s["prospect"])
		if len(prospect) == 0 {
			add("РАЗВЕДКА АСТЕРОИДА", muted("Запустите дрон-разведчик, чтобы увидеть состав и ценность."))
		} else {
			decision := map[string]string{"mine": "Можно добывать", "skip": "Пропустить", "unknown": "Недостаточно данных"}[v.Str(prospect["decision"])]
			if decision == "" {
				decision = "Недостаточно данных"
			}
			role := "warn"
			if prospect["decision"] == "mine" {
				role = "good"
			}
			rows := []HUDLine{{decision, role}}
			if prospect["remaining"] != nil {
				rows = append(rows, muted("Осталось в астероиде: "+Number(prospect["remaining"])+" %"))
			}
			materials := append([]M(nil), v.Rows(prospect["materials"])...)
			sort.SliceStable(materials, func(i, j int) bool { return v.Num(materials[i]["percent"]) > v.Num(materials[j]["percent"]) })
			for i, r := range materials {
				if i == 3 {
					break
				}
				rows = append(rows, line(v.Text(r["name"], r["key"])+" · "+Number(r["percent"])+" %"))
			}
			if core := v.Str(prospect["motherlode"]); core != "" {
				rows = append(rows, line("Ядро: "+hudGood(s, core)))
			}
			for i, r := range v.Arr(prospect["reasons"]) {
				if i == 2 {
					break
				}
				rows = append(rows, muted(v.Str(r)))
			}
			add("РАЗВЕДКА АСТЕРОИДА", rows...)
		}
		cargo()
		add("ТЕКУЩИЙ ЗАХОД", line("Добыто: "+Number(v.At(s, "mining", "current", "refinedTons"))+" т · Темп: "+Number(v.At(s, "mining", "current", "tonsPerHour"))+" т/ч"), muted("Дроны в трюме: "+Number(v.At(s, "mining", "cargo", "limpets"))))
		hudDestination(&doc, s, now)
	case "surface":
		surface := v.Obj(s["surface"])
		rows := []HUDLine{line(v.Text(surface["site"], "Участок ещё не определён"))}
		for i, r := range v.Rows(surface["lastRefined"]) {
			if i == 3 {
				break
			}
			rows = append(rows, line(v.Text(r["name"], r["key"])+": "+Number(r["count"])+" т"))
		}
		if len(rows) == 1 {
			rows = append(rows, muted("Произведённые товары появятся после события переработки."))
		}
		add("ПЛАНЕТАРНАЯ ДОБЫЧА", rows...)
		cargo()
		hudTargets(&doc, s, "surface")
	case "raw":
		rows := []HUDLine{}
		for _, r := range v.Rows(v.At(s, "engineering", "materials")) {
			if v.Str(r["category"]) == "raw" && v.Num(r["missing"]) > 0 {
				rows = append(rows, line(v.Str(r["name"])+": не хватает "+Number(r["missing"])+" ед."))
				if len(rows) == 4 {
					break
				}
			}
		}
		if len(rows) == 0 {
			if len(v.Rows(v.At(s, "engineering", "items"))) == 0 {
				rows = append(rows, muted("Добавьте план улучшений, чтобы выбрать нужное сырьё."))
			} else if !v.Bool(v.At(s, "engineering", "shopping", "knownCategories", "raw")) {
				rows = append(rows, warn("Запасы сырья ещё неизвестны. Откройте инвентарь материалов в игре."))
			} else {
				rows = append(rows, muted("В инженерном плане нет дефицита сырья."))
			}
		}
		add("НУЖНОЕ СЫРЬЁ", rows...)
		hudTargets(&doc, s, "raw")
	case "trade":
		cargo()
		plan := v.Obj(s["tradePlan"])
		if len(plan) > 0 {
			rows := []HUDLine{line(v.Text(plan["name"], "План перевозки")), line("Груз: " + Number(v.First(plan["quantity"], plan["targetTons"])) + " т")}
			if margin := v.First(plan["grossMargin"], plan["profit"]); margin != nil {
				rows = append(rows, line("Плановая маржа: "+Number(margin)+" кр."))
			}
			add("ПЕРЕВОЗКА", rows...)
		}
		hudDestination(&doc, s, now)
	case "carrier":
		c := v.Obj(s["carrier"])
		if !v.Bool(c["owned"]) {
			add("ФЛОТОНОСЕЦ", muted("Личный флотоносец ещё не обнаружен в журнале или Frontier."))
			break
		}
		add("ФЛОТОНОСЕЦ", line(v.Text(c["name"], "Личный флотоносец")+" · "+v.Str(c["callsign"])), muted(v.Text(v.At(c, "location", "system"), "Система неизвестна")), line("Тритий в баке: "+Number(v.At(c, "fuel", "tankTons"))+" т · В резерве: "+Number(v.At(c, "fuel", "reserveTons"))+" т"), muted("Свободный склад: "+Number(v.At(c, "space", "freeTons"))+" т"))
		if v.Bool(v.At(c, "fuel", "needsRefresh")) {
			add("ТОПЛИВО", warn("После прыжка нужен свежий снимок бака. Обновите данные флотоносца в игре или Frontier."))
		}
		pending := v.Obj(v.At(c, "jump", "pending"))
		if len(pending) > 0 {
			add("НАЗНАЧЕННЫЙ ПРЫЖОК", line(v.Text(pending["system"], pending["name"], "Назначение неизвестно")), line("Отправление: "+Date(pending["departureTime"])))
		}
		route := v.Obj(s["carrierRoute"])
		summary := v.Obj(route["summary"])
		stops := v.Rows(v.At(route, "route", "stops"))
		idx := v.Int(v.At(route, "progress", "nextIndex"))
		if idx <= 0 {
			idx = v.Int(v.At(route, "route", "activeIndex"))
		}
		if idx > 0 && idx < len(stops) {
			stop := stops[idx]
			rows := []HUDLine{line("Далее: " + v.Text(stop["name"], stop["system"])), line("Расход: " + Number(stop["fuelUsedTons"]) + " т · Дистанция: " + Number(stop["distanceLy"]) + " св. л.")}
			if v.Num(stop["restockTons"]) > 0 {
				rows = append(rows, warn("Дозаправка: "+Number(stop["restockTons"])+" т"))
			}
			rows = append(rows, muted("Осталось прыжков: "+Number(summary["remainingJumps"])+" · Весь план: "+Number(summary["totalMinutes"])+" мин"))
			add("МАРШРУТ ФЛОТОНОСЦА", rows...)
		} else if v.Str(route["error"]) != "" {
			add("МАРШРУТ ФЛОТОНОСЦА", warn(v.Str(route["error"])))
		}
	case "engineering":
		shopping := v.Obj(v.At(s, "engineering", "shopping"))
		rows := []HUDLine{line("По плану нужно: " + Number(shopping["neededUnits"]) + " ед.")}
		if v.Num(shopping["unknownMaterials"]) > 0 {
			rows = append(rows, warn("Часть запасов неизвестна: откройте инвентарь материалов в игре."))
		} else {
			rows = append(rows, line("Не хватает: "+Number(shopping["missingUnits"])+" ед."))
		}
		for _, r := range v.Rows(v.At(s, "engineering", "materials")) {
			if v.Num(r["missing"]) > 0 {
				rows = append(rows, line(v.Str(r["name"])+": "+Number(r["missing"])+" ед."))
				if len(rows) >= 6 {
					break
				}
			}
		}
		add("МАТЕРИАЛЫ ДЛЯ УЛУЧШЕНИЙ", rows...)
		for _, r := range v.Rows(v.At(s, "engineering", "trader", "offers")) {
			if v.Bool(r["affordable"]) && v.Bool(r["storageFits"]) {
				add("ДОСТУПНЫЙ ОБМЕН", line(Number(r["requiredFrom"])+" × "+v.Str(r["fromName"])), line("→ "+Number(r["received"])+" × "+v.Str(r["toName"])))
				break
			}
		}
	case "colonization":
		project := v.Obj(v.At(s, "colonization", "activeProject"))
		if len(project) == 0 {
			add("КОЛОНИЗАЦИЯ", muted("Выберите колонию в приложении, чтобы увидеть её очередь."))
			break
		}
		rows := []HUDLine{line(v.Text(project["name"], project["targetSystem"])), muted(v.Text(project["targetSystem"], "Система неизвестна") + " · " + Label(project["status"]))}
		if project["deadline"] != nil {
			rows = append(rows, muted("Срок первичного порта: "+Date(project["deadline"])))
		}
		add("ВЫБРАННАЯ КОЛОНИЯ", rows...)
		if status := v.Str(project["status"]); status == "cancelled" || status == "complete" {
			break
		}
		if task := v.Obj(project["nextTask"]); len(task) > 0 {
			add("СЛЕДУЮЩЕЕ ДЕЙСТВИЕ", line(v.Text(task["text"], "Откройте план строительства.")))
		}
		rows = nil
		if n := v.Int(project["unknownBills"]); n > 0 {
			rows = append(rows, warn("Сметы неизвестны: "+Number(n)+". Посетите строительные депо."))
		}
		for _, r := range v.Rows(project["operationalSupplies"]) {
			if v.Num(r["missing"]) <= 0 {
				continue
			}
			text := v.Text(r["name"], r["commodity"]) + ": осталось " + Number(r["missing"]) + " т"
			if r["cargoAvailable"] != nil {
				text += " · на борту " + Number(r["cargoAvailable"]) + " т"
			}
			role := "text"
			if !v.Bool(r["authoritative"]) && v.Str(r["source"]) == "manual" {
				text += " (оценка)"
				role = "warn"
			}
			rows = append(rows, HUDLine{text, role})
			if len(rows) >= 5 {
				break
			}
		}
		if len(rows) == 0 {
			if v.Int(project["unknownBills"]) > 0 {
				rows = append(rows, warn("Потребность пока неизвестна."))
			} else if len(v.Rows(project["supplyQueue"])) == 0 {
				rows = append(rows, muted("Добавьте объекты в план строительства."))
			} else {
				rows = append(rows, muted("Известные сметы обеспечены. Проверьте завершение в игре."))
			}
		}
		add("СНАБЖЕНИЕ КОЛОНИИ", rows...)
	default:
		fuel := v.Obj(v.At(s, "pilot", "fuel"))
		add("СОСТОЯНИЕ КОРАБЛЯ", line("Топливо: "+Number(fuel["mainTons"])+" / "+Number(fuel["mainCapacityTons"])+" т"), line("Груз: "+Number(cmd["cargoTons"])+" / "+Number(cmd["cargoCapacity"])+" т"))
		target := v.Obj(v.At(s, "pilot", "navTarget"))
		if v.Str(target["system"]) != "" {
			add("СЛЕДУЮЩИЙ ПРЫЖОК", line(v.Str(target["system"])), muted("Класс звезды: "+v.Text(target["starClass"], "неизвестен")))
		} else {
			for _, r := range v.Rows(v.At(s, "pilot", "navRoute")) {
				if !v.Bool(r["visited"]) {
					add("СЛЕДУЮЩИЙ ПРЫЖОК", line(v.Str(r["system"])), muted("Класс звезды: "+v.Text(r["starClass"], "неизвестен")))
					break
				}
			}
		}
		for _, r := range v.Rows(s["objectives"]) {
			if status := v.Str(r["status"]); status == "active" || status == "planned" {
				add("ЛИЧНАЯ ЦЕЛЬ", line(v.Str(r["title"])), muted(v.Text(r["system"], r["station"], r["notes"])))
				break
			}
		}
	}
	return doc
}

func hudActivity(id string) string {
	for _, a := range Activities {
		if a.Value == id {
			return strings.ToUpper(a.Label)
		}
	}
	return "ПОЛЁТ И НАВИГАЦИЯ"
}
func hudGood(s M, key string) string {
	for _, r := range v.Rows(s["goods"]) {
		if v.Key(r["key"]) == v.Key(key) {
			return v.Text(r["name"], key)
		}
	}
	return key
}
func hudTargets(doc *HUDDocument, s M, kind string) {
	rows := []HUDLine{}
	for _, r := range v.Rows(s["targets"]) {
		if r["kind"] != "surface" {
			continue
		}
		if kind == "raw" && len(v.Rows(r["materials"])) == 0 {
			continue
		}
		rows = append(rows, HUDLine{v.Text(r["system"]) + " · " + v.Text(r["body"]), "text"}, HUDLine{Label(r["tier"]) + " · " + Number(r["distanceLy"]) + " св. л. · " + Label(r["confidence"]), "muted"})
		if len(rows) >= 4 {
			break
		}
	}
	if len(rows) == 0 {
		rows = append(rows, HUDLine{"Сканируйте тела или выполните поиск мест добычи в приложении.", "muted"})
	}
	doc.Sections = append(doc.Sections, HUDSection{"МЕСТА ДОБЫЧИ", rows})
}
func hudDestination(doc *HUDDocument, s M, now time.Time) {
	d := v.Obj(v.At(s, "trading", "destination"))
	if len(d) == 0 {
		doc.Sections = append(doc.Sections, HUDSection{"КУДА ВЕЗТИ ГРУЗ", []HUDLine{{"Найдите рынок и выберите его назначением во вкладке «Торговля».", "muted"}}})
		return
	}
	rows := []HUDLine{{v.Text(d["station"], "Станция неизвестна"), "text"}, {v.Text(d["system"], "Система неизвестна") + " · " + Number(d["distanceLy"]) + " св. л.", "muted"}}
	quotes := v.Rows(d["quotes"])
	if len(quotes) == 0 && d["price"] != nil {
		quotes = []M{d}
	}
	for i, q := range quotes {
		if i == 2 {
			break
		}
		rows = append(rows, HUDLine{v.Text(q["name"], hudGood(s, v.Str(q["key"]))) + ": " + Number(q["price"]) + " кр./т · спрос " + Number(q["demand"]) + " т", "text"})
	}
	if at := v.First(d["updatedAt"], d["timestamp"]); at != nil {
		rows = append(rows, HUDLine{"Котировка: " + Age(at, now), "muted"})
	}
	if v.Bool(d["expired"]) {
		rows = append(rows, HUDLine{"Котировка устарела или её время неизвестно. Обновите поиск перед продажей.", "warn"})
	}
	if v.Bool(d["cargoChanged"]) {
		rows = append(rows, HUDLine{"Состав груза изменился после выбора рынка. Повторите поиск.", "warn"})
	}
	if v.Bool(d["bulkTaxRisk"]) {
		rows = append(rows, HUDLine{"Низкий спрос: фактическая цена может снизиться из-за объёма груза.", "warn"})
	}
	doc.Sections = append(doc.Sections, HUDSection{"ВЫБРАННЫЙ РЫНОК", rows})
}
