package learning

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestLearningCleanupPreservesExercisesQuizzesAndReaderAnchors(t *testing.T) {
	for _, language := range []string{"ru", "en"} {
		raw := contentJSON
		if language == "en" {
			raw = englishContentJSON
		}
		var source struct {
			Courses []courseSeed `json:"courses"`
		}
		if err := json.Unmarshal(raw, &source); err != nil {
			t.Fatal(err)
		}
		catalog := catalogForLanguage(language)
		if len(catalog.Courses) != 9 || len(catalog.Lessons) != 27 {
			t.Fatal("learning content lost", language)
		}
		lessons := map[string]Lesson{}
		for _, lesson := range catalog.Lessons {
			lessons[lesson.ID] = lesson
			for _, section := range lesson.Sections {
				if section.ID == "outcome" {
					t.Fatal("generic repeated outcome returned", lesson.ID)
				}
				for _, text := range section.Paragraphs {
					if strings.TrimSpace(text) == "" {
						t.Fatal("empty section paragraph", lesson.ID, section.ID)
					}
				}
			}
		}
		for _, seed := range source.Courses {
			practice := lessons[seed.ID+".practice"]
			if !reflect.DeepEqual(practice.Sections[0].Paragraphs, seed.Practice) || !reflect.DeepEqual(practice.Sections[1].Checklist, seed.Checklist) {
				t.Fatal("concrete workflow/checklist changed", seed.ID)
			}
			if practice.Summary != seed.Practice[0] {
				t.Fatal("practice preview must describe its own workflow", seed.ID)
			}
			checkpoint := lessons[seed.ID+".checkpoint"]
			if !reflect.DeepEqual(checkpoint.Quiz, seed.Quiz) || checkpoint.Sections[0].Paragraphs[0] != seed.Exercise || checkpoint.Sections[1].Paragraphs[0] != seed.Solution {
				t.Fatal("worked exercise, solution or quiz lost", seed.ID)
			}
			for _, section := range checkpoint.Sections {
				if section.ID == "reflection" && seed.Reflection == "" {
					t.Fatal("removed coaching became an empty section", seed.ID)
				}
			}
		}
	}
}

func TestRemovedCoachingAnchorDoesNotEraseSavedLearningProgress(t *testing.T) {
	saved := M{"selectedLessonId": "odyssey.checkpoint", "completed": M{"odyssey.checkpoint": "2026-11-08T10:00:00Z"}, "bookmarks": M{"odyssey.checkpoint": true}, "positions": M{"odyssey.checkpoint": M{"sectionId": "reflection", "ratio": .5}, "wmm.practice": M{"sectionId": "workflow", "ratio": .4}}, "checklists": M{"wmm.practice": M{"checklist:2": true}}, "answers": M{"wmm.checkpoint": M{"share": 1}}}
	library := New(saved)
	exported := library.ExportSettings()
	if v.At(exported, "completed", "odyssey.checkpoint") == nil || !v.Bool(v.At(exported, "bookmarks", "odyssey.checkpoint")) || v.At(exported, "positions", "wmm.practice", "sectionId") != "workflow" || !v.Bool(v.At(exported, "checklists", "wmm.practice", "checklist:2")) || v.Int(v.At(exported, "answers", "wmm.checkpoint", "share")) != 1 {
		t.Fatal("unrelated learning progress lost", exported)
	}
	if v.At(exported, "positions", "odyssey.checkpoint") != nil {
		t.Fatal("stale removed anchor retained")
	}
}
