package colonization

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/value"
)

// Facility indices from DaftMav's public in-game measurements (Stats sheet)
// and the Silverback facility score measurements. See docs/colonization-development.md.
//
//go:embed development_catalog.json
var developmentCatalogJSON []byte

var developmentKeys = [...]string{"initialPopulation", "maxPopulation", "security", "technology", "wealth", "livingStandards", "development", "architectScore"}

type developmentType struct {
	Values      []int  `json:"values"`
	Pad         string `json:"pad"`
	Economy     string `json:"economy"`
	LinkEconomy string `json:"linkEconomy"`
	Tier2       int    `json:"t2"`
	Tier3       int    `json:"t3"`
}

var developmentTypes = func() map[string]developmentType {
	var types map[string]developmentType
	if err := json.Unmarshal(developmentCatalogJSON, &types); err != nil {
		panic(err)
	}
	for id, item := range types {
		if len(item.Values) != len(developmentKeys) || item.Pad == "" {
			panic("invalid colonization impact data for " + id)
		}
	}
	return types
}()

var settlementFamilies = []struct{ id, title string }{
	{"agriculture", "сельское хозяйство"}, {"extraction", "добыча"}, {"industrial", "промышленность"},
	{"military", "военное"}, {"hightech", "биологические исследования"}, {"tourism", "туризм"},
}

func enrichDevelopmentCatalog(original []M) []M {
	all := make([]M, 0, len(original)+18)
	for _, entry := range original {
		id := str(entry["id"])
		if meta, ok := developmentTypes[id]; ok {
			if entry["tier"] == nil {
				tier := 1
				if meta.Tier2 < 0 {
					tier = 2
				}
				if meta.Tier3 < 0 {
					tier = 3
				}
				entry["tier"], entry["pointReward"] = tier, max(meta.Tier2, meta.Tier3, 0)
				if tier != 3 {
					entry["pointCost"] = max(0, -meta.Tier2)
				}
			}
			if meta.LinkEconomy != "" && entry["kind"] != "port" {
				entry["economy"] = meta.LinkEconomy
			}
			if id == "dodec" {
				entry["name"] = "Додекаэдр"
				delete(entry, "note")
			}
			if entry["kind"] == "installation" {
				delete(entry, "note")
			}
			entry["prerequisites"] = typePrerequisites(id)
		}
		all = append(all, entry)
		if entry["kind"] != "settlement" {
			continue
		}
		for _, family := range settlementFamilies {
			id := id + "-" + family.id
			meta := developmentTypes[id]
			item := copyM(entry)
			item["id"], item["name"], item["economy"] = id, str(entry["name"])+" · "+family.title, family.id
			item["pointCost"], item["pointReward"] = max(0, -meta.Tier2), max(meta.Tier2, meta.Tier3, 0)
			item["prerequisites"] = typePrerequisites(id)
			if meta.Tier2 < 0 {
				// Research and tourism settlements can have a T1-sized layout
				// while consuming T2 points and granting T3 points.
				item["costTier"], item["rewardTier"] = 2, 3
			}
			delete(item, "note")
			all = append(all, item)
		}
	}
	return all
}

// Resolve unambiguous game station types without replacing a manually chosen
// catalogue item. Generic Outpost and CraterOutpost reveal no specialization.
func buildingCatalogType(building M) M {
	id := str(building["typeId"])
	if id == "custom" && building["journalSource"] == "station" {
		switch value.Key(building["journalStationType"]) {
		case "dodec", "dodecstarport", "dodecahedron":
			id = "dodec"
		case "coriolis":
			id = "coriolis"
		case "orbis":
			id = "orbis"
		case "ocellus":
			id = "ocellus"
		case "asteroidbase":
			id = "asteroid-base"
		}
	}
	return typesByID[id]
}

