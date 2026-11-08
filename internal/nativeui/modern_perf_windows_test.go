//go:build windows

package nativeui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/app"
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func syntheticPerformanceEngine(b *testing.B, large bool) *app.Engine {
	b.Helper()
	profileDir, journalDir := b.TempDir(), b.TempDir()
	if large {
		goals, bookmarks := []M{}, []M{}
		for i := range 200 {
			goals = append(goals, M{"id": fmt.Sprintf("synthetic-goal-%03d", i), "title": fmt.Sprintf("Синтетическая цель %03d", i), "kind": "other", "status": "active", "system": "Synthetic Origin", "notes": strings.Repeat("Учебная запись. ", 80)})
			bookmarks = append(bookmarks, M{"id": fmt.Sprintf("synthetic-bookmark-%03d", i), "system": fmt.Sprintf("Synthetic System %03d", i), "body": "Synthetic Body", "notes": "Synthetic benchmark bookmark"})
		}
		encoded, err := json.Marshal(M{"objectives": goals, "bookmarks": bookmarks})
		if err != nil {
			b.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(profileDir, "settings-go-v2.json"), encoded, 0600); err != nil {
			b.Fatal(err)
		}
	}
	engine, err := app.New(app.Options{DataDir: profileDir, JournalDir: journalDir, DisableNetwork: true, ManualPolling: true})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(engine.Close)
	for _, event := range []M{
		{"event": "Fileheader", "gameversion": "4.4.2", "build": "synthetic"},
		{"event": "LoadGame", "Commander": "Synthetic Benchmark", "FID": "FSYNTHETIC", "Ship": "panthermkii", "ShipID": 1, "Credits": 1000000},
		{"event": "Loadout", "Ship": "panthermkii", "ShipID": 1, "CargoCapacity": 100, "Modules": []M{{"Item": "hpt_mininglaser_fixed_small"}}},
		{"event": "Location", "StarSystem": "Synthetic Origin", "SystemAddress": 42, "StarPos": []float64{0, 0, 0}, "Docked": true, "StationName": "Synthetic Station"},
		{"event": "Cargo", "Vessel": "Ship", "Inventory": []M{{"Name": "Gold", "Count": 10}}},
	} {
		event["timestamp"] = "2026-11-08T12:00:00Z"
		engine.Apply(event, M{})
	}
	bodyCount := 1
	if large {
		bodyCount = 200
	}
	for i := range bodyCount {
		name := fmt.Sprintf("Synthetic Origin %d", i+1)
		engine.Apply(M{"event": "Scan", "timestamp": "2026-11-08T12:00:00Z", "BodyID": i + 1, "BodyName": name, "Landable": true, "SurfaceGravity": 9.80665, "ReserveLevel": "PristineResources", "Rings": []M{{"Name": name + " A Ring", "RingClass": "eRingClass_Metallic"}}, "Materials": []M{{"Name": "Iron", "Percent": 20}}}, M{})
	}
	return engine
}

// BenchmarkSyntheticWebPipeline uses only generated local profiles. "large" is
// bounded at 200 goals/bookmarks/bodies; it never reads a player's profile,
// starts the journal polling worker, opens a window, or enables network access.
func BenchmarkSyntheticWebPipeline(b *testing.B) {
	for _, fixture := range []struct {
		name  string
		large bool
	}{{"normal", false}, {"large", true}} {
		b.Run(fixture.name, func(b *testing.B) {
			engine := syntheticPerformanceEngine(b, fixture.large)
			state := engine.Snapshot()
			encodedState, err := json.Marshal(state)
			if err != nil {
				b.Fatal(err)
			}
			b.Logf("synthetic full snapshot: %d bytes; goals=%d, bodies=%d", len(encodedState), len(v.Rows(state["objectives"])), len(v.Rows(state["targets"])))
			stages := []struct {
				name string
				run  func()
			}{
				{"snapshot", func() { engine.Snapshot() }},
				{"marshal-full-snapshot", func() {
					if _, err := json.Marshal(state); err != nil {
						b.Fatal(err)
					}
				}},
				{"build-overview", func() { p.Build("overview", "", "", state) }},
				{"web-state-clone-redact", func() { webState(state) }},
				{"dispatch-view", func() {
					if _, err := dispatchWeb(engine, "view", M{"tab": "overview"}); err != nil {
						b.Fatal(err)
					}
				}},
			}
			response, err := dispatchWeb(engine, "view", M{"tab": "overview"})
			if err != nil {
				b.Fatal(err)
			}
			responseBytes, err := json.Marshal(response)
			if err != nil {
				b.Fatal(err)
			}
			b.Logf("synthetic overview response: %d bytes", len(responseBytes))
			stages = append(stages, struct {
				name string
				run  func()
			}{"send-redact-marshal", func() {
				if _, err := json.Marshal(redactWebSecrets(M{"event": "response", "data": M{"id": "synthetic", "result": response}})); err != nil {
					b.Fatal(err)
				}
			}})
			for _, stage := range stages {
				b.Run(stage.name, func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						stage.run()
					}
				})
			}
		})
	}
}
