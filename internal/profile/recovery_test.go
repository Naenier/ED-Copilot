package profile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func testSettings(revision string) v.M {
	return v.M{
		"settings":     v.M{"language": "ru", "revision": revision},
		"objectives":   []v.M{{"title": "Trade trip"}},
		"colonization": v.M{"projects": []v.M{{"id": "p1"}, {"id": "p2"}}},
	}
}

func TestLegacySettingsMigrationKeepsValidatedPreviousCopy(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, legacyFile)
	original := []byte("\xef\xbb\xbf" + `{"settings":{"language":"ru"},"bookmarks":null,"objectives":[{"title":"Legacy"}]}`)
	writeFixture(t, legacy, original)
	old, err := LoadSettings(dir)
	if err != nil || v.At(old, "settings", "language") != "ru" {
		t.Fatalf("legacy load: %v %v", old, err)
	}
	if err := SaveSettings(dir, testSettings("new")); err != nil {
		t.Fatal(err)
	}
	current, err := Read(filepath.Join(dir, SettingsFile))
	if err != nil || v.Int(current["profileVersion"]) != settingsVersion {
		t.Fatalf("flat versioned settings unreadable: %v %v", current, err)
	}
	backups, err := ListGenerations(dir)
	if err != nil || len(backups) != 1 || backups[0].Language != "ru" || backups[0].Counts["goals"] != 1 || backups[0].SavedAt.IsZero() {
		t.Fatalf("unexpected legacy backup: %v %v", backups, err)
	}
	assertFile(t, legacy, original)
	if err := RestoreGeneration(dir, backups[0].ID); err != nil {
		t.Fatal(err)
	}
	restored, err := LoadSettings(dir)
	if err != nil || v.Rows(restored["objectives"])[0]["title"] != "Legacy" {
		t.Fatalf("wrong restored profile: %v %v", restored, err)
	}
}

func TestSettingsBackupRetentionAndUnchangedSave(t *testing.T) {
	dir := t.TempDir()
	for _, revision := range []string{"1", "2", "3", "4", "5", "6", "7"} {
		if err := SaveSettings(dir, testSettings(revision)); err != nil {
			t.Fatal(err)
		}
	}
	backups, err := ListGenerations(dir)
	if err != nil || len(backups) != backupCount {
		t.Fatalf("bounded backups: %v %v", backups, err)
	}
	for _, backup := range backups {
		if backup.SavedAt.IsZero() || backup.Counts["colonies"] != 2 {
			t.Fatalf("incomplete metadata: %#v", backup)
		}
	}
	if err := SaveSettings(dir, testSettings("7")); err != nil {
		t.Fatal(err)
	}
	unchanged, err := ListGenerations(dir)
	if err != nil || !reflect.DeepEqual(unchanged, backups) {
		t.Fatalf("unchanged save rotated history: %v %v", unchanged, err)
	}
	if err := RestoreGeneration(dir, backups[2].ID); err != nil {
		t.Fatal(err)
	}
	restored, err := LoadSettings(dir)
	if err != nil || v.At(restored, "settings", "revision") != "4" {
		t.Fatalf("wrong generation restored: %v %v", restored, err)
	}
	if err := RestoreGeneration(dir, "../"+SettingsFile); !errors.Is(err, ErrGenerationUnavailable) {
		t.Fatalf("unrecognized generation accepted: %v", err)
	}
}

func TestCorruptPrimaryRequiresRecoveryAndKeepsOriginalBytes(t *testing.T) {
	dir := t.TempDir()
	if err := SaveSettings(dir, testSettings("working")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, SettingsFile)
	damaged := []byte(`{"settings":{"language":"en"}`)
	writeFixture(t, file, damaged)
	var recovery *RecoveryError
	if _, err := LoadSettings(dir); !errors.As(err, &recovery) || !errors.Is(err, ErrInvalidSettings) || len(recovery.Generations) != 1 {
		t.Fatalf("corruption was not surfaced: %v", err)
	}
	if err := SaveSettings(dir, testSettings("new defaults")); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("damaged primary was writable without a recovery decision: %v", err)
	}
	assertFile(t, file, damaged)
	if err := RestoreGeneration(dir, recovery.Generations[0].ID); err != nil {
		t.Fatal(err)
	}
	archived, err := filepath.Glob(filepath.Join(dir, "settings-go-v2.damaged-*.json"))
	if err != nil || len(archived) != 1 {
		t.Fatalf("original file not preserved: %v %v", archived, err)
	}
	assertFile(t, archived[0], damaged)
	settings, err := LoadSettings(dir)
	if err != nil || v.At(settings, "settings", "revision") != "working" {
		t.Fatalf("restored settings: %v %v", settings, err)
	}
}

