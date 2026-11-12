package carrier

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const Capacity = 25000
const TankCapacity = 1000

func EmptyRouteState() M {
	return M{"running": false, "status": "idle", "error": nil, "options": DefaultRouteOptions(), "jobId": nil, "source": "Spansh", "sourceUrl": "https://spansh.co.uk/fleet-carrier", "updatedAt": nil, "progress": M{"stage": "idle", "label": "", "completedJumps": 0, "totalJumps": 0, "nextIndex": nil}, "route": nil, "summary": nil, "warnings": []string{}, "assumptions": []string{}}
}
func DefaultRouteOptions() M {
	return M{"sourceSystem": "", "destinationSystem": "", "waypoints": []string{}, "refuelSystems": []string{}, "calculateStartingFuel": true, "usedCapacityTons": 0, "tankTons": 1000, "cargoTritiumTons": 0, "reserveTritiumTons": 0, "departureTime": nil, "jumpMinutes": 15, "cooldownMinutes": 5, "overheadMinutes": 1, "maxQueueMinutes": 0, "miningTonsPerHour": 150, "refuelMinutes": 0, "refuelMethod": "mine", "tritiumPrice": 0}
}
func namesList(raw any) ([]string, error) {
	a := v.Arr(raw)
	if s, ok := raw.(string); ok {
		a = nil
		for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == '\r' || r == '\n' || r == ';' }) {
			a = append(a, p)
		}
	}
	if len(a) > 100 {
		return nil, errors.New("Допускается до 100 систем.")
	}
	out := []string{}
	for _, raw := range a {
		s := strings.TrimSpace(v.Str(raw))
		if len(s) > 150 {
			return nil, errors.New("Слишком длинное название системы.")
		}
		if s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}
