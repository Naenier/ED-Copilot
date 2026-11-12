package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Naenier/ED-Copilot/internal/engineering"
	"github.com/Naenier/ED-Copilot/internal/exploration"
	"github.com/Naenier/ED-Copilot/internal/localization"
	"github.com/Naenier/ED-Copilot/internal/notifications"
	"github.com/Naenier/ED-Copilot/internal/secondscreen"
	"github.com/Naenier/ED-Copilot/internal/shipbuild"
	"github.com/Naenier/ED-Copilot/internal/shiproute"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type sectionSeven struct {
	exploration                     *exploration.Service
	routes                          *shiproute.Store
	routePlanner                    *shiproute.Planner
	notifications                   *notifications.Service
	screens                         *secondscreen.Service
	builds, itineraries             M
	loadout, stored                 M
	observedOwner                   string
	lowFuel, engineeringReady       bool
	carrierJump                     M
	screenOwner                     string
	itineraryCache                  M
	itineraryRevision               uint64
	itineraryOwner, itinerarySystem string
	explorationRevision             uint64
}

func (e *Engine) initSectionSeven(saved M) {
	e.section7 = sectionSeven{
		exploration:   exploration.New(v.Obj(saved["exploration"])),
		routes:        shiproute.New(saved["shipRoutes"]),
		routePlanner:  shiproute.NewPlanner(e.opts.HTTPClient),
		notifications: notifications.New(saved["notifications"]),
		screens:       secondscreen.New(e.secondScreenView),
		builds:        v.Copy(v.Obj(saved["shipBuilds"])),
		itineraries:   v.Copy(v.Obj(saved["engineeringItineraries"])),
	}
}

func (e *Engine) startSectionSeven() {
	go e.section7.notifications.Run(e.ctx, notifications.Speak)
	// A pending jump can approach departure while Status.json is unchanged.
	go func() {
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-e.ctx.Done():
				return
			case now := <-tick.C:
				e.domainMu.Lock()
				changed := e.carrierDepartureNotice(now)
				e.domainMu.Unlock()
				if changed {
					e.Notify()
				}
			}
		}
	}()
}

func (e *Engine) resetSectionSevenObservations() {
	s := &e.section7
	s.loadout, s.stored, s.carrierJump, s.observedOwner = nil, nil, nil, ""
	s.lowFuel, s.engineeringReady = false, false
	s.itineraryCache = nil
	e.sectionSevenContext()
	s.screens.Invalidate()
}

func (e *Engine) sectionSevenContext() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.clearClipboardLocked("ship-route")
	if cancel := e.cancelJobs["shipRouteSearch"]; cancel != nil {
		cancel()
	}
	e.jobIDs["shipRouteSearch"]++
	delete(e.network, "shipRouteSearch")
	e.section7.notifications.Context(e.planWorkspace.key, v.Str(e.settings["activity"]), v.Str(e.settings["language"]))
	e.rebindSecondScreenLocked()
}

func (e *Engine) rebindSecondScreenLocked() {
	owner := e.planWorkspace.key
	if e.section7.screenOwner != "" && owner != "" && owner != e.section7.screenOwner {
		e.section7.screens.Stop()
		e.section7.screenOwner = ""
	}
	e.section7.screens.Invalidate()
}

func (e *Engine) saveSectionSeven(saved M) {
	s := &e.section7
	saved["exploration"] = s.exploration.ExportSettings()
	saved["shipRoutes"] = s.routes.Export()
	saved["shipBuilds"] = v.Copy(s.builds)
	saved["engineeringItineraries"] = v.Copy(s.itineraries)
	saved["notifications"] = s.notifications.Export()
}

