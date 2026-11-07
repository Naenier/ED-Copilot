package mining

import (
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

func (s *Service) resetObserved() {
	s.World = map[string]M{}
	s.Signals = map[string][]M{}
	s.LocalSignals = map[string]bool{}
	s.Prospect = nil
	s.Surface = M{"site": "", "body": "", "system": "", "lastRefined": []M{}, "timestamp": nil}
	s.StatsTotal = M{}
	for _, k := range []string{"refinedTons", "materialUnits", "soldTons", "revenue", "miningRevenue", "prospected", "accepted", "rejected", "asteroidsCracked"} {
		s.StatsTotal[k] = float64(0)
	}
	s.StatsRows = map[string]M{}
	s.Daily = map[string]M{}
	s.Recent = nil
	s.FirstStatsEvent = ""
	s.intervals = map[string][2]time.Time{}
	s.session = 0
	s.lastSystem = ""
}
func (s *Service) system(event, commander M) M {
	address := v.First(event["SystemAddress"], commander["systemAddress"])
	name := v.Text(event["StarSystem"], event["SystemName"], commander["system"])
	key := "name:" + strings.ToLower(name)
	if address != nil {
		key = v.Str(address)
	}
	system := s.World[key]
	if system == nil {
		for _, r := range s.World {
			if strings.EqualFold(v.Str(r["name"]), name) && name != "" {
				system = r
				break
			}
		}
	}
	if system == nil {
		system = M{"key": key, "address": address, "name": name, "coordinates": nil, "bodies": M{}, "timestamp": v.First(event["timestamp"], nowISO())}
		s.World[key] = system
	}
	if name != "" {
		system["name"] = name
	}
	if address != nil {
		system["address"] = address
	}
	if len(trading.Coords(event["StarPos"])) == 3 {
		system["coordinates"] = event["StarPos"]
	} else if system["coordinates"] == nil && len(trading.Coords(commander["coordinates"])) == 3 {
		system["coordinates"] = commander["coordinates"]
	}
	return system
}
func bodyFor(system M, id any, name string) M {
	key := "name:" + name
	if id != nil {
		key = v.Str(id)
	}
	bodies := v.Obj(system["bodies"])
	body := v.Obj(bodies[key])
	if len(body) == 0 && name != "" {
		for _, r := range bodies {
			if strings.EqualFold(v.Str(v.Obj(r)["name"]), name) {
				body = v.Obj(r)
				break
			}
		}
	}
	if len(body) == 0 {
		body = M{"id": v.First(id, key), "name": v.Text(name, system["name"]), "rings": []M{}, "materials": []M{}, "source": "journal"}
		bodies[key] = body
	}
	if name != "" {
		body["name"] = name
	}
	system["bodies"] = bodies
	return body
}
func (s *Service) record(event, context M, kind string, resource M, count, value float64) {
	if context["countInPeriod"] == false {
		return
	}
	timestamp := v.Text(event["timestamp"], nowISO())
	if s.FirstStatsEvent == "" {
		s.FirstStatsEvent = timestamp
	}
	date := timestamp
	if len(date) > 10 {
		date = date[:10]
	}
	day := s.Daily[date]
	if day == nil {
		day = M{"date": date, "refinedTons": 0, "materialUnits": 0, "prospected": 0, "revenue": 0}
		s.Daily[date] = day
	}
	key := v.Text(resource["key"], kind)
	name := v.Text(resource["name"], map[string]string{"prospect": "Исследован астероид", "cracked": "Расколото ядро"}[kind], kind)
	row := s.StatsRows[key]
	if resource != nil && row == nil {
		row = M{"key": key, "name": name, "refinedTons": 0, "materialUnits": 0, "soldTons": 0, "revenue": 0}
		s.StatsRows[key] = row
	}
	switch kind {
	case "refined":
		increment(s.StatsTotal, "refinedTons", count)
		increment(day, "refinedTons", count)
		increment(row, "refinedTons", count)
	case "material":
		increment(s.StatsTotal, "materialUnits", count)
		increment(day, "materialUnits", count)
		increment(row, "materialUnits", count)
	case "sale":
		increment(s.StatsTotal, "revenue", value)
		increment(s.StatsTotal, "soldTons", count)
		increment(day, "revenue", value)
		increment(row, "soldTons", count)
		increment(row, "revenue", value)
	case "prospect":
		increment(s.StatsTotal, "prospected", 1)
		if s.Prospect["decision"] == "mine" {
			increment(s.StatsTotal, "accepted", 1)
		}
		if s.Prospect["decision"] == "skip" {
			increment(s.StatsTotal, "rejected", 1)
		}
		increment(day, "prospected", 1)
	case "cracked":
		increment(s.StatsTotal, "asteroidsCracked", 1)
	}
	s.Recent = append([]M{{"timestamp": timestamp, "kind": kind, "key": key, "name": name, "count": count, "value": value, "system": v.At(context, "commander", "system")}}, s.Recent...)
	if len(s.Recent) > 120 {
		s.Recent = s.Recent[:120]
	}
}
func (s *Service) observe(event, context M) {
	typ := v.Str(event["event"])
	cmd := v.Obj(context["commander"])
	timestamp := v.Str(event["timestamp"])
	if typ == "LoadGame" {
		s.session++
	}
	if context["countInPeriod"] != false {
		file := v.Text(context["file"], fmt.Sprint(s.session))
		at := v.Time(timestamp)
		interval := s.intervals[file]
		if interval[0].IsZero() || at.Before(interval[0]) {
			interval[0] = at
		}
		if interval[1].IsZero() || at.After(interval[1]) {
			interval[1] = at
		}
		s.intervals[file] = interval
	}
	if typ == "Location" || typ == "FSDJump" || typ == "CarrierJump" {
		s.system(event, cmd)
		systemName := v.Str(cmd["system"])
		if systemName != s.lastSystem {
			s.Prospect = nil
			s.Surface = M{"site": "", "body": "", "system": systemName, "lastRefined": []M{}, "timestamp": timestamp}
			s.lastSystem = systemName
		}
	}
	if typ == "Scan" {
		system := s.system(event, cmd)
		body := bodyFor(system, event["BodyID"], v.Str(event["BodyName"]))
		body["timestamp"] = timestamp
		body["source"] = "journal"
		body["remoteOnly"] = false
		if b, ok := event["Landable"].(bool); ok {
			body["landable"] = b
		}
		if event["PlanetClass"] != nil {
			body["planetClass"] = event["PlanetClass"]
		}
		if x, ok := v.Number(event["SurfaceGravity"]); ok {
			body["gravity"] = x / 9.80665
		}
		if x, ok := v.Number(event["DistanceFromArrivalLS"]); ok {
			body["distanceLs"] = x
		}
		if event["ReserveLevel"] != nil {
			body["reserve"] = event["ReserveLevel"]
		}
		if event["Materials"] != nil {
			materials := []M{}
			if rows := v.Rows(event["Materials"]); rows != nil {
				for _, r := range rows {
					materials = append(materials, M{"key": v.Key(r["Name"]), "percent": v.Num(r["Percent"])})
				}
			} else {
				for name, percent := range v.Obj(event["Materials"]) {
					materials = append(materials, M{"key": v.Key(name), "percent": v.Num(percent)})
				}
			}
			body["materials"] = materials
		}
		if event["Rings"] != nil {
			rings := []M{}
			for _, r := range v.Rows(event["Rings"]) {
				if v.Str(r["Name"]) != "" {
					rings = append(rings, M{"name": r["Name"], "type": v.First(r["RingClass"], ""), "inner": r["InnerRad"], "outer": r["OuterRad"]})
				}
			}
			body["rings"] = rings
		}
	}
	if (typ == "SAASignalsFound" || typ == "FSSBodySignals") && event["BodyName"] != nil && event["Signals"] != nil {
		system := s.system(event, cmd)
		s.Signals[v.Str(system["key"])+":"+v.Str(event["BodyName"])] = v.Rows(v.Clone(event["Signals"]))
		s.LocalSignals[v.Str(system["key"])+":"+v.Str(event["BodyName"])] = true
		if !strings.HasSuffix(v.Str(event["BodyName"]), "Ring") {
			bodyFor(system, event["BodyID"], v.Str(event["BodyName"]))["timestamp"] = timestamp
		}
	}
	if typ == "ProspectedAsteroid" {
		materials := []M{}
		for _, entry := range v.Rows(event["Materials"]) {
			g := trading.Good(entry["Name"], entry["Name_Localised"])
			materials = append(materials, M{"key": g["key"], "name": g["name"], "percent": withDefault(entry["Proportion"], 0), "tier": v.Text(g["tier"], "unknown")})
		}
		s.Prospect = AssessProspect(M{"timestamp": timestamp, "system": cmd["system"], "body": cmd["body"], "remaining": positive(event["Remaining"]), "content": v.First(event["Content_Localised"], event["Content"], ""), "motherlode": v.Key(event["MotherlodeMaterial"]), "materials": materials}, v.Obj(context["settings"]), trading.Resources())
		s.record(event, context, "prospect", nil, 1, 0)
	}
	if typ == "AsteroidCracked" {
		s.record(event, context, "cracked", nil, 1, 0)
		if s.Prospect != nil {
			s.Prospect["coreCracked"] = true
		}
	}
	if typ == "ApproachBody" || typ == "Touchdown" || typ == "SupercruiseExit" || typ == "ApproachSettlement" {
		destination := v.Str(event["NearestDestination"])
		if strings.Contains(destination, "PlanetaryMiningLocation") {
			site := "Участок планетарной добычи"
			if match := regexp.MustCompile(`#index=(\d+)`).FindStringSubmatch(destination); len(match) > 1 {
				site = "Участок " + match[1]
			}
			s.Surface = M{"site": site, "body": v.First(event["Body"], event["BodyName"], cmd["body"]), "system": cmd["system"], "lastRefined": []M{}, "timestamp": timestamp}
		}
	}
	if typ == "MiningRefined" {
		g := trading.Good(event["Type"], event["Type_Localised"])
		count := withDefault(event["Count"], 1)
		s.record(event, context, "refined", g, count, 0)
		if trading.Vessel(cmd["vehicle"]) == "srv" {
			rows := v.Rows(s.Surface["lastRefined"])
			found := false
			for _, r := range rows {
				if r["key"] == g["key"] {
					increment(r, "count", count)
					found = true
				}
			}
			if !found {
				rows = append(rows, M{"key": g["key"], "name": g["name"], "count": count})
			}
			s.Surface["lastRefined"] = rows
			s.Surface["body"] = cmd["body"]
			s.Surface["system"] = cmd["system"]
			s.Surface["timestamp"] = timestamp
			if v.Str(cmd["system"]) != "" && v.Str(cmd["body"]) != "" {
				body := bodyFor(s.system(event, cmd), nil, v.Str(cmd["body"]))
				siteName := v.Text(s.Surface["site"], "Участок не определён")
				finds := v.Rows(body["surfaceFinds"])
				var site M
				for _, r := range finds {
					if r["site"] == siteName {
						site = r
					}
				}
				if site == nil {
					site = M{"site": siteName, "resources": []M{}, "timestamp": timestamp}
					finds = append(finds, site)
				}
				products := v.Rows(site["resources"])
				var product M
				for _, r := range products {
					if r["key"] == g["key"] {
						product = r
					}
				}
				if product == nil {
					product = M{"key": g["key"], "name": g["name"], "count": 0}
					products = append(products, product)
				}
				increment(product, "count", count)
				site["resources"] = products
				site["timestamp"] = timestamp
				body["surfaceFinds"] = finds
			}
		}
	}
	if typ == "MaterialCollected" {
		g := trading.Resource(event["Name"])
		if g["kind"] == "raw" || strings.EqualFold(v.Str(event["Category"]), "raw") {
			if g == nil {
				g = trading.Good(event["Name"], event["Name_Localised"])
			}
			s.record(event, context, "material", g, withDefault(event["Count"], 1), 0)
		}
	}
	if typ == "MarketSell" && !v.Bool(context["ownMarket"]) && !v.Bool(v.At(context, "tradeResult", "internal")) {
		g := trading.Good(event["Type"], event["Type_Localised"])
		count := withDefault(event["Count"], 1)
		s.record(event, context, "sale", g, count, withDefault(event["TotalSale"], v.Num(event["SellPrice"])*count))
		if context["countInPeriod"] != false {
			increment(s.StatsTotal, "miningRevenue", v.Num(v.At(context, "tradeResult", "minedRevenue")))
		}
	}
}
func (s *Service) WorldSnapshot() []M {
	rows := []M{}
	for _, system := range s.World {
		row := v.Copy(system)
		row["bodies"] = bodiesRows(system["bodies"])
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return v.Str(rows[i]["name"]) < v.Str(rows[j]["name"]) })
	return rows
}
func (s *Service) MergeRemoteSystem(incoming M) {
	name := v.Str(incoming["name"])
	system := s.system(M{"StarSystem": name, "SystemAddress": incoming["address"], "StarPos": incoming["coordinates"], "timestamp": incoming["timestamp"]}, M{})
	for _, body := range bodiesRows(incoming["bodies"]) {
		old := bodyFor(system, body["id"], v.Str(body["name"]))
		local := old["source"] == "journal" && old["timestamp"] != nil
		rings := v.Rows(old["rings"])
		byName := map[string]M{}
		for _, r := range rings {
			byName[v.Str(r["name"])] = r
		}
		for _, r := range v.Rows(body["rings"]) {
			name := v.Str(r["name"])
			previous := byName[name]
			if previous == nil {
				previous = M{}
			}
			byName[name] = v.Merge(previous, r)
		}
		rings = []M{}
		for _, r := range byName {
			rings = append(rings, r)
		}
		if local {
			if len(v.Rows(old["rings"])) == 0 {
				old["rings"] = rings
			}
			if len(v.Rows(old["materials"])) == 0 {
				old["materials"] = body["materials"]
			}
			for _, field := range []string{"landable", "gravity", "distanceLs", "reserve", "planetClass"} {
				if old[field] == nil {
					old[field] = body[field]
				}
			}
		} else {
			v.Merge(old, body)
			old["rings"] = rings
			old["remoteOnly"] = true
		}
		key := v.Str(system["key"]) + ":" + v.Str(old["name"])
		if len(v.Rows(body["signals"])) > 0 && !s.LocalSignals[key] {
			s.Signals[key] = v.Rows(v.Clone(body["signals"]))
		}
		for _, ring := range v.Rows(body["rings"]) {
			key = v.Str(system["key"]) + ":" + v.Str(ring["name"])
			if s.LocalSignals[key] || len(v.Rows(ring["signals"])) == 0 {
				continue
			}
			combined := map[string]M{}
			for _, signal := range s.Signals[key] {
				combined[v.Key(v.First(signal["key"], signal["Type"], signal["name"]))] = signal
			}
			for _, signal := range v.Rows(ring["signals"]) {
				combined[v.Key(v.First(signal["key"], signal["Type"], signal["name"]))] = signal
			}
			rows := []M{}
			for _, signal := range combined {
				rows = append(rows, signal)
			}
			s.Signals[key] = rows
		}
	}
	system["timestamp"] = v.First(incoming["timestamp"], system["timestamp"])
}
func (s *Service) observedSnapshot(context M) M {
	resources := v.Rows(context["resources"])
	if len(resources) == 0 {
		resources = trading.Resources()
	}
	prospect := AssessProspect(s.Prospect, v.Obj(context["settings"]), resources)
	world := s.WorldSnapshot()
	statsRows := []M{}
	for _, r := range s.StatsRows {
		statsRows = append(statsRows, v.Copy(r))
	}
	sort.Slice(statsRows, func(i, j int) bool { return v.Num(statsRows[i]["refinedTons"]) > v.Num(statsRows[j]["refinedTons"]) })
	daily := []M{}
	for _, r := range s.Daily {
		daily = append(daily, v.Copy(r))
	}
	sort.Slice(daily, func(i, j int) bool { return v.Str(daily[i]["date"]) > v.Str(daily[j]["date"]) })
	intervals := [][2]time.Time{}
	for _, r := range s.intervals {
		if r[1].After(r[0]) {
			intervals = append(intervals, r)
		}
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i][0].Before(intervals[j][0]) })
	elapsed := float64(0)
	var last time.Time
	for _, r := range intervals {
		start := r[0]
		if last.After(start) {
			start = last
		}
		if r[1].After(start) {
			elapsed += r[1].Sub(start).Seconds()
		}
		if r[1].After(last) {
			last = r[1]
		}
	}
	var rate any
	if elapsed >= 60 {
		rate = v.Num(s.StatsTotal["miningRevenue"]) * 3600 / elapsed
	}
	return M{"targets": BuildTargets(world, s.Signals, resources, v.Obj(context["commander"]), v.Obj(context["settings"])), "prospect": prospect, "surface": v.Copy(s.Surface), "world": world, "stats": M{"since": v.First(context["statsSince"], s.FirstStatsEvent, nowISO()), "total": v.Copy(s.StatsTotal), "byResource": statsRows, "sessions": daily, "recent": v.Clone(s.Recent), "elapsedSeconds": math.Max(0, elapsed), "timeBasis": "journal-sessions", "revenuePerHour": rate}}
}
