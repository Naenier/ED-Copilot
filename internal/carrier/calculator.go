package carrier

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// Calculator plans ownership costs; it never changes a carrier in the game.
// Prices and service capacities were checked against PTN's public calculator
// on 2026-11-08. They are game data, independently represented and calculated.
const CalculatorSourceURL = "https://pilotstradenetwork.com/tools/carrier-calculator/"

type calculatorService struct {
	Key, Name                          string
	Price, Active, Suspended, Capacity int64
	Mandatory                          bool
}
type calculatorType struct {
	Key, Name                         string
	Price, Upkeep, Capacity, JumpCost int64
	Services                          []calculatorService
}

var calculatorServices = []calculatorService{
	{"refuel", "Заправка кораблей", 40000000, 1500000, 750000, 500, false},
	{"repair", "Ремонт", 50000000, 1500000, 750000, 180, false},
	{"rearm", "Боеприпасы и дроны", 95000000, 1500000, 750000, 250, false},
	{"voucherredemption", "Служба погашения", 150000000, 1850000, 850000, 100, false},
	{"shipyard", "Верфь", 250000000, 6500000, 1800000, 3000, false},
	{"outfitting", "Снаряжение", 250000000, 5000000, 1500000, 1750, false},
	{"blackmarket", "Защищённый склад", 165000000, 2000000, 1250000, 250, false},
	{"exploration", "Universal Cartographics", 150000000, 1850000, 700000, 120, false},
	{"bartender", "Бар", 200000000, 1750000, 1250000, 150, false},
	{"vistagenomics", "Vista Genomics", 150000000, 1500000, 700000, 120, false},
	{"pioneersupplies", "Снабжение «Пионер»", 250000000, 5000000, 1500000, 200, false},
}

func calculatorCatalog(kind string) (calculatorType, bool) {
	t := calculatorType{Key: kind, Name: "Личный флотоносец", Price: 5000000000, Upkeep: 5000000, Capacity: 25000, JumpCost: 100000}
	if kind != "fleet" && kind != "squadron" {
		return t, false
	}
	if kind == "squadron" {
		t.Name, t.Price, t.Upkeep, t.Capacity, t.JumpCost = "Флотоносец эскадрильи", 25000000000, 10000000, 60000, 200000
	}
	for _, service := range calculatorServices {
		if kind == "squadron" {
			if service.Key == "voucherredemption" || service.Key == "blackmarket" {
				continue
			}
			if service.Key == "shipyard" {
				service.Price, service.Active, service.Suspended, service.Mandatory = 0, 0, 0, true
			}
		}
		t.Services = append(t.Services, service)
	}
	return t, true
}

func calculatorDefault(kind string) M {
	t, _ := calculatorCatalog(kind)
	services := M{}
	for _, service := range t.Services {
		services[service.Key] = "off"
		if service.Mandatory {
			services[service.Key] = "active"
		}
	}
	return M{"services": services, "jumpsPerWeek": 0, "reserveWeeks": 12, "bankBalance": nil, "purchaseBudget": nil, "cargoTons": 0, "tritiumPerJump": nil, "tritiumPrice": nil}
}

type Calculator struct {
	mu       sync.Mutex
	selected string
	plans    map[string]M
}

func NewCalculator(saved M) *Calculator {
	c := &Calculator{selected: "fleet", plans: map[string]M{"fleet": calculatorDefault("fleet"), "squadron": calculatorDefault("squadron")}}
	for _, kind := range []string{"fleet", "squadron"} {
		if raw := v.Obj(v.At(saved, "plans", kind)); len(raw) > 0 {
			if clean, err := calculatorNormalize(kind, v.Merge(calculatorDefault(kind), raw)); err == nil {
				c.plans[kind] = clean
			}
		}
	}
	if _, ok := calculatorCatalog(v.Str(saved["selectedType"])); ok {
		c.selected = v.Str(saved["selectedType"])
	}
	return c
}

