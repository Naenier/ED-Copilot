package mining

import (
	"encoding/json"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

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
	encoded, _ := json.Marshal(M{"world": s.hudWorldVersion, "coordinates": v.At(context, "commander", "coordinates"), "settings": context["settings"], "resources": resources})
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
		for _, candidate := range BuildTargets(world, s.Signals, resources, v.Obj(context["commander"]), v.Obj(context["settings"])) {
			if candidate["kind"] != "surface" {
				continue
			}
			if len(s.hudSurfaceTargets) < 2 {
				s.hudSurfaceTargets = append(s.hudSurfaceTargets, v.Copy(candidate))
			}
			if len(v.Rows(candidate["materials"])) > 0 && len(s.hudRawTargets) < 2 {
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
