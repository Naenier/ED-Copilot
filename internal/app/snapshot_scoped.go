package app

import (
	"time"

	"github.com/Naenier/ED-Copilot/internal/theme"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// SnapshotForUI computes only domains required by a displayed tab. Full
// Snapshot remains available to exports, network planners and compatibility
// callers. In particular, changing a setting never scans the explored galaxy.
func (e *Engine) SnapshotForUI(tab string) M { return e.snapshotScoped(tab) }

// SnapshotForHUD uses the selected activity rather than the open desktop tab.
func (e *Engine) SnapshotForHUD() M { return e.snapshotScoped("hud") }

// SnapshotForSync contains observations consumed by Inara, without presentation
// catalogues, historical mining worlds, target evaluation or lesson content.
func (e *Engine) SnapshotForSync() M { return e.snapshotScoped("sync") }

func oneOf(value string, values ...string) bool {
	for _, item := range values {
		if value == item {
			return true
		}
	}
	return false
}

func (e *Engine) snapshotScoped(tab string) M {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.RLock()
	cmd, settings, game := v.Copy(e.commander), v.Copy(e.settings), v.Copy(e.game)
	running, revision := e.gameRunning, e.revision
	cmd["locationUpdatedAt"], cmd["cargoUpdatedAt"] = e.locationAt, e.cargoAt
	cargoSync := v.Copy(v.Obj(e.cargoSync[trading.Vessel(cmd["vehicle"])]))
	activity, hud, syncOnly := v.Text(settings["activity"], "flight"), tab == "hud", tab == "sync"
	appearance, desktop := M{}, M{}
	var navigation []string
	var goals, bookmarks, notices any
	manual, prices, network := M{}, M{}, map[string]M{}
	if !syncOnly {
		appearance, desktop = v.Copy(e.appearance), v.Copy(e.desktop)
		navigation = append([]string(nil), e.navigation...)
		goals, bookmarks, notices = v.Clone(e.objectives), v.Clone(e.bookmarks), v.Clone(e.notices)
		manual, prices = v.Copy(e.manualPrices), v.Copy(e.prices)
		for key, value := range e.network {
			if key == "trading" && (oneOf(tab, "sale", "deals", "statistics", "carrier-trade", "encyclopedia", "reference") || hud && oneOf(activity, "trade", "asteroid")) || key == "tradeRoutes" && tab == "deals" || key == "search" && tab == "targets" {
				network[key] = v.Copy(value)
			}
		}
	}
	e.mu.RUnlock()
	connection := M{}
	if e.Reader != nil {
		connection = e.Reader.State()
	}
	connection["gameRunning"] = running
	cmd["cargoForSync"], cmd["cargoForSyncAt"], cmd["cargoVessel"] = cargoSync["inventory"], cargoSync["timestamp"], cmd["vehicle"]
	state := M{"revision": revision, "app": M{"name": "ED Copilot", "version": Version, "profileDir": e.opts.DataDir, "runtime": "Go · native Windows"}, "commander": cmd, "settings": settings, "game": game, "connection": connection, "pilot": e.Pilot.Snapshot()}
	if syncOnly {
		state["engineering"] = e.Engineering.InventorySnapshot()
		return state
	}
	palette, _ := theme.Palette(appearance)
	state["desktop"], state["theme"], state["navigation"] = desktop, M{"settings": appearance, "palette": palette, "presets": theme.Presets()}, M{"tabs": navigation}
	state["objectives"], state["bookmarks"], state["notices"] = goals, bookmarks, notices
	ledger := e.Trading.Snapshot(M{"commander": cmd, "marketOptions": v.Obj(network["trading"])["options"], "now": v.ISO(time.Now())})
	cargo, tons := v.Rows(ledger["inventory"]), float64(0)
	for _, row := range cargo {
		tons += v.Num(row["count"])
	}
	cmd["cargo"], cmd["cargoTons"] = cargo, tons
	state["tradeLedger"], state["tradePlan"] = ledger, ledger["plan"]
	resources, goods := trading.Resources(), v.Rows(ledger["goods"])
	for _, rows := range [][]M{resources, goods} {
		for _, row := range rows {
			key := v.Str(row["key"])
			auto, own := v.Obj(prices[key]), v.Obj(manual[key])
			row["automaticPrice"], row["automaticPriceAt"], row["automaticPriceSource"] = auto["price"], auto["priceAt"], auto["priceSource"]
			row["manualPrice"] = own["price"]
			selected := auto
			if v.Num(own["price"]) > 0 {
				selected = M{"price": own["price"], "priceAt": own["timestamp"], "priceSource": "Введено вручную"}
			}
			row["price"], row["priceAt"], row["priceSource"] = selected["price"], selected["priceAt"], selected["priceSource"]
		}
	}
	state["resources"], state["goods"] = resources, goods
	fullEngineering := oneOf(tab, "engineering", "targets", "encyclopedia") || hud && oneOf(activity, "raw", "engineering")
	eng := M{}
	if fullEngineering {
		eng = e.Engineering.Snapshot(nil)
	} else {
		eng = e.Engineering.InventorySnapshot()
	}
	materials := []M{}
	for _, row := range v.Rows(eng["materials"]) {
		if row["category"] == "raw" && v.Num(row["owned"]) > 0 {
			materials = append(materials, row)
		}
	}
	cmd["materials"] = materials
	if fullEngineering {
		state["engineering"] = eng
	}
	ctx := M{"commander": cmd, "settings": settings, "resources": resources, "gameRunning": running, "now": v.ISO(time.Now()), "game": game}
	wantCarrier := oneOf(tab, "overview", "carrier", "route", "carrier-trade", "colonization", "statistics") || hud && oneOf(activity, "carrier", "colonization")
	if wantCarrier {
		state["carrier"] = e.Carrier.Snapshot(ctx)
		ctx["carrier"] = state["carrier"]
	}
	if oneOf(tab, "overview", "colonization", "statistics") || hud && activity == "colonization" {
		state["colonization"] = e.Colonies.Snapshot(ctx)
	} else if settings["activity"] == "colonization" || !hud {
		state["colonization"] = e.Colonies.SelectorSnapshot()
	}
	if tab == "route" || hud && activity == "carrier" {
		state["carrierRoute"] = e.Route.Snapshot()
	}
	if tab == "mining" || tab == "targets" || hud && oneOf(activity, "asteroid", "surface", "trade") {
		ms := e.Mining.SnapshotForUI(ctx, tab == "targets")
		state["mining"], state["prospect"], state["surface"], state["targets"] = ms, ms["prospect"], ms["surface"], ms["targets"]
	}
	if tab == "statistics" {
		state["stats"] = e.Mining.SnapshotForStatistics(ctx)["stats"]
	}
	if hud && oneOf(activity, "surface", "raw") {
		state["targets"] = e.Mining.HUDTargets(ctx, activity == "raw")
	}
	if tab == "carrier-calculator" {
		state["carrierCalculator"] = e.Calculator.Snapshot()
	}
	if tab == "carrier-trade" {
		state["carrierTrade"] = e.carrierTradeSnapshot(v.Obj(state["carrier"]))
	}
	if tab == "learning" {
		state["learning"] = e.Learning.Snapshot()
	} else if !hud {
		state["learning"] = e.Learning.ProgressSnapshot()
	}
	if tr := network["trading"]; tr != nil {
		updated := []M{}
		for _, row := range v.Rows(tr["results"]) {
			updated = append(updated, trading.QuoteState(row, v.Obj(tr["options"]), time.Now()))
		}
		tr["results"] = updated
		state["trading"] = tr
	}
	if state["trading"] == nil {
		state["trading"] = M{"results": []M{}}
	}
	v.Obj(state["trading"])["destination"] = ledger["destination"]
	if tab == "deals" {
		state["tradeRoutes"] = network["tradeRoutes"]
	}
	if tab == "targets" {
		state["search"] = network["search"]
	}
	if e.Frontier != nil {
		state["frontier"] = e.Frontier.Status(cmd)
	} else {
		state["frontier"] = M{"connected": false, "storageAvailable": true}
	}
	if e.Inara != nil {
		state["integrations"] = e.Inara.Status(M{"commander": cmd, "game": game, "connection": connection})
	} else {
		state["integrations"] = M{"enabled": false, "apiKeySet": false, "storageAvailable": true}
	}
	return state
}
