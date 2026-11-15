package controls

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const fixture = `<?xml version="1.0" encoding="UTF-8"?>
<Root PresetName="Custom" MajorVersion="4" MinorVersion="2" Future="keep &quot;exactly&quot;">
	<KeyboardLayout>en-US</KeyboardLayout>
	<LandingGearToggle Extra="preserve">
		<Primary Device="Keyboard" Key="Key_G" Unknown="before Key='Key_F' after">
			<Modifier Device="Keyboard" Key="Key_LeftShift" Future="1" />
			<Hold Value="1" />
			<!-- unknown child survives -->
			<FutureMode Value="5" />
		</Primary>
		<Secondary Device="{NoDevice}" Key="" />
		<ToggleOn Value="0" />
		<MouseWheel Direction="Down" />
	</LandingGearToggle>
	<YawAxisRaw>
		<Binding Device="15320A57" Key="GamePad_LStickX" />
		<Inverted Value="0" />
		<Deadzone Value="0.00000000" />
	</YawAxisRaw>
</Root>
`

func writeFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func updateOK(t *testing.T, s *Service, input M) M {
	t.Helper()
	result, err := s.Update(input)
	if err != nil {
		t.Fatalf("%s: %v", input["action"], err)
	}
	return result
}

func TestControlProfileEditingPreservesUnknownXML(t *testing.T) {
	base := t.TempDir()
	local := filepath.Join(base, "game")
	path := filepath.Join(local, "Custom.4.2.binds")
	data := []byte("\uFEFF" + strings.ReplaceAll(fixture, "\n", "\r\n"))
	writeFixture(t, path, data)
	s := New(local, filepath.Join(base, "backups"), func() bool { return false })
	profileID := "local:Custom.4.2.binds"
	view, err := s.View(M{"profileId": profileID})
	if err != nil {
		t.Fatal(err)
	}
	profile := v.Obj(view["profile"])
	if len(v.Rows(profile["actions"])) != 2 || profile["keyboardLayout"] != "en-US" {
		t.Fatalf("unexpected profile %#v", profile)
	}
	result := updateOK(t, s, M{"action": "edit", "profileId": profileID, "expectedRevision": profile["revision"], "changes": []M{
		{"action": "LandingGearToggle", "slot": "primary", "device": "Keyboard", "key": "Key_H", "hold": false, "toggle": true,
			"modifiers": []M{{"device": "Keyboard", "key": "Key_RightShift"}}},
		{"action": "LandingGearToggle", "slot": "secondary", "device": "Keyboard", "key": "Key_L", "hold": true,
			"modifiers": []M{{"device": "Keyboard", "key": "Key_LeftControl"}}},
		{"action": "YawAxisRaw", "slot": "axis", "inverted": true, "deadzone": 0.05},
	}})
	edited := readFixture(t, path)
	want := strings.NewReplacer(
		`Key="Key_G"`, `Key="Key_H"`, `Key="Key_LeftShift"`, `Key="Key_RightShift"`,
		`<Hold Value="1" />`, `<Hold Value="0" />`,
		`<Secondary Device="{NoDevice}" Key="" />`, `<Secondary Device="Keyboard" Key="Key_L"><Modifier Device="Keyboard" Key="Key_LeftControl" /><Hold Value="1" /></Secondary>`,
		`<ToggleOn Value="0"`, `<ToggleOn Value="1"`, `<Inverted Value="0"`, `<Inverted Value="1"`, `0.00000000`, `0.05000000`,
	).Replace(string(data))
	if string(edited) != want {
		t.Fatalf("edited XML differs:\nwant:\n%s\ngot:\n%s", want, edited)
	}
	if len(v.Rows(result["backups"])) != 1 {
		t.Fatalf("expected pre-edit backup: %#v", result["backups"])
	}
	if _, err := s.Update(M{"action": "edit", "profileId": profileID, "expectedRevision": profile["revision"]}); err == nil {
		t.Fatal("stale revision was accepted")
	}
	backupID := v.Str(v.Rows(result["backups"])[0]["id"])
	updateOK(t, s, M{"action": "restore", "backupId": backupID})
	if after := readFixture(t, path); !bytes.Equal(after, data) {
		t.Fatal("backup did not restore the original bytes")
	}
}

