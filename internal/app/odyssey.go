package app

import (
	"math"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (e *Engine) odysseyObservations(request M) (M, error) {
	e.mu.RLock()
	workspace, fid, game := e.planWorkspace, v.Str(e.commander["fid"]), v.Copy(e.game)
	e.mu.RUnlock()
	if err := validatePlanWorkspace(request, workspace.key); err != nil {
		return nil, err
	}
	var locker, backpack M
	if workspace.branch == "live" && game["odyssey"] != false {
		locker, backpack = e.CommanderProfile.OdysseyInventorySnapshot(fid)
	}
	return M{"planWorkspace": workspace.key, "shipLocker": locker, "backpack": backpack, "game": game}, nil
}

func (e *Engine) OdysseyLoadouts(request M) (M, error) {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	context, err := e.odysseyObservations(request)
	if err != nil {
		return nil, err
	}
	return e.Engineering.OdysseyLoadouts(context), nil
}

func (e *Engine) updateOdysseyLoadout(request M) (M, error) {
	result, err := func() (M, error) {
		e.domainMu.Lock()
		defer e.domainMu.Unlock()
		context, err := e.odysseyObservations(request)
		if err != nil {
			return nil, err
		}
		projection, err := e.Engineering.UpdateOdysseyLoadout(request, context)
		if err == nil {
			e.mu.Lock()
			e.engineeringRoadmapRevision++
			e.mu.Unlock()
		}
		return projection, err
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

const odysseyPickupLimit = 4

func (e *Engine) observeOdysseyPickupsLocked(event, meta M, previousWorkspace string) {
	if e.commander["vehicle"] != "foot" || e.planWorkspace.key == "" || e.planWorkspace.key != previousWorkspace {
		e.odysseyPickups = nil
		return
	}
	if event["event"] != "BackpackChange" || v.Bool(meta["replay"]) || v.Bool(meta["side"]) {
		return
	}
	at := v.Time(event["timestamp"])
	if at.IsZero() {
		return
	}
	newPickups := make([]M, 0, odysseyPickupLimit)
	seen := map[string]int{}
	added := v.Rows(event["Added"])
	for _, row := range added[:min(len(added), 128)] {
		key, category := v.Key(row["Name"]), v.Key(row["Type"])
		mission := v.Str(row["MissionID"])
		count, known := v.Number(row["Count"])
		if key == "" || category == "consumable" || category == "consumables" ||
			mission != "" && mission != "0" && mission != "18446744073709551615" ||
			!known || count <= 0 || count > 100000 || math.Trunc(count) != count {
			continue
		}
		if index, exists := seen[key]; exists {
			newPickups[index]["count"] = v.Num(newPickups[index]["count"]) + count
			continue
		}
		if len(newPickups) >= odysseyPickupLimit {
			continue
		}
		seen[key] = len(newPickups)
		newPickups = append(newPickups, M{"key": key, "count": count, "observedAt": v.ISO(at)})
	}
	if len(newPickups) == 0 {
		return
	}
	for _, row := range e.odysseyPickups {
		if len(newPickups) >= odysseyPickupLimit {
			break
		}
		if _, exists := seen[v.Str(row["key"])]; !exists && at.Sub(v.Time(row["observedAt"])) <= 45*time.Second {
			newPickups = append(newPickups, row)
		}
	}
	e.odysseyPickups = newPickups
}
