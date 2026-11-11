package app

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/riches"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const richesJob = "riches"

func (e *Engine) RichesStatus() M {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.richesSnapshotLocked(true)
}

type richesBodyObservation struct {
	Scanned bool `json:"scanned,omitempty"`
	Mapped  bool `json:"mapped,omitempty"`
	Manual  bool `json:"manual,omitempty"`
}

func (o richesBodyObservation) journalComplete(useMapping bool) bool {
	return o.Mapped || !useMapping && o.Scanned
}

// A journey and its observations belong to one commander. Engine.mu protects
// route replacement, journal updates and the clipboard request as one operation.
type richesJourney struct {
	Route          riches.Route                     `json:"route"`
	CreatedAt      time.Time                        `json:"createdAt"`
	LastJumpAt     time.Time                        `json:"lastJumpAt"`
	NextStop       int                              `json:"nextStop"`
	CompletedJumps int                              `json:"completedJumps"`
	LegJumps       int                              `json:"legJumps"`
	Observed       map[string]richesBodyObservation `json:"observed,omitempty"`
}

func restoreRichesJourneys(raw any) map[string]*richesJourney {
	journeys := make(map[string]*richesJourney)
	if raw == nil {
		return journeys
	}
	data, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(data, &journeys) != nil || journeys == nil {
		return make(map[string]*richesJourney)
	}
	for owner, journey := range journeys {
		if journey == nil || journey.CreatedAt.IsZero() || len(journey.Route.Stops) == 0 ||
			journey.NextStop < 0 || journey.NextStop > len(journey.Route.Stops) ||
			journey.CompletedJumps < 0 || journey.LegJumps < 0 {
			delete(journeys, owner)
		}
	}
	return journeys
}

func (e *Engine) startRichesRoute(body M) (M, error) {
	if err := e.networkAllowed(); err != nil {
		return nil, err
	}
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.RLock()
	owner, currentSystem, closed := e.planWorkspace.key, v.Str(e.commander["system"]), e.closed
	e.mu.RUnlock()
	if closed {
		return nil, context.Canceled
	}
	if err := validatePlanWorkspace(body, owner); err != nil {
		return nil, err
	}
	opts, err := richesOptions(body, currentSystem)
	if err != nil {
		return nil, err
	}
	input := v.Obj(v.Clone(opts))
	return e.start(richesJob, input, func(ctx context.Context, id uint64) (M, error) {
		route, err := e.richesPlanner.Plan(ctx, opts)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			return nil, err
		}
		if len(route.Stops) == 0 {
			return M{"status": "В заданном радиусе подходящих миров не найдено", "empty": true, "options": input}, nil
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		if ctx.Err() != nil || e.closed || e.jobIDs[richesJob] != id || e.planWorkspace.key != owner {
			return nil, context.Canceled
		}
		journey := e.newRichesJourneyLocked(route)
		e.richesJourneys[owner] = journey
		e.queueRichesClipboardLocked(journey)
		e.revision++
		return M{"status": "Маршрут построен", "options": input}, nil
	}), nil
}

func richesOptions(body M, currentSystem string) (riches.Options, error) {
	maxDistance, minMapping := float64(riches.DefaultMaxDistanceLS), int64(riches.DefaultMinMappingValue)
	opts := riches.Options{
		Origin: strings.TrimSpace(v.Text(body["origin"], currentSystem)), Destination: strings.TrimSpace(v.Str(body["destination"])),
		JumpRangeLY: v.Num(body["jumpRangeLy"]), RadiusLY: v.Num(body["radiusLy"]),
		MaxResults: riches.DefaultMaxResults, MaxDistanceLS: &maxDistance, MinMappingValue: &minMapping,
		UseMappingValue: true, AvoidThargoids: true, Loop: true,
	}
	if raw, present := body["maxResults"]; present {
		n, ok := v.Number(raw)
		if !ok || n < 1 || n > riches.MaxStops || math.Trunc(n) != n {
			return opts, errors.New("Укажите целое максимальное число систем.")
		}
		opts.MaxResults = int(n)
	}
	if raw, present := body["maxDistanceLs"]; present {
		n, ok := v.Number(raw)
		if !ok {
			return opts, errors.New("Укажите расстояние от звезды в световых секундах.")
		}
		maxDistance = n
	}
	if raw, present := body["minMappingValue"]; present {
		n, ok := v.Number(raw)
		if !ok || n < 0 || n > riches.MaxMappingValue || math.Trunc(n) != n {
			return opts, errors.New("Укажите целую минимальную награду за DSS в кредитах.")
		}
		minMapping = int64(n)
	}
	for key, flag := range map[string]*bool{"useMappingValue": &opts.UseMappingValue, "avoidThargoids": &opts.AvoidThargoids, "loop": &opts.Loop} {
		if raw, present := body[key]; present {
			checked, ok := raw.(bool)
			if !ok {
				return opts, errors.New("Параметры маршрута должны быть включены или выключены.")
			}
			*flag = checked
		}
	}
	return opts, riches.Validate(opts)
}

