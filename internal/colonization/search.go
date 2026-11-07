package colonization

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/value"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Search is cancellable and synchronous. Call it from a worker; Snapshot may be
// read concurrently to present progressive pages. Starting a new search cancels the old one.
func (p *Planner) Search(ctx context.Context, input M) (M, error) {
	p.mu.Lock()
	originValue, originProvided := input["originSystem"]
	if !originProvided {
		originValue = p.config["originSystem"]
	}
	factionValue, factionProvided := input["faction"]
	if !factionProvided {
		factionValue = p.config["faction"]
	}
	origin := text(originValue, 200)
	faction := text(factionValue, 200)
	if origin == "" {
		p.mu.Unlock()
		return nil, errors.New("Введите систему происхождения.")
	}
	if p.cancel != nil {
		p.cancel()
	}
	child, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.searchSerial++
	serial := p.searchSerial
	p.config["originSystem"], p.config["faction"] = origin, faction
	p.search = initialSearch()
	p.search["running"], p.search["status"] = true, "resolving"
	p.search["options"] = M{"originSystem": origin, "faction": faction, "rangeLy": 15}
	p.mu.Unlock()
	defer cancel()
	err := p.runSearch(child, origin, faction, serial)
	p.mu.Lock()
	defer p.mu.Unlock()
	if serial != p.searchSerial {
		return copyM(p.search), context.Canceled
	}
	p.cancel = nil
	p.search["running"] = false
	if err != nil {
		p.search["status"] = "error"
		p.search["error"] = text(err.Error(), 600)
		complete := value.Obj(p.search["completeness"])
		complete["complete"], complete["reason"] = false, "Поиск прерван; список неполный."
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.search["status"], p.search["error"] = "cancelled", ""
			complete["reason"] = "Поиск отменён; показана только полученная часть."
		}
		p.search["completeness"] = complete
	} else {
		p.search["status"] = "complete"
	}
	return copyM(p.search), err
}
func (p *Planner) CancelSearch() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.searchSerial++
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	if value.Bool(p.search["running"]) {
		p.search["running"], p.search["status"], p.search["error"] = false, "cancelled", ""
		complete := value.Obj(p.search["completeness"])
		complete["complete"], complete["reason"] = false, "Поиск отменён; список неполный."
		p.search["completeness"] = complete
	}
}
func (p *Planner) searchUpdate(serial uint64, fn func(M)) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if serial != p.searchSerial {
		return context.Canceled
	}
	fn(p.search)
	return nil
}
func (p *Planner) request(ctx context.Context, method, path string, body M) (M, error) {
	var r io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		r = bytes.NewReader(b)
	}
	request, e := http.NewRequestWithContext(ctx, method, p.baseURL+path, r)
	if e != nil {
		return nil, e
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, e := p.client.Do(request)
	if e != nil {
		return nil, e
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Spansh: HTTP %d. Повторите поиск позднее.", response.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(response.Body, 10*1024*1024+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 10*1024*1024 {
		return nil, errors.New("Ответ Spansh слишком большой.")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var out M
	if e = d.Decode(&out); e != nil {
		return nil, errors.New("Spansh вернул некорректный JSON.")
	}
	return out, nil
}
func (p *Planner) runSearch(ctx context.Context, origin, faction string, serial uint64) error {
	found, e := p.request(ctx, http.MethodGet, "/systems/field_values/system_names?q="+url.QueryEscape(origin), nil)
	if e != nil {
		return e
	}
	var exact M
	for _, r := range rows(found["min_max"]) {
		if equal(r["name"], origin) {
			exact = r
			break
		}
	}
	if str(exact["id64"]) == "" {
		return errors.New("Spansh не нашёл точного совпадения системы. Укажите полное название.")
	}
	response, e := p.request(ctx, http.MethodGet, "/system/"+url.PathEscape(str(exact["id64"])), nil)
	if e != nil {
		return e
	}
	record := value.Obj(response["record"])
	coords := coordinates(record)
	if coords == nil || !equal(record["name"], origin) {
		return errors.New("Spansh не вернул координаты выбранной системы.")
	}
	factions := []M{}
	for _, r := range rows(record["minor_faction_presences"]) {
		name := text(r["name"], 200)
		if name != "" {
			influence, ok := value.Number(r["influence"])
			factions = append(factions, M{"name": name, "influence": value.Nullable(influence, ok), "government": text(r["government"], 200), "allegiance": text(r["allegiance"], 200)})
		}
	}
	stations := []M{}
	for _, r := range rows(record["stations"]) {
		typ := strings.ToLower(str(r["type"]))
		if strings.Contains(typ, "carrier") || strings.Contains(typ, "megaship") {
			continue
		}
		var contact any
		for _, service := range stringsOf(r["services"]) {
			if colonisationService(service) {
				contact = true
			}
		}
		distance, known := value.Number(r["distance_to_arrival"])
		stations = append(stations, M{"marketId": marketID(r["market_id"]), "name": text(r["name"], 200), "system": record["name"], "type": text(r["type"], 200), "faction": factionName(r["controlling_minor_faction"]), "hasColonisationContact": contact, "distanceLs": value.Nullable(distance, known), "updatedAt": text(record["updated_at"], 200), "source": "spansh"})
	}
	p.mu.RLock()
	observed := []M{}
	for _, r := range p.stations {
		observed = append(observed, copyM(r))
	}
	p.mu.RUnlock()
	for _, r := range observed {
		if !equal(r["system"], record["name"]) {
			continue
		}
		var contact any
		for _, s := range stringsOf(r["services"]) {
			if colonisationService(s) {
				contact = true
			}
		}
		overlay := M{"marketId": r["marketId"], "name": r["name"], "system": r["system"], "faction": r["faction"], "hasColonisationContact": contact, "updatedAt": r["timestamp"], "source": "journal"}
		known := false
		for _, station := range stations {
			if str(station["marketId"]) != "" && str(station["marketId"]) == str(r["marketId"]) {
				merge(station, overlay)
				known = true
				break
			}
		}
		if !known {
			stations = append(stations, overlay)
		}
	}
	claimStations := []M{}
	hasContact := false
	for _, station := range stations {
		if faction == "" || equal(station["faction"], faction) {
			claimStations = append(claimStations, station)
			hasContact = hasContact || value.Bool(station["hasColonisationContact"])
		}
		if name := str(station["faction"]); name != "" {
			known := false
			for _, f := range factions {
				if equal(f["name"], name) {
					known = true
				}
			}
			if !known {
				factions = append(factions, M{"name": name, "influence": nil})
			}
		}
	}
	warnings := []string{}
	if faction != "" && len(claimStations) == 0 {
		warnings = append(warnings, "У выбранной фракции не найдено станции в системе происхождения. Одного присутствия фракции в системе недостаточно.")
	}
	if !hasContact {
		warnings = append(warnings, "Наличие контакта колонизации не подтверждено. Проверьте службу и контролирующую фракцию станции в игре.")
	}
	if value.Bool(record["needs_permit"]) {
		warnings = append(warnings, "Система происхождения требует разрешение. Убедитесь, что можете посетить выбранную станцию.")
	}
	warnings = append(warnings, "База сообщества не гарантирует отсутствие свежей заявки. Окончательную доступность показывает контакт колонизации в игре.")
	if e = p.searchUpdate(serial, func(search M) {
		originData := M{"name": record["name"], "id64": str(record["id64"]), "coordinates": coords, "population": natural(record["population"], 1e12), "updatedAt": text(record["updated_at"], 200), "source": "spansh"}
		search["origin"], search["origins"], search["factions"], search["claimStations"], search["warnings"], search["status"] = originData, []M{originData}, factions, claimStations, warnings, "searching"
	}); e != nil {
		return e
	}
	collected := map[string]M{}
	seenProvider := map[string]bool{}
	providerTotal := -1
	providerRows := 0
	changed := false
	for page := 0; page < 100; page++ {
		if e = ctx.Err(); e != nil {
			return e
		}
		data, e := p.request(ctx, http.MethodPost, "/systems/search", M{"filters": M{"distance": M{"min": 0, "max": 15}}, "sort": []M{{"distance": M{"direction": "asc"}}}, "size": 100, "page": page, "reference_coords": coords})
		if e != nil {
			return e
		}
		if data["results"] == nil {
			return errors.New("Неизвестный формат ответа Spansh. Список не считается полным.")
		}
		total, ok := value.Number(data["count"])
		if !ok || total < 0 || math.Trunc(total) != total {
			return errors.New("Spansh не вернул число систем.")
		}
		if providerTotal >= 0 && providerTotal != int(total) {
			changed = true
		}
		providerTotal = int(total)
		batch := rows(data["results"])
		providerRows += len(batch)
		newRows := 0
		for _, r := range batch {
			id := firstText(r["id64"], r["id"], r["name"])
			if !seenProvider[id] {
				newRows++
				seenProvider[id] = true
			}
			if equal(r["name"], record["name"]) {
				continue
			}
			candidate := ClassifyCandidate(r, coords, value.ISO(p.now()))
			if distance := candidate["distanceLy"]; distance != nil && num(distance) > 15.000001 {
				continue
			}
			collected[str(candidate["id"])] = candidate
		}
		results := []M{}
		for _, r := range collected {
			results = append(results, r)
		}
		sort.SliceStable(results, func(i, j int) bool {
			a, b := results[i]["distanceLy"], results[j]["distanceLy"]
			if a == nil && b != nil {
				return false
			}
			if a != nil && b == nil {
				return true
			}
			if num(a) != num(b) {
				return num(a) < num(b)
			}
			return str(results[i]["name"]) < str(results[j]["name"])
		})
		if e = p.searchUpdate(serial, func(search M) {
			search["results"] = results
			search["completeness"] = M{"complete": false, "providerTotal": providerTotal, "fetched": providerRows, "pages": page + 1, "reason": "Загружаются страницы Spansh."}
			if changed {
				ws := stringsOf(search["warnings"])
				message := "Во время поиска база обновилась; число систем изменилось. Повторите поиск."
				if !has(ws, message) {
					search["warnings"] = append(ws, message)
				}
			}
		}); e != nil {
			return e
		}
		if providerRows >= providerTotal {
			if len(seenProvider) < providerTotal {
				return errors.New("Spansh вернул повторяющиеся записи. Список неполный.")
			}
			return p.searchUpdate(serial, func(search M) {
				complete := value.Obj(search["completeness"])
				complete["complete"] = !changed
				if !changed {
					complete["reason"] = "Получены все страницы систем, известных Spansh в радиусе 15 св. лет. Неизвестные базе системы не включены."
				} else {
					complete["reason"] = "База изменилась при поиске; повторите запрос."
				}
				search["completeness"] = complete
			})
		}
		if len(batch) == 0 {
			return errors.New("Spansh вернул пустую страницу до конца списка.")
		}
		if page > 0 && newRows == 0 {
			return errors.New("Spansh вернул повторяющиеся страницы. Список неполный.")
		}
	}
	return p.searchUpdate(serial, func(search M) {
		complete := value.Obj(search["completeness"])
		complete["reason"] = "Достигнут предел 10 000 записей. Полученный список неполный."
		complete["complete"] = false
		search["completeness"] = complete
	})
}
func factionName(v any) string {
	if m, ok := v.(M); ok {
		return text(firstText(m["name"], m["Name"]), 200)
	}
	return text(v, 200)
}
func colonisationService(v any) bool {
	return has([]string{"systemcolonisation", "systemcolonization", "colonisation", "colonization"}, value.Key(v))
}
func ClassifyCandidate(row, originCoordinates M, checkedAt string) M {
	coords := coordinates(row)
	distance := dist(coords, originCoordinates)
	population := natural(row["population"], 1e12)
	reasons, unknown := []string{}, []string{}
	permanent := 0
	for _, station := range rows(row["stations"]) {
		if !strings.Contains(strings.ToLower(str(station["type"])), "carrier") {
			permanent++
		}
	}
	if num(population) > 0 {
		reasons = append(reasons, "Есть население.")
	}
	if population == nil {
		unknown = append(unknown, "Население неизвестно.")
	}
	if value.Bool(row["needs_permit"]) {
		reasons = append(reasons, "Требуется разрешение.")
	}
	if value.Bool(row["is_colonised"]) {
		reasons = append(reasons, "Система уже колонизирована.")
	}
	if value.Bool(row["is_being_colonised"]) {
		reasons = append(reasons, "Отмечена активная колонизация.")
	}
	if permanent > 0 {
		reasons = append(reasons, "В базе есть постоянные станции или поселения.")
	}
	for _, field := range []struct{ key, message string }{{"needs_permit", "Разрешение не проверено."}, {"is_colonised", "Статус колонии не наблюдался."}, {"is_being_colonised", "Свежая заявка не проверена."}} {
		if _, ok := row[field.key].(bool); !ok {
			unknown = append(unknown, field.message)
		}
	}
	if coords == nil {
		unknown = append(unknown, "Нет координат.")
	}
	if distance != nil && num(distance) > 15.000001 {
		reasons = append(reasons, "Дальше 15 св. лет от системы происхождения.")
	}
	availability := "candidate"
	if len(reasons) > 0 {
		availability = "unavailable"
	} else if len(unknown) > 0 {
		availability = "unknown"
	}
	id := firstText(row["id64"], row["id"], row["name"])
	id64 := firstText(row["id64"], row["id"])
	factions := []string{}
	for _, f := range rows(row["minor_faction_presences"]) {
		factions = append(factions, text(f["name"], 200))
	}
	var systemURL any
	if id64 != "" {
		systemURL = "https://spansh.co.uk/system/" + url.PathEscape(id64)
	}
	return M{"id": id, "id64": id64, "system": text(row["name"], 200), "name": text(row["name"], 200), "coordinates": coords, "distanceLy": distance, "population": population, "permit": nullableBool(row["needs_permit"]), "isColonised": nullableBool(row["is_colonised"]), "isBeingColonised": nullableBool(row["is_being_colonised"]), "availability": availability, "reasons": append(reasons, unknown...), "factions": factions, "permanentStationCount": permanent, "updatedAt": text(row["updated_at"], 200), "checkedAt": checkedAt, "source": "spansh", "url": systemURL}
}
func nullableBool(v any) any {
	if b, ok := v.(bool); ok {
		return b
	}
	return nil
}
