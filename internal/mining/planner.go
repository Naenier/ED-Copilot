package mining

import (
	"errors"
	"github.com/Naenier/ED-Copilot/internal/data"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

type M = v.M

var methods = map[string]bool{"laser": true, "core": true, "subsurface": true, "surface": true, "surface-planet": true, "raw": true}
var boundaries = map[string]string{"Docked": "Посадка на станцию", "FSDJump": "Переход в другую систему", "CarrierJump": "Прыжок флотоносца", "SupercruiseEntry": "Выход в суперкруиз", "Shutdown": "Завершение игры", "Died": "Потеря корабля", "LoadGame": "Новый вход в игру"}

func DefaultPlan() M {
	return M{"resourceKey": "platinum", "method": "laser", "mode": "mining", "targetTons": nil, "sellPrice": nil, "purchasePrice": nil, "tripMinutes": 20, "operatingCost": 0, "targetCredits": nil, "tonsPerHour": nil}
}
func positive(raw any) any {
	n, ok := v.Number(raw)
	if !ok {
		return nil
	}
	return math.Max(0, n)
}
func withDefault(raw any, fallback float64) float64 {
	n, ok := v.Number(raw)
	if !ok {
		return fallback
	}
	return math.Max(0, n)
}
func increment(m M, key string, delta float64) { m[key] = v.Num(m[key]) + delta }
func CargoCapacity(commander, loadout M) any {
	if trading.Vessel(commander["vehicle"]) == "srv" {
		return positive(commander["srvCargoCapacity"])
	}
	return positive(v.First(commander["cargoCapacity"], loadout["cargoCapacity"]))
}
func ClassifyModule(module M) M {
	item := strings.ToLower(v.Text(module["Item"], module["item"]))
	tags := []string{}
	for _, r := range []struct {
		pattern string
		tags    []string
	}{{`^hpt_(mininglaser_|miningtoolv2_)`, []string{"laser"}}, {`^hpt_mining_seismchrgwarhd_`, []string{"core"}}, {`^hpt_mining_subsurfdispmisle_`, []string{"subsurface"}}, {`^hpt_mining_abrblstr_`, []string{"abrasion"}}, {`^hpt_mrascanner_`, []string{"pulse"}}, {`^int_refinery_`, []string{"refinery"}}, {`^int_dronecontrol_collection_`, []string{"collector"}}, {`^int_dronecontrol_prospector_`, []string{"prospector"}}, {`^int_multidronecontrol_(miningv?2?|universal)_`, []string{"collector", "prospector"}}, {`^int_multidronecontrol_operations_`, []string{"collector"}}, {`^int_detailedsurfacescanner_`, []string{"dss"}}, {`^int_(mkii)?largebuggybay_`, []string{"large-srv-bay"}}, {`^int_(buggybay|largebuggybay|mkiilargebuggybay)_`, []string{"srv-bay"}}, {`^int_shieldgenerator_`, []string{"shield"}}} {
		if regexp.MustCompile(r.pattern).MatchString(item) {
			tags = append(tags, r.tags...)
		}
	}
	var ammo any
	if module["AmmoInClip"] != nil || module["AmmoInHopper"] != nil {
		ammo = withDefault(module["AmmoInClip"], 0) + withDefault(module["AmmoInHopper"], 0)
	}
	return M{"slot": v.Str(module["Slot"]), "item": item, "tags": tags, "enabled": module["On"] != false, "health": positive(module["Health"]), "ammo": ammo}
}
func has(values any, wanted string) bool {
	for _, raw := range v.Arr(values) {
		if v.Str(raw) == wanted {
			return true
		}
	}
	return false
}
func Preflight(loadout M, method string, commander M, limpets any) M {
	if !methods[method] {
		method = "laser"
	}
	required := [][2]string{{"refinery", "Очиститель"}, {method, v.Str(v.At(data.Object("catalog.json"), "methods", method))}}
	switch method {
	case "raw":
		required = [][2]string{{"srv-bay", "Ангар для ТРП"}}
	case "surface-planet":
		required = [][2]string{{"large-srv-bay", "Ангар для Rhino"}}
	case "surface":
		required[1] = [2]string{"abrasion", "Абразивный бластер"}
	case "core":
		required = append(required, [2]string{"abrasion", "Абразивный бластер"})
	}
	recommended := [][2]string{}
	if method != "raw" && method != "surface-planet" {
		recommended = [][2]string{{"collector", "Контроллер дронов-сборщиков"}, {"prospector", "Контроллер дронов-разведчиков"}, {"dss", "Подробный сканер поверхности"}}
		if method == "core" || method == "subsurface" || method == "surface" {
			recommended = append(recommended, [2]string{"pulse", "Импульсный волновой анализатор"})
		}
	}
	checks := []M{}
	test := func(pair [2]string, required bool) {
		found, usable, empty := 0, 0, 0
		for _, module := range v.Rows(loadout["modules"]) {
			if !has(module["tags"], pair[0]) {
				continue
			}
			found++
			if v.Bool(module["enabled"]) && module["health"] != float64(0) && module["health"] != 0 {
				usable++
				if module["ammo"] != nil && v.Num(module["ammo"]) == 0 {
					empty++
				}
			}
		}
		status := "ok"
		if !v.Bool(loadout["known"]) {
			status = "unknown"
		} else if found == 0 {
			status = "missing"
		} else if usable == 0 {
			status = "disabled"
		} else if usable == empty {
			status = "empty"
		}
		checks = append(checks, M{"key": pair[0], "name": pair[1], "required": required, "status": status, "count": found, "usableCount": usable})
	}
	for _, r := range required {
		test(r, true)
	}
	for _, r := range recommended {
		test(r, false)
	}
	capacity, tons := CargoCapacity(commander, loadout), positive(commander["cargoTons"])
	if method != "raw" {
		status := "unknown"
		var free any
		if capacity != nil && tons != nil {
			free = math.Max(0, v.Num(capacity)-v.Num(tons))
			status = "full"
			if v.Num(capacity) > v.Num(tons) {
				status = "ok"
			}
		}
		checks = append(checks, M{"key": "cargo", "name": "Место в грузовом трюме", "required": true, "status": status, "freeTons": free})
	}
	if method != "raw" && method != "surface-planet" {
		status := "unknown"
		if limpets != nil {
			status = "empty"
			if v.Num(limpets) > 0 {
				status = "ok"
			}
		}
		checks = append(checks, M{"key": "limpets", "name": "Дроны в трюме", "required": false, "status": status, "count": limpets})
	}
	status := "ready"
	for _, c := range checks {
		if !v.Bool(c["required"]) {
			continue
		}
		if c["status"] == "unknown" && status == "ready" {
			status = "unknown"
		}
		if c["status"] == "missing" || c["status"] == "disabled" || c["status"] == "empty" || c["status"] == "full" {
			status = "incomplete"
		}
	}
	return M{"method": method, "known": v.Bool(loadout["known"]), "ship": v.Text(loadout["ship"], commander["ship"]), "timestamp": loadout["timestamp"], "status": status, "checks": checks, "help": "Проверяются модули последнего снимка Loadout. Наличие ангара не подтверждает тип находящегося в нём ТРП; активные дроны и распределение энергии проверяются в игре."}
}

type Service struct {
	Options                          M
	ResetAt                          string
	Current                          M
	History                          []M
	Loadout                          M
	sequence                         int
	LastEventAt                      string
	Limpets                          any
	LastLimpetsAt                    string
	GameRunning, resetApplied        bool
	World                            map[string]M
	Signals                          map[string][]M
	LocalSignals                     map[string]bool
	Prospect, Surface, StatsTotal    M
	StatsRows, Daily                 map[string]M
	Recent                           []M
	FirstStatsEvent                  string
	intervals                        map[string][2]time.Time
	session                          int
	lastSystem                       string
	hudWorldVersion                  uint64
	hudTargetsKey                    string
	hudSurfaceTargets, hudRawTargets []M
}

func New(saved M) *Service {
	s := &Service{Options: DefaultPlan(), ResetAt: v.Str(saved["resetAt"])}
	options := v.Obj(saved["options"])
	if len(options) == 0 {
		options = v.Obj(saved["plan"])
	}
	if len(options) == 0 {
		options = saved
	}
	_ = s.Update(options)
	s.Reset()
	return s
}
func (s *Service) Reset() {
	s.Current = nil
	s.History = nil
	s.Loadout = nil
	s.sequence = 0
	s.LastEventAt = ""
	s.Limpets = nil
	s.LastLimpetsAt = ""
	s.GameRunning = false
	s.resetApplied = false
	s.resetObserved()
}
func (s *Service) ExportSettings() M { return M{"options": v.Copy(s.Options), "resetAt": s.ResetAt} }
func (s *Service) Update(update M) error {
	if action := v.Str(update["action"]); action == "reset" {
		return s.ResetRun(v.Text(update["timestamp"], v.ISO(time.Now())))
	}
	if update["resourceKey"] != nil {
		s.Options["resourceKey"] = v.Key(update["resourceKey"])
	}
	if methods[v.Str(update["method"])] {
		s.Options["method"] = update["method"]
	}
	if update["mode"] == "mining" || update["mode"] == "trade" {
		s.Options["mode"] = update["mode"]
	}
	for _, key := range []string{"targetTons", "sellPrice", "purchasePrice", "tripMinutes", "operatingCost", "targetCredits", "tonsPerHour"} {
		if raw, ok := update[key]; ok {
			if raw != nil && raw != "" {
				n, nok := v.Number(raw)
				if !nok || n < 0 {
					return errors.New("Параметры плана должны быть неотрицательными числами.")
				}
			}
			s.Options[key] = positive(raw)
		}
	}
	if raw, ok := update["buyPrice"]; ok && update["purchasePrice"] == nil {
		s.Options["purchasePrice"] = positive(raw)
	}
	return nil
}
func (s *Service) ResetRun(timestamp string) error {
	if v.Time(timestamp).IsZero() {
		return errors.New("Некорректная дата новой сессии.")
	}
	s.finish(timestamp, "Начата новая сессия вручную")
	s.ResetAt = timestamp
	s.resetApplied = true
	return nil
}
func (s *Service) begin(event, commander M) M {
	if s.Current != nil {
		return s.Current
	}
	s.sequence++
	s.Current = M{"id": v.ID(), "startedAt": event["timestamp"], "lastEventAt": event["timestamp"], "lastMiningAt": event["timestamp"], "endedAt": nil, "closeReason": nil, "system": v.Str(commander["system"]), "body": v.Str(commander["body"]), "ship": v.Str(commander["ship"]), "vehicle": v.Text(commander["vehicle"], "unknown"), "refinedTons": 0, "materialUnits": 0, "prospectScans": 0, "prospectors": 0, "collectors": 0, "asteroidsCracked": 0, "yields": M{}, "composition": M{}, "rawContent": M{"high": 0, "medium": 0, "low": 0, "unknown": 0}}
	return s.Current
}
func (s *Service) finish(timestamp, reason string) {
	if s.Current == nil {
		return
	}
	s.Current["endedAt"] = timestamp
	s.Current["lastEventAt"] = timestamp
	s.Current["closeReason"] = reason
	if v.Num(s.Current["refinedTons"])+v.Num(s.Current["materialUnits"])+v.Num(s.Current["prospectScans"])+v.Num(s.Current["asteroidsCracked"]) > 0 {
		s.History = append([]M{s.Current}, s.History...)
		if len(s.History) > 40 {
			s.History = s.History[:40]
		}
	}
	s.Current = nil
}
func (s *Service) Apply(event, context M) {
	at := v.Str(event["timestamp"])
	if v.Time(at).IsZero() {
		return
	}
	s.observe(event, context)
	typ := v.Str(event["event"])
	cmd := v.Obj(context["commander"])
	if !s.resetApplied && s.ResetAt != "" && !v.Time(at).Before(v.Time(s.ResetAt)) {
		s.finish(s.ResetAt, "Начата новая сессия вручную")
		s.resetApplied = true
	}
	s.LastEventAt = at
	s.GameRunning = typ != "Shutdown"
	if typ == "Loadout" && event["Modules"] != nil {
		modules := []M{}
		for _, row := range v.Rows(event["Modules"]) {
			modules = append(modules, ClassifyModule(row))
		}
		s.Loadout = M{"known": true, "timestamp": at, "ship": v.Text(event["Ship_Localised"], cmd["ship"], event["Ship"]), "shipId": event["ShipID"], "cargoCapacity": positive(event["CargoCapacity"]), "modules": modules}
	}
	for _, change := range []string{"ModuleBuy", "ModuleSell", "ModuleStore", "ModuleRetrieve", "ModuleSwap", "MassModuleStore", "ShipyardSwap", "ShipyardBuy", "ShipyardNew"} {
		if typ == change && s.Loadout != nil {
			s.Loadout["known"] = false
		}
	}
	if typ == "LoadGame" && s.Loadout != nil && event["ShipID"] != nil && v.Str(s.Loadout["shipId"]) != v.Str(event["ShipID"]) {
		s.Loadout["known"] = false
	}
	if typ == "Cargo" && event["Inventory"] != nil && trading.Vessel(event["Vessel"]) != "srv" {
		s.Limpets = float64(0)
		for _, r := range v.Rows(event["Inventory"]) {
			key := v.Key(v.First(r["Name"], r["Type"]))
			if key == "drones" || key == "limpet" || key == "limpets" {
				s.Limpets = withDefault(r["Count"], 0)
			}
		}
		s.LastLimpetsAt = at
	}
	if s.Limpets != nil && (typ == "BuyDrones" || typ == "SellDrones" || typ == "LaunchDrone") {
		delta := -1.0
		if typ == "BuyDrones" {
			delta = withDefault(event["Count"], 0)
		}
		if typ == "SellDrones" {
			delta = -withDefault(event["Count"], 0)
		}
		s.Limpets = math.Max(0, v.Num(s.Limpets)+delta)
		s.LastLimpetsAt = at
	}
	if s.Current != nil && at >= v.Str(s.Current["lastEventAt"]) {
		s.Current["lastEventAt"] = at
	}
	if reason := boundaries[typ]; reason != "" {
		s.finish(at, reason)
		return
	}
	if context["countInPeriod"] == false {
		return
	}
	raw := typ == "MaterialCollected" && (trading.Resource(event["Name"])["kind"] == "raw" || strings.EqualFold(v.Str(event["Category"]), "raw"))
	drone := typ == "LaunchDrone" && (event["Type"] == "Collection" || event["Type"] == "Prospector")
	if typ != "ProspectedAsteroid" && typ != "MiningRefined" && typ != "AsteroidCracked" && !raw && !drone {
		return
	}
	run := s.begin(event, cmd)
	run["lastMiningAt"] = at
	if drone {
		key := "collectors"
		if event["Type"] == "Prospector" {
			key = "prospectors"
		}
		increment(run, key, 1)
	}
	if typ == "AsteroidCracked" {
		increment(run, "asteroidsCracked", 1)
	}
	if typ == "MiningRefined" || raw {
		symbol := event["Type"]
		if raw {
			symbol = event["Name"]
		}
		g := trading.Good(symbol, v.First(event["Name_Localised"], event["Type_Localised"]))
		key := v.Str(g["key"])
		if key != "" {
			yields := v.Obj(run["yields"])
			row := v.Obj(yields[key])
			if len(row) == 0 {
				row = M{"key": key, "name": g["name"], "refinedTons": 0, "materialUnits": 0}
				yields[key] = row
			}
			field := "refinedTons"
			if raw {
				field = "materialUnits"
			}
			count := withDefault(event["Count"], 1)
			increment(row, field, count)
			increment(run, field, count)
		}
	}
	if typ == "ProspectedAsteroid" {
		increment(run, "prospectScans", 1)
		content := strings.ToLower(v.Str(event["Content"]))
		level := "unknown"
		for _, name := range []string{"high", "medium", "low"} {
			if strings.Contains(content, name) {
				level = name
				break
			}
		}
		increment(v.Obj(run["rawContent"]), level, 1)
		entries := append([]M{}, v.Rows(event["Materials"])...)
		motherlode := v.Key(event["MotherlodeMaterial"])
		if motherlode != "" {
			entries = append(entries, M{"Name": event["MotherlodeMaterial"], "Proportion": nil})
		}
		seen := map[string]bool{}
		composition := v.Obj(run["composition"])
		for _, entry := range entries {
			g := trading.Good(entry["Name"], entry["Name_Localised"])
			key := v.Str(g["key"])
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			row := v.Obj(composition[key])
			if len(row) == 0 {
				row = M{"key": key, "name": g["name"], "scans": 0, "samples": 0, "sumPercent": 0, "minPercent": nil, "maxPercent": nil, "motherlodes": 0, "above": M{"10": 0, "15": 0, "20": 0, "30": 0, "40": 0}}
				composition[key] = row
			}
			increment(row, "scans", 1)
			if key == motherlode {
				increment(row, "motherlodes", 1)
			}
			percent, ok := v.Number(entry["Proportion"])
			if ok && percent >= 0 && percent <= 100 {
				increment(row, "samples", 1)
				increment(row, "sumPercent", percent)
				if row["minPercent"] == nil || percent < v.Num(row["minPercent"]) {
					row["minPercent"] = percent
				}
				if row["maxPercent"] == nil || percent > v.Num(row["maxPercent"]) {
					row["maxPercent"] = percent
				}
				for _, threshold := range []float64{10, 15, 20, 30, 40} {
					if percent >= threshold {
						increment(v.Obj(row["above"]), v.Str(threshold), 1)
					}
				}
			}
		}
	}
}
func (s *Service) runSnapshot(run, context M) M {
	if run == nil {
		return nil
	}
	out := v.Copy(run)
	end := v.Time(run["lastEventAt"])
	now := v.Time(context["now"])
	if now.IsZero() {
		now = time.Now()
	}
	live := run["endedAt"] == nil && v.Bool(context["gameRunning"])
	capTime := end.Add(5 * time.Minute)
	clockCapped := false
	if live {
		liveEnd := now
		if liveEnd.After(capTime) {
			liveEnd = capTime
			clockCapped = true
		}
		if liveEnd.After(end) {
			end = liveEnd
		}
	}
	seconds := math.Max(0, end.Sub(v.Time(run["startedAt"])).Seconds())
	out["elapsedSeconds"] = seconds
	out["timeBasis"] = "elapsed-operation"
	out["clockCapped"] = clockCapped
	out["tonsPerHour"] = nil
	out["refinementsPerMinute"] = nil
	if seconds >= 60 {
		out["tonsPerHour"] = v.Num(run["refinedTons"]) * 3600 / seconds
		out["refinementsPerMinute"] = v.Num(run["refinedTons"]) * 60 / seconds
	}
	out["tonsPerProspector"] = nil
	if v.Num(run["prospectors"]) > 0 {
		out["tonsPerProspector"] = v.Num(run["refinedTons"]) / v.Num(run["prospectors"])
	}
	yields := []M{}
	for _, raw := range v.Obj(run["yields"]) {
		row := v.Copy(v.Obj(raw))
		row["tonsPerHour"] = nil
		if seconds >= 60 {
			row["tonsPerHour"] = v.Num(row["refinedTons"]) * 3600 / seconds
		}
		yields = append(yields, row)
	}
	sort.Slice(yields, func(i, j int) bool { return v.Num(yields[i]["refinedTons"]) > v.Num(yields[j]["refinedTons"]) })
	out["yields"] = yields
	composition := []M{}
	for _, raw := range v.Obj(run["composition"]) {
		row := v.Copy(v.Obj(raw))
		row["occurrencePercent"] = nil
		if v.Num(run["prospectScans"]) > 0 {
			row["occurrencePercent"] = v.Num(row["scans"]) * 100 / v.Num(run["prospectScans"])
		}
		row["averagePercent"] = nil
		if v.Num(row["samples"]) > 0 {
			row["averagePercent"] = v.Num(row["sumPercent"]) / v.Num(row["samples"])
		}
		above := []M{}
		for key, scans := range v.Obj(row["above"]) {
			percent := float64(0)
			if v.Num(run["prospectScans"]) > 0 {
				percent = v.Num(scans) * 100 / v.Num(run["prospectScans"])
			}
			above = append(above, M{"threshold": v.Num(key), "scans": scans, "percentOfAllScans": percent})
		}
		sort.Slice(above, func(i, j int) bool { return v.Num(above[i]["threshold"]) < v.Num(above[j]["threshold"]) })
		row["above"] = above
		composition = append(composition, row)
	}
	sort.Slice(composition, func(i, j int) bool { return v.Num(composition[i]["scans"]) > v.Num(composition[j]["scans"]) })
	out["composition"] = composition
	return out
}
func (s *Service) Snapshot(context M) M {
	cmd := v.Obj(context["commander"])
	current := s.runSnapshot(s.Current, context)
	var currentValue any
	if current != nil {
		currentValue = current
	}
	history := []M{}
	for _, run := range s.History {
		history = append(history, s.runSnapshot(run, M{}))
	}
	limpets := s.Limpets
	if limpets == nil {
		for _, row := range v.Rows(cmd["cargo"]) {
			key := v.Key(v.First(row["key"], row["Name"], row["name"]))
			if key == "drones" || key == "limpet" || key == "limpets" {
				limpets = withDefault(v.First(row["count"], row["Count"]), 0)
			}
		}
	}
	capacity, tons := CargoCapacity(cmd, s.Loadout), positive(cmd["cargoTons"])
	var free, full, fill any
	if capacity != nil && tons != nil {
		free = math.Max(0, v.Num(capacity)-v.Num(tons))
		if v.Num(capacity) > 0 {
			full = v.Num(tons) / v.Num(capacity) * 100
		}
		if v.Num(current["tonsPerHour"]) > 0 {
			fill = v.Num(free) / v.Num(current["tonsPerHour"]) * 60
		}
	}
	latest := current
	if latest == nil && len(history) > 0 {
		latest = history[0]
	}
	var prospect any
	if latest != nil {
		prospect = M{"runId": latest["id"], "prospectScans": latest["prospectScans"], "rawContent": latest["rawContent"], "composition": latest["composition"]}
	}
	planContext := v.Merge(v.Copy(context), M{"current": current, "history": history, "freeTons": free})
	return v.Merge(M{"options": v.Copy(s.Options), "resetAt": s.ResetAt, "current": currentValue, "history": history, "cargo": M{"capacity": capacity, "tons": tons, "freeTons": free, "fullPercent": full, "limpets": limpets, "limpetsVessel": "ship", "limpetsAt": s.LastLimpetsAt, "minutesToFull": fill}, "prospecting": prospect, "preflight": Preflight(s.Loadout, v.Str(s.Options["method"]), cmd, limpets), "plan": EstimatePlan(s.Options, planContext)}, s.observedSnapshot(context))
}
func EstimatePlan(options, context M) M {
	plan := v.Merge(DefaultPlan(), options)
	resource := trading.Resource(plan["resourceKey"])
	key := v.Text(resource["key"], v.Key(plan["resourceKey"]))
	for _, r := range v.Rows(context["resources"]) {
		if v.Key(r["key"]) == key {
			resource = r
			break
		}
	}
	cmd := v.Obj(context["commander"])
	aboard := float64(0)
	for _, row := range v.Rows(cmd["cargo"]) {
		if v.Key(v.First(row["key"], row["Name"])) == key {
			aboard = withDefault(v.First(row["count"], row["Count"]), 0)
		}
	}
	capacity := CargoCapacity(cmd, nil)
	target := positive(v.First(plan["targetTons"], capacity))
	var remaining, sell, purchase, margin, revenue, acquisition, operating, profit, trips, travel, mining, total, rate, observedRate, rateRun, quoteAge any
	if target != nil {
		remaining = math.Max(0, v.Num(target)-aboard)
	}
	sell = positive(v.First(plan["sellPrice"], resource["price"]))
	if resource["kind"] == "raw" {
		sell = nil
	}
	purchase = positive(plan["purchasePrice"])
	if plan["mode"] == "mining" && purchase == nil {
		purchase = float64(0)
	}
	if sell != nil && purchase != nil {
		margin = v.Num(sell) - v.Num(purchase)
	}
	runs := append([]M{v.Obj(context["current"])}, v.Rows(context["history"])...)
	for _, run := range runs {
		if v.Num(run["elapsedSeconds"]) < 60 {
			continue
		}
		for _, yield := range v.Rows(run["yields"]) {
			if yield["key"] == key && v.Num(yield["tonsPerHour"]) > 0 {
				observedRate = yield["tonsPerHour"]
				rateRun = run["id"]
				break
			}
		}
		if observedRate != nil {
			break
		}
	}
	rate = observedRate
	if v.Num(plan["tonsPerHour"]) > 0 {
		rate = positive(plan["tonsPerHour"])
	}
	if plan["mode"] == "trade" {
		mining = float64(0)
	} else if remaining != nil && v.Num(remaining) == 0 {
		mining = float64(0)
	} else if v.Num(rate) > 0 && remaining != nil {
		mining = v.Num(remaining) / v.Num(rate) * 60
	}
	if target != nil && v.Num(target) == 0 {
		trips = float64(0)
	} else if v.Num(capacity) > 0 && target != nil {
		trips = math.Ceil(v.Num(target) / v.Num(capacity))
	}
	if trips != nil && plan["tripMinutes"] != nil {
		travel = v.Num(trips) * withDefault(plan["tripMinutes"], 0)
	}
	if mining != nil && travel != nil {
		total = v.Num(mining) + v.Num(travel)
	}
	if target != nil && sell != nil {
		revenue = v.Num(target) * v.Num(sell)
	}
	if target != nil && purchase != nil {
		acquisition = v.Num(target) * v.Num(purchase)
	}
	if trips != nil && plan["operatingCost"] != nil {
		operating = v.Num(trips) * withDefault(plan["operatingCost"], 0)
	}
	if revenue != nil && acquisition != nil && operating != nil {
		profit = v.Num(revenue) - v.Num(acquisition) - v.Num(operating)
	}
	priceAt := resource["priceAt"]
	if plan["sellPrice"] != nil {
		priceAt = nil
	}
	if !v.Time(priceAt).IsZero() {
		now := v.Time(context["now"])
		if now.IsZero() {
			now = time.Now()
		}
		quoteAge = math.Max(0, now.Sub(v.Time(priceAt)).Hours())
	}
	warnings := []string{}
	if resource["kind"] == "raw" {
		warnings = append(warnings, "Сырьевые материалы не продаются на товарном рынке.")
	}
	if sell == nil {
		warnings = append(warnings, "Цена продажи неизвестна.")
	}
	if plan["mode"] == "trade" && purchase == nil {
		warnings = append(warnings, "Нужна цена закупки.")
	}
	if plan["mode"] == "mining" && mining == nil {
		warnings = append(warnings, "Скорость добычи неизвестна: задайте т/ч или накопите минимум минуту статистики.")
	}
	if capacity == nil || v.Num(capacity) == 0 {
		warnings = append(warnings, "Вместимость трюма неизвестна.")
	}
	if v.Num(quoteAge) > 24 {
		warnings = append(warnings, "Цена старше суток.")
	}
	if margin != nil && v.Num(margin) <= 0 {
		warnings = append(warnings, "Выручка с тонны не покрывает закупку.")
	}
	var profitHour, tonsForTarget, source any
	if v.Num(total) > 0 && profit != nil {
		profitHour = v.Num(profit) * 60 / v.Num(total)
	}
	if v.Num(plan["targetCredits"]) > 0 && v.Num(margin) > 0 {
		tonsForTarget = v.Num(plan["targetCredits"]) / v.Num(margin)
	}
	if v.Num(plan["tonsPerHour"]) > 0 {
		source = "manual"
	} else if v.Num(observedRate) > 0 {
		source = "journal"
	}
	return M{"key": key, "name": v.Text(resource["name"], key), "mode": plan["mode"], "targetTons": target, "aboardTons": aboard, "remainingTons": remaining, "sellPrice": sell, "purchasePrice": purchase, "unitMargin": margin, "revenue": revenue, "acquisitionCost": acquisition, "operatingCost": operating, "profit": profit, "trips": trips, "tripMinutes": positive(plan["tripMinutes"]), "travelMinutes": travel, "miningMinutes": mining, "totalMinutes": total, "tonsPerHour": rate, "rateSource": source, "rateRunId": rateRun, "priceSource": v.First(resource["priceSource"], "Цена пользователя"), "priceAt": priceAt, "quoteAgeHours": quoteAge, "estimatedProfitPerHour": profitHour, "tonsForCreditTarget": tonsForTarget, "applicable": resource["kind"] != "raw", "warnings": warnings}
}
