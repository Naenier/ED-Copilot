package pilot

import (
	v "github.com/Naenier/ED-Copilot/internal/value"
	"testing"
)

func TestNavigationMissionLifecycleAndFreshness(t *testing.T) {
	p := New()
	at := "2026-11-07T20:00:00Z"
	ctx := M{"commander": M{"system": "A"}}
	p.Apply(M{"event": "LoadGame", "timestamp": at, "Credits": 100, "FuelLevel": 8, "FuelCapacity": 16}, ctx)
	p.Apply(M{"event": "NavRoute", "timestamp": at, "Route": []M{{"StarSystem": "A"}, {"StarSystem": "B"}, {"StarSystem": "C"}}}, ctx)
	if v.Int(v.Obj(p.Snapshot()["navTarget"])["remainingJumps"]) != 2 {
		t.Fatal(p.Snapshot())
	}
	p.Apply(M{"event": "FSDJump", "timestamp": "2026-11-07T20:01:00Z", "StarSystem": "B", "JumpDist": 12.5}, ctx)
	if v.Obj(p.Snapshot()["navTarget"])["system"] != "C" {
		t.Fatal("route did not progress")
	}
	p.Apply(M{"event": "NavRouteClear", "timestamp": "2026-11-07T20:02:00Z"}, ctx)
	p.Apply(M{"event": "NavRoute", "timestamp": at, "Route": []M{{"StarSystem": "Old"}}}, ctx)
	if len(v.Rows(p.Snapshot()["navRoute"])) != 0 {
		t.Fatal("stale side route resurrected cleared path")
	}
	p.Apply(M{"event": "Status", "timestamp": "2026-11-07T19:00:00Z", "Balance": 1, "Fuel": M{"FuelMain": 1}}, ctx)
	if v.Num(p.Snapshot()["credits"]) != 100 {
		t.Fatal("stale balance replaced newer snapshot")
	}
	p.Apply(M{"event": "MissionAccepted", "timestamp": at, "MissionID": 1, "LocalisedName": "Доставить груз", "DestinationSystem": "B"}, ctx)
	p.Apply(M{"event": "MissionRedirected", "timestamp": at, "MissionID": 1, "NewDestinationSystem": "C"}, ctx)
	if v.Rows(p.Snapshot()["missions"])[0]["destinationSystem"] != "C" {
		t.Fatal("mission redirect lost")
	}
	p.Apply(M{"event": "MissionCompleted", "timestamp": at, "MissionID": 1}, ctx)
	if len(v.Rows(p.Snapshot()["missions"])) != 0 {
		t.Fatal("completed mission remained active")
	}
	p.Apply(M{"event": "Missions", "timestamp": at, "Active": []M{{"MissionID": 2, "Expires": 600}}}, ctx)
	if len(v.Rows(p.Snapshot()["missions"])) != 1 {
		t.Fatal("active mission snapshot missing")
	}
	p.Reset()
	if len(v.Rows(p.Snapshot()["missions"])) != 0 {
		t.Fatal("replay reset failed")
	}
}
func TestObjectives(t *testing.T) {
	if _, e := NormalizeObjective(M{}, nil); e == nil {
		t.Fatal("blank objective accepted")
	}
	a, e := NormalizeObjective(M{"title": "Новая колония", "kind": "colonization"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	b, e := NormalizeObjective(M{"title": "Снабжение"}, a)
	if e != nil || a["id"] != b["id"] {
		t.Fatal("editing changed identity")
	}
	if _, e = NormalizeObjective(M{"title": "x", "kind": "unknown"}, nil); e == nil {
		t.Fatal("unknown kind accepted")
	}
}
