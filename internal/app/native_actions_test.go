package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func nativeAction(t *testing.T, e *Engine, tab, mode, id string, selected M) p.Action {
	t.Helper()
	for _, a := range p.CoreActions(tab, mode, e.Snapshot(), selected, nil) {
		if a.ID == id {
			if a.Disabled {
				t.Fatalf("native action %s unexpectedly disabled", id)
			}
			return a
		}
	}
	t.Fatalf("native action %s missing", id)
	return p.Action{}
}
func nativeBody(t *testing.T, a p.Action, overrides M) M {
	t.Helper()
	values := M{}
	for _, f := range a.Fields {
		raw := any(p.FieldText(f))
		if f.Kind == "bool" {
			raw = v.Bool(f.Default)
		}
		if f.Kind == "select" {
			raw = v.Str(f.Default)
			found := false
			for _, choice := range f.Choices {
				found = found || choice.Value == raw
			}
			if !found && len(f.Choices) > 0 {
				raw = f.Choices[0].Value
			}
		}
		if override, ok := overrides[f.Key]; ok {
			raw = override
		}
		value, err := p.ParseField(f, raw)
		if err != nil {
			t.Fatalf("%s.%s: %v", a.ID, f.Key, err)
		}
		values[f.Key] = value
	}
	for k := range overrides {
		if _, ok := values[k]; !ok {
			t.Fatalf("%s does not contain %s", a.ID, k)
		}
	}
	body, err := a.Body(values)
	if err != nil {
		t.Fatalf("%s: %v", a.ID, err)
	}
	return body
}
func runNative(t *testing.T, e *Engine, tab, mode, id string, selected, overrides M) M {
	t.Helper()
	a := nativeAction(t, e, tab, mode, id, selected)
	return doOK(t, e, a.Path, nativeBody(t, a, overrides))
}

