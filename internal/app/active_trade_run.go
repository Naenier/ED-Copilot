package app

import (
	"errors"
	"time"

	"github.com/Naenier/ED-Copilot/internal/profile"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// updateActiveTradeRun resolves the selected quote from the displayed search
// or the commander's favorites. The renderer never submits a route payload or
// arbitrarily chooses the text placed on the clipboard.
func (e *Engine) updateActiveTradeRun(input M) (M, error) {
	e.saveMu.Lock()
	defer e.saveMu.Unlock()
	e.domainMu.Lock()
	defer e.domainMu.Unlock()

	e.mu.RLock()
	owner := e.planWorkspace.key
	commander := v.Copy(e.commander)
	closed := e.closed
	var result M
	id := v.Str(input["routeId"])
	for _, row := range v.Rows(e.network["tradeRoutes"]["results"]) {
		if row["id"] == id {
			result = v.Copy(row)
			break
		}
	}
	e.mu.RUnlock()
	if closed {
		return nil, errors.New("Приложение завершает работу.")
	}
	if err := validatePlanWorkspace(input, owner); err != nil {
		return nil, err
	}
	if owner == "" {
		return nil, errors.New("Сначала подключите журнал командира.")
	}
	previous := e.tradeRuns.Export()
	var clipboard string
	switch v.Str(input["action"]) {
	case "start", "select":
		if id == "" {
			return nil, errors.New("Выбранный рейс недоступен.")
		}
		if result == nil {
			for _, row := range e.Trading.SavedRoutesFor(owner) {
				if row["id"] == id {
					result = row
					break
				}
			}
		}
		next, err := e.tradeRuns.Select(owner, result, commander, time.Now())
		if err != nil {
			return nil, err
		}
		clipboard = next
	case "stop", "clear":
		e.tradeRuns.Clear(owner)
	default:
		return nil, errors.New("Неизвестное действие торгового рейса.")
	}

	saved, _ := e.persistenceSnapshotLocked()
	if err := profile.SaveSettings(e.opts.DataDir, saved); err != nil {
		e.tradeRuns = trading.NewLiveRoutes(previous)
		return nil, err
	}
	e.mu.Lock()
	e.clearClipboardLocked("trade-route")
	if clipboard != "" {
		e.queueClipboardLocked("trade-route", clipboard)
	}
	e.revision++
	e.mu.Unlock()
	e.Notify()
	return M{"ok": true, "tradeRun": e.tradeRuns.Snapshot(owner)}, nil
}
