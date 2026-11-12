package app

import (
	"context"
	"errors"
	"time"

	"github.com/Naenier/ED-Copilot/internal/colonization"
)

const colonyHistoryCadence = 3 * time.Hour

// A single worker polls only known colony systems. Network work runs outside
// the engine and planner locks, and provider failures never interrupt journal
// ingestion or replace dated observations.
func (e *Engine) startColonyHistory() {
	if e.opts.DisableNetwork || e.opts.ManualPolling {
		return
	}
	go func() {
		tick := time.NewTicker(20 * time.Second)
		defer tick.Stop()
		next := map[colonization.HistoryTarget]time.Time{}
		var owner string
		var pause time.Time
		for {
			select {
			case <-e.ctx.Done():
				return
			case now := <-tick.C:
				if now.Before(pause) {
					continue
				}
				e.mu.RLock()
				workspace := e.planWorkspace
				e.mu.RUnlock()
				if workspace.key == "" || workspace.branch != "live" {
					continue
				}
				if owner != workspace.key {
					owner, next = workspace.key, map[colonization.HistoryTarget]time.Time{}
				}
				targets := e.Colonies.HistoryTargets()
				target, found := nextColonyObservation(targets, next, now)
				if !found {
					continue
				}
				next[target] = now.Add(colonyHistoryCadence)
				if retryAt := e.observeColonyPublicSystem(workspace.key, target); retryAt.After(now) {
					pause = retryAt
					if ceiling := now.Add(time.Hour); pause.After(ceiling) {
						pause = ceiling
					}
					next[target] = pause
				}
			}
		}
	}()
}

func nextColonyObservation(targets []colonization.HistoryTarget, next map[colonization.HistoryTarget]time.Time, now time.Time) (colonization.HistoryTarget, bool) {
	active := map[colonization.HistoryTarget]bool{}
	var selected colonization.HistoryTarget
	found := false
	for _, target := range targets {
		active[target] = true
		at := next[target]
		if !at.After(now) && (!found || at.Before(next[selected])) {
			selected, found = target, true
		}
	}
	for target := range next {
		if !active[target] {
			delete(next, target)
		}
	}
	return selected, found
}

func (e *Engine) observeColonyPublicSystem(owner string, target colonization.HistoryTarget) time.Time {
	ctx, cancel := context.WithTimeout(e.ctx, 20*time.Second)
	defer cancel()
	observation, err := e.Politics.PublicSystem(ctx, target.Name, target.Address)
	if err != nil {
		var retry interface{ RetryAt() time.Time }
		if errors.As(err, &retry) {
			return retry.RetryAt()
		}
		return time.Time{}
	}
	e.domainMu.Lock()
	e.mu.RLock()
	current, closed := e.planWorkspace, e.closed
	e.mu.RUnlock()
	added := !closed && ctx.Err() == nil && current.key == owner && current.branch == "live" && e.Colonies.ObservePublicSystem(target, observation)
	if added {
		e.mu.Lock()
		e.revision++
		e.mu.Unlock()
	}
	e.domainMu.Unlock()
	if added {
		e.Notify()
		_ = e.Save()
	}
	return time.Time{}
}
