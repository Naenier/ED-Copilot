package carrier

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type TradeOption func(*TradePlanner)

func TradeWithClock(now func() time.Time) TradeOption {
	return func(p *TradePlanner) {
		if now != nil {
			p.now = now
		}
	}
}

// TradePlanner estimates the owner's whole-trip economics. It never places
// orders, transfers funds or assumes that a reported price can fill a carrier.
type TradePlanner struct {
	mu            sync.Mutex
	now           func() time.Time
	options, legs M
}

func tradeDefaults() (M, M) {
	options := M{"preset": "custom", "carrierType": "fleet", "returnTrip": true, "servicesTons": 0, "otherCargoTons": 0, "tankTons": 1000, "cargoTritiumTons": nil, "arrivalReserveTons": 500, "tritiumPrice": nil, "weeklyUpkeep": 5000000, "durationHours": nil, "extraCosts": 0, "carrierInvestment": 5000000000, "haulerPad": "", "haulerCapacity": nil, "maxQuoteAgeHours": 24, "demandRatio": 4, "holidayConfirmed": false}
	leg := func(key string, quantity int) M {
		return M{"commodityKey": key, "quantity": quantity, "sourceSystem": "", "destinationSystem": "", "buyPrice": nil, "sellPrice": nil, "loadingMode": "self", "unloadingMode": "self", "loadRewardPerTon": 0, "unloadRewardPerTon": 0, "loadingHours": nil, "unloadingHours": nil, "jumps": nil, "tritiumTons": nil, "travelHours": nil, "buyMarket": M{}, "sellMarket": M{}, "route": M{}}
	}
	return options, M{"outbound": leg("agronomictreatment", 10000), "return": leg("gold", 0)}
}

func NewTradePlanner(saved M, opts ...TradeOption) *TradePlanner {
	p := &TradePlanner{now: time.Now}
	p.options, p.legs = tradeDefaults()
	for _, option := range opts {
		option(p)
	}
	if options, legs, err := tradeNormalize(v.Merge(v.Copy(p.options), v.Obj(saved["options"])), tradeMergeLegs(p.legs, v.Obj(saved["legs"]))); err == nil {
		p.options, p.legs = options, legs
	}
	return p
}

