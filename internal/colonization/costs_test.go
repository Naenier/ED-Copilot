package colonization

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/freshness"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func costPrices() M {
	return M{"steel": M{"price": 2, "observedAt": v.ISO(testNow), "source": "Market.json"}, "aluminium": M{"price": 3, "observedAt": v.ISO(testNow), "source": "Frontier CAPI"}}
}

func costMetric(t *testing.T, summary M, field string, want float64) {
	t.Helper()
	metric := v.Obj(summary[field])
	if !v.Bool(metric["complete"]) || metric["value"] == nil || num(metric["value"]) != want || num(metric["knownValue"]) != want {
		t.Fatalf("%s want %v got %v", field, want, metric)
	}
}

func TestColonyBudgetAggregatesOpenBuildingsAndAllocatesCargoOnlyOnce(t *testing.T) {
	p := New(M{"includeCarrierWarehouse": true}, WithClock(func() time.Time { return testNow }))
	project := colony(t, p, "Budget Alpha", 100, 0)
	id := project["id"]
	bs := rows(project["buildings"])
	action(t, p, M{"action": "record-delivery", "projectId": id, "buildingId": bs[0]["id"], "commodity": "steel", "count": 20})
	action(t, p, M{"action": "add-building", "projectId": id, "typeId": "hub-refinery", "name": "Second steel build", "status": "building"})
	bs = rows(active(p)["buildings"])
	second := bs[len(bs)-1]["id"]
	action(t, p, M{"action": "set-resource-plan", "projectId": id, "buildingId": second, "resources": []M{{"commodity": "steel", "required": 50}}, "sourceNote": "Synthetic estimate"})
	action(t, p, M{"action": "record-delivery", "projectId": id, "buildingId": second, "commodity": "steel", "count": 15})
	action(t, p, M{"action": "add-building", "projectId": id, "typeId": "hub-refinery", "name": "Completed build", "status": "complete"})
	bs = rows(active(p)["buildings"])
	action(t, p, M{"action": "set-resource-plan", "projectId": id, "buildingId": bs[len(bs)-1]["id"], "resources": []M{{"commodity": "steel", "required": 900}}, "sourceNote": "Completed excluded"})
	ctx := M{"commander": M{"fid": "FBUDGET", "vehicle": "ship", "cargoForSync": []M{{"Name": "steel", "Count": 10}}}}
	p.ApplyCAPI(M{"status": M{"connected": true, "fid": "FBUDGET"}, "data": M{"carrier": M{"market": M{"id": 42}, "cargo": []M{{"commodity": "steel", "qty": 40}}, "name": M{"callsign": "BUD-001"}}}, "metadata": M{"observedAt": M{"carrier": v.ISO(testNow)}}}, v.Obj(ctx["commander"]))
	before, _ := json.Marshal(p.ExportSettings())
	snap := p.Snapshot(ctx)
	EnrichCosts(snap, costPrices())
	after, _ := json.Marshal(p.ExportSettings())
	if string(before) != string(after) {
		t.Fatal("cost projection mutated planner")
	}
	summary := v.Obj(v.At(snap, "activeProject", "costSummary"))
	for field, want := range map[string]float64{"required": 600, "delivered": 70, "missing": 530, "cargoCovered": 20, "carrierCovered": 80, "toBuy": 430, "toBuyAtMost": 430} {
		costMetric(t, summary, field, want)
	}
	if num(summary["commodityCount"]) != 2 || num(summary["pricedCommodityCount"]) != 2 || num(summary["unknownBills"]) != 0 {
		t.Fatal(summary)
	}
	for _, row := range rows(v.At(snap, "activeProject", "supplies")) {
		if row["commodity"] == "steel" && num(row["requiredCost"]) != 300 {
			t.Fatal("wrong shared steel row", row)
		}
	}
}

