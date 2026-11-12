package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/Naenier/ED-Copilot/internal/journal"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type planWorkspace struct {
	key, name, branch string
}

func (w planWorkspace) snapshot() M {
	return M{"key": w.key, "name": w.name, "branch": w.branch}
}

func (e *Engine) observePlanWorkspaceLocked(kind string, event, meta M) {
	if v.Bool(meta["side"]) {
		return
	}
	switch kind {
	case "Fileheader":
		session := journal.SessionKey(v.Str(meta["file"]))
		branch := journal.GameMode(v.Str(e.game["version"]))
		if v.Int(event["part"]) > 1 && session != "" && session == e.planSession && branch != "" {
			if e.planWorkspace.branch != branch {
				e.planWorkspace = planWorkspace{}
			}
			if e.replayedPlanWorkspace.branch != branch {
				e.replayedPlanWorkspace = planWorkspace{}
			}
			return
		}
		e.planSession = session
		e.planWorkspace, e.replayedPlanWorkspace = planWorkspace{}, planWorkspace{}
	case "Commander", "LoadGame":
		version := v.Text(e.game["version"], event["gameversion"])
		fid, branch := v.Str(event["FID"]), journal.GameMode(version)
		if fid == "" || branch == "" {
			return
		}
		digest := sha256.Sum256([]byte(branch + "\x00" + fid))
		key := hex.EncodeToString(digest[:16])
		name := v.Text(event["Name"], event["Commander"])
		current := &e.planWorkspace
		if v.Bool(meta["replay"]) {
			current = &e.replayedPlanWorkspace
		}
		if name == "" && current.key == key {
			name = current.name
		}
		*current = planWorkspace{key: key, name: name, branch: branch}
	}
}

func (e *Engine) finishPlanReplay() {
	if e.Reader == nil || v.Str(e.Reader.State()["error"]) != "" {
		return
	}
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.planReplay {
		e.planWorkspace = e.replayedPlanWorkspace
		e.adoptRichesJourneyLocked()
		e.replayedPlanWorkspace = planWorkspace{}
		e.planReplay = false
		e.section7.notifications.Context(e.planWorkspace.key, v.Str(e.settings["activity"]), v.Str(e.settings["language"]))
		e.rebindSecondScreenLocked()
		e.Notify()
	}
}

func validatePlanWorkspace(body M, current string) error {
	if expected, present := body["expectedPlanWorkspace"]; present {
		if key, ok := expected.(string); !ok || key != current {
			return errors.New("Командир сменился. Откройте действие заново.")
		}
	}
	return nil
}

func goalOwner(body M, active, fallback string) (string, error) {
	raw, present := body["scope"]
	if !present {
		return fallback, nil
	}
	switch raw {
	case "shared":
		return "", nil
	case "personal":
		if active != "" {
			return active, nil
		}
		return "", errors.New("Для личного плана нужен командир из журнала игры.")
	default:
		return "", errors.New("Выберите область плана.")
	}
}

func goalsForWorkspace(goals []M, owner planWorkspace) []M {
	visible := make([]M, 0, len(goals))
	for _, goal := range goals {
		key := v.Str(goal["planOwner"])
		if key == "" || key == owner.key {
			visible = append(visible, goal)
		}
	}
	out := v.Rows(v.Clone(visible))
	for _, goal := range out {
		key := v.Str(goal["planOwner"])
		delete(goal, "planOwner")
		goal["scope"] = "shared"
		if key != "" {
			goal["scope"] = "personal"
			goal["commanderName"] = owner.name
		}
	}
	return out
}

func (e *Engine) saveWorkspaceRoute(body M) (M, error) {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.RLock()
	owner := e.planWorkspace.key
	e.mu.RUnlock()
	if err := validatePlanWorkspace(body, owner); err != nil {
		return nil, err
	}
	return e.Trading.SaveRouteFor(body, owner)
}