func tradeText(raw any, limit int) string {
	value := strings.TrimSpace(v.Str(raw))
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
func tradeMergeLegs(old, changes M) M {
	out := v.Copy(old)
	for _, key := range []string{"outbound", "return"} {
		out[key] = v.Merge(v.Copy(v.Obj(old[key])), v.Obj(changes[key]))
	}
	return out
}

type tradeNumberSpec struct {
	key, label        string
	max               float64
	optional, integer bool
}

func tradeNumbers(out, input M, specs []tradeNumberSpec) error {
	for _, spec := range specs {
		raw := input[spec.key]
		if raw == nil || strings.TrimSpace(v.Str(raw)) == "" {
			if spec.optional {
				out[spec.key] = nil
				continue
			}
			return fmt.Errorf("%s: укажите число.", spec.label)
		}
		number, ok := v.Number(raw)
		_, boolean := raw.(bool)
		if !ok || boolean || number < 0 || number > spec.max || spec.integer && number != math.Trunc(number) {
			return fmt.Errorf("%s: корректное %sчисло от 0 до %g.", spec.label, func() string {
				if spec.integer {
					return "целое "
				}
				return ""
			}(), spec.max)
		}
		out[spec.key] = number
	}
	return nil
}

var tradeOptionNumbers = []tradeNumberSpec{
	{"servicesTons", "Объём служб", 100000, false, false}, {"otherCargoTons", "Другие грузы", 1000000, false, false}, {"tankTons", "Тритий в баке перед вылетом", 1000, false, false}, {"cargoTritiumTons", "Тритий в трюме перед вылетом", 100000, true, false}, {"arrivalReserveTons", "Резерв трития по возвращении", 100000, false, false}, {"tritiumPrice", "Оценка трития, кр./т", 1e9, true, false}, {"weeklyUpkeep", "Содержание за неделю", 1e12, false, false}, {"durationHours", "Полная длительность рейса, ч", 100000, true, false}, {"extraCosts", "Прочие расходы", 1e15, false, false}, {"carrierInvestment", "Сумма для расчёта окупаемости", 1e15, true, false}, {"haulerCapacity", "Трюм корабля-перевозчика", 100000, true, true}, {"maxQuoteAgeHours", "Предельный возраст котировки, ч", 720, false, false}, {"demandRatio", "Запас спроса к объёму", 100, false, false},
}
var tradeLegNumbers = []tradeNumberSpec{
	{"quantity", "Объём товара, т", 1000000, false, true}, {"buyPrice", "Цена закупки, кр./т", 1e9, true, false}, {"sellPrice", "Цена продажи, кр./т", 1e9, true, false}, {"loadRewardPerTon", "Оплата погрузки, кр./т", 1e9, false, false}, {"unloadRewardPerTon", "Оплата разгрузки, кр./т", 1e9, false, false}, {"loadingHours", "Погрузка, ч", 100000, true, false}, {"unloadingHours", "Разгрузка, ч", 100000, true, false}, {"jumps", "Число прыжков", 50000, true, true}, {"tritiumTons", "Расход трития, т", 1000000, true, false}, {"travelHours", "Перелёт и дозаправки, ч", 100000, true, false},
}

func tradeNormalize(options, legs M) (M, M, error) {
	out, cleanLegs := M{}, M{}
	if _, ok := calculatorCatalog(v.Str(options["carrierType"])); !ok {
		return nil, nil, errors.New("Выберите тип флотоносца.")
	}
	out["carrierType"] = options["carrierType"]
	out["preset"] = v.Text(options["preset"], "custom")
	if out["preset"] != "custom" && out["preset"] != "booze-cruise" {
		return nil, nil, errors.New("Неизвестный шаблон рейса.")
	}
	for _, key := range []string{"returnTrip", "holidayConfirmed"} {
		value, ok := options[key].(bool)
		if !ok {
			return nil, nil, fmt.Errorf("%s: требуется переключатель.", key)
		}
		out[key] = value
	}
	pad := v.Str(options["haulerPad"])
	if pad != "" && pad != "S" && pad != "M" && pad != "L" {
		return nil, nil, errors.New("Площадка корабля: S, M или L.")
	}
	out["haulerPad"] = pad
	if err := tradeNumbers(out, options, tradeOptionNumbers); err != nil {
		return nil, nil, err
	}
	if v.Num(out["maxQuoteAgeHours"]) <= 0 || v.Num(out["demandRatio"]) < 1 {
		return nil, nil, errors.New("Возраст котировки должен быть больше нуля, запас спроса — не меньше единицы.")
	}
	if out["haulerCapacity"] != nil && v.Num(out["haulerCapacity"]) == 0 {
		return nil, nil, errors.New("Трюм корабля должен быть больше нуля.")
	}
	typeData, _ := calculatorCatalog(v.Str(out["carrierType"]))
	if v.Num(out["weeklyUpkeep"]) < float64(typeData.Upkeep) {
		return nil, nil, fmt.Errorf("Базовое содержание выбранного флотоносца — %d кр./нед.", typeData.Upkeep)
	}
	if out["carrierType"] == "squadron" && v.Num(out["servicesTons"]) < 3000 {
		return nil, nil, errors.New("Встроенная верфь флотоносца эскадрильи занимает 3 000 т.")
	}
	for _, name := range []string{"outbound", "return"} {
		input := v.Obj(legs[name])
		leg := M{}
		if err := tradeNumbers(leg, input, tradeLegNumbers); err != nil {
			return nil, nil, err
		}
		leg["commodityKey"] = v.Key(input["commodityKey"])
		if leg["commodityKey"] == "" {
			return nil, nil, errors.New("Укажите товар для каждого плеча рейса.")
		}
		for _, key := range []string{"sourceSystem", "destinationSystem"} {
			leg[key] = tradeText(input[key], 200)
		}
		for _, key := range []string{"loadingMode", "unloadingMode"} {
			mode := v.Text(input[key], "self")
			if mode != "self" && mode != "haulers" {
				return nil, nil, errors.New("Выберите самостоятельную перевозку или помощь других пилотов.")
			}
			leg[key] = mode
		}
		for _, key := range []string{"buyMarket", "sellMarket", "route"} {
			leg[key] = v.Copy(v.Obj(input[key]))
		}
		cleanLegs[name] = leg
	}
	if out["preset"] == "booze-cruise" {
		leg := v.Obj(cleanLegs["outbound"])
		if (leg["commodityKey"] != "wine" && leg["commodityKey"] != "liquor") || v.Key(leg["destinationSystem"]) != "hip58832" {
			return nil, nil, errors.New("Для Booze Cruise выберите вино или спиртное и назначение HIP 58832.")
		}
	}
	return out, cleanLegs, nil
}

func (p *TradePlanner) Update(body M) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	options, legs := v.Copy(p.options), v.Copy(p.legs)
	switch v.Text(body["action"], "configure") {
	case "reset":
		options, legs = tradeDefaults()
	case "preset":
		preset := v.Text(body["preset"], "custom")
		if preset != "custom" && preset != "booze-cruise" {
			return errors.New("Шаблон рейса не найден.")
		}
		base, newLegs := tradeDefaults()
		for _, key := range []string{"carrierType", "servicesTons", "otherCargoTons", "tankTons", "cargoTritiumTons", "weeklyUpkeep", "carrierInvestment", "haulerPad", "haulerCapacity"} {
			base[key] = options[key]
		}
		options, legs = base, newLegs
		options["preset"] = preset
		if preset == "booze-cruise" {
			out := v.Obj(legs["outbound"])
			out["commodityKey"], out["sourceSystem"], out["destinationSystem"] = "wine", tradeText(body["sourceSystem"], 200), "HIP 58832"
			out["sellMarket"] = M{"station": "Rackham's Peak", "system": "HIP 58832", "pad": "M", "source": "known-station"}
			back := v.Obj(legs["return"])
			back["sourceSystem"], back["destinationSystem"] = "HIP 58832", out["sourceSystem"]
			back["buyMarket"] = M{"station": "Rackham's Peak", "system": "HIP 58832", "pad": "M", "source": "known-station"}
		}
	case "configure":
		changes := v.Obj(body["options"])
		if len(changes) == 0 {
			changes = body
		}
		if next := v.Str(changes["carrierType"]); next != "" && next != v.Str(options["carrierType"]) {
			if t, ok := calculatorCatalog(next); ok {
				options["weeklyUpkeep"], options["carrierInvestment"], options["servicesTons"] = t.Upkeep, t.Price, 0
				if next == "squadron" {
					options["servicesTons"] = 3000
				}
			}
		}
		options = v.Merge(options, changes)
		for name, raw := range v.Obj(body["legs"]) {
			if name != "outbound" && name != "return" {
				return errors.New("Неизвестное плечо рейса.")
			}
			change := v.Obj(raw)
			old := v.Obj(legs[name])
			if key := v.Key(change["commodityKey"]); key != "" && key != v.Str(old["commodityKey"]) {
				old["buyMarket"], old["sellMarket"] = M{}, M{}
				for _, price := range []string{"buyPrice", "sellPrice"} {
					if _, explicit := change[price]; !explicit {
						old[price] = nil
					}
				}
			}
			legs[name] = v.Merge(old, change)
		}
	case "market":
		name, side := v.Str(body["leg"]), v.Str(body["side"])
		leg := v.Obj(legs[name])
		if len(leg) == 0 || (side != "buy" && side != "sell") {
			return errors.New("Выберите плечо и сторону рынка.")
		}
		if key := v.Key(body["commodityKey"]); key != "" && key != v.Str(leg["commodityKey"]) {
			leg["commodityKey"] = key
			leg["buyPrice"], leg["sellPrice"], leg["buyMarket"], leg["sellMarket"] = nil, nil, M{}, M{}
		}
		market, price, err := tradeMarket(v.Obj(body["market"]), v.Str(leg["commodityKey"]))
		if err != nil {
			return err
		}
		if options["preset"] == "booze-cruise" && (name == "outbound" && side == "sell" || name == "return" && side == "buy") {
			if !tradeRackham(market) {
				return errors.New("Для этого плеча Booze Cruise нужна котировка Rackham's Peak в HIP 58832.")
			}
			if market["pad"] == "L" {
				return errors.New("Rackham's Peak не принимает большие корабли: максимальная площадка M.")
			}
			market["pad"] = "M"
		}
		leg[side+"Market"], leg[side+"Price"] = market, price
		if side == "buy" {
			leg["sourceSystem"] = market["system"]
		} else {
			leg["destinationSystem"] = market["system"]
		}
	case "route":
		name := v.Str(body["leg"])
		leg := v.Obj(legs[name])
		if len(leg) == 0 {
			return errors.New("Выберите плечо маршрута.")
		}
		if kind := v.Str(body["carrierType"]); kind != "" && kind != v.Str(options["carrierType"]) {
			return errors.New("Маршрут рассчитан для другого типа флотоносца.")
		}
		for _, key := range []string{"sourceSystem", "destinationSystem"} {
			value := tradeText(body[key], 200)
			if value != "" {
				if previous := v.Str(leg[key]); previous != "" && !strings.EqualFold(previous, value) {
					return fmt.Errorf("Маршрут: %s не совпадает с планом рейса (%s / %s).", key, previous, value)
				}
				leg[key] = value
			}
		}
		summary := v.Obj(body["routeSummary"])
		for _, pair := range [][2]string{{"jumps", "totalJumps"}, {"tritiumTons", "fuelTons"}, {"travelHours", "totalMinutes"}} {
			raw := summary[pair[1]]
			if raw == nil {
				return errors.New("Маршрут должен содержать число прыжков, расход трития и время.")
			}
			number, ok := v.Number(raw)
			if !ok || number < 0 {
				return errors.New("Некорректные итоги маршрута.")
			}
			if pair[0] == "travelHours" {
				number /= 60
			}
			leg[pair[0]] = number
		}
		leg["route"] = M{"sourceSystem": leg["sourceSystem"], "destinationSystem": leg["destinationSystem"], "sourceUrl": tradeText(body["sourceUrl"], 2000), "importedAt": v.ISO(p.now()), "carrierType": options["carrierType"], "quantity": leg["quantity"], "servicesTons": options["servicesTons"], "otherCargoTons": options["otherCargoTons"]}
	default:
		return errors.New("Неизвестное действие планировщика торговли флотоносцем.")
	}
	clean, cleanLegs, err := tradeNormalize(options, legs)
	if err != nil {
		return err
	}
	p.options, p.legs = clean, cleanLegs
	return nil
}

