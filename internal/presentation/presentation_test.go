package presentation

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/carrier"
	"github.com/Naenier/ED-Copilot/internal/colonization"
	"github.com/Naenier/ED-Copilot/internal/data"
	"github.com/Naenier/ED-Copilot/internal/engineering"
	"github.com/Naenier/ED-Copilot/internal/learning"
	"github.com/Naenier/ED-Copilot/internal/mining"
	"github.com/Naenier/ED-Copilot/internal/pilot"
	"github.com/Naenier/ED-Copilot/internal/politics"
	"github.com/Naenier/ED-Copilot/internal/ranks"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// Generate fixtures through the real modules so a schema change cannot leave
// apparently passing presentation tests that merely repeat a stale UI schema.
func presentationState(t *testing.T) M {
	t.Helper()
	at := "2026-11-07T12:00:00Z"
	cmd := M{"system": "Synthetic A", "body": "A 1", "ship": "Python Mk II", "shipName": "Звёздный странник", "shipIdent": "RU-01", "vehicle": "ship", "cargoCapacity": 100, "cargoTons": 20, "cargoUpdatedAt": at, "cargo": []M{{"key": "platinum", "name": "Платина", "count": 10}, {"key": "drones", "name": "Дроны", "count": 10}}, "cargoForSync": []M{{"Name": "steel", "Count": 20}}}
	ctx := M{"commander": cmd, "resources": trading.Resources(), "now": "2026-11-07T12:07:00Z", "gameRunning": true, "settings": M{"method": "laser", "minTier": "low"}}
	event := func(kind string, values M) M { return v.Merge(M{"event": kind, "timestamp": at}, values) }
	p := pilot.New()
	for _, e := range []M{event("LoadGame", M{"Credits": 100000}), event("NavRoute", M{"Route": []M{{"StarSystem": "Synthetic A", "StarClass": "G"}, {"StarSystem": "Synthetic B", "StarClass": "K"}}}), event("MissionAccepted", M{"MissionID": 77, "LocalisedName": "Доставка груза", "DestinationSystem": "Synthetic B", "DestinationStation": "Порт назначения", "Expiry": "2026-11-08T12:00:00Z"})} {
		p.Apply(e, ctx)
	}
	m := mining.New(M{"options": M{"resourceKey": "platinum", "method": "laser", "targetTons": 100, "sellPrice": 1000, "tonsPerHour": 60, "tripMinutes": 10}})
	m.Apply(event("Loadout", M{"ShipID": 1, "CargoCapacity": 100, "Modules": []M{{"Item": "hpt_mininglaser_fixed_small"}, {"Item": "int_refinery_size2_class2"}, {"Item": "int_multidronecontrol_mining_size3_class2"}}}), ctx)
	m.Apply(event("MiningRefined", M{"Type": "Platinum", "Count": 5}), ctx)
	m.Apply(v.Merge(event("Docked", nil), M{"timestamp": "2026-11-07T12:02:00Z"}), ctx)
	m.Apply(v.Merge(event("ProspectedAsteroid", M{"Materials": []M{{"Name": "Platinum", "Proportion": 30}}, "Remaining": 100}), M{"timestamp": "2026-11-07T12:03:00Z"}), ctx)
	m.Apply(v.Merge(event("MiningRefined", M{"Type": "Platinum", "Count": 10}), M{"timestamp": "2026-11-07T12:04:00Z"}), ctx)
	ms := m.Snapshot(ctx)
	eng := engineering.New(M{"items": []M{{"id": "eng-one", "moduleKey": "am", "blueprintKey": "AFM_Shielded", "rolls": []any{1, 1, 0, 0, 0}, "quantity": 1}, {"id": "eng-two", "moduleKey": "t", "blueprintKey": "Engine_Dirty", "rolls": []any{1, 0, 0, 0, 0}, "quantity": 1}}, "options": M{"tradeTargetKey": "carbon", "tradeTargetAmount": 1, "showAllTrades": true}})
	eng.Apply(event("Materials", M{"Raw": []M{{"Name": "iron", "Count": 100}, {"Name": "carbon", "Count": 0}}, "Manufactured": []M{}, "Encoded": []M{}}), nil)
	c := carrier.New(nil)
	c.Apply(event("CarrierStats", M{"CarrierID": "42", "Name": "Новый дом", "Callsign": "AAA-111", "FuelLevel": 500, "Finance": M{"CarrierBalance": 10000, "ReserveBalance": 1000, "AvailableBalance": 9000}, "Crew": []M{{"CrewRole": "repair", "Activated": true, "Enabled": false, "CrewName": "Механик"}}}), nil)
	if err := c.SetStockBaseline(M{"items": []M{{"key": "tritium", "count": 100}}, "timestamp": at}); err != nil {
		t.Fatal(err)
	}
	c.Apply(event("CarrierTradeOrder", M{"CarrierID": "42", "Commodity": "tritium", "Price": 60000, "PurchaseOrder": 100}), nil)
	c.Apply(event("CarrierBankTransfer", M{"CarrierID": "42", "Deposit": 1000, "CarrierBalance": 11000}), nil)
	opts, err := carrier.NormalizeRouteOptions(M{"sourceSystem": "Synthetic A", "destinationSystem": "Synthetic B", "tankTons": 500, "calculateStartingFuel": false, "departureTime": at, "tritiumPrice": 50000, "reserveTritiumTons": 50}, nil)
	if err != nil {
		t.Fatal(err)
	}
	route, err := carrier.NormalizeProviderRoute(M{"status": "ok", "job": "12345678-1234-1234-1234-123456789012", "result": M{"capacity_used": 0, "fuel_loaded": 500, "tritium_stored": 0, "jumps": []M{{"name": "Synthetic A", "id64": "18446744073709551613", "x": 0, "y": 0, "z": 0, "distance": 0, "fuel_used": 0, "fuel_in_tank": 500, "tritium_in_market": 0, "restock_amount": 0}, {"name": "Synthetic B", "id64": "18446744073709551614", "x": 400, "y": 0, "z": 0, "distance": 400, "fuel_used": 100, "fuel_in_tank": 400, "tritium_in_market": 0, "restock_amount": 0}}}}, opts, M{"createdAt": at})
	if err != nil {
		t.Fatal(err)
	}
	col := colonization.New(nil)
	apply := func(body M) {
		t.Helper()
		if err := col.Update(body); err != nil {
			t.Fatal(err)
		}
	}
	apply(M{"action": "create-project", "targetSystem": "Synthetic colony", "name": "Дом", "status": "building"})
	projectID := v.At(col.Snapshot(nil), "activeProject", "id")
	for _, typ := range []string{"coriolis", "hub-refinery"} {
		apply(M{"action": "add-building", "projectId": projectID, "typeId": typ, "name": "Объект " + typ})
		bs := v.Rows(v.At(col.Snapshot(nil), "activeProject", "buildings"))
		apply(M{"action": "set-resource-plan", "projectId": projectID, "buildingId": bs[len(bs)-1]["id"], "resources": []M{{"commodity": "steel", "required": 100}}, "sourceNote": "Синтетический пример сметы"})
	}
	bs := v.Rows(v.At(col.Snapshot(nil), "activeProject", "buildings"))
	apply(M{"action": "record-delivery", "projectId": projectID, "buildingId": bs[0]["id"], "commodity": "steel", "count": 10, "timestamp": at})
	apply(M{"action": "move-supply-building", "projectId": projectID, "id": bs[1]["id"], "direction": "up"})
	colState := col.Snapshot(ctx)
	colState["search"] = M{"results": []M{{"id": "candidate", "name": "Synthetic candidate", "distanceLy": 9, "availability": "candidate", "updatedAt": at}}}
	ledger := trading.New(nil)
	ledger.Apply(event("MarketBuy", M{"Type": "Platinum", "Count": 10, "TotalCost": 1000}), ctx)
	ledger.Apply(event("MarketSell", M{"Type": "Platinum", "Count": 2, "TotalSale": 400}), ctx)
	ls := ledger.Snapshot(ctx)
	market := func(id, station string, price int) M {
		return M{"id": id, "marketId": id, "station": station, "system": "Synthetic A", "coordinates": M{"x": 0, "y": 0, "z": 0}, "distanceLy": 10, "updatedAt": at, "quotes": []M{{"key": "platinum", "name": "Платина", "price": price, "supply": 100, "demand": 1000}}}
	}
	buy, sell := market("1", "Порт закупки", 100), market("2", "Порт сбыта", 200)
	deals := trading.BuildRoutes(map[string]M{"platinum": {"buy": []M{buy}, "sell": []M{sell}}}, trading.RouteOptions(M{"cargoCapacity": 100, "credits": 100000}, cmd))
	return M{"commander": cmd, "pilot": p.Snapshot(), "mining": ms, "stats": ms["stats"], "engineering": eng.Snapshot(nil), "carrier": c.Snapshot(nil), "carrierRoute": route, "carrierTrade": carrier.NewTradePlanner(nil).Snapshot(), "colonization": colState, "tradeLedger": ls, "goods": ls["goods"], "trading": M{"results": []M{sell}}, "tradeRoutes": M{"results": deals}, "savedTradeRoutes": deals, "objectives": []M{{"id": "goal", "title": "Звёздный рейс", "kind": "trade", "priority": "high", "status": "active", "system": "Synthetic A", "notes": "Привезти сырьё"}}, "targets": []M{{"id": "target", "system": "Synthetic A", "body": "A 1", "ring": "A Ring", "tier": "high", "confidence": "confirmed", "distanceLy": 5}}, "connection": M{"journalDir": "C:\\Synthetic\\Journals"}, "settings": M{"activity": "trade"}, "theme": M{"settings": M{"preset": "blue"}}, "app": M{"profileDir": "C:\\Synthetic\\Profile"}, "frontier": M{"connected": true, "summary": M{"profile": M{"commander": "Synthetic CMDR", "credits": 100000, "system": "Synthetic A", "ship": "Python Mk II", "shipName": "Звёздный странник"}}, "lastSync": at}, "integrations": M{"apiKeySet": true, "enabled": true, "queued": 1, "sent": 4, "lastSuccess": at}}
}

