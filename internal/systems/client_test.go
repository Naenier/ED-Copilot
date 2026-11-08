package systems

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func options(server *httptest.Server) Options {
	return Options{HTTPClient: server.Client(), Endpoint: server.URL, MinInterval: -1}
}
func query(t *testing.T, c *Client, name string, seq uint64) Result {
	t.Helper()
	got, err := c.Query(context.Background(), Request{Query: name, FieldID: "input", Sequence: seq})
	if err != nil {
		t.Fatal(err)
	}
	return got
}
func TestPublicWireShapeCanonicalNamesAndPrecisionArePreserved(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.Query().Get("q") != "Synthetic Alpha" || r.Header.Get("Accept") != "application/json" || r.Header.Get("User-Agent") == "" {
			t.Error("wrong public autocomplete request")
		}
		fmt.Fprint(w, `{"values":["Synthetic Alpha","Synthetic Alphabet","Unrelated station","Synthetic Alpha"],"min_max":[{"name":"Synthetic Alpha","id64":18446744073709551615,"x":0,"y":1.5,"z":-2}]}`)
	}))
	defer server.Close()
	c := NewClient(options(server))
	defer c.Close()
	r := query(t, c, "Synthetic Alpha", 1)
	if r.Status != "ready" || len(r.Suggestions) != 2 || r.Suggestions[0].Name != "Synthetic Alpha" || r.Suggestions[0].Match != "exact" || r.Suggestions[0].ID64 != "18446744073709551615" || r.Suggestions[0].Coordinates == nil || r.Suggestions[0].Coordinates.X != 0 || r.Suggestions[1].Match != "prefix" {
		t.Fatal("canonical name, precise ID64, zero coordinate or relevance lost", r)
	}
	r.Suggestions[0].Name = "mutated"
	r.Suggestions[0].Coordinates.X = 999
	r = query(t, c, "  SYNTHETIC   ALPHA ", 2)
	if !r.Cached || calls.Load() != 1 || r.Query != "SYNTHETIC ALPHA" || r.Sequence != 2 || r.Suggestions[0].Name != "Synthetic Alpha" || r.Suggestions[0].Coordinates.X != 0 {
		t.Fatal("cache leaked aliases or previous request identity", r)
	}
}
func TestSagittariusTypoRequiresCanonicalProviderConfirmation(t *testing.T) {
	for _, confirmed := range []bool{true, false} {
		t.Run(fmt.Sprint(confirmed), func(t *testing.T) {
			var queries []string
			var mu sync.Mutex
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query().Get("q")
				mu.Lock()
				queries = append(queries, q)
				mu.Unlock()
				if q == "Sagittarius A*" && confirmed {
					fmt.Fprint(w, `{"values":["Sagittarius A*"],"min_max":[{"name":"Sagittarius A*","id64":20578934,"x":25.21875,"y":-20.90625,"z":25899.96875}]}`)
				} else {
					fmt.Fprint(w, `{"values":["A Capricorni","Aunair AA-A f1216"]}`)
				}
			}))
			defer server.Close()
			c := NewClient(options(server))
			defer c.Close()
			r := query(t, c, "Sagittarious A*", 1)
			if len(queries) != 2 || queries[1] != "Sagittarius A*" {
				t.Fatal("correction was not verified with the provider", queries)
			}
			if confirmed {
				if len(r.Suggestions) != 1 || r.Suggestions[0].Name != "Sagittarius A*" || r.Suggestions[0].Match != "correction" || r.Query != "Sagittarious A*" {
					t.Fatal("canonical correction or original input lost", r)
				}
			} else if r.Status != "empty" || len(r.Suggestions) != 0 {
				t.Fatal("invented a system absent from the provider", r)
			}
		})
	}
}
func TestShortInvalidAndBroadQueriesAreBounded(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		values := []string{}
		for i := 0; i < 80; i++ {
			values = append(values, fmt.Sprintf("HIP %d", i))
		}
		json.NewEncoder(w).Encode(map[string]any{"values": values})
	}))
	defer server.Close()
	c := NewClient(options(server))
	defer c.Close()
	for i, q := range []string{"", "S", "So", "***"} {
		r := query(t, c, q, uint64(i+1))
		if r.Status != "too-short" || len(r.Suggestions) != 0 {
			t.Fatal("short query sent a search", r)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("short input reached provider")
	}
	for i, q := range []string{strings.Repeat("x", 201), "Sol\x00"} {
		r, err := c.Query(context.Background(), Request{Query: q, FieldID: "bad", Sequence: uint64(i + 1)})
		if err == nil || r.ErrorCode != "INVALID_QUERY" {
			t.Fatal("invalid input accepted", r)
		}
	}
	r, err := c.Query(context.Background(), Request{Query: "HIP", FieldID: "broad", Sequence: 1, Limit: 10000})
	if err != nil || len(r.Suggestions) != MaxResults || !r.Limited {
		t.Fatal("broad response/caller limit exceeded cap", r, err)
	}
	r, err = c.Query(context.Background(), Request{Query: "HIP", FieldID: "broad", Sequence: 2, Limit: 3})
	if err != nil || len(r.Suggestions) != 3 || !r.Cached || !r.Limited || calls.Load() != 1 {
		t.Fatal("bounded cached projection failed", r, err)
	}
}
func TestCacheUsesTTLNegativeTTLAndLRUEviction(t *testing.T) {
	now := time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		q := r.URL.Query().Get("q")
		if q == "Absent" {
			fmt.Fprint(w, `{"values":[]}`)
		} else {
			json.NewEncoder(w).Encode(map[string]any{"values": []string{q}})
		}
	}))
	defer server.Close()
	o := options(server)
	o.Now = func() time.Time { return now }
	o.CacheSize = 2
	o.TTL = time.Minute
	c := NewClient(o)
	defer c.Close()
	query(t, c, "Alpha", 1)
	query(t, c, "Beta", 2)
	query(t, c, "Alpha", 3)
	query(t, c, "Gamma", 4)
	if len(c.cache) != 2 || calls.Load() != 3 {
		t.Fatal("LRU grew beyond bound")
	}
	if !query(t, c, "Alpha", 5).Cached {
		t.Fatal("recently used entry evicted")
	}
	if query(t, c, "Beta", 6).Cached || calls.Load() != 4 {
		t.Fatal("least recent entry survived eviction")
	}
	now = now.Add(61 * time.Second)
	if query(t, c, "Beta", 7).Cached {
		t.Fatal("expired entry served")
	}
	query(t, c, "Absent", 8)
	if !query(t, c, "Absent", 9).Cached {
		t.Fatal("negative result not cached")
	}
	now = now.Add(31 * time.Second)
	if query(t, c, "Absent", 10).Cached {
		t.Fatal("negative result hid new provider data for too long")
	}
}
func TestNewSequenceCancelsOnlyItsOwnFieldAndRejectsLateResponses(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	var oldOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "Old System" {
			oldOnce.Do(func() { close(started) })
			<-r.Context().Done()
			close(cancelled)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"values": []string{q}})
	}))
	defer server.Close()
	c := NewClient(options(server))
	defer c.Close()
	type response struct {
		r   Result
		err error
	}
	done := make(chan response, 1)
	go func() {
		r, e := c.Query(context.Background(), Request{Query: "Old System", FieldID: "left", Sequence: 1})
		done <- response{r, e}
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("old query did not start")
	}
	newResult, err := c.Query(context.Background(), Request{Query: "New System", FieldID: "left", Sequence: 2})
	if err != nil || newResult.Status != "ready" || newResult.Sequence != 2 {
		t.Fatal("newer query was lost", newResult, err)
	}
	select {
	case old := <-done:
		if !errors.Is(old.err, context.Canceled) || old.r.Status != "cancelled" || len(old.r.Suggestions) != 0 {
			t.Fatal("old result survived cancellation", old)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("superseded query kept running")
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP request was not cancelled")
	}
	c.Cancel("left", 1)
	other, err := c.Query(context.Background(), Request{Query: "Other System", FieldID: "right", Sequence: 1})
	if err != nil || other.Status != "ready" {
		t.Fatal("another field was cancelled", other, err)
	}
	stale, err := c.Query(context.Background(), Request{Query: "Old System", FieldID: "left", Sequence: 1})
	if !errors.Is(err, context.Canceled) || stale.Status != "cancelled" {
		t.Fatal("out-of-order request restarted old search", stale, err)
	}
	c.Cancel("cancel-before-start", 3)
	if r, e := c.Query(context.Background(), Request{Query: "Never Requested", FieldID: "cancel-before-start", Sequence: 3}); !errors.Is(e, context.Canceled) || r.Status != "cancelled" {
		t.Fatal("queued cancelled query began later", r, e)
	}
}
func TestRateLimitBackoffDoesNotRetryOrLeakProviderBody(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
		fmt.Fprint(w, "private provider diagnostic")
	}))
	defer server.Close()
	c := NewClient(options(server))
	defer c.Close()
	for seq := uint64(1); seq <= 2; seq++ {
		r, e := c.Query(context.Background(), Request{Query: fmt.Sprintf("Query %d", seq), FieldID: "rate", Sequence: seq})
		if e == nil || r.Status != "rate-limited" || r.ErrorCode != "RATE_LIMIT" || r.RetryAfterMS < 119000 || strings.Contains(e.Error(), "private") {
			t.Fatal("rate limit classification/backoff failed", r, e)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("rate-limited provider was queried again immediately")
	}
}
func TestMalformedOversizeHTTPAndRedirectResponsesAreSafe(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"malformed", "not json", 200, "RESPONSE_INVALID"}, {"shape", "{}", 200, "RESPONSE_INVALID"}, {"oversize", strings.Repeat("x", maxResponseBytes+1), 200, "RESPONSE_TOO_LARGE"}, {"server", "diagnostic", 503, "HTTP_503"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			c := NewClient(options(server))
			defer c.Close()
			r, e := c.Query(context.Background(), Request{Query: "Synthetic", FieldID: "bad", Sequence: 1})
			if e == nil || r.ErrorCode != tc.want || len(r.Suggestions) != 0 {
				t.Fatal("unsafe failure response", r, e)
			}
		})
	}
	var followed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/next" {
			followed.Store(true)
		}
		http.Redirect(w, r, "/next", 302)
	}))
	defer server.Close()
	c := NewClient(options(server))
	defer c.Close()
	r, e := c.Query(context.Background(), Request{Query: "Synthetic", FieldID: "redirect", Sequence: 1})
	if e == nil || r.ErrorCode != "HTTP_302" || followed.Load() {
		t.Fatal("unexpected redirect followed", r, e)
	}
}
func TestRequestSpacingAndClosedClientKeepWorkBounded(t *testing.T) {
	var mu sync.Mutex
	times := []time.Time{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"values": []string{r.URL.Query().Get("q")}})
	}))
	defer server.Close()
	o := options(server)
	o.MinInterval = 25 * time.Millisecond
	c := NewClient(o)
	query(t, c, "Alpha", 1)
	query(t, c, "Beta", 2)
	if len(times) != 2 || times[1].Sub(times[0]) < 20*time.Millisecond {
		t.Fatal("provider spacing was not respected", times)
	}
	for i := 0; i < 100; i++ {
		c.Query(context.Background(), Request{Query: "x", FieldID: fmt.Sprint(i), Sequence: 1})
	}
	if len(c.scopes) > maxScopes {
		t.Fatal("per-input cancellation state grew unbounded")
	}
	c.Close()
	if len(c.cache) != 0 || len(c.scopes) != 0 {
		t.Fatal("closed client retained ephemeral data")
	}
	r, e := c.Query(context.Background(), Request{Query: "Gamma", FieldID: "closed", Sequence: 1})
	if !errors.Is(e, context.Canceled) || r.Status != "cancelled" {
		t.Fatal("closed client accepted work", r, e)
	}
}
