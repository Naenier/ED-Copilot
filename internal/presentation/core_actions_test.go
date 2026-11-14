package presentation

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/carrier"
	"github.com/Naenier/ED-Copilot/internal/engineering"
	"github.com/Naenier/ED-Copilot/internal/mining"
	"github.com/Naenier/ED-Copilot/internal/theme"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func coreState() M {
	ledger := trading.New(nil).Snapshot(nil)
	appearance := theme.Default()
	palette, _ := theme.Palette(appearance)
	return M{"commander": M{"system": "Sol", "vehicle": "ship", "cargoCapacity": 100}, "settings": M{"minTier": "medium", "method": "all", "radiusLy": 50, "maxArrivalLs": 10000, "minLaserPercent": 15, "overlayEnabled": true, "hideDepleted": true}, "goods": ledger["goods"], "resources": trading.Resources(), "theme": M{"settings": appearance, "palette": palette}, "engineering": engineering.New(nil).Snapshot(nil), "mining": M{"options": mining.DefaultPlan()}, "carrierRoute": M{"options": carrier.DefaultRouteOptions()}, "carrier": M{"planning": M{"reserveTritium": 200, "targetTankTons": 1000}, "fuel": M{"tankTons": 500, "reserveTons": 100}, "warehouse": M{"complete": false}}}
}
func coreAction(t *testing.T, tab, mode, id string, state, selected, prefill M) Action {
	t.Helper()
	for _, a := range CoreActions(tab, mode, state, selected, prefill) {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("%s/%s missing action %s", tab, mode, id)
	return Action{}
}
func coreField(t *testing.T, a Action, key string) Field {
	t.Helper()
	for _, f := range a.Fields {
		if f.Key == key {
			return f
		}
	}
	t.Fatalf("%s missing field %s", a.ID, key)
	return Field{}
}
func coreBody(t *testing.T, a Action, overrides M) M {
	t.Helper()
	values := M{}
	for _, f := range a.Fields {
		raw := any(FieldText(f))
		if f.Kind == "bool" {
			raw = v.Bool(f.Default)
		}
		if f.Kind == "select" {
			raw = v.Str(f.Default)
			found := false
			for _, c := range f.Choices {
				found = found || c.Value == raw
			}
			if !found && len(f.Choices) > 0 {
				raw = f.Choices[0].Value
			}
		}
		if override, ok := overrides[f.Key]; ok {
			raw = override
		}
		value, err := ParseField(f, raw)
		if err != nil {
			t.Fatalf("%s.%s: %v", a.ID, f.Key, err)
		}
		values[f.Key] = value
	}
	for key := range overrides {
		if _, ok := values[key]; !ok {
			t.Fatalf("%s has no overridden field %s", a.ID, key)
		}
	}
	body, err := a.Body(values)
	if err != nil {
		t.Fatalf("%s: %v", a.ID, err)
	}
	return body
}
func TestCoreActionsEveryTabAndVisibleMode(t *testing.T) {
	s := coreState()
	selected := M{"id": "selected", "key": "gold", "system": "Sol", "title": "Test goal", "kind": "trade", "moduleKey": "am", "blueprintKey": "AFM_Shielded", "rolls": []int{1, 2, 3, 4, 5}, "index": 1, "quotes": []M{{"key": "gold", "price": 200, "quantity": 10}}, "sourceIds": []string{"manual"}, "url": "https://spansh.co.uk"}
	kinds := map[string]bool{"": true, "export": true, "copy": true, "navigate": true, "purchase": true, "url": true, "details": true, "help": true, "theme-import": true, "folder": true, "import-plan": true, "tabs-reset": true, "engineering-trade": true}
	fieldKinds := map[string]bool{"text": true, "password": true, "number": true, "integer": true, "bool": true, "select": true, "combo": true, "datetime": true, "multiline": true, "color": true, "file": true}
	if len(Tabs) != 28 {
		t.Fatal("tab coverage changed", len(Tabs))
	}
	count := 0
	for _, tab := range Tabs {
		page := Build(tab.ID, "", "", s)
		modes := page.Modes
		if len(modes) == 0 {
			modes = []string{""}
		}
		for _, mode := range modes {
			t.Run(tab.ID+"/"+mode, func(t *testing.T) {
				for _, row := range []M{nil, selected} {
					actions := CoreActions(tab.ID, mode, s, row, nil)
					if SectionSevenCustom(tab.ID, mode) || tab.ID == "engineering" && (mode == EngineeringProgression || mode == EngineeringOdyssey) {
						if len(actions) != 0 {
							t.Fatal("custom page received legacy actions")
						}
						continue
					}
					if len(actions) == 0 {
						t.Fatal("page has no actions")
					}
					seen := map[string]bool{}
					for _, a := range actions {
						count++
						if a.ID == "" || a.Title == "" || seen[a.ID] || !kinds[a.Kind] {
							t.Fatalf("invalid action %+v", a)
						}
						seen[a.ID] = true
						if a.Kind == "" && a.Path == "" {
							t.Fatal("local action missing command", a.ID)
						}
						keys := map[string]bool{}
						for _, f := range a.Fields {
							if f.Key == "" || f.Label == "" || keys[f.Key] || !fieldKinds[f.Kind] {
								t.Fatalf("invalid field in %s: %+v", a.ID, f)
							}
							keys[f.Key] = true
							choiceKeys := map[string]bool{}
							for _, c := range f.Choices {
								if c.Label == "" || choiceKeys[c.Value] {
									t.Fatalf("invalid choices %s.%s", a.ID, f.Key)
								}
								choiceKeys[c.Value] = true
							}
						}
						text := strings.ToLower(a.Title + " " + a.Path)
						if strings.Contains(text, "edmc") || strings.Contains(text, "client id") || strings.Contains(text, "oauth") {
							t.Fatal("developer-only control exposed", a.ID)
						}
					}
				}
			})
		}
	}
	if count < 200 {
		t.Fatal("insufficient mode/action coverage", count)
	}
}

func TestFrontierActionsKeepAuthenticationAndHideManualRefreshAndImport(t *testing.T) {
	s := coreState()
	account := CoreActions("account", "", s, nil, nil)
	login, logout := false, false
	for _, a := range account {
		login = login || a.ID == "frontier.login" && a.Path == "/api/frontier/login"
		logout = logout || a.ID == "frontier.logout" && a.Path == "/api/frontier/logout"
		if a.Path == "/api/frontier/sync" || a.ID == "frontier.sync" {
			t.Fatal("manual Frontier refresh remains in the user interface")
		}
	}
	if !login || !logout {
		t.Fatal("automatic refresh removed account connection controls")
	}
	for _, mode := range Build("carrier", "", "", s).Modes {
		for _, a := range CoreActions("carrier", mode, s, nil, nil) {
			if a.ID == "carrier.stock.frontier" || a.Path == "/api/frontier/carrier-stock" {
				t.Fatal("automatic carrier stock still requires a manual import action")
			}
		}
	}
}

func TestTradingNativeInputsAndSelectedQuoteDirections(t *testing.T) {
	s := coreState()
	s["pilot"] = M{"credits": 100000}
	s["tradePlan"] = M{"key": "gold", "buyPrice": 100, "sellPrice": 200, "source": "Old source", "destination": "Old destination"}
	selected := M{"id": "market:99", "system": "New system", "station": "New market", "quotes": []M{{"key": "gold", "quantity": 10, "price": 150}}}
	for _, mode := range []string{"Продать", "Купить"} {
		a := coreAction(t, "sale", mode, "market.search", s, selected, M{"resourceKey": "tritium", "quantity": 500})
		body := coreBody(t, a, nil)
		want := "sell"
		if mode == "Купить" {
			want = "buy"
		}
		if body["mode"] != want || body["resourceKey"] != "tritium" || v.Num(body["quantity"]) != 500 || v.Bool(body["useCargo"]) != (mode == "Продать") {
			t.Fatal(mode, body)
		}
		plan := coreBody(t, coreAction(t, "sale", mode, "trade.plan", s, selected, nil), nil)
		if mode == "Купить" {
			if v.Num(plan["buyPrice"]) != 150 || v.Num(plan["sellPrice"]) != 200 || plan["source"] != "New system" || plan["destination"] != "Old destination" {
				t.Fatal(plan)
			}
		} else {
			if v.Num(plan["buyPrice"]) != 100 || v.Num(plan["sellPrice"]) != 150 || plan["destination"] != "New system" {
				t.Fatal(plan)
			}
		}
		ledger := trading.New(nil)
		plan["action"] = "plan"
		if err := ledger.Update(plan); err != nil {
			t.Fatal(err)
		}
	}
	if !coreAction(t, "sale", "Продать", "market.destination", s, nil, nil).Disabled || coreAction(t, "sale", "Продать", "market.destination", s, selected, nil).Disabled {
		t.Fatal("market selection guard")
	}
	if !coreAction(t, "targets", "Места добычи", "target.bookmark", s, nil, nil).Disabled {
		t.Fatal("bookmark enabled without selection")
	}
	a := coreAction(t, "deals", "", "deals.search", s, nil, nil)
	body := coreBody(t, a, M{"sourcePower": "Nakato Kaine", "sourcePowerState": "Stronghold", "targetPower": "Zemina Torval", "targetPowerState": "Fortified"})
	if body["sourcePower"] != "Nakato Kaine" || body["sourcePowerState"] != "Stronghold" || body["targetPower"] != "Zemina Torval" || body["targetPowerState"] != "Fortified" || body["resourceKeys"] != nil {
		t.Fatal(body)
	}
	purchase := coreAction(t, "carrier", "", "carrier.tritium", s, nil, nil)
	if purchase.Kind != "purchase" || purchase.Path != "sale" || purchase.Payload["resourceKey"] != "tritium" {
		t.Fatal(purchase)
	}
	buy := coreBody(t, coreAction(t, "sale", "Купить", "market.search", s, nil, nil), nil)
	if buy["resourceKey"] != "tritium" {
		t.Fatal("default buy form hides its requested commodity", buy)
	}
	selected["quotes"] = []M{{"key": "tritium", "quantity": 10, "price": 50000}}
	newGood := coreBody(t, coreAction(t, "sale", "Продать", "trade.plan", s, selected, nil), nil)
	if v.Num(newGood["buyPrice"]) != 0 {
		t.Fatal("another commodity's purchase cost used for new plan", newGood)
	}
}

func TestEngineeringNativeRollTransformAndCompatibility(t *testing.T) {
	s := coreState()
	a := coreAction(t, "engineering", "План", "engineering.add", s, nil, nil)
	body := coreBody(t, a, M{"moduleKey": "am", "blueprintKey": "AFM_Shielded", "quantity": 2, "roll1": 1, "roll2": 2, "roll3": 3, "roll4": 4, "roll5": 5, "experimentalKey": ""})
	if !reflect.DeepEqual(body["rolls"], []any{1, 2, 3, 4, 5}) || body["roll1"] != nil || body["action"] != "add" {
		t.Fatal(body)
	}
	plan := engineering.New(nil)
	if err := plan.Update(body); err != nil {
		t.Fatal(err)
	}
	item := v.Rows(plan.Snapshot(nil)["items"])[0]
	edit := coreAction(t, "engineering", "План", "engineering.edit", s, item, nil)
	body = coreBody(t, edit, M{"roll1": 0, "roll2": 0, "roll3": 2, "roll4": 0, "roll5": 0, "quantity": 3})
	if err := plan.Update(body); err != nil {
		t.Fatal(err)
	}
	if v.Rows(plan.Snapshot(nil)["items"])[0]["id"] != item["id"] {
		t.Fatal("edit created another upgrade")
	}
	var incompatible string
	for _, bp := range v.Rows(v.At(s, "engineering", "catalog", "blueprints")) {
		if bp["key"] != "AFM_Shielded" {
			incompatible = v.Str(bp["key"])
			break
		}
	}
	invalid := v.Copy(body)
	invalid["blueprintKey"] = incompatible
	if err := plan.Update(invalid); err == nil {
		t.Fatal("incompatible module/blueprint accepted", incompatible)
	}
	if _, err := ParseField(coreField(t, a, "roll1"), "1.5"); err == nil {
		t.Fatal("fractional attempts accepted")
	}
	trade := coreBody(t, coreAction(t, "engineering", "Обмен", "engineering.trade", s, nil, nil), M{"tradeTargetKey": "carbon", "tradeTargetAmount": 10, "protectPlan": true, "showAllTrades": true})
	if err := plan.Update(trade); err != nil || v.At(plan.Snapshot(nil), "trader", "targetKey") != "carbon" {
		t.Fatal(trade, err)
	}
}

func TestMiningModesAndRouteFormMatchDomainContracts(t *testing.T) {
	s := coreState()
	a := coreAction(t, "mining", "План", "mining.plan", s, nil, nil)
	for _, choice := range coreField(t, a, "mode").Choices {
		body := coreBody(t, a, M{"mode": choice.Value, "resourceKey": "gold", "targetTons": 100, "purchasePrice": 100, "sellPrice": 200})
		p := mining.New(nil)
		if err := p.Update(body); err != nil || p.Options["mode"] != choice.Value {
			t.Fatal(choice, body, err)
		}
	}
	raw := false
	for _, c := range coreField(t, a, "resourceKey").Choices {
		raw = raw || c.Value == "polonium"
	}
	if !raw {
		t.Fatal("raw material plan cannot be selected")
	}
	for _, field := range []string{"resourceKey"} {
		market := coreAction(t, "sale", "Купить", "market.search", s, nil, nil)
		for _, c := range coreField(t, market, field).Choices {
			if c.Value == "polonium" {
				t.Fatal("raw material offered as market commodity")
			}
		}
	}
	route := coreAction(t, "route", "", "route.calculate", s, nil, nil)
	for _, choice := range coreField(t, route, "refuelMethod").Choices {
		body := coreBody(t, route, M{"sourceSystem": "A", "destinationSystem": "C", "waypoints": "B\r\nOther", "refuelSystems": "B", "refuelMethod": choice.Value, "usedCapacityTons": 100, "cargoTritiumTons": 100, "departureTime": "07.11.2026 21:45"})
		options, err := carrier.NormalizeRouteOptions(body, s)
		if err != nil {
			t.Fatal(choice, body, err)
		}
		if !reflect.DeepEqual(options["waypoints"], []string{"B", "Other"}) || options["refuelMethod"] != choice.Value {
			t.Fatal(options)
		}
	}
	s["carrierRoute"] = carrier.EmptyRouteState()
	v.Obj(s["carrier"])["space"] = M{"capacityTons": 25000, "freeTons": 24000}
	route = coreAction(t, "route", "", "route.calculate", s, nil, nil)
	body := coreBody(t, route, M{"destinationSystem": "C"})
	if v.Num(body["usedCapacityTons"]) != 1000 || v.Num(body["tankTons"]) != 500 || v.Num(body["cargoTritiumTons"]) != 100 || v.Num(body["reserveTritiumTons"]) != 200 {
		t.Fatal("idle route defaults masked observed carrier state", body)
	}
	if _, err := carrier.NormalizeRouteOptions(body, s); err != nil {
		t.Fatal(body, err)
	}
	s["carrierRoute"] = carrier.NewRoutePlanner(M{"version": 1, "state": M{"options": M{"sourceSystem": "Saved origin", "destinationSystem": "Saved destination", "tankTons": 300, "usedCapacityTons": 1000}}}).Snapshot()
	resumed := coreBody(t, coreAction(t, "route", "", "route.calculate", s, nil, nil), nil)
	if resumed["sourceSystem"] != "Saved origin" || resumed["destinationSystem"] != "Saved destination" || v.Num(resumed["tankTons"]) != 300 {
		t.Fatal("idle route form discarded resumed draft", resumed)
	}
}

func TestPaletteNativeTransformAndManualPriceClear(t *testing.T) {
	s := coreState()
	a := coreAction(t, "settings", "", "settings.palette", s, nil, nil)
	for _, preset := range []string{"orange", "green", "purple", "blue"} {
		body := coreBody(t, a, M{"mode": "preset", "preset": preset})
		settings, err := theme.Normalize(v.Obj(body["settings"]))
		if err != nil {
			t.Fatal(body, err)
		}
		want, _ := theme.Normalize(M{"preset": preset})
		if !reflect.DeepEqual(settings["colors"], want["colors"]) {
			t.Fatal("previous custom colors override chosen preset", preset, settings)
		}
		palette, err := theme.Palette(settings)
		if err != nil || palette["accent"] != v.At(want, "colors", "accent") {
			t.Fatal(palette, err)
		}
	}
	body := coreBody(t, a, M{"mode": "custom", "accent": "#Aa12FF"})
	palette, err := theme.Palette(v.Obj(body["settings"]))
	if err != nil || palette["accent"] != "#aa12ff" {
		t.Fatal(palette, err)
	}
	body = coreBody(t, a, M{"mode": "matrix", "matrix00": 0, "matrix01": 1, "matrix10": 1, "matrix11": 0})
	palette, err = theme.Palette(v.Obj(body["settings"]))
	if err != nil || palette["accent"] != "#7fff00" {
		t.Fatal(palette, err)
	}
	price := coreBody(t, coreAction(t, "settings", "", "settings.prices", s, nil, nil), M{"key": "gold", "price": ""})
	rows := v.Rows(price["prices"])
	if len(rows) != 1 || rows[0]["key"] != "gold" || rows[0]["price"] != nil {
		t.Fatal("blank manual price did not clear override", price)
	}
	encoded, _ := json.Marshal(body)
	if strings.Contains(string(encoded), "matrix00") {
		t.Fatal("form-only matrix fields escaped transform")
	}
}
