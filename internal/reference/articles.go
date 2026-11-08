package reference

import (
	"strings"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (s *Store) buildArticles() {
	catalogue := data.Object("encyclopedia.json")
	for _, section := range v.Rows(catalogue["sections"]) {
		s.groupNames["article-"+v.Str(section["id"])] = v.Str(section["name"])
	}
	for _, raw := range v.Rows(catalogue["articles"]) {
		e := Entry{ID: "article/" + v.Str(raw["id"]), Category: "articles", Group: "article-" + v.Str(raw["section"]), Title: v.Str(raw["title"]), Subtitle: s.groupNames["article-"+v.Str(raw["section"])], Description: v.Str(raw["summary"])}
		paragraphs := []string{}
		for _, body := range v.Arr(raw["body"]) {
			paragraphs = append(paragraphs, v.Str(body))
		}
		e.Sections = []Section{{ID: "mechanics", Title: "Как это работает", Type: "prose", Paragraphs: paragraphs}}
		if tips := stringItems(raw["tips"]); len(tips) > 0 {
			e.Sections = append(e.Sections, Section{ID: "practice", Title: "На практике", Type: "requirements", Items: tips})
		}
		r := s.add(e)
		for _, id := range v.Arr(raw["related"]) {
			r.relatedIDs = append(r.relatedIDs, "article/"+v.Str(id))
		}
		switch v.Str(raw["section"]) {
		case "engineering":
			r.relatedIDs = append(r.relatedIDs, "engineer/300100", "module/fsd", "material/iron")
		case "mining":
			r.relatedIDs = append(r.relatedIDs, "commodity/platinum", "commodity/painite", "module/pc", "module/rf")
		case "carrier":
			r.relatedIDs = append(r.relatedIDs, "commodity/tritium", "article/carrier-fuel")
		case "ships":
			r.relatedIDs = append(r.relatedIDs, "ship/sidewinder", "module/fsd", "module/pp", "module/sg")
		}
		if strings.Contains(v.Str(raw["id"]), "tritium") {
			r.relatedIDs = append(r.relatedIDs, "commodity/tritium")
		}
	}
}

func (s *Store) buildSystems() {
	type location struct{ base, engineer string }
	bySystem := map[string][]location{}
	for _, r := range s.records {
		if r.entry.Category != "engineers" {
			continue
		}
		name, base := "", ""
		for _, p := range r.entry.Properties {
			if p.Label == "Система" {
				name = p.Value
			}
			if p.Label == "База" {
				base = p.Value
			}
		}
		if name != "" {
			bySystem[name] = append(bySystem[name], location{base, r.entry.ID})
		}
	}
	for name, places := range bySystem {
		e := UnknownSystem(name)
		e.Group, e.Subtitle = "engineer-systems", "Система инженерной базы"
		e.Description = "В системе " + name + " расположена инженерная мастерская. Доступ к системе и доступ к самому инженеру проверяются отдельно."
		rows := [][]string{}
		links := [][]string{}
		for _, place := range places {
			rows = append(rows, []string{place.base, s.records[place.engineer].entry.Title})
			links = append(links, []string{"", place.engineer})
		}
		e.Properties = []Property{{Label: "Название", Value: name}}
		e.Actions = []Action{{Type: "copy-system", Label: "Копировать систему", System: name}}
		e.Sections = append([]Section{{ID: "destinations", Title: "Мастерские и точки назначения", Type: "table", Columns: []string{"База", "Инженер"}, Rows: rows, CellLinks: links}}, e.Sections...)
		r := s.add(e, name)
		for _, place := range places {
			r.relatedIDs = append(r.relatedIDs, place.engineer)
		}
	}
	s.groupNames["engineer-systems"] = "Системы инженеров"
}

func UnknownSystem(name string) Entry {
	name = trimRunes(strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, name), 180)
	return Entry{ID: SystemID(name), Category: "systems", Title: name, Subtitle: "Звёздная система", Description: "Карточка системы " + name + ". Орбитальные тела, кольца, станции и расстояния дополняются по фактическим игровым наблюдениям.", Sections: []Section{{ID: "planning", Title: "Перед перелётом", Type: "requirements", Items: []Item{{Title: "Разрешение на посещение", Text: "Некоторые системы закрыты разрешением. Наличие приглашения инженера не заменяет системное разрешение."}, {Title: "Цель внутри системы", Text: "После входа выберите точную базу на карте системы; межзвёздная дальность и путь до станции измеряются по-разному."}, {Title: "Посадка", Text: "Для планетарной базы нужны подходящий режим игры, возможность посадки и свободное место на площадке."}}}}, Related: []Summary{}}
}
