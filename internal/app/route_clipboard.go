package app

import (
	"strconv"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// Clipboard commands are volatile: replaying journals or restarting the app
// must never replace the user's clipboard with an old destination.
func (e *Engine) queueNextRouteClipboard() {
	next := e.Route.ClipboardProgress()
	e.mu.Lock()
	defer e.mu.Unlock()
	if enabled, exists := e.settings["routeAutoCopyNext"]; exists && !v.Bool(enabled) {
		e.clearClipboardLocked("carrier-route")
		return
	}
	if v.Int(next["completedJumps"]) == 0 {
		return
	}
	if v.Str(next["system"]) == "" {
		e.clearClipboardLocked("carrier-route")
		return
	}
	e.queueClipboardLocked("carrier-route", v.Str(next["system"]))
}

func (e *Engine) queueClipboardLocked(kind, text string) {
	e.clipboardSequence++
	e.clipboardCommand = M{"id": strconv.FormatUint(e.clipboardSequence, 10), "text": text, "kind": kind}
}

func (e *Engine) clearClipboardLocked(kind string) {
	if e.clipboardCommand["kind"] == kind {
		e.clipboardCommand = nil
	}
}

func (e *Engine) PendingClipboard() M {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.clipboardCommand == nil {
		return nil
	}
	return v.Copy(e.clipboardCommand)
}

func (e *Engine) AcknowledgeClipboard(id string) {
	e.mu.Lock()
	if v.Str(e.clipboardCommand["id"]) == id {
		e.clipboardCommand = nil
	}
	e.mu.Unlock()
}

func (e *Engine) clearRouteClipboard() {
	e.mu.Lock()
	e.clearClipboardLocked("carrier-route")
	e.mu.Unlock()
}
