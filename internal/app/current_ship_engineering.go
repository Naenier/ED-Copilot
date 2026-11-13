package app

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/profile"
	"github.com/Naenier/ED-Copilot/internal/ships"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const currentShipEngineeringLimit = 96

type shipEngineeringContext struct {
	owner, shipID  string
	ship, shipName string
	loadout        M
	observations   M
	closed         bool
}

func (e *Engine) currentShipEngineeringContext() shipEngineeringContext {
	e.mu.RLock()
	defer e.mu.RUnlock()
	cmd := e.commander
	ctx := shipEngineeringContext{
		owner: e.planWorkspace.key, closed: e.closed,
		observations: M{"game": e.game, "system": cmd["system"], "position": cmd["coordinates"]},
	}
	if ctx.owner != "" && ctx.owner == e.section7.observedOwner {
		ctx.shipID = e.section7.observedShipID
		commanderShipID := engineeringShipID(cmd["shipId"])
		if ctx.shipID != "" && (commanderShipID == "" || commanderShipID == ctx.shipID) {
			ctx.loadout = e.section7.loadout
			ctx.ship, ctx.shipName = v.Str(cmd["ship"]), v.Str(cmd["shipName"])
		} else {
			ctx.shipID = ""
		}
	}
	return ctx
}

func engineeringShipID(raw any) string {
	id, err := strconv.ParseUint(v.Str(raw), 10, 64)
	if err != nil {
		return ""
	}
	return strconv.FormatUint(id, 10)
}

func observedEngineeringShipID(event M) string {
	ship := v.Text(event["Ship"], event["ShipType"])
	if ship == "" || ships.VehicleKind(ship) != "ship" {
		return ""
	}
	if event["event"] == "ShipyardNew" {
		return engineeringShipID(v.First(event["NewShipID"], event["ShipID"]))
	}
	return engineeringShipID(event["ShipID"])
}

// CurrentShipEngineering combines the observed build with the saved targets
// for this commander and ship. Totals and visits are recalculated on read.
func (e *Engine) CurrentShipEngineering(input M) (M, error) {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	ctx := e.currentShipEngineeringContext()
	if err := validatePlanWorkspace(input, ctx.owner); err != nil {
		return nil, err
	}
	targets := e.currentShipTargets(ctx)
	detailInput, implicitTarget := plannedShipDetailInput(input, targets, ctx.loadout)
	result, err := e.Engineering.CurrentShipEngineering(ctx.loadout, detailInput, ctx.observations)
	if err != nil && implicitTarget {
		// A catalogue change can invalidate a saved goal. Preserve it in the
		// plan and leave the current slot's available recipes accessible.
		result, err = e.Engineering.CurrentShipEngineering(ctx.loadout, input, ctx.observations)
	}
	if err != nil {
		return nil, err
	}
	return e.withCurrentShipPlan(result, ctx, targets), nil
}

func (e *Engine) currentShipTargets(ctx shipEngineeringContext) []M {
	if ctx.owner == "" || ctx.shipID == "" {
		return nil
	}
	saved := v.Rows(v.At(e.section7.currentShipEngineeringPlans, ctx.owner, ctx.shipID))
	targets := make([]M, 0, min(currentShipEngineeringLimit, len(saved)))
	seen := make(map[string]bool, cap(targets))
	for _, row := range saved {
		target, err := shipEngineeringTarget(row)
		if err != nil || seen[v.Str(target["slot"])] {
			continue
		}
		targets = append(targets, target)
		seen[v.Str(target["slot"])] = true
		if len(targets) == currentShipEngineeringLimit {
			break
		}
	}
	return targets
}

func (e *Engine) withCurrentShipPlan(result M, ctx shipEngineeringContext, targets []M) M {
	if ctx.shipID != "" {
		result["shipID"] = ctx.shipID
		result["ship"], result["shipName"] = v.Text(ctx.ship, result["ship"]), v.Text(ctx.shipName, result["shipName"])
	}
	if detail := v.Obj(result["detail"]); len(detail) > 0 {
		detail["item"] = currentShipItem(ctx.loadout, v.Str(detail["slot"]))
	}
	result["plan"] = e.Engineering.CurrentShipPlan(ctx.loadout, targets, ctx.observations)
	return result
}

func plannedShipDetailInput(input M, targets []M, loadout M) (M, bool) {
	for _, key := range []string{"blueprintKey", "grade", "experimentalKey"} {
		if _, chosen := input[key]; chosen {
			return input, false
		}
	}
	slot := v.Str(input["slot"])
	if slot == "" {
		return input, false
	}
	item := currentShipItem(loadout, slot)
	for _, target := range targets {
		if target["slot"] == slot && sameEngineeringItem(target["item"], item) {
			return v.Merge(v.Copy(input), target), true
		}
	}
	return input, false
}

func currentShipItem(loadout M, slot string) string {
	for _, module := range v.Rows(loadout["Modules"]) {
		if module["Slot"] == slot {
			return v.Str(module["Item"])
		}
	}
	return ""
}

func sameEngineeringItem(a, b any) bool {
	symbol := func(raw any) string {
		item := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v.Str(raw))), "$")
		return strings.TrimSuffix(strings.TrimSuffix(item, ";"), "_name")
	}
	left, right := symbol(a), symbol(b)
	return left != "" && left == right
}

