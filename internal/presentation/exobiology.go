package presentation

import (
	"fmt"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func ExobiologyPage(mode, query string, state M) Page {
	ex := v.Obj(state["exobiology"])
	p := Page{Title: "Экзобиология", Description: "Планеты, генетические образцы и продажа данных Vista Genomics.", Modes: []string{"Обзор", "Планеты", "Образцы", "Продажи", "Находки", "Места", "Статистика"}, Columns: []string{"Планета", "Система", "Биосигналы", "Картографирована", "Первый след"}}
	summary := v.Obj(ex["summary"])
	p.Summary = fmt.Sprintf("Планет: %s · Образцов к продаже: %s · Оценка: %s кр.", Number(summary["bodies"]), Number(summary["pendingSamples"]), Number(summary["pendingEstimatedCredits"]))
	add := func(r M, cells []string, details string) {
		if contains(query, append([]any{r["name"], r["name_en"], r["system"], r["body"]}, stringAny(cells)...)...) {
			p.Rows = append(p.Rows, row(r, cells, details))
		}
	}
	switch mode {
	case "Образцы", "Продажи", "Находки":
		path := "pending"
		if mode == "Продажи" {
			path = "sales"
		}
		if mode == "Находки" {
			path = "discoveries"
		}
		p.Columns = []string{"Вид", "Система", "Планета", "Прогресс", "Дата", "Выплата, кр."}
		for _, r := range v.Rows(ex[path]) {
			add(r, []string{v.Text(r["name"], r["name_en"]), v.Str(r["system"]), v.Str(r["body"]), exoPhase(r), Date(v.First(r["at"], r["completedAt"], r["updatedAt"])), Number(v.First(r["credits"], r["baseCredits"]))}, "Выплата подтверждается после продажи в Vista Genomics.")
		}
	case "Места":
		p.Columns = []string{"Планета", "Система", "Биосигналы", "Оценка от, кр.", "Оценка до, кр."}
		for _, r := range v.Rows(ex["targets"]) {
			value := v.Obj(r["valueRange"])
			add(r, []string{v.Str(r["name"]), v.Str(r["system"]), Number(r["signals"]), Number(value["minimum"]), Number(value["maximum"])}, "Оценка по возможным родам. Наличие видов и первый след требуют подтверждения.")
		}
	case "Статистика":
		p.Columns = []string{"Показатель", "Значение"}
		for _, pair := range [][2]string{{"bodies", "Наблюдавшиеся планеты"}, {"systems", "Наблюдавшиеся системы"}, {"completedSpeciesBodies", "Полные наборы образцов"}, {"pendingSamples", "Не проданные наборы"}, {"soldCredits", "Получено от Vista Genomics, кр."}, {"salesCount", "Продано наборов"}, {"firstFootfallBodies", "Планеты с первым следом"}} {
			add(M{"id": pair[0]}, []string{pair[1], Number(summary[pair[0]])}, "")
		}
	default:
		for _, r := range v.Rows(ex["bodies"]) {
			add(r, []string{v.Str(r["name"]), v.Str(r["system"]), Number(r["signals"]), Format(r["mapped"]), exoFootfall(r["firstFootfall"])}, fields(v.Obj(r["scan"]), "SurfaceTemperature", "Температура, K", "SurfaceGravity", "Гравитация, м/с²"))
		}
	}
	return p
}

func stringAny(values []string) []any {
	out := make([]any, len(values))
	for i, s := range values {
		out[i] = s
	}
	return out
}
func exoPhase(r M) string {
	switch r["phase"] {
	case "complete":
		return "Образцы собраны"
	case "sold":
		return "Продано"
	case "lost":
		return "Данные потеряны"
	}
	if n, ok := v.Number(r["sampleCount"]); ok {
		return fmt.Sprintf("%s / 3", Number(n))
	}
	return "—"
}
func exoFootfall(raw any) string {
	switch raw {
	case true, "first", "own", "own-first-footfall":
		return "Первый след"
	case "unwalked":
		return "Первый след доступен"
	case false, "visited", "walked":
		return "Уже посещена"
	}
	return "Неизвестно"
}

func ExobiologyActions(mode string, state, selected M) []Action {
	settings := v.Obj(v.At(state, "exobiology", "settings"))
	bodyKey := v.Text(selected["bodyKey"], selected["key"], v.At(state, "exobiology", "selectedBodyKey"))
	actions := []Action{{ID: "exobiology.configure", Title: "Настройки экзобиологии", Path: "/api/exobiology", Payload: M{"action": "configure"}, Fields: []Field{
		boolField("includeBacterium", "Включать бактерии", settings["includeBacterium"]), boolField("showUnlikely", "Показывать маловероятные виды", settings["showUnlikely"]), boolField("onlyUnwalked", "Только планеты без чужого первого следа", settings["onlyUnwalked"]), boolField("trackSamples", "Отслеживать расстояния между пробами", settings["trackSamples"]), boolField("predictionAlerts", "Уведомлять о новых подтверждениях", settings["predictionAlerts"]), boolField("pranavAntalBonus", "Учитывать бонус Pranav Antal", settings["pranavAntalBonus"]), numberField("minValue", "Минимальная оценка вида, кр.", v.First(settings["minValue"], 0), 0, 1000000000), numberField("minSignals", "Минимум биосигналов", v.First(settings["minSignals"], 1), 0, 100), numberField("maxArrivalLs", "Максимум от точки прибытия, св. с.", v.First(settings["maxArrivalLs"], 0), 0, 1000000000), numberField("radiusLy", "Радиус поиска, св. л.", v.First(settings["radiusLy"], 0), 0, 1000000),
	}}, {ID: "exobiology.select", Title: "Открыть планету", Path: "/api/exobiology", Payload: M{"action": "select-body", "bodyKey": bodyKey}, Disabled: bodyKey == ""}, {ID: "copy.system", Title: "Копировать систему", Kind: "copy", Path: v.Str(selected["system"]), Disabled: v.Str(selected["system"]) == ""}, {ID: "exobiology.bookmark", Title: "Сохранить планету", Path: "/api/exobiology", Payload: M{"action": "bookmark", "bodyKey": bodyKey}, Disabled: bodyKey == "", Fields: []Field{textField("notes", "Заметка", "")}}}
	return actions
}

func hudExobiology(doc *HUDDocument, state M) {
	ex := v.Obj(state["exobiology"])
	body := v.Obj(ex["currentBody"])
	tracker := v.Obj(ex["tracker"])
	docked := v.Bool(v.At(state, "commander", "docked"))

	// Sample separation changes the pilot's next action, so it comes first
	// while an unfinished set is active.
	sampling := tracker["phase"] == "tracking" && v.Int(tracker["sampleCount"]) > 0
	if sampling && !docked {
		hudExobiologySample(doc, tracker, body)
	}
	if !docked {
		if len(body) > 0 {
			hudExobiologyBody(doc, body, sampling && hudSameSampleBody(tracker, body))
			hudExobiologyMarks(doc, ex)
		} else {
			hudExobiologySurvey(doc, ex)
		}
	}
	if len(doc.Sections) == 0 || docked {
		summary := v.Obj(ex["summary"])
		if v.Num(summary["pendingSamples"]) > 0 {
			rows := []HUDLine{hudValue("Наборов", Number(summary["pendingSamples"]), "text")}
			if credits := v.Num(summary["pendingEstimatedCredits"]); credits > 0 {
				estimate := "≈ " + Number(credits) + " кр."
				if v.Num(summary["pendingUnknownPrices"]) > 0 {
					estimate = "≥ " + Number(credits) + " кр."
				}
				rows = append(rows, hudValue("Оценка", estimate, "muted"))
			}
			hudCard(doc, "bio", "ДАННЫЕ VISTA GENOMICS", "", 0, rows...)
		}
	}
}

func hudExobiologySample(doc *HUDDocument, tracker, body M) {
	rows := []HUDLine{hudValue("Собрано проб", Number(tracker["sampleCount"])+" / 3", "text")}
	sameBody := hudSameSampleBody(tracker, body)
	if bodyName := v.Str(tracker["bodyName"]); bodyName != "" && !sameBody {
		rows = append(rows, hudValue("Планета набора", bodyName, "muted"))
	}
	switch {
	case sameBody && tracker["nearestSampleMeetsMin"] == true:
		rows = append(rows, HUDLine{Text: "Можно брать пробу", Role: "good"})
	case sameBody && hudKnown(tracker["remainingDistanceM"]):
		rows = append(rows, hudValue("Ещё отойти", Number(tracker["remainingDistanceM"])+" м", "warn"))
	case hudKnown(tracker["minSampleDistanceM"]):
		rows = append(rows, hudValue("Между пробами", "≥ "+Number(tracker["minSampleDistanceM"])+" м", "muted"))
	}
	hudCard(doc, "bio", "ГЕНЕТИЧЕСКИЕ ОБРАЗЦЫ", v.Text(tracker["name"], tracker["name_en"]), 0, rows...)
}

func hudSameSampleBody(tracker, body M) bool {
	bodyKey := v.Str(body["key"])
	return bodyKey != "" && bodyKey == v.Str(tracker["bodyKey"])
}

func hudExobiologyBody(doc *HUDDocument, body M, sampling bool) {
	rows := []HUDLine{}
	if signals := body["signals"]; hudKnown(signals) {
		value := Number(signals)
		if complete := v.Num(v.At(body, "progress", "completeGenera")); complete > 0 {
			value = Number(complete) + " / " + value
			rows = append(rows, hudValue("Наборов собрано", value, "text"))
		} else {
			rows = append(rows, hudValue("Биосигналы", value, "text"))
		}
	}
	if footfall := exoFootfall(body["firstFootfall"]); footfall == "Первый след доступен" || footfall == "Первый след" {
		rows = append(rows, HUDLine{Text: footfall, Role: "good"})
	}
	more := 0
	if !sampling && !v.Bool(v.At(body, "progress", "allSamplesComplete")) {
		predictions := 0
		for _, species := range v.Rows(body["predictions"]) {
			name := v.Text(species["name"], species["name_en"])
			if name == "" || species["tier"] == "unlikely" || species["phase"] == "complete" || species["phase"] == "sold" {
				continue
			}
			if predictions >= 2 {
				more++
				continue
			}
			var estimate string
			if v.Num(species["estimatedCredits"]) > 0 {
				estimate = "≈ " + Number(species["estimatedCredits"]) + " кр."
			}
			role := "text"
			if species["confirmed"] != true {
				name, role = "? "+name, "muted"
			}
			rows = append(rows, hudValue(name, estimate, role))
			predictions++
		}
		if predictions == 0 {
			value := v.Obj(body["valueRange"])
			if minimum, maximum := v.Num(value["minimum"]), v.Num(value["maximum"]); maximum > 0 {
				estimate := "≈ " + Number(maximum) + " кр."
				if minimum > 0 && minimum < maximum {
					estimate = Number(minimum) + "–" + Number(maximum) + " кр."
				}
				rows = append(rows, hudValue("Оценка", estimate, "muted"))
			}
		}
	}
	hudCard(doc, "bio", "ПЛАНЕТА", v.Str(body["name"]), more, rows...)
}

func hudExobiologyMarks(doc *HUDDocument, ex M) {
	rows := []HUDLine{}
	for _, mark := range v.Rows(ex["surfaceMarks"]) {
		if mark["kind"] == "sample" || !hudKnown(mark["distanceM"]) || !hudKnown(mark["bearing"]) {
			continue
		}
		value := Number(mark["distanceM"]) + " м · " + Number(mark["bearing"]) + "°"
		if mark["confidence"] != "recorded-position" {
			value = "≈ " + value
		}
		rows = append(rows, hudValue(v.Text(mark["label"], "Точка поверхности"), value, "muted"))
		if len(rows) == 2 {
			break
		}
	}
	hudCard(doc, "bio", "ТОЧКИ ПОВЕРХНОСТИ", "", 0, rows...)
}

func hudExobiologySurvey(doc *HUDDocument, ex M) {
	system := v.Obj(ex["system"])
	rows := []HUDLine{}
	if total := system["bodiesTotal"]; hudKnown(total) {
		rows = append(rows, hudValue("FSS", Number(system["scannedBodies"])+" / "+Number(total), "text"))
	} else if v.Num(system["scannedBodies"]) > 0 {
		rows = append(rows, hudValue("FSS", Number(system["scannedBodies"]), "text"))
	}
	if count := v.Num(system["biologicalBodies"]); count > 0 {
		rows = append(rows, hudValue("Планет с биосигналами", Number(count), "text"))
	}
	if targets := v.Rows(ex["targets"]); len(targets) > 0 {
		planet := v.Str(targets[0]["name"])
		systemName := v.Str(targets[0]["system"])
		if systemName != "" && planet != "" && !strings.HasPrefix(strings.ToLower(planet), strings.ToLower(systemName)+" ") {
			planet = systemName + " · " + planet
		}
		if planet != "" {
			rows = append(rows, hudValue("Следующая планета", planet, "muted"))
		}
	}
	hudCard(doc, "bio", "РАЗВЕДКА СИСТЕМЫ", v.Str(system["name"]), 0, rows...)
}