func TestSettingsRejectMalformedAndNewerSchema(t *testing.T) {
	for _, payload := range []string{`{}`, `null`, `[]`, `{"unrelated":123}`, `{"settings":null}`, `{"settings":[]}`, `{"objectives":["bad"]}`, `{"settings":{}} null`, `{"settings":{},"profileVersion":"1"}`} {
		t.Run(payload, func(t *testing.T) {
			dir := t.TempDir()
			writeFixture(t, filepath.Join(dir, SettingsFile), []byte(payload))
			if _, err := LoadSettings(dir); !errors.Is(err, ErrInvalidSettings) {
				t.Fatalf("malformed payload was accepted: %v", err)
			}
		})
	}
	dir := t.TempDir()
	if err := SaveSettings(dir, testSettings("working")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, SettingsFile)
	future := []byte(`{"settings":{},"profileVersion":2}`)
	writeFixture(t, file, future)
	var recovery *RecoveryError
	if _, err := LoadSettings(dir); !errors.Is(err, ErrUnsupportedSettingsVersion) || !errors.As(err, &recovery) || len(recovery.Generations) != 1 {
		t.Fatalf("newer schema loaded as older: %v", err)
	}
	if err := RestoreGeneration(dir, recovery.Generations[0].ID); !errors.Is(err, ErrUnsupportedSettingsVersion) {
		t.Fatalf("newer schema unexpectedly restorable: %v", err)
	}
	assertFile(t, file, future)
}

func TestMissingPrimaryDistinguishesFirstSaveRetryFromRecovery(t *testing.T) {
	dir := t.TempDir()
	if data, err := LoadSettings(dir); data != nil || err != nil {
		t.Fatalf("fresh directory: %v %v", data, err)
	}
	data, err := encodeSettings(testSettings("staged"))
	if err != nil {
		t.Fatal(err)
	}
	staged, err := decodeSettings(data)
	if err != nil {
		t.Fatal(err)
	}
	// This is the on-disk state after staging the initial generation when
	// installing the primary has failed.
	if err := keepBackup(dir, staged, ""); err != nil {
		t.Fatal(err)
	}
	var recovery *RecoveryError
	if _, err := LoadSettings(dir); !errors.As(err, &recovery) || len(recovery.Generations) != 1 {
		t.Fatalf("missing primary started with defaults: %v", err)
	}
	if err := SaveSettings(dir, testSettings("other")); !errors.As(err, &recovery) {
		t.Fatalf("unrelated snapshot replaced missing settings: %v", err)
	}
	if err := SaveSettings(dir, testSettings("staged")); err != nil {
		t.Fatalf("matching retry failed: %v", err)
	}
	assertFile(t, filepath.Join(dir, SettingsFile), data)
}

func TestFailedBackupWriteAndInvalidCandidatePreservePrimary(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, SettingsFile)
	old := []byte(`{"settings":{"language":"en"}}`)
	writeFixture(t, file, old)
	if err := SaveSettings(dir, v.M{"settings": "broken"}); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("invalid replacement accepted: %v", err)
	}
	writeFixture(t, filepath.Join(dir, "profile-recovery"), []byte("blocks directory creation"))
	if err := SaveSettings(dir, testSettings("new")); err == nil {
		t.Fatal("primary changed after failed backup")
	}
	assertFile(t, file, old)
}

