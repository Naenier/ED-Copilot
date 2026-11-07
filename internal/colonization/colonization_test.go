package colonization

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/value"
)

var testNow = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

func active(p *Planner) M { return value.Obj(p.Snapshot(nil)["activeProject"]) }
func action(t *testing.T, p *Planner, m M) {
	t.Helper()
	if e := p.Update(m); e != nil {
		t.Fatal(e)
	}
}
func colony(t *testing.T, p *Planner, name string, capacity int, market int64) M {
	t.Helper()
	action(t, p, M{"action": "create-project", "targetSystem": name, "status": "building"})
	id := active(p)["id"]
	action(t, p, M{"action": "configure", "projectId": id, "cargoCapacity": capacity})
	for i, typ := range []string{"coriolis", "hub-refinery"} {
		m := M{"action": "add-building", "projectId": id, "typeId": typ, "name": fmt.Sprintf("%s stage %d", name, i)}
		if i == 0 {
			m["marketId"] = market
		}
		action(t, p, m)
		bs := rows(active(p)["buildings"])
		key := "steel"
		if i == 1 {
			key = "aluminium"
		}
		action(t, p, M{"action": "set-resource-plan", "projectId": id, "buildingId": bs[len(bs)-1]["id"], "resources": []M{{"commodity": key, "required": capacity}}, "sourceNote": "Synthetic fixture"})
	}
	return active(p)
}
func observed(p *Planner, event M, offset int, system string) {
	event["timestamp"] = value.ISO(testNow)
	p.Apply(event, M{"file": "Journal.synthetic.log", "offset": offset, "commander": M{"system": system, "systemAddress": 123, "station": "Synthetic depot", "vehicle": "ship"}})
}
func byID(p *Planner, id any) M {
	for _, r := range rows(p.Snapshot(nil)["projects"]) {
		if str(r["id"]) == str(id) {
			return r
		}
	}
	return nil
}
func requirement(p *Planner, colonyID any, index int) M {
	return rows(rows(byID(p, colonyID)["buildings"])[index]["requirements"])[0]
}

