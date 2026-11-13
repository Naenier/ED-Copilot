package app

import v "github.com/Naenier/ED-Copilot/internal/value"

// CurrentShipEngineering reads the active commander's last complete loadout.
func (e *Engine) CurrentShipEngineering(input M) (M, error) {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.RLock()
	owner := e.planWorkspace.key
	game := v.Copy(e.game)
	commander := v.Copy(e.commander)
	e.mu.RUnlock()
	if err := validatePlanWorkspace(input, owner); err != nil {
		return nil, err
	}
	var loadout M
	if owner != "" && owner == e.section7.observedOwner {
		loadout = e.section7.loadout
		if shipID := v.Str(commander["shipId"]); shipID != "" && shipID != v.Str(loadout["ShipID"]) {
			loadout = nil
		}
	}
	return e.Engineering.CurrentShipEngineering(loadout, input, M{"game": game})
}
