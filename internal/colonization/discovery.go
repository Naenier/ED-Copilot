package colonization

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/value"
)

// A local claim is evidence of ownership. Visiting a depot, contributing cargo
// or seeing a beacon is not: those activities are possible in another colony.
func cleanJournalOwnership(raw any) any {
	m := value.Obj(raw)
	if m["source"] != "journal-claim" || validDate(m["claimedAt"]) == nil {
		return nil
	}
	status := str(m["status"])
	if !has([]string{"claimed", "beacon", "released"}, status) {
		return nil
	}
	owner := str(m["ownerKey"])
	if decoded, err := hex.DecodeString(owner); owner != "" && (err != nil || len(decoded) != sha256.Size) {
		return nil
	}
	return M{"source": "journal-claim", "automatic": value.Bool(m["automatic"]), "ownerKey": owner, "claimedAt": validDate(m["claimedAt"]), "beaconAt": validDate(m["beaconAt"]), "releasedAt": validDate(m["releasedAt"]), "timestamp": validDate(m["timestamp"]), "status": status}
}

func ownerKey(fid string) string {
	if fid == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(fid))
	return hex.EncodeToString(hash[:])
}

func (p *Planner) projectVisible(project M) bool {
	ownership := value.Obj(project["journalOwnership"])
	return !value.Bool(ownership["automatic"]) || str(ownership["ownerKey"]) == "" || p.commander == "" || ownership["ownerKey"] == ownerKey(p.commander)
}

func sameColony(project M, system, address string) bool {
	knownAddress := str(project["systemAddress"])
	if knownAddress != "" && address != "" {
		return knownAddress == address
	}
	return system != "" && equal(project["targetSystem"], system)
}

func (p *Planner) observedProject(system, address string) M {
	for _, project := range p.projects {
		if p.projectVisible(project) && sameColony(project, system, address) {
			return project
		}
	}
	return nil
}

func (p *Planner) ensureSelectedLocked() {
	if p.projectLocked(p.selected) != nil {
		return
	}
	p.selected = ""
	for _, project := range p.projects {
		if p.projectVisible(project) {
			p.selected = str(project["id"])
			return
		}
	}
}

func (p *Planner) discoverClaimLocked(record M, typ string) {
	system, address := str(record["system"]), str(record["systemAddress"])
	project := p.observedProject(system, address)
	if project == nil {
		if typ != "ColonisationSystemClaim" || system == "" || len(p.projects) >= 100 {
			return
		}
		key := firstText(address, strings.ToLower(system))
		hash := sha256.Sum256([]byte(ownerKey(p.commander) + ":" + key))
		project = cleanProject(M{"id": "journal-" + hex.EncodeToString(hash[:12]), "name": system, "targetSystem": system, "originSystem": record["originSystem"], "systemAddress": address, "status": "claimed", "claimedAt": record["claimedAt"]})
		project["journalOwnership"] = M{"source": "journal-claim", "automatic": true, "ownerKey": ownerKey(p.commander), "claimedAt": record["claimedAt"], "timestamp": record["timestamp"], "status": "claimed"}
		p.projects = append(p.projects, project)
	}
	ownership := value.Obj(project["journalOwnership"])
	if ownership["source"] != "journal-claim" {
		if typ != "ColonisationSystemClaim" {
			return
		}
		ownership = M{"source": "journal-claim", "automatic": false, "ownerKey": ownerKey(p.commander), "claimedAt": record["claimedAt"]}
		project["journalOwnership"] = ownership
	}
	// A saved release is newer than the claim replayed during startup. Likewise
	// a completed manual or observed plan must not be downgraded by old records.
	rank := map[string]int{"claimed": 1, "beacon": 2, "released": 3}
	previousAt, observedAt := value.Time(ownership["timestamp"]), value.Time(record["timestamp"])
	if previousAt.After(observedAt) || previousAt.Equal(observedAt) && rank[str(ownership["status"])] > rank[str(record["status"])] {
		p.ensureSelectedLocked()
		return
	}
	previousStatus := str(ownership["status"])
	ownership["timestamp"], ownership["status"] = record["timestamp"], record["status"]
	switch typ {
	case "ColonisationSystemClaim":
		ownership["claimedAt"], ownership["beaconAt"], ownership["releasedAt"] = record["claimedAt"], nil, nil
		if project["claimedAt"] == nil {
			project["claimedAt"] = record["claimedAt"]
		}
		if value.Bool(ownership["automatic"]) && project["status"] != "complete" && (project["status"] != "cancelled" || previousStatus == "released") {
			project["status"] = "claimed"
		}
	case "ColonisationBeaconDeployed":
		ownership["beaconAt"] = record["beaconAt"]
		if value.Bool(ownership["automatic"]) && project["status"] != "complete" && project["status"] != "cancelled" {
			project["status"] = "building"
		}
	case "ColonisationSystemClaimRelease":
		ownership["releasedAt"] = record["releasedAt"]
		if value.Bool(ownership["automatic"]) && project["status"] != "complete" {
			project["status"] = "cancelled"
		}
	}
	if str(project["systemAddress"]) == "" && address != "" {
		project["systemAddress"] = address
	}
	p.ensureSelectedLocked()
	if ownership["status"] != "released" {
		depots := []M{}
		for _, depot := range p.depots {
			depots = append(depots, depot)
		}
		sort.Slice(depots, func(i, j int) bool { return num(depots[i]["sequence"]) < num(depots[j]["sequence"]) })
		for _, depot := range depots {
			p.discoverDepotLocked(depot)
		}
		stations := []M{}
		for _, station := range p.stations {
			stations = append(stations, station)
		}
		sort.Slice(stations, func(i, j int) bool {
			if stations[i]["timestamp"] == stations[j]["timestamp"] {
				return str(stations[i]["marketId"]) < str(stations[j]["marketId"])
			}
			return str(stations[i]["timestamp"]) < str(stations[j]["timestamp"])
		})
		for _, station := range stations {
			if station["visited"] == true {
				p.discoverStationLocked(station)
			}
		}
	}
}

