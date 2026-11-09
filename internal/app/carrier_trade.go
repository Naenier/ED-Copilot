package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Naenier/ED-Copilot/internal/carrier"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const carrierTradeOpportunityJob = "carrierTradeOpportunities"

func sameTradeOpportunityContext(a, b M) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}

func (e *Engine) carrierTradeSnapshot(carrierState M) M {
	state := e.CarrierTrade.Snapshot()
	state["context"] = M{"carrier": carrierState, "calculator": e.Calculator.Snapshot(), "route": e.Route.Snapshot()}
	e.mu.RLock()
	state["opportunities"] = v.Copy(e.network[carrierTradeOpportunityJob])
	e.mu.RUnlock()
	state["networkDisabled"] = e.opts.DisableNetwork
	return state
}

func (e *Engine) startCarrierTradeOpportunities(input M) (M, error) {
	if err := e.networkAllowed(); err != nil {
		return nil, err
	}
	request := M{"scenario": "booze-cruise", "system": v.Str(input["system"])}
	if credits, known := v.Number(input["credits"]); known && credits > 0 && credits <= 1e15 {
		request["credits"] = credits
	}
	snapshot := e.SnapshotForUI("carrier-trade")
	trade := v.Obj(snapshot["carrierTrade"])
	key := carrier.TradeOpportunityContext(trade, snapshot, request)
	cached := v.Obj(trade["opportunities"])
	at := v.Time(cached["updatedAt"])
	if !v.Bool(input["force"]) && (sameTradeOpportunityContext(key, v.Obj(cached["context"])) &&
		(v.Bool(cached["running"]) || !at.IsZero() && time.Since(at) < 10*time.Minute) ||
		v.Bool(cached["running"]) && sameTradeOpportunityContext(request, v.Obj(cached["options"]))) {
		return M{"ok": true, "cached": true}, nil
	}
	// Keep a missing *http.Client out of the interface so the search uses
	// its default clients.
	var client trading.HTTPDoer
	if e.opts.HTTPClient != nil {
		client = e.opts.HTTPClient
	}
	return e.start(carrierTradeOpportunityJob, request, func(ctx context.Context, id uint64) (M, error) {
		return carrier.FindTradeOpportunities(ctx, client, trade, snapshot, request, func(state M) {
			state["running"] = true
			e.updateNetwork(carrierTradeOpportunityJob, id, state)
		})
	}), nil
}

func (e *Engine) applyCarrierTradeOpportunity(id string) (M, error) {
	snapshot := e.SnapshotForUI("carrier-trade")
	trade := v.Obj(snapshot["carrierTrade"])
	search := v.Obj(trade["opportunities"])
	at := v.Time(search["updatedAt"])
	if v.Bool(search["running"]) || at.IsZero() || time.Since(at) > 10*time.Minute ||
		!sameTradeOpportunityContext(carrier.TradeOpportunityContext(trade, snapshot, v.Obj(search["options"])), v.Obj(search["context"])) {
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
	body := v.Copy(input)
	switch v.Str(body["action"]) {
	case "apply-opportunity":
		configured, err := e.applyCarrierTradeOpportunity(v.Str(body["id"]))
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
	return e.CarrierTrade.Update(body)
}
