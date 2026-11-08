package reference

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

var moduleAliases = map[string]string{"bsg": "sg", "psg": "sg", "advmc": "mc", "amr": "mr"}
var moduleDescriptions = map[string]string{
	"am":  "Восстанавливает целостность повреждённых модулей за счёт собственного запаса ремонтных материалов. Не восстанавливает корпус и не заменяет средства ремонта силовой установки.",
	"bh":  "Определяют базовую защиту корпуса и его сопротивления урону. Более тяжёлые варианты повышают защищённость ценой массы, которая влияет на полёт и прыжки.",
	"bl":  "Лазер непрерывного действия с высокой тепловой и энергетической нагрузкой. Для длительного огня особенно важны распределитель питания и температура.",
	"c":   "Кинетическое орудие с ограниченным боезапасом и сравнительно медленными снарядами. Лучше раскрывается при точном выборе дистанции и упреждения.",
	"cc":  "Управляет дронами, подбирающими груз и материалы. Время работы, дальность и число активных дронов зависят от конкретного класса и исполнения контроллера.",
	"ch":  "Создаёт помехи наведению части оружия противника. Имеет ограниченный боезапас и не заменяет щит или манёвр уклонения.",
	"cs":  "Позволяет исследовать грузовой трюм выбранного корабля. Длительность и дальность сканирования влияют на практическое применение.",
	"csl": "Собирает накопленную каустическую нагрузку и позволяет сбросить заполненный поглотитель. Применяется в условиях каустической среды; это другое устройство, чем тепловые ловушки.",
	"ec":  "Активная защита от совместимых управляемых боеприпасов и устройств. Требует своевременного применения и соблюдения дальности действия.",
	"fc":  "Орудие ближней дистанции с залпом множества кинетических снарядов. Эффективность сильно зависит от рассеивания и положения цели.",
	"fi":  "Помогает вывести другой корабль из суперкруиза. Работает с собственной дальностью, углом захвата и процедурой противодействия.",
	"fs":  "Пополняет топливо корабля у подходящей звезды в суперкруизе. Дальность гиперпрыжка не определяет скорость забора топлива; важны класс и рейтинг этого модуля.",
	"fsd": "Обеспечивает суперкруиз и гиперпрыжки. Дальность зависит от возможностей двигателя, массы корабля и топлива; SCO доступен только поддерживающему его исполнению.",
	"fx":  "Управляет дронами передачи топлива другому кораблю. Использует запас топлива вашего корабля и требует расходных дронов.",
	"hb":  "Управляет дронами взлома грузового люка. Доступ к содержимому и правовые последствия зависят от выбранной цели и обстановки.",
	"hr":  "Повышает прочность корпуса и может менять сопротивления урону. Использует дополнительный отсек и увеличивает массу корабля.",
	"hs":  "Отводит накопленное тепло в сбрасываемую ловушку. Ограниченный боезапас требует планирования на длинном маршруте или в бою.",
	"kw":  "Выявляет дополнительные награды за голову выбранного корабля. Само сканирование не делает стрельбу по цели законной в текущей юрисдикции.",
	"ls":  "Поддерживает жизнеобеспечение кабины. При разгерметизации запас аварийного кислорода определяется установленным исполнением и может пополняться синтезом.",
	"mc":  "Кинетическое оружие с устойчивым темпом стрельбы и ограниченным боезапасом. Обычные и улучшенные варианты собраны на этой странице; их доступные рецепты могут различаться.",
	"mr":  "Оружие с неуправляемыми ракетами и ограниченным боезапасом. Обычный и улучшенный варианты имеют разные условия применения и совместимость модификаций.",
	"nl":  "Ракетная установка с захватом цели. Время захвата, боезапас и противодействие управляемым боеприпасам влияют на её результативность.",
	"pa":  "Орудие, выпускающее мощные плазменные сгустки. Большая нагрузка на распределитель и нагрев требуют согласованной работы сборки.",
	"pc":  "Исследует состав астероида до начала добычи. Конкретный класс и рейтинг контроллера важны для числа дронов и эффективности выбранного добывающего цикла.",
	"pd":  "Распределяет запас энергии между системами, двигателями и оружием. Ёмкость конденсаторов и скорость пополнения — разные параметры, которые выбирают под задачу.",
	"pl":  "Лазер импульсного действия для термического урона. Важны баланс дальности, потребления, нагрева и устойчивого огня.",
	"po":  "Автоматическая защита от части приближающихся боеприпасов и дронов. Расположение установки на корпусе и запас боеприпасов влияют на защиту.",
	"pp":  "Производит энергию для модулей и влияет на тепловой режим. Общая выработка и тепловая эффективность не являются одним показателем.",
	"rf":  "Перерабатывает собранные минеральные фрагменты в товарный груз. Число бункеров и состав фрагментов определяют, как часто придётся управлять очисткой.",
	"rg":  "Электромагнитное оружие с задержкой зарядки перед выстрелом. Точность, энергозатраты и нагрев важны для эффективной работы.",
	"s":   "Определяют работу корабельных сенсоров и обнаружение контактов. Более дальний вариант может увеличить массу и энергопотребление.",
	"sb":  "Усиливает генератор щита и меняет итоговую защиту с учётом собственных модификаций. Не создаёт щит без самого генератора.",
	"scb": "Пополняет действующий щит за счёт ограниченных зарядов. Требует энергии и создаёт существенную тепловую нагрузку.",
	"sg":  "Создаёт щит корабля. Стандартный, би-плетёный и призматический варианты отличаются запасом, восстановлением и условиями приобретения; они объединены в одну страницу семейства.",
	"ss":  "Картографирует поверхность и кольца зондами. Инженерная модификация помогает повышать покрытие и эффективность картографирования.",
	"t":   "Обеспечивают движение в обычном пространстве. Масса корабля, допустимая масса двигателя и модификации влияют на скорость и управляемость.",
	"tp":  "Выпускает тяжёлые управляемые торпеды. Небольшой боезапас делает каждое попадание и выбор экспериментального эффекта особенно важными.",
	"ul":  "Лазер, выпускающий короткие серии импульсов. Комбинирует термический урон с нагрузкой на распределитель и систему охлаждения.",
	"ws":  "Сканирует следы FSD и собирает связанные с переходами данные. Следы высокоэнергетических переходов и обычные контакты кораблей сканируются по-разному.",
}

