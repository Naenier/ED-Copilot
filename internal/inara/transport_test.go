package inara

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransportPartialReorderedAcknowledgements(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		var body v.M
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid request")
		}
		if v.Str(v.At(body, "header", "APIkey")) != "synthetic-key-only" || v.Str(v.At(body, "header", "appName")) != "ED Copilot" {
			t.Error("wrong header")
		}
		if len(v.Rows(body["events"])) != 2 {
			t.Error("invalid events not removed")
		}
		fmt.Fprint(w, `{"header":{"eventStatus":200},"events":[{"eventCustomID":2,"eventStatus":200},{"eventCustomID":1,"eventStatus":503,"eventStatusText":"retry"}]}`)
	}))
	defer server.Close()
	events := []Event{{"setCommanderCredits", v.ISO(now), v.M{"commanderCredits": 100, "commanderLoan": 0}}, {"setCommanderTravelLocation", v.ISO(now), v.M{"starsystemName": "Sol"}}, {"unsupported", v.ISO(now), v.M{}}}
	result, err := SendBatch(context.Background(), SendOptions{HTTPClient: server.Client(), Endpoint: server.URL, APIKey: "synthetic-key-only", CommanderName: "Synthetic Pilot", FrontierID: "F1", Events: events, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Results[0].Retryable || result.Results[0].OK || !result.Results[1].OK || result.Results[2].Retryable || result.Results[2].Status != 410 || received.Load() != 1 {
		t.Fatal("partial ACK correspondence", result)
	}
}
func TestTransportHTTPThrottleRedactionAndMalformedResponse(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	event := Event{"setCommanderCredits", v.ISO(now), v.M{"commanderCredits": 100, "commanderLoan": 0}}
	cases := []struct {
		name, body  string
		status      int
		expectError bool
		retry       bool
		setup       bool
	}{{"throttle", "", 429, true, true, false}, {"server", "", 503, true, true, false}, {"validation", "", 400, true, false, false}, {"missing header", `{"events":[]}`, 200, true, true, false}, {"bad json", "secret synthetic-key-only malformed", 200, true, true, false}, {"setup", `{"header":{"eventStatus":400,"eventStatusText":"synthetic-key-only wrong key"}}`, 200, false, false, true}, {"missing event", `{"header":{"eventStatus":200},"events":[]}`, 200, false, true, false}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			result, err := SendBatch(context.Background(), SendOptions{HTTPClient: server.Client(), Endpoint: server.URL, APIKey: "synthetic-key-only", CommanderName: "Synthetic Pilot", Events: []Event{event}, Now: func() time.Time { return now }})
			if test.expectError {
				var issue *Error
				if !errors.As(err, &issue) || issue.Retryable != test.retry {
					t.Fatal("retry classification", err)
				}
				if strings.Contains(err.Error(), "synthetic-key-only") {
					t.Fatal("key leaked in error")
				}
				if test.status == 429 && issue.RetryAfter < 2*time.Minute {
					t.Fatal("Retry-After lost")
				}
			} else {
				if err != nil || result.SetupRequired != test.setup {
					t.Fatal(result, err)
				}
				if strings.Contains(result.Message, "synthetic-key-only") {
					t.Fatal("key leaked in response")
				}
				if test.retry && !result.Results[0].Retryable {
					t.Fatal("unacknowledged event discarded")
				}
			}
		})
	}
}
func TestTransportRedirectAndOversizeRejected(t *testing.T) {
	now := time.Now()
	event := Event{"setCommanderCredits", v.ISO(now), v.M{"commanderCredits": 1, "commanderLoan": 0}}
	var redirectFollowed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/target" {
			redirectFollowed.Store(true)
			fmt.Fprint(w, "{}")
			return
		}
		http.Redirect(w, r, "/target", 307)
	}))
	defer server.Close()
	if _, err := SendBatch(context.Background(), SendOptions{HTTPClient: server.Client(), Endpoint: server.URL, APIKey: "synthetic-key", CommanderName: "Pilot", Events: []Event{event}}); err == nil || redirectFollowed.Load() {
		t.Fatal("credential request followed redirect", err)
	}
	large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("x", 2*1024*1024+1)) }))
	defer large.Close()
	if _, err := SendBatch(context.Background(), SendOptions{HTTPClient: large.Client(), Endpoint: large.URL, APIKey: "synthetic-key", CommanderName: "Pilot", Events: []Event{event}}); err == nil {
		t.Fatal("unbounded response accepted")
	}
	if _, err := SendBatch(context.Background(), SendOptions{APIKey: "bad\nkey", CommanderName: "Pilot", Events: []Event{event}}); err == nil {
		t.Fatal("newline key accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := SendBatch(ctx, SendOptions{HTTPClient: server.Client(), Endpoint: server.URL, APIKey: "synthetic-key", CommanderName: "Pilot", Events: []Event{event}}); err == nil {
		t.Fatal("cancelled send attempted")
	}
}
