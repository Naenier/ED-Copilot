package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestBackgroundJournalReplayMatchesSynchronousStartup(t *testing.T) {
	journalDir := t.TempDir()
	file := filepath.Join(journalDir, "Journal.2026-11-18T120000.01.log")
	writeEvents(t, file, false, []M{
		testEvent("Fileheader", M{"gameversion": "4.4.2"}),
		testEvent("LoadGame", M{"Commander": "Startup Pilot", "FID": "FSTARTUP", "Ship": "Python", "ShipID": 9, "Credits": 725000}),
		testEvent("Location", M{"StarSystem": "Sol", "SystemAddress": 10477373803, "StarPos": []float64{0, 0, 0}}),
		testEvent("FSDJump", M{"StarSystem": "Sirius", "SystemAddress": 121569805492, "StarPos": []float64{6.25, -1.28, -5.75}}),
	})
	start := func(background bool) *Engine {
		t.Helper()
		engine, err := New(Options{
			DataDir: t.TempDir(), JournalDir: journalDir, DisableNetwork: true,
			ManualPolling: !background, BackgroundJournal: background,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(engine.Close)
		return engine
	}
	want := start(false)
	got := start(true)
	deadline := time.Now().Add(5 * time.Second)
	for v.Bool(got.Reader.State()["scanning"]) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	reader := got.Reader.State()
	if v.Bool(reader["scanning"]) || v.Int(reader["eventCount"]) != 4 || v.Str(reader["error"]) != "" {
		t.Fatalf("background replay did not finish: %v", reader)
	}
	for _, key := range []string{"name", "fid", "ship", "shipId", "system", "systemAddress"} {
		if v.Str(got.Commander()[key]) != v.Str(want.Commander()[key]) {
			t.Fatalf("background replay diverged at %s", key)
		}
	}
	if got.planWorkspace != want.planWorkspace || got.planWorkspace.key == "" {
		t.Fatal("commander workspace was not restored")
	}
	if err := got.CloseWithSave(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got.journalDone:
	default:
		t.Fatal("shutdown did not wait for the reader")
	}
}

func TestBackgroundJournalReportsUnreadableDirectory(t *testing.T) {
	journalDir := filepath.Join(t.TempDir(), "missing")
	engine, err := New(Options{DataDir: t.TempDir(), JournalDir: journalDir, DisableNetwork: true, BackgroundJournal: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	deadline := time.Now().Add(5 * time.Second)
	for v.Bool(engine.Reader.State()["scanning"]) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	state := engine.Reader.State()
	if v.Bool(state["scanning"]) || v.Str(state["error"]) == "" {
		t.Fatalf("failed replay left the UI waiting: %v", state)
	}
	if _, err = os.Stat(journalDir); !os.IsNotExist(err) {
		t.Fatal("reader modified the missing journal directory")
	}
}
