package colonization

import (
	"strings"

	"github.com/Naenier/ED-Copilot/internal/value"
)

// IsColonisationShip recognizes the claim's supply vessel, not a building.
// SurfaceStation alone is insufficient: the game uses it for ordinary ports too.
func IsColonisationShip(row M) bool {
	for _, key := range []string{"name", "station", "journalStationName"} {
		name := strings.ToLower(strings.TrimSpace(str(row[key])))
		for _, prefix := range []string{"$ext_panel_colonisationship;", "$ext_panel_colonizationship;", "colonisation ship:", "colonization ship:"} {
			if strings.HasPrefix(name, prefix) {
				return true
			}
		}
	}
	return has([]string{"colonisationship", "colonizationship", "systemcolonisationship"}, value.Key(firstText(row["type"], row["stationType"], row["journalStationType"])))
}

// IsPermanentStation requires an observed station type. A name, purchase,
// contribution or bare CompleteConstruction event cannot establish a facility.
func IsPermanentStation(row M) bool {
	if IsColonisationShip(row) {
		return false
	}
	typ := value.Key(firstText(row["type"], row["stationType"], row["journalStationType"]))
	if !has([]string{"coriolis", "orbis", "ocellus", "outpost", "surfacestation", "craterport", "crateroutpost", "dodec", "dodecstarport"}, typ) {
		return false
	}
	for _, key := range []string{"name", "station", "journalStationName"} {
		name := strings.ToLower(str(row[key]))
		if strings.HasPrefix(name, "planetary construction site:") || strings.HasPrefix(name, "orbital construction site:") || strings.HasPrefix(name, "surface construction site:") {
			return false
		}
	}
	for _, service := range stringsOf(row["services"]) {
		if value.Key(service) == "colonisationcontribution" || value.Key(service) == "colonizationcontribution" {
			return false
		}
	}
	return true
}

func pruneShipBuildings(project M, market string) {
	kept, removed := []M{}, map[string]bool{}
	for _, building := range rows(project["buildings"]) {
		if IsColonisationShip(building) || market != "" && str(building["marketId"]) == market {
			removed[str(building["id"])] = true
		} else {
			kept = append(kept, building)
		}
	}
	if len(removed) == 0 {
		return
	}
	project["buildings"] = kept
	filter := func(raw any) []string {
		out := []string{}
		for _, id := range stringsOf(raw) {
			if !removed[id] {
				out = append(out, id)
			}
		}
		return out
	}
	project["supplyOrder"] = filter(project["supplyOrder"])
	for _, building := range kept {
		building["dependsOn"] = filter(building["dependsOn"])
	}
}

func (p *Planner) discoverStationLocked(station M) {
	if station["visited"] != true || !IsPermanentStation(station) {
		return
	}
	address := str(station["systemAddress"])
	project := p.observedProject(str(station["system"]), address)
	// Unlike an explicit own claim, merely docking is not ownership evidence.
	// Match the confirmed claim by address, not an ambiguous system name.
	if project == nil || address == "" || str(project["systemAddress"]) != address {
		return
	}
	p.discoverDepotLocked(M{"marketId": station["marketId"], "system": station["system"], "systemAddress": address, "station": station["name"], "stationType": station["type"], "stationDisplayName": station["displayName"], "timestamp": station["timestamp"], "complete": true, "source": "station"})
}
