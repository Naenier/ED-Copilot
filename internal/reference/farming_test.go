package reference

import (
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestFarmingCatalogueLinksAndRoutes(t *testing.T) {
	s := Catalog()
	for _, record := range s.ordered {
		entry := record.entry
		if entry.Category != "materials" && entry.Category != "engineers" {
			continue
		}
		linked := false
		for _, section := range entry.Sections {
			if section.ID != "farming" {
				continue
			}
			for _, item := range section.Items {
				target := s.records[item.EntryID]
				if target == nil || target.entry.Group != "article-engineering-farming" {
					t.Errorf("%s has invalid farming link %q", entry.ID, item.EntryID)
				}
				linked = true
			}
		}
		if !linked {
			t.Errorf("%s has no farming route", entry.ID)
		}
	}
	for _, file := range farmingFiles {
		for _, row := range data.Array(file) {
			id := "article/" + v.Str(row["key"])
			entry, ok := s.Entry(id)
			if !ok || entry.Title == "" || entry.EnglishTitle == "" || entry.EnglishDescription == "" || len(entry.Sections) < 2 || len(v.Rows(row["sources"])) == 0 {
				t.Errorf("incomplete guide %s", id)
			}
			if v.Str(row["image"]) != "" && entry.Image == nil {
				t.Errorf("guide %s is missing its diagram", id)
			}
			for _, section := range entry.Sections {
				if section.EnglishTitle == "" || len(section.Items) == 0 {
					t.Errorf("incomplete section %s/%s", id, section.ID)
				}
				for _, step := range section.Items {
					if step.Title == "" || step.EnglishTitle == "" || step.Text == "" || step.EnglishText == "" || strings.ContainsAny(step.System, "\r\n") {
						t.Errorf("incomplete step %s/%s/%s", id, section.ID, step.Title)
					}
				}
			}
		}
	}
}
