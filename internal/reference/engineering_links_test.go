package reference

import (
	"testing"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestEngineeringReferenceLinksResolve(t *testing.T) {
	s := Catalog()
	resolved := map[string]bool{}
	check := func(origin, link string) {
		if link == "" {
			return
		}
		if _, checked := resolved[link]; !checked {
			_, resolved[link] = s.Entry(link)
		}
		if !resolved[link] {
			t.Errorf("%s links to missing %s", origin, link)
		}
	}
	for _, record := range s.ordered {
		e := record.entry
		switch e.Category {
		case "engineers", "modules", "materials", "blueprints", "experimentals":
		default:
			continue
		}
		for _, prop := range e.Properties {
			check(e.ID, prop.EntryID)
		}
		for _, section := range e.Sections {
			for _, item := range section.Items {
				check(e.ID, item.EntryID)
			}
			for _, cells := range section.CellLinks {
				for _, link := range cells {
					check(e.ID, link)
				}
			}
			for _, grade := range section.Grades {
				for _, material := range grade.Materials {
					check(e.ID, material.EntryID)
				}
			}
		}
	}
	roadmap := data.Object("engineering-roadmap.json")
	for _, row := range v.Rows(roadmap["engineers"]) {
		check("progression", "engineer/"+v.Str(row["key"]))
	}
	for _, row := range v.Rows(roadmap["odysseyRecipes"]) {
		check("progression", "blueprint/"+v.Str(row["key"]))
	}
	t.Logf("checked %d distinct engineering references", len(resolved))
}

func TestMaterialsLinkBackToRecipes(t *testing.T) {
	s := Catalog()
	for _, id := range []string{"material/carbon", "material/odyssey-weaponcomponent"} {
		e, _ := s.Entry(id)
		found := false
		for _, section := range e.Sections {
			if section.ID == "applications" && len(section.Rows) > 0 && len(section.CellLinks) == len(section.Rows) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s has no linked application recipes", id)
		}
	}
}