func TestBuildEveryTabAndModeUsesNativeColumns(t *testing.T) {
	s := presentationState(t)
	s["exobiology"] = exobiologyPresentationFixture()
	lib := learning.New(nil)
	lessons := v.Rows(lib.Snapshot()["lessons"])
	for _, body := range []M{{"action": "select", "lessonId": lessons[0]["id"]}, {"action": "complete", "lessonId": lessons[0]["id"], "completed": true}, {"action": "bookmark", "lessonId": lessons[0]["id"], "bookmarked": true}, {"action": "select", "lessonId": lessons[1]["id"]}} {
		if err := lib.Update(body); err != nil {
			t.Fatal(err)
		}
	}
	s["learning"] = lib.Snapshot()
	rankTracker := ranks.New(nil)
	rankTracker.Apply(M{"event": "Rank", "timestamp": "2026-11-08T12:00:00Z", "Combat": 1, "Trade": 4}, M{"commander": M{"fid": "FSYNTHETIC"}})
	s["ranks"] = rankTracker.Snapshot()
	political := politics.New(nil, politics.Options{DisableNetwork: true})
	political.Apply(M{"event": "Location", "timestamp": "2026-11-08T12:00:00Z", "StarSystem": "Synthetic A", "SystemAddress": 1})
	s["politics"] = political.Snapshot()
	s["carrierCalculator"] = carrier.NewCalculator(nil).Snapshot()
	s["bookmarks"] = []M{{"id": "saved-one", "targetId": "target", "system": "Synthetic A", "body": "A 1", "ring": "A Ring", "kind": "laser", "addedAt": "2026-11-07T12:00:00Z"}}
	s["commanderProfile"] = M{"finance": M{"credits": 0}, "fleet": M{"known": true, "count": 1, "currentShip": M{"known": true, "ship": "Python"}, "entries": []M{{"shipId": "42", "ship": "Python", "system": "Synthetic A"}}}, "storedModules": M{"known": true, "count": 0, "entries": []M{}}, "inventory": M{"materials": M{"raw": M{"known": true, "count": 2, "entries": []M{{"key": "iron", "name": "Iron", "count": 2}}}}}, "statistics": M{"sections": []M{{"id": "Combat", "label": "Combat", "entries": []M{{"id": "Bounties_Claimed", "label": "Bounties Claimed", "value": 0}}}}}, "reputation": M{"federation": 0}}
	if len(v.Rows(v.At(s, "pilot", "recent"))) == 0 {
		v.Obj(s["pilot"])["recent"] = []M{{"timestamp": "2026-11-07T12:00:00Z", "event": "LoadGame", "label": "Начало сеанса", "system": "Synthetic A"}}
	}
	if len(Tabs) != 22 {
		t.Fatal("unexpected navigation scope", len(Tabs))
	}
	for _, tab := range Tabs {
		initial := Build(tab.ID, "", "", s)
		modes := append([]string{""}, initial.Modes...)
		for _, mode := range modes {
			t.Run(tab.ID+"/"+mode, func(t *testing.T) {
				p := Build(tab.ID, mode, "", s)
				if p.Title != Title(tab.ID) {
					t.Fatalf("lost page identity: title %q, expected %q", p.Title, Title(tab.ID))
				}
				if !reflect.DeepEqual(p.Modes, initial.Modes) {
					t.Fatalf("mode picker changed: %v -> %v", initial.Modes, p.Modes)
				}
				if len(p.Rows) == 0 {
					t.Fatal("populated domain fixture rendered empty")
				}
				for i, r := range p.Rows {
					if r.ID == "" {
						t.Fatalf("row %d cannot preserve native selection without identity", i)
					}
					if len(r.Cells) != len(p.Columns) {
						t.Fatalf("row %d has %d cells for %d columns", i, len(r.Cells), len(p.Columns))
					}
					for _, cell := range r.Cells {
						if strings.TrimSpace(cell) == "" {
							t.Fatalf("row %d has blank unknown value", i)
						}
					}
				}
			})
		}
	}
}

