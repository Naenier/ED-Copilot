// Package surfacenav calculates the shortest course along a body's surface.
package surfacenav

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

const (
	radians        = math.Pi / 180
	arrivalM       = 2
	angularEpsilon = 1e-12
)

var (
	ErrLatitude  = errors.New("Широта должна быть от −90 до 90.")
	ErrLongitude = errors.New("Долгота должна быть от −180 до 180.")
	ErrPair      = errors.New("Введите широту и долготу через косую черту.")
)

type Point struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (p Point) Valid() bool {
	return math.Abs(p.Latitude) <= 90 && math.Abs(p.Longitude) <= 180
}

func ParsePoint(latitude, longitude string) (Point, error) {
	lat, ok := decimal(latitude)
	if !ok || math.Abs(lat) > 90 {
		return Point{}, ErrLatitude
	}
	lon, ok := decimal(longitude)
	if !ok || math.Abs(lon) > 180 {
		return Point{}, ErrLongitude
	}
	return Point{Latitude: lat, Longitude: lon}, nil
}

// ParsePair accepts a slash, semicolon, space or comma between coordinates.
// Use a slash or space between coordinates written with decimal commas.
func ParsePair(text string) (Point, error) {
	if len(text) > 160 {
		return Point{}, ErrPair
	}
	if split := strings.IndexAny(text, "/;"); split >= 0 {
		return ParsePoint(text[:split], text[split+1:])
	}
	if lat, lon, ok := strings.Cut(text, ","); ok && strings.Count(text, ",") == 1 &&
		len(strings.Fields(lat)) == 1 && len(strings.Fields(lon)) == 1 {
		return ParsePoint(lat, lon)
	}
	if fields := strings.Fields(text); len(fields) == 2 {
		return ParsePoint(fields[0], fields[1])
	}
	return Point{}, ErrPair
}

func decimal(text string) (float64, bool) {
	text = strings.TrimSpace(text)
	if len(text) > 64 {
		return 0, false
	}
	text = strings.NewReplacer("−", "-", ",", ".").Replace(text)
	if strings.ContainsFunc(text, func(c rune) bool {
		return (c < '0' || c > '9') && c != '.' && c != '+' && c != '-'
	}) {
		return 0, false
	}
	value, err := strconv.ParseFloat(text, 64)
	if value == 0 {
		value = 0 // Canonicalize negative zero for display and persistence.
	}
	return value, err == nil
}

type Guidance struct {
	Bearing         *float64 `json:"bearing,omitempty"`
	DistanceM       *float64 `json:"distanceM,omitempty"`
	RelativeHeading *float64 `json:"relativeHeading,omitempty"`
	Arrived         bool     `json:"arrived"`
}

// Navigate gives the initial bearing clockwise from local north and the
// shortest distance on the surface, independent of the vehicle's altitude.
// Relative heading is the shortest turn, positive clockwise. Position fixes
// at the destination, exactly at a pole, or exactly antipodal have no bearing.
func Navigate(current, target Point, radiusM float64, heading *float64) Guidance {
	if !current.Valid() || !target.Valid() {
		return Guidance{}
	}

	fromSin, fromCos := latitudeComponents(current.Latitude)
	toSin, toCos := latitudeComponents(target.Latitude)
	deltaLon := math.Remainder(target.Longitude-current.Longitude, 360) * radians
	deltaSin, deltaCos := math.Sincos(deltaLon)
	east := toCos * deltaSin
	north := fromCos*toSin - fromSin*toCos*deltaCos
	tangent := math.Hypot(east, north)
	dot := fromSin*toSin + fromCos*toCos*deltaCos
	angle := math.Atan2(tangent, dot)

	result := Guidance{Arrived: angle < angularEpsilon}
	if radiusM > 0 && !math.IsInf(radiusM, 0) {
		distance := radiusM * angle
		if !math.IsInf(distance, 0) {
			result.DistanceM = &distance
			result.Arrived = result.Arrived || distance <= arrivalM
		}
	}
	if result.Arrived || fromCos == 0 || tangent < angularEpsilon {
		return result
	}

	course := wrapHeading(math.Atan2(east, north) / radians)
	result.Bearing = &course
	if heading != nil && !math.IsNaN(*heading) && !math.IsInf(*heading, 0) {
		turn := math.Mod(course-wrapHeading(*heading)+540, 360) - 180
		result.RelativeHeading = &turn
	}
	return result
}

func latitudeComponents(degrees float64) (sin, cos float64) {
	if math.Abs(degrees) == 90 {
		return math.Copysign(1, degrees), 0
	}
	return math.Sincos(degrees * radians)
}

func wrapHeading(degrees float64) float64 {
	wrapped := math.Mod(degrees, 360)
	if wrapped < 0 {
		wrapped += 360
	}
	if wrapped == 360 || wrapped == 0 {
		return 0
	}
	return wrapped
}