func TestNativeFormsDriveGoalsTradingMiningAndEngineering(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	runNative(t, e, "overview", "Планы", "goal.add", nil, M{"title": "Доставить тритий", "kind": "carrier", "system": "Sol", "priority": "high", "notes": "Синтетический план"})
	goal := v.Rows(e.Snapshot()["objectives"])[0]
	runNative(t, e, "targets", "Личные цели", "goal.edit", goal, M{"title": "Купить и доставить тритий"})
	runNative(t, e, "targets", "Личные цели", "goal.complete", goal, nil)
	if v.Rows(e.Snapshot()["objectives"])[0]["status"] != "completed" {
		t.Fatal("goal completion not reflected")
	}
	runNative(t, e, "targets", "Личные цели", "goal.reopen", goal, nil)
	selected := M{"station": "Synthetic Seller", "system": "Source", "quotes": []M{{"key": "gold", "quantity": 10, "price": 100}}}
	runNative(t, e, "sale", "Купить", "trade.plan", selected, M{"sellPrice": 150})
	expectNumber(t, v.At(e.Snapshot(), "tradePlan", "grossMargin"), 500)
	if v.At(e.Snapshot(), "tradePlan", "source") != "Source" {
		t.Fatal("purchase market not selected as origin")
	}
	runNative(t, e, "sale", "Продать", "market.destination", selected, nil)
	if v.At(e.Snapshot(), "trading", "destination", "station") != "Synthetic Seller" {
		t.Fatal("market target not reflected")
	}
	runNative(t, e, "sale", "Продать", "market.clear", nil, nil)
	if v.At(e.Snapshot(), "trading", "destination") != nil {
		t.Fatal("market destination not cleared")
	}
	runNative(t, e, "mining", "План", "mining.plan", nil, M{"mode": "trade", "resourceKey": "gold", "targetTons": 100, "purchasePrice": 100, "sellPrice": 200, "operatingCost": 100})
	state := e.Snapshot()
	if v.At(state, "mining", "options", "mode") != "trade" {
		t.Fatal("native trade mode silently ignored")
	}
	expectNumber(t, v.At(state, "mining", "plan", "profit"), 9900)
	runNative(t, e, "mining", "План", "mining.plan", nil, M{"mode": "mining", "method": "raw", "resourceKey": "polonium"})
	if v.At(e.Snapshot(), "mining", "plan", "applicable") != false {
		t.Fatal("engineering material assigned market revenue")
	}
	runNative(t, e, "engineering", "План", "engineering.add", nil, M{"moduleKey": "am", "blueprintKey": "AFM_Shielded", "quantity": 2, "experimentalKey": "", "roll1": 1, "roll2": 2, "roll3": 3, "roll4": 4, "roll5": 5})
	items := v.Rows(v.At(e.Snapshot(), "engineering", "items"))
	if len(items) != 1 {
		t.Fatal(items)
	}
	runNative(t, e, "engineering", "План", "engineering.edit", items[0], M{"quantity": 3, "roll1": 0, "roll2": 0, "roll3": 1, "roll4": 0, "roll5": 0})
	items = v.Rows(v.At(e.Snapshot(), "engineering", "items"))
	expectNumber(t, items[0]["quantity"], 3)
	if v.Num(v.At(e.Snapshot(), "engineering", "shopping", "neededUnits")) <= 0 {
		t.Fatal("upgrade did not populate required materials")
	}
	runNative(t, e, "engineering", "Обмен", "engineering.trade", nil, M{"tradeTargetKey": "carbon", "tradeTargetAmount": 10, "protectPlan": true, "showAllTrades": true})
	if v.At(e.Snapshot(), "engineering", "trader", "targetKey") != "carbon" {
		t.Fatal("material trader settings dropped")
	}
	engineer := v.Rows(v.At(e.Snapshot(), "engineering", "engineers"))[0]
	runNative(t, e, "engineering", "Инженеры", "engineer.mark", engineer, M{"unlocked": true})
	if v.Rows(v.At(e.Snapshot(), "engineering", "engineers"))[0]["status"] != "unlocked" {
		t.Fatal("manual engineer access not reflected")
	}
	runNative(t, e, "engineering", "Инженеры", "engineer.reset", engineer, nil)
	runNative(t, e, "engineering", "План", "engineering.remove", items[0], nil)
	if len(v.Rows(v.At(e.Snapshot(), "engineering", "items"))) != 0 {
		t.Fatal("upgrade not removed")
	}
	runNative(t, e, "engineering", "План", "engineering.clear", nil, nil)
	runNative(t, e, "targets", "Личные цели", "goal.delete", goal, nil)
	if len(v.Rows(e.Snapshot()["objectives"])) != 0 {
		t.Fatal("personal goal not removed")
	}
}