func TestPresentationActualDomainContracts(t *testing.T) {
	s := presentationState(t)
	s["exobiology"] = exobiologyPresentationFixture()
	deals := Build("deals", "", "", s)
	if len(deals.Rows) != 1 || !reflect.DeepEqual(deals.Rows[0].Cells, []string{"Synthetic A → Synthetic A", "Порт закупки → Порт сбыта", "Платина · 100 т", "10 000", "10 000", "40 000"}) || !strings.Contains(deals.Rows[0].Details, "Станция: Порт закупки") || !strings.Contains(deals.Rows[0].Details, "Спрос с запасом, т: 250") {
		t.Fatalf("nested route legs lost: %+v", deals.Rows)
	}
	route := Build("route", "", "", s)
	if route.Rows[1].Cells[4] != "400" || !strings.Contains(route.Summary, "Расход трития, т: 100") {
		t.Fatalf("route fuel schema lost: %s / %+v", route.Summary, route.Rows)
	}
	orders := Build("carrier", "Торговые приказы", "", s)
	if orders.Rows[0].Cells[1] != "Покупка" {
		t.Fatal(orders.Rows)
	}
	if got := Build("sale", "Купить", "", s); got.Rows[0].Cells[5] != "100" || got.Columns[5] != "Запас, т" || got.Columns[2] != "Площадка" {
		t.Fatal("purchase displayed demand rather than supply", got.Rows)
	}
	if got := Build("sale", "Продать", "", s); got.Rows[0].Cells[5] != "1 000" || got.Columns[5] != "Спрос, т" {
		t.Fatal("sale displayed supply rather than demand", got.Rows)
	}
	warehouse := Build("carrier", "Склад", "", s)
	if warehouse.Rows[0].Cells[2] == "—" {
		t.Fatal("warehouse source missing")
	}
	transactions := Build("carrier", "История", "", s)
	if transactions.Rows[0].Cells[2] != "1 000" || transactions.Rows[0].Cells[1] == "—" {
		t.Fatal(transactions.Rows)
	}
	current := Build("mining", "Текущий заход", "", s)
	if current.Rows[2].Cells[1] != "1" || current.Rows[5].Cells[1] == "—" || current.Rows[6].Cells[1] == "—" {
		t.Fatalf("mining estimate schema lost: %+v", current.Rows[:7])
	}
	plan := Build("mining", "План", "", s)
	if len(plan.Rows) != 14 || plan.Rows[8].Cells[1] == "—" {
		t.Fatal("missing full income plan")
	}
	build := Build("colonization", "Строительство", "", s)
	supply := Build("colonization", "Очередь снабжения", "", s)
	if build.Rows[0].ID == supply.Rows[0].ID || build.Rows[0].ID != supply.Rows[1].ID {
		t.Fatal("independent queue lost")
	}
	if got := Label("journal+contribution"); got != "Журнал игры + новые поставки" {
		t.Fatal(got)
	}
}

