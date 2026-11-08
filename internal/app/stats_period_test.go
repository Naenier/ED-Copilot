package app

import (
	"path/filepath"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestStatisticsSnapshotsShareObservedJournalStartWithoutMining(t *testing.T) {
	engine, file, _ := fixtureEngine(t, nil)
	engine.Apply(testEvent("Docked", M{"timestamp": "2026-11-07T13:00:00Z"}), M{"file": filepath.Base(file), "replay": true})
	for name, snapshot := range map[string]M{"full": engine.Snapshot(), "statistics": engine.SnapshotForUI("statistics"), "mining": engine.SnapshotForUI("mining")} {
		stats := v.Obj(snapshot["stats"])
		if name == "mining" {
			stats = v.Obj(v.At(snapshot, "mining", "stats"))
		}
		if stats["since"] != "2026-11-07T12:00:00Z" || v.Num(stats["elapsedSeconds"]) != 3600 || v.Num(v.At(stats, "total", "refinedTons")) != 0 {
			t.Fatal(name, "disagrees about the non-mining period", stats)
		}
	}
}

func TestStatisticsSnapshotsKeepSavedResetBoundaryAndExcludeOlderEvents(t *testing.T) {
	const cutoff = "2026-11-07T12:30:00Z"
	engine, file, _ := fixtureEngine(t, M{"statsSince": cutoff})
	for _, event := range []M{
		testEvent("MiningRefined", M{"timestamp": "2026-11-07T12:15:00Z", "Type": "Gold", "Count": 20}),
		testEvent("LoadGame", M{"timestamp": "2026-11-07T13:00:00Z"}),
		testEvent("MiningRefined", M{"timestamp": "2026-11-07T13:30:00Z", "Type": "Gold", "Count": 3}),
		testEvent("Docked", M{"timestamp": "2026-11-07T14:00:00Z"}),
	} {
		engine.Apply(event, M{"file": filepath.Base(file), "replay": true})
	}
	for name, snapshot := range map[string]M{"full": engine.Snapshot(), "statistics": engine.SnapshotForUI("statistics"), "mining": engine.SnapshotForUI("mining")} {
		stats := v.Obj(snapshot["stats"])
		if name == "mining" {
			stats = v.Obj(v.At(snapshot, "mining", "stats"))
		}
		if stats["since"] != cutoff || v.Num(stats["elapsedSeconds"]) != 3600 || v.Num(v.At(stats, "total", "refinedTons")) != 3 {
			t.Fatal(name, "lost the saved reset boundary", stats)
		}
	}
}
