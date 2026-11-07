package app

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/profile"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestV21CalculatorAndLearningSurviveReplayAndRestart(t *testing.T) {
	engine, file, dir := fixtureEngine(t, M{"objectives": []M{{"id": "legacy-goal", "title": "Старая задача", "status": "active"}}, "manualPrices": M{"gold": M{"price": 12345}}})
	original, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	state := engine.Snapshot()
	if v.At(state, "carrierCalculator", "selectedType") != "fleet" || len(v.Rows(v.At(state, "learning", "courses"))) < 9 || len(v.Rows(v.At(state, "learning", "lessons"))) < 27 {
		t.Fatal("new local modules missing", state["carrierCalculator"])
	}
	doOK(t, engine, "/api/carrier/calculator", M{"carrierType": "fleet", "jumpsPerWeek": 4, "services": M{"refuel": "active"}, "bankBalance": 800000000})
	doOK(t, engine, "/api/carrier/calculator", M{"carrierType": "squadron", "jumpsPerWeek": 1, "services": M{"repair": "suspended"}, "purchaseBudget": 26000000000})
	doOK(t, engine, "/api/learning", M{"action": "select", "lessonId": "trade-alerts.basics", "sectionId": "mechanics", "ratio": 0.45})
	doOK(t, engine, "/api/learning", M{"action": "bookmark", "lessonId": "trade-alerts.basics", "bookmarked": true})
	doOK(t, engine, "/api/learning", M{"action": "complete", "lessonId": "trade-alerts.basics"})
	doOK(t, engine, "/api/learning", M{"action": "select", "lessonId": "trade-alerts.checkpoint", "sectionId": "exercise", "ratio": 0.5})
	doOK(t, engine, "/api/learning", M{"action": "answer", "lessonId": "trade-alerts.checkpoint", "questionId": "direction", "choiceIndex": 0})
	doOK(t, engine, "/api/learning", M{"action": "configure", "courseId": "trade-alerts", "filter": "bookmarked", "query": "перевозки"})
	calculatorSaved, learningSaved := engine.Calculator.ExportSettings(), engine.Learning.ExportSettings()
	disk, err := profile.Read(filepath.Join(dir, "settings-go-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calculatorSaved, v.Obj(disk["carrierCalculator"])) || !reflect.DeepEqual(learningSaved, v.Obj(disk["learning"])) {
		t.Fatal("successful command did not save new modules")
	}
	if v.At(disk, "manualPrices", "gold", "price") == nil || len(v.Rows(disk["objectives"])) != 1 {
		t.Fatal("new saves discarded existing plans")
	}
	engine.Reader.Rescan()
	engine.Reader.Poll()
	if !reflect.DeepEqual(calculatorSaved, engine.Calculator.ExportSettings()) || !reflect.DeepEqual(learningSaved, engine.Learning.ExportSettings()) {
		t.Fatal("journal replay erased personal calculator/study progress")
	}
	engine.Close()
	restored, err := New(Options{DataDir: dir, JournalDir: filepath.Dir(file), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if !reflect.DeepEqual(calculatorSaved, restored.Calculator.ExportSettings()) || !reflect.DeepEqual(learningSaved, restored.Learning.ExportSettings()) {
		t.Fatal("new modules did not survive restart")
	}
	state = restored.Snapshot()
	if v.At(state, "carrierCalculator", "selectedType") != "squadron" || v.At(state, "learning", "selectedLessonId") != "trade-alerts.checkpoint" {
		t.Fatal("selected workspaces were not restored")
	}
	if v.Int(v.At(state, "learning", "activeLesson", "quizResult", "correct")) != 1 || v.Int(v.At(state, "learning", "progress", "completed")) != 1 || v.Int(v.At(state, "learning", "progress", "bookmarked")) != 1 {
		t.Fatal("quiz or learning progress lost", state["learning"])
	}
	doOK(t, restored, "/api/carrier/calculator", M{"action": "select", "carrierType": "fleet"})
	if v.At(restored.Snapshot(), "carrierCalculator", "options", "services", "refuel") != "active" || v.Int(v.At(restored.Snapshot(), "carrierCalculator", "options", "jumpsPerWeek")) != 4 {
		t.Fatal("switching types discarded the other independent plan")
	}
	after, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("version 1.x profile changed", err)
	}
}

func TestV21CommandsRejectInvalidUpdatesWithoutSavingAndNotifySuccess(t *testing.T) {
	e, _, dir := fixtureEngine(t, nil)
	doOK(t, e, "/api/carrier/calculator", M{"action": "select", "carrierType": "squadron"})
	doOK(t, e, "/api/learning", M{"action": "select", "lessonId": "trade-alerts.basics"})
	for _, tc := range []struct {
		path string
		body M
	}{
		{"/api/carrier/calculator", M{"services": M{"shipyard": "off"}, "jumpsPerWeek": 20}},
		{"/api/carrier/calculator", M{"jumpsPerWeek": -1}},
		{"/api/learning", M{"action": "select", "lessonId": "trade-alerts.basics", "sectionId": "missing", "ratio": 0.9}},
		{"/api/learning", M{"action": "complete", "lessonId": "trade-alerts.basics", "completed": "yes"}},
		{"/api/learning", M{"action": "answer", "lessonId": "trade-alerts.checkpoint", "questionId": "direction", "choiceIndex": 99}},
	} {
		before, err := os.ReadFile(filepath.Join(dir, "settings-go-v2.json"))
		if err != nil {
			t.Fatal(err)
		}
		calculator, learning := e.Calculator.ExportSettings(), e.Learning.ExportSettings()
		if _, err = e.Do(tc.path, tc.body); err == nil {
			t.Fatalf("invalid command accepted: %s %#v", tc.path, tc.body)
		}
		after, err := os.ReadFile(filepath.Join(dir, "settings-go-v2.json"))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("rejected command overwrote the profile", err)
		}
		if !reflect.DeepEqual(calculator, e.Calculator.ExportSettings()) || !reflect.DeepEqual(learning, e.Learning.ExportSettings()) {
			t.Fatal("rejected command partially changed state")
		}
	}
	for {
		select {
		case <-e.Updates():
			continue
		default:
			goto drained
		}
	}
drained:
	doOK(t, e, "/api/learning", M{"action": "bookmark", "lessonId": "trade-alerts.basics", "bookmarked": true})
	select {
	case <-e.Updates():
	default:
		t.Fatal("successful learning command did not notify the UI")
	}
	doOK(t, e, "/api/carrier/calculator", M{"jumpsPerWeek": 2})
	select {
	case <-e.Updates():
	default:
		t.Fatal("successful calculator command did not notify the UI")
	}
}

