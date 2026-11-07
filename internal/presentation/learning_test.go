package presentation

import (
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/learning"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestAcademyCoursesReaderFiltersAndActions(t *testing.T) {
	library := learning.New(nil)
	state := M{"learning": library.Snapshot()}
	page := LearningPage("Все курсы", "", state)
	if page.Title != "Академия" || len(page.Rows) != 9 || len(page.Modes) != 5 {
		t.Fatal(page.Title, len(page.Rows))
	}
	for _, row := range page.Rows {
		if row.ID == "" || v.Str(row.Data["lessonId"]) == "" || !strings.Contains(row.Details, "Учебный маршрут") {
			t.Fatal(row)
		}
	}
	page = LearningPage("Все уроки", "", state)
	if len(page.Rows) != 27 {
		t.Fatal("lesson catalogue", len(page.Rows))
	}
	for _, mode := range []string{"Продолжить", "Избранное", "Изучено"} {
		if len(LearningPage(mode, "", state).Rows) != 0 {
			t.Fatal("new profile invented progress", mode)
		}
	}
	selected := page.Rows[0].Data
	actions := LearningActions("Все уроки", state, selected)
	for _, id := range []string{"learning.open", "learning.complete", "learning.bookmark"} {
		found := false
		for _, action := range actions {
			if action.ID == id {
				body, err := action.Body(nil)
				if err != nil {
					t.Fatal(err)
				}
				if err := library.Update(body); err != nil {
					t.Fatal(err)
				}
				found = true
			}
		}
		if !found {
			t.Fatal("missing reader action", id)
		}
	}
	state["learning"] = library.Snapshot()
	if len(LearningPage("Избранное", "", state).Rows) != 1 || len(LearningPage("Изучено", "", state).Rows) != 1 {
		t.Fatal("marked lessons hidden")
	}
	foundCarrier := false
	for _, row := range LearningPage("Все курсы", "тритий", state).Rows {
		if row.ID == "carrier-basics" {
			foundCarrier = true
		}
	}
	if !foundCarrier {
		t.Fatal("course search does not inspect lesson content")
	}
	if err := library.Update(M{"action": "configure", "courseId": "odyssey"}); err != nil {
		t.Fatal(err)
	}
	state["learning"] = library.Snapshot()
	if len(LearningPage("Все курсы", "", state).Rows) != 1 || len(LearningPage("Все уроки", "", state).Rows) != 3 {
		t.Fatal("saved library filters ignored")
	}
	if len(LearningPage("Все курсы", "тритий", state).Rows) != 0 {
		t.Fatal("query bypassed course filter")
	}
}

func TestAcademyDetailsPlainTextAndSafeFallback(t *testing.T) {
	page := LearningPage("Все уроки", "робиго", nil)
	if len(page.Rows) != 3 {
		t.Fatal("Russian game-name alias cannot be found")
	}
	page = LearningPage("Все уроки", "Robigo", nil)
	if len(page.Rows) != 3 {
		t.Fatal("offline catalogue unavailable", len(page.Rows))
	}
	for _, row := range page.Rows {
		if strings.Contains(row.Details, "<p>") || !strings.Contains(row.Details, "https://pilotstradenetwork.com/guides/") {
			t.Fatal(row.Details)
		}
	}
	actions := LearningActions("Все курсы", nil, M{})
	if !actions[0].Disabled {
		t.Fatal("reader can open a nonexistent row")
	}
	for _, lesson := range learning.Catalog() {
		if lesson["kind"] != "practice" {
			continue
		}
		actions = LearningActions("Все уроки", nil, lesson)
		found := false
		for _, action := range actions {
			if action.Kind == "navigate" {
				found = true
				if action.Path == "" {
					t.Fatal("missing app link")
				}
			}
		}
		if !found {
			t.Fatal("practice lacks a link to its Copilot section", lesson["id"])
		}
	}
}
