package shiproute

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type journeys struct {
	Active string   `json:"active"`
	Routes []*Route `json:"routes"`
}

type Store struct{ owners map[string]*journeys }

func New(saved any) *Store {
	s := &Store{owners: map[string]*journeys{}}
	raw, err := json.Marshal(saved)
	if err != nil || len(raw) > 10<<20 {
		return s
	}
	var old map[string]*journeys
	if json.Unmarshal(raw, &old) != nil {
		return s
	}
	savedStops := 0
	for owner, entry := range old {
		if !validText(owner, 128) || entry == nil || len(s.owners) >= maxOwners {
			continue
		}
		restored := &journeys{}
		for _, route := range entry.Routes {
			if route == nil || len(restored.Routes) >= maxRoutes || !validRoute(*route) || !validText(route.ID, 80) || restored.find(route.ID) != nil {
				continue
			}
			if savedStops+len(route.Stops) > maxSavedStops {
				continue
			}
			savedStops += len(route.Stops)
			restored.Routes = append(restored.Routes, route)
		}
		if len(restored.Routes) == 0 {
			continue
		}
		restored.Active = restored.Routes[0].ID
		if restored.find(entry.Active) != nil {
			restored.Active = entry.Active
		}
		s.owners[owner] = restored
	}
	return s
}

func (s *Store) Export() any { return v.Clone(s.owners) }

func (entry *journeys) find(id string) *Route {
	if entry != nil && id != "" {
		for _, r := range entry.Routes {
			if r.ID == id {
				return r
			}
		}
	}
	return nil
}

func (s *Store) active(owner string) *Route {
	if entry := s.owners[owner]; entry != nil {
		return entry.find(entry.Active)
	}
	return nil
}

func validRoute(r Route) bool {
	if !validText(r.Name, 160) || len(r.Stops) == 0 || len(r.Stops) > maxStops || r.Next < 0 || r.Next > len(r.Stops) ||
		!finiteRange(r.JumpRangeLY, 0, 500) || !finiteRange(r.MaxFuelPerJump, 0, 100) ||
		(r.Mode != "neutron" && r.Mode != "exact" && r.Mode != "manual" && r.Mode != "game") {
		return false
	}
	for _, stop := range r.Stops {
		if !validText(stop.System, 160) || stop.Station != "" && !validText(stop.Station, 160) || len([]rune(stop.Action)) > 1000 ||
			stop.DistanceLY != nil && !finiteRange(*stop.DistanceLY, 0, 1e6) || stop.Jumps < 0 || stop.Jumps > maxStops {
			return false
		}
	}
	return true
}

func (s *Store) Add(owner string, route Route, contexts ...M) error {
	if !validText(owner, 128) {
		return errors.New("Сначала подключите журнал командира.")
	}
	if !validRoute(route) {
		return errors.New("Маршрут корабля содержит неверные путевые точки.")
	}
	used := len(route.Stops)
	for _, saved := range s.owners {
		for _, current := range saved.Routes {
			used += len(current.Stops)
		}
	}
	if used > maxSavedStops {
		return errors.New("Удалите старый маршрут перед сохранением нового.")
	}
	entry := s.owners[owner]
	if entry == nil {
		if len(s.owners) >= maxOwners {
			return errors.New("Достигнут предел профилей маршрутов.")
		}
		entry = &journeys{}
		s.owners[owner] = entry
	}
	if len(entry.Routes) >= maxRoutes {
		return errors.New("Удалите старый маршрут перед сохранением нового.")
	}
	route.ID = v.ID()
	route.CreatedAt = v.ISO(time.Now())
	route.ObservedFrom = route.CreatedAt
	route.LastArrivalAt = ""
	route.Next = 0
	if len(contexts) > 0 {
		cmd := v.Obj(contexts[0]["commander"])
		for i, stop := range route.Stops {
			if i > 0 && route.Mode != "game" {
				break
			}
			if stop.Station == "" && matches(stop, v.Str(cmd["system"]), v.Str(cmd["systemAddress"])) {
				route.Next = i + 1
			}
		}
	}
	entry.Routes = append([]*Route{&route}, entry.Routes...)
	entry.Active = route.ID
	return nil
}

