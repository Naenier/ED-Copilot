// Package shiproute plans and tracks travel for the commander's own ship.
package shiproute

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"unicode"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type M = v.M

const (
	maxStops      = 2000
	maxRoutes     = 12
	maxOwners     = 20
	maxSavedStops = 4000
)

type Options struct {
	Origin         string            `json:"origin"`
	Destination    string            `json:"destination"`
	Name           string            `json:"name,omitempty"`
	Mode           string            `json:"mode"`
	JumpRangeLY    float64           `json:"jumpRangeLy,omitempty"`
	MaxFuelPerJump float64           `json:"maxFuelPerJump,omitempty"`
	Efficiency     float64           `json:"efficiency,omitempty"`
	Cargo          float64           `json:"cargo,omitempty"`
	ShipBuild      *FlightParameters `json:"-"`
	AutoCopy       bool              `json:"autoCopy"`
}

type FlightParameters struct {
	FuelPower        float64 `json:"fuelPower"`
	FuelMultiplier   float64 `json:"fuelMultiplier"`
	OptimalMass      float64 `json:"optimalMass"`
	BaseMass         float64 `json:"baseMass"`
	TankSize         float64 `json:"tankSize"`
	InternalTankSize float64 `json:"internalTankSize"`
	ReserveSize      float64 `json:"reserveSize"`
	MaxFuelPerJump   float64 `json:"maxFuelPerJump"`
	RangeBoost       float64 `json:"rangeBoost"`
	UseSupercharge   bool    `json:"useSupercharge"`
}

type Stop struct {
	System        string      `json:"system"`
	SystemAddress string      `json:"systemAddress,omitempty"`
	StarClass     string      `json:"starClass,omitempty"`
	Station       string      `json:"station,omitempty"`
	Action        string      `json:"action,omitempty"`
	DistanceLY    *float64    `json:"distanceLy,omitempty"`
	Coordinates   *[3]float64 `json:"coordinates,omitempty"`
	Jumps         int         `json:"jumps,omitempty"`
	Refuel        bool        `json:"refuel,omitempty"`
	Boost         string      `json:"boost,omitempty"`
	FuelLeft      *float64    `json:"fuelLeft,omitempty"`
	FuelUsed      *float64    `json:"fuelUsed,omitempty"`
}

type Route struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Mode           string  `json:"mode"`
	Origin         string  `json:"origin"`
	Destination    string  `json:"destination"`
	CreatedAt      string  `json:"createdAt"`
	ObservedFrom   string  `json:"observedFrom,omitempty"`
	LastArrivalAt  string  `json:"lastArrivalAt,omitempty"`
	JumpRangeLY    float64 `json:"jumpRangeLy,omitempty"`
	MaxFuelPerJump float64 `json:"maxFuelPerJump,omitempty"`
	AutoCopy       bool    `json:"autoCopy"`
	Next           int     `json:"next"`
	Stops          []Stop  `json:"stops"`
}

func validText(s string, limit int) bool {
	return s != "" && len([]rune(s)) <= limit && !strings.ContainsFunc(s, unicode.IsControl)
}

func ParseOptions(input, context M) (Options, error) {
	opts := Options{
		Origin:      strings.TrimSpace(v.Text(input["origin"], v.At(context, "commander", "system"))),
		Destination: strings.TrimSpace(v.Str(input["destination"])),
		Name:        strings.TrimSpace(v.Str(input["name"])),
		Mode:        v.Text(input["mode"], "neutron"), AutoCopy: v.Bool(input["autoCopy"]),
		JumpRangeLY:    v.Num(v.First(input["jumpRangeLy"], v.At(context, "commander", "jumpRangeLy"))),
		MaxFuelPerJump: v.Num(input["maxFuelPerJump"]), Efficiency: v.Num(v.First(input["efficiency"], 60)),
		Cargo: v.Num(v.First(input["cargo"], v.At(context, "commander", "cargoTons"))),
	}
	if raw := strings.TrimSpace(v.Str(input["shipBuild"])); raw != "" {
		var build FlightParameters
		if len(raw) > 64000 || !strings.HasPrefix(raw, "{") || json.Unmarshal([]byte(raw), &build) != nil {
			return Options{}, errors.New("Проверьте параметры корабля для точного маршрута.")
		}
		opts.ShipBuild = &build
		opts.MaxFuelPerJump = build.MaxFuelPerJump
	}
	if err := validateOptions(opts); err != nil {
		return Options{}, err
	}
	return opts, nil
}

