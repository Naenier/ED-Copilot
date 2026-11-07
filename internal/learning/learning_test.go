package learning

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func fixedLibrary() *Library {
	return New(nil, WithClock(func() time.Time { return time.Date(2026, 11, 8, 10, 30, 0, 0, time.UTC) }))
}
func update(t *testing.T, l *Library, body M) {
	t.Helper()
	if err := l.Update(body); err != nil {
		t.Fatal(err)
	}
}
func lessonRow(t *testing.T, state M, id string) M {
	t.Helper()
	for _, row := range v.Rows(state["lessons"]) {
		if row["id"] == id {
			return row
		}
	}
	t.Fatalf("lesson not found: %s", id)
	return nil
}

func TestNinePTNTopicsHaveCompleteOriginalLearningPaths(t *testing.T) {
	c := catalog()
	if len(c.Courses) != 9 || len(c.Lessons) != 27 {
		t.Fatal(len(c.Courses), len(c.Lessons))
	}
	if err := validateCatalog(c); err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{"trade-alerts": true, "wmm": true, "booze-cruise": true, "afk-combat": true, "odyssey": true, "operations": true, "carrier-basics": true, "carrier-trade": true, "robigo": true}
	for _, course := range c.Courses {
		if !expected[course.ID] || len(course.LessonIDs) != 3 || course.Minutes != 18 {
			t.Fatal(course)
		}
		delete(expected, course.ID)
		for i, id := range course.LessonIDs {
			lesson := lessonRow(t, fixedLibrary().Snapshot(), id)
			if v.Int(lesson["order"]) != i+1 || lesson["courseId"] != course.ID {
				t.Fatal(lesson)
			}
			if !strings.Contains(LessonTextForTest(lesson), " ") {
				t.Fatal("empty lesson", id)
			}
		}
	}
	if len(expected) > 0 {
		t.Fatal("missing topics", expected)
	}
	var source struct{ Courses []courseSeed }
	if err := json.Unmarshal(contentJSON, &source); err != nil {
		t.Fatal(err)
	}
	for _, course := range source.Courses {
		// The short mechanics digest and factual quiz stay bounded per PTN
		// article; the distinct app workflows and fictional exercises are ours.
		derived := strings.Join(course.Mechanics, " ")
		for _, q := range course.Quiz {
			derived += " " + q.Prompt + " " + strings.Join(q.Choices, " ") + " " + q.Explanation
		}
		if count := len(strings.Fields(derived)); count > 200 {
			t.Fatalf("%s digest exceeds bound: %d", course.ID, count)
		}
		if len(course.Practice) < 3 || len(course.Checklist) < 5 || len(course.Exercise) < 80 || len(course.Solution) < 80 {
			t.Fatal("incomplete original practice", course.ID)
		}
	}
	text := string(contentJSON)
	for _, forbidden := range []string{"EDMC", "EDDN", "Client ID", "Developer Zone", "<script", "<iframe", "http://localhost"} {
		if strings.Contains(text, forbidden) {
			t.Fatal("obsolete or executable content", forbidden)
		}
	}
}

func LessonTextForTest(lesson M) string {
	out := v.Str(lesson["title"])
	for _, s := range v.Rows(lesson["sections"]) {
		for _, p := range v.Arr(s["paragraphs"]) {
			out += " " + v.Str(p)
		}
	}
	return out
}

