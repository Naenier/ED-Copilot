package freshness

import (
	"testing"
	"time"
)

func TestAgeAndCoverageAreIndependent(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	old := now.Add(-time.Hour)
	cases := []struct {
		name              string
		known             bool
		coverage          Coverage
		observed, fetched time.Time
		want              State
	}{
		{"unknown", false, "", time.Time{}, now, Unknown},
		{"confirmed empty", true, Empty, time.Time{}, now, Fresh},
		{"partial old observation despite recent retrieval", true, Partial, old, now, Stale},
		{"dated result with no download", true, Complete, now, time.Time{}, Fresh},
		{"future timestamp", true, Complete, now.Add(time.Hour), now, Undated},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Describe(now, 30*time.Minute, c.known, c.coverage, c.observed, c.fetched)
			if got.State != c.want || got.Coverage != c.coverage {
				t.Fatal(got)
			}
		})
	}
}
