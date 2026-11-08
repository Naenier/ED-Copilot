package mining

import (
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestStatisticsPeriodStartsAtTrackedJournalTimeWithoutMining(t *testing.T) {
	s := New(nil)
	ctx := M{"file": "Journal.synthetic-session"}
	s.Apply(me("LoadGame", "2026-11-07T12:00:00Z", nil), ctx)
	s.Apply(me("Docked", "2026-11-07T13:00:00Z", nil), ctx)
	stats := v.Obj(s.SnapshotForStatistics(M{"now": "2026-11-08T20:00:00Z"})["stats"])
	if stats["since"] != "2026-11-07T12:00:00Z" || v.Num(stats["elapsedSeconds"]) != 3600 {
		t.Fatal("journal clock and period start disagree", stats)
	}
	if s.FirstStatsEvent != "" || v.Num(v.At(stats, "total", "refinedTons")) != 0 || v.Num(v.At(stats, "total", "materialUnits")) != 0 {
		t.Fatal("non-mining session invented mining observations", stats)
	}
}

func TestStatisticsPeriodUsesEarliestIntervalAndPreservesOverlapAccounting(t *testing.T) {
	s := New(nil)
	for _, interval := range []struct{ file, from, to string }{
		{"later", "2026-11-07T12:00:00Z", "2026-11-07T13:00:00Z"},
		{"earlier", "2026-11-07T10:00:00Z", "2026-11-07T10:30:00Z"},
		{"overlapping", "2026-11-07T12:30:00Z", "2026-11-07T13:30:00Z"},
	} {
		s.Apply(me("LoadGame", interval.from, nil), M{"file": interval.file})
		s.Apply(me("Shutdown", interval.to, nil), M{"file": interval.file})
	}
	s.Apply(me("MiningRefined", "2026-11-07T12:45:00Z", M{"Type": "Gold", "Count": 1}), M{"file": "overlapping"})
	stats := v.Obj(s.SnapshotForStatistics(nil)["stats"])
	if stats["since"] != "2026-11-07T10:00:00Z" || s.FirstStatsEvent != "2026-11-07T12:45:00Z" {
		t.Fatal("period was narrowed to the first mining event", stats["since"])
	}
	if v.Num(stats["elapsedSeconds"]) != 7200 || stats["timeBasis"] != "journal-sessions" || v.Num(v.At(stats, "total", "refinedTons")) != 1 {
		t.Fatal("existing elapsed time or counters changed", stats)
	}
}

func TestStatisticsPeriodDoesNotInventAStartAndKeepsSingleEventTime(t *testing.T) {
	s := New(nil)
	s.Apply(me("LoadGame", "invalid-date", nil), M{"file": "Journal.invalid"})
	stats := v.Obj(s.SnapshotForStatistics(M{"now": "2026-11-08T20:00:00Z"})["stats"])
	if stats["since"] != nil || v.Num(stats["elapsedSeconds"]) != 0 {
		t.Fatal("empty or invalid observations invented a date", stats)
	}
	s.Apply(me("LoadGame", "2026-11-07T12:00:00Z", nil), M{"file": "Journal.one-event"})
	stats = v.Obj(s.SnapshotForStatistics(M{"statsSince": "not-a-reset-date"})["stats"])
	if stats["since"] != "2026-11-07T12:00:00Z" || v.Num(stats["elapsedSeconds"]) != 0 {
		t.Fatal("single valid observation lost its start", stats)
	}
	stats = v.Obj(s.SnapshotForStatistics(M{"statsSince": "2026-11-07T11:30:00Z"})["stats"])
	if stats["since"] != "2026-11-07T11:30:00Z" {
		t.Fatal("explicit period boundary was ignored", stats["since"])
	}
}

func TestStatisticsResetBoundarySurvivesRescanWithoutOldCounters(t *testing.T) {
	s := New(nil)
	s.Apply(me("MiningRefined", "2026-11-07T11:00:00Z", M{"Type": "Gold", "Count": 20}), M{"file": "Journal.before-reset"})
	s.Reset() // The journal rescan clears observations and replays the period.
	for _, event := range []M{
		me("LoadGame", "2026-11-07T11:00:00Z", nil),
		me("MiningRefined", "2026-11-07T11:30:00Z", M{"Type": "Gold", "Count": 20}),
	} {
		s.Apply(event, M{"file": "Journal.before-reset", "countInPeriod": false})
	}
	for _, event := range []M{
		me("LoadGame", "2026-11-07T13:00:00Z", nil),
		me("MiningRefined", "2026-11-07T13:30:00Z", M{"Type": "Gold", "Count": 3}),
		me("Docked", "2026-11-07T14:00:00Z", nil),
	} {
		s.Apply(event, M{"file": "Journal.after-reset", "countInPeriod": true})
	}
	stats := v.Obj(s.SnapshotForStatistics(M{"statsSince": "2026-11-07T12:30:00Z"})["stats"])
	if stats["since"] != "2026-11-07T12:30:00Z" || v.Num(stats["elapsedSeconds"]) != 3600 || v.Num(v.At(stats, "total", "refinedTons")) != 3 {
		t.Fatal("rescan used pre-reset observations or hid the reset date", stats)
	}
}
