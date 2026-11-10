package politics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/freshness"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 4 << 20

type requestFailure struct {
	reason      freshness.Reason
	description string
	retryAt     time.Time
	failedAt    time.Time
}

func (f requestFailure) Error() string { return f.description }

func failureDetails(err error, now time.Time) requestFailure {
	var f requestFailure
	if !errors.As(err, &f) {
		f = requestFailure{reason: freshness.Unavailable, description: err.Error()}
	}
	f.failedAt = now
	if floor := now.Add(time.Minute); f.retryAt.Before(floor) {
		f.retryAt = floor
	}
	if ceiling := now.Add(time.Hour); f.retryAt.After(ceiling) {
		f.retryAt = ceiling
	}
	return f
}

func failedStatus(kind string, params M, failure requestFailure) M {
	status := M{"running": false, "kind": kind, "parameters": params, "error": failure.description, "reason": string(failure.reason), "failedAt": v.ISO(failure.failedAt)}
	if !failure.retryAt.IsZero() {
		status["retryAt"] = v.ISO(failure.retryAt)
	}
	return status
}

func incomplete(message string) requestFailure {
	return requestFailure{reason: freshness.Incomplete, description: message}
}

func (s *Service) request(ctx context.Context, method, address string, body M) (M, error) {
	var input io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		input = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, address, input)
	if err != nil {
		return nil, errors.New("Invalid public endpoint")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ED-Copilot/politics (public observations)")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := s.opts.HTTPClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, requestFailure{reason: freshness.Offline, description: "Public data request failed"}
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests {
		retry := response.Header.Get("Retry-After")
		at, _ := http.ParseTime(retry)
		if seconds, err := strconv.ParseInt(retry, 10, 64); err == nil && seconds > 0 {
			at = s.opts.Now().Add(time.Duration(min(seconds, 3600)) * time.Second)
		}
		return nil, requestFailure{reason: freshness.RateLimited, description: "Public data HTTP 429", retryAt: at}
	}
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("Public data HTTP %d", response.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, errors.New("Could not read public data")
	}
	if len(b) > maxResponseBytes {
		return nil, errors.New("Public response exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	var result M
	if err = decoder.Decode(&result); err != nil || result == nil {
		return nil, incomplete("Invalid public JSON response")
	}
	var tail any
	if decoder.Decode(&tail) != io.EOF {
		return nil, incomplete("Invalid trailing public JSON")
	}
	return result, nil
}

func (s *Service) fetch(ctx context.Context, kind, name, power, state, language string, page int) (M, error) {
	base := strings.TrimRight(s.opts.SpanshURL, "/")
	if kind == "goals" {
		separator := "?"
		if strings.Contains(s.opts.GoalsURL, "?") {
			separator = "&"
		}
		raw, err := s.request(ctx, "GET", s.opts.GoalsURL+separator+"lang="+language, nil)
		if err != nil {
			return nil, err
		}
		items, ok := raw["activeInitiatives"]
		if !ok || v.Arr(items) == nil {
			return nil, incomplete("Community-goal response has no activeInitiatives list")
		}
		entries := []M{}
		for _, r := range boundedRows(items, 100) {
			if clean(r["id"], 100) == "" {
				continue
			}
			expiry := clean(r["expiry"], 100)
			if parsed, err := time.Parse("2006-01-02 15:04:05", expiry); err == nil {
				expiry = v.ISO(parsed)
			} else {
				expiry = date(expiry)
			}
			entries = append(entries, M{"id": clean(r["id"], 100), "title": clean(r["title"], 500), "system": clean(r["starsystem_name"], 200), "station": clean(r["market_name"], 200), "expiry": expiry, "activityType": clean(r["activityType"], 100), "objective": goalText(r["objective"], 4000), "commodities": goalText(r["target_commodity_list"], 4000), "currentTotal": numericString(r["qty"]), "target": numericString(r["target_qty"]), "description": goalText(v.First(r["bulletin"], r["news"]), 24000)})
		}
		goals := meta("frontier-public", "", s.now(), "https://www.elitedangerous.com/community/goals")
		goals["entries"] = entries
		goals["partial"] = len(entries) < len(v.Arr(items))
		goals["language"] = language
		return M{"goals": goals}, nil
	}
	if kind == "system" {
		names, err := s.request(ctx, "GET", base+"/systems/field_values/system_names?q="+url.QueryEscape(name), nil)
		if err != nil {
			return nil, err
		}
		if v.Arr(names["min_max"]) == nil {
			return nil, incomplete("Public system name index is missing")
		}
		address := ""
		for _, candidate := range boundedRows(names["min_max"], 30) {
			if key(v.Str(candidate["name"])) == key(name) {
				address = clean(candidate["id64"], 100)
				break
			}
		}
		if address == "" {
			return nil, requestFailure{reason: freshness.NoMatch, description: "Exact system is not present in the public name index"}
		}
		for _, r := range address {
			if r < '0' || r > '9' {
				return nil, errors.New("Invalid public system address")
			}
		}
		raw, err := s.request(ctx, "GET", base+"/system/"+address, nil)
		if err != nil {
			return nil, err
		}
		record := v.Obj(raw["record"])
		if key(v.Str(record["name"])) != key(name) {
			return nil, incomplete("Public system response does not match the requested name")
		}
		now := s.now()
		normalized := NormalizePublicSystem(record, now)
		if normalized == nil {
			return nil, incomplete("Public system record is missing")
		}
		normalized["detailsFetchedAt"] = now
		return M{"system": normalized}, nil
	}
	field := "power_state"
	if kind == "thargoid" {
		field = "thargoid_war_state"
	}
	index := M{}
	if kind == "powerplay" {
		s.mu.RLock()
		index = v.Copy(s.powerIndex)
		s.mu.RUnlock()
		if !v.Bool(index["known"]) || v.Time(index["fetchedAt"]).IsZero() || s.opts.Now().Sub(v.Time(index["fetchedAt"])) >= refreshInterval {
			index = M{}
		}
	}
	if len(index) == 0 {
		indexRaw, err := s.request(ctx, "GET", base+"/systems/field_values/"+field, nil)
		if err != nil {
			return nil, err
		}
		if v.Arr(indexRaw["values"]) == nil {
			return nil, incomplete("Public state index is missing")
		}
		index = meta("spansh", "", s.now(), "https://spansh.co.uk/systems")
		index["states"] = indexRows(indexRaw)
		if kind == "powerplay" {
			powers, err := s.request(ctx, "GET", base+"/systems/field_values/controlling_power", nil)
			if err != nil {
				return nil, err
			}
			if v.Arr(powers["values"]) == nil {
				return nil, incomplete("Public controlling power index is missing")
			}
			index["powers"] = indexRows(powers)
			index["fetchedAt"] = s.now()
		}
	}
	filters := M{}
	if kind == "powerplay" && power != "" {
		found := false
		for _, row := range v.Rows(index["powers"]) {
			if row["name"] == power {
				found = true
				break
			}
		}
		if !found {
			return M{"index": index}, errors.New("Power is not present in the public index")
		}
		filters["controlling_power"] = M{"value": []string{power}}
	}
	values := []string{}
	if state != "" {
		values = []string{state}
	} else if kind == "thargoid" {
		for _, row := range v.Rows(index["states"]) {
			name := v.Str(row["name"])
			if name != "None" {
				values = append(values, name)
			}
		}
	} else {
		// An unfiltered systems query includes ordinary, unexplored systems.
		// Keep the automatic first page relevant to Powerplay.
		values = []string{"Stronghold", "Fortified", "Exploited", "Contested"}
	}
	if kind == "thargoid" && state == "None" {
		return nil, errors.New("Choose an affected Thargoid state, not None")
	}
	search := meta("spansh", "", s.now(), "https://spansh.co.uk/systems")
	search["kind"], search["power"], search["state"], search["page"], search["limit"], search["entries"], search["states"] = kind, power, state, page, 10, []M{}, index["states"]
	if kind == "powerplay" {
		search["scope"] = "controlled"
	}
	if kind == "thargoid" && len(values) == 0 {
		// A successful state-index response can establish no indexed affected state,
		// not that the galaxy contains no Thargoid activity.
		search["total"], search["indexHasAffectedStates"] = 0, false
		return M{"search": search}, nil
	}
	if len(values) > 0 {
		filters[field] = M{"value": values}
	}
	request := M{"filters": filters, "sort": []M{{"name": M{"direction": "asc"}}}, "size": 10, "page": page}
	raw, err := s.request(ctx, "POST", base+"/systems/search", request)
	if err != nil {
		return M{"index": index}, err
	}
	items, ok := raw["results"]
	if !ok || v.Arr(items) == nil {
		return M{"index": index}, incomplete("Public system search results are missing")
	}
	entries := []M{}
	now := s.now()
	for _, r := range boundedRows(items, 10) {
		if normalized := NormalizePublicSystem(r, now); normalized != nil {
			entries = append(entries, normalized)
		}
	}
	search["entries"], search["total"], search["indexHasAffectedStates"] = entries, number(raw["count"]), true
	search["fetchedAt"] = now
	search["partial"] = len(entries) < len(v.Arr(items)) || search["total"] == nil
	return M{"index": index, "search": search}, nil
}

func numericString(raw any) any {
	return number(raw)
}
func indexRows(raw M) []M {
	entries := []M{}
	counts := v.Obj(raw["min_max"])
	for _, name := range states(raw["values"]) {
		entries = append(entries, M{"name": name, "indexedSystems": number(counts[name])})
	}
	sort.Slice(entries, func(i, j int) bool { return v.Str(entries[i]["name"]) < v.Str(entries[j]["name"]) })
	return entries
}
