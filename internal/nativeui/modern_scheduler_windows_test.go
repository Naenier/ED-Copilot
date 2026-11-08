//go:build windows

package nativeui

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestModernWorkIsDeferredAndDiscardedAfterClose(t *testing.T) {
	for _, closeBeforeDrain := range []bool{false, true} {
		queue := []func(){}
		u := &Window{}
		m := &modernShell{owner: u, uiPost: func(work func()) { queue = append(queue, work) }}
		calls := 0
		m.postUI(func() { calls++ })
		if calls != 0 || len(queue) != 1 {
			t.Fatal("work ran inside the callback", calls, len(queue))
		}
		m.closed = closeBeforeDrain
		queue[0]()
		if calls != map[bool]int{false: 1, true: 0}[closeBeforeDrain] {
			t.Fatal("closed shell ran queued work", calls)
		}
	}
}

func TestModernSnapshotsCoalesceToOneLatestUIUpdate(t *testing.T) {
	queue := []func(){}
	u := &Window{modeValue: "windowed", refreshRequested: make(chan struct{}, 1)}
	m := &modernShell{owner: u, uiPost: func(work func()) { queue = append(queue, work) }}
	u.modern = m
	for revision := 0; revision < 100; revision++ {
		m.postSnapshot(M{"revision": revision, "desktop": M{"displayMode": "windowed"}})
		u.refreshModern()
	}
	if len(queue) != 1 || len(u.refreshRequested) != 1 || u.state != nil {
		t.Fatal("snapshot producer flooded the UI queue", len(queue), len(u.refreshRequested), u.state)
	}
	queue[0]()
	if u.state["revision"] != 99 || m.stateQueued || m.pendingState != nil {
		t.Fatal("latest snapshot was not applied", u.state, m.stateQueued)
	}
}

func TestModernSnapshotGateUsesEventsAndSlowIdleFallback(t *testing.T) {
	start := time.Unix(1000, 0)
	gate := modernSnapshotGate{last: start}
	for i := 1; i <= 12; i++ {
		if gate.due(start.Add(time.Duration(i) * 400 * time.Millisecond)) {
			t.Fatal("idle ticker requested a backend snapshot", i)
		}
	}
	if !gate.due(start.Add(5 * time.Second)) {
		t.Fatal("slow time-dependent refresh was lost")
	}
	gate.dirty = true
	if gate.due(start.Add(5200*time.Millisecond)) || !gate.dirty {
		t.Fatal("event burst bypassed the 400ms limit")
	}
	if !gate.due(start.Add(5400*time.Millisecond)) || gate.dirty {
		t.Fatal("dirty state did not refresh on its next eligible tick")
	}
	if gate.due(start.Add(5800 * time.Millisecond)) {
		t.Fatal("completed update left idle polling enabled")
	}
}

func TestModernHUDRefreshQueueStaysBoundedWithoutSnapshot(t *testing.T) {
	queue := []func(){}
	u := &Window{}
	m := &modernShell{owner: u, uiPost: func(work func()) { queue = append(queue, work) }}
	for i := 0; i < 100; i++ {
		m.postHUDRefresh()
	}
	if len(queue) != 1 || !m.hudQueued || u.state != nil {
		t.Fatal("cached HUD polling created a snapshot or flooded UI", len(queue), m.hudQueued)
	}
	queue[0]()
	if m.hudQueued {
		t.Fatal("cached HUD refresh did not release queue slot")
	}
}

type blockedDesktopController struct {
	platformController
	entered chan struct{}
	gate    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	calls   []M
}

func (c *blockedDesktopController) Do(path string, body M) (M, error) {
	c.once.Do(func() { close(c.entered) })
	<-c.gate
	c.mu.Lock()
	c.calls = append(c.calls, v.Copy(body))
	c.mu.Unlock()
	return M{"ok": true}, nil
}

func TestModernBoundsPersistenceDoesNotBlockUIAndCoalesces(t *testing.T) {
	backend := &blockedDesktopController{entered: make(chan struct{}), gate: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(backend.gate) })
	u := &Window{backend: backend, refreshRequested: make(chan struct{}, 1)}
	m := &modernShell{owner: u}
	u.modern = m
	m.persistDesktop(M{"maximized": false})
	select {
	case <-backend.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("desktop worker did not start")
	}
	// A profile/domain operation remains blocked while window actions update
	// the single pending payload without waiting for that operation on the STA.
	for i := 0; i < 100; i++ {
		m.persistDesktop(M{"maximized": i%2 == 0, "displayMode": "fullscreen", "last": i})
	}
	m.desktopMu.Lock()
	pending := v.Copy(m.desktopPending)
	m.desktopMu.Unlock()
	if v.Int(pending["last"]) != 99 {
		t.Fatal("latest window action was lost", pending)
	}
	release.Do(func() { close(backend.gate) })
	finished := make(chan struct{})
	go func() { m.flushDesktop(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("desktop worker did not finish")
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.calls) != 2 || v.Int(backend.calls[1]["last"]) != 99 {
		t.Fatal("desktop actions were not coalesced", backend.calls)
	}
}

func TestEncodedWebResponseIsSerializableAndRedactsSecrets(t *testing.T) {
	wire, err := encodeWebEvent("response", webResponse("7", M{"apiKey": "secret", "text": "Русский интерфейс", "rows": []M{{"refreshToken": "hidden", "value": 1}}}, nil))
	if err != nil {
		t.Fatal(err)
	}
	var decoded M
	err = json.Unmarshal([]byte(wire), &decoded)
	if err != nil || v.At(decoded, "data", "id") != "7" || v.At(decoded, "data", "result", "text") != "Русский интерфейс" {
		t.Fatal(wire, err)
	}
	if strings.Contains(wire, "secret") || strings.Contains(wire, "hidden") {
		t.Fatal("secret was serialized", wire)
	}
}
