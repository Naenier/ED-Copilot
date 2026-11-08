package learning

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unicode"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestRichSectionSchemaKeepsExistingReaderAnchorsAndLocalizedBuilderText(t *testing.T) {
	var source struct {
		ReviewedAt string       `json:"reviewedAt"`
		Courses    []courseSeed `json:"courses"`
	}
	if err := json.Unmarshal(contentJSON, &source); err != nil {
		t.Fatal(err)
	}
	source.Courses = source.Courses[:1]
	seed := &source.Courses[0]
	seed.BasicsSections = []Section{{ID: "decision", Title: "Decision path", Paragraphs: []string{"A distinct original teaching scenario."}}}
	seed.PracticeSections = []Section{{ID: "practice-errors", Title: "Recovering from an error", Paragraphs: []string{"Compare the actual result with the planned checkpoint."}}}
	seed.CheckpointSections = []Section{{ID: "extra-exercise", Title: "Second exercise", Paragraphs: []string{"Explain the next step."}}}
	seed.BasicsMinutes, seed.PracticeMinutes, seed.CheckpointMinutes = 12, 18, 10
	raw, _ := json.Marshal(source)
	c, err := buildCatalog(raw, "en")
	if err != nil {
		t.Fatal(err)
	}
	if c.Courses[0].Minutes != 40 || c.Lessons[0].Minutes != 12 || c.Lessons[1].Minutes != 18 || c.Lessons[2].Minutes != 10 {
		t.Fatal("explicit lesson durations were discarded")
	}
	for i, expected := range [][]string{{"mechanics", "decision"}, {"workflow", "checklist", "practice-errors"}, {"exercise", "solution", "reflection", "extra-exercise"}} {
		ids := []string{}
		for _, section := range c.Lessons[i].Sections {
			ids = append(ids, section.ID)
		}
		if !reflect.DeepEqual(ids, expected) {
			t.Fatal("extra content replaced stable reading or checklist anchors", ids)
		}
	}
	if c.Lessons[0].Sections[0].Title != "How the activity works" || c.Lessons[1].Sections[1].Title != "Before you begin" || !strings.HasPrefix(c.Lessons[2].Title, "Self-check: ") {
		t.Fatal("standard lesson framing was not localized")
	}
	seed.BasicsSections = append(seed.BasicsSections, Section{ID: "mechanics", Title: "Duplicate", Paragraphs: []string{"Must not override saved anchors."}})
	raw, _ = json.Marshal(source)
	if _, err := buildCatalog(raw, "en"); err == nil {
		t.Fatal("extra content silently shadowed an existing reader anchor")
	}
}

func TestEmbeddedEnglishCatalogPreservesAllProgressKeys(t *testing.T) {
	if _, err := contentFiles.ReadFile("content_en.json"); err != nil {
		t.Fatal("the shipped Academy must include its complete English seed", err)
	}
	ru, en := catalog(), catalogForLanguage("en")
	if en.Language != "en" || len(en.Courses) != 9 || len(en.Lessons) != 27 {
		t.Fatal("English catalogue is missing or incomplete")
	}
	if err := validateCatalogParity(ru, en); err != nil {
		t.Fatal(err)
	}
	legacyAnswers := map[string]map[string]int{
		"trade-alerts": {"direction": 0, "margin": 0}, "wmm": {"share": 1, "budget": 0},
		"booze-cruise": {"pad": 0, "schedule": 1}, "afk-combat": {"autonomy": 1, "loss": 2},
		"odyssey": {"suit": 0, "time": 0}, "operations": {"requirements": 0, "retry": 1},
		"carrier-basics": {"fuel": 1, "reserve": 2}, "carrier-trade": {"transfer": 0, "net": 2},
		"robigo": {"navigation": 1, "pace": 1},
	}
	assertEnglish := func(id, field, text string) {
		t.Helper()
		if strings.IndexFunc(text, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) >= 0 {
			t.Fatalf("English Academy has untranslated visible text: %s %s: %q", id, field, text)
		}
	}
	for _, course := range en.Courses {
		assertEnglish(course.ID, "title", course.Title)
		assertEnglish(course.ID, "summary", course.Summary)
		assertEnglish(course.ID, "category", course.Category)
		assertEnglish(course.ID, "difficulty", course.Difficulty)
	}
	for i, lesson := range en.Lessons {
		if lesson.Title == ru.Lessons[i].Title || lesson.Summary == ru.Lessons[i].Summary {
			t.Fatal("English lesson retained Russian framing", lesson.ID)
		}
		assertEnglish(lesson.ID, "title", lesson.Title)
		assertEnglish(lesson.ID, "summary", lesson.Summary)
		for _, section := range lesson.Sections {
			assertEnglish(lesson.ID, section.ID, section.Title)
			for _, text := range append(append([]string{}, section.Paragraphs...), section.Checklist...) {
				assertEnglish(lesson.ID, section.ID, text)
			}
		}
		if lesson.Kind == "checkpoint" && len(lesson.Quiz) != len(legacyAnswers[lesson.CourseID]) {
			t.Fatal("saved question IDs were removed or replaced", lesson.ID)
		}
		for _, question := range lesson.Quiz {
			if correct, exists := legacyAnswers[lesson.CourseID][question.ID]; !exists || correct != question.CorrectIndex {
				t.Fatal("an existing saved answer now points at another question or answer", lesson.ID, question.ID)
			}
			assertEnglish(lesson.ID, question.ID, question.Prompt+" "+question.Explanation+" "+strings.Join(question.Choices, " "))
		}
	}
}

