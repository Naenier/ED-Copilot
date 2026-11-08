package politics

import (
	"context"
	"encoding/json"
	"fmt"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testClock() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) }
func jump() M {
	return M{"event": "FSDJump", "timestamp": "2026-11-08T11:00:00Z", "StarSystem": "Fixture", "SystemAddress": json.Number("9007199254740999"), "SystemFaction": M{"Name": "Civic League", "FactionState": "Election"}, "Factions": []M{{"Name": "Civic League", "Influence": .42, "FactionState": "Election", "PendingStates": []M{{"State": "Boom", "Trend": 1}}}, {"Name": "Unknown Influence", "FactionState": "None"}}, "Conflicts": []M{{"WarType": "election", "Status": "active", "Faction1": M{"Name": "Civic League", "WonDays": 0}, "Faction2": M{"Name": "Other League", "Stake": "Outpost", "WonDays": 2}}}, "PowerplayState": "Contested", "Powers": []string{"Power A", "Power B"}, "ThargoidWar": M{"CurrentState": "Invasion", "WarProgress": .25, "RemainingPorts": 0}}
}

func TestJournalSeparatesUnknownFromZeroAndReportsActualPairings(t *testing.T) {
	s := New(nil, Options{Now: testClock})
	s.Apply(jump())
	state := s.Snapshot()
	active := v.Obj(state["activeSystem"])
	if active["address"] != "9007199254740999" {
		t.Fatal("system address lost precision", active["address"])
	}
	fs := v.Rows(v.At(active, "factions", "entries"))
	if len(fs) != 2 || v.Num(fs[0]["influence"]) != .42 || fs[1]["influence"] != nil {
		t.Fatal("unknown faction influence became zero", fs)
	}
	pair := v.Rows(v.At(active, "conflicts", "entries"))[0]
	if v.At(pair, "faction1", "wonDays") == nil || v.Num(v.At(pair, "faction2", "wonDays")) != 2 {
		t.Fatal("observed conflict scores lost", pair)
	}
	if v.At(active, "thargoid", "remainingPorts") == nil || v.Num(v.At(active, "thargoid", "progress")) != .25 {
		t.Fatal("known zero and progress lost")
	}
	if v.At(active, "powerplay", "controllingPower") != "" {
		t.Fatal("inferred a controlling power from contesting powers")
	}
	// Snapshot containers never expose mutable internal observations.
	fs[0]["name"] = "mutated"
	if v.Rows(v.At(s.Snapshot(), "activeSystem", "factions", "entries"))[0]["name"] == "mutated" {
		t.Fatal("snapshot changed stored observations")
	}
}

func TestNewerDomainWinsAndMissingConflictDataDoesNotInventPeace(t *testing.T) {
	s := New(nil, Options{Now: testClock})
	s.Apply(jump())
	s.mu.Lock()
	s.mergeLocked(NormalizePublicSystem(M{"name": "Fixture", "id64": json.Number("9007199254740999"), "updated_at": "2026-11-08T10:00:00Z", "minor_faction_presences": []M{{"name": "Old public faction", "influence": .9}}, "power_state": "Stronghold"}, s.now()))
	s.mu.Unlock()
	active := v.Obj(s.Snapshot()["activeSystem"])
	if v.Rows(v.At(active, "factions", "entries"))[0]["name"] != "Civic League" || v.At(active, "powerplay", "state") != "Contested" {
		t.Fatal("old public snapshot overrode journal")
	}
	if len(v.Rows(v.At(active, "conflicts", "entries"))) != 1 {
		t.Fatal("public absence erased an observed conflict")
	}
	sparse := jump()
	sparse["timestamp"] = "2026-11-08T11:30:00Z"
	delete(sparse, "Factions")
	delete(sparse, "Conflicts")
	s.Apply(sparse)
	if len(v.Rows(v.At(s.Snapshot(), "activeSystem", "conflicts", "entries"))) != 1 {
		t.Fatal("unobserved conflicts became an empty list")
	}
	sparse["timestamp"] = "2026-11-08T11:45:00Z"
	sparse["Conflicts"] = []M{}
	s.Apply(sparse)
	if len(v.Rows(v.At(s.Snapshot(), "activeSystem", "conflicts", "entries"))) != 0 || v.At(s.Snapshot(), "activeSystem", "conflicts", "known") != true {
		t.Fatal("explicit empty observation not recorded")
	}
}

