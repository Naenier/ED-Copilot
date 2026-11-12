package colonization

import (
	"github.com/Naenier/ED-Copilot/internal/value"
)

func (p *Planner) Apply(event, context M) {
	p.mu.Lock()
	defer p.mu.Unlock()
	typ := str(event["event"])
	if typ == "LoadGame" && str(event["FID"]) != "" {
		fid := str(event["FID"])
		if bound := str(p.config["squadronWarehouseAccessFID"]); bound != "" && bound != fid {
			p.clearSquadronAccess()
		}
		if p.carrierOwner != "" && p.carrierOwner != fid {
			p.carrierStocks, p.carrierOwner = M{}, ""
		}
		if p.commander != "" && p.commander != fid {
			p.resetLocked()
		}
		p.commander = fid
		p.ensureSelectedLocked()
	}
	p.observeJournalSystem(event, context)
	if typ == "Scan" || typ == "SAASignalsFound" {
		p.observeDevelopmentBody(event, context)
		return
	}
	if !has([]string{"ColonisationConstructionDepot", "ColonisationContribution", "ColonisationSystemClaim", "ColonisationSystemClaimRelease", "ColonisationBeaconDeployed", "Docked", "Location", "CargoTransfer", "MarketBuy", "MarketSell"}, typ) {
		return
	}
	var identity string
	_, fileKnown := context["file"]
	_, offsetKnown := context["offset"]
	if fileKnown && offsetKnown {
		identity = str(context["file"]) + ":" + str(context["offset"])
		if p.seen[identity] {
			return
		}
		p.seen[identity] = true
	}
	p.sequence++
	sequence := p.sequence
	timestamp := validDate(event["timestamp"])
	if timestamp == nil {
		timestamp = value.ISO(p.now())
	}
	commander := value.Obj(context["commander"])
	system := text(firstText(event["StarSystem"], commander["system"], commander["starSystem"]), 200)
	station := text(firstText(event["StationName"], commander["station"]), 200)
	if typ == "CargoTransfer" || typ == "MarketBuy" || typ == "MarketSell" {
		p.applyCarrierMovement(event, context)
		return
	}
	if typ == "Docked" || typ == "Location" {
		id := marketID(event["MarketID"])
		if id != "" && station != "" {
			observedAt := any(nil)
			if value.Bool(value.At(context, "game", "live")) && !value.Bool(context["side"]) && !value.Bool(value.At(context, "game", "multicrew")) {
				observedAt = validDate(event["timestamp"])
			}
			localized := text(firstText(event["StationName_Localised"], event["StationName_Localized"]), 200)
			if localized == "" && p.stations[id]["name"] == station {
				localized = str(p.stations[id]["displayName"])
			}
			observed := M{"marketId": id, "name": station, "displayName": localized, "system": system, "systemAddress": marketID(firstText(event["SystemAddress"], commander["systemAddress"])), "faction": text(value.Obj(event["StationFaction"])["Name"], 200), "type": text(event["StationType"], 200), "services": stringsOf(event["StationServices"]), "hasColonisationContact": colonisationContact(event, event["StationServices"]), "timestamp": timestamp, "visited": typ == "Docked" || value.Bool(event["Docked"])}
			p.stations[id] = developmentStationSnapshot(observed, p.stations[id], event, value.Time(timestamp))
			if typ == "Docked" || value.Bool(event["Docked"]) {
				p.stations[id]["historyAt"] = observedAt
				p.discoverStationLocked(p.stations[id])
			}
		}
		return
	}
	if typ == "ColonisationConstructionDepot" {
		id := marketID(event["MarketID"])
		if id == "" || event["ResourcesRequired"] == nil {
			return
		}
		station = text(firstText(event["StationName"], p.stations[id]["name"], station), 200)
		stationDisplayName := text(firstText(event["StationName_Localised"], event["StationName_Localized"], p.stations[id]["displayName"]), 200)
		rs := []M{}
		for _, r := range limitedRows(event["ResourcesRequired"], 500) {
			key := value.Key(r["Name"])
			required, provided := natural(r["RequiredAmount"], 1e9), natural(r["ProvidedAmount"], 1e9)
			if key == "" || required == nil || provided == nil {
				continue
			}
			rs = append(rs, M{"commodity": key, "name": commodityName(key, str(r["Name_Localised"])), "required": required, "delivered": provided, "payment": natural(r["Payment"], 1e9), "missing": max0(num(required) - num(provided)), "source": "journal", "authoritative": true})
		}
		progress, known := value.Number(event["ConstructionProgress"])
		p.depots[id] = M{"marketId": id, "system": system, "systemAddress": marketID(firstText(event["SystemAddress"], commander["systemAddress"])), "station": station, "stationType": text(firstText(event["StationType"], p.stations[id]["type"]), 200), "stationDisplayName": stationDisplayName, "timestamp": timestamp, "sequence": sequence, "constructionProgress": value.Nullable(value.Clamp(progress, 0, 1), known), "complete": value.Bool(event["ConstructionComplete"]), "failed": value.Bool(event["ConstructionFailed"]), "requirements": rs}
		if value.Bool(value.At(context, "game", "live")) && !value.Bool(context["side"]) && !value.Bool(value.At(context, "game", "multicrew")) {
			p.depots[id]["historyAt"] = validDate(event["timestamp"])
		}
		p.discoverDepotLocked(p.depots[id])
		p.reconcileSupplyReservations(context, id, value.Time(timestamp), nil)
		return
	}
	if typ == "ColonisationContribution" {
		id := marketID(event["MarketID"])
		if id == "" {
			return
		}
		rs := []M{}
		for _, r := range limitedRows(event["Contributions"], 500) {
			key := value.Key(r["Name"])
			count := natural(r["Amount"], 1e9)
			if key != "" && num(count) > 0 {
				rs = append(rs, M{"commodity": key, "name": commodityName(key, str(r["Name_Localised"])), "count": count})
			}
		}
		if len(rs) == 0 {
			return
		}
		idEvent := value.ID()
		if identity != "" {
			idEvent = eventID(context["file"], context["offset"])
		}
		p.contributions = append(p.contributions, M{"id": idEvent, "marketId": id, "system": system, "timestamp": timestamp, "sequence": sequence, "resources": rs, "source": "journal"})
		p.reconcileSupplyReservations(context, id, value.Time(timestamp), rs)
		return
	}
	target := text(firstText(event["SystemName"], event["StarSystem"], system), 200)
	address := marketID(event["SystemAddress"])
	if address == "" && typ == "ColonisationBeaconDeployed" {
		address = marketID(commander["systemAddress"])
	}
	key := address
	if key == "" {
		key = stringsLower(target)
	}
	if key == "" {
		return
	}
	record := copyM(p.claims[key])
	record = merge(record, M{"system": target, "systemAddress": address, "timestamp": timestamp, "sequence": sequence})
	switch typ {
	case "ColonisationSystemClaim":
		record["claimedAt"], record["beaconAt"], record["releasedAt"], record["status"] = timestamp, nil, nil, "claimed"
		if origin := text(commander["system"], 200); origin != "" && !equal(origin, target) {
			record["originSystem"] = origin
		}
	case "ColonisationBeaconDeployed":
		record["beaconAt"], record["status"] = timestamp, "beacon"
	case "ColonisationSystemClaimRelease":
		record["releasedAt"], record["status"] = timestamp, "released"
	}
	p.claims[key] = record
	p.discoverClaimLocked(record, typ)
}