func (e *Engine) observeSectionSeven(owner string, event, meta M, richesCopied bool) {
	s := &e.section7
	if owner == "" {
		return
	}
	if owner != s.observedOwner {
		s.loadout, s.stored, s.carrierJump = nil, nil, nil
		s.lowFuel, s.engineeringReady = false, false
		s.observedOwner = owner
	}
	kind := v.Str(event["event"])
	switch kind {
	case "LoadGame", "ShipyardSwap", "ShipyardNew", "ShipyardBuy":
		s.loadout = nil
	case "Loadout":
		s.loadout = v.Copy(event)
	case "StoredModules":
		s.stored = v.Copy(event)
	case "EngineerCraft", "EngineerLegacyConvert":
		s.loadout = shipbuild.ApplyObservedCraft(s.loadout, event)
	}
	findings := s.exploration.Apply(owner, event, meta)
	switch kind {
	case "Location", "FSDJump", "CarrierJump", "Scan", "FSSDiscoveryScan", "FSSAllBodiesFound", "SAAScanComplete", "Touchdown", "SupercruiseExit", "SellExplorationData", "MultiSellExplorationData", "Died":
		s.explorationRevision++
	}
	clipboard := s.routes.Observe(owner, event, meta)
	if v.Bool(meta["replay"]) || owner != e.planWorkspace.key {
		return
	}
	if clipboard != "" && !(richesCopied && v.At(meta, "settings", "activity") == "cartography") {
		e.mu.Lock()
		e.queueClipboardLocked("ship-route", clipboard)
		e.mu.Unlock()
	}
	if v.Bool(meta["side"]) && kind != "Status" {
		return
	}
	activity := v.Str(v.At(meta, "settings", "activity"))
	if oneOf(kind, "Location", "FSDJump", "CarrierJump") {
		s.notifications.SetSystem(v.Str(v.At(meta, "commander", "system")))
	}
	for _, finding := range findings {
		s.notifications.Emit(notifications.Event{
			ID: v.Str(finding["id"]), Kind: "finding", Scope: owner, Activity: activity,
			Title: v.Str(finding["title"]), EnglishTitle: v.Str(finding["englishTitle"]),
			Detail: v.Str(finding["detail"]), EnglishDetail: v.Str(finding["englishDetail"]),
			System: v.Str(finding["system"]), Body: v.Str(finding["body"]), Link: "exploration",
			SystemAddress: v.Str(finding["systemAddress"]), BodyID: v.Str(finding["bodyId"]),
			ObservedAt: v.Time(finding["observedAt"]),
		})
	}
	at := v.Time(event["timestamp"])
	if at.IsZero() || time.Since(at) > 2*time.Minute || at.After(time.Now().Add(time.Minute)) {
		return
	}
	cmd := v.Obj(meta["commander"])
	notice := notifications.Event{Scope: owner, Activity: activity, System: v.Str(cmd["system"]), ObservedAt: at}
	switch kind {
	case "Status", "FuelScoop", "FSDJump", "ReservoirReplenished", "RefuelAll", "RefuelPartial":
		if s.notifications.Enabled("fuel") {
			fuel := v.Obj(e.Pilot.Snapshot()["fuel"])
			current, known := v.Number(fuel["mainTons"])
			capacity := v.Num(fuel["mainCapacityTons"])
			if known && capacity > 0 {
				ratio := current / capacity
				if ratio > .15 {
					s.lowFuel = false
				}
				if ratio <= .10 && !s.lowFuel {
					s.lowFuel = true
					notice.ID, notice.Kind, notice.Link = v.ISO(at), "fuel", "ship-route"
					notice.Title, notice.EnglishTitle = "Мало топлива", "Low fuel"
					notice.Detail = fmt.Sprintf("В основном баке %.1f т", current)
					notice.EnglishDetail = fmt.Sprintf("%.1f t in the main tank", current)
					s.notifications.Emit(notice)
				}
			}
		}
	case "MissionCompleted":
		if id := v.Str(event["MissionID"]); id != "" {
			notice.ID, notice.Kind, notice.Link = id, "task", "overview"
			notice.Title, notice.EnglishTitle = "Миссия завершена", "Mission completed"
			notice.Detail = v.Text(event["LocalisedName"], event["Name_Localised"])
			notice.EnglishDetail = v.Text(event["MissionName"], notice.Detail)
			s.notifications.Emit(notice)
		}
	case "CarrierJumpRequest":
		if v.Str(event["CarrierID"]) != "" && v.Str(event["CarrierID"]) == v.Str(e.Carrier.OwnerID) {
			s.carrierJump = v.Copy(event)
			e.carrierDepartureNotice(time.Now())
		}
	case "CarrierJumpCancelled", "CarrierJump", "Shutdown":
		if kind == "Shutdown" || v.Str(event["CarrierID"]) == v.Str(e.Carrier.OwnerID) {
			s.carrierJump = nil
		}
	case "Materials", "MaterialCollected", "MaterialTrade", "EngineerCraft":
		e.engineeringReadyNotice(notice)
	}
}

