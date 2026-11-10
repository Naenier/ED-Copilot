package reference

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// EngineeringRecipeFacts uses the same descriptions and grades as the
// encyclopedia for the one recipe currently open in the planner.
func EngineeringRecipeFacts(blueprintKey, effectKey string) (string, string, map[int][]Property) {
	catalog := data.Object("engineering-data.json")
	stats := data.Object("encyclopedia-engineering-stats.json")
	grades := map[int][]Property{}
	description, experimental := "", ""
	if blueprintKey != "" {
		for _, blueprint := range v.Rows(catalog["blueprints"]) {
			if blueprint["key"] != blueprintKey {
				continue
			}
			description = blueprintDescription(v.Str(blueprint["english"]))
			for grade := 1; grade <= 5; grade++ {
				properties := blueprintModifiers(stats, blueprintKey, grade)
				if len(properties) == 0 {
					properties = catalogueModifiers(blueprint, grade)
				}
				if len(properties) != 0 {
					grades[grade] = properties
				}
			}
			break
		}
	}
	if effectKey != "" {
		for _, effect := range v.Rows(catalog["experimentals"]) {
			if effect["key"] == effectKey {
				experimental = experimentalDescription(v.Str(effect["english"]))
				break
			}
		}
	}
	return description, experimental, grades
}

