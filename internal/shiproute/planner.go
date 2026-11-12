package shiproute

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const providerBase = "https://spansh.co.uk/api"
const providerLimit = 6 << 20

var jobPattern = regexp.MustCompile(`^[A-Fa-f0-9-]{16,64}$`)

type Planner struct {
	client *http.Client
	poll   time.Duration
}

func NewPlanner(client *http.Client) *Planner {
	c := &http.Client{Timeout: 20 * time.Second}
	if client != nil {
		*c = *client
	}
	c.Jar = nil
	c.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &Planner{client: c, poll: 1500 * time.Millisecond}
}

func (p *Planner) Plan(ctx context.Context, o Options) (Route, error) {
	if err := validateOptions(o); err != nil {
		return Route{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	path := "/route"
	form := url.Values{"from": {o.Origin}, "to": {o.Destination}, "range": {number(o.JumpRangeLY)}, "efficiency": {number(o.Efficiency)}, "supercharge_multiplier": {"4"}}
	if o.Mode == "exact" {
		a, err := p.resolve(ctx, o.Origin)
		if err != nil {
			return Route{}, err
		}
		b, err := p.resolve(ctx, o.Destination)
		if err != nil {
			return Route{}, err
		}
		path, form = "/generic/route", exactForm(a, b, o)
	}
	response, err := p.request(ctx, http.MethodPost, path, form)
	if err != nil {
		return Route{}, err
	}
	if response["status"] == "ok" {
		return parseRoute(response, o)
	}
	job := v.Str(response["job"])
	if response["status"] != "queued" || !jobPattern.MatchString(job) {
		return Route{}, errors.New("Spansh не принял параметры маршрута.")
	}
	timer := time.NewTicker(p.poll)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return Route{}, ctx.Err()
		case <-timer.C:
			response, err = p.request(ctx, http.MethodGet, "/results/"+job, nil)
			if err != nil {
				return Route{}, err
			}
			switch response["status"] {
			case "ok":
				return parseRoute(response, o)
			case "queued", "running", "processing":
			default:
				return Route{}, errors.New("Spansh не нашёл подходящий маршрут.")
			}
		}
	}
}

func number(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }

func exactForm(a, b string, o Options) url.Values {
	params := o.ShipBuild
	boost := "0"
	if params.UseSupercharge {
		boost = "1"
	}
	return url.Values{
		"source": {a}, "destination": {b}, "is_supercharged": {"0"}, "use_supercharge": {boost},
		"use_injections": {"0"}, "use_injections_when_required": {"0"}, "exclude_secondary": {"0"},
		"refuel_every_scoopable": {"0"}, "fuel_power": {number(params.FuelPower)}, "fuel_multiplier": {number(params.FuelMultiplier)},
		"optimal_mass": {number(params.OptimalMass)}, "base_mass": {number(params.BaseMass)}, "tank_size": {number(params.TankSize)},
		"internal_tank_size": {number(params.InternalTankSize)}, "reserve_size": {number(params.ReserveSize)},
		"max_fuel_per_jump": {number(params.MaxFuelPerJump)}, "range_boost": {number(params.RangeBoost)},
		"cargo": {number(o.Cargo)}, "max_time": {"120"}, "algorithm": {"optimistic"}, "supercharge_multiplier": {"4"}, "injection_multiplier": {"1"},
		// The API requires a build metadata field even with fully specified
		// manual parameters. No commander data or external build is sent.
		"ship_build": {"{}"},
	}
}

func (p *Planner) resolve(ctx context.Context, name string) (string, error) {
	rows, err := p.request(ctx, http.MethodGet, "/search/systems?"+url.Values{"q": {name}}.Encode(), nil)
	if err != nil {
		return "", err
	}
	for _, row := range v.Rows(rows["results"]) {
		if !strings.EqualFold(v.Str(row["name"]), name) {
			continue
		}
		id := v.Str(row["id64"])
		if _, err := strconv.ParseUint(id, 10, 64); err == nil {
			return id, nil
		}
	}
	return "", errors.New("Уточните название системы, которую не нашёл Spansh.")
}

