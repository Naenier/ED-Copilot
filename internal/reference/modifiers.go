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
		if key == "optmul" && strings.HasPrefix(recipe, "ShieldGenerator_") {
			label = "Оптимальный множитель щита"
		}
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
	"ammo": "Общий боезапас", "angle": "Угол сканирования", "boot": "Время запуска", "brokenregen": "Восстановление разрушенного щита", "burst": "Выстрелов в очереди", "burstrof": "Темп внутри очереди", "cargo": "Грузовой запас", "clip": "Ёмкость магазина", "damage": "Урон", "distdraw": "Расход энергии распределителя", "duration": "Продолжительность работы", "eff": "Тепловой коэффициент силовой установки", "engcap": "Ёмкость ENG", "engrate": "Пополнение ENG", "explres": "Взрывное сопротивление", "facinglimit": "Угол захвата", "falloff": "Дальность начала падения урона", "fallofffromrange": "Начало падения урона", "hullboost": "Усиление корпуса", "hullreinforcement": "Добавленная прочность корпуса", "integrity": "Целостность модуля", "jitter": "Разброс", "kinres": "Кинетическое сопротивление", "mass": "Масса", "optmass": "Оптимальная масса", "optmul": "Оптимальный множитель двигателя", "pgen": "Выработка энергии", "piercing": "Бронепробитие", "power": "Потребление энергии", "proberadius": "Покрытие зонда", "range": "Дальность", "ranget": "Дальность захвата", "reload": "Время перезарядки", "rof": "Интервал между очередями", "scantime": "Время сканирования", "shieldboost": "Усиление щита", "shieldreinforcement": "Добавленный запас щита", "shotspeed": "Скорость снаряда", "spinup": "Время подготовки", "syscap": "Ёмкость SYS", "sysrate": "Пополнение SYS", "thermload": "Тепловая нагрузка", "thermres": "Термическое сопротивление", "wepcap": "Ёмкость WEP", "weprate": "Пополнение WEP",
}

