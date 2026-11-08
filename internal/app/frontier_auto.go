package app

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/Naenier/ED-Copilot/internal/frontier"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const frontierRequestTimeout = 90 * time.Second

// One daemon owns the refresh schedule. UI and journal callbacks only cancel
// or wake it; neither performs network or profile writes on the caller thread.
type frontierAutomatic struct {
	mu     sync.Mutex
	wake   chan struct{}
	cancel context.CancelFunc
	done   chan struct{}
}

func newFrontierAutomatic() *frontierAutomatic {
	return &frontierAutomatic{wake: make(chan struct{}, 1)}
}

func (e *Engine) wakeFrontier() {
	if e.frontierAuto == nil {
		return
	}
	select {
	case e.frontierAuto.wake <- struct{}{}:
	default:
	}
}

func (e *Engine) cancelFrontierRequest() {
	if e.frontierAuto == nil {
		return
	}
	e.frontierAuto.mu.Lock()
	if e.frontierAuto.cancel != nil {
		e.frontierAuto.cancel()
	}
	e.frontierAuto.mu.Unlock()
	e.wakeFrontier()
}

func (e *Engine) startAutomaticFrontier() {
	if e.opts.DisableNetwork || e.opts.ManualPolling || e.Frontier == nil {
		return
	}
	e.frontierAuto.done = make(chan struct{})
	go func() { defer close(e.frontierAuto.done); e.runAutomaticFrontier(e.ctx) }()
}

func frontierRetry(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 5 {
		attempt = 5
	}
	delay := time.Minute * time.Duration(1<<(attempt-1))
	if delay > 15*time.Minute {
		return 15 * time.Minute
	}
	return delay
}

// Provider cooldowns count attempts, including failures. Backoff can postpone
// a request further, but never makes it earlier than the CAPI rate limit.
func frontierDelay(now time.Time, status M, retryAt time.Time) time.Duration {
	if !v.Bool(status["connected"]) || v.Bool(status["pending"]) || v.Bool(status["commanderMismatch"]) {
		return 30 * time.Second
	}
	next := now
	profile, carrier := v.Num(status["nextSyncAt"]), v.Num(status["nextCarrierSyncAt"])
	if profile > 0 && carrier > 0 {
		next = time.UnixMilli(int64(profile))
		c := time.UnixMilli(int64(carrier))
		if c.Before(next) {
			next = c
		}
	}
	if retryAt.After(next) {
		next = retryAt
	}
	if next.After(now) {
		return next.Sub(now)
	}
	return 0
}

func (e *Engine) runAutomaticFrontier(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	var retryAt time.Time
	failures, identity := 0, ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.frontierAuto.wake:
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		cmd := e.Commander()
		status := e.Frontier.Status(cmd)
		key := v.Str(cmd["fid"]) + "|" + v.Str(status["fid"]) + "|" + v.Str(status["connected"]) + "|" + v.Str(status["pending"]) + "|" + strconv.FormatUint(e.Frontier.SessionRevision(), 10)
		if identity != key {
			identity, failures, retryAt = key, 0, time.Time{}
		}
		delay := frontierDelay(time.Now(), status, retryAt)
		if e.Reader != nil && v.Bool(e.Reader.State()["scanning"]) {
			delay = time.Second
		}
		if delay > 0 {
			resetFrontierTimer(timer, delay)
			continue
		}
		request, cancel := context.WithTimeout(ctx, frontierRequestTimeout)
		e.frontierAuto.mu.Lock()
		e.frontierAuto.cancel = cancel
		e.frontierAuto.mu.Unlock()
		result, err := e.syncFrontier(request, true)
		e.frontierAuto.mu.Lock()
		e.frontierAuto.cancel = nil
		e.frontierAuto.mu.Unlock()
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err != nil || len(v.Obj(result["errors"])) > 0 {
			failures++
			retryAt = time.Now().Add(frontierRetry(failures))
		} else {
			failures, retryAt = 0, time.Time{}
		}
		// Account changes wake the loop immediately; ordinary completion goes
		// back through provider cooldowns rather than polling the network.
		resetFrontierTimer(timer, frontierDelay(time.Now(), e.Frontier.Status(e.Commander()), retryAt))
	}
}

func resetFrontierTimer(timer *time.Timer, delay time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	if delay <= 0 {
		delay = time.Millisecond
	}
	timer.Reset(delay)
}

func (e *Engine) syncFrontier(ctx context.Context, carrier bool) (M, error) {
	if err := e.networkAllowed(); err != nil {
		return nil, err
	}
	if e.Frontier == nil {
		return nil, errors.New("Вход Frontier недоступен")
	}
	result, err := e.Frontier.Sync(ctx, carrier)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !e.consumeFrontierResult(ctx, result) {
		return nil, context.Canceled
	}
	if e.Inara != nil && ctx.Err() == nil {
		e.Inara.ObserveCAPI(result)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(v.Obj(result["data"])) > 0 {
		if err := e.Save(); err != nil {
			return nil, err
		}
	}
	e.Notify()
	return result, nil
}

func (e *Engine) consumeFrontierResult(ctx context.Context, result M) bool {
	market := frontier.MarketSnapshot(result)
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	if ctx.Err() != nil {
		return false
	}
	cmd, status := e.Commander(), v.Obj(result["status"])
	if !v.Bool(status["connected"]) || v.Bool(status["commanderMismatch"]) || v.Str(cmd["fid"]) != "" && v.Str(status["fid"]) != v.Str(cmd["fid"]) {
		return false
	}
	e.applyVerifiedProfileCAPILocked(result)
	e.Carrier.ApplyCAPI(result, cmd)
	if market != nil && v.Bool(cmd["docked"]) && v.Str(cmd["marketId"]) == v.Str(market["MarketID"]) {
		e.applyLocked(market, M{"side": true})
	} else if len(v.Obj(result["data"])) > 0 {
		e.mu.Lock()
		e.revision++
		e.mu.Unlock()
	}
	return true
}
