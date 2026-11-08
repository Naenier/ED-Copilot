package commander

import (
	"reflect"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// A full inventory is an observation, not a ledger. When a later change is
// seen without a replacement inventory, preserve the last observation for
// inspection while making its current total explicitly unknown. This avoids
// estimating mission ownership, resource transfers or module swaps twice.
func (t *Tracker) invalidate(key string, section M, at string) bool {
	if !t.fresh(key, at, "journal") || !v.Bool(section["known"]) && !v.Bool(section["needsSnapshot"]) {
		return false
	}
	if section["invalidatedAt"] == at {
		return false
	}
	if section["needsSnapshot"] != true {
		section["lastKnownCount"] = section["count"]
	}
	section["known"], section["count"], section["needsSnapshot"], section["invalidatedAt"] = false, nil, true, at
	t.stamps[key] = stamp{at, "journal"}
	if t.latest == "" || v.Time(at).After(v.Time(t.latest)) {
		t.latest = at
	}
	return true
}

func (t *Tracker) invalidateInventory(container string, at string) bool {
	groups := v.Obj(v.At(t.state, "inventory", container))
	changed := false
	for key, raw := range groups {
		changed = t.invalidate("inventory."+container+"."+key, v.Obj(raw), at) || changed
	}
	return changed
}

func (t *Tracker) invalidateChanges(event M, at string) bool {
	kind := v.Str(event["event"])
	changed := false
	switch kind {
	case "MaterialCollected", "MaterialDiscarded", "MaterialTrade", "EngineerCraft", "Synthesis", "TechnologyBroker":
		changed = t.invalidateInventory("materials", at)
	case "EngineerContribution":
		if v.Key(event["Type"]) == "materials" {
			changed = t.invalidateInventory("materials", at)
		}
	case "MissionCompleted":
		if len(v.Arr(event["MaterialsReward"])) > 0 {
			changed = t.invalidateInventory("materials", at)
		}
	case "BackpackChange":
		changed = t.invalidateInventory("backpack", at)
	case "TransferMicroResources", "CollectItems", "DropItems", "UseConsumable", "BuyMicroResources", "SellMicroResources", "TradeMicroResources", "UpgradeSuit", "UpgradeWeapon":
		changed = t.invalidateInventory("backpack", at)
		changed = t.invalidateInventory("shipLocker", at) || changed
	case "ShipyardBuy", "ShipyardNew", "ShipyardSell", "ShipyardSwap", "ShipyardTransfer", "SellShipOnRebuy":
		changed = t.invalidate("fleet", v.Obj(t.state["fleet"]), at)
	case "ModuleBuy", "ModuleSell", "ModuleStore", "ModuleRetrieve", "MassModuleStore", "ModuleBuyAndStore", "FetchRemoteModule", "SellModuleOnRebuy":
		changed = t.invalidate("storedModules", v.Obj(t.state["storedModules"]), at)
	}
	return changed
}

// sameObservation compares JSON numeric representations consistently, because
// snapshots use json.Number while synthetic callers may provide native ints.
func sameObservation(a, b any) bool { return reflect.DeepEqual(v.Clone(a), v.Clone(b)) }
