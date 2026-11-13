package surfacenav

import (
	"errors"
	"math"
	"testing"
)

func TestParseCoordinates(t *testing.T) {
	for _, raw := range []string{
		"44.4825 / -61.4854",
		"44,4825 / −61,4854",
		"44,4825; -61,4854",
		"44,4825 -61,4854",
		"44.4825, -61.4854",
		"44.4825 -61,4854",
	} {
		point, err := ParsePair(raw)
		if err != nil || point != (Point{44.4825, -61.4854}) {
			t.Errorf("%q: got %v, %v", raw, point, err)
		}
	}
	for _, tc := range []struct {
		raw  string
		want error
	}{
		{"44.4825", ErrPair},
		{" / 1", ErrLatitude},
		{"90.0001 / 0", ErrLatitude},
		{"-90.0001 / 0", ErrLatitude},
		{"1 / 180.0001", ErrLongitude},
		{"1 / -180.0001", ErrLongitude},
		{"1 / NaN", ErrLongitude},
		{"Inf / 0", ErrLatitude},
		{"1.2.3 / 0", ErrLatitude},
		{"1 / 2 / 3", ErrLongitude},
	} {
		if _, err := ParsePair(tc.raw); !errors.Is(err, tc.want) {
			t.Errorf("%q: got %v, want %v", tc.raw, err, tc.want)
		}
	}
	if point, err := ParsePoint("−0", "+180"); err != nil || math.Signbit(point.Latitude) || point.Longitude != 180 {
		t.Errorf("signed zero and boundary: got %v, %v", point, err)
	}
}

func TestNavigationCoursesAndSurfaceDistances(t *testing.T) {
	for _, tc := range []struct {
		name         string
		current      Point
		target       Point
		heading      float64
		bearing      float64
		relativeTurn float64
		angleDegrees float64
	}{
		{"due north", Point{0, 0}, Point{1, 0}, 355, 0, 5, 1},
		{"due south", Point{0, 0}, Point{-1, 0}, 185, 180, -5, 1},
		{"across longitude seam east", Point{0, 179.9}, Point{0, -179.9}, 80, 90, 10, .2},
		{"across longitude seam west", Point{0, -179.9}, Point{0, 179.9}, 280, 270, -10, .2},
		{"toward the north pole", Point{89, 15}, Point{90, -145}, 360, 0, 0, 1},
		{"toward the south pole", Point{-89, 15}, Point{-90, -145}, -180, 180, 0, 1},
		{"quarter meridian", Point{0, 18}, Point{90, 5}, 270, 0, 90, 90},
		{"near opposite hemisphere", Point{0, 0}, Point{0, 179.99999}, 0, 90, 90, 179.99999},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const radius = 1000000
			got := Navigate(tc.current, tc.target, radius, &tc.heading)
			checkNear(t, got.Bearing, tc.bearing, 1e-9)
			checkNear(t, got.RelativeHeading, tc.relativeTurn, 1e-9)
			checkNear(t, got.DistanceM, tc.angleDegrees*radians*radius, 1e-5)
			if got.Arrived {
				t.Error("unexpected arrival")
			}
		})
	}
}

func TestNavigationIndeterminateAndArrival(t *testing.T) {
	const radius = 1000000
	heading := 45.0
	for _, tc := range []struct {
		name    string
		from    Point
		to      Point
		dist    float64
		arrived bool
	}{
		{"same position", Point{44.4825, -61.4854}, Point{44.4825, -61.4854}, 0, true},
		{"same seam point", Point{45, -180}, Point{45, 180}, 0, true},
		{"same pole", Point{90, -180}, Point{90, 45}, 0, true},
		{"inside arrival tolerance", Point{}, Point{0, .00005}, radius * radians * .00005, true},
		{"antipodal", Point{31, 82}, Point{-31, -98}, math.Pi * radius, false},
		{"polar origin", Point{90, 45}, Point{0, 180}, math.Pi * radius / 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Navigate(tc.from, tc.to, radius, &heading)
			checkNear(t, got.DistanceM, tc.dist, 1e-6)
			if got.Arrived != tc.arrived || got.Bearing != nil || got.RelativeHeading != nil {
				t.Errorf("unexpected guidance: %+v", got)
			}
		})
	}
}

func TestNavigationSupportsUnknownRadiusAndHeading(t *testing.T) {
	heading := math.NaN()
	for _, radius := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		got := Navigate(Point{}, Point{0, -1}, radius, &heading)
		checkNear(t, got.Bearing, 270, 1e-9)
		if got.DistanceM != nil || got.RelativeHeading != nil || got.Arrived {
			t.Errorf("invented navigation data for radius %v: %+v", radius, got)
		}
	}
	for _, invalid := range []Point{{91, 0}, {0, -181}, {math.NaN(), 0}, {0, math.Inf(1)}} {
		if got := Navigate(invalid, Point{}, 1000000, nil); got != (Guidance{}) {
			t.Errorf("guidance for invalid point: %+v", got)
		}
	}
}

func checkNear(t *testing.T, actual *float64, expected, tolerance float64) {
	t.Helper()
	if actual == nil {
		t.Fatalf("missing value, want %f", expected)
	}
	if math.Abs(*actual-expected) > tolerance {
		t.Errorf("got %.10f, want %.10f", *actual, expected)
	}
}
