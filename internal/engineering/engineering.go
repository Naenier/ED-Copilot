package engineering

import (
	"errors"
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"sort"
	"strings"
	"sync"
)

type M = v.M

var catalog = data.Object("engineering-data.json")
var materialRows = v.Rows(catalog["materials"])
var materials, blueprints, modules, effects, engineers = byKey(materialRows), byKey(v.Rows(catalog["blueprints"])), byKey(v.Rows(catalog["modules"])), byKey(v.Rows(catalog["experimentals"])), byKey(v.Rows(catalog["engineers"]))
var aliases = func() map[string]string {
	r := map[string]string{}
	for _, m := range materialRows {
		for _, field := range []string{"key", "symbol", "english", "name"} {
			r[v.Key(m[field])] = v.Str(m["key"])
		}
	}
	r[v.Key("Adaptive Encyptors Capture")] = "adaptiveencryptors"
	return r
}()

func byKey(rows []M) map[string]M {
	r := map[string]M{}
	for _, m := range rows {
		r[v.Str(m["key"])] = m
	}
	return r
}
func MaterialKey(name any) string { return aliases[v.Key(name)] }
func contains(a any, s string) bool {
	for _, x := range v.Arr(a) {
		if v.Str(x) == s {
			return true
		}
	}
	return false
}
func integer(x any, label string, min, max int) (int, error) {
	n, ok := v.Number(x)
	if !ok || n != math.Trunc(n) || n < float64(min) || n > float64(max) {
		return 0, fmt.Errorf("%s: требуется целое число от %d до %d", label, min, max)
	}
	return int(n), nil
}
func ValidateItem(input M) (M, error) {
	module, bp := modules[v.Str(input["moduleKey"])], blueprints[v.Str(input["blueprintKey"])]
	if module == nil || bp == nil || !contains(module["blueprints"], v.Str(bp["key"])) {
		return nil, errors.New("Выберите совместимые модуль и чертёж")
	}
	raw := v.Arr(input["rolls"])
	if len(raw) > 5 {
		return nil, errors.New("Допускаются пять ступеней улучшения")
	}
	rolls := make([]any, 5)
	has := false
	for i := 0; i < 5; i++ {
		x := any(0)
		if i < len(raw) {
			x = raw[i]
		} else if m, ok := input["rolls"].(M); ok {
			x = v.First(m[fmt.Sprint(i+1)], 0)
		}
		n, e := integer(x, "Попытки", 0, 100)
		if e != nil {
			return nil, e
		}
		if n > 0 && v.Obj(bp["grades"])[fmt.Sprint(i+1)] == nil {
			return nil, fmt.Errorf("У чертежа нет ступени G%d", i+1)
		}
		rolls[i] = n
		has = has || n > 0
	}
	effect := v.Str(input["experimentalKey"])
	if effect != "" && (effects[effect] == nil || !contains(module["experimentals"], effect)) {
		return nil, errors.New("Экспериментальный эффект несовместим с модулем")
	}
	if !has && effect == "" {
		return nil, errors.New("Укажите попытки или экспериментальный эффект")
	}
	quantity, e := integer(v.First(input["quantity"], 1), "Количество модулей", 1, 100)
	if e != nil {
		return nil, e
	}
	id := v.Text(input["id"], v.ID())
	return M{"id": id, "moduleKey": module["key"], "blueprintKey": bp["key"], "rolls": rolls, "experimentalKey": effect, "quantity": quantity, "label": strings.TrimSpace(v.Str(input["label"]))}, nil
}
func ItemMaterials(item M) []M {
	needed := map[string]int{}
	add := func(rows any, mult int) {
		for _, r := range v.Rows(rows) {
			needed[v.Str(r["key"])] += v.Int(r["count"]) * mult
		}
	}
	for i, n := range v.Arr(item["rolls"]) {
		if v.Int(n) > 0 {
			add(v.Obj(blueprints[v.Str(item["blueprintKey"])]["grades"])[fmt.Sprint(i+1)], v.Int(n)*v.Int(item["quantity"]))
		}
	}
	if key := v.Str(item["experimentalKey"]); key != "" {
		add(effects[key]["materials"], v.Int(item["quantity"]))
	}
	r := []M{}
	for key, count := range needed {
		r = append(r, M{"key": key, "name": materials[key]["name"], "count": count})
	}
	sort.Slice(r, func(i, j int) bool { return v.Str(r[i]["key"]) < v.Str(r[j]["key"]) })
	return r
}
func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
func TradeRatio(from, to M) M {
	if from == nil || to == nil || from["key"] == to["key"] || !v.Bool(from["tradeable"]) || !v.Bool(to["tradeable"]) || from["category"] != to["category"] {
		return nil
	}
	d := v.Int(to["grade"]) - v.Int(from["grade"])
	cost, gain := 1, 1
	if d > 0 {
		cost = int(math.Pow(6, float64(d)))
	} else if d < 0 {
		gain = int(math.Pow(3, float64(-d)))
	}
	if from["family"] != to["family"] {
		cost *= 6
	}
	g := gcd(cost, gain)
	return M{"cost": cost / g, "gain": gain / g}
}
func TradeOffer(from, to M, wanted int, available, targetOwned any, protected int) M {
	ratio := TradeRatio(from, to)
	if ratio == nil {
		return nil
	}
	cost, gain := v.Int(ratio["cost"]), v.Int(ratio["gain"])
	batches := int(math.Ceil(float64(wanted) / float64(gain)))
	var space, maxBatches any
	if _, ok := v.Number(targetOwned); ok {
		space = math.Max(0, v.Num(to["capacity"])-v.Num(targetOwned))
		maxBatches = int(v.Num(space)) / gain
	}
	if _, ok := v.Number(available); ok {
		aff := v.Int(available) / cost
		if maxBatches == nil || aff < v.Int(maxBatches) {
			maxBatches = aff
		}
	}
	r := M{"fromKey": from["key"], "toKey": to["key"], "fromName": from["name"], "fromEnglish": from["english"], "toName": to["name"], "category": from["category"], "fromGrade": from["grade"], "toGrade": to["grade"], "sameFamily": from["family"] == to["family"], "cost": cost, "gain": gain, "requested": wanted, "batches": batches, "requiredFrom": batches * cost, "received": batches * gain, "available": available, "protectedCount": protected, "targetSpace": space, "stockKnown": available != nil, "targetStockKnown": targetOwned != nil}
	if available != nil {
		r["affordable"] = v.Int(available) >= batches*cost
	}
	if space != nil {
		r["storageFits"] = v.Int(space) >= batches*gain
	}
	if maxBatches != nil {
		r["maxGain"] = v.Int(maxBatches) * gain
		r["maxCost"] = v.Int(maxBatches) * cost
	}
	return r
}