func TestSelectBookmarkPositionQuizCompleteAndRestart(t *testing.T) {
	l := fixedLibrary()
	if len(v.Obj(l.Snapshot()["activeLesson"])) != 0 {
		t.Fatal("new library selected a lesson without the user")
	}
	update(t, l, M{"action": "select", "lessonId": "wmm.checkpoint"})
	update(t, l, M{"action": "position", "lessonId": "wmm.checkpoint", "sectionId": "solution", "ratio": 0.64})
	update(t, l, M{"action": "bookmark", "lessonId": "wmm.checkpoint", "bookmarked": true})
	update(t, l, M{"action": "answer", "lessonId": "wmm.checkpoint", "questionId": "share", "choiceIndex": 0})
	s := l.Snapshot()
	active := v.Obj(s["activeLesson"])
	if active["id"] != "wmm.checkpoint" || v.Bool(active["completed"]) || v.Str(active["status"]) != "in-progress" || v.Num(v.At(active, "position", "ratio")) != 0.64 {
		t.Fatal(active)
	}
	if v.Int(v.At(active, "quizResult", "correct")) != 0 || v.Bool(v.At(active, "quizResult", "passed")) {
		t.Fatal("wrong answer counted as correct", active["quizResult"])
	}
	update(t, l, M{"action": "answer", "questionId": "share", "choiceIndex": 1})
	update(t, l, M{"action": "answer", "questionId": "budget", "choiceIndex": 0})
	active = v.Obj(l.Snapshot()["activeLesson"])
	if !v.Bool(v.At(active, "quizResult", "passed")) || v.Bool(active["completed"]) {
		t.Fatal("quiz should not silently mark reading complete", active)
	}
	update(t, l, M{"action": "complete", "completed": true})
	s = l.Snapshot()
	if v.Int(v.At(s, "progress", "completed")) != 1 || v.Int(v.At(s, "progress", "percent")) != 4 || v.Int(v.At(s, "progress", "bookmarked")) != 1 {
		t.Fatal(s["progress"])
	}
	if v.Rows(s["recent"])[0]["id"] != "wmm.checkpoint" {
		t.Fatal(s["recent"])
	}
	reloaded := New(l.ExportSettings()).Snapshot()
	if !reflect.DeepEqual(s, reloaded) {
		t.Fatal("restart lost progress, bookmark, reader position or answer")
	}
	update(t, l, M{"action": "complete", "completed": false})
	if v.Bool(v.At(l.Snapshot(), "activeLesson", "completed")) {
		t.Fatal("unable to repeat a completed lesson")
	}
	update(t, l, M{"action": "bookmark", "bookmarked": false})
	if v.Bool(v.At(l.Snapshot(), "activeLesson", "bookmarked")) {
		t.Fatal("unable to remove bookmark")
	}
}

func TestFiltersSearchAndCourseProgressRemainIndependent(t *testing.T) {
	l := fixedLibrary()
	update(t, l, M{"action": "select", "lessonId": "booze-cruise.basics"})
	update(t, l, M{"action": "bookmark"})
	update(t, l, M{"action": "select", "lessonId": "wmm.basics"})
	update(t, l, M{"action": "complete"})
	update(t, l, M{"action": "configure", "courseId": "booze-cruise", "query": "HIP 58832", "filter": "bookmarked"})
	s := l.Snapshot()
	filtered := v.Rows(s["filteredLessons"])
	if len(filtered) != 1 || filtered[0]["id"] != "booze-cruise.basics" {
		t.Fatal(filtered)
	}
	if s["selectedLessonId"] != "wmm.basics" {
		t.Fatal("filter changed active reader", s["selectedLessonId"])
	}
	for _, course := range v.Rows(s["courses"]) {
		if course["id"] == "wmm" && v.Int(course["completedLessons"]) != 1 {
			t.Fatal(course)
		}
		if course["id"] == "booze-cruise" && v.Int(course["completedLessons"]) != 0 {
			t.Fatal("course completion crossed topics", course)
		}
	}
	update(t, l, M{"action": "configure", "courseId": "", "query": "", "filter": "in-progress"})
	filtered = v.Rows(l.Snapshot()["filteredLessons"])
	if len(filtered) != 1 || filtered[0]["id"] != "booze-cruise.basics" {
		t.Fatal("completed lesson appeared as in-progress", filtered)
	}
	if !Matches(M{"title": "Завершённый полёт", "tags": []string{"Type-10", "AFK"}}, "завершенный type-10") || Matches(M{"title": "Торговля"}, "тритий") {
		t.Fatal("Unicode/multiword search")
	}
}

func TestInvalidCommandsAreAtomicAndStoredUnknownIDsAreIgnored(t *testing.T) {
	l := fixedLibrary()
	update(t, l, M{"action": "select", "id": "odyssey.basics"})
	before := l.ExportSettings()
	for _, body := range []M{
		{"action": "select", "lessonId": "missing"}, {"action": "select", "lessonId": "wmm.basics", "sectionId": "missing"},
		{"action": "select", "lessonId": "wmm.basics", "ratio": 2}, {"action": "position", "sectionId": "missing", "ratio": 0.5},
		{"action": "position", "ratio": math.NaN()}, {"action": "position", "ratio": -0.1},
		{"action": "answer", "questionId": "none", "choiceIndex": 0}, {"action": "answer", "lessonId": "wmm.checkpoint", "questionId": "share", "choiceIndex": 1.5},
		{"action": "answer", "lessonId": "wmm.checkpoint", "questionId": "share", "choiceIndex": 3},
		{"action": "complete", "completed": "false"}, {"action": "bookmark", "bookmarked": 1},
		{"action": "configure", "courseId": "missing", "query": "would partially apply"}, {"action": "configure", "filter": "no-such-filter"},
		{"action": "configure", "query": strings.Repeat("ё", 257)}, {"action": "configure", "query": 7}, {"action": "reset-progress", "courseId": "missing"}, {"action": "erase-disk"},
	} {
		if err := l.Update(body); err == nil {
			t.Fatal("invalid command accepted", body)
		}
		if !reflect.DeepEqual(before, l.ExportSettings()) {
			t.Fatal("invalid command partially applied", body)
		}
	}
	bad := M{"selectedLessonId": "gone", "completed": M{"gone": "2026-11-08T10:30:00Z", "wmm.basics": "bad date"}, "bookmarks": M{"gone": true}, "positions": M{"wmm.basics": M{"sectionId": "missing", "ratio": 2}}, "answers": M{"wmm.checkpoint": M{"share": 99, "gone": 0}}, "options": M{"courseId": "gone"}}
	s := New(bad).Snapshot()
	if s["selectedLessonId"] != "" || v.Int(v.At(s, "progress", "completed")) != 0 || v.Int(v.At(s, "progress", "bookmarked")) != 0 {
		t.Fatal("invalid stored IDs survived", s["progress"])
	}
	if len(v.Obj(lessonRow(t, s, "wmm.basics")["position"])) > 0 || v.Int(v.At(lessonRow(t, s, "wmm.checkpoint"), "quizResult", "answered")) != 0 {
		t.Fatal("invalid stored reader state survived")
	}
}

