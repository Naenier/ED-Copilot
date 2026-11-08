package mining

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func hudRawNeededKeys(context M) []string {
	set := map[string]bool{}
	for _, material := range v.Rows(v.At(context, "engineering", "materials")) {
		missing, known := v.Number(material["missing"])
		if material["category"] == "raw" && v.Num(material["needed"]) > 0 && (!known || missing > 0) {
			if key := v.Key(material["key"]); key != "" {
				set[key] = true
			}
		}
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func hudCurrentSystem(candidate, commander M) bool {
	current := strings.TrimSpace(v.Str(commander["system"]))
	return current != "" && strings.EqualFold(current, strings.TrimSpace(v.Str(candidate["system"])))
}

func hudTargetNearby(candidate, commander M, radius float64) bool {
	if hudCurrentSystem(candidate, commander) {
		return true
	}
	distance, known := v.Number(candidate["distanceLy"])
	return known && distance >= 0 && distance <= radius
}

func hudSurfaceSite(candidate M) bool {
	return v.Num(candidate["miningSites"]) > 0 || len(v.Rows(candidate["observedResources"])) > 0 || len(v.Rows(candidate["siteFinds"])) > 0 || has(candidate["methods"], "surface-planet")
}

func hudRawSite(candidate M, needed map[string]bool) bool {
	materials := v.Rows(candidate["materials"])
	if !v.Bool(candidate["landable"]) || len(materials) == 0 {
		return false
	}
	if len(needed) == 0 {
		return true
	}
	for _, material := range materials {
		if needed[v.Key(material["key"])] {
			return true
		}
	}
	return false
}

func (s *Service) invalidateHUDTargets() {
	s.hudWorldVersion++
	s.hudTargetsKey = ""
	s.hudSurfaceTargets, s.hudRawTargets = nil, nil
}

// HUDTargets returns just the two sites displayed by the overlay. It evaluates
// surface candidates without cloning rings/the whole world and caches the
// small result until observations or relevant navigation/filter inputs change.
// As with the other Service methods, the app holds its domain lock.
func (s *Service) HUDTargets(context M, raw bool) []M {
	resources := v.Rows(context["resources"])
	commander, settings := v.Obj(context["commander"]), v.Obj(context["settings"])
	neededKeys := hudRawNeededKeys(context)
	encoded, _ := json.Marshal(M{"world": s.hudWorldVersion, "system": commander["system"], "coordinates": commander["coordinates"], "settings": settings, "resources": resources, "rawNeededKeys": neededKeys})
	key := string(encoded)
	if key != s.hudTargetsKey {
		world := make([]M, 0, len(s.World))
		for _, system := range s.World {
			bodies := []M{}
			for _, body := range bodiesRows(system["bodies"]) {
				// Ring evaluation is never used by the surface/raw HUD panel.
				next := M{}
				for field, value := range body {
					if field != "rings" {
						next[field] = value
					}
				}
				next["rings"] = []M{}
				bodies = append(bodies, next)
			}
			world = append(world, M{"key": system["key"], "address": system["address"], "name": system["name"], "coordinates": system["coordinates"], "timestamp": system["timestamp"], "bodies": bodies})
		}
		s.hudSurfaceTargets, s.hudRawTargets = []M{}, []M{}
		radius := float64(50)
		if configured, valid := v.Number(settings["radiusLy"]); valid && configured > 0 {
			radius = math.Min(100, configured)
		}
		threshold := max(Tiers["medium"], Tiers[v.Str(settings["minTier"])])
		needed := map[string]bool{}
		for _, key := range neededKeys {
			needed[key] = true
		}
		// The activity, rather than a saved laser/core search filter, determines
		// which planet candidates the two HUD panels may present.
		targetSettings := v.Merge(M{}, settings, M{"method": "all"})
		candidates := BuildTargets(world, s.Signals, resources, commander, targetSettings)
		sort.SliceStable(candidates, func(i, j int) bool {
			a, b := candidates[i], candidates[j]
			currentA, currentB := hudCurrentSystem(a, commander), hudCurrentSystem(b, commander)
			if currentA != currentB {
				return currentA
			}
			distance := func(candidate M) float64 {
				if value, known := v.Number(candidate["distanceLy"]); known && value >= 0 {
					return value
				}
				return math.Inf(1)
			}
			if distance(a) != distance(b) {
				return distance(a) < distance(b)
			}
			if v.Num(a["score"]) != v.Num(b["score"]) {
				return v.Num(a["score"]) > v.Num(b["score"])
			}
			return v.Str(a["id"]) < v.Str(b["id"])
		})
		for _, candidate := range candidates {
			if candidate["kind"] != "surface" || !hudTargetNearby(candidate, commander, radius) || Tiers[v.Str(candidate["tier"])] < threshold {
				continue
			}
			if hudSurfaceSite(candidate) && len(s.hudSurfaceTargets) < 2 {
				s.hudSurfaceTargets = append(s.hudSurfaceTargets, v.Copy(candidate))
			}
			if hudRawSite(candidate, needed) && len(s.hudRawTargets) < 2 {
				s.hudRawTargets = append(s.hudRawTargets, v.Copy(candidate))
			}
			if len(s.hudSurfaceTargets) == 2 && len(s.hudRawTargets) == 2 {
				break
			}
		}
		s.hudTargetsKey = key
	}
	if raw {
		return v.Rows(v.Clone(s.hudRawTargets))
	}
	return v.Rows(v.Clone(s.hudSurfaceTargets))
}
