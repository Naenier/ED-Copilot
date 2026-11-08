package app

import (
	"reflect"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestSyntheticExobiologySeedsJournalProgressWithoutChangingCommander(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	defer e.Close()
	commander := v.Clone(e.SnapshotForUI("overview")["commander"])
	if err := e.SeedDemoExobiology(); err != nil {
		t.Fatal(err)
	}
	state := e.SnapshotForUI("exobiology")
	ex := v.Obj(state["exobiology"])
	if !reflect.DeepEqual(commander, v.Clone(state["commander"])) {
		t.Fatal("synthetic domain fixture changed the commander's actual context")
	}
	_, knownDistance := v.Number(v.At(ex, "tracker", "nearestSampleDistanceM"))
	if v.Int(v.At(ex, "tracker", "sampleCount")) != 2 || len(v.Rows(v.At(ex, "tracker", "anchors"))) != 2 || v.At(ex, "tracker", "positionKnown") != true || !knownDistance {
		t.Fatal("two live samples and an actual measured separation are required", ex["tracker"])
	}
	if len(v.Rows(ex["pending"])) != 1 || len(v.Rows(ex["sales"])) != 1 || len(v.Rows(ex["bodies"])) < 2 || len(v.Rows(ex["surfaceMarks"])) < 2 {
		t.Fatal("fixture needs unsold data, a confirmed sale and distinct survey bodies", ex["summary"])
	}
	if ex["selectedBodyKey"] != "42424000024:2" || v.At(ex, "selectedBody", "firstFootfall") != "walked" {
		t.Fatal("selected body is not the live sampling target")
	}
}

func TestSyntheticExobiologyRefusesOnlineProfileBeforeAnyObservation(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	defer e.Close()
	before := e.Exobiology.Snapshot()
	e.opts.DisableNetwork = false
	if e.SeedDemoExobiology() == nil {
		t.Fatal("online profile accepted a synthetic journal")
	}
	if !reflect.DeepEqual(before, e.Exobiology.Snapshot()) {
		t.Fatal("rejected fixture changed biological observations")
	}
}
