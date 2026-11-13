package app

import (
	"errors"
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/profile"
	"github.com/Naenier/ED-Copilot/internal/surfacenav"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const statusHasSurfaceCoordinates = 1 << 21

type surfaceBody struct {
	name, system, address string
}

func (body surfaceBody) matches(other surfaceBody) bool {
	if body.name == "" || !strings.EqualFold(body.name, other.name) {
		return false
	}
	if body.address != "" && other.address != "" {
		return body.address == other.address
	}
	return body.system == "" || other.system == "" || strings.EqualFold(body.system, other.system)
}

type surfaceTarget struct {
	point surfacenav.Point
	body  surfaceBody
}

type surfacePosition struct {
	point   surfacenav.Point
	body    surfaceBody
	heading *float64
	radius  float64
}

type surfaceNavigation struct {
	targets          map[string]surfaceTarget
	current          *surfacePosition
	approached, left surfaceBody
	changedAt        time.Time
	statusAt         time.Time
}

func (nav *surfaceNavigation) resetPosition(at time.Time) {
	nav.current, nav.approached, nav.left = nil, surfaceBody{}, surfaceBody{}
	nav.changedAt, nav.statusAt = at, time.Time{}
}

func (nav *surfaceNavigation) adoptTarget(owner string) {
	if pending, ok := nav.targets[""]; owner != "" && ok {
		nav.targets[owner] = pending
		delete(nav.targets, "")
	}
}

func restoreSurfaceTargets(raw M) map[string]surfaceTarget {
	targets := make(map[string]surfaceTarget)
	for owner, value := range raw {
		row := v.Obj(value)
		lat, latOK := v.Number(row["latitude"])
		lon, lonOK := v.Number(row["longitude"])
		point := surfacenav.Point{Latitude: lat, Longitude: lon}
		if latOK && lonOK && point.Valid() {
			targets[owner] = surfaceTarget{point, surfaceBody{v.Str(row["bodyName"]), v.Str(row["systemName"]), v.Str(row["systemAddress"])}}
		}
	}
	return targets
}

func (nav *surfaceNavigation) savedTargets() M {
	saved := M{}
	for owner, target := range nav.targets {
		saved[owner] = surfacePointView(target.point, target.body)
	}
	return saved
}

func surfacePointView(point surfacenav.Point, body surfaceBody) M {
	result := M{"latitude": point.Latitude, "longitude": point.Longitude}
	if body.name != "" {
		result["bodyName"] = body.name
		result["systemName"] = body.system
		result["systemAddress"] = body.address
	}
	return result
}

func (e *Engine) observeSurfaceNavigationLocked(kind string, event, meta M) {
	nav := &e.surfaceNavigation
	cmd := e.commander
	body := surfaceBody{
		name:    v.Text(event["BodyName"], event["Body"]),
		system:  v.Text(event["StarSystem"], cmd["system"]),
		address: v.Text(event["SystemAddress"], cmd["systemAddress"]),
	}
	switch kind {
	case "LoadGame", "Shutdown", "FSDJump", "CarrierJump", "Location", "ApproachBody", "LeaveBody":
		previous := nav.approached
		if nav.current != nil {
			previous = nav.current.body
		}
		nav.resetPosition(v.Time(event["timestamp"]))
		if kind == "ApproachBody" || kind == "Location" && event["BodyType"] == "Planet" {
			nav.approached = body
		}
		switch kind {
		case "LeaveBody":
			nav.left = body
		case "FSDJump", "CarrierJump":
			// Status has no system ID. Keep an old body name from being
			// reinterpreted in the newly arrived system.
			nav.left = surfaceBody{name: previous.name}
		}
		return
	case "Status":
		if !v.Bool(meta["side"]) || e.gameRunning == false || e.planReplay {
			return
		}
	default:
		return
	}
	at := v.Time(event["timestamp"])
	if at.IsZero() || at.Before(nav.changedAt) || at.Before(nav.statusAt) {
		return
	}
	nav.statusAt = at
	lat, latOK := v.Number(event["Latitude"])
	lon, lonOK := v.Number(event["Longitude"])
	point := surfacenav.Point{Latitude: lat, Longitude: lon}
	flags, flagsKnown := v.Number(event["Flags"])
	if body.name == "" && nav.approached.name != "" {
		body = nav.approached
	}
	conflictingApproach := nav.approached.name != "" && !nav.approached.matches(body) && !at.After(nav.changedAt)
	if !latOK || !lonOK || !point.Valid() || flagsKnown && uint64(flags)&statusHasSurfaceCoordinates == 0 ||
		body.name == "" || nav.left.matches(body) || conflictingApproach {
		nav.current = nil
		return
	}
	nav.approached = body
	position := &surfacePosition{point: point, body: body}
	if heading, ok := v.Number(event["Heading"]); ok && heading >= 0 && heading <= 360 {
		if heading == 360 {
			heading = 0
		}
		position.heading = &heading
	}
	if radius, ok := v.Number(event["PlanetRadius"]); ok && radius > 0 {
		position.radius = radius
	} else if nav.current != nil && nav.current.body.matches(body) {
		position.radius = nav.current.radius
	}
	nav.current = position
	owner := e.planWorkspace.key
	target, found := nav.targets[owner]
	if found && target.body.name == "" {
		target.body = body
		nav.targets[owner] = target
	}
}

func (e *Engine) SurfaceNavigationSnapshot() M {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.surfaceNavigationViewLocked()
}

func (e *Engine) surfaceNavigationViewLocked() M {
	result := M{"status": "no-target", "revision": e.revision, "owner": e.planWorkspace.key}
	nav := &e.surfaceNavigation
	target, hasTarget := nav.targets[e.planWorkspace.key]
	if hasTarget {
		result["target"] = surfacePointView(target.point, target.body)
		result["status"] = "waiting-position"
	}
	if nav.current == nil {
		return result
	}
	current := nav.current
	view := surfacePointView(current.point, current.body)
	if current.heading != nil {
		view["heading"] = *current.heading
	}
	result["current"] = view
	if !hasTarget {
		return result
	}
	if !target.body.matches(current.body) {
		result["status"] = "wrong-body"
		return result
	}
	guidance := surfacenav.Navigate(current.point, target.point, current.radius, current.heading)
	calculated := M{"arrived": guidance.Arrived}
	for key, value := range map[string]*float64{
		"bearing": guidance.Bearing, "distanceM": guidance.DistanceM, "relativeHeading": guidance.RelativeHeading,
	} {
		if value != nil {
			calculated[key] = *value
		}
	}
	result["guidance"] = calculated
	result["status"] = "active"
	if guidance.Arrived {
		result["status"] = "arrived"
	}
	return result
}

func (e *Engine) updateSurfaceNavigation(input M) (M, error) {
	action := v.Str(input["action"])
	var point surfacenav.Point
	var err error
	switch action {
	case "set":
		if pair, ok := input["coordinates"].(string); ok {
			point, err = surfacenav.ParsePair(pair)
		} else {
			point, err = surfacenav.ParsePoint(v.Str(input["latitude"]), v.Str(input["longitude"]))
		}
	case "clear":
	default:
		return nil, errors.New("Неизвестное действие.")
	}
	if err != nil {
		return nil, err
	}

	e.saveMu.Lock()
	defer e.saveMu.Unlock()
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.Lock()
	owner := e.planWorkspace.key
	if e.closed {
		e.mu.Unlock()
		return nil, errors.New("Приложение завершает работу.")
	}
	if err := validatePlanWorkspace(input, owner); err != nil {
		e.mu.Unlock()
		return nil, err
	}
	nav := &e.surfaceNavigation
	previous, existed := nav.targets[owner]
	if action == "clear" {
		delete(nav.targets, owner)
	} else {
		body := nav.approached
		if nav.current != nil {
			body = nav.current.body
		}
		nav.targets[owner] = surfaceTarget{point, body}
	}
	e.mu.Unlock()
	saved, _ := e.persistenceSnapshotLocked()
	if err := profile.SaveSettings(e.opts.DataDir, saved); err != nil {
		e.mu.Lock()
		if existed {
			nav.targets[owner] = previous
		} else {
			delete(nav.targets, owner)
		}
		e.mu.Unlock()
		return nil, err
	}
	e.mu.Lock()
	e.revision++
	result := e.surfaceNavigationViewLocked()
	e.mu.Unlock()
	e.Notify()
	return M{"surfaceNavigation": result}, nil
}
