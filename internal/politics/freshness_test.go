package politics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/freshness"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestPublicRefreshDistinguishesEmptyPartialRateLimitAndMissingNetwork(t *testing.T) {
	now := testClock()
	var mode atomic.Value
	mode.Store("empty")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch mode.Load() {
		case "empty":
			fmt.Fprint(w, `{"activeInitiatives":[]}`)
		case "partial":
			fmt.Fprint(w, `{"activeInitiatives":[{"id":"1","title":"Example"},{"title":"No ID"}]}`)
		case "limited":
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	s := New(nil, Options{HTTPClient: server.Client(), GoalsURL: server.URL, Now: func() time.Time { return now }})
	body := M{"kind": "goals"}
	fetch := func() error { _, err := s.Fetch(context.Background(), body); return err }
	quality := func() freshness.Observation {
		return v.At(s.Snapshot(), "communityGoals", "quality").(freshness.Observation)
	}
	if err := fetch(); err != nil || quality().Coverage != freshness.Empty {
		t.Fatal("successful empty response was not confirmed", err, quality())
	}
	now = now.Add(6 * time.Minute)
	mode.Store("partial")
	if err := fetch(); err != nil || quality().Coverage != freshness.Partial {
		t.Fatal("usable records were not retained as partial", err, quality())
	}
	now = now.Add(6 * time.Minute)
	mode.Store("limited")
	if err := fetch(); err == nil || v.At(s.Snapshot(), "status", "reason") != "rate-limited" {
		t.Fatal("429 not recognized", err, s.Snapshot()["status"])
	}
	retry := v.Time(v.At(s.Snapshot(), "status", "retryAt"))
	if !retry.Equal(now.Add(2*time.Minute)) || quality().Coverage != freshness.Partial {
		t.Fatal("retry schedule or cached data was lost", retry, quality())
	}
	now = now.Add(90 * time.Second)
	if err := fetch(); err == nil || calls.Load() != 3 {
		t.Fatal("provider retry interval ignored", err, calls.Load())
	}
	now = now.Add(time.Minute)
	mode.Store("missing")
	if err := fetch(); err == nil || v.At(s.Snapshot(), "status", "reason") != "incomplete" || quality().Coverage != freshness.Partial {
		t.Fatal("incomplete response replaced cached observations", err)
	}
	now = now.Add(6 * time.Minute)
	server.Close()
	if err := fetch(); err == nil || v.At(s.Snapshot(), "status", "reason") != "offline" {
		t.Fatal("network failure was not identified", err, s.Snapshot()["status"])
	}
}

func TestPoliticalRefreshCannotRevertAnExplicitSystemSelection(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/systems/field_values/system_names":
			fmt.Fprint(w, `{"min_max":[{"name":"First","id64":1}]}`)
		case "/system/1":
			close(entered)
			<-release
			fmt.Fprint(w, `{"record":{"name":"First","id64":1,"population":25}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	s := New(nil, Options{HTTPClient: server.Client(), SpanshURL: server.URL, Now: testClock})
	done := make(chan error, 1)
	go func() {
		_, err := s.Fetch(context.Background(), M{"kind": "system", "system": "First"})
		done <- err
	}()
	<-entered
	err := s.Update(M{"action": "select-system", "system": "Second"})
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("obsolete response applied", err)
	}
	snapshot := s.Snapshot()
	if snapshot["selectedSystem"] != "Second" || len(v.Rows(snapshot["systems"])) != 0 || v.At(snapshot, "status", "running") != false {
		t.Fatal("obsolete response changed the active system", snapshot)
	}
}
