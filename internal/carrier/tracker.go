package carrier

import (
	"errors"
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"sort"
	"strings"
	"time"
)

type M = v.M

var ownerEvents = map[string]bool{"CarrierBuy": true, "CarrierStats": true, "CarrierLocation": true, "CarrierJumpRequest": true, "CarrierJumpCancelled": true, "CarrierFinance": true, "CarrierBankTransfer": true, "CarrierCrewServices": true, "CarrierTradeOrder": true, "CarrierNameChanged": true, "CarrierDockingPermission": true, "CarrierDecommission": true, "CarrierCancelDecommission": true, "CarrierShipPack": true, "CarrierModulePack": true}
var serviceNames = map[string]string{"captain": "Управление", "refuel": "Заправка кораблей", "repair": "Ремонт", "rearm": "Боеприпасы и дроны", "commodities": "Товарный рынок", "carrierfuel": "Тритиевое хранилище", "blackmarket": "Чёрный рынок", "voucherredemption": "Служба погашения", "shipyard": "Верфь", "outfitting": "Снаряжение", "exploration": "Universal Cartographics", "vistagenomics": "Vista Genomics", "bartender": "Бар", "pioneersupplies": "Снабжение «Пионер»"}

func numeric(x any) any {
	n, ok := v.Number(x)
	if !ok {
		return nil
	}
	return n
}
func emptyFuel() M {
	return M{"tankTons": nil, "tankCapacityTons": 1000, "depositedTons": 0, "jumpRangeCurrent": nil, "jumpRangeMax": nil, "updatedAt": nil, "needsRefresh": false}
}
func emptySpace() M {
	return M{"capacityTons": nil, "cargoTons": nil, "reservedTons": nil, "crewTons": nil, "shipPacksTons": nil, "modulePacksTons": nil, "freeTons": nil, "updatedAt": nil, "estimated": false}
}
func emptyFinance() M {
	return M{"balance": nil, "reserveBalance": nil, "availableBalance": nil, "reservePercent": nil, "taxRates": M{}, "updatedAt": nil}
}
func planning(input M) M {
	upkeep := numeric(input["weeklyUpkeep"])
	if v.Num(upkeep) <= 0 {
		upkeep = nil
	}
	return M{"weeklyUpkeep": upkeep, "reserveTritium": v.Clamp(v.Num(input["reserveTritium"]), 0, 25000), "targetTankTons": v.Clamp(v.Num(v.First(input["targetTankTons"], 1000)), 0, 1000)}
}
func newCarrier(id string) M {
	return M{"id": id, "name": "", "callsign": "", "updatedAt": nil, "statsAt": nil, "location": M{}, "dockingAccess": nil, "allowNotorious": nil, "pendingDecommission": false, "scrapTime": nil, "fuel": emptyFuel(), "space": emptySpace(), "finance": emptyFinance(), "services": M{}, "orders": M{}, "items": M{}, "movements": []M{}, "transactions": []M{}, "jump": M{"pending": nil, "lastArrival": nil}}
}

type Tracker struct {
	Carriers                                map[string]M
	OwnerID, DockedCarrierID, DockedStation string
	Planning, StockBaseline                 M
	Now                                     func() time.Time
	OtherCarrierEvents                      int
}