func (e *Engine) carrierDepartureNotice(now time.Time) bool {
	s := &e.section7
	departure := v.Time(s.carrierJump["DepartureTime"])
	if s.observedOwner == "" || s.observedOwner != e.planWorkspace.key || !departure.After(now) || departure.Sub(now) > 5*time.Minute {
		return false
	}
	system := v.Str(s.carrierJump["SystemName"])
	clock := departure.Local().Format("15:04")
	return s.notifications.Emit(notifications.Event{
		ID: v.Str(s.carrierJump["CarrierID"]) + "/" + v.ISO(departure), Kind: "carrier", Scope: s.observedOwner,
		Title: "Скоро прыжок флотоносца", EnglishTitle: "Fleet carrier departure soon",
		Detail: system + " · " + clock, EnglishDetail: system + " · " + clock, System: system, Link: "carrier", ObservedAt: now,
	})
}

func (e *Engine) engineeringReadyNotice(notice notifications.Event) {
	s := &e.section7
	if !s.notifications.Enabled("engineering") {
		return
	}
	eng := e.Engineering.Snapshot(nil)
	shopping := v.Obj(eng["shopping"])
	ready := v.Num(shopping["neededUnits"]) > 0 && v.Bool(shopping["complete"])
	if ready && !s.engineeringReady {
		notice.ID, notice.Kind, notice.Link = v.ISO(notice.ObservedAt), "engineering", "engineering"
		notice.Title, notice.EnglishTitle = "Материалы для плана собраны", "Engineering materials ready"
		s.notifications.Emit(notice)
	}
	s.engineeringReady = ready
}

func (e *Engine) shipRouteContext(cmd, pilot M) M {
	if inventory := v.At(e.cargoSync, "ship", "inventory"); inventory != nil {
		tons := float64(0)
		for _, item := range v.Rows(inventory) {
			tons += v.Num(item["Count"])
		}
		cmd["cargoTons"] = tons
	}
	return M{"commander": cmd, "pilot": pilot, "shipRouteSearch": v.Copy(e.network["shipRouteSearch"]), "gameRunning": e.gameRunning}
}

func (e *Engine) startShipRoute(body M) (M, error) {
	if err := e.networkAllowed(); err != nil {
		return nil, err
	}
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.RLock()
	owner := e.planWorkspace.key
	snapshot := e.shipRouteContext(v.Copy(e.commander), e.Pilot.Snapshot())
	e.mu.RUnlock()
	if owner == "" {
		return nil, errors.New("Сначала подключите журнал командира.")
	}
	if err := validatePlanWorkspace(body, owner); err != nil {
		return nil, err
	}
	options, err := shiproute.ParseOptions(body, snapshot)
	if err != nil {
		return nil, err
	}
	return e.start("shipRouteSearch", nil, func(ctx context.Context, id uint64) (M, error) {
		route, err := e.section7.routePlanner.Plan(ctx, options)
		if err != nil {
			return nil, err
		}
		e.domainMu.Lock()
		defer e.domainMu.Unlock()
		e.mu.RLock()
		current, job := e.planWorkspace.key, e.jobIDs["shipRouteSearch"]
		state := e.shipRouteContext(v.Copy(e.commander), nil)
		e.mu.RUnlock()
		if current != owner || job != id || ctx.Err() != nil {
			return nil, context.Canceled
		}
		err = e.section7.routes.Add(owner, route, state)
		if err == nil {
			e.mu.Lock()
			e.clearClipboardLocked("ship-route")
			e.mu.Unlock()
		}
		return M{"complete": err == nil}, err
	}), nil
}

