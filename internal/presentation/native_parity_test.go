package presentation

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/engineering"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestSavedMiningPlacesAreReachableAndEditable(t *testing.T) {
	s := presentationState(t)
	s["bookmarks"] = []M{{"id": "a", "targetId": "target", "system": "Synthetic A", "ring": "A Ring", "kind": "laser"}, {"id": "b", "system": "Synthetic B", "body": "B 1", "kind": "surface-planet"}, {"id": "c", "system": "Synthetic A", "body": "A 2", "kind": "raw"}}
	before, _ := json.Marshal(s)
	p := Build("targets", "Сохранённые места", "", s)
	if len(p.Rows) != 3 || p.Rows[0].ID != "a" || p.Rows[1].Cells[0] != "Synthetic B" {
		t.Fatal("saved places disappeared", p.Rows)
	}
	a := coreAction(t, "targets", "Сохранённые места", "bookmark.remove", s, p.Rows[1].Data, nil)
	body, err := a.Body(nil)
	if err != nil || a.Disabled || a.Path != "/api/bookmarks" || body["action"] != "remove" || body["id"] != "b" {
		t.Fatal("wrong saved place removed", a, body, err)
	}
	if !coreAction(t, "targets", "Сохранённые места", "bookmark.remove", s, nil, nil).Disabled {
		t.Fatal("remove enabled without selection")
	}
	copy := coreAction(t, "targets", "Сохранённые места", "bookmarks.copy", s, nil, nil)
	if copy.Kind != "copy" || copy.Path != "Synthetic A\r\nSynthetic B" {
		t.Fatal("saved systems lost order or deduplication", copy)
	}
	if len(Build("targets", "Сохранённые места", "B 1", s).Rows) != 1 {
		t.Fatal("saved-place search unavailable")
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("render mutated saved places")
	}
}

func TestStatisticsSummaryRetainsObservedPeriodAfterRemovingDuplicateOverviewMode(t *testing.T) {
	s := presentationState(t)
	overview := Build("overview", "", "", s)
	if !reflect.DeepEqual(overview.Modes, []string{"Обзор", "Корабль и груз", "Миссии", "Журнал"}) {
		t.Fatal("duplicate overview mode", overview.Modes)
	}
	values := map[string]string{}
	for _, r := range Build("statistics", "Итоги", "", s).Rows {
		values[r.ID] = r.Cells[1]
	}
	if values["total.refinedTons"] != "15" || values["total.prospected"] != "1" || values["since"] == "—" {
		t.Fatal("observed period totals hidden", values)
	}
}

