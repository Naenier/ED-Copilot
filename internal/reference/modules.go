package reference

import (
	"fmt"
	"sort"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (s *Store) buildModules(modules, engineers []v.M, blueprints, effects map[string]v.M) {
	facts := data.Object("module-reference.json")
	byFamily := make(map[string][]v.M)
	for _, module := range modules {
		key := v.Str(module["key"])
		family := canonicalModule(key)
		byFamily[family] = append(byFamily[family], module)
		s.aliases["module/"+key] = "module/" + family
	}
	for family, variants := range byFamily {
		fact := v.Obj(facts[family])
		entry := Entry{
			ID:                 "module/" + family,
			Category:           "modules",
			Group:              moduleGroup(family),
			Description:        v.Str(fact["description"]),
			EnglishDescription: v.Str(fact["englishDescription"]),
		}
		keywords := []string{family, entry.EnglishDescription}
		for _, variant := range variants {
			name, english := v.Str(variant["name"]), v.Str(variant["english"])
			keywords = append(keywords, name, english)
			if v.Str(variant["key"]) == family {
				entry.Title, entry.EnglishTitle = name, english
			}
		}
		if family == "csl" {
			entry.Title = "Пусковая установка каустических ловушек"
		}
		notes := []Item{}
		for _, note := range v.Rows(fact["notes"]) {
			notes = append(notes, Item{
				Title: v.Str(note["title"]), EnglishTitle: v.Str(note["englishTitle"]),
				Text: v.Str(note["text"]), EnglishText: v.Str(note["englishText"]), EntryID: v.Str(note["entryId"]),
			})
		}
		if len(notes) > 0 {
			entry.Sections = append(entry.Sections, Section{
				ID: "operation", Title: "Применение", EnglishTitle: "Practical use", Type: "prose", Items: notes,
			})
		}
		if items := moduleEngineerLinks(variants, engineers); len(items) > 0 {
			entry.Sections = append(entry.Sections, Section{
				ID: "engineers", Title: "Где улучшить модуль", EnglishTitle: "Where to engineer it", Type: "prose", Items: items,
			})
		}
		record := s.add(entry, keywords...)
		for _, variant := range variants {
			for _, key := range v.Arr(variant["blueprints"]) {
				if bp := blueprints[v.Str(key)]; bp != nil {
					record.relatedIDs = append(record.relatedIDs, blueprintID(bp))
				}
			}
			for _, key := range v.Arr(variant["experimentals"]) {
				if effect := effects[v.Str(key)]; effect != nil {
					record.relatedIDs = append(record.relatedIDs, experimentalID(effect))
				}
			}
		}
	}
}

func moduleEngineerLinks(variants, engineers []v.M) []Item {
	maxGrade := make(map[string]int)
	for _, variant := range variants {
		for _, recipe := range v.Obj(variant["engineers"]) {
			for grade, names := range v.Obj(recipe) {
				for _, name := range v.Arr(names) {
					name := v.Str(name)
					maxGrade[name] = max(maxGrade[name], v.Int(grade))
				}
			}
		}
	}
	type engineerLink struct {
		grade int
		item  Item
	}
	var links []engineerLink
	for _, engineer := range engineers {
		name := v.Str(engineer["english"])
		if grade := maxGrade[name]; grade > 0 {
			links = append(links, engineerLink{grade: grade, item: Item{
				Title: v.Text(engineer["name"], name), EnglishTitle: name,
				Text:        fmt.Sprintf("До G%d · %s", grade, v.Str(engineer["system"])),
				EnglishText: fmt.Sprintf("Up to G%d · %s", grade, v.Str(engineer["system"])),
				EntryID:     "engineer/" + v.Str(engineer["key"]),
			}})
		}
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i].grade != links[j].grade {
			return links[i].grade > links[j].grade
		}
		return links[i].item.EnglishTitle < links[j].item.EnglishTitle
	})
	items := make([]Item, 0, len(links))
	for _, link := range links {
		items = append(items, link.item)
	}
	return items
}