func tradeMarket(raw M, key string) (M, any, error) {
	if v.Str(raw["station"]) == "" || v.Str(raw["system"]) == "" {
		return nil, nil, errors.New("Котировка должна указывать станцию и систему.")
	}
	quote := M{}
	for _, row := range v.Rows(raw["quotes"]) {
		if v.Key(row["key"]) == key {
			quote = row
			break
		}
	}
	if len(quote) == 0 && v.Key(raw["commodityKey"]) == key {
		quote = raw
	}
	price, ok := v.Number(quote["price"])
	if !ok || price < 0 || price > 1e9 {
		return nil, nil, errors.New("В выбранной котировке нет цены нужного товара.")
	}
	pad := v.Str(raw["pad"])
	if pad != "" && pad != "S" && pad != "M" && pad != "L" {
		return nil, nil, errors.New("Неизвестный размер площадки в котировке.")
	}
	out := M{"station": tradeText(raw["station"], 200), "system": tradeText(raw["system"], 200), "pad": pad, "updatedAt": tradeText(v.First(raw["sourceUpdatedAt"], raw["updatedAt"]), 80), "source": tradeText(raw["source"], 80), "sourceUrl": tradeText(raw["sourceUrl"], 2000), "marketId": tradeText(raw["marketId"], 40), "commodityKey": key, "price": price}
	for _, field := range []string{"supply", "demand"} {
		if n, known := v.Number(quote[field]); known && n >= 0 {
			out[field] = n
		} else {
			out[field] = nil
		}
	}
	for _, field := range []string{"distanceLy", "distanceLs"} {
		if n, known := v.Number(raw[field]); known && n >= 0 {
			out[field] = n
		} else {
			out[field] = nil
		}
	}
	return out, price, nil
}

func (p *TradePlanner) ExportSettings() M {
	p.mu.Lock()
	defer p.mu.Unlock()
	return M{"version": 1, "options": v.Copy(p.options), "legs": v.Copy(p.legs)}
}
func (p *TradePlanner) Snapshot() M {
	p.mu.Lock()
	defer p.mu.Unlock()
	return tradeSnapshot(p.options, p.legs, p.now())
}

func tradeCommodity(key string) string { return trading.Name(key, nil) }
