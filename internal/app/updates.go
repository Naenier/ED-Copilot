package app

import (
	"github.com/Naenier/ED-Copilot/internal/updater"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (e *Engine) initializeUpdates() {
	if e.opts.DisableNetwork {
		return
	}
	manager, err := updater.New(updater.Options{CurrentVersion: Version, DataDir: e.opts.DataDir, HTTPClient: e.opts.HTTPClient, OnChange: e.Notify})
	if err != nil {
		e.updateError = err.Error()
		return
	}
	e.UpdateManager = manager
	manager.SetEnabled(v.Bool(e.settings["autoUpdateEnabled"]))
	if notice, err := manager.ConsumeNotice(); err == nil && notice != nil {
		e.updateNotice = v.Obj(v.Clone(notice))
	}
	if !e.opts.ManualPolling {
		e.updateDone = make(chan struct{})
		go func() { defer close(e.updateDone); manager.Run(e.ctx) }()
	}
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
	return e.UpdateManager.ApplyOnExit(false)
}
