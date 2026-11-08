package app

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type onlineGateTransport struct {
	base                         accountFixtureTransport
	profileOnce, inaraOnce       sync.Once
	profileEntered, inaraEntered chan struct{}
	profileRelease, inaraRelease chan struct{}
}

func (g *onlineGateTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	var entered, release chan struct{}
	var once *sync.Once
	if request.URL.Hostname() == "companion.orerve.net" && request.URL.Path == "/profile" {
		entered, release, once = g.profileEntered, g.profileRelease, &g.profileOnce
	} else if request.URL.Hostname() == "inara.cz" {
		entered, release, once = g.inaraEntered, g.inaraRelease, &g.inaraOnce
	}
	if once != nil {
		once.Do(func() { close(entered) })
		select {
		case <-release:
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
	}
	return g.base.RoundTrip(request)
}

func TestOnlineFrontierAndInaraRequestsDoNotBlockLocalStateOrCommands(t *testing.T) {
	root, journals := t.TempDir(), t.TempDir()
	stamp := v.ISO(time.Now().Add(-time.Second))
	file := filepath.Join(journals, "Journal.synthetic.log")
	writeEvents(t, file, false, []M{
		{"event": "Fileheader", "timestamp": stamp, "gameversion": "4.4.2"},
		{"event": "LoadGame", "timestamp": stamp, "Commander": "Synthetic Pilot", "FID": "F123", "Ship": "panthermkii", "ShipID": 7, "Credits": 1000, "Loan": 0},
		{"event": "Location", "timestamp": stamp, "StarSystem": "Sol", "Docked": true, "StationName": "Galileo", "MarketID": 100},
	})
	gate := &onlineGateTransport{profileEntered: make(chan struct{}), inaraEntered: make(chan struct{}), profileRelease: make(chan struct{}), inaraRelease: make(chan struct{})}
	engine, err := New(Options{DataDir: root, JournalDir: journals, CallbackPort: -1, ManualPolling: true, HTTPClient: &http.Client{Transport: gate}})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	defer func() {
		select {
		case <-gate.profileRelease:
		default:
			close(gate.profileRelease)
		}
		select {
		case <-gate.inaraRelease:
		default:
			close(gate.inaraRelease)
		}
	}()
	login, err := engine.Do("/api/frontier/login", nil)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(v.Str(login["url"]))
	if err := engine.Frontier.HandleCallbackContext(context.Background(), url.Values{"state": {parsed.Query().Get("state")}, "code": {"synthetic-code"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Do("/api/integrations/configure", M{"apiKey": "synthetic-inara-key", "enabled": true}); err != nil {
		t.Fatal(err)
	}
	localWork := func(stage string) {
		t.Helper()
		done := make(chan error, 1)
		go func() {
			engine.Snapshot()
			_, err := engine.Do("/api/settings", M{"overlayEnabled": false})
			engine.Reader.Poll()
			engine.Snapshot()
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(stage, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal(stage + " request blocked local snapshot/settings/journal polling")
		}
	}
	for _, stage := range []struct {
		name, path       string
		entered, release chan struct{}
	}{{"Frontier CAPI", "/api/frontier/sync", gate.profileEntered, gate.profileRelease}, {"Inara", "/api/integrations/sync", gate.inaraEntered, gate.inaraRelease}} {
		done := make(chan error, 1)
		go func() { _, err := engine.Do(stage.path, nil); done <- err }()
		select {
		case <-stage.entered:
		case err := <-done:
			t.Fatalf("%s did not reach fake HTTP: %v", stage.name, err)
		case <-time.After(10 * time.Second):
			t.Fatal(stage.name + " did not reach fake HTTP")
		}
		localWork(stage.name)
		close(stage.release)
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(stage.name, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal(stage.name + " did not finish after HTTP release")
		}
	}
}

func BenchmarkEngineSnapshotSyntheticWorld(b *testing.B) {
	for _, systems := range []int{0, 100, 500} {
		b.Run(fmt.Sprint(systems, "-systems"), func(b *testing.B) {
			engine, err := New(Options{DataDir: b.TempDir(), JournalDir: b.TempDir(), DisableNetwork: true, ManualPolling: true})
			if err != nil {
				b.Fatal(err)
			}
			defer engine.Close()
			for index := 0; index < systems; index++ {
				name := fmt.Sprintf("Synthetic System %04d", index)
				bodies := M{}
				for bodyIndex := 0; bodyIndex < 10; bodyIndex++ {
					bodyName := fmt.Sprint(name, " ", bodyIndex)
					bodies[fmt.Sprint(bodyIndex)] = M{"id": bodyIndex, "name": bodyName, "source": "journal", "distanceLs": 100, "rings": []M{{"name": bodyName + " A Ring", "class": "metallic", "reserve": "pristine"}}, "materials": []M{{"key": "polonium", "percent": 1}}}
				}
				engine.Mining.World[name] = M{"key": name, "name": name, "coordinates": []float64{float64(index), 0, 0}, "bodies": bodies}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				engine.Snapshot()
			}
			b.StopTimer()
		})
	}
}

func BenchmarkEngineScopedSnapshotsLargeWorld(b *testing.B) {
	engine, err := New(Options{DataDir: b.TempDir(), JournalDir: b.TempDir(), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()
	for index := 0; index < 500; index++ {
		name := fmt.Sprintf("Synthetic System %04d", index)
		bodies := M{}
		for bodyIndex := 0; bodyIndex < 10; bodyIndex++ {
			bodyName := fmt.Sprint(name, " ", bodyIndex)
			bodies[fmt.Sprint(bodyIndex)] = M{"id": bodyIndex, "name": bodyName, "landable": true, "source": "journal", "rings": []M{{"name": bodyName + " A Ring", "type": "eRingClass_Metallic"}}, "materials": []M{{"key": "polonium", "percent": 1}}}
		}
		engine.Mining.World[name] = M{"key": name, "name": name, "coordinates": []float64{float64(index), 0, 0}, "bodies": bodies}
	}
	for _, test := range []struct {
		name, activity string
		snapshot       func() M
	}{
		{"full", "flight", engine.Snapshot},
		{"overview", "flight", func() M { return engine.SnapshotForUI("overview") }},
		{"hud-flight", "flight", engine.SnapshotForHUD},
		{"hud-raw-cached", "raw", engine.SnapshotForHUD},
		{"sync", "flight", engine.SnapshotForSync},
	} {
		b.Run(test.name, func(b *testing.B) {
			engine.mu.Lock()
			engine.settings["activity"] = test.activity
			engine.mu.Unlock()
			test.snapshot()
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				test.snapshot()
			}
			b.StopTimer()
		})
	}
}
