package presentation

import "testing"

func TestFlightHUDPutsSurfaceNavigationFirstAndShowsKnownValues(t *testing.T) {
	state := M{
		"settings":  M{"activity": "flight", "language": "en"},
		"commander": M{"system": "Synthetic System", "vehicle": "ship"},
		"pilot":     M{"fuel": M{"mainTons": 3}},
		"surfaceNavigation": M{
			"target":   M{"latitude": 44.4825, "longitude": -61.4854, "bodyName": "Торговля A 1", "systemName": "Торговля"},
			"current":  M{"latitude": 40, "longitude": 0},
			"guidance": M{"bearing": 359.99, "distanceM": 1530},
			"status":   "active",
		},
	}
	doc := BuildHUD(state, hudFixtureNow)
	if len(doc.Sections) != 2 {
		t.Fatalf("expected coordinate and fuel cards, got %+v", doc.Sections)
	}
	card := doc.Sections[0]
	if card.Kind != "surface-navigation" || card.Title != "SURFACE COORDINATES" || card.Subtitle != "Торговля A 1" {
		t.Fatalf("incorrect card priority, title or body: %+v", card)
	}
	want := []HUDLine{
		{Text: "Course", Value: "000°", Role: "good"},
		{Text: "Target · 44.4825 / -61.4854", Role: "text"},
		{Text: "Current · 40 / 0", Role: "muted"},
		{Text: "Surface distance", Value: "1.53 km", Role: "text"},
	}
	if len(card.Lines) != len(want) {
		t.Fatalf("expected %d rows, got %+v", len(want), card.Lines)
	}
	for i, line := range card.Lines {
		if line != want[i] {
			t.Errorf("row %d: got %+v, want %+v", i, line, want[i])
		}
	}
}

func TestFlightHUDDoesNotShowUnusableSurfaceGuidance(t *testing.T) {
	state := M{"settings": M{"activity": "flight", "language": "ru"}}
	nav := M{
		"target":   M{"latitude": 0, "longitude": 0, "bodyName": "Test A 1"},
		"current":  M{"latitude": 0, "longitude": 0},
		"guidance": M{"bearing": 0, "distanceM": 0},
	}
	state["surfaceNavigation"] = nav
	for _, test := range []struct {
		status string
		rows   int
		label  string
	}{
		{"wrong-body", 3, "Цель на другом небесном теле"},
		{"waiting-position", 3, "Ждём координаты поверхности"},
		{"arrived", 4, "Точка достигнута"},
	} {
		t.Run(test.status, func(t *testing.T) {
			nav["status"] = test.status
			card := hudSection(BuildHUD(state, hudFixtureNow), "ПОЛЁТ ПО КООРДИНАТАМ")
			if card == nil || len(card.Lines) != test.rows || card.Lines[0].Text != test.label || card.Lines[1].Text != "Цель · 0 / 0" {
				t.Fatalf("unexpected status projection: %+v", card)
			}
		})
	}
	nav["status"] = "active"
	nav["guidance"] = M{"bearing": 0}
	card := hudSection(BuildHUD(state, hudFixtureNow), "ПОЛЁТ ПО КООРДИНАТАМ")
	if card == nil || len(card.Lines) != 3 || card.Lines[0].Value != "000°" {
		t.Fatalf("north without a known radius should omit distance only: %+v", card)
	}
	nav["status"] = "no-target"
	if len(BuildHUD(state, hudFixtureNow).Sections) != 0 {
		t.Fatal("cleared target reserved HUD space")
	}
	nav["status"] = "active"
	state["settings"] = M{"activity": "cartography"}
	if len(BuildHUD(state, hudFixtureNow).Sections) != 0 {
		t.Fatal("surface guidance leaked into an unrelated activity")
	}
}
