package politics

import (
	v "github.com/Naenier/ED-Copilot/internal/value"
	"sort"
	"strings"
)

func canonicalEnum(raw, localized any) string {
	value := clean(v.First(raw, localized), 100)
	if strings.HasPrefix(value, "$") && strings.HasSuffix(value, ";") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "$"), ";")
		for _, prefix := range []string{"government_", "system_security_", "galaxy_map_info_state_"} {
			if strings.HasPrefix(strings.ToLower(value), prefix) {
				value = value[len(prefix):]
				break
			}
		}
	}
	for _, name := range []string{"Anarchy", "Confederacy", "Cooperative", "Corporate", "Communism", "Democracy", "Dictatorship", "Feudal", "Patronage", "PrisonColony", "Theocracy", "High", "Medium", "Low", "Lawless"} {
		if strings.EqualFold(value, name) {
			return name
		}
	}
	return value // New provider/game enum values remain visible.
}

func states(raw any) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, item := range v.Arr(raw) {
		name := clean(item, 100)
		if row, ok := item.(M); ok {
			name = clean(v.First(row["State"], row["state"]), 100)
		}
		if name != "" && !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
		if len(out) >= 32 {
			break
		}
	}
	return out
}
func factions(raw any, journal bool) []M {
	out := []M{}
	for _, r := range boundedRows(raw, 64) {
		row := M{}
		if journal {
			row = M{"name": clean(r["Name"], 200), "allegiance": clean(r["Allegiance"], 100), "government": clean(r["Government"], 100), "influence": fraction(r["Influence"]), "state": clean(r["FactionState"], 100), "activeStates": states(r["ActiveStates"]), "pendingStates": states(r["PendingStates"]), "recoveringStates": states(v.First(r["RecoveringStates"], r["RecovingStates"]))}
		} else {
			row = M{"name": clean(r["name"], 200), "allegiance": clean(r["allegiance"], 100), "government": clean(r["government"], 100), "influence": fraction(r["influence"]), "state": clean(r["state"], 100), "activeStates": states(r["active_states"]), "pendingStates": states(r["pending_states"]), "recoveringStates": states(r["recovering_states"])}
		}
		if row["name"] != "" {
			out = append(out, row)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, ak := v.Number(out[i]["influence"])
		b, bk := v.Number(out[j]["influence"])
		if ak != bk {
			return ak
		}
		if a == b {
			return v.Str(out[i]["name"]) < v.Str(out[j]["name"])
		}
		return a > b
	})
	return out
}

func (s *Service) Apply(event M) {
	typ := v.Str(event["event"])
	at := date(event["timestamp"])
	s.mu.Lock()
	defer s.mu.Unlock()
	if typ == "LoadGame" {
		fid := clean(event["FID"], 100)
		if s.commander != "" && fid != "" && s.commander != fid {
			s.personalGoals = M{}
			s.pledge = M{}
		}
		s.commander = fid
		return
	}
	if typ == "CommunityGoal" {
		entries := []M{}
		for _, r := range boundedRows(event["CurrentGoals"], 100) {
			if id := clean(r["CGID"], 100); id != "" {
				entries = append(entries, M{"id": id, "title": clean(r["Title"], 500), "system": clean(r["SystemName"], 200), "station": clean(r["MarketName"], 200), "expiry": date(r["Expiry"]), "complete": r["IsComplete"], "currentTotal": number(r["CurrentTotal"]), "playerContribution": number(r["PlayerContribution"]), "contributors": number(r["NumContributors"]), "tier": clean(r["TierReached"], 100), "percentileBand": number(r["PlayerPercentileBand"]), "bonus": number(r["Bonus"]), "topRankSize": number(r["TopRankSize"]), "playerInTopRank": r["PlayerInTopRank"], "joined": true, "observedAt": at})
			}
		}
		if !v.Time(s.personalGoals["observedAt"]).After(v.Time(at)) {
			s.personalGoals = meta("journal", at, "", "")
			s.personalGoals["entries"] = entries
		}
		return
	}
	if typ == "CommunityGoalDiscard" || typ == "CommunityGoalReward" || typ == "CommunityGoalJoin" {
		for _, r := range v.Rows(s.personalGoals["entries"]) {
			if v.Str(r["id"]) == v.Str(event["CGID"]) {
				previousAt := v.First(r["observedAt"], s.personalGoals["observedAt"])
				if v.Time(previousAt).After(v.Time(at)) {
					continue
				}
				if typ == "CommunityGoalDiscard" {
					r["joined"] = false
					r["discarded"] = true
				}
				if typ == "CommunityGoalReward" {
					r["rewardReceived"] = number(event["Reward"])
					r["complete"] = true
				}
				if typ == "CommunityGoalJoin" {
					r["joined"], r["discarded"] = true, false
				}
				r["observedAt"] = at
				if !v.Time(s.personalGoals["observedAt"]).After(v.Time(at)) {
					s.personalGoals["observedAt"] = at
				}
			}
		}
		return
	}
	if typ == "Powerplay" || typ == "PowerplayJoin" || typ == "PowerplayLeave" || typ == "PowerplayDefect" {
		if v.Time(s.pledge["observedAt"]).After(v.Time(at)) {
			return
		}
		s.pledge = meta("journal", at, "", "")
		s.pledge["power"] = clean(v.First(event["Power"], event["ToPower"]), 100)
		if typ == "PowerplayLeave" {
			s.pledge["power"] = ""
			s.pledge["pledged"] = false
		} else {
			s.pledge["pledged"] = s.pledge["power"] != ""
		}
		s.pledge["merits"] = number(event["Merits"])
		s.pledge["rank"] = number(event["Rank"])
		return
	}
	if typ != "FSDJump" && typ != "Location" && typ != "CarrierJump" {
		return
	}
	name := clean(event["StarSystem"], 200)
	if name == "" {
		return
	}
	if s.selected == "" && key(s.current) != key(name) {
		s.selectionID++
	}
	s.current = name
	row := M{"name": name, "address": clean(event["SystemAddress"], 100)}
	info := meta("journal", at, "", "")
	for target, source := range map[string]string{"allegiance": "SystemAllegiance", "government": "SystemGovernment", "security": "SystemSecurity", "population": "Population"} {
		if raw, ok := event[source]; ok {
			if target == "population" {
				info[target] = number(raw)
			} else {
				info[target] = canonicalEnum(raw, event[source+"_Localised"])
			}
		}
	}
	if raw := v.Obj(event["SystemFaction"]); len(raw) > 0 {
		info["controllingFaction"] = clean(raw["Name"], 200)
		info["state"] = clean(raw["FactionState"], 100)
	}
	if len(info) > 5 {
		row["info"] = info
	}
	if raw, ok := event["Factions"]; ok {
		d := meta("journal", at, "", "")
		d["entries"] = factions(raw, true)
		row["factions"] = d
	}
	if raw, ok := event["Conflicts"]; ok {
		d := meta("journal", at, "", "")
		entries := []M{}
		for _, r := range boundedRows(raw, 32) {
			side := func(raw any) M {
				f := v.Obj(raw)
				return M{"name": clean(f["Name"], 200), "stake": clean(f["Stake"], 200), "wonDays": number(f["WonDays"])}
			}
			entries = append(entries, M{"type": clean(r["WarType"], 100), "status": clean(r["Status"], 100), "faction1": side(r["Faction1"]), "faction2": side(r["Faction2"])})
		}
		d["entries"] = entries
		row["conflicts"] = d
	}
	if _, ok := event["PowerplayState"]; ok {
		d := meta("journal", at, "", "")
		d["state"] = clean(event["PowerplayState"], 100)
		d["powers"] = states(event["Powers"])
		d["controllingPower"] = clean(event["ControllingPower"], 100)
		d["controlProgress"] = fraction(event["PowerplayStateControlProgress"])
		d["reinforcement"] = number(event["PowerplayStateReinforcement"])
		d["undermining"] = number(event["PowerplayStateUndermining"])
		// Multiple Powers do not by themselves establish a contested system.
		row["powerplay"] = d
	}
	if raw := v.Obj(event["ThargoidWar"]); len(raw) > 0 {
		d := meta("journal", at, "", "")
		d["state"] = clean(raw["CurrentState"], 100)
		d["progress"] = fraction(raw["WarProgress"])
		d["nextStateSuccess"] = clean(raw["NextStateSuccess"], 100)
		d["nextStateFailure"] = clean(raw["NextStateFailure"], 100)
		d["remainingPorts"] = number(raw["RemainingPorts"])
		d["estimatedRemainingTime"] = clean(raw["EstimatedRemainingTime"], 200)
		d["successStateReached"] = raw["SuccessStateReached"]
		row["thargoid"] = d
	}
	s.mergeLocked(row)
}