var engineerImages = map[string]string{"300000": "didi_vatermann", "300010": "bill_turner", "300030": "broo_tarquin", "300040": "the_sarge", "300050": "zacariah_nemo", "300080": "liz_ryder2", "300090": "hera_tani", "300100": "felicity_farseer", "300110": "ram_tah", "300120": "lei_cheung", "300130": "petra_olmanova", "300140": "bris_dekker", "300150": "marsha_hicks", "300160": "elvira_martuuk", "300180": "the_dweller", "300200": "marco_qwent", "300210": "selene_jean", "300220": "prof_palin", "300230": "lori_jameson", "300250": "juri_ishmaak", "300260": "tod_mcquinn", "300270": "tiana_fortune", "300280": "mel_brandon", "300290": "etienne_dorn", "300300": "chloe_sedesi"}

func canonicalModule(key string) string {
	if canonical := moduleAliases[key]; canonical != "" {
		return canonical
	}
	return key
}

func blueprintID(raw v.M) string { return "blueprint/" + slug(v.Text(raw["english"], raw["name"])) }
func experimentalID(raw v.M) string {
	return "experimental/" + slug(v.Text(raw["english"], raw["name"]))
}

func (s *Store) buildEngineering() {
	c := data.Object("engineering-data.json")
	stats := data.Object("encyclopedia-engineering-stats.json")
	mods := v.Rows(c["modules"])
	engineers := v.Rows(c["engineers"])
	materials := map[string]v.M{}
	bps, effects := map[string]v.M{}, map[string]v.M{}
	for _, m := range v.Rows(c["materials"]) {
		materials[v.Str(m["key"])] = m
		s.addMaterial(m)
	}
	for _, m := range v.Rows(c["blueprints"]) {
		bps[v.Str(m["key"])] = m
	}
	for _, m := range v.Rows(c["experimentals"]) {
		effects[v.Str(m["key"])] = m
	}
	moduleGroups := map[string][]v.M{}
	for _, m := range mods {
		key := v.Str(m["key"])
		canonical := canonicalModule(key)
		moduleGroups[canonical] = append(moduleGroups[canonical], m)
		s.aliases["module/"+key] = "module/" + canonical
	}
	for key, variants := range moduleGroups {
		title := v.Str(variants[0]["name"])
		for _, m := range variants {
			if v.Str(m["key"]) == key {
				title = v.Str(m["name"])
			}
		}
		if key == "csl" {
			title = "Пусковая установка каустических ловушек"
		}
		group := moduleGroup(key)
		e := Entry{ID: "module/" + key, Category: "modules", Group: group, Title: title, Subtitle: "Семейство корабельных модулей", Description: moduleDescriptions[key], Properties: []Property{{Label: "Тип", Value: s.groupTitle(group)}, {Label: "Вариантов в каталоге", Value: fmt.Sprint(len(variants))}}}
		rows := [][]string{}
		for _, m := range variants {
			rows = append(rows, []string{v.Str(m["name"]), fmt.Sprint(len(v.Arr(m["blueprints"]))), fmt.Sprint(len(v.Arr(m["experimentals"])))})
		}
		e.Sections = []Section{{ID: "variants", Title: "Исполнения модуля", Type: "table", Columns: []string{"Исполнение", "Чертежи", "Экспериментальные эффекты"}, Rows: rows}, {ID: "selection", Title: "Выбор класса и исполнения", Type: "requirements", Items: []Item{{Title: "Совместимость с кораблём", Text: "Класс модуля и ограничения отсека проверяются в оснащении. Одинаковое семейство не означает одинаковые массу, выработку или допустимую нагрузку."}, {Title: "Инженерная модификация", Text: "Чертёж меняет набор характеристик. Учитывайте побочные эффекты и число модулей в плане."}, {Title: "Экспериментальный эффект", Text: "Дополняет совместимый чертёж и оплачивается отдельным рецептом. Установка доступна при личном посещении подходящего инженера."}}}}
		r := s.add(e, key)
		seenBP, seenEX := map[string]bool{}, map[string]bool{}
		for _, m := range variants {
			for _, key := range v.Arr(m["blueprints"]) {
				if bp := bps[v.Str(key)]; bp != nil {
					id := blueprintID(bp)
					if !seenBP[id] {
						r.relatedIDs = append(r.relatedIDs, id)
						seenBP[id] = true
					}
				}
			}
			for _, key := range v.Arr(m["experimentals"]) {
				if ex := effects[v.Str(key)]; ex != nil {
					id := experimentalID(ex)
					if !seenEX[id] {
						r.relatedIDs = append(r.relatedIDs, id)
						seenEX[id] = true
					}
				}
			}
		}
	}
	for _, raw := range v.Rows(c["blueprints"]) {
		id := blueprintID(raw)
		s.aliases["blueprint/"+v.Str(raw["key"])], s.aliases["blueprints/"+v.Str(raw["key"])] = id, id
		r := s.records[id]
		if r == nil {
			r = s.add(Entry{ID: id, Category: "blueprints", Group: "ship-blueprints", Title: v.Str(raw["name"]), Subtitle: v.Str(raw["english"]), Description: blueprintDescription(v.Str(raw["english"])), Properties: []Property{{Label: "Применение", Value: "Корабельные модули"}}, Sections: []Section{{ID: "about", Title: "Как читать параметры", Type: "prose", Paragraphs: []string{"Стрелка показывает диапазон развития выбранного класса от начального к предельному значению. Проценты относятся к параметру модуля, а п. п. — к изменению сопротивления. Итоговые показатели корабля зависят от всей сборки.", "Материалы указаны за один ролл, а не за полное завершение класса. Экспериментальный эффект и другие модули рассчитываются отдельно."}}}}, v.Str(raw["english"]))
		}
		moduleNames := []string{}
		for _, m := range mods {
			if hasKey(m["blueprints"], v.Str(raw["key"])) {
				moduleNames = append(moduleNames, v.Str(m["name"]))
				r.relatedIDs = append(r.relatedIDs, "module/"+canonicalModule(v.Str(m["key"])))
			}
		}
		grades := []Grade{}
		for grade := 1; grade <= 5; grade++ {
			recipe := v.Rows(v.Obj(raw["grades"])[fmt.Sprint(grade)])
			if len(recipe) == 0 {
				continue
			}
			ingredients := recipeIngredients(recipe, materials)
			grades = append(grades, Grade{Grade: grade, Title: fmt.Sprintf("Класс G%d: один ролл", grade), Materials: ingredients, Effects: blueprintModifiers(stats, v.Str(raw["key"]), grade)})
			for _, item := range ingredients {
				if mat := s.records[item.EntryID]; mat != nil {
					mat.relatedIDs = append(mat.relatedIDs, id)
				}
				r.relatedIDs = append(r.relatedIDs, item.EntryID)
			}
		}
		r.entry.Sections = append(r.entry.Sections, Section{ID: "recipe-" + v.Str(raw["key"]), Title: strings.Join(unique(moduleNames), " · "), Type: "grades", Grades: grades})
	}
	for _, raw := range v.Rows(c["experimentals"]) {
		id := experimentalID(raw)
		s.aliases["experimental/"+v.Str(raw["key"])], s.aliases["experimentals/"+v.Str(raw["key"])] = id, id
		r := s.records[id]
		if r == nil {
			r = s.add(Entry{ID: id, Category: "experimentals", Group: "ship-effects", Title: v.Str(raw["name"]), Subtitle: v.Str(raw["english"]), Description: experimentalDescription(v.Str(raw["english"]))}, v.Str(raw["english"]))
		}
		names := []string{}
		for _, m := range mods {
			if hasKey(m["experimentals"], v.Str(raw["key"])) {
				names = append(names, v.Str(m["name"]))
				r.relatedIDs = append(r.relatedIDs, "module/"+canonicalModule(v.Str(m["key"])))
			}
		}
		ingredients := recipeIngredients(v.Rows(raw["materials"]), materials)
		r.entry.Sections = append(r.entry.Sections, Section{ID: "recipe-" + v.Str(raw["key"]), Title: strings.Join(unique(names), " · "), Type: "grades", Grades: []Grade{{Title: "Установка эффекта", Materials: ingredients}}})
		for _, item := range ingredients {
			r.relatedIDs = append(r.relatedIDs, item.EntryID)
			if mat := s.records[item.EntryID]; mat != nil {
				mat.relatedIDs = append(mat.relatedIDs, id)
			}
		}
	}
	s.buildEngineers(engineers, mods, bps)
	s.groupNames["ship-blueprints"], s.groupNames["ship-effects"] = "Корабельные чертежи", "Корабельные эффекты"
}