func TestPersonalGoalsAreNotPublicCampaignsAndCommanderChangeClearsThem(t *testing.T) {
	s := New(nil, Options{Now: testClock})
	s.Apply(M{"event": "LoadGame", "FID": "A"})
	s.Apply(M{"event": "CommunityGoal", "timestamp": "2026-11-08T11:00:00Z", "CurrentGoals": []M{{"CGID": 7, "Title": "A fictional goal", "PlayerContribution": 0, "Bonus": 500, "PlayerPercentileBand": 75}}})
	state := s.Snapshot()
	if v.Bool(v.At(state, "communityGoals", "known")) || len(v.Rows(v.At(state, "personalGoals", "entries"))) != 1 {
		t.Fatal("journal goals treated as complete public catalogue")
	}
	row := v.Rows(v.At(state, "personalGoals", "entries"))[0]
	if row["playerContribution"] == nil || v.Num(row["bonus"]) != 500 {
		t.Fatal("goal details lost")
	}
	s.Apply(M{"event": "CommunityGoalReward", "CGID": 7, "Reward": 600, "timestamp": "2026-11-08T11:15:00Z"})
	if v.Num(v.Rows(v.At(s.Snapshot(), "personalGoals", "entries"))[0]["rewardReceived"]) != 600 {
		t.Fatal("actual reward not recorded")
	}
	s.Apply(M{"event": "LoadGame", "FID": "B"})
	if len(v.Rows(v.At(s.Snapshot(), "personalGoals", "entries"))) != 0 {
		t.Fatal("another commander's personal goal leaked")
	}
}

func TestStaleGoalMutationsCannotRollBackNewerSnapshotsOrAnotherGoal(t *testing.T) {
	s := New(nil, Options{Now: testClock})
	s.Apply(M{"event": "CommunityGoal", "timestamp": "2026-11-08T12:00:00Z", "CurrentGoals": []M{{"CGID": 1, "Title": "First", "IsComplete": false, "PlayerContribution": 250}, {"CGID": 2, "Title": "Second", "IsComplete": false}}})
	before := s.Snapshot()["personalGoals"]
	for _, event := range []string{"CommunityGoalReward", "CommunityGoalDiscard", "CommunityGoalJoin"} {
		s.Apply(M{"event": event, "timestamp": "2026-11-08T11:59:00Z", "CGID": 1, "Reward": 999})
		if !reflect.DeepEqual(before, s.Snapshot()["personalGoals"]) {
			t.Fatal("older mutation rolled back a newer goal snapshot", event)
		}
	}
	s.Apply(M{"event": "CommunityGoalReward", "timestamp": "2026-11-08T12:30:00Z", "CGID": 1, "Reward": 600})
	s.Apply(M{"event": "CommunityGoalDiscard", "timestamp": "2026-11-08T12:15:00Z", "CGID": 1})
	first := v.Rows(v.At(s.Snapshot(), "personalGoals", "entries"))[0]
	if v.Bool(first["discarded"]) || v.Num(first["rewardReceived"]) != 600 || first["observedAt"] != "2026-11-08T12:30:00Z" {
		t.Fatal("older discard replaced a newer reward", first)
	}
	// Goal two's newer observation is valid even though goal one's reward is
	// later; the envelope must keep its most recent timestamp.
	s.Apply(M{"event": "CommunityGoalDiscard", "timestamp": "2026-11-08T12:15:00Z", "CGID": 2})
	state := s.Snapshot()
	second := v.Rows(v.At(state, "personalGoals", "entries"))[1]
	if !v.Bool(second["discarded"]) || v.At(state, "personalGoals", "observedAt") != "2026-11-08T12:30:00Z" {
		t.Fatal("per-goal ordering or envelope time is wrong", state["personalGoals"])
	}
}

func TestJournalEnumsPreferCanonicalRawTokensOverLocalizedDisplayText(t *testing.T) {
	s := New(nil, Options{Now: testClock})
	e := jump()
	e["SystemGovernment"], e["SystemGovernment_Localised"] = "$government_Democracy;", "Демократия"
	e["SystemSecurity"], e["SystemSecurity_Localised"] = "$SYSTEM_SECURITY_medium;", "Средняя"
	s.Apply(e)
	active := v.Obj(s.Snapshot()["activeSystem"])
	if v.At(active, "info", "government") != "Democracy" || v.At(active, "info", "security") != "Medium" {
		t.Fatal("localized labels replaced canonical enums", active["info"])
	}
	if v.At(active, "info", "controllingFaction") != "Civic League" {
		t.Fatal("faction identity changed")
	}
	if canonicalEnum("$GAlAXY_MAP_INFO_state_high;", "Высокая") != "High" || canonicalEnum("$government_FutureGovernance;", "Неизвестное правительство") != "FutureGovernance" {
		t.Fatal("alternate token or unknown enum lost")
	}
}

