package colonization

import (
	"encoding/json"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const historyAddressA = "9007199254740997"
const historyAddressB = "9007199254740998"

func TestColonyHistoryPreservesSparseDatedLiveObservations(t *testing.T) {
	now := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	p := New(M{"projects": []M{
		{"id": "alpha", "targetSystem": "Alpha", "systemAddress": historyAddressA, "status": "building"},
		{"id": "beta", "targetSystem": "Beta", "systemAddress": historyAddressB, "status": "complete"},
	}}, WithClock(func() time.Time { return now }))
	ctx := M{"game": M{"live": true}, "replay": true}
	observe := func(name, address, at string, population any, context M) {
		p.Apply(M{"event": "FSDJump", "StarSystem": name, "SystemAddress": json.Number(address), "timestamp": at,
			"Population": population, "SystemEconomy": "$economy_HighTech;", "SystemSecurity": "$SYSTEM_SECURITY_low;",
			"SystemFaction": M{"Name": "Faction A", "FactionState": "Boom"},
			"Factions":      []M{{"Name": "Faction A", "Influence": 0.3, "Happiness": "$Faction_HappinessBand2;"}},
		}, context)
	}
	observe("Alpha", historyAddressA, "2026-11-09T07:00:00Z", 0, ctx)
	observe("Alpha", historyAddressA, "2026-11-11T07:00:00Z", 1300, ctx)
	observe("Alpha", historyAddressA, "2026-11-11T07:00:00Z", 1300, ctx)
	observe("Alpha", historyAddressB, "2026-11-11T10:00:00Z", 4, ctx)
	observe("Alpha", historyAddressA, "2026-11-11T10:00:00Z", 9, M{"game": M{"live": false}})
	observe("Alpha", historyAddressA, "2026-11-11T10:00:00Z", 9, M{"game": M{"live": true, "multicrew": true}})
	observe("Beta", historyAddressB, "2026-11-10T15:00:00Z", 270, ctx)
	target := HistoryTarget{"alpha", "Alpha", historyAddressA}
	public := M{"name": "Alpha", "address": historyAddressA, "info": M{"observedAt": "2026-11-11T07:00:00Z", "population": 2, "secondEconomy": "Refinery"}}
	if !p.ObservePublicSystem(target, public) || p.ObservePublicSystem(target, public) {
		t.Fatal("same-date public evidence must fill missing fields exactly once")
	}
	public["address"] = historyAddressB
	if p.ObservePublicSystem(target, public) {
		t.Fatal("mismatched ID64 accepted")
	}
	public["address"] = historyAddressA
	public["info"] = M{"fetchedAt": "2026-11-12T07:00:00Z", "population": 2500}
	if p.ObservePublicSystem(target, public) {
		t.Fatal("download time manufactured a trend point")
	}
	observe("Alpha", historyAddressA, "2026-11-12T08:00:00Z", nil, ctx)
	result := rows(p.SystemHistorySnapshot()["samples"])
	if len(result) != 3 || result[0]["population"] == nil || num(result[0]["population"]) != 0 || num(result[1]["population"]) != 1300 ||
		result[1]["secondEconomy"] != "Refinery" || result[1]["economy"] != "High Tech" || result[1]["security"] != "Low" || result[2]["population"] != nil {
		t.Fatalf("unexpected timeline: %+v", result)
	}
	if faction := rows(result[1]["factions"])[0]; faction["happiness"] != "Happy" {
		t.Fatalf("journal happiness enum failed to normalize: %+v", faction)
	}
	reloaded := New(p.ExportSettings(), WithClock(func() time.Time { return now }))
	if actual := rows(reloaded.SystemHistorySnapshot()["samples"]); len(actual) != 3 || actual[2]["population"] != nil {
		t.Fatalf("history did not survive restart: %+v", actual)
	}
	action(t, reloaded, M{"action": "select-project", "id": "beta"})
	result = rows(reloaded.SystemHistorySnapshot()["samples"])
	if len(result) != 1 || num(result[0]["population"]) != 270 {
		t.Fatalf("colony history leaked into another project: %+v", result)
	}
}

func TestColonyHistoryKeepsLongTermSamplesBounded(t *testing.T) {
	now := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	p := New(M{"projects": []M{{"id": "alpha", "targetSystem": "Alpha", "status": "building"}}}, WithClock(func() time.Time { return now }))
	target := HistoryTarget{"alpha", "Alpha", historyAddressA}
	for day := 179; day >= 0; day-- {
		for step := 0; step < 8; step++ {
			at := now.Add(-time.Duration(day+1)*24*time.Hour + time.Duration(step)*historyPeriod)
			p.addHistoryLocked(target, M{"at": v.ISO(at), "source": "journal", "population": day + step})
		}
	}
	rows := rows(p.SystemHistorySnapshot()["samples"])
	if len(rows) > historyPerColony || len(rows) < 300 || now.Sub(v.Time(rows[0]["at"])) < 120*24*time.Hour {
		t.Fatalf("history range lost granularity or bound: %d samples, first %v", len(rows), rows[0]["at"])
	}
}

func TestColonyMilestonesPreferConfirmedCompletionOverDocking(t *testing.T) {
	now := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	p := New(M{}, WithClock(func() time.Time { return now }))
	ctx := M{"game": M{"live": true}, "commander": M{"system": "Origin"}}
	p.Apply(M{"event": "LoadGame", "FID": "test-commander"}, ctx)
	p.Apply(M{"event": "ColonisationSystemClaim", "SystemName": "Alpha", "SystemAddress": historyAddressA, "timestamp": "2026-11-01T10:00:00Z"}, ctx)
	dock := M{"event": "Docked", "StarSystem": "Alpha", "SystemAddress": historyAddressA, "StationName": "Test Port", "StationType": "Coriolis", "MarketID": 42, "timestamp": "2026-11-06T10:00:00Z"}
	p.Apply(dock, ctx)
	got := rows(p.SystemHistorySnapshot()["milestones"])
	if len(got) != 1 || got[0]["kind"] != "first-visit" {
		t.Fatalf("docking should only establish a visit: %+v", got)
	}
	depot := M{"event": "ColonisationConstructionDepot", "MarketID": 42, "SystemAddress": historyAddressA, "timestamp": "2026-11-05T09:00:00Z",
		"StarSystem": "Alpha", "StationName": "Test Port", "ConstructionComplete": true, "ResourcesRequired": []M{}}
	p.Apply(depot, ctx)
	p.Apply(dock, ctx)
	p.Apply(depot, ctx)
	reloaded := New(p.ExportSettings(), WithClock(func() time.Time { return now }))
	got = rows(reloaded.SystemHistorySnapshot()["milestones"])
	if len(got) != 1 || got[0]["kind"] != "completion" || got[0]["at"] != "2026-11-05T09:00:00Z" {
		t.Fatalf("completion was not deduplicated or kept: %+v", got)
	}
}