func validateOptions(o Options) error {
	if !validText(o.Origin, 160) || !validText(o.Destination, 160) || strings.EqualFold(o.Origin, o.Destination) {
		return errors.New("Укажите разные системы отправления и назначения.")
	}
	if o.Name != "" && !validText(o.Name, 120) {
		return errors.New("Название путешествия слишком длинное.")
	}
	if o.Mode != "neutron" && o.Mode != "exact" {
		return errors.New("Неизвестный способ расчёта маршрута корабля.")
	}
	if !finiteRange(o.JumpRangeLY, 0, 500) || o.Mode == "neutron" && o.JumpRangeLY < 1 || !finiteRange(o.MaxFuelPerJump, 0, 100) {
		return errors.New("Проверьте дальность и расход топлива корабля.")
	}
	if !finiteRange(o.Efficiency, 1, 100) || !finiteRange(o.Cargo, 0, 10000) {
		return errors.New("Проверьте груз и параметры расчёта.")
	}
	if b := o.ShipBuild; o.Mode == "exact" && (b == nil || !finiteRange(b.FuelPower, 1, 5) ||
		!finiteRange(b.FuelMultiplier, 0.000001, 1) || !finiteRange(b.OptimalMass, 1, 100000) ||
		!finiteRange(b.BaseMass, 1, 100000) || !finiteRange(b.TankSize, 1, 2000) ||
		!finiteRange(b.InternalTankSize, 0, 100) || !finiteRange(b.ReserveSize, 0, b.TankSize) ||
		!finiteRange(b.MaxFuelPerJump, .01, min(100, b.TankSize)) || !finiteRange(b.RangeBoost, 0, 50)) {
		return errors.New("Для точного расчёта нужны характеристики корпуса, FSD и топливных баков.")
	}
	return nil
}

func finiteRange(n, low, high float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0) && n >= low && n <= high
}

func floatPointer(raw any, high float64) *float64 {
	n, ok := v.Number(raw)
	if !ok || !finiteRange(n, 0, high) {
		return nil
	}
	return &n
}

func coords(raw any) *[3]float64 {
	values := v.Arr(raw)
	if len(values) != 3 {
		return nil
	}
	var result [3]float64
	for i, raw := range values {
		n, ok := v.Number(raw)
		if !ok || !finiteRange(n, -1e6, 1e6) {
			return nil
		}
		result[i] = n
	}
	return &result
}

func distance(a, b *[3]float64) *float64 {
	if a == nil || b == nil {
		return nil
	}
	d := math.Hypot(math.Hypot(a[0]-b[0], a[1]-b[1]), a[2]-b[2])
	return &d
}

func matches(stop Stop, system, address string) bool {
	if stop.SystemAddress != "" && address != "" {
		return stop.SystemAddress == address
	}
	return system != "" && strings.EqualFold(stop.System, system)
}

func StarKind(class string) string {
	class = strings.ToUpper(strings.TrimSpace(class))
	if class == "NEUTRON STAR" || class == "N" {
		return "neutron"
	}
	if class == "SCOOPABLE" {
		return "scoopable"
	}
	if class == "" || class == "UNKNOWN" || class == "?" {
		return "unknown"
	}
	if len(class) == 1 && strings.Contains("OBAFGKM", class) {
		return "scoopable"
	}
	// These are known Frontier main-sequence suffixes and subtypes. A class
	// such as MS, Ae/Be or a black hole must not inherit the first letter.
	if strings.Contains("OBAFGKM", class[:1]) && (class[1] >= '0' && class[1] <= '9' || class[1:] == "V" || class == "A_BLUEWHITE") {
		return "scoopable"
	}
	if strings.Contains(class, "STAR") || class == "H" || class == "X" || class == "L" || class == "T" || class == "Y" || class == "TTS" ||
		class == "MS" || class == "S" || class == "C" || class == "CN" || class == "CJ" || class == "CH" || class == "CHD" ||
		class == "AE/BE" || strings.HasPrefix(class, "D") || strings.HasPrefix(class, "W") {
		return "unscoopable"
	}
	return "unknown"
}