func TestInvalidBackupIsNeverOfferedAndMissingPrimaryDoesNotReset(t *testing.T) {
	dir := t.TempDir()
	if err := SaveSettings(dir, testSettings("first")); err != nil {
		t.Fatal(err)
	}
	slots, err := backupSlots(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range slots {
		if slot.modified.IsZero() {
			continue
		}
		data, err := os.ReadFile(slot.file)
		if err != nil {
			t.Fatal(err)
		}
		writeFixture(t, slot.file, bytes.Replace(data, []byte("first"), []byte("other"), 1))
	}
	backups, err := ListGenerations(dir)
	if err != nil || len(backups) != 0 {
		t.Fatalf("tampered backup was offered: %v %v", backups, err)
	}
	if err := os.Remove(filepath.Join(dir, SettingsFile)); err != nil {
		t.Fatal(err)
	}
	var recovery *RecoveryError
	if _, err := LoadSettings(dir); !errors.As(err, &recovery) || len(recovery.Generations) != 0 {
		t.Fatalf("failed backup made a missing profile look fresh: %v", err)
	}
}

func TestPortableProfileKeepsPlansAndStripsPrivateData(t *testing.T) {
	parent := t.TempDir()
	file := filepath.Join(parent, "portable.json")
	settings := testSettings("unsaved")
	settings["journalDir"] = "C:/private-journals"
	settings["frontier"] = v.M{"refreshToken": "unlisted-frontier-token"}
	settings["galacticPrices"] = v.M{"old": "excluded-price-cache"}
	settings["learning"] = v.M{"completed": v.M{"first": true}}
	v.Obj(settings["settings"])["nested"] = []v.M{{"frontierRefreshToken": "nested-refresh-token", "shared_key": "nested-shared-key", "note": "note to retain"}}
	if err := ExportPortable(file, settings); err != nil {
		t.Fatal(err)
	}
	metadata, err := InspectPortable(file)
	if err != nil || metadata.Counts["colonies"] != 2 || metadata.Language != "ru" {
		t.Fatalf("wrong archive preview: %v %v", metadata, err)
	}
	payload, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"private-journals", "unlisted-frontier-token", "nested-refresh-token", "nested-shared-key", "excluded-price-cache"} {
		if strings.Contains(string(payload), marker) {
			t.Fatalf("private or derived payload escaped: %s", marker)
		}
	}
	target := filepath.Join(parent, "separate")
	if err := ImportPortable(file, target); err != nil {
		t.Fatal(err)
	}
	imported, err := LoadSettings(target)
	if err != nil || v.At(imported, "settings", "revision") != "unsaved" || !v.Bool(v.At(imported, "learning", "completed", "first")) ||
		v.Rows(v.At(imported, "settings", "nested"))[0]["note"] != "note to retain" {
		t.Fatalf("incomplete round trip: %v %v", imported, err)
	}
	if err := ImportPortable(file, target); !errors.Is(err, ErrDestinationNotEmpty) {
		t.Fatalf("import overwrote an existing profile: %v", err)
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 1 || entries[0].Name() != SettingsFile {
		t.Fatalf("import contained extra files: %v %v", entries, err)
	}
	if err := SaveSettings(target, imported); err != nil {
		t.Fatal(err)
	}
	backups, err := ListGenerations(target)
	if err != nil || len(backups) != 1 {
		t.Fatalf("first save of an imported profile skipped its backup: %v %v", backups, err)
	}
}

func TestPortableRejectsDamagedUnknownAndPrivateArchivesBeforeCreatingDestination(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "portable.json")
	for _, test := range []struct {
		name string
		data string
		edit func(*profileArchive)
		want error
	}{
		{"unknown section", `{"profileVersion":1,"settings":{},"unknown":{}}`, nil, ErrInvalidArchive},
		{"journal path", `{"journalDir":"some path","profileVersion":1,"settings":{}}`, nil, ErrInvalidArchive},
		{"credentials", `{"profileVersion":1,"settings":{"api-key":"secret"}}`, nil, ErrInvalidArchive},
		{"shadowed credentials", `{"profileVersion":1,"settings":{"workspace":{"api-key":"secret"},"workspace":{}}}`, nil, ErrInvalidArchive},
		{"unsupported settings", `{"profileVersion":2,"settings":{}}`, nil, ErrUnsupportedSettingsVersion},
		{"unsupported archive", `{"profileVersion":1,"settings":{}}`, func(a *profileArchive) { a.Version++ }, ErrArchiveVersion},
		{"checksum", `{"profileVersion":1,"settings":{}}`, func(a *profileArchive) { a.Checksum = strings.Repeat("0", 64) }, ErrInvalidArchive},
	} {
		t.Run(test.name, func(t *testing.T) {
			archive := newArchive(portableFormat, []byte(test.data))
			if test.edit != nil {
				test.edit(&archive)
			}
			if err := writeArchive(file, archive); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(dir, "must-not-exist")
			if err := ImportPortable(file, destination); !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
			if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid archive created a destination: %v", err)
			}
		})
	}
}

func writeFixture(t *testing.T, file string, data []byte) {
	t.Helper()
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, file string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s: want %q, got %q (%v)", filepath.Base(file), want, got, err)
	}
}