func TestResetOneCoursePreservesOtherProgressAndBookmarks(t *testing.T) {
	l := fixedLibrary()
	for _, id := range []string{"wmm.basics", "wmm.checkpoint", "odyssey.basics"} {
		update(t, l, M{"action": "select", "lessonId": id})
		update(t, l, M{"action": "complete"})
		update(t, l, M{"action": "bookmark"})
	}
	update(t, l, M{"action": "answer", "lessonId": "wmm.checkpoint", "questionId": "share", "choiceIndex": 1})
	update(t, l, M{"action": "reset-progress", "courseId": "wmm"})
	s := l.Snapshot()
	if v.Int(v.At(s, "progress", "completed")) != 1 || v.Int(v.At(s, "progress", "bookmarked")) != 3 {
		t.Fatal(s["progress"])
	}
	if lessonRow(t, s, "wmm.basics")["status"] != "not-started" || !v.Bool(lessonRow(t, s, "odyssey.basics")["completed"]) {
		t.Fatal("reset crossed course boundary")
	}
	if v.Int(v.At(lessonRow(t, s, "wmm.checkpoint"), "quizResult", "answered")) != 0 {
		t.Fatal("course quiz retained answers after reset")
	}
	update(t, l, M{"action": "reset-progress"})
	s = l.Snapshot()
	if v.Int(v.At(s, "progress", "completed")) != 0 || len(v.Rows(s["recent"])) != 0 || v.Int(v.At(s, "progress", "bookmarked")) != 3 {
		t.Fatal(s["progress"])
	}
}

func TestSnapshotsAndExportsCannotMutateTheLibraryOrCatalogue(t *testing.T) {
	l := fixedLibrary()
	update(t, l, M{"action": "select", "lessonId": "wmm.checkpoint"})
	update(t, l, M{"action": "answer", "questionId": "share", "choiceIndex": 1})
	s := l.Snapshot()
	v.Rows(s["courses"])[0]["title"] = "changed"
	v.Rows(s["lessons"])[0]["title"] = "changed"
	v.Obj(v.At(s, "activeLesson", "quizResult", "answers"))["share"] = 0
	settings := l.ExportSettings()
	v.Obj(settings["opened"])["wmm.checkpoint"] = "changed"
	lesson := Catalog()
	v.Rows(lesson[0]["sections"])[0]["title"] = "changed"
	courses := CourseCatalog()
	courses[0]["title"] = "changed"
	fresh := l.Snapshot()
	if v.Int(v.At(fresh, "activeLesson", "quizResult", "answers", "share")) != 1 || v.Rows(fresh["courses"])[0]["title"] == "changed" || v.Rows(Catalog()[0]["sections"])[0]["title"] == "changed" {
		t.Fatal("mutable snapshot escaped")
	}
}

func TestConcurrentReaderUpdatesAndSnapshots(t *testing.T) {
	l := fixedLibrary()
	var wg sync.WaitGroup
	for worker := 0; worker < 6; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for step := 0; step < 12; step++ {
				if worker%2 == 0 {
					if err := l.Update(M{"action": "position", "lessonId": "carrier-basics.practice", "sectionId": "workflow", "ratio": float64(step) / 12}); err != nil {
						t.Error(err)
					}
				} else {
					if len(v.Rows(l.Snapshot()["lessons"])) != 27 {
						t.Error("incomplete concurrent snapshot")
					}
					_ = l.ExportSettings()
				}
			}
		}(worker)
	}
	wg.Wait()
}

