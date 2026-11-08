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
