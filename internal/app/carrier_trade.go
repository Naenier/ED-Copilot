package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Naenier/ED-Copilot/internal/carrier"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	carrierTradeOpportunityJob = "carrierTradeOpportunities"
	carrierTradeResultLifetime = 10 * time.Minute
)

type carrierTradeSearchRequest struct {
	system  string
	credits float64
}

func parseCarrierTradeSearch(input M) (carrierTradeSearchRequest, bool, error) {
	var request carrierTradeSearchRequest
	if raw := input["system"]; raw != nil {
		system, ok := raw.(string)
		system = strings.TrimSpace(system)
		if !ok || !utf8.ValidString(system) || utf8.RuneCountInString(system) > 150 || strings.ContainsFunc(system, unicode.IsControl) {
			return request, false, errors.New("Укажите корректное название системы.")
		}
		request.system = system
	}
	if raw := input["credits"]; raw != nil {
		credits, ok := v.Number(raw)
		_, isText := raw.(string)
		if !ok || isText || credits <= 0 || credits > 1e15 {
			return request, false, errors.New("Укажите корректный бюджет на закупку.")
		}
		request.credits = credits
	}
	if raw := input["scenario"]; raw != nil && v.Str(raw) != carrier.TradeOpportunityScenario {
		return request, false, errors.New("Неизвестный сценарий поиска рейсов.")
	}
	force, validForce := input["force"].(bool)
	if input["force"] != nil && !validForce {
		return request, false, errors.New("Недопустимый параметр обновления рейсов.")
	}
	return request, force, nil
}

func (r carrierTradeSearchRequest) options() M {
	options := M{"scenario": carrier.TradeOpportunityScenario, "system": r.system}
	if r.credits > 0 {
		options["credits"] = r.credits
	}
	return options
}

// Protected by Engine.mu. Only view crosses into the UI snapshot.
type carrierTradeSearch struct {
	request carrierTradeSearchRequest
	context M
	view    M
}

func sameTradeOpportunityContext(a, b M) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}

func (e *Engine) carrierTradeSnapshot(snapshot M) M {
	state := e.CarrierTrade.Snapshot()
	state["context"] = M{"carrier": snapshot["carrier"], "calculator": e.Calculator.Snapshot(), "route": e.Route.Snapshot()}
	e.mu.RLock()
	state["opportunityScope"] = e.carrierTradeScope
	if search := e.carrierTradeSearch; !e.closed && search != nil && search.matches(state, snapshot) {
		state["opportunities"] = search.snapshot()
	}
	e.mu.RUnlock()
	state["networkDisabled"] = e.opts.DisableNetwork
	return state
}

func carrierTradeFields(source M, names ...string) M {
	result := M{}
	for _, name := range names {
		if value, exists := source[name]; exists {
			result[name] = value
		}
	}
	return result
}

func (s *carrierTradeSearch) snapshot() M {
	out := carrierTradeFields(s.view, "running", "options", "stage", "updatedAt", "status", "error", "partial", "limited")
	rows := []M{}
	for _, row := range v.Rows(s.view["results"]) {
		card := carrierTradeFields(row, "id", "commodityName", "quantity", "purchaseCost", "estimatedCarrier", "fuelPurchaseTons", "fuelPurchaseCost")
		card["summary"] = carrierTradeFields(v.Obj(row["summary"]), "availableCargoTons", "profit", "jumpCount", "fuelTons",
			"fuelCost", "totalOperatingCosts", "durationHours", "servicesTons", "cargoTritiumTons", "arrivalReserveTons",
			"workingHours", "loadingHours", "unloadingHours", "fuelLoadingHours", "travelHours", "loadingSorties", "unloadingSorties")
		card["options"] = carrierTradeFields(v.Obj(row["options"]), "servicesTons", "arrivalReserveTons", "maxQuoteAgeHours")
		route := v.Obj(row["route"])
		path := carrierTradeFields(route, "origin")
		for _, phase := range []string{"reposition", "loaded", "return"} {
			hops := v.Rows(route[phase])
			if len(hops) == 0 {
				continue
			}
			list := make([]M, 0, len(hops))
			for _, hop := range hops {
				list = append(list, carrierTradeFields(hop, "from", "to", "distanceLy"))
			}
			path[phase] = list
		}
		card["route"] = path
		for _, side := range []string{"buy", "sell", "fuelMarket"} {
			quote := v.Obj(row[side])
			market := carrierTradeFields(quote, "station", "system", "pad", "price", "supply", "demand", "distanceLs", "updatedAt")
			if orders := v.Rows(quote["orders"]); len(orders) > 0 {
				list := make([]M, 0, len(orders))
				for _, order := range orders {
					list = append(list, carrierTradeFields(order, "station", "system", "quantity", "price"))
				}
				market["orders"] = list
			}
			card[side] = market
		}
		rows = append(rows, card)
	}
	out["results"] = rows
	return v.Copy(out)
}

