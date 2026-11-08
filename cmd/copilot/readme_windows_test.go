//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/app"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestDocumentationControllerRequiresSyntheticTemporaryEnglishProfile(t *testing.T) {
	root, err := os.MkdirTemp("", "ed-copilot-native-smoke-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	journals := filepath.Join(root, "journals")
	if err = os.MkdirAll(journals, 0700); err != nil {
		t.Fatal(err)
	}
	engine, err := app.New(app.Options{DataDir: filepath.Join(root, "profile"), JournalDir: journals, DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if _, err = newReadmeController(engine); err == nil {
		t.Fatal("ordinary profile accepted")
	}
	engine.Apply(v.M{"event": "LoadGame", "FID": "FNATIVE-SYNTHETIC", "Commander": "Тестовый командир", "Ship": "sidewinder"}, v.M{})
	engine.Apply(v.M{"event": "Location", "StarSystem": "Тестовая колония А", "Docked": false}, v.M{})
	controller, err := newReadmeController(engine)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []v.M{controller.SnapshotForUI("overview"), controller.SnapshotForHUD(), controller.Snapshot()} {
		if v.At(state, "commander", "name") != "Demo Commander" || v.At(state, "commander", "system") != "Demo Colony Alpha" {
			t.Fatal("fictional header did not retain its English names")
		}
	}
	if engine.Commander()["name"] != "Тестовый командир" {
		t.Fatal("photography modified raw observations")
	}
	if _, err = engine.Do("/api/settings", v.M{"language": "ru"}); err != nil {
		t.Fatal(err)
	}
	if _, err = newReadmeController(engine); err == nil {
		t.Fatal("Russian profile accepted for English photographs")
	}
}
