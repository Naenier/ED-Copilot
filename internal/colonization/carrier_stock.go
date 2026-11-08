package colonization

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"time"

	"github.com/Naenier/ED-Copilot/internal/value"
)

// CAPI is an observation, not a reservation or an assurance that another
// squadron member has not moved the shared cargo. Never merge the squadron
// bank, sale orders, the ship hold or a manual tracker baseline into this list.
// Endpoint and shape verified against EDDiscovery/CAPI CAPI.cs ("/squadron")
// and Squadrons.cs ("squadronCarrier") on 2026-11-08:
// https://github.com/EDDiscovery/CAPI/blob/master/CAPI/CAPI.cs
// https://github.com/EDDiscovery/CAPI/blob/master/CAPI/Squadrons.cs
const carrierStockMaxAge = 15 * time.Minute

func (p *Planner) clearSquadronAccess() {
	p.config["squadronWarehouseAccessConfirmed"] = false
	delete(p.config, "squadronWarehouseAccessFID")
	delete(p.config, "squadronWarehouseAccessCarrierID")
}

func (p *Planner) squadronAccessConfirmed() bool {
	id := marketID(value.At(p.carrierStocks, "squadron", "id"))
	boundID := str(p.config["squadronWarehouseAccessCarrierID"])
	return value.Bool(p.config["squadronWarehouseAccessConfirmed"]) && p.carrierOwner != "" && str(p.config["squadronWarehouseAccessFID"]) == p.carrierOwner && boundID != "" && (id == "" || boundID == id)
}

func stockQuantity(raw any) any {
	n, ok := value.Number(raw)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 1e9 || math.Trunc(n) != n {
		return nil
	}
	return n
}

func readCarrierStock(raw M, received any) M {
	id := marketID(value.At(raw, "market", "id"))
	if id == "" || value.Time(received).IsZero() || !isRowArray(raw["cargo"]) {
		return nil
	}
	if list, ok := raw["cargo"].([]any); ok && len(rows(raw["cargo"])) != len(list) {
		return nil
	}
	if len(rows(raw["cargo"])) > 1000 {
		return nil
	}
	items := M{}
	for _, row := range rows(raw["cargo"]) {
		key := value.Key(row["commodity"])
		count := stockQuantity(row["qty"])
		if key == "" || count == nil {
			return nil // A malformed complete list must not manufacture a zero.
		}
		for _, flag := range []string{"stolen", "mission"} {
			if rawFlag, exists := row[flag]; exists && rawFlag != nil {
				if _, valid := rawFlag.(bool); !valid {
					return nil
				}
			}
		}
		if value.Bool(row["stolen"]) || value.Bool(row["mission"]) {
			continue
		}
		items[key] = num(items[key]) + num(count)
	}
	return M{"id": id, "callsign": text(value.At(raw, "name", "callsign"), 40), "receivedAt": validDate(received), "updatedAt": validDate(received), "items": items, "complete": true}
}

// ApplyCAPI accepts only the result verified for the currently selected
// commander. Caches contain commodity counts and identities, never credentials.
func (p *Planner) ApplyCAPI(result, commander M) {
	p.mu.Lock()
	defer p.mu.Unlock()
	status := value.Obj(result["status"])
	fid := str(commander["fid"])
	if fid == "" || !value.Bool(status["connected"]) || value.Bool(status["commanderMismatch"]) || str(status["fid"]) != fid || p.commander != "" && p.commander != fid {
		return
	}
	if p.carrierOwner != fid {
		p.carrierStocks = M{}
		p.clearSquadronAccess()
	}
	p.carrierOwner = fid
	data, observed := value.Obj(result["data"]), value.Obj(value.At(result, "metadata", "observedAt"))
	for _, pair := range [][2]string{{"fleet", "carrier"}, {"squadron", "squadron"}} {
		kind, endpoint := pair[0], pair[1]
		raw, present := data[endpoint]
		if !present {
			continue // Profile-only sync must not refresh a carrier observation.
		}
		record := value.Obj(raw)
		if kind == "squadron" {
			record = value.Obj(record["squadronCarrier"])
		}
		stock := readCarrierStock(record, observed[endpoint])
		if kind == "fleet" && raw == nil && !value.Time(observed[endpoint]).IsZero() {
			// A successful 204 from /fleetcarrier confirms no personal carrier.
			stock = M{"receivedAt": validDate(observed[endpoint]), "updatedAt": validDate(observed[endpoint]), "complete": true, "absent": true, "items": M{}}
		}
		if stock == nil {
			stock = M{"id": marketID(value.At(record, "market", "id")), "receivedAt": validDate(observed[endpoint]), "complete": false, "items": M{}}
		}
		previous := value.Obj(p.carrierStocks[kind])
		started := value.Time(value.At(result, "metadata", "requestStartedAt", endpoint))
		// A response requested before a journal transfer may describe the old
		// hold even though it arrived afterwards. Keep the corrected observation
		// (or unknown if none exists); do not manufacture stock in both holds.
		if !started.IsZero() && (value.Time(p.carrierMovementAt[str(stock["id"])]).After(started) || str(previous["id"]) == str(stock["id"]) && value.Time(previous["updatedAt"]).After(started)) {
			continue
		}
		if !value.Time(previous["receivedAt"]).IsZero() && !value.Time(stock["receivedAt"]).After(value.Time(previous["receivedAt"])) {
			continue
		}
		// /fleetcarrier can return an older server-side hold even when the HTTP
		// request itself starts after a local transfer. Receipt time does not
		// prove that the payload already includes that transfer. Retain our
		// baseline + observed deltas for at most 15 minutes after the latest
		// pending movement. The original receipt still controls stock freshness;
		// protecting a recent movement must not make the older baseline fresh.
		// After the bounded guard, an absolute verified observation can include
		// legitimate transactions by other commanders and may replace it.
		// In particular, never add those deltas to the incoming payload: it may
		// already include some or all of them.
		latestPending := value.Time(previous["receivedAt"])
		for _, rawTime := range value.Obj(previous["pendingMovements"]) {
			if moved := value.Time(rawTime); moved.After(latestPending) {
				latestPending = moved
			}
		}
		lease := latestPending.Add(carrierStockMaxAge)
		if str(previous["id"]) == str(stock["id"]) && value.Time(stock["receivedAt"]).Before(lease) && pendingCarrierStockDiffers(previous, stock) {
			previous["awaitingConfirmation"] = true
			continue
		}
		if kind == "squadron" && str(stock["id"]) != "" && str(p.config["squadronWarehouseAccessCarrierID"]) != str(stock["id"]) {
			p.clearSquadronAccess()
		}
		p.carrierStocks[kind] = stock
	}
}