func calculatorNormalize(kind string, input M) (M, error) {
	t, ok := calculatorCatalog(kind)
	if !ok {
		return nil, errors.New("Выберите личный флотоносец или флотоносец эскадрильи.")
	}
	output := calculatorDefault(kind)
	for _, spec := range []struct {
		key, label        string
		max               float64
		optional, integer bool
	}{
		{"jumpsPerWeek", "Прыжки за неделю", 10000, false, true},
		{"reserveWeeks", "Резерв, недель", 520, false, true},
		{"bankBalance", "Баланс банка", 1e15, true, true},
		{"purchaseBudget", "Бюджет покупки", 1e15, true, true},
		{"cargoTons", "Занято другими грузами", 1000000, false, false},
		{"tritiumPerJump", "Средний расход трития", 1000, true, false},
		{"tritiumPrice", "Цена трития", 1e9, true, true},
	} {
		raw, exists := input[spec.key]
		if !exists {
			continue
		}
		if raw == nil || strings.TrimSpace(v.Str(raw)) == "" {
			if spec.optional {
				output[spec.key] = nil
				continue
			}
			return nil, fmt.Errorf("%s: укажите число.", spec.label)
		}
		number, known := v.Number(raw)
		if !known || number < 0 || number > spec.max || spec.integer && number != math.Trunc(number) {
			return nil, fmt.Errorf("%s: требуется %sчисло от 0 до %g.", spec.label, func() string {
				if spec.integer {
					return "целое "
				}
				return ""
			}(), spec.max)
		}
		if _, boolean := raw.(bool); boolean {
			return nil, fmt.Errorf("%s: требуется число.", spec.label)
		}
		output[spec.key] = number
	}
	valid := map[string]calculatorService{}
	for _, service := range t.Services {
		valid[service.Key] = service
	}
	services := v.Obj(output["services"])
	for key, raw := range v.Obj(input["services"]) {
		service, exists := valid[key]
		if !exists {
			return nil, fmt.Errorf("Эта служба недоступна для выбранного типа флотоносца: %s.", key)
		}
		state := v.Str(raw)
		if state != "off" && state != "active" && state != "suspended" {
			return nil, errors.New("Состояние службы: не установлена, работает или приостановлена.")
		}
		if service.Mandatory && state != "active" {
			return nil, errors.New("Встроенную верфь флотоносца эскадрильи нельзя снять или приостановить.")
		}
		services[key] = state
	}
	return output, nil
}

// Update validates a complete candidate before committing any settings.
func (c *Calculator) Update(input M) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	kind := v.Text(input["carrierType"], input["selectedType"], c.selected)
	if _, ok := calculatorCatalog(kind); !ok {
		return errors.New("Неизвестный тип флотоносца.")
	}
	action := v.Text(input["action"], "configure")
	if action == "select" {
		c.selected = kind
		return nil
	}
	if action == "reset" {
		c.plans[kind], c.selected = calculatorDefault(kind), kind
		return nil
	}
	if action != "configure" && action != "service" {
		return errors.New("Неизвестное действие калькулятора флотоносца.")
	}
	candidate := v.Copy(c.plans[kind])
	for _, key := range []string{"jumpsPerWeek", "reserveWeeks", "bankBalance", "purchaseBudget", "cargoTons", "tritiumPerJump", "tritiumPrice"} {
		if raw, exists := input[key]; exists {
			candidate[key] = raw
		}
	}
	services := v.Obj(candidate["services"])
	if raw, exists := input["services"]; exists {
		if _, valid := raw.(map[string]any); !valid {
			return errors.New("Некорректный список служб.")
		}
		for key, state := range v.Obj(raw) {
			services[key] = state
		}
	}
	if action == "service" {
		key := v.Str(input["serviceKey"])
		if key == "" {
			return errors.New("Выберите службу.")
		}
		services[key] = input["state"]
	}
	clean, err := calculatorNormalize(kind, candidate)
	if err != nil {
		return err
	}
	c.plans[kind], c.selected = clean, kind
	return nil
}

func (c *Calculator) ExportSettings() M {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exportLocked()
}
func (c *Calculator) exportLocked() M {
	return v.Copy(M{"version": 1, "selectedType": c.selected, "plans": M{"fleet": c.plans["fleet"], "squadron": c.plans["squadron"]}})
}

func calculatorServiceRows(t calculatorType, options M) ([]M, float64, float64, float64) {
	rows := []M{}
	price, upkeep, capacity := float64(t.Price), float64(t.Upkeep), 0.0
	for _, service := range t.Services {
		state := v.Str(v.At(options, "services", service.Key))
		buy, weekly, used := 0.0, 0.0, 0.0
		if state != "off" {
			buy, weekly, used = float64(service.Price), float64(service.Active), float64(service.Capacity)
			if state == "suspended" {
				weekly = float64(service.Suspended)
			}
		}
		price += buy
		upkeep += weekly
		capacity += used
		rows = append(rows, M{"id": service.Key, "key": service.Key, "name": service.Name, "state": state, "mandatory": service.Mandatory, "installed": state != "off", "buyPrice": buy, "weeklyUpkeep": weekly, "capacityTons": used, "installationPrice": service.Price, "activeWeeklyUpkeep": service.Active, "suspendedWeeklyUpkeep": service.Suspended, "installedCapacityTons": service.Capacity})
	}
	return rows, price, upkeep, capacity
}

