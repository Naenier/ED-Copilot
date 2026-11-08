package colonization

import (
	"bytes"
	"encoding/json"
	"github.com/Naenier/ED-Copilot/internal/value"
	"os"
	"testing"
	"time"
)

// The fixture is a sanitized 1.8 planner scenario, captured before changing the
// runtime language. It verifies calculations against the preceding application.
func TestLegacyBehaviorParity(t *testing.T) {
	raw, err := os.ReadFile("testdata/legacy-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture M
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err = decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	p := New(value.Obj(fixture["saved"]), WithClock(func() time.Time { return testNow }))
	for _, row := range rows(fixture["events"]) {
		p.Apply(value.Obj(row["event"]), value.Obj(row["context"]))
	}
	got := []M{}
	for _, project := range rows(p.Snapshot(value.Obj(fixture["context"]))["projects"]) {
		r := M{}
		for _, key := range []string{"id", "status", "cargoCapacity", "totalRequired", "totalMissing", "unknownBills", "pointsProjected", "nextTask"} {
			r[key] = project[key]
		}
		bs := []M{}
		for _, b := range rows(project["buildings"]) {
			row := M{}
			for _, key := range []string{"id", "status", "pointCost", "pointReward", "requirements"} {
				row[key] = b[key]
			}
			bs = append(bs, row)
		}
		ids := []string{}
		for _, b := range rows(project["supplyQueue"]) {
			ids = append(ids, str(b["id"]))
		}
		r["buildings"], r["supplyOrder"] = bs, ids
		got = append(got, r)
	}
	// Go omits a zero millisecond fraction; JavaScript writes .000Z.
	// Carrier supply is now explicit opt-in and requires a verified observation.
	// Keep the historic fixture intact; only its old implicit warehouse field
	// changes. The new accounting contract has dedicated carrier_stock tests.
	for _, project := range rows(fixture["want"]) {
		for _, building := range rows(project["buildings"]) {
			for _, requirement := range rows(building["requirements"]) {
				requirement["carrierAvailable"] = nil
			}
		}
	}
	actual, _ := json.MarshalIndent(normalizeTimes(value.Clone(got)), "", "  ")
	expected, _ := json.MarshalIndent(normalizeTimes(fixture["want"]), "", "  ")
	if !bytes.Equal(actual, expected) {
		t.Fatalf("Go planner differs from previous behavior:\nGOT %s\nWANT %s", actual, expected)
	}
}

func normalizeTimes(v any) any {
	switch x := v.(type) {
	case string:
		if t := value.Time(x); !t.IsZero() {
			return value.ISO(t)
		}
	case M:
		for key, item := range x {
			x[key] = normalizeTimes(item)
		}
	case []any:
		for i, item := range x {
			x[i] = normalizeTimes(item)
		}
	}
	return v
}

func TestMalformedInventoriesStayUnknown(t *testing.T) {
	for _, raw := range []any{nil, M{"invalid": true}, "not inventory"} {
		if _, known := AvailableCargo(M{"vehicle": "ship", "cargoForSync": raw}); known {
			t.Fatal("malformed cargo declared known")
		}
		if stock := availableWarehouse(M{"owned": true, "warehouse": M{"items": raw, "complete": true}}); stock != nil {
			t.Fatal("malformed complete warehouse declared known")
		}
	}
}