func (e *Engine) newRichesJourneyLocked(route riches.Route) *richesJourney {
	journey := &richesJourney{Route: route, CreatedAt: time.Now().UTC()}
	if len(route.Stops) != 0 && matchesRichesSystem(route.Stops[0], v.Str(e.commander["system"]), v.Str(e.commander["systemAddress"])) {
		journey.NextStop = 1
	}
	return journey
}

func (e *Engine) resetRichesSearchLocked() {
	if cancel := e.cancelJobs[richesJob]; cancel != nil {
		cancel()
		delete(e.cancelJobs, richesJob)
	}
	e.jobIDs[richesJob]++
	delete(e.network, richesJob)
	e.clearClipboardLocked(richesJob)
}

func (e *Engine) adoptRichesJourneyLocked() {
	owner := e.planWorkspace.key
	if owner != "" && e.richesJourneys[owner] == nil && e.richesJourneys[""] != nil {
		e.richesJourneys[owner] = e.richesJourneys[""]
		delete(e.richesJourneys, "")
	}
}

func (e *Engine) clearRichesRoute() error {
	e.mu.Lock()
	e.resetRichesSearchLocked()
	delete(e.richesJourneys, e.planWorkspace.key)
	e.revision++
	e.mu.Unlock()
	e.Notify()
	return e.Save()
}

func (e *Engine) copyRichesDestination() error {
	e.mu.Lock()
	journey := e.richesJourneys[e.planWorkspace.key]
	if journey == nil || journey.NextStop == len(journey.Route.Stops) {
		e.mu.Unlock()
		return errors.New("На маршруте нет следующей системы")
	}
	e.queueRichesClipboardLocked(journey)
	e.mu.Unlock()
	e.Notify()
	return nil
}

func (e *Engine) completeRichesBody(input M) error {
	if err := e.setRichesBodyCompletion(input); err != nil {
		return err
	}
	e.Notify()
	return e.Save()
}

