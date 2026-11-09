package trading

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}
type Searcher struct {
	Client   HTTPDoer
	BaseURL  string
	EDSMURL  string
	Now      func() time.Time
	OnUpdate func(M)
}

func NewSearcher(client HTTPDoer) *Searcher {
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	return &Searcher{Client: client, BaseURL: "https://spansh.co.uk/api", EDSMURL: "https://www.edsm.net/api-v1/system", Now: time.Now}
}
func NormalizeOptions(input M) M {
	out := M{"mode": "sell", "system": "", "resourceKey": "", "useCargo": true, "useCarrierCargo": false, "radiusLy": 300, "maxAgeHours": 24, "carrierMaxAgeHours": 6, "minDemand": 1, "minSupply": 1, "includeCarriers": true, "includePlanetary": true, "includeOwn": false, "minPad": "L", "maxArrivalLs": 10000, "quantity": 1, "demandRatio": 4}
	if input["mode"] == "buy" {
		out["mode"] = "buy"
	}
	out["system"] = strings.TrimSpace(v.Str(input["system"]))
	out["resourceKey"] = v.Key(input["resourceKey"])
	for _, k := range []string{"includeCarriers", "includePlanetary", "includeOwn", "useCargo", "useCarrierCargo"} {
		if b, ok := input[k].(bool); ok {
			out[k] = b
		}
	}
	switch input["marketType"] {
	case "carriers", "stations", "all":
		out["marketType"] = input["marketType"]
		out["includeCarriers"] = input["marketType"] != "stations"
	}
	for _, r := range []struct {
		k        string
		min, max float64
	}{{"radiusLy", 5, 5000}, {"maxAgeHours", 1, 720}, {"carrierMaxAgeHours", .25, 168}, {"minDemand", 1, 1e9}, {"minSupply", 1, 1e9}, {"maxArrivalLs", 0, 1e7}, {"quantity", 1, 1e6}, {"demandRatio", 1, 20}} {
		if x, ok := v.Number(input[r.k]); ok {
			out[r.k] = v.Clamp(x, r.min, r.max)
		}
	}
	out["quantity"] = math.Floor(v.Num(out["quantity"]))
	switch input["minPad"] {
	case "L", "M", "S", "any":
		out["minPad"] = input["minPad"]
	}
	return out
}
func BuildBasket(state, options M) ([]M, []M, error) {
	cargo := v.Rows(v.At(state, "commander", "cargo"))
	if v.Bool(options["useCarrierCargo"]) {
		cargo = v.Rows(v.At(state, "carrier", "warehouse", "items"))
	}
	key := v.Str(options["resourceKey"])
	if options["mode"] == "buy" || key != "" || !v.Bool(options["useCargo"]) {
		if key == "" {
			key = "tritium"
		}
		g := Good(key, nil)
		if g["kind"] == "raw" {
			return nil, nil, errors.New("Сырьевые материалы не продаются на товарном рынке.")
		}
		quantity := v.Num(options["quantity"])
		for _, r := range cargo {
			if v.Key(r["key"]) == key && options["mode"] == "sell" && v.Bool(options["useCargo"]) && v.Num(r["count"]) > 0 {
				quantity = v.Num(r["count"])
			}
		}
		return []M{{"key": key, "name": g["name"], "english": g["english"], "marketName": g["english"], "quantity": quantity}}, nil, nil
	}
	items := []M{}
	for _, r := range cargo {
		key := v.Key(r["key"])
		g := Good(key, r["name"])
		if key == "" || key == "drones" || v.Num(r["count"]) <= 0 || g["kind"] == "raw" {
			continue
		}
		items = append(items, M{"key": g["key"], "name": g["name"], "english": g["english"], "marketName": g["english"], "quantity": math.Floor(v.Num(r["count"]))})
	}
	sort.Slice(items, func(i, j int) bool { return n(items[i], "quantity") > n(items[j], "quantity") })
	if len(items) == 0 {
		return nil, nil, errors.New("Нет товарного груза для продажи. Выберите ресурс вручную.")
	}
	if len(items) > 6 {
		return items[:6], items[6:], nil
	}
	return items, nil, nil
}
func Coords(raw any) []float64 {
	a := v.Arr(raw)
	if len(a) == 0 {
		m := v.Obj(raw)
		a = []any{m["x"], m["y"], m["z"]}
	}
	if len(a) != 3 {
		return nil
	}
	r := make([]float64, 3)
	for i, x := range a {
		n, ok := v.Number(x)
		if !ok {
			return nil
		}
		r[i] = n
	}
	return r
}
func Distance(a, b any) float64 {
	x, y := Coords(a), Coords(b)
	if len(x) != 3 || len(y) != 3 {
		return math.NaN()
	}
	return math.Sqrt(math.Pow(x[0]-y[0], 2) + math.Pow(x[1]-y[1], 2) + math.Pow(x[2]-y[2], 2))
}
func stationPad(s M) string {
	for _, p := range []struct{ k, c string }{{"large", "L"}, {"medium", "M"}, {"small", "S"}} {
		if v.Bool(s["has_"+p.k+"_pad"]) || v.Num(s[p.k+"_pads"]) > 0 {
			return p.c
		}
	}
	return ""
}
func Quote(raw, requested M, mode, timestamp string, now time.Time) M {
	normalized := v.Key(v.First(raw["symbol"], raw["commodity"], raw["name"]))
	resource := Resource(normalized)
	if normalized != v.Str(requested["key"]) && resource["key"] != requested["key"] && normalized != v.Key(requested["english"]) {
		return nil
	}
	field, availableField := "sell_price", "demand"
	if mode == "buy" {
		field = "buy_price"
		availableField = "supply"
	}
	price, ok := v.Number(raw[field])
	available, aok := v.Number(raw[availableField])
	if !ok || !aok || price <= 0 || available <= 0 {
		return nil
	}
	quantity := math.Min(n(requested, "quantity"), math.Floor(available))
	return M{"key": requested["key"], "name": requested["name"], "price": price, "demand": raw["demand"], "supply": raw["supply"], "requestedQuantity": requested["quantity"], "quantity": quantity, "subtotal": quantity * price, "updatedAt": timestamp, "ageHours": math.Max(0, now.Sub(v.Time(timestamp)).Hours())}
}
func StationResult(station M, basket []M, options M, now time.Time, reference any) M {
	timestamp := v.Str(station["market_updated_at"])
	updated := v.Time(timestamp)
	if updated.IsZero() || updated.After(now.Add(10*time.Minute)) {
		return nil
	}
	stationType := strings.ToLower(v.Str(station["type"]))
	carrier := strings.Contains(stationType, "carrier")
	planetary := v.Bool(station["is_planetary"]) || strings.Contains(stationType, "planet") || strings.Contains(stationType, "settlement")
	if carrier && !v.Bool(options["includeCarriers"]) || !carrier && options["marketType"] == "carriers" ||
		planetary && options["includePlanetary"] == false {
		return nil
	}
	age := math.Max(0, now.Sub(updated).Hours())
	maxAge := n(options, "maxAgeHours")
	if carrier {
		maxAge = n(options, "carrierMaxAgeHours")
	}
	if age > maxAge {
		return nil
	}
	coords := Coords([]any{station["system_x"], station["system_y"], station["system_z"]})
	distance, ok := v.Number(station["distance"])
	if len(coords) == 3 && len(Coords(reference)) == 3 {
		distance = Distance(coords, reference)
		ok = true
	}
	if !ok || distance > n(options, "radiusLy") {
		return nil
	}
	arrival, aok := v.Number(station["distance_to_arrival"])
	if n(options, "maxArrivalLs") > 0 && aok && arrival > n(options, "maxArrivalLs") {
		return nil
	}
	pad := stationPad(station)
	rank := map[string]int{"": 0, "S": 1, "M": 2, "L": 3, "any": 0}
	if rank[pad] < rank[v.Str(options["minPad"])] {
		return nil
	}
	quotes := []M{}
	bulk := false
	covered, total, requested := float64(0), float64(0), float64(0)
	mode := v.Str(options["mode"])
	for _, r := range basket {
		requested += n(r, "quantity")
		for _, raw := range v.Rows(station["market"]) {
			q := Quote(raw, r, mode, timestamp, now)
			if q == nil {
				continue
			}
			if mode == "sell" && n(q, "demand") < n(options, "minDemand") || mode == "buy" && n(q, "supply") < n(options, "minSupply") {
				break
			}
			risk := mode == "sell" && !carrier && n(q, "demand") < n(q, "requestedQuantity")*n(options, "demandRatio")
			q["bulkTaxRisk"] = risk
			bulk = bulk || risk
			covered += n(q, "quantity")
			total += n(q, "subtotal")
			quotes = append(quotes, q)
			break
		}
	}
	if len(quotes) == 0 {
		return nil
	}
	marketID := v.Text(station["market_id"], station["id"])
	warnings := []string{}
	if bulk {
		warnings = append(warnings, "Спрос ниже выбранного запаса к объёму груза. Цена при продаже может снизиться.")
	}
	if carrier {
		warnings = append(warnings, "Флотоносец мог переместиться или изменить заказ после обновления рынка.")
	}
	if covered < requested {
		warnings = append(warnings, "Известный спрос или запас покрывает только часть груза.")
	}
	if pad == "" {
		warnings = append(warnings, "Размер площадки не подтверждён.")
	}
	if !aok {
		warnings = append(warnings, "Расстояние от звезды неизвестно.")
	}
	id := "market:" + marketID
	if marketID == "" {
		id = "station:" + v.Str(station["system_name"]) + ":" + v.Str(station["name"])
	}
	return M{"id": id, "marketId": marketID, "station": v.Str(station["name"]), "system": v.Str(station["system_name"]), "systemAddress": station["system_id64"], "type": station["type"], "carrier": carrier, "pad": pad, "planetary": planetary, "coordinates": coords, "distanceLy": distance, "distanceLs": v.Nullable(arrival, aok), "updatedAt": timestamp, "sourceUpdatedAt": timestamp, "ageHours": age, "quotes": quotes, "estimatedTotal": total, "requestedTons": requested, "coveredTons": covered, "missingTons": math.Max(0, requested-covered), "coverageComplete": covered >= requested, "demandSafe": !bulk, "bulkTaxRisk": bulk, "averagePrice": total / covered, "warnings": warnings, "source": "Spansh", "sourceUrl": "https://spansh.co.uk/station/" + url.PathEscape(marketID)}
}
func RankResults(rows []M, mode string) []M {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if v.Bool(a["coverageComplete"]) != v.Bool(b["coverageComplete"]) {
			return v.Bool(a["coverageComplete"])
		}
		if mode == "sell" && v.Bool(a["demandSafe"]) != v.Bool(b["demandSafe"]) {
			return v.Bool(a["demandSafe"])
		}
		if mode == "sell" && n(a, "estimatedTotal") != n(b, "estimatedTotal") {
			return n(a, "estimatedTotal") > n(b, "estimatedTotal")
		}
		if mode == "buy" && n(a, "averagePrice") != n(b, "averagePrice") {
			return n(a, "averagePrice") < n(b, "averagePrice")
		}
		if n(a, "coveredTons") != n(b, "coveredTons") {
			return n(a, "coveredTons") > n(b, "coveredTons")
		}
		if n(a, "distanceLy") != n(b, "distanceLy") {
			return n(a, "distanceLy") < n(b, "distanceLy")
		}
		return n(a, "ageHours") < n(b, "ageHours")
	})
	return rows
}

