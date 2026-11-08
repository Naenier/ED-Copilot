package app

import (
	"errors"
	"time"

	"github.com/Naenier/ED-Copilot/internal/politics"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// SeedDemoCommanderAndPolitics never accepts an online or personal commander.
// It exercises the actual rank/journal and cached-public observation paths.
func (e *Engine) SeedDemoCommanderAndPolitics() error {
	if !e.opts.DisableNetwork || v.Str(e.Commander()["fid"]) != "FNATIVE-SYNTHETIC" {
		return errors.New("Commander demonstration requires the isolated synthetic profile")
	}
	now := time.Now().UTC()
	at := v.ISO(now)
	e.Apply(M{"event": "Rank", "timestamp": at, "Combat": 4, "Trade": 9, "Explore": 8, "Soldier": 3, "Exobiologist": 5, "CQC": 1, "Federation": 4, "Empire": 12}, M{"replay": true})
	e.Apply(M{"event": "Progress", "timestamp": at, "Combat": 42, "Trade": 69, "Explore": 32, "Soldier": 10, "Exobiologist": 0, "CQC": 7, "Federation": 99, "Empire": 37}, M{"replay": true})
	// These are fictional observations of the isolated synthetic identity.
	// None changes the application's commander, location or current ship.
	for _, event := range []M{
		{"event": "Status", "Balance": 2486000000},
		{"event": "Statistics", "Bank_Account": M{"Current_Wealth": 8734000000, "Spent_On_Ships": 1640000000, "Spent_On_Outfitting": 926000000, "Insurance_Claims": 7, "Spent_On_Repairs": 14800000}, "Combat": M{"Bounties_Claimed": 412, "Bounty_Hunting_Profit": 283000000, "Combat_Bonds": 84}, "Exploration": M{"Systems_Visited": 1684, "Total_Hyperspace_Distance": 142680.5, "Planets_Scanned_To_Level_3": 2371, "Highest_Payout": 68300000}, "Trading": M{"Markets_Traded_With": 186, "Market_Profits": 1526000000, "Resources_Traded": 42860}, "Mining": M{"Mining_Profits": 634000000, "Quantity_Mined": 18420}},
		{"event": "StoredShips", "StarSystem": "Demo Fleet System", "StationName": "Demo Shipyard", "MarketID": "424242001", "ShipsHere": []M{{"ShipID": 42, "ShipType": "python", "Name": "Synthetic Miner", "Value": 126000000, "Hot": false}}, "ShipsRemote": []M{{"ShipID": 43, "ShipType": "anaconda", "Name": "Synthetic Explorer", "StarSystem": "Demo Expedition Base", "ShipMarketID": "424242002", "Value": 342000000, "Hot": false, "TransferPrice": 1240000, "TransferTime": 4200}, {"ShipID": 44, "ShipType": "krait_mkii", "Name": "Synthetic Patrol", "StarSystem": "Demo Combat Base", "ShipMarketID": "424242003", "Value": 186000000, "Hot": false, "TransferPrice": 680000, "TransferTime": 1800}}},
		{"event": "StoredModules", "Items": []M{{"StorageSlot": 1, "Name": "int_hyperdrive_size6_class5", "Name_Localised": "6A Frame Shift Drive", "StarSystem": "Demo Fleet System", "MarketID": "424242001", "BuyPrice": 16179953, "Hot": false}, {"StorageSlot": 2, "Name": "int_fuelscoop_size5_class5", "Name_Localised": "5A Fuel Scoop", "StarSystem": "Demo Expedition Base", "MarketID": "424242002", "BuyPrice": 9071680, "Hot": false}}},
		{"event": "Reputation", "Federation": 86.5, "Empire": 100, "Alliance": 42.8, "Independent": 0},
		{"event": "Materials", "Raw": []M{{"Name": "carbon", "Count": 168}, {"Name": "iron", "Count": 212}, {"Name": "phosphorus", "Count": 85}}, "Manufactured": []M{{"Name": "Chemical Processors", "Count": 46}, {"Name": "Mechanical Components", "Count": 73}}, "Encoded": []M{{"Name": "Datamined Wake Exceptions", "Count": 18}, {"Name": "Classified Scan Databanks", "Count": 31}}},
		{"event": "ShipLocker", "Items": []M{{"Name": "geneticrepairmeds", "Name_Localised": "Genetic Repair Meds", "Count": 18}}, "Components": []M{{"Name": "graphene", "Name_Localised": "Graphene", "Count": 24}, {"Name": "aerogel", "Name_Localised": "Aerogel", "Count": 17}}, "Consumables": []M{{"Name": "healthpack", "Name_Localised": "Medkit", "Count": 48}, {"Name": "powerbattery", "Name_Localised": "Energy Cell", "Count": 36}, {"Name": "amm_grenade_emp", "Name_Localised": "Shield Disruptor", "Count": 12}}, "Data": []M{{"Name": "manufacturinginstructions", "Name_Localised": "Manufacturing Instructions", "Count": 21}}},
		{"event": "Backpack", "Items": []M{{"Name": "geneticrepairmeds", "Name_Localised": "Genetic Repair Meds", "Count": 1}}, "Components": []M{{"Name": "graphene", "Name_Localised": "Graphene", "Count": 3}}, "Consumables": []M{{"Name": "healthpack", "Name_Localised": "Medkit", "Count": 2}, {"Name": "powerbattery", "Name_Localised": "Energy Cell", "Count": 1}}, "Data": []M{{"Name": "manufacturinginstructions", "Name_Localised": "Manufacturing Instructions", "Count": 1}}},
	} {
		event["timestamp"] = at
		e.Apply(event, M{"replay": true, "side": true})
	}
	meta := func(extra M) M {
		return v.Merge(M{"source": "synthetic", "known": true, "observedAt": at, "fetchedAt": at}, extra)
	}
	service := politics.New(M{
		"selectedSystem": "Demo Political System",
		"publicGoals":    meta(M{"entries": []M{{"id": "900001", "title": "Synthetic relief campaign", "system": "Demo Political System", "station": "Demo Relief Port", "objective": "Deliver basic medicines", "currentTotal": 125000, "target": 500000, "contributors": 812, "expiry": v.ISO(now.Add(72 * time.Hour)), "description": "Fictional campaign for interface testing. Deliver medical supplies to the designated port.", "reward": "Fictional participation reward", "complete": false}}}),
		"powerIndex":     meta(M{"powers": []M{{"name": "Li Yong-Rui", "indexedSystems": 250}, {"name": "Aisling Duval", "indexedSystems": 180}}, "states": []M{{"name": "Stronghold", "indexedSystems": 12}, {"name": "Fortified", "indexedSystems": 30}, {"name": "Contested", "indexedSystems": 2}}}),
	}, politics.Options{DisableNetwork: true})
	service.Apply(M{"event": "LoadGame", "timestamp": at, "FID": "FNATIVE-SYNTHETIC"})
	service.Apply(M{"event": "Location", "timestamp": at, "StarSystem": "Demo Political System", "SystemAddress": "42424242424242", "SystemAllegiance": "Independent", "SystemGovernment": "Corporate", "SystemSecurity": "High", "Population": 4500000, "SystemFaction": M{"Name": "Demo Cooperative", "FactionState": "Boom"},
		"Factions":         []M{{"Name": "Demo Cooperative", "Allegiance": "Independent", "Government": "Cooperative", "Influence": .55, "FactionState": "Boom", "ActiveStates": []M{{"State": "Boom"}}}, {"Name": "Demo Industries", "Allegiance": "Federation", "Government": "Corporate", "Influence": .30, "FactionState": "War", "ActiveStates": []M{{"State": "War"}}}, {"Name": "Demo Union", "Allegiance": "Independent", "Government": "Democracy", "Influence": .15, "FactionState": "War", "ActiveStates": []M{{"State": "War"}}, "PendingStates": []M{{"State": "Expansion"}}}},
		"Conflicts":        []M{{"WarType": "war", "Status": "active", "Faction1": M{"Name": "Demo Industries", "Stake": "Demo Industrial Port", "WonDays": 2}, "Faction2": M{"Name": "Demo Union", "Stake": "Demo Union Port", "WonDays": 1}}},
		"ControllingPower": "Li Yong-Rui", "PowerplayState": "Fortified", "Powers": []string{"Li Yong-Rui"}, "PowerplayStateControlProgress": .67, "PowerplayStateReinforcement": 12300, "PowerplayStateUndermining": 1700,
		"ThargoidWar": M{"CurrentState": "ThargoidRecovery", "WarProgress": .62, "RemainingPorts": 3, "NextStateSuccess": "None", "NextStateFailure": "ThargoidRecovery"}})
	service.Apply(M{"event": "CommunityGoal", "timestamp": at, "CurrentGoals": []M{{"CGID": "900001", "Title": "Synthetic relief campaign", "SystemName": "Demo Political System", "MarketName": "Demo Relief Port", "CurrentTotal": 125000, "PlayerContribution": 128, "NumContributors": 812, "TierReached": "Tier 2", "PlayerPercentileBand": 50, "Bonus": 1500000, "Expiry": v.ISO(now.Add(72 * time.Hour)), "IsComplete": false}}})
	e.domainMu.Lock()
	e.Politics = service
	e.domainMu.Unlock()
	e.Notify()
	return nil
}