func blueprintModifiers(stats v.M, recipe string, grade int) []Property {
	features := v.Obj(v.Obj(v.Obj(stats["blueprints"])[recipe])[fmt.Sprint(grade)])
	definitions := v.Obj(stats["properties"])
	keys := []string{}
	for key := range features {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := []Property{}
	for _, key := range keys {
		limits := v.Arr(features[key])
		if len(limits) != 2 {
			continue
		}
		label := modifierLabels[key]
		if label == "" {
			continue
		}
		definition := v.Obj(definitions[key])
		format := func(raw any) string {
			n := v.Num(raw)
			if v.Str(definition["type"]) == "percentage" {
				unit := "%"
				if v.Str(definition["method"]) == "additive" {
					unit = " п. п."
				}
				return fmt.Sprintf("%+.4g%s", n*100, unit)
			}
			if key == "jitter" {
				return fmt.Sprintf("%g°", n)
			}
			if key == "fallofffromrange" && n == 1 {
				return "равна дальности оружия"
			}
			return fmt.Sprintf("%g", n)
		}
		first, last := format(limits[0]), format(limits[1])
		value := last
		if first != last {
			value = first + " → " + last
		}
		out = append(out, Property{Label: label, Value: value})
	}
	return out
}

func catalogueModifiers(blueprint v.M, grade int) []Property {
	stats := v.Obj(v.At(blueprint, "gradeStats", fmt.Sprint(grade)))
	out := []Property{}
	labels := map[string]string{"RefuelRate": "Скорость забора топлива", "PowerDraw": "Потребление энергии", "Damage": "Урон"}
	for _, feature := range v.Rows(stats["features"]) {
		key := v.Str(feature["label"])
		if key == "GuardianModuleResistance" {
			out = append(out, Property{Label: "Защита от антистражевых полей", Value: "Включена"})
			continue
		}
		label := v.Text(labels[key], key)
		low, high := fmt.Sprintf("%+.4g%%", v.Num(feature["min"])*100), fmt.Sprintf("%+.4g%%", v.Num(feature["max"])*100)
		if low != high {
			high = low + " → " + high
		}
		out = append(out, Property{Label: label, Value: high})
	}
	for _, damage := range []struct{ key, title string }{{"thermal", "Термический урон"}, {"absolute", "Абсолютный урон"}, {"kinetic", "Кинетический урон"}, {"explosive", "Взрывной урон"}} {
		if share, ok := v.Number(v.At(stats, "damageDistribution", damage.key)); ok {
			out = append(out, Property{Label: damage.title, Value: fmt.Sprintf("%.4g%%", share*100)})
		}
	}
	return out
}

var modifierLabels = map[string]string{
	"ammo": "Общий боезапас", "angle": "Угол сканирования", "boot": "Время запуска", "brokenregen": "Восстановление разрушенного щита", "burst": "Выстрелов в очереди", "burstrof": "Темп внутри очереди", "cargo": "Грузовой запас", "clip": "Ёмкость магазина", "damage": "Урон", "distdraw": "Расход энергии распределителя", "duration": "Продолжительность работы", "eff": "Тепловой коэффициент силовой установки", "engcap": "Ёмкость ENG", "engrate": "Пополнение ENG", "explres": "Взрывное сопротивление", "facinglimit": "Угол захвата", "falloff": "Дальность начала падения урона", "fallofffromrange": "Начало падения урона", "hullboost": "Усиление корпуса", "hullreinforcement": "Добавленная прочность корпуса", "integrity": "Целостность модуля", "jitter": "Разброс", "kinres": "Кинетическое сопротивление", "mass": "Масса", "optmass": "Оптимальная масса", "optmul": "Оптимальный множитель двигателя", "pgen": "Выработка энергии", "piercing": "Бронепробитие", "power": "Потребление энергии", "proberadius": "Покрытие зонда", "range": "Дальность", "ranget": "Дальность захвата", "reload": "Время перезарядки", "rof": "Скорострельность", "scantime": "Время сканирования", "shieldboost": "Усиление щита", "shieldreinforcement": "Добавленный запас щита", "shotspeed": "Скорость снаряда", "spinup": "Время подготовки", "syscap": "Ёмкость SYS", "sysrate": "Пополнение SYS", "thermload": "Тепловая нагрузка", "thermres": "Термическое сопротивление", "wepcap": "Ёмкость WEP", "weprate": "Пополнение WEP",
}

// descriptions describe the mechanic. Numerical module-specific tradeoffs live
// alongside the relevant grade; families are not incorrectly treated as equal.
var blueprintDescriptions = map[string]string{
	"Anti-Guardian Zone Resistance":  "Защищает совместимый модуль Стражей от деградации в антистражевом поле.",
	"Plasma conversion":              "Переводит часть урона лазера в абсолютный урон. Меняет потребление энергии и общий урон.",
	"Scoop rate enhanced":            "Повышает скорость забора топлива за счёт дополнительного энергопотребления.",
	"Ammo capacity":                  "Увеличивает запас зарядов совместимого устройства. Компромисс по массе и другим характеристикам зависит от типа пусковой установки.",
	"Armoured":                       "Укрепляет силовую установку и меняет её энергетические и тепловые характеристики. Повышенная прочность не делает установку неуязвимой, а увеличение массы влияет на полёт.",
	"Blast resistant":                "Усиливает взрывную защиту совместимого защитного модуля. Сопоставляйте её с кинетическим и термическим сопротивлением всей сборки.",
	"Charge enhanced":                "Ускоряет пополнение конденсаторов распределителя за счёт их ёмкости. Подходит для устойчивой нагрузки, но требует оценки запаса на единичный форсаж или залп.",
	"Clean":                          "Модифицирует двигатели с акцентом на пониженную тепловую нагрузку. Скорость и расход энергии оцениваются вместе с массой корабля.",
	"Dirty":                          "Повышает скоростные характеристики двигателей ценой большей тепловой нагрузки и других компромиссов. Итог зависит от отношения массы корабля к параметрам двигателя.",
	"Double shot":                    "Меняет дробовую пушку для дополнительного выстрела в очереди. Учитывайте скорость расходования магазина, боезапаса и энергии.",
	"Efficient":                      "Улучшает энергетическую и тепловую экономичность совместимого оружия. Изменения урона и прочих параметров смотрите для конкретного семейства.",
	"Engine focused":                 "Смещает возможности распределителя в сторону конденсатора двигателей ENG. Ёмкость и пополнение SYS и WEP могут уменьшиться.",
	"Enhanced low power":             "Снижает массу и энергопотребление генератора щита. Защиту и побочные изменения нужно оценивать относительно итогового щита корабля.",
	"Expanded capture arc":           "Расширяет угол захвата перехватчика FSD. Это повышает удобство начала перехвата, но меняет другие рабочие параметры устройства.",
	"Expanded Probe Scanning Radius": "Расширяет область покрытия зонда подробного сканера поверхности. Помогает картографировать тело меньшим числом зондов.",
	"Fast scan":                      "Уменьшает время сканирования совместимым сканером. Выбирайте вместе с нужной рабочей дальностью, массой и энергетическим бюджетом.",
	"Faster boot sequence":           "Сокращает запуск FSD после включения модуля. Не следует путать время запуска выключенного модуля со стандартной подготовкой каждого гиперпрыжка.",
	"Focused":                        "Меняет оружие для большей эффективной дальности и бронепробития. Точные прибавки и энергетические компромиссы зависят от вида оружия.",
	"Heavy duty":                     "Повышает основные защитные характеристики брони или усилителя. Масса и изменения сопротивлений зависят от выбранного типа модуля.",
	"High capacity":                  "Увеличивает боезапас и ёмкость магазина совместимого оружия. Рассчитывайте массу, перезарядки и продолжительность вылета для конкретного модуля.",
	"High charge capacity":           "Увеличивает ёмкость конденсаторов распределителя, жертвуя скоростью пополнения. Помогает с пиковой потребностью, но не гарантирует более долгую непрерывную работу.",
	"Increased range":                "Увеличивает оптимальную массу FSD, повышая потенциальную дальность гиперпрыжка. Итоговая дальность зависит от массы корабля, топлива и экспериментального эффекта.",
	"Kinetic resistant":              "Усиливает кинетическую защиту совместимого модуля. Итоговую живучесть определяет сочетание сопротивлений и абсолютного запаса защиты.",
	"Lightweight":                    "Снижает массу совместимого модуля. Прочность, боезапас и другие параметры могут измениться; рецепт и компромиссы различаются по оборудованию.",
	"Long range":                     "Увеличивает рабочую дальность оружия, сканера или другого совместимого оборудования. Дальность начала падения урона и скорость снарядов относятся только к подходящему оружию.",
	"Low emissions":                  "Снижает тепловой коэффициент силовой установки ценой выработки и массы. Сначала проверьте, достаточно ли остаётся энергии для рабочего оснащения.",
	"Overcharged":                    "Повышает основной выходной параметр: урон оружия либо выработку силовой установки. Тепловые, энергетические и прочностные компромиссы указаны отдельно для каждого оборудования.",
	"Rapid charge":                   "Ускоряет выдачу энергии банкой щита. Меняет длительность и эффективность заряда; полезность зависит от управления нагревом и момента применения.",
	"Rapid fire":                     "Повышает темп работы оружия. Учитывайте расход боезапаса, разброс, нагрев и нагрузку на распределитель при непрерывном огне.",
	"Reinforced":                     "Усиливает основные защитные характеристики генератора или совместимого защитного оборудования. Результат сопровождается изменениями массы, энергии или сопротивлений.",
	"Resistance augmented":           "Смещает усилитель щита в сторону сопротивлений разным типам урона. Оценивайте совокупные сопротивления всех усилителей, а не складывайте проценты напрямую.",
	"Shielded":                       "Увеличивает целостность совместимого модуля, обычно повышая энергопотребление. Не добавляет отдельный энергетический щит самому устройству.",
	"Short range":                    "Повышает боевые параметры оружия ценой рабочей дальности и сопутствующих ограничений. Предполагает удержание цели на короткой дистанции.",
	"Specialised":                    "Оптимизирует работу банки щита и её тепловые параметры. Сравнивайте изменения ёмкости, времени подготовки и расхода заряда для выбранного класса.",
	"Sturdy":                         "Укрепляет оружие и меняет его бронепробитие и тепловые параметры. Повышенная масса входит в общий бюджет сборки.",
	"System focused":                 "Смещает возможности распределителя в пользу системного конденсатора SYS. Учитывайте влияние на запас и пополнение энергии двигателей и оружия.",
	"Thermal resistant":              "Усиливает термическую защиту совместимого защитного модуля. Кинетическое и взрывное сопротивления могут измениться вместе с ней.",
	"Weapon focused":                 "Смещает возможности распределителя в пользу оружейного конденсатора WEP. Помогает выбранной огневой нагрузке ценой возможностей других конденсаторов.",
	"Wide angle":                     "Расширяет рабочий угол сканера. Дальность и время сканирования могут отличаться от других чертежей этого же устройства.",
}

func lookupDescription(values map[string]string, name string) string {
	for key, value := range values {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}