func NormalizeRouteOptions(input, snapshot M) (M, error) {
	options := DefaultRouteOptions()
	carrier := v.Obj(snapshot["carrier"])
	options["sourceSystem"] = v.Text(v.At(carrier, "location", "system"), v.At(snapshot, "commander", "system"))
	if n, ok := v.Number(v.At(carrier, "space", "capacityTons")); ok {
		if free, fok := v.Number(v.At(carrier, "space", "freeTons")); fok {
			options["usedCapacityTons"] = n - free
		}
	}
	for _, r := range [][3]string{{"tankTons", "fuel", "tankTons"}, {"cargoTritiumTons", "fuel", "reserveTons"}, {"reserveTritiumTons", "planning", "reserveTritium"}} {
		if x := numeric(v.At(carrier, r[1], r[2])); x != nil {
			options[r[0]] = x
			if r[1] == "fuel" {
				options["calculateStartingFuel"] = false
			}
		}
	}
	options["usedCapacityTons"] = math.Max(v.Num(options["usedCapacityTons"]), v.Num(options["cargoTritiumTons"]))
	for _, k := range []string{"sourceSystem", "destinationSystem"} {
		if input[k] != nil {
			options[k] = strings.TrimSpace(v.Str(input[k]))
		}
		if len(v.Str(options[k])) > 150 {
			return nil, errors.New("Слишком длинное название системы.")
		}
	}
	for _, k := range []string{"waypoints", "refuelSystems"} {
		list, e := namesList(input[k])
		if e != nil {
			return nil, e
		}
		options[k] = list
	}
	if b, ok := input["calculateStartingFuel"].(bool); ok {
		options["calculateStartingFuel"] = b
	}
	for _, r := range []struct {
		k        string
		min, max float64
		integer  bool
	}{{"usedCapacityTons", 0, 25000, true}, {"tankTons", 0, 1000, true}, {"cargoTritiumTons", 0, 25000, true}, {"reserveTritiumTons", 0, 26000, true}, {"jumpMinutes", 1, 240, false}, {"cooldownMinutes", 0, 120, false}, {"overheadMinutes", 0, 240, false}, {"maxQueueMinutes", 0, 240, false}, {"miningTonsPerHour", 1, 10000, false}, {"refuelMinutes", 0, 10080, false}, {"tritiumPrice", 0, 1e8, false}} {
		if input[r.k] != nil && input[r.k] != "" {
			n, ok := v.Number(input[r.k])
			if !ok || n < r.min || n > r.max || r.integer && math.Trunc(n) != n {
				return nil, fmt.Errorf("%s: введите число от %g до %g", r.k, r.min, r.max)
			}
			options[r.k] = n
		}
	}
	if input["departureTime"] != nil && input["departureTime"] != "" {
		d := v.Time(input["departureTime"])
		if d.IsZero() {
			return nil, errors.New("Некорректное время отправления.")
		}
		options["departureTime"] = v.ISO(d)
	}
	if input["refuelMethod"] != nil {
		switch input["refuelMethod"] {
		case "mine", "buy", "manual":
			options["refuelMethod"] = input["refuelMethod"]
		default:
			return nil, errors.New("Неизвестный способ пополнения трития.")
		}
	}
	if v.Str(options["sourceSystem"]) == "" || v.Str(options["destinationSystem"]) == "" {
		return nil, errors.New("Укажите исходную и конечную систему флотоносца.")
	}
	if v.Num(options["usedCapacityTons"]) < v.Num(options["cargoTritiumTons"]) {
		return nil, errors.New("Складской тритий входит в занятую вместимость.")
	}
	destinations := append(v.Arr(options["waypoints"]), options["destinationSystem"])
	previous := v.Str(options["sourceSystem"])
	for _, raw := range destinations {
		name := v.Str(raw)
		if strings.EqualFold(previous, name) {
			return nil, errors.New("Соседние точки маршрута должны отличаться.")
		}
		previous = name
	}
	for _, refuel := range v.Arr(options["refuelSystems"]) {
		found := false
		for _, destination := range destinations {
			found = found || strings.EqualFold(v.Str(refuel), v.Str(destination))
		}
		if !found {
			return nil, fmt.Errorf("Точка дозаправки «%s» должна быть промежуточной или конечной системой", v.Str(refuel))
		}
	}
	return options, nil
}
func BuildRoutePayload(options M, systems []M, overrides M) M {
	calculate := v.Bool(options["calculateStartingFuel"])
	if b, ok := overrides["calculateStartingFuel"].(bool); ok {
		calculate = b
	}
	destinations := []string{}
	refuels := []string{}
	for _, s := range systems[1:] {
		id := v.Str(s["id64"])
		destinations = append(destinations, id)
		for _, raw := range v.Arr(options["refuelSystems"]) {
			if strings.EqualFold(v.Str(raw), v.Str(s["name"])) {
				refuels = append(refuels, id)
				break
			}
		}
	}
	used := v.Num(options["usedCapacityTons"])
	if calculate {
		used -= v.Num(options["cargoTritiumTons"])
	}
	if x, ok := v.Number(overrides["usedCapacityTons"]); ok {
		used = x
	}
	payload := M{"source": v.Str(systems[0]["id64"]), "destinations": destinations, "mass": 25000, "capacity": 25000, "capacity_used": used, "calculate_starting_fuel": 0}
	if calculate {
		payload["calculate_starting_fuel"] = 1
		payload["refuel_destinations"] = refuels
	} else {
		payload["fuel_loaded"] = v.First(overrides["tankTons"], options["tankTons"])
		payload["tritium_stored"] = v.First(overrides["cargoTritiumTons"], options["cargoTritiumTons"])
	}
	return payload
}
func PayloadForm(payload M) url.Values {
	form := url.Values{}
	for key, raw := range payload {
		a := v.Arr(raw)
		if a != nil {
			for _, x := range a {
				form.Add(key, v.Str(x))
			}
		} else {
			form.Add(key, v.Str(raw))
		}
	}
	return form
}
func RouteAssumptions(options M) []string {
	return []string{"Расход каждого прыжка и начальная загрузка рассчитаны Spansh для личного флотоносца: масса 25 000 т, вместимость 25 000 т, бак 1 000 т, дальность 500 св. лет.", "Бак в таблице показан по прибытии. Перелив со склада запланирован перед следующим прыжком, когда трития не хватает или в баке меньше 100 т.", fmt.Sprintf("Подготовка %g мин, перезарядка %g мин между прыжками, прочее %g мин. Очередь и задержки могут быть больше.", v.Num(options["jumpMinutes"]), v.Num(options["cooldownMinutes"]), v.Num(options["overheadMinutes"])), "Разрешения, свободная орбита, наличие трития и удобство добычи маршрутом не гарантируются."}
}
func sumRows(rows []M, key string) float64 {
	total := float64(0)
	for _, r := range rows {
		total += v.Num(r[key])
	}
	return total
}
func NormalizeProviderRoute(response, options, metadata M) (M, error) {
	raw := v.Obj(response["result"])
	jumps := v.Rows(raw["jumps"])
	if response["status"] != "ok" || len(jumps) < 2 {
		return nil, errors.New("Spansh не вернул полный маршрут флотоносца.")
	}
	stops := []M{}
	for index, jump := range jumps {
		coords := trading.Coords([]any{jump["x"], jump["y"], jump["z"]})
		id := v.Str(jump["id64"])
		if len(coords) != 3 || v.Str(jump["name"]) == "" || id == "" {
			return nil, errors.New("В маршруте нет координат или идентификатора системы.")
		}
		if _, e := strconv.ParseUint(id, 10, 64); e != nil {
			return nil, errors.New("Некорректный ID64 в маршруте.")
		}
		for _, field := range []string{"fuel_used", "fuel_in_tank", "tritium_in_market", "restock_amount"} {
			n, ok := v.Number(jump[field])
			if !ok || n < 0 {
				return nil, errors.New("Spansh вернул неполные данные о тритии.")
			}
		}
		if v.Num(jump["fuel_in_tank"]) > 1000 || v.Num(jump["tritium_in_market"]) > 25000 {
			return nil, errors.New("Spansh вернул невозможный запас трития.")
		}
		stop := M{"index": index, "name": jump["name"], "id64": id, "coordinates": coords, "distanceLy": v.Num(jump["distance"]), "remainingLy": numeric(jump["distance_to_destination"]), "fuelUsedTons": v.Num(jump["fuel_used"]), "tankTons": v.Num(jump["fuel_in_tank"]), "cargoTritiumTons": v.Num(jump["tritium_in_market"]), "restockTons": v.Num(jump["restock_amount"]), "mustRestock": v.Bool(jump["must_restock"]), "hasIcyRing": v.Bool(jump["has_icy_ring"]), "isPristine": v.Bool(jump["is_system_pristine"]), "isDestination": v.Bool(jump["is_desired_destination"]), "visited": index == 0, "visitedAt": nil, "confirmation": nil}
		if index == 0 {
			stop["confirmation"] = "source"
		}
		stops = append(stops, stop)
	}
	for index := 1; index < len(stops); index++ {
		previous, current := stops[index-1], stops[index]
		distance := trading.Distance(previous["coordinates"], current["coordinates"])
		if math.IsNaN(distance) || distance > 500.01 || math.Abs(distance-v.Num(current["distanceLy"])) > .05 {
			return nil, errors.New("Длина прыжка не соответствует координатам или превышает 500 св. лет.")
		}
		expected := v.Num(previous["tankTons"]) + v.Num(previous["cargoTritiumTons"]) - v.Num(current["fuelUsedTons"]) + v.Num(current["restockTons"])
		if math.Abs(expected-v.Num(current["tankTons"])-v.Num(current["cargoTritiumTons"])) > .01 {
			return nil, errors.New("Баланс трития между прыжками не сходится.")
		}
		if v.Num(previous["tankTons"])-v.Num(current["fuelUsedTons"]) < -.01 {
			return nil, errors.New("Прыжок не обеспечен топливом в баке.")
		}
	}
	totalJumps := len(stops) - 1
	first, last := stops[0], stops[len(stops)-1]
	startTank, startCargo, ok := plannedStartingFuel(raw, first, options)
	if !ok {
		return nil, errors.New("Не удалось определить начальный запас трития.")
	}
	startRestock := v.Num(first["restockTons"])
	if v.Bool(options["calculateStartingFuel"]) {
		startRestock = 0
	}
	if !projectRouteFuel(stops, 0, startTank, startCargo, routeTritiumCapacity(v.First(raw["capacity_used"], options["usedCapacityTons"]), startCargo), startRestock) {
		return nil, errors.New("Прыжок не обеспечен топливом в баке.")
	}
	restock := sumRows(stops[1:], "restockTons")
	restockStops := 0
	missingIce := false
	for _, stop := range stops[1:] {
		if v.Bool(stop["mustRestock"]) || v.Num(stop["restockTons"]) > 0 {
			restockStops++
			if !v.Bool(stop["hasIcyRing"]) {
				missingIce = true
			}
		}
	}
	miningMinutes := float64(0)
	if options["refuelMethod"] == "mine" {
		miningMinutes = restock / v.Num(options["miningTonsPerHour"]) * 60
	}
	refuelMinutes := float64(restockStops) * v.Num(options["refuelMinutes"])
	travelMinutes := float64(totalJumps)*(v.Num(options["jumpMinutes"])+v.Num(options["overheadMinutes"])) + math.Max(0, float64(totalJumps-1))*v.Num(options["cooldownMinutes"])
	totalMinutes := travelMinutes + miningMinutes + refuelMinutes
	maxMinutes := totalMinutes + float64(totalJumps)*v.Num(options["maxQueueMinutes"])
	departure := v.Time(v.First(options["departureTime"], metadata["createdAt"]))
	if departure.IsZero() {
		departure = time.Now()
	}
	elapsed, maximum := float64(0), float64(0)
	first["estimatedArrival"] = v.ISO(departure)
	first["estimatedArrivalLatest"] = first["estimatedArrival"]
	at := func(minutes float64) string {
		return v.ISO(departure.Add(time.Duration(minutes * float64(time.Minute))))
	}
	for index, stop := range stops {
		stopMinutes := float64(0)
		if index > 0 {
			if options["refuelMethod"] == "mine" {
				stopMinutes += v.Num(stop["restockTons"]) / v.Num(options["miningTonsPerHour"]) * 60
			}
			if v.Bool(stop["mustRestock"]) || v.Num(stop["restockTons"]) > 0 {
				stopMinutes += v.Num(options["refuelMinutes"])
			}
		}
		stop["refuelMinutes"] = stopMinutes
		stop["elapsedMinutes"] = elapsed
		stop["estimatedReady"] = at(elapsed + stopMinutes)
		if index < len(stops)-1 {
			cooldown := float64(0)
			if index > 0 {
				cooldown = v.Num(options["cooldownMinutes"])
			}
			stop["estimatedDeparture"] = at(elapsed + stopMinutes + cooldown + v.Num(options["jumpMinutes"]))
			elapsed += stopMinutes + cooldown + v.Num(options["jumpMinutes"]) + v.Num(options["overheadMinutes"])
			maximum += stopMinutes + cooldown + v.Num(options["jumpMinutes"]) + v.Num(options["overheadMinutes"]) + v.Num(options["maxQueueMinutes"])
			stops[index+1]["estimatedArrival"] = at(elapsed)
			stops[index+1]["estimatedArrivalLatest"] = at(maximum)
		} else {
			stop["estimatedDeparture"] = nil
		}
	}
	starting := startTank + startCargo
	arrival, _ := routeFuelTotal(last)
	initial, acquire := v.Num(first["restockTons"]), sumRows(stops, "restockTons")
	if v.Bool(options["calculateStartingFuel"]) {
		initial = starting
		acquire = starting + restock
	}
	fuel := sumRows(stops, "fuelUsedTons")
	var fuelCost, acquireCost any
	if v.Num(options["tritiumPrice"]) > 0 {
		fuelCost = fuel * v.Num(options["tritiumPrice"])
		acquireCost = acquire * v.Num(options["tritiumPrice"])
	}
	shortfall := math.Max(0, v.Num(options["reserveTritiumTons"])-arrival)
	summary := M{"totalJumps": totalJumps, "completedJumps": 0, "remainingJumps": totalJumps, "distanceLy": sumRows(stops, "distanceLy"), "directDistanceLy": trading.Distance(first["coordinates"], last["coordinates"]), "fuelTons": fuel, "initialLoadTons": initial, "restockTons": restock, "acquireTritiumTons": acquire, "additionalTritiumNeededTons": math.Max(0, starting-v.Num(options["tankTons"])-v.Num(options["cargoTritiumTons"])) + restock, "restockStops": restockStops, "reserveTritiumTons": options["reserveTritiumTons"], "miningMinutes": miningMinutes, "refuelMinutes": refuelMinutes, "travelMinutes": travelMinutes, "totalMinutes": totalMinutes, "timeRange": M{"minMinutes": totalMinutes, "maxMinutes": maxMinutes, "bounded": false}, "departureTime": v.ISO(departure), "estimatedArrival": last["estimatedArrival"], "estimatedArrivalLatest": last["estimatedArrivalLatest"], "estimatedComplete": at(totalMinutes), "readyAgain": v.ISO(v.Time(last["estimatedArrival"]).Add(time.Duration(v.Num(options["cooldownMinutes"]) * float64(time.Minute)))), "fuelCost": fuelCost, "acquireCost": acquireCost, "minimumStartingTritiumTons": metadata["minimumStartingTritiumTons"]}
	updateRouteFuelSummary(summary, stops)
	warnings := []string{}
	if shortfall > 0 {
		warnings = append(warnings, fmt.Sprintf("По прибытии не хватает %g т до выбранного резерва.", shortfall))
	}
	if restock > 0 {
		warnings = append(warnings, fmt.Sprintf("По дороге нужно получить %g т трития. Наличие продавца или места добычи не подтверждено.", restock))
	}
	if options["refuelMethod"] == "mine" && missingIce {
		warnings = append(warnings, "Часть дозаправок не имеет известных ледяных колец.")
	}
	if v.Bool(options["calculateStartingFuel"]) && arrival == 0 {
		warnings = append(warnings, "Минимальная загрузка оставляет нулевой тритий по прибытии. Задайте резерв.")
	}
	if options["refuelMethod"] != "mine" && restockStops > 0 && v.Num(options["refuelMinutes"]) == 0 {
		warnings = append(warnings, "Время приобретения и доставки трития не задано.")
	}
	job := v.Str(response["job"])
	return M{"route": M{"stops": stops, "fuelModel": "arrival", "activeIndex": 1, "createdAt": v.First(metadata["createdAt"], v.ISO(time.Now())), "options": v.Copy(options), "sourceSystem": first["name"], "destinationSystem": last["name"], "jobId": job, "sourceUrl": "https://spansh.co.uk/fleet-carrier/results/" + url.PathEscape(job), "providerParameters": response["parameters"], "providerResult": raw, "mass": M{"baseTons": 25000, "capacityTons": 25000, "usedCapacityTons": raw["capacity_used"], "tankCapacityTons": 1000, "startingTotalMassTons": 25000 + v.Num(raw["capacity_used"]) - v.Num(raw["tritium_stored"]) + starting + startRestock}}, "summary": summary, "warnings": warnings, "assumptions": RouteAssumptions(options)}, nil
}
func RouteCSV(state M) (string, error) {
	stops := v.Rows(v.At(state, "route", "stops"))
	if len(stops) == 0 {
		return "", errors.New("Сначала рассчитайте маршрут.")
	}
	var b strings.Builder
	b.WriteString("\ufeff")
	w := csv.NewWriter(&b)
	w.Comma = ';'
	_ = w.Write([]string{"№", "Система", "ID64", "X", "Y", "Z", "Прыжок, св. лет", "Осталось, св. лет", "Расход, т", "Бак по прибытии, т", "Склад по прибытии, т", "Перелить со склада, т", "Получить извне, т", "Источник остатка в баке", "Ледяные кольца", "Расчётное прибытие UTC", "Посещено"})
	safe := func(raw any) string {
		s := v.Str(raw)
		if strings.HasPrefix(s, "=") || strings.HasPrefix(s, "+") || strings.HasPrefix(s, "@") || strings.HasPrefix(s, "\t") || strings.HasPrefix(s, "\r") {
			s = "'" + s
		}
		return s
	}
	for _, stop := range stops {
		coords := trading.Coords(stop["coordinates"])
		var x, y, z string
		if len(coords) == 3 {
			x, y, z = fmt.Sprint(coords[0]), fmt.Sprint(coords[1]), fmt.Sprint(coords[2])
		}
		yes := func(raw any) string {
			if v.Bool(raw) {
				return "Да"
			}
			return "Нет"
		}
		var basis string
		if stop["tankTons"] != nil {
			switch {
			case v.Bool(stop["tankObserved"]):
				basis = "Замер"
			case v.Bool(stop["tankEstimated"]):
				basis = "Оценка"
			default:
				basis = "План"
			}
		}
		_ = w.Write([]string{safe(stop["index"]), safe(stop["name"]), safe(stop["id64"]), x, y, z, safe(stop["distanceLy"]), safe(stop["remainingLy"]), safe(stop["fuelUsedTons"]), safe(stop["tankTons"]), safe(stop["cargoTritiumTons"]), safe(stop["transferTons"]), safe(stop["restockTons"]), basis, yes(stop["hasIcyRing"]), safe(stop["estimatedArrival"]), yes(stop["visited"])})
	}
	w.Flush()
	return b.String(), w.Error()
}

