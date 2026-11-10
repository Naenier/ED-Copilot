package presentation

import (
	"sort"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func hudValue(label, value, role string) HUDLine {
	return HUDLine{Text: label, Value: value, Role: role}
}

func hudCargo(s M, empty, limpets bool) []HUDLine {
	cmd, cargo := v.Obj(s["commander"]), v.Obj(v.At(s, "mining", "cargo"))
	used, capacity := v.First(cargo["tons"], cmd["cargoTons"]), v.First(cargo["capacity"], cmd["cargoCapacity"])
	if cmd["vehicle"] == "srv" {
		capacity = v.First(cargo["capacity"], cmd["srvCargoCapacity"])
	}
	rows := []HUDLine{}
	if hudKnown(used) && (v.Num(used) > 0 || empty) {
		label, amount, role := "Груз", Number(used)+" т", "text"
		if hudKnown(capacity) {
			if v.Num(capacity) == 0 && v.Num(used) == 0 {
				label, amount, role = "Грузовой трюм отсутствует", "", "warn"
			} else {
				label, amount = "Трюм", Number(used)+" / "+Number(capacity)+" т"
				if v.Num(capacity) <= v.Num(used) {
					role = "warn"
				}
			}
		}
		rows = append(rows, hudValue(label, amount, role))
	} else if empty && hudKnown(capacity) && v.Num(capacity) > 0 {
		rows = append(rows, hudValue("Трюм", "— / "+Number(capacity)+" т", "muted"))
	}
	if limpets && hudKnown(cargo["limpets"]) {
		if v.Num(cargo["limpets"]) == 0 {
			rows = append(rows, hudWarn("Дронов в трюме нет"))
		} else {
			rows = append(rows, hudValue("Дроны", Number(cargo["limpets"]), "muted"))
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
			rows = append(rows, hudValue("Класс звезды", star, "muted"))
		}
		if v.Num(target["remainingJumps"]) > 0 {
			rows = append(rows, hudValue("Прыжков осталось", Number(target["remainingJumps"]), "muted"))
		}
		hudCard(doc, "navigation", "СЛЕДУЮЩИЙ ПРЫЖОК", "", 0, rows...)
	} else {
		goals := append([]M(nil), v.Rows(s["objectives"])...)
		sort.SliceStable(goals, func(i, j int) bool { return goals[i]["priority"] == "high" && goals[j]["priority"] != "high" })
		for _, goal := range goals {
			if (goal["status"] == "active" || goal["status"] == "planned") && v.Str(goal["title"]) != "" {
				rows := []HUDLine{hudText(v.Str(goal["title"]))}
				if v.Str(goal["system"]) != "" && !hudEqual(goal["system"], cmd["system"]) {
					rows = append(rows, hudMuted(v.Str(goal["system"])))
				}
				hudCard(doc, "navigation", "ЛИЧНАЯ ЦЕЛЬ", "", 0, rows...)
				break
			}
		}
	}
	fuel := v.Obj(v.At(s, "pilot", "fuel"))
	rows := []HUDLine{}
	if hudKnown(fuel["mainTons"]) {
		text := Number(fuel["mainTons"])
		if hudKnown(fuel["mainCapacityTons"]) {
			text += " / " + Number(fuel["mainCapacityTons"])
		}
		text += " т"
		role := "text"
		if v.Num(fuel["mainTons"]) == 0 {
			role = "warn"
		}
		rows = append(rows, hudValue("Топливо", text, role))
	}
	rows = append(rows, hudCargo(s, false, false)...)
	hudCard(doc, "navigation", "ТОПЛИВО И ГРУЗ", "", 0, rows...)
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
		rows := []HUDLine{{Text: decision, Role: role}}
		// A zero laser remainder does not cancel an assessed, uncracked core.
		// AssessProspect deliberately keeps that core mineable independently.
		coreAvailable := v.Str(p["motherlode"]) != "" && !v.Bool(p["coreCracked"]) && p["decision"] == "mine"
		depleted := hudKnown(p["remaining"]) && v.Num(p["remaining"]) == 0 && !coreAvailable
		if depleted {
			rows = []HUDLine{hudWarn("Астероид истощён")}
		} else {
			if core := v.Str(p["motherlode"]); core != "" {
				label := "Ядро"
				if v.Bool(p["coreCracked"]) {
					label = "Расколотое ядро"
				}
				rows = append(rows, hudValue(label, hudGood(s, core), "text"))
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
				rows = append(rows, hudValue(v.Text(material["name"], material["key"]), Number(material["percent"])+" %", "text"))
				shown++
				if shown == 2 {
					break
				}
			}
			if hudKnown(p["remaining"]) && (!coreAvailable || v.Num(p["remaining"]) > 0) {
				rows = append(rows, hudValue("Остаток для лазера", Number(p["remaining"])+" %", "muted"))
			}
		}
		hudCard(doc, "mining", "СКАН АСТЕРОИДА", "", 0, rows...)
	}
	rows := hudCargo(s, true, true)
	current := v.Obj(v.At(s, "mining", "current"))
	if hudContextMatches(current, cmd) && current["endedAt"] == nil && v.Num(current["refinedTons"]) > 0 {
		text := Number(current["refinedTons"]) + " т"
		if v.Num(current["tonsPerHour"]) > 0 {
			text += " · " + Number(current["tonsPerHour"]) + " т/ч"
		}
		rows = append(rows, hudValue("За заход", text, "muted"))
	}
	hudCard(doc, "mining", "ДОБЫЧА", "", 0, rows...)
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
	site := ""
	if hudContextMatches(surface, cmd) && !v.Bool(cmd["docked"]) {
		site = v.Str(surface["site"])
		for _, r := range v.Rows(surface["lastRefined"]) {
			if v.Num(r["count"]) > 0 {
				rows = append(rows, hudValue(v.Text(r["name"], r["key"]), Number(r["count"])+" т", "text"))
				if len(rows) >= 3 {
					break
				}
			}
		}
	}
	if len(rows) == 0 && site == "" {
		hudTargets(doc, s, "surface")
	}
	rows = append(rows, hudCargo(s, true, false)...)
	title := "ГРУЗ ПОВЕРХНОСТИ"
	if site != "" {
		title = "ТЕКУЩИЙ УЧАСТОК"
	}
	hudCard(doc, "mining", title, site, 0, rows...)
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
	shown := min(len(materials), 4)
	for _, r := range materials[:shown] {
		value, role := Number(r["missing"]), "text"
		if !hudKnown(r["missing"]) {
			value, role = "≤ "+Number(r["needed"]), "warn"
		}
		rows = append(rows, hudValue(v.Text(r["name"], r["key"]), value, role))
	}
	if rawOnly && !v.Bool(v.At(s, "engineering", "shopping", "knownCategories", "raw")) {
		unknown = max(unknown, 1)
	} else if !rawOnly {
		unknown = max(unknown, v.Int(v.At(s, "engineering", "shopping", "unknownMaterials")))
	}
	if unknown > 0 {
		text := "Запасы материалов известны не полностью"
		if rawOnly {
			text = "Запасы сырья известны не полностью"
		}
		rows = append(rows, hudWarn(text))
	}
	if len(rows) == 0 && v.Num(v.At(s, "engineering", "shopping", "neededUnits")) > 0 {
		text := "Материалы плана собраны"
		if rawOnly {
			text = "Сырьё для плана собрано"
		}
		rows = append(rows, HUDLine{Text: text, Role: "good"})
	}
	title := "МАТЕРИАЛЫ ДЛЯ УЛУЧШЕНИЙ"
	if rawOnly {
		title = "НУЖНОЕ СЫРЬЁ"
	}
	hudCard(doc, "engineering", title, "", len(materials)-shown, rows...)
}
func hudExchange(doc *HUDDocument, s M) {
	if len(v.Rows(v.At(s, "engineering", "items"))) == 0 {
		return
	}
	for _, offer := range v.Rows(v.At(s, "engineering", "trader", "offers")) {
		if v.Bool(offer["affordable"]) && v.Bool(offer["storageFits"]) && v.Num(offer["requiredFrom"]) > 0 && v.Num(offer["received"]) > 0 {
			hudCard(doc, "engineering", "ОБМЕН МАТЕРИАЛОВ", "", 0,
				hudValue(v.Str(offer["fromName"]), "−"+Number(offer["requiredFrom"]), "text"),
				hudValue(v.Str(offer["toName"]), "+"+Number(offer["received"]), "good"))
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
	wanted := map[string]bool{}
	if kind == "raw" {
		needed, _ := hudMaterialRows(s, true)
		for _, r := range needed {
			wanted[v.Key(r["key"])] = true
		}
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
		if system := v.Str(r["system"]); system != "" && !hudEqual(system, cmd["system"]) && !strings.HasPrefix(strings.ToLower(label), strings.ToLower(system)+" ") {
			label = v.Str(r["system"]) + " · " + label
		}
		facts := []string{}
		if !hudEqual(r["system"], cmd["system"]) {
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
		rows = append(rows, hudValue(label, strings.Join(facts, " · "), "muted"))
		if len(seen) == 2 {
			break
		}
	}
	hudCard(doc, "mining", "МЕСТА ПОБЛИЗОСТИ", "", 0, rows...)
}
func hudTradePlan(doc *HUDDocument, s M) {
	plan, cmd := v.Obj(s["tradePlan"]), v.Obj(s["commander"])
	rows := []HUDLine{}
	if v.Str(plan["key"]) != "" && v.Num(v.First(plan["quantity"], plan["targetTons"])) > 0 {
		name := v.Text(plan["name"], hudGood(s, v.Str(plan["key"])))
		amount := Number(v.First(plan["quantity"], plan["targetTons"])) + " т"
		for _, r := range v.Rows(cmd["cargo"]) {
			if v.Key(r["key"]) == v.Key(plan["key"]) && hudKnown(r["count"]) {
				amount = Number(r["count"]) + " / " + amount
				break
			}
		}
		rows = append(rows, hudValue(name, amount, "text"))
		if margin := v.First(plan["grossMargin"], plan["profit"]); margin != nil {
			rows = append(rows, hudValue("Плановая маржа", Number(margin)+" кр.", "muted"))
		}
	}
	rows = append(rows, hudCargo(s, false, false)...)
	hudCard(doc, "market", "ПЕРЕВОЗКА", "", 0, rows...)
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
	location := v.Text(d["station"], "Станция неизвестна")
	if arrived {
		title += " · НА МЕСТЕ"
	} else {
		if v.Str(d["system"]) != "" && !hudEqual(d["system"], cmd["system"]) {
			location += " · " + v.Str(d["system"])
		}
		if v.Num(d["distanceLy"]) > 0 {
			location += " · " + Number(d["distanceLy"]) + " св. л."
		}
	}
	quotes := append([]M(nil), v.Rows(d["quotes"])...)
	if len(quotes) == 0 && d["price"] != nil {
		quotes = []M{d}
	}
	if doc.Activity == "asteroid" && len(v.Rows(cmd["cargo"])) > 0 {
		cargo := map[string]bool{}
		for _, item := range v.Rows(cmd["cargo"]) {
			if v.Num(v.First(item["count"], item["Count"])) > 0 {
				cargo[v.Key(v.First(item["key"], item["Name"]))] = true
			}
		}
		matching := quotes[:0]
		for _, q := range quotes {
			if cargo[v.Key(q["key"])] {
				matching = append(matching, q)
			}
		}
		quotes = matching
	}
	planKey := v.Key(v.At(s, "tradePlan", "key"))
	if planKey != "" {
		sort.SliceStable(quotes, func(i, j int) bool { return v.Key(quotes[i]["key"]) == planKey && v.Key(quotes[j]["key"]) != planKey })
	}
	for _, q := range quotes {
		if len(rows) == 2 {
			break
		}
		availability, amount := "спрос", q["demand"]
		if buying {
			availability, amount = "запас", q["supply"]
		}
		facts := []string{}
		if hudKnown(q["price"]) {
			facts = append(facts, Number(q["price"])+" кр./т")
		}
		if hudKnown(amount) {
			facts = append(facts, availability+" "+Number(amount)+" т")
		}
		if len(facts) == 0 {
			continue
		}
		role := "text"
		if hudKnown(amount) && v.Num(amount) == 0 {
			role = "warn"
		}
		rows = append(rows, hudValue(v.Text(q["name"], hudGood(s, v.Str(q["key"]))), strings.Join(facts, " · "), role))
	}
	at := v.First(d["sourceUpdatedAt"], d["updatedAt"], d["timestamp"])
	date := v.Time(at)
	flags := []string{}
	if date.IsZero() || date.After(now.Add(10*time.Minute)) {
		flags = append(flags, "Время цены неизвестно")
	} else {
		rows = append(rows, hudValue("Котировка", Age(at, now), "muted"))
	}
	if v.Bool(d["cargoChanged"]) {
		flags = append(flags, "Состав груза изменился")
	}
	if v.Bool(d["bulkTaxRisk"]) {
		flags = append(flags, "Низкий спрос")
	}
	if len(flags) > 0 {
		rows = append(rows, hudWarn(strings.Join(flags, " · ")))
	}
	hudCard(doc, "market", title, location, 0, rows...)
}
func hudCarrierNextStop(route M) M {
	if v.Str(route["error"]) != "" || v.Bool(route["running"]) {
		return nil
	}
	stops := v.Rows(v.At(route, "route", "stops"))
	idx := v.Int(v.At(route, "progress", "nextIndex"))
	if idx <= 0 {
		idx = v.Int(v.At(route, "route", "activeIndex"))
	}
	if idx > 0 && idx < len(stops) && !v.Bool(stops[idx]["visited"]) {
		return stops[idx]
	}
	return nil
}

func hudCarrier(doc *HUDDocument, s M, now time.Time) {
	c := v.Obj(s["carrier"])
	if !v.Bool(c["owned"]) {
		return
	}
	fuel := v.Obj(c["fuel"])
	route := v.Obj(s["carrierRoute"])
	stop := hudCarrierNextStop(route)
	pending := v.Obj(v.At(c, "jump", "pending"))
	name := v.Text(pending["system"], pending["name"])
	rows := []HUDLine{}
	pendingPast := false
	if name != "" {
		if at := v.Time(pending["departureTime"]); !at.IsZero() {
			remaining := at.Sub(now)
			if remaining > 0 {
				rows = append(rows, hudValue("До отправления", Number(int64((remaining+time.Minute-1)/time.Minute))+" мин", "text"))
			} else {
				pendingPast = true
				rows = append(rows, hudMuted("Ожидаем подтверждение прибытия"))
			}
		} else {
			rows = append(rows, hudMuted("Прыжок запланирован"))
		}
		if stop != nil && !hudEqual(name, v.First(stop["name"], stop["system"])) {
			rows = append(rows, hudWarn("По плану далее: "+v.Text(stop["name"], stop["system"])))
			stop = nil
		}
	}
	if name == "" && stop != nil {
		name = v.Text(stop["name"], stop["system"])
	}
	if stop != nil {
		if !pendingPast && !v.Bool(fuel["needsRefresh"]) && hudKnown(fuel["tankTons"]) && hudKnown(stop["fuelUsedTons"]) && v.Num(fuel["tankTons"]) < v.Num(stop["fuelUsedTons"]) {
			need := v.Num(stop["fuelUsedTons"]) - v.Num(fuel["tankTons"])
			switch {
			case !hudKnown(fuel["reserveTons"]):
				rows = append(rows, hudValue("Не хватает в баке", Number(need)+" т", "warn"))
			case v.Num(fuel["reserveTons"]) < need:
				rows = append(rows, hudValue("Не хватает трития", Number(need-v.Num(fuel["reserveTons"]))+" т", "warn"))
			default:
				rows = append(rows, hudValue("Перелить в бак", Number(need)+" т", "warn"))
			}
		}
		var distance, usage string
		if v.Num(stop["distanceLy"]) > 0 {
			distance = Number(stop["distanceLy"]) + " св. л."
		}
		if hudKnown(stop["fuelUsedTons"]) {
			usage = Number(stop["fuelUsedTons"]) + " т"
		}
		switch {
		case distance != "" && usage != "":
			rows = append(rows, hudValue("Дальность и расход", distance+" · "+usage, "muted"))
		case distance != "":
			rows = append(rows, hudValue("Дальность", distance, "muted"))
		case usage != "":
			rows = append(rows, hudValue("Расход", usage, "muted"))
		}
		if v.Num(stop["restockTons"]) > 0 {
			rows = append(rows, hudValue("Заправка после прыжка", Number(stop["restockTons"])+" т", "warn"))
		}
		if v.Num(v.At(route, "summary", "remainingJumps")) > 0 {
			rows = append(rows, hudValue("Прыжков осталось", Number(v.At(route, "summary", "remainingJumps")), "muted"))
		}
	}
	if v.Str(route["error"]) != "" {
		rows = append(rows, hudWarn(v.Str(route["error"])))
	}
	if name != "" && len(rows) == 0 {
		rows = append(rows, hudText(name))
		name = ""
	}
	hudCard(doc, "carrier", "ПРЫЖОК ФЛОТОНОСЦА", name, 0, rows...)
	info := []HUDLine{}
	label := v.Text(c["callsign"], c["name"])
	if location := v.Str(v.At(c, "location", "system")); location != "" && !hudEqual(location, v.At(s, "commander", "system")) {
		if label != "" {
			label += " · "
		}
		label += location
	}
	staleFuel := v.Bool(fuel["needsRefresh"])
	if !staleFuel && hudKnown(fuel["tankTons"]) {
		info = append(info, hudValue("Тритий в баке", Number(fuel["tankTons"])+" т", "text"))
	}
	switch {
	case staleFuel:
		info = append(info, hudWarn("Топливо после прыжка не обновлено"))
	case hudKnown(fuel["reserveTons"]):
		info = append(info, hudValue("Тритий на складе", Number(fuel["reserveTons"])+" т", "text"))
	case stop != nil && (!hudKnown(fuel["tankTons"]) || v.Num(stop["fuelUsedTons"]) > v.Num(fuel["tankTons"])):
		info = append(info, hudWarn("Запас трития неизвестен"))
	}
	if free := v.At(c, "space", "freeTons"); hudKnown(free) && stop == nil {
		info = append(info, hudValue("Свободно на складе", Number(free)+" т", "muted"))
	}
	hudCard(doc, "carrier", "ФЛОТОНОСЕЦ", label, 0, info...)
}