func (p *Planner) discoverDepotLocked(depot M) {
	if IsColonisationShip(depot) {
		for _, project := range p.projects {
			pruneShipBuildings(project, str(depot["marketId"]))
		}
		return
	}
	project := p.observedProject(str(depot["system"]), str(depot["systemAddress"]))
	if project == nil {
		return
	}
	ownership := value.Obj(project["journalOwnership"])
	if ownership["source"] != "journal-claim" || ownership["status"] == "released" {
		return
	}
	// Old snapshots from a previous claim must not become a new claim's bill.
	if value.Time(depot["timestamp"]).Before(value.Time(ownership["claimedAt"])) {
		return
	}
	id := str(depot["marketId"])
	if id == "" {
		return
	}
	var linked M
	for _, other := range p.projects {
		for _, candidate := range rows(other["buildings"]) {
			if candidate["marketId"] == id {
				if other["id"] != project["id"] {
					return // An explicit link in another plan is never reassigned.
				}
				linked = candidate
			}
		}
	}
	if linked == nil {
		bs := rows(project["buildings"])
		if len(bs) >= 200 {
			return
		}
		hash := sha256.Sum256([]byte(str(project["id"]) + ":" + id))
		linked = cleanBuilding(M{"id": "journal-" + hex.EncodeToString(hash[:12]), "typeId": "custom", "name": firstText(depot["station"], "Стройплощадка из журнала"), "marketId": id, "status": "building", "journalDiscovered": true})
		project["buildings"] = append(bs, linked)
		if order := stringsOf(project["supplyOrder"]); len(order) > 0 {
			project["supplyOrder"] = append(order, str(linked["id"]))
		}
	}
	p.recordConstructionMilestone(project, depot)
	if value.Time(linked["journalUpdatedAt"]).After(value.Time(depot["timestamp"])) {
		return
	}
	if str(linked["journalStationName"]) == "" {
		linked["journalStationName"] = firstText(depot["station"], linked["name"])
	}
	if typ := str(depot["stationType"]); typ != "" {
		linked["journalStationType"] = typ
	}
	if depot["source"] == "station" {
		linked["journalSource"] = "station"
	} else if str(linked["journalSource"]) == "" {
		linked["journalSource"] = "depot"
	}
	if value.Bool(linked["journalDiscovered"]) {
		label := str(depot["stationDisplayName"])
		if label == "" && depot["station"] != linked["journalStationName"] {
			label = str(depot["station"])
		}
		if label != "" {
			linked["journalDisplayName"] = label
		}
	}
	status := "building"
	if value.Bool(depot["failed"]) {
		status = "failed"
	} else if value.Bool(depot["complete"]) {
		status = "complete"
	}
	linked["journalStatus"], linked["journalUpdatedAt"] = status, depot["timestamp"]
	if value.Bool(linked["journalDiscovered"]) && status != "failed" {
		linked["status"] = status
	}
}