func (s *Store) Update(owner string, input, context M) error {
	switch v.Str(input["action"]) {
	case "import", "game":
		r, err := importRoute(input, context)
		if err != nil {
			return err
		}
		return s.Add(owner, r, context)
	}
	entry := s.owners[owner]
	r := s.active(owner)
	if r == nil {
		return errors.New("Сначала добавьте маршрут корабля.")
	}
	switch v.Str(input["action"]) {
	case "activate":
		id := v.Str(input["id"])
		next := entry.find(id)
		if next == nil {
			return errors.New("Путешествие не найдено.")
		}
		if entry.Active != id {
			next.ObservedFrom = v.ISO(time.Now())
		}
		entry.Active = id
	case "remove":
		id := v.Text(input["id"], r.ID)
		for i, saved := range entry.Routes {
			if saved.ID != id {
				continue
			}
			entry.Routes = append(entry.Routes[:i], entry.Routes[i+1:]...)
			if len(entry.Routes) == 0 {
				delete(s.owners, owner)
			} else if entry.Active == id {
				entry.Active = entry.Routes[0].ID
				entry.Routes[0].ObservedFrom = v.ISO(time.Now())
			}
			return nil
		}
		return errors.New("Путешествие не найдено.")
	case "move":
		if r.Mode == "exact" || r.Mode == "neutron" {
			return errors.New("Для перестановки импортируйте список путевых точек.")
		}
		index, indexOK := v.Number(input["index"])
		shift, shiftOK := v.Number(input["delta"])
		if !indexOK || index != math.Trunc(index) || !shiftOK || shift != 1 && shift != -1 || index < float64(r.Next) || index >= float64(len(r.Stops)) ||
			index+shift < float64(r.Next) || index+shift >= float64(len(r.Stops)) {
			return errors.New("Можно переставлять только непройденные точки.")
		}
		a, b := int(index), int(index+shift)
		r.Stops[a], r.Stops[b] = r.Stops[b], r.Stops[a]
		for i := 1; i < len(r.Stops); i++ {
			r.Stops[i].DistanceLY = distance(r.Stops[i-1].Coordinates, r.Stops[i].Coordinates)
			if r.Mode == "game" && i >= r.Next {
				r.Stops[i].Jumps = 0
			}
		}
		if r.Mode == "game" {
			r.Mode = "manual"
		}
		r.Origin, r.Destination = r.Stops[0].System, r.Stops[len(r.Stops)-1].System
		r.ObservedFrom = v.ISO(time.Now())
	case "autocopy":
		enabled, ok := input["enabled"].(bool)
		if !ok {
			return errors.New("Проверьте настройку буфера обмена.")
		}
		r.AutoCopy = enabled
	default:
		return errors.New("Неизвестное действие с маршрутом корабля.")
	}
	return nil
}

