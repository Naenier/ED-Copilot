package presentation

import (
	"sort"
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/colonization"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func hudAdd(doc *HUDDocument, title string, lines ...HUDLine) {
	clean, seen := []HUDLine{}, map[string]bool{}
	for _, line := range lines {
		line.Text = strings.TrimSpace(line.Text)
		if line.Text != "" && !seen[line.Text] {
			clean = append(clean, line)
			seen[line.Text] = true
		}
	}
	if len(clean) > 0 {
		doc.Sections = append(doc.Sections, HUDSection{title, clean})
	}
}
func hudText(text string) HUDLine  { return HUDLine{text, "text"} }
func hudMuted(text string) HUDLine { return HUDLine{text, "muted"} }
func hudWarn(text string) HUDLine  { return HUDLine{text, "warn"} }
func hudKnown(raw any) bool        { n, known := v.Number(raw); return known && n >= 0 }
func hudEqual(a, b any) bool {
	return v.Str(a) != "" && strings.EqualFold(strings.TrimSpace(v.Str(a)), strings.TrimSpace(v.Str(b)))
}
func hudContextMatches(record, commander M) bool {
	for _, field := range []string{"system", "body"} {
		if v.Str(record[field]) != "" && v.Str(commander[field]) != "" && !hudEqual(record[field], commander[field]) {
			return false
		}
	}
	return true
}

func buildHUDContent(s M, now time.Time) HUDDocument {
	activity := v.Text(v.At(s, "settings", "activity"), "flight")
	doc := HUDDocument{Activity: activity}
	switch activity {
	case "exobiology":
		hudExobiology(&doc, s)
	case "asteroid":
		hudAsteroid(&doc, s, now)
	case "surface":
		hudSurface(&doc, s)
	case "raw":
		hudMaterials(&doc, s, true)
		hudTargets(&doc, s, "raw")
	case "trade":
		hudDestination(&doc, s, now)
		hudTradePlan(&doc, s)
	case "carrier":
		hudCarrier(&doc, s, now)
	case "engineering":
		hudMaterials(&doc, s, false)
		hudExchange(&doc, s)
	case "colonization":
		hudColony(&doc, s, now)
	default:
		hudFlight(&doc, s)
	}
	hudCompactContext(&doc, v.Obj(s["commander"]))
	return doc
}

// Location supports the current task rather than occupying a permanent panel.
// A body or destination already named by a working section is not repeated.
func hudCompactContext(doc *HUDDocument, cmd M) {
	if len(doc.Sections) == 0 {
		return
	}
	parts := []string{v.Str(cmd["system"])}
	if v.Bool(cmd["docked"]) {
		parts = append(parts, v.Str(cmd["station"]))
	} else if cmd["vehicle"] == "foot" || cmd["vehicle"] == "srv" {
		parts = append(parts, v.Str(cmd["body"]))
	}
	known := []string{}
	for _, section := range doc.Sections {
		known = append(known, strings.ToLower(section.Title))
		for _, line := range section.Lines {
			known = append(known, strings.ToLower(line.Text))
		}
	}
	missing, seen := []string{}, map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		key := strings.ToLower(part)
		if part == "" || seen[key] {
			continue
		}
		seen[key] = true
		mentioned := false
		for _, text := range known {
			if strings.Contains(text, key) {
				mentioned = true
				break
			}
		}
		if !mentioned {
			missing = append(missing, part)
		}
	}
	if len(missing) > 0 {
		doc.Sections[0].Lines = append(doc.Sections[0].Lines, hudMuted(strings.Join(missing, " · ")))
	}
}

