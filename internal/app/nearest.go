package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/localization"
	"github.com/Naenier/ED-Copilot/internal/nearest"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (e *Engine) SearchNearest(input M) (M, error) {
	sequence, err := systemSequence(input)
	if err != nil {
		return nil, err
	}
	e.mu.RLock()
	origin := v.Str(e.commander["system"])
	position := v.Arr(e.commander["coordinates"])
	e.mu.RUnlock()
	q, err := nearest.FromInput(input, origin)
	if err != nil {
		return nil, err
	}
	if e.opts.DisableNetwork {
		return M{"rows": []M{}, "status": "disabled", "link": nearest.InaraLink(q)}, nil
	}
	const key = "nearest"
	e.mu.Lock()
	if sequence <= e.jobIDs[key] {
		e.mu.Unlock()
		return M{"rows": []M{}, "status": "cancelled"}, nil
	}
	if cancel := e.cancelJobs[key]; cancel != nil {
		cancel()
	}
	ctx, cancel := context.WithTimeout(e.ctx, 32*time.Second)
	e.jobIDs[key], e.cancelJobs[key] = sequence, cancel
	e.mu.Unlock()
	defer func() {
		cancel()
		e.mu.Lock()
		if e.jobIDs[key] == sequence {
			delete(e.cancelJobs, key)
		}
		e.mu.Unlock()
	}()
	client := nearest.New(e.observedHTTP(e.opts.HTTPClient, nil))
	client.EDSM = e.EDSM
	if q.Origin != "" && strings.EqualFold(q.Origin, origin) && len(position) == 3 {
		for _, component := range position {
			n, ok := v.Number(component)
			if !ok {
				q.OriginCoords = nil
				break
			}
			q.OriginCoords = append(q.OriginCoords, n)
		}
	}
	if e.Politics != nil && (q.Category == "misc" && q.Filters["kind"] == "conflicts" ||
		q.Category == "outfitting" && q.Filters["stronghold"] == "friendly") {
		politics := e.Politics.Snapshot()
		if q.Category == "misc" {
			q.Politics = v.Rows(politics["systems"])
		} else {
			q.Filters["pledgedPower"] = v.Str(v.At(politics, "pledge", "power"))
		}
	}
	result, err := client.Search(ctx, q)
	if errors.Is(ctx.Err(), context.Canceled) {
		return M{"rows": []M{}, "status": "cancelled"}, nil
	}
	if err != nil {
		status := "unavailable"
		if strings.Contains(err.Error(), "HTTP 400") {
			status = "invalid"
		}
		return M{"rows": []M{}, "status": status, "link": nearest.InaraLink(q)}, nil
	}
	result.Note = localization.Text(e.Language(), result.Note)
	out := v.Obj(v.Clone(result))
	out["status"] = "ok"
	return out, nil
}

func (e *Engine) CancelNearest(input M) (M, error) {
	sequence, err := systemSequence(input)
	if err != nil {
		return nil, err
	}
	const key = "nearest"
	e.mu.Lock()
	defer e.mu.Unlock()
	if sequence >= e.jobIDs[key] {
		e.jobIDs[key] = sequence
		if cancel := e.cancelJobs[key]; cancel != nil {
			cancel()
			delete(e.cancelJobs, key)
		}
	}
	return M{"ok": true}, nil
}
