package app

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestRichesJourneyFollowsShipJumpsAndMapping(t *testing.T) {
	previous, journalPath, profileDir := fixtureEngine(t, nil)
	previous.Close()
	client := &http.Client{Transport: economyRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "spansh.co.uk" || request.URL.Path != "/api/bodies/search" {
			t.Errorf("unexpected request: %s", request.URL)
		}
		const payload = `{"count":3,"reference":{"name":"Synthetic Origin","id64":18446744073709551615,"x":0,"y":0,"z":0},
		  "results":[
		    {"system_name":"Riches A","system_id64":10,"system_x":42,"system_y":0,"system_z":0,"body_id":1,"name":"Riches A 1","subtype":"Water world","distance_to_arrival":25,"estimated_scan_value":50,"estimated_mapping_value":300},
		    {"system_name":"Riches A","system_id64":10,"system_x":42,"system_y":0,"system_z":0,"body_id":2,"name":"Riches A 2","subtype":"Earth-like world","distance_to_arrival":40,"estimated_scan_value":100,"estimated_mapping_value":600},
		    {"system_name":"Riches B","system_id64":11,"system_x":60,"system_y":0,"system_z":0,"body_id":3,"name":"Riches B A 3","subtype":"Earth-like world","distance_to_arrival":125,"estimated_scan_value":100,"estimated_mapping_value":600}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload))}, nil
	})}
	engine, err := New(Options{
		DataDir: profileDir, JournalDir: filepath.Dir(journalPath), HTTPClient: client,
		ManualPolling: true, CallbackPort: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	if _, err := engine.Do("/api/riches/route", M{"jumpRangeLy": 25, "radiusLy": 100, "avoidThargoids": false, "loop": false, "minMappingValue": 0}); err != nil {
		t.Fatal(err)
	}
	waitFrontierCondition(t, func() bool { return !v.Bool(v.At(engine.RichesStatus(), "search", "running")) })
	if got := engine.PendingClipboard(); got["text"] != "Riches A" || got["kind"] != richesJob {
		t.Fatalf("first waypoint was not queued: %v, %v", got, engine.RichesStatus())
	}
	progress := func() M { return v.Obj(engine.RichesStatus()["progress"]) }
	if got := progress(); got["remainingJumps"] != 3 || got["totalMappingCredits"] != int64(1500) ||
		v.At(got, "nextTarget", "designation") != "1" {
		t.Fatal(got)
	}
	engine.clearRouteClipboard()
	initial := engine.PendingClipboard()
	if initial == nil {
		t.Fatal("carrier actions cleared the ship route clipboard")
	}
	start := time.Now().Add(2 * time.Second).UTC()
	event := func(kind string, seconds int, fields M) M {
		return testEvent(kind, v.Merge(M{"timestamp": v.ISO(start.Add(time.Duration(seconds) * time.Second))}, fields))
	}
	add := func(events ...M) {
		t.Helper()
		writeEvents(t, journalPath, true, events)
		engine.Reader.Poll()
	}
	jumpA := event("FSDJump", 3, M{"StarSystem": "Riches A", "SystemAddress": 10, "StarPos": []float64{42, 0, 0}})
	add(
		event("FSDJump", 1, M{"StarSystem": "Riches A", "SystemAddress": 10, "Taxi": true}),
		event("FSDJump", 2, M{"StarSystem": "Intermediate", "SystemAddress": 99, "StarPos": []float64{24, 0, 0}}),
	)
	if got := progress(); got["completedJumps"] != 1 || got["remainingJumps"] != 2 || engine.PendingClipboard()["id"] != initial["id"] {
		t.Fatalf("taxi or intermediate arrival advanced the waypoint: %v", got)
	}
	add(jumpA, jumpA)
	if got := progress(); got["completedJumps"] != 2 || got["nextSystem"] != "Riches B" ||
		v.At(got, "target", "name") != "Riches A 1" || engine.PendingClipboard()["text"] != "Riches B" {
		t.Fatalf("arrival did not advance the route: %v", got)
	}
	mark := M{"system": "Riches A", "body": "Riches A 1", "completed": true}
	if _, err := engine.Do("/api/riches/route/complete", mark); err != nil || v.At(progress(), "target", "name") != "Riches A 2" {
		t.Fatalf("manual completion did not advance the current body: %v", err)
	}
	data, err := engine.ExportPlan("riches")
	if err != nil || !strings.Contains(string(data), `"manualCompleted": true`) {
		t.Fatalf("route JSON omitted manual progress: %v", err)
	}
	dataCSV, err := engine.ExportCSV("riches")
	if err != nil || !strings.Contains(dataCSV, "Riches A 1;Water world;;25;50;300;2;true") {
		t.Fatalf("route CSV omitted target and progress: %v %q", err, dataCSV)
	}
	mark["completed"] = false
	if _, err := engine.Do("/api/riches/route/complete", mark); err != nil || v.At(progress(), "target", "name") != "Riches A 1" {
		t.Fatalf("unchecking a manual completion did not restore the body: %v", err)
	}
	add(event("SAAScanComplete", 4, M{"SystemAddress": 10, "BodyID": 1}))
	if got := progress(); v.At(got, "target", "name") != "Riches A 2" {
		t.Fatalf("mapping did not advance the current planet: %v", got)
	}
	add(event("FSDJump", 5, M{"StarSystem": "Riches B", "SystemAddress": 11, "StarPos": []float64{60, 0, 0}}))
	if got := progress(); v.Bool(got["finished"]) || got["remainingJumps"] != 0 ||
		v.At(got, "target", "name") != "Riches B A 3" || engine.PendingClipboard() != nil {
		t.Fatalf("the final system's planet should remain visible: %v", got)
	}
	add(event("SAAScanComplete", 6, M{"BodyName": "Riches B A 3", "SystemAddress": 11}))
	if got := progress(); !v.Bool(got["finished"]) || got["completedJumps"] != 3 {
		t.Fatal(got)
	}
	if err := engine.CloseWithSave(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(Options{
		DataDir: profileDir, JournalDir: filepath.Dir(journalPath), DisableNetwork: true, ManualPolling: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	if got := v.Obj(reopened.RichesStatus()["progress"]); got["completedJumps"] != 3 ||
		!v.Bool(got["finished"]) || reopened.PendingClipboard() != nil {
		t.Fatalf("replay duplicated progress or copied a historical waypoint: %v", got)
	}
}

func TestRichesRequestDefaultsAndExplicitOptions(t *testing.T) {
	defaulted, err := richesOptions(M{"jumpRangeLy": 30, "radiusLy": 100}, "Sol")
	if err != nil || defaulted.Origin != "Sol" || defaulted.MaxResults != 100 ||
		*defaulted.MinMappingValue != 1_000_000 || *defaulted.MaxDistanceLS != 10_000 ||
		!defaulted.UseMappingValue || !defaulted.AvoidThargoids || !defaulted.Loop {
		t.Fatalf("unexpected defaults: %+v %v", defaulted, err)
	}
	explicit, err := richesOptions(M{"origin": "Sol", "destination": "Brestla", "jumpRangeLy": 30, "radiusLy": 100,
		"maxResults": 3, "minMappingValue": 0, "maxDistanceLs": 0, "useMappingValue": false, "avoidThargoids": false, "loop": false}, "")
	if err != nil || explicit.Destination != "Brestla" || explicit.MaxResults != 3 ||
		*explicit.MinMappingValue != 0 || *explicit.MaxDistanceLS != 0 || explicit.UseMappingValue || explicit.AvoidThargoids || explicit.Loop {
		t.Fatalf("explicit choices were lost: %+v %v", explicit, err)
	}
}
