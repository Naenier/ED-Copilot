package app

import (
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Naenier/ED-Copilot/internal/edsm"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const galaxyNearbyLimit = 2000

// Galaxy is a read-only bridge. Network waits and sorting take place outside
// the journal and profile locks. API data does not become journal observations.
func (e *Engine) Galaxy(operation string, input M) M {
	name := strings.TrimSpace(v.Str(input["name"]))
	if name == "" || utf8.RuneCountInString(name) > 150 || strings.ContainsFunc(name, unicode.IsControl) {
		return M{"status": "invalid"}
	}
	if e.opts.DisableNetwork || e.EDSM == nil {
		return M{"status": "disabled"}
	}
	switch operation {
	case "galaxy.nearby":
		radius, ok := v.Number(input["radius"])
		if !ok || radius < 1 || radius > 100 || math.Trunc(radius) != radius {
			return M{"status": "invalid"}
		}
		raw, err := e.EDSM.Nearby(e.ctx, name, int(radius))
		if err != nil {
			failure := galaxyFailure(err)
			if failure["status"] == "unavailable" {
				if fallback := e.galaxyNearbyFallback(name, int(radius)); fallback != nil {
					return fallback
				}
			}
			return failure
		}
		rows := make([]M, 0, len(raw))
		seen := make(map[string]bool, len(raw))
		for _, system := range raw {
			row := galaxySystem(system)
			if row == nil || v.Str(system["id64"]) == "0" {
				continue
			}
			key := ""
			if address := v.Str(row["address"]); address != "" {
				key = "address:" + address
			} else if id := v.Str(row["id"]); id != "" {
				key = "edsm:" + id
			}
			if key != "" {
				if seen[key] {
					continue
				}
				seen[key] = true
			}
			rows = append(rows, row)
		}
		sort.SliceStable(rows, func(i, j int) bool {
			a, aKnown := v.Number(rows[i]["distance"])
			b, bKnown := v.Number(rows[j]["distance"])
			if aKnown != bKnown {
				return aKnown
			}
			if aKnown && a != b {
				return a < b
			}
			return galaxyLessName(rows[i], rows[j])
		})
		result := galaxyResult(rows, galaxyNearbyLimit)
		result["source"] = "EDSM"
		return result
	case "galaxy.system":
		return e.galaxySystemSection(name, v.Str(input["section"]))
	default:
		return M{"status": "invalid"}
	}
}

func (e *Engine) galaxySystemSection(name, section string) M {
	switch section {
	case "summary":
		raw, err := e.EDSM.System(e.ctx, name)
		if err != nil {
			return galaxyFailure(err)
		}
		if system := galaxySystem(raw); system != nil {
			return M{"status": "ok", "system": system}
		}
		return M{"status": "not-found"}
	case "bodies":
		raw, err := e.EDSM.Bodies(e.ctx, name)
		return galaxySection(raw, err, "bodies", galaxyBody, false, 500)
	case "stations":
		raw, err := e.EDSM.Stations(e.ctx, name)
		return galaxySection(raw, err, "stations", galaxyStation, false, 500)
	case "factions":
		raw, err := e.EDSM.Factions(e.ctx, name)
		return galaxySection(raw, err, "factions", galaxyFaction, true, 200)
	default:
		return M{"status": "invalid"}
	}
}

func galaxyFailure(err error) M {
	var limited *edsm.RateLimitError
	switch {
	case errors.Is(err, edsm.ErrNotFound):
		return M{"status": "not-found"}
	case errors.As(err, &limited):
		return M{"status": "rate-limited", "retryAt": limited.RetryAt.UTC().Format(time.RFC3339)}
	default:
		return M{"status": "unavailable"}
	}
}

func galaxySection(raw M, err error, key string, project func(M) M, byInfluence bool, limit int) M {
	if err != nil {
		return galaxyFailure(err)
	}
	rows := []M{}
	for _, item := range v.Rows(raw[key]) {
		if row := project(item); v.Str(row["name"]) != "" {
			rows = append(rows, row)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if byInfluence {
			a, b := v.Num(rows[i]["influence"]), v.Num(rows[j]["influence"])
			if a != b {
				return a > b
			}
		}
		return galaxyLessName(rows[i], rows[j])
	})
	return galaxyResult(rows, limit)
}

func galaxyResult(rows []M, limit int) M {
	return M{"status": "ok", "rows": rows[:min(limit, len(rows))], "total": len(rows), "truncated": len(rows) > limit}
}

func galaxyLessName(a, b M) bool {
	return strings.ToLower(v.Str(a["name"])) < strings.ToLower(v.Str(b["name"]))
}

func galaxySystem(raw M) M {
	name := v.Str(raw["name"])
	if name == "" {
		return nil
	}
	info, coords, star := v.Obj(raw["information"]), v.Obj(raw["coords"]), v.Obj(raw["primaryStar"])
	result := M{"name": name, "id": galaxyID(raw["id"]), "address": galaxyID(raw["id64"])}
	galaxyCopyText(result, info, "allegiance", "government", "security", "economy", "secondEconomy", "faction", "factionState")
	if v.Str(result["secondEconomy"]) == "" {
		galaxyText(result, "secondEconomy", info["economySecond"])
	}
	galaxyNumber(result, "population", info["population"])
	for _, key := range []string{"x", "y", "z"} {
		galaxyNumber(result, key, coords[key])
	}
	galaxyNumber(result, "distance", raw["distance"])
	galaxyBool(result, "permitRequired", raw["requirePermit"])
	galaxyText(result, "permitName", raw["permitName"])
	galaxyText(result, "starType", star["type"])
	galaxyText(result, "starName", star["name"])
	galaxyBool(result, "scoopable", star["isScoopable"])
	return result
}

func galaxyBody(raw M) M {
	result := M{}
	galaxyCopyText(result, raw, "name", "type", "subType")
	galaxyText(result, "atmosphere", raw["atmosphereType"])
	galaxyText(result, "terraforming", raw["terraformingState"])
	galaxyText(result, "volcanism", raw["volcanismType"])
	galaxyTime(result, "updatedAt", raw["updateTime"])
	galaxyNumber(result, "distance", raw["distanceToArrival"])
	galaxyNumber(result, "gravity", raw["gravity"])
	galaxyNumber(result, "temperature", raw["surfaceTemperature"])
	galaxyBool(result, "landable", raw["isLandable"])
	materials := []M{}
	for name, rawPercent := range v.Obj(raw["materials"]) {
		if percent, ok := v.Number(rawPercent); ok {
			materials = append(materials, M{"name": name, "percent": percent})
		}
	}
	sort.Slice(materials, func(i, j int) bool { return galaxyLessName(materials[i], materials[j]) })
	rings := []M{}
	for _, rawRing := range v.Rows(raw["rings"]) {
		ring := M{}
		galaxyCopyText(ring, rawRing, "name", "type")
		galaxyNumber(ring, "innerRadius", rawRing["innerRadius"])
		galaxyNumber(ring, "outerRadius", rawRing["outerRadius"])
		rings = append(rings, ring)
	}
	result["materials"], result["rings"] = materials, rings
	return result
}

func galaxyStation(raw M) M {
	result := M{}
	galaxyCopyText(result, raw, "name", "type", "economy", "allegiance", "government")
	galaxyText(result, "faction", v.Obj(raw["controllingFaction"])["name"])
	galaxyText(result, "body", v.Obj(raw["body"])["name"])
	galaxyNumber(result, "distance", raw["distanceToArrival"])
	galaxyBool(result, "market", raw["haveMarket"])
	galaxyBool(result, "shipyard", raw["haveShipyard"])
	galaxyBool(result, "outfitting", raw["haveOutfitting"])
	result["services"] = galaxyStrings(raw["otherServices"])
	galaxyTime(result, "updatedAt", v.Obj(raw["updateTime"])["information"])
	return result
}

func galaxyFaction(raw M) M {
	result := M{}
	galaxyCopyText(result, raw, "name", "allegiance", "government", "state")
	if fraction, ok := v.Number(raw["influence"]); ok && fraction >= 0 && fraction <= 1 {
		result["influence"] = fraction * 100
	}
	galaxyBool(result, "player", raw["isPlayer"])
	for _, key := range []string{"activeStates", "pendingStates", "recoveringStates"} {
		states := []string{}
		for _, item := range v.Rows(raw[key]) {
			if state := v.Str(item["state"]); state != "" {
				states = append(states, state)
			}
		}
		result[key] = states
	}
	galaxyTime(result, "updatedAt", raw["lastUpdate"])
	return result
}

func galaxyCopyText(target, input M, keys ...string) {
	for _, key := range keys {
		galaxyText(target, key, input[key])
	}
}

func galaxyText(target M, key string, raw any) {
	if text, ok := raw.(string); ok && text != "" {
		target[key] = text
	}
}

func galaxyNumber(target M, key string, raw any) {
	if number, ok := v.Number(raw); ok {
		target[key] = number
	}
}

func galaxyBool(target M, key string, raw any) {
	if value, ok := raw.(bool); ok {
		target[key] = value
	}
}

func galaxyID(raw any) string {
	text := v.Str(raw)
	id, err := strconv.ParseUint(text, 10, 64)
	if err != nil || id == 0 {
		return ""
	}
	return strconv.FormatUint(id, 10)
}

func galaxyStrings(raw any) []string {
	result := []string{}
	for _, value := range v.Arr(raw) {
		if text, ok := value.(string); ok && text != "" {
			result = append(result, text)
		}
	}
	return result
}

func galaxyTime(target M, key string, raw any) {
	if epoch, ok := v.Number(raw); ok && epoch > 0 && epoch <= float64(time.Now().Add(24*time.Hour).Unix()) {
		target[key] = time.Unix(int64(epoch), 0).UTC().Format(time.RFC3339)
	} else if stamp := v.Str(raw); stamp != "" {
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
			if at, err := time.Parse(layout, stamp); err == nil {
				target[key] = at.UTC().Format(time.RFC3339)
				break
			}
		}
	}
}