func shipEngineeringSlot(raw any) (string, bool) {
	slot, ok := raw.(string)
	return slot, ok && slot != "" && len(slot) <= 120 && !strings.ContainsAny(slot, "\x00\r\n")
}

func shipEngineeringTarget(input M) (M, error) {
	slot, validSlot := shipEngineeringSlot(input["slot"])
	item, validItem := input["item"].(string)
	blueprint, validBlueprint := input["blueprintKey"].(string)
	effect, validEffect := input["experimentalKey"].(string)
	grade, known := v.Number(input["grade"])
	if !validSlot || !validItem || item == "" || len(item) > 200 || !validBlueprint || blueprint == "" || len(blueprint) > 200 ||
		!validEffect || len(effect) > 200 || !known || grade < 1 || grade > 5 || math.Trunc(grade) != grade {
		return nil, errors.New("Выберите модуль, чертёж и ступень улучшения.")
	}
	return M{"slot": slot, "item": item, "blueprintKey": blueprint, "grade": int(grade), "experimentalKey": effect}, nil
}

// UpdateCurrentShipEngineeringPlan commits the settings before exposing the
// updated plan. The lock order matches Save and prevents concurrent changes
// to the observed ship or plan between validation and persistence.
func (e *Engine) UpdateCurrentShipEngineeringPlan(input M) (M, error) {
	e.saveMu.Lock()
	defer e.saveMu.Unlock()
	e.domainMu.Lock()
	defer e.domainMu.Unlock()

	ctx := e.currentShipEngineeringContext()
	if ctx.closed || ctx.owner == "" || ctx.shipID == "" {
		return nil, errors.New("Не удалось определить текущий корабль по журналу.")
	}
	if input["expectedPlanWorkspace"] != ctx.owner {
		return nil, errors.New("Командир сменился. Откройте действие заново.")
	}
	if input["expectedShipID"] != ctx.shipID {
		return nil, errors.New("Корабль сменился. Откройте план заново.")
	}
	previous := e.currentShipTargets(ctx)
	next, result, err := e.editCurrentShipEngineeringPlan(ctx, previous, input)
	if err != nil {
		return nil, err
	}
	if !sameShipEngineeringTargets(next, previous) {
		plans := e.section7.currentShipEngineeringPlans
		oldShips := v.Obj(plans[ctx.owner])
		ships := v.Copy(oldShips)
		if len(next) == 0 {
			delete(ships, ctx.shipID)
		} else {
			ships[ctx.shipID] = next
		}
		if len(ships) > 0 {
			plans[ctx.owner] = ships
		} else {
			delete(plans, ctx.owner)
		}
		saved, _ := e.persistenceSnapshotLocked()
		if err := profile.SaveSettings(e.opts.DataDir, saved); err != nil {
			if len(oldShips) > 0 {
				plans[ctx.owner] = oldShips
			} else {
				delete(plans, ctx.owner)
			}
			return nil, err
		}
		e.mu.Lock()
		e.revision++
		e.engineeringRoadmapRevision++
		e.mu.Unlock()
		e.Notify()
	}
	return e.withCurrentShipPlan(result, ctx, next), nil
}

func sameShipEngineeringTargets(a, b []M) bool {
	if len(a) != len(b) {
		return false
	}
	for i, row := range a {
		for key, value := range row {
			if b[i][key] != value {
				return false
			}
		}
	}
	return true
}

func (e *Engine) editCurrentShipEngineeringPlan(ctx shipEngineeringContext, goals []M, input M) ([]M, M, error) {
	action := v.Str(input["action"])
	switch action {
	case "upsert":
		target, err := shipEngineeringTarget(input)
		if err != nil {
			return nil, nil, err
		}
		slot := v.Str(target["slot"])
		item := currentShipItem(ctx.loadout, slot)
		if !sameEngineeringItem(target["item"], item) {
			return nil, nil, errors.New("Модуль в этом слоте сменился. Откройте его заново.")
		}
		target["item"] = item
		result, err := e.Engineering.CurrentShipEngineering(ctx.loadout, target, ctx.observations)
		if err != nil {
			return nil, nil, err
		}
		detail := v.Obj(result["detail"])
		if detail["blueprintKey"] != target["blueprintKey"] || !v.Bool(v.At(detail, "recipe", "compatible")) {
			return nil, nil, errors.New("Это улучшение недоступно для выбранного модуля.")
		}
		for i, goal := range goals {
			if goal["slot"] == slot {
				next := append([]M(nil), goals...)
				next[i] = target
				return next, result, nil
			}
		}
		if len(goals) == currentShipEngineeringLimit {
			return nil, nil, errors.New("Достигнут предел целей для этого корабля.")
		}
		return append(goals, target), result, nil
	case "remove":
		slot, ok := shipEngineeringSlot(input["slot"])
		if !ok {
			return nil, nil, errors.New("Некорректный слот модуля")
		}
		for i, goal := range goals {
			if goal["slot"] == slot {
				next := make([]M, 0, len(goals)-1)
				next = append(next, goals[:i]...)
				goals = append(next, goals[i+1:]...)
				break
			}
		}
	case "clear":
		goals = []M{}
	default:
		return nil, nil, errors.New("Неизвестное действие с планом инженеров.")
	}
	result, err := e.Engineering.CurrentShipEngineering(ctx.loadout, nil, ctx.observations)
	return goals, result, err
}