// QuoteState projects the age of a saved market observation without rewriting
// its original timestamps or treating a new application snapshot as fresh data.
func QuoteState(row, options M, now time.Time) M {
	out := v.Copy(row)
	options = NormalizeOptions(options)
	maximum := v.Num(options["maxAgeHours"])
	if v.Bool(row["carrier"]) {
		maximum = v.Num(options["carrierMaxAgeHours"])
	}
	updated := v.Time(row["updatedAt"])
	var age any
	if !updated.IsZero() {
		age = math.Max(0, now.Sub(updated).Hours())
	}
	out["ageHours"] = age
	out["expired"] = age == nil || v.Num(age) > maximum || updated.After(now.Add(10*time.Minute))
	quotes := []M{}
	for _, quote := range v.Rows(row["quotes"]) {
		q := v.Copy(quote)
		qt := v.Time(quote["updatedAt"])
		q["ageHours"] = age
		if !qt.IsZero() {
			q["ageHours"] = math.Max(0, now.Sub(qt).Hours())
		}
		quotes = append(quotes, q)
	}
	out["quotes"] = quotes
	return out
}
func SearchPayload(resource, options M, reference []float64, page int, now time.Time) M {
	availability, minimum, sortField, direction := "demand", n(options, "minDemand"), "market_sell_price", "desc"
	if options["mode"] == "buy" {
		availability = "supply"
		minimum = n(options, "minSupply")
		sortField = "market_buy_price"
		direction = "asc"
	}
	since := now.Add(-time.Duration(math.Max(n(options, "maxAgeHours"), n(options, "carrierMaxAgeHours")) * float64(time.Hour)))
	filters := M{"has_market": M{"value": true}, "market": []M{{"name": v.First(resource["marketName"], resource["english"]), availability: M{"value": []string{fmt.Sprint(minimum), "999999999999"}, "comparison": "<=>"}}}, "market_updated_at": M{"value": []string{v.ISO(since), v.ISO(now.Add(10 * time.Minute))}, "comparison": "<=>"}, "distance": M{"min": 0, "max": options["radiusLy"]}}
	// Apply the scope before selecting the most competitive pages. Otherwise
	// excluded carrier or surface quotes can crowd out qualifying stations.
	const carrierType = "Drake-Class Carrier"
	if options["marketType"] == "carriers" {
		filters["type"] = M{"value": []string{carrierType}}
	} else {
		excluded := []string{}
		if options["includeCarriers"] == false {
			excluded = append(excluded, carrierType)
		}
		if options["includePlanetary"] == false {
			excluded = append(excluded, "Dockable Planet Station", "Planetary Construction Depot", "Planetary Outpost",
				"Planetary Port", "Settlement", "Surface Settlement")
		}
		if len(excluded) > 0 {
			filters["type"] = M{"value": excluded, "logic": "not"}
		}
	}
	if options["minPad"] == "L" {
		filters["has_large_pad"] = M{"value": true}
	}
	if n(options, "maxArrivalLs") > 0 {
		filters["distance_to_arrival"] = M{"value": []any{0, options["maxArrivalLs"]}, "comparison": "<=>"}
	}
	return M{"filters": filters, "sort": []M{{sortField: []M{{"name": v.First(resource["marketName"], resource["english"]), "direction": direction}}}, {"distance": M{"direction": "asc"}}}, "size": 60, "page": page, "reference_coords": M{"x": reference[0], "y": reference[1], "z": reference[2]}}
}
func (s *Searcher) request(ctx context.Context, method, endpoint string, payload any) (M, error) {
	result, err := FetchJSON(ctx, s.Client, method, endpoint, payload)
	if err != nil {
		return nil, err
	}
	if m, ok := result.(M); ok {
		return m, nil
	}
	return nil, errors.New("Неизвестный формат ответа сервиса.")
}
func (s *Searcher) Reference(ctx context.Context, state, options M) (M, error) {
	cmd := v.Obj(state["commander"])
	system := v.Str(options["system"])
	if system == "" {
		system = v.Str(cmd["system"])
		if v.Bool(options["useCarrierCargo"]) {
			system = v.Str(v.At(state, "carrier", "location", "system"))
		}
	}
	if system == "" {
		return nil, errors.New("Укажите исходную систему.")
	}
	if strings.EqualFold(system, v.Str(cmd["system"])) && len(Coords(cmd["coordinates"])) == 3 {
		return M{"system": system, "coordinates": cmd["coordinates"], "systemAddress": cmd["systemAddress"]}, nil
	}
	for _, known := range v.Rows(state["world"]) {
		if strings.EqualFold(system, v.Str(known["name"])) && len(Coords(known["coordinates"])) == 3 {
			return M{"system": known["name"], "coordinates": known["coordinates"], "systemAddress": known["address"]}, nil
		}
	}
	result, e := s.request(ctx, "GET", s.EDSMURL+"?"+url.Values{"systemName": {system}, "showCoordinates": {"1"}, "showId": {"1"}}.Encode(), nil)
	if e != nil {
		return nil, e
	}
	coords := Coords(result["coords"])
	if len(coords) != 3 {
		return nil, errors.New("Координаты системы не найдены.")
	}
	return M{"system": v.Text(result["name"], system), "coordinates": coords, "systemAddress": result["id64"]}, nil
}
func (s *Searcher) Search(ctx context.Context, input, state M) (M, error) {
	options := NormalizeOptions(input)
	basket, skipped, e := BuildBasket(state, options)
	if e != nil {
		return nil, e
	}
	reference, e := s.Reference(ctx, state, options)
	if e != nil {
		return nil, e
	}
	out := M{"running": false, "options": options, "basket": basket, "skippedGoods": skipped, "results": []M{}, "coverageLimited": len(skipped) > 0, "reference": reference, "source": "Spansh", "updatedAt": v.ISO(s.Now())}
	results := map[string]M{}
	errorsFound := []string{}
	seen := 0
	for _, resource := range basket {
		scanned, total := 0, 0
		for page := 0; page < 2; page++ {
			if e = ctx.Err(); e != nil {
				return out, e
			}
			payload := SearchPayload(resource, options, Coords(reference["coordinates"]), page, s.Now())
			response, err := s.request(ctx, "POST", s.BaseURL+"/stations/search", payload)
			if err != nil {
				errorsFound = append(errorsFound, v.Str(resource["name"])+": "+err.Error())
				break
			}
			stations := v.Rows(response["results"])
			total = v.Int(response["count"])
			if total == 0 {
				total = len(stations)
			}
			scanned += len(stations)
			seen += len(stations)
			for _, raw := range stations {
				own := v.Text(raw["market_id"], raw["id"]) == v.Str(v.At(state, "carrier", "id"))
				if own && !v.Bool(options["includeOwn"]) {
					continue
				}
				r := StationResult(raw, basket, options, s.Now(), reference["coordinates"])
				if r == nil {
					continue
				}
				r["own"] = own
				previous := results[v.Str(r["id"])]
				if previous == nil || v.Str(r["updatedAt"]) >= v.Str(previous["updatedAt"]) {
					results[v.Str(r["id"])] = r
				}
			}
			rows := []M{}
			for _, r := range results {
				rows = append(rows, r)
			}
			out["results"] = RankResults(rows, v.Str(options["mode"]))
			out["progress"] = M{"checked": 0, "total": len(basket), "stationsSeen": seen, "resultsCount": len(rows), "label": "Сравнение рынков"}
			if s.OnUpdate != nil {
				s.OnUpdate(v.Copy(out))
			}
			if len(stations) < 60 || scanned >= total {
				break
			}
		}
		if scanned < total {
			out["coverageLimited"] = true
		}
	}
	rows := v.Rows(out["results"])
	if len(rows) > 120 {
		out["results"] = rows[:120]
		out["coverageLimited"] = true
	}
	out["status"] = "Рынки сравнены по последним котировкам."
	if len(rows) == 0 {
		out["status"] = "Подходящих рынков не найдено."
	}
	if len(errorsFound) > 0 {
		out["error"] = strings.Join(errorsFound, " · ")
	}
	return out, nil
}
func (s *Service) Update(body M) error {
	switch v.Str(body["action"]) {
	case "plan", "set-plan":
		g := s.good(body["key"], nil)
		quantity, qok := v.Number(body["quantity"])
		buy, bok := v.Number(body["buyPrice"])
		sell, sok := v.Number(body["sellPrice"])
		if g == nil || !qok || !bok || !sok || quantity <= 0 || quantity > 25000 || buy < 0 || sell < 0 || buy > 1e9 || sell > 1e9 {
			return errors.New("Проверьте товар, тоннаж и цены в плане.")
		}
		for _, side := range []string{"buy", "sell"} {
			if body[side] != nil {
				market := v.Obj(body[side])
				if v.Str(market["system"]) == "" || v.Str(market["station"]) == "" {
					return errors.New("Проверьте систему и станцию выбранного плеча")
				}
			}
		}
		s.Plan = M{"key": g["key"], "name": g["name"], "quantity": quantity, "buyPrice": buy, "sellPrice": sell, "purchaseCost": quantity * buy, "revenue": quantity * sell, "grossMargin": quantity * (sell - buy), "source": v.Str(body["source"]), "destination": v.Str(body["destination"]), "updatedAt": v.ISO(time.Now())}
		for _, side := range []string{"buy", "sell"} {
			if body[side] != nil {
				s.Plan[side] = v.Copy(v.Obj(body[side]))
			}
		}
	case "destination", "set-destination":
		if body["destination"] == nil {
			s.Destination = M{}
			return nil
		}
		d := v.Copy(v.Obj(body["destination"]))
		if len(v.Rows(d["quotes"])) == 0 || v.Str(d["station"]) == "" || v.Str(d["system"]) == "" {
			return errors.New("Выберите рыночное предложение.")
		}
		if len(v.Str(d["station"])) > 150 || len(v.Str(d["system"])) > 150 {
			return errors.New("Название системы или станции слишком длинное.")
		}
		d["selectedAt"] = v.ISO(time.Now())
		d["selectionMode"] = v.Text(d["selectionMode"], body["mode"], "sell")
		if d["selectionMode"] != "buy" {
			d["selectionMode"] = "sell"
		}
		options := NormalizeOptions(v.Obj(d["quoteOptions"]))
		options["mode"] = d["selectionMode"]
		d["quoteOptions"] = options
		vessel := v.Text(v.At(body, "commander", "vehicle"), s.ActiveVessel)
		d["cargoManifest"] = s.cargoManifest(vessel)
		s.Destination = d
	case "clear-plan":
		s.Plan = M{}
	default:
		return errors.New("Неизвестное действие торговли.")
	}
	return nil
}

// FetchJSON is the bounded community-data transport shared by market and mining searches.
// It preserves JSON integer identifiers and retries only transient overload responses.
func FetchJSON(ctx context.Context, client HTTPDoer, method, endpoint string, payload any) (any, error) {
	var encoded []byte
	var err error
	if payload != nil {
		encoded, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		req, err := http.NewRequestWithContext(requestCtx, method, endpoint, bytes.NewReader(encoded))
		if err != nil {
			cancel()
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "ED-Copilot/2.0")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		response, err := client.Do(req)
		if err != nil {
			cancel()
			return nil, err
		}
		if attempt == 0 && (response.StatusCode == 429 || response.StatusCode == 503) {
			response.Body.Close()
			cancel()
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			response.Body.Close()
			cancel()
			return nil, fmt.Errorf("Сервис данных: HTTP %d", response.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 16000001))
		response.Body.Close()
		cancel()
		if err != nil {
			return nil, err
		}
		if len(body) > 16000000 {
			return nil, errors.New("Ответ сервиса слишком большой.")
		}
		var out any
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		if decoder.Decode(&out) != nil {
			return nil, errors.New("Сервис вернул неизвестный формат.")
		}
		return out, nil
	}
	return nil, errors.New("Сервис временно недоступен.")
}
