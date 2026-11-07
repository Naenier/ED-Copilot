// Package inara maps supported personal-account observations and persists a
// commander-bound, opt-in queue. Historical journal replay is never exported.
package inara

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/Naenier/ED-Copilot/internal/ships"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"sort"
	"strings"
	"time"
)

const Endpoint = "https://inara.cz/inapi/v1/"
const MaxAge = 30 * 24 * time.Hour

type Event struct {
	Name      string `json:"eventName"`
	Timestamp string `json:"eventTimestamp"`
	Data      any    `json:"eventData"`
}
type Live struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason"`
}

var categories = []string{"raw", "manufactured", "encoded"}
var ranks = map[string]string{"Combat": "combat", "Trade": "trade", "Explore": "explore", "CQC": "cqc", "Soldier": "soldier", "Exobiologist": "exobiologist", "Federation": "federation", "Empire": "empire"}
var Supported = map[string]bool{}

func init() {
	for _, name := range strings.Fields("setCommanderCredits setCommanderGameStatistics setCommanderRankEngineer setCommanderRankPilot setCommanderRankPower setCommanderReputationMajorFaction setCommanderReputationMinorFaction setCommanderInventoryCargo setCommanderInventoryMaterials resetCommanderInventory setCommanderInventory setCommanderStorageModules setCommanderShip setCommanderShipLoadout addCommanderShip delCommanderShip setCommanderShipTransfer setCommanderSuitLoadout updateCommanderSuitLoadout delCommanderSuitLoadout addCommanderTravelCarrierJump addCommanderTravelDock addCommanderTravelFSDJump addCommanderTravelLand setCommanderTravelLocation addCommanderMission setCommanderMissionAbandoned setCommanderMissionCompleted setCommanderMissionFailed addCommanderPermit") {
		Supported[name] = true
	}
}
func number(raw any) (float64, bool) {
	if _, ok := raw.(string); ok {
		return 0, false
	}
	return v.Number(raw)
}
func integer(raw any) (float64, bool) {
	n, ok := number(raw)
	return n, ok && n >= 0 && n <= 9007199254740991 && math.Trunc(n) == n
}
func str(raw any) string {
	s, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}
