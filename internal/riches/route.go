// Package riches plans mapping stops around a reference system or flight corridor.
package riches

import (
	"errors"
	"math"
	"sort"
	"strings"
	"unicode"
)

const (
	MaxStops               = 2000
	DefaultMaxResults      = 100
	DefaultMaxDistanceLS   = 10_000
	DefaultMinMappingValue = 1_000_000
	MaxArrivalDistanceLS   = 1_000_000
	MaxMappingValue        = 1_000_000_000
)

type Options struct {
	Origin          string   `json:"origin"`
	Destination     string   `json:"destination"`
	JumpRangeLY     float64  `json:"jumpRangeLy"`
	RadiusLY        float64  `json:"radiusLy"`
	MaxResults      int      `json:"maxResults"`
	MaxDistanceLS   *float64 `json:"maxDistanceLs"`
	MinMappingValue *int64   `json:"minMappingValue"`
	UseMappingValue bool     `json:"useMappingValue"`
	AvoidThargoids  bool     `json:"avoidThargoids"`
	Loop            bool     `json:"loop"`
}

type Route struct {
	Origin                   string      `json:"origin"`
	OriginSystemAddress      string      `json:"originSystemAddress,omitempty"`
	OriginCoordinates        *[3]float64 `json:"originCoordinates,omitempty"`
	Destination              string      `json:"destination,omitempty"`
	DestinationSystemAddress string      `json:"destinationSystemAddress,omitempty"`
	JumpRangeLY              float64     `json:"jumpRangeLy"`
	RadiusLY                 float64     `json:"radiusLy"`
	MaxResults               int         `json:"maxResults,omitempty"`
	MaxDistanceLS            float64     `json:"maxDistanceLs"`
	MinMappingValue          int64       `json:"minMappingValue"`
	UseMappingValue          bool        `json:"useMappingValue"`
	AvoidThargoids           bool        `json:"avoidThargoids"`
	Loop                     bool        `json:"loop"`
	Stops                    []Stop      `json:"stops"`
	Truncated                bool        `json:"truncated,omitempty"`
}

type Stop struct {
	System         string     `json:"system"`
	SystemAddress  string     `json:"systemAddress,omitempty"`
	Coordinates    [3]float64 `json:"coordinates"`
	DistanceLY     float64    `json:"distanceLy"`
	EstimatedJumps int        `json:"estimatedJumps"`
	Bodies         []Body     `json:"bodies"`
	Transit        bool       `json:"transit,omitempty"`
}

type Body struct {
	ID             *int     `json:"bodyId,omitempty"`
	Name           string   `json:"name"`
	Designation    string   `json:"designation"`
	Type           string   `json:"type"`
	DistanceLS     *float64 `json:"distanceLs"`
	ScanCredits    *int64   `json:"scanCredits"`
	MappingCredits *int64   `json:"mappingCredits"`
	Terraformable  *bool    `json:"terraformable"`
}

func Validate(opts Options) error {
	if !validSystemName(strings.TrimSpace(opts.Origin)) {
		return errors.New("Укажите название стартовой системы.")
	}
	if destination := strings.TrimSpace(opts.Destination); destination != "" && !validSystemName(destination) {
		return errors.New("Проверьте название системы назначения.")
	}
	if math.IsNaN(opts.JumpRangeLY) || opts.JumpRangeLY < 0.1 || opts.JumpRangeLY > 500 {
		return errors.New("Дальность прыжка должна быть от 0,1 до 500 св. лет.")
	}
	if math.IsNaN(opts.RadiusLY) || opts.RadiusLY < 1 || opts.RadiusLY > 5000 {
		return errors.New("Радиус поиска должен быть от 1 до 5 000 св. лет.")
	}
	if opts.MaxResults < 0 || opts.MaxResults > MaxStops {
		return errors.New("Максимальное число систем должно быть от 1 до 2 000.")
	}
	if n := opts.MaxDistanceLS; n != nil && (math.IsNaN(*n) || *n < 0 || *n > MaxArrivalDistanceLS) {
		return errors.New("Расстояние от звезды должно быть от 0 до 1 000 000 св. с.")
	}
	if n := opts.MinMappingValue; n != nil && (*n < 0 || *n > MaxMappingValue) {
		return errors.New("Минимальная награда за DSS должна быть от 0 до 1 000 000 000 кр.")
	}
	return nil
}

func validSystemName(s string) bool {
	return s != "" && len(s) <= 200 && !strings.ContainsFunc(s, unicode.IsControl)
}

func normalize(opts Options) Route {
	route := Route{
		JumpRangeLY: opts.JumpRangeLY, RadiusLY: opts.RadiusLY,
		MaxResults: DefaultMaxResults, MaxDistanceLS: DefaultMaxDistanceLS, MinMappingValue: DefaultMinMappingValue,
		UseMappingValue: opts.UseMappingValue, AvoidThargoids: opts.AvoidThargoids, Loop: opts.Loop,
	}
	if opts.MaxResults != 0 {
		route.MaxResults = opts.MaxResults
	}
	if opts.MaxDistanceLS != nil {
		route.MaxDistanceLS = *opts.MaxDistanceLS
	}
	if opts.MinMappingValue != nil {
		route.MinMappingValue = *opts.MinMappingValue
	}
	return route
}

