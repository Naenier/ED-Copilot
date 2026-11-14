package trading

import (
	"context"
	"errors"
	"math"
	"net/http"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func stationRouteFilters(options, travel M, now time.Time, role string) M {
	filters := stationSearchFilters(travel, now)
	if !v.Bool(options["allowSurface"]) {
		filters["is_planetary"] = M{"value": false}
	}
	if role != "" {
		power := canonicalRoutePower(options[role+"Power"])
		if power != "" && power != "none" {
			filters["system_controlling_power"] = M{"value": []string{power}}
		}
		if state := v.Str(options[role+"PowerState"]); state != "" {
			filters["system_power_state"] = M{"value": []string{state}}
		}
	}
	return filters
}

type stationRouteScan struct {
	markets []*stationRouteMarket
	checked int
	total   int
	limited bool
}

// FindStationRoutes uses the full commodity markets of matching stations. A
// role-specific query stops nearby stations controlled by other Powers from
// crowding out the ports eligible under the form.
func (s *Searcher) FindStationRoutes(ctx context.Context, input, state M) (M, error) {
	options := StationRouteOptions(input, v.Obj(state["commander"]))
	out := M{"options": options, "results": []M{}, "coverageScope": "regional", "checked": 0, "total": 0}
	if v.Str(options["system"]) == "" {
		return out, errors.New("Укажите исходную систему поиска.")
	}
	if n(options, "cargoCapacity") < 1 {
		return out, errors.New("Укажите грузовместимость корабля.")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	origin, err := s.Reference(ctx, state, options)
	if err != nil {
		return out, err
	}
	options["system"] = origin["system"]
	coords := Coords(origin["coordinates"])
	now := s.Now()
	goods := make(map[string]M)
	var checked, total int
	limited := false
	publish := func(scan stationRouteScan) {
		out["checked"], out["total"] = checked+scan.checked, max(checked+scan.checked, total+scan.total)
		out["coverageLimited"] = limited || scan.limited
		out["updatedAt"] = v.ISO(s.Now())
		if s.OnUpdate != nil {
			s.OnUpdate(v.Copy(out))
		}
	}
	scan := func(role string, inner, outer float64, pages int, previous map[string]*stationRouteMarket) ([]*stationRouteMarket, error) {
		observations, err := s.scanStationRouteMarkets(ctx, options, role, coords, inner, outer, pages, now, goods, previous, publish)
		checked += observations.checked
		total += observations.total
		limited = limited || observations.limited
		return observations.markets, err
	}
	var sources, targets []*stationRouteMarket
	samePolitics := options["sourcePower"] == options["targetPower"] && options["sourcePowerState"] == options["targetPowerState"]
	if samePolitics {
		// Reserve part of the request budget for distant systems, even if
		// there are enough freshly updated stations nearby to fill every page.
		radii := []float64{0, 90, 170, stationRouteNearLy}
		previous := make(map[string]*stationRouteMarket)
		for i := 1; i < len(radii); i++ {
			markets, scanErr := scan("source", radii[i-1], radii[i], stationRoutePages/(len(radii)-1), previous)
			if scanErr != nil {
				if len(sources) == 0 || ctx.Err() != nil {
					return out, scanErr
				}
				limited = true
				continue
			}
			for _, market := range markets {
				previous[market.id] = market
			}
			sources = append(sources, markets...)
		}
		targets = sources
	} else {
		sources, err = scan("source", 0, stationRouteNearLy, stationRoutePages, nil)
		if err != nil {
			return out, err
		}
		targets, err = scan("target", 0, math.Min(6000, stationRouteNearLy+n(options, "maxDistance")), stationRoutePages, nil)
		if err != nil {
			return out, err
		}
	}
	out["results"], err = buildStationRoutes(ctx, sources, targets, options)
	if err != nil {
		return out, err
	}
	publish(stationRouteScan{})
	out["status"] = "Торговые рейсы рассчитаны по доступным котировкам."
	if len(v.Rows(out["results"])) == 0 {
		out["status"] = "Прибыльных рейсов с этими ограничениями не найдено."
	}
	return out, nil
}

func (s *Searcher) scanStationRouteMarkets(ctx context.Context, options M, role string, origin []float64, inner, outer float64, pages int, now time.Time,
	goods map[string]M, previous map[string]*stationRouteMarket, progress func(stationRouteScan)) (stationRouteScan, error) {
	travel := stationRouteTravelOptions(options, outer)
	filters := stationRouteFilters(options, travel, now, role)
	filters["distance"] = M{"min": inner, "max": outer}
	order := []M{{"distance": M{"direction": "asc"}}, {"market_updated_at": M{"direction": "desc"}}}
	power := canonicalRoutePower(options[role+"Power"])
	state := v.Str(options[role+"PowerState"])
	scan := stationRouteScan{}
	seen := make(map[string]bool, stationRoutePageSize*pages)
	for page := 0; page < pages; page++ {
		payload := stationSearchPayload(filters, order, origin, page, stationRoutePageSize)
		response, err := s.request(ctx, http.MethodPost, s.BaseURL+"/stations/search", payload)
		if err != nil {
			if page > 0 && ctx.Err() == nil {
				scan.limited = true
				break
			}
			return scan, err
		}
		stations := v.Rows(response["results"])
		scan.checked += len(stations)
		remoteCount := v.Int(response["count"])
		scan.total = max(scan.checked, min(remoteCount, stationRoutePageSize*pages))
		scan.limited = remoteCount > scan.checked || remoteCount == 0 && len(stations) == stationRoutePageSize
		for _, raw := range stations[:min(len(stations), stationRoutePageSize)] {
			if port := stationRouteMarketFrom(raw, options, travel, now, origin, goods); port != nil &&
				previous[port.id] == nil && !seen[port.id] && stationRouteLimitMatch(port.port, power, state) {
				scan.markets = append(scan.markets, port)
				seen[port.id] = true
			}
		}
		progress(scan)
		if len(stations) < stationRoutePageSize || remoteCount > 0 && scan.checked >= remoteCount {
			break
		}
	}
	return scan, nil
}
