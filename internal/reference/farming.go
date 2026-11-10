package reference

import (
	"strings"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

var farmingFiles = []string{
	"engineering-farming-ship.json",
	"engineering-farming-guardian.json",
	"engineering-farming-odyssey.json",
	"engineering-farming-special.json",
}

func (s *Store) buildEngineeringFarming() {
	const group = "article-engineering-farming"
	const groupTitle = "Фарм материалов"
	s.groupNames[group] = groupTitle
	byFamily := map[string][]string{}
	for _, row := range v.Rows(data.Object("engineering-data.json")["materials"]) {
		if tradeable, _ := row["tradeable"].(bool); tradeable {
			family := "@" + v.Str(row["category"])
			byFamily[family] = append(byFamily[family], v.Str(row["key"]))
		}
	}
	odysseyKeys := map[string]bool{}
	for _, row := range data.Array("engineering-farming-odyssey.json") {
		for _, key := range stringArray(row["materials"]) {
			odysseyKeys[key] = true
		}
	}
	for _, row := range v.Rows(data.Object("engineering-roadmap.json")["odysseyMaterials"]) {
		key := v.Str(row["key"])
		if !odysseyKeys[key] {
			continue
		}
		byFamily["@odyssey"] = append(byFamily["@odyssey"], key)
		const odyssey = "material-odyssey"
		s.groupNames[odyssey] = "Материалы Odyssey"
		s.add(Entry{ID: "material/odyssey-" + key, Category: "materials", Group: odyssey,
			Title: v.Str(row["name"]), EnglishTitle: v.Str(row["english"]), Subtitle: "Odyssey"}, v.Str(row["symbol"]))
	}

	// Specific routes are shown before the general category or exchange guide.
	specific, general := map[string][]string{}, map[string][]string{}
	for _, file := range farmingFiles {
		for _, row := range data.Array(file) {
			id := "article/" + v.Str(row["key"])
			e := Entry{ID: id, Category: "articles", Group: group, Subtitle: groupTitle,
				Title: v.Str(row["title"]), EnglishTitle: v.Str(row["englishTitle"]),
				Description: v.Str(row["description"]), EnglishDescription: v.Str(row["englishDescription"])}
			for _, section := range v.Rows(row["sections"]) {
				part := Section{ID: v.Str(section["key"]), Title: v.Str(section["title"]), EnglishTitle: v.Str(section["englishTitle"]), Type: "route"}
				for _, step := range v.Rows(section["steps"]) {
					part.Items = append(part.Items, Item{
						Title: v.Str(step["title"]), EnglishTitle: v.Str(step["englishTitle"]),
						Text: v.Str(step["text"]), EnglishText: v.Str(step["englishText"]), System: v.Str(step["system"]),
					})
				}
				e.Sections = append(e.Sections, part)
			}
			keywords := append(stringArray(row["keywords"]), e.EnglishTitle, e.EnglishDescription)
			odyssey := file == "engineering-farming-odyssey.json"
			for _, material := range stringArray(row["materials"]) {
				if strings.HasPrefix(material, "@") {
					for _, key := range byFamily[material] {
						key = farmingMaterialID(key, odyssey)
						general[key] = append(general[key], id)
					}
					continue
				}
				key := farmingMaterialID(material, odyssey)
				specific[key] = append(specific[key], id)
				if r := s.records[key]; r != nil {
					keywords = append(keywords, r.entry.Title, r.entry.EnglishTitle, r.entry.Subtitle, material)
				}
			}
			r := s.add(e, keywords...)
			r.imageKey = v.Str(row["image"])
			if odyssey && id != "article/farm-odyssey" {
				r.relatedIDs = []string{"article/farm-odyssey"}
			}
			if file == "engineering-farming-guardian.json" {
				r.relatedIDs = []string{"article/farm-guardian", "article/farm-guardian-components", "article/farm-guardian-obelisks", "article/farm-guardian-blueprints"}
			}
		}
	}
	for id, r := range s.records {
		if r.entry.Category == "materials" {
			routes := unique(append(specific[id], general[id]...))
			if len(routes) > 0 {
				r.entry.Sections = append(r.entry.Sections, s.farmingLinks(routes))
			}
		}
		if r.entry.Category != "engineers" {
			continue
		}
		routes := []string{"article/farm-material-trading", "article/farm-raw-nearby", "article/farm-manufactured-hge", "article/farm-encoded-jameson"}
		if strings.HasPrefix(id, "engineer/odyssey-") {
			routes = []string{"article/farm-odyssey", "article/farm-odyssey-unlocks"}
		} else if id == "engineer/300110" {
			routes = append([]string{"article/farm-guardian", "article/farm-guardian-components", "article/farm-guardian-obelisks", "article/farm-guardian-blueprints", "article/farm-thargoid-titans"}, routes...)
		} else if id == "engineer/300220" || id == "engineer/300300" {
			routes = append([]string{"article/farm-thargoid-encounters"}, routes...)
		} else if id == "engineer/300010" {
			routes = append([]string{"article/farm-merc-coins"}, routes...)
		}
		r.entry.Sections = append(r.entry.Sections, s.farmingLinks(routes))
		if r.imageKey != "" {
			r.entry.Badges = append(r.entry.Badges, Badge{Label: "Авторская иллюстрация"})
		}
	}
}

func farmingMaterialID(key string, odyssey bool) string {
	if odyssey {
		return "material/odyssey-" + key
	}
	return "material/" + key
}

func (s *Store) farmingLinks(ids []string) Section {
	section := Section{ID: "farming", Title: "Где найти материалы", EnglishTitle: "Where to find materials", Type: "links"}
	for _, id := range ids {
		if r := s.records[id]; r != nil {
			section.Items = append(section.Items, Item{Title: r.entry.Title, EnglishTitle: r.entry.EnglishTitle, EntryID: id})
		}
	}
	return section
}
