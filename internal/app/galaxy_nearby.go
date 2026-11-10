package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	nearbyFallbackPage  = 50
	nearbyFallbackBytes = 4 << 20
)

// The fallback uses the coordinates from the exact EDSM lookup. Its single
// bounded search page does not hydrate systems with additional requests.
func (e *Engine) galaxyNearbyFallback(name string, radius int) M {
	ctx, cancel := context.WithTimeout(e.ctx, 10*time.Second)
	defer cancel()
	raw, err := e.EDSM.System(ctx, name)
	if err != nil {
		return nil
	}
	coords := v.Obj(raw["coords"])
	x, xOK := v.Number(coords["x"])
	y, yOK := v.Number(coords["y"])
	z, zOK := v.Number(coords["z"])
	if !xOK || !yOK || !zOK {
		return nil
	}
	payload, err := json.Marshal(M{
		"filters":          M{"distance": M{"min": 0, "max": radius}},
		"sort":             []M{{"distance": M{"direction": "asc"}}},
		"size":             nearbyFallbackPage,
		"page":             0,
		"reference_coords": M{"x": x, "y": y, "z": z},
	})
	if err != nil {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://spansh.co.uk/api/systems/search", bytes.NewReader(payload))
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ED-Copilot/"+Version)
	client := http.Client{}
	if e.opts.HTTPClient != nil {
		client = *e.opts.HTTPClient
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > nearbyFallbackBytes {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, nearbyFallbackBytes+1))
	if err != nil || len(body) > nearbyFallbackBytes {
		return nil
	}
	var response M
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&response) != nil || v.Arr(response["results"]) == nil {
		return nil
	}
	rows := []M{}
	seen := map[string]bool{}
	for _, candidate := range v.Rows(response["results"]) {
		cx, cxOK := v.Number(candidate["x"])
		cy, cyOK := v.Number(candidate["y"])
		cz, czOK := v.Number(candidate["z"])
		address := galaxyID(candidate["id64"])
		if v.Str(candidate["name"]) == "" || address == "" || seen[address] || !cxOK || !cyOK || !czOK {
			continue
		}
		distance := math.Hypot(math.Hypot(cx-x, cy-y), cz-z)
		if distance > float64(radius) {
			continue
		}
		row := M{"name": candidate["name"], "address": address, "x": cx, "y": cy, "z": cz, "distance": distance}
		galaxyCopyText(row, candidate, "allegiance", "government", "security")
		galaxyText(row, "economy", candidate["primary_economy"])
		galaxyText(row, "faction", candidate["controlling_minor_faction"])
		galaxyText(row, "factionState", candidate["controlling_minor_faction_state"])
		galaxyNumber(row, "population", candidate["population"])
		galaxyBool(row, "permitRequired", candidate["needs_permit"])
		rows = append(rows, row)
		seen[address] = true
	}
	result := galaxyResult(rows, nearbyFallbackPage)
	if total, ok := v.Number(response["count"]); ok && total >= float64(len(rows)) {
		result["total"] = total
		result["truncated"] = total > float64(len(v.Arr(response["results"]))) || len(rows) > nearbyFallbackPage
	} else {
		result["truncated"] = len(v.Arr(response["results"])) >= nearbyFallbackPage
	}
	result["source"] = "Spansh"
	return result
}