func TestNativeFormsDriveSettingsPaletteAndCarrierStock(t *testing.T) {
	e, file, dir := fixtureEngine(t, nil)
	runNative(t, e, "settings", "", "settings.filters", nil, M{"minTier": "high", "method": "core", "radiusLy": 75, "maxArrivalLs": 5000, "minLaserPercent": 20, "hideDepleted": false, "overlayEnabled": false})
	if v.At(e.Snapshot(), "settings", "minTier") != "high" || v.At(e.Snapshot(), "settings", "overlayEnabled") != false {
		t.Fatal("native settings not applied")
	}
	for _, mode := range []string{"fullscreen", "borderless", "windowed"} {
		runNative(t, e, "settings", "", "settings.window", nil, M{"displayMode": mode})
		if v.At(e.Snapshot(), "desktop", "displayMode") != mode {
			t.Fatal("display mode dropped", mode)
		}
	}
	runNative(t, e, "settings", "", "settings.palette", nil, M{"mode": "preset", "preset": "orange"})
	if v.At(e.Snapshot(), "theme", "palette", "accent") != "#ff8a24" || v.At(e.Snapshot(), "theme", "settings", "colors", "accent") != "#ff8a24" {
		t.Fatal("preset retained previous blue colours")
	}
	runNative(t, e, "settings", "", "settings.palette", nil, M{"mode": "custom", "accent": "#12AaFF"})
	if v.At(e.Snapshot(), "theme", "palette", "accent") != "#12aaff" {
		t.Fatal("custom colour not applied")
	}
	runNative(t, e, "settings", "", "settings.prices", nil, M{"key": "gold", "price": 123456})
	found := false
	for _, r := range v.Rows(e.Snapshot()["goods"]) {
		if r["key"] == "gold" {
			found = v.Num(r["manualPrice"]) == 123456
		}
	}
	if !found {
		t.Fatal("manual estimate missing")
	}
	runNative(t, e, "settings", "", "settings.prices", nil, M{"key": "gold", "price": ""})
	for _, r := range v.Rows(e.Snapshot()["goods"]) {
		if r["key"] == "gold" && r["manualPrice"] != nil {
			t.Fatal("blank manual estimate not cleared")
		}
	}
	paletteFile := filepath.Join(dir, "GraphicsConfigurationOverride.xml")
	const paletteXML = `<GraphicsConfig><GUIColour><Default><MatrixRed>0,1,0</MatrixRed><MatrixGreen>1,0,0</MatrixGreen><MatrixBlue>0,0,1</MatrixBlue></Default></GUIColour></GraphicsConfig>`
	if err := os.WriteFile(paletteFile, []byte(paletteXML), 0600); err != nil {
		t.Fatal(err)
	}
	imported := runNative(t, e, "settings", "", "settings.import.palette", nil, M{"filePath": paletteFile})
	doOK(t, e, "/api/theme", M{"settings": v.At(imported, "theme", "settings")})
	if v.At(e.Snapshot(), "theme", "settings", "mode") != "game" {
		t.Fatal("imported HUD theme not applied", imported)
	}
	if b, err := os.ReadFile(paletteFile); err != nil || string(b) != paletteXML {
		t.Fatal("theme import modified source file", err)
	}
	writeEvents(t, file, true, []M{testEvent("CarrierStats", M{"CarrierID": "42", "Name": "Synthetic Carrier", "Callsign": "SYN-42", "FuelLevel": 500, "SpaceUsage": M{"TotalCapacity": 25000, "FreeSpace": 24000, "Cargo": 1000}})})
	e.Reader.Poll()
	runNative(t, e, "carrier", "Состояние", "carrier.planning", nil, M{"weeklyUpkeep": 10000000, "reserveTritium": 200, "targetTankTons": 800})
	expectNumber(t, v.At(e.Snapshot(), "carrier", "planning", "reserveTritium"), 200)
	runNative(t, e, "carrier", "Склад", "carrier.stock", nil, M{"key": "gold", "count": 12, "complete": false})
	e.Reader.Poll()
	if v.At(e.Snapshot(), "carrier", "warehouse", "complete") != false || v.At(e.Snapshot(), "carrier", "fuel", "reserveTons") != nil {
		t.Fatal("partial stock falsely complete")
	}
	runNative(t, e, "carrier", "Склад", "carrier.stock", nil, M{"key": "tritium", "count": 100, "complete": false})
	e.Reader.Poll()
	warehouse := v.Rows(v.At(e.Snapshot(), "carrier", "warehouse", "items"))
	if len(warehouse) != 2 {
		t.Fatal("stock-row upsert dropped known item", warehouse)
	}
	expectNumber(t, v.At(e.Snapshot(), "carrier", "fuel", "reserveTons"), 100)
	a := nativeAction(t, e, "carrier", "Склад", "carrier.stock.frontier", nil)
	if _, err := e.Do(a.Path, nativeBody(t, a, nil)); err == nil {
		t.Fatal("unconfirmed Frontier warehouse snapshot accepted")
	}
	runNative(t, e, "settings", "", "settings.rescan", nil, nil)
	e.Reader.Poll()
	for _, kind := range []string{"statistics", "trading", "engineering"} {
		csv, err := e.ExportCSV(kind)
		if err != nil || !strings.HasPrefix(csv, "\ufeff") {
			t.Fatal(kind, err)
		}
	}
}
