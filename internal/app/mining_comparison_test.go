package app

import (
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestMiningComparisonFollowsJournalThroughReplayAndDesktopSnapshot(t *testing.T) {
	engine, journal, _ := fixtureEngine(t, M{"mining": M{"options": M{"resourceKey": "platinum", "sellPrice": 30000}}})
	writeEvents(t, journal, true, []M{
		testEvent("Undocked", M{"timestamp": "2026-11-07T12:02:00Z"}),
		testEvent("SupercruiseExit", M{"timestamp": "2026-11-07T12:05:00Z", "Body": "Synthetic Origin 1 A Ring", "BodyType": "PlanetaryRing"}),
		testEvent("MiningRefined", M{"timestamp": "2026-11-07T12:06:00Z", "Type": "Platinum", "Count": 80}),
		testEvent("SupercruiseEntry", M{"timestamp": "2026-11-07T12:10:00Z"}),
		testEvent("Docked", M{"timestamp": "2026-11-07T12:12:00Z", "StationName": "Synthetic Station", "MarketID": 42}),
		testEvent("MarketSell", M{"timestamp": "2026-11-07T12:13:00Z", "Type": "Platinum", "Count": 50, "TotalSale": 1400000}),
	})
	engine.Reader.Poll()
	assertRun := func() string {
		t.Helper()
		rows := v.Rows(v.At(engine.SnapshotForUI("mining"), "mining", "history"))
		if len(rows) != 1 {
			t.Fatalf("expected one journal mining run: %v", rows)
		}
		r := rows[0]
		if r["body"] != "Synthetic Origin 1 A Ring" || r["ship"] != "Panther Clipper Mk II" ||
			v.Num(r["confirmedRevenue"]) != 1400000 || v.Num(r["estimatedRemainingValue"]) != 900000 ||
			v.Num(r["remainingTons"]) != 30 || v.Num(r["tripSeconds"]) != 600 {
			t.Fatalf("comparison did not reconcile journal and ledger: %v", r)
		}
		return v.Str(r["id"])
	}
	id := assertRun()
	engine.Reader.Rescan()
	engine.Reader.Poll()
	if got := assertRun(); got == "" || got != id {
		t.Fatalf("run identity changed after replay: %q -> %q", id, got)
	}
}