func stationEconomies(raw any) []M {
	result := []M{}
	for _, row := range limitedRows(raw, 20) {
		proportion, known := value.Number(row["Proportion"])
		name := text(firstText(row["Name_Localised"], row["Name"]), 100)
		if known && proportion >= 0 && proportion <= 1 && name != "" {
			result = append(result, M{"name": name, "fraction": proportion})
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return num(result[i]["fraction"]) > num(result[j]["fraction"]) })
	return result
}

var requirementsForType = map[string][]string{
	"installation-research": {"settlement-hightech"}, "installation-military": {"settlement-military"},
	"installation-security": {"installation-relay"}, "installation-tourist": {"settlement-tourism"},
	"hub-extraction": {"settlement-extraction"}, "hub-civilian": {"settlement-agriculture"},
	"hub-exploration": {"installation-communications"}, "hub-outpost": {"installation-farm"},
	"hub-military": {"installation-military"}, "hub-industrial": {"installation-mining"},
}

func typePrerequisites(id string) []string {
	if strings.HasPrefix(id, "settlement-") && strings.HasSuffix(id, "-tourism") {
		return []string{"installation-satellite"}
	}
	return requirementsForType[id]
}

// Frontier's service rules depend on both facilities and how their ports link.
// These are potential unlocks, listed on the contributing building.
var unlocksForType = map[string][]string{
	"dodec":                       {"human-broker"},
	"installation-satellite":      {"tourism-settlements", "survey-linked"},
	"installation-communications": {"exploration-hub", "survey-linked", "market-linked"},
	"installation-relay":          {"security-installation", "survey-linked", "market-linked"},
	"installation-farm":           {"outpost-hub"}, "installation-mining": {"industrial-hub"},
	"installation-pirate": {"black-market-linked"}, "installation-bar": {"lounge-system", "market-system"},
	"installation-military": {"military-hub", "shipyard-system", "outfitting-system"},
	"installation-medical":  {"vista-system"}, "installation-research": {"cartographics-system"},
	"installation-tourist": {"market-system"}, "hub-outpost": {"market-system"},
	"hub-exploration": {"cartographics-system"}, "hub-scientific": {"vista-system"},
	"hub-hightech":   {"outfitting-system", "shipyard-linked"},
	"hub-industrial": {"shipyard-system", "outfitting-linked"},
}

func typeUnlocks(id string, building M) []string {
	out := append([]string{}, unlocksForType[id]...)
	if building["kind"] == "port" {
		out = append(out, "pioneer-supplies")
		if num(building["tier"]) >= 2 {
			out = append(out, "market-port", "technology-floor")
		}
	} else if building["kind"] == "settlement" {
		out = append(out, "market-port")
		for _, pair := range [][2]string{{"-military", "military-installation"}, {"-hightech", "research-installation"}, {"-tourism", "tourist-installation"}} {
			if strings.HasSuffix(id, pair[0]) {
				out = append(out, pair[1])
			}
		}
	}
	return out
}

var primaryWeight = [8]int{100, 100, 140, 120, 140, 140, 140, 100}
var secondaryWeight = [8]int{100, 100, 90, 75, 75, 80, 90, 100}

func developmentImpact(building M, primaryKnown bool, station M) M {
	id := firstText(building["catalogTypeId"], building["typeId"])
	meta, ok := developmentTypes[id]
	if !ok && id == "custom" && len(station) == 0 {
		return nil
	}
	out := M{"typeId": id, "known": ok, "prerequisites": typePrerequisites(id), "unlocks": typeUnlocks(id, building)}
	if ok {
		metrics := M{}
		for index, key := range developmentKeys {
			base := meta.Values[index]
			var weighted any
			switch {
			case value.Bool(building["isPrimary"]):
				weighted = float64(base*primaryWeight[index]) / 100
			case primaryKnown || !value.Bool(typesByID[id]["canBePrimary"]) || primaryWeight[index] == secondaryWeight[index] || base == 0:
				weighted = float64(base*secondaryWeight[index]) / 100
			}
			metrics[key] = M{"base": base, "weighted": weighted}
		}
		out["metrics"], out["pad"], out["economy"], out["linkEconomy"] = metrics, meta.Pad, meta.Economy, meta.LinkEconomy
	}
	if len(station) > 0 && station["visited"] == true && IsPermanentStation(station) {
		out["observedEconomy"], out["observedEconomies"] = station["economy"], value.Clone(station["economies"])
		out["observedServices"], out["observedAt"] = append([]string(nil), stringsOf(station["services"])...), station["timestamp"]
	}
	return out
}