func TestChecklistPersistsSeparatelyAndValidatesBeforeMutation(t *testing.T) {
	l := fixedLibrary()
	update(t, l, M{"action": "select", "lessonId": "carrier-basics.practice"})
	update(t, l, M{"action": "checklist", "sectionId": "checklist", "index": 2, "checked": true})
	state := l.Snapshot()
	if !v.Bool(v.At(state, "activeLesson", "checklistState", "checklist:2")) || v.Bool(v.At(state, "activeLesson", "completed")) {
		t.Fatal("checklist completed the lesson implicitly")
	}
	copy := New(l.ExportSettings()).Snapshot()
	if !v.Bool(v.At(copy, "activeLesson", "checklistState", "checklist:2")) {
		t.Fatal("checklist did not survive restart")
	}
	before := l.ExportSettings()
	for _, body := range []M{{"action": "checklist", "sectionId": "missing", "index": 0, "checked": true}, {"action": "checklist", "sectionId": "checklist", "index": 100, "checked": true}, {"action": "checklist", "sectionId": "checklist", "index": 0.2, "checked": true}, {"action": "checklist", "sectionId": "checklist", "index": 0, "checked": "true"}} {
		if l.Update(body) == nil || !reflect.DeepEqual(before, l.ExportSettings()) {
			t.Fatal("invalid checklist update applied", body)
		}
	}
	update(t, l, M{"action": "checklist", "sectionId": "checklist", "index": 2, "checked": false})
	if v.Bool(v.At(l.Snapshot(), "activeLesson", "checklistState", "checklist:2")) {
		t.Fatal("unable to clear checklist item")
	}
	update(t, l, M{"action": "checklist", "sectionId": "checklist", "index": 2, "checked": true})
	update(t, l, M{"action": "reset-progress", "courseId": "carrier-basics"})
	if len(v.Obj(v.At(l.Snapshot(), "activeLesson", "checklistState"))) != 0 {
		t.Fatal("course reset retained checklist marks")
	}
}

func TestDelayedPositionSaveDoesNotReorderTheCurrentLesson(t *testing.T) {
	at := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	l := New(nil, WithClock(func() time.Time { return at }))
	update(t, l, M{"action": "select", "lessonId": "wmm.basics"})
	at = at.Add(time.Minute)
	update(t, l, M{"action": "select", "lessonId": "odyssey.basics"})
	at = at.Add(time.Second)
	update(t, l, M{"action": "position", "lessonId": "wmm.basics", "sectionId": "mechanics", "ratio": 0.8})
	s := l.Snapshot()
	if v.Rows(s["recent"])[0]["id"] != "odyssey.basics" || s["selectedLessonId"] != "odyssey.basics" {
		t.Fatal("late scroll save replaced current lesson")
	}
	if v.Num(v.At(lessonRow(t, s, "wmm.basics"), "position", "ratio")) != 0.8 {
		t.Fatal("late scroll position lost")
	}
}

func TestBlankUnicodeSearchKeepsCompleteIndependentSnapshot(t *testing.T) {
	lesson := Catalog()[1]
	for _, query := range []string{"", " \t\r\n", "\u00a0", "\u2003\u202f\u3000"} {
		if !Matches(lesson, query) {
			t.Fatalf("blank query %q excluded a lesson", query)
		}
	}
	l := fixedLibrary()
	before := l.Snapshot()
	update(t, l, M{"action": "configure", "query": "\t\u00a0\u2003\u202f\u3000\n"})
	after := l.Snapshot()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("clearing search with Unicode whitespace changed the unfiltered snapshot")
	}
	filtered, all := v.Rows(after["filteredLessons"]), v.Rows(after["lessons"])
	if len(filtered) != len(all) || len(all) == 0 {
		t.Fatal("blank search omitted lessons")
	}
	original := v.Rows(all[0]["sections"])[0]["title"]
	v.Rows(filtered[0]["sections"])[0]["title"] = "changed filtered copy"
	if v.Rows(all[0]["sections"])[0]["title"] != original {
		t.Fatal("unfiltered search results share mutable lesson sections")
	}
	fresh := l.Snapshot()
	if v.Rows(v.Rows(fresh["filteredLessons"])[0]["sections"])[0]["title"] != original {
		t.Fatal("unfiltered search result mutated the next snapshot")
	}
}

func BenchmarkMatchesBlankQuery(b *testing.B) {
	// Build and decode the catalogue before timing the common no-search path.
	lesson := Catalog()[1]
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !Matches(lesson, "") {
			b.Fatal("blank query excluded a lesson")
		}
	}
}
