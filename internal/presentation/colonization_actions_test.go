package presentation

import (
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/colonization"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func colonyTestState(p *colonization.Planner) M {
	return M{"colonization": p.Snapshot(nil), "commander": M{"system": "Synthetic Origin"}, "goods": []M{{"key": "steel", "name": "Сталь", "kind": "commodity"}, {"key": "aluminium", "name": "Алюминий", "kind": "commodity"}}, "resources": []M{{"key": "iron", "name": "Железо", "kind": "raw"}}}
}
func colonyTestAction(t *testing.T, actions []Action, id string) Action {
	t.Helper()
	for _, a := range actions {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("missing action %s", id)
	return Action{}
}
func colonyTestBody(t *testing.T, a Action, overrides M) M {
	t.Helper()
	values := M{}
	for _, f := range a.Fields {
		values[f.Key] = v.Clone(f.Default)
	}
	for k, x := range overrides {
		values[k] = x
	}
	body, err := a.Body(values)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func colonyTestApply(t *testing.T, p *colonization.Planner, a Action, overrides M) {
	t.Helper()
	if a.Disabled {
		t.Fatal("attempted disabled action")
	}
	if a.Path != "/api/colonization" {
		t.Fatalf("not a domain action: %s", a.Path)
	}
	if err := p.Update(colonyTestBody(t, a, overrides)); err != nil {
		t.Fatal(err)
	}
}
func colonyTestProject(p *colonization.Planner) M { return v.Obj(p.Snapshot(nil)["activeProject"]) }

func TestColonyCarrierStockOptionsUseGlobalTypedConfiguration(t *testing.T) {
	p := colonization.New(nil)
	p.ApplyCAPI(M{"status": M{"connected": true, "fid": "F1"}, "data": M{"squadron": M{"squadronCarrier": M{"market": M{"id": "43"}, "cargo": []M{}}}}, "metadata": M{"observedAt": M{"squadron": "2026-11-08T12:00:00Z"}}}, M{"fid": "F1"})
	for _, mode := range []string{"Выбор системы", "Строительство", "Снабжение"} {
		a := colonyTestAction(t, ColonyActions(mode, colonyTestState(p), nil), "colony.carrier-stock")
		body := colonyTestBody(t, a, M{"includeCarrierWarehouse": true, "squadronWarehouseAccessConfirmed": true})
		if body["action"] != "configure" || body["projectId"] != nil {
			t.Fatal("warehouse options incorrectly depend on a selected colony", body)
		}
		if err := p.Update(body); err != nil {
			t.Fatal(err)
		}
	}
	if !v.Bool(p.ExportSettings()["includeCarrierWarehouse"]) || !v.Bool(p.ExportSettings()["squadronWarehouseAccessConfirmed"]) {
		t.Fatal("typed warehouse options were not persisted")
	}
}

func TestNativeColoniesFormWorkflow(t *testing.T) {
	p := colonization.New(nil)
	state := colonyTestState(p)
	create := colonyTestAction(t, ColonyActions("Выбор системы", state, nil), "colony.create")
	colonyTestApply(t, p, create, M{"targetSystem": "Synthetic Alpha", "name": "Мой первый дом"})
	alpha := colonyTestProject(p)
	id := v.Str(alpha["id"])
	state = colonyTestState(p)
	a := colonyTestAction(t, ColonyActions("Строительство", state, nil), "colony.add-building")
	colonyTestApply(t, p, a, M{"typeId": "coriolis", "isPrimary": true, "name": "Альфа-порт"})
	first := v.Rows(colonyTestProject(p)["buildings"])[0]
	a = colonyTestAction(t, ColonyActions("Строительство", colonyTestState(p), nil), "colony.add-building")
	colonyTestApply(t, p, a, M{"typeId": "hub-refinery", "isPrimary": false, "name": "Переработка"})
	second := v.Rows(colonyTestProject(p)["buildings"])[1]
	a = colonyTestAction(t, ColonyActions("Строительство", colonyTestState(p), second), "colony.dependency-add")
	colonyTestApply(t, p, a, M{"dependencyId": first["id"]})
	second = v.Rows(colonyTestProject(p)["buildings"])[1]
	if len(v.Arr(second["dependsOn"])) != 1 {
		t.Fatal("native dependency form lost predecessor")
	}
	a = colonyTestAction(t, ColonyActions("Строительство", colonyTestState(p), second), "colony.dependency-remove")
	colonyTestApply(t, p, a, M{"dependencyId": first["id"]})
	second = v.Rows(colonyTestProject(p)["buildings"])[1]
	if len(v.Arr(second["dependsOn"])) != 0 {
		t.Fatal("native dependency removal failed")
	}
	a = colonyTestAction(t, ColonyActions("Очередь снабжения", colonyTestState(p), second), "colony.move-up")
	body := colonyTestBody(t, a, nil)
	if v.Str(body["action"]) != "move-supply-building" {
		t.Fatal("supply queue action changed construction queue")
	}
	colonyTestApply(t, p, a, nil)
	if v.Str(v.Rows(colonyTestProject(p)["supplyQueue"])[0]["id"]) != v.Str(second["id"]) || v.Str(v.Rows(colonyTestProject(p)["buildings"])[0]["id"]) != v.Str(first["id"]) {
		t.Fatal("queues no longer independent")
	}
	a = colonyTestAction(t, ColonyActions("Снабжение", colonyTestState(p), nil), "colony.capacity")
	colonyTestApply(t, p, a, M{"cargoCapacity": "512"})
	if v.Num(colonyTestProject(p)["cargoCapacity"]) != 512 {
		t.Fatal("per-colony cargo capacity not applied")
	}
	a = colonyTestAction(t, ColonyActions("Строительство", colonyTestState(p), first), "colony.edit")
	colonyTestApply(t, p, a, M{"pointsTier2": "3", "pointsTier3": "6", "claimedAt": "07.11.2026 20:30", "deadline": "05.12.2026 20:30", "externalPortCount": 2, "goalEconomy": "refinery"})
	project := colonyTestProject(p)
	if v.Num(v.At(project, "points", "tier2")) != 3 || v.Num(v.At(project, "points", "tier3")) != 6 || v.Time(project["claimedAt"]).Local().Format("02.01.2006 15:04") != "07.11.2026 20:30" {
		t.Fatal("native project form did not map points/date")
	}
	a = colonyTestAction(t, ColonyActions("Выбор системы", colonyTestState(p), nil), "colony.create")
	colonyTestApply(t, p, a, M{"targetSystem": "Synthetic Beta", "name": "Вторая колония"})
	beta := colonyTestProject(p)
	if len(v.Rows(p.Snapshot(nil)["projects"])) != 2 {
		t.Fatal("second colony replaced first")
	}
	a = colonyTestAction(t, ColonyActions("Поставки", colonyTestState(p), nil), "colony.select")
	colonyTestApply(t, p, a, M{"id": id})
	if v.Str(colonyTestProject(p)["id"]) != id || v.Num(colonyTestProject(p)["cargoCapacity"]) != 512 {
		t.Fatal("colony picker did not restore independent plan")
	}
	for _, choice := range a.Fields[0].Choices {
		if choice.Value == v.Str(beta["id"]) && !strings.Contains(choice.Label, "Synthetic Beta") {
			t.Fatal("native choice lost system name")
		}
	}
}

func TestNativeSupplyFormsUseCorrectStageAndCommodityIDs(t *testing.T) {
	p := colonization.New(nil)
	if err := p.Update(M{"action": "create-project", "targetSystem": "Alpha", "status": "building"}); err != nil {
		t.Fatal(err)
	}
	projectID := colonyTestProject(p)["id"]
	for i, typ := range []string{"coriolis", "hub-refinery"} {
		if err := p.Update(M{"action": "add-building", "projectId": projectID, "typeId": typ, "name": []string{"Порт", "Переработка"}[i]}); err != nil {
			t.Fatal(err)
		}
	}
	bs := v.Rows(colonyTestProject(p)["buildings"])
	for i, b := range bs {
		if err := p.Update(M{"action": "set-resource-plan", "projectId": projectID, "buildingId": b["id"], "resources": []M{{"commodity": "aluminium", "required": 10 * (i + 1)}}, "sourceNote": "План архитектора"}); err != nil {
			t.Fatal(err)
		}
	}
	state := colonyTestState(p)
	selected := M{"commodity": "aluminium", "name": "Алюминий", "required": 30, "missing": 25, "buildings": []M{{"id": bs[1]["id"], "name": "Переработка"}}}
	actions := ColonyActions("Снабжение", state, selected)
	upsert := colonyTestAction(t, actions, "colony.resource-upsert")
	body := colonyTestBody(t, upsert, nil)
	if upsert.Path != "/colonization/resource-row" || v.Str(body["buildingId"]) != v.Str(bs[1]["id"]) || v.Num(body["required"]) != 20 || v.Str(body["commodity"]) != "aluminium" {
		t.Fatalf("resource helper mapped wrong stage: %#v", body)
	}
	for _, f := range upsert.Fields {
		if f.Key == "commodity" {
			for _, choice := range f.Choices {
				if choice.Value == "iron" {
					t.Fatal("engineering material offered as construction commodity")
				}
				if choice.Value == "aluminium" && choice.Label != "Алюминий" {
					t.Fatal("display/commodity identity mixed")
				}
			}
		}
	}
	delivery := colonyTestAction(t, actions, "colony.delivery-add")
	colonyTestApply(t, p, delivery, M{"commodity": "$aluminium_name;", "count": "12", "notes": "Ручной рейс"})
	ds := v.Rows(colonyTestProject(p)["manualDeliveries"])
	if len(ds) != 1 || v.Str(ds[0]["commodity"]) != "aluminium" || v.Str(ds[0]["buildingId"]) != v.Str(bs[1]["id"]) {
		t.Fatal("manual delivery form lost colony/stage association")
	}
	purchase := colonyTestAction(t, actions, "colony.purchase")
	body = colonyTestBody(t, purchase, nil)
	if purchase.Kind != "purchase" || purchase.Path != "sale" || v.Str(body["resourceKey"]) != "aluminium" || v.Num(body["quantity"]) != 25 || v.Str(body["system"]) != "Synthetic Origin" {
		t.Fatal("purchase handoff wrong")
	}
	remove := colonyTestAction(t, ColonyActions("Поставки", colonyTestState(p), ds[0]), "colony.delivery-remove")
	if remove.Confirm == "" {
		t.Fatal("removal has no concrete confirmation")
	}
	colonyTestApply(t, p, remove, nil)
	for _, a := range ColonyActions("Поставки", colonyTestState(p), M{"id": "journal", "source": "journal"}) {
		if a.ID == "colony.delivery-remove" {
			t.Fatal("native UI allows deletion of journal event")
		}
	}
}

func TestNativeColonizationSearchGuardsAndValidation(t *testing.T) {
	p := colonization.New(nil)
	state := colonyTestState(p)
	unavailable := M{"id": "candidate", "name": "Unavailable", "availability": "unavailable"}
	a := colonyTestAction(t, ColonyActions("Выбор системы", state, unavailable), "colony.choose-candidate")
	if !a.Disabled {
		t.Fatal("unavailable candidate selectable")
	}
	a = colonyTestAction(t, ColonyActions("Выбор системы", state, M{"id": "candidate", "name": "Unknown", "availability": "unknown"}), "colony.choose-candidate")
	if a.Disabled || a.Payload["targetSystem"] != "Unknown" {
		t.Fatal("unconfirmed candidate cannot be planned")
	}
	search := colonyTestAction(t, ColonyActions("Выбор системы", state, nil), "colony.search")
	if search.Path != "/api/colonization/search" {
		t.Fatal("search route incorrect")
	}
	if !colonyTestAction(t, ColonyActions("Выбор системы", state, nil), "colony.cancel-search").Disabled {
		t.Fatal("idle cancel enabled")
	}
	if _, err := colonyParseDate("35.14.2026 99:12"); err == nil {
		t.Fatal("invalid date accepted")
	}
	for _, s := range []string{"07.11.2026 21:45", "2026-11-07T21:45", "2026-11-07 21:45", "2026-11-07", "2026-11-07T21:45:00Z"} {
		date, err := colonyParseDate(s)
		if err != nil || v.Time(date).IsZero() {
			t.Fatalf("date %s rejected", s)
		}
	}
	if date, err := colonyParseDate(""); err != nil || date != nil {
		t.Fatal("cleared date not null")
	}
	for _, raw := range []any{-1, 1.5, "bad", 1e10} {
		if _, err := colonyOptionalInteger(raw, 1, 1e9); err == nil {
			t.Fatalf("invalid integer %v accepted", raw)
		}
	}
	if err := p.Update(M{"action": "create-project", "targetSystem": "Alpha"}); err != nil {
		t.Fatal(err)
	}
	id := colonyTestProject(p)["id"]
	if err := p.Update(M{"action": "add-building", "projectId": id, "typeId": "coriolis", "marketId": "90071992547409999"}); err != nil {
		t.Fatal(err)
	}
	b := v.Rows(colonyTestProject(p)["buildings"])[0]
	link := colonyTestAction(t, ColonyActions("Строительство", colonyTestState(p), b), "colony.link-depot")
	found := false
	for _, choice := range link.Fields[0].Choices {
		found = found || choice.Value == "90071992547409999"
	}
	if !found {
		t.Fatal("saved depot association disappears before journal replay")
	}
	for _, mode := range []string{"Выбор системы", "Строительство", "Снабжение", "Очередь снабжения", "Поставки"} {
		seen := map[string]bool{}
		for _, action := range ColonyActions(mode, colonyTestState(p), b) {
			if seen[action.ID] {
				t.Fatalf("duplicate action %s", action.ID)
			}
			seen[action.ID] = true
			if action.Title == "" || action.Path == "" {
				t.Fatal("native action not reviewable")
			}
			for _, f := range action.Fields {
				if f.Key == "" || f.Label == "" || f.Kind == "" {
					t.Fatal("incomplete native field")
				}
			}
		}
	}
}
