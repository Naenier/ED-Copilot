package diagnostics

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWeeklyRotationAndRetention(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "diagnostics")
	now := time.Date(2026, 11, 29, 23, 59, 59, 0, time.UTC)
	store := New(dir)
	store.now = func() time.Time { return now }
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("creating the store unexpectedly touched the disk: %v", err)
	}
	store.Log(Info, "app", "boot", "Sunday")
	now = now.Add(time.Second)
	store.Log(Info, "app", "boot", "Monday")

	for _, week := range []string{"2026-11-23", "2026-11-30"} {
		if _, err := os.Stat(filepath.Join(dir, segment{week: week}.name())); err != nil {
			t.Fatal(err)
		}
	}
	unrelated := filepath.Join(dir, "other.jsonl")
	if err := os.WriteFile(unrelated, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 5; index++ {
		now = now.AddDate(0, 0, 7)
		store.Log(Warn, "journal", "retry", fmt.Sprint(index))
	}
	files, err := store.segments()
	if err != nil || len(files) != weeksToKeep {
		t.Fatalf("segments = %+v, %v", files, err)
	}
	if _, err = os.Stat(unrelated); err != nil {
		t.Fatal("rotation removed a file outside the log naming scheme", err)
	}
	page, err := store.Query(Filter{})
	if err != nil || len(page.Entries) != weeksToKeep || page.Entries[0].Message != "5" ||
		page.Entries[weeksToKeep-1].Message != "2" {
		t.Fatalf("page = %+v, %v", page, err)
	}
}

func TestBoundedSegmentsWithinAWeek(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	now := time.Now()
	store.now = func() time.Time { return now }
	week := monday(now).Format(time.DateOnly)
	for i := range segmentsPerWeek {
		path := filepath.Join(dir, segment{week: week, index: i}.name())
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = errors.Join(file.Truncate(segmentBytes), file.Close())
		if err != nil {
			t.Fatal(err)
		}
	}
	store.Log(Info, "app", "open", "Newest")
	files, err := store.segments()
	if err != nil || len(files) != segmentsPerWeek || files[0].index != segmentsPerWeek || files[len(files)-1].index != 1 {
		t.Fatalf("size rotation failed: %+v, %v", files, err)
	}
	page, err := store.Query(Filter{})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Message != "Newest" {
		t.Fatalf("new record was lost: %+v, %v", page, err)
	}
}