func validTime(raw string, now time.Time) bool {
	at := v.Time(raw)
	return !at.IsZero() && !at.After(now.Add(5*time.Minute)) && !at.Before(now.Add(-MaxAge))
}
func ValidateLive(meta v.M) Live {
	version := strings.ToLower(v.Text(meta["gameVersion"], meta["gameversion"], meta["version"]))
	if v.Bool(meta["isBeta"]) || v.Bool(meta["beta"]) || strings.Contains(version, "beta") || strings.Contains(version, "alpha") || strings.Contains(version, "test") {
		return Live{false, "Inara не принимает данные бета-версий Elite Dangerous."}
	}
	if v.Bool(meta["legacy"]) || v.Bool(meta["isLegacy"]) || meta["live"] == false || strings.Contains(version, "legacy") || strings.HasPrefix(version, "3.") {
		return Live{false, "Inara принимает только Live-галактику: Odyssey / Horizons 4.0."}
	}
	if v.Bool(meta["multicrew"]) || v.Bool(meta["inMulticrew"]) {
		return Live{false, "Синхронизация личного имущества недоступна в чужом экипаже."}
	}
	major := strings.SplitN(version, ".", 2)[0]
	if v.Bool(meta["live"]) || v.Num(major) >= 4 {
		return Live{true, ""}
	}
	return Live{false, "Нужен журнал сеанса Live-версии игры; версия галактики пока неизвестна."}
}
func mapped(source v.M, fields map[string]string) v.M {
	out := v.M{}
	for target, key := range fields {
		raw := source[key]
		_, boolean := raw.(bool)
		_, numeric := number(raw)
		if str(raw) != "" || boolean || numeric {
			out[target] = raw
		}
	}
	return out
}
func canonical(raw any) []byte {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(raw) != nil {
		return nil
	}
	result := bytes.TrimSpace(b.Bytes())
	result = bytes.ReplaceAll(result, []byte(`\u2028`), []byte("\u2028"))
	return bytes.ReplaceAll(result, []byte(`\u2029`), []byte("\u2029"))
}
func EventKey(event Event, fid string) string {
	body := canonical(v.M{"commander": fid, "eventName": event.Name, "eventTimestamp": event.Timestamp, "eventData": event.Data})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func SnapshotKey(event Event) string {
	switch event.Name {
	case "setCommanderInventoryCargo", "setCommanderInventoryMaterials", "setCommanderCredits", "setCommanderGameStatistics", "setCommanderStorageModules", "setCommanderTravelLocation":
		return event.Name
	case "setCommanderShipLoadout":
		data := v.Obj(event.Data)
		return event.Name + ":" + v.String(data, "shipType") + ":" + v.Str(data["shipGameID"])
	}
	return ""
}
func inventory(raw any) ([]v.M, bool) {
	if raw == nil {
		return nil, false
	}
	rows := v.Arr(raw)
	if rows == nil {
		return nil, false
	}
	out := []v.M{}
	for _, item := range rows {
		row := v.Obj(item)
		name := str(row["Name"])
		count, ok := integer(row["Count"])
		if name == "" || !ok {
			return nil, false
		}
		data := v.M{"itemName": name, "itemCount": count}
		if mission, valid := integer(row["MissionID"]); valid {
			data["missionGameID"] = mission
		}
		if stolen, valid := integer(row["Stolen"]); valid && stolen > 0 {
			if stolen > count {
				return nil, false
			}
			copy := v.Copy(data)
			copy["itemCount"] = stolen
			copy["isStolen"] = true
			out = append(out, copy)
			if count > stolen {
				data["itemCount"] = count - stolen
				data["isStolen"] = false
				out = append(out, data)
			}
		} else {
			out = append(out, data)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i]["itemName"] == out[j]["itemName"] {
			return v.Num(out[i]["missionGameID"]) < v.Num(out[j]["missionGameID"])
		}
		return v.String(out[i], "itemName") < v.String(out[j], "itemName")
	})
	return out, true
}
func engineer(row v.M) v.M {
	data := v.M{"engineerName": v.Text(row["Engineer"], row["english"])}
	stage := v.Text(row["Progress"], row["journalStatus"])
	if stage == "Invited" || stage == "Acquainted" || stage == "Unlocked" || stage == "Barred" {
		data["rankStage"] = stage
	}
	if rank, ok := integer(v.First(row["Rank"], row["rank"])); ok && rank >= 1 && rank <= 5 {
		data["rankValue"] = rank
	}
	if data["engineerName"] == "" || len(data) == 1 {
		return nil
	}
	return data
}
func engineering(raw any) v.M {
	source := v.Obj(raw)
	data := mapped(source, map[string]string{"blueprintName": "BlueprintName", "blueprintLevel": "Level", "blueprintQuality": "Quality", "experimentalEffect": "ExperimentalEffect"})
	if rows := v.Arr(source["Modifiers"]); rows != nil {
		mods := []v.M{}
		for _, item := range rows {
			row := v.Obj(item)
			if str(row["Label"]) == "" {
				continue
			}
			valueField := "Value"
			if _, ok := number(row["Value"]); !ok {
				valueField = "ValueStr"
			}
			mods = append(mods, v.Merge(v.M{"name": row["Label"]}, mapped(row, map[string]string{"value": valueField, "originalValue": "OriginalValue", "lessIsGood": "LessIsGood"})))
		}
		data["modifiers"] = mods
	}
	if len(data) == 0 {
		return nil
	}
	return data
}
func shipID(event, context v.M) v.M {
	raw := v.Text(event["Ship"], event["ShipType"], context["shipType"])
	id, ok := integer(v.First(event["ShipID"], context["shipId"]))
	if raw == "" || !ok || ships.VehicleKind(raw) != "ship" || strings.Contains(strings.ToLower(raw), "fighter") {
		return nil
	}
	return v.M{"shipType": raw, "shipGameID": id}
}
func modules(raw any) ([]v.M, bool) {
	if raw == nil {
		return nil, false
	}
	rows := v.Arr(raw)
	if rows == nil {
		return nil, false
	}
	out := []v.M{}
	for _, item := range rows {
		row := v.Obj(item)
		if str(row["Slot"]) == "" || str(row["Item"]) == "" {
			return nil, false
		}
		data := v.Merge(v.M{"slotName": row["Slot"], "itemName": row["Item"]}, mapped(row, map[string]string{"itemHealth": "Health", "isOn": "On", "isHot": "Hot", "itemPriority": "Priority", "itemAmmoClip": "AmmoInClip", "itemAmmoHopper": "AmmoInHopper", "itemValue": "Value"}))
		if mods := engineering(row["Engineering"]); mods != nil {
			data["engineering"] = mods
		}
		out = append(out, data)
	}
	return out, true
}
func location(event, commander v.M) v.M {
	system := v.Text(event["StarSystem"], event["System"], commander["system"])
	if system == "" {
		return nil
	}
	data := v.M{"starsystemName": system}
	coords := v.Arr(v.First(event["StarPos"], commander["coordinates"]))
	if len(coords) == 3 {
		valid := true
		for _, x := range coords {
			if _, ok := number(x); !ok {
				valid = false
			}
		}
		if valid {
			data["starsystemCoords"] = coords
		}
	}
	return data
}
func BuildSnapshotEvents(state, context v.M, now time.Time) []Event {
	if !ValidateLive(context).OK {
		return nil
	}
	out := []Event{}
	add := func(name, timestamp string, data any) {
		if validTime(timestamp, now) {
			out = append(out, Event{name, timestamp, data})
		}
	}
	cmdr, pilot, stocks := v.Obj(state["commander"]), v.Obj(state["pilot"]), v.Obj(state["engineering"])
	at := v.Text(context["cargoForSyncAt"], cmdr["cargoForSyncAt"])
	if cmdr["vehicle"] == "ship" || v.Bool(context["cargoIsShip"]) {
		raw := cmdr["cargoForSync"]
		if _, ok := raw.(map[string]any); ok {
			raw = v.Obj(raw)["Inventory"]
		}
		if data, ok := inventory(raw); ok {
			add("setCommanderInventoryCargo", at, data)
		}
	}
	known := v.Obj(v.At(stocks, "shopping", "knownCategories"))
	allKnown := true
	times := []time.Time{}
	for _, category := range categories {
		at := v.Time(v.At(stocks, "shopping", "updatedAt", category))
		if !v.Bool(known[category]) || !validTime(v.ISO(at), now) {
			allKnown = false
		}
		times = append(times, at)
	}
	materials := v.Rows(stocks["materials"])
	if allKnown && len(materials) > 0 {
		rows := []v.M{}
		for _, row := range materials {
			if _, ok := integer(row["owned"]); !ok {
				allKnown = false
			}
			rows = append(rows, v.M{"Name": v.Text(row["symbol"], row["key"]), "Count": row["owned"]})
		}
		if data, ok := inventory(rows); ok && allKnown {
			latest := times[0]
			for _, t := range times {
				if latest.Before(t) {
					latest = t
				}
			}
			add("setCommanderInventoryMaterials", v.ISO(latest), data)
		}
	}
	for _, row := range v.Rows(stocks["engineers"]) {
		if data := engineer(row); data != nil {
			add("setCommanderRankEngineer", v.String(row, "updatedAt"), data)
		}
	}
	credits, hasCredits := integer(pilot["credits"])
	loan, hasLoan := integer(v.First(context["loan"], pilot["loan"]))
	if hasCredits && hasLoan {
		add("setCommanderCredits", v.String(pilot, "creditsAt"), v.M{"commanderCredits": credits, "commanderLoan": loan})
	}
	if current := location(nil, cmdr); current != nil {
		if v.Bool(cmdr["docked"]) && str(cmdr["station"]) != "" {
			current["stationName"] = cmdr["station"]
			if n, ok := integer(cmdr["marketId"]); ok {
				current["marketID"] = n
			}
		}
		add("setCommanderTravelLocation", v.Text(context["locationUpdatedAt"], cmdr["locationUpdatedAt"]), current)
	}
	return out
}
func contains(values []string, value string) bool {
	for _, x := range values {
		if x == value {
			return true
		}
	}
	return false
}
func JournalEvents(event, context v.M, now time.Time) []Event {
	if event == nil || str(event["event"]) == "" || v.Bool(context["replay"]) || v.Bool(context["historical"]) || !ValidateLive(context).OK || !validTime(v.String(event, "timestamp"), now) {
		return nil
	}
	out := []Event{}
	add := func(name string, data any) { out = append(out, Event{name, v.String(event, "timestamp"), data}) }
	typ := v.String(event, "event")
	cmdr := v.Obj(context["commander"])
	if len(cmdr) == 0 {
		cmdr = v.Obj(v.At(context, "state", "commander"))
	}
	switch typ {
	case "LoadGame":
		if n, ok := integer(event["Credits"]); ok {
			data := v.M{"commanderCredits": n}
			if loan, ok := integer(event["Loan"]); ok {
				data["commanderLoan"] = loan
			}
			add("setCommanderCredits", data)
		}
	case "Statistics":
		data := v.Copy(event)
		delete(data, "event")
		delete(data, "timestamp")
		if len(data) > 0 {
			add("setCommanderGameStatistics", data)
		}
	case "Rank", "Promotion", "Progress":
		rows := []v.M{}
		fields := []string{"Combat", "Trade", "Explore", "CQC", "Soldier", "Exobiologist", "Federation", "Empire"}
		for _, field := range fields {
			n, ok := number(event[field])
			if !ok {
				continue
			}
			if typ == "Progress" && n >= 0 && n <= 100 {
				rows = append(rows, v.M{"rankName": ranks[field], "rankProgress": n / 100})
			} else if typ != "Progress" && n >= 0 && n <= 14 && math.Trunc(n) == n {
				rows = append(rows, v.M{"rankName": ranks[field], "rankValue": n})
			}
		}
		if len(rows) > 0 {
			add("setCommanderRankPilot", rows)
		}
	case "EngineerProgress":
		rows := v.Rows(event["Engineers"])
		if rows == nil {
			rows = []v.M{event}
		}
		data := []v.M{}
		for _, row := range rows {
			if item := engineer(row); item != nil {
				data = append(data, item)
			}
		}
		if len(data) > 0 {
			add("setCommanderRankEngineer", data)
		}
	case "Reputation":
		rows := []v.M{}
		for _, key := range []string{"Empire", "Federation", "Alliance", "Independent"} {
			if n, ok := number(event[key]); ok && math.Abs(n) <= 100 {
				rows = append(rows, v.M{"majorfactionName": strings.ToLower(key), "majorfactionReputation": n / 100})
			}
		}
		if len(rows) > 0 {
			add("setCommanderReputationMajorFaction", rows)
		}
	case "Powerplay", "PowerplayRank", "PowerplayJoin", "PowerplayDefect", "PowerplayLeave":
		name := str(event["Power"])
		rank, ok := integer(event["Rank"])
		if typ == "PowerplayDefect" {
			name = str(event["ToPower"])
		}
		if typ == "PowerplayLeave" {
			rank = -1
			ok = true
		} else if typ == "PowerplayJoin" || typ == "PowerplayDefect" {
			rank = 1
			ok = true
		}
		if name != "" && ok {
			data := v.M{"powerName": name, "rankValue": rank}
			if n, ok := integer(event["Merits"]); ok {
				data["meritsValue"] = n
			}
			add("setCommanderRankPower", data)
		}
	case "Cargo":
		if event["Vessel"] == nil || strings.EqualFold(str(event["Vessel"]), "ship") {
			if data, ok := inventory(event["Inventory"]); ok {
				add("setCommanderInventoryCargo", data)
			}
		}
	case "Materials":
		rows := []any{}
		complete := true
		for _, key := range []string{"Raw", "Manufactured", "Encoded"} {
			if arr := v.Arr(event[key]); arr == nil {
				complete = false
			} else {
				rows = append(rows, arr...)
			}
		}
		if complete {
			if data, ok := inventory(rows); ok {
				add("setCommanderInventoryMaterials", data)
			}
		}
	case "ShipLocker", "ShipLockerMaterials":
		rows, empty := []v.M{}, []v.M{}
		complete := true
		for _, group := range []string{"Items", "Components", "Data", "Consumables"} {
			inv, ok := inventory(event[group])
			if !ok {
				complete = false
				break
			}
			if len(inv) == 0 {
				empty = append(empty, v.M{"itemType": group, "itemLocation": "ShipLocker"})
			}
			for _, row := range inv {
				row["itemType"] = group
				row["itemLocation"] = "ShipLocker"
				rows = append(rows, row)
			}
		}
		if complete {
			if len(empty) > 0 {
				add("resetCommanderInventory", empty)
			}
			if len(rows) > 0 {
				add("setCommanderInventory", rows)
			}
		}
	case "Loadout", "SetUserShipName":
		if id := shipID(event, context); id != nil {
			fields := map[string]string{"shipName": "ShipName", "shipIdent": "ShipIdent", "isHot": "Hot", "shipHullValue": "HullValue", "shipModulesValue": "ModulesValue", "shipRebuyCost": "Rebuy", "shipMaxJumpRange": "MaxJumpRange", "shipCargoCapacity": "CargoCapacity"}
			if typ == "SetUserShipName" {
				fields["shipName"] = "UserShipName"
				fields["shipIdent"] = "UserShipId"
			}
			data := v.Merge(v.Copy(id), mapped(event, fields))
			if typ == "Loadout" {
				data["isCurrentShip"] = true
			}
			add("setCommanderShip", data)
			if loadout, ok := modules(event["Modules"]); ok && typ == "Loadout" {
				add("setCommanderShipLoadout", v.Merge(v.Copy(id), v.M{"shipLoadout": loadout}))
			}
		}
	case "ShipyardBuy", "ShipyardSwap":
		if n, ok := integer(event["SellShipID"]); ok && str(event["SellOldShip"]) != "" {
			add("delCommanderShip", v.M{"shipType": event["SellOldShip"], "shipGameID": n})
		}
		if n, ok := integer(event["StoreShipID"]); ok && str(event["StoreOldShip"]) != "" && str(cmdr["system"]) != "" && str(cmdr["station"]) != "" {
			data := v.M{"shipType": event["StoreOldShip"], "shipGameID": n, "starsystemName": cmdr["system"], "stationName": cmdr["station"]}
			if m, ok := integer(cmdr["marketId"]); ok {
				data["marketID"] = m
			}
			add("setCommanderShip", data)
		}
		if typ == "ShipyardSwap" {
			if id := shipID(event, context); id != nil {
				id["isCurrentShip"] = true
				add("setCommanderShip", id)
			}
		}
	case "ShipyardNew", "ShipyardSell", "SellShipOnRebuy":
		next := v.Copy(event)
		if typ == "ShipyardNew" {
			next["ShipID"] = v.First(event["NewShipID"], event["ShipID"])
		} else {
			next["ShipID"] = v.First(event["SellShipID"], event["ShipID"])
		}
		if id := shipID(next, context); id != nil {
			if typ == "ShipyardNew" {
				add("addCommanderShip", id)
			} else {
				add("delCommanderShip", id)
			}
		}
	case "ShipyardTransfer":
		if id := shipID(event, context); id != nil && str(cmdr["system"]) != "" && str(cmdr["station"]) != "" {
			id["starsystemName"] = cmdr["system"]
			id["stationName"] = cmdr["station"]
			v.Merge(id, mapped(event, map[string]string{"transferTime": "TransferTime"}))
			if n, ok := integer(cmdr["marketId"]); ok {
				id["marketID"] = n
			}
			add("setCommanderShipTransfer", id)
		}
	case "StoredShips":
		if event["ShipsHere"] != nil && event["ShipsRemote"] != nil {
			for _, group := range []string{"ShipsHere", "ShipsRemote"} {
				for _, row := range v.Rows(event[group]) {
					id := shipID(row, nil)
					if id == nil {
						continue
					}
					v.Merge(id, mapped(row, map[string]string{"shipName": "Name", "isHot": "Hot", "starsystemName": "StarSystem", "marketID": "ShipMarketID"}))
					if group == "ShipsHere" {
						if s := v.Text(event["StarSystem"], cmdr["system"]); s != "" {
							id["starsystemName"] = s
						}
						if s := v.Text(event["StationName"], cmdr["station"]); s != "" {
							id["stationName"] = s
						}
						if n, ok := integer(event["MarketID"]); ok {
							id["marketID"] = n
						}
					}
					add("setCommanderShip", id)
				}
			}
		}
	case "StoredModules":
		if event["Items"] != nil {
			items := []v.M{}
			complete := true
			for _, row := range v.Rows(event["Items"]) {
				if str(row["Name"]) == "" {
					complete = false
					break
				}
				data := v.Merge(v.M{"itemName": row["Name"]}, mapped(row, map[string]string{"itemValue": "BuyPrice", "isHot": "Hot", "starsystemName": "StarSystem", "marketID": "MarketID"}))
				raw := row["Engineering"]
				if _, ok := raw.(string); ok {
					raw = v.M{"BlueprintName": raw, "Level": row["Level"], "Quality": row["Quality"]}
				}
				if mods := engineering(raw); mods != nil {
					data["engineering"] = mods
				}
				items = append(items, data)
			}
			if complete {
				add("setCommanderStorageModules", items)
			}
		}
	case "SuitLoadout", "CreateSuitLoadout":
		loadID, lok := integer(event["LoadoutID"])
		suitID, sok := integer(event["SuitID"])
		if lok && sok && str(event["SuitName"]) != "" && event["Modules"] != nil && event["SuitMods"] != nil {
			items := []v.M{}
			valid := true
			for _, row := range v.Rows(event["Modules"]) {
				id, iok := integer(row["SuitModuleID"])
				class, cok := integer(row["Class"])
				if !iok || !cok || str(row["SlotName"]) == "" || str(row["ModuleName"]) == "" {
					valid = false
					break
				}
				mods := []v.M{}
				for _, mod := range v.Arr(row["WeaponMods"]) {
					if str(mod) != "" {
						mods = append(mods, v.M{"blueprintName": mod})
					}
				}
				items = append(items, v.M{"slotName": row["SlotName"], "itemName": row["ModuleName"], "itemClass": class, "itemGameID": id, "engineering": mods})
			}
			suitMods := []string{}
			for _, mod := range v.Arr(event["SuitMods"]) {
				if str(mod) != "" {
					suitMods = append(suitMods, str(mod))
				}
			}
			if valid {
				add("setCommanderSuitLoadout", v.M{"loadoutGameID": loadID, "loadoutName": v.String(event, "LoadoutName"), "suitGameID": suitID, "suitType": event["SuitName"], "suitMods": suitMods, "suitLoadout": items})
			}
		}
	case "DeleteSuitLoadout", "RenameSuitLoadout":
		if id, ok := integer(event["LoadoutID"]); ok {
			data := v.M{"loadoutGameID": id}
			if typ == "DeleteSuitLoadout" {
				add("delCommanderSuitLoadout", data)
			} else if str(event["LoadoutName"]) != "" {
				data["loadoutName"] = event["LoadoutName"]
				add("updateCommanderSuitLoadout", data)
			}
		}
	case "MissionAccepted":
		if id, ok := integer(event["MissionID"]); ok && str(event["Name"]) != "" {
			data := v.Merge(v.M{"missionGameID": id, "missionName": event["Name"]}, mapped(event, map[string]string{"missionExpiry": "Expiry", "influenceGain": "Influence", "reputationGain": "Reputation", "minorfactionNameOrigin": "Faction", "starsystemNameTarget": "DestinationSystem", "stationNameTarget": "DestinationStation", "minorfactionNameTarget": "TargetFaction", "commodityName": "Commodity", "commodityCount": "Count", "targetName": "Target", "targetType": "TargetType", "killCount": "KillCount", "passengerType": "PassengerType", "passengerCount": "PassengerCount", "passengerIsVIP": "PassengerVIPs", "passengerIsWanted": "PassengerWanted"}))
			if str(cmdr["system"]) != "" {
				data["starsystemNameOrigin"] = cmdr["system"]
			}
			if str(cmdr["station"]) != "" {
				data["stationNameOrigin"] = cmdr["station"]
			}
			add("addCommanderMission", data)
		}
	case "MissionCompleted", "MissionFailed", "MissionAbandoned":
		if id, ok := integer(event["MissionID"]); ok {
			data := v.M{"missionGameID": id}
			if typ == "MissionCompleted" {
				v.Merge(data, mapped(event, map[string]string{"donationCredits": "Donation", "rewardCredits": "Reward"}))
				if event["PermitsAwarded"] != nil {
					permits := []v.M{}
					for _, name := range v.Arr(event["PermitsAwarded"]) {
						if str(name) != "" {
							permit := v.M{"starsystemName": name}
							permits = append(permits, permit)
							add("addCommanderPermit", permit)
						}
					}
					data["rewardPermits"] = permits
				}
				for target, field := range map[string]string{"rewardCommodities": "CommodityReward", "rewardMaterials": "MaterialsReward"} {
					if rows, ok := inventory(event[field]); ok {
						data[target] = rows
					}
				}
				if event["FactionEffects"] != nil {
					effects := []v.M{}
					for _, row := range v.Rows(event["FactionEffects"]) {
						if str(row["Faction"]) == "" {
							continue
						}
						effect := v.Merge(v.M{"minorfactionName": row["Faction"]}, mapped(row, map[string]string{"reputationGain": "Reputation"}))
						longest := ""
						for _, gain := range v.Rows(row["Influence"]) {
							s := str(gain["Influence"])
							if strings.Trim(s, "+") == "" && len(s) > len(longest) {
								longest = s
							}
						}
						if longest != "" {
							effect["influenceGain"] = longest
						}
						effects = append(effects, effect)
					}
					data["minorfactionEffects"] = effects
				}
			}
			names := map[string]string{"MissionCompleted": "setCommanderMissionCompleted", "MissionFailed": "setCommanderMissionFailed", "MissionAbandoned": "setCommanderMissionAbandoned"}
			add(names[typ], data)
		}
	}
	if contains([]string{"Location", "FSDJump", "CarrierJump"}, typ) {
		rows := []v.M{}
		for _, row := range v.Rows(event["Factions"]) {
			if n, ok := number(row["MyReputation"]); ok && math.Abs(n) <= 100 && str(row["Name"]) != "" {
				rows = append(rows, v.M{"minorfactionName": row["Name"], "minorfactionReputation": n / 100})
			}
		}
		if len(rows) > 0 {
			add("setCommanderReputationMinorFaction", rows)
		}
	}
	if contains([]string{"Location", "FSDJump", "CarrierJump", "Docked", "Touchdown", "ApproachSettlement", "SupercruiseExit"}, typ) {
		if data := location(event, cmdr); data != nil {
			apiName := "setCommanderTravelLocation"
			if _, ok := number(event["JumpDist"]); ok && typ == "FSDJump" {
				apiName = "addCommanderTravelFSDJump"
				data["jumpDistance"] = event["JumpDist"]
			}
			if typ == "CarrierJump" {
				apiName = "addCommanderTravelCarrierJump"
			}
			if typ == "Docked" && !v.Bool(context["suppressDock"]) {
				apiName = "addCommanderTravelDock"
			}
			if typ == "Touchdown" && v.Bool(event["PlayerControlled"]) && v.Text(event["Body"], event["BodyName"]) != "" {
				apiName = "addCommanderTravelLand"
			}
			if typ == "Docked" || typ == "CarrierJump" || typ == "Location" && v.Bool(event["Docked"]) || typ == "ApproachSettlement" {
				name := str(event["StationName"])
				if typ == "ApproachSettlement" {
					name = str(event["Name"])
				}
				if name != "" {
					data["stationName"] = name
				}
				if n, ok := integer(event["MarketID"]); ok {
					data["marketID"] = n
				}
			}
			if event["BodyType"] == "Planet" || typ == "Touchdown" || typ == "ApproachSettlement" {
				if body := v.Text(event["Body"], event["BodyName"]); body != "" {
					data["starsystemBodyName"] = body
				}
				lat, latOK := number(event["Latitude"])
				lon, lonOK := number(event["Longitude"])
				if latOK && lonOK {
					data["starsystemBodyCoords"] = []float64{lat, lon}
				}
			}
			if strings.HasPrefix(apiName, "addCommanderTravel") {
				if v.Bool(event["Taxi"]) || v.Bool(context["taxi"]) {
					if v.Bool(event["Dropship"]) || v.Bool(context["dropship"]) {
						data["isTaxiDropship"] = true
					} else {
						data["isTaxiShuttle"] = true
					}
				} else {
					v.Merge(data, shipID(nil, context))
				}
			}
			if typ != "Touchdown" || v.Bool(event["PlayerControlled"]) {
				add(apiName, data)
			}
		}
	}
	return out
}