type Planner struct {
	mu        sync.RWMutex
	items     []M
	options   M
	manual    M
	stock     map[string]int
	known     map[string]bool
	uncertain map[string]bool
	at        M
	progress  map[string]M
	roadmap   roadmapObservations
}

func New(saved M) *Planner {
	p := &Planner{options: M{"tradeTargetKey": "", "tradeTargetAmount": 1, "protectPlan": true, "showAllTrades": false}, manual: M{}}
	for _, item := range v.Rows(saved["items"]) {
		if row, e := ValidateItem(item); e == nil && len(p.items) < 200 {
			p.items = append(p.items, row)
		}
	}
	p.configure(v.Obj(saved["options"]))
	for k, x := range v.Obj(saved["manualEngineers"]) {
		if roadmapData.engineers[k] != nil {
			if _, ok := x.(bool); ok {
				p.manual[k] = x
			}
		}
	}
	p.reset()
	return p
}
func (p *Planner) reset() {
	p.stock = map[string]int{}
	p.known = map[string]bool{}
	p.uncertain = map[string]bool{}
	p.at = M{}
	p.progress = map[string]M{}
	p.resetRoadmap()
}
func (p *Planner) Reset() { p.mu.Lock(); defer p.mu.Unlock(); p.reset() }
func (p *Planner) ExportSettings() M {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return v.Copy(M{"items": p.items, "options": p.options, "manualEngineers": p.manual})
}
func (p *Planner) configure(body M) error {
	next := v.Copy(p.options)
	if x, ok := body["tradeTargetKey"]; ok {
		key := MaterialKey(x)
		if v.Str(x) != "" && key == "" {
			return errors.New("Материал не найден")
		}
		next["tradeTargetKey"] = key
	}
	if x, ok := body["tradeTargetAmount"]; ok {
		n, e := integer(x, "Нужное количество", 1, 100000)
		if e != nil {
			return e
		}
		next["tradeTargetAmount"] = n
	}
	for _, k := range []string{"protectPlan", "showAllTrades"} {
		if x, ok := body[k].(bool); ok {
			next[k] = x
		}
	}
	p.options = next
	return nil
}
func (p *Planner) Update(body M) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch v.Str(body["action"]) {
	case "add":
		if len(p.items) >= 200 {
			return errors.New("В плане уже 200 улучшений")
		}
		x := v.Copy(body)
		x["id"] = v.ID()
		item, e := ValidateItem(x)
		if e != nil {
			return e
		}
		p.items = append(p.items, item)
	case "update":
		for i, row := range p.items {
			if row["id"] == body["id"] {
				x, e := ValidateItem(v.Merge(v.Copy(row), body))
				if e != nil {
					return e
				}
				p.items[i] = x
				return nil
			}
		}
		return errors.New("Улучшение не найдено")
	case "remove":
		for i, row := range p.items {
			if row["id"] == body["id"] {
				p.items = append(p.items[:i], p.items[i+1:]...)
				return nil
			}
		}
		return errors.New("Улучшение не найдено")
	case "clear":
		p.items = nil
	case "configure":
		return p.configure(body)
	case "engineer":
		key := v.Text(body["key"], body["id"])
		if roadmapData.engineers[key] == nil {
			return errors.New("Инженер не найден")
		}
		if body["unlocked"] == nil {
			delete(p.manual, key)
		} else if b, ok := body["unlocked"].(bool); ok {
			p.manual[key] = b
		} else {
			return errors.New("Некорректный статус инженера")
		}
	case "import":
		rows := v.Arr(body["items"])
		if rows == nil {
			rows = v.Arr(v.Obj(body["plan"])["items"])
		}
		if rows == nil || len(rows) > 200 {
			return errors.New("План должен содержать до 200 улучшений")
		}
		next := []M{}
		for _, raw := range rows {
			x := v.Copy(v.Obj(raw))
			x["id"] = v.ID()
			item, e := ValidateItem(x)
			if e != nil {
				return e
			}
			next = append(next, item)
		}
		p.items = next
	default:
		return errors.New("Неизвестное действие инженерии")
	}
	return nil
}
func (p *Planner) change(name any, delta int, event M) {
	key := MaterialKey(name)
	if key == "" || delta == 0 {
		return
	}
	group := v.Str(materials[key]["category"])
	if p.known[group] && p.stock[key]+delta < 0 {
		p.uncertain[key] = true
	}
	p.stock[key] = max(0, p.stock[key]+delta)
	p.at[group] = event["timestamp"]
}
func (p *Planner) Apply(event, context M) {
	p.mu.Lock()
	defer p.mu.Unlock()
	kind := v.Str(event["event"])
	p.observeRoadmap(kind, event)
	switch kind {
	case "Materials":
		for field, group := range map[string]string{"Raw": "raw", "Manufactured": "manufactured", "Encoded": "encoded"} {
			if event[field] == nil {
				continue
			}
			for key, m := range materials {
				if m["category"] == group {
					delete(p.stock, key)
					delete(p.uncertain, key)
				}
			}
			for _, r := range v.Rows(event[field]) {
				key := MaterialKey(r["Name"])
				if m := materials[key]; m != nil && m["category"] == group {
					if n, e := integer(r["Count"], "Запас", 0, 1000000); e == nil {
						p.stock[key] = n
					}
				}
			}
			p.known[group] = true
			p.at[group] = event["timestamp"]
		}
	case "MaterialCollected", "MaterialDiscarded":
		n := max(0, v.Int(event["Count"]))
		if kind == "MaterialDiscarded" {
			n = -n
		}
		p.change(event["Name"], n, event)
	case "MaterialTrade":
		paid, received := v.Obj(event["Paid"]), v.Obj(event["Received"])
		p.change(v.First(paid["Material"], paid["Name"]), -max(0, v.Int(v.First(paid["Quantity"], paid["Count"]))), event)
		p.change(v.First(received["Material"], received["Name"]), max(0, v.Int(v.First(received["Quantity"], received["Count"]))), event)
	case "EngineerContribution":
		if strings.EqualFold(v.Str(event["Type"]), "materials") {
			p.change(event["Material"], -max(0, v.Int(event["Quantity"])), event)
		}
	case "MissionCompleted":
		for _, r := range v.Rows(event["MaterialsReward"]) {
			p.change(r["Name"], max(0, v.Int(r["Count"])), event)
		}
	case "EngineerCraft", "Synthesis", "TechnologyBroker":
		rows := event["Materials"]
		if kind == "EngineerCraft" {
			rows = event["Ingredients"]
		}
		for _, r := range v.Rows(rows) {
			p.change(r["Name"], -max(0, v.Int(r["Count"])), event)
		}
	case "EngineerProgress":
		rows := v.Rows(event["Engineers"])
		if rows == nil {
			rows = []M{event}
		}
		for _, r := range rows {
			key := roadmapEngineerKey(r["EngineerID"], r["Engineer"])
			if key == "" {
				continue
			}
			if at := v.Time(event["timestamp"]); !at.IsZero() && at.Before(v.Time(p.progress[key]["timestamp"])) {
				continue
			}
			x := v.Merge(v.Copy(p.progress[key]), M{"timestamp": event["timestamp"]})
			for _, f := range []string{"Rank", "RankProgress"} {
				if n, ok := v.Number(r[f]); ok {
					if f == "Rank" {
						x["rank"] = n
					} else {
						x["progress"] = n
					}
				}
			}
			if s := v.Str(r["Progress"]); s != "" {
				x["status"] = s
				if !strings.EqualFold(s, "Unlocked") && !strings.EqualFold(s, "Barred") {
					delete(x, "rank")
					delete(x, "progress")
				}
			} else if v.Num(r["Rank"]) > 0 {
				x["status"] = "Unlocked"
			}
			p.progress[key] = x
		}
	}
}
func (p *Planner) Snapshot(context M) M {
	p.mu.RLock()
	defer p.mu.RUnlock()
	totals := map[string]int{}
	items := []M{}
	for _, raw := range p.items {
		item := v.Copy(raw)
		recipe := ItemMaterials(item)
		grade := 0
		for i, n := range v.Arr(item["rolls"]) {
			if v.Int(n) > 0 {
				grade = i + 1
			}
		}
		for _, r := range recipe {
			totals[v.Str(r["key"])] += v.Int(r["count"])
		}
		module, bp := modules[v.Str(item["moduleKey"])], blueprints[v.Str(item["blueprintKey"])]
		item["moduleName"] = module["name"]
		item["blueprintName"] = bp["name"]
		item["maxGrade"] = grade
		item["materials"] = recipe
		item["experimentalName"] = effects[v.Str(item["experimentalKey"])]["name"]
		item["engineerKeys"] = []any{}
		names := v.Obj(v.Obj(module["engineers"])[v.Str(bp["key"])])[fmt.Sprint(max(1, grade))]
		for id, e := range engineers {
			if contains(names, v.Str(e["english"])) {
				item["engineerKeys"] = append(v.Arr(item["engineerKeys"]), id)
			}
		}
		items = append(items, item)
	}
	rows := []M{}
	neededUnits, missingUnits, unknown := 0, 0, 0
	familyNames := map[string]string{}
	for _, m := range materialRows {
		fk := v.Str(m["category"]) + ":" + v.Str(m["family"])
		if familyNames[fk] == "" {
			familyNames[fk] = v.Str(m["name"])
		}
	}
	for _, raw := range materialRows {
		r := v.Copy(raw)
		key, group := v.Str(r["key"]), v.Str(r["category"])
		need := totals[key]
		var owned any
		if p.known[group] && !p.uncertain[key] {
			owned = p.stock[key]
		}
		r["owned"] = owned
		r["needed"] = need
		r["updatedAt"] = p.at[group]
		r["categoryName"] = map[string]string{"raw": "Сырьё", "manufactured": "Производственные материалы", "encoded": "Данные", "currency": "Валюта"}[group]
		r["familyName"] = familyNames[group+":"+v.Str(r["family"])]
		if owned != nil {
			r["missing"] = max(0, need-v.Int(owned))
			r["surplus"] = max(0, v.Int(owned)-need)
			missingUnits += v.Int(r["missing"])
		} else if need > 0 {
			unknown++
		}
		neededUnits += need
		rows = append(rows, r)
	}
	offers := []M{}
	var target M
	for _, r := range rows {
		if r["key"] == p.options["tradeTargetKey"] {
			target = r
			break
		}
	}
	if target != nil {
		for _, r := range rows {
			available := r["owned"]
			protected := 0
			if v.Bool(p.options["protectPlan"]) {
				available = r["surplus"]
				protected = v.Int(r["needed"])
			}
			offer := TradeOffer(r, target, v.Int(p.options["tradeTargetAmount"]), available, target["owned"], protected)
			if offer != nil && (v.Bool(p.options["showAllTrades"]) || offer["available"] == nil || v.Int(offer["maxGain"]) > 0) {
				offers = append(offers, offer)
			}
		}
	}
	sort.SliceStable(offers, func(i, j int) bool {
		a, b := offers[i], offers[j]
		aa, bb := v.Bool(a["affordable"]) && a["storageFits"] != false, v.Bool(b["affordable"]) && b["storageFits"] != false
		if aa != bb {
			return aa
		}
		if a["sameFamily"] != b["sameFamily"] {
			return v.Bool(a["sameFamily"])
		}
		return v.Int(a["requiredFrom"]) < v.Int(b["requiredFrom"])
	})
	erows := []M{}
	unlocked := 0
	for _, raw := range v.Rows(catalog["engineers"]) {
		e := v.Copy(raw)
		id := v.Str(e["key"])
		pr := p.progress[id]
		status := strings.ToLower(v.Str(pr["status"]))
		if status == "" {
			status = "unknown"
		}
		if b, ok := p.manual[id].(bool); ok {
			if b {
				status = "unlocked"
			} else {
				status = "locked"
			}
		}
		if status == "unlocked" {
			unlocked++
		}
		caps := []M{}
		for _, m := range v.Rows(catalog["modules"]) {
			for bp, grades := range v.Obj(m["engineers"]) {
				grade := 0
				for g, names := range v.Obj(grades) {
					if contains(names, v.Str(e["english"])) {
						grade = max(grade, v.Int(g))
					}
				}
				if grade > 0 {
					caps = append(caps, M{"moduleKey": m["key"], "moduleName": m["name"], "blueprintKey": bp, "blueprintName": blueprints[bp]["name"], "maxGrade": grade})
				}
			}
		}
		v.Merge(e, M{"status": status, "manualUnlocked": p.manual[id], "journalStatus": pr["status"], "rank": pr["rank"], "progress": pr["progress"], "updatedAt": pr["timestamp"], "capabilities": caps})
		erows = append(erows, e)
	}
	var missing any = missingUnits
	if unknown > 0 {
		missing = nil
	}
	known, updated := M{}, M{}
	for key, value := range p.known {
		known[key] = value
	}
	for key, value := range p.at {
		updated[key] = value
	}
	return M{"catalog": catalog, "items": items, "materials": rows, "shopping": M{"neededUnits": neededUnits, "missingUnits": missing, "unknownMaterials": unknown, "inventoryKnown": unknown == 0, "complete": unknown == 0 && missingUnits == 0, "knownCategories": known, "updatedAt": updated}, "trader": M{"targetKey": p.options["tradeTargetKey"], "targetName": target["name"], "targetEnglish": target["english"], "targetAmount": p.options["tradeTargetAmount"], "protectPlan": p.options["protectPlan"], "showAllTrades": p.options["showAllTrades"], "offers": offers}, "engineers": erows, "engineerSummary": M{"unlocked": unlocked, "total": len(erows)}, "options": v.Copy(p.options), "sources": catalog["sources"]}
}