func (p *Planner) restoreCarrierStocks(saved M) {
	fid := text(saved["fid"], 100)
	if fid == "" {
		return
	}
	for _, kind := range []string{"fleet", "squadron"} {
		raw := value.Obj(value.At(saved, "warehouses", kind))
		_, itemsValid := raw["items"].(map[string]any)
		absent := kind == "fleet" && value.Bool(raw["absent"]) && len(value.Obj(raw["items"])) == 0
		if marketID(raw["id"]) == "" && !absent || value.Time(raw["receivedAt"]).IsZero() || !value.Bool(raw["complete"]) || !itemsValid || len(value.Obj(raw["items"])) > 1000 {
			continue
		}
		items, valid := M{}, true
		for key, amount := range value.Obj(raw["items"]) {
			count := stockQuantity(amount)
			if value.Key(key) != key || count == nil {
				valid = false
				break
			}
			items[key] = count
		}
		if valid {
			pending := M{}
			for key, rawTime := range value.Obj(raw["pendingMovements"]) {
				at := value.Time(rawTime)
				if _, exists := items[key]; exists && value.Key(key) == key && !at.IsZero() && at.After(value.Time(raw["receivedAt"])) && !at.After(value.Time(raw["updatedAt"])) {
					pending[key] = validDate(rawTime)
				}
			}
			// Older caches recorded the corrected items and one update time, but
			// not the per-commodity fence. Protect that already observed projection
			// conservatively; do not reconstruct quantities from historical deltas.
			if _, exists := raw["pendingMovements"]; !exists && len(uniqueStrings(raw["movementKeys"], 1000)) > 0 && value.Time(raw["updatedAt"]).After(value.Time(raw["receivedAt"])) {
				for key := range items {
					pending[key] = validDate(raw["updatedAt"])
				}
			}
			p.carrierStocks[kind] = M{"id": marketID(raw["id"]), "callsign": text(raw["callsign"], 40), "receivedAt": validDate(raw["receivedAt"]), "updatedAt": validDate(raw["updatedAt"]), "complete": true, "absent": absent, "items": items, "movementKeys": uniqueStrings(raw["movementKeys"], 1000), "pendingMovements": pending, "awaitingConfirmation": value.Bool(raw["awaitingConfirmation"])}
		}
	}
	p.carrierOwner = fid
}

func pendingCarrierStockDiffers(previous, incoming M) bool {
	if !value.Bool(previous["complete"]) {
		return false
	}
	for key := range value.Obj(previous["pendingMovements"]) {
		if !value.Bool(incoming["complete"]) || num(value.Obj(previous["items"])[key]) != num(value.Obj(incoming["items"])[key]) {
			return true
		}
	}
	return false
}

