package presentation

import (
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/freshness"
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
	if len(supply.Rows) != 0 || len(supply.Columns) != 3 {
		t.Fatal("undelivered supplies should not appear as journal deliveries", supply.Rows)
	}
}

func TestColonyDeliveryLogListsJournalResourcesWithoutProvenanceColumns(t *testing.T) {
	s := M{"colonization": M{"activeProject": M{"id": "test", "journalDeliveries": []M{
		{"timestamp": "2031-01-01T10:00:00Z", "resources": []M{{"name": "Сталь", "count": 30}, {"name": "Алюминий", "count": 20}}},
	}, "manualDeliveries": []M{{"timestamp": "2031-01-01T09:00:00Z", "name": "Медь", "count": 7}}}}}
	for _, mode := range []string{colonySupplyLog, "Снабжение", "Поставки", "supplies"} {
		p := Build("colonization", mode, "", s)
		if len(p.Rows) != 3 || len(p.Columns) != 3 || p.Rows[0].Cells[1] != "Сталь" || p.Rows[2].Cells[2] != "7" || p.Rows[0].ID == p.Rows[1].ID {
			t.Fatalf("%s: wrong delivery ordering, identity or columns: %v %v", mode, p.Columns, p.Rows)
		}
	}
}

func TestColonySystemSelectionShowsOnlySuitableCandidates(t *testing.T) {
	results := []M{
		{"id": "blocked", "name": "Occupied", "availability": "unavailable", "distanceLy": 4},
		{"id": "unknown", "name": "Unverified", "availability": "unknown", "distanceLy": 5},
		{"id": "candidate", "name": "Suitable", "availability": "candidate", "distanceLy": 7},
	}
	search := M{"results": results}
	s := M{"colonization": M{"search": search}}
	p := Build("colonization", "Выбор системы", "", s)
	if len(p.Rows) != 1 || p.Rows[0].ID != "candidate" || p.Rows[0].Cells[0] != "Suitable" || p.Rows[0].Cells[1] != "7" {
		t.Fatal("only a suitable system should be selectable", p.Rows)
	}
	search["results"] = results[:2]
	p = Build("colonization", "Выбор системы", "", s)
	if len(p.Rows) != 0 || p.Summary != "" {
		t.Fatal("an empty search must not present unsuitable rows or a generic summary", p)
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

func TestColonyBudgetShowsPriceWithoutAgeWarning(t *testing.T) {
	for _, language := range []string{"ru", "en"} {
		row := M{"commodity": "steel", "galacticAveragePrice": 50, "priceQuality": freshness.Observation{State: freshness.Stale, FetchedAt: "2026-11-01T12:00:00Z"}}
		state := M{"settings": M{"language": language}, "colonization": M{"activeProject": M{"id": "selected", "supplies": []M{row}}}}
		page := Build("colonization", "Бюджет", "", state)
		if page.Rows[0].Cells[3] != "50" || !strings.Contains(page.Rows[0].Details, "2026") {
			t.Fatal(language, page.Rows[0])
		}
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
