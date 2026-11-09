package carrier

import (
	"fmt"
	"math"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func tradeSum(values ...any) any {
	total := float64(0)
	for _, raw := range values {
		number, known := v.Number(raw)
		if !known {
			return nil
		}
		total += number
	}
	return total
}
func tradeTimes(value any, multiplier float64) any {
	if multiplier == 0 {
		return float64(0)
	}
	number, known := v.Number(value)
	if !known {
		return nil
	}
	return number * multiplier
}
func tradeMoney(raw any) any {
	if value, known := v.Number(raw); known {
		return math.Round(value)
	}
	return nil
}
func tradeRackham(market M) bool {
	return v.Key(market["system"]) == "hip58832" && v.Key(market["station"]) == "rackhamspeak"
}

func tradeSnapshot(options, legs M, now time.Time) M {
	typeData, _ := calculatorCatalog(v.Str(options["carrierType"]))
	issues := []M{}
	missing := []string{}
	issue := func(level, code, title, detail, field string) {
		issues = append(issues, M{"level": level, "code": code, "title": title, "detail": detail, "field": field})
	}
	need := func(key string) { missing = append(missing, key) }
	results := []M{}
	legGross, legNet, legFees := []any{}, []any{}, []any{}
	fuels, jumps, hours := []any{}, []any{}, []any{}
	maxQuantity := float64(0)
	for _, name := range []string{"outbound", "return"} {
		leg := v.Copy(v.Obj(legs[name]))
		active := name == "outbound" || v.Bool(options["returnTrip"])
		q := v.Num(leg["quantity"])
		if !active {
			q = 0
		}
		loadReward, unloadReward := float64(0), float64(0)
		if leg["loadingMode"] == "haulers" {
			loadReward = v.Num(leg["loadRewardPerTon"])
		}
		if leg["unloadingMode"] == "haulers" {
			unloadReward = v.Num(leg["unloadRewardPerTon"])
		}
		buy, sell := tradeTimes(leg["buyPrice"], q), tradeTimes(leg["sellPrice"], q)
		var gross any
		if buy != nil && sell != nil {
			gross = v.Num(sell) - v.Num(buy)
		}
		fees := q * (loadReward + unloadReward)
		var contribution any
		if gross != nil {
			contribution = v.Num(gross) - fees
		}
		jump, fuel, travel := leg["jumps"], leg["tritiumTons"], leg["travelHours"]
		if !active {
			jump, fuel, travel = float64(0), float64(0), float64(0)
		} else if jump != nil && v.Num(jump) == 0 {
			if fuel == nil {
				fuel = float64(0)
			}
			if travel == nil {
				travel = float64(0)
			}
		}
		loading, unloading := leg["loadingHours"], leg["unloadingHours"]
		if q == 0 {
			loading, unloading = float64(0), float64(0)
		}
		duration := tradeSum(travel, loading, unloading)
		var sorties any
		if v.Num(options["haulerCapacity"]) > 0 {
			sorties = math.Ceil(q / v.Num(options["haulerCapacity"]))
		}
		leg["id"], leg["name"], leg["active"], leg["commodityName"] = name, map[string]string{"outbound": "Туда", "return": "Обратно"}[name], active, tradeCommodity(v.Str(leg["commodityKey"]))
		leg["buyCost"], leg["saleRevenue"], leg["grossMargin"], leg["haulingFees"], leg["contribution"] = tradeMoney(buy), tradeMoney(sell), tradeMoney(gross), tradeMoney(fees), tradeMoney(contribution)
		leg["carrierBuyPrice"], leg["carrierSellPrice"] = nil, nil
		if leg["buyPrice"] != nil {
			leg["carrierBuyPrice"] = v.Num(leg["buyPrice"]) + loadReward
		}
		if leg["sellPrice"] != nil {
			leg["carrierSellPrice"] = v.Num(leg["sellPrice"]) - unloadReward
		}
		leg["durationHours"], leg["haulerSortiesEachWay"], leg["effectiveQuantity"] = duration, sorties, q
		if active {
			if q > maxQuantity {
				maxQuantity = q
			}
			if q > 0 {
				for _, field := range []string{"buyPrice", "sellPrice"} {
					if leg[field] == nil {
						need(name + "." + field)
					}
				}
			}
			if jump == nil {
				need(name + ".jumps")
			}
			if fuel == nil {
				need(name + ".tritiumTons")
			}
			if fuel != nil && jump != nil && v.Num(jump) == 0 && v.Num(fuel) > 0 {
				issue("warn", "fuel-without-jumps", "Проверьте маршрут", "Указан расход трития при нулевом количестве прыжков.", name+".jumps")
			}
			if q > 0 && leg["carrierSellPrice"] != nil && v.Num(leg["carrierSellPrice"]) < 0 {
				issue("error", "negative-order-price", "Невозможная цена продажи флотоносца", "Оплата разгрузки превышает цену продажи на станции.", name+".unloadRewardPerTon")
			}
			for _, side := range []string{"buy", "sell"} {
				if q == 0 {
					continue
				}
				market := v.Copy(v.Obj(leg[side+"Market"]))
				if quoted, known := v.Number(market["price"]); known && leg[side+"Price"] != nil && quoted != v.Num(leg[side+"Price"]) {
					issue("warn", "price-overridden", "Цена изменена вручную", "Цена сценария отличается от выбранной котировки. Возраст и объём рынка относятся к исходной котировке, а не к этой цене.", name+"."+side+"Price")
				}
				systemField := "sourceSystem"
				if side == "sell" {
					systemField = "destinationSystem"
				}
				if v.Str(market["system"]) != "" && !strings.EqualFold(v.Str(market["system"]), v.Str(leg[systemField])) {
					issue("warn", "market-system-changed", "Рынок относится к другой системе", "Система рейса изменена после выбора станции. Выберите подходящий рынок заново.", name+"."+side+"Market")
				}
				pad := v.Str(market["pad"])
				if tradeRackham(market) {
					pad = "M"
					market["pad"] = "M"
				}
				shipPad := v.Str(options["haulerPad"])
				rank := map[string]int{"S": 1, "M": 2, "L": 3}
				var compatible any
				if rank[pad] > 0 && rank[shipPad] > 0 {
					compatible = rank[shipPad] <= rank[pad]
					if compatible == false {
						issue("error", "pad-incompatible", "Корабль не сядет на станцию", fmt.Sprintf("%s: площадка %s, выбранный корабль требует %s. Используйте подходящий корабль для погрузки/продажи.", v.Text(market["station"], "Станция"), pad, shipPad), name+"."+side+"Market")
					}
				} else {
					issue("warn", "pad-unknown", "Площадка ещё не проверена", "Укажите размер площадки корабля и выберите станцию с известным размером площадки.", "haulerPad")
				}
				market["padCompatible"] = compatible
				at := v.Time(market["updatedAt"])
				var age any
				if !at.IsZero() {
					age = now.Sub(at).Hours()
					if v.Num(age) < -1.0/6 || v.Num(age) > v.Num(options["maxQuoteAgeHours"]) {
						issue("warn", "quote-stale", "Котировка требует обновления", "Возраст цены не соответствует выбранному пределу. Текущая цена и доступный объём могут отличаться.", name+"."+side+"Market")
					}
				} else {
					issue("warn", "quote-unverified", "Рыночная цена не подтверждена", "Ручная цена подходит для сценария; для рейса выберите свежую котировку станции.", name+"."+side+"Market")
				}
				market["ageHours"] = age
				availabilityKey := "supply"
				if side == "sell" {
					availabilityKey = "demand"
				}
				available := market[availabilityKey]
				if available == nil {
					issue("warn", "availability-unknown", "Объём рынка неизвестен", "Не подтверждено, что станция сможет выполнить весь плановый объём.", name+"."+side+"Market")
				} else if v.Num(available) == 0 && side == "sell" {
					issue("warn", "zero-demand", "Нулевой спрос требует проверки", "Отчёт с нулевым спросом не считается подтверждением неограниченной закупки.", name+".sellMarket")
				} else if v.Num(available) < q {
					issue("warn", "insufficient-market-volume", "Рынок не покрывает весь груз", fmt.Sprintf("Нужно %g т, в котировке указано %g т. Доход всего груза остаётся сценарной оценкой.", q, v.Num(available)), name+"."+side+"Market")
				}
				saleHaul := q
				if cap := v.Num(options["haulerCapacity"]); cap > 0 {
					saleHaul = math.Min(saleHaul, cap)
				}
				if side == "sell" && v.Num(available) > 0 && v.Num(available) < saleHaul*v.Num(options["demandRatio"]) {
					issue("warn", "bulk-tax-risk", "Есть риск снижения цены за крупный груз", "Спрос меньше выбранного запаса к трюму перевозчика. Цена продажи может уменьшиться; проверьте её в игре.", name+".sellMarket")
				}
				leg[side+"Market"] = market
			}
			route := v.Obj(leg["route"])
			if len(route) > 0 {
				for _, field := range []string{"sourceSystem", "destinationSystem"} {
					if v.Str(route[field]) != "" && !strings.EqualFold(v.Str(route[field]), v.Str(leg[field])) {
						issue("error", "route-stale", "Маршрут не соответствует рейсу", "Системы изменены после импорта маршрута. Пересчитайте расход для текущей загрузки.", name+".route")
					}
				}
				if v.Str(route["carrierType"]) != "" && route["carrierType"] != options["carrierType"] || route["quantity"] != nil && v.Num(route["quantity"]) != q || route["servicesTons"] != nil && v.Num(route["servicesTons"]) != v.Num(options["servicesTons"]) || route["otherCargoTons"] != nil && v.Num(route["otherCargoTons"]) != v.Num(options["otherCargoTons"]) {
					issue("warn", "route-load-changed", "Загрузка после импорта изменена", "Пересчитайте расход маршрута для текущего типа флотоносца, товара, служб и других грузов. Старое значение топлива остаётся сценарной оценкой.", name+".route")
				}
			}
		}
		results = append(results, leg)
		legGross = append(legGross, gross)
		legNet = append(legNet, contribution)
		legFees = append(legFees, fees)
		fuels = append(fuels, fuel)
		jumps = append(jumps, jump)
		hours = append(hours, duration)
	}
	fuelTons, totalJumps := tradeSum(fuels...), tradeSum(jumps...)
	var hold any = options["cargoTritiumTons"]
	if hold == nil && fuelTons != nil {
		hold = math.Max(0, v.Num(fuelTons)+v.Num(options["arrivalReserveTons"])-v.Num(options["tankTons"]))
	}
	var capacity, over, fuelFits any
	if hold != nil {
		capacity = math.Max(0, float64(typeData.Capacity)-v.Num(options["servicesTons"])-v.Num(options["otherCargoTons"])-v.Num(hold))
		over = math.Max(0, v.Num(options["servicesTons"])+v.Num(options["otherCargoTons"])+v.Num(hold)+maxQuantity-float64(typeData.Capacity))
		if v.Num(over) > 0 {
			issue("error", "cargo-overflow", "Груз не помещается", fmt.Sprintf("Не хватает %g т. Службы и тритий в трюме занимают объём; тритий в баке — нет.", v.Num(over)), "quantity")
		}
	}
	if hold != nil && fuelTons != nil {
		fuelFits = v.Num(options["tankTons"])+v.Num(hold) >= v.Num(fuelTons)+v.Num(options["arrivalReserveTons"])
		if fuelFits == false {
			issue("error", "fuel-shortfall", "Не хватает трития на весь рейс", "Учтите обратное плечо и резерв. Склад не заправляет бак автоматически: потребуется перенос трития через корабль.", "cargoTritiumTons")
		}
	}
	if hold == nil {
		need("cargoTritiumTons")
	}
	duration := options["durationHours"]
	if duration == nil {
		duration = tradeSum(hours...)
	}
	if duration == nil {
		need("durationHours")
	}
	fuelCost := tradeTimes(options["tritiumPrice"], v.Num(fuelTons))
	if fuelTons == nil {
		fuelCost = nil
	}
	if fuelTons != nil && v.Num(fuelTons) > 0 && options["tritiumPrice"] == nil {
		need("tritiumPrice")
	}
	jumpCost := tradeTimes(totalJumps, float64(typeData.JumpCost))
	upkeep := tradeTimes(duration, v.Num(options["weeklyUpkeep"])/168)
	fixed := tradeSum(fuelCost, jumpCost, upkeep, options["extraCosts"])
	gross, contribution, fees := tradeSum(legGross...), tradeSum(legNet...), tradeSum(legFees...)
	var profit any
	if contribution != nil && fixed != nil {
		profit = v.Num(contribution) - v.Num(fixed)
	}
	var breakEvenPrice, breakEvenSpread, breakEvenQuantity, roi, payback, paybackHours, capital, weeklyReserve, fuelProvision any
	out := v.Obj(legs["outbound"])
	outQ := v.Num(out["quantity"])
	returnContribution := legNet[1]
	if fixed != nil && returnContribution != nil && out["buyPrice"] != nil && outQ > 0 {
		outFees := float64(0)
		if out["loadingMode"] == "haulers" {
			outFees += v.Num(out["loadRewardPerTon"])
		}
		if out["unloadingMode"] == "haulers" {
			outFees += v.Num(out["unloadRewardPerTon"])
		}
		breakEvenSpread = outFees + (v.Num(fixed)-v.Num(returnContribution))/outQ
		breakEvenPrice = math.Max(0, v.Num(out["buyPrice"])+v.Num(breakEvenSpread))
		if out["sellPrice"] != nil {
			unit := v.Num(out["sellPrice"]) - v.Num(out["buyPrice"]) - outFees
			if unit > 0 {
				breakEvenQuantity = math.Max(0, math.Ceil((v.Num(fixed)-v.Num(returnContribution))/unit))
			}
		}
	}
	if duration != nil {
		weeklyReserve = math.Ceil(v.Num(duration)/168) * v.Num(options["weeklyUpkeep"])
	}
	if fuelTons != nil {
		fuelProvision = tradeTimes(options["tritiumPrice"], v.Num(fuelTons)+v.Num(options["arrivalReserveTons"]))
	}
	capital = tradeSum(results[0]["buyCost"], results[1]["buyCost"], fees, fuelProvision, jumpCost, weeklyReserve, options["extraCosts"])
	if profit != nil && capital != nil && v.Num(capital) > 0 {
		roi = v.Num(profit) / v.Num(capital) * 100
	}
	if profit != nil && v.Num(profit) > 0 && options["carrierInvestment"] != nil {
		payback = math.Ceil(v.Num(options["carrierInvestment"]) / v.Num(profit))
		if duration != nil {
			paybackHours = v.Num(payback) * v.Num(duration)
		}
	}
	if options["preset"] == "booze-cruise" && !v.Bool(options["holidayConfirmed"]) {
		issue("warn", "holiday-unconfirmed", "Public Holiday не подтверждён", "Праздничная цена вина и само событие не гарантированы. Подтвердите состояние в игре перед вылетом; пресет не подставляет цену.", "holidayConfirmed")
	}
	blocked, warnings := false, 0
	for _, item := range issues {
		blocked = blocked || item["level"] == "error"
		if item["level"] == "warn" {
			warnings++
		}
	}
	status := "ready"
	if warnings > 0 {
		status = "review"
	}
	if len(missing) > 0 {
		status = "needs-data"
	}
	if blocked {
		status = "blocked"
	}
	return M{"options": v.Copy(options), "legs": results, "summary": M{"status": status, "profit": tradeMoney(profit), "grossMargin": tradeMoney(gross), "haulingFees": tradeMoney(fees), "fuelTons": fuelTons, "fuelCost": tradeMoney(fuelCost), "jumpCount": totalJumps, "jumpCost": tradeMoney(jumpCost), "jumpCostPerJump": typeData.JumpCost, "allocatedUpkeep": tradeMoney(upkeep), "upkeepBankReserve": tradeMoney(weeklyReserve), "extraCosts": options["extraCosts"], "totalOperatingCosts": tradeMoney(tradeSum(fixed, fees)), "durationHours": duration, "profitPerHour": func() any {
		if profit != nil && v.Num(duration) > 0 {
			return math.Round(v.Num(profit) / v.Num(duration))
		}
		return nil
	}(), "capacityTons": typeData.Capacity, "servicesTons": options["servicesTons"], "cargoTritiumTons": hold, "availableCargoTons": capacity, "overCapacityTons": over, "fuelFits": fuelFits, "requiredFuelTons": tradeSum(fuelTons, options["arrivalReserveTons"]), "fuelProvisionValue": tradeMoney(fuelProvision), "workingCapital": tradeMoney(capital), "returnOnCapitalPercent": roi, "breakEvenSellPrice": breakEvenPrice, "breakEvenSpread": breakEvenSpread, "breakEvenQuantity": breakEvenQuantity, "paybackTrips": payback, "paybackHours": paybackHours}, "issues": issues, "missing": missing, "ready": !blocked && len(missing) == 0 && warnings == 0, "sources": []M{{"title": "Торговля флотоносцем · PTN", "url": "https://pilotstradenetwork.com/guides/fleet-carrier-trading/"}, {"title": "Booze Cruise · PTN", "url": "https://pilotstradenetwork.com/booze-cruise/"}, {"title": "Службы и содержание · PTN", "url": CalculatorSourceURL}}, "notes": []string{"Учитывается доход владельца: при помощи других пилотов их оплата уменьшает маржу владельца.", "Содержание распределяется пропорционально времени рейса; фактический недельный счёт списывается отдельно. Резерв банка показан консервативно целыми неделями.", "Расход трития вводится из расчёта маршрута для текущей загрузки. Дальность и число прыжков не заменяют расчёт топлива.", "Капитал в рейсе — консервативный бюджет: сумма закупок обоих плеч, оплаты перевозчиков, топлива с резервом, прыжков и недельного банка. Выручка от первого плеча может уменьшить фактическую потребность в новых кредитах. Уже имеющееся топливо также включено по оценочной стоимости.", "Безубыточность рассчитана при тех же расходах маршрута. Изменение объёма требует пересчёта топлива.", "Котировки — наблюдения рынка, а не гарантия цены, спроса, наличия товара или свободной стоянки."}}
}
