package ranks

import (
	"encoding/json"
	"strings"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestRankGuidesKeepFactsWithoutEmptyGoalCoaching(t *testing.T) {
	for _, language := range []string{"ru", "en"} {
		for _, discipline := range disciplines {
			guide := guideFor(discipline.ID, language)
			sections := v.Rows(guide["sections"])
			if len(sections) == 0 {
				t.Fatal("missing progression method", discipline.ID, language)
			}
			for _, section := range sections {
				if section["title"] == "" || len(v.Arr(section["paragraphs"])) == 0 {
					t.Fatal("empty rank section", section)
				}
			}
			body, _ := json.Marshal(guide)
			for _, filler := range []string{"Ранг как рабочая цель", "Практический следующий шаг", "Turn rank into a practical goal", "A practical next step", "ограниченный боевой сеанс", "bounded combat session", "сравните процент до и после", "compare progress before and after"} {
				if strings.Contains(string(body), filler) {
					t.Fatal("generic filler returned", discipline.ID, language, filler)
				}
			}
		}
	}
	checks := map[string][]string{
		"federation":   {"100%", "Federal Navy", "Sol", "Federal Corvette"},
		"empire":       {"100%", "Imperial Navy", "Imperial Courier", "Imperial Cutter"},
		"cqc":          {"Prestige", "50", "CD-43 11917", "20%", "30%"},
		"exobiologist": {"Vista Genomics"},
		"explore":      {"Universal Cartographics"},
	}
	for id, facts := range checks {
		for _, language := range []string{"ru", "en"} {
			body, _ := json.Marshal(guideFor(id, language))
			for _, fact := range facts {
				if !strings.Contains(string(body), fact) {
					t.Fatal("concrete game fact removed", id, language, fact)
				}
			}
		}
	}
}