func TestLanguageSwitchKeepsSelectionAnswersChecklistPositionAndSavedSearch(t *testing.T) {
	l := fixedLibrary()
	for _, command := range []M{
		{"action": "select", "lessonId": "wmm.checkpoint", "sectionId": "solution", "ratio": .64},
		{"action": "answer", "questionId": "share", "choiceIndex": 1},
		{"action": "bookmark", "bookmarked": true},
		{"action": "complete"},
		{"action": "checklist", "lessonId": "wmm.practice", "sectionId": "checklist", "index": 2, "checked": true},
		{"action": "configure", "query": "HIP 58832", "filter": "all"},
	} {
		update(t, l, command)
	}
	before := l.ExportSettings()
	ru, en := l.Snapshot(), l.SnapshotForLanguage("en")
	for _, state := range []M{ru, en} {
		active := v.Obj(state["activeLesson"])
		if active["id"] != "wmm.checkpoint" || !v.Bool(active["completed"]) || !v.Bool(active["bookmarked"]) || v.Str(v.At(active, "position", "sectionId")) != "solution" || v.Num(v.At(active, "position", "ratio")) != .64 || v.Int(v.At(active, "quizResult", "answers", "share")) != 1 || v.Int(v.At(active, "quizResult", "correct")) != 1 {
			t.Fatal("switching language changed persistent learning state", active)
		}
		if !v.Bool(v.At(lessonRow(t, state, "wmm.practice"), "checklistState", "checklist:2")) || v.At(state, "options", "query") != "HIP 58832" {
			t.Fatal("switching language rewrote checklists or the typed query")
		}
	}
	if !reflect.DeepEqual(before, l.ExportSettings()) || !reflect.DeepEqual(ru["progress"], en["progress"]) || !reflect.DeepEqual(ru["progress"], l.ProgressSnapshot()["progress"]) {
		t.Fatal("localized snapshot mutated stored options or progress")
	}
	if v.Str(v.At(en, "activeLesson", "title")) == v.Str(v.At(ru, "activeLesson", "title")) {
		t.Fatal("reader did not change to the English course")
	}
	reloaded := New(l.ExportSettings())
	if !reflect.DeepEqual(en, reloaded.SnapshotForLanguage("en")) {
		t.Fatal("English reading progress did not survive a restart")
	}
}

func TestLocalizedSearchUsesChosenTextAndSnapshotsRemainIndependent(t *testing.T) {
	l := fixedLibrary()
	update(t, l, M{"action": "configure", "courseId": "wmm", "query": "Before you begin"})
	en, ru := l.SnapshotForLanguage("en"), l.Snapshot()
	if len(v.Rows(en["filteredLessons"])) != 1 || v.Rows(en["filteredLessons"])[0]["id"] != "wmm.practice" || len(v.Rows(ru["filteredLessons"])) != 0 {
		t.Fatal("search did not use the requested language's lesson text")
	}
	before := l.ExportSettings()
	row := v.Rows(en["filteredLessons"])[0]
	v.Rows(row["sections"])[0]["title"] = "mutated translated result"
	v.Rows(en["courses"])[0]["title"] = "mutated course"
	if strings.Contains(LessonTextForTest(lessonRow(t, l.SnapshotForLanguage("en"), "wmm.practice")), "mutated") || !reflect.DeepEqual(before, l.ExportSettings()) || v.Rows(lessonRow(t, en, "wmm.practice")["sections"])[0]["title"] == "mutated translated result" {
		t.Fatal("a translated result escaped into the cache, all-lessons view or persistent state")
	}
}

func TestConcurrentLocalizedReadsPreserveOneSharedProgressModel(t *testing.T) {
	l := fixedLibrary()
	var group sync.WaitGroup
	for _, language := range []string{"ru", "en"} {
		group.Add(1)
		go func(language string) {
			defer group.Done()
			for i := 0; i < 10; i++ {
				state := l.SnapshotForLanguage(language)
				if state["language"] != language || len(v.Rows(state["lessons"])) != 27 {
					t.Error("localized catalogue crossed concurrent readers", language)
				}
			}
		}(language)
	}
	for i := 0; i < 10; i++ {
		update(t, l, M{"action": "position", "lessonId": "wmm.basics", "sectionId": "mechanics", "ratio": float64(i) / 10})
	}
	group.Wait()
}

func BenchmarkLearningSnapshots(b *testing.B) {
	l := fixedLibrary()
	for _, language := range []string{"ru", "en"} {
		_ = l.SnapshotForLanguage(language)
		b.Run(language, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = l.SnapshotForLanguage(language)
			}
		})
	}
	b.Run("progress-only", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = l.ProgressSnapshot()
		}
	})
}