func recipeIngredients(rows []v.M, materials map[string]v.M) []Ingredient {
	out := []Ingredient{}
	for _, item := range rows {
		key := v.Str(item["key"])
		out = append(out, Ingredient{Name: v.Text(materials[key]["name"], key), Count: v.Int(item["count"]), EntryID: "material/" + key})
	}
	return out
}

func (s *Store) addMaterial(m v.M) {
	category := v.Str(m["category"])
	title := map[string]string{"raw": "Сырьё", "manufactured": "Произведённые материалы", "encoded": "Данные"}[category]
	description := map[string]string{"raw": "Природный инженерный элемент. Хранится в отдельном запасе сырья, не занимает грузовой трюм и применяется в чертежах и синтезе.", "manufactured": "Компонент для инженерных модификаций. Собирается и учитывается в запасе материалов отдельно от товарного груза.", "encoded": "Инженерные данные, получаемые при сканировании и выполнении задач. Они хранятся отдельно от грузового трюма."}[category]
	gather := map[string]string{"raw": "Проверяйте элементный состав тела. Для наземного сбора нужны посадка и подходящие образования; сырьё некоторых видов также получается при добыче астероидов.", "manufactured": "Ищите подходящие источники сигналов, обломки кораблей и награды заданий. Тип сигнала, фракция и состояние системы влияют на доступные компоненты.", "encoded": "Используйте подходящий тип сканирования: обычные корабельные сканы, сканирование следов FSD, точки данных и награды миссий дают разные наборы данных."}[category]
	key := v.Str(m["key"])
	if strings.Contains(key, "wake") || strings.Contains(key, "hyperspace") {
		gather = "Собирайте данные сканером следов FSD у высокоэнергетических следов уходящих кораблей. Дальность сканера и устойчивый поток вылетающих кораблей определяют удобство сбора."
	}
	e := Entry{ID: "material/" + key, Category: "materials", Group: "material-" + category, Title: v.Str(m["name"]), Subtitle: v.Str(m["english"]), Description: description, Properties: []Property{{Label: "Категория", Value: title}, {Label: "Класс", Value: fmt.Sprintf("G%d", v.Int(m["grade"]))}, {Label: "Лимит хранения", Value: fmt.Sprint(v.Int(m["capacity"]))}}, Badges: []Badge{{Label: fmt.Sprintf("G%d", v.Int(m["grade"]))}}, Sections: []Section{{ID: "collection", Title: "Получение и хранение", Type: "prose", Paragraphs: []string{gather, "Лимит относится к этому виду материала. Он не является вместимостью грузового отсека корабля."}}, {ID: "trade", Title: "Обмен и использование", Type: "requirements", Items: []Item{{Title: "Обменник своей категории", Text: "Сырьё, произведённые компоненты и данные обмениваются раздельно. Разные классы и семейства имеют разные коэффициенты обмена."}, {Title: "Сначала инженерный план", Text: "Резервируйте необходимое количество для выбранных чертежей и эффектов; остаток можно использовать для обмена."}, {Title: "Расход на рецепт", Text: "В карточке чертежа указан расход за один ролл. Для полного улучшения класса могут понадобиться несколько роллов."}}}}}
	tradeable, exchangeKnown := m["tradeable"].(bool)
	exchange := "Не подтверждён"
	if tradeable {
		exchange = "Доступен"
	} else if exchangeKnown {
		exchange = "Недоступен"
	}
	e.Properties = append(e.Properties, Property{Label: "Обмен у торговца", Value: exchange})
	if !tradeable {
		statusText := "Для этого материала возможность обмена у обычного торговца материалами не подтверждена. Не закладывайте такой обмен в план получения."
		if exchangeKnown {
			statusText = "Этот материал нельзя получить или сдать у обычного торговца материалами. Запас пополняется сбором из соответствующих источников."
		}
		e.Sections[1] = Section{ID: "trade", Title: "Доступность обмена и применение", Type: "requirements", Items: []Item{{Title: "Обмен у обычного торговца", Text: statusText}, {Title: "Сохраните нужный запас", Text: "Резервируйте материал под конкретную технологию, рецепт или условие открытия. Не рассчитывайте заменить его обычными сырьём, компонентами или данными через обмен."}, {Title: "Проверьте конкретное требование", Text: "Специальные материалы применяются в определённых технологиях и задачах. Сверяйте название и количество с требованием выбранного улучшения или контакта."}}}
		if exchangeKnown {
			e.Badges = append(e.Badges, Badge{Label: "Не обменивается"})
			guardian := strings.HasPrefix(key, "guardian") || strings.HasPrefix(key, "ancient")
			if guardian {
				e.Description = "Материал или данные Стражей для связанных технологий. Хранятся в отдельном запасе материалов и не занимают товарный трюм. Обмен у обычных торговцев материалами недоступен."
				e.Sections[0].Paragraphs[0] = "Ищите на древних объектах и в руинах Стражей. Способы получения компонентов, данных и фрагментов чертежей различаются."
			} else {
				e.Description = "Специальный материал или данные таргоидского происхождения. Хранятся отдельно от товарного груза; обычный торговец материалами не обменивает этот запас."
				e.Sections[0].Paragraphs[0] = "Источник зависит от конкретного материала: таргоидские корабли, объекты или связанные с ними обломки. Обычные трофеи человеческих кораблей и стандартные сканы не заменяют эти специальные источники."
				if key == "unknownenergysource" {
					e.Sections[0].Paragraphs[0] = "Ищите у таргоидских объектов и в нечеловеческих источниках сигналов. Обломки также получают при разрушении таргоидского сенсора."
				}
				if key == "unknownwakedata" {
					e.Sections[0].Paragraphs[0] = "Получается при изучении таргоидских кораблей. Это специальные данные таргоидских следов, а не обычные результаты сканирования следов человеческих кораблей."
				}
			}
		}
	}
	s.add(e, v.Str(m["symbol"]))
	s.groupNames[e.Group] = title
}