func (p *Planner) request(ctx context.Context, method, path string, form url.Values) (M, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, providerBase+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ED Copilot")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errors.New("Spansh ограничил частоту запросов. Повторите позже.")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("Spansh временно не предоставил маршрут.")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, providerLimit+1))
	if err != nil || len(raw) > providerLimit {
		return nil, errors.New("Ответ Spansh превышает допустимый размер.")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var result M
	if dec.Decode(&result) != nil {
		return nil, errors.New("Не удалось прочитать ответ Spansh.")
	}
	return result, nil
}

func parseRoute(response M, o Options) (Route, error) {
	raw := v.Obj(response["result"])
	rows := v.Rows(raw["system_jumps"])
	if o.Mode == "exact" {
		rows = v.Rows(v.First(raw["jumps"], raw["system_jumps"]))
	}
	if len(rows) == 0 || len(rows) > maxStops {
		return Route{}, errors.New("Spansh не вернул список путевых точек.")
	}
	stops := make([]Stop, 0, len(rows)+1)
	for _, row := range rows {
		system := v.Text(row["system"], row["name"])
		if !validText(system, 160) {
			return Route{}, errors.New("Ответ Spansh содержит неверное название системы.")
		}
		stop := Stop{System: system, SystemAddress: v.Text(row["id64"], row["system_address"]),
			DistanceLY: floatPointer(v.First(row["distance_jumped"], row["distance"]), 1e6),
			StarClass:  v.Text(row["star_class"], row["starClass"]), Jumps: max(0, v.Int(row["jumps"])),
			Coordinates: coords(v.First(row["coordinates"], []any{row["x"], row["y"], row["z"]}))}
		if v.Bool(row["neutron_star"]) || v.Bool(row["has_neutron"]) && stop.StarClass == "" {
			stop.StarClass = "N"
		}
		if o.Mode == "exact" {
			if stop.Jumps == 0 {
				stop.Jumps = 1
			}
			stop.Refuel = v.Bool(row["must_refuel"])
			if stop.StarClass == "" && v.Bool(row["is_scoopable"]) {
				stop.StarClass = "scoopable"
			}
			stop.FuelLeft = floatPointer(row["fuel_in_tank"], 2000)
			stop.FuelUsed = floatPointer(row["fuel_used"], 100)
			if v.Bool(row["must_supercharge"]) {
				stop.Boost = "neutron"
			}
		}
		stops = append(stops, stop)
	}
	if !strings.EqualFold(stops[len(stops)-1].System, o.Destination) {
		return Route{}, errors.New("Ответ Spansh не достиг системы назначения.")
	}
	if !strings.EqualFold(stops[0].System, o.Origin) {
		stops = append([]Stop{{System: o.Origin}}, stops...)
	}
	stops[0].DistanceLY = nil
	stops[0].Jumps = 0
	if o.JumpRangeLY > 0 && (o.Mode == "neutron" || o.ShipBuild != nil && o.ShipBuild.UseSupercharge) {
		for i := 1; i < len(stops); i++ {
			if next := stops[i]; next.Jumps == 1 && next.DistanceLY != nil && *next.DistanceLY > o.JumpRangeLY && StarKind(stops[i-1].StarClass) == "neutron" {
				stops[i-1].Boost = "neutron"
			}
		}
	}
	r := Route{Name: v.Text(o.Name, o.Origin+" → "+o.Destination), Origin: o.Origin, Destination: o.Destination, Mode: o.Mode,
		JumpRangeLY: o.JumpRangeLY, MaxFuelPerJump: o.MaxFuelPerJump, AutoCopy: o.AutoCopy, Stops: stops}
	if o.Mode == "exact" && o.ShipBuild != nil {
		r.MaxFuelPerJump = o.ShipBuild.MaxFuelPerJump
	}
	if !validRoute(r) {
		return Route{}, errors.New("Ответ Spansh содержит неверные данные маршрута.")
	}
	return r, nil
}
