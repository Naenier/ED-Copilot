package engineering

import (
	"reflect"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/inara"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestInventorySnapshotPreservesInaraObservationsWithoutUICatalogue(t *testing.T) {
	p := New(nil)
	stamp := v.ISO(time.Now().Add(-time.Second))
	p.Apply(M{"event": "Materials", "timestamp": stamp, "Raw": []M{{"Name": "carbon", "Count": 12}}, "Manufactured": []M{{"Name": "chemicalprocessors", "Count": 3}}, "Encoded": []M{{"Name": "dataminedwake", "Count": 2}}}, nil)
	engineer := v.Rows(catalog["engineers"])[0]
	p.Apply(M{"event": "EngineerProgress", "timestamp": stamp, "Engineer": engineer["english"], "Rank": 5, "Progress": "Unlocked"}, nil)
	full, small := p.Snapshot(nil), p.InventorySnapshot()
	context := M{"gameVersion": "4.4.2", "live": true}
	want := inara.BuildSnapshotEvents(M{"engineering": full}, context, time.Now())
	got := inara.BuildSnapshotEvents(M{"engineering": small}, context, time.Now())
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sync observations differ:\ngot=%v\nwant=%v", got, want)
	}
	if len(got) < 2 {
		t.Fatal("fixture did not exercise materials and engineer rank", got)
	}
	if small["catalog"] != nil || small["trader"] != nil || small["items"] != nil {
		t.Fatal("UI catalogue escaped into synchronization state")
	}
	for _, row := range v.Rows(small["engineers"]) {
		if row["capabilities"] != nil {
			t.Fatal("engineer capability expansion was performed")
		}
	}
	v.Rows(small["materials"])[0]["owned"] = 999
	if reflect.DeepEqual(small, p.InventorySnapshot()) {
		t.Fatal("inventory snapshot aliases planner state")
	}
}
