package riches

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	searchURL       = "https://spansh.co.uk/api/bodies/search"
	systemSearchURL = "https://spansh.co.uk/api/search/systems"
	pageSize        = 250
	maxPages        = 20
	maxPageBytes    = 4 << 20
)

var errUnknownOrigin = errors.New("Spansh не нашёл стартовую систему.")

type Planner struct {
	client *http.Client
}

func NewPlanner(client *http.Client) *Planner {
	if client == nil {
		client = http.DefaultClient
	}
	return &Planner{client: client}
}

type bodySearch struct {
	ReferenceSystem string           `json:"reference_system,omitempty"`
	ReferenceRoute  *referenceRoute  `json:"reference_route,omitempty"`
	Filters         map[string]any   `json:"filters"`
	Sort            []map[string]any `json:"sort"`
	Size            int              `json:"size"`
	Page            int              `json:"page"`
}

type referenceRoute struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

type searchResult struct {
	Count     int             `json:"count"`
	Reference json.RawMessage `json:"reference"`
	Results   []bodyRow       `json:"results"`
}

type reference struct {
	Name    string      `json:"name"`
	Address json.Number `json:"id64"`
	X       *float64    `json:"x"`
	Y       *float64    `json:"y"`
	Z       *float64    `json:"z"`
}

type bodyRow struct {
	System            string      `json:"system_name"`
	Address           json.Number `json:"system_id64"`
	X                 *float64    `json:"system_x"`
	Y                 *float64    `json:"system_y"`
	Z                 *float64    `json:"system_z"`
	ID                *int        `json:"body_id"`
	Name              string      `json:"name"`
	Subtype           string      `json:"subtype"`
	DistanceLS        *float64    `json:"distance_to_arrival"`
	ScanValue         *int64      `json:"estimated_scan_value"`
	MappingValue      *int64      `json:"estimated_mapping_value"`
	TerraformingState string      `json:"terraforming_state"`
}

// The native Road to Riches router omits many systems near inhabited space.
// Spansh's body search finds the selected planet types directly and includes
// the local body IDs used by the game journal.
func (p *Planner) Plan(ctx context.Context, opts Options) (Route, error) {
	if err := Validate(opts); err != nil {
		return Route{}, err
	}
	route := normalize(opts)
	query := newBodySearch(opts, route)
	candidates := make(map[string]*Stop)
	seenBodies := make(map[string]struct{})
	var origin [3]float64
	var destination *Stop
	var blocked unsafeSystems
	for page := 0; page < maxPages; page++ {
		query.Page = page
		result, err := p.search(ctx, query)
		if page == 0 && errors.Is(err, errUnknownOrigin) {
			result, err = p.retryCanonicalSearch(ctx, &query)
		}
		if err != nil {
			return Route{}, err
		}
		if page == 0 {
			start, end, err := parseReferences(result.Reference, query.ReferenceRoute != nil)
			if err != nil {
				return Route{}, err
			}
			route.Origin, route.OriginSystemAddress, origin = start.System, start.SystemAddress, start.Coordinates
			route.OriginCoordinates = &origin
			if end != nil {
				route.Destination, route.DestinationSystemAddress, destination = end.System, end.SystemAddress, end
				query.ReferenceRoute = &referenceRoute{Source: start.System, Destination: end.System}
			} else {
				query.ReferenceSystem = start.System
				if strings.EqualFold(strings.TrimSpace(opts.Destination), route.Origin) {
					route.Destination, route.DestinationSystemAddress, destination = start.System, start.SystemAddress, start
				}
			}
			if route.AvoidThargoids {
				blocked, err = p.thargoidSystems(ctx)
				if err != nil {
					return Route{}, err
				}
				final := destination
				if final == nil && route.Loop {
					final = start
				}
				if final != nil && blocked.contains(final.System, final.SystemAddress) {
					return Route{}, errors.New("Конечная система отмечена в Spansh как затронутая таргоидами.")
				}
			}
		}
		for _, row := range result.Results {
			if !blocked.contains(row.System, id64(row.Address)) {
				appendBody(candidates, seenBodies, row, origin, destination, route)
			}
		}
		more := len(result.Results) == pageSize && (page+1)*pageSize < result.Count
		if len(candidates) > route.MaxResults || !more || page == maxPages-1 {
			route.Truncated = len(candidates) > route.MaxResults || more
			break
		}
	}
	route.Stops = orderStops(route, origin, destination, candidates)
	return route, nil
}