func TestThreeColoniesIndependent(t *testing.T) {
	p := New(M{"cargoCapacity": 50})
	a := colony(t, p, "Alpha", 100, 101)
	b := colony(t, p, "Beta", 200, 202)
	c := colony(t, p, "Gamma", 300, 303)
	ab, bb := rows(a["buildings"]), rows(b["buildings"])
	action(t, p, M{"action": "record-delivery", "projectId": a["id"], "buildingId": ab[0]["id"], "commodity": "steel", "count": 20})
	for _, tc := range []struct {
		id      any
		missing float64
	}{{a["id"], 180}, {b["id"], 400}, {c["id"], 600}} {
		if num(byID(p, tc.id)["totalMissing"]) != tc.missing {
			t.Fatalf("wrong total for %v", tc.id)
		}
	}
	action(t, p, M{"action": "move-supply-building", "projectId": a["id"], "id": ab[1]["id"], "direction": "up"})
	action(t, p, M{"action": "move-building", "projectId": b["id"], "id": bb[1]["id"], "direction": "up"})
	if str(rows(byID(p, a["id"])["buildings"])[0]["id"]) != str(ab[0]["id"]) || str(rows(byID(p, a["id"])["supplyQueue"])[0]["id"]) != str(ab[1]["id"]) {
		t.Fatal("supply/build order not independent")
	}
	if str(rows(byID(p, b["id"])["buildings"])[0]["id"]) != str(bb[1]["id"]) {
		t.Fatal("B order did not change")
	}
	for _, m := range []M{{"action": "record-delivery", "projectId": b["id"], "buildingId": ab[0]["id"], "commodity": "steel", "count": 2}, {"action": "link-depot", "projectId": b["id"], "buildingId": bb[1]["id"], "marketId": 101}, {"action": "create-project", "targetSystem": "alpha"}} {
		before, _ := json.Marshal(p.ExportSettings())
		if p.Update(m) == nil {
			t.Fatal("cross-colony/duplicate action accepted")
		}
		after, _ := json.Marshal(p.ExportSettings())
		if !bytes.Equal(before, after) {
			t.Fatal("failed update partially changed state")
		}
	}
	observed(p, M{"event": "ColonisationConstructionDepot", "MarketID": 101, "ResourcesRequired": []M{{"Name": "$steel_name;", "RequiredAmount": 500, "ProvidedAmount": 50}}}, 1, "Alpha")
	observed(p, M{"event": "ColonisationContribution", "MarketID": 101, "Contributions": []M{{"Name": "$steel_name;", "Amount": 10}}}, 2, "Alpha")
	if num(requirement(p, a["id"], 0)["missing"]) != 440 || num(byID(p, b["id"])["totalMissing"]) != 400 || len(rows(byID(p, c["id"])["journalDeliveries"])) != 0 {
		t.Fatal("journal supply leaked into another colony")
	}
	action(t, p, M{"action": "select-project", "id": a["id"]})
	if num(p.Snapshot(nil)["cargoCapacity"]) != 100 || str(value.Obj(active(p)["nextTask"])["buildingId"]) != str(ab[1]["id"]) {
		t.Fatal("selected colony did not drive overlay task")
	}
	saved := p.ExportSettings()
	restored := New(saved)
	if str(restored.Snapshot(nil)["selectedProjectId"]) != str(a["id"]) || len(rows(restored.Snapshot(nil)["projects"])) != 3 || num(restored.Snapshot(nil)["cargoCapacity"]) != 100 || str(rows(active(restored)["supplyQueue"])[0]["id"]) != str(ab[1]["id"]) {
		t.Fatal("multi-colony persistence failed")
	}
	rows(saved["projects"])[0]["name"] = "External mutation"
	if str(active(p)["name"]) == "External mutation" {
		t.Fatal("export aliases planner")
	}
}
func TestJournalDedupeAndCargo(t *testing.T) {
	p := New(nil, WithClock(func() time.Time { return testNow }))
	c := colony(t, p, "Target", 100, 123)
	b := rows(c["buildings"])[0]
	action(t, p, M{"action": "record-delivery", "projectId": c["id"], "buildingId": b["id"], "commodity": "steel", "count": 90})
	observed(p, M{"event": "ColonisationConstructionDepot", "MarketID": 123, "ResourcesRequired": []M{{"Name": "$steel_name;", "RequiredAmount": 100, "ProvidedAmount": 20}}}, 10, "Target")
	for _, offset := range []int{11, 11, 12} {
		observed(p, M{"event": "ColonisationContribution", "MarketID": 123, "Contributions": []M{{"Name": "$steel_name;", "Amount": 5}}}, offset, "Target")
	}
	r := requirement(p, c["id"], 0)
	if num(r["delivered"]) != 30 || num(r["manualDelivered"]) != 90 {
		t.Fatalf("double counted: %#v", r)
	}
	observed(p, M{"event": "ColonisationConstructionDepot", "MarketID": 123, "ResourcesRequired": []M{{"Name": "$steel_name;", "RequiredAmount": 100, "ProvidedAmount": 30}}}, 13, "Target")
	if num(requirement(p, c["id"], 0)["delivered"]) != 30 {
		t.Fatal("snapshot double counted contribution")
	}
	s := p.Snapshot(M{"commander": M{"vehicle": "ship", "cargoForSync": []M{{"Name": "steel", "Count": 10, "Stolen": 3}, {"Name": "steel", "Count": 50, "MissionID": 99}}}, "carrier": M{"owned": true, "warehouse": M{"items": []M{{"key": "aluminium", "count": 4}}, "complete": false}}})
	r = rows(rows(value.Obj(s["activeProject"])["buildings"])[0]["requirements"])[0]
	if num(r["cargoAvailable"]) != 7 || r["carrierAvailable"] != nil {
		t.Fatal("cargo legality or partial warehouse lost")
	}
	for _, m := range []M{{"vehicle": "srv", "cargoForSync": []M{}}, {"vehicle": "ship"}} {
		if _, known := AvailableCargo(m); known {
			t.Fatal("unknown/SRV cargo treated as freight")
		}
	}
	p.Reset()
	if len(rows(p.Snapshot(nil)["depots"])) != 0 || len(rows(p.Snapshot(nil)["projects"])) != 1 {
		t.Fatal("reset failed")
	}
	observed(p, M{"event": "LoadGame", "FID": "A"}, 20, "Target")
	observed(p, M{"event": "ColonisationConstructionDepot", "MarketID": 123, "ConstructionComplete": true, "ResourcesRequired": []M{}}, 21, "Target")
	observed(p, M{"event": "LoadGame", "FID": "B"}, 22, "Target")
	if len(rows(p.Snapshot(nil)["depots"])) != 0 {
		t.Fatal("commander change retained observations")
	}
}
func TestLifecycleDependenciesAndExports(t *testing.T) {
	p := New(nil, WithClock(func() time.Time { return testNow }))
	c := colony(t, p, "Alpha; quoted", 100, 123)
	bs := rows(c["buildings"])
	action(t, p, M{"action": "update-building", "projectId": c["id"], "id": bs[0]["id"], "isPrimary": true, "dependsOn": []any{bs[1]["id"]}})
	if p.Update(M{"action": "update-building", "projectId": c["id"], "id": bs[1]["id"], "dependsOn": []any{bs[0]["id"]}}) == nil {
		t.Fatal("cycle accepted")
	}
	if num(rows(active(p)["buildings"])[0]["pointCost"]) != 0 {
		t.Fatal("primary charged points")
	}
	if len(rows(active(p)["buildAdvice"])) < 3 {
		t.Fatal("development advice missing")
	}
	observed(p, M{"event": "ColonisationSystemClaim", "StarSystem": "Alpha; quoted", "SystemAddress": 123}, 1, "Origin")
	observed(p, M{"event": "ColonisationBeaconDeployed"}, 2, "Alpha; quoted")
	if value.Time(active(p)["deadline"]).Sub(testNow) != 28*24*time.Hour {
		t.Fatal("claim deadline lost")
	}
	csvText, e := p.ExportCSV(str(c["id"]))
	if e != nil {
		t.Fatal(e)
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(csvText, "\uFEFF")))
	r.Comma = ';'
	records, e := r.ReadAll()
	if e != nil || len(records) != 3 || records[1][1] != "Alpha; quoted" {
		t.Fatal("CSV scope/quoting failed")
	}
	if raw, e := p.ExportPlan(""); e != nil || !bytes.Contains(raw, []byte("Alpha; quoted")) {
		t.Fatal("plan export failed")
	}
	if _, e := p.ExportCSV("missing"); e == nil {
		t.Fatal("exported unknown colony")
	}
	for _, body := range []M{{"action": "configure", "projectId": c["id"], "cargoCapacity": -1}, {"action": "configure", "cargoCapacity": 1.5}, {"action": "set-resource-plan", "projectId": c["id"], "buildingId": bs[0]["id"], "resources": []M{{"commodity": "steel", "required": -1}}, "sourceNote": "test"}, {"action": "reorder-supply", "projectId": c["id"], "ids": []string{"wrong"}}, {"action": "invalid", "projectId": c["id"]}} {
		if p.Update(body) == nil {
			t.Fatalf("invalid action accepted %#v", body)
		}
	}
	action(t, p, M{"action": "reorder-supply", "projectId": c["id"], "ids": []any{bs[1]["id"], bs[0]["id"]}})
	action(t, p, M{"action": "update-project", "id": c["id"], "status": "cancelled"})
	if active(p)["nextTask"] != nil || len(rows(active(p)["operationalSupplies"])) != 0 {
		t.Fatal("cancelled colony has tasks")
	}
	action(t, p, M{"action": "remove-building", "projectId": c["id"], "id": bs[1]["id"]})
	if len(stringsOf(rows(active(p)["buildings"])[0]["dependsOn"])) != 0 {
		t.Fatal("removed dependency retained")
	}
	action(t, p, M{"action": "remove-project", "id": c["id"]})
	if p.Snapshot(nil)["activeProject"] != nil {
		t.Fatal("removed colony retained")
	}
}