func TestV21ConcurrentLocalCommandsSnapshotsAndPersistence(t *testing.T) {
	e, _, dir := fixtureEngine(t, nil)
	var wg sync.WaitGroup
	for worker := range 4 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for step := range 6 {
				switch index {
				case 0:
					if _, err := e.Do("/api/carrier/calculator", M{"jumpsPerWeek": step}); err != nil {
						t.Error(err)
					}
				case 1:
					if _, err := e.Do("/api/learning", M{"action": "bookmark", "lessonId": "trade-alerts.basics", "bookmarked": step%2 == 0}); err != nil {
						t.Error(err)
					}
				case 2:
					snapshot := e.Snapshot()
					if len(v.Rows(v.At(snapshot, "learning", "lessons"))) < 27 || len(v.Rows(v.At(snapshot, "carrierCalculator", "services"))) != 11 {
						t.Error("incomplete concurrent snapshot")
					}
				case 3:
					if err := e.Save(); err != nil {
						t.Error(err)
					}
				}
			}
		}(worker)
	}
	wg.Wait()
	if err := e.Save(); err != nil {
		t.Fatal(err)
	}
	disk, err := profile.Read(filepath.Join(dir, "settings-go-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v.Obj(disk["carrierCalculator"]), e.Calculator.ExportSettings()) || !reflect.DeepEqual(v.Obj(disk["learning"]), e.Learning.ExportSettings()) {
		t.Fatal("concurrent saves lost the final local plans")
	}
}
