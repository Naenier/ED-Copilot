package presentation

import (
	"sort"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

var presentationMaterialNames = func() map[string]string {
	out := map[string]string{}
	for _, row := range v.Rows(data.Object("engineering-data.json")["materials"]) {
		out[v.Str(row["key"])] = v.Str(row["name"])
	}
	return out
}()

func materialName(key any) string {
	if name := presentationMaterialNames[v.Str(key)]; name != "" {
		return name
	}
	return v.Str(key)
}

func materialView(mode string) (category string, planOnly, ok bool) {
	planOnly = strings.Contains(mode, "плана")
	switch EngineeringMode(mode) {
	case EngineeringMaterials, "Материалы плана":
		return "", planOnly, true
	case "Сырьё", "Сырьё плана":
		return "raw", planOnly, true
	case "Промышленные материалы", "Промышленные материалы плана":
		return "manufactured", planOnly, true
	case "Данные", "Данные плана":
		return "encoded", planOnly, true
	}
	return "", false, false
}
func referenceSources(refs []M) string {
	lines := []string{}
	for _, ref := range refs {
		if url := v.Str(ref["url"]); url != "" {
			lines = append(lines, v.Text(ref["title"], ref["name"], "Источник")+": "+url)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "Источники:\r\n" + strings.Join(lines, "\r\n")
}
func referenceData(raw M, refs []M) M {
	out := v.Copy(raw)
	out["sources"] = v.Clone(refs)
	if v.Str(out["url"]) == "" {
		for _, ref := range refs {
			if url := v.Str(ref["url"]); url != "" {
				out["url"] = url
				break
			}
		}
	}
	return out
}
func shipReferenceDetails(ship M) string {
	details := fields(ship, "symbol", "Код модели в журнале", "manufacturer", "Производитель", "pad", "Посадочная площадка", "hullMass", "Масса корпуса, т", "crew", "Мест экипажа", "utilitySlots", "Внешних подвесок", "description", "Описание", "verifiedAt", "Дата справочника")
	hardpoints := []string{}
	for _, mount := range v.Rows(ship["hardpoints"]) {
		hardpoints = append(hardpoints, Number(mount["count"])+" × класс "+Number(mount["grade"]))
	}
	if len(hardpoints) > 0 {
		details += "\r\nОрудийные подвески: " + strings.Join(hardpoints, "; ")
	}
	slots := []string{}
	for _, slot := range v.Arr(ship["optionalSlots"]) {
		slots = append(slots, Number(slot))
	}
	if len(slots) > 0 {
		details += "\r\nКлассы внутренних отсеков: " + strings.Join(slots, ", ")
	}
	if ship["specsKnown"] == false {
		details += "\r\nПолные характеристики этой модели в локальном каталоге пока неизвестны."
	}
	if source := v.Str(ship["sourceUrl"]); source != "" {
		details += "\r\nИсточник: " + source
	}
	return details
}
func engineeringReferenceDetails(kind string, row, catalog M) string {
	out := fields(row, "english", "Название", "description", "Описание", "category", "Категория", "grade", "Уровень материала", "capacity", "Вместимость", "tradeable", "Обменивается у торговца", "system", "Система", "base", "База", "url", "Руководство")
	switch kind {
	case "blueprints":
		grades := v.Obj(row["grades"])
		keys := make([]string, 0, len(grades))
		for key := range grades {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, grade := range keys {
			out += "\r\n\r\nРецепт G" + grade + ":\r\n" + recipeDetails(grades[grade])
		}
	case "experimentals":
		out += "\r\n\r\nРецепт экспериментального эффекта:\r\n" + recipeDetails(row["materials"])
	case "modules":
		blueprints, effects, engineers := map[string]string{}, map[string]string{}, map[string]string{}
		for _, r := range v.Rows(catalog["blueprints"]) {
			blueprints[v.Str(r["key"])] = v.Str(r["name"])
		}
		for _, r := range v.Rows(catalog["experimentals"]) {
			effects[v.Str(r["key"])] = v.Str(r["name"])
		}
		for _, r := range v.Rows(catalog["engineers"]) {
			engineers[v.Str(r["key"])] = v.Str(r["name"])
		}
		for _, id := range v.Arr(row["blueprints"]) {
			key := v.Str(id)
			out += "\r\nЧертёж: " + v.Text(blueprints[key], key)
			grades := v.Obj(v.Obj(row["engineers"])[key])
			keys := make([]string, 0, len(grades))
			for g := range grades {
				keys = append(keys, g)
			}
			sort.Strings(keys)
			for _, grade := range keys {
				names := []string{}
				for _, engineer := range v.Arr(grades[grade]) {
					names = append(names, v.Text(engineers[v.Str(engineer)], engineer))
				}
				out += "\r\nG" + grade + ": " + strings.Join(names, ", ")
			}
		}
		effectNames := []string{}
		for _, effect := range v.Arr(row["experimentals"]) {
			effectNames = append(effectNames, v.Text(effects[v.Str(effect)], effect))
		}
		if len(effectNames) > 0 {
			out += "\r\nЭкспериментальные эффекты: " + strings.Join(effectNames, ", ")
		}
	}
	return out + "\r\n\r\n" + referenceSources(v.Rows(catalog["sources"]))
}