func developmentBodyKey(address, system, body string) string {
	if address == "" {
		address = strings.ToLower(strings.Join(strings.Fields(system), " "))
	}
	return address + "\x00" + strings.ToLower(strings.Join(strings.Fields(body), " "))
}

func (p *Planner) observeDevelopmentBody(event, context M) {
	if value.Bool(context["side"]) || value.Bool(value.At(context, "game", "multicrew")) {
		return
	}
	commander := value.Obj(context["commander"])
	body := text(event["BodyName"], 200)
	address := marketID(firstText(event["SystemAddress"], commander["systemAddress"]))
	system := text(firstText(event["StarSystem"], commander["system"]), 200)
	at := value.Time(event["timestamp"])
	if body == "" || at.IsZero() || address == "" && system == "" {
		return
	}
	// Store a small cache only for systems already tracked as colonies.
	tracked := false
	for _, project := range p.projects {
		projectAddress := str(project["systemAddress"])
		if p.projectVisible(project) && (address != "" && projectAddress == address || projectAddress == "" && system != "" && equal(project["targetSystem"], system)) {
			tracked = true
			break
		}
	}
	if !tracked {
		return
	}
	key := developmentBodyKey(address, system, body)
	entry := p.developmentBodies[key]
	if entry == nil {
		if len(p.developmentBodies) >= 4096 {
			return
		}
		entry = M{}
		p.developmentBodies[key] = entry
	}
	if event["event"] == "SAASignalsFound" {
		if at.Before(value.Time(entry["signalAt"])) {
			return
		}
		for _, signal := range limitedRows(event["Signals"], 12) {
			kind := value.Key(signal["Type"])
			if kind == "saasignaltypebiological" || kind == "saasignaltypegeological" {
				entry[kind] = num(signal["Count"]) > 0
			}
		}
		entry["signalAt"] = value.ISO(at)
		return
	}
	if at.Before(value.Time(entry["scanAt"])) {
		return
	}
	for _, field := range []string{"PlanetClass", "StarType", "TerraformState", "Volcanism", "TidalLock", "ReserveLevel"} {
		if raw, known := event[field]; known {
			entry[field] = raw
		}
	}
	if _, known := event["Rings"]; known {
		entry["rings"] = len(rows(event["Rings"])) > 0
	}
	entry["scanAt"] = value.ISO(at)
}

func bodyEconomyOverrides(body M) []string {
	var result []string
	add := func(economies ...string) {
		for _, economy := range economies {
			if !has(result, economy) {
				result = append(result, economy)
			}
		}
	}
	planet := value.Key(body["PlanetClass"])
	star := str(body["StarType"])
	switch {
	case star == "H" || star == "N" || strings.HasPrefix(star, "D"):
		add("hightech", "tourism")
	case star != "":
		add("military")
	case strings.HasPrefix(planet, "earthlike"):
		add("agriculture", "hightech", "military", "tourism")
	case planet == "waterworld":
		add("agriculture", "tourism")
	case planet == "ammoniaworld":
		add("hightech", "tourism")
	case strings.Contains(planet, "gasgiant") || strings.HasSuffix(planet, "lifegiant"):
		add("hightech", "industrial")
	case strings.Contains(planet, "highmetalcontent") || strings.Contains(planet, "metalrich"):
		add("extraction")
	case planet == "rockyicebody":
		add("industrial", "refinery")
	case planet == "rockybody":
		add("refinery")
	case planet == "icybody":
		add("industrial")
	}
	if body["rings"] == true {
		add("extraction")
	}
	if body["saasignaltypebiological"] == true {
		add("agriculture", "terraforming")
	}
	if body["saasignaltypegeological"] == true {
		add("extraction", "industrial")
	}
	return result
}

