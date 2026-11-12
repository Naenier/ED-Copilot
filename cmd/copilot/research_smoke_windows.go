//go:build windows

package main

import (
	"github.com/Naenier/ED-Copilot/internal/app"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func researchSmoke(engine *app.Engine) (v.M, bool) {
	checks := v.M{}
	exp, err := engine.Do("/api/exploration/update", v.M{"action": "save-expedition", "name": "Synthetic expedition"})
	checks["expedition"] = err == nil && len(v.Rows(exp["expeditions"])) > 0
	route, err := engine.Do("/api/ship-route", v.M{"action": "import", "name": "Synthetic research route", "waypoints": "Synthetic waypoint"})
	checks["shipRoute"] = err == nil && v.At(route, "route", "nextStop", "system") == "Synthetic waypoint" && engine.PendingClipboard() == nil
	build, err := engine.Do("/api/ship-builds", nil)
	checks["shipOutfitting"] = err == nil && build["targets"] != nil
	itinerary, err := engine.Do("/api/engineering/itinerary", nil)
	checks["engineeringItinerary"] = err == nil && itinerary["ownerKnown"] == true
	checks["surfaceRadar"] = engine.SnapshotForUI("surface-radar")["surfaceRadar"] != nil
	notices, err := engine.Do("/api/notifications", nil)
	checks["notificationsOptIn"] = err == nil && v.At(v.Obj(v.Clone(notices)), "preferences", "enabled") == false
	screens, err := engine.Do("/api/second-screen", nil)
	checks["secondScreenOptIn"] = err == nil && screens["running"] == false
	for _, passed := range checks {
		if passed != true {
			return checks, false
		}
	}
	return checks, true
}