func TestUnknownFormattingAndCyrillicSearch(t *testing.T) {
	for _, fn := range []func(any) string{Format, Number, Date, Label} {
		if fn(nil) != "—" {
			t.Fatal("unknown is not an em dash")
		}
	}
	if Number(1234567.5) != "1 234 567,5" || Number(-12345) != "-12 345" || Format(false) != "Нет" {
		t.Fatal("Russian formats changed")
	}
	if Ship(M{"ship": "Python Mk II", "shipName": "Дом", "shipIdent": "AA-01"}) != "Python Mk II «Дом» · AA-01" {
		t.Fatal("ship identity composition")
	}
	if got := Format("2026-11-07T12:00:00Z"); got != Date("2026-11-07T12:00:00Z") {
		t.Fatal("timestamp not localized")
	}
	if !contains("  ЗВЕЗДНЫЙ  ", "ЗВЁЗДНЫЙ") || !contains("СЫРЬЁ", "сырье") || contains("xyz", "Звёздный") {
		t.Fatal("Cyrillic search folding")
	}
	s := presentationState(t)
	s["exobiology"] = exobiologyPresentationFixture()
	for _, q := range []string{"ЗВЕЗДНЫЙ", "звёздный", "СЫРЬЕ", "сырьё"} {
		if len(Build("targets", "Личные цели", q, s).Rows) != 1 {
			t.Fatalf("missing search %q", q)
		}
	}
	for _, tab := range Tabs {
		for _, mode := range append([]string{""}, Build(tab.ID, "", "", nil).Modes...) {
			p := Build(tab.ID, mode, "", nil)
			for _, r := range p.Rows {
				for _, cell := range r.Cells {
					if strings.TrimSpace(cell) == "" {
						t.Fatalf("%s/%s blank value", tab.ID, mode)
					}
				}
			}
		}
	}
	if Age(nil, time.Now()) != "Возраст неизвестен" || Age("2026-11-07T12:00:00Z", time.Date(2026, 11, 7, 12, 30, 0, 0, time.UTC)) != "30 мин назад" {
		t.Fatal("age formatting")
	}
}