// Reuse only observations matched to this colony and body. Published
// measurements cover a few configurations; leave all others unestimated.
func (p *Planner) addDevelopmentBody(impact, project, building M) {
	address, system, name := str(project["systemAddress"]), str(project["targetSystem"]), str(building["body"])
	if name == "" {
		return
	}
	body := p.developmentBodies[developmentBodyKey(address, system, name)]
	if body == nil && system != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(system)+" ") {
		body = p.developmentBodies[developmentBodyKey(address, system, system+" "+name)]
	}
	if body == nil {
		return
	}
	impact["bodyType"] = firstText(body["PlanetClass"], body["StarType"])
	impact["bodyScanAt"] = body["scanAt"]
	if impact["economy"] == "colony" {
		impact["bodyEconomies"] = bodyEconomyOverrides(body)
	}
	planet, kind := value.Key(body["PlanetClass"]), str(impact["typeId"])
	switch {
	case strings.HasPrefix(planet, "earthlike") && has([]string{"orbis", "ocellus"}, kind):
		impact["populationCapacityExample"] = 3100000000
	case planet == "waterworld" && has([]string{"orbis", "ocellus"}, kind):
		impact["populationCapacityExample"] = 1825000000
	case strings.HasPrefix(planet, "earthlike") && kind == "coriolis":
		impact["populationCapacityExample"] = 100000000
	case planet == "highmetalcontentbody" && strings.HasPrefix(kind, "surface-outpost-"):
		impact["populationCapacityExample"] = 48000000
	}
}

// Impact totals track the known contribution of the structures in this plan.
// A separate count makes partial totals explicit rather than treating an
// unidentified facility as a zero contribution.
func (p *Planner) developmentProjection(project M, buildings []M, includeDetails bool) M {
	primaryKnown := false
	for _, b := range buildings {
		if b["isPrimary"] == true && !has([]string{"cancelled", "failed"}, str(b["status"])) {
			primaryKnown = true
			break
		}
	}
	var counts [2]int
	var totals [2][len(developmentKeys)]float64
	var known [2][len(developmentKeys)]int
	var ports [2]int
	active := []M{}
	for _, b := range buildings {
		status := str(b["status"])
		impact := developmentImpact(b, primaryKnown, p.stations[str(b["marketId"])])
		if includeDetails {
			if impact != nil {
				p.addDevelopmentBody(impact, project, b)
			}
			b["impact"] = impact
		}
		if !has([]string{"complete", "building", "planned"}, status) {
			continue
		}
		active = append(active, b)
		group := 0
		if status != "complete" {
			group = 1
		}
		counts[group]++
		if b["kind"] == "port" && num(b["tier"]) >= 2 {
			ports[group]++
		}
		for index, key := range developmentKeys {
			if n, ok := value.Number(value.At(impact, "metrics", key, "weighted")); ok {
				totals[group][index] += n
				known[group][index]++
			}
		}
	}
	if includeDetails {
		addDevelopmentLinks(str(project["targetSystem"]), active)
	}
	metrics := make([]M, 0, len(developmentKeys))
	for index, key := range developmentKeys {
		built, planned := totals[0][index], totals[1][index]
		metrics = append(metrics, M{"key": key, "built": built, "planned": planned, "total": built + planned, "builtKnown": known[0][index], "plannedKnown": known[1][index]})
	}
	return M{"metrics": metrics, "builtCount": counts[0], "plannedCount": counts[1], "builtTechFloor": ports[0] > 0, "plannedTechFloor": ports[0]+ports[1] > 0, "primaryKnown": primaryKnown, "externalPorts": project["externalPortCount"]}
}

type bodyPorts struct{ orbital, surface M }