func TestEngineeringMaterialFiltersAndDirectExchange(t *testing.T) {
	s := presentationState(t)
	materials := v.Rows(v.At(s, "engineering", "materials"))
	planRows := 0
	for _, r := range materials {
		if v.Num(r["needed"]) > 0 {
			planRows++
		}
	}
	if got := Build("engineering", "Материалы плана", "", s); len(got.Rows) != planRows || planRows == 0 {
		t.Fatal("plan-only filter discarded or invented rows", len(got.Rows), planRows)
	}
	for _, mode := range []string{"Сырьё", "Промышленные материалы", "Данные", "Сырьё плана", "Промышленные материалы плана", "Данные плана"} {
		category, planOnly, _ := materialView(mode)
		page := Build("engineering", mode, "", s)
		if len(page.Rows) == 0 {
			t.Fatal("empty category fixture", mode)
		}
		for _, r := range page.Rows {
			if r.Data["category"] != category || planOnly && v.Num(r.Data["needed"]) <= 0 {
				t.Fatal("wrong category/plan filter", mode, r.Data)
			}
		}
	}
	selected := Build("engineering", "Данные плана", "", s).Rows[0].Data
	a := coreAction(t, "engineering", "Данные плана", "engineering.trade", s, selected, nil)
	if a.Disabled || len(a.Fields) < 2 || a.Fields[0].Default != selected["key"] || v.Num(a.Fields[1].Default) != 1 {
		t.Fatal("exchange form missing selected material and quantity", a)
	}
	for _, item := range a.Fields[0].Choices {
		if item.Value == "guardianpowercell" || item.Value == "mercenarycoin" {
			t.Fatal("special material offered for exchange", item)
		}
	}
	body, err := a.Body(M{"tradeTargetKey": a.Fields[0].Default, "tradeTargetAmount": a.Fields[1].Default})
	if err != nil {
		t.Fatal(err)
	}
	planner := engineering.New(nil)
	if err = planner.Update(body); err != nil {
		t.Fatal(err)
	}
	if v.At(planner.Snapshot(nil), "trader", "targetKey") != selected["key"] {
		t.Fatal("exchange target did not update domain")
	}
	if coreAction(t, "engineering", "Материалы", "engineering.trade", s, nil, nil).Disabled {
		t.Fatal("exchange form cannot pick a target without a selected row")
	}
	copy := coreAction(t, "engineering", "Материалы плана", "engineering.copy-shopping", s, nil, nil)
	if copy.Kind != "copy" || copy.Disabled || !strings.Contains(copy.Path, v.Str(selected["name"])) {
		t.Fatal("shopping list copy lost", copy)
	}
	if len(Build("engineering", "Материалы", "СЫРЬЕ", s).Rows) == 0 {
		t.Fatal("Russian category search unavailable")
	}
}

func TestMarketWarningsAndQuoteObservationDateRemainVisible(t *testing.T) {
	s := presentationState(t)
	market := v.Rows(v.At(s, "trading", "results"))[0]
	market["expired"] = true
	market["warnings"] = []any{"Спрос ниже объёма груза", M{"text": "Флотоносец мог переместиться"}}
	page := Build("sale", "Продать", "", s)
	if page.Rows[0].Cells[6] != Date(market["updatedAt"]) || !strings.Contains(page.Rows[0].Details, "Спрос ниже объёма груза") || !strings.Contains(page.Rows[0].Details, "Флотоносец мог переместиться") || strings.Contains(page.Rows[0].Details, "устарела") {
		t.Fatal("market risk suppressed", page.Rows[0])
	}
}

func TestTradeRouteCanCopyEachMarketAndTrackTheEntireRoute(t *testing.T) {
	s := presentationState(t)
	s["planWorkspace"] = M{"key": "commander"}
	selected := v.Copy(v.Rows(v.At(s, "tradeRoutes", "results"))[0])
	leg := v.Rows(selected["legs"])[0]
	firstBuy, firstSell := v.Copy(v.Obj(leg["buy"])), v.Copy(v.Obj(leg["sell"]))
	firstBuy["system"], firstSell["system"] = "Система закупки", "Система продажи"
	secondBuy, secondSell := v.Copy(firstSell), v.Copy(firstBuy)
	secondBuy["system"], secondSell["system"] = "Обратная закупка", "Дом"
	selected["legs"] = []M{v.Merge(v.Copy(leg), M{"buy": firstBuy, "sell": firstSell}), v.Merge(v.Copy(leg), M{"buy": secondBuy, "sell": secondSell, "quantity": 50})}
	seen := map[string]bool{}
	for _, a := range CoreActions("deals", "", s, selected, nil) {
		if seen[a.ID] {
			t.Fatal("duplicate leg action", a.ID)
		}
		seen[a.ID] = true
	}
	for _, tc := range []struct {
		suffix, buy, sell string
	}{{"0", "Система закупки", "Система продажи"}, {"1", "Обратная закупка", "Дом"}} {
		if got := coreAction(t, "deals", "", "deals.copy-buy."+tc.suffix, s, selected, nil); got.Disabled || got.Path != tc.buy {
			t.Fatal(got)
		}
		if got := coreAction(t, "deals", "", "deals.copy-sell."+tc.suffix, s, selected, nil); got.Disabled || got.Path != tc.sell {
			t.Fatal(got)
		}
	}
	a := coreAction(t, "deals", "", "deals.track", s, selected, nil)
	body, err := a.Body(nil)
	if err != nil || a.Path != "/api/trading/active-route" || body["routeId"] != selected["id"] || body["action"] != "start" {
		t.Fatal("route tracking did not receive the result id", a, body)
	}
	if got := CoreActions("deals", "", s, nil, nil); len(got) != 2 {
		t.Fatal("row-specific actions enabled without route", got)
	}
}

