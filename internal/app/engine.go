// Package app coordinates native UI commands, journal replay, persistence and domains.
package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/carrier"
	"github.com/Naenier/ED-Copilot/internal/colonization"
	"github.com/Naenier/ED-Copilot/internal/engineering"
	"github.com/Naenier/ED-Copilot/internal/exobiology"
	"github.com/Naenier/ED-Copilot/internal/frontier"
	"github.com/Naenier/ED-Copilot/internal/inara"
	"github.com/Naenier/ED-Copilot/internal/journal"
	"github.com/Naenier/ED-Copilot/internal/learning"
	"github.com/Naenier/ED-Copilot/internal/localization"
	"github.com/Naenier/ED-Copilot/internal/mining"
	"github.com/Naenier/ED-Copilot/internal/pilot"
	"github.com/Naenier/ED-Copilot/internal/politics"
	"github.com/Naenier/ED-Copilot/internal/presentation"
	"github.com/Naenier/ED-Copilot/internal/profile"
	"github.com/Naenier/ED-Copilot/internal/ranks"
	"github.com/Naenier/ED-Copilot/internal/ships"
	"github.com/Naenier/ED-Copilot/internal/systems"
	"github.com/Naenier/ED-Copilot/internal/theme"
	"github.com/Naenier/ED-Copilot/internal/trading"
	"github.com/Naenier/ED-Copilot/internal/updater"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type M = v.M

const Version = "2.6.0"

type Options struct {
	DataDir, JournalDir           string
	HTTPClient                    *http.Client
	CallbackPort                  int
	DisableNetwork, ManualPolling bool
}
type Engine struct {
	mu                                             sync.RWMutex
	domainMu                                       sync.Mutex
	saveMu                                         sync.Mutex
	opts                                           Options
	ctx                                            context.Context
	cancel                                         context.CancelFunc
	Reader                                         *journal.Reader
	Colonies                                       *colonization.Planner
	Engineering                                    *engineering.Planner
	Exobiology                                     *exobiology.Service
	Pilot                                          *pilot.Tracker
	Ranks                                          *ranks.Tracker
	Politics                                       *politics.Service
	Trading                                        *trading.Service
	Carrier                                        *carrier.Tracker
	Calculator                                     *carrier.Calculator
	CarrierTrade                                   *carrier.TradePlanner
	Learning                                       *learning.Library
	Route                                          *carrier.RoutePlanner
	clipboardCommand                               M
	clipboardSequence                              uint64
	Mining                                         *mining.Service
	Systems                                        *systems.Client
	Frontier                                       *frontier.Client
	Inara                                          *inara.Manager
	commander, game, settings, appearance, desktop M
	objectives, bookmarks                          []M
	navigation                                     []string
	manualPrices, prices, cargoSync                M
	statsSince                                     string
	gameRunning                                    any
	locationAt, cargoAt                            string
	network                                        map[string]M
	cancelJobs                                     map[string]context.CancelFunc
	jobIDs                                         map[string]uint64
	notices                                        []M
	revision                                       uint64
	updates                                        chan struct{}
	callback                                       *http.Server
	listener                                       net.Listener
	closed                                         bool
	onlineCache                                    *onlineCache
	UpdateManager                                  *updater.Manager
	updateNotice                                   M
	updateError                                    string
	updateDone                                     chan struct{}
}

