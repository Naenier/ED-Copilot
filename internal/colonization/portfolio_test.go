package colonization

import (
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/value"
)

func TestSupplyPortfolioSharesStockAcrossColoniesAndReconcilesJournal(t *testing.T) {
	p := New(M{
		"includeCarrierWarehouse": true,
		"projects": []M{
			{"id": "alpha", "targetSystem": "Alpha", "status": "complete", "buildings": []M{
				{"id": "alpha-expansion", "status": "planned", "resourcePlan": []M{
					{"commodity": "steel", "required": 160},
					{"commodity": "copper", "required": 80},
				}},
				{"id": "alpha-done", "status": "complete", "resourcePlan": []M{{"commodity": "steel", "required": 999}}},
			}},
			{"id": "beta", "targetSystem": "Beta", "status": "building", "buildings": []M{
				{"id": "beta-depot", "status": "building", "marketId": 202, "resourcePlan": []M{{"commodity": "steel", "required": 999}}},
			}},
		},
	}, WithClock(func() time.Time { return testNow }))
	ctx := M{"commander": M{"fid": "F1", "vehicle": "ship", "cargoForSync": []M{{"Name": "copper", "Count": 30}}}}
	observed(p, M{"event": "LoadGame", "FID": "F1"}, 1, "Beta")
	observed(p, M{"event": "ColonisationConstructionDepot", "MarketID": 202, "ResourcesRequired": []M{
		{"Name": "$steel_name;", "RequiredAmount": 300, "ProvidedAmount": 30},
		{"Name": "$copper_name;", "RequiredAmount": 140, "ProvidedAmount": 20},
	}}, 2, "Beta")
	for _, offset := range []int{3, 3} {
		observed(p, M{"event": "ColonisationContribution", "MarketID": 202, "Contributions": []M{{"Name": "$steel_name;", "Amount": 30}}}, offset, "Beta")
	}
	personal := capiStock("42", 100)
	personal["cargo"] = append(rows(personal["cargo"]), M{"commodity": "copper", "qty": 50})
	p.ApplyCAPI(stockResult(personal, nil, testNow), value.Obj(ctx["commander"]))

	selection := M{"projectIds": []string{"alpha", "beta", "beta"}}
	result, err := p.SupplyPortfolio(ctx, selection)
	if err != nil {
		t.Fatal(err)
	}
	if result["projectCount"] != 2 || result["buildingCount"] != 2 || result["unknownBills"] != 0 {
		t.Fatal("wrong selection counts", result)
	}
	byCommodity := map[string]M{}
	for _, row := range rows(result["supplies"]) {
		byCommodity[str(row["commodity"])] = row
	}
	for commodity, want := range map[string]M{
		"steel":  {"missing": float64(400), "cargoCovered": float64(0), "carrierCovered": float64(100), "toBuy": float64(300)},
		"copper": {"missing": float64(200), "cargoCovered": float64(30), "carrierCovered": float64(50), "toBuy": float64(120)},
	} {
		for field, expected := range want {
			if got := byCommodity[commodity][field]; got != expected {
				t.Errorf("%s %s = %v, want %v", commodity, field, got, expected)
			}
		}
	}

	observed(p, M{"event": "ColonisationConstructionDepot", "MarketID": 202, "ResourcesRequired": []M{
		{"Name": "$steel_name;", "RequiredAmount": 300, "ProvidedAmount": 60},
		{"Name": "$copper_name;", "RequiredAmount": 140, "ProvidedAmount": 20},
	}}, 4, "Beta")
	confirmed, err := p.SupplyPortfolio(ctx, selection)
	if err != nil || num(rows(confirmed["supplies"])[0]["toBuy"]) != 300 {
		t.Fatal("journal confirmation counted a contribution twice", confirmed, err)
	}
}