func (s *carrierTradeSearch) matches(trade, snapshot M) bool {
	return sameTradeOpportunityContext(s.context, carrier.TradeOpportunityContext(trade, snapshot, s.request.options()))
}

// The caller holds domainMu so the commander, carrier and planner are sampled
// together. Give the worker only the fields needed to resolve the search.
func (e *Engine) carrierTradeSearchInputsLocked() (M, M) {
	e.mu.RLock()
	commander := M{}
	for _, key := range []string{"system", "systemAddress", "coordinates", "shipModel", "shipType", "ship", "cargoCapacity", "vehicle"} {
		commander[key] = e.commander[key]
	}
	e.mu.RUnlock()
	current := e.Carrier.Current()
	state := M{"commander": commander, "carrier": M{"owned": current != nil, "location": current["location"], "space": current["space"]},
		"pilot": M{"credits": e.Pilot.Credits()}}
	return e.CarrierTrade.Snapshot(), v.Copy(state)
}

func (e *Engine) clearCarrierTradeSearchLocked() {
	if cancel := e.cancelJobs[carrierTradeOpportunityJob]; cancel != nil {
		cancel()
		delete(e.cancelJobs, carrierTradeOpportunityJob)
	}
	e.carrierTradeSearch = nil
}

func (e *Engine) resetCarrierTradeSearchLocked() {
	e.clearCarrierTradeSearchLocked()
	e.carrierTradeScope++
}

func carrierTradeHTTPClient(base *http.Client) *http.Client {
	client := http.Client{Timeout: 45 * time.Second}
	if base != nil {
		client.Transport = base.Transport
		if base.Timeout > 0 && base.Timeout < client.Timeout {
			client.Timeout = base.Timeout
		}
	}
	client.CheckRedirect = func(req *http.Request, previous []*http.Request) error {
		if len(previous) >= 3 || req.URL.Scheme != "https" || req.URL.User != nil ||
			!strings.EqualFold(req.URL.Host, previous[0].URL.Host) {
			return http.ErrUseLastResponse
		}
		if base != nil && base.CheckRedirect != nil {
			return base.CheckRedirect(req, previous)
		}
		return nil
	}
	return &client
}

func (e *Engine) startCarrierTradeOpportunities(input M) (M, error) {
	if err := e.networkAllowed(); err != nil {
		return nil, err
	}
	request, force, err := parseCarrierTradeSearch(input)
	if err != nil {
		return nil, err
	}
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	trade, snapshot := e.carrierTradeSearchInputsLocked()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, errors.New("Приложение завершает работу.")
	}
	key := carrier.TradeOpportunityContext(trade, snapshot, request.options())
	if cached := e.carrierTradeSearch; !force && cached != nil && cached.request == request &&
		sameTradeOpportunityContext(key, cached.context) &&
		(v.Bool(cached.view["running"]) || time.Since(v.Time(cached.view["updatedAt"])) < carrierTradeResultLifetime) {
		return M{"ok": true, "cached": true}, nil
	}
	e.clearCarrierTradeSearchLocked()
	ctx, cancel := context.WithCancel(e.ctx)
	job := &carrierTradeSearch{request: request, context: key, view: M{"running": true, "options": request.options(), "results": []M{}}}
	e.carrierTradeSearch = job
	e.cancelJobs[carrierTradeOpportunityJob] = cancel
	client := carrierTradeHTTPClient(e.opts.HTTPClient)
	go func() {
		defer cancel()
		result, err := carrier.FindTradeOpportunities(ctx, client, trade, snapshot, request.options(), func(state M) {
			e.publishCarrierTradeSearch(ctx, job, state, true, nil)
		})
		e.publishCarrierTradeSearch(ctx, job, result, false, err)
	}()
	e.Notify()
	return M{"ok": true, "started": true}, nil
}