func (e *Engine) sectionSevenCommand(path string, body M) (M, error, bool) {
	if !oneOf(path, "/api/exploration/query", "/api/exploration/update", "/api/exploration/export",
		"/api/ship-builds", "/api/ship-builds/update", "/api/ship-builds/export", "/api/engineering/itinerary", "/api/engineering/itinerary/update",
		"/api/ship-route", "/api/ship-route/calculate", "/api/notifications", "/api/notifications/update", "/api/second-screen", "/api/second-screen/update") {
		return nil, nil, false
	}
	if path == "/api/ship-route/calculate" {
		out, err := e.startShipRoute(body)
		return out, err, true
	}
	out, mutated, err := e.sectionSevenLocal(path, body)
	if mutated && err == nil {
		err = e.Save()
		e.Notify()
	}
	return out, err, true
}

func (e *Engine) sectionSevenLocal(path string, body M) (M, bool, error) {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.RLock()
	owner, language := e.planWorkspace.key, v.Str(e.settings["language"])
	e.mu.RUnlock()
	if err := validatePlanWorkspace(body, owner); err != nil {
		return nil, false, err
	}
	s := &e.section7
	switch path {
	case "/api/exploration/query":
		return s.exploration.Snapshot(owner, body), false, nil
	case "/api/exploration/update":
		out, err := s.exploration.Update(owner, body)
		if err == nil {
			s.explorationRevision++
		}
		return out, true, err
	case "/api/exploration/export":
		out, err := s.exploration.ExportSurvey(owner, v.Str(body["id"]))
		return out, false, err
	case "/api/ship-builds", "/api/ship-builds/update", "/api/ship-builds/export":
		return e.shipBuildCommand(owner, path, body)
	case "/api/engineering/itinerary", "/api/engineering/itinerary/update":
		return e.engineeringItineraryCommand(owner, path, body)
	case "/api/ship-route":
		e.mu.RLock()
		ctx := e.shipRouteContext(v.Copy(e.commander), e.Pilot.Snapshot())
		e.mu.RUnlock()
		action := v.Str(body["action"])
		if action != "" && action != "view" {
			if action == "cancel" {
				e.cancelJob("shipRouteSearch")
				return M{"ok": true}, false, nil
			}
			if err := s.routes.Update(owner, body, ctx); err != nil {
				return nil, false, err
			}
			e.mu.Lock()
			e.clearClipboardLocked("ship-route")
			e.mu.Unlock()
			return s.routes.Snapshot(owner, ctx), true, nil
		}
		return s.routes.Snapshot(owner, ctx), false, nil
	case "/api/notifications":
		return s.notifications.Snapshot(owner), false, nil
	case "/api/notifications/update":
		if body["action"] == "test" {
			if e.opts.DisableNetwork {
				return nil, false, errors.New("Голос отключён в тестовом профиле.")
			}
			err := s.notifications.Preview(language, body["preferences"])
			return s.notifications.Snapshot(owner), false, err
		}
		err := s.notifications.Configure(body["preferences"])
		return s.notifications.Snapshot(owner), true, err
	case "/api/second-screen":
		return v.Obj(v.Clone(s.screens.Status())), false, nil
	case "/api/second-screen/update":
		if err := e.networkAllowed(); err != nil {
			return nil, false, err
		}
		var err error
		switch body["action"] {
		case "start":
			if owner == "" {
				return nil, false, errors.New("Сначала подключите журнал командира.")
			}
			modes := []string{}
			for _, item := range v.Arr(body["modes"]) {
				modes = append(modes, v.Str(item))
			}
			err = s.screens.Start(v.Str(body["host"]), modes)
			if err == nil {
				s.screenOwner = owner
			}
		case "stop":
			s.screens.Stop()
			s.screenOwner = ""
		case "rotate":
			err = s.screens.Rotate()
		case "revoke":
			s.screens.Revoke(v.Str(body["id"]))
		default:
			err = errors.New("Неизвестное действие второго экрана.")
		}
		return v.Obj(v.Clone(s.screens.Status())), false, err
	}
	return nil, false, errors.New("Действие недоступно.")
}

