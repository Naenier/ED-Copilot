package reference

import (
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestModuleArticlesAndCrossLinks(t *testing.T) {
	s := Catalog()
	seen := map[string]bool{}
	for _, raw := range v.Rows(data.Object("engineering-data.json")["modules"]) {
		key := v.Str(raw["key"])
		entry, found := s.Entry("module/" + key)
		if !found {
			t.Errorf("module %s has no article", key)
			continue
		}
		if seen[entry.ID] {
			continue
		}
		seen[entry.ID] = true
		t.Run(entry.ID, func(t *testing.T) {
			if entry.Title == "" || entry.EnglishTitle == "" || entry.Description == "" || entry.EnglishDescription == "" {
				t.Fatal("missing bilingual module title or description")
			}
			if len(entry.Sections) == 0 || entry.Sections[0].ID != "operation" || len(entry.Sections[0].Items) < 2 {
				t.Fatal("module has no practical notes")
			}
			noteLinks := 0
			for _, section := range entry.Sections {
				for _, item := range section.Items {
					if item.Title == "" || item.EnglishTitle == "" || item.Text == "" || item.EnglishText == "" {
						t.Fatalf("incomplete bilingual note in %s", section.ID)
					}
					if item.EntryID != "" {
						if _, ok := s.Entry(item.EntryID); !ok {
							t.Errorf("broken %s link %s", section.ID, item.EntryID)
						}
						if section.ID == "operation" {
							noteLinks++
						}
					}
				}
			}
			if noteLinks == 0 {
				t.Error("no article linked from practical notes")
			}
		})
	}
	if len(seen) != len(data.Object("module-reference.json")) {
		t.Errorf("module articles published: %d, authored: %d", len(seen), len(data.Object("module-reference.json")))
	}
	fsd, _ := s.Entry("module/fsd")
	for _, section := range fsd.Sections {
		for _, item := range section.Items {
			if item.EntryID == "engineer/300100" && strings.HasPrefix(item.EnglishText, "Up to G5") {
				return
			}
		}
	}
	t.Error("FSD article should link to Felicity Farseer with her maximum grade")
}