func New(saved M) *Tracker {
	t := &Tracker{Planning: planning(v.Obj(saved["planning"])), StockBaseline: v.Copy(v.Obj(saved["stockBaseline"])), Now: time.Now}
	t.Reset()
	return t
}
func (t *Tracker) Reset() {
	t.Carriers = map[string]M{}
	t.OwnerID = ""
	t.DockedCarrierID = ""
	t.DockedStation = ""
	t.OtherCarrierEvents = 0
}
func (t *Tracker) ExportSettings() M {
	return M{"planning": v.Copy(t.Planning), "stockBaseline": v.Copy(t.StockBaseline)}
}
func (t *Tracker) Current() M { return t.Carriers[t.OwnerID] }
func (t *Tracker) IsOwnDockedMarket(id any) bool {
	candidate := v.Text(id, t.DockedCarrierID)
	return t.OwnerID != "" && candidate == t.OwnerID
}
func (t *Tracker) Update(body M) error {
	switch v.Str(body["action"]) {
	case "", "planning":
		t.Planning = planning(v.Merge(v.Copy(t.Planning), body))
		return nil
	case "stock":
		return t.SetStockBaseline(body)
	default:
		return errors.New("Неизвестное действие флотоносца.")
	}
}
func (t *Tracker) SetStockBaseline(body M) error {
	id := v.Text(body["carrierId"], t.OwnerID)
	if id == "" || id != t.OwnerID {
		return errors.New("Сначала откройте управление личным флотоносцем в игре.")
	}
	if body["items"] == nil {
		return errors.New("Нужен список ресурсов склада.")
	}
	rows := v.Rows(body["items"])
	if len(rows) > 512 {
		return errors.New("Слишком большой список склада.")
	}
	items := []M{}
	for _, row := range rows {
		key := v.Key(v.First(row["key"], row["name"]))
		count, ok := v.Number(row["count"])
		if key == "" || !ok || count < 0 || count > 25000 {
			return errors.New("Некорректное количество ресурса на складе.")
		}
		items = append(items, M{"key": key, "name": trading.Name(key, row["name"]), "count": count})
	}
	timestamp := v.Text(body["timestamp"], v.ISO(t.Now()))
	if v.Time(timestamp).IsZero() {
		return errors.New("Некорректное время снимка склада.")
	}
	complete := true
	if raw, exists := body["complete"]; exists {
		var valid bool
		complete, valid = raw.(bool)
		if !valid {
			return errors.New("Укажите, является ли снимок склада полным.")
		}
	}
	t.StockBaseline = M{"carrierId": id, "timestamp": timestamp, "items": items, "complete": complete}
	return nil
}
func (t *Tracker) transaction(c, event M, kind, label string, amount any) {
	a := append([]M{{"timestamp": event["timestamp"], "kind": kind, "label": label, "amount": numeric(amount), "balance": v.At(c, "finance", "balance")}}, v.Rows(c["transactions"])...)
	if len(a) > 100 {
		a = a[:100]
	}
	c["transactions"] = a
}
func applyFinance(c, values M, timestamp any) {
	f := v.Obj(c["finance"])
	for _, pair := range [][2]string{{"CarrierBalance", "balance"}, {"ReserveBalance", "reserveBalance"}, {"AvailableBalance", "availableBalance"}, {"ReservePercent", "reservePercent"}} {
		if value := numeric(values[pair[0]]); value != nil {
			f[pair[1]] = value
		}
	}
	rates := v.Obj(f["taxRates"])
	if value := numeric(values["TaxRate"]); value != nil {
		rates["all"] = value
	}
	for key, value := range values {
		if strings.HasPrefix(key, "TaxRate_") && numeric(value) != nil {
			rates[strings.TrimPrefix(key, "TaxRate_")] = numeric(value)
		}
	}
	f["updatedAt"] = timestamp
}
func (t *Tracker) moveStock(c M, key, label string, delta float64, timestamp any, kind string) {
	if key == "" || delta == 0 {
		return
	}
	items := v.Obj(c["items"])
	item := v.Obj(items[key])
	if len(item) == 0 {
		item = M{"key": key, "name": trading.Name(key, label), "count": 0, "netTransferred": 0, "basis": "По записанным перемещениям", "snapshotAt": nil}
	}
	item["count"] = math.Max(0, v.Num(item["count"])+delta)
	item["netTransferred"] = v.Num(item["netTransferred"]) + delta
	item["updatedAt"] = timestamp
	items[key] = item
	c["movements"] = append(v.Rows(c["movements"]), M{"key": key, "name": item["name"], "delta": delta, "timestamp": timestamp, "kind": kind})
	space := v.Obj(c["space"])
	if space["cargoTons"] != nil {
		space["cargoTons"] = math.Max(0, v.Num(space["cargoTons"])+delta)
		if space["freeTons"] != nil {
			space["freeTons"] = math.Max(0, v.Num(space["freeTons"])-delta)
		}
		space["updatedAt"] = timestamp
		space["estimated"] = true
	}
}
func (t *Tracker) Apply(event, context M) {
	typ := v.Str(event["event"])
	timestamp := event["timestamp"]
	if typ == "Docked" || typ == "Location" {
		id := v.Str(event["MarketID"])
		t.DockedCarrierID = ""
		if strings.Contains(strings.ToLower(v.Str(event["StationType"])), "carrier") || id == t.OwnerID {
			t.DockedCarrierID = id
		}
		t.DockedStation = v.Str(event["StationName"])
	}
	if typ == "Undocked" {
		t.DockedCarrierID = ""
		t.DockedStation = ""
	}
	c := t.Current()
	explicitID := v.Str(event["CarrierID"])
	if ownerEvents[typ] {
		if event["CarrierType"] != nil && event["CarrierType"] != "FleetCarrier" {
			t.OtherCarrierEvents++
			return
		}
		if explicitID == "" {
			return
		}
		c = t.Carriers[explicitID]
		if c == nil {
			c = newCarrier(explicitID)
			t.Carriers[explicitID] = c
		}
		t.OwnerID = explicitID
	}
	if c == nil || explicitID != "" && explicitID != t.OwnerID {
		return
	}
	fuel, space, jump := v.Obj(c["fuel"]), v.Obj(c["space"]), v.Obj(c["jump"])
	switch typ {
	case "CarrierBuy":
		for _, p := range [][2]string{{"Callsign", "callsign"}, {"Name", "name"}} {
			if event[p[0]] != nil {
				c[p[1]] = event[p[0]]
			}
		}
		if event["Location"] != nil {
			c["location"] = M{"system": event["Location"], "systemAddress": event["SystemAddress"], "bodyId": nil, "coordinates": nil, "updatedAt": timestamp}
		}
		t.transaction(c, event, "purchase", "Покупка флотоносца", -v.Num(event["Price"]))
	case "CarrierStats":
		for _, p := range [][2]string{{"Callsign", "callsign"}, {"Name", "name"}, {"DockingAccess", "dockingAccess"}, {"AllowNotorious", "allowNotorious"}, {"PendingDecommission", "pendingDecommission"}} {
			if event[p[0]] != nil {
				c[p[1]] = event[p[0]]
			}
		}
		c["statsAt"] = timestamp
		for _, p := range [][2]string{{"FuelLevel", "tankTons"}, {"JumpRangeCurr", "jumpRangeCurrent"}, {"JumpRangeMax", "jumpRangeMax"}} {
			if x := numeric(event[p[0]]); x != nil {
				fuel[p[1]] = x
			}
		}
		fuel["updatedAt"] = timestamp
		fuel["needsRefresh"] = false
		usage := v.Obj(event["SpaceUsage"])
		for _, p := range [][2]string{{"TotalCapacity", "capacityTons"}, {"Cargo", "cargoTons"}, {"CargoSpaceReserved", "reservedTons"}, {"Crew", "crewTons"}, {"ShipPacks", "shipPacksTons"}, {"ModulePacks", "modulePacksTons"}, {"FreeSpace", "freeTons"}} {
			if x := numeric(usage[p[0]]); x != nil {
				space[p[1]] = x
			}
		}
		space["updatedAt"] = timestamp
		space["estimated"] = false
		applyFinance(c, v.Obj(event["Finance"]), timestamp)
		if event["Crew"] != nil {
			services := M{}
			for _, r := range v.Rows(event["Crew"]) {
				key := v.Key(r["CrewRole"])
				if key == "" {
					continue
				}
				services[key] = M{"key": key, "name": v.Text(serviceNames[key], r["CrewRole"]), "installed": v.Bool(r["Activated"]), "enabled": v.Bool(r["Enabled"]), "crewName": r["CrewName"], "updatedAt": timestamp}
			}
			c["services"] = services
		}
	case "CarrierLocation", "CarrierJump":
		if typ == "CarrierJump" && v.Text(event["MarketID"], event["CarrierID"]) != t.OwnerID {
			break
		}
		c["location"] = M{"system": v.Text(event["StarSystem"]), "systemAddress": event["SystemAddress"], "bodyId": event["BodyID"], "coordinates": event["StarPos"], "updatedAt": timestamp}
		pending := v.Obj(jump["pending"])
		arrived := typ == "CarrierJump"
		if len(pending) > 0 && !v.Time(pending["departureTime"]).IsZero() && v.Str(event["StarSystem"]) == v.Str(pending["system"]) && !v.Time(timestamp).Before(v.Time(pending["departureTime"])) && (pending["bodyId"] == nil || event["BodyID"] == nil || v.Str(pending["bodyId"]) == v.Str(event["BodyID"])) {
			arrived = true
		}
		if arrived {
			jump["lastArrival"] = timestamp
			jump["pending"] = nil
			fuel["needsRefresh"] = true
		}
	case "CarrierJumpRequest":
		departure := v.Time(event["DepartureTime"])
		var millis any
		if !departure.IsZero() {
			millis = departure.UnixMilli()
		}
		jump["pending"] = M{"system": v.Text(event["SystemName"]), "body": event["Body"], "bodyId": event["BodyID"], "systemAddress": v.First(event["SystemAddress"], event["SystemID"]), "departureTime": event["DepartureTime"], "departureMs": millis, "requestedAt": timestamp}
	case "CarrierJumpCancelled":
		jump["pending"] = nil
	case "CarrierDepositFuel":
		if explicitID == t.OwnerID {
			if total := numeric(event["Total"]); total != nil {
				fuel["tankTons"] = total
			}
			fuel["depositedTons"] = v.Num(fuel["depositedTons"]) + math.Max(0, v.Num(event["Amount"]))
			fuel["updatedAt"] = timestamp
			fuel["needsRefresh"] = false
			t.transaction(c, event, "fuel", fmt.Sprintf("Заправлено трития: %g т", v.Num(event["Amount"])), nil)
		}
	case "CarrierFinance":
		applyFinance(c, event, timestamp)
	case "CarrierBankTransfer":
		applyFinance(c, event, timestamp)
		deposit := numeric(event["Deposit"])
		if deposit != nil {
			t.transaction(c, event, "bank", "Пополнение счёта", deposit)
		} else {
			t.transaction(c, event, "bank", "Снятие со счёта", -v.Num(event["Withdraw"]))
		}
	case "CarrierNameChanged":
		if event["Name"] != nil {
			c["name"] = event["Name"]
		}
		if event["Callsign"] != nil {
			c["callsign"] = event["Callsign"]
		}
	case "CarrierDockingPermission":
		for _, p := range [][2]string{{"DockingAccess", "dockingAccess"}, {"AllowNotorious", "allowNotorious"}} {
			if event[p[0]] != nil {
				c[p[1]] = event[p[0]]
			}
		}
	case "CarrierCrewServices":
		key := v.Key(event["CrewRole"])
		if key != "" {
			services := v.Obj(c["services"])
			service := v.Obj(services[key])
			service["key"] = key
			service["name"] = v.Text(serviceNames[key], event["CrewRole"])
			switch strings.ToLower(v.Str(event["Operation"])) {
			case "activate", "resume":
				service["installed"] = true
				service["enabled"] = true
			case "deactivate":
				service["installed"] = false
				service["enabled"] = false
			case "pause":
				service["enabled"] = false
			}
			service["crewName"] = v.First(event["CrewName"], service["crewName"])
			service["updatedAt"] = timestamp
			services[key] = service
			t.transaction(c, event, "service", "Услуга: "+v.Str(service["name"])+" · "+v.Str(event["Operation"]), nil)
		}
	case "CarrierTradeOrder":
		key := v.Key(event["Commodity"])
		if key != "" {
			orderKey := key + ":legal"
			if v.Bool(event["BlackMarket"]) {
				orderKey = key + ":black"
			}
			orders := v.Obj(c["orders"])
			if v.Bool(event["CancelTrade"]) {
				delete(orders, orderKey)
			} else {
				side := ""
				var quantity any
				if numeric(event["PurchaseOrder"]) != nil {
					side = "buy"
					quantity = numeric(event["PurchaseOrder"])
				} else if numeric(event["SaleOrder"]) != nil {
					side = "sell"
					quantity = numeric(event["SaleOrder"])
				}
				if side != "" {
					orders[orderKey] = M{"key": key, "name": trading.Name(key, event["Commodity_Localised"]), "side": side, "price": numeric(event["Price"]), "quantity": quantity, "blackMarket": v.Bool(event["BlackMarket"]), "timestamp": timestamp}
					if side == "sell" {
						items := v.Obj(c["items"])
						old := v.Obj(items[key])
						items[key] = M{"key": key, "name": trading.Name(key, event["Commodity_Localised"]), "count": quantity, "netTransferred": v.Num(old["netTransferred"]), "basis": "Количество в заявке продажи", "snapshotAt": timestamp, "updatedAt": timestamp}
					}
				}
			}
		}
	case "CargoTransfer":
		if t.IsOwnDockedMarket(nil) {
			for _, r := range v.Rows(event["Transfers"]) {
				key := v.Key(r["Type"])
				count := v.Num(r["Count"])
				direction := strings.ToLower(v.Str(r["Direction"]))
				if count <= 0 || direction != "tocarrier" && direction != "toship" {
					continue
				}
				if direction == "toship" {
					count = -count
				}
				t.moveStock(c, key, v.Str(r["Type_Localised"]), count, timestamp, "transfer")
				t.transaction(c, event, "cargo", trading.Name(key, r["Type_Localised"]), nil)
			}
		}
	case "MarketBuy", "MarketSell":
		if t.IsOwnDockedMarket(event["MarketID"]) {
			key := v.Key(event["Type"])
			count := v.Num(event["Count"])
			if count > 0 {
				if typ == "MarketBuy" {
					count = -count
				}
				t.moveStock(c, key, v.Str(event["Type_Localised"]), count, timestamp, "owner-market")
				t.transaction(c, event, "owner-market", "Собственный рынок: "+trading.Name(key, event["Type_Localised"]), nil)
			}
		}
	case "CarrierDecommission":
		c["pendingDecommission"] = true
		if n, ok := v.Number(event["ScrapTime"]); ok {
			c["scrapTime"] = v.ISO(time.Unix(int64(n), 0))
		} else {
			c["scrapTime"] = event["ScrapTime"]
		}
	case "CarrierCancelDecommission":
		c["pendingDecommission"] = false
		c["scrapTime"] = nil
	case "CarrierShipPack", "CarrierModulePack":
		cost := numeric(event["Cost"])
		amount := numeric(event["Refund"])
		if cost != nil {
			amount = -v.Num(cost)
		}
		t.transaction(c, event, "pack", "Комплект: "+v.Str(event["PackTheme"]), amount)
	}
	if strings.HasPrefix(typ, "Carrier") && (explicitID == t.OwnerID || typ == "CarrierJump") {
		c["updatedAt"] = timestamp
	}
	location := v.Obj(c["location"])
	if (typ == "Location" || typ == "FSDJump" || typ == "CarrierJump") && location["system"] == event["StarSystem"] && event["StarPos"] != nil {
		location["coordinates"] = event["StarPos"]
	}
	cmd := v.Obj(context["commander"])
	if cmd["system"] == location["system"] && location["coordinates"] == nil && cmd["coordinates"] != nil {
		location["coordinates"] = cmd["coordinates"]
	}
}
func (t *Tracker) warehouse(c M) M {
	if c == nil {
		return M{"items": []M{}, "knownTons": 0, "totalReportedTons": nil, "complete": false, "sourceLabel": "Склад ещё не прочитан", "updatedAt": nil}
	}
	items := map[string]M{}
	for key, raw := range v.Obj(c["items"]) {
		items[key] = v.Copy(v.Obj(raw))
	}
	baseline := t.StockBaseline
	hasBaseline := v.Str(baseline["carrierId"]) == t.OwnerID && baseline["items"] != nil
	complete := hasBaseline && baseline["complete"] != false
	if hasBaseline {
		items = map[string]M{}
		basis, after := "Ручной снимок склада", "После ручного снимка"
		if baseline["source"] == "Frontier CAPI" {
			basis, after = "Снимок Frontier CAPI", "После снимка Frontier CAPI"
		}
		for _, row := range v.Rows(baseline["items"]) {
			items[v.Str(row["key"])] = v.Merge(v.Copy(row), M{"netTransferred": 0, "basis": basis, "snapshotAt": baseline["timestamp"], "updatedAt": baseline["timestamp"]})
		}
		for _, move := range v.Rows(c["movements"]) {
			if v.Str(move["timestamp"]) <= v.Str(baseline["timestamp"]) {
				continue
			}
			key := v.Str(move["key"])
			row := items[key]
			if row == nil {
				row = M{"key": key, "name": move["name"], "count": 0, "netTransferred": 0, "basis": after, "snapshotAt": baseline["timestamp"]}
			}
			row["count"] = math.Max(0, v.Num(row["count"])+v.Num(move["delta"]))
			row["netTransferred"] = v.Num(row["netTransferred"]) + v.Num(move["delta"])
			row["updatedAt"] = move["timestamp"]
			items[key] = row
		}
		for key, raw := range v.Obj(c["items"]) {
			row := v.Obj(raw)
			if v.Str(row["snapshotAt"]) > v.Str(baseline["timestamp"]) {
				items[key] = v.Copy(row)
			}
		}
	}
	rows := []M{}
	tons := float64(0)
	updated := v.At(c, "space", "updatedAt")
	for _, row := range items {
		if v.Num(row["count"]) > 0 || v.Num(row["netTransferred"]) != 0 || row["snapshotAt"] != nil {
			rows = append(rows, row)
			tons += v.Num(row["count"])
			if v.Str(row["updatedAt"]) > v.Str(updated) {
				updated = row["updatedAt"]
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return v.Num(rows[i]["count"]) > v.Num(rows[j]["count"]) })
	label := "Заявки продажи и записанные перемещения"
	if complete {
		label = "Ручной снимок и последующие записанные операции"
		if baseline["source"] == "Frontier CAPI" {
			label = "Снимок Frontier CAPI и последующие записанные операции"
		}
	} else if hasBaseline {
		label = "Неполный ручной снимок известных остатков и последующие операции"
	}
	return M{"items": rows, "knownTons": tons, "totalReportedTons": v.At(c, "space", "cargoTons"), "complete": complete, "sourceLabel": label, "updatedAt": updated, "manualAt": baseline["timestamp"], "caveat": "Сделки других командиров и начальный состав склада могут отсутствовать в журнале."}
}
func (t *Tracker) Snapshot(context M) M {
	now := t.Now()
	if supplied := v.Time(context["now"]); !supplied.IsZero() {
		now = supplied
	}
	c := t.Current()
	warehouse := t.warehouse(c)
	fuel := emptyFuel()
	finance := emptyFinance()
	space := emptySpace()
	if c != nil {
		fuel = v.Copy(v.Obj(c["fuel"]))
		finance = v.Copy(v.Obj(c["finance"]))
		space = v.Copy(v.Obj(c["space"]))
	}
	fuel["reserveTons"] = nil
	for _, item := range v.Rows(warehouse["items"]) {
		if item["key"] == "tritium" {
			fuel["reserveTons"] = item["count"]
		}
	}
	if fuel["reserveTons"] == nil && v.Bool(warehouse["complete"]) {
		fuel["reserveTons"] = 0
	}
	finance["weeklyUpkeep"] = t.Planning["weeklyUpkeep"]
	finance["runwayWeeks"] = nil
	if finance["balance"] != nil && v.Num(t.Planning["weeklyUpkeep"]) > 0 {
		finance["runwayWeeks"] = math.Max(0, v.Num(finance["balance"])/v.Num(t.Planning["weeklyUpkeep"]))
	}
	jump := v.Obj(c["jump"])
	pending := v.Obj(jump["pending"])
	arrival := v.Time(jump["lastArrival"])
	phase := "idle"
	var cooldown, seconds any
	if !arrival.IsZero() {
		cooldown = arrival.Add(5 * time.Minute).UnixMilli()
		if now.Before(arrival.Add(5 * time.Minute)) {
			phase = "cooldown"
		}
	}
	if len(pending) > 0 {
		phase = "countdown"
		departure := v.Time(pending["departureTime"])
		if !departure.IsZero() {
			seconds = math.Max(0, departure.Sub(now).Seconds())
			if !now.Before(departure) {
				phase = "departure-passed"
			}
		}
	}
	var lastArrival any
	if !arrival.IsZero() {
		lastArrival = M{"timestamp": jump["lastArrival"], "system": v.At(c, "location", "system"), "bodyId": v.At(c, "location", "bodyId")}
	}
	services, orders := []M{}, []M{}
	for _, r := range v.Obj(c["services"]) {
		services = append(services, v.Copy(v.Obj(r)))
	}
	for _, r := range v.Obj(c["orders"]) {
		orders = append(orders, v.Copy(v.Obj(r)))
	}
	sort.Slice(orders, func(i, j int) bool { return v.Str(orders[i]["timestamp"]) > v.Str(orders[j]["timestamp"]) })
	plan := v.Copy(t.Planning)
	plan["tritiumNeededTank"] = nil
	plan["tritiumNeededReserve"] = nil
	if fuel["tankTons"] != nil {
		plan["tritiumNeededTank"] = math.Max(0, v.Num(plan["targetTankTons"])-v.Num(fuel["tankTons"]))
	}
	if fuel["reserveTons"] != nil {
		plan["tritiumNeededReserve"] = math.Max(0, v.Num(plan["reserveTritium"])-v.Num(fuel["reserveTons"]))
	}
	advisories := []M{}
	if c != nil && c["statsAt"] == nil {
		advisories = append(advisories, M{"level": "info", "text": "Откройте управление флотоносцем в игре для актуального снимка."})
	}
	if v.Bool(fuel["needsRefresh"]) {
		advisories = append(advisories, M{"level": "info", "text": "После прыжка обновите управление флотоносцем: запас топлива мог измениться."})
	}
	if fuel["tankTons"] != nil && v.Num(fuel["tankTons"]) < 100 {
		advisories = append(advisories, M{"level": "warn", "text": "В баке менее 100 т трития."})
	}
	if v.Num(finance["runwayWeeks"]) > 0 && v.Num(finance["runwayWeeks"]) < 4 {
		advisories = append(advisories, M{"level": "warn", "text": "Баланса хватит менее чем на четыре недели."})
	}
	access := map[string]string{"all": "Все", "none": "Только владелец", "friends": "Друзья", "squadron": "Эскадрилья", "squadronfriends": "Друзья и эскадрилья"}
	return M{"owned": c != nil, "id": c["id"], "name": v.Str(c["name"]), "callsign": v.Str(c["callsign"]), "updatedAt": c["updatedAt"], "statsAt": c["statsAt"], "location": v.Copy(v.Obj(c["location"])), "dockingAccess": c["dockingAccess"], "dockingAccessLabel": v.Text(access[v.Str(c["dockingAccess"])], c["dockingAccess"], "Неизвестно"), "allowNotorious": c["allowNotorious"], "pendingDecommission": v.Bool(c["pendingDecommission"]), "scrapTime": c["scrapTime"], "dockedHere": t.IsOwnDockedMarket(nil), "finance": finance, "fuel": fuel, "space": space, "services": services, "warehouse": warehouse, "orders": orders, "jump": M{"pending": jump["pending"], "lastArrival": lastArrival, "cooldownUntilMs": cooldown, "cooldownEstimated": true, "phase": phase, "secondsToDeparture": seconds, "arrivalConfirmed": len(pending) == 0 && len(v.Obj(c["location"])) > 0}, "transactions": v.Clone(c["transactions"]), "advisories": advisories, "planning": plan}
}
