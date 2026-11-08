package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/frontier"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestAutomaticFrontierScheduleHonorsBothProviderCooldownsAndBackoff(t *testing.T) {
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	status := M{"connected": true}
	if frontierDelay(now, status, time.Time{}) != 0 {
		t.Fatal("saved login should refresh immediately")
	}
	status["nextSyncAt"], status["nextCarrierSyncAt"] = now.Add(time.Minute).UnixMilli(), now.Add(15*time.Minute).UnixMilli()
	if frontierDelay(now, status, time.Time{}) != time.Minute {
		t.Fatal("profile cadence changed")
	}
	status["nextCarrierSyncAt"] = now.Add(-time.Second).UnixMilli()
	if frontierDelay(now, status, time.Time{}) != 0 {
		t.Fatal("due carrier starved behind profile")
	}
	if frontierDelay(now, status, now.Add(4*time.Minute)) != 4*time.Minute {
		t.Fatal("failure backoff ignored")
	}
	for _, flag := range []string{"pending", "commanderMismatch"} {
		status[flag] = true
		if frontierDelay(now, status, time.Time{}) <= 0 {
			t.Fatal("blocked account would spin")
		}
		delete(status, flag)
	}
	status["connected"] = false
	if frontierDelay(now, status, time.Time{}) <= 0 {
		t.Fatal("logged out account would spin")
	}
	for i, want := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute} {
		if frontierRetry(i+1) != want {
			t.Fatal("unbounded/missing backoff", i)
		}
	}
}

func syntheticFrontierLogin(t *testing.T, e *Engine) {
	t.Helper()
	result, err := e.Do("/api/frontier/login", nil)
	if err != nil {
		t.Fatal(err)
	}
	login, _ := url.Parse(v.Str(result["url"]))
	if callbackRequest(t, e, url.Values{"state": {login.Query().Get("state")}, "code": {"synthetic-code"}}, "", "/api/frontier/callback", "GET") != 200 {
		t.Fatal("login callback failed")
	}
}

func waitFrontierCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("automatic Frontier did not settle")
		case <-tick.C:
		}
	}
}

func TestAutomaticFrontierStartsFromSavedLoginWithoutManualRefresh(t *testing.T) {
	e, transport, _ := accountFixture(t)
	syntheticFrontierLogin(t, e)
	if e.frontierAuto.done != nil {
		t.Fatal("manual-polling fixture unexpectedly started network daemon")
	}
	e.opts.ManualPolling = false
	e.startAutomaticFrontier()
	waitFrontierCondition(t, func() bool {
		return v.Num(v.At(e.SnapshotForUI("commander"), "commanderProfile", "finance", "credits")) == 987654
	})
	transport.mu.Lock()
	calls := transport.profileCalls
	transport.mu.Unlock()
	if calls != 1 {
		t.Fatal("startup did not perform a single profile request", calls)
	}
	if v.At(e.Frontier.Status(), "nextCarrierSyncAt") == nil {
		t.Fatal("carrier refresh omitted")
	}
	// Re-entering the retained internal API during the cooldown does no HTTP,
	// and a daemon wake cannot turn the provider cooldown into busy network work.
	if _, err := e.syncFrontier(e.ctx, true); err != nil {
		t.Fatal(err)
	}
	e.wakeFrontier()
	transport.mu.Lock()
	calls = transport.profileCalls
	transport.mu.Unlock()
	if calls != 1 {
		t.Fatal("refresh duplicated inside provider cooldown")
	}
}

func TestAutomaticFrontierSuccessfulLoginWakesAnIdleDaemon(t *testing.T) {
	e, _, _ := accountFixture(t)
	e.opts.ManualPolling = false
	e.startAutomaticFrontier()
	syntheticFrontierLogin(t, e)
	waitFrontierCondition(t, func() bool {
		return v.Num(v.At(e.SnapshotForUI("commander"), "commanderProfile", "finance", "credits")) == 987654
	})
}

