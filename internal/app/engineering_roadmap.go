package app

import v "github.com/Naenier/ED-Copilot/internal/value"

// EngineeringRoadmap combines live observations from the active journal
// owner. It does not publish catalogue data in the periodic desktop snapshot.
func (e *Engine) EngineeringRoadmap(input M) (M, error) {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.RLock()
	fid := v.Str(e.commander["fid"])
	game := v.Copy(e.game)
	owner := e.planWorkspace.key
	e.mu.RUnlock()
	if err := validatePlanWorkspace(input, owner); err != nil {
		return nil, err
	}
	return e.Engineering.RoadmapWithContext(input, M{
		"shipLocker": e.CommanderProfile.ShipLockerSnapshot(fid),
		"game":       game,
	})
}

func affectsEngineeringRoadmap(event M) bool {
	switch v.Str(event["event"]) {
	case "Fileheader", "Commander", "LoadGame", "Rank", "Promotion", "Statistics",
		"Materials", "MaterialCollected", "MaterialDiscarded", "MaterialTrade", "Synthesis", "TechnologyBroker",
		"EngineerProgress", "EngineerContribution", "EngineerCraft", "EngineerLegacyConvert",
		"Cargo", "MarketBuy", "MarketSell", "CargoTransfer", "CollectCargo", "EjectCargo", "MiningRefined",
		"CargoDepot", "ColonisationContribution", "CarrierDepositFuel",
		"Loadout", "StoredModules", "ShipyardSwap", "ShipyardNew", "ModuleBuy", "ModuleSell", "ModuleStore", "ModuleRetrieve", "ModuleSwap", "MassModuleStore",
		"SuitLoadout", "SwitchSuitLoadout", "CreateSuitLoadout", "DeleteSuitLoadout", "LoadoutEquipModule", "LoadoutRemoveModule",
		"BuySuit", "UpgradeSuit", "SellSuit", "BuyWeapon", "UpgradeWeapon", "SellWeapon",
		"ShipLocker", "ShipLockerMaterials", "TransferMicroResources", "CollectItems", "DropItems", "UseConsumable",
		"BuyMicroResources", "SellMicroResources", "TradeMicroResources":
		return true
	case "MissionCompleted":
		return len(v.Arr(event["MaterialsReward"])) != 0
	default:
		return false
	}
}