func distance(a, b [3]float64) float64 {
	return math.Hypot(math.Hypot(a[0]-b[0], a[1]-b[1]), a[2]-b[2])
}

func searchDistance(point, start [3]float64, endpoint *Stop) float64 {
	if endpoint == nil || endpoint.Coordinates == start {
		return distance(point, start)
	}
	end := endpoint.Coordinates
	x, y, z := end[0]-start[0], end[1]-start[1], end[2]-start[2]
	position := ((point[0]-start[0])*x + (point[1]-start[1])*y + (point[2]-start[2])*z) / (x*x + y*y + z*z)
	position = math.Max(0, math.Min(position, 1))
	return distance(point, [3]float64{start[0] + position*x, start[1] + position*y, start[2] + position*z})
}

func designation(system, name string) string {
	if len(name) > len(system) && strings.EqualFold(name[:len(system)], system) && name[len(system)] == ' ' {
		return strings.TrimSpace(name[len(system):])
	}
	return name
}

func bodyType(subtype string) string {
	switch strings.ToLower(strings.TrimSpace(subtype)) {
	case "earth-like world":
		return "earthLike"
	case "water world":
		return "waterWorld"
	default:
		return ""
	}
}

func terraformable(state string) *bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "terraformable":
		value := true
		return &value
	case "not terraformable", "terraformed", "terraforming":
		value := false
		return &value
	default:
		return nil
	}
}

func stopValue(stop *Stop, useMappingValue bool) int64 {
	best := int64(-1)
	for _, body := range stop.Bodies {
		value := body.ScanCredits
		if useMappingValue {
			value = body.MappingCredits
		}
		if value != nil && *value > best {
			best = *value
		}
	}
	return best
}

// Rank mapping targets by reward, then construct the flight through the selected
// systems. An explicit destination or a return leg is placed last.
func orderStops(route Route, origin [3]float64, destination *Stop, candidates map[string]*Stop) []Stop {
	remaining := make([]*Stop, 0, len(candidates))
	rewards := make(map[*Stop]int64, len(candidates))
	for _, stop := range candidates {
		remaining = append(remaining, stop)
		rewards[stop] = stopValue(stop, route.UseMappingValue)
	}
	sort.Slice(remaining, func(i, j int) bool {
		a, b := remaining[i], remaining[j]
		atOrigin, btOrigin := strings.EqualFold(a.System, route.Origin), strings.EqualFold(b.System, route.Origin)
		if atOrigin != btOrigin {
			return atOrigin
		}
		if rewards[a] != rewards[b] {
			return rewards[a] > rewards[b]
		}
		da, db := distance(origin, a.Coordinates), distance(origin, b.Coordinates)
		if da != db {
			return da < db
		}
		return a.System < b.System
	})
	remaining = remaining[:min(len(remaining), route.MaxResults)]
	stops := make([]Stop, 0, len(remaining)+1)
	position := origin
	addStop := func(stop Stop) {
		stop.DistanceLY = distance(position, stop.Coordinates)
		stop.EstimatedJumps = int(math.Ceil(stop.DistanceLY / route.JumpRangeLY))
		sort.SliceStable(stop.Bodies, func(i, j int) bool {
			a, b := stop.Bodies[i], stop.Bodies[j]
			if a.DistanceLS != nil && b.DistanceLS != nil && *a.DistanceLS != *b.DistanceLS {
				return *a.DistanceLS < *b.DistanceLS
			}
			if (a.DistanceLS == nil) != (b.DistanceLS == nil) {
				return a.DistanceLS != nil
			}
			return a.Name < b.Name
		})
		stops = append(stops, stop)
		position = stop.Coordinates
	}

	final := destination
	if destination != nil && !strings.EqualFold(destination.System, route.Origin) {
		for i, stop := range remaining {
			if strings.EqualFold(stop.System, destination.System) {
				final = stop
				remaining = append(remaining[:i], remaining[i+1:]...)
				break
			}
		}
	}
	for len(remaining) > 0 {
		nearest, nearestDistance := 0, distance(position, remaining[0].Coordinates)
		for index, candidate := range remaining[1:] {
			if d := distance(position, candidate.Coordinates); d < nearestDistance {
				nearest, nearestDistance = index+1, d
			}
		}
		addStop(*remaining[nearest])
		remaining = append(remaining[:nearest], remaining[nearest+1:]...)
	}
	if final == nil && route.Loop {
		final = &Stop{System: route.Origin, SystemAddress: route.OriginSystemAddress, Coordinates: origin, Bodies: []Body{}, Transit: true}
	}
	if final != nil && (!strings.EqualFold(final.System, route.Origin) || position != origin) {
		addStop(*final)
	}
	return stops
}
