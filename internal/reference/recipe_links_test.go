package reference

import (
	"slices"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestBlueprintWorkshopGrades(t *testing.T) {
	entry, ok := Catalog().Entry("blueprint/FSD_LongRange")
	if !ok {
		t.Fatal("missing FSD blueprint")
	}
	for _, section := range entry.Sections {
		if section.ID != "workshops" {
			continue
		}
		for i, links := range section.CellLinks {
			if slices.Equal(links, []string{"module/fsd", "engineer/300100", ""}) && section.Rows[i][2] == "G5" {
				return
			}
		}
	}
	t.Fatal("Felicity Farseer's G5 FSD upgrade missing from workshop list")
}

func TestExperimentalModifiersAreSpecificToModule(t *testing.T) {
	tests := []struct{ key, label, value string }{
		{"special_fsd_heavy", "Оптимальная масса FSD", "+4%"},
		{"special_fsd_fuelcapacity", "Максимум топлива на прыжок", "+10%"},
		{"special_engine_cooled", "Масса", "+5%"},
		{"special_fsd_cooled", "Тепловая нагрузка FSD", "-10%"},
		{"special_shield_kinetic", "Кинетическое сопротивление", "+8 п. п."},
		{"special_shieldbooster_kinetic", "Кинетическое сопротивление", "+2 п. п."},
	}
	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			entry, ok := Catalog().Entry("experimental/" + test.key)
			if ok {
				for _, section := range entry.Sections {
					if section.ID == "recipe-"+test.key && len(section.Grades) == 1 {
						for _, effect := range section.Grades[0].Effects {
							if effect.Label == test.label && effect.Value == test.value {
								return
							}
						}
					}
				}
			}
			t.Errorf("missing %q = %q in effect variant", test.label, test.value)
		})
	}
}

func TestOdysseyBlueprintVariantsMatchWorkshopRecipes(t *testing.T) {
	s := Catalog()
	for _, recipe := range v.Rows(data.Object("engineering-roadmap.json")["odysseyRecipes"]) {
		key := v.Str(recipe["key"])
		t.Run(key, func(t *testing.T) {
			entry, ok := s.Entry("blueprint/" + key)
			if !ok || entry.ID != odysseyBlueprintID(recipe) || entry.Description == "" || entry.EnglishDescription == "" {
				t.Fatal("missing localized Odyssey blueprint")
			}
			index := slices.IndexFunc(entry.Sections, func(section Section) bool { return section.ID == "recipe-"+key })
			if index < 0 || len(entry.Sections[index].Grades) != 1 {
				t.Fatal("missing exact recipe variant")
			}
			materials := entry.Sections[index].Grades[0].Materials
			rows := v.Rows(recipe["materials"])
			if len(materials) != len(rows) {
				t.Fatalf("got %d materials, want %d", len(materials), len(rows))
			}
			for i, row := range rows {
				material := materials[i]
				if material.EntryID != "material/odyssey-"+v.Str(row["key"]) || material.Count != v.Int(row["count"]) {
					t.Errorf("incorrect requirement: %+v", material)
				}
			}
			for _, engineerKey := range stringArray(recipe["engineerKeys"]) {
				engineer, ok := s.Entry("engineer/" + engineerKey)
				if !ok || !slices.ContainsFunc(engineer.Sections[1].Items, func(item Item) bool { return item.EntryID == entry.ID }) {
					t.Errorf("missing recipe backlink from %s", engineerKey)
				}
			}
		})
	}
}
