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
	maxPages        = 4
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
	ReferenceSystem string           `json:"reference_system"`
	Filters         map[string]any   `json:"filters"`
	Sort            []map[string]any `json:"sort"`
	Size            int              `json:"size"`
	Page            int              `json:"page"`
}

type searchResult struct {
	Count     int        `json:"count"`
	Reference *reference `json:"reference"`
	Results   []bodyRow  `json:"results"`
}

type reference struct {
	Name    string      `json:"name"`
	Address json.Number `json:"id64"`
	X       *float64    `json:"x"`
	Y       *float64    `json:"y"`
	Z       *float64    `json:"z"`
}

type bodyRow struct {
	System       string      `json:"system_name"`
	Address      json.Number `json:"system_id64"`
	X            *float64    `json:"system_x"`
	Y            *float64    `json:"system_y"`
	Z            *float64    `json:"system_z"`
	ID           *int        `json:"body_id"`
	Name         string      `json:"name"`
	Subtype      string      `json:"subtype"`
	DistanceLS   *float64    `json:"distance_to_arrival"`
	ScanValue    *int64      `json:"estimated_scan_value"`
	MappingValue *int64      `json:"estimated_mapping_value"`
}

// The native Road to Riches router omits many systems near inhabited space.
// Spansh's body search finds the selected planet types directly and includes
// the local body IDs used by the game journal.
func (p *Planner) Plan(ctx context.Context, opts Options) (Route, error) {
	if err := Validate(opts); err != nil {
		return Route{}, err
	}
	query := bodySearch{
		ReferenceSystem: strings.Join(strings.Fields(opts.Origin), " "),
		Filters: map[string]any{
			"subtype":  map[string][]string{"value": {"Earth-like world", "Water world"}},
			"distance": map[string]float64{"min": 0, "max": opts.RadiusLY},
		},
		Sort: []map[string]any{
			{"distance": map[string]string{"direction": "asc"}},
			{"name": map[string]string{"direction": "asc"}},
		},
		Size: pageSize,
	}
	route := Route{JumpRangeLY: opts.JumpRangeLY, RadiusLY: opts.RadiusLY}
	candidates := make(map[string]*Stop)
	seenBodies := make(map[string]struct{})
	var origin [3]float64
	for page := 0; page < maxPages; page++ {
		if err := ctx.Err(); err != nil {
			return Route{}, err
		}
		query.Page = page
		result, err := p.search(ctx, query)
		if page == 0 && errors.Is(err, errUnknownOrigin) {
			name, lookupErr := p.canonicalSystem(ctx, query.ReferenceSystem)
			if lookupErr != nil {
				return Route{}, lookupErr
			}
			if name != "" && name != query.ReferenceSystem {
				query.ReferenceSystem = name
				result, err = p.search(ctx, query)
			}
		}
		if err != nil {
			return Route{}, err
		}
		if page == 0 {
			ref := result.Reference
			if ref == nil || ref.X == nil || ref.Y == nil || ref.Z == nil || strings.TrimSpace(ref.Name) == "" {
				return Route{}, errors.New("Spansh не вернул координаты стартовой системы.")
			}
			route.Origin = strings.TrimSpace(ref.Name)
			route.OriginSystemAddress = id64(ref.Address)
			origin = [3]float64{*ref.X, *ref.Y, *ref.Z}
			query.ReferenceSystem = route.Origin
		}
		for _, row := range result.Results {
			appendBody(candidates, seenBodies, row, origin, opts.RadiusLY)
		}
		more := len(result.Results) == pageSize && (page+1)*pageSize < result.Count
		if len(candidates) > MaxStops || !more || page == maxPages-1 {
			route.Truncated = len(candidates) > MaxStops || more
			break
		}
	}
	route.Stops = orderStops(route.Origin, origin, candidates, opts.JumpRangeLY)
	return route, nil
}

func appendBody(candidates map[string]*Stop, seen map[string]struct{}, row bodyRow, origin [3]float64, radius float64) {
	system, name, kind := strings.TrimSpace(row.System), strings.TrimSpace(row.Name), bodyType(row.Subtype)
	if system == "" || name == "" || kind == "" || row.X == nil || row.Y == nil || row.Z == nil {
		return
	}
	coords := [3]float64{*row.X, *row.Y, *row.Z}
	if distance(origin, coords) > radius+1e-6 {
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
		DistanceLS:     nonnegative(row.DistanceLS),
		ScanCredits:    nonnegative(row.ScanValue),
		MappingCredits: nonnegative(row.MappingValue),
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