func calculatorSummary(t calculatorType, options M) (M, []M) {
	services, buy, upkeep, capacity := calculatorServiceRows(t, options)
	jumps, weeks := v.Num(options["jumpsPerWeek"]), v.Num(options["reserveWeeks"])
	jumpCost := jumps * float64(t.JumpCost)
	weekly := upkeep + jumpCost
	fuelPerJump, fuelKnown := v.Number(options["tritiumPerJump"])
	fuelPrice, priceKnown := v.Number(options["tritiumPrice"])
	fuelCostKnown := jumps == 0 || fuelKnown && priceKnown
	reserveKnown := weeks == 0 || fuelCostKnown
	fuelCost := jumps * fuelPerJump * fuelPrice
	minimumBudget := buy + weekly*weeks
	weeklyWithFuel := weekly + fuelCost
	remaining := float64(t.Capacity) - capacity - v.Num(options["cargoTons"])
	bank, bankKnown := v.Number(options["bankBalance"])
	budget, budgetKnown := v.Number(options["purchaseBudget"])
	var affordable any
	if budgetKnown {
		if budget < minimumBudget {
			affordable = false
		} else if reserveKnown {
			affordable = budget >= buy+weeklyWithFuel*weeks
		}
	}
	summary := M{
		"carrierType": t.Key, "name": t.Name, "hullBuyPrice": t.Price, "totalBuyPrice": buy, "servicesBuyPrice": buy - float64(t.Price),
		"capacityTons": t.Capacity, "servicesCapacityTons": capacity, "cargoTons": options["cargoTons"], "availableBeforeCargoTons": float64(t.Capacity) - capacity, "availableCargoTons": math.Max(0, remaining), "overCapacityTons": math.Max(0, -remaining), "capacityFits": remaining >= 0,
		"baseWeeklyUpkeep": t.Upkeep, "servicesWeeklyUpkeep": upkeep - float64(t.Upkeep), "weeklyServicesAndCore": upkeep, "jumpCostPerJump": t.JumpCost, "jumpsPerWeek": jumps, "weeklyJumpCost": jumpCost, "weeklyUpkeep": weekly, "monthlyUpkeep": math.Round(weekly * 30 / 7), "yearlyUpkeep": math.Round(weekly * 365 / 7), "monthDays": 30, "yearDays": 365,
		"weeklyTritiumTons": v.Nullable(jumps*fuelPerJump, jumps == 0 || fuelKnown), "weeklyFuelCost": v.Nullable(fuelCost, fuelCostKnown), "fuelCostKnown": fuelCostKnown, "weeklyCostWithFuel": v.Nullable(weeklyWithFuel, fuelCostKnown), "monthlyCostWithFuel": v.Nullable(math.Round(weeklyWithFuel*30/7), fuelCostKnown), "yearlyCostWithFuel": v.Nullable(math.Round(weeklyWithFuel*365/7), fuelCostKnown),
		"reserveWeeks": weeks, "minimumBankReserve": weekly * weeks, "recommendedBankReserve": v.Nullable(weeklyWithFuel*weeks, reserveKnown), "reserveIncludesFuel": reserveKnown, "minimumRequiredBudget": minimumBudget, "requiredBudget": v.Nullable(buy+weeklyWithFuel*weeks, reserveKnown), "purchaseBudget": options["purchaseBudget"], "affordable": affordable,
		"minimumBudgetGap": v.Nullable(math.Max(0, minimumBudget-budget), budgetKnown), "budgetGap": v.Nullable(math.Max(0, buy+weeklyWithFuel*weeks-budget), budgetKnown && reserveKnown),
		"bankBalance": options["bankBalance"], "bankWeeksWithoutFuel": v.Nullable(bank/weekly, bankKnown), "bankWholeWeeksWithoutFuel": v.Nullable(math.Floor(bank/weekly), bankKnown), "bankWeeksWithFuel": v.Nullable(bank/weeklyWithFuel, bankKnown && fuelCostKnown), "bankWholeWeeksWithFuel": v.Nullable(math.Floor(bank/weeklyWithFuel), bankKnown && fuelCostKnown),
	}
	return summary, services
}

func (c *Calculator) Snapshot() M {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, _ := calculatorCatalog(c.selected)
	options := v.Copy(c.plans[c.selected])
	summary, services := calculatorSummary(t, options)
	comparisons := []M{}
	for _, kind := range []string{"fleet", "squadron"} {
		typ, _ := calculatorCatalog(kind)
		totals, _ := calculatorSummary(typ, c.plans[kind])
		comparisons = append(comparisons, totals)
	}
	return v.Copy(M{"selectedType": c.selected, "name": t.Name, "options": options, "services": services, "summary": summary, "comparison": comparisons, "settings": c.exportLocked(), "source": M{"name": "Pilots Trade Network · игровые параметры служб", "url": CalculatorSourceURL, "checkedAt": "2026-11-08"}, "notes": []string{
		"Содержание за 30 и 365 дней — пропорциональная оценка по недельной сумме. Реальные списания происходят по недельному циклу игры, поэтому календарный месяц может включать четыре или пять списаний.",
		"Обслуживание прыжка не включает покупку трития. Для полного бюджета движения укажите средний расход и цену трития; неизвестные значения не считаются нулём.",
		"Приостановленная служба остаётся установленной: её цена и занимаемая вместимость сохраняются, уменьшается только недельное содержание.",
		"Верфь эскадрильного флотоносца встроена, бесплатна и постоянно занимает 3 000 т. Снять или приостановить её нельзя.",
		"Прочий занятый объём включает товары, тритий в трюме и пакеты кораблей/модулей. Отдельный топливный бак не отнимается здесь повторно.",
		"Резерв и срок работы банка рассчитаны при неизменных службах, графике прыжков и цене топлива, без учёта торгового дохода.",
	}})
}
