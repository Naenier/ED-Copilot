package app

import (
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/Naenier/ED-Copilot/internal/shipfits"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (e *Engine) shipFitsObserved(body M) (M, error) {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.RLock()
	owner := e.planWorkspace.key
	e.mu.RUnlock()
	if err := validatePlanWorkspace(body, owner); err != nil {
		return nil, err
	}
	if owner != "" && owner == e.section7.observedOwner && len(e.section7.loadout) > 0 {
		return v.Copy(e.section7.loadout), nil
	}
	return nil, nil
}

func (e *Engine) shipFitMaterials(body M) (M, error) {
	parts := v.Rows(body["modules"])
	if len(parts) != len(v.Arr(body["modules"])) || len(parts) > 120 {
		return nil, errors.New("Некорректный список модулей.")
	}
	seen := map[string]bool{}
	for _, part := range parts {
		slot, blueprint, experimental := v.Str(part["slot"]), v.Str(part["blueprint"]), v.Str(part["experimental"])
		grade, valid := v.Number(part["grade"])
		if slot == "" || len(slot) > 120 || seen[slot] || len(blueprint) > 120 || len(experimental) > 120 ||
			blueprint != "" && !v.Bool(part["engineeringLocked"]) && (!valid || grade < 1 || grade > 5 || grade != float64(int(grade))) {
			return nil, errors.New("Некорректный список модулей.")
		}
		seen[slot] = true
	}
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	if err := validatePlanWorkspace(body, e.planWorkspace.key); err != nil {
		return nil, err
	}
	return e.Engineering.ShipFitMaterials(parts, !e.journalObservationPending(e.windowConnection(nil))), nil
}

func shipFitRecord(body M) (shipfits.Record, error) {
	var fit shipfits.Record
	build := v.Obj(body["build"])
	payload, err := json.Marshal(build)
	if err != nil || len(build) == 0 || len(payload) > 64<<10 {
		return fit, errors.New("Некорректные данные сборки.")
	}
	return shipfits.Record{ID: v.Str(body["id"]), Name: v.Str(body["name"]), Ship: v.Str(body["ship"]), Build: payload}, nil
}

func (e *Engine) shipFitsCommand(path string, body M) (M, error) {
	store := shipfits.New(filepath.Join(e.opts.DataDir, "ship-fits.db"))
	switch path {
	case "/api/ship-fits":
		observed, err := e.shipFitsObserved(body)
		if err != nil {
			return nil, err
		}
		fits, err := store.List(false)
		return M{"fits": fits, "observed": observed}, err
	case "/api/ship-fits/get":
		fit, err := store.Get(v.Str(body["id"]))
		return M{"fit": fit}, err
	case "/api/ship-fits/save":
		fit, err := shipFitRecord(body)
		if err != nil {
			return nil, err
		}
		fit, err = store.Save(fit)
		return M{"fit": fit}, err
	case "/api/ship-fits/delete":
		err := store.Delete(v.Str(body["id"]))
		return M{"ok": err == nil}, err
	case "/api/ship-fits/export":
		fits, err := store.List(true)
		return M{"fits": fits}, err
	case "/api/ship-fits/import":
		rows := v.Rows(body["fits"])
		if len(rows) == 0 || len(rows) > 1000 || len(rows) != len(v.Arr(body["fits"])) {
			return nil, errors.New("Некорректный список сборок.")
		}
		fits := make([]shipfits.Record, 0, len(rows))
		for _, row := range rows {
			fit, err := shipFitRecord(row)
			if err != nil {
				return nil, err
			}
			fits = append(fits, fit)
		}
		fits, err := store.Import(fits)
		return M{"fits": fits}, err
	default:
		return nil, errors.New("Действие со сборками недоступно.")
	}
}
