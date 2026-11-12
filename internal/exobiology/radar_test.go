package exobiology

import (
	"math"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestRadarProjectionAcrossLongitudeSeamAndNearPole(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		lat, lon, targetLat, targetLon float64
		heading, wantBearing           float64
	}{
		{"antimeridian", 0, 179.99, 0, -179.99, 90, 90},
		{"pole", 89.99, 0, 89.99, 180, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			point := radarProjection(tc.lat, tc.lon, tc.targetLat, tc.targetLon, 1e6, tc.heading)
			if math.Abs(v.Num(point["distanceM"])-349) > 1 || point["bearing"] != tc.wantBearing ||
				point["x"] != float64(0) || v.Num(point["y"]) >= 0 {
				t.Fatalf("wrong heading or distance: %+v", point)
			}
		})
	}
}

func TestSurfaceRadarSeparatesFreshFixesAndOwnedEvidence(t *testing.T) {
	s, ctx := fixture()
	ctx["side"] = true
	now := time.Now().UTC()
	status := ev("Status", M{"BodyName": "Synthetic Exo A 1", "Latitude": 0, "Longitude": 0,
		"PlanetRadius": 1e6, "Heading": 90, "Flags": 1 << 21, "timestamp": now.Add(-time.Minute).Format(time.RFC3339Nano)})
	s.Apply(status, ctx)
	if radar := s.SnapshotSurfaceRadar(); radar["available"] != true || radar["stale"] != true || len(v.Rows(radar["trail"])) != 0 {
		t.Fatalf("old Status file produced live guidance or a trail: %+v", radar)
	}
	status["timestamp"] = now.Format(time.RFC3339Nano)
	s.Apply(status, ctx)
	b := s.current
	s.tracker = M{"bodyKey": b, "runKey": "current", "phase": "tracking", "minSampleDistanceM": 500}
	s.marks = []M{
		{"id": "confirmed", "label": "Confirmed sample", "kind": "sample", "runKey": "current", "bodyKey": b, "owner": s.commander, "latitude": 0, "longitude": .01},
		{"id": "estimated", "label": "Estimated contact", "kind": "codex", "locationBasis": "crossing", "uncertaintyM": 30, "bodyKey": b, "owner": s.commander, "latitude": 0, "longitude": .02},
		{"id": "other-owner", "kind": "waypoint", "bodyKey": b, "owner": "other", "latitude": 0, "longitude": .03},
		{"id": "other-body", "kind": "waypoint", "bodyKey": "other", "owner": s.commander, "latitude": 0, "longitude": .04},
	}
	if err := s.Update(M{"action": "radar-target", "id": "estimated", "bodyKey": b}); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(M{"action": "radar-target", "id": "other-owner", "bodyKey": b}); err == nil {
		t.Fatal("another commander's mark must be unselectable")
	}
	radar := s.SnapshotSurfaceRadar()
	points := v.Rows(radar["points"])
	if radar["stale"] != false || len(v.Rows(radar["trail"])) != 1 || len(points) != 2 || points[0]["id"] != "estimated" ||
		v.Num(points[0]["uncertaintyM"]) != 30 || v.Num(points[1]["exclusionM"]) != 500 {
		t.Fatalf("radar lost or mixed evidence: %+v", radar)
	}
	status["Flags"] = 0
	s.Apply(status, ctx)
	if s.SnapshotSurfaceRadar()["available"] != false {
		t.Fatal("Status without HasLatLong cannot retain surface guidance")
	}
}

func TestSurfaceTrackBoundRestoreAndClear(t *testing.T) {
	s, _ := fixture()
	now := time.Now().UTC().Add(-time.Hour)
	body := s.current
	for i := 0; i < maxRadarTrail+10; i++ {
		s.recordRadarFix(M{"bodyKey": body, "latitude": 0, "longitude": float64(i) * .001,
			"planetRadiusM": 1e6, "at": now.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano)}, M{"vehicle": "srv"})
	}
	if err := s.Update(M{"action": "radar-scale", "scaleM": 1000}); err != nil {
		t.Fatal(err)
	}
	restored := New(s.ExportSettings())
	restored.commander, restored.current = s.commander, body
	i := restored.trackIndex(s.commander, body)
	if i < 0 || len(restored.radarTracks[i].Points) != maxRadarTrail || restored.trackIndex("other", body) != -1 ||
		restored.radarPrefs[s.commander].Scale != 1000 {
		t.Fatal("saved surface track lost ownership, limit, or preferences")
	}
	if err := restored.Update(M{"action": "radar-clear-track", "bodyKey": "other"}); err == nil || restored.trackIndex(s.commander, body) == -1 {
		t.Fatal("clearing another body must leave the current track intact")
	}
	if err := restored.Update(M{"action": "radar-clear-track", "bodyKey": body}); err != nil || restored.trackIndex(s.commander, body) != -1 {
		t.Fatal("current track was not cleared")
	}
}