// descriptions describe the mechanic. Numerical module-specific tradeoffs live
// alongside the relevant grade; families are not incorrectly treated as equal.
var blueprintDescriptions = map[string]string{
	"Anti-Guardian Zone Resistance":  "Рам Та защищает совместимый модуль Стражей от деградации в антистражевом поле. Такая защита нужна для оружия Стражей в зоне действия поля.",
	"Plasma conversion":              "У термического оружия наёмников 15,5% урона становятся абсолютными. На G5 общий урон снижается на 11%, а потребление энергии растёт на 40%.",
	"Scoop rate enhanced":            "На G5 топливозаборник принимает на 50% больше топлива в секунду и потребляет на 15% больше энергии. За один и тот же проход у звезды можно заправить больше топлива.",
	"Ammo capacity":                  "Увеличивает запас зарядов совместимого устройства. Компромисс по массе и другим характеристикам зависит от типа пусковой установки.",
	"Armoured":                       "На G5 целостность силовой установки растёт на 120%, а выработка на 12%. Коэффициент нагрева снижается на 12%, масса увеличивается на 20%.",
	"Blast resistant":                "Усиливает взрывную защиту совместимого защитного модуля. Сопоставляйте её с кинетическим и термическим сопротивлением всей сборки.",
	"Charge enhanced":                "На G5 SYS, ENG и WEP пополняются на 45% быстрее, а ёмкость каждого конденсатора уменьшается на 5%. Подходит для повторных форсажей и продолжительного огня.",
	"Clean":                          "На G5 тепловая нагрузка двигателей снижается на 60%, а оптимальный множитель тяги растёт на 28%. Двигатели потребляют на 16% больше энергии даже при меньшем нагреве от тяги.",
	"Dirty":                          "На G5 оптимальный множитель тяги растёт на 40%. Тепловая нагрузка повышается на 60%, а оптимальная масса падает на 12,5%, поэтому проверяйте скорость при полной загрузке.",
	"Double shot":                    "Меняет дробовую пушку для дополнительного выстрела в очереди. Учитывайте скорость расходования магазина, боезапаса и энергии.",
	"Efficient":                      "Улучшает энергетическую и тепловую экономичность совместимого оружия. Изменения урона и прочих параметров смотрите для конкретного семейства.",
	"Engine focused":                 "Смещает возможности распределителя в сторону конденсатора двигателей ENG. Ёмкость и пополнение SYS и WEP могут уменьшиться.",
	"Enhanced low power":             "Снижает массу и энергопотребление генератора щита. Защиту и побочные изменения нужно оценивать относительно итогового щита корабля.",
	"Expanded capture arc":           "Расширяет угол захвата перехватчика FSD. Это повышает удобство начала перехвата, но меняет другие рабочие параметры устройства.",
	"Expanded Probe Scanning Radius": "На G5 радиус покрытия зонда подробного сканера поверхности увеличивается на 50%. Крупное тело можно картографировать меньшим числом зондов и легче уложиться в лимит для бонуса эффективности.",
	"Fast scan":                      "Уменьшает время сканирования совместимым сканером. Выбирайте вместе с нужной рабочей дальностью, массой и энергетическим бюджетом.",
	"Faster boot sequence":           "На G5 время запуска FSD после включения сокращается на 80%, а оптимальная масса растёт на 15%. Пригодится, если FSD отключается при выпуске оружия из-за приоритетов питания. Время зарядки для прыжка остаётся прежним.",
	"Focused":                        "Меняет оружие для большей эффективной дальности и бронепробития. Точные прибавки и энергетические компромиссы зависят от вида оружия.",
	"Heavy duty":                     "Повышает основные защитные характеристики брони или усилителя. Масса и изменения сопротивлений зависят от выбранного типа модуля.",
	"High capacity":                  "Увеличивает боезапас и ёмкость магазина совместимого оружия. Рассчитывайте массу, перезарядки и продолжительность вылета для конкретного модуля.",
	"High charge capacity":           "На G5 ёмкость каждого конденсатора увеличивается на 42%, скорость пополнения падает на 18%. Позволяет дольше держать первый залп с полным WEP, но удлиняет паузу до следующего.",
	"Increased range":                "На G5 оптимальная масса FSD растёт на 55%. Для дальних рейсов сравните итоговую дальность с «Управлением массой» и «Глубокой зарядкой» на загруженном корабле.",
	"Kinetic resistant":              "Усиливает кинетическую защиту совместимого модуля. Итоговую живучесть определяет сочетание сопротивлений и абсолютного запаса защиты.",
	"Lightweight":                    "Снижает массу совместимого модуля. Прочность, боезапас и другие параметры могут измениться; рецепт и компромиссы различаются по оборудованию.",
	"Long range":                     "У оружия G5 удваивает предельную дальность и переносит начало падения урона на эту границу. У сканеров и перехватчиков даёт дальность захвата ценой меньшего угла.",
	"Low emissions":                  "На G5 коэффициент нагрева силовой установки снижается на 65%, выработка падает на 15%, масса растёт на 20%. Пригодится для холодной сборки, если оставшейся мощности хватает на все нужные модули.",
	"Overcharged":                    "У силовой установки G5 выработка растёт на 40%, но она становится горячее и уязвимее. У оружия G5 урон выстрела растёт на 70% ценой расхода энергии WEP и нагрева. У мультипушки уменьшается магазин.",
	"Rapid charge":                   "На G4 время подготовки банки щита сокращается на 40%. Заряд длится меньше, зато даёт на 20% больше защиты. Короткое окно запуска оставляет противнику меньше времени для Feedback Cascade.",
	"Rapid fire":                     "На G5 интервал между очередями сокращается на 44%, время перезарядки на 65%. Снижается урон выстрела и добавляется 0,5° разброса, что особенно заметно на дальней дистанции.",
	"Reinforced":                     "Усиливает основные защитные характеристики генератора или совместимого защитного оборудования. Результат сопровождается изменениями массы, энергии или сопротивлений.",
	"Resistance augmented":           "Смещает усилитель щита в сторону сопротивлений разным типам урона. Оценивайте совокупные сопротивления всех усилителей, а не складывайте проценты напрямую.",
	"Shielded":                       "На G5 большинство вспомогательных устройств получает +300% целостности и удвоенное потребление. У FSD вырастает оптимальная масса и снижается тепловая нагрузка, а распределитель питания потребляет меньше энергии.",
	"Short range":                    "На G5 урон растёт на 75%, предельная дальность сокращается вдвое, тепловая нагрузка растёт на 40%. Подходит для оружия, которым вы стабильно попадаете с близкого расстояния.",
	"Specialised":                    "Банка щита G4 выделяет на 24% меньше тепла и восстанавливает на 10% больше щита. Потребление энергии растёт, а целостность снижается.",
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