func importRoute(input, context M) (Route, error) {
	mode := "manual"
	var waypoints []M
	if input["action"] == "game" {
		mode = "game"
		waypoints = v.Rows(v.At(context, "pilot", "navRoute"))
	} else if raw, ok := input["waypoints"].(string); ok {
		if len(raw) > 320000 {
			return Route{}, errors.New("Список систем слишком длинный.")
		}
		for _, line := range strings.Split(raw, "\n") {
			if name := strings.TrimSpace(line); name != "" {
				waypoints = append(waypoints, M{"system": name})
			}
		}
	} else {
		waypoints = v.Rows(input["waypoints"])
	}
	if len(waypoints) == 0 || len(waypoints) > maxStops {
		return Route{}, errors.New("Добавьте от одной до 2 000 путевых точек.")
	}
	stops := make([]Stop, 0, len(waypoints)+1)
	cmd := v.Obj(context["commander"])
	for _, waypoint := range waypoints {
		stop := Stop{
			System:        strings.TrimSpace(v.Text(waypoint["system"], waypoint["name"], waypoint["StarSystem"])),
			SystemAddress: v.Text(waypoint["systemAddress"], waypoint["SystemAddress"]),
			StarClass:     v.Text(waypoint["starClass"], waypoint["StarClass"]), Coordinates: coords(v.First(waypoint["coordinates"], waypoint["StarPos"])),
			Station: strings.TrimSpace(v.Str(waypoint["station"])), Action: strings.TrimSpace(v.Str(waypoint["action"])),
		}
		if len(stops) > 0 {
			previous := stops[len(stops)-1]
			if matches(previous, stop.System, stop.SystemAddress) && previous.Station == stop.Station && previous.Action == stop.Action {
				continue
			}
			stop.DistanceLY = distance(previous.Coordinates, stop.Coordinates)
			if mode == "game" {
				stop.Jumps = 1
			}
		}
		stops = append(stops, stop)
	}
	if current := v.Str(cmd["system"]); validText(current, 160) && len(stops) < maxStops && (!matches(stops[0], current, v.Str(cmd["systemAddress"])) || stops[0].Station != "") && mode == "manual" {
		start := Stop{System: current, SystemAddress: v.Str(cmd["systemAddress"]), Coordinates: coords(cmd["coordinates"])}
		stops[0].DistanceLY = distance(start.Coordinates, stops[0].Coordinates)
		stops = append([]Stop{start}, stops...)
	}
	first, last := stops[0].System, stops[len(stops)-1].System
	name := v.Text(input["name"], first+" → "+last)
	return Route{Name: name, Mode: mode, Origin: first, Destination: last, Stops: stops, AutoCopy: v.Bool(input["autoCopy"]),
		JumpRangeLY: v.Num(v.First(input["jumpRangeLy"], cmd["jumpRangeLy"])), MaxFuelPerJump: v.Num(input["maxFuelPerJump"])}, nil
}

// Observe advances only an exact pending destination recorded in the journal.
// A replay can catch up arrivals that happened while the app was closed.
func (s *Store) Observe(owner string, event, meta M) string {
	if v.Bool(meta["side"]) {
		return ""
	}
	r := s.active(owner)
	if r == nil || r.Next >= len(r.Stops) {
		return ""
	}
	at := v.Time(event["timestamp"])
	replay := v.Bool(meta["replay"])
	threshold := v.Time(v.Text(r.ObservedFrom, r.CreatedAt)).Truncate(time.Second)
	if at.IsZero() || at.Before(threshold) || replay && !at.After(threshold) || !at.After(v.Time(r.LastArrivalAt)) {
		return ""
	}
	kind := v.Str(event["event"])
	if v.Bool(event["Taxi"]) || v.Bool(event["Multicrew"]) {
		return ""
	}
	stop := r.Stops[r.Next]
	if !matches(stop, v.Str(event["StarSystem"]), v.Str(event["SystemAddress"])) {
		return ""
	}
	if stop.Station == "" && kind != "FSDJump" || stop.Station != "" && (kind != "Docked" || !strings.EqualFold(stop.Station, v.Str(event["StationName"]))) {
		return ""
	}
	r.Next++
	r.LastArrivalAt = v.Str(event["timestamp"])
	if !replay && r.AutoCopy && r.Next < len(r.Stops) {
		return r.Stops[r.Next].System
	}
	return ""
}

