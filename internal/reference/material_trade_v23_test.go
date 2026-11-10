package reference

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestShipMaterialArticlesHaveDirectRoutesAndValidExchangeLinks(t *testing.T) {
	s := Catalog()
	rows := v.Rows(data.Object("engineering-data.json")["materials"])
	byID := map[string]v.M{}
	for _, row := range rows {
		byID["material/"+v.Str(row["key"])] = row
	}
	for _, row := range rows {
		key := v.Str(row["key"])
		t.Run(key, func(t *testing.T) {
			e, found := s.Entry("material/" + key)
			if !found {
				t.Fatal("missing material")
			}
			requireMaterialCollection(t, e)

			trade := materialSection(e, "trade")
			if !v.Bool(row["tradeable"]) {
				if len(trade.Items) != 0 {
					t.Fatal("ordinary trader advice on a special material")
				}
				return
			}
			grade := v.Int(row["grade"])
			top := 5
			if row["category"] == "raw" {
				top = 4
			}
			if grade == top && len(trade.Items) == 0 {
				return
			}
			if len(trade.Items) != 1 || trade.Type != "links" {
				t.Fatalf("expected a single trade link, got %+v", trade)
			}
			item := trade.Items[0]
			target := byID[item.EntryID]
			if _, ok := s.Entry(item.EntryID); !ok || !v.Bool(target["tradeable"]) || target["category"] != row["category"] {
				t.Fatalf("invalid exchange target %q", item.EntryID)
			}
			cost, gain := 6, 1
			if grade < top {
				if target["family"] != row["family"] || v.Int(target["grade"]) != top {
					t.Fatalf("expected grade %d in the same family, got %q", top, item.EntryID)
				}
				cost = 1
				for level := grade; level < top; level++ {
					gain *= 3
				}
			}
			if !strings.Contains(item.EnglishText, fmt.Sprintf("Trade %d for %d ", cost, gain)) || !strings.Contains(item.Text, fmt.Sprintf("Отдайте %d и получите %d ", cost, gain)) {
				t.Fatalf("incorrect exchange ratio %+v", item)
			}
		})
	}
}

func TestMaterialRoutesNameTheSpecificSource(t *testing.T) {
	s := Catalog()
	cases := []struct {
		id, system, clue string
	}{
		{"chromium", "Deciat", "9.86%"},
		{"antimony", "HIP 29560", "−47.1081"},
		{"selenium", "HR 3230", "Polyporous"},
		{"imperialshielding", "", "Election"},
		{"pharmaceuticalisolators", "", "Outbreak"},
		{"improvisedcomponents", "", "Civil Unrest"},
		{"biotechconductors", "Robigo", "Sothis"},
		{"electrochemicalarrays", "", "Degraded Emissions"},
		{"adaptiveencryptors", "HIP 12099", "wreck"},
		{"disruptedwakeechoes", "", "Wake Scanner"},
		{"guardianmoduleblueprint", "Synuefe NL-N C23-4", "pylons"},
		{"unknownenergysource", "Solati", "25"},
		{"odyssey-chemicalpatents", "", "LAB"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			e, _ := s.Entry("material/" + tc.id)
			items := materialSection(e, "collection").Items
			systemFound := tc.system == ""
			clueFound := false
			for _, item := range items {
				systemFound = systemFound || strings.EqualFold(item.System, tc.system)
				clueFound = clueFound || strings.Contains(item.EnglishTitle+" "+item.EnglishText, tc.clue)
			}
			if !systemFound || !clueFound {
				t.Fatalf("wanted system %q and clue %q, got %+v", tc.system, tc.clue, items)
			}
		})
	}
}

func TestOdysseyMaterialArticlesHaveDirectRoutesAndAssetBarter(t *testing.T) {
	s := Catalog()
	count := 0
	for id, rec := range s.records {
		if !strings.HasPrefix(id, "material/odyssey-") {
			continue
		}
		count++
		t.Run(id, func(t *testing.T) {
			e := rec.entry
			requireMaterialCollection(t, e)
			for _, item := range materialSection(e, "trade").Items {
				if !strings.HasPrefix(item.EntryID, "material/odyssey-") || item.EntryID == id {
					t.Fatalf("invalid Odyssey barter link %q", item.EntryID)
				}
				if _, ok := s.Entry(item.EntryID); !ok {
					t.Fatalf("missing Odyssey barter target %q", item.EntryID)
				}
			}
		})
	}
	if count != 98 {
		t.Fatalf("expected acquisition coverage for 98 Odyssey articles, got %d", count)
	}
	e, _ := s.Entry("material/odyssey-aerogel")
	trade := materialSection(e, "trade")
	if len(trade.Items) != 1 || trade.Items[0].EntryID != "material/odyssey-graphene" || !strings.Contains(trade.Items[0].EnglishText, "13 points") {
		t.Fatalf("incorrect chemical barter route %+v", trade)
	}
	e, _ = s.Entry("material/odyssey-medicaltrialrecords")
	if len(materialSection(e, "trade").Items) != 0 {
		t.Fatal("Odyssey data cannot be bartered at a concourse")
	}
}

func TestMaterialEngineerUnlockLinks(t *testing.T) {
	s := Catalog()
	cases := []struct{ material, engineer, clue string }{
		{"scandatabanks", "300110", "50"},
		{"unknownenergysource", "300220", "25"},
		{"unknownenergysource", "300300", "25"},
		{"odyssey-catmedia", "odyssey-wellington-beck", "15"},
		{"odyssey-factionassociates", "odyssey-yi-shen", "Baltanos"},
	}
	for _, tc := range cases {
		e, _ := s.Entry("material/" + tc.material)
		id := "engineer/" + tc.engineer
		found := false
		for _, item := range materialSection(e, "engineer-unlocks").Items {
			if item.EntryID == id && strings.Contains(item.EnglishText, tc.clue) {
				found = true
			}
		}
		if _, ok := s.Entry(id); !found || !ok {
			t.Fatalf("missing link from %s to %s with requirement %q", tc.material, id, tc.clue)
		}
	}
}

func materialSection(e Entry, id string) Section {
	for _, section := range e.Sections {
		if section.ID == id {
			return section
		}
	}
	return Section{}
}

func requireMaterialCollection(t *testing.T, e Entry) {
	t.Helper()
	section := materialSection(e, "collection")
	if section.Type != "route" || len(section.Items) == 0 {
		t.Fatalf("missing direct acquisition route for %s", e.ID)
	}
	for _, item := range section.Items {
		if item.Title == "" || item.Text == "" || item.EnglishTitle == "" || item.EnglishText == "" {
			t.Fatalf("incomplete bilingual route item %+v", item)
		}
	}
}