// NormalizePublicSystem deliberately ignores bodies, stations and unrelated
// fields from the public system response. Exact conflict pairings are unknown
// unless the journal actually reports Conflicts; faction War is not a pairing.
func NormalizePublicSystem(raw M, fetched string) M {
	name := clean(raw["name"], 200)
	if name == "" {
		return nil
	}
	at := date(raw["updated_at"])
	url := "https://spansh.co.uk/system/" + v.Str(raw["id64"])
	row := M{"name": name, "address": clean(raw["id64"], 100)}
	info := meta("spansh", at, fetched, url)
	for _, k := range []string{"allegiance", "government", "security"} {
		info[k] = clean(raw[k], 100)
	}
	info["population"] = number(raw["population"])
	info["economy"] = clean(raw["primary_economy"], 100)
	info["secondEconomy"] = clean(raw["secondary_economy"], 100)
	info["controllingFaction"] = clean(raw["controlling_minor_faction"], 200)
	info["state"] = clean(raw["controlling_minor_faction_state"], 100)
	row["info"] = info
	if f := raw["minor_faction_presences"]; v.Arr(f) != nil {
		d := meta("spansh", at, fetched, url)
		d["entries"] = factions(f, false)
		row["factions"] = d
	}
	if raw["power_state"] != nil || raw["controlling_power"] != nil {
		d := meta("spansh", at, fetched, url)
		d["state"] = clean(raw["power_state"], 100)
		d["controllingPower"] = clean(raw["controlling_power"], 100)
		d["powers"] = states(raw["power"])
		d["controlProgress"] = fraction(raw["power_state_control_progress"])
		d["reinforcement"] = number(raw["power_state_reinforcement"])
		d["undermining"] = number(raw["power_state_undermining"])
		row["powerplay"] = d
	}
	if raw["thargoid_war_state"] != nil {
		d := meta("spansh", at, fetched, url)
		d["state"] = clean(raw["thargoid_war_state"], 100)
		d["progress"] = fraction(raw["thargoid_war_progress"])
		d["nextStateSuccess"] = clean(raw["thargoid_war_success_state"], 100)
		d["nextStateFailure"] = clean(raw["thargoid_war_failure_state"], 100)
		row["thargoid"] = d
	}
	return row
}

func goalText(raw any, limit int) string {
	// The official bulletin is plain text, rendered escaped. Template markers
	// for website leaderboards are not useful in a local read-only goal card.
	return strings.ReplaceAll(clean(raw, limit), "{{top5}}", "")
}
