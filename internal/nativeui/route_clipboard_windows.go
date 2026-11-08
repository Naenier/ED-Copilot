//go:build windows

package nativeui

import (
	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/walk"
)

func attachRouteClipboard(state M, backend Controller) M {
	source, ok := backend.(interface{ PendingClipboard() M })
	if !ok {
		return state
	}
	command := source.PendingClipboard()
	if len(command) == 0 {
		return state
	}
	out := make(M, len(state)+1)
	for key, value := range state {
		out[key] = value
	}
	out["nativeClipboard"] = command
	return out
}

// Only a successful clipboard write is acknowledged. A temporarily busy
// clipboard is retried by the bounded background snapshot, including while
// the main window is hidden and the player remains in the game.
func deliverRouteClipboard(lastID string, command M, write func(string) error, acknowledge func(string)) (string, error) {
	id, text := v.Str(command["id"]), v.Str(command["text"])
	if id == "" || text == "" || id == lastID {
		return lastID, nil
	}
	if err := write(text); err != nil {
		return lastID, err
	}
	acknowledge(id)
	return id, nil
}

func (u *Window) syncRouteClipboard(state M) {
	ack, ok := u.backend.(interface{ AcknowledgeClipboard(string) })
	if !ok {
		return
	}
	write := u.routeClipboardWriter
	if write == nil {
		write = walk.Clipboard().SetText
	}
	last, _ := deliverRouteClipboard(u.lastRouteClipboardID, v.Obj(state["nativeClipboard"]), write, ack.AcknowledgeClipboard)
	u.lastRouteClipboardID = last
}