func TestColonyBudgetUnknownPricesAndBillsRemainPartial(t *testing.T) {
	p := New(nil)
	project := colony(t, p, "Budget Unknown", 100, 0)
	action(t, p, M{"action": "add-building", "projectId": project["id"], "typeId": "hub-refinery"})
	snap := p.Snapshot(M{"commander": M{"vehicle": "ship", "cargoForSync": []M{}}})
	EnrichCosts(snap, M{"steel": costPrices()["steel"]})
	summary := v.Obj(v.At(snap, "activeProject", "costSummary"))
	required := v.Obj(summary["required"])
	if required["value"] != nil || v.Bool(required["complete"]) || num(required["knownValue"]) != 200 || num(required["unpricedCommodityCount"]) != 1 || num(summary["unknownBills"]) != 1 {
		t.Fatal(summary)
	}
	delivered := v.Obj(summary["delivered"])
	if num(delivered["unpricedCommodityCount"]) != 0 || delivered["value"] != nil {
		t.Fatal("zero quantity needs no price; unknown bill still prevents whole-plan total", delivered)
	}
	for _, row := range rows(v.At(snap, "activeProject", "supplies")) {
		if row["commodity"] == "aluminium" && (row["galacticAveragePrice"] != nil || row["requiredCost"] != nil) {
			t.Fatal("invented price", row)
		}
	}
}

func TestBudgetUsesOldPricesAsEstimatesAndDistinguishesRetrievalTime(t *testing.T) {
	steel := M{"commodity": "steel", "required": 10, "missing": 10}
	aluminium := M{"commodity": "aluminium", "required": 20, "missing": 20}
	titanium := M{"commodity": "titanium", "required": 30, "missing": 30}
	snapshot := M{"activeProject": M{"supplies": []M{steel, aluminium, titanium}}}
	prices := costPrices()
	v.Obj(prices["steel"])["observedAt"] = v.ISO(testNow.Add(-8 * 24 * time.Hour))
	enrichCostsAt(snapshot, prices, testNow)
	old, loaded, unknown := steel["priceQuality"].(freshness.Observation), aluminium["priceQuality"].(freshness.Observation), titanium["priceQuality"].(freshness.Observation)
	if old.State != freshness.Stale || old.ObservedAt == "" || old.FetchedAt != "" || steel["requiredCost"] != float64(20) {
		t.Fatal("stale observation was discarded or misrepresented", steel)
	}
	if loaded.State != freshness.Fresh || loaded.ObservedAt != "" || loaded.FetchedAt != v.ISO(testNow) {
		t.Fatal("Frontier retrieval was presented as a provider observation", loaded)
	}
	if unknown.State != freshness.Unknown || titanium["galacticAveragePrice"] != nil || titanium["requiredCost"] != nil {
		t.Fatal("unknown price became zero", titanium)
	}
	cost := v.Obj(v.At(snapshot, "activeProject", "costSummary"))
	if v.Int(cost["stalePriceCount"]) != 1 || v.Int(cost["pricedCommodityCount"]) != 2 || v.At(cost, "required", "value") != nil {
		t.Fatal("budget lost partial coverage", cost)
	}
}

func TestColonyBudgetSelectionSeparatesColoniesAndExcludesClosedBuildings(t *testing.T) {
	p := New(nil)
	a := colony(t, p, "Budget Alpha", 100, 1001)
	b := colony(t, p, "Budget Beta", 300, 1002)
	for _, tc := range []struct {
		id   any
		cost float64
	}{{a["id"], 500}, {b["id"], 1500}} {
		action(t, p, M{"action": "select-project", "id": tc.id})
		snap := p.Snapshot(nil)
		EnrichCosts(snap, costPrices())
		costMetric(t, v.Obj(v.At(snap, "activeProject", "costSummary")), "required", tc.cost)
	}
}

func TestColonyCostUnknownCargoAndPartialWarehouseDoNotInventCoverage(t *testing.T) {
	row := M{"commodity": "steel", "required": 100, "delivered": 0, "missing": 100, "cargoAvailable": nil, "carrierIncluded": true, "carrierAvailable": 50, "carrierComplete": true, "toBuy": nil, "toBuyAtMost": nil}
	snap := M{"activeProject": M{"buildings": []M{{"status": "building", "requirements": []M{row}}}, "supplies": []M{row}}}
	EnrichCosts(snap, costPrices())
	if row["requiredCost"] != float64(200) || row["cargoCoveredCost"] != nil || row["carrierCoveredCost"] != nil || row["toBuyCost"] != nil {
		t.Fatal(row)
	}
	row["cargoAvailable"], row["carrierComplete"], row["toBuyAtMost"] = 10, false, 40
	EnrichCosts(snap, costPrices())
	if num(row["cargoCoveredCost"]) != 20 || row["carrierCoveredCost"] != nil || num(row["toBuyAtMostCost"]) != 80 {
		t.Fatal(row)
	}
	row["carrierAvailable"] = 100
	EnrichCosts(snap, costPrices())
	if num(row["carrierCoveredCost"]) != 180 {
		t.Fatal("known lower bound fully covers need", row)
	}
}