func TestSupplyPortfolioHonorsSelectionAndUnknownStock(t *testing.T) {
	p := New(M{
		"includeCarrierWarehouse": true,
		"projects": []M{
			{"id": "alpha", "targetSystem": "Alpha", "buildings": []M{
				{"id": "known", "status": "planned", "resourcePlan": []M{{"commodity": "steel", "required": 160}}},
				{"id": "unknown", "status": "building"},
				{"id": "cancelled-build", "status": "cancelled", "resourcePlan": []M{{"commodity": "steel", "required": 999}}},
			}},
			{"id": "beta", "targetSystem": "Beta", "status": "cancelled", "buildings": []M{
				{"id": "beta-depot", "status": "building", "resourcePlan": []M{{"commodity": "steel", "required": 999}}},
			}},
			{"id": "other", "targetSystem": "Other", "journalOwnership": M{
				"source": "journal-claim", "automatic": true, "ownerKey": ownerKey("F2"), "claimedAt": value.ISO(testNow), "status": "claimed",
			}},
		},
	}, WithClock(func() time.Time { return testNow }))
	ctx := M{"commander": M{"fid": "F1", "vehicle": "ship", "cargoForSync": []M{{"Name": "steel", "Count": 20}}}}
	observed(p, M{"event": "LoadGame", "FID": "F1"}, 1, "Alpha")
	all, err := p.SupplyPortfolio(ctx, M{"projectIds": []any{"alpha", "beta"}})
	if err != nil || all["unknownBills"] != 1 || all["buildingCount"] != 2 {
		t.Fatal("active work and unknown requirements were lost", all, err)
	}
	filtered, err := p.SupplyPortfolio(ctx, M{"projectIds": []any{"alpha"}, "buildingIds": []any{"known", "known"}})
	if err != nil || filtered["unknownBills"] != 0 || filtered["buildingCount"] != 1 {
		t.Fatal("building filter did not isolate the selection", filtered, err)
	}
	steel := rows(filtered["supplies"])[0]
	if steel["missing"] != float64(160) || steel["cargoCovered"] != float64(20) ||
		steel["carrierCovered"] != nil || steel["toBuy"] != nil || steel["toBuyAtMost"] != float64(140) {
		t.Fatal("unknown warehouse became a confirmed zero", steel)
	}
	p.ApplyCAPI(stockResult(capiStock("42", 30), capiStock("43", 40), testNow), value.Obj(ctx["commander"]))
	action(t, p, M{"action": "configure", "squadronWarehouseAccessConfirmed": true})
	next := testNow.Add(time.Second)
	p.now = func() time.Time { return next }
	p.ApplyCAPI(stockResult(capiStock("42", 30), nil, next), value.Obj(ctx["commander"]))
	partial, err := p.SupplyPortfolio(ctx, M{"projectIds": []string{"alpha"}, "buildingIds": []string{"known"}})
	if err != nil {
		t.Fatal(err)
	}
	steel = rows(partial["supplies"])[0]
	if steel["carrierAvailable"] != float64(30) || steel["carrierCovered"] != nil ||
		steel["toBuy"] != nil || steel["toBuyAtMost"] != float64(110) {
		t.Fatal("partially known stock must reduce the upper bound only", steel)
	}
	empty, err := p.SupplyPortfolio(ctx, M{"projectIds": []string{"alpha"}, "buildingIds": []string{}})
	if err != nil || empty["buildingCount"] != 0 || len(rows(empty["supplies"])) != 0 {
		t.Fatal("explicit empty building selection fell back to all buildings", empty, err)
	}
	for _, selection := range []M{
		{"projectIds": []string{"alpha"}, "buildingIds": []string{"beta-depot"}},
		{"projectIds": []string{"other"}},
		{"projectIds": []any{123}},
	} {
		if result, err := p.SupplyPortfolio(ctx, selection); err == nil {
			t.Fatal("invalid or foreign selection was accepted", selection, result)
		}
	}
	otherContext := M{"commander": M{"fid": "F2"}}
	if result, err := p.SupplyPortfolio(otherContext, M{"projectIds": []string{"alpha"}}); err == nil {
		t.Fatal("mismatched active commander was accepted", result)
	}
}