func (e *Engine) shipBuildCommand(owner, path string, body M) (M, bool, error) {
	s := &e.section7
	saved := v.Obj(s.builds[owner])
	body = v.Copy(body)
	var loadout M
	if owner != "" && owner == s.observedOwner {
		loadout = s.loadout
		body["storedModules"] = s.stored["Items"]
	}
	switch path {
	case "/api/ship-builds":
		out, err := shipbuild.View(saved, loadout, body)
		return e.shipBuildNeeds(out), false, err
	case "/api/ship-builds/export":
		out, err := shipbuild.Export(saved, body)
		return out, false, err
	default:
		if owner == "" {
			return nil, false, errors.New("Сначала подключите журнал командира.")
		}
		next, out, err := shipbuild.Update(saved, loadout, body)
		if err == nil {
			s.builds[owner] = next
			s.itineraryCache = nil
		}
		return e.shipBuildNeeds(out), true, err
	}
}

func (e *Engine) shipBuildNeeds(view M) M {
	if goals := v.Rows(view["upgradeGoals"]); len(goals) > 0 {
		e.mu.RLock()
		game := v.Copy(e.game)
		e.mu.RUnlock()
		view["needs"] = e.Engineering.BuildNeeds(goals, M{"game": game})
	}
	return view
}

func (e *Engine) engineeringItineraryCommand(owner, path string, body M) (M, bool, error) {
	if path == "/api/engineering/itinerary" {
		return e.sectionSevenItinerary(owner), false, nil
	}
	if owner == "" {
		return nil, false, errors.New("Сначала подключите журнал командира.")
	}
	s := &e.section7
	next, err := engineering.UpdateItinerary(v.Obj(s.itineraries[owner]), body)
	if err != nil {
		return nil, false, err
	}
	s.itineraries[owner] = next
	s.itineraryCache = nil
	return e.sectionSevenItinerary(owner), true, nil
}

func (e *Engine) sectionSevenItinerary(owner string) M {
	if owner == "" {
		return M{"ownerKnown": false}
	}
	s := &e.section7
	e.mu.RLock()
	system, position, revision := v.Str(e.commander["system"]), e.commander["coordinates"], e.engineeringRoadmapRevision
	e.mu.RUnlock()
	if s.itineraryCache != nil && s.itineraryRevision == revision && s.itineraryOwner == owner && s.itinerarySystem == system {
		return v.Copy(s.itineraryCache)
	}
	options := v.Obj(s.itineraries[owner])
	var loadout M
	var stored any
	if owner == s.observedOwner {
		loadout, stored = s.loadout, s.stored["Items"]
	}
	view, _ := shipbuild.View(v.Obj(s.builds[owner]), loadout, M{"storedModules": stored, "liveBaseline": true})
	needs := e.shipBuildNeeds(view)
	result := e.Engineering.Itinerary(options, M{
		"system": system, "position": position, "buildItems": v.At(needs, "needs", "items"),
		"buildAvailable": len(v.Obj(view["target"])) > 0, "buildName": v.At(view, "target", "name"),
	})
	result["ownerKnown"] = true
	s.itineraryCache, s.itineraryRevision, s.itineraryOwner, s.itinerarySystem = v.Copy(result), revision, owner, system
	return result
}

