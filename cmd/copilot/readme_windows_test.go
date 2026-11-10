//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/app"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestDocumentationControllerRequiresSyntheticTemporaryProfileAndAdaptsOnlyEnglish(t *testing.T) {
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
	russian, err := newReadmeController(engine)
	if err != nil {
		t.Fatal("Russian isolated capture rejected", err)
	}
	for _, state := range []v.M{russian.SnapshotForUI("overview"), russian.SnapshotForHUD(), russian.Snapshot()} {
		if v.At(state, "settings", "language") != "ru" || v.At(state, "commander", "name") != "Тестовый командир" || v.At(state, "commander", "system") != "Тестовая колония А" {
			t.Fatal("Russian documentation changed synthetic names or language", v.Obj(state["commander"]))
		}
	}
	if _, err = engine.Do("/api/settings", v.M{"language": "en"}); err != nil {
		t.Fatal(err)
	}
	if v.At(controller.SnapshotForUI("overview"), "commander", "name") != "Demo Commander" || engine.Commander()["name"] != "Тестовый командир" {
		t.Fatal("returning to English did not preserve raw fixture observations")
	}
	nearby := v.Rows(controller.Galaxy("galaxy.nearby", v.M{"name": "Demo Colony Alpha", "radius": 25})["rows"])
	if len(nearby) < 2 || nearby[1]["name"] != "Demo Atlas" {
		t.Fatal("documentation does not show the fictional nearby systems", nearby)
	}
	if engine.Galaxy("galaxy.nearby", v.M{"name": "Demo Colony Alpha", "radius": 25})["status"] != "disabled" {
		t.Fatal("documentation fixture did not keep the engine offline")
	}
	bodies := v.Rows(controller.Galaxy("galaxy.system", v.M{"name": "Demo Atlas", "section": "bodies"})["rows"])
	if len(bodies) != 3 || len(v.Rows(bodies[1]["materials"])) == 0 {
		t.Fatal("documentation body details are missing", bodies)
	}
}

func TestDocumentationLanguageOptionIsRestrictedToTheCaptureMode(t *testing.T) {
	for _, tc := range []struct {
		enabled   bool
		requested string
		want      string
		valid     bool
	}{
		{false, "", "en", true}, {true, "", "en", true},
		{true, "en", "en", true}, {true, "ru", "ru", true},
		{false, "en", "", false}, {false, "ru", "", false},
		{true, "de", "", false}, {true, "EN", "", false}, {true, "ru ", "", false},
	} {
		got, err := readmeCaptureLanguage(tc.enabled, tc.requested)
		if (err == nil) != tc.valid || got != tc.want {
			t.Fatalf("%+v: %q, %v", tc, got, err)
		}
	}
}

func TestDocumentationControllerRejectsSyntheticNamedFoldersOutsideTheIsolatedTempRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ed-copilot-native-smoke-misleading")
	engine, err := app.New(app.Options{DataDir: filepath.Join(root, "profile"), JournalDir: filepath.Join(root, "journals"), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.Apply(v.M{"event": "LoadGame", "FID": "FNATIVE-SYNTHETIC", "Commander": "Тестовый командир", "Ship": "sidewinder"}, v.M{})
	for _, language := range []string{"en", "ru"} {
		if _, err = engine.Do("/api/settings", v.M{"language": language}); err != nil {
			t.Fatal(err)
		}
		if _, err = newReadmeController(engine); err == nil {
			t.Fatal("folder name substituted for an actual isolated temporary profile", language)
		}
	}
}
