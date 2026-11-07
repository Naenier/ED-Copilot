package mining

import (
	"context"
	"errors"
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Searcher struct {
	Client           trading.HTTPDoer
	BaseURL, EDSMURL string
	Now              func() time.Time
	OnSystem         func(M)
	OnUpdate         func(M)
}

func NewSearcher(client trading.HTTPDoer) *Searcher {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Searcher{Client: client, BaseURL: "https://spansh.co.uk/api", EDSMURL: "https://www.edsm.net", Now: time.Now}
}
func signalsRows(raw any) []M {
	if nested := v.Obj(raw)["signals"]; nested != nil {
		raw = nested
	}
	out := []M{}
	if rows := v.Rows(raw); rows != nil {
		for _, r := range rows {
			typ := v.First(r["Type"], r["name"], r["type"], r["key"])
			count := v.Num(v.First(r["Count"], r["count"]))
			if v.Str(typ) != "" && count > 0 {
				out = append(out, M{"Type": typ, "Count": math.Floor(count)})
			}
		}
	} else {
		for typ, count := range v.Obj(raw) {
			if v.Num(count) > 0 {
				out = append(out, M{"Type": typ, "Count": math.Floor(v.Num(count))})
			}
		}
	}
	if len(out) > 256 {
		out = out[:256]
	}
	return out
}
func materialRows(raw any) []M {
	out := []M{}
	if rows := v.Rows(raw); rows != nil {
		for _, r := range rows {
			name := v.First(r["Name"], r["name"], r["key"])
			percent, ok := v.Number(v.First(r["Percent"], r["share"], r["percent"]))
			if key := v.Key(name); key != "" && ok && percent >= 0 && percent <= 100 {
				out = append(out, M{"key": key, "percent": percent})
			}
		}
	} else {
		for name, rawPercent := range v.Obj(raw) {
			percent, ok := v.Number(rawPercent)
			if key := v.Key(name); key != "" && ok && percent >= 0 && percent <= 100 {
				out = append(out, M{"key": key, "percent": percent})
			}
		}
	}
	if len(out) > 128 {
		out = out[:128]
	}
	return out
}
func SpanshSystemFromDump(payload M) (M, error) {
	system := v.Obj(payload["system"])
	if v.Str(system["name"]) == "" || system["bodies"] == nil {
		return nil, errors.New("Spansh вернул ответ без данных системы.")
	}
	bodies := []M{}
	for _, raw := range v.Rows(system["bodies"]) {
		if len(bodies) >= 4096 {
			break
		}
		if v.Str(raw["name"]) == "" {
			continue
		}
		rings := []M{}
		for _, ring := range v.Rows(raw["rings"]) {
			if len(rings) >= 64 {
				break
			}
			if v.Str(ring["name"]) == "" {
				continue
			}
			signals := []M{}
			for _, signal := range signalsRows(ring["signals"]) {
				g := trading.Good(signal["Type"], nil)
				signals = append(signals, M{"key": g["key"], "name": signal["Type"], "count": signal["Count"]})
			}
			rings = append(rings, M{"name": ring["name"], "type": v.First(ring["type"], ""), "signals": signals})
		}
		bodies = append(bodies, M{"id": v.First(raw["bodyId"], raw["name"]), "name": raw["name"], "source": "spansh", "timestamp": raw["updateTime"], "landable": v.Bool(raw["isLandable"]), "planetClass": v.First(raw["subType"], ""), "distanceLs": numericSearch(raw["distanceToArrival"]), "gravity": numericSearch(raw["gravity"]), "reserve": v.First(raw["reserveLevel"], ""), "materials": materialRows(raw["materials"]), "rings": rings, "signals": signalsRows(raw["signals"])})
	}
	return M{"address": system["id64"], "name": system["name"], "coordinates": trading.Coords(system["coords"]), "bodies": bodies}, nil
}
func numericSearch(raw any) any { n, ok := v.Number(raw); return v.Nullable(n, ok) }
func EDSMSystem(payload, reference M) M {
	bodies := []M{}
	for _, body := range v.Rows(payload["bodies"]) {
		rings := []M{}
		for _, ring := range v.Rows(body["rings"]) {
			rings = append(rings, M{"name": ring["name"], "type": v.First(ring["type"], ""), "signals": []M{}})
		}
		bodies = append(bodies, M{"id": v.First(body["bodyId"], body["id"]), "name": body["name"], "source": "edsm", "timestamp": v.First(body["updateTime"], nowISO()), "landable": v.Bool(body["isLandable"]), "distanceLs": numericSearch(body["distanceToArrival"]), "gravity": numericSearch(body["gravity"]), "reserve": v.First(body["reserveLevel"], ""), "materials": materialRows(body["materials"]), "rings": rings})
	}
	coords := trading.Coords(reference["coords"])
	if len(coords) != 3 {
		coords = trading.Coords(payload["coords"])
	}
	return M{"address": v.First(payload["id64"], reference["id64"]), "name": v.First(payload["name"], reference["name"]), "coordinates": coords, "bodies": bodies}
}
func SpanshSystems(payload M) []M {
	systems := map[string]M{}
	for _, body := range v.Rows(payload["results"]) {
		name := v.Str(body["system_name"])
		if name == "" || v.Str(body["name"]) == "" {
			continue
		}
		system := systems[name]
		if system == nil {
			system = M{"name": name, "address": body["system_id64"], "coordinates": trading.Coords([]any{body["system_x"], body["system_y"], body["system_z"]}), "bodies": []M{}}
			systems[name] = system
		}
		rings := []M{}
		for _, ring := range v.Rows(body["rings"]) {
			signals := []M{}
			for _, signal := range v.Rows(ring["signals"]) {
				signals = append(signals, M{"key": v.Key(signal["name"]), "name": signal["name"], "count": signal["count"]})
			}
			rings = append(rings, M{"name": ring["name"], "type": v.First(ring["type"], ""), "signals": signals})
		}
		system["bodies"] = append(v.Rows(system["bodies"]), M{"id": v.First(body["body_id"], body["id64"], body["name"]), "name": body["name"], "source": "spansh", "timestamp": v.First(body["updated_at"], nowISO()), "landable": v.Bool(body["is_landable"]), "distanceLs": numericSearch(body["distance_to_arrival"]), "gravity": numericSearch(body["gravity"]), "reserve": v.First(body["reserve_level"], ""), "materials": materialRows(body["materials"]), "rings": rings})
	}
	out := []M{}
	for _, s := range systems {
		out = append(out, s)
	}
	return out
}
func (s *Searcher) Search(ctx context.Context, input, state M) (M, error) {
	center := strings.TrimSpace(v.Text(input["system"], v.At(state, "commander", "system")))
	if center == "" || len(center) > 150 {
		return nil, errors.New("Укажите название системы.")
	}
	radius := v.Clamp(withDefault(v.First(input["radiusLy"], v.At(state, "settings", "radiusLy")), 50), 5, 100)
	_ = radius
	return s.search(ctx, input, state, center, radius)
}
func (s *Searcher) search(ctx context.Context, input, state M, center string, radius float64) (M, error) {
	out := M{"running": false, "system": center, "radiusLy": radius, "checked": 0, "total": 0, "status": "Определение координат системы…", "error": nil, "finishedAt": nil, "systems": []M{}, "coverageLimited": true}
	systems := map[string]M{}
	errs := []string{}
	publish := func(label string) {
		out["status"] = label
		if s.OnUpdate != nil {
			s.OnUpdate(v.Copy(out))
		}
	}
	merge := func(system M) {
		name := v.Str(system["name"])
		systems[name] = system
		for _, body := range v.Rows(system["bodies"]) {
			if signals := v.Rows(body["signals"]); len(signals) > 0 {
				body["signals"] = signals
			}
		}
		if s.OnSystem != nil {
			s.OnSystem(system)
		}
	}
	var reference M
	for _, known := range v.Rows(state["world"]) {
		if strings.EqualFold(v.Str(known["name"]), center) && len(trading.Coords(known["coordinates"])) == 3 {
			coords := trading.Coords(known["coordinates"])
			reference = M{"name": known["name"], "id64": known["address"], "coords": M{"x": coords[0], "y": coords[1], "z": coords[2]}}
			break
		}
	}
	if reference == nil {
		raw, e := trading.FetchJSON(ctx, s.Client, "GET", s.EDSMURL+"/api-v1/system?"+url.Values{"systemName": {center}, "showCoordinates": {"1"}, "showId": {"1"}}.Encode(), nil)
		if e != nil {
			return out, e
		}
		reference = v.Obj(raw)
	}
	from := trading.Coords(reference["coords"])
	if len(from) != 3 {
		return out, errors.New("Координаты системы не найдены.")
	}
	dumpLoaded := false
	dumpFailures := 0
	loadDump := func(entry M) bool {
		id := v.Str(entry["id64"])
		if id == "" {
			return false
		}
		raw, e := trading.FetchJSON(ctx, s.Client, "GET", s.BaseURL+"/dump/"+url.PathEscape(id), nil)
		if e == nil {
			system, parseErr := SpanshSystemFromDump(v.Obj(raw))
			if parseErr == nil {
				merge(system)
				return true
			}
			e = parseErr
		}
		errs = append(errs, v.Str(entry["name"])+": "+e.Error())
		return false
	}
	if reference["id64"] != nil {
		dumpLoaded = loadDump(reference)
		if !dumpLoaded {
			dumpFailures++
		}
	}
	selected := trading.Resource(input["resourceKey"])
	names := []string{"Platinum", "Painite", "Monazite", "Musgravite", "Low Temperature Diamonds"}
	if selected["kind"] == "commodity" && len(v.Arr(selected["ringTypes"])) > 0 {
		names = []string{v.Str(selected["english"])}
	}
	out["total"] = len(names)
	for _, name := range names {
		if e := ctx.Err(); e != nil {
			return out, e
		}
		publish("Поиск подтверждённых горячих точек: " + trading.Name(name, nil))
		raw, e := trading.FetchJSON(ctx, s.Client, "POST", s.BaseURL+"/bodies/search", M{"filters": M{"ring_signals": []M{{"name": name, "value": []int{1, 9999}, "comparison": "<=>"}}}, "sort": []M{{"distance": M{"direction": "asc"}}}, "size": 50, "page": 0, "reference_coords": M{"x": from[0], "y": from[1], "z": from[2]}})
		if e != nil {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			errs = append(errs, "Spansh: "+e.Error())
		} else {
			for _, entry := range SpanshSystems(v.Obj(raw)) {
				distance := trading.Distance(from, entry["coordinates"])
				if !math.IsNaN(distance) && distance <= radius {
					merge(entry)
				}
			}
		}
		increment(out, "checked", 1)
	}
	nearbyRaw, e := trading.FetchJSON(ctx, s.Client, "GET", s.EDSMURL+"/api-v1/sphere-systems?"+url.Values{"systemName": {center}, "radius": {fmt.Sprint(radius)}, "showCoordinates": {"1"}, "showId": {"1"}}.Encode(), nil)
	nearby := []M{}
	if e != nil {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		errs = append(errs, "EDSM: "+e.Error())
	} else {
		nearby = v.Rows(nearbyRaw)
	}
	entries := []M{reference}
	for _, entry := range nearby {
		if !strings.EqualFold(v.Str(entry["name"]), center) {
			entries = append(entries, entry)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return v.Num(entries[i]["distance"]) < v.Num(entries[j]["distance"]) })
	if len(entries) > 20 {
		entries = entries[:20]
	}
	increment(out, "total", float64(len(entries)))
	for _, entry := range entries {
		if e := ctx.Err(); e != nil {
			return out, e
		}
		publish("Проверка тел: " + v.Str(entry["name"]))
		complete := dumpLoaded && strings.EqualFold(v.Str(entry["name"]), center)
		if !complete && entry["id64"] != nil && dumpFailures < 2 {
			complete = loadDump(entry)
			if complete {
				dumpFailures = 0
			} else {
				dumpFailures++
			}
		}
		if !complete {
			raw, e := trading.FetchJSON(ctx, s.Client, "GET", s.EDSMURL+"/api-system-v1/bodies?"+url.Values{"systemName": {v.Str(entry["name"])}}.Encode(), nil)
			if e != nil {
				if ctx.Err() != nil {
					return out, ctx.Err()
				}
				errs = append(errs, v.Str(entry["name"])+": "+e.Error())
			} else {
				merge(EDSMSystem(v.Obj(raw), entry))
			}
		}
		increment(out, "checked", 1)
	}
	rows := []M{}
	for _, system := range systems {
		rows = append(rows, system)
	}
	out["systems"] = rows
	out["finishedAt"] = v.ISO(s.Now())
	out["status"] = "Поиск завершён. Проверены горячие точки и до 20 ближайших систем. Охват базы сообщества ограничен."
	if len(errs) > 0 {
		if len(errs) > 3 {
			errs = errs[:3]
		}
		out["error"] = strings.Join(errs, " · ")
	}
	return out, nil
}
