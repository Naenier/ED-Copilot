package mining

import (
	"reflect"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestScopedSnapshotsOmitWorldAndOnlyEvaluateExplicitTargets(t *testing.T) {
	s := New(nil)
	context := M{"statsSince": "2026-11-08T12:00:00Z", "commander": M{"system": "Synthetic", "coordinates": []float64{0, 0, 0}}, "settings": M{"method": "all", "minTier": "medium"}, "resources": trading.Resources()}
	scopedApply(s, M{"event": "Scan", "BodyName": "Synthetic 1", "BodyID": 1, "Landable": true, "Materials": []M{{"Name": "polonium", "Percent": 1}}}, context)
	full, small := s.Snapshot(context), s.SnapshotForUI(context, false)
	if full["world"] == nil || len(v.Rows(full["targets"])) == 0 {
		t.Fatal("fixture has no observed world/targets")
	}
	if small["world"] != nil || small["targets"] != nil {
		t.Fatal("unrequested world or targets were evaluated")
	}
	for _, key := range []string{"current", "history", "cargo", "preflight", "plan", "prospect", "surface", "stats"} {
		if !reflect.DeepEqual(full[key], small[key]) {
			t.Fatal("scoped mining changed", key)
		}
	}
	targets := s.SnapshotForUI(context, true)
	if !reflect.DeepEqual(full["targets"], targets["targets"]) || targets["world"] != nil {
		t.Fatal("explicit target view lost target parity or returned world")
	}
}

func TestHUDTargetsAreBoundedCachedAndInvalidatedByRelevantObservations(t *testing.T) {
	s := New(nil)
	context := M{"commander": M{"system": "Synthetic", "coordinates": []float64{0, 0, 0}}, "settings": M{"method": "all", "minTier": "medium"}, "resources": trading.Resources()}
	for i, name := range []string{"Synthetic A", "Synthetic B", "Synthetic C"} {
		cmd := M{"system": name, "coordinates": []float64{float64(i), 0, 0}}
		scopedApply(s, M{"event": "Location", "StarSystem": name, "SystemAddress": i + 1, "StarPos": cmd["coordinates"]}, M{"commander": cmd})
		scopedApply(s, M{"event": "Scan", "BodyName": name + " 1", "BodyID": 1, "Landable": true, "Materials": []M{{"Name": "polonium", "Percent": 1}}}, M{"commander": cmd})
	}
	full := BuildTargets(s.WorldSnapshot(), s.Signals, trading.Resources(), v.Obj(context["commander"]), v.Obj(context["settings"]))
	got := s.HUDTargets(context, true)
	if len(got) != 2 || len(full) < 2 {
		t.Fatal("fixture has too few surface targets")
	}
	for _, rows := range [][]M{got, full[:2]} {
		for _, row := range rows {
			if len(v.Rows(row["siteFinds"])) == 0 {
				delete(row, "siteFinds")
			}
		}
	}
	if !reflect.DeepEqual(v.Clone(got), v.Clone(full[:2])) {
		t.Fatal("HUD sites changed ranking", got, full)
	}
	key := s.hudTargetsKey
	scopedApply(s, M{"event": "Status", "Fuel": M{"FuelMain": 5}}, context)
	if s.hudTargetsKey != key {
		t.Fatal("unrelated telemetry invalidated target cache")
	}
	got[0]["system"] = "external mutation"
	if s.HUDTargets(context, true)[0]["system"] == "external mutation" {
		t.Fatal("HUD target result aliases cache")
	}
	scopedApply(s, M{"event": "Scan", "BodyName": "Synthetic New", "BodyID": 2, "Landable": true, "Materials": []M{{"Name": "polonium", "Percent": 2}}}, context)
	if s.hudTargetsKey != "" {
		t.Fatal("new scan failed to invalidate targets")
	}
	s.HUDTargets(context, true)
	if s.hudTargetsKey == key {
		t.Fatal("HUD reused pre-scan cache")
	}
}

func scopedApply(s *Service, event, context M) {
	event["timestamp"] = "2026-11-08T12:00:00Z"
	s.Apply(event, context)
}
