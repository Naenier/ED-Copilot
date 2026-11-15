//go:build windows

package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestControlsEngineAndDialogFiles(t *testing.T) {
	root := t.TempDir()
	bindings := filepath.Join(root, "Bindings")
	if err := os.Mkdir(bindings, 0700); err != nil {
		t.Fatal(err)
	}
	const file = "Layout.4.2.binds"
	raw := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<Root PresetName="Layout" MajorVersion="4" MinorVersion="2">
  <FocusLeftPanel><Primary Device="Keyboard" Key="Key_1" /><Secondary Device="{NoDevice}" Key="" /></FocusLeftPanel>
</Root>
`)
	if err := os.WriteFile(filepath.Join(bindings, file), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bindings, "StartPreset.4.start"), []byte("Layout\nLayout\nLayout\nLayout\n"), 0600); err != nil {
		t.Fatal(err)
	}
	e, err := New(Options{DataDir: filepath.Join(root, "app"), JournalDir: root, BindingsDir: bindings, DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	state, err := e.Do("/api/controls", nil)
	id := v.Str(v.At(state, "profile", "id"))
	if err != nil || !v.Bool(state["available"]) || id == "" || len(v.Rows(v.At(state, "profile", "actions"))) != 1 {
		t.Fatalf("profile not visible through the app: %v %v", state, err)
	}
	if err := e.ExportControlsFile(id, filepath.Join(bindings, file)); err == nil {
		t.Fatal("export to the game's profile was not blocked")
	}
	if err := e.ExportControlsFile(id, filepath.Join(root, "export.binds")); err != nil {
		t.Fatal(err)
	}
	exported, err := os.ReadFile(filepath.Join(root, "export.binds"))
	if err != nil || !bytes.Equal(raw, exported) {
		t.Fatalf("not an exact export: %v", err)
	}
	if _, err := e.ImportControlsFile(filepath.Join(root, "unrelated.txt")); err == nil {
		t.Fatal("import accepted a path with a different extension")
	}
}
