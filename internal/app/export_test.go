package app

import (
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func exportCSVRecords(t *testing.T, text string) [][]string {
	t.Helper()
	if !strings.HasPrefix(text, "\ufeff") {
		t.Fatal("export lacks Excel UTF-8 BOM")
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\ufeff")))
	r.Comma = ';'
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal("invalid CSV escaping", err)
	}
	return rows
}

func TestTradingCSVExportsEveryTransactionBeyondRecentLimit(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	const station = `=HYPERLINK("synthetic"); станция`
	e.Apply(testEvent("Docked", M{"StationName": station, "StarSystem": "Synthetic Origin", "MarketID": 99}), M{})
	start := time.Date(2026, 11, 7, 12, 1, 0, 0, time.UTC)
	for i := 0; i < 260; i++ {
		e.Apply(testEvent("MarketBuy", M{"Type": "Gold", "Count": 1, "TotalCost": 100 + i, "timestamp": v.ISO(start.Add(time.Duration(i) * time.Second))}), M{})
	}
	if len(e.Trading.Recent) != 200 || len(e.Trading.Transactions) != 260 {
		t.Fatal("fixture did not exceed display limit", len(e.Trading.Recent), len(e.Trading.Transactions))
	}
	text, err := e.ExportCSV("trading")
	if err != nil {
		t.Fatal(err)
	}
	records := exportCSVRecords(t, text)
	if len(records) != 261 {
		t.Fatalf("CSV exported %d transactions; want all 260", len(records)-1)
	}
	if records[1][0] != v.ISO(start) || records[260][0] != v.ISO(start.Add(259*time.Second)) {
		t.Fatal("first or final transaction lost or reordered")
	}
	if records[1][4] != "100" || records[260][4] != "359" {
		t.Fatal("transaction values changed", records[1], records[260])
	}
	if records[1][8] != "'"+station {
		t.Fatal("station field became an executable spreadsheet formula", records[1][8])
	}
	if e.Trading.Transactions[0]["station"] != station {
		t.Fatal("CSV escaping mutated ledger")
	}
}

func TestExportsRetainExplicitInactiveColonyAndSelectedWrapper(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	alpha := createEngineColony(t, e, "Alpha; экспорт", 100, 101)
	alphaID := v.Str(alpha["id"])
	var csvAction, planAction p.Action
	for _, a := range p.CoreActions("colonization", "Снабжение", e.Snapshot(), nil, nil) {
		if a.ID == "colony.export-csv" {
			csvAction = a
		}
		if a.ID == "colony.export-json" {
			planAction = a
		}
	}
	beta := createEngineColony(t, e, "Beta", 200, 202)
	if v.Str(v.At(e.Snapshot(), "colonization", "selectedProjectId")) != v.Str(beta["id"]) {
		t.Fatal("second colony not active")
	}
	for _, a := range []p.Action{csvAction, planAction} {
		if v.Str(a.Payload["projectId"]) != alphaID {
			t.Fatal("native action omitted its captured colony", a)
		}
	}
	text, err := e.ExportCSVFor("colonization", v.Str(csvAction.Payload["projectId"]))
	if err != nil {
		t.Fatal(err)
	}
	alphaRows := exportCSVRecords(t, text)
	if len(alphaRows) != 3 {
		t.Fatal("inactive colony requirements not exported", alphaRows)
	}
	for _, row := range alphaRows[1:] {
		if row[1] != "Alpha; экспорт" || row[5] != "100" {
			t.Fatal("wrong colony leaked into explicit CSV", row)
		}
	}
	selected, err := e.ExportCSV("colonization")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range exportCSVRecords(t, selected)[1:] {
		if row[1] != "Beta" || row[5] != "200" {
			t.Fatal("wrapper did not export selected colony", row)
		}
	}
	raw, err := e.ExportPlanFor("colonization", v.Str(planAction.Payload["projectId"]))
	if err != nil {
		t.Fatal(err)
	}
	var plan M
	if err = json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	if plan["id"] != alphaID || plan["targetSystem"] != "Alpha; экспорт" || len(v.Rows(plan["buildings"])) != 2 {
		t.Fatal("explicit JSON exported wrong colony", plan)
	}
	raw, err = e.ExportPlan("colonization")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &plan); err != nil || plan["id"] != beta["id"] {
		t.Fatal("selected JSON exported wrong colony", err)
	}
	if _, err = e.ExportCSVFor("colonization", "unknown-colony"); err == nil {
		t.Fatal("unknown explicit colony silently fell back to selected")
	}
	if _, err = e.ExportPlanFor("colonization", "unknown-colony"); err == nil {
		t.Fatal("unknown explicit colony JSON silently fell back")
	}
	if v.Str(v.At(e.Snapshot(), "colonization", "selectedProjectId")) != v.Str(beta["id"]) {
		t.Fatal("export changed overlay colony selection")
	}
}

func TestEngineeringPlanExportRoundTripAndUnknownExportKinds(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	doOK(t, e, "/api/engineering", M{"action": "add", "moduleKey": "am", "blueprintKey": "AFM_Shielded", "rolls": []int{1, 2, 3, 4, 5}, "quantity": 2, "label": "Походный модуль"})
	text, err := e.ExportCSVFor("engineering", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(exportCSVRecords(t, text)) < 2 {
		t.Fatal("engineering shopping CSV empty")
	}
	raw, err := e.ExportPlanFor("engineering", "")
	if err != nil {
		t.Fatal(err)
	}
	var plan M
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err = decoder.Decode(&plan); err != nil {
		t.Fatal(err)
	}
	if plan["format"] != "ed-copilot-engineering" || v.Num(plan["version"]) != 1 || len(v.Rows(plan["items"])) != 1 {
		t.Fatal("engineering export not importable", plan)
	}
	doOK(t, e, "/api/engineering", M{"action": "clear"})
	doOK(t, e, "/api/engineering", M{"action": "import", "items": plan["items"]})
	item := v.Rows(v.At(e.Snapshot(), "engineering", "items"))[0]
	if item["label"] != "Походный модуль" || v.Num(item["quantity"]) != 2 {
		t.Fatal("export/import lost personal configuration", item)
	}
	for _, kind := range []string{"unknown", "route.json", "carrier"} {
		if _, err = e.ExportCSVFor(kind, ""); err == nil {
			t.Fatal("unknown CSV kind exported unrelated statistics", kind)
		}
		if _, err = e.ExportPlanFor(kind, ""); err == nil {
			t.Fatal("unknown JSON kind exported unrelated goals", kind)
		}
	}
}
