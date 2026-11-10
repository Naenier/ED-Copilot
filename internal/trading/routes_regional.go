package trading

import (
	"context"
	"errors"
	"maps"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	regionalRoutePages    = 3
	regionalRoutePageSize = 32
)

// Station search includes each station's full market. Read bounded pages of
// nearby markets and compare every commodity on them, including unfamiliar ones.
func (s *Searcher) findRegionalRoutes(ctx context.Context, state, options, out M) (M, error) {
	reference, err := s.Reference(ctx, state, options)
	if err != nil {
		return nil, err
	}
	coords, now := Coords(reference["coordinates"]), s.Now()
	out["coverageScope"] = "regional"
	markets := map[string]M{}
	selected := map[string]bool{}
	for _, key := range v.Arr(options["resourceKeys"]) {
		selected[v.Str(key)] = true
	}
	goods := map[string]M{}
	seen := map[string]bool{}
	filters := stationSearchFilters(options, now)
	order := []M{{"distance": M{"direction": "asc"}}, {"market_updated_at": M{"direction": "desc"}}}
	own := v.Str(v.At(state, "carrier", "id"))
	for page := 0; page < regionalRoutePages; page++ {
		payload := stationSearchPayload(filters, order, coords, page, regionalRoutePageSize)
		response, err := s.request(ctx, "POST", s.BaseURL+"/stations/search", payload)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil, ctx.Err()
			}
			if page == 0 && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, err
			}
			out["coverageLimited"] = true
			break
		}
		stations := v.Rows(response["results"])
		for _, raw := range stations[:min(len(stations), regionalRoutePageSize)] {
			station := marketStation(raw, options, now, coords)
			if station == nil || own != "" && station["marketId"] == own || seen[v.Str(station["id"])] {
				continue
			}
			seen[v.Str(station["id"])] = true
			sides := map[string]M{"buy": station, "sell": maps.Clone(station)}
			quotes := map[string]map[string]M{"buy": {}, "sell": {}}
			for side, market := range sides {
				market["routeQuotes"] = quotes[side]
			}
			for _, row := range v.Rows(raw["market"]) {
				name := v.First(row["commodity"], row["symbol"], row["name"])
				key := v.Key(name)
				if key == "" {
					continue
				}
				good, known := goods[key]
				if !known {
					good = Good(name, nil)
					good["quantity"] = options["cargoCapacity"]
					goods[key] = good
				}
				key = v.Str(good["key"])
				if good["kind"] == "raw" || key == "drones" || len(selected) > 0 && !selected[key] {
					continue
				}
				for side, market := range sides {
					if quotes[side][key] != nil {
						continue
					}
					quote := marketQuote(row, good, side, v.Str(station["updatedAt"]), now)
					if quote == nil {
						continue
					}
					quotes[side][key] = quote
					if markets[key] == nil {
						markets[key] = M{}
					}
					rows, _ := markets[key][side].([]M)
					markets[key][side] = append(rows, market)
				}
			}
		}
		scanned, total := page*regionalRoutePageSize+len(stations), v.Int(response["count"])
		out["checked"], out["total"] = scanned, max(scanned, min(total, regionalRoutePageSize*regionalRoutePages))
		out["coverageLimited"] = total > scanned
		if len(stations) < regionalRoutePageSize || total > 0 && scanned >= total {
			break
		}
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil, ctx.Err()
	}
	out["results"] = BuildRoutes(markets, options)
	out["status"] = "Торговые рейсы рассчитаны по доступным котировкам."
	if len(v.Rows(out["results"])) == 0 {
		out["status"] = "Прибыльных рейсов с этими ограничениями не найдено."
	}
	out["updatedAt"] = v.ISO(s.Now())
	return out, nil
}