func TestPurchaseAppliesFormTransformsBeforeNavigation(t *testing.T) {
	a := Action{Payload: M{"quantity": 5, "system": "Old", "resourceKey": "steel"}, Transform: func(body M) (M, error) {
		if v.Num(body["quantity"]) <= 0 {
			return nil, errors.New("invalid quantity")
		}
		body["quantity"] = v.Num(body["quantity"]) * 2
		return body, nil
	}}
	prefill, err := PurchasePrefill(a, M{"quantity": 12, "system": "New"})
	if err != nil || v.Num(prefill["quantity"]) != 24 || prefill["system"] != "New" || a.Payload["system"] != "Old" || v.Num(a.Payload["quantity"]) != 5 {
		t.Fatal("purchase bypassed transform or mutated action", prefill, a, err)
	}
	if _, err = PurchasePrefill(a, M{"quantity": 0}); err == nil {
		t.Fatal("invalid form navigated to market")
	}
}

func TestStatisticsActionsReachResetAndBothExports(t *testing.T) {
	s := presentationState(t)
	for _, mode := range Build("statistics", "", "", s).Modes {
		reset := coreAction(t, "statistics", mode, "stats.reset", s, nil, nil)
		body, err := reset.Body(nil)
		if err != nil || reset.Path != "/api/session" || body["action"] != "reset" || reset.Confirm == "" {
			t.Fatal("statistics period action unreachable", reset, body)
		}
		for _, id := range []string{"stats.export", "stats.trade.export"} {
			a := coreAction(t, "statistics", mode, id, s, nil, nil)
			if a.Kind != "export" || !strings.HasSuffix(a.Path, ".csv") {
				t.Fatal(a)
			}
		}
	}
}

func TestFleetRouteTotalsAndCarrierServicesRetainDomainResults(t *testing.T) {
	s := presentationState(t)
	p := Build("route", "Итоги", "", s)
	values := map[string]string{}
	for _, r := range p.Rows {
		values[r.ID] = r.Cells[1]
	}
	if values["fuelTons"] != "100" || values["startingTritiumTons"] != "500" || values["arrivalTritiumTons"] != "400" || values["reserveTritiumTons"] != "50" || values["reserveShortfallTons"] != "0" || values["fuelCost"] != "5 000 000" || values["totalMinutes"] == "—" {
		t.Fatal("fuel budget results hidden", values)
	}
	if !coreAction(t, "route", "Итоги", "route.visited", s, p.Rows[0].Data, nil).Disabled {
		t.Fatal("totals row can mark nonexistent stop visited")
	}
	services := Build("carrier", "Службы", "", s)
	if len(services.Rows) != 1 || !reflect.DeepEqual(services.Rows[0].Cells[:4], []string{"Ремонт", "Да", "Нет", "Механик"}) {
		t.Fatal("service installation/suspension lost", services.Rows)
	}
	finance := Build("carrier", "Финансы", "", s)
	values = map[string]string{}
	for _, r := range finance.Rows {
		values[r.ID] = r.Cells[1]
	}
	if values["finance.reserveBalance"] != "1 000" || values["finance.availableBalance"] != "9 000" {
		t.Fatal("finance balances hidden", values)
	}
	empty := Build("route", "Итоги", "", nil)
	for _, r := range empty.Rows {
		if r.Cells[1] != "—" {
			t.Fatal("unknown route value treated as zero", r)
		}
	}
}