type RoutePlanner struct {
	mu                    sync.Mutex
	state                 M
	Client                trading.HTTPDoer
	BaseURL               string
	PollInterval, Timeout time.Duration
	OnUpdate              func(M)
	cancel                context.CancelFunc
	Now                   func() time.Time
}

func NewRoutePlanner(saved M) *RoutePlanner {
	state := EmptyRouteState()
	if saved["state"] != nil {
		saved = v.Obj(saved["state"])
	}
	hasRoute := len(v.Rows(v.At(saved, "route", "stops"))) > 0
	if hasRoute || len(v.Obj(saved["options"])) > 0 {
		state = v.Merge(state, v.Copy(saved))
		state["options"] = v.Merge(DefaultRouteOptions(), v.Obj(state["options"]))
		state["running"] = false
		state["status"] = "idle"
		progress := v.Copy(v.Obj(state["progress"]))
		progress["stage"], progress["label"] = "idle", ""
		if hasRoute {
			state["status"] = "ready"
			progress["stage"], progress["label"] = "ready", "Сохранённый маршрут"
			restoreRouteFuel(state)
		}
		state["progress"] = progress
		state["error"] = nil
	}
	return &RoutePlanner{state: state, Client: &http.Client{Timeout: 45 * time.Second}, BaseURL: "https://spansh.co.uk/api", PollInterval: 1500 * time.Millisecond, Timeout: 10 * time.Minute, Now: time.Now}
}
func (p *RoutePlanner) Snapshot(_ ...M) M { p.mu.Lock(); defer p.mu.Unlock(); return v.Copy(p.state) }
func (p *RoutePlanner) SnapshotForCarrier(carrier M) M {
	state := p.Snapshot()
	applyLiveRouteFuel(state, carrier)
	return state
}
func (p *RoutePlanner) ExportSettings() M { return M{"version": 1, "state": p.Snapshot()} }
func (p *RoutePlanner) publish(update M) {
	p.mu.Lock()
	v.Merge(p.state, update)
	snapshot := v.Copy(p.state)
	p.mu.Unlock()
	if p.OnUpdate != nil {
		p.OnUpdate(snapshot)
	}
}
func (p *RoutePlanner) request(ctx context.Context, method, path string, payload M) (M, error) {
	var body io.Reader
	if payload != nil {
		body = strings.NewReader(PayloadForm(payload).Encode())
	}
	req, e := http.NewRequestWithContext(ctx, method, p.BaseURL+path, body)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ED-Copilot/2.0 FleetCarrierRoute")
	if payload != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	response, e := p.Client.Do(req)
	if e != nil {
		return nil, e
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Spansh: HTTP %d", response.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(response.Body, 16000001))
	if e != nil {
		return nil, e
	}
	if len(b) > 16000000 {
		return nil, errors.New("Ответ Spansh слишком большой.")
	}
	var out M
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	if e = decoder.Decode(&out); e != nil {
		return nil, errors.New("Spansh вернул неизвестный формат.")
	}
	return out, nil
}
func (p *RoutePlanner) Resolve(ctx context.Context, name string) (M, error) {
	response, e := p.request(ctx, "GET", "/search/systems?q="+url.QueryEscape(name), nil)
	if e != nil {
		return nil, e
	}
	for _, s := range v.Rows(response["results"]) {
		if strings.EqualFold(v.Str(s["name"]), name) && len(trading.Coords(s)) == 3 {
			if _, e := strconv.ParseUint(v.Str(s["id64"]), 10, 64); e == nil {
				return M{"name": s["name"], "id64": v.Str(s["id64"]), "coordinates": trading.Coords(s)}, nil
			}
		}
	}
	return nil, fmt.Errorf("Система «%s» не найдена в Spansh", name)
}
func (p *RoutePlanner) job(ctx context.Context, payload M) (M, error) {
	response, e := p.request(ctx, "POST", "/fleetcarrier/route", payload)
	if e != nil {
		return nil, e
	}
	if response["status"] == "ok" {
		return response, nil
	}
	if response["status"] != "queued" || v.Str(response["job"]) == "" {
		return nil, fmt.Errorf("Spansh: %s", v.Text(response["error"], "не удалось построить маршрут"))
	}
	job := v.Str(response["job"])
	p.publish(M{"jobId": job, "status": "queued", "sourceUrl": "https://spansh.co.uk/fleet-carrier/results/" + url.PathEscape(job)})
	deadline := time.NewTimer(p.Timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(p.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, errors.New("Spansh не завершил расчёт за отведённое время.")
		case <-ticker.C:
			result, e := p.request(ctx, "GET", "/results/"+url.PathEscape(job), nil)
			if e != nil {
				return nil, e
			}
			switch result["status"] {
			case "ok":
				return result, nil
			case "queued", "running", "processing":
				p.publish(M{"status": result["status"]})
			default:
				return nil, fmt.Errorf("Spansh: %s", v.Text(result["error"], result["message"], "маршрут не найден"))
			}
		}
	}
}
func (p *RoutePlanner) Search(ctx context.Context, input, snapshot M) (M, error) {
	options, e := NormalizeRouteOptions(input, snapshot)
	if e != nil {
		return nil, e
	}
	p.mu.Lock()
	if p.cancel != nil {
		p.mu.Unlock()
		return nil, errors.New("Расчёт маршрута уже идёт.")
	}
	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.mu.Unlock()
	defer func() {
		cancel()
		p.mu.Lock()
		p.cancel = nil
		p.state["running"] = false
		p.state["updatedAt"] = v.ISO(p.Now())
		p.mu.Unlock()
	}()
	p.publish(M{"running": true, "options": options, "status": "resolving", "error": nil})
	result, e := p.run(ctx, options, snapshot)
	if e != nil {
		status := "error"
		var message any = e.Error()
		if ctx.Err() != nil {
			status = "cancelled"
			message = nil
		}
		p.publish(M{"status": status, "error": message})
		return p.Snapshot(), e
	}
	p.publish(v.Merge(result, M{"status": "ready", "running": false, "error": nil, "progress": M{"stage": "ready", "label": "Маршрут рассчитан Spansh", "completedJumps": 0, "totalJumps": v.At(result, "summary", "totalJumps"), "nextIndex": 1}}))
	return p.Snapshot(), nil
}
func (p *RoutePlanner) run(ctx context.Context, options, snapshot M) (M, error) {
	systems := []M{}
	cache := map[string]M{}
	names := append([]any{options["sourceSystem"]}, v.Arr(options["waypoints"])...)
	names = append(names, options["destinationSystem"])
	for _, raw := range names {
		name := v.Str(raw)
		key := strings.ToLower(name)
		system := cache[key]
		if system == nil {
			var e error
			system, e = p.Resolve(ctx, name)
			if e != nil {
				return nil, e
			}
			cache[key] = system
		}
		systems = append(systems, system)
	}
	created := v.ISO(p.Now())
	response, e := p.job(ctx, BuildRoutePayload(options, systems, M{}))
	if e != nil {
		return nil, e
	}
	result, e := NormalizeProviderRoute(response, options, M{"createdAt": created})
	if e != nil {
		return nil, e
	}
	minimum := v.Num(v.At(result, "summary", "startingTritiumTons"))
	if v.Bool(options["calculateStartingFuel"]) && v.Num(options["reserveTritiumTons"]) > 0 && len(v.Arr(options["refuelSystems"])) == 0 {
		loaded := minimum + v.Num(options["reserveTritiumTons"])
		baseUsed := v.Num(options["usedCapacityTons"]) - v.Num(options["cargoTritiumTons"])
		for attempt := 0; attempt < 8; attempt++ {
			tank := math.Min(1000, loaded)
			hold := math.Max(0, loaded-tank)
			if baseUsed+hold > 25000 {
				return nil, errors.New("Для маршрута и резерва недостаточно вместимости. Добавьте дозаправку.")
			}
			response, e = p.job(ctx, BuildRoutePayload(options, systems, M{"calculateStartingFuel": false, "usedCapacityTons": baseUsed + hold, "tankTons": tank, "cargoTritiumTons": hold}))
			if e != nil {
				return nil, e
			}
			result, e = NormalizeProviderRoute(response, options, M{"createdAt": created, "minimumStartingTritiumTons": minimum})
			if e != nil {
				return nil, e
			}
			deficit := v.Num(v.At(result, "summary", "reserveShortfallTons")) + v.Num(v.At(result, "summary", "restockTons"))
			if deficit == 0 {
				break
			}
			loaded += math.Ceil(deficit)
			if attempt == 7 {
				return nil, errors.New("Spansh не смог подтвердить резерв без дозаправки.")
			}
		}
	}
	stops := v.Rows(v.At(result, "route", "stops"))
	if systems[0]["id64"] != stops[0]["id64"] || systems[len(systems)-1]["id64"] != stops[len(stops)-1]["id64"] {
		return nil, errors.New("Начало или конец ответа не совпадает с запросом.")
	}
	previous := 0
	for _, system := range systems[1:] {
		found := -1
		for i := previous + 1; i < len(stops); i++ {
			if stops[i]["id64"] == system["id64"] {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, errors.New("Ответ не сохранил порядок промежуточных систем.")
		}
		previous = found
	}
	warnings := v.Arr(result["warnings"])
	if v.Bool(v.At(snapshot, "carrier", "fuel", "needsRefresh")) {
		warnings = append(warnings, "Топливо могло измениться после прыжка. Обновите снимок в игре.")
	}
	if v.Bool(v.At(snapshot, "carrier", "owned")) && v.At(snapshot, "carrier", "fuel", "reserveTons") == nil {
		warnings = append(warnings, "Полный состав склада неизвестен. Подтвердите тритий вручную.")
	}
	result["warnings"] = warnings
	return result, nil
}
func (p *RoutePlanner) Cancel() {
	p.mu.Lock()
	cancel := p.cancel
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
func (p *RoutePlanner) MarkVisited(index int, confirmation, timestamp string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.markVisited(index, confirmation, timestamp)
}
func (p *RoutePlanner) markVisited(index int, confirmation, timestamp string) error {
	route := v.Obj(p.state["route"])
	stops := v.Rows(route["stops"])
	if index < 0 || index >= len(stops) {
		return errors.New("Неизвестная точка маршрута.")
	}
	if v.Time(timestamp).IsZero() {
		return errors.New("Некорректное время посещения.")
	}
	completed := 0
	var next any
	for i, stop := range stops {
		if i <= index {
			stop["visited"] = true
			if stop["visitedAt"] == nil {
				stop["visitedAt"] = timestamp
			}
			if stop["confirmation"] == nil {
				stop["confirmation"] = confirmation
				if i != index {
					stop["confirmation"] = "progress"
				}
			}
		}
		if i > 0 && v.Bool(stop["visited"]) {
			completed++
		}
		if !v.Bool(stop["visited"]) && next == nil {
			next = i
		}
	}
	route["activeIndex"] = next
	summary := v.Obj(p.state["summary"])
	summary["completedJumps"] = completed
	summary["remainingJumps"] = v.Int(summary["totalJumps"]) - completed
	p.state["progress"] = M{"stage": "ready", "completedJumps": completed, "totalJumps": summary["totalJumps"], "nextIndex": next}
	return nil
}
func (p *RoutePlanner) SelectStop(index int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	route := v.Obj(p.state["route"])
	if index < 0 || index >= len(v.Rows(route["stops"])) {
		return errors.New("Неизвестная точка маршрута.")
	}
	route["activeIndex"] = index
	return nil
}
func (p *RoutePlanner) ObserveCarrier(carrier M) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if v.Bool(p.state["running"]) || !v.Bool(carrier["owned"]) {
		return false
	}
	route := v.Obj(p.state["route"])
	location := v.Obj(carrier["location"])
	timestamp := v.Str(location["updatedAt"])
	if timestamp == "" || v.Time(timestamp).Before(v.Time(route["createdAt"])) {
		return false
	}
	for i, stop := range v.Rows(route["stops"]) {
		if v.Bool(stop["visited"]) {
			continue
		}
		matches := strings.EqualFold(v.Str(location["system"]), v.Str(stop["name"]))
		if location["systemAddress"] != nil {
			matches = v.Str(location["systemAddress"]) == v.Str(stop["id64"])
		}
		if !matches {
			return false
		}
		return p.markVisited(i, "carrier-journal", timestamp) == nil
	}
	return false
}
func (p *RoutePlanner) ExportCSV() (string, error) { return RouteCSV(p.Snapshot()) }
