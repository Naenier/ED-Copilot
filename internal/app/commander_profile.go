package app

import (
	"github.com/Naenier/ED-Copilot/internal/engineering"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// applyVerifiedProfileCAPI binds the returned authenticated account identity to
// the still active journal FID while excluding a concurrent owner transition.
func (e *Engine) applyVerifiedProfileCAPI(result M) bool {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	return e.applyVerifiedProfileCAPILocked(result)
}

func (e *Engine) applyVerifiedProfileCAPILocked(result M) bool {
	cmd := e.Commander()
	status := v.Obj(result["status"])
	if !v.Bool(status["connected"]) || v.Bool(status["commanderMismatch"]) || v.Str(cmd["fid"]) == "" || v.Str(status["fid"]) != v.Str(cmd["fid"]) {
		return false
	}
	e.Colonies.ApplyCAPI(result, cmd)
	context := M{"identityVerified": true, "commander": cmd, "observedAt": v.At(result, "metadata", "observedAt", "profile")}
	profile := v.Obj(v.At(result, "data", "profile"))
	e.observeFrontierFleet(result, profile, cmd)
	changed := e.Ranks.ApplyCAPI(profile, context)
	return e.CommanderProfile.ApplyCAPI(profile, context) || changed
}

// commanderProfileSnapshot combines owner-scoped journal observations with the
// existing engineering ledger, which tracks material expenses and collection.
// Callers already hold domainMu, so this cannot mix two journal events or FIDs.
func (e *Engine) commanderProfileSnapshot(inventory M) M {
	state := e.CommanderProfile.Snapshot()
	materials := v.Obj(v.At(state, "inventory", "materials"))
	known := v.Obj(v.At(inventory, "shopping", "knownCategories"))
	for _, category := range []string{"raw", "manufactured", "encoded"} {
		previous := v.Obj(materials[category])
		at := v.Str(v.At(inventory, "shopping", "updatedAt", category))
		// The ledger updates timestamps per affected category. Another
		// category can stay current without a new snapshot of its contents.
		previousAt := v.Str(previous["updatedAt"])
		if previousAt != "" && (at == "" || v.Time(at).Before(v.Time(previousAt))) {
			for _, row := range v.Rows(previous["entries"]) {
				if name, english := engineering.MaterialNames(row["symbol"]); name != "" {
					row["name"], row["english"] = name, english
				}
			}
			continue
		}
		rows, total := []M{}, 0
		complete := v.Bool(known[category])
		for _, raw := range v.Rows(inventory["materials"]) {
			if raw["category"] != category {
				continue
			}
			if raw["owned"] == nil {
				complete = false
				continue
			}
			count := v.Int(raw["owned"])
			if count < 0 {
				complete = false
				continue
			}
			total += count
			if count == 0 {
				continue
			}
			rows = append(rows, M{"key": raw["key"], "symbol": raw["symbol"], "name": raw["name"], "english": raw["english"], "count": count, "ownerId": "", "missionId": ""})
		}
		var count any
		if complete {
			count = total
		}
		var updatedAt any
		if at != "" {
			updatedAt = at
		}
		materials[category] = M{"known": complete, "entries": rows, "count": count, "updatedAt": updatedAt, "source": "journal", "truncated": false, "uncertain": v.Bool(known[category]) && !complete, "projection": "engineeringInventory"}
	}
	for _, container := range []string{"shipLocker", "backpack"} {
		for _, group := range v.Obj(v.At(state, "inventory", container)) {
			for _, row := range v.Rows(v.Obj(group)["entries"]) {
				if name, english := engineering.OdysseyMaterialNames(row["symbol"]); name != "" {
					row["name"], row["english"] = name, english
				}
			}
		}
	}
	return state
}