func addDevelopmentLinks(system string, buildings []M) {
	byBody := map[string]bodyPorts{}
	bodyOrder := []string{}
	prefix := strings.ToLower(strings.Join(strings.Fields(system), " ")) + " "
	bodyKey := func(raw any) string {
		return strings.TrimPrefix(strings.ToLower(strings.Join(strings.Fields(str(raw)), " ")), prefix)
	}
	for _, b := range buildings {
		key := bodyKey(b["body"])
		if key == "" || b["kind"] != "port" {
			continue
		}
		current, exists := byBody[key]
		if !exists {
			bodyOrder = append(bodyOrder, key)
		}
		switch b["location"] {
		case "surface":
			if preferredPort(b, current.surface) {
				current.surface = b
			}
		case "orbital":
			if preferredPort(b, current.orbital) {
				current.orbital = b
			}
		}
		byBody[key] = current
	}
	record := func(from, to M, kind string) {
		if from == nil || to == nil || from["id"] == to["id"] {
			return
		}
		state := "planned"
		if from["status"] == "complete" && to["status"] == "complete" {
			state = "built"
		}
		for index, b := range [2]M{from, to} {
			impact := value.Obj(b["impact"])
			if len(impact) == 0 {
				impact = M{"known": false}
				b["impact"] = impact
			}
			count := value.Int(impact["linkCount"])
			impact["linkCount"] = count + 1
			if count < 8 {
				other := to
				direction := "out"
				if index == 1 {
					direction, other = "in", from
				}
				impact["links"] = append(rows(impact["links"]), M{"id": other["id"], "name": other["name"], "body": other["body"], "strength": kind, "state": state, "direction": direction})
			}
		}
	}
	// Strong links are listed first so a large plan still shows local effects.
	for _, b := range buildings {
		body := bodyKey(b["body"])
		if body == "" {
			continue
		}
		current := byBody[body]
		port := current.orbital
		if b["location"] == "surface" && current.surface != nil && current.surface["id"] != b["id"] {
			port = current.surface
		}
		record(b, port, "strong")
	}
	for _, b := range buildings {
		body := bodyKey(b["body"])
		if body == "" || !has([]string{"port", "settlement", "hub", "installation"}, str(b["kind"])) {
			continue
		}
		ports := byBody[body]
		if b["kind"] == "port" && (ports.orbital["id"] == b["id"] || ports.orbital == nil && ports.surface["id"] == b["id"]) {
			continue
		}
		for _, other := range bodyOrder {
			if body == other {
				continue
			}
			target := byBody[other].orbital
			if target == nil {
				target = byBody[other].surface
			}
			record(b, target, "weak")
		}
	}
}

func preferredPort(candidate, current M) bool {
	if current == nil || num(candidate["tier"]) != num(current["tier"]) {
		return current == nil || num(candidate["tier"]) > num(current["tier"])
	}
	if candidate["isPrimary"] != current["isPrimary"] {
		return candidate["isPrimary"] == true
	}
	if (candidate["status"] == "complete") != (current["status"] == "complete") {
		return candidate["status"] == "complete"
	}
	candidateAt, currentAt := value.Time(candidate["journalUpdatedAt"]), value.Time(current["journalUpdatedAt"])
	return !candidateAt.IsZero() && !currentAt.IsZero() && candidateAt.Before(currentAt)
}

// Preserve a richer docking snapshot when a subsequent Location event omits
// the market composition or station services.
func developmentStationSnapshot(station, previous, event M, at time.Time) M {
	station["economy"] = text(firstText(event["StationEconomy_Localised"], event["StationEconomy"], previous["economy"]), 100)
	station["economies"] = stationEconomies(event["StationEconomies"])
	if len(rows(station["economies"])) == 0 {
		station["economies"] = previous["economies"]
	}
	if _, exists := event["StationServices"]; !exists {
		station["services"] = previous["services"]
	}
	if at.Before(value.Time(previous["timestamp"])) {
		return previous
	}
	return station
}
