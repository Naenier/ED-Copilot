package reference

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (s *Store) buildEngineers(engineers, mods []v.M, bps map[string]v.M) {
	rawCatalog, _ := data.Load("engineering-roadmap.json")
	catalogue := v.Obj(rawCatalog)
	roadmap := map[string]v.M{}
	for _, row := range v.Rows(catalogue["engineers"]) {
		roadmap[v.Str(row["key"])] = row
	}
	personalRecipes := map[string][]Item{}
	for _, recipe := range v.Rows(catalogue["odysseyRecipes"]) {
		recipeID := odysseyBlueprintID(recipe)
		title, englishTitle := odysseyRecipeVariant(recipe)
		for _, key := range stringArray(recipe["engineerKeys"]) {
			personalRecipes[key] = append(personalRecipes[key], Item{Title: v.Str(recipe["name"]), EnglishTitle: v.Str(recipe["english"]), Text: title, EnglishText: englishTitle, EntryID: recipeID})
		}
	}
	facts := map[string]v.M{}
	for _, f := range objectRows("encyclopedia-engineers.json") {
		facts[v.Str(f["key"])] = f
	}
	for _, raw := range engineers {
		key := v.Str(raw["key"])
		f := facts[key]
		verified := roadmap[key]
		system, base := v.Text(verified["system"], raw["system"]), v.Text(verified["base"], raw["base"])
		group := v.Text(f["group"], "engineer-bubble")
		english := v.Str(raw["english"])
		type capability struct{ row, links []string }
		capabilities := []capability{}
		related := []string{SystemID(system), "article/engineer-access", "article/pinned-blueprints"}
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
			Properties: []Property{{Label: "Система", Value: system, EntryID: SystemID(system)}, {Label: "База", Value: base}, {Label: "Специализация", Value: "Корабельные модули"}}, Badges: []Badge{{Label: "Корабельный инженер"}}, Actions: engineerActions(key, system),
			Sections: []Section{{ID: "unlock", Title: "Как получить доступ", Type: "requirements", Items: engineerRequirements(f, verified)},
				{ID: "modifications", Title: "Чертежи и максимальные классы", Type: "table", Columns: []string{"Модуль", "Чертёж", "Максимум"}, Rows: rows, CellLinks: links}}}
		reputation := Section{ID: "workshop", Title: "Как повысить доступ", EnglishTitle: "Raising workshop access", Type: "prose"}
		for _, method := range v.Rows(verified["reputation"]) {
			reputation.Items = append(reputation.Items, Item{Title: v.Str(method["text"]), EnglishTitle: v.Str(method["english"])})
		}
		if len(reputation.Items) != 0 {
			e.Sections = append(e.Sections, reputation)
		}
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
		verified := roadmap[key]
		system, base := v.Text(verified["system"], f["system"]), v.Text(verified["base"], f["base"])
		modifications := personalRecipes[key]
		if len(modifications) == 0 {
			modifications = stringItems(f["modifications"])
		}
		e := Entry{ID: "engineer/" + key, Category: "engineers", Group: v.Text(f["group"], "engineer-odyssey"), Title: v.Str(f["name"]), Subtitle: v.Str(f["english"]), Description: v.Str(f["description"]),
			Properties: []Property{{Label: "Система", Value: system, EntryID: SystemID(system)}, {Label: "База", Value: base}, {Label: "Специализация", Value: "Костюмы и личное оружие"}}, Badges: []Badge{{Label: "Odyssey"}}, Actions: engineerActions(key, system),
			Sections: []Section{{ID: "unlock", Title: "Открытие и рекомендация", Type: "requirements", Items: engineerRequirements(f, verified)}, {ID: "modifications", Title: "Модификации личного снаряжения", Type: "requirements", Items: modifications}}}
		r := s.add(e)
		if image := v.Str(f["image"]); image != "" {
			r.imageKey = "engineer-" + image
		}
		r.relatedIDs = append([]string{SystemID(system), "article/suits", "article/onfoot-weapons"}, stringArray(f["related"])...)
	}
	s.groupNames["engineer-bubble"] = "Корабельные инженеры · Обитаемый пузырь"
	s.groupNames["engineer-colonia"] = "Корабельные инженеры · Колония"
	s.groupNames["engineer-odyssey"] = "Личное снаряжение · Обитаемый пузырь"
	s.groupNames["engineer-odyssey-colonia"] = "Личное снаряжение · Колония"
}

func engineerActions(key, system string) []Action {
	return []Action{
		{Type: "copy-system", Label: "Копировать систему", System: system},
		{Type: "engineering-roadmap", Label: "Открыть план прокачки", EngineerKey: key},
	}
}

func engineerRequirements(f, verified v.M) []Item {
	out := []Item{}
	if verified != nil {
		labels := map[string]string{"referral": "Узнать об инженере", "invitation": "Получить приглашение", "permit": "Получить разрешение", "unlock": "Открыть мастерскую"}
		for _, requirement := range v.Rows(verified["requirements"]) {
			if text := v.Str(requirement["text"]); text != "" {
				item := Item{Title: v.Text(labels[v.Str(requirement["kind"])], "Условия доступа"), Text: text, EnglishText: v.Str(requirement["english"])}
				if key := v.Str(requirement["engineerKey"]); key != "" {
					item.EntryID = "engineer/" + key
				}
				out = append(out, item)
			}
		}
		return out
	}
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