func (e *Engine) publishCarrierTradeSearch(ctx context.Context, job *carrierTradeSearch, result M, running bool, err error) {
	if ctx.Err() != nil {
		return
	}
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	trade, snapshot := e.carrierTradeSearchInputsLocked()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.carrierTradeSearch != job || ctx.Err() != nil {
		return
	}
	if !job.matches(trade, snapshot) {
		e.resetCarrierTradeSearchLocked()
		e.Notify()
		return
	}
	view := carrierTradeFields(result, "stage", "results", "updatedAt", "status", "partial", "limited")
	view["running"], view["options"] = running, job.request.options()
	if !running {
		delete(e.cancelJobs, carrierTradeOpportunityJob)
	}
	if err != nil {
		view["error"] = "Не удалось получить данные для расчёта рейсов."
	}
	job.view = view
	e.Notify()
}

// The caller holds domainMu and mu so selection and planner mutation are
// atomic with respect to a commander switch, replacement search or shutdown.
func (e *Engine) applyCarrierTradeOpportunityLocked(id string, trade, snapshot M) (M, error) {
	job := e.carrierTradeSearch
	if job == nil || !job.matches(trade, snapshot) {
		return nil, errors.New("Условия рейса изменились. Список рейсов обновится автоматически.")
	}
	search := job.view
	at := v.Time(search["updatedAt"])
	if v.Bool(search["running"]) || at.IsZero() || time.Since(at) > carrierTradeResultLifetime {
		return nil, errors.New("Условия рейса изменились. Список рейсов обновится автоматически.")
	}
	for _, result := range v.Rows(search["results"]) {
		if result["id"] == id {
			maxAge := v.Num(v.At(result, "options", "maxQuoteAgeHours"))
			for _, side := range []string{"buy", "sell", "fuelMarket"} {
				quote := v.Obj(result[side])
				if quote["price"] != nil && time.Since(v.Time(quote["updatedAt"])).Hours() > maxAge {
					return nil, errors.New("Котировки рейса устарели. Список рейсов обновится автоматически.")
				}
			}
			legs := M{}
			for _, leg := range v.Rows(result["legs"]) {
				legs[v.Str(leg["id"])] = leg
			}
			return M{"action": "configure", "options": result["options"], "legs": legs}, nil
		}
	}
	return nil, errors.New("Этот вариант больше недоступен.")
}

func (e *Engine) updateCarrierTrade(input M) error {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	body := v.Copy(input)
	action := v.Str(body["action"])
	var trade, snapshot M
	e.mu.RLock()
	hasSearch := e.carrierTradeSearch != nil
	e.mu.RUnlock()
	if hasSearch || action == "apply-opportunity" {
		trade, snapshot = e.carrierTradeSearchInputsLocked()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("Приложение завершает работу.")
	}
	switch action {
	case "apply-opportunity":
		configured, err := e.applyCarrierTradeOpportunityLocked(v.Str(body["id"]), trade, snapshot)
		if err != nil {
			return err
		}
		body = configured
	case "import-route":
		route := e.Route.Snapshot()
		if len(v.Rows(v.At(route, "route", "stops"))) < 2 {
			return errors.New("Сначала рассчитайте маршрут флотоносца")
		}
		body = M{"action": "route", "carrierType": "fleet", "leg": v.Text(body["leg"], "outbound"), "routeSummary": route["summary"], "sourceSystem": v.First(v.At(route, "route", "sourceSystem"), v.At(route, "options", "sourceSystem")), "destinationSystem": v.First(v.At(route, "route", "destinationSystem"), v.At(route, "options", "destinationSystem")), "sourceUrl": v.At(route, "route", "sourceUrl")}
	case "import-carrier":
		calculator := e.Calculator.Snapshot()
		summary := v.Obj(calculator["summary"])
		options := M{"carrierType": calculator["selectedType"], "weeklyUpkeep": summary["weeklyServicesAndCore"], "carrierInvestment": summary["totalBuyPrice"], "servicesTons": summary["servicesCapacityTons"]}
		// Import the configured services without copying the live cargo or fuel.
		if options["servicesTons"] == nil {
			options["servicesTons"] = summary["servicesTons"]
		}
		if options["servicesTons"] == nil {
			delete(options, "servicesTons")
		}
		body = M{"action": "configure", "options": options}
	}
	if err := e.CarrierTrade.Update(body); err != nil {
		return err
	}
	if search := e.carrierTradeSearch; search != nil && !search.matches(e.CarrierTrade.Snapshot(), snapshot) {
		e.resetCarrierTradeSearchLocked()
	}
	return nil
}
