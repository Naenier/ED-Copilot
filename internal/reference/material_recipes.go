package reference

import (
	"fmt"
	"sort"
	"strings"
)

type materialRecipeUse struct {
	entryID, title string
	grades         [6]bool
	min, max       int
}

func (s *Store) linkMaterialRecipes() {
	uses := map[string]map[string]*materialRecipeUse{}
	for _, record := range s.records {
		e := record.entry
		if e.Category != "blueprints" && e.Category != "experimentals" {
			continue
		}
		for _, section := range e.Sections {
			for _, grade := range section.Grades {
				for _, ingredient := range grade.Materials {
					if s.records[ingredient.EntryID] == nil || ingredient.Count <= 0 {
						continue
					}
					byRecipe := uses[ingredient.EntryID]
					if byRecipe == nil {
						byRecipe = map[string]*materialRecipeUse{}
						uses[ingredient.EntryID] = byRecipe
					}
					use := byRecipe[e.ID]
					if use == nil {
						use = &materialRecipeUse{entryID: e.ID, title: e.Title, min: ingredient.Count, max: ingredient.Count}
						byRecipe[e.ID] = use
					}
					if grade.Grade >= 0 && grade.Grade < len(use.grades) {
						use.grades[grade.Grade] = true
					}
					use.min, use.max = min(use.min, ingredient.Count), max(use.max, ingredient.Count)
				}
			}
		}
	}
	for id, byRecipe := range uses {
		material := s.records[id]
		if material.entry.Category != "materials" {
			continue
		}
		rows := make([]*materialRecipeUse, 0, len(byRecipe))
		for _, use := range byRecipe {
			rows = append(rows, use)
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].title == rows[j].title {
				return rows[i].entryID < rows[j].entryID
			}
			return normalize(rows[i].title) < normalize(rows[j].title)
		})
		section := Section{ID: "applications", Title: "Где применяется", EnglishTitle: "Recipes that use this material", Type: "table",
			Columns: []string{"Чертёж или эффект", "Ступени", "Расход за применение"}}
		for _, use := range rows {
			grades := []string{}
			for grade, present := range use.grades {
				if !present {
					continue
				}
				if grade == 0 {
					grades = append(grades, "Установка")
				} else {
					grades = append(grades, fmt.Sprintf("G%d", grade))
				}
			}
			quantity := fmt.Sprint(use.min)
			if use.min != use.max {
				quantity = fmt.Sprintf("%d–%d", use.min, use.max)
			}
			section.Rows = append(section.Rows, []string{use.title, strings.Join(grades, ", "), quantity})
			section.CellLinks = append(section.CellLinks, []string{use.entryID, "", ""})
		}
		material.entry.Sections = append(material.entry.Sections, section)
		related := material.relatedIDs[:0]
		for _, link := range material.relatedIDs {
			if byRecipe[link] == nil {
				related = append(related, link)
			}
		}
		material.relatedIDs = related
	}
}