func TestBoundedSystemCacheAndOfflineReload(t *testing.T) {
	s := New(nil, Options{Now: testClock})
	for i := 0; i < 70; i++ {
		e := jump()
		e["StarSystem"] = fmt.Sprintf("Fixture %03d", i)
		s.Apply(e)
	}
	if len(v.Rows(s.Snapshot()["systems"])) != MaxSystems {
		t.Fatal("system cache not bounded")
	}
	saved := s.ExportSettings()
	restored := New(saved, Options{Now: testClock, DisableNetwork: true})
	if len(v.Rows(restored.Snapshot()["systems"])) != MaxSystems {
		t.Fatal("offline saved observations lost")
	}
	r, err := restored.Fetch(context.Background(), M{"kind": "goals"})
	if err != nil || r["status"] != "offline" {
		t.Fatal("offline refresh attempted network", r, err)
	}
}

func fixtureServer(t *testing.T, now *time.Time) (*Service, *atomic.Int32, *atomic.Bool) {
	t.Helper()
	count := &atomic.Int32{}
	fail := &atomic.Bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		if fail.Load() {
			http.Error(w, "unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/goals":
			if r.Header.Get("Accept") != "application/json" || r.URL.Query().Get("lang") != "en" {
				t.Error("public goals request lacks explicit JSON/language")
			}
			fmt.Fprint(w, `{"activeInitiatives":[{"id":"1","title":"Fictional campaign","starsystem_name":"Fixture","market_name":"Port","qty":"0","target_qty":"1000","expiry":"2026-11-19 11:00:00","objective":"Deliver supplies","bulletin":"Fictional reward text {{top5}}"}]}`)
		case "/systems/field_values/system_names":
			fmt.Fprint(w, `{"min_max":[{"name":"Fixture","id64":9007199254740999}]}`)
		case "/system/9007199254740999":
			fmt.Fprint(w, `{"record":{"name":"Fixture","id64":9007199254740999,"updated_at":"2026-11-01T09:00:00Z","minor_faction_presences":[{"name":"Public faction","state":"War","active_states":["War"]}],"controlling_power":"Power A","power_state":"Stronghold","thargoid_war_state":"None"}}`)
		case "/systems/field_values/controlling_power":
			fmt.Fprint(w, `{"values":["Power A","Power B"],"min_max":{"Power A":12,"Power B":0}}`)
		case "/systems/field_values/power_state":
			fmt.Fprint(w, `{"values":["Stronghold","Fortified","Unoccupied"],"min_max":{"Stronghold":2,"Fortified":3,"Unoccupied":40}}`)
		case "/systems/field_values/thargoid_war_state":
			fmt.Fprint(w, `{"values":["None"],"min_max":{"None":5000}}`)
		case "/systems/search":
			var input M
			_ = json.NewDecoder(r.Body).Decode(&input)
			if v.Num(input["size"]) != 10 || v.Int(input["page"]) > 100 {
				t.Error("unbounded search", input)
			}
			fmt.Fprint(w, `{"count":22,"results":[{"name":"Public result","id64":42,"updated_at":"2026-11-08T10:00:00Z","controlling_power":"Power A","power_state":"Contested"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return New(nil, Options{HTTPClient: server.Client(), SpanshURL: server.URL, GoalsURL: server.URL + "/goals", Now: func() time.Time { return *now }}), count, fail
}

func TestPublicRequestsCacheAndFailedRefreshPreservesLastData(t *testing.T) {
	now := testClock()
	s, count, fail := fixtureServer(t, &now)
	if _, err := s.Fetch(context.Background(), M{"kind": "system", "system": "Fixture"}); err != nil {
		t.Fatal(err)
	}
	first := count.Load()
	if first != 2 {
		t.Fatal("expected name resolution and one record", first)
	}
	if v.At(s.Snapshot(), "activeSystem", "info", "stale") != true {
		t.Fatal("recent retrieval concealed an old observation")
	}
	if _, err := s.Fetch(context.Background(), M{"kind": "system", "system": "Fixture"}); err != nil || count.Load() != first {
		t.Fatal("cache miss", err)
	}
	if _, err := s.Fetch(context.Background(), M{"kind": "goals"}); err != nil {
		t.Fatal(err)
	}
	g := v.Rows(v.At(s.Snapshot(), "communityGoals", "entries"))[0]
	if g["currentTotal"] == nil || g["target"] == nil || strings.Contains(v.Str(g["description"]), "{{top5}}") {
		t.Fatal("public goal details invalid", g)
	}
	before := v.Str(v.At(s.Snapshot(), "communityGoals", "fetchedAt"))
	now = now.Add(6 * time.Minute)
	fail.Store(true)
	if _, err := s.Fetch(context.Background(), M{"kind": "goals"}); err == nil {
		t.Fatal("failed HTTP was accepted")
	}
	after := s.Snapshot()
	if v.Str(v.At(after, "communityGoals", "fetchedAt")) != before || v.At(after, "status", "running") != false {
		t.Fatal("failure replaced successful observations", after)
	}
	tries := count.Load()
	if _, err := s.Fetch(context.Background(), M{"kind": "goals"}); err == nil || count.Load() != tries {
		t.Fatal("failed refresh has no retry backoff")
	}
}

func TestPowerIndexPaginationAndThargoidNoAffectedIndexAreDifferentFromUnknown(t *testing.T) {
	now := testClock()
	s, count, _ := fixtureServer(t, &now)
	if _, err := s.Fetch(context.Background(), M{"kind": "powerplay", "power": "Power A", "state": "Contested", "page": 1}); err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if len(v.Rows(v.At(snap, "powerIndex", "powers"))) != 2 || v.At(snap, "search", "total") == nil || v.Num(v.At(snap, "search", "page")) != 1 {
		t.Fatal("index or pagination lost", snap)
	}
	if _, err := s.Fetch(context.Background(), M{"kind": "thargoid"}); err != nil {
		t.Fatal(err)
	}
	snap = s.Snapshot()
	if v.At(snap, "search", "indexHasAffectedStates") != false || v.At(snap, "search", "total") == nil || v.Num(v.At(snap, "search", "total")) != 0 {
		t.Fatal("known-empty index not distinguished from unknown", snap)
	}
	// A previous query's TTL must not return the other view's result.
	before := count.Load()
	if _, err := s.Fetch(context.Background(), M{"kind": "powerplay", "power": "Power A", "state": "Contested", "page": 1}); err != nil || count.Load() == before || v.At(s.Snapshot(), "search", "kind") != "powerplay" {
		t.Fatal("different cached search masked requested view", err)
	}
	if err := s.Update(M{"action": "select-system", "system": "Public result"}); err != nil || v.At(s.Snapshot(), "activeSystem", "powerplay", "state") != "Contested" {
		t.Fatal("result system cannot be opened", err)
	}
}

func TestHTTPBoundaryRejectsOversizeRedirectMalformedAndMissingShape(t *testing.T) {
	for _, mode := range []string{"large", "redirect", "malformed", "shape"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				switch mode {
				case "large":
					fmt.Fprint(w, strings.Repeat("x", maxResponseBytes+1))
				case "redirect":
					http.Redirect(w, r, "/other", http.StatusFound)
				case "malformed":
					fmt.Fprint(w, `{"activeInitiatives":[]} trailing`)
				case "shape":
					fmt.Fprint(w, `{}`)
				}
			}))
			defer server.Close()
			s := New(nil, Options{HTTPClient: server.Client(), GoalsURL: server.URL})
			if _, err := s.Fetch(context.Background(), M{"kind": "goals"}); err == nil {
				t.Fatal("invalid response accepted")
			}
			if calls != 1 {
				t.Fatal("redirect followed or automatic retry", calls)
			}
		})
	}
}

func TestRefreshRejectsInvalidFiltersBeforeNetworkAndDoesNotHoldObservationLock(t *testing.T) {
	now := testClock()
	s, count, _ := fixtureServer(t, &now)
	for _, body := range []M{{"kind": "system", "system": 42}, {"kind": "goals", "state": strings.Repeat("a", 101)}, {"kind": "system", "system": "line\nbreak"}, {"kind": "powerplay", "page": 1.5}, {"kind": "powerplay", "page": 101}} {
		if _, err := s.Fetch(context.Background(), body); err == nil {
			t.Fatal("invalid filters accepted", body)
		}
	}
	if count.Load() != 0 {
		t.Fatal("invalid filters caused public requests")
	}
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		fmt.Fprint(w, `{"activeInitiatives":[]}`)
	}))
	defer server.Close()
	blocked := New(nil, Options{HTTPClient: server.Client(), GoalsURL: server.URL, Now: testClock})
	done := make(chan error, 1)
	go func() { _, err := blocked.Fetch(context.Background(), M{"kind": "goals"}); done <- err }()
	<-entered
	observed := make(chan struct{})
	go func() { blocked.Apply(jump()); _ = blocked.Snapshot(); close(observed) }()
	select {
	case <-observed:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("network request blocked journal application or UI snapshot")
	}
	if _, err := blocked.Fetch(context.Background(), M{"kind": "goals"}); err == nil {
		close(release)
		t.Fatal("concurrent refresh not bounded")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
