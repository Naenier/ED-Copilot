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

func TestPilotAndStatisticsSummaryRetainObservedValues(t *testing.T) {
	s := presentationState(t)
	pilot := v.Obj(s["pilot"])
	pilot["fuel"] = M{"mainTons": 8, "mainCapacityTons": 16, "updatedAt": "2026-11-07T12:00:00Z"}
	pilot["session"] = M{"since": "2026-11-07T11:00:00Z", "jumps": 3, "distanceLy": 42.5}
	pilot["legalState"] = "Wanted"
	values := map[string]string{}
	for _, r := range Build("overview", "Состояние", "", s).Rows {
		values[r.ID] = r.Cells[1]
	}
	for key, want := range map[string]string{"pilot.fuel.mainTons": "8", "pilot.fuel.mainCapacityTons": "16", "pilot.session.jumps": "3", "pilot.session.distanceLy": "42,5", "pilot.legalState": "В розыске"} {
		if values[key] != want {
			t.Fatal(key, values[key], want)
		}
	}
	values = map[string]string{}
	for _, r := range Build("statistics", "Итоги", "", s).Rows {
		values[r.ID] = r.Cells[1]
	}
	if values["total.refinedTons"] != "15" || values["total.prospected"] != "1" || values["since"] == "—" {
		t.Fatal("observed period totals hidden", values)
	}
	for _, r := range Build("overview", "Состояние", "", nil).Rows {
		if r.ID != "ship" && r.Cells[1] != "—" {
			t.Fatal("unknown observed value became zero", r)
		}
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
	a := coreAction(t, "engineering", "Данные плана", "engineering.material-trade", s, selected, nil)
	body, err := a.Body(nil)
	if err != nil || a.Kind != "engineering-trade" || a.Disabled || body["tradeTargetKey"] != selected["key"] || v.Num(body["tradeTargetAmount"]) != 1 {
		t.Fatal("quick exchange missing material/quantity", a, body, err)
	}
	planner := engineering.New(nil)
	if err = planner.Update(body); err != nil {
		t.Fatal(err)
	}
	if v.At(planner.Snapshot(nil), "trader", "targetKey") != selected["key"] {
		t.Fatal("exchange target did not update domain")
	}
	if !coreAction(t, "engineering", "Материалы", "engineering.material-trade", s, nil, nil).Disabled {
		t.Fatal("exchange enabled without material")
	}
	copy := coreAction(t, "engineering", "Материалы плана", "engineering.copy-shopping", s, nil, nil)
	if copy.Kind != "copy" || copy.Disabled || !strings.Contains(copy.Path, v.Str(selected["name"])) {
		t.Fatal("shopping list copy lost", copy)
	}
	if len(Build("engineering", "Материалы", "СЫРЬЕ", s).Rows) == 0 {
		t.Fatal("Russian category search unavailable")
	}
}

func TestMarketWarningsAndExpiredQuoteRemainVisible(t *testing.T) {
	s := presentationState(t)
	market := v.Rows(v.At(s, "trading", "results"))[0]
	market["expired"] = true
	market["warnings"] = []any{"Спрос ниже объёма груза", M{"text": "Флотоносец мог переместиться"}}
	page := Build("sale", "Продать", "", s)
	if !strings.HasPrefix(page.Rows[0].Cells[5], "Устарела") || !strings.Contains(page.Rows[0].Details, "Спрос ниже объёма груза") || !strings.Contains(page.Rows[0].Details, "Флотоносец мог переместиться") || !strings.Contains(page.Rows[0].Details, "повторите поиск") {
		t.Fatal("market risk suppressed", page.Rows[0])
	}
}

func TestTradeRouteEachLegCanCopyBuyAndSellAndSetDestination(t *testing.T) {
	s := presentationState(t)
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
	for i, tc := range []struct {
		suffix, buy, sell string
		quantity          float64
	}{{"0", "Система закупки", "Система продажи", 100}, {"1", "Обратная закупка", "Дом", 50}} {
		if got := coreAction(t, "deals", "", "deals.copy-buy."+tc.suffix, s, selected, nil); got.Disabled || got.Path != tc.buy {
			t.Fatal(got)
		}
		if got := coreAction(t, "deals", "", "deals.copy-sell."+tc.suffix, s, selected, nil); got.Disabled || got.Path != tc.sell {
			t.Fatal(got)
		}
		a := coreAction(t, "deals", "", "deals.destination."+tc.suffix, s, selected, nil)
		body, err := a.Body(nil)
		if err != nil || a.Disabled || a.Path != "/api/trading/destination" || v.At(body, "destination", "system") != tc.sell {
			t.Fatal("route buyer did not reach HUD", i, a, body)
		}
		purchase := coreAction(t, "deals", "", "deals.purchase."+tc.suffix, s, selected, nil)
		prefill, err := PurchasePrefill(purchase, M{"quantity": 75, "system": "Уточнённая система"})
		if err != nil || purchase.Kind != "purchase" || prefill["resourceKey"] != "platinum" || v.Num(prefill["quantity"]) != 75 || prefill["system"] != "Уточнённая система" || v.Bool(prefill["useCargo"]) || v.Bool(prefill["useCarrierCargo"]) {
			t.Fatal("purchase edit discarded", prefill, err)
		}
		if v.Num(purchase.Payload["quantity"]) != tc.quantity {
			t.Fatal("another leg quantity used")
		}
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