func hudCargo(s M, empty, limpets bool) []HUDLine {
	cmd, cargo := v.Obj(s["commander"]), v.Obj(v.At(s, "mining", "cargo"))
	used, capacity := v.First(cargo["tons"], cmd["cargoTons"]), v.First(cargo["capacity"], cmd["cargoCapacity"])
	if cmd["vehicle"] == "srv" {
		capacity = v.First(cargo["capacity"], cmd["srvCargoCapacity"])
	}
	rows := []HUDLine{}
	if hudKnown(used) && (v.Num(used) > 0 || empty) {
		text, role := "Груз: "+Number(used)+" т", "text"
		if hudKnown(capacity) {
			text = "Трюм: " + Number(used) + " / " + Number(capacity) + " т"
			if v.Num(capacity) == 0 && v.Num(used) == 0 {
				text, role = "Грузовой трюм отсутствует", "warn"
			} else if v.Num(capacity) <= v.Num(used) {
				text = "Трюм полон: " + Number(used) + " / " + Number(capacity) + " т"
				role = "warn"
			} else {
				text += " · свободно " + Number(v.Num(capacity)-v.Num(used)) + " т"
			}
		}
		rows = append(rows, HUDLine{text, role})
	} else if empty && hudKnown(capacity) && v.Num(capacity) > 0 {
		rows = append(rows, hudMuted("Трюм: "+Number(capacity)+" т · груз неизвестен"))
	}
	if limpets && hudKnown(cargo["limpets"]) {
		if v.Num(cargo["limpets"]) == 0 {
			rows = append(rows, hudWarn("Дронов в трюме нет"))
		} else {
			rows = append(rows, hudMuted("Дроны: "+Number(cargo["limpets"])))
		}
	}
	return rows
}
func hudFlight(doc *HUDDocument, s M) {
	cmd := v.Obj(s["commander"])
	target := v.Obj(v.At(s, "pilot", "navTarget"))
	if v.Str(target["system"]) == "" || hudEqual(target["system"], cmd["system"]) {
		for _, row := range v.Rows(v.At(s, "pilot", "navRoute")) {
			if !v.Bool(row["visited"]) && !hudEqual(row["system"], cmd["system"]) {
				target = row
				break
			}
		}
	}
	if v.Str(target["system"]) != "" && !hudEqual(target["system"], cmd["system"]) {
		rows := []HUDLine{hudText(v.Str(target["system"]))}
		facts := []string{}
		star := v.Str(target["starClass"])
		if star == "" {
			for _, row := range v.Rows(v.At(s, "pilot", "navRoute")) {
				if hudEqual(row["system"], target["system"]) {
					star = v.Str(row["starClass"])
					break
				}
			}
		}
		if star != "" {
			facts = append(facts, "Звезда "+star)
		}
		if v.Num(target["remainingJumps"]) > 0 {
			facts = append(facts, "Осталось "+Number(target["remainingJumps"])+" прыжков")
		}
		rows = append(rows, hudMuted(strings.Join(facts, " · ")))
		hudAdd(doc, "СЛЕДУЮЩИЙ ПРЫЖОК", rows...)
	} else {
		goals := append([]M(nil), v.Rows(s["objectives"])...)
		sort.SliceStable(goals, func(i, j int) bool { return goals[i]["priority"] == "high" && goals[j]["priority"] != "high" })
		for _, goal := range goals {
			if (goal["status"] == "active" || goal["status"] == "planned") && v.Str(goal["title"]) != "" {
				rows := []HUDLine{hudText(v.Str(goal["title"]))}
				if v.Str(goal["system"]) != "" && !hudEqual(goal["system"], cmd["system"]) {
					rows = append(rows, hudMuted(v.Str(goal["system"])))
				}
				hudAdd(doc, "ЛИЧНАЯ ЦЕЛЬ", rows...)
				break
			}
		}
	}
	fuel := v.Obj(v.At(s, "pilot", "fuel"))
	rows := []HUDLine{}
	if hudKnown(fuel["mainTons"]) {
		text := "Топливо: " + Number(fuel["mainTons"])
		if hudKnown(fuel["mainCapacityTons"]) {
			text += " / " + Number(fuel["mainCapacityTons"])
		}
		text += " т"
		role := "text"
		if v.Num(fuel["mainTons"]) == 0 {
			role = "warn"
		}
		rows = append(rows, HUDLine{text, role})
	}
	rows = append(rows, hudCargo(s, false, false)...)
	hudAdd(doc, "ТОПЛИВО И ГРУЗ", rows...)
}
func hudAsteroid(doc *HUDDocument, s M, now time.Time) {
	cmd, p := v.Obj(s["commander"]), v.Obj(s["prospect"])
	if len(p) > 0 && !v.Bool(cmd["docked"]) && hudContextMatches(p, cmd) {
		decision := map[string]string{"mine": "Можно добывать", "skip": "Пропустить астероид", "unknown": "Состав требует проверки"}[v.Str(p["decision"])]
		if decision == "" {
			decision = "Состав требует проверки"
		}
		role := "warn"
		if p["decision"] == "mine" {
			role = "good"
		}
		rows := []HUDLine{{decision, role}}
		// A zero laser remainder does not cancel an assessed, uncracked core.
		// AssessProspect deliberately keeps that core mineable independently.
		coreAvailable := v.Str(p["motherlode"]) != "" && !v.Bool(p["coreCracked"]) && p["decision"] == "mine"
		depleted := hudKnown(p["remaining"]) && v.Num(p["remaining"]) == 0 && !coreAvailable
		if depleted {
			rows = []HUDLine{hudWarn("Астероид истощён · выберите следующий")}
		} else {
			if core := v.Str(p["motherlode"]); core != "" {
				text := "Ядро: " + hudGood(s, core)
				if v.Bool(p["coreCracked"]) {
					text += " · расколото"
				}
				rows = append(rows, hudText(text))
			}
			materials := append([]M(nil), v.Rows(p["materials"])...)
			sort.SliceStable(materials, func(i, j int) bool {
				a, b := hudTier(materials[i]["tier"]), hudTier(materials[j]["tier"])
				if a != b {
					return a > b
				}
				return v.Num(materials[i]["percent"]) > v.Num(materials[j]["percent"])
			})
			shown := 0
			minimum := hudTier(v.First(v.At(s, "settings", "minTier"), "medium"))
			for _, material := range materials {
				if !hudKnown(material["percent"]) || v.Num(material["percent"]) <= 0 {
					continue
				}
				if tier := hudTier(material["tier"]); tier > 0 && tier < minimum {
					continue
				}
				rows = append(rows, hudText(v.Text(material["name"], material["key"])+" · "+Number(material["percent"])+" %"))
				shown++
				if shown == 3 {
					break
				}
			}
			if hudKnown(p["remaining"]) {
				rows = append(rows, hudMuted("Осталось в астероиде: "+Number(p["remaining"])+" %"))
			}
		}
		hudAdd(doc, "СКАН АСТЕРОИДА", rows...)
	}
	rows := hudCargo(s, true, true)
	current := v.Obj(v.At(s, "mining", "current"))
	if hudContextMatches(current, cmd) && current["endedAt"] == nil && v.Num(current["refinedTons"]) > 0 {
		text := "За заход: " + Number(current["refinedTons"]) + " т"
		if v.Num(current["tonsPerHour"]) > 0 {
			text += " · " + Number(current["tonsPerHour"]) + " т/ч"
		}
		rows = append(rows, hudMuted(text))
	}
	hudAdd(doc, "ДОБЫЧА", rows...)
	if hudMiningDestinationRelevant(s) {
		hudDestination(doc, s, now)
	}
}
func hudMiningDestinationRelevant(s M) bool {
	d := v.Obj(v.At(s, "trading", "destination"))
	if len(d) == 0 || d["selectionMode"] == "buy" {
		return false
	}
	cargo := v.Rows(v.At(s, "commander", "cargo"))
	if len(cargo) == 0 {
		return v.Num(v.At(s, "commander", "cargoTons")) > 0
	}
	for _, q := range v.Rows(d["quotes"]) {
		for _, item := range cargo {
			if v.Key(v.First(item["key"], item["Name"])) == v.Key(q["key"]) && v.Num(v.First(item["count"], item["Count"])) > 0 {
				return true
			}
		}
	}
	return false
}
func hudSurface(doc *HUDDocument, s M) {
	surface, cmd := v.Obj(s["surface"]), v.Obj(s["commander"])
	rows := []HUDLine{}
	if hudContextMatches(surface, cmd) && !v.Bool(cmd["docked"]) {
		if site := v.Str(surface["site"]); site != "" {
			rows = append(rows, hudText(site))
		}
		for _, r := range v.Rows(surface["lastRefined"]) {
			if v.Num(r["count"]) > 0 {
				rows = append(rows, hudText(v.Text(r["name"], r["key"])+": "+Number(r["count"])+" т"))
				if len(rows) >= 4 {
					break
				}
			}
		}
	}
	if len(rows) > 0 {
		hudAdd(doc, "ТЕКУЩИЙ УЧАСТОК", rows...)
	} else {
		hudTargets(doc, s, "surface")
	}
	hudAdd(doc, "ГРУЗ ПОВЕРХНОСТИ", hudCargo(s, true, false)...)
}
func hudMaterialRows(s M, rawOnly bool) ([]M, int) {
	rows := []M{}
	unknown := 0
	for _, r := range v.Rows(v.At(s, "engineering", "materials")) {
		if rawOnly && r["category"] != "raw" {
			continue
		}
		if v.Num(r["needed"]) <= 0 && v.Num(r["missing"]) <= 0 {
			continue
		}
		if !hudKnown(r["missing"]) && v.Num(r["needed"]) > 0 {
			unknown++
			rows = append(rows, r)
		} else if v.Num(r["missing"]) > 0 {
			rows = append(rows, r)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if v.Num(rows[i]["missing"]) != v.Num(rows[j]["missing"]) {
			return v.Num(rows[i]["missing"]) > v.Num(rows[j]["missing"])
		}
		return v.Str(rows[i]["name"]) < v.Str(rows[j]["name"])
	})
	return rows, unknown
}
func hudMaterials(doc *HUDDocument, s M, rawOnly bool) {
	if len(v.Rows(v.At(s, "engineering", "items"))) == 0 {
		return
	}
	materials, unknown := hudMaterialRows(s, rawOnly)
	if rawOnly {
		hasRawNeed := false
		for _, r := range v.Rows(v.At(s, "engineering", "materials")) {
			hasRawNeed = hasRawNeed || r["category"] == "raw" && (v.Num(r["needed"]) > 0 || v.Num(r["missing"]) > 0)
		}
		if !hasRawNeed {
			return
		}
	}
	rows := []HUDLine{}
	for i, r := range materials {
		if i == 3 {
			break
		}
		text, role := v.Text(r["name"], r["key"])+": нужно ещё "+Number(r["missing"]), "text"
		if !hudKnown(r["missing"]) {
			text, role = v.Text(r["name"], r["key"])+": нужно "+Number(r["needed"])+" · запас неизвестен", "warn"
		} else if hudKnown(r["owned"]) {
			text += " · есть " + Number(r["owned"])
		}
		rows = append(rows, HUDLine{text, role})
	}
	if rawOnly && !v.Bool(v.At(s, "engineering", "shopping", "knownCategories", "raw")) {
		unknown = max(unknown, 1)
	} else if !rawOnly {
		unknown = max(unknown, v.Int(v.At(s, "engineering", "shopping", "unknownMaterials")))
	}
	if unknown > 0 {
		text := "Часть запасов материалов неизвестна"
		if rawOnly {
			text = "Запасы сырья ещё неизвестны"
		}
		rows = append(rows, hudWarn(text))
	}
	if len(rows) == 0 && v.Num(v.At(s, "engineering", "shopping", "neededUnits")) > 0 {
		text := "Материалы плана собраны"
		if rawOnly {
			text = "Сырьё для плана собрано"
		}
		rows = append(rows, HUDLine{text, "good"})
	}
	title := "МАТЕРИАЛЫ ДЛЯ УЛУЧШЕНИЙ"
	if rawOnly {
		title = "НУЖНОЕ СЫРЬЁ"
	}
	hudAdd(doc, title, rows...)
}
func hudExchange(doc *HUDDocument, s M) {
	if len(v.Rows(v.At(s, "engineering", "items"))) == 0 {
		return
	}
	for _, offer := range v.Rows(v.At(s, "engineering", "trader", "offers")) {
		if v.Bool(offer["affordable"]) && v.Bool(offer["storageFits"]) && v.Num(offer["requiredFrom"]) > 0 && v.Num(offer["received"]) > 0 {
			hudAdd(doc, "ДОСТУПНЫЙ ОБМЕН", hudText(Number(offer["requiredFrom"])+" × "+v.Str(offer["fromName"])+" → "+Number(offer["received"])+" × "+v.Str(offer["toName"])))
			break
		}
	}
}
func hudTier(raw any) int { return map[string]int{"low": 1, "medium": 2, "high": 3}[v.Str(raw)] }
func hudTargets(doc *HUDDocument, s M, kind string) {
	cmd := v.Obj(s["commander"])
	radius := 50.0
	if n, ok := v.Number(v.At(s, "settings", "radiusLy")); ok && n > 0 {
		radius = min(n, 100)
	}
	minimum := max(hudTier("medium"), hudTier(v.At(s, "settings", "minTier")))
	needed, _ := hudMaterialRows(s, true)
	wanted := map[string]bool{}
	for _, r := range needed {
		wanted[v.Key(r["key"])] = true
	}
	candidates := []M{}
	for _, r := range v.Rows(s["targets"]) {
		if r["kind"] != "surface" || v.Str(r["body"]) == "" {
			continue
		}
		same := hudEqual(r["system"], cmd["system"])
		distance, known := v.Number(r["distanceLy"])
		if !same && (!known || distance < 0 || distance > radius) {
			continue
		}
		if hudTier(r["tier"]) < minimum {
			continue
		}
		if kind == "raw" {
			materials := v.Rows(r["materials"])
			if !v.Bool(r["landable"]) || len(materials) == 0 {
				continue
			}
			if len(wanted) > 0 {
				match := false
				for _, m := range materials {
					match = match || wanted[v.Key(m["key"])]
				}
				if !match {
					continue
				}
			}
		} else {
			useful := v.Num(r["miningSites"]) > 0 || len(v.Rows(r["observedResources"])) > 0 || len(v.Rows(r["siteFinds"])) > 0
			for _, method := range v.Arr(r["methods"]) {
				useful = useful || method == "surface-planet"
			}
			if !useful {
				continue
			}
		}
		candidates = append(candidates, r)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := hudEqual(candidates[i]["system"], cmd["system"]), hudEqual(candidates[j]["system"], cmd["system"])
		if a != b {
			return a
		}
		if v.Num(candidates[i]["distanceLy"]) != v.Num(candidates[j]["distanceLy"]) {
			return v.Num(candidates[i]["distanceLy"]) < v.Num(candidates[j]["distanceLy"])
		}
		if v.Num(candidates[i]["score"]) != v.Num(candidates[j]["score"]) {
			return v.Num(candidates[i]["score"]) > v.Num(candidates[j]["score"])
		}
		return v.Str(candidates[i]["id"]) < v.Str(candidates[j]["id"])
	})
	rows := []HUDLine{}
	seen := map[string]bool{}
	for _, r := range candidates {
		id := v.Str(r["system"]) + "|" + v.Str(r["body"])
		if seen[id] {
			continue
		}
		seen[id] = true
		label := v.Str(r["body"])
		if !hudEqual(r["system"], cmd["system"]) {
			label = v.Str(r["system"]) + " · " + label
		}
		rows = append(rows, hudText(label))
		facts := []string{}
		if hudEqual(r["system"], cmd["system"]) {
			facts = append(facts, "В этой системе")
		} else {
			facts = append(facts, Number(r["distanceLy"])+" св. л.")
		}
		if kind == "raw" {
			names := []string{}
			for _, m := range v.Rows(r["materials"]) {
				if len(wanted) == 0 || wanted[v.Key(m["key"])] {
					names = append(names, v.Text(m["name"], hudGood(s, v.Str(m["key"]))))
					if len(names) == 2 {
						break
					}
				}
			}
			facts = append(facts, strings.Join(names, ", "))
		} else if v.Bool(r["compositionUnknown"]) {
			facts = append(facts, "Состав залежей неизвестен")
		} else if n := v.Num(r["miningSites"]); n > 0 {
			facts = append(facts, "Участков: "+Number(n))
		}
		rows = append(rows, hudMuted(strings.Join(facts, " · ")))
		if len(seen) == 2 {
			break
		}
	}
	hudAdd(doc, "МЕСТА ПОБЛИЗОСТИ", rows...)
}
func hudTradePlan(doc *HUDDocument, s M) {
	plan, cmd := v.Obj(s["tradePlan"]), v.Obj(s["commander"])
	rows := []HUDLine{}
	if v.Str(plan["key"]) != "" && v.Num(v.First(plan["quantity"], plan["targetTons"])) > 0 {
		text := v.Text(plan["name"], hudGood(s, v.Str(plan["key"]))) + " · " + Number(v.First(plan["quantity"], plan["targetTons"])) + " т"
		for _, r := range v.Rows(cmd["cargo"]) {
			if v.Key(r["key"]) == v.Key(plan["key"]) && hudKnown(r["count"]) {
				text += " · на борту " + Number(r["count"]) + " т"
				break
			}
		}
		rows = append(rows, hudText(text))
		if margin := v.First(plan["grossMargin"], plan["profit"]); margin != nil {
			rows = append(rows, hudMuted("Плановая маржа: "+Number(margin)+" кр."))
		}
	}
	rows = append(rows, hudCargo(s, false, false)...)
	hudAdd(doc, "ПЕРЕВОЗКА", rows...)
}
func hudDestination(doc *HUDDocument, s M, now time.Time) {
	d, cmd := v.Obj(v.At(s, "trading", "destination")), v.Obj(s["commander"])
	if len(d) == 0 {
		return
	}
	buying := d["selectionMode"] == "buy"
	title := "ПРОДАЖА ГРУЗА"
	if buying {
		title = "МЕСТО ЗАКУПКИ"
	}
	rows := []HUDLine{}
	arrived := v.Bool(cmd["docked"]) && hudEqual(cmd["station"], d["station"]) && hudEqual(cmd["system"], d["system"])
	if arrived {
		title += " · НА МЕСТЕ"
	} else {
		text := v.Text(d["station"], "Станция неизвестна")
		if v.Str(d["system"]) != "" && !hudEqual(d["system"], cmd["system"]) {
			text += " · " + v.Str(d["system"])
		}
		if hudKnown(d["distanceLy"]) {
			text += " · " + Number(d["distanceLy"]) + " св. л."
		}
		rows = append(rows, hudText(text))
	}
	quotes := append([]M(nil), v.Rows(d["quotes"])...)
	if len(quotes) == 0 && d["price"] != nil {
		quotes = []M{d}
	}
	planKey := v.Key(v.At(s, "tradePlan", "key"))
	if planKey != "" {
		sort.SliceStable(quotes, func(i, j int) bool { return v.Key(quotes[i]["key"]) == planKey && v.Key(quotes[j]["key"]) != planKey })
	}
	for i, q := range quotes {
		if i == 2 {
			break
		}
		availability, amount := "спрос", q["demand"]
		if buying {
			availability, amount = "запас", q["supply"]
		}
		text := v.Text(q["name"], hudGood(s, v.Str(q["key"]))) + ": " + Number(q["price"]) + " кр./т · " + availability + " " + Number(amount) + " т"
		role := "text"
		if hudKnown(amount) && v.Num(amount) == 0 {
			role = "warn"
		}
		rows = append(rows, HUDLine{text, role})
	}
	at := v.First(d["sourceUpdatedAt"], d["updatedAt"], d["timestamp"])
	date := v.Time(at)
	expired := v.Bool(d["expired"]) || date.IsZero() || date.After(now.Add(10*time.Minute))
	if !date.IsZero() {
		limit := 24.0
		if v.Bool(d["carrier"]) {
			limit = 6
		}
		if n, ok := v.Number(v.At(d, "quoteOptions", map[bool]string{false: "maxAgeHours", true: "carrierMaxAgeHours"}[v.Bool(d["carrier"])])); ok {
			limit = n
		}
		expired = expired || now.Sub(date).Hours() > limit
	}
	flags := []string{}
	if expired {
		flags = append(flags, "Котировка устарела / время не подтверждено")
	} else {
		flags = append(flags, "Цена: "+Age(at, now))
	}
	if v.Bool(d["cargoChanged"]) {
		flags = append(flags, "Состав груза изменился")
	}
	if v.Bool(d["bulkTaxRisk"]) {
		flags = append(flags, "Низкий спрос")
	}
	role := "muted"
	if expired || len(flags) > 1 {
		role = "warn"
	}
	rows = append(rows, HUDLine{strings.Join(flags, " · "), role})
	hudAdd(doc, title, rows...)
}
func hudCarrier(doc *HUDDocument, s M, now time.Time) {
	c := v.Obj(s["carrier"])
	if !v.Bool(c["owned"]) {
		return
	}
	fuel := v.Obj(c["fuel"])
	route := v.Obj(s["carrierRoute"])
	stops := v.Rows(v.At(route, "route", "stops"))
	idx := v.Int(v.At(route, "progress", "nextIndex"))
	if idx <= 0 {
		idx = v.Int(v.At(route, "route", "activeIndex"))
	}
	var stop M
	if idx > 0 && idx < len(stops) && !v.Bool(stops[idx]["visited"]) {
		stop = stops[idx]
	}
	if v.Str(route["error"]) != "" || v.Bool(route["running"]) {
		stop = nil
	}
	pending := v.Obj(v.At(c, "jump", "pending"))
	name := v.Text(pending["system"], pending["name"])
	rows := []HUDLine{}
	if name != "" {
		rows = append(rows, hudText(name))
		if at := v.Time(pending["departureTime"]); !at.IsZero() {
			remaining := at.Sub(now)
			if remaining > 0 {
				rows = append(rows, hudMuted("Отправление через "+Number(int64((remaining+time.Minute-1)/time.Minute))+" мин"))
			} else {
				rows = append(rows, hudMuted("Ожидаем подтверждение прибытия"))
			}
		}
		if stop != nil && !hudEqual(name, v.First(stop["name"], stop["system"])) {
			rows = append(rows, hudWarn("По плану далее: "+v.Text(stop["name"], stop["system"])))
			stop = nil
		}
	}
	if len(rows) == 0 && stop != nil {
		rows = append(rows, hudText("Далее: "+v.Text(stop["name"], stop["system"])))
	}
	if stop != nil {
		facts := []string{}
		if hudKnown(stop["fuelUsedTons"]) {
			facts = append(facts, "Расход: "+Number(stop["fuelUsedTons"])+" т")
		}
		if hudKnown(stop["distanceLy"]) {
			facts = append(facts, Number(stop["distanceLy"])+" св. л.")
		}
		rows = append(rows, hudMuted(strings.Join(facts, " · ")))
		if v.Num(stop["restockTons"]) > 0 {
			rows = append(rows, hudWarn("Дозаправка: "+Number(stop["restockTons"])+" т"))
		}
		if !v.Bool(fuel["needsRefresh"]) && hudKnown(fuel["tankTons"]) && hudKnown(stop["fuelUsedTons"]) && v.Num(fuel["tankTons"]) < v.Num(stop["fuelUsedTons"]) {
			need := v.Num(stop["fuelUsedTons"]) - v.Num(fuel["tankTons"])
			text := "Перелейте в бак " + Number(need) + " т"
			if !hudKnown(fuel["reserveTons"]) {
				text += " · склад неизвестен"
			} else if v.Num(fuel["reserveTons"]) < need {
				text = "Не хватает трития: " + Number(need-v.Num(fuel["reserveTons"])) + " т"
			}
			rows = append(rows, hudWarn(text))
		}
		if v.Num(v.At(route, "summary", "remainingJumps")) > 0 {
			rows = append(rows, hudMuted("Осталось прыжков: "+Number(v.At(route, "summary", "remainingJumps"))))
		}
	}
	if v.Str(route["error"]) != "" {
		rows = append(rows, hudWarn(v.Str(route["error"])))
	}
	hudAdd(doc, "ПРЫЖОК ФЛОТОНОСЦА", rows...)
	info := []HUDLine{}
	label := v.Text(c["callsign"], c["name"])
	if location := v.Str(v.At(c, "location", "system")); location != "" && !hudEqual(location, v.At(s, "commander", "system")) {
		if label != "" {
			label += " · "
		}
		label += location
	}
	if label != "" {
		info = append(info, hudMuted(label))
	}
	if hudKnown(fuel["tankTons"]) || hudKnown(fuel["reserveTons"]) || len(rows) > 0 {
		info = append(info, hudText("Тритий: бак "+Number(fuel["tankTons"])+" · склад "+Number(fuel["reserveTons"])+" т"))
	}
	if v.Bool(fuel["needsRefresh"]) {
		info = append(info, hudWarn("Топливо после прыжка не обновлено"))
	}
	if free := v.At(c, "space", "freeTons"); hudKnown(free) {
		info = append(info, hudMuted("Свободно на складе: "+Number(free)+" т"))
	}
	hudAdd(doc, "ФЛОТОНОСЕЦ", info...)
}
func hudColony(doc *HUDDocument, s M, now time.Time) {
	p := v.Obj(v.At(s, "colonization", "activeProject"))
	if len(p) == 0 {
		return
	}
	status := v.Str(p["status"])
	label := v.Text(p["name"], p["targetSystem"])
	if v.Str(p["targetSystem"]) != "" && !hudEqual(p["targetSystem"], label) && !hudEqual(p["targetSystem"], v.At(s, "commander", "system")) {
		label += " · " + v.Str(p["targetSystem"])
	}
	label += " · " + Label(status)
	rows := []HUDLine{hudMuted(label)}
	if status == "complete" || status == "cancelled" {
		hudAdd(doc, "КОЛОНИЯ", rows...)
		return
	}
	colony := v.Obj(s["colonization"])
	supplies := hudColonySupplies(colony, p)
	pending := hudPendingColonySupplies(colony, supplies)
	if len(supplies) > 0 && len(pending) == 0 {
		rows = append(rows, HUDLine{"Ресурсы на ФН · доставка на стройки ещё нужна", "good"})
	} else if len(supplies) == 0 && v.Str(v.At(p, "nextTask", "text")) != "" {
		text := v.Str(v.At(p, "nextTask", "text"))
		rows = append(rows, hudText(text))
	}
	if deadline := v.Time(p["deadline"]); !colonization.PrimaryPortComplete(p) && p["deadlineActive"] != false && !deadline.IsZero() {
		if deadline.Before(now) {
			rows = append(rows, hudWarn("Расчётный срок первичного порта истёк"))
		} else {
			rows = append(rows, hudMuted("Срок первичного порта: "+Date(p["deadline"])))
		}
	}
	lines := hudColonySupplyLines(colony, p, supplies)
	// Five actions per section can flow into the left safe band without an
	// oversized ten-row panel covering cockpit instruments. Context shares
	// the first panel instead of consuming an independent header window.
	if len(pending) > 5 {
		hudAdd(doc, "СНАБЖЕНИЕ КОЛОНИИ", append(rows, lines[:5]...)...)
		hudAdd(doc, "СНАБЖЕНИЕ · ПРОДОЛЖЕНИЕ", lines[5:]...)
	} else if len(lines) > 0 {
		hudAdd(doc, "СНАБЖЕНИЕ КОЛОНИИ", append(rows, lines...)...)
	} else {
		hudAdd(doc, "ВЫБРАННАЯ КОЛОНИЯ", rows...)
	}
}
