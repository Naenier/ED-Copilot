package engineering

import (
	"encoding/json"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"os"
	"testing"
)

func same(t *testing.T, a, b any) {
	t.Helper()
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	if string(x) != string(y) {
		t.Fatalf("got %s; want %s", x, y)
	}
}
func TestParityAndPlannerActions(t *testing.T) {
	b, e := os.ReadFile("testdata/parity.json")
	if e != nil {
		t.Fatal(e)
	}
	var fixture M
	if e = json.Unmarshal(b, &fixture); e != nil {
		t.Fatal(e)
	}
	p := New(nil)
	input := v.Copy(v.Obj(fixture["input"]))
	input["action"] = "add"
	if e = p.Update(input); e != nil {
		t.Fatal(e)
	}
	for _, event := range v.Rows(fixture["events"]) {
		p.Apply(event, nil)
	}
	options := v.Merge(v.Copy(v.Obj(fixture["options"])), M{"action": "configure"})
	if e = p.Update(options); e != nil {
		t.Fatal(e)
	}
	got := p.Snapshot(nil)
	want := v.Obj(fixture["expected"])
	for key, expected := range v.Obj(want["shopping"]) {
		same(t, v.Obj(got["shopping"])[key], expected)
	}
	for _, expected := range v.Rows(want["materials"]) {
		found := false
		for _, row := range v.Rows(got["materials"]) {
			if row["key"] == expected["key"] {
				for k, x := range expected {
					same(t, row[k], x)
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatal("missing material", expected["key"])
		}
	}
	offers := map[string]M{}
	for _, row := range v.Rows(v.Obj(got["trader"])["offers"]) {
		offers[v.Str(row["fromKey"])] = row
	}
	for _, expected := range v.Rows(want["offers"]) {
		row := offers[v.Str(expected["fromKey"])]
		if row == nil {
			t.Fatal("missing offer", expected["fromKey"])
		}
		for k, x := range expected {
			same(t, row[k], x)
		}
	}
	saved := p.ExportSettings()
	p2 := New(saved)
	if len(v.Rows(p2.Snapshot(nil)["items"])) != 1 {
		t.Fatal("plan lost on restore")
	}
	if v.Obj(p2.Snapshot(nil)["shopping"])["missingUnits"] != nil {
		t.Fatal("restored observations must remain unknown until material snapshot")
	}
	id := v.Rows(saved["items"])[0]["id"]
	if e = p.Update(M{"action": "update", "id": id, "quantity": 3}); e != nil {
		t.Fatal(e)
	}
	if e = p.Update(M{"action": "import", "items": []any{M{"moduleKey": "bad"}}}); e == nil {
		t.Fatal("invalid import accepted")
	}
	if len(v.Rows(p.ExportSettings()["items"])) != 1 {
		t.Fatal("failed import destroyed plan")
	}
	if e = p.Update(M{"action": "remove", "id": id}); e != nil {
		t.Fatal(e)
	}
	if len(v.Rows(p.Snapshot(nil)["items"])) != 0 {
		t.Fatal("remove failed")
	}
	if e = p.Update(M{"action": "no-such-action"}); e == nil {
		t.Fatal("unknown action accepted")
	}
}
func TestStockUnknownOverspendAndEngineerOverride(t *testing.T) {
	p := New(nil)
	key := v.Str(materialRows[0]["key"])
	symbol := materialRows[0]["symbol"]
	group := v.Str(materialRows[0]["category"])
	field := map[string]string{"raw": "Raw", "manufactured": "Manufactured", "encoded": "Encoded"}[group]
	p.Apply(M{"event": "Materials", field: []M{{"Name": symbol, "Count": 2}}}, nil)
	p.Apply(M{"event": "MaterialDiscarded", "Name": symbol, "Count": 3}, nil)
	for _, r := range v.Rows(p.Snapshot(nil)["materials"]) {
		if r["key"] == key && r["owned"] != nil {
			t.Fatal("negative accounting incorrectly became authoritative zero")
		}
	}
	p.Apply(M{"event": "Materials", field: []M{{"Name": symbol, "Count": 1}}}, nil)
	for _, r := range v.Rows(p.Snapshot(nil)["materials"]) {
		if r["key"] == key && v.Int(r["owned"]) != 1 {
			t.Fatal("full snapshot must restore certainty")
		}
	}
	engineer := v.Rows(catalog["engineers"])[0]
	id := engineer["key"]
	p.Apply(M{"event": "EngineerProgress", "Engineer": engineer["english"], "Rank": 5, "Progress": "Unlocked"}, nil)
	if e := p.Update(M{"action": "engineer", "key": id, "unlocked": false}); e != nil {
		t.Fatal(e)
	}
	for _, r := range v.Rows(p.Snapshot(nil)["engineers"]) {
		if r["key"] == id && r["status"] != "locked" {
			t.Fatal("manual engineer override ignored")
		}
	}
	_ = p.Update(M{"action": "engineer", "key": id, "unlocked": nil})
	for _, r := range v.Rows(p.Snapshot(nil)["engineers"]) {
		if r["key"] == id && r["status"] != "unlocked" {
			t.Fatal("reset did not return to journal")
		}
	}
}
func TestTradeWholeBatchesCapacityAndCrossCategories(t *testing.T) {
	a := M{"key": "a", "tradeable": true, "category": "raw", "family": "one", "grade": 1, "capacity": 300}
	b := M{"key": "b", "tradeable": true, "category": "raw", "family": "two", "grade": 3, "capacity": 200}
	same(t, TradeRatio(a, b), M{"cost": 216, "gain": 1})
	same(t, TradeRatio(b, a), M{"cost": 2, "gain": 3})
	offer := TradeOffer(b, a, 7, 6, 298, 0)
	if v.Int(offer["received"]) != 9 || offer["storageFits"] != false || v.Int(offer["maxGain"]) != 0 {
		t.Fatal(offer)
	}
	b["category"] = "encoded"
	if TradeRatio(a, b) != nil {
		t.Fatal("cross-category trade accepted")
	}
}
