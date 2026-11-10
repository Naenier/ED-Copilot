package engineering

import (
	"strconv"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type roadmapObservations struct {
	metrics       M
	cargo         map[string]int
	cargoKnown    bool
	contributions map[string]map[string]int
	ship          map[string]M
	shipID        string
	stored        map[string]M
	personal      map[string]M
}

func (p *Planner) resetRoadmap() {
	p.roadmap = roadmapObservations{metrics: M{}, cargo: map[string]int{}, contributions: map[string]map[string]int{}, ship: map[string]M{}, stored: map[string]M{}, personal: map[string]M{}}
}

func (p *Planner) observeRoadmap(kind string, event M) {
	r := &p.roadmap
	switch kind {
	case "Rank", "Promotion":
		for field, key := range map[string]string{"Combat": "combatRank", "Trade": "tradeRank", "Explore": "explorationRank", "Federation": "federationRank", "Empire": "empireRank"} {
			if n, ok := v.Number(event[field]); ok && n >= 0 && n <= 100 {
				r.metrics[key] = n
			}
		}
	case "Statistics":
		if distance, ok := v.Number(v.At(event, "Exploration", "Greatest_Distance_From_Start")); ok && distance >= 0 {
			r.metrics["distanceFromStart"] = distance
		}
	case "Cargo":
		if vessel := v.Str(event["Vessel"]); vessel != "" && !strings.EqualFold(vessel, "ship") {
			return
		}
		if rows := v.Arr(event["Inventory"]); rows != nil {
			if len(rows) > 1024 {
				r.cargoKnown = false
				return
			}
			next, valid := map[string]int{}, true
			for _, raw := range rows {
				row := v.Obj(raw)
				key := v.Key(row["Name"])
				count, err := integer(row["Count"], "", 0, 1000000)
				if key == "" || err != nil {
					valid = false
					continue
				}
				mission := v.Str(row["MissionID"])
				missionCargo := mission != "" && mission != "0" && mission != "18446744073709551615"
				if v.Int(row["Stolen"]) > 0 || missionCargo {
					// Stolen or mission-specific goods are not reliable hand-in stock.
					count -= v.Int(row["Stolen"])
					if missionCargo {
						count = 0
					}
				}
				next[key] += max(0, count)
			}
			r.cargo, r.cargoKnown = next, valid
		}
	case "MarketBuy", "MarketSell", "CargoTransfer", "CollectCargo", "EjectCargo", "MiningRefined", "CargoDepot", "ColonisationContribution", "CarrierDepositFuel":
		// The next Cargo snapshot will establish the ship inventory again.
		r.cargoKnown = false
	case "EngineerContribution":
		engineer := roadmapEngineerKey(event["EngineerID"], event["Engineer"])
		total, valid := v.Number(event["TotalQuantity"])
		if engineer == "" || !valid || total < 0 || total > 1e12 {
			return
		}
		category := v.Key(event["Type"])
		item := v.Key(v.First(event["Commodity"], event["Material"], event["Name"]))
		if category == "commodity" {
			r.cargoKnown = false
		}
		if category == "materials" {
			if key := MaterialKey(v.First(event["Material"], event["Name"])); key != "" {
				item = key
			}
		}
		if item == "" {
			item = category
		}
		if r.contributions[engineer] == nil {
			r.contributions[engineer] = map[string]int{}
		}
		if _, present := r.contributions[engineer][item]; !present && len(r.contributions[engineer]) >= 128 {
			return
		}
		r.contributions[engineer][item] = int(total)
	case "LoadGame", "ShipyardSwap", "ShipyardNew":
		id := v.Str(event["ShipID"])
		if id == "" || id != r.shipID {
			r.ship, r.shipID = map[string]M{}, id
		}
		r.cargoKnown = false
	case "Loadout":
		r.observeLoadout(event)
	case "StoredModules":
		r.observeStoredModules(event)
	case "EngineerCraft", "EngineerLegacyConvert":
		if !v.Bool(event["IsPreview"]) {
			r.observeCraft(event)
		}
	case "ModuleBuy", "ModuleSell", "ModuleStore", "ModuleRetrieve", "ModuleSwap", "MassModuleStore":
		if slot := v.Text(event["Slot"], event["BuySlot"], event["FromSlot"]); slot != "" {
			delete(r.ship, slot)
			if kind == "ModuleSwap" {
				delete(r.ship, v.Str(event["ToSlot"]))
			}
		} else {
			r.ship = map[string]M{}
		}
		if kind == "ModuleStore" || kind == "ModuleRetrieve" || kind == "MassModuleStore" {
			r.invalidateStoredSlot(event)
		}
	case "ModuleSellRemote", "FetchRemoteModule":
		r.invalidateStoredSlot(event)
	case "SuitLoadout", "SwitchSuitLoadout", "CreateSuitLoadout":
		r.observePersonalLoadout(event)
	case "BuySuit", "UpgradeSuit":
		r.observePersonal("suit", event["SuitID"], event["Name"], event["Name_Localised"], event["Class"], event["SuitMods"])
	case "BuyWeapon", "UpgradeWeapon":
		r.observePersonal("weapon", event["SuitModuleID"], event["Name"], event["Name_Localised"], event["Class"], event["WeaponMods"])
	case "SellSuit":
		delete(r.personal, "suit:"+v.Str(event["SuitID"]))
	case "SellWeapon":
		delete(r.personal, "weapon:"+v.Str(event["SuitModuleID"]))
	}
}

func shipModule(symbol string) string {
	if key := roadmapData.moduleSymbols[symbol]; key != "" {
		return key
	}
	// Symbols use a family prefix followed by equipment-specific size, rating,
	// mount and variant segments. Match the most specific families first.
	var families = []struct{ prefix, key string }{
		{"int_dronecontrol_collection", "cc"}, {"int_dronecontrol_prospector", "pc"},
		{"int_dronecontrol_fueltransfer", "fx"}, {"int_dronecontrol_unkvesselresearch", ""},
		{"int_dronecontrol_resourcesiphon", "hb"}, {"int_dronecontrol_hatchbreaker", "hb"},
		{"hpt_advancedmulticannon", "advmc"}, {"hpt_advancedmissilerack", "amr"},
		{"hpt_causticsinklauncher", "csl"}, {"hpt_heatsinklauncher", "hs"}, {"hpt_chafflauncher", "ch"},
		{"hpt_electroniccountermeasure", "ec"}, {"hpt_pointdefence", "po"},
		{"hpt_cargoscanner", "cs"}, {"hpt_crimescanner", "kw"}, {"hpt_cloudscanner", "ws"},
		{"hpt_shieldbooster", "sb"}, {"hpt_beamlaser", "bl"}, {"hpt_burstlaser", "ul"}, {"hpt_pulselaser", "pl"},
		{"hpt_plasmaaccelerator", "pa"}, {"hpt_railgun", "rg"}, {"hpt_multicannon", "mc"},
		{"hpt_cannon", "c"}, {"hpt_slugshot", "fc"},
		{"hpt_dumbfiremissilerack", "mr"}, {"hpt_missilerack", "nl"}, {"hpt_torpedopylon", "tp"},
		{"int_hyperdrive", "fsd"}, {"int_engine", "t"}, {"int_powerplant", "pp"},
		{"int_powerdistributor", "pd"}, {"int_lifesupport", "ls"}, {"int_sensors", "s"},
		{"int_shieldcellbank", "scb"}, {"int_hullreinforcement", "hr"},
		{"int_fuelscoop", "fs"}, {"int_fsdinterdictor", "fi"}, {"int_detailedsurfacescanner", "ss"},
		{"int_stellarbodydiscoveryscanner", "ss"}, {"int_repairer", "am"}, {"int_refinery", "rf"},
	}
	if strings.HasPrefix(symbol, "int_shieldgenerator") {
		if strings.HasSuffix(symbol, "_fast") {
			return "bsg"
		}
		if strings.HasSuffix(symbol, "_strong") {
			return "psg"
		}
		return "sg"
	}
	if strings.Contains(symbol, "_armour_") {
		return "bh"
	}
	for _, family := range families {
		if symbol == family.prefix || strings.HasPrefix(symbol, family.prefix+"_") {
			return family.key
		}
	}
	return ""
}

func (r *roadmapObservations) observeLoadout(event M) {
	if v.Arr(event["Modules"]) == nil {
		return
	}
	r.shipID, r.ship = v.Text(event["ShipID"], r.shipID), map[string]M{}
	for _, module := range v.Rows(event["Modules"]) {
		if len(r.ship) >= 96 {
			break
		}
		symbol, slot := symbolText(module["Item"]), v.Str(module["Slot"])
		key := shipModule(symbol)
		if key == "" || slot == "" || len(slot) > 120 {
			continue
		}
		row := M{"key": shipEquipmentKey(r.shipID, slot), "moduleKey": key, "name": v.Text(module["Item_Localised"], modules[key]["name"]), "slot": slot, "symbol": symbol, "grade": 0, "observedAt": event["timestamp"], "known": true, "effectKnown": true}
		if mod := v.Obj(module["Engineering"]); len(mod) > 0 {
			applyShipUpgrade(row, mod)
		}
		r.ship[slot] = row
	}
}

func (r *roadmapObservations) observeCraft(event M) {
	symbol, slot := symbolText(event["Module"]), v.Str(event["Slot"])
	key := shipModule(symbol)
	if key == "" || slot == "" || len(slot) > 120 {
		return
	}
	row := r.ship[slot]
	if row == nil || row["symbol"] != symbol {
		if len(r.ship) >= 96 {
			return
		}
		row = M{"key": shipEquipmentKey(r.shipID, slot), "moduleKey": key, "name": modules[key]["name"], "slot": slot, "symbol": symbol}
	}
	applyShipUpgrade(row, event)
	row["observedAt"], row["known"] = event["timestamp"], true
	r.ship[slot] = row
}

func shipEquipmentKey(id, slot string) string {
	if id == "" {
		return "ship:" + slot
	}
	return "ship:" + id + ":" + slot
}

func (r *roadmapObservations) observeStoredModules(event M) {
	if v.Arr(event["Items"]) == nil {
		return
	}
	r.stored = map[string]M{}
	for _, module := range v.Rows(event["Items"]) {
		if len(r.stored) >= 240 {
			break
		}
		symbol, slot := symbolText(module["Name"]), v.Str(module["StorageSlot"])
		key := shipModule(symbol)
		if key == "" || slot == "" || len(slot) > 12 {
			continue
		}
		row := M{"key": "stored:" + slot, "moduleKey": key, "name": v.Text(module["Name_Localised"], modules[key]["name"]),
			"slot": slot, "symbol": symbol, "system": module["StarSystem"], "stored": true, "grade": 0, "observedAt": event["timestamp"], "known": true}
		if v.Str(module["EngineerModifications"]) != "" {
			applyShipUpgrade(row, module)
		}
		r.stored[slot] = row
	}
}

func (r *roadmapObservations) invalidateStoredSlot(event M) {
	if slot := v.Str(event["StorageSlot"]); slot != "" {
		delete(r.stored, slot)
	} else {
		r.stored = map[string]M{}
	}
}

func applyShipUpgrade(row, mod M) {
	if key := roadmapEngineerKey(mod["EngineerID"], mod["Engineer"]); key != "" {
		engineer := roadmapData.engineers[key]
		row["engineerKey"], row["engineerName"], row["engineerEnglish"] = key, engineer["name"], engineer["english"]
	}
	if name := v.First(mod["BlueprintName"], mod["EngineerModifications"]); v.Key(name) != "" {
		symbol := v.Key(name)
		key := journalUpgradeKey(v.Str(row["moduleKey"]), "blueprints", symbol, blueprints, roadmapData.blueprintKeys)
		if key == "" {
			row["unmappedBlueprint"] = name
		} else {
			delete(row, "unmappedBlueprint")
		}
		if key != row["blueprintKey"] {
			delete(row, "quality")
			if row["blueprintKey"] != nil {
				delete(row, "experimentalKey")
				row["effectKnown"] = false
			}
		}
		row["blueprintKey"] = key
	}
	if grade, err := integer(mod["Level"], "", 1, 5); err == nil {
		if grade != v.Int(row["grade"]) {
			delete(row, "quality")
		}
		row["grade"] = grade
	}
	if quality, ok := v.Number(mod["Quality"]); ok && quality >= 0 && quality <= 1 {
		row["quality"] = quality
	}
	if symbol := v.Key(v.First(mod["ApplyExperimentalEffect"], mod["ExperimentalEffect"])); symbol != "" {
		key := journalUpgradeKey(v.Str(row["moduleKey"]), "experimentals", symbol, effects, roadmapData.effectKeys)
		row["experimentalKey"] = key
		row["effectKnown"] = key != ""
	}
}

func (r *roadmapObservations) observePersonalLoadout(event M) {
	r.observePersonal("suit", event["SuitID"], event["SuitName"], event["SuitName_Localised"], nil, event["SuitMods"])
	for _, weapon := range v.Rows(event["Modules"]) {
		r.observePersonal("weapon", weapon["SuitModuleID"], weapon["ModuleName"], weapon["ModuleName_Localised"], weapon["Class"], weapon["WeaponMods"])
	}
}

func (r *roadmapObservations) observePersonal(kind string, id, item, localized, class, mods any) {
	identity, symbol := v.Str(id), symbolText(item)
	if identity == "" || len(identity) > 30 || symbol == "" || len(symbol) > 120 {
		return
	}
	key := kind + ":" + identity
	row := r.personal[key]
	if row == nil {
		if len(r.personal) >= 96 {
			return
		}
		row = M{"key": key, "moduleKey": kind, "symbol": symbol, "name": v.Text(localized, item)}
	}
	grade := v.Int(class)
	if grade == 0 && kind == "suit" {
		if _, suffix, found := strings.Cut(symbol, "_class"); found {
			grade, _ = strconv.Atoi(suffix)
		}
	}
	if grade >= 1 && grade <= 5 {
		row["grade"] = grade
	}
	if raw := v.Arr(mods); raw != nil {
		values := []string{}
		for _, mod := range raw {
			if value := symbolText(mod); value != "" && len(value) <= 120 && len(values) < 8 {
				values = append(values, value)
			}
		}
		row["mods"] = values
	}
	r.personal[key] = row
}