func (e *Engine) setRichesBodyCompletion(input M) error {
	system, name := v.Str(input["system"]), v.Str(input["body"])
	checked, valid := input["completed"].(bool)
	if system == "" || name == "" || !valid {
		return errors.New("Укажите планету и состояние её исследования.")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	owner := e.planWorkspace.key
	if err := validatePlanWorkspace(input, owner); err != nil {
		return err
	}
	if journey := e.richesJourneys[owner]; journey != nil {
		for _, stop := range journey.Route.Stops {
			if !strings.EqualFold(stop.System, system) {
				continue
			}
			for _, body := range stop.Bodies {
				if !strings.EqualFold(body.Name, name) {
					continue
				}
				key := richesBodyKey(stop, body)
				observation := journey.Observed[key]
				observation.Manual = checked
				if observation == (richesBodyObservation{}) {
					delete(journey.Observed, key)
				} else {
					if journey.Observed == nil {
						journey.Observed = make(map[string]richesBodyObservation)
					}
					journey.Observed[key] = observation
				}
				e.revision++
				return nil
			}
		}
	}
	return errors.New("Планета не найдена на текущем маршруте.")
}

func (e *Engine) queueRichesClipboardLocked(journey *richesJourney) {
	if journey.NextStop == len(journey.Route.Stops) {
		e.clearClipboardLocked(richesJob)
		return
	}
	e.queueClipboardLocked(richesJob, journey.Route.Stops[journey.NextStop].System)
}

func matchesRichesSystem(stop riches.Stop, system, address string) bool {
	if stop.SystemAddress != "" && address != "" {
		return stop.SystemAddress == address
	}
	return system != "" && strings.EqualFold(stop.System, system)
}

func richesBodyKey(stop riches.Stop, body riches.Body) string {
	return strings.ToLower(stop.System + "\x00" + body.Name)
}

func (e *Engine) estimateRichesNextLegLocked(journey *richesJourney, stop riches.Stop) int {
	coords := v.Arr(e.commander["coordinates"])
	if len(coords) == 3 && journey.Route.JumpRangeLY > 0 {
		x, xOK := v.Number(coords[0])
		y, yOK := v.Number(coords[1])
		z, zOK := v.Number(coords[2])
		if xOK && yOK && zOK {
			d := math.Hypot(math.Hypot(stop.Coordinates[0]-x, stop.Coordinates[1]-y), stop.Coordinates[2]-z)
			return max(1, int(math.Ceil(d/journey.Route.JumpRangeLY)))
		}
	}
	return max(1, stop.EstimatedJumps-journey.LegJumps)
}

func (e *Engine) observeRichesLocked(kind string, event, meta M) {
	if v.Bool(meta["side"]) || !oneOf(kind, "FSDJump", "Scan", "SAAScanComplete") {
		return
	}
	replay := v.Bool(meta["replay"])
	owner := e.planWorkspace.key
	if replay {
		owner = e.replayedPlanWorkspace.key
	}
	journey := e.richesJourneys[owner]
	if journey == nil {
		return
	}
	at := v.Time(event["timestamp"])
	if !at.After(journey.CreatedAt) {
		return
	}
	if kind == "FSDJump" {
		if journey.NextStop == len(journey.Route.Stops) || !at.After(journey.LastJumpAt) ||
			v.Bool(event["Taxi"]) || v.Bool(event["Multicrew"]) || v.Bool(e.game["multicrew"]) {
			return
		}
		system, address := v.Str(event["StarSystem"]), v.Str(event["SystemAddress"])
		if system == "" {
			return
		}
		journey.LastJumpAt = at
		journey.CompletedJumps++
		journey.LegJumps++
		if matchesRichesSystem(journey.Route.Stops[journey.NextStop], system, address) {
			journey.NextStop++
			journey.LegJumps = 0
			if !replay {
				e.queueRichesClipboardLocked(journey)
			}
		}
		return
	}
	system := v.Text(event["StarSystem"], e.commander["system"])
	address := v.Text(event["SystemAddress"], e.commander["systemAddress"])
	bodyName := v.Str(event["BodyName"])
	bodyID, hasBodyID := v.Number(event["BodyID"])
	for _, stop := range journey.Route.Stops {
		if !matchesRichesSystem(stop, system, address) {
			continue
		}
		for _, body := range stop.Bodies {
			if !strings.EqualFold(body.Name, bodyName) &&
				!(body.ID != nil && hasBodyID && float64(*body.ID) == bodyID) {
				continue
			}
			if journey.Observed == nil {
				journey.Observed = make(map[string]richesBodyObservation)
			}
			key := richesBodyKey(stop, body)
			observation := journey.Observed[key]
			observation.Scanned = true
			observation.Mapped = observation.Mapped || kind == "SAAScanComplete"
			journey.Observed[key] = observation
			return
		}
	}
}

func (e *Engine) richesSnapshotLocked(includeRoute bool) M {
	out := M{"networkDisabled": e.opts.DisableNetwork, "suggestedJumpRangeLy": e.commander["jumpRangeLy"]}
	if includeRoute {
		out["search"] = v.Copy(e.network[richesJob])
	}
	journey := e.richesJourneys[e.planWorkspace.key]
	if journey == nil {
		return out
	}
	system, address := v.Str(e.commander["system"]), v.Str(e.commander["systemAddress"])
	progress := M{"currentSystem": system, "completedJumps": journey.CompletedJumps,
		"completedStops": journey.NextStop, "totalStops": len(journey.Route.Stops)}
	var route M
	var rows []M
	if includeRoute {
		route = v.Obj(v.Clone(journey.Route))
		rows = v.Rows(route["stops"])
		out["route"] = route
	}
	var totalScan, totalMapping int64
	useMapping := journey.Route.MaxResults == 0 || journey.Route.UseMappingValue
	estimated, remaining := 0, 0
	for i, stop := range journey.Route.Stops {
		estimated += stop.EstimatedJumps
		current := matchesRichesSystem(stop, system, address)
		next := i == journey.NextStop
		if i >= journey.NextStop {
			jumps := stop.EstimatedJumps
			if next {
				jumps = e.estimateRichesNextLegLocked(journey, stop)
				progress["nextSystem"] = stop.System
			}
			remaining += jumps
		}
		var bodyRows []M
		if includeRoute {
			rows[i]["visited"] = i < journey.NextStop
			bodyRows = v.Rows(rows[i]["bodies"])
		}
		for j, body := range stop.Bodies {
			if body.ScanCredits != nil {
				totalScan += *body.ScanCredits
			}
			if body.MappingCredits != nil {
				totalMapping += *body.MappingCredits
			}
			observation := journey.Observed[richesBodyKey(stop, body)]
			journalCompleted := observation.journalComplete(useMapping)
			completed := journalCompleted || observation.Manual
			if includeRoute {
				bodyRows[j]["scanned"], bodyRows[j]["mapped"] = observation.Scanned, observation.Mapped
				bodyRows[j]["completed"], bodyRows[j]["manualCompleted"], bodyRows[j]["journalCompleted"] = completed, observation.Manual, journalCompleted
			}
			if current && !completed && progress["target"] == nil {
				progress["target"] = v.Clone(body)
			}
			if next && !completed && progress["nextTarget"] == nil {
				progress["nextTarget"] = v.Clone(body)
			}
		}
	}
	progress["estimatedJumps"], progress["remainingJumps"] = estimated, remaining
	progress["totalScanCredits"], progress["totalMappingCredits"] = totalScan, totalMapping
	progress["useMappingValue"], progress["totalRewardCredits"] = useMapping, totalScan
	if useMapping {
		progress["totalRewardCredits"] = totalMapping
	}
	progress["finished"] = progress["nextSystem"] == nil && progress["target"] == nil
	out["progress"] = progress
	return out
}
