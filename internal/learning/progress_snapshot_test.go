package learning

import (
	"reflect"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestProgressSummaryMatchesFullLibraryAfterReaderCommands(t *testing.T) {
	library := fixedLibrary()
	commands := []M{
		{"action": "select", "lessonId": "wmm.basics"},
		{"action": "bookmark", "lessonId": "wmm.basics"},
		{"action": "complete", "lessonId": "odyssey.basics"},
		{"action": "position", "lessonId": "wmm.practice", "sectionId": "workflow", "ratio": 0.5},
		{"action": "complete", "lessonId": "wmm.practice"},
		{"action": "complete", "lessonId": "wmm.practice", "completed": false},
		{"action": "reset-progress", "courseId": "wmm"},
		{"action": "reset-progress"},
	}
	for step := -1; step < len(commands); step++ {
		if step >= 0 {
			update(t, library, commands[step])
		}
		full, summary := library.Snapshot(), library.ProgressSnapshot()
		if !reflect.DeepEqual(v.Copy(v.Obj(full["progress"])), v.Copy(v.Obj(summary["progress"]))) || len(summary) != 1 {
			t.Fatalf("dashboard progress differs at step %d: full=%v summary=%v", step, full["progress"], summary)
		}
		v.Obj(summary["progress"])["completed"] = -100
		if v.Int(v.At(library.ProgressSnapshot(), "progress", "completed")) < 0 {
			t.Fatal("dashboard summary escaped mutable library state")
		}
	}
}
