package app

import (
	"errors"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"time"
)

// SeedDemoShowcase is used only by the isolated CLI demo/smoke launcher. It is
// not a renderer command and refuses an online user profile.
func (e *Engine) SeedDemoShowcase() error {
	if !e.opts.DisableNetwork {
		return errors.New("Демонстрационные данные доступны только в изолированном профиле без сети")
	}
	options := M{"servicesTons": 1000, "otherCargoTons": 500, "weeklyUpkeep": 7000000, "durationHours": 24, "extraCosts": 600000, "tritiumPrice": 50000, "carrierInvestment": 5000000000, "haulerPad": "M", "haulerCapacity": 400, "cargoTritiumTons": 500}
	legs := M{"outbound": M{"sourceSystem": "Тестовая колония А", "destinationSystem": "Тестовая колония Б", "commodityKey": "gold", "quantity": 10000, "buyPrice": 1000, "sellPrice": 4000, "loadingMode": "haulers", "unloadingMode": "haulers", "loadRewardPerTon": 500, "unloadRewardPerTon": 700, "jumps": 2, "tritiumTons": 100}, "return": M{"sourceSystem": "Тестовая колония Б", "destinationSystem": "Тестовая колония А", "commodityKey": "gold", "quantity": 5000, "buyPrice": 2000, "sellPrice": 3000, "jumps": 2, "tritiumTons": 80}}
	if err := e.CarrierTrade.Update(M{"action": "configure", "options": options, "legs": legs}); err != nil {
		return err
	}
	at := v.ISO(time.Now().UTC())
	market := func(id, name, pad string, price float64) M {
		return M{"id": id, "station": name, "system": "Тестовая колония Б", "type": "Coriolis", "pad": pad, "distanceLy": 10, "distanceLs": 420, "updatedAt": at, "sourceUpdatedAt": at, "source": "Демонстрационные данные", "quotes": []M{{"key": "gold", "name": "Золото", "price": price, "quantity": 10000, "requestedQuantity": 10000, "demand": 60000, "supply": 20000, "updatedAt": at}}, "estimatedTotal": price * 10000, "coveredTons": 10000, "requestedTons": 10000, "coverageComplete": true, "demandSafe": true, "warnings": []string{"Синтетический пример. Не является рыночной котировкой."}}
	}
	e.mu.Lock()
	e.network["trading"] = M{"running": false, "status": "Демонстрационные предложения", "options": M{"mode": "sell", "system": "Тестовая колония А", "resourceKey": "gold", "quantity": 10000, "maxAgeHours": 24, "carrierMaxAgeHours": 6}, "results": []M{market("demo-market-a", "Демонстрационный порт Альфа", "L", 4000), market("demo-market-b", "Демонстрационный порт Бета", "M", 3800)}}
	e.mu.Unlock()
	e.Apply(M{"event": "EngineerProgress", "timestamp": at, "Engineers": []M{{"Engineer": "Felicity Farseer", "EngineerID": 300100, "Progress": "Unlocked", "Rank": 5, "RankProgress": 100}}}, M{"replay": true})
	e.Notify()
	return e.Save()
}