func TestCursorAndFiltersStayStableDuringAppends(t *testing.T) {
	store := New(t.TempDir())
	store.Log(Warn, "journal", "replay", "Чтение завершилось")
	store.Log(Info, "journal", "replay", "Чтение началось")
	store.Log(Warn, "network", "retry", "Повторный запрос")
	store.Log(Warn, "journal", "replay", "ЧТЕНИЕ прервано")

	filter := Filter{Level: Warn, Subsystem: "journal", Search: "чТеНиЕ", Limit: 1}
	page, err := store.Query(filter)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Message != "ЧТЕНИЕ прервано" ||
		page.Next != page.Entries[0].ID || !reflect.DeepEqual(page.Subsystems, []string{"journal", "network"}) {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	store.Log(Warn, "journal", "replay", "Чтение возобновилось")
	filter.Before = page.Next
	page, err = store.Query(filter)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Message != "Чтение завершилось" || page.Next != "" {
		t.Fatalf("older page = %+v, %v", page, err)
	}
	for _, bad := range []Filter{{Before: "../settings:20"}, {Level: "critical"}, {Subsystem: "a b"}} {
		if _, err := store.Query(bad); !errors.Is(err, ErrFilter) {
			t.Fatalf("filter %+v accepted: %v", bad, err)
		}
	}
}

func TestSearchMatchesDisplayedAndOriginalText(t *testing.T) {
	store := New(t.TempDir())
	store.Log(Info, "journal", "replay", "Чтение завершилось")
	match := func(entry Entry) string {
		if entry.Event == "replay" {
			return "Journal replay completed"
		}
		return ""
	}
	for _, search := range []string{"COMPLETED", "ЗАВЕРШИЛОСЬ", "replay"} {
		page, err := store.Query(Filter{Search: search, MatchText: match})
		if err != nil || len(page.Entries) != 1 {
			t.Fatalf("search %q = %+v, %v", search, page, err)
		}
	}
	_, err := store.Query(Filter{MatchText: func(Entry) string { t.Fatal("unfiltered query localized a record"); return "" }})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSubsystemIndexFollowsAppendsAndRetention(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	store.Log(Info, "app", "open", "Initial")
	page, err := store.Query(Filter{})
	if err != nil || !reflect.DeepEqual(page.Subsystems, []string{"app"}) {
		t.Fatalf("initial subsystems = %v, %v", page.Subsystems, err)
	}
	store.Log(Warn, "journal", "replay", "New area")
	page, err = store.Query(Filter{Subsystem: "journal"})
	if err != nil || len(page.Entries) != 1 || !reflect.DeepEqual(page.Subsystems, []string{"app", "journal"}) {
		t.Fatalf("append did not refresh the index: %+v, %v", page, err)
	}
	now = now.AddDate(0, 0, 7*weeksToKeep)
	store.Log(Info, "network", "status", "New week")
	page, err = store.Query(Filter{})
	if err != nil || len(page.Entries) != 1 || !reflect.DeepEqual(page.Subsystems, []string{"network"}) {
		t.Fatalf("expired subsystems remained in index: %+v, %v", page, err)
	}
	reopened := New(dir)
	reopened.now = store.now
	cold, err := reopened.Query(Filter{})
	if err != nil || !reflect.DeepEqual(cold, page) {
		t.Fatalf("cold and cached results differ: %+v, %+v, %v", cold, page, err)
	}
}

func TestRestartRecoversAnIncompleteAppend(t *testing.T) {
	dir := t.TempDir()
	first := New(dir)
	first.Log(Info, "app", "boot", "Initial")
	files, err := first.segments()
	if err != nil || len(files) != 1 {
		t.Fatalf("files = %+v, %v", files, err)
	}
	file, err := os.OpenFile(filepath.Join(dir, files[0].name()), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"timestamp":`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	next := New(dir)
	next.Log(Info, "app", "boot", "Следующая\nстрока"+strings.Repeat("я", 800))
	page, err := next.Query(Filter{})
	if err != nil || len(page.Entries) != 2 || !strings.HasPrefix(page.Entries[0].Message, "Следующая строка") ||
		len(page.Entries[0].Message) > 605 || page.Entries[1].Message != "Initial" {
		t.Fatalf("restart recovery: %+v, %v", page, err)
	}
}

func TestConcurrentAppendsRemainComplete(t *testing.T) {
	store := New(t.TempDir())
	var workers sync.WaitGroup
	const threads, count = 8, 20
	for thread := range threads {
		workers.Go(func() {
			for i := range count {
				store.Log(Debug, "app", "task", fmt.Sprintf("%d/%d", thread, i))
			}
		})
	}
	workers.Wait()
	page, err := store.Query(Filter{Limit: threads * count})
	if err != nil || len(page.Entries) != threads*count {
		t.Fatalf("read %d records, %v", len(page.Entries), err)
	}
	seen := make(map[string]bool)
	for _, entry := range page.Entries {
		if seen[entry.Message] {
			t.Fatalf("duplicate entry %q", entry.Message)
		}
		seen[entry.Message] = true
	}
}

func TestUnavailableLogRecoversAfterItsRetryWindow(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	if err := os.WriteFile(dir, []byte("obstructed"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	store := New(dir)
	store.now = func() time.Time { return now }
	store.Log(Info, "app", "boot", "Failed append")
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	store.Log(Info, "app", "boot", "Within retry window")
	if _, err := store.Query(Filter{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("the viewer should report the failure during backoff: %v", err)
	}

	now = now.Add(time.Second)
	store.Log(Info, "app", "boot", "Recovered")
	page, err := store.Query(Filter{})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Message != "Recovered" {
		t.Fatalf("the store did not recover: %+v, %v", page, err)
	}
}
