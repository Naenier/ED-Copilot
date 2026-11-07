package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestEngineStopAndReplaceSearchRejectLatePreviousResults(t *testing.T) {
	for _, kind := range []string{"market", "mining"} {
		for _, operation := range []string{"stop", "replace"} {
			t.Run(kind+"/"+operation, func(t *testing.T) {
				e, _, _ := fixtureEngine(t, nil)
				base := &economyTransport{calls: map[string]int{}, stamp: v.ISO(time.Now())}
				started, cancelled, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
				var releaseOnce sync.Once
				defer releaseOnce.Do(func() { close(release) })
				var mu sync.Mutex
				seen := false
				e.opts.HTTPClient = &http.Client{Transport: economyRoundTripper(func(r *http.Request) (*http.Response, error) {
					matches := kind == "market" && r.URL.Path == "/api/stations/search" || kind == "mining" && r.URL.Path == "/api-v1/sphere-systems"
					mu.Lock()
					old := matches && !seen
					if old {
						seen = true
					}
					mu.Unlock()
					if !old {
						return base.RoundTrip(r)
					}
					close(started)
					// Deliver a late response even after observing cancellation. This tests
					// the app's job boundary independently of net/http's usual early exit.
					select {
					case <-r.Context().Done():
						close(cancelled)
					case <-release:
						return nil, context.Canceled
					}
					<-release
					defer close(returned)
					if kind == "mining" {
						return base.RoundTrip(r)
					}
					payload := M{"count": 1, "results": []M{{"market_id": 199, "name": "Late previous buyer", "system_name": "Old destination", "type": "Coriolis", "has_large_pad": true, "market_updated_at": base.stamp, "system_x": 1, "system_y": 0, "system_z": 0, "distance_to_arrival": 10, "market": []M{{"commodity": "Gold", "sell_price": 1, "demand": 100}}}}}
					raw, _ := json.Marshal(payload)
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw)), Request: r}, nil
				})}
				e.opts.DisableNetwork = false
				path, stop, name, body := "/api/trading/search", "/api/trading/cancel", "trading", M{"mode": "sell", "resourceKey": "gold", "useCargo": true}
				if kind == "mining" {
					path, stop, name, body = "/api/search", "/api/search/cancel", "search", M{"system": "Synthetic Origin", "resourceKey": "platinum"}
				}
				doOK(t, e, path, body)
				select {
				case <-started:
				case <-time.After(3 * time.Second):
					t.Fatal("first request did not start")
				}
				if operation == "stop" {
					doOK(t, e, stop, nil)
					stopped := v.Obj(e.Snapshot()[name])
					if v.Bool(stopped["running"]) || stopped["status"] != "Поиск отменён" {
						t.Fatal("stop action did not clear running state", stopped)
					}
				}
				doOK(t, e, path, body)
				select {
				case <-cancelled:
				case <-time.After(3 * time.Second):
					t.Fatal("previous HTTP request context not cancelled")
				}
				fresh := waitEngineJob(t, e, name)
				if kind == "market" {
					rows := v.Rows(fresh["results"])
					if len(rows) != 1 || rows[0]["station"] != "Synthetic Buyer" {
						t.Fatal("new search did not complete", fresh)
					}
				} else if fresh["finishedAt"] == nil {
					t.Fatal("new mining search lacks terminal result", fresh)
				}
				releaseOnce.Do(func() { close(release) })
				select {
				case <-returned:
				case <-time.After(3 * time.Second):
					t.Fatal("late previous response not delivered")
				}
				// Updates are asynchronous; observe the settled state through several UI
				// refresh opportunities after releasing the cancelled request.
				deadline := time.Now().Add(250 * time.Millisecond)
				for time.Now().Before(deadline) {
					state := v.Obj(e.Snapshot()[name])
					if v.Bool(state["running"]) || state["status"] != fresh["status"] || v.Str(state["error"]) != "" {
						t.Fatal("old completion replaced current search status", state)
					}
					if kind == "market" {
						rows := v.Rows(state["results"])
						if len(rows) != 1 || rows[0]["station"] != "Synthetic Buyer" {
							t.Fatal("late old market result replaced new buyer", state)
						}
					} else if state["finishedAt"] != fresh["finishedAt"] {
						t.Fatal("old cancelled mining result replaced completed search", state)
					}
					time.Sleep(5 * time.Millisecond)
				}
			})
		}
	}
}