func TestProfileSelectionCloneAndRestore(t *testing.T) {
	base := t.TempDir()
	local := filepath.Join(base, "game")
	selection := filepath.Join(local, startFile)
	original := []byte("\uFEFFConsoleX360\r\nCustom\r\nConsoleX360\r\nConsoleX360")
	writeFixture(t, selection, original)
	writeFixture(t, filepath.Join(local, "Custom.4.2.binds"), []byte(fixture))
	builtins := filepath.Join(base, "builtins")
	stock := strings.NewReplacer(`PresetName="Custom"`, `PresetName="ConsoleX360"`, `MajorVersion="4" MinorVersion="2"`, `MajorVersion="1" MinorVersion="8"`).Replace(fixture)
	writeFixture(t, filepath.Join(builtins, "ConsoleX360.binds"), []byte(stock))
	s := New(local, filepath.Join(base, "backups"), func() bool { return false }).WithBuiltIns(builtins)
	manual := updateOK(t, s, M{"action": "backup"})
	stockID := "builtin:ConsoleX360.binds"
	if len(v.Rows(manual["profiles"])) != 2 || v.Rows(manual["selections"])[0]["profileId"] != stockID {
		t.Fatalf("incorrect profile selections: %#v", manual["selections"])
	}
	clone := updateOK(t, s, M{"action": "clone", "profileId": stockID, "name": "Ambient.Impact", "changes": []M{
		{"action": "LandingGearToggle", "slot": "primary", "device": "Keyboard", "key": "Key_H"},
	}})
	cloneID := v.Str(v.Obj(clone["profile"])["id"])
	updateOK(t, s, M{"action": "activate", "profileId": cloneID, "context": "ship"})
	if got := string(readFixture(t, selection)); got != strings.Replace(string(original), "Custom", "Ambient.Impact", 1) {
		t.Fatalf("activation altered the format or other selections: %q", got)
	}
	updateOK(t, s, M{"action": "restore", "backupId": manual["backupId"]})
	if got := readFixture(t, selection); !bytes.Equal(got, original) {
		t.Fatalf("restore did not restore selections: %q", got)
	}
	if data, err := s.Export(cloneID); err != nil || !bytes.Contains(data, []byte(`Key="Key_H"`)) || !bytes.Contains(data, []byte(`MajorVersion="4" MinorVersion="2"`)) {
		t.Fatalf("expected edited stock clone to survive restore: %v", err)
	}
	imported, err := s.Import([]byte(fixture), "")
	if err != nil || v.Obj(imported["profile"])["name"] != "Custom 2" {
		t.Fatalf("expected import collision protected: %v %#v", err, imported)
	}
	if _, err = s.Import([]byte(strings.Replace(fixture, `MajorVersion="4"`, `MajorVersion="3"`, 1)), "Legacy"); err == nil {
		t.Fatal("incompatible legacy file imported")
	}
}

func TestControlSafeguards(t *testing.T) {
	base := t.TempDir()
	local := filepath.Join(base, "game")
	selection := filepath.Join(local, startFile)
	writeFixture(t, selection, []byte("Custom\n"))
	writeFixture(t, filepath.Join(local, "Custom.4.2.binds"), []byte(fixture))
	running := true
	s := New(local, filepath.Join(base, "backups"), func() bool { return running })
	manual := updateOK(t, s, M{"action": "backup"})
	if _, err := s.Update(M{"action": "restore", "backupId": manual["backupId"]}); err == nil {
		t.Fatal("game running did not prevent restore")
	}
	if _, err := s.Import([]byte(fixture), "Other"); err == nil {
		t.Fatal("game running did not prevent import")
	}
	running = false
	if _, err := s.Update(M{"action": "restore", "backupId": "../outside"}); err == nil {
		t.Fatal("backup path traversal permitted")
	}
	if _, err := s.Import([]byte(fixture), "LPT1.foo"); err == nil {
		t.Fatal("device-like filename permitted")
	}
	if _, err := s.Import([]byte(fixture), "../outside"); err == nil {
		t.Fatal("profile path traversal permitted")
	}
	result, err := s.Import([]byte(fixture), "Other")
	if err != nil {
		t.Fatal(err)
	}
	updateOK(t, s, M{"action": "activate", "profileId": v.Obj(result["profile"])["id"], "context": "srv"})
	if got := string(readFixture(t, selection)); got != "Custom\nCustom\nOther\nCustom\n" {
		t.Fatalf("legacy active selection was not expanded: %q", got)
	}
	path := filepath.Join(s.backupDir, v.Str(manual["backupId"]), "Custom.4.2.binds")
	writeFixture(t, path, []byte("corrupted"))
	if _, err := s.Update(M{"action": "restore", "backupId": manual["backupId"]}); err == nil {
		t.Fatal("altered backup was accepted")
	}
}

func TestRestoreRollsBackIfGameStartsDuringCommit(t *testing.T) {
	base := t.TempDir()
	local := filepath.Join(base, "game")
	path := filepath.Join(local, "Custom.4.2.binds")
	selection := filepath.Join(local, startFile)
	newer := strings.Replace(fixture, "Key_G", "Key_J", 1)
	writeFixture(t, path, []byte(fixture))
	writeFixture(t, selection, []byte("Custom\n"))
	calls, failCall := 0, 0
	s := New(local, filepath.Join(base, "backups"), func() bool { calls++; return calls == failCall })
	manual := updateOK(t, s, M{"action": "backup"})
	writeFixture(t, path, []byte(newer))
	if err := os.Remove(selection); err != nil {
		t.Fatal(err)
	}
	// Fail the guard immediately before the second write of the restore.
	calls, failCall = 0, 5
	if _, err := s.Update(M{"action": "restore", "backupId": manual["backupId"]}); err == nil {
		t.Fatal("restore ignored running game")
	}
	if got := string(readFixture(t, path)); got != newer {
		t.Fatal("first write of restore was not rolled back")
	}
	if _, err := os.Stat(selection); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed restore modified selection")
	}
}
