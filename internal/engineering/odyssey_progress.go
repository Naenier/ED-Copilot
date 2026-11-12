package engineering

import (
	"errors"
	"sort"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const maxOdysseyWorkspaces = 64

func (p *Planner) restoreOdysseyLoadouts(saved M) {
	p.odysseyPlans = map[string]*odysseyWorkspace{}
	for _, owner := range sortedOdysseyKeys(saved) {
		if !odysseyScopePattern.MatchString(owner) || len(p.odysseyPlans) >= maxOdysseyWorkspaces {
			continue
		}
		raw := v.Obj(saved[owner])
		ws := &odysseyWorkspace{loadouts: []M{}, progress: map[string]M{}}
		seen := map[string]bool{}
		for _, row := range v.Rows(raw["loadouts"]) {
			id := v.Str(row["id"])
			if !odysseyScopePattern.MatchString(id) || seen[id] || len(ws.loadouts) >= maxOdysseyLoadouts {
				continue
			}
			if kit, err := validateOdysseyLoadout(row); err == nil {
				seen[id] = true
				ws.loadouts = append(ws.loadouts, kit)
			}
		}
		if len(ws.loadouts) == 0 {
			continue
		}
		ws.selected = v.Str(raw["selectedId"])
		if !seen[ws.selected] {
			ws.selected = v.Str(ws.loadouts[0]["id"])
		}
		referenced := odysseyBindings(ws.loadouts)
		for _, row := range v.Rows(raw["equipmentProgress"]) {
			key := v.Str(row["key"])
			if referenced[key] && odysseyItemPattern.MatchString(key) {
				if progress := validateOdysseyProgress(row); progress != nil {
					ws.progress[key] = progress
				}
			}
		}
		p.odysseyPlans[owner] = ws
	}
}

func (p *Planner) saveOdysseyLoadouts() M {
	out := M{}
	for owner, ws := range p.odysseyPlans {
		progress := make([]M, 0, len(ws.progress))
		keys := make([]string, 0, len(ws.progress))
		for key := range ws.progress {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			progress = append(progress, ws.progress[key])
		}
		out[owner] = M{"selectedId": ws.selected, "loadouts": ws.loadouts, "equipmentProgress": progress}
	}
	return out
}

func validateOdysseyProgress(row M) M {
	if v.Time(row["observedAt"]).IsZero() {
		return nil
	}
	out := M{"key": row["key"], "observedAt": row["observedAt"]}
	if v.Bool(row["sold"]) {
		out["sold"] = true
		return out
	}
	symbol := symbolText(row["symbol"])
	model := odysseyEquipmentByKey[odysseyItemKey(symbol)]
	if model == nil {
		return nil
	}
	out["symbol"], out["moduleKey"] = symbol, model["moduleKey"]
	if grade, err := integer(row["grade"], "", 1, 5); err == nil {
		out["grade"], out["gradeObservedAt"] = grade, row["gradeObservedAt"]
	}
	if raw := v.Arr(row["mods"]); raw != nil {
		mods := []string{}
		for _, value := range raw {
			if mod := symbolText(value); mod != "" && len(mod) <= 120 && len(mods) < 8 && !contains(mods, mod) {
				mods = append(mods, mod)
			}
		}
		out["mods"], out["modsObservedAt"] = mods, row["modsObservedAt"]
	}
	return out
}

func odysseyBindings(loadouts []M) map[string]bool {
	refs := map[string]bool{}
	for _, kit := range loadouts {
		for _, item := range v.Rows(kit["items"]) {
			if key := v.Str(item["equipmentKey"]); key != "" {
				refs[key] = true
			}
		}
	}
	return refs
}

func (p *Planner) captureOdysseyProgress(owner, kind string, event M) {
	ws := p.odysseyPlans[owner]
	if ws == nil {
		return
	}
	switch kind {
	case "SuitLoadout", "SwitchSuitLoadout", "CreateSuitLoadout", "LoadoutEquipModule", "BuySuit", "UpgradeSuit", "BuyWeapon", "UpgradeWeapon":
		p.refreshOdysseyProgress(owner, ws)
	case "SellSuit", "SellWeapon":
		key := "suit:" + v.Str(event["SuitID"])
		if kind == "SellWeapon" {
			key = "weapon:" + v.Str(event["SuitModuleID"])
		}
		at := v.Time(event["timestamp"])
		if _, known := ws.progress[key]; !at.IsZero() && (known || odysseyBindings(ws.loadouts)[key]) &&
			!at.Before(v.Time(ws.progress[key]["observedAt"])) && p.roadmap.personal[key] == nil {
			ws.progress[key] = M{"key": key, "observedAt": event["timestamp"], "sold": true}
		}
	}
}

func (p *Planner) refreshOdysseyProgress(owner string, ws *odysseyWorkspace) {
	refs := odysseyBindings(ws.loadouts)
	for key := range ws.progress {
		if !refs[key] {
			delete(ws.progress, key)
		}
	}
	if p.odysseyObservationOwner != owner {
		return
	}
	for key := range refs {
		if candidate := validateOdysseyProgress(p.roadmap.personal[key]); candidate != nil {
			ws.progress[key] = mergeOdysseyProgress(ws.progress[key], candidate)
		}
	}
}

func mergeOdysseyProgress(older, observed M) M {
	if len(older) == 0 {
		return observed
	}
	if len(observed) == 0 {
		return older
	}
	newer := !v.Time(observed["observedAt"]).Before(v.Time(older["observedAt"]))
	if v.Bool(older["sold"]) || v.Bool(observed["sold"]) || odysseyItemKey(older["symbol"]) != odysseyItemKey(observed["symbol"]) {
		if !newer || v.Bool(older["sold"]) && v.Time(older["observedAt"]).Equal(v.Time(observed["observedAt"])) {
			return older
		}
		return observed
	}
	result := v.Merge(M{}, older)
	if newer {
		result["symbol"], result["observedAt"] = observed["symbol"], observed["observedAt"]
	}
	if observed["grade"] != nil && v.Int(observed["grade"]) >= v.Int(older["grade"]) &&
		!v.Time(observed["gradeObservedAt"]).Before(v.Time(older["gradeObservedAt"])) {
		result["grade"], result["gradeObservedAt"] = observed["grade"], observed["gradeObservedAt"]
	}
	if observed["mods"] != nil && !v.Time(observed["modsObservedAt"]).Before(v.Time(older["modsObservedAt"])) {
		result["mods"], result["modsObservedAt"] = observed["mods"], observed["modsObservedAt"]
	}
	return result
}

func (p *Planner) odysseyEquipmentFor(owner string, ws *odysseyWorkspace, key string) M {
	row := ws.progress[key]
	if p.odysseyObservationOwner == owner {
		row = mergeOdysseyProgress(row, p.roadmap.personal[key])
	}
	if v.Bool(row["sold"]) {
		return nil
	}
	return row
}

func (p *Planner) checkOdysseyBindings(owner string, ws *odysseyWorkspace, kit M, replaced int) error {
	current := map[string]bool{}
	if replaced >= 0 {
		current = odysseyBindings([]M{ws.loadouts[replaced]})
	}
	for _, item := range v.Rows(kit["items"]) {
		key := v.Str(item["equipmentKey"])
		if key == "" {
			continue
		}
		observation := p.odysseyEquipmentFor(owner, ws, key)
		if observation == nil && !current[key] ||
			observation != nil && odysseyItemKey(observation["symbol"]) != item["itemKey"] {
			return errors.New("Наблюдаемое снаряжение не соответствует выбранной модели.")
		}
	}
	return nil
}

func sortedOdysseyKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
