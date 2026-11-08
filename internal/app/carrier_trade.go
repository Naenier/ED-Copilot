package app

import (
	"errors"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (e *Engine) carrierTradeSnapshot(carrierState M) M {
	state := e.CarrierTrade.Snapshot()
	state["context"] = M{"carrier": carrierState, "calculator": e.Calculator.Snapshot(), "route": e.Route.Snapshot()}
	return state
}

func (e *Engine) updateCarrierTrade(input M) error {
	body := v.Copy(input)
	switch v.Str(body["action"]) {
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
		// The installed-services calculator is an explicit plan, not a reading
		// of the in-game cargo. Never replace unknown quantities with zero.
		if options["servicesTons"] == nil {
			options["servicesTons"] = summary["servicesTons"]
		}
		if options["servicesTons"] == nil {
			delete(options, "servicesTons")
		}
		e.domainMu.Lock()
		owned := e.Carrier.Snapshot(nil)
		e.domainMu.Unlock()
		if fuel := v.At(owned, "fuel", "tankTons"); fuel != nil && calculator["selectedType"] == "fleet" {
			options["tankTons"] = fuel
		}
		if reserve := v.At(owned, "fuel", "reserveTons"); reserve != nil && calculator["selectedType"] == "fleet" {
			options["cargoTritiumTons"] = reserve
		}
		body = M{"action": "configure", "options": options}
	}
	return e.CarrierTrade.Update(body)
}
