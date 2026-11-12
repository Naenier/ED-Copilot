package presentation

import (
	"strings"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestOdysseyHUDShowsNeededMaterialsAndFreshPickupsOnlyOnFoot(t *testing.T) {
	now := time.Now()
	state := M{
		"settings":  M{"activity": "odyssey", "language": "en"},
		"commander": M{"vehicle": "foot"},
		"odyssey": M{
			"activeLoadout": M{"name": "Колонизация"},
			"materials": []M{
				{"key": "graphene", "name": "Графен", "english": "Graphene", "count": 5, "missing": 2},
				{"key": "aerogel", "name": "Аэрогель", "english": "Aerogel", "count": 8, "missingAtMost": 4},
				{"key": "carbonfibre", "name": "Углеродное волокно", "english": "Carbon Fibre", "count": 2, "missing": 0, "spare": 1},
			},
			"recentPickups": []M{
				{"key": "graphene", "count": 2, "observedAt": v.ISO(now.Add(-2 * time.Second))},
				{"key": "aerogel", "count": 1, "observedAt": v.ISO(now.Add(-time.Minute))},
				{"key": "junk", "count": 1, "observedAt": v.ISO(now)},
			},
		},
	}
	doc := BuildHUD(state, now)
	if len(doc.Sections) != 2 || doc.Sections[0].Subtitle != "Колонизация" || len(doc.Sections[0].Lines) != 2 ||
		!strings.Contains(doc.Sections[0].Lines[0].Value, "≤ 4") || doc.Sections[1].Title != "ADDED TO BACKPACK" ||
		len(doc.Sections[1].Lines) != 1 || doc.Sections[1].Lines[0].Text != "Graphene +2" {
		t.Fatalf("wrong Odyssey HUD: %+v", doc)
	}
	v.Obj(state["commander"])["vehicle"] = "ship"
	if doc = BuildHUD(state, now); len(doc.Sections) != 0 {
		t.Fatalf("Odyssey materials occupied the cockpit: %+v", doc)
	}
}
