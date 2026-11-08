package reference

import (
	"fmt"
	"sort"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (s *Store) buildEngineers(engineers, mods []v.M, bps map[string]v.M) {
	facts := map[string]v.M{}
	for _, f := range objectRows("encyclopedia-engineers.json") {
		facts[v.Str(f["key"])] = f
	}
	for _, raw := range engineers {
		key := v.Str(raw["key"])
		f := facts[key]
		group := v.Text(f["group"], "engineer-bubble")
		english := v.Str(raw["english"])
		type capability struct{ row, links []string }
		capabilities := []capability{}
		related := []string{SystemID(v.Str(raw["system"])), "article/engineer-access", "article/pinned-blueprints"}
		for _, module := range mods {
			for bpKey, grades := range v.Obj(module["engineers"]) {
				maxGrade := 0
				for grade, names := range v.Obj(grades) {
					if hasKey(names, english) {
						maxGrade = max(maxGrade, v.Int(grade))
					}
				}
				if maxGrade == 0 {
					continue
				}
				bp := bps[bpKey]
				moduleID := "module/" + canonicalModule(v.Str(module["key"]))
				moduleName := v.Str(module["name"])
				if v.Str(module["key"]) == "csl" {
					moduleName = s.records[moduleID].entry.Title
				}
				capabilities = append(capabilities, capability{[]string{moduleName, v.Str(bp["name"]), fmt.Sprintf("G%d", maxGrade)}, []string{moduleID, blueprintID(bp), ""}})
				related = append(related, "module/"+canonicalModule(v.Str(module["key"])), blueprintID(bp))
			}
		}
		sort.Slice(capabilities, func(i, j int) bool {
			return strings.Join(capabilities[i].row, " ") < strings.Join(capabilities[j].row, " ")
		})
		rows, links := [][]string{}, [][]string{}
		for _, capability := range capabilities {
			rows = append(rows, capability.row)
			links = append(links, capability.links)
		}
		modules := []string{}
		for _, row := range rows {
			modules = append(modules, row[0])
		}
		description := v.Text(f["description"], "Корабельный инженер. Модифицирует оборудование на своей базе; доступные направления и максимальные классы приведены в таблице ниже.")
		e := Entry{ID: "engineer/" + key, Category: "engineers", Group: group, Title: v.Str(raw["name"]), Subtitle: english, Description: description,
			Properties: []Property{{Label: "Система", Value: v.Str(raw["system"]), EntryID: SystemID(v.Str(raw["system"]))}, {Label: "База", Value: v.Str(raw["base"])}, {Label: "Специализация", Value: "Корабельные модули"}}, Badges: []Badge{{Label: "Корабельный инженер"}}, Actions: []Action{{Type: "copy-system", Label: "Копировать систему", System: v.Str(raw["system"])}},
			Sections: []Section{{ID: "unlock", Title: "Как получить доступ", Type: "requirements", Items: engineerRequirements(f)},
				{ID: "modifications", Title: "Чертежи и максимальные классы", Type: "table", Columns: []string{"Модуль", "Чертёж", "Максимум"}, Rows: rows, CellLinks: links},
				{ID: "workshop", Title: "Работа с инженером", Type: "requirements", Items: []Item{{Title: "Репутация", Text: "Выполняйте модификации у инженера, чтобы открывать более высокие классы. Материалы за каждый ролл списываются отдельно."}, {Title: "Закреплённый чертёж", Text: "Можно закрепить один чертёж этого инженера для удалённой мастерской. Удалённая модификация не повышает репутацию у инженера."}, {Title: "Экспериментальный эффект", Text: "Для установки эффекта посетите совместимого инженера лично. Эффект требует отдельного рецепта материалов."}}}}}
		r := s.add(e, strings.Join(unique(modules), " "))
		r.imageKey = "engineer-" + engineerImages[key]
		r.relatedIDs = append(related, stringArray(f["related"])...)
	}
	// Odyssey engineers modify personal equipment, not ship modules. Their
	// invitation/referral steps must remain separate from a ship engineer's rank.
	for key, f := range facts {
		if !strings.HasPrefix(key, "odyssey-") {
			continue
		}
		e := Entry{ID: "engineer/" + key, Category: "engineers", Group: v.Text(f["group"], "engineer-odyssey"), Title: v.Str(f["name"]), Subtitle: v.Str(f["english"]), Description: v.Str(f["description"]),
			Properties: []Property{{Label: "Система", Value: v.Str(f["system"]), EntryID: SystemID(v.Str(f["system"]))}, {Label: "База", Value: v.Str(f["base"])}, {Label: "Специализация", Value: "Костюмы и личное оружие"}}, Badges: []Badge{{Label: "Odyssey"}}, Actions: []Action{{Type: "copy-system", Label: "Копировать систему", System: v.Str(f["system"])}},
			Sections: []Section{{ID: "unlock", Title: "Открытие и рекомендация", Type: "requirements", Items: engineerRequirements(f)}, {ID: "modifications", Title: "Модификации личного снаряжения", Type: "requirements", Items: stringItems(f["modifications"])}, {ID: "workshop", Title: "Перед установкой", Type: "prose", Paragraphs: []string{"Модификации занимают открытые слоты костюма или оружия. Повышение класса снаряжения и установка модификаций — разные операции.", "Планируйте набор модификаций заранее: установленную модификацию личного снаряжения нельзя просто снять и заменить. Условия рекомендации следующему инженеру выполняются отдельно от открытия услуг текущего."}}}}
		r := s.add(e)
		if image := v.Str(f["image"]); image != "" {
			r.imageKey = "engineer-" + image
		}
		r.relatedIDs = append([]string{SystemID(v.Str(f["system"])), "article/suits", "article/onfoot-weapons"}, stringArray(f["related"])...)
	}
	s.groupNames["engineer-bubble"] = "Корабельные инженеры · Обитаемый пузырь"
	s.groupNames["engineer-colonia"] = "Корабельные инженеры · Колония"
	s.groupNames["engineer-odyssey"] = "Личное снаряжение · Обитаемый пузырь"
	s.groupNames["engineer-odyssey-colonia"] = "Личное снаряжение · Колония"
}

func engineerRequirements(f v.M) []Item {
	out := []Item{}
	for _, row := range []struct{ key, title string }{{"discovery", "Узнать об инженере"}, {"invitation", "Получить приглашение"}, {"donation", "Открыть мастерскую"}, {"referral", "Получить рекомендацию"}} {
		if text := v.Str(f[row.key]); text != "" {
			out = append(out, Item{Title: row.title, Text: text})
		}
	}
	return out
}

func stringArray(raw any) []string {
	out := []string{}
	for _, value := range v.Arr(raw) {
		if text := v.Str(value); text != "" {
			out = append(out, text)
		}
	}
	return out
}