func moduleGroup(key string) string {
	if strings.Contains(" bl c fc mc mr nl pa pl rg tp ul ", " "+key+" ") {
		return "module-weapons"
	}
	if strings.Contains(" fsd t pp pd ls s bh ", " "+key+" ") {
		return "module-core"
	}
	if strings.Contains(" ch csl ec hs kw po sb ws cs ", " "+key+" ") {
		return "module-utility"
	}
	return "module-optional"
}

func (s *Store) groupTitle(group string) string {
	title := map[string]string{"module-weapons": "Вооружение", "module-core": "Основные модули", "module-utility": "Служебные устройства", "module-optional": "Дополнительные модули"}[group]
	s.groupNames[group] = title
	return title
}
func hasKey(values any, key string) bool {
	for _, v0 := range v.Arr(values) {
		if v.Str(v0) == key {
			return true
		}
	}
	return false
}
func unique(values []string) []string {
	set := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if value != "" && !set[value] {
			set[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func blueprintDescription(name string) string {
	if text := lookupDescription(blueprintDescriptions, name); text != "" {
		return text
	}
	if text := map[string]string{"Increased range": "Увеличивает дальность гиперпрыжка через параметры FSD. Конкретный класс улучшения меняет результат и сопутствующие характеристики.", "Lightweight": "Уменьшает массу совместимого модуля. Побочные изменения прочности и энергопотребления зависят от семейства модуля.", "Heavy duty": "Усиливает защитные характеристики совместимого модуля. Масса и итоговая защита рассчитываются для конкретного исполнения.", "Dirty": "Повышает рабочие возможности двигателей ценой сопутствующих ограничений и тепловой нагрузки.", "Clean": "Меняет характеристики двигателей с акцентом на более холодную работу. Результат зависит от массы корабля и выбранного класса.", "Overcharged": "Повышает основной выходной параметр: у оружия — урон, у силовой установки — выработку. Побочные эффекты и рецепты зависят от типа модуля.", "Long range": "Увеличивает рабочую дальность совместимого оборудования. Для оружия, сканеров и других устройств применяются разные рецепты и изменения характеристик.", "Expanded Probe Scanning Radius": "Увеличивает область покрытия зонда подробного сканера поверхности. Помогает картографировать тело меньшим количеством зондов."}[name]; text != "" {
		return text
	}
	return "Семейство инженерных чертежей «" + name + "». Выберите ниже тип модуля: рецепт и сопутствующие изменения относятся именно к этому варианту, а не ко всем модулям с похожим названием."
}

func experimentalDescription(name string) string {
	if text := lookupDescription(effectDescriptions, name); text != "" {
		return text
	}
	if text := map[string]string{"Mass Manager": "Увеличивает оптимальную массу FSD и применяется как отдельный эффект к совместимому инженерному чертежу.", "Deep Charge": "Меняет допустимый расход топлива FSD за прыжок. Выгоду нужно оценивать вместе с классом двигателя и требуемым запасом топлива.", "Corrosive Shell": "Добавляет коррозионное воздействие при попадании совместимого оружия. Эффект и боезапас учитываются отдельно от основного чертежа.", "Thermal Vent": "Меняет тепловое поведение лучевого лазера при попадании и промахе. Работа эффекта зависит от удержания луча на цели.", "Stripped Down": "Дополнительно уменьшает массу совместимого модуля. Рецепт зависит от устройства, на которое устанавливается эффект."}[name]; text != "" {
		return text
	}
	return "Дополнительный инженерный эффект «" + name + "». Он не заменяет основной чертёж и требует отдельного набора материалов при установке у инженера. Совместимые модули и их рецепты приведены ниже."
}
