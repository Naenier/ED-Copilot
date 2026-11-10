package app

import (
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestEngineeringRoadmapRefreshTracksJournalObservations(t *testing.T) {
	engine, _, _ := fixtureEngine(t, nil)
	revision := func() uint64 {
		t.Helper()
		state := engine.SnapshotForUI("engineering-roadmap")
		n, ok := state["engineeringRoadmapRevision"].(uint64)
		if !ok || n == 0 || state["engineering"] != nil {
			t.Fatal("progression view must have a revision and avoid the engineering catalogue snapshot")
		}
		return n
	}
	before := revision()
	engine.Apply(testEvent("Music", M{"MusicTrack": "GalaxyMap"}), nil)
	if got := revision(); got != before {
		t.Fatal("unrelated journal event invalidated the roadmap", before, got)
	}
	engine.Apply(testEvent("EngineerProgress", M{"EngineerID": 300100, "Progress": "Unlocked", "Rank": 5}), nil)
	if got := revision(); got <= before {
		t.Fatal("engineer observation did not invalidate the roadmap")
	}

	result, err := engine.EngineeringRoadmap(M{"engineerIDs": []any{"300100"}, "moduleKey": "fsd", "blueprintKey": "FSD_LongRange", "grade": 5})
	if err != nil {
		t.Fatal(err)
	}
	recipe := v.Obj(result["recipe"])
	if len(v.Rows(recipe["grades"])) != 5 || len(v.Rows(recipe["totals"])) == 0 || v.At(recipe, "engineer", "key") != "300100" ||
		v.At(recipe, "engineer", "status") != "unlocked" || v.Int(v.At(recipe, "engineer", "rank")) != 5 {
		t.Fatal("recipe did not use the observed engineer rank", recipe)
	}
	if _, err = engine.EngineeringRoadmap(M{"engineerIDs": []any{"not-an-engineer"}}); err == nil {
		t.Fatal("invalid engineer filter was accepted")
	}
	if _, err = engine.EngineeringRoadmap(M{"expectedPlanWorkspace": "former-owner"}); err == nil {
		t.Fatal("an old workspace could read the active commander's roadmap")
	}
}
