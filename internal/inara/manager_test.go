package inara

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testClock struct{ n atomic.Int64 }

func clockFixture() *testClock {
	c := &testClock{}
	c.n.Store(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC).UnixNano())
	return c
}
func (c *testClock) Now() time.Time          { return time.Unix(0, c.n.Load()) }
func (c *testClock) Advance(d time.Duration) { c.n.Add(int64(d)) }

type stateFixture struct {
	mu    sync.RWMutex
	state v.M
}

func newStateFixture() *stateFixture {
	return &stateFixture{state: v.M{"app": v.M{"version": "2.0.0"}, "commander": v.M{"name": "Synthetic Pilot", "fid": "FSYNTHETIC", "system": "Sol", "shipType": "python", "shipId": 7, "vehicle": "ship", "cargoVessel": "ship"}, "game": v.M{"version": "4.4.2", "live": true, "loan": 0}, "connection": v.M{"scanning": false}, "engineering": v.M{}, "pilot": v.M{}}}
}
func (s *stateFixture) Get() v.M            { s.mu.RLock(); defer s.mu.RUnlock(); return v.Copy(s.state) }
func (s *stateFixture) change(fn func(v.M)) { s.mu.Lock(); defer s.mu.Unlock(); fn(s.state) }
func cryptFixture(t *testing.T) (func([]byte) ([]byte, error), func([]byte) ([]byte, error)) {
	t.Helper()
	key := make([]byte, 32)
	rand.Read(key)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	return func(plain []byte) ([]byte, error) {
			nonce := make([]byte, gcm.NonceSize())
			rand.Read(nonce)
			return append(nonce, gcm.Seal(nil, nonce, plain, nil)...), nil
		}, func(blob []byte) ([]byte, error) {
			if len(blob) < gcm.NonceSize() {
				return nil, fmt.Errorf("short fixture")
			}
			return gcm.Open(nil, blob[:gcm.NonceSize()], blob[gcm.NonceSize():], nil)
		}
}
func dockEvent(now time.Time, station string) v.M {
	return v.M{"event": "Docked", "timestamp": v.ISO(now), "StarSystem": "Sol", "StationName": station, "MarketID": 100}
}
func ackAll(_ context.Context, o SendOptions) (BatchResult, error) {
	r := BatchResult{}
	for i := range o.Events {
		r.Results = append(r.Results, Outcome{Index: i, Status: 200, OK: true})
	}
	return r, nil
}
func TestProtectedLegacyMigrationReplayAndRestart(t *testing.T) {
	dir := t.TempDir()
	encrypt, decrypt := cryptFixture(t)
	clock, state := clockFixture(), newStateFixture()
	event := Event{"addCommanderTravelDock", v.ISO(clock.Now().Add(-time.Second)), v.M{"starsystemName": "Sol", "stationName": "Old Dock"}}
	legacy := v.M{"version": 1, "config": v.M{"publisher": "copilot", "inaraEnabled": true, "inaraApiKey": "synthetic-key-only", "boundCommander": v.M{"name": "Synthetic Pilot", "fid": "FSYNTHETIC"}}, "queue": []v.M{{"key": "inara:" + EventKey(event, "FSYNTHETIC"), "service": "inara", "identity": v.M{"name": "Synthetic Pilot", "fid": "FSYNTHETIC"}, "payload": event, "timestamp": event.Timestamp}, {"key": "unused", "service": "removed-service", "identity": v.M{"fid": "FSYNTHETIC"}}}, "delivered": []string{"inara:known"}, "cursors": v.M{"Journal.synthetic.log": 100}, "services": v.M{"inara": v.M{"sent": 7, "failed": 2, "retryAt": 0}}, "enabledAt": clock.Now().Add(-time.Hour).UnixMilli()}
	plain, _ := json.Marshal(legacy)
	blob, _ := encrypt(plain)
	oldFile := filepath.Join(dir, "integration-state.bin")
	os.WriteFile(oldFile, blob, 0600)
	var calls atomic.Int32
	options := Options{DataDir: dir, Encrypt: encrypt, Decrypt: decrypt, Now: clock.Now, GetState: state.Get, AutoDelay: time.Hour, Send: func(ctx context.Context, o SendOptions) (BatchResult, error) {
		calls.Add(1)
		if o.APIKey != "synthetic-key-only" {
			t.Error("key not preserved")
		}
		return ackAll(ctx, o)
	}}
	manager, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	manager.Status(state.Get())
	status := manager.Status()
	if !v.Bool(status["enabled"]) || !v.Bool(status["apiKeySet"]) || v.Int(status["queued"]) != 1 || v.Int(status["sent"]) != 7 {
		t.Fatal("migration state", status)
	}
	manager.ApplyJournal(dockEvent(clock.Now(), "Replay"), v.M{"file": "Journal.synthetic.log", "offset": 100, "replay": true, "state": state.Get()})
	manager.ApplyJournal(dockEvent(clock.Now(), "New Dock"), v.M{"file": "Journal.synthetic.log", "offset": 200, "state": state.Get()})
	manager.ApplyJournal(dockEvent(clock.Now(), "New Dock"), v.M{"file": "Journal.synthetic.log", "offset": 200, "state": state.Get()})
	if v.Int(manager.Status()["queued"]) != 2 {
		t.Fatal("replay/dedupe boundary", manager.Status())
	}
	manager.Close()
	unchanged, _ := os.ReadFile(oldFile)
	if !bytes.Equal(blob, unchanged) {
		t.Fatal("rollback profile mutated")
	}
	newBlob, _ := os.ReadFile(filepath.Join(dir, "integration-go-state.bin"))
	if bytes.Contains(newBlob, []byte("synthetic-key-only")) {
		t.Fatal("API key in plaintext")
	}
	public, _ := json.Marshal(status)
	if bytes.Contains(public, []byte("synthetic-key-only")) {
		t.Fatal("API key in public status")
	}
	restored, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restored.Status(state.Get())
	if err = restored.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || v.Int(restored.Status()["queued"]) != 0 || v.Int(restored.Status()["sent"]) != 9 {
		t.Fatal("persisted queue not sent", restored.Status())
	}
	restored.ApplyJournal(dockEvent(clock.Now(), "New Dock"), v.M{"file": "Journal.synthetic.log", "offset": 200, "state": state.Get()})
	if v.Int(restored.Status()["queued"]) != 0 {
		t.Fatal("cursor lost across restart")
	}
}
func TestPartialAcknowledgementsBackoffAndOptOut(t *testing.T) {
	clock, state := clockFixture(), newStateFixture()
	encrypt, decrypt := cryptFixture(t)
	var calls atomic.Int32
	manager, err := New(Options{DataDir: t.TempDir(), Encrypt: encrypt, Decrypt: decrypt, Now: clock.Now, GetState: state.Get, AutoDelay: time.Hour, Send: func(ctx context.Context, o SendOptions) (BatchResult, error) {
		if calls.Add(1) == 1 {
			return BatchResult{Results: []Outcome{{OK: true, Status: 200}, {Retryable: true, Status: 503, Message: "temporary failure"}}}, nil
		}
		return ackAll(ctx, o)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err = manager.Configure(v.M{"apiKey": "synthetic-key", "enabled": true}); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"Dock A", "Dock B"} {
		manager.ApplyJournal(dockEvent(clock.Now(), name), v.M{"file": "Journal.test", "offset": i * 100, "state": state.Get()})
	}
	if err = manager.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if v.Int(status["sent"]) != 1 || v.Int(status["queued"]) != 1 || status["nextRetryAt"] == nil {
		t.Fatal("partial ack handling", status)
	}
	manager.Sync(context.Background())
	if calls.Load() != 1 {
		t.Fatal("retry deadline ignored")
	}
	clock.Advance(2 * time.Minute)
	if err = manager.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || v.Int(manager.Status()["sent"]) != 2 || v.Int(manager.Status()["queued"]) != 0 {
		t.Fatal("retry failed", manager.Status())
	}
	if err = manager.Configure(v.M{"enabled": false}); err != nil {
		t.Fatal(err)
	}
	manager.ApplyJournal(dockEvent(clock.Now(), "Disabled Dock"), v.M{"file": "Journal.test", "offset": 500, "state": state.Get()})
	if v.Int(manager.Status()["queued"]) != 0 {
		t.Fatal("opt-out did not stop queueing")
	}
}
func TestSnapshotCoalescingIdentityLiveAndKeyRetention(t *testing.T) {
	clock, state := clockFixture(), newStateFixture()
	encrypt, decrypt := cryptFixture(t)
	var calls atomic.Int32
	manager, _ := New(Options{DataDir: t.TempDir(), Encrypt: encrypt, Decrypt: decrypt, Now: clock.Now, GetState: state.Get, AutoDelay: time.Hour, Send: func(ctx context.Context, o SendOptions) (BatchResult, error) { calls.Add(1); return ackAll(ctx, o) }})
	defer manager.Close()
	if err := manager.Configure(v.M{"apiKey": "synthetic-key", "enabled": true}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		clock.Advance(time.Second)
		manager.ApplyJournal(v.M{"event": "Cargo", "timestamp": v.ISO(clock.Now()), "Vessel": "Ship", "Inventory": []v.M{{"Name": "gold", "Count": i + 1}}}, v.M{"file": "Journal.test", "offset": i * 100, "state": state.Get()})
	}
	if v.Int(manager.Status()["queued"]) != 1 {
		t.Fatal("complete snapshots not coalesced")
	}
	state.change(func(s v.M) { v.Obj(s["commander"])["fid"] = "FOTHER" })
	manager.Status(state.Get())
	manager.Sync(context.Background())
	if calls.Load() != 0 || v.Bool(manager.Status()["identityMatches"]) {
		t.Fatal("commander gate failed")
	}
	if err := manager.Configure(v.M{"enabled": true}); err == nil {
		t.Fatal("wrong commander silently rebound")
	}
	if err := manager.Configure(v.M{"enabled": true, "rebindCommander": true}); err != nil {
		t.Fatal(err)
	}
	if v.Int(manager.Status()["queued"]) != 0 {
		t.Fatal("rebind retained other commander's queue")
	}
	state.change(func(s v.M) { v.Obj(s["game"])["isBeta"] = true })
	manager.Status(state.Get())
	manager.ApplyJournal(dockEvent(clock.Now(), "Beta Dock"), v.M{"file": "Journal.test", "offset": 400, "state": state.Get()})
	manager.Sync(context.Background())
	if calls.Load() != 0 || v.Int(manager.Status()["queued"]) != 0 {
		t.Fatal("beta export")
	}
	if err := manager.Configure(v.M{"apiKey": "", "enabled": false}); err != nil {
		t.Fatal(err)
	}
	if !v.Bool(manager.Status()["apiKeySet"]) {
		t.Fatal("blank field erased saved key")
	}
	if err := manager.Configure(v.M{"clearApiKey": true}); err != nil {
		t.Fatal(err)
	}
	if v.Bool(manager.Status()["apiKeySet"]) {
		t.Fatal("explicit clear ignored")
	}
}
func TestCancellationKeepsUnacknowledgedQueue(t *testing.T) {
	clock, state := clockFixture(), newStateFixture()
	encrypt, decrypt := cryptFixture(t)
	started := make(chan struct{})
	manager, _ := New(Options{DataDir: t.TempDir(), Encrypt: encrypt, Decrypt: decrypt, Now: clock.Now, GetState: state.Get, AutoDelay: time.Hour, Send: func(ctx context.Context, _ SendOptions) (BatchResult, error) {
		close(started)
		<-ctx.Done()
		return BatchResult{}, &Error{Message: "cancelled", Retryable: true}
	}})
	defer manager.Close()
	manager.Configure(v.M{"apiKey": "synthetic-key", "enabled": true})
	manager.ApplyJournal(dockEvent(clock.Now(), "Dock A"), v.M{"file": "Journal.test", "offset": 100, "state": state.Get()})
	done := make(chan error, 1)
	go func() { done <- manager.Sync(context.Background()) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("send did not begin")
	}
	manager.Configure(v.M{"enabled": false})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("send not cancelled")
	}
	if v.Int(manager.Status()["queued"]) != 1 || v.Int(manager.Status()["sent"]) != 0 {
		t.Fatal("cancelled event lost", manager.Status())
	}
}
func TestQueueAgeAndPermanentValidation(t *testing.T) {
	clock, state := clockFixture(), newStateFixture()
	encrypt, decrypt := cryptFixture(t)
	manager, _ := New(Options{DataDir: t.TempDir(), Encrypt: encrypt, Decrypt: decrypt, Now: clock.Now, GetState: state.Get, AutoDelay: time.Hour, Send: func(context.Context, SendOptions) (BatchResult, error) {
		return BatchResult{Results: []Outcome{{Status: 410, Message: "invalid event"}}}, nil
	}})
	defer manager.Close()
	manager.Configure(v.M{"apiKey": "synthetic-key", "enabled": true})
	manager.ApplyJournal(dockEvent(clock.Now(), "Dock A"), v.M{"file": "Journal.test", "offset": 100, "state": state.Get()})
	manager.Sync(context.Background())
	if v.Int(manager.Status()["failed"]) != 1 || v.Int(manager.Status()["queued"]) != 0 {
		t.Fatal("permanent validation infinitely retried")
	}
	clock.Advance(time.Second)
	manager.ApplyJournal(dockEvent(clock.Now(), "Dock B"), v.M{"file": "Journal.test", "offset": 200, "state": state.Get()})
	clock.Advance(31 * 24 * time.Hour)
	manager.Sync(context.Background())
	if v.Int(manager.Status()["failed"]) != 2 || v.Int(manager.Status()["queued"]) != 0 {
		t.Fatal("outdated event sent", manager.Status())
	}
}