func (e *Engine) secondScreenView(mode string) secondscreen.View {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	e.mu.RLock()
	cmd := v.Copy(e.commander)
	lang, owner := v.Str(e.settings["language"]), e.planWorkspace.key
	routeCtx := e.shipRouteContext(cmd, e.Pilot.Snapshot())
	e.mu.RUnlock()
	view := secondscreen.View{Language: lang, Cards: []secondscreen.Card{}}
	if owner == "" || owner != e.section7.screenOwner {
		return view
	}
	row := func(label, value string) secondscreen.Row {
		return secondscreen.Row{Label: localization.Text(lang, label), Value: value}
	}
	card := func(title string, rows ...secondscreen.Row) {
		visible := []secondscreen.Row{}
		for _, r := range rows {
			if r.Value != "" {
				visible = append(visible, r)
			}
		}
		if len(visible) > 0 {
			view.Cards = append(view.Cards, secondscreen.Card{Title: localization.Text(lang, title), Rows: visible})
		}
	}
	switch mode {
	case "navigation":
		next := e.section7.routes.HUD(owner)
		card("Навигация", row("Система", v.Str(cmd["system"])), row("Следующая система", v.Str(next["system"])), row("Станция", v.Str(next["station"])), row("Топливо", knownTons(v.At(routeCtx, "pilot", "fuel", "mainTons"), lang)))
	case "expedition":
		name, destination, path := e.section7.exploration.ScreenPath(owner)
		if name != "" {
			card("Экспедиция", row("Экспедиция", name), row("Система", v.Str(cmd["system"])), row("Система возвращения", destination))
			view.Cards[len(view.Cards)-1].Map = path
		}
	case "engineering":
		result := e.sectionSevenItinerary(owner)
		next := v.Obj(result["currentStop"])
		card("Инженерия", row("Система", v.Str(next["system"])), row("Станция", v.Str(next["station"])), row("Действие", itineraryAction(next, lang)))
	case "colonization":
		selector := e.Colonies.SelectorSnapshot()
		if len(v.Rows(selector["projects"])) == 0 {
			break
		}
		context := M{"commander": cmd, "now": v.ISO(time.Now())}
		context["carrier"] = e.Carrier.Snapshot(context)
		colony := v.Obj(e.Colonies.Snapshot(context)["activeProject"])
		for _, project := range v.Rows(selector["projects"]) {
			if project["id"] != selector["selectedProjectId"] {
				continue
			}
			card("Колонизация", row("Система", v.Str(project["targetSystem"])), row("Статус", localization.Text(lang, v.Str(project["status"]))))
			break
		}
		items := v.Rows(colony["supplies"])
		rows := make([]secondscreen.Row, 0, min(6, len(items)))
		for _, item := range items {
			if v.Num(item["missing"]) <= 0 {
				continue
			}
			rows = append(rows, row(v.Str(item["name"]), knownTons(item["missing"], lang)))
			if len(rows) == 6 {
				break
			}
		}
		card("Осталось доставить", rows...)
	}
	return view
}

func (e *Engine) notifyGoalCompleted(owner string, goal M, at time.Time) {
	title := v.Str(goal["title"])
	e.section7.notifications.Emit(notifications.Event{
		ID: "goal/" + v.Str(goal["id"]), Kind: "task", Scope: owner, Activity: v.Str(e.settings["activity"]),
		Title: "Цель выполнена", EnglishTitle: "Objective completed", Detail: title, EnglishDetail: title,
		System: v.Str(goal["system"]), Link: "targets", ObservedAt: at,
	})
}

func knownTons(value any, lang string) string {
	if n, known := v.Number(value); known {
		return strconv.FormatFloat(n, 'f', 1, 64) + " " + localization.Text(lang, "т")
	}
	return ""
}

func itineraryAction(stop M, lang string) string {
	switch stop["action"] {
	case "trade":
		return localization.Text(lang, "Обменять материалы")
	case "unlock":
		return localization.Text(lang, "Открыть инженера")
	case "upgrade":
		return localization.Text(lang, "Улучшить модули")
	default:
		return ""
	}
}
