package mining

import (
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"sort"
	"strings"
	"time"
)

var Tiers = map[string]int{"unknown": 0, "low": 1, "medium": 2, "high": 3}
var ringNames = map[string]string{"metallic": "Металлическое", "metalrich": "Богатое металлами", "rocky": "Каменистое", "icy": "Ледяное"}
var reserveNames = map[string]string{"pristine": "Нетронутые", "major": "Богатые", "common": "Обычные", "low": "Бедные", "depleted": "Истощённые"}

func RingKey(raw any) string {
	key := strings.TrimPrefix(v.Key(raw), "eringclass")
	switch {
	case strings.Contains(key, "metalrich"):
		return "metalrich"
	case strings.Contains(key, "metallic") || strings.Contains(key, "metalic"):
		return "metallic"
	case strings.Contains(key, "rocky"):
		return "rocky"
	case strings.Contains(key, "icy") || strings.Contains(key, "ice"):
		return "icy"
	}
	return ""
}
func ReserveKey(raw any) string { return strings.TrimSuffix(v.Key(raw), "resources") }
func maxTier(resources []M) string {
	tier := "unknown"
	for _, r := range resources {
		if Tiers[v.Str(r["tier"])] > Tiers[tier] {
			tier = v.Str(r["tier"])
		}
	}
	return tier
}
func resourceView(resource M) M {
	methods := resource["methods"]
	switch values := methods.(type) {
	case []string:
		if values != nil {
			cloned := make([]string, len(values))
			copy(cloned, values)
			methods = cloned
		}
	case []any:
		if values != nil {
			cloned := make([]any, len(values))
			copy(cloned, values)
			methods = cloned
		}
	}
	return M{"key": resource["key"], "name": resource["name"], "tier": v.Text(resource["tier"], "unknown"), "methods": methods, "requiresCore": v.Bool(resource["requiresCore"])}
}
func ringResource(resource M, ringType string) M {
	r := v.Copy(resource)
	methodSet := map[string]bool{}
	available := resource["availability"]
	if available != nil {
		for _, row := range v.Rows(available) {
			matches := false
			for _, typ := range v.Arr(row["ringTypes"]) {
				matches = matches || RingKey(typ) == ringType
			}
			if matches {
				methodSet[v.Str(row["method"])] = true
			}
		}
	} else {
		for _, raw := range v.Arr(resource["methods"]) {
			methodSet[v.Str(raw)] = true
		}
	}
	methods := []string{}
	for method := range methodSet {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	r["methods"] = methods
	r["requiresCore"] = methodSet["core"] && methodSet["surface"] && !methodSet["laser"]
	return r
}
func bodiesRows(raw any) []M {
	if rows := v.Rows(raw); rows != nil {
		return rows
	}
	rows := []M{}
	for _, r := range v.Obj(raw) {
		rows = append(rows, v.Obj(r))
	}
	return rows
}
func BuildTargets(world []M, signalMap map[string][]M, resources []M, commander, settings M) []M {
	result := []M{}
	lookup := map[string]M{}
	commodities := []M{}
	for _, r := range resources {
		lookup[v.Str(r["key"])] = r
		if r["kind"] == "commodity" {
			commodities = append(commodities, r)
		}
	}
	aliases := map[string]M{}
	find := func(raw any) M {
		key := v.Key(raw)
		if known := lookup[key]; known != nil {
			return known
		}
		if cached, seen := aliases[key]; seen {
			return cached
		}
		if r := trading.Resource(raw); r != nil {
			aliases[key] = lookup[v.Str(r["key"])]
		} else {
			aliases[key] = nil
		}
		return aliases[key]
	}
	// Inputs and resource prices are constant for this evaluation. Reusing
	// per-type templates avoids JSON-copying the same resource for every ring.
	ringTemplates := map[string]M{}
	forRing := func(resource M, ringType string) M {
		if lookup[v.Str(resource["key"])] == nil {
			return ringResource(resource, ringType)
		}
		key := v.Str(resource["key"]) + "\x00" + ringType
		if cached, exists := ringTemplates[key]; exists {
			return cached
		}
		row := ringResource(resource, ringType)
		ringTemplates[key] = row
		return row
	}
	possibleByType := map[string][]M{}
	possibleFor := func(ringType string) []M {
		if cached, exists := possibleByType[ringType]; exists {
			return cached
		}
		possible := []M{}
		for _, resource := range commodities {
			available := false
			if resource["availability"] != nil {
				for _, entry := range v.Rows(resource["availability"]) {
					for _, typ := range v.Arr(entry["ringTypes"]) {
						available = available || RingKey(typ) == ringType
					}
				}
			} else {
				for _, typ := range v.Arr(resource["ringTypes"]) {
					available = available || RingKey(typ) == ringType
				}
			}
			if available {
				possible = append(possible, resource)
			}
		}
		possibleByType[ringType] = possible
		return possible
	}
	method := v.Text(settings["method"], "all")
	for _, system := range world {
		for _, body := range bodiesRows(system["bodies"]) {
			distance := trading.Distance(commander["coordinates"], system["coordinates"])
			var ly any
			if !math.IsNaN(distance) {
				ly = distance
			}
			base := M{"system": system["name"], "systemAddress": system["address"], "body": body["name"], "bodyId": body["id"], "distanceLy": ly, "distanceLs": body["distanceLs"], "gravity": body["gravity"], "landable": v.Bool(body["landable"]), "source": v.First(body["source"], "journal"), "updatedAt": v.First(body["timestamp"], system["timestamp"]), "coordinates": system["coordinates"]}
			for _, ring := range v.Rows(body["rings"]) {
				if method == "surface-planet" || method == "raw" || strings.Contains(strings.ToLower(v.Str(ring["name"])), "belt") {
					continue
				}
				typ, reserve := RingKey(ring["type"]), ReserveKey(body["reserve"])
				signals, ok := signalMap[v.Str(system["key"])+":"+v.Str(ring["name"])]
				if !ok {
					signals = v.Rows(ring["signals"])
				}
				hotspots := []M{}
				for _, signal := range signals {
					symbol := v.First(signal["key"], signal["Type"], signal["name"])
					resource := find(symbol)
					count := v.Num(v.First(signal["Count"], signal["count"]))
					if count <= 0 {
						count = 1
					}
					if resource == nil {
						hotspots = append(hotspots, M{"key": v.Key(symbol), "name": v.Text(signal["Type_Localised"], signal["name"], symbol), "tier": "unknown", "methods": []string{}, "count": count})
						continue
					}
					if resource["kind"] != "commodity" {
						continue
					}
					hotspots = append(hotspots, v.Merge(resourceView(forRing(resource, typ)), M{"count": count}))
				}
				possible := possibleFor(typ)
				candidates := hotspots
				if len(candidates) == 0 {
					for _, r := range possible {
						if Tiers[v.Str(r["tier"])] >= 2 {
							candidates = append(candidates, r)
						}
					}
				}
				useful := []M{}
				for _, r := range candidates {
					resource := find(r["key"])
					if resource == nil {
						resource = r
					}
					resource = forRing(resource, typ)
					if method == "all" || has(resource["methods"], method) {
						useful = append(useful, resource)
					}
				}
				if method != "all" && len(useful) == 0 {
					continue
				}
				tier := maxTier(useful)
				score := 15.0
				if tier == "high" {
					score = 70
				} else if tier == "medium" {
					score = 45
				}
				reasons := []string{}
				if len(hotspots) > 0 {
					names := []string{}
					unknown := false
					for _, r := range hotspots {
						names = append(names, v.Str(r["name"]))
						unknown = unknown || r["tier"] == "unknown"
					}
					reasons = append(reasons, "Горячие точки подтверждены сканированием: "+strings.Join(names, ", "))
					score += 15
					if unknown {
						reasons = append(reasons, "Часть сигналов отсутствует в справочнике; их ценность неизвестна.")
					}
				} else {
					reasons = append(reasons, "Оценка по типу кольца. Нужны зонды ПСП и разведчик.")
					if typ == "icy" {
						tier = "low"
						score = 22
					} else if tier == "high" {
						tier = "medium"
						score = 50
					}
				}
				methodSet := map[string]bool{}
				for _, r := range useful {
					for _, m := range v.Arr(r["methods"]) {
						methodSet[v.Str(m)] = true
					}
				}
				methods := []string{}
				for m := range methodSet {
					methods = append(methods, m)
				}
				sort.Strings(methods)
				laser := methodSet["laser"] && (method == "all" || method == "laser")
				if laser && reserve == "pristine" {
					score += 10
					reasons = append(reasons, "Нетронутые запасы — преимущество для лазерной добычи.")
				} else if laser && (reserve == "low" || reserve == "depleted") {
					score -= 18
					reasons = append(reasons, "Для лазерной добычи запасы слабые. Ядра оценивайте отдельно.")
				}
				if typ == "metalrich" {
					for _, r := range candidates {
						if r["key"] == "platinum" || r["key"] == "painite" {
							reasons = append(reasons, "Платина и пейнит здесь требуют разработки ядра. Для лазерной добычи ищите металлическое кольцо.")
							break
						}
					}
				}
				if method == "surface" {
					for _, r := range useful {
						if v.Bool(r["requiresCore"]) {
							reasons = append(reasons, "Сначала нужно раскрыть ядро сейсмическими зарядами, затем снять отложения абразивным бластером.")
							break
						}
					}
				}
				if v.Num(body["distanceLs"]) > 10000 {
					score -= 8
					reasons = append(reasons, "Долгий перелёт от звезды прибытия.")
				}
				resourceViews := []M{}
				for _, r := range useful {
					resourceViews = append(resourceViews, resourceView(r))
				}
				confidence, recommendation := "potential", "Разведать кольцо перед добычей."
				if len(hotspots) > 0 {
					confidence = "confirmed"
					recommendation = "Выберите метод добычи и проверьте астероид разведчиком."
				}
				result = append(result, v.Merge(v.Copy(base), M{"id": v.Str(system["key"]) + ":" + v.Str(body["id"]) + ":ring:" + v.Str(ring["name"]), "kind": "ring", "ring": ring["name"], "ringType": v.Text(ringNames[typ], "Тип неизвестен"), "ringTypeKey": typ, "reserve": v.Text(reserveNames[reserve], "Запасы неизвестны"), "reserveKey": reserve, "tier": tier, "score": v.Clamp(score, 0, 100), "confidence": confidence, "hotspots": hotspots, "materials": []M{}, "resources": resourceViews, "methods": methods, "reasons": reasons, "recommendation": recommendation, "miningSites": 0, "compositionUnknown": false}))
			}
			materials, valuable := []M{}, []M{}
			for _, material := range v.Rows(body["materials"]) {
				r := find(v.First(material["key"], material["Name"]))
				if r == nil || r["kind"] != "raw" {
					continue
				}
				row := M{"key": r["key"], "name": r["name"], "percent": v.First(material["percent"], material["Percent"], 0), "grade": v.First(r["grade"], 1), "tier": r["tier"]}
				materials = append(materials, row)
				if v.Num(row["grade"]) >= 3 {
					valuable = append(valuable, row)
				}
			}
			sort.Slice(materials, func(i, j int) bool {
				if v.Num(materials[i]["grade"]) != v.Num(materials[j]["grade"]) {
					return v.Num(materials[i]["grade"]) > v.Num(materials[j]["grade"])
				}
				return v.Num(materials[i]["percent"]) > v.Num(materials[j]["percent"])
			})
			miningSites, geology := float64(0), float64(0)
			for _, signal := range signalMap[v.Str(system["key"])+":"+v.Str(body["name"])] {
				name := strings.ToLower(v.Text(signal["Type"], signal["key"], signal["name"]))
				count := v.Num(v.First(signal["Count"], signal["count"]))
				if strings.Contains(name, "planetarymininglocation") {
					miningSites += count
				}
				if strings.Contains(name, "geological") || strings.Contains(name, "geology") {
					geology += count
				}
			}
			if miningSites == 0 && !(v.Bool(body["landable"]) && len(valuable) > 0) {
				continue
			}
			if method != "all" && method != "raw" && method != "surface-planet" {
				continue
			}
			if method == "surface-planet" && miningSites == 0 || method == "raw" && !(v.Bool(body["landable"]) && len(valuable) > 0) {
				continue
			}
			showRaw := method != "surface-planet"
			reasons := []string{}
			observed := map[string]M{}
			finds := v.Rows(body["surfaceFinds"])
			for _, site := range finds {
				names := []string{}
				for _, entry := range v.Rows(site["resources"]) {
					key := v.Str(entry["key"])
					previous := observed[key]
					if previous == nil {
						observed[key] = v.Copy(entry)
					} else {
						increment(previous, "count", v.Num(entry["count"]))
					}
					names = append(names, fmt.Sprintf("%s %g т", v.Str(entry["name"]), v.Num(entry["count"])))
				}
				if len(names) > 0 {
					reasons = append(reasons, v.Str(site["site"])+". По журналу: "+strings.Join(names, ", "))
				}
			}
			observedRows, observedResources := []M{}, []M{}
			for _, r := range observed {
				observedRows = append(observedRows, r)
				if resource := find(r["key"]); resource != nil {
					observedResources = append(observedResources, resource)
				}
			}
			if miningSites > 0 {
				reasons = append(reasons, fmt.Sprintf("Сканер подтвердил участки планетарной добычи: %g. Состав требует разведки.", miningSites))
			}
			if showRaw && len(valuable) > 0 {
				reasons = append(reasons, "Есть сырьё высоких категорий для инженерии.")
			}
			if geology > 0 {
				reasons = append(reasons, fmt.Sprintf("Геологических сигналов: %g.", geology))
			}
			if v.Num(body["gravity"]) > 1.5 {
				reasons = append(reasons, "Высокая гравитация усложняет посадку.")
			}
			tier := "unknown"
			grade4 := false
			for _, r := range valuable {
				grade4 = grade4 || v.Num(r["grade"]) == 4
			}
			if showRaw && grade4 {
				tier = "high"
			} else if showRaw && len(valuable) > 0 || Tiers[maxTier(observedResources)] >= 2 {
				tier = "medium"
			}
			score := 35.0
			if tier == "high" {
				score = 70
			} else if tier == "medium" {
				score = 48
			}
			if miningSites > 0 {
				score += 10
			}
			if geology > 0 {
				score += 8
			}
			if v.Num(body["gravity"]) > 1.5 {
				score -= 10
			}
			methods, resourceViews := []string{}, []M{}
			if miningSites > 0 && method != "raw" {
				methods = append(methods, "surface-planet")
			}
			if showRaw && len(valuable) > 0 {
				methods = append(methods, "raw")
				for _, r := range valuable {
					resourceViews = append(resourceViews, resourceView(find(r["key"])))
				}
			}
			recommendation := "Посадка ради сырья для инженеров и синтеза. Проценты не определяют скорость сбора."
			if miningSites > 0 && method != "raw" {
				recommendation = "Выберите участок в навигации и разведайте залежь на Rhino."
			}
			result = append(result, v.Merge(v.Copy(base), M{"id": v.Str(system["key"]) + ":" + v.Str(body["id"]) + ":surface", "kind": "surface", "ring": "", "ringType": "", "reserve": "", "tier": tier, "score": math.Min(100, score), "confidence": "confirmed", "hotspots": []M{}, "materials": materials, "resources": resourceViews, "methods": methods, "miningSites": miningSites, "compositionUnknown": miningSites > 0 && method != "raw", "geology": geology, "observedResources": observedRows, "siteFinds": finds, "reasons": reasons, "recommendation": recommendation}))
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if v.Num(result[i]["score"]) != v.Num(result[j]["score"]) {
			return v.Num(result[i]["score"]) > v.Num(result[j]["score"])
		}
		if result[i]["distanceLy"] == nil {
			return false
		}
		if result[j]["distanceLy"] == nil {
			return true
		}
		return v.Num(result[i]["distanceLy"]) < v.Num(result[j]["distanceLy"])
	})
	return result
}
func AssessProspect(prospect, settings M, resources []M) M {
	if prospect == nil {
		return nil
	}
	lookup := map[string]M{}
	for _, r := range resources {
		lookup[v.Str(r["key"])] = r
	}
	priced := []M{}
	for _, entry := range v.Rows(prospect["materials"]) {
		r := trading.Resource(entry["key"])
		row := v.Copy(entry)
		row["name"] = v.First(r["name"], entry["name"])
		row["tier"] = v.Text(r["tier"], "unknown")
		row["price"] = lookup[v.Str(entry["key"])]["price"]
		priced = append(priced, row)
	}
	sort.Slice(priced, func(i, j int) bool {
		if Tiers[v.Str(priced[i]["tier"])] != Tiers[v.Str(priced[j]["tier"])] {
			return Tiers[v.Str(priced[i]["tier"])] > Tiers[v.Str(priced[j]["tier"])]
		}
		return v.Num(priced[i]["percent"]) > v.Num(priced[j]["percent"])
	})
	core := trading.Resource(prospect["motherlode"])
	threshold := Tiers[v.Text(settings["minTier"], "medium")]
	method := v.Text(settings["method"], "all")
	allowCore, allowLaser := method == "all" || method == "core", method == "all" || method == "laser"
	minPercent := withDefault(settings["minLaserPercent"], 15)
	reasons := []string{}
	decision := "skip"
	if allowCore && core != nil && Tiers[v.Str(core["tier"])] >= threshold && !v.Bool(prospect["coreCracked"]) {
		decision = "mine"
		reasons = append(reasons, "Ядро: "+v.Str(core["name"])+". Нужен комплект для подрыва.")
	}
	laser := []string{}
	for _, r := range priced {
		if Tiers[v.Str(r["tier"])] >= threshold && v.Num(r["percent"]) >= minPercent && has(trading.Resource(r["key"])["methods"], "laser") {
			laser = append(laser, fmt.Sprintf("%s %g%%", v.Str(r["name"]), v.Num(r["percent"])))
		}
	}
	if allowLaser && len(laser) > 0 && (prospect["remaining"] == nil || v.Num(prospect["remaining"]) != 0) {
		decision = "mine"
		reasons = append(reasons, "Подходящий лазерный состав: "+strings.Join(laser, ", "))
	}
	if prospect["remaining"] != nil && v.Num(prospect["remaining"]) == 0 && decision != "mine" {
		reasons = append([]string{"Астероид выработан."}, reasons...)
	} else if decision == "skip" {
		reasons = append(reasons, fmt.Sprintf("Нет ресурсов выбранного приоритета. Порог для лазеров %g%%.", minPercent))
	}
	if len(priced) == 0 && core == nil {
		decision = "unknown"
		reasons = []string{"Недостаточно данных о составе."}
	}
	if !allowCore && !allowLaser {
		decision = "unknown"
		reasons = []string{"Разведчик не подтверждает отдельные залежи выбранного метода. Нужна проверка подходящим сканером."}
	} else if v.Bool(prospect["coreCracked"]) && allowCore && !allowLaser {
		decision = "unknown"
		reasons = []string{"После разведки зарегистрирован раскол астероида. Проверьте оставшееся ядро."}
	}
	var estimated any
	known, total := false, float64(0)
	for _, r := range priced {
		if r["price"] != nil && has(trading.Resource(r["key"])["methods"], "laser") {
			known = true
			total += v.Num(r["price"]) * v.Num(r["percent"]) / 100
		}
	}
	if known {
		estimated = math.Round(total)
	}
	return v.Merge(v.Copy(prospect), M{"materials": priced, "decision": decision, "reasons": reasons, "estimatedPerTonne": estimated})
}
func nowISO() string { return v.ISO(time.Now()) }
