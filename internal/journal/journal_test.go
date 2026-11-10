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

func TestReplayAcceptsOnlyCompleteSingleObjectsAndMatchingSideFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	const name = "Journal.1.log"
	write(name, strings.Join([]string{
		"\xef\xbb\xbf{\"event\":\"fileheader\",\"gameversion\":\"4.4.0\"}",
		`{"event":"LoadGame","timestamp":"2026-01-10T00:00:00Z","FID":"FTEST"}`,
		`{"event":"Location","SystemAddress":18446744073709551615}`,
		`{"event":"MarketBuy","Count":99}{"event":"MarketBuy","Count":100}`,
		`null`, `[{"event":"MarketBuy"}]`, `{"event":71}`, `{"event":" "}`,
		"{\"event\":\"MarketSell\",\"Name\":\"\xff\"}",
		`{"event":"SyntheticFutureEvent","Unknown":true}`,
		`{"event":"FSDJump","StarSystem":"Incomplete"}`,
	}, "\n"))
	write("Status.json", `{"event":"LoadGame","FID":"FSPOOF"}`)
	write("Cargo.json", `{"event":"Cargo","timestamp":"2026-01-09T23:59:59Z","Count":99}`)
	write("NavRoute.json", `{"Route":[]}`)

	var events []string
	reader := New(dir, func(e, meta M) {
		events = append(events, v.Str(e["event"]))
		if e["event"] == "Location" && v.Str(e["SystemAddress"]) != "18446744073709551615" {
			t.Fatalf("lossy system address: %v", e["SystemAddress"])
		}
	}, func() { events = nil })
	want := "Fileheader,LoadGame,Location,SyntheticFutureEvent,NavRoute"
	for i := 0; i < 3; i++ {
		if i == 2 {
			reader.Rescan()
		}
		reader.Poll()
		if got := strings.Join(events, ","); got != want || v.Int(reader.State()["eventCount"]) != 4 {
			t.Fatalf("poll %d: events %s, state %v", i, got, reader.State())
		}
	}
	write("Cargo.json", `{"event":"Cargo","timestamp":"2026-01-10T00:00:01Z","Count":0}`)
	reader.Poll()
	if got := strings.Join(events, ","); got != want+",Cargo" {
		t.Fatalf("new valid cargo snapshot missed: %s", got)
	}
	write("NavRoute.json", `{"event":"NavRouteClear","timestamp":"2026-01-10T00:00:02Z"}`)
	reader.Poll()
	if got := strings.Join(events, ","); got != want+",Cargo,NavRouteClear" {
		t.Fatalf("valid route clear missed: %s", got)
	}
}

func TestReplayKeepsCommanderUntilNewIdentityLineIsComplete(t *testing.T) {
	dir := t.TempDir()
	const header = "{\"event\":\"Fileheader\",\"gameversion\":\"4.4.0\"}\n"
	older := filepath.Join(dir, "Journal.1.log")
	newer := filepath.Join(dir, "Journal.2.log")
	const first = `{"event":"LoadGame","FID":"FTEST-A"}`
	const second = `{"event":"LoadGame","FID":"FTEST-B"}`
	if err := os.WriteFile(older, []byte(header+first+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte(header+second), 0600); err != nil {
		t.Fatal(err)
	}
	var owners []string
	reader := New(dir, func(e, meta M) {
		if fid := v.Str(e["FID"]); fid != "" {
			owners = append(owners, fid)
		}
	}, func() { owners = nil })
	reader.Poll()
	if got := strings.Join(owners, ","); got != "FTEST-A" {
		t.Fatalf("unterminated line changed commander: %s", got)
	}
	f, err := os.OpenFile(newer, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	reader.Poll()
	if got := strings.Join(owners, ","); got != "FTEST-B" {
		t.Fatalf("completed identity did not rebuild history: %s", got)
	}
}

func TestReplaySeparatesKnownGameModesAndRechecksReplacedIdentity(t *testing.T) {
	dir := t.TempDir()
	write := func(name, version, fid, tag string) {
		t.Helper()
		data := "{\"event\":\"Fileheader\",\"gameversion\":\"" + version + "\"}\n" +
			"{\"event\":\"LoadGame\",\"FID\":\"" + fid + "\",\"tag\":\"" + tag + "\"}\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("Journal.1.log", "3.8.0", "FTEST-A", "legacy")
	write("Journal.2.log", "4.4.0", "FTEST-A", "live")
	write("Journal.3.log", "4.4.0", "FTEST-A", "copy")
	var history []string
	reader := New(dir, func(e, meta M) {
		if e["tag"] != nil {
			history = append(history, v.Str(e["tag"]))
		}
	}, func() { history = nil })
	reader.Poll()
	if got := strings.Join(history, ","); got != "live,copy" {
		t.Fatalf("mixed game branches: %s", got)
	}
	write("Journal.3.log", "3.8.0", "FTEST-B", "swap")
	reader.Rescan()
	reader.Poll()
	if got := strings.Join(history, ","); got != "swap" {
		t.Fatalf("manual replay reused cached file identity: %s", got)
	}
	for version, want := range map[string]string{" 4.4.0 ": "live", "3.8.0": "legacy", "4.0.0 Beta": "", "4.0 alpha": "", "": ""} {
		if got := GameMode(version); got != want {
			t.Errorf("GameMode(%q) = %q, want %q", version, got, want)
		}
	}
}

func TestContinuationUsesItsOwnSessionCommander(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, part string, rows string) {
		t.Helper()
		header := "{\"event\":\"Fileheader\",\"gameversion\":\"4.4.0\",\"part\":" + part + "}\n"
		if err := os.WriteFile(filepath.Join(dir, "Journal."+name+".log"), []byte(header+rows), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("older.01", "1", "{\"event\":\"LoadGame\",\"FID\":\"FTEST-B\"}\n")
	write("older.02", "2", "{\"event\":\"FSDJump\",\"StarSystem\":\"Other Commander\"}\n")
	write("recent.01", "1", "{\"event\":\"LoadGame\",\"FID\":\"FTEST-A\"}\n")
	write("recent.02", "2", "{\"event\":\"FSDJump\",\"StarSystem\":\"Current Commander\"}\n")
	write("unfinished.01", "1", "{\"event\":\"FSDJump\",\"StarSystem\":\"No Identity\"}\n")
	var systems []string
	reader := New(dir, func(event, meta M) {
		if event["event"] == "FSDJump" {
			systems = append(systems, v.Str(event["StarSystem"]))
		}
	}, func() { systems = nil })
	reader.Poll()
	reader.Rescan()
	reader.Poll()
	if got := strings.Join(systems, ","); got != "Current Commander" {
		t.Fatalf("replayed another commander's continuation or an unconfirmed session: %s", got)
	}
}