func TestNativeRowIdentityPreservesSelectionAcrossRefresh(t *testing.T) {
	a := row(M{"timestamp": "2026-11-07T12:00:00Z", "event": "FSDJump", "system": "Sol", "detail": "A"}, []string{"A"}, "")
	b := row(M{"timestamp": "2026-11-07T12:00:01Z", "event": "FSDJump", "system": "Sol", "detail": "B"}, []string{"B"}, "")
	if a.ID == b.ID || a.ID == "Sol" {
		t.Fatal("journal selection aliases another event")
	}
	x := row(M{"fromKey": "iron", "toKey": "carbon"}, nil, "")
	y := row(M{"fromKey": "iron", "toKey": "sulphur"}, nil, "")
	if x.ID == "" || x.ID == y.ID {
		t.Fatal("material exchange selection has no identity")
	}
	if got := row(M{"date": "2026-11-07"}, nil, "").ID; got != "2026-11-07" {
		t.Fatal("daily statistics selection has no identity", got)
	}
}

func TestEncyclopediaAllCatalogsAndSourceDetails(t *testing.T) {
	s := presentationState(t)
	s["exobiology"] = exobiologyPresentationFixture()
	catalog := data.Object("engineering-data.json")
	want := map[string]int{"Статьи": len(v.Rows(data.Object("encyclopedia.json")["articles"])), "Корабли": len(data.Array("encyclopedia-ships.json")), "Товары": len(v.Rows(s["goods"]))}
	for mode, key := range map[string]string{"Материалы": "materials", "Инженеры": "engineers", "Чертежи": "blueprints", "Модули": "modules", "Эффекты": "experimentals"} {
		want[mode] = len(v.Rows(catalog[key]))
	}
	for mode, count := range want {
		t.Run(mode, func(t *testing.T) {
			p := Build("encyclopedia", mode, "", s)
			if len(p.Rows) != count || count == 0 {
				t.Fatalf("%d rows, want %d", len(p.Rows), count)
			}
			for _, r := range p.Rows {
				if r.Details == "" {
					t.Fatal("missing reference details", r.ID)
				}
				if mode == "Статьи" && r.Cells[1] == "—" {
					t.Fatal("missing article section", r.ID)
				}
			}
		})
	}
	blueprints := Build("encyclopedia", "Чертежи", "", s)
	for _, r := range blueprints.Rows {
		if !strings.Contains(r.Details, "Рецепт G1:") || !strings.Contains(r.Details, "https://") || v.Str(r.Data["url"]) == "" {
			t.Fatalf("blueprint recipe/source missing: %s", r.ID)
		}
	}
	for _, r := range Build("encyclopedia", "Корабли", "", s).Rows {
		if v.Bool(r.Data["specsKnown"]) && !strings.Contains(r.Details, "Масса корпуса, т:") {
			t.Fatal("known ship specifications dropped", r.ID)
		}
		if url := v.Str(r.Data["sourceUrl"]); url != "" && !strings.Contains(r.Details, url) {
			t.Fatal("ship source dropped", r.ID)
		}
	}
	if got := recipeDetails([]M{{"key": "iron", "count": 2}}); got != "Железо: 2" {
		t.Fatal("recipe material untranslated", got)
	}
	matched := Build("encyclopedia", "Статьи", "колонизац", s)
	if len(matched.Rows) == 0 || len(matched.Rows) >= want["Статьи"] {
		t.Fatal("article filter ignored")
	}
}