func newBodySearch(opts Options, route Route) bodySearch {
	origin, destination := cleanSystemName(opts.Origin), cleanSystemName(opts.Destination)
	ranking := "estimated_scan_value"
	if route.UseMappingValue {
		ranking = "estimated_mapping_value"
	}
	query := bodySearch{
		ReferenceSystem: origin,
		Filters: map[string]any{
			"subtype":  map[string][]string{"value": {"Earth-like world", "Water world"}},
			"distance": map[string]float64{"min": 0, "max": opts.RadiusLY},
		},
		Sort: []map[string]any{
			{ranking: map[string]string{"direction": "desc"}},
			{"distance": map[string]string{"direction": "asc"}},
			{"name": map[string]string{"direction": "asc"}},
		},
		Size: pageSize,
	}
	if destination != "" && !strings.EqualFold(origin, destination) {
		query.ReferenceRoute = &referenceRoute{Source: origin, Destination: destination}
		query.ReferenceSystem = ""
	}
	if route.MaxDistanceLS > 0 {
		query.Filters["distance_to_arrival"] = map[string]any{"comparison": "<=>", "value": []float64{0, route.MaxDistanceLS}}
	}
	if route.MinMappingValue > 0 {
		query.Filters["estimated_mapping_value"] = map[string]any{"comparison": "<=>", "value": []int64{route.MinMappingValue, MaxMappingValue}}
	}
	return query
}

func cleanSystemName(name string) string { return strings.Join(strings.Fields(name), " ") }

func parseReferences(raw json.RawMessage, corridor bool) (*Stop, *Stop, error) {
	var origin, destination reference
	if corridor {
		var endpoints []reference
		if err := json.Unmarshal(raw, &endpoints); err == nil && len(endpoints) == 2 {
			origin, destination = endpoints[0], endpoints[1]
		}
	} else {
		_ = json.Unmarshal(raw, &origin)
	}
	start, end := origin.stop(), destination.stop()
	if start == nil {
		return nil, nil, errors.New("Spansh не вернул координаты стартовой системы.")
	}
	if corridor && end == nil {
		return nil, nil, errors.New("Spansh не вернул координаты системы назначения.")
	}
	return start, end, nil
}

func (r reference) stop() *Stop {
	name := strings.TrimSpace(r.Name)
	if name == "" || r.X == nil || r.Y == nil || r.Z == nil {
		return nil
	}
	return &Stop{System: name, SystemAddress: id64(r.Address), Coordinates: [3]float64{*r.X, *r.Y, *r.Z}, Bodies: []Body{}, Transit: true}
}

func (p *Planner) retryCanonicalSearch(ctx context.Context, query *bodySearch) (searchResult, error) {
	if query.ReferenceRoute == nil {
		name, err := p.canonicalSystem(ctx, query.ReferenceSystem)
		if err != nil {
			return searchResult{}, err
		}
		if name == "" || name == query.ReferenceSystem {
			return searchResult{}, errUnknownOrigin
		}
		query.ReferenceSystem = name
		return p.search(ctx, *query)
	}
	source, err := p.canonicalSystem(ctx, query.ReferenceRoute.Source)
	if err != nil {
		return searchResult{}, err
	}
	destination, err := p.canonicalSystem(ctx, query.ReferenceRoute.Destination)
	if err != nil {
		return searchResult{}, err
	}
	if source == "" {
		return searchResult{}, errUnknownOrigin
	}
	if destination == "" || source == query.ReferenceRoute.Source && destination == query.ReferenceRoute.Destination {
		return searchResult{}, errors.New("Spansh не нашёл систему назначения.")
	}
	query.ReferenceRoute = &referenceRoute{Source: source, Destination: destination}
	return p.search(ctx, *query)
}

