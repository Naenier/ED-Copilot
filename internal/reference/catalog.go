package reference

import (
	"net/url"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/Naenier/ED-Copilot/internal/data"
	"github.com/Naenier/ED-Copilot/internal/localization"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type record struct {
	entry      Entry
	relatedIDs []string
	search     string
	imageKey   string
}

// Store owns immutable catalogue records. Callers receive independent DTOs.
type Store struct {
	records    map[string]*record
	ordered    []*record
	categories []Category
	groupNames map[string]string
	aliases    map[string]string
	media      sync.Map
}

var once sync.Once
var shared *Store

func Catalog() *Store {
	once.Do(func() {
		s := &Store{records: map[string]*record{}, groupNames: map[string]string{}, aliases: map[string]string{}}
		s.build()
		shared = s
	})
	return shared
}

func (s *Store) List(q Query) ListResult {
	if q.Category == "" {
		q.Category = "articles"
	}
	if q.Limit <= 0 {
		q.Limit = 24
	}
	q.Limit = min(q.Limit, 48)
	q.Offset = max(0, q.Offset)
	terms := strings.Fields(normalize(trimRunes(q.Query, 160)))
	groups := map[string]int{}
	matched := []*record{}
	for _, r := range s.ordered {
		if q.Category != "all" && r.entry.Category != q.Category {
			continue
		}
		found := true
		for _, term := range terms {
			if !strings.Contains(r.search, term) {
				found = false
				break
			}
		}
		if !found {
			continue
		}
		groups[r.entry.Group]++
		if q.Group == "" || q.Group == "all" || r.entry.Group == q.Group {
			matched = append(matched, r)
		}
	}
	out := ListResult{Categories: append([]Category{}, s.categories...), Groups: []Group{}, Entries: []Summary{}, Total: len(matched), Offset: min(q.Offset, len(matched)), Limit: q.Limit}
	for id, count := range groups {
		if id != "" {
			out.Groups = append(out.Groups, Group{ID: id, Title: v.Text(s.groupNames[id], id), Count: count})
		}
	}
	sort.Slice(out.Groups, func(i, j int) bool { return out.Groups[i].Title < out.Groups[j].Title })
	for _, r := range matched[out.Offset:min(out.Offset+q.Limit, len(matched))] {
		out.Entries = append(out.Entries, s.summary(r, true))
	}
	return out
}

func (s *Store) Entry(id string) (Entry, bool) {
	if decoded, err := url.PathUnescape(id); err == nil {
		id = decoded
	}
	if alias := s.aliases[id]; alias != "" {
		id = alias
	}
	if alias := s.aliases[aliasKey(id)]; alias != "" {
		id = alias
	}
	r, ok := s.records[id]
	if !ok {
		return Entry{}, false
	}
	out := cloneEntry(r.entry)
	out.Image = s.image(r.imageKey, false, out.Title)
	out.Related = []Summary{}
	seen := map[string]bool{id: true}
	for _, related := range r.relatedIDs {
		if other := s.records[related]; other != nil && !seen[related] {
			out.Related = append(out.Related, s.summary(other, false))
			seen[related] = true
		}
	}
	return out, true
}

func (s *Store) summary(r *record, withImage bool) Summary {
	e := r.entry
	out := Summary{ID: e.ID, Category: e.Category, Title: e.Title, Subtitle: e.Subtitle, Description: trimRunes(e.Description, 180), EnglishTitle: e.EnglishTitle, EnglishDescription: trimRunes(e.EnglishDescription, 180), Group: e.Group, Badges: append([]Badge{}, e.Badges...), CommodityKey: e.CommodityKey}
	if len(e.Properties) > 0 {
		out.Meta = append([]Property{}, e.Properties[:min(2, len(e.Properties))]...)
	}
	if withImage {
		out.Image = s.image(r.imageKey, true, e.Title)
	}
	return out
}

func (s *Store) add(e Entry, keywords ...string) *record {
	if e.Sections == nil {
		e.Sections = []Section{}
	}
	if e.Related == nil {
		e.Related = []Summary{}
	}
	r := &record{entry: e, search: normalize(strings.Join(append([]string{e.ID, e.Title, e.Subtitle, e.Description, localization.Text("en", e.Title), localization.Text("en", e.Subtitle)}, keywords...), " "))}
	s.records[e.ID] = r
	return r
}

func (s *Store) build() {
	s.categories = []Category{{ID: "articles", Title: "Игровые механики", Icon: "book-open"}, {ID: "ships", Title: "Корабли", Icon: "ship"}, {ID: "engineers", Title: "Инженеры", Icon: "wrench"}, {ID: "modules", Title: "Модули", Icon: "boxes"}, {ID: "materials", Title: "Материалы", Icon: "atom"}, {ID: "blueprints", Title: "Чертежи", Icon: "drafting-compass"}, {ID: "experimentals", Title: "Экспериментальные эффекты", Icon: "flask-conical"}, {ID: "commodities", Title: "Товары", Icon: "package"}, {ID: "systems", Title: "Звёздные системы", Icon: "orbit"}}
	s.buildArticles()
	s.buildShips()
	s.buildEngineering()
	s.buildEngineeringFarming()
	s.buildOdysseyRecipes()
	s.linkMaterialRecipes()
	s.buildCommodities()
	s.buildSystems()
	normalizedAliases := map[string]string{}
	for alias, canonical := range s.aliases {
		normalizedAliases[aliasKey(alias)] = canonical
	}
	for alias, canonical := range normalizedAliases {
		s.aliases[alias] = canonical
	}
	counts := map[string]int{}
	for _, r := range s.records {
		prefix := strings.SplitN(r.entry.ID, "/", 2)[0] + "/"
		s.aliases[aliasKey(r.entry.ID)] = r.entry.ID
		s.aliases[aliasKey(prefix+r.entry.Title)] = r.entry.ID
		if r.entry.Category == "engineers" || r.entry.Category == "materials" || r.entry.Category == "commodities" {
			s.aliases[aliasKey(prefix+r.entry.Subtitle)] = r.entry.ID
		}
		if r.entry.Category == "blueprints" || r.entry.Category == "experimentals" {
			sort.Slice(r.entry.Sections, func(i, j int) bool { return r.entry.Sections[i].ID < r.entry.Sections[j].ID })
		}
		s.ordered = append(s.ordered, r)
		counts[r.entry.Category]++
	}
	for i := range s.categories {
		s.categories[i].Count = counts[s.categories[i].ID]
	}
	sort.Slice(s.ordered, func(i, j int) bool {
		a, b := s.ordered[i].entry, s.ordered[j].entry
		if a.Category == b.Category {
			return normalize(a.Title) < normalize(b.Title)
		}
		return a.Category < b.Category
	})
}

func aliasKey(s string) string {
	parts := strings.SplitN(normalize(strings.TrimSpace(s)), "/", 2)
	if len(parts) != 2 {
		return s
	}
	return parts[0] + "/" + strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, parts[1])
}

