package app

import (
	"reflect"
	"sync"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/profile"
	"github.com/Naenier/ED-Copilot/internal/updater"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"path/filepath"
)

func TestRankJournalIntegrationIsScopedBilingualAndSurvivesReplay(t *testing.T) {
	e, journalFile, data := fixtureEngine(t, nil)
	e.Apply(M{"event": "Rank", "timestamp": "2026-11-08T12:00:00Z", "Combat": 2, "Trade": 8}, nil)
	e.Apply(M{"event": "Progress", "timestamp": "2026-11-08T12:01:00Z", "Combat": 0, "Trade": 69}, nil)
	state := e.SnapshotForUI("ranks")
	rows := v.Rows(v.At(state, "ranks", "disciplines"))
	if len(rows) != 8 || rows[0]["rankName"] != "Novice" || v.Num(rows[0]["progressPercent"]) != 0 || rows[0]["progressPercent"] == nil || rows[2]["progressPercent"] != nil {
		t.Fatal("rank or unknown/zero progress was lost", rows)
	}
	if _, exists := e.SnapshotForUI("overview")["ranks"]; exists {
		t.Fatal("rank guide catalogue leaked into dashboard refreshes")
	}
	doOK(t, e, "/api/settings", M{"language": "ru"})
	if title := v.At(e.SnapshotForUI("ranks"), "ranks", "disciplines"); len(v.Rows(title)) != 8 || v.Rows(title)[0]["title"] == "Combat" {
		t.Fatal("rank catalogue ignored saved language")
	}
	if err := e.Save(); err != nil {
		t.Fatal(err)
	}
	e.Close()
	reopened, err := New(Options{DataDir: data, JournalDir: filepath.Dir(journalFile), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	latest := v.Rows(v.At(reopened.SnapshotForUI("ranks"), "ranks", "disciplines"))
	if v.Int(latest[0]["rankIndex"]) != 2 || latest[0]["progressPercent"] == nil || v.Num(latest[1]["progressPercent"]) != 69 {
		t.Fatal("journal replay discarded newer saved rank observations", latest)
	}
}

func TestPoliticsJournalScopeAndOfflineCommandBoundary(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	e.Apply(M{"event": "Location", "timestamp": "2026-11-08T12:00:00Z", "StarSystem": "Synthetic Politics", "SystemAddress": "424242", "Factions": []M{{"Name": "Synthetic Faction", "Influence": 0, "FactionState": "Boom"}}, "Conflicts": []M{}}, nil)
	state := e.SnapshotForUI("politics")
	if v.At(state, "politics", "activeSystem", "name") != "Synthetic Politics" || v.Num(v.Rows(v.At(state, "politics", "activeSystem", "factions", "entries"))[0]["influence"]) != 0 {
		t.Fatal("political journal observations missing", state["politics"])
	}
	if _, exists := e.SnapshotForUI("overview")["politics"]; exists {
		t.Fatal("political cache leaked into dashboard refreshes")
	}
	before := e.Politics.Snapshot()
	if _, err := e.Do("/api/politics/refresh", M{"kind": "goals"}); err == nil {
		t.Fatal("offline fixture reached a public politics service")
	}
	if !reflect.DeepEqual(before, e.Politics.Snapshot()) {
		t.Fatal("rejected network operation changed cached politics")
	}
}

func TestAutoUpdatePreferenceDefaultsOnAndPersistsTypedOptOut(t *testing.T) {
	e, _, data := fixtureEngine(t, nil)
	if v.At(e.SnapshotForUI("settings"), "settings", "autoUpdateEnabled") != true {
		t.Fatal("auto updates are not enabled by default")
	}
	if _, err := e.Do("/api/settings", M{"autoUpdateEnabled": "false"}); err == nil {
		t.Fatal("untyped update flag was accepted")
	}
	doOK(t, e, "/api/settings", M{"autoUpdateEnabled": false})
	saved, err := profile.Read(filepath.Join(data, "settings-go-v2.json"))
	if err != nil || v.At(saved, "settings", "autoUpdateEnabled") != false {
		t.Fatal("update opt-out was not saved", err)
	}
	if e.UpdateManager != nil {
		t.Fatal("offline test initialized a real updater")
	}
	if _, err := e.Do("/api/updates/check", nil); err == nil {
		t.Fatal("isolated fixture allowed update networking")
	}
	if err := e.ApplyPendingUpdate(); err != nil {
		t.Fatal("isolated exit attempted an installer", err)
	}
}

func TestCommanderDemoRequiresSyntheticOfflineIdentityAndKeepsLocation(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	before := e.Politics.Snapshot()
	if e.SeedDemoCommanderAndPolitics() == nil || !reflect.DeepEqual(before, e.Politics.Snapshot()) {
		t.Fatal("personal commander accepted the demonstration")
	}
	e.Apply(M{"event": "Commander", "timestamp": "2026-11-08T12:00:00Z", "Name": "Demo Commander", "FID": "FNATIVE-SYNTHETIC"}, nil)
	commander := e.Commander()
	if err := e.SeedDemoCommanderAndPolitics(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(commander, e.Commander()) {
		t.Fatal("politics fixture changed the player's current system")
	}
	if v.Int(v.At(e.SnapshotForUI("ranks"), "ranks", "summary", "known")) != 8 || len(v.Rows(v.At(e.SnapshotForUI("politics"), "politics", "activeSystem", "factions", "entries"))) != 3 {
		t.Fatal("demo does not exercise actual rank and political domain models")
	}
}

func TestConcurrentUpdatePreferencesKeepBackgroundManagerInAgreement(t *testing.T) {
	e, _, data := fixtureEngine(t, nil)
	manager, err := updater.New(updater.Options{CurrentVersion: Version, DataDir: data, OnChange: e.Notify})
	if err != nil {
		t.Fatal(err)
	}
	e.UpdateManager = manager
	var group sync.WaitGroup
	for i := 0; i < 40; i++ {
		group.Add(1)
		go func(enabled bool) {
			defer group.Done()
			if _, err := e.Do("/api/settings", M{"autoUpdateEnabled": enabled}); err != nil {
				t.Error(err)
			}
		}(i%2 == 0)
	}
	group.Wait()
	e.mu.RLock()
	enabled := v.Bool(e.settings["autoUpdateEnabled"])
	e.mu.RUnlock()
	if manager.Snapshot().Enabled != enabled {
		t.Fatal("background updater disagrees with the saved opt-out")
	}
	doOK(t, e, "/api/settings", M{"autoUpdateEnabled": false})
	if manager.Snapshot().Enabled {
		t.Fatal("background updater survived the final explicit opt-out")
	}
}
