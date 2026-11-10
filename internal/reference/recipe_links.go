package reference

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type workshopRecipe struct {
	module, engineer, moduleID, engineerID string
	maxGrade                               int
}

func (s *Store) buildRecipeWorkshops(modules, engineers []v.M, blueprints, effects map[string]v.M) {
	engineerIDs := map[string]string{}
	for _, engineer := range engineers {
		engineerIDs[v.Str(engineer["english"])] = "engineer/" + v.Str(engineer["key"])
	}
	workshops := map[string][]workshopRecipe{}
	for _, module := range modules {
		moduleID := "module/" + canonicalModule(v.Str(module["key"]))
		moduleTitle := v.Str(module["name"])
		moduleEngineers := map[string]bool{}
		for recipeKey, rawGrades := range v.Obj(module["engineers"]) {
			recipe := blueprints[recipeKey]
			if recipe == nil || !hasKey(module["blueprints"], recipeKey) {
				continue
			}
			highest := map[string]int{}
			for rawGrade, rawNames := range v.Obj(rawGrades) {
				grade, err := strconv.Atoi(rawGrade)
				if err != nil || grade < 1 || grade > 5 || len(v.Rows(v.At(recipe, "grades", rawGrade))) == 0 {
					continue
				}
				for _, name := range stringArray(rawNames) {
					if id := engineerIDs[name]; s.records[id] != nil {
						highest[id] = max(highest[id], grade)
						moduleEngineers[id] = true
					}
				}
			}
			page := blueprintID(recipe)
			for id, grade := range highest {
				workshops[page] = append(workshops[page], workshopRecipe{moduleTitle, s.records[id].entry.Title, moduleID, id, grade})
			}
		}
		for _, key := range stringArray(module["experimentals"]) {
			if effect := effects[key]; effect != nil {
				page := experimentalID(effect)
				for id := range moduleEngineers {
					workshops[page] = append(workshops[page], workshopRecipe{moduleTitle, s.records[id].entry.Title, moduleID, id, 0})
				}
			}
		}
	}

	for page, rows := range workshops {
		record := s.records[page]
		if record == nil {
			continue
		}
		slices.SortFunc(rows, func(a, b workshopRecipe) int {
			return cmp.Or(cmp.Compare(a.module, b.module), cmp.Compare(a.engineer, b.engineer), cmp.Compare(b.maxGrade, a.maxGrade))
		})
		section := Section{ID: "workshops", Title: "Инженеры", EnglishTitle: "Engineers", Type: "table", Columns: []string{"Модуль", "Инженер"}}
		if record.entry.Category == "blueprints" {
			section.Columns = append(section.Columns, "Максимальный класс")
		}
		var previous workshopRecipe
		for _, row := range rows {
			if previous.module == row.module && previous.engineerID == row.engineerID {
				continue
			}
			values := []string{row.module, row.engineer}
			links := []string{row.moduleID, row.engineerID}
			if row.maxGrade > 0 {
				values = append(values, fmt.Sprintf("G%d", row.maxGrade))
				links = append(links, "")
			}
			section.Rows = append(section.Rows, values)
			section.CellLinks = append(section.CellLinks, links)
			previous = row
		}
		record.entry.Sections = append(record.entry.Sections, section)
	}
	s.linkRelatedRecipes(
		"blueprint/FSD_LongRange", "blueprint/FSD_FastBoot", "blueprint/FSD_Shielded",
		"experimental/special_fsd_heavy", "experimental/special_fsd_fuelcapacity",
	)
	s.linkRelatedRecipes("blueprint/Engine_Dirty", "blueprint/Engine_Tuned", "experimental/special_engine_overloaded", "experimental/special_engine_haulage")
	s.linkRelatedRecipes("blueprint/PowerPlant_Armoured", "blueprint/PowerPlant_Stealth", "blueprint/PowerPlant_Boosted", "experimental/special_powerplant_cooled")
	s.linkRelatedRecipes("blueprint/ShieldCellBank_Rapid", "blueprint/ShieldCellBank_Specialised", "experimental/special_feedback_cascade_cooled")
}

func (s *Store) linkRelatedRecipes(keys ...string) {
	for _, key := range keys {
		record := s.records[s.aliases[key]]
		if record == nil {
			continue
		}
		section := Section{ID: "related-recipes", Title: "Связанные улучшения", EnglishTitle: "Related upgrades", Type: "prose"}
		for _, candidate := range keys {
			related := s.records[s.aliases[candidate]]
			if related == nil || related == record {
				continue
			}
			section.Items = append(section.Items, Item{Title: related.entry.Title, EnglishTitle: related.entry.Subtitle, EntryID: related.entry.ID})
		}
		if len(section.Items) != 0 {
			record.entry.Sections = append(record.entry.Sections, section)
		}
	}
}