func TestAutomaticFrontierRapidSameCommanderReauthenticationDropsOldBackoff(t *testing.T) {
	e, transport, _ := accountFixture(t)
	syntheticFrontierLogin(t, e)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	var calls atomic.Int32
	httpClient := &http.Client{Transport: automaticFrontierTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() == "companion.orerve.net" && r.URL.Path == "/profile" && calls.Add(1) == 1 {
			close(entered)
			<-r.Context().Done()
			<-release
			return nil, r.Context().Err()
		}
		return transport.RoundTrip(r)
	})}
	client, err := frontier.New(frontier.Options{DataDir: e.opts.DataDir, HTTPClient: httpClient, GetCommander: e.Commander, OnUpdate: e.Notify})
	if err != nil {
		t.Fatal(err)
	}
	e.Frontier = client
	e.opts.ManualPolling = false
	e.startAutomaticFrontier()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first refresh did not start")
	}
	before := client.SessionRevision()
	if _, err = e.Do("/api/frontier/logout", nil); err != nil {
		unblock()
		t.Fatal(err)
	}
	syntheticFrontierLogin(t, e)
	if client.SessionRevision() == before {
		unblock()
		t.Fatal("same-FID authentication did not change session")
	}
	unblock()
	waitFrontierCondition(t, func() bool {
		return v.Num(v.At(e.SnapshotForUI("commander"), "commanderProfile", "finance", "credits")) == 987654
	})
	if calls.Load() != 2 {
		t.Fatal("fresh session did not immediately refresh once", calls.Load())
	}
	if _, err = e.syncFrontier(e.ctx, true); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("reauthentication bypassed the new session cooldown")
	}
}

type automaticFrontierTransport func(*http.Request) (*http.Response, error)

func (f automaticFrontierTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAutomaticFrontierRequestsDoNotBlockViewsAndCancelOnOwnerLogoutOrExit(t *testing.T) {
	for _, kind := range []string{"FID", "logout", "exit"} {
		t.Run(kind, func(t *testing.T) {
			e, transport, _ := accountFixture(t)
			syntheticFrontierLogin(t, e)
			entered, canceled := make(chan struct{}), make(chan struct{})
			httpClient := &http.Client{Transport: automaticFrontierTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Hostname() == "companion.orerve.net" && r.URL.Path == "/profile" {
					close(entered)
					<-r.Context().Done()
					close(canceled)
					return nil, r.Context().Err()
				}
				return transport.RoundTrip(r)
			})}
			client, err := frontier.New(frontier.Options{DataDir: e.opts.DataDir, HTTPClient: httpClient, GetCommander: e.Commander, OnUpdate: e.Notify})
			if err != nil {
				t.Fatal(err)
			}
			e.Frontier = client
			e.opts.ManualPolling = false
			e.startAutomaticFrontier()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("background request did not start")
			}
			view := make(chan struct{})
			go func() { _ = e.SnapshotForUI("overview"); close(view) }()
			select {
			case <-view:
			case <-time.After(3 * time.Second):
				t.Fatal("network held the UI/domain locks")
			}
			switch kind {
			case "FID":
				e.Apply(M{"event": "LoadGame", "timestamp": v.ISO(time.Now()), "FID": "F999", "Commander": "Other Pilot", "Ship": "python", "ShipID": 4}, M{"replay": true})
			case "logout":
				if _, err = e.Do("/api/frontier/logout", nil); err != nil {
					t.Fatal(err)
				}
			case "exit":
				e.Close()
			}
			select {
			case <-canceled:
			case <-time.After(3 * time.Second):
				t.Fatal("request survived account/shutdown boundary")
			}
			if v.Num(v.At(e.SnapshotForUI("commander"), "commanderProfile", "finance", "credits")) == 987654 {
				t.Fatal("canceled account response applied")
			}
		})
	}
}

