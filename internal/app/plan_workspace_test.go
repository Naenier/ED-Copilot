package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/profile"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestCommanderPlansSurviveOwnerChangesAndPortableRestore(t *testing.T) {
	engine, journalFile, dir := fixtureEngine(t, M{
		"objectives": []M{{"id": "legacy-goal", "title": "Legacy shared goal", "kind": "travel", "notes": "Keep these notes"}},
	})
	owner := v.Str(v.At(engine.Snapshot(), "planWorkspace", "key"))
	if owner == "" {
		t.Fatal("initial journal did not activate its owner")
	}
	create := func(name string) {
		doOK(t, engine, "/api/objectives", M{"action": "add", "title": name})
	}
	create("Owner A")
	doOK(t, engine, "/api/objectives", M{"action": "update", "id": "legacy-goal", "scope": "personal", "expectedPlanWorkspace": owner})
	first := v.Rows(engine.SnapshotForUI("targets")["objectives"])
	if len(first) != 2 || first[0]["notes"] != "Keep these notes" || first[0]["scope"] != "personal" {
		t.Fatalf("legacy assignment changed goal content: %v", first)
	}

	engine.Apply(testEvent("Commander", M{"FID": "FSECOND", "Name": "Synthetic Commander"}), nil)
	if goals := v.Rows(engine.SnapshotForUI("hud")["objectives"]); len(goals) != 0 {
		t.Fatalf("another commander inherited goals in HUD: %v", goals)
	}
	if _, err := engine.Do("/api/objectives", M{"action": "add", "title": "Stale form", "expectedPlanWorkspace": owner}); err == nil {
		t.Fatal("action from a stale workspace was accepted")
	}
	if _, err := engine.Do("/api/objectives", M{"action": "remove", "id": "legacy-goal"}); err == nil {
		t.Fatal("another commander removed a personal goal by ID")
	}
	create("Owner B")

	engine.Apply(testEvent("Commander", M{"FID": "FSYNTHETIC", "Name": "Synthetic Commander"}), nil)
	if goals := v.Rows(engine.Snapshot()["objectives"]); len(goals) != 2 || goals[1]["title"] != "Owner A" {
		t.Fatalf("original workspace goals did not return: %v", goals)
	}
	doOK(t, engine, "/api/objectives", M{"action": "update", "id": "legacy-goal", "scope": "shared", "expectedPlanWorkspace": owner})
	engine.Apply(testEvent("Fileheader", M{"gameversion": "3.8.0"}), nil)
	engine.Apply(testEvent("Commander", M{"FID": "FSYNTHETIC", "Name": "Synthetic Commander"}), nil)
	if got := engine.SnapshotForUI("targets"); v.At(got, "planWorkspace", "branch") != "legacy" || len(v.Rows(got["objectives"])) != 1 || v.Rows(got["objectives"])[0]["scope"] != "shared" {
		t.Fatalf("legacy branch inherited Live goals: %v", got["objectives"])
	}

	archive := filepath.Join(filepath.Dir(dir), "portable.json")
	copyDir := filepath.Join(filepath.Dir(dir), "restored")
	if err := engine.ExportProfile(archive); err != nil {
		t.Fatal(err)
	}
	if err := profile.ImportPortable(archive, copyDir); err != nil {
		t.Fatal(err)
	}
	restored, err := New(Options{DataDir: copyDir, JournalDir: filepath.Dir(journalFile), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if goals := v.Rows(restored.SnapshotForUI("targets")["objectives"]); len(goals) != 2 || goals[0]["scope"] != "shared" || goals[1]["title"] != "Owner A" {
		t.Fatalf("portable restoration lost scoped plans: %v", goals)
	}
	restored.Apply(testEvent("Commander", M{"FID": "FSECOND", "Name": "Synthetic Commander"}), nil)
	if goals := v.Rows(restored.SnapshotForUI("targets")["objectives"]); len(goals) != 2 || goals[1]["title"] != "Owner B" {
		t.Fatalf("portable restoration lost inactive owner's goal: %v", goals)
	}
}

func TestUnconfirmedWorkspaceKeepsPersonalPlansPrivate(t *testing.T) {
	engine, _, _ := fixtureEngine(t, nil)
	doOK(t, engine, "/api/objectives", M{"action": "add", "title": "Private"})
	engine.resetObserved()
	engine.Apply(testEvent("Fileheader", M{"gameversion": "4.0 beta"}), nil)
	engine.Apply(testEvent("Commander", M{"FID": "FSYNTHETIC", "Name": "Synthetic Commander"}), nil)
	if goals := v.Rows(engine.SnapshotForUI("hud")["objectives"]); len(goals) != 0 {
		t.Fatalf("beta journal activated private plans: %v", goals)
	}
	engine.Apply(testEvent("Fileheader", M{"gameversion": "4.0.0"}), nil)
	engine.Apply(testEvent("Commander", M{"FID": "FSYNTHETIC", "Name": "Synthetic Commander"}), M{"side": true})
	if _, err := engine.Do("/api/objectives", M{"action": "add", "title": "Unconfirmed", "scope": "personal"}); err == nil {
		t.Fatal("side file activated private plans")
	}
}

func TestCommanderPlansRemainVisibleAcrossJournalContinuation(t *testing.T) {
	engine, journalFile, _ := fixtureEngine(t, nil)
	owner := v.Str(v.At(engine.SnapshotForUI("targets"), "planWorkspace", "key"))
	doOK(t, engine, "/api/objectives", M{"action": "add", "title": "Private objective"})
	continuation := strings.TrimSuffix(journalFile, ".01.log") + ".02.log"
	writeEvents(t, continuation, false, []M{
		testEvent("Fileheader", M{"gameversion": "4.4.2", "part": 2}),
		testEvent("Location", M{"StarSystem": "Continuation"}),
	})
	assertOwner := func() {
		t.Helper()
		got := engine.SnapshotForUI("targets")
		if v.At(got, "planWorkspace", "key") != owner || len(v.Rows(got["objectives"])) != 1 {
			t.Fatalf("journal continuation lost the workspace owner: %v", got["planWorkspace"])
		}
	}
	engine.Reader.Poll()
	assertOwner()
	engine.Reader.Rescan()
	engine.Reader.Poll()
	assertOwner()
}