func appendBody(candidates map[string]*Stop, seen map[string]struct{}, row bodyRow, origin [3]float64, destination *Stop, route Route) {
	system, name, kind := strings.TrimSpace(row.System), strings.TrimSpace(row.Name), bodyType(row.Subtype)
	if system == "" || name == "" || kind == "" || row.X == nil || row.Y == nil || row.Z == nil {
		return
	}
	arrival, mapping := nonnegative(row.DistanceLS), nonnegative(row.MappingValue)
	if route.MaxDistanceLS > 0 && (arrival == nil || *arrival > route.MaxDistanceLS) ||
		route.MinMappingValue > 0 && (mapping == nil || *mapping < route.MinMappingValue) {
		return
	}
	coords := [3]float64{*row.X, *row.Y, *row.Z}
	if searchDistance(coords, origin, destination) > route.RadiusLY+1e-6 {
		return
	}
	key := strings.ToLower(system)
	bodyKey := key + "\x00" + strings.ToLower(name)
	if _, duplicate := seen[bodyKey]; duplicate {
		return
	}
	seen[bodyKey] = struct{}{}

	stop := candidates[key]
	if stop == nil {
		stop = &Stop{System: system, SystemAddress: id64(row.Address), Coordinates: coords}
		candidates[key] = stop
	}
	stop.Bodies = append(stop.Bodies, Body{
		ID:             nonnegative(row.ID),
		Name:           name,
		Designation:    designation(system, name),
		Type:           kind,
		DistanceLS:     arrival,
		ScanCredits:    nonnegative(row.ScanValue),
		MappingCredits: mapping,
		Terraformable:  terraformable(row.TerraformingState),
	})
}

func nonnegative[T int | int64 | float64](value *T) *T {
	if value != nil && *value >= 0 {
		return value
	}
	return nil
}

func id64(value json.Number) string {
	if _, err := strconv.ParseUint(value.String(), 10, 64); err != nil {
		return ""
	}
	return value.String()
}

func (p *Planner) search(ctx context.Context, query bodySearch) (searchResult, error) {
	payload, err := json.Marshal(query)
	if err != nil {
		return searchResult{}, err
	}
	var result searchResult
	if err := p.request(ctx, http.MethodPost, searchURL, bytes.NewReader(payload), &result); err != nil {
		return searchResult{}, err
	}
	if result.Results == nil {
		return searchResult{}, errors.New("Spansh вернул неизвестный формат.")
	}
	return result, nil
}

func (p *Planner) canonicalSystem(ctx context.Context, name string) (string, error) {
	var result struct {
		Results []reference `json:"results"`
	}
	endpoint := systemSearchURL + "?" + url.Values{"q": {name}}.Encode()
	if err := p.request(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
		return "", err
	}
	for _, system := range result.Results {
		if strings.EqualFold(system.Name, name) {
			return system.Name, nil
		}
	}
	return "", nil
}

func (p *Planner) request(ctx context.Context, method, endpoint string, payload io.Reader, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ED-Copilot Road to Riches")
	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("Не удалось связаться со Spansh.")
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusBadRequest, http.StatusNotFound:
		return errUnknownOrigin
	default:
		return fmt.Errorf("Ошибка Spansh (HTTP %d).", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes+1))
	if err != nil {
		return errors.New("Не удалось прочитать ответ Spansh.")
	}
	if len(raw) > maxPageBytes {
		return errors.New("Ответ Spansh слишком большой.")
	}
	if json.Unmarshal(raw, out) != nil {
		return errors.New("Spansh вернул неизвестный формат.")
	}
	return nil
}
