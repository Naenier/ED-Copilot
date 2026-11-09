package app

import (
	"errors"
	"github.com/Naenier/ED-Copilot/internal/carrier"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"time"
)

// SeedDemoShowcase is used only by the isolated CLI demo/smoke launcher. It is
// not a renderer command and refuses an online user profile.
func (e *Engine) SeedDemoShowcase() error {
	if !e.opts.DisableNetwork {
		return errors.New("Демонстрационные данные доступны только в изолированном профиле без сети")
	}
	options := M{"preset": "booze-cruise", "servicesTons": 1000, "weeklyUpkeep": 7000000, "durationHours": 24, "extraCosts": 600000, "tritiumPrice": 50000, "carrierInvestment": 5000000000, "haulerPad": "M", "haulerCapacity": 400}
	legs := M{"outbound": M{"sourceSystem": "Sol", "destinationSystem": "HIP 58832", "commodityKey": "wine", "quantity": 10000, "buyPrice": 1000, "sellPrice": 42000, "loadingMode": "haulers", "unloadingMode": "haulers", "loadRewardPerTon": 500, "unloadRewardPerTon": 700, "jumps": 18, "tritiumTons": 1900}, "return": M{"sourceSystem": "HIP 58832", "destinationSystem": "Sol", "jumps": 18, "tritiumTons": 1300}}
	if err := e.CarrierTrade.Update(M{"action": "configure", "options": options, "legs": legs}); err != nil {
		return err
	}
	at := v.ISO(time.Now().UTC())
	// Use the same normalizer as a provider response: distances, fuel balance,
	// carrier mass, transfer amounts and the time range are all derived here.
	// The systems and per-jump observations are explicitly synthetic.
	routePlanOptions, err := carrier.NormalizeRouteOptions(M{"sourceSystem": "Тестовая колония А", "destinationSystem": "Тестовая колония Б", "waypoints": []string{"Учебная промежуточная система"}, "usedCapacityTons": 6500, "tankTons": 1000, "cargoTritiumTons": 500, "reserveTritiumTons": 1000, "calculateStartingFuel": false, "departureTime": at, "tritiumPrice": 50000, "maxQueueMinutes": 10}, nil)
	if err != nil {
		return err
	}
	routeResponse := M{"status": "ok", "job": "00000000-0000-4000-8000-000000000023", "result": M{"capacity_used": 6500, "fuel_loaded": 1000, "tritium_stored": 500, "jumps": []M{
		{"name": "Тестовая колония А", "id64": "100000000001", "x": 0, "y": 0, "z": 0, "distance": 0, "distance_to_destination": 800, "fuel_used": 0, "fuel_in_tank": 1000, "tritium_in_market": 500, "restock_amount": 0},
		{"name": "Учебная промежуточная система", "id64": "100000000002", "x": 400, "y": 0, "z": 0, "distance": 400, "distance_to_destination": 400, "fuel_used": 100, "fuel_in_tank": 900, "tritium_in_market": 500, "restock_amount": 0},
		{"name": "Тестовая колония Б", "id64": "100000000003", "x": 800, "y": 0, "z": 0, "distance": 400, "distance_to_destination": 0, "fuel_used": 100, "fuel_in_tank": 800, "tritium_in_market": 500, "restock_amount": 0, "is_desired_destination": true},
	}}}
	routeState, err := carrier.NormalizeProviderRoute(routeResponse, routePlanOptions, M{"createdAt": at})
	if err != nil {
		return err
	}
	routeState["options"], routeState["updatedAt"] = routePlanOptions, at
	routeState["source"], routeState["sourceUrl"] = "Демонстрационные данные", nil
	v.Obj(routeState["route"])["sourceUrl"] = nil
	routeState["warnings"] = []string{"Демонстрационные данные: системы, координаты и расход трития вымышлены. Не используйте этот маршрут в игре."}
	e.domainMu.Lock()
	e.Route = carrier.NewRoutePlanner(M{"state": routeState})
	if e.opts.HTTPClient != nil {
		e.Route.Client = e.opts.HTTPClient
	}
	e.Route.OnUpdate = func(M) { e.Notify() }
	e.domainMu.Unlock()
	market := func(id, name, pad string, price float64) M {
		return M{"id": id, "station": name, "system": "Тестовая колония Б", "type": "Coriolis", "pad": pad, "distanceLy": 10, "distanceLs": 420, "updatedAt": at, "sourceUpdatedAt": at, "source": "Демонстрационные данные", "quotes": []M{{"key": "gold", "name": "Золото", "price": price, "quantity": 10000, "requestedQuantity": 10000, "demand": 60000, "supply": 20000, "updatedAt": at}}, "estimatedTotal": price * 10000, "coveredTons": 10000, "requestedTons": 10000, "coverageComplete": true, "demandSafe": true, "warnings": []string{"Синтетический пример. Не является рыночной котировкой."}}
	}
	routeOptions := trading.RouteOptions(M{"system": "Тестовая колония А", "cargoCapacity": 100, "credits": 500000, "roundTrip": true}, nil)
	routeMarket := func(id, system, station, key, name string, price, x float64) M {
		return M{"id": id, "system": system, "station": station, "type": "Coriolis", "pad": "L", "distanceLy": x, "distanceLs": 420, "coordinates": []float64{x, 0, 0}, "updatedAt": at, "source": "Демонстрационные данные", "quotes": []M{{"key": key, "name": name, "price": price, "supply": 1000, "demand": 10000, "updatedAt": at}}}
	}
	routes := trading.BuildRoutes(map[string]M{
		"gold":   {"buy": []M{routeMarket("demo-route-a", "Тестовая колония А", "Учебный торговый порт", "gold", "Золото", 100, 0)}, "sell": []M{routeMarket("demo-route-b", "Тестовая колония Б", "Демонстрационный терминал", "gold", "Золото", 250, 30)}},
		"silver": {"buy": []M{routeMarket("demo-route-b", "Тестовая колония Б", "Демонстрационный терминал", "silver", "Серебро", 150, 30)}, "sell": []M{routeMarket("demo-route-a", "Тестовая колония А", "Учебный торговый порт", "silver", "Серебро", 220, 0)}},
	}, routeOptions)
	for _, route := range routes {
		route["options"] = v.Copy(routeOptions)
	}
	e.domainMu.Lock()
	if len(routes) > 0 {
		if _, err := e.Trading.SaveRoute(M{"action": "save-route", "route": routes[0]}); err != nil {
			e.domainMu.Unlock()
			return err
		}
	}
	e.domainMu.Unlock()
	e.mu.Lock()
	// These reference prices are fictional and exist only in the isolated
	// showcase, alongside its fictional stations, construction bills and cargo.
	for key, price := range map[string]float64{"steel": 4200, "aluminium": 2500, "titanium": 4400, "ceramiccomposites": 400, "superconductors": 6500, "gold": 50000} {
		e.galacticPrices[key] = M{"price": price, "observedAt": at, "source": "Market.json"}
	}
	e.network["tradeRoutes"] = M{"running": false, "results": routes, "options": routeOptions, "checked": 4, "total": 4, "status": "Демонстрационный рейс. Цены вымышлены."}
	e.network["trading"] = M{"running": false, "status": "Демонстрационные предложения", "options": M{"mode": "sell", "system": "Тестовая колония А", "resourceKey": "gold", "quantity": 10000, "maxAgeHours": 24, "carrierMaxAgeHours": 6}, "results": []M{market("demo-market-a", "Демонстрационный порт Альфа", "L", 4000), market("demo-market-b", "Демонстрационный порт Бета", "M", 3800)}}
	e.mu.Unlock()
	e.Apply(M{"event": "EngineerProgress", "timestamp": at, "Engineers": []M{{"Engineer": "Felicity Farseer", "EngineerID": 300100, "Progress": "Unlocked", "Rank": 5, "RankProgress": 100}}}, M{"replay": true})
	e.Notify()
	return e.Save()
}