func normalize(s string) string { return strings.ReplaceAll(strings.ToLower(s), "ё", "е") }
func trimRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return string(r)
}
func slug(s string) string {
	var out strings.Builder
	dash := false
	for _, r := range normalize(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
			dash = false
		} else if out.Len() > 0 && !dash {
			out.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(out.String(), "-")
}

func SystemID(name string) string { return "system/" + slug(name) }

func stringItems(values any) []Item {
	out := []Item{}
	for _, raw := range v.Arr(values) {
		if text := v.Str(raw); text != "" {
			out = append(out, Item{Title: text})
		}
	}
	return out
}

func cloneEntry(in Entry) Entry {
	out := in
	out.Badges = append([]Badge{}, in.Badges...)
	out.Properties = append([]Property{}, in.Properties...)
	out.Actions = append([]Action{}, in.Actions...)
	out.Sections = append([]Section{}, in.Sections...)
	for i, section := range in.Sections {
		out.Sections[i].Paragraphs = append([]string{}, section.Paragraphs...)
		out.Sections[i].Items = append([]Item{}, section.Items...)
		out.Sections[i].Columns = append([]string{}, section.Columns...)
		out.Sections[i].Rows = make([][]string, len(section.Rows))
		for j, row := range section.Rows {
			out.Sections[i].Rows[j] = append([]string{}, row...)
		}
		out.Sections[i].CellLinks = make([][]string, len(section.CellLinks))
		for j, row := range section.CellLinks {
			out.Sections[i].CellLinks[j] = append([]string{}, row...)
		}
		out.Sections[i].Grades = append([]Grade{}, section.Grades...)
		for j, grade := range section.Grades {
			out.Sections[i].Grades[j].Materials = append([]Ingredient{}, grade.Materials...)
			out.Sections[i].Grades[j].Effects = append([]Property{}, grade.Effects...)
		}
	}
	return out
}

func objectRows(name string) []v.M { return data.Array(name) }