func TestHelpNativeWorkflowAndReadableTables(t *testing.T) {
	articles := data.Array("help.json")
	if len(articles) != 26 {
		t.Fatal("help articles lost", len(articles))
	}
	tables := 0
	for _, r := range articles {
		body := v.Str(r["body"])
		if len(body) < 100 {
			t.Fatal("empty help", r["id"])
		}
		if strings.Contains(body, " | ") {
			tables++
		}
		for _, forbidden := range []string{"<table", "<div", "<button", "</", "EDMC", "EDDN", "Client ID", "clientId", "Developer Zone", "PKCE", "WebView", "Electron", "вставьте содержимое экспортированного"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%v contains obsolete instruction %q", r["id"], forbidden)
			}
		}
	}
	if tables < 5 {
		t.Fatal("help table cell separators lost", tables)
	}
	for _, tab := range Tabs {
		if len(HelpFor(tab.ID)) < 100 {
			t.Fatal("missing contextual help", tab.ID)
		}
	}
	if !strings.Contains(HelpFor("settings"), "цен") || !strings.Contains(HelpFor("colonization"), "Очередь снабжения") || !strings.Contains(HelpFor("help"), "Ещё") {
		t.Fatal("wrong contextual help topic")
	}
}

func TestHelpDoesNotAdvertiseBrowserApplication(t *testing.T) {
	for _, article := range data.Array("help.json") {
		body := strings.ToLower(v.Str(article["body"]))
		for _, obsolete := range []string{"браузерная версия", "в обычном браузере кнопка", "разрешите всплывающие окна", "http://localhost", "https://localhost", "http://127.0.0.1", "карточк"} {
			if strings.Contains(body, obsolete) {
				t.Fatalf("%v advertises obsolete web application flow: %s", article["id"], obsolete)
			}
		}
	}
	if !strings.Contains(HelpFor("account"), "браузере") || !strings.Contains(HelpFor("account"), "официальном сайте Frontier") {
		t.Fatal("legitimate Frontier browser login instructions were removed")
	}
	if !strings.Contains(HelpFor("integrations"), "https://inara.cz/elite/cmdr-settings-api/") {
		t.Fatal("external service instructions were removed")
	}
}
