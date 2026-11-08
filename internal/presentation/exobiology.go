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
	if len(body) == 0 && v.Bool(v.At(state, "commander", "docked")) && len(tracker) == 0 && v.Num(v.At(ex, "summary", "pendingSamples")) == 0 && len(v.Rows(ex["targets"])) == 0 {
		return
	}
	if len(body) == 0 {
		system := v.Obj(ex["system"])
		if len(system) > 0 && (system["bodiesTotal"] != nil || v.Num(system["scannedBodies"]) > 0) {
			lines := []HUDLine{hudMuted("FSS: " + Number(system["scannedBodies"]) + " / " + Number(system["bodiesTotal"]))}
			if v.Num(system["biologicalBodies"]) > 0 {
				lines = append(lines, hudText("Планет с биосигналами: "+Number(system["biologicalBodies"])), hudMuted("Картографировано: "+Number(system["mappedBiologicalBodies"])))
			}
			hudAdd(doc, "РАЗВЕДКА СИСТЕМЫ", lines...)
		} else {
			hudAdd(doc, "ЭКЗОБИОЛОГИЯ", hudMuted("Выберите планету с биосигналами. FSS и DSS уточнят её состав."))
		}
	} else {
		lines := []HUDLine{hudText(v.Str(body["name"]))}
		if body["signals"] != nil {
			lines = append(lines, hudMuted("Биосигналы: "+Number(body["signals"])+" · "+exoFootfall(body["firstFootfall"])))
		}
		value := v.Obj(body["valueRange"])
		if value["maximum"] != nil {
			lines = append(lines, hudMuted("Оценка: "+Number(value["minimum"])+"–"+Number(value["maximum"])+" кр."))
		}
		hudAdd(doc, "ПЛАНЕТА", lines...)
	}
	if len(tracker) > 0 {
		lines := []HUDLine{hudText(v.Text(tracker["name"], tracker["name_en"]) + " · " + Number(tracker["sampleCount"]) + " / 3")}
		if tracker["bodyKey"] != body["key"] && v.Str(tracker["bodyName"]) != "" {
			lines = append(lines, hudWarn("Незавершённый набор: "+v.Str(tracker["bodyName"])))
		}
		if tracker["phase"] == "complete" {
			lines = append(lines, hudText("Образцы собраны — можно искать следующий вид"))
		} else {
			if tracker["minSampleDistanceM"] != nil {
				lines = append(lines, hudMuted("Разнообразие: не менее "+Number(tracker["minSampleDistanceM"])+" м"))
			}
			if tracker["nearestSampleDistanceM"] != nil {
				text := "До ближайшей пробы: " + Number(tracker["nearestSampleDistanceM"]) + " м"
				if tracker["nearestSampleMeetsMin"] == true {
					lines = append(lines, hudText(text+" · расстояние достигнуто"))
				} else {
					lines = append(lines, hudWarn(text+" · осталось "+Number(tracker["remainingDistanceM"])+" м"))
				}
			} else {
				lines = append(lines, hudMuted("Нет актуальной координаты для расчёта расстояния"))
			}
		}
		hudAdd(doc, "ГЕНЕТИЧЕСКИЕ ОБРАЗЦЫ", lines...)
	}
	predictions := []HUDLine{}
	for _, r := range v.Rows(body["predictions"]) {
		if r["tier"] == "unlikely" {
			continue
		}
		name := v.Text(r["name"], r["name_en"])
		if r["estimatedCredits"] != nil {
			name += " · " + Number(r["estimatedCredits"]) + " кр."
		}
		if r["tier"] != "confirmed" {
			name += " · проверить"
		}
		predictions = append(predictions, hudMuted(name))
		if len(predictions) >= 3 {
			break
		}
	}
	hudAdd(doc, "ВОЗМОЖНЫЕ ВИДЫ", predictions...)
	marks := []HUDLine{}
	for _, mark := range v.Rows(ex["surfaceMarks"]) {
		if mark["kind"] == "sample" {
			continue
		}
		text := v.Text(mark["label"], "Точка поверхности")
		if mark["distanceM"] != nil && mark["bearing"] != nil {
			text += " · " + Number(mark["distanceM"]) + " м · " + Number(mark["bearing"]) + "°"
		} else {
			continue
		}
		if mark["confidence"] != "recorded-position" {
			text += " · приблизительно"
		}
		marks = append(marks, hudMuted(text))
		if len(marks) >= 2 {
			break
		}
	}
	hudAdd(doc, "ТОЧКИ ПОВЕРХНОСТИ", marks...)
	sum := v.Obj(ex["summary"])
	if v.Num(sum["pendingSamples"]) > 0 {
		hudAdd(doc, "ДАННЫЕ VISTA GENOMICS", hudText("К продаже: "+Number(sum["pendingSamples"])+" · оценка "+Number(sum["pendingEstimatedCredits"])+" кр."))
	}
	if len(body) == 0 {
		targets := v.Rows(ex["targets"])
		if len(targets) > 0 {
			r := targets[0]
			hudAdd(doc, "СЛЕДУЮЩАЯ ПЛАНЕТА", hudMuted(strings.TrimSpace(v.Str(r["system"])+" · "+v.Str(r["name"]))))
		}
	}
}
