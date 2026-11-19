package app

import (
	"time"

	"github.com/Naenier/ED-Copilot/internal/diagnostics"
	"github.com/Naenier/ED-Copilot/internal/updater"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (e *Engine) initializeUpdates() {
	if e.opts.DisableNetwork {
		return
	}
	manager, err := updater.New(updater.Options{
		CurrentVersion: Version, DataDir: e.opts.DataDir, HTTPClient: e.observedHTTP(e.opts.HTTPClient, nil), OnChange: e.Notify,
		OnCheck: func(status string, started time.Time, err error) {
			event := "check.complete"
			switch status {
			case "staged", "current", "portable", "recovery":
				event = "check." + status
			}
			e.RecordOperation("updates", event, started, nil, err)
		},
	})
	if err != nil {
		e.diagnostics.Log(diagnostics.Error, "updates", "initialize", "Не удалось запустить службу обновлений")
		e.updateError = err.Error()
		return
	}
	e.UpdateManager = manager
	manager.SetEnabled(v.Bool(e.settings["autoUpdateEnabled"]))
	e.consumeUpdateNotice()
	if !e.opts.ManualPolling {
		e.updateDone = make(chan struct{})
		go func() { defer close(e.updateDone); manager.Run(e.ctx) }()
	}
}

func (e *Engine) consumeUpdateNotice() {
	started := time.Now()
	notice, err := e.UpdateManager.ConsumeNotice()
	if err != nil {
		e.RecordOperation("updates", "notice", started, nil, err)
		return
	}
	if notice == nil {
		return
	}
	e.mu.Lock()
	e.updateNotice = v.Obj(v.Clone(notice))
	e.mu.Unlock()
	e.RecordOperation("updates", "notice", started, nil, nil)
	e.Notify()
}

func (e *Engine) ConfirmUpdateWorkspace() {
	if e.UpdateManager == nil {
		return
	}
	started := time.Now()
	if err := e.UpdateManager.MarkWorkspaceReady(); err != nil {
		e.RecordOperation("updates", "confirm-installation", started, nil, err)
		e.mu.Lock()
		e.updateError = err.Error()
		e.mu.Unlock()
		e.Notify()
		return
	}
	e.consumeUpdateNotice()
}

func (e *Engine) updateSnapshot() M {
	e.mu.RLock()
	enabled, notice, problem := v.Bool(e.settings["autoUpdateEnabled"]), v.Copy(e.updateNotice), e.updateError
	e.mu.RUnlock()
	state := M{"enabled": enabled, "status": "idle", "version": Version, "staged": false}
	if e.UpdateManager != nil {
		state = v.Obj(v.Clone(e.UpdateManager.Snapshot()))
	}
	if !enabled {
		state["status"] = "disabled"
	}
	if problem != "" {
		state["error"], state["status"] = problem, "error"
	}
	if len(notice) > 0 {
		state["notice"] = notice
	}
	return state
}

// ApplyPendingUpdate is called only after the native message loop ends and
// Engine.Close has flushed the profile. Closing a window to the tray does not
// reach this method. The detached helper waits for this process to disappear.
func (e *Engine) ApplyPendingUpdate() error {
	if e.opts.DisableNetwork || e.UpdateManager == nil {
		return nil
	}
	e.mu.RLock()
	closed, enabled := e.closed, v.Bool(e.settings["autoUpdateEnabled"])
	e.mu.RUnlock()
	if !closed || !enabled {
		return nil
	}
	started := time.Now()
	err := e.UpdateManager.ApplyOnExit(false)
	if err != nil {
		e.RecordOperation("updates", "apply", started, nil, err)
	}
	return err
}
