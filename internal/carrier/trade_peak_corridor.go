package carrier

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// The published Rackham's Peak corridor, with system addresses and galactic
// coordinates checked against Spansh. Coordinates are in light-years.
// https://www.edsm.net/en/system/id/53148050/name/HIP+58832
var tradePeakSystems = [...]struct {
	name, address string
	coords        [3]float64
}{
	{"Sol", "10477373803", [3]float64{0, 0, 0}},
	{"Wregoe MC-Y b44-0", "670150763905", [3]float64{-49.40625, 487.25, -98.15625}},
	{"Wregoe MY-R c20-0", "83786240690", [3]float64{-64.15625, 977.1875, -184.8125}},
	{"Plaa Trua GN-K d8-1", "44837407051", [3]float64{-58.28125, 1440.09375, -339.5625}},
	{"HD 103780", "83854070394", [3]float64{13.28125, 1894.28125, -425.125}},
	{"HIP 58494", "10477832507", [3]float64{-16.28125, 2291.46875, -504.96875}},
	{"HIP 58428", "84056052330", [3]float64{105.375, 2662.9375, -534.0625}},
	{"HD 109214", "84123325114", [3]float64{156.59375, 2879.875, -113.90625}},
	{"HIP 59567", "84727370410", [3]float64{495.21875, 2944.28125, -199.65625}},
	{"HD 102897", "10645784899", [3]float64{801.84375, 3135.46875, -404.96875}},
	{"HD 102000", "85130351194", [3]float64{753.15625, 3373.59375, -592.90625}},
	{"HD 102779", "85197754978", [3]float64{799.8125, 3710.21875, -564.625}},
	{"HD 104392", "84862571106", [3]float64{599.8125, 4155.3125, -562.40625}},
	{"HIP 56843", "10612427019", [3]float64{596.625, 4081, -965.5}},
	{"HIP 57478", "84325798410", [3]float64{292.28125, 4277.90625, -1020.28125}},
	{"HIP 57784", "10562226435", [3]float64{352.5625, 4744.125, -1033.15625}},
	{"HD 104495", "83990712850", [3]float64{72.375, 4846.09375, -965.5}},
	{"HD 105341", "84192399914", [3]float64{184.5, 5273.375, -864.375}},
	{tradePeakSystem, "10461677819", [3]float64{-111.6875, 5319.21875, -1115.375}},
}

const tradePeakConnectorMaxHops = (len(tradePeakSystems) - 1) / 2
const tradePeakJumpLimit = 500

func knownTradePeakOrigin(system string) M {
	for _, point := range tradePeakSystems {
		if strings.EqualFold(point.name, strings.TrimSpace(system)) {
			return M{"system": point.name, "systemAddress": point.address, "coordinates": point.coords[:]}
		}
	}
	return nil
}

func tradePeakSuffix(index int) []tradeHop {
	hops := make([]tradeHop, 0, len(tradePeakSystems)-index-1)
	for i := index; i+1 < len(tradePeakSystems); i++ {
		from, to := tradePeakSystems[i], tradePeakSystems[i+1]
		hops = append(hops, tradeHop{from.name, to.name, trading.Distance(from.coords[:], to.coords[:])})
	}
	return hops
}

// An origin on the corridor follows its original suffix. A nearby origin can
// join at the furthest directly reachable point in the first half.
func directTradePeakCorridor(origin M) ([]tradeHop, bool) {
	name := strings.TrimSpace(v.Str(origin["system"]))
	if name == "" {
		return nil, false
	}
	for i, point := range tradePeakSystems {
		if strings.EqualFold(name, point.name) {
			return tradePeakSuffix(i), true
		}
	}
	for i := tradePeakConnectorMaxHops; i >= 0; i-- {
		point := tradePeakSystems[i]
		distance := trading.Distance(origin["coordinates"], point.coords[:])
		if distance > 0 && distance <= tradePeakJumpLimit {
			return append([]tradeHop{{name, point.name, distance}}, tradePeakSuffix(i)...), true
		}
	}
	return nil, false
}

func tradePeakCorridor(ctx context.Context, route *RoutePlanner, origin M) ([]tradeHop, error) {
	if hops, ok := directTradePeakCorridor(origin); ok {
		return hops, nil
	}
	connectorError := errors.New("Не найден короткий маршрут к началу коридора HIP 58832.")
	anchor, nearest := -1, math.Inf(1)
	for i := 0; i <= tradePeakConnectorMaxHops; i++ {
		distance := trading.Distance(origin["coordinates"], tradePeakSystems[i].coords[:])
		if distance < nearest {
			anchor, nearest = i, distance
		}
	}
	name := v.Str(origin["system"])
	if name == "" || anchor < 0 || nearest > float64(tradePeakConnectorMaxHops)*tradePeakJumpLimit {
		return nil, connectorError
	}
	point := tradePeakSystems[anchor]
	source := M{"name": name, "id64": origin["systemAddress"]}
	if id, err := strconv.ParseUint(v.Str(source["id64"]), 10, 64); err != nil || id == 0 {
		source, err = route.Resolve(ctx, name)
		if err != nil {
			return nil, err
		}
	}
	target := M{"name": point.name, "id64": point.address}
	options := M{"calculateStartingFuel": true}
	response, err := route.job(ctx, BuildRoutePayload(options, []M{source, target}, nil))
	if err != nil {
		return nil, err
	}
	if len(v.Rows(v.At(response, "result", "jumps"))) > tradePeakConnectorMaxHops+1 {
		return nil, connectorError
	}
	normalized, err := NormalizeProviderRoute(response, options, nil)
	if err != nil {
		return nil, err
	}
	stops := v.Rows(v.At(normalized, "route", "stops"))
	first, last := stops[0], stops[len(stops)-1]
	if v.Str(first["id64"]) != v.Str(source["id64"]) || v.Str(last["id64"]) != point.address ||
		trading.Distance(first["coordinates"], origin["coordinates"]) > .01 ||
		trading.Distance(last["coordinates"], point.coords[:]) > .01 {
		return nil, connectorError
	}
	first["name"], last["name"] = name, point.name
	hops := make([]tradeHop, 0, len(stops)-1+len(tradePeakSystems)-anchor-1)
	for i := 1; i < len(stops); i++ {
		from, to := stops[i-1], stops[i]
		distance := trading.Distance(from["coordinates"], to["coordinates"])
		if distance <= 0 || distance > tradePeakJumpLimit {
			return nil, connectorError
		}
		hops = append(hops, tradeHop{v.Str(from["name"]), v.Str(to["name"]), distance})
	}
	return append(hops, tradePeakSuffix(anchor)...), nil
}