func (p *Planner) applyCarrierMovement(event, context M) {
	if fid := str(value.At(context, "commander", "fid")); fid != "" && p.carrierOwner != "" && fid != p.carrierOwner {
		return
	}
	id := marketID(firstText(event["CarrierID"], event["MarketID"], context["carrierId"], value.At(context, "commander", "marketId")))
	stamp := value.Time(event["timestamp"])
	if id == "" || stamp.IsZero() {
		return
	}
	if stamp.After(value.Time(p.carrierMovementAt[id])) {
		if len(p.carrierMovementAt) >= 1000 {
			p.carrierMovementAt = M{}
		}
		p.carrierMovementAt[id] = validDate(event["timestamp"])
	}
	raw, _ := json.Marshal(M{"event": event, "file": context["file"], "offset": context["offset"]})
	digest := sha256.Sum256(raw)
	identity := hex.EncodeToString(digest[:])
	for _, kind := range []string{"fleet", "squadron"} {
		s := value.Obj(p.carrierStocks[kind])
		if str(s["id"]) != id || !value.Bool(s["complete"]) || !stamp.After(value.Time(s["receivedAt"])) || stamp.Before(value.Time(s["updatedAt"])) {
			continue
		}
		keys := stringsOf(s["movementKeys"])
		if stamp.After(value.Time(s["updatedAt"])) {
			keys = nil
		}
		if has(keys, identity) {
			continue
		}
		if len(keys) >= 1000 {
			s["complete"] = false
			continue
		}
		s["movementKeys"] = append(keys, identity)
		items := value.Obj(s["items"])
		move := func(raw any, amount any, direction string) {
			key, count := value.Key(raw), stockQuantity(amount)
			if key == "" || count == nil {
				s["complete"] = false
				return
			}
			delta := num(count)
			if direction == "toship" {
				delta = -delta
			} else if direction != "tocarrier" {
				s["complete"] = false
				return
			}
			if num(items[key])+delta < 0 {
				s["complete"] = false // The baseline cannot explain this withdrawal.
			}
			items[key] = math.Max(0, num(items[key])+delta)
			pending := value.Obj(s["pendingMovements"])
			pending[key] = validDate(event["timestamp"])
			s["pendingMovements"] = pending
		}
		if str(event["event"]) == "CargoTransfer" {
			for _, transfer := range rows(event["Transfers"]) {
				if value.Bool(transfer["Stolen"]) || str(transfer["MissionID"]) != "" && str(transfer["MissionID"]) != "0" {
					continue
				}
				move(transfer["Type"], transfer["Count"], str(transfer["Direction"]))
			}
		} else if !value.Bool(event["BlackMarket"]) {
			direction := "toship"
			if str(event["event"]) == "MarketSell" {
				direction = "tocarrier"
			}
			move(event["Type"], event["Count"], direction)
		}
		s["updatedAt"] = validDate(event["timestamp"])
	}
}

func (p *Planner) carrierWarehouses(context M) ([]M, *stock) {
	enabled := value.Bool(p.config["includeCarrierWarehouse"])
	squadronAccess := p.squadronAccessConfirmed()
	combined := &stock{items: map[string]float64{}, complete: true, excluded: !enabled}
	result, counted := []M{}, map[string]bool{}
	fid := str(value.At(context, "commander", "fid"))
	for _, kind := range []string{"fleet", "squadron"} {
		s := value.Obj(p.carrierStocks[kind])
		if fid == "" || fid != p.carrierOwner {
			s = M{}
		}
		age := p.now().Sub(value.Time(s["receivedAt"]))
		known := fid != "" && fid == p.carrierOwner && value.Bool(s["complete"]) && age >= 0 && age <= carrierStockMaxAge
		included := enabled && (kind == "fleet" || squadronAccess)
		reason := "Ожидается свежий снимок склада Frontier. После входа данные обновляются автоматически."
		if known {
			reason = "Снимок Frontier CAPI; данные сервера могут запаздывать."
			if len(value.Obj(s["pendingMovements"])) > 0 || value.Bool(s["awaitingConfirmation"]) {
				reason = "Склад скорректирован по журналу. Frontier пока не подтвердил последние перемещения."
			}
			if value.Bool(s["absent"]) {
				reason = "Frontier подтвердил отсутствие личного флотоносца."
			}
		}
		if kind == "squadron" && !squadronAccess {
			reason = "Общий склад не учтён: подтвердите право вывоза груза."
		}
		list, total := []M{}, float64(0)
		if known {
			keys := []string{}
			for key := range value.Obj(s["items"]) {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				amount := num(value.Obj(s["items"])[key])
				list = append(list, M{"key": key, "count": amount})
				total += amount
			}
		}
		var knownTons any
		if known {
			knownTons = total
		}
		accessible := fid != "" && fid == p.carrierOwner && (kind == "fleet" || squadronAccess)
		result = append(result, M{"kind": kind, "id": s["id"], "callsign": s["callsign"], "known": known, "accessible": accessible, "included": included, "receivedAt": s["receivedAt"], "updatedAt": s["updatedAt"], "awaitingConfirmation": len(value.Obj(s["pendingMovements"])) > 0 || value.Bool(s["awaitingConfirmation"]), "knownTons": knownTons, "items": list, "reason": reason})
		if !included {
			continue
		}
		if !known {
			combined.complete = false
			continue
		}
		if counted[str(s["id"])] {
			continue
		}
		counted[str(s["id"])] = true
		for key, amount := range value.Obj(s["items"]) {
			combined.items[key] += num(amount)
		}
	}
	return result, combined
}
