package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/profile"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestEngineRecoversFromVerifiedProfileGeneration(t *testing.T) {
	engine, journal, dir := fixtureEngine(t, nil)
	doOK(t, engine, "/api/objectives", M{"action": "add", "title": "First objective"})
	doOK(t, engine, "/api/objectives", M{"action": "add", "title": "Second objective"})
	doOK(t, engine, "/api/objectives", M{"action": "add", "title": "Third objective"})
	if err := engine.CloseWithSave(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, profile.SettingsFile)
	damaged := []byte("{\"settings\":")
	if err := os.WriteFile(file, damaged, 0600); err != nil {
		t.Fatal(err)
	}

	opts := Options{DataDir: dir, JournalDir: filepath.Dir(journal), DisableNetwork: true, ManualPolling: true}
	_, err := New(opts)
	var recovery *profile.RecoveryError
	if !errors.As(err, &recovery) || len(recovery.Generations) < 2 {
		t.Fatalf("expected a choice of profile generations, got %v", err)
	}
	if err := profile.RestoreGeneration(dir, recovery.Generations[1].ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	goals := v.Rows(reopened.Snapshot()["objectives"])
	if len(goals) != 1 || goals[0]["title"] != "First objective" {
		t.Fatalf("wrong generation restored: %v", goals)
	}
}

func TestEngineExportsUnsavedChangesAfterFailedClose(t *testing.T) {
	engine, journal, dir := fixtureEngine(t, nil)
	doOK(t, engine, "/api/objectives", M{"action": "add", "title": "Saved objective"})
	primary := filepath.Join(dir, profile.SettingsFile)
	held := filepath.Join(dir, "held-settings.json")
	if err := os.Rename(primary, held); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(primary, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Do("/api/objectives", M{"action": "add", "title": "Unsaved objective"}); err == nil {
		t.Fatal("expected write failure")
	}
	if err := engine.CloseWithSave(); err == nil {
		t.Fatal("close hid the save failure")
	}
	copyDir := t.TempDir()
	archive := filepath.Join(copyDir, "portable.json")
	restoredDir := filepath.Join(copyDir, "restored")
	if err := engine.ExportProfile(archive); err != nil {
		t.Fatal(err)
	}
	if err := profile.ImportPortable(archive, restoredDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(primary); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(held, primary); err != nil {
		t.Fatal(err)
	}
	if err := engine.Save(); err != nil {
		t.Fatalf("save retry after clearing the obstruction: %v", err)
	}
	for _, dataDir := range []string{restoredDir, dir} {
		reopened, err := New(Options{DataDir: dataDir, JournalDir: filepath.Dir(journal), DisableNetwork: true, ManualPolling: true})
		if err != nil {
			t.Fatal(err)
		}
		goals := v.Rows(reopened.Snapshot()["objectives"])
		reopened.Close()
		if len(goals) != 2 || goals[1]["title"] != "Unsaved objective" {
			t.Fatalf("missing in-memory changes after reopening profile: %v", goals)
		}
	}
}
