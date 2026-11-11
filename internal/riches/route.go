// Package riches plans mapping stops around a reference system.
package riches

import (
	"errors"
	"math"
	"sort"
	"strings"
	"unicode"
)

const MaxStops = 100

type Options struct {
	Origin      string  `json:"origin"`
	JumpRangeLY float64 `json:"jumpRangeLy"`
	RadiusLY    float64 `json:"radiusLy"`
}

type Route struct {
	Origin              string  `json:"origin"`
	OriginSystemAddress string  `json:"originSystemAddress,omitempty"`
	JumpRangeLY         float64 `json:"jumpRangeLy"`
	RadiusLY            float64 `json:"radiusLy"`
	Stops               []Stop  `json:"stops"`
	Truncated           bool    `json:"truncated,omitempty"`
}

type Stop struct {
	System         string     `json:"system"`
	SystemAddress  string     `json:"systemAddress,omitempty"`
	Coordinates    [3]float64 `json:"coordinates"`
	DistanceLY     float64    `json:"distanceLy"`
	EstimatedJumps int        `json:"estimatedJumps"`
	Bodies         []Body     `json:"bodies"`
}

type Body struct {
	ID             *int     `json:"bodyId,omitempty"`
	Name           string   `json:"name"`
	Designation    string   `json:"designation"`
	Type           string   `json:"type"`
	DistanceLS     *float64 `json:"distanceLs"`
	ScanCredits    *int64   `json:"scanCredits"`
	MappingCredits *int64   `json:"mappingCredits"`
}

func Validate(opts Options) error {
	origin := strings.TrimSpace(opts.Origin)
	if origin == "" || len(origin) > 200 || strings.ContainsFunc(origin, unicode.IsControl) {
		return errors.New("Укажите название стартовой системы.")
	}
	if math.IsNaN(opts.JumpRangeLY) || opts.JumpRangeLY < 0.1 || opts.JumpRangeLY > 500 {
		return errors.New("Дальность прыжка должна быть от 0,1 до 500 св. лет.")
	}
	if math.IsNaN(opts.RadiusLY) || opts.RadiusLY < 1 || opts.RadiusLY > 5000 {
		return errors.New("Радиус поиска должен быть от 1 до 5 000 св. лет.")
	}
	return nil
}

func distance(a, b [3]float64) float64 {
	return math.Hypot(math.Hypot(a[0]-b[0], a[1]-b[1]), a[2]-b[2])
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

// Order the nearest candidates first, then take the nearest unvisited system
// from each stop. The game map resolves the intermediate jumps for each leg.
func orderStops(originName string, origin [3]float64, candidates map[string]*Stop, jumpRange float64) []Stop {
	remaining := make([]*Stop, 0, len(candidates))
	for _, stop := range candidates {
		remaining = append(remaining, stop)
	}
	sort.Slice(remaining, func(i, j int) bool {
		a, b := remaining[i], remaining[j]
		atOrigin, btOrigin := strings.EqualFold(a.System, originName), strings.EqualFold(b.System, originName)
		if atOrigin != btOrigin {
			return atOrigin
		}
		da, db := distance(origin, a.Coordinates), distance(origin, b.Coordinates)
		if da != db {
			return da < db
		}
		return a.System < b.System
	})
	remaining = remaining[:min(len(remaining), MaxStops)]

	stops := make([]Stop, 0, len(remaining))
	position := origin
	for len(remaining) > 0 {
		nearest, nearestDistance := 0, distance(position, remaining[0].Coordinates)
		for index, candidate := range remaining[1:] {
			if d := distance(position, candidate.Coordinates); d < nearestDistance {
				nearest, nearestDistance = index+1, d
			}
		}
		stop := *remaining[nearest]
		stop.DistanceLY = nearestDistance
		stop.EstimatedJumps = int(math.Ceil(nearestDistance / jumpRange))
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
		remaining = append(remaining[:nearest], remaining[nearest+1:]...)
		position = stop.Coordinates
	}
	return stops
}