func (s *Store) Snapshot(owner string, context M) M {
	out := M{"saved": []M{}, "route": nil, "search": context["shipRouteSearch"], "ownerKnown": owner != "",
		"gameRouteKnown": len(v.Rows(v.At(context, "pilot", "navRoute"))) > 0, "currentSystem": v.At(context, "commander", "system"),
		"suggestedJumpRangeLy": v.At(context, "commander", "jumpRangeLy"), "cargoTons": v.At(context, "commander", "cargoTons"),
		"fuel": v.Copy(v.Obj(v.At(context, "pilot", "fuel")))}
	entry := s.owners[owner]
	if entry == nil {
		return out
	}
	saved := make([]M, 0, len(entry.Routes))
	for _, r := range entry.Routes {
		saved = append(saved, M{"id": r.ID, "name": r.Name, "mode": r.Mode, "stops": len(r.Stops), "active": r.ID == entry.Active})
	}
	out["saved"] = saved
	if r := entry.find(entry.Active); r != nil {
		out["route"] = r.snapshot(context)
	}
	return out
}

func (r Route) snapshot(context M) M {
	out := v.Obj(v.Clone(r))
	rows := v.Rows(out["stops"])
	fuel, fuelKnown := v.Number(v.At(context, "pilot", "fuel", "mainTons"))
	capacity, capacityKnown := v.Number(v.At(context, "pilot", "fuel", "mainCapacityTons"))
	budget := 0
	budgetKnown := fuelKnown && r.MaxFuelPerJump > 0
	if budgetKnown {
		budget = int(math.Floor(fuel / r.MaxFuelPerJump))
		out["fuelSafeJumps"] = budget
	}
	var remainingDistance float64
	knownRemaining := true
	segmentJumps := 0
	for i, stop := range r.Stops {
		row := rows[i]
		row["visited"] = i < r.Next
		row["starKind"] = StarKind(stop.StarClass)
		if i < r.Next {
			continue
		}
		jumps := stop.Jumps
		if jumps == 0 && stop.DistanceLY != nil && r.JumpRangeLY > 0 {
			jumps = int(math.Ceil(*stop.DistanceLY / r.JumpRangeLY))
			row["estimatedJumps"] = jumps
		}
		if i == 0 && stop.DistanceLY == nil {
			jumps = 0
		}
		if stop.DistanceLY != nil {
			remainingDistance += *stop.DistanceLY
		} else if i > 0 {
			knownRemaining = false
		}
		if r.Mode == "exact" {
			if i == r.Next && fuelKnown && stop.FuelUsed != nil && fuel+0.001 < *stop.FuelUsed {
				row["fuelCaution"] = true
			}
		} else if budgetKnown && jumps > 0 {
			segmentJumps += jumps
			if segmentJumps > budget {
				row["fuelCaution"] = true
			}
		}
		departureBoost := i > 0 && r.Stops[i-1].Boost != ""
		if r.JumpRangeLY > 0 && stop.DistanceLY != nil && *stop.DistanceLY > r.JumpRangeLY && jumps <= 1 && !departureBoost {
			row["rangeCaution"] = true
		}
		if row["starKind"] == "scoopable" || stop.Refuel {
			segmentJumps = 0
			budgetKnown = capacityKnown && r.MaxFuelPerJump > 0
			if budgetKnown {
				budget = int(math.Floor(capacity / r.MaxFuelPerJump))
			}
		}
	}
	nextScoop := 0.0
	scoopKnown := false
	for i := len(rows) - 1; i >= r.Next; i-- {
		if i+1 < len(rows) && r.Stops[i+1].DistanceLY != nil && scoopKnown {
			nextScoop += *r.Stops[i+1].DistanceLY
			rows[i]["distanceToScoopLy"] = nextScoop
		} else {
			scoopKnown = false
		}
		if rows[i]["starKind"] == "scoopable" {
			nextScoop, scoopKnown = 0, true
		}
	}
	out["stops"] = rows
	if knownRemaining {
		out["remainingDistanceLy"] = remainingDistance
	}
	if r.Next < len(rows) {
		out["nextStop"] = rows[r.Next]
	}
	return out
}
