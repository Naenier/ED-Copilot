package presentation

import (
	"strings"
	"testing"
)

func TestColonyBudgetKeepsCoveredGoodsAndShowsUnknownPrices(t *testing.T) {
	s := M{"colonization": M{"activeProject": M{"id": "selected", "supplies": []M{
		{"commodity": "steel", "name": "Сталь", "required": 100, "delivered": 10, "missing": 90, "carrierAvailable": 90, "toBuy": 0, "galacticAveragePrice": 50, "requiredCost": 5000, "missingCost": 4500, "toBuyCost": 0},
		{"commodity": "aluminium", "name": "Алюминий", "required": 20, "missing": 20, "toBuy": nil, "toBuyAtMost": 20, "galacticAveragePrice": nil, "requiredCost": nil, "missingCost": nil, "toBuyCost": nil, "toBuyAtMostCost": nil},
	}}, "projects": []M{{"id": "other", "supplies": []M{{"name": "Other colony", "requiredCost": 999}}}}}}
	p := Build("colonization", "Бюджет", "", s)
	if len(p.Rows) != 2 || len(p.Columns) != 8 || !strings.Contains(strings.Join(p.Modes, "|"), "Бюджет") {
		t.Fatalf("budget table lost selected-colony scope or covered commodity: %#v", p)
	}
	for index, want := range []string{"Сталь", "100", "90", "50", "5 000", "4 500", "0", "—"} {
		if p.Rows[0].Cells[index] != want {
			t.Fatalf("steel cell %d = %q, want %q", index, p.Rows[0].Cells[index], want)
		}
	}
	if p.Rows[1].Cells[3] != "—" || p.Rows[1].Cells[4] != "—" || p.Rows[1].Cells[6] != "—" {
		t.Fatal("unknown price became free resource", p.Rows[1].Cells)
	}
	supply := Build("colonization", "Снабжение", "", s)
	if supply.Rows[1].Cells[6] != "≤20" || supply.Rows[0].Cells[7] != "50" || supply.Rows[0].Cells[8] != "4 500" {
		t.Fatal("supply table lost procurement bound or row valuation", supply.Rows)
	}
}

func TestColonyBudgetEnglishColumnsAndDetailKeepPriceSourceClear(t *testing.T) {
	s := M{"settings": M{"language": "en"}, "colonization": M{"activeProject": M{"id": "selected", "supplies": []M{{"commodity": "steel", "name": "Сталь", "required": 10, "galacticAveragePrice": 50, "requiredCost": 500, "toBuyAtMostCost": 500}}}}}
	p := Build("colonization", "Бюджет", "", s)
	columns := strings.Join(p.Columns, "|")
	if !strings.Contains(columns, "Galactic average, CR/t") || !strings.Contains(columns, "Full value, CR") || p.Rows[0].Cells[6] != "≤500" {
		t.Fatal("budget is missing English financial columns", columns, p.Rows)
	}
	if !strings.Contains(p.Rows[0].Details, "not actual spending or a market offer") {
		t.Fatal("estimate reads like a market quote", p.Rows[0].Details)
	}
}

func TestColonyProcurementUsesMissingPurchaseInsteadOfAlreadyCoveredCargo(t *testing.T) {
	s := M{"commander": M{"system": "Synthetic origin"}, "colonization": M{"activeProject": M{"id": "selected", "buildings": []M{{"id": "site", "status": "building"}}}}}
	for _, mode := range []string{"Снабжение", "Бюджет"} {
		for _, tc := range []struct {
			name     string
			row      M
			quantity float64
			disabled bool
		}{
			{"known deficit", M{"commodity": "steel", "missing": 100, "toBuy": 25}, 25, false},
			{"unknown warehouse bound", M{"commodity": "steel", "missing": 100, "toBuy": nil, "toBuyAtMost": 60}, 60, false},
			{"covered", M{"commodity": "steel", "missing": 100, "toBuy": 0}, 1, true},
			{"legacy unknown", M{"commodity": "steel", "missing": 100}, 100, false},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				a := colonyTestAction(t, ColonyActions(mode, s, tc.row), "colony.purchase")
				if a.Disabled != tc.disabled || a.Payload["quantity"] != tc.quantity {
					t.Fatalf("wrong procurement default: %#v", a)
				}
			})
		}
	}
}