func New(o Options) (*Engine, error) {
	if o.DataDir == "" {
		o.DataDir = profile.DefaultDir()
	}
	if e := os.MkdirAll(o.DataDir, 0700); e != nil {
		return nil, e
	}
	saved, e := profile.Read(filepath.Join(o.DataDir, "settings-go-v2.json"))
	if os.IsNotExist(e) {
		saved, e = profile.Read(filepath.Join(o.DataDir, "settings.json"))
	}
	if e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	if saved == nil {
		saved = M{}
	}
	if o.JournalDir == "" {
		o.JournalDir = v.Text(saved["journalDir"], profile.DefaultJournals())
	}
	ctx, cancel := context.WithCancel(context.Background())
	x := &Engine{opts: o, ctx: ctx, cancel: cancel, settings: M{"minTier": "medium", "method": "all", "radiusLy": 50, "maxArrivalLs": 10000, "minLaserPercent": 15, "hideDepleted": true, "overlayEnabled": true, "activity": "flight"}, manualPrices: v.Copy(v.Obj(saved["manualPrices"])), prices: M{}, cargoSync: M{}, statsSince: v.Str(saved["statsSince"]), network: map[string]M{}, cancelJobs: map[string]context.CancelFunc{}, jobIDs: map[string]uint64{}, updates: make(chan struct{}, 1)}
	v.Merge(x.settings, v.Obj(saved["settings"]))
	x.settings["language"] = localization.Normalize(v.Text(x.settings["language"], "en"))
	if _, exists := x.settings["routeAutoCopyNext"]; !exists {
		x.settings["routeAutoCopyNext"] = true
	}
	if _, exists := x.settings["autoUpdateEnabled"]; !exists {
		x.settings["autoUpdateEnabled"] = true
	}
	delete(x.settings, "hideUnfocused")
	x.appearance, _ = theme.Normalize(v.Obj(saved["appearance"]))
	if x.appearance == nil {
		x.appearance = theme.Default()
	}
	x.navigation = presentation.MigrateNavigation(v.At(saved, "navigation", "tabs"), v.Int(v.At(saved, "navigation", "version")))
	x.desktop, _ = profile.Read(filepath.Join(o.DataDir, "desktop-go-v2.json"))
	if x.desktop == nil {
		x.desktop, _ = profile.Read(filepath.Join(o.DataDir, "desktop-settings.json"))
	}
	if x.desktop == nil {
		x.desktop = M{"displayMode": "windowed"}
	}
	x.objectives = v.Rows(v.Clone(v.Rows(saved["objectives"])))
	x.bookmarks = v.Rows(v.Clone(v.Rows(saved["bookmarks"])))
	x.Colonies = colonization.New(v.Obj(saved["colonization"]))
	x.Engineering = engineering.New(v.Obj(saved["engineering"]))
	x.Exobiology = exobiology.New(v.Obj(saved["exobiology"]))
	x.Pilot = pilot.New()
	x.Ranks = ranks.New(v.Obj(saved["ranks"]))
	x.Politics = politics.New(v.Obj(saved["politics"]), politics.Options{HTTPClient: o.HTTPClient, DisableNetwork: o.DisableNetwork})
	x.Trading = trading.New(M{"tradePlan": saved["tradePlan"], "tradingDestination": saved["tradingDestination"], "savedTradeRoutes": saved["savedTradeRoutes"]})
	x.Carrier = carrier.New(v.Obj(saved["carrier"]))
	x.Calculator = carrier.NewCalculator(v.Obj(saved["carrierCalculator"]))
	x.CarrierTrade = carrier.NewTradePlanner(v.Obj(saved["carrierTrade"]))
	x.Learning = learning.New(v.Obj(saved["learning"]))
	routeSaved := v.Obj(saved["carrierRouteGo"])
	if len(routeSaved) == 0 {
		if old, err := profile.Read(filepath.Join(o.DataDir, "carrier-route.json")); err == nil {
			routeSaved = old
		}
	}
	x.Route = carrier.NewRoutePlanner(routeSaved)
	if o.HTTPClient != nil {
		x.Route.Client = o.HTTPClient
	}
	x.Route.OnUpdate = func(M) { x.Notify() }
	x.Mining = mining.New(v.Obj(saved["mining"]))
	x.Systems = systems.NewClient(systems.Options{HTTPClient: o.HTTPClient})
	var cacheErr error
	x.onlineCache, cacheErr = openOnlineCache(o.DataDir)
	if cacheErr != nil {
		x.Notice("Не удалось прочитать кэш найденных систем: " + cacheErr.Error())
	}
	x.resetObserved()
	x.Reader = journal.New(o.JournalDir, x.Apply, x.resetObserved)
	x.Reader.AfterReplay = x.restoreRemoteCache
	x.Reader.Poll()
	if !o.DisableNetwork {
		port := o.CallbackPort
		if port < 0 {
			port = 0
		} else if port == 0 {
			port = 7122
		}
		listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			cancel()
			return nil, errors.New("Не удалось открыть локальный вход Frontier. Возможно, ED Copilot уже работает")
		}
		x.listener = listener
		redirect := "http://localhost:" + strconv.Itoa(listener.Addr().(*net.TCPAddr).Port) + "/api/frontier/callback"
		x.Frontier, e = frontier.New(frontier.Options{DataDir: o.DataDir, HTTPClient: o.HTTPClient, RedirectURI: redirect, GetCommander: x.Commander, OnUpdate: x.Notify})
		if e != nil {
			listener.Close()
			cancel()
			return nil, e
		}
		x.callback = &http.Server{Handler: http.HandlerFunc(x.handleCallback), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 75 * time.Second, MaxHeaderBytes: 16 * 1024}
		go func() { _ = x.callback.Serve(listener) }()
		x.Inara, e = inara.New(inara.Options{DataDir: o.DataDir, HTTPClient: o.HTTPClient, AppVersion: Version, GetState: x.SnapshotForSync, OnUpdate: x.Notify})
		if e != nil {
			x.Close()
			return nil, e
		}
	}
	if !o.ManualPolling {
		go x.Reader.Run(ctx)
	}
	x.initializeUpdates()
	return x, nil
}
func (e *Engine) Notify() {
	select {
	case e.updates <- struct{}{}:
	default:
	}
}
func (e *Engine) Updates() <-chan struct{} { return e.updates }
func (e *Engine) Commander() M             { e.mu.RLock(); defer e.mu.RUnlock(); return v.Copy(e.commander) }
func (e *Engine) resetObserved() {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.Lock()
	e.commander = M{"name": "", "fid": "", "system": "", "vehicle": "unknown", "docked": false}
	e.game = M{"live": false, "isBeta": false}
	e.clipboardCommand = nil
	e.prices = M{}
	e.cargoSync = M{}
	e.gameRunning = nil
	e.locationAt = ""
	e.cargoAt = ""
	e.mu.Unlock()
	e.Carrier.Reset()
	e.Trading.Reset()
	e.Mining.Reset()
	e.Engineering.Reset()
	e.Exobiology.Reset()
	e.Pilot.Reset()
	e.Ranks.Reset()
	e.Politics.ResetObserved()
	e.Colonies.Reset()
	e.Notify()
}
func (e *Engine) Apply(event, meta M) {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	kind, at := v.Str(event["event"]), v.Str(event["timestamp"])
	side := v.Bool(meta["side"])
	e.mu.Lock()
	cmd := e.commander
	switch kind {
	case "Fileheader":
		version := v.Str(event["gameversion"])
		e.game = M{"version": version, "build": event["build"], "isBeta": strings.Contains(strings.ToLower(version), "beta"), "live": strings.HasPrefix(version, "4.") && !strings.Contains(strings.ToLower(version), "beta")}
	case "Commander", "LoadGame":
		cmd["name"] = v.Text(event["Name"], event["Commander"], cmd["name"])
		cmd["fid"] = v.First(event["FID"], cmd["fid"])
		if kind == "LoadGame" {
			for _, f := range []string{"Horizons", "Odyssey"} {
				if x, ok := event[f].(bool); ok {
					e.game[strings.ToLower(f)] = x
				}
			}
			e.game["loan"] = event["Loan"]
			e.game["multicrew"] = false
		}
	case "JoinACrew", "CrewJoin":
		e.game["multicrew"] = true
	case "QuitACrew", "CrewQuit":
		e.game["multicrew"] = false
	case "Location", "FSDJump", "CarrierJump":
		cmd["system"] = v.First(event["StarSystem"], cmd["system"])
		cmd["systemAddress"] = v.First(event["SystemAddress"], cmd["systemAddress"])
		cmd["coordinates"] = v.First(event["StarPos"], cmd["coordinates"])
		e.locationAt = at
		if docked, ok := event["Docked"].(bool); ok {
			cmd["docked"] = docked
			if docked {
				cmd["station"] = event["StationName"]
				cmd["marketId"] = event["MarketID"]
			} else {
				cmd["station"] = ""
				cmd["marketId"] = nil
			}
		} else if kind == "FSDJump" {
			cmd["docked"] = false
			cmd["station"] = ""
			cmd["marketId"] = nil
		}
		if event["BodyType"] == "Planet" {
			cmd["body"] = event["Body"]
		}
	case "Docked":
		cmd["docked"] = true
		cmd["station"] = event["StationName"]
		cmd["marketId"] = event["MarketID"]
		if event["StarSystem"] != nil {
			cmd["system"] = event["StarSystem"]
		}
		e.locationAt = at
	case "Undocked":
		cmd["docked"] = false
		cmd["station"] = ""
		cmd["marketId"] = nil
	case "ApproachBody", "Touchdown", "SupercruiseExit", "ApproachSettlement":
		cmd["body"] = v.First(event["Body"], event["BodyName"], cmd["body"])
	case "Disembark":
		cmd["vehicle"] = "foot"
	case "Embark":
		if v.Bool(event["SRV"]) {
			cmd["vehicle"] = "srv"
		} else {
			cmd["vehicle"] = "ship"
		}
	case "LaunchSRV":
		cmd["vehicle"] = "srv"
	case "DockSRV":
		cmd["vehicle"] = "ship"
	}
	cmd = ships.ApplyEvent(cmd, event)
	if event["CargoCapacity"] != nil && (kind == "LoadGame" || kind == "Loadout") {
		cmd["cargoCapacity"] = event["CargoCapacity"]
	}
	e.commander = cmd
	if kind == "Cargo" && event["Inventory"] != nil {
		vessel := trading.Vessel(v.First(event["Vessel"], cmd["vehicle"]))
		e.cargoSync[vessel] = M{"inventory": v.Clone(event["Inventory"]), "timestamp": at}
		e.cargoAt = at
	}
	switch kind {
	case "MarketBuy", "MarketSell", "MiningRefined", "CollectCargo", "EjectCargo", "CarrierDepositFuel", "ColonisationContribution":
		delete(e.cargoSync, trading.Vessel(cmd["vehicle"]))
		e.cargoAt = at
	case "CargoTransfer":
		e.cargoSync = M{}
		e.cargoAt = at
	}
	if !side {
		e.gameRunning = kind != "Shutdown"
	}
	settings, game := v.Copy(e.settings), v.Copy(e.game)
	since := e.statsSince
	cmdCopy := v.Copy(cmd)
	running := e.gameRunning
	e.revision++
	e.mu.Unlock()
	ctx := M{"commander": cmdCopy, "settings": settings, "game": game, "countInPeriod": since == "" || !v.Time(at).Before(v.Time(since)), "gameRunning": running, "replay": meta["replay"], "file": meta["file"], "side": meta["side"]}
	e.Carrier.Apply(event, ctx)
	ctx["carrierId"] = v.First(event["CarrierID"], e.Carrier.OwnerID)
	if kind == "CargoTransfer" {
		ctx["carrierId"] = v.First(event["CarrierID"], e.Carrier.DockedCarrierID)
	}
	ctx["ownMarket"] = e.Carrier.IsOwnDockedMarket(v.First(event["MarketID"], cmdCopy["marketId"]))
	ctx["station"] = cmdCopy["station"]
	tradeResult := e.Trading.Apply(event, ctx)
	ctx["tradeResult"] = tradeResult
	e.Mining.Apply(event, ctx)
	e.Engineering.Apply(event, ctx)
	e.Exobiology.Apply(event, v.Merge(v.Copy(meta), ctx))
	e.Pilot.Apply(event, ctx)
	e.Ranks.Apply(event, ctx)
	e.Politics.Apply(event)
	e.Colonies.Apply(event, v.Merge(v.Copy(meta), ctx))
	advanced := e.Route.ObserveCarrier(e.Carrier.Snapshot(ctx))
	if advanced && !v.Bool(meta["replay"]) && !side && (kind == "CarrierJump" || kind == "CarrierLocation") {
		e.queueNextRouteClipboard()
	}
	if kind == "Market" && event["Items"] != nil {
		e.market(event, cmdCopy)
	}
	if e.Inara != nil {
		m := v.Merge(v.Copy(meta), M{"commander": cmdCopy, "game": game, "connection": M{"scanning": meta["replay"], "gameRunning": running}, "app": M{"version": Version}})
		if side {
			e.Inara.ApplySide(event, m)
		} else {
			e.Inara.ApplyJournal(event, m)
		}
	}
	e.Notify()
}
func (e *Engine) market(event, commander M) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range v.Rows(event["Items"]) {
		g := trading.Good(r["Name"], r["Name_Localised"])
		price := v.Num(r["SellPrice"])
		if g != nil && price > 0 {
			e.prices[v.Str(g["key"])] = M{"price": price, "priceAt": event["timestamp"], "priceSource": v.Text(event["Source"], event["StationName"], "Журнал игры")}
		}
	}
}
func (e *Engine) Snapshot() M {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.RLock()
	cmd, settings, game := v.Copy(e.commander), v.Copy(e.settings), v.Copy(e.game)
	appearance, desktop := v.Copy(e.appearance), v.Copy(e.desktop)
	goals, bookmarks := v.Clone(e.objectives), v.Clone(e.bookmarks)
	nav := append([]string(nil), e.navigation...)
	network := map[string]M{}
	for k, m := range e.network {
		network[k] = v.Copy(m)
	}
	manual, prices := v.Copy(e.manualPrices), v.Copy(e.prices)
	cargoSync := v.Copy(v.Obj(e.cargoSync[trading.Vessel(cmd["vehicle"])]))
	running, statsSince := e.gameRunning, e.statsSince
	revision := e.revision
	notices := v.Clone(e.notices)
	cmd["locationUpdatedAt"] = e.locationAt
	cmd["cargoUpdatedAt"] = e.cargoAt
	e.mu.RUnlock()
	if tr := network["trading"]; tr != nil {
		updated := []M{}
		for _, r := range v.Rows(tr["results"]) {
			updated = append(updated, trading.QuoteState(r, v.Obj(tr["options"]), time.Now()))
		}
		tr["results"] = updated
	}
	ledger := e.Trading.Snapshot(M{"commander": cmd, "marketOptions": v.Obj(network["trading"])["options"], "now": v.ISO(time.Now())})
	cargo := v.Rows(ledger["inventory"])
	tons := 0.0
	for _, r := range cargo {
		tons += v.Num(r["count"])
	}
	cmd["cargo"] = cargo
	cmd["cargoTons"] = tons
	cmd["cargoForSync"] = cargoSync["inventory"]
	cmd["cargoForSyncAt"] = cargoSync["timestamp"]
	cmd["cargoVessel"] = cmd["vehicle"]
	eng := e.Engineering.Snapshot(nil)
	materials := []M{}
	for _, r := range v.Rows(eng["materials"]) {
		if r["category"] == "raw" && v.Num(r["owned"]) > 0 {
			materials = append(materials, r)
		}
	}
	cmd["materials"] = materials
	resources := trading.Resources()
	goods := v.Rows(ledger["goods"])
	annotate := func(rows []M) {
		for _, r := range rows {
			key := v.Str(r["key"])
			auto, own := v.Obj(prices[key]), v.Obj(manual[key])
			r["automaticPrice"] = auto["price"]
			r["automaticPriceAt"] = auto["priceAt"]
			r["automaticPriceSource"] = auto["priceSource"]
			r["manualPrice"] = own["price"]
			selected := auto
			if v.Num(own["price"]) > 0 {
				selected = M{"price": own["price"], "priceAt": own["timestamp"], "priceSource": "Введено вручную"}
			}
			r["price"] = selected["price"]
			r["priceAt"] = selected["priceAt"]
			r["priceSource"] = selected["priceSource"]
		}
	}
	annotate(resources)
	annotate(goods)
	ctx := M{"commander": cmd, "settings": settings, "resources": resources, "gameRunning": running, "now": v.ISO(time.Now()), "game": game, "statsSince": statsSince}
	c := e.Carrier.Snapshot(ctx)
	ctx["carrier"] = c
	ms := e.Mining.Snapshot(ctx)
	palette, _ := theme.Palette(appearance)
	connection := M{}
	if e.Reader != nil {
		connection = e.Reader.State()
	}
	connection["gameRunning"] = running
	state := M{"revision": revision, "app": M{"name": "ED Copilot", "version": Version, "language": settings["language"], "profileDir": e.opts.DataDir, "runtime": "Go · native Windows"}, "commander": cmd, "settings": settings, "game": game, "desktop": desktop, "connection": connection, "theme": M{"settings": appearance, "palette": palette, "presets": theme.Presets()}, "navigation": M{"tabs": nav}, "objectives": goals, "bookmarks": bookmarks, "resources": resources, "goods": goods, "tradeLedger": ledger, "tradePlan": ledger["plan"], "savedTradeRoutes": e.Trading.SavedRoutesSnapshot(), "carrier": c, "carrierRoute": e.Route.Snapshot(), "engineering": eng, "exobiology": e.Exobiology.Snapshot(), "pilot": e.Pilot.Snapshot(), "colonization": e.Colonies.Snapshot(ctx), "mining": ms, "targets": ms["targets"], "prospect": ms["prospect"], "surface": ms["surface"], "stats": ms["stats"], "world": e.Mining.WorldSnapshot(), "trading": network["trading"], "tradeRoutes": network["tradeRoutes"], "search": network["search"], "notices": notices}
	state["carrierCalculator"] = e.Calculator.Snapshot()
	state["carrierTrade"] = e.carrierTradeSnapshot(c)
	state["learning"] = e.Learning.SnapshotForLanguage(v.Str(settings["language"]))
	state["ranks"] = e.Ranks.SnapshotForLanguage(v.Str(settings["language"]))
	state["politics"] = e.Politics.Snapshot()
	state["updates"] = e.updateSnapshot()
	if len(v.Obj(state["trading"])) == 0 {
		state["trading"] = M{"results": []M{}}
	}
	v.Obj(state["trading"])["destination"] = ledger["destination"]
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
func (e *Engine) Save() error {
	e.saveMu.Lock()
	defer e.saveMu.Unlock()
	e.domainMu.Lock()
	e.mu.RLock()
	saved := M{"settings": v.Copy(e.settings), "appearance": v.Copy(e.appearance), "navigation": M{"version": 3, "tabs": append([]string(nil), e.navigation...)}, "journalDir": e.opts.JournalDir, "bookmarks": v.Clone(e.bookmarks), "manualPrices": v.Copy(e.manualPrices), "statsSince": e.statsSince, "objectives": v.Clone(e.objectives)}
	desktop := v.Copy(e.desktop)
	e.mu.RUnlock()
	saved["carrier"] = e.Carrier.ExportSettings()
	saved["carrierCalculator"] = e.Calculator.ExportSettings()
	saved["learning"] = e.Learning.ExportSettings()
	saved["ranks"] = e.Ranks.ExportSettings()
	saved["politics"] = e.Politics.ExportSettings()
	saved["carrierRouteGo"] = e.Route.ExportSettings()
	saved["carrierTrade"] = e.CarrierTrade.ExportSettings()
	saved["colonization"] = e.Colonies.ExportSettings()
	saved["engineering"] = e.Engineering.ExportSettings()
	saved["exobiology"] = e.Exobiology.ExportSettings()
	saved["mining"] = e.Mining.ExportSettings()
	v.Merge(saved, e.Trading.ExportSettings())
	e.domainMu.Unlock()
	if err := profile.Write(filepath.Join(e.opts.DataDir, "settings-go-v2.json"), saved); err != nil {
		return err
	}
	return profile.Write(filepath.Join(e.opts.DataDir, "desktop-go-v2.json"), desktop)
}
func (e *Engine) Notice(text string) {
	e.mu.Lock()
	e.notices = append([]M{{"id": v.ID(), "timestamp": v.ISO(time.Now()), "text": text}}, e.notices...)
	if len(e.notices) > 20 {
		e.notices = e.notices[:20]
	}
	e.mu.Unlock()
	e.Notify()
}
func (e *Engine) handleCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	host := "localhost:" + strconv.Itoa(e.listener.Addr().(*net.TCPAddr).Port)
	if r.Method != "GET" || r.URL.Path != "/api/frontier/callback" || r.Host != host {
		http.Error(w, "Доступ запрещён", http.StatusForbidden)
		return
	}
	if e.Frontier == nil {
		http.Error(w, "Вход недоступен", 503)
		return
	}
	if err := e.Frontier.HandleCallbackContext(r.Context(), r.URL.Query()); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	fmt.Fprint(w, "Вход Frontier завершён. Можно вернуться в ED Copilot.")
	e.Notify()
}
func (e *Engine) Close() {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	for _, cancel := range e.cancelJobs {
		cancel()
	}
	e.mu.Unlock()
	e.cancel()
	if e.updateDone != nil {
		<-e.updateDone
	}
	if e.Systems != nil {
		e.Systems.Close()
	}
	e.Colonies.CancelSearch()
	e.Route.Cancel()
	if e.Inara != nil {
		e.Inara.Close()
	}
	if e.callback != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = e.callback.Shutdown(ctx)
		cancel()
	}
	_ = e.Save()
}
func rows(m M, path ...string) []M { return v.Rows(v.At(m, path...)) }
