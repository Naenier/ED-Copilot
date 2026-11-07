package trading

import (
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

type M = v.M

var catalogOnce sync.Once
var resources []M
var resourceIndex map[string]M
var names, russian M

func initCatalog() {
	catalogOnce.Do(func() {
		c := data.Object("catalog.json")
		resources = v.Rows(c["resources"])
		names = data.Object("commodity-names.json")
		russian = data.Object("commodity-ru.json")
		resourceIndex = map[string]M{}
		for _, r := range resources {
			for _, s := range []any{r["key"], r["name"], r["english"]} {
				resourceIndex[v.Key(s)] = r
			}
		}
		for alias, key := range v.Obj(c["resourceAliases"]) {
			if r := resourceIndex[v.Key(key)]; r != nil {
				resourceIndex[v.Key(alias)] = r
			}
		}
	})
}
func Resources() []M { initCatalog(); return v.Rows(v.Clone(resources)) }
func Resource(raw any) M {
	initCatalog()
	r := resourceIndex[v.Key(raw)]
	if r == nil {
		return nil
	}
	return v.Copy(r)
}
func Good(raw, localized any) M {
	initCatalog()
	r := resourceIndex[v.Key(raw)]
	key := v.Key(raw)
	if r != nil {
		key = v.Str(r["key"])
	}
	if key == "" {
		return nil
	}
	english := v.Text(r["english"])
	if english == "" {
		a := v.Arr(names[key])
		if len(a) > 0 {
			english = v.Str(a[0])
		}
	}
	if english == "" {
		english = v.Str(raw)
	}
	name := v.Text(r["name"], russian[key], russian[key+"name"], localized, english)
	out := v.Copy(r)
	out["key"] = key
	out["name"] = name
	out["english"] = english
	if out["kind"] == nil {
		out["kind"] = "commodity"
	}
	return out
}
func Name(raw, localized any) string { return v.Str(Good(raw, localized)["name"]) }
func Vessel(raw any) string {
	s := strings.ToLower(v.Str(raw))
	for _, part := range []string{"srv", "scarab", "scorpion", "rhino", "nomad", "buggy", "lander01"} {
		if strings.Contains(s, part) {
			return "srv"
		}
	}
	return "ship"
}
func amount(x any, fallback ...float64) float64 {
	n, ok := v.Number(x)
	if !ok {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return 0
	}
	if n < 0 {
		return 0
	}
	return n
}
func n(m M, key string) float64           { return amount(m[key]) }
func plus(m M, key string, delta float64) { m[key] = v.Num(m[key]) + delta }
func sortedValues(m map[string]M, field string, desc bool) []M {
	a := make([]M, 0, len(m))
	for _, r := range m {
		a = append(a, v.Copy(r))
	}
	sort.SliceStable(a, func(i, j int) bool {
		if desc {
			return v.Num(a[i][field]) > v.Num(a[j][field])
		}
		return v.Str(a[i][field]) < v.Str(a[j][field])
	})
	return a
}

type lot struct {
	Count    float64
	UnitCost *float64
	Origin   string
}
type store map[string][]lot
type Service struct {
	Holds                map[string]store
	Warehouses           map[string]store
	Goods                map[string]M
	ActiveVessel         string
	Totals               M
	Rows                 map[string]M
	Recent, Transactions []M
	Daily                map[string]M
	Plan, Destination    M
}

func New(saved M) *Service {
	s := &Service{Plan: v.Copy(v.Obj(saved["tradePlan"])), Destination: v.Copy(v.Obj(saved["tradingDestination"]))}
	s.Reset()
	return s
}
func (s *Service) Reset() {
	initCatalog()
	s.Holds = map[string]store{"ship": {}, "srv": {}}
	s.Warehouses = map[string]store{}
	s.Goods = map[string]M{}
	for key := range names {
		s.Goods[key] = Good(key, nil)
	}
	for _, r := range resources {
		if r["kind"] == "commodity" {
			s.Goods[v.Str(r["key"])] = v.Copy(r)
		}
	}
	s.ActiveVessel = "ship"
	s.Totals = M{}
	for _, k := range []string{"boughtTons", "soldTons", "purchaseCost", "revenue", "knownCost", "knownRevenue", "grossMargin", "unknownCostTons", "miningRevenue", "internalTons", "operatingCosts"} {
		s.Totals[k] = float64(0)
	}
	s.Rows = map[string]M{}
	s.Recent = nil
	s.Transactions = nil
	s.Daily = map[string]M{}
}
func (s *Service) good(raw, localized any) M {
	r := Good(raw, localized)
	if r != nil {
		s.Goods[v.Str(r["key"])] = r
	}
	return r
}
func (s *Service) hold(vessel string) store {
	vessel = Vessel(vessel)
	if s.Holds[vessel] == nil {
		s.Holds[vessel] = store{}
	}
	return s.Holds[vessel]
}
func (s *Service) warehouse(id any) store {
	key := v.Text(id, "unknown")
	if s.Warehouses[key] == nil {
		s.Warehouses[key] = store{}
	}
	return s.Warehouses[key]
}
func add(st store, key string, lots []lot) {
	for _, l := range lots {
		if l.Count > 0 {
			st[key] = append(st[key], l)
		}
	}
}
func take(st store, key string, requested float64) []lot {
	lots := st[key]
	out := []lot{}
	for requested > 0 && len(lots) > 0 {
		l := lots[0]
		count := l.Count
		if count > requested {
			count = requested
		}
		out = append(out, lot{count, l.UnitCost, l.Origin})
		lots[0].Count -= count
		requested -= count
		if lots[0].Count <= 0 {
			lots = lots[1:]
		}
	}
	if requested > 0 {
		out = append(out, lot{Count: requested, Origin: "unknown"})
	}
	st[key] = lots
	return out
}
func (s *Service) Reconcile(event, context M) {
	cmd := v.Obj(context["commander"])
	vessel := Vessel(v.First(event["Vessel"], cmd["vehicle"]))
	s.ActiveVessel = vessel
	st := s.hold(vessel)
	actual := map[string]float64{}
	for _, row := range v.Rows(event["Inventory"]) {
		g := s.good(row["Name"], row["Name_Localised"])
		if g != nil {
			actual[v.Str(g["key"])] += amount(row["Count"])
		}
	}
	keys := map[string]bool{}
	for k := range st {
		keys[k] = true
	}
	for k := range actual {
		keys[k] = true
	}
	for k := range keys {
		current := float64(0)
		for _, l := range st[k] {
			current += l.Count
		}
		expected := actual[k]
		if expected > current {
			add(st, k, []lot{{Count: expected - current, Origin: "unknown"}})
		}
		if expected < current {
			take(st, k, current-expected)
		}
	}
}
func (s *Service) Market(event M) {
	for _, row := range v.Rows(event["Items"]) {
		s.good(row["Name"], row["Name_Localised"])
	}
}
func (s *Service) record(event, t M, countInPeriod bool) {
	if !countInPeriod {
		return
	}
	date := v.Str(event["timestamp"])
	if len(date) > 10 {
		date = date[:10]
	}
	day := s.Daily[date]
	if day == nil {
		day = M{"date": date, "purchaseCost": 0, "revenue": 0, "grossMargin": 0, "minedTons": 0}
		s.Daily[date] = day
	}
	kind := v.Str(t["kind"])
	count := n(t, "count")
	value := n(t, "value")
	internal := v.Bool(t["internal"])
	if internal {
		plus(s.Totals, "internalTons", count)
	} else {
		switch kind {
		case "buy":
			plus(s.Totals, "boughtTons", count)
			plus(s.Totals, "purchaseCost", value)
			plus(day, "purchaseCost", value)
		case "sell":
			plus(s.Totals, "soldTons", count)
			plus(s.Totals, "revenue", value)
			for _, p := range [][2]string{{"knownCost", "knownCost"}, {"knownRevenue", "knownRevenue"}, {"grossMargin", "margin"}, {"unknownCostTons", "unknownTons"}, {"miningRevenue", "minedRevenue"}} {
				plus(s.Totals, p[0], n(t, p[1]))
			}
			plus(day, "revenue", value)
			plus(day, "grossMargin", n(t, "margin"))
		case "cost":
			plus(s.Totals, "operatingCosts", value)
		}
	}
	key := v.Str(t["key"])
	if key != "" {
		r := s.Rows[key]
		if r == nil {
			r = M{"key": key, "name": t["name"], "boughtTons": 0, "soldTons": 0, "revenue": 0, "purchaseCost": 0, "grossMargin": 0, "unknownCostTons": 0}
			s.Rows[key] = r
		}
		if !internal && kind == "buy" {
			plus(r, "boughtTons", count)
			plus(r, "purchaseCost", value)
		}
		if !internal && kind == "sell" {
			plus(r, "soldTons", count)
			plus(r, "revenue", value)
			plus(r, "grossMargin", n(t, "margin"))
			plus(r, "unknownCostTons", n(t, "unknownTons"))
		}
	}
	record := v.Merge(M{"timestamp": event["timestamp"]}, t)
	s.Transactions = append(s.Transactions, record)
	s.Recent = append([]M{record}, s.Recent...)
	if len(s.Recent) > 200 {
		s.Recent = s.Recent[:200]
	}
}
func (s *Service) Apply(event, context M) M {
	typ := v.Str(event["event"])
	cmd := v.Obj(context["commander"])
	st := s.hold(Vessel(cmd["vehicle"]))
	warehouse := s.warehouse(context["carrierId"])
	internal := v.Bool(context["ownMarket"])
	period := context["countInPeriod"] != false
	result := M{"internal": false, "minedRevenue": 0}
	switch typ {
	case "Cargo":
		s.Reconcile(event, context)
	case "Market":
		s.Market(event)
	case "MiningRefined", "CollectCargo":
		g := s.good(event["Type"], event["Type_Localised"])
		if g != nil {
			l := lot{Count: amount(event["Count"], 1), Origin: "collected"}
			if typ == "MiningRefined" {
				zero := float64(0)
				l.UnitCost = &zero
				l.Origin = "mined"
			}
			add(st, v.Str(g["key"]), []lot{l})
		}
	case "EjectCargo":
		g := s.good(event["Type"], event["Type_Localised"])
		if g != nil {
			take(st, v.Str(g["key"]), amount(event["Count"], 1))
		}
	case "ColonisationContribution":
		for _, r := range v.Rows(event["Contributions"]) {
			g := s.good(r["Name"], r["Name_Localised"])
			if g != nil {
				take(st, v.Str(g["key"]), amount(r["Amount"]))
			}
		}
	case "CarrierDepositFuel":
		count := amount(event["Amount"])
		take(st, "tritium", count)
		s.record(event, M{"kind": "transfer", "key": "tritium", "name": "Тритий", "count": count, "value": 0, "internal": true, "direction": "carrier-fuel", "system": cmd["system"], "station": context["station"]}, period)
		result["internal"] = true
	case "CargoTransfer":
		for _, r := range v.Rows(event["Transfers"]) {
			g := s.good(r["Type"], r["Type_Localised"])
			count := amount(r["Count"])
			direction := strings.ToLower(v.Str(r["Direction"]))
			if g == nil || count <= 0 || (direction != "tocarrier" && direction != "toship") {
				continue
			}
			key := v.Str(g["key"])
			if direction == "tocarrier" {
				add(warehouse, key, take(st, key, count))
			} else {
				add(st, key, take(warehouse, key, count))
			}
			s.record(event, M{"kind": "transfer", "key": key, "name": g["name"], "count": count, "value": 0, "internal": true, "system": cmd["system"], "station": context["station"], "direction": direction}, period)
		}
		result["internal"] = true
	case "MarketBuy", "MarketSell":
		g := s.good(event["Type"], event["Type_Localised"])
		count := amount(event["Count"])
		if g == nil || count <= 0 {
			return result
		}
		key := v.Str(g["key"])
		value := amount(event["TotalSale"], amount(event["SellPrice"])*count)
		kind := "sell"
		if typ == "MarketBuy" {
			kind = "buy"
			value = amount(event["TotalCost"], amount(event["BuyPrice"])*count)
		}
		t := M{"kind": kind, "key": key, "name": g["name"], "count": count, "value": value, "internal": internal, "system": cmd["system"], "station": context["station"], "knownCost": 0, "knownRevenue": 0, "margin": 0, "unknownTons": 0, "minedRevenue": 0}
		if typ == "MarketBuy" {
			if internal {
				add(st, key, take(warehouse, key, count))
			} else {
				cost := value / count
				add(st, key, []lot{{count, &cost, "bought"}})
			}
		} else {
			lots := take(st, key, count)
			if internal {
				add(warehouse, key, lots)
			} else {
				for _, l := range lots {
					saleValue := value * l.Count / count
					cost := l.UnitCost
					if cost == nil && amount(event["AvgPricePaid"]) > 0 {
						c := amount(event["AvgPricePaid"])
						cost = &c
					}
					if cost != nil {
						plus(t, "knownCost", *cost*l.Count)
						plus(t, "knownRevenue", saleValue)
					} else {
						plus(t, "unknownTons", l.Count)
					}
					if l.Origin == "mined" {
						plus(t, "minedRevenue", saleValue)
					}
				}
				t["margin"] = n(t, "knownRevenue") - n(t, "knownCost")
			}
		}
		s.record(event, t, period)
		result["internal"] = internal
		result["minedRevenue"] = t["minedRevenue"]
	case "Died":
		s.Holds = map[string]store{"ship": {}, "srv": {}}
	}
	expenses := map[string]string{"RefuelAll": "Заправка", "RefuelPartial": "Заправка", "Repair": "Ремонт", "RepairAll": "Ремонт", "RestockVehicle": "Пополнение техники", "Resurrect": "Страховка", "BuyAmmo": "Боеприпасы"}
	if label := expenses[typ]; label != "" && amount(event["Cost"]) > 0 {
		s.record(event, M{"kind": "cost", "name": label, "count": 0, "value": amount(event["Cost"]), "cost": amount(event["Cost"]), "internal": false, "system": cmd["system"], "station": context["station"]}, period)
	}
	return result
}
func (s *Service) Snapshot(context M) M {
	cmd := context
	if c, ok := context["commander"]; ok {
		cmd = v.Obj(c)
	}
	vessel := v.Str(cmd["vehicle"])
	if vessel != "ship" && vessel != "srv" {
		vessel = s.ActiveVessel
	}
	inventory := []M{}
	for key, lots := range s.hold(vessel) {
		count, known, cost, mined := float64(0), float64(0), float64(0), float64(0)
		for _, l := range lots {
			count += l.Count
			if l.UnitCost != nil {
				known += l.Count
				cost += l.Count * *l.UnitCost
			}
			if l.Origin == "mined" {
				mined += l.Count
			}
		}
		if count <= 0 {
			continue
		}
		var unit any
		if known == count {
			unit = cost / count
		}
		inventory = append(inventory, M{"key": key, "name": s.Goods[key]["name"], "count": count, "knownCostTons": known, "unknownCostTons": count - known, "purchaseCost": cost, "unitCost": unit, "minedTons": mined})
	}
	sort.Slice(inventory, func(i, j int) bool { return n(inventory[i], "count") > n(inventory[j], "count") })
	daily := sortedValues(s.Daily, "date", false)
	sort.Slice(daily, func(i, j int) bool { return v.Str(daily[i]["date"]) > v.Str(daily[j]["date"]) })
	var plan, destination any
	if len(s.Plan) > 0 {
		plan = v.Copy(s.Plan)
	}
	if len(s.Destination) > 0 {
		now := v.Time(context["now"])
		if now.IsZero() {
			now = time.Now()
		}
		projected := QuoteState(s.Destination, v.Obj(context["marketOptions"]), now)
		if d := Distance(cmd["coordinates"], projected["coordinates"]); !math.IsNaN(d) {
			projected["distanceLy"] = d
		}
		if manifest, ok := s.Destination["cargoManifest"].(string); ok {
			projected["cargoChanged"] = manifest != s.cargoManifest(vessel)
		}
		destination = projected
	}
	return M{"plan": plan, "destination": destination, "totals": v.Copy(s.Totals), "inventory": inventory, "goods": sortedValues(s.Goods, "name", false), "byResource": sortedValues(s.Rows, "revenue", true), "recent": v.Clone(s.Recent), "daily": daily, "help": "Валовая маржа учитывает известную себестоимость проданных партий. Неизвестная себестоимость показана отдельно. Переводы на собственный флотоносец не считаются внешней выручкой."}
}

func (s *Service) cargoManifest(vessel string) string {
	parts := []string{}
	for key, lots := range s.hold(vessel) {
		count := 0.0
		for _, l := range lots {
			count += l.Count
		}
		if count > 0 {
			parts = append(parts, fmt.Sprintf("%s:%g", key, count))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}
func (s *Service) ExportSettings() M {
	return M{"tradePlan": v.Copy(s.Plan), "tradingDestination": v.Copy(s.Destination)}
}
