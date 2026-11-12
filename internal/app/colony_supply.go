package app

import (
	"errors"
	"math"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (e *Engine) colonySupplyPortfolio(selection M) (M, error) {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	context, err := e.colonySupplyContext(selection)
	if err != nil {
		return nil, err
	}
	return e.Colonies.SupplyPortfolio(context, selection)
}

func (e *Engine) updateColonySupplyReservation(request M) (M, error) {
	result, err := func() (M, error) {
		e.domainMu.Lock()
		defer e.domainMu.Unlock()
		context, err := e.colonySupplyContext(request)
		if err != nil {
			return nil, err
		}
		return e.Colonies.UpdateSupplyReservation(context, request)
	}()
	if err != nil {
		return nil, err
	}
	if err := e.Save(); err != nil {
		return nil, err
	}
	e.Notify()
	return result, nil
}

func (e *Engine) colonySupplyContext(request M) (M, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if err := validatePlanWorkspace(request, e.planWorkspace.key); err != nil {
		return nil, err
	}
	vehicle := e.commander["vehicle"]
	commander := M{"fid": e.commander["fid"], "vehicle": vehicle, "system": e.commander["system"],
		"cargoForSync": v.At(e.cargoSync, trading.Vessel(vehicle), "inventory")}
	return M{"commander": commander, "planWorkspace": e.planWorkspace.key}, nil
}

func (e *Engine) colonySupplyMarket(request M) (M, error) {
	context, err := e.colonySupplyContext(request)
	if err != nil {
		return nil, err
	}
	commodity := v.Key(request["commodity"])
	quantity, ok := v.Number(request["quantity"])
	if commodity == "" || len(commodity) > 120 || !ok || quantity < 1 || quantity > 1e9 || math.Trunc(quantity) != quantity {
		return nil, errors.New("Выберите товар и объём для закупки.")
	}
	system := strings.TrimSpace(v.Str(v.At(context, "commander", "system")))
	return M{"navigate": "sale", "mode": "Купить", "prefill": M{
		"resourceKey": commodity, "quantity": quantity, "system": system, "useCargo": false, "useCarrierCargo": false,
	}}, nil
}
