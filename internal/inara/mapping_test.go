package inara

import (
	"bytes"
	"encoding/json"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestJournalReferenceParity(t *testing.T) {
	raw, err := os.ReadFile("testdata/journal-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Now   string `json:"now"`
		Cases []struct {
			Name     string  `json:"name"`
			Event    v.M     `json:"event"`
			Context  v.M     `json:"context"`
			Expected []Event `json:"expected"`
		} `json:"cases"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err = decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	for _, test := range fixture.Cases {
		t.Run(test.Name, func(t *testing.T) {
			got := JournalEvents(test.Event, test.Context, v.Time(fixture.Now))
			left, right := canonical(got), canonical(test.Expected)
			var a, b any
			json.Unmarshal(left, &a)
			json.Unmarshal(right, &b)
			if got == nil && len(test.Expected) == 0 {
				return
			}
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("mapping differs\ngot %s\nwant %s", left, right)
			}
		})
	}
}
func TestSnapshotCompletenessAndObservationTimes(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	at := v.ISO(now.Add(-time.Minute))
	state := v.M{"commander": v.M{"name": "Pilot", "fid": "F1", "system": "Sol", "vehicle": "ship", "cargoForSyncAt": at, "cargoForSync": []v.M{{"Name": "gold", "Count": 10, "Stolen": 2}}, "locationUpdatedAt": at}, "pilot": v.M{"credits": 10000, "creditsAt": at}, "engineering": v.M{"materials": []v.M{{"key": "iron", "owned": 20}}, "shopping": v.M{"knownCategories": v.M{"raw": true, "manufactured": false, "encoded": false}, "updatedAt": v.M{"raw": at, "manufactured": at, "encoded": at}}}}
	events := BuildSnapshotEvents(state, v.M{"live": true}, now)
	names := map[string]Event{}
	for _, event := range events {
		names[event.Name] = event
		if event.Timestamp != at {
			t.Fatal("observation rewritten to click time")
		}
	}
	if _, ok := names["setCommanderInventoryMaterials"]; ok {
		t.Fatal("partial material inventory exported")
	}
	if _, ok := names["setCommanderCredits"]; ok {
		t.Fatal("unknown loan forgiven")
	}
	if _, ok := names["setCommanderInventoryCargo"]; !ok {
		t.Fatal("complete cargo omitted")
	}
	v.Obj(state["pilot"])["loan"] = 0
	v.Obj(v.At(state, "engineering", "shopping"))["knownCategories"] = v.M{"raw": true, "manufactured": true, "encoded": true}
	events = BuildSnapshotEvents(state, v.M{"live": true}, now)
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.Name] = true
	}
	if !seen["setCommanderInventoryMaterials"] || !seen["setCommanderCredits"] {
		t.Fatal("complete authoritative snapshots omitted", seen)
	}
	v.Obj(state["commander"])["vehicle"] = "srv"
	events = BuildSnapshotEvents(state, v.M{"live": true}, now)
	for _, event := range events {
		if event.Name == "setCommanderInventoryCargo" {
			t.Fatal("SRV cargo published as ship inventory")
		}
	}
}
func TestInventoryValidationAndStableEventKeys(t *testing.T) {
	if _, ok := inventory([]v.M{{"Name": "gold", "Count": 2, "Stolen": 3}}); ok {
		t.Fatal("inconsistent stolen cargo accepted")
	}
	if _, ok := inventory([]v.M{{"Name": "gold", "Count": 1.5}}); ok {
		t.Fatal("fractional tonnes accepted")
	}
	if _, ok := inventory([]v.M{{"Name": "gold", "Count": "2"}}); ok {
		t.Fatal("string count accepted")
	}
	event := Event{"setCommanderCredits", "2026-11-07T12:00:00Z", v.M{"commanderLoan": 0, "commanderCredits": 100}}
	other := Event{event.Name, event.Timestamp, v.M{"commanderCredits": 100, "commanderLoan": 0}}
	if EventKey(event, "F1") != EventKey(other, "F1") || EventKey(event, "F1") == EventKey(event, "F2") {
		t.Fatal("event identity unstable")
	}
	if SnapshotKey(event) == "" || SnapshotKey(Event{Name: "addCommanderTravelDock"}) != "" {
		t.Fatal("history/snapshot coalescing")
	}
}
