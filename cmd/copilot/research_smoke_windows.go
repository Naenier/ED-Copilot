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
	shipPlan, err := engine.Do("/api/engineering/current-ship", nil)
	shipEngineeringOK := err == nil && v.Bool(shipPlan["loadoutKnown"])
	for _, status := range []string{"complete", "inProgress", "available", "locked"} {
		shipEngineeringOK = shipEngineeringOK && v.Int(v.At(shipPlan, "summary", status)) > 0
	}
	checks["currentShipEngineering"] = shipEngineeringOK
	checks["currentShipSavedPlan"] = smokeCurrentShipPlan(engine, shipPlan)
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

func smokeCurrentShipPlan(engine *app.Engine, view v.M) bool {
	const planPath = "/api/engineering/current-ship/plan"
	request := v.M{
		"expectedPlanWorkspace": v.At(engine.SnapshotForUI("engineering-roadmap"), "planWorkspace", "key"),
		"expectedShipID":        view["shipID"],
	}
	for _, target := range []v.M{
		{"slot": "FrameShiftDrive", "item": "int_hyperdrive_overcharge_size6_class5", "blueprintKey": "FSD_LongRange", "grade": 5, "experimentalKey": "special_fsd_cooled"},
		{"slot": "MainEngines", "item": "int_engine_size7_class5", "blueprintKey": "Engine_Dirty", "grade": 3, "experimentalKey": ""},
	} {
		if _, err := engine.Do(planPath, v.Merge(v.Copy(request), target, v.M{"action": "upsert"})); err != nil {
			return false
		}
	}
	view, err := engine.Do("/api/engineering/current-ship", nil)
	plan := v.Obj(view["plan"])
	return err == nil && len(v.Rows(plan["goals"])) == 2 && len(v.Rows(plan["materials"])) > 0 &&
		len(v.Rows(v.At(plan, "itinerary", "stops"))) == 1
}
