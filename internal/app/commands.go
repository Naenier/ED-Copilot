package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/mining"
	"github.com/Naenier/ED-Copilot/internal/pilot"
	"github.com/Naenier/ED-Copilot/internal/presentation"
	"github.com/Naenier/ED-Copilot/internal/theme"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"net/http"
	"strings"
	"time"
)

func (e *Engine) networkAllowed() error {
	if e.opts.DisableNetwork {
		return errors.New("Сетевые запросы отключены в тестовом профиле")
	}
	return nil
}
func (e *Engine) updateNetwork(name string, id uint64, state M) {
	e.mu.Lock()
	if e.jobIDs[name] == id {
		e.network[name] = v.Copy(state)
	}
	e.mu.Unlock()
	e.Notify()
}
func (e *Engine) start(name string, input M, job func(context.Context, uint64) (M, error)) M {
	e.mu.Lock()
	if c := e.cancelJobs[name]; c != nil {
		c()
	}
	ctx, cancel := context.WithCancel(e.ctx)
	e.cancelJobs[name] = cancel
	e.jobIDs[name]++
	id := e.jobIDs[name]
	e.network[name] = M{"running": true, "status": "Поиск выполняется", "options": v.Copy(input), "results": []M{}}
	e.mu.Unlock()
	go func() {
		defer cancel()
		state, err := job(ctx, id)
		if state == nil {
			state = M{}
		}
		state["running"] = false
		if err != nil {
			if errors.Is(err, context.Canceled) {
				state["status"] = "Поиск отменён"
			} else {
				state["error"] = err.Error()
				e.Notice(err.Error())
			}
		}
		e.updateNetwork(name, id, state)
		_ = e.Save()
	}()
	e.Notify()
	return M{"ok": true, "started": true}
}
func (e *Engine) cancelJob(name string) {
	e.mu.Lock()
	if c := e.cancelJobs[name]; c != nil {
		c()
	}
	e.jobIDs[name]++
	v.Obj(e.network[name])["running"] = false
	v.Obj(e.network[name])["status"] = "Поиск отменён"
	e.mu.Unlock()
	e.Notify()
}
func (e *Engine) Do(path string, body M) (M, error) {
	if body == nil {
		body = M{}
	}
	switch path {
	case "/api/updates/dismiss":
		e.mu.Lock()
		e.updateNotice = nil
		e.mu.Unlock()
		e.Notify()
		return M{"ok": true}, nil
	case "/api/updates/check":
		if err := e.networkAllowed(); err != nil {
			return nil, err
		}
		if e.UpdateManager == nil {
			return nil, errors.New("Обновления пока недоступны")
		}
		return e.start("updates", nil, func(ctx context.Context, id uint64) (M, error) {
			err := e.UpdateManager.Check(ctx)
			return e.updateSnapshot(), err
		}), nil
	case "/api/politics":
		if err := e.Politics.Update(body); err != nil {
			return nil, err
		}
	case "/api/politics/refresh":
		if err := e.networkAllowed(); err != nil {
			return nil, err
		}
		return e.start("politics", body, func(ctx context.Context, id uint64) (M, error) { return e.Politics.Fetch(ctx, body) }), nil
	case "/api/exobiology":
		if err := e.Exobiology.Update(body); err != nil {
			return nil, err
		}
	case "/api/carrier/trade":
		if body["action"] == "opportunities" {
			return e.startCarrierTradeOpportunities(body)
		}
		if err := e.updateCarrierTrade(body); err != nil {
			return nil, err
		}
	case "/api/carrier/calculator":
		if err := e.Calculator.Update(body); err != nil {
			return nil, err
		}
	case "/api/learning":
		if err := e.Learning.Update(body); err != nil {
			return nil, err
		}
	case "/api/settings":
		e.mu.Lock()
		next := v.Copy(e.settings)
		for k, allowed := range map[string][]string{"language": {"ru", "en"}, "minTier": {"low", "medium", "high"}, "method": {"all", "laser", "core", "subsurface", "surface", "surface-planet", "raw"}, "activity": {"flight", "asteroid", "surface", "raw", "trade", "carrier", "engineering", "colonization", "exobiology"}} {
			if raw, ok := body[k]; ok {
				valid := false
				for _, x := range allowed {
					if v.Str(raw) == x {
						valid = true
					}
				}
				if !valid {
					e.mu.Unlock()
					return nil, fmt.Errorf("Недопустимое значение настройки %s", k)
				}
				next[k] = raw
			}
		}
		for _, r := range []struct {
			k        string
			min, max float64
		}{{"radiusLy", 5, 100}, {"maxArrivalLs", 100, 1000000}, {"minLaserPercent", 1, 100}} {
			if raw, ok := body[r.k]; ok {
				n, valid := v.Number(raw)
				if !valid {
					e.mu.Unlock()
					return nil, errors.New("Укажите корректное числовое значение")
				}
				next[r.k] = v.Clamp(n, r.min, r.max)
			}
		}
		for _, k := range []string{"hideDepleted", "overlayEnabled", "routeAutoCopyNext", "autoUpdateEnabled"} {
			if raw, ok := body[k]; ok {
				b, valid := raw.(bool)
				if !valid {
					e.mu.Unlock()
					return nil, errors.New("Некорректный переключатель")
				}
				next[k] = b
			}
		}
		e.settings = next
		if disabled, exists := next["routeAutoCopyNext"]; exists && !v.Bool(disabled) {
			e.clipboardCommand = nil
		}
		if _, changed := body["autoUpdateEnabled"]; changed && e.UpdateManager != nil {
			e.UpdateManager.SetEnabled(v.Bool(next["autoUpdateEnabled"]))
		}
		e.mu.Unlock()
	case "/api/navigation":
		tabs := presentation.NormalizeTabs(body["tabs"])
		if body["tabs"] == nil {
			return nil, errors.New("Нужен список вкладок")
		}
		e.mu.Lock()
		e.navigation = tabs
		e.mu.Unlock()
	case "/api/desktop":
		e.mu.Lock()
		next := v.Copy(e.desktop)
		if mode, ok := body["displayMode"]; ok {
			if mode != "windowed" && mode != "fullscreen" && mode != "borderless" {
				e.mu.Unlock()
				return nil, errors.New("Неизвестный режим окна")
			}
			next["displayMode"] = mode
		}
		for _, key := range []string{"bounds", "windowedBounds", "maximized"} {
			if raw, ok := body[key]; ok {
				next[key] = v.Clone(raw)
			}
		}
		for _, key := range []string{"sidebarCollapsed", "inspectorOpen", "focusReading", "useGamePalette"} {
			if raw, present := body[key]; present {
				value, valid := raw.(bool)
				if !valid {
					e.mu.Unlock()
					return nil, errors.New("Некорректный параметр рабочего пространства")
				}
				next[key] = value
			}
		}
		e.desktop = next
		e.mu.Unlock()
	case "/api/theme":
		settings, err := theme.Normalize(v.Obj(body["settings"]))
		if err != nil {
			return nil, err
		}
		e.mu.Lock()
		e.appearance = settings
		e.mu.Unlock()
	case "/api/theme/preview":
		settings, err := theme.Normalize(v.Obj(body["settings"]))
		if err != nil {
			return nil, err
		}
		p, err := theme.Palette(settings)
		return M{"ok": err == nil, "theme": M{"settings": settings, "palette": p}}, err
	case "/api/theme/import":
		if len(v.Str(body["filePath"])) > 4096 {
			return nil, errors.New("Слишком длинный путь")
		}
		out, err := theme.ReadGamePalette(v.Str(body["filePath"]))
		if err != nil {
			return nil, err
		}
		return M{"ok": true, "theme": out}, nil
	case "/api/journal-dir":
		dir := strings.TrimSpace(v.Str(body["path"]))
		if dir == "" {
			return nil, errors.New("Укажите папку журналов")
		}
		if err := e.Reader.SetDirectory(dir); err != nil {
			return nil, err
		}
		e.mu.Lock()
		e.opts.JournalDir = dir
		e.mu.Unlock()
		go e.Reader.Poll()
	case "/api/rescan":
		e.Reader.Rescan()
		go e.Reader.Poll()
		e.Notify()
		return M{"ok": true}, nil
	case "/api/market":
		entries := v.Rows(body["prices"])
		if entries == nil {
			return nil, errors.New("Нужен список цен")
		}
		e.mu.Lock()
		next := v.Copy(e.manualPrices)
		for _, r := range entries {
			good := trading.Good(r["key"], nil)
			if good == nil || good["kind"] == "raw" {
				e.mu.Unlock()
				return nil, errors.New("Товар не найден")
			}
			key := v.Str(good["key"])
			if r["price"] == nil || r["price"] == "" {
				delete(next, key)
				continue
			}
			price, valid := v.Number(r["price"])
			if !valid || price <= 0 || price > 1e9 {
				e.mu.Unlock()
				return nil, errors.New("Цена должна быть положительным числом")
			}
			next[key] = M{"price": price, "timestamp": v.ISO(time.Now())}
		}
		e.manualPrices = next
		e.mu.Unlock()
	case "/api/objectives":
		if err := e.objective(body); err != nil {
			return nil, err
		}
	case "/api/bookmarks":
		state := e.Snapshot()
		e.mu.Lock()
		switch body["action"] {
		case "remove":
			out := e.bookmarks[:0]
			for _, r := range e.bookmarks {
				if r["id"] != body["id"] {
					out = append(out, r)
				}
			}
			e.bookmarks = out
		case "add":
			var found M
			for _, target := range v.Rows(state["targets"]) {
				if target["id"] == body["targetId"] {
					found = target
					break
				}
			}
			if found == nil {
				e.mu.Unlock()
				return nil, errors.New("Место не найдено")
			}
			exists := false
			for _, r := range e.bookmarks {
				exists = exists || r["targetId"] == body["targetId"]
			}
			if !exists {
				e.bookmarks = append(e.bookmarks, M{"id": v.ID(), "targetId": found["id"], "system": found["system"], "body": found["body"], "ring": found["ring"], "kind": found["kind"], "addedAt": v.ISO(time.Now())})
			}
		default:
			e.mu.Unlock()
			return nil, errors.New("Неизвестное действие с местом")
		}
		e.mu.Unlock()
	case "/api/session":
		if body["action"] != "reset" {
			return nil, errors.New("Неизвестное действие периода")
		}
		e.mu.Lock()
		e.statsSince = v.ISO(time.Now())
		e.mu.Unlock()
		e.Reader.Rescan()
		go e.Reader.Poll()
	case "/api/colonization":
		if err := e.Colonies.Update(body); err != nil {
			return nil, err
		}
	case "/colonization/resource-row":
		if err := e.colonyResource(body); err != nil {
			return nil, err
		}
	case "/colonization/manual-delivery":
		if err := e.Colonies.Update(v.Merge(v.Copy(body), M{"action": "record-delivery"})); err != nil {
			return nil, err
		}
	case "/api/colonization/search":
		if err := e.networkAllowed(); err != nil {
			return nil, err
		}
		request := v.Copy(body)
		return e.start("colonizationSearch", body, func(ctx context.Context, id uint64) (M, error) { return e.Colonies.Search(ctx, request) }), nil
	case "/api/colonization/cancel":
		e.Colonies.CancelSearch()
		e.cancelJob("colonizationSearch")
		return M{"ok": true}, nil
	case "/api/engineering":
		if err := e.Engineering.Update(body); err != nil {
			return nil, err
		}
	case "/api/mining/plan":
		e.domainMu.Lock()
		err := e.Mining.Update(body)
		e.domainMu.Unlock()
		if err != nil {
			return nil, err
		}
	case "/api/mining/reset":
		e.domainMu.Lock()
		err := e.Mining.ResetRun(v.ISO(time.Now()))
		e.domainMu.Unlock()
		if err != nil {
			return nil, err
		}
		e.Reader.Rescan()
		go e.Reader.Poll()
	case "/api/carrier/planning", "/api/carrier/stock":
		e.domainMu.Lock()
		action := "planning"
		if strings.HasSuffix(path, "stock") {
			action = "stock"
		}
		err := e.Carrier.Update(v.Merge(v.Copy(body), M{"action": action}))
		e.domainMu.Unlock()
		if err != nil {
			return nil, err
		}
		if action == "stock" {
			e.Reader.Rescan()
			go e.Reader.Poll()
		}
	case "/carrier/stock-row":
		state := e.Snapshot()
		own := v.Obj(state["carrier"])
		if !v.Bool(own["owned"]) {
			return nil, errors.New("Откройте управление личным флотоносцем в игре")
		}
		key := v.Key(body["key"])
		count, ok := v.Number(body["count"])
		if key == "" || !ok || count < 0 || count > 25000 {
			return nil, errors.New("Проверьте товар и количество")
		}
		items := []M{}
		for _, r := range v.Rows(v.At(own, "warehouse", "items")) {
			if v.Key(r["key"]) != key {
				items = append(items, M{"key": r["key"], "count": r["count"]})
			}
		}
		items = append(items, M{"key": key, "count": count})
		e.domainMu.Lock()
		err := e.Carrier.SetStockBaseline(M{"carrierId": own["id"], "items": items, "timestamp": v.ISO(time.Now()), "complete": v.Bool(body["complete"])})
		e.domainMu.Unlock()
		if err != nil {
			return nil, err
		}
		e.Reader.Rescan()
		go e.Reader.Poll()
	case "/api/frontier/carrier-stock":
		state := e.Snapshot()
		account, own := v.Obj(state["frontier"]), v.Obj(state["carrier"])
		actual := v.Obj(v.At(account, "summary", "carrier"))
		if !v.Bool(account["connected"]) || !v.Bool(own["owned"]) || !v.Bool(actual["owned"]) || v.Str(actual["callsign"]) == "" || v.Str(actual["callsign"]) != v.Str(own["callsign"]) || v.Str(account["lastCarrierSync"]) == "" {
			return nil, errors.New("Ожидается подтверждённый снимок личного флотоносца из Frontier с совпадающим позывным")
		}
		items := []M{}
		observedCargo := []M{}
		for _, r := range v.Rows(actual["cargo"]) {
			items = append(items, M{"name": r["name"], "count": r["count"]})
			observedCargo = append(observedCargo, M{"commodity": r["name"], "qty": r["count"], "stolen": r["stolen"], "mission": r["mission"]})
		}
		// Re-use the same verified observation contract as a normal CAPI sync.
		// The planner rejects same-date reapplication after journal movements.
		e.Colonies.ApplyCAPI(M{"status": account, "data": M{"carrier": M{"market": M{"id": own["id"]}, "name": M{"callsign": actual["callsign"]}, "cargo": observedCargo}}, "metadata": M{"observedAt": M{"carrier": account["lastCarrierSync"]}}}, e.Commander())
		e.domainMu.Lock()
		err := e.Carrier.SetStockBaseline(M{"carrierId": own["id"], "items": items, "timestamp": account["lastCarrierSync"], "complete": true})
		e.domainMu.Unlock()
		if err != nil {
			return nil, err
		}
		e.Reader.Rescan()
		go e.Reader.Poll()
	case "/api/carrier/route":
		if err := e.networkAllowed(); err != nil {
			return nil, err
		}
		snapshot := e.SnapshotForUI("carrier")
		e.clearRouteClipboard()
		return e.start("carrierRoute", body, func(ctx context.Context, id uint64) (M, error) { return e.Route.Search(ctx, body, snapshot) }), nil
	case "/api/carrier/route/cancel":
		e.Route.Cancel()
		e.cancelJob("carrierRoute")
		return M{"ok": true}, nil
	case "/api/carrier/route/visited":
		e.clearRouteClipboard()
		if err := e.Route.MarkVisited(v.Int(body["index"]), "manual", v.ISO(time.Now())); err != nil {
			return nil, err
		}
	case "/api/carrier/route/select":
		if err := e.Route.SelectStop(v.Int(body["index"])); err != nil {
			return nil, err
		}
	case "/api/trading/saved-routes":
		e.domainMu.Lock()
		result, err := e.Trading.SaveRoute(body)
		e.domainMu.Unlock()
		if err != nil {
			return nil, err
		}
		if err := e.Save(); err != nil {
			return nil, err
		}
		e.Notify()
		return result, nil
	case "/api/trading/plan/clear":
		e.domainMu.Lock()
		err := e.Trading.Update(M{"action": "clear-plan"})
		e.domainMu.Unlock()
		if err != nil {
			return nil, err
		}
	case "/api/trading/plan", "/api/trading/destination":
		action := "plan"
		if strings.HasSuffix(path, "destination") {
			action = "destination"
		}
		e.domainMu.Lock()
		err := e.Trading.Update(v.Merge(v.Copy(body), M{"action": action}))
		e.domainMu.Unlock()
		if err != nil {
			return nil, err
		}
	case "/api/trading/search", "/api/trading/routes":
		if err := e.networkAllowed(); err != nil {
			return nil, err
		}
		snapshot := e.SnapshotForUI("carrier")
		client := e.opts.HTTPClient
		if client == nil {
			client = &http.Client{Timeout: 30 * time.Second}
		}
		searcher := trading.NewSearcher(client)
		name := "trading"
		if path == "/api/trading/routes" {
			name = "tradeRoutes"
		}
		return e.start(name, body, func(ctx context.Context, id uint64) (M, error) {
			searcher.OnUpdate = func(s M) { s["running"] = true; e.updateNetwork(name, id, s) }
			if name == "tradeRoutes" {
				return searcher.FindRoutes(ctx, body, snapshot)
			}
			return searcher.Search(ctx, body, snapshot)
		}), nil
	case "/api/trading/cancel":
		e.cancelJob("trading")
		e.cancelJob("tradeRoutes")
		return M{"ok": true}, nil
	case "/api/search":
		if err := e.networkAllowed(); err != nil {
			return nil, err
		}
		snapshot := e.Snapshot()
		client := e.opts.HTTPClient
		if client == nil {
			client = &http.Client{Timeout: 30 * time.Second}
		}
		searcher := mining.NewSearcher(client)
		return e.start("search", body, func(ctx context.Context, id uint64) (M, error) {
			searcher.OnUpdate = func(s M) { s["running"] = true; e.updateNetwork("search", id, s) }
			searcher.OnSystem = e.MergeRemoteSystem
			return searcher.Search(ctx, body, snapshot)
		}), nil
	case "/api/search/cancel":
		e.cancelJob("search")
		return M{"ok": true}, nil
	case "/api/frontier/login":
		if err := e.networkAllowed(); err != nil {
			return nil, err
		}
		if e.Frontier == nil {
			return nil, errors.New("Вход Frontier недоступен")
		}
		e.cancelFrontierRequest()
		url, err := e.Frontier.LoginURL()
		e.wakeFrontier()
		return M{"ok": err == nil, "url": url}, err
	case "/api/frontier/sync":
		if err := e.networkAllowed(); err != nil {
			return nil, err
		}
		if e.Frontier == nil {
			return nil, errors.New("Вход Frontier недоступен")
		}
		ctx, cancel := context.WithTimeout(e.ctx, frontierRequestTimeout)
		defer cancel()
		_, err := e.syncFrontier(ctx, body["carrier"] != false)
		return M{"ok": err == nil}, err
	case "/api/frontier/logout":
		e.cancelFrontierRequest()
		if e.Frontier != nil {
			if err := e.Frontier.Logout(); err != nil {
				return nil, err
			}
		}
		e.wakeFrontier()
		e.Notify()
		return M{"ok": true}, nil
	case "/api/integrations/configure":
		if e.Inara == nil {
			return nil, errors.New("Защищённая синхронизация недоступна в этом профиле")
		}
		if err := e.Inara.Configure(body); err != nil {
			return nil, err
		}
		e.Notify()
		return M{"ok": true}, nil
	case "/api/integrations/sync":
		if err := e.networkAllowed(); err != nil {
			return nil, err
		}
		if e.Inara == nil {
			return nil, errors.New("Синхронизация недоступна")
		}
		if err := e.Inara.Sync(e.ctx); err != nil {
			return nil, err
		}
		e.Notify()
		return M{"ok": true}, nil
	default:
		return nil, errors.New("Неизвестное действие приложения")
	}
	if err := e.Save(); err != nil {
		return nil, err
	}
	e.Notify()
	return M{"ok": true}, nil
}
func (e *Engine) objective(body M) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	id := v.Str(body["id"])
	index := -1
	for i, r := range e.objectives {
		if v.Str(r["id"]) == id {
			index = i
			break
		}
	}
	switch body["action"] {
	case "add":
		if len(e.objectives) >= 200 {
			return errors.New("В списке уже 200 целей")
		}
		r, err := pilot.NormalizeObjective(body, nil)
		if err != nil {
			return err
		}
		e.objectives = append(e.objectives, r)
	case "update":
		if index < 0 {
			return errors.New("Цель не найдена")
		}
		r, err := pilot.NormalizeObjective(body, e.objectives[index])
		if err != nil {
			return err
		}
		e.objectives[index] = r
	case "complete", "reopen", "remove":
		if index < 0 {
			return errors.New("Цель не найдена")
		}
		if body["action"] == "remove" {
			e.objectives = append(e.objectives[:index], e.objectives[index+1:]...)
		} else {
			r := e.objectives[index]
			if body["action"] == "complete" {
				r["status"] = "completed"
				r["completedAt"] = v.ISO(time.Now())
			} else {
				r["status"] = "active"
				r["completedAt"] = nil
			}
		}
	default:
		return errors.New("Неизвестное действие с целью")
	}
	return nil
}
func (e *Engine) colonyResource(body M) error {
	settings := e.Colonies.ExportSettings()
	var building M
	for _, project := range v.Rows(settings["projects"]) {
		if v.Str(project["id"]) == v.Str(body["projectId"]) {
			for _, b := range v.Rows(project["buildings"]) {
				if v.Str(b["id"]) == v.Str(body["buildingId"]) {
					building = b
				}
			}
		}
	}
	if building == nil {
		return errors.New("Объект не найден в выбранной колонии")
	}
	key := v.Key(body["commodity"])
	if key == "" {
		return errors.New("Выберите товар")
	}
	next := []M{}
	for _, r := range v.Rows(building["resourcePlan"]) {
		if v.Key(r["commodity"]) != key {
			next = append(next, r)
		}
	}
	if !v.Bool(body["remove"]) {
		n, ok := v.Number(body["required"])
		if !ok || n <= 0 || n != math.Trunc(n) || n > 1e9 {
			return errors.New("Укажите положительное целое количество")
		}
		next = append(next, M{"commodity": key, "required": n})
	}
	source := v.Text(body["sourceNote"], building["sourceNote"])
	return e.Colonies.Update(M{"action": "set-resource-plan", "projectId": body["projectId"], "buildingId": body["buildingId"], "resources": next, "sourceNote": source})
}
func (e *Engine) DataDir() string { return e.opts.DataDir }
