package edsm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := NewClient(server.Client())
	client.endpoint = server.URL
	client.interval = 0
	return client
}

func TestExactSystemPreservesIDsAndCachesCanonicalQueries(t *testing.T) {
	var calls atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		query := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/api-v1/system" || query.Get("systemName") != "Sagittarius A*" ||
			query.Get("showId") != "1" || query.Get("showCoordinates") != "1" || query.Get("showInformation") != "1" ||
			query.Get("showPrimaryStar") != "1" || query.Get("showPermit") != "1" || query.Get("apiKey") != "" {
			t.Errorf("unexpected EDSM request: %s %s", r.Method, r.URL)
		}
		fmt.Fprint(w, `{"name":"Sagittarius A*","id64":9007199254740993,"coords":{"x":0,"y":0,"z":1},"information":{"population":42}}`)
	})
	got, err := client.System(t.Context(), " Sagittarius A* ")
	if err != nil || got["id64"] != json.Number("9007199254740993") {
		t.Fatalf("first lookup returned %v, %v", got, err)
	}
	got["name"] = "changed by caller"
	got["information"].(Object)["population"] = 0
	got, err = client.System(t.Context(), "sagittarius a*")
	if err != nil || got["name"] != "Sagittarius A*" || got["information"].(Object)["population"] != json.Number("42") || calls.Load() != 1 {
		t.Fatalf("cached lookup returned %v, %v with %d requests", got, err, calls.Load())
	}
}

func TestEDSMRateLimitPausesOtherEndpointsAndServesCachedData(t *testing.T) {
	var calls atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case systemData.path:
			fmt.Fprint(w, `{"name":"Sol","id":27}`)
		case bodiesData.path:
			w.Header().Set("Retry-After", "90")
			w.WriteHeader(http.StatusTooManyRequests)
		case stationData.path:
			fmt.Fprint(w, `{"name":"Sol","stations":[]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	now := time.Now()
	client.now = func() time.Time { return now }
	if _, err := client.System(t.Context(), "Sol"); err != nil {
		t.Fatal(err)
	}
	checkRate := func(err error) {
		t.Helper()
		var limit *RateLimitError
		if !errors.As(err, &limit) || !limit.RetryAt.Equal(now.Add(90*time.Second)) {
			t.Fatalf("unexpected rate limit error: %v", err)
		}
	}
	_, err := client.Bodies(t.Context(), "Sol")
	checkRate(err)
	_, err = client.Stations(t.Context(), "Sol")
	checkRate(err)
	if _, err := client.System(t.Context(), "sol"); err != nil || calls.Load() != 2 {
		t.Fatalf("during cooldown received %v with %d requests", err, calls.Load())
	}
	now = now.Add(91 * time.Second)
	if _, err := client.Stations(t.Context(), "Sol"); err != nil || calls.Load() != 3 {
		t.Fatalf("after cooldown received %v with %d requests", err, calls.Load())
	}
}

func TestEDSMSharesRequestWhenOneCallerCancels(t *testing.T) {
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
			fmt.Fprint(w, `{"name":"Sol","id":27}`)
		case <-r.Context().Done():
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { _, err := client.System(ctx, "Sol"); first <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first HTTP request did not start")
	}
	go func() { _, err := client.System(t.Context(), "sol"); second <- err }()
	deadline := time.Now().Add(time.Second)
	for {
		client.mu.Lock()
		shared := false
		for _, item := range client.pending {
			shared = item.waiters == 2
		}
		client.mu.Unlock()
		if shared {
			break
		}
		if time.Now().After(deadline) {
			close(release)
			t.Fatal("second caller did not join the shared request")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("first request returned %v", err)
	}
	close(release)
	if err := <-second; err != nil || calls.Load() != 1 {
		t.Fatalf("second request returned %v with %d HTTP calls", err, calls.Load())
	}
}

func TestEDSMBoundedLookupsAndEmptyResponseShapes(t *testing.T) {
	var calls atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case systemData.path:
			fmt.Fprint(w, "[]")
		case nearbyData.path:
			if r.URL.Query().Get("radius") != "1" || r.URL.Query().Get("showCoordinates") != "1" {
				t.Errorf("nearby query = %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, "{}")
		case prefixData.path:
			if r.URL.Query().Get("systemName") == "Wregoe MC-Y" {
				fmt.Fprint(w, `[{"name":"Wregoe MC-Y b44-0"},{"name":"Wregoe MC-Y b44-1"}]`)
				return
			}
			// A chunked body has no Content-Length. The actual read limit
			// must protect against an unexpectedly large prefix response.
			w.(http.Flusher).Flush()
			fmt.Fprint(w, strings.Repeat(" ", int(prefixData.maxBytes)+2))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	for range 2 {
		if _, err := client.System(t.Context(), "Unknown system"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("missing system returned %v", err)
		}
	}
	if nearby, err := client.Nearby(t.Context(), "Sol", 1); err != nil || len(nearby) != 0 {
		t.Fatalf("nearby returned %v, %v", nearby, err)
	}
	if _, err := client.Nearby(t.Context(), "Sol", 101); err == nil {
		t.Fatal("radius above provider limit was accepted")
	}
	if _, err := client.Prefix(t.Context(), "Synuefe", 12); !errors.Is(err, ErrTooManyMatches) {
		t.Fatalf("broad prefix returned %v", err)
	}
	if _, err := client.Prefix(t.Context(), "Some_prefix", 12); !errors.Is(err, ErrTooManyMatches) {
		t.Fatalf("wildcard prefix returned %v", err)
	}
	if _, err := client.Prefix(t.Context(), "Wregoe MC-Y", 1); !errors.Is(err, ErrTooManyMatches) {
		t.Fatalf("too many rows returned %v", err)
	}
	if matches, err := client.Prefix(t.Context(), "Wregoe MC-Y", 2); err != nil || len(matches) != 2 {
		t.Fatalf("specific prefix returned %v, %v", matches, err)
	}
	if _, err := client.Prefix(t.Context(), "Blaa Eohn LX-P", 12); !errors.Is(err, ErrTooManyMatches) {
		t.Fatalf("oversize prefix returned %v", err)
	}
	if calls.Load() != 4 {
		t.Fatalf("expected four HTTP requests for validated misses, got %d", calls.Load())
	}
}

func TestEDSMDoesNotRetryForbiddenResponses(t *testing.T) {
	var calls atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "<html>Access challenge</html>")
	})
	_, err := client.Factions(t.Context(), "Sol")
	if err == nil || err.Error() != "EDSM returned HTTP 403" || calls.Load() != 1 {
		t.Fatalf("403 returned %v with %d HTTP requests", err, calls.Load())
	}
}
