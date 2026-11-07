package journal

import (
	v "github.com/Naenier/ED-Copilot/internal/value"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPartialUnicodeOffsetsDedupeResetAndCallbackReentry(t *testing.T) {
	dir := t.TempDir()
	name := "Journal.2026-11-07.01.log"
	file := filepath.Join(dir, name)
	header := "{\"event\":\"LoadGame\",\"FID\":\"FTEST\",\"Commander\":\"Тест\"}\r\n"
	line := "{\"event\":\"ColonisationContribution\",\"MarketID\":42,\"Contributions\":[{\"Name\":\"steel\",\"Amount\":1}]}\n"
	if e := os.WriteFile(file, []byte(header+line+line[:20]), 0600); e != nil {
		t.Fatal(e)
	}
	events, metas := []M{}, []M{}
	resets := 0
	var r *Reader
	r = New(dir, func(e, m M) { _ = r.State(); events = append(events, e); metas = append(metas, m) }, func() { resets++; events = nil; metas = nil })
	done := make(chan struct{})
	go func() { r.Poll(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("callback state read deadlocked journal reader")
	}
	if len(events) != 2 {
		t.Fatal("partial record applied", len(events))
	}
	if v.Int(metas[1]["offset"]) != len([]byte(header)) {
		t.Fatal("UTF-8 offset was not byte-exact", metas)
	}
	f, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString(line[20:])
	f.Close()
	r.Poll()
	if len(events) != 3 {
		t.Fatal("completion lost or duplicated record", len(events))
	}
	if v.Int(metas[2]["offset"]) != len([]byte(header+line)) {
		t.Fatal("identical events should have distinct cursors")
	}
	r.Poll()
	if len(events) != 3 {
		t.Fatal("unchanged file replayed")
	}
	r.Rescan()
	r.Poll()
	if resets != 2 || len(events) != 3 {
		t.Fatal("rescan must reset and rebuild exactly")
	}
	if e := os.WriteFile(file, []byte(header), 0600); e != nil {
		t.Fatal(e)
	}
	r.Poll()
	if resets != 3 || len(events) != 1 {
		t.Fatal("truncated journal was not rebuilt")
	}
}
func TestCommanderSelectionSideFilesAndLargeIDs(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "Journal.1.log"), []byte("{\"event\":\"LoadGame\",\"FID\":\"FA\"}\n"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "Journal.2.log"), []byte("{\"event\":\"LoadGame\",\"FID\":\"FB\"}\n{\"event\":\"Location\",\"SystemAddress\":18446744073709551614}\n"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "Cargo.json"), []byte("{\"event\":\"Cargo\",\"Inventory\":[]}"), 0600)
	rows := []M{}
	r := New(dir, func(e, m M) { rows = append(rows, e) }, func() { rows = nil })
	r.Poll()
	if len(rows) != 3 || rows[0]["FID"] != "FB" {
		t.Fatal("different commander mixed into replay", rows)
	}
	if v.Str(rows[1]["SystemAddress"]) != "18446744073709551614" {
		t.Fatal("64-bit game identifier lost precision")
	}
	r.Poll()
	if len(rows) != 3 {
		t.Fatal("unchanged side file duplicated")
	}
	if e := r.SetDirectory(filepath.Join(dir, "missing")); e == nil {
		t.Fatal("nonexistent journal directory accepted")
	}
	b, _ := Decode([]byte("\xef\xbb\xbf{\"event\":\"Test\"}"))
	if !strings.Contains(v.Str(b["event"]), "Test") {
		t.Fatal("BOM rejected")
	}
}

func TestRemovedAndEarlierImportedSessionsRebuildInOrder(t *testing.T) {
	dir := t.TempDir()
	write := func(name, tag string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{\"event\":\"LoadGame\",\"FID\":\"FTEST\",\"tag\":\""+tag+"\"}\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("Journal.2.log", "second")
	write("Journal.3.log", "third")
	rows := []string{}
	resets, hooks := 0, 0
	var reader *Reader
	reader = New(dir, func(event, meta M) { rows = append(rows, v.Str(event["tag"])) }, func() { resets++; rows = nil })
	reader.AfterReplay = func() { _ = reader.State(); hooks++; rows = append(rows, "cache") }
	reader.Poll()
	if strings.Join(rows, ",") != "second,third,cache" {
		t.Fatal(rows)
	}
	reader.Poll()
	if hooks != 1 || resets != 1 {
		t.Fatal("unchanged poll invoked rebuild", resets, hooks)
	}
	write("Journal.1.log", "first")
	reader.Poll()
	if strings.Join(rows, ",") != "first,second,third,cache" || resets != 2 {
		t.Fatal("imported history was applied after current session", rows, resets)
	}
	if err := os.Remove(filepath.Join(dir, "Journal.2.log")); err != nil {
		t.Fatal(err)
	}
	reader.Poll()
	if strings.Join(rows, ",") != "first,third,cache" || resets != 3 {
		t.Fatal("removed history remained in totals", rows, resets)
	}
	write("Journal.4.log", "fourth")
	reader.Poll()
	if strings.Join(rows, ",") != "first,third,cache,fourth,cache" || resets != 3 {
		t.Fatal("newer session needlessly reset or hook missed", rows, resets)
	}
	for _, name := range []string{"Journal.1.log", "Journal.3.log", "Journal.4.log"} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	reader.Poll()
	if strings.Join(rows, ",") != "cache" || reader.State()["lastEvent"] != "" {
		t.Fatal("empty history retained events", rows, reader.State())
	}
}