func TestAutomaticFrontierCanceledResultCannotUpdateTheSameCommander(t *testing.T) {
	e, _, _ := accountFixture(t)
	ctx, cancel := context.WithCancel(e.ctx)
	cancel()
	r := M{"status": M{"connected": true, "fid": "F123"}, "data": M{"profile": M{"commander": M{"name": "Synthetic Pilot", "credits": 987654}}}}
	if e.consumeFrontierResult(ctx, r) {
		t.Fatal("canceled response accepted")
	}
	if v.Num(v.At(e.SnapshotForUI("commander"), "commanderProfile", "finance", "credits")) == 987654 {
		t.Fatal("canceled response changed finance")
	}
}

func TestAutomaticFrontierDisabledInOfflineAndManualProfiles(t *testing.T) {
	e, _, _ := accountFixture(t)
	e.startAutomaticFrontier()
	if e.frontierAuto.done != nil {
		t.Fatal("manual fixture started daemon")
	}
	e.opts.ManualPolling = false
	e.opts.DisableNetwork = true
	e.startAutomaticFrontier()
	if e.frontierAuto.done != nil {
		t.Fatal("offline fixture started daemon")
	}
}

func TestAutomaticFrontierStockReachesHUDRevisionWithoutJournalRescan(t *testing.T) {
	e, transport, _ := accountFixture(t)
	syntheticFrontierLogin(t, e)
	e.Apply(M{"event": "CarrierStats", "timestamp": v.ISO(time.Now().Add(-time.Second)), "CarrierID": 42, "Callsign": "SYN-001", "Name": "Synthetic"}, M{"replay": true})
	if _, err := e.Do("/api/settings", M{"activity": "colonization"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Do("/api/colonization", M{"action": "configure", "includeCarrierWarehouse": true}); err != nil {
		t.Fatal(err)
	}
	raw := M{"market": M{"id": 42}, "name": M{"callsign": "SYN-001"}, "cargo": []M{{"commodity": "gold", "qty": 5}, {"commodity": "gold", "qty": 2, "stolen": true}, {"commodity": "tritium", "qty": 10, "mission": true}}}
	httpClient := &http.Client{Transport: automaticFrontierTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() == "companion.orerve.net" && r.URL.Path == "/fleetcarrier" {
			b, _ := json.Marshal(raw)
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(b)), Request: r}, nil
		}
		return transport.RoundTrip(r)
	})}
	client, err := frontier.New(frontier.Options{DataDir: e.opts.DataDir, HTTPClient: httpClient, GetCommander: e.Commander, OnUpdate: e.Notify})
	if err != nil {
		t.Fatal(err)
	}
	e.Frontier = client
	before, events := v.Num(e.SnapshotForHUD()["revision"]), v.Num(e.Reader.State()["eventCount"])
	for len(e.updates) > 0 {
		<-e.updates
	}
	e.opts.ManualPolling = false
	e.startAutomaticFrontier()
	var hud M
	waitFrontierCondition(t, func() bool {
		hud = e.SnapshotForHUD()
		for _, row := range v.Rows(v.At(hud, "colonization", "carrierWarehouses")) {
			if row["kind"] == "fleet" && v.Bool(row["known"]) {
				return true
			}
		}
		return false
	})
	if v.Num(hud["revision"]) <= before {
		t.Fatal("HUD refresh revision did not change")
	}
	if v.Num(v.At(hud, "carrier", "warehouse", "knownTons")) != 17 {
		t.Fatal("personal physical hold not auto-imported", v.At(hud, "carrier", "warehouse"))
	}
	warehouses := v.Rows(v.At(hud, "colonization", "carrierWarehouses"))
	if len(warehouses) != 2 || v.Num(warehouses[0]["knownTons"]) != 5 {
		t.Fatal("procurement mixed stolen/mission cargo into legal supply", warehouses)
	}
	if v.Num(e.Reader.State()["eventCount"]) != events || v.Bool(e.Reader.State()["scanning"]) {
		t.Fatal("stock refresh replayed journals")
	}
	select {
	case <-e.Updates():
	case <-time.After(time.Second):
		t.Fatal("UI was not notified")
	}
}
