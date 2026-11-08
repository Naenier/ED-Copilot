package reference

import (
	"sort"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/data"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (s *Store) buildCommodities() {
	keys := map[string]bool{}
	englishNames, categories := map[string]string{}, map[string]string{}
	for _, row := range objectRows("encyclopedia-commodity-metadata.json") {
		key := v.Str(row["key"])
		keys[key], englishNames[key], categories[key] = true, v.Str(row["english"]), v.Str(row["category"])
	}
	for alias, raw := range data.Object("commodity-names.json") {
		values := v.Arr(raw)
		if len(values) < 2 {
			continue
		}
		key := v.Key(values[1])
		keys[key], englishNames[key] = true, v.Str(values[0])
		s.aliases["commodity/"+alias] = "commodity/" + v.Str(trading.Good(key, nil)["key"])
	}
	for raw := range data.Object("commodity-ru.json") {
		keys[strings.TrimSuffix(raw, "name")] = true
	}
	for _, resource := range trading.Resources() {
		if v.Str(resource["kind"]) == "commodity" {
			keys[v.Str(resource["key"])] = true
		}
	}
	ordered := []string{}
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		g := trading.Good(key, nil)
		if v.Str(g["kind"]) != "commodity" {
			continue
		}
		key = v.Str(g["key"])
		if s.records["commodity/"+key] != nil {
			continue
		}
		methods := stringArray(g["methods"])
		category := categories[key]
		group, label := "commodity-"+slug(category), commodityCategories[category]
		if label == "" {
			group, label = "commodity-special", "Редкие и специальные товары"
		}
		if len(methods) > 0 {
			label += " · добыча"
		}
		description := v.Text(g["description"], "Товар для перевозки в грузовом трюме и операций на товарном рынке. Выгодность рейса определяется конкретными предложениями станций, объёмом спроса и затратами на доставку.")
		if text := commodityCategoryDescriptions[category]; v.Str(g["description"]) == "" && text != "" {
			description = text
		}
		if text := commodityDescriptions[key]; text != "" {
			description = text
		}
		title, english := v.Str(g["name"]), v.Text(englishNames[key], g["english"])
		if name := commodityRussian[key]; name != "" {
			title = name
		} else if title == key {
			title = english
		}
		e := Entry{ID: "commodity/" + key, Category: "commodities", Group: group, Title: title, Subtitle: english, Description: description, CommodityKey: key, Badges: []Badge{{Label: label}}, Properties: []Property{{Label: "Учёт", Value: "Грузовой трюм · тонны"}, {Label: "Категория", Value: label}},
			Actions:  []Action{{Type: "trade", Label: "Купить", Mode: "buy", CommodityKey: key}, {Type: "trade", Label: "Продать", Mode: "sell", CommodityKey: key}},
			Sections: []Section{{ID: "market", Title: "Покупка и продажа", Type: "requirements", Items: []Item{{Title: "Для покупки", Text: "Откройте поиск продавцов: сравните цену продажи станции, наличие товара и расстояние до системы и посадочной площадки."}, {Title: "Для продажи", Text: "Откройте поиск покупателей: сравните закупочную цену, спрос и свежесть наблюдения. Спрос должен подходить под объём вашего груза."}, {Title: "Правовой статус", Text: "Проверяйте запреты и отметки груза в текущей юрисдикции. Легальность зависит от товара, владельца груза и местных правил."}, {Title: "Котировки", Text: "В этой справочной карточке нет текущей рыночной цены. Актуальные предложения и их время обновления доступны в поиске торговли."}}}}}
		if len(methods) > 0 {
			items := []Item{}
			for _, raw := range v.Rows(g["availability"]) {
				rings := []string{}
				for _, ring := range stringArray(raw["ringTypes"]) {
					rings = append(rings, map[string]string{"metallic": "металлические", "metalrich": "богатые металлами", "rocky": "каменистые", "icy": "ледяные"}[ring])
				}
				method := map[string]string{"laser": "Лазерная добыча", "core": "Разработка ядра", "surface": "Поверхностные залежи астероида", "subsurface": "Подповерхностные залежи"}[v.Str(raw["method"])]
				items = append(items, Item{Title: method, Text: "Кольца: " + strings.Join(rings, ", ") + "."})
			}
			if len(items) > 0 {
				e.Sections = append(e.Sections, Section{ID: "mining", Title: "Где и как добывать", Type: "requirements", Items: items})
			}
		}
		if tips := stringItems(g["tips"]); len(tips) > 0 {
			e.Sections = append(e.Sections, Section{ID: "practice", Title: "Практические советы", Type: "requirements", Items: tips})
		}
		r := s.add(e, key)
		s.groupNames[group] = strings.Split(label, " · ")[0]
		r.relatedIDs = []string{"article/commodity-market", "article/trade-profit", "article/mining-methods"}
		if key == "tritium" {
			r.relatedIDs = append(r.relatedIDs, "article/carrier-fuel")
		}
	}
}

var commodityCategories = map[string]string{"Metals": "Металлы", "Minerals": "Минералы", "Foods": "Продукты", "Textiles": "Текстиль", "Industrial Materials": "Промышленные материалы", "Chemicals": "Химикаты", "Medicines": "Медицина", "Legal Drugs": "Алкоголь и вещества", "Machinery": "Машины и оборудование", "Technology": "Технологии", "Weapons": "Вооружение", "Consumer Items": "Потребительские товары", "Slavery": "Работорговля", "Waste": "Отходы", "Salvage": "Находки и спасательные грузы", "NonMarketable": "Служебные расходники"}
var commodityCategoryDescriptions = map[string]string{
	"Metals":               "Металл для промышленного снабжения и товарной торговли. Источник поставки, закупочный спрос и транспортные затраты определяют экономику рейса; отдельные металлы доступны также при добыче астероидов.",
	"Minerals":             "Минеральный товар для промышленных рынков. Не путайте тонны минерала в трюме с инженерными элементами в отдельном запасе материалов. Доступные способы добычи приведены ниже, если они известны.",
	"Foods":                "Продовольственный товар для снабжения станций и торговли. При сравнении рейсов учитывайте доступный объём у продавца, реальный спрос покупателя и место в трюме.",
	"Textiles":             "Текстильный товар для промышленных и потребительских рынков. Это груз, а не инженерный материал; перевозится и продаётся в тоннах.",
	"Industrial Materials": "Промышленное сырьё или компонент товарного рынка. Может потребоваться для снабжения и строительства; наличие в списке товаров не означает хранение в инженерном инвентаре.",
	"Chemicals":            "Химический товар для снабжения и торговли. Назначение груза и местные запреты нужно проверять отдельно; экономический результат зависит от предложения конкретных станций.",
	"Medicines":            "Медицинский товар для снабжения, торговли и отдельных задач доступа. Не расходуется из запаса инженерных материалов: требуется грузовой отсек.",
	"Legal Drugs":          "Потребительский товар категории алкоголя и веществ. Несмотря на название рыночной категории, отдельные товары могут быть запрещены в конкретной юрисдикции.",
	"Machinery":            "Машины или оборудование, перевозимые как товарный груз. Используются в торговых цепочках и снабжении; отдельные позиции нужны для инженерных требований и строительства.",
	"Technology":           "Технологический товар для снабжения станций и торговли. Сравнивайте доступное количество, спрос и полную логистику, а не только разницу цены за тонну.",
	"Weapons":              "Оружейный товар, перевозимый в трюме. Не является устанавливаемым модулем корабля. Перед доставкой проверяйте легальность в системе назначения.",
	"Consumer Items":       "Потребительский товар для торговли и снабжения. Доступность и спрос зависят от станции и текущей обстановки, а перевозка требует места в грузовом трюме.",
	"Slavery":              "Груз категории работорговли. Его правовой статус различается по юрисдикциям; покупка на одной станции не означает разрешённую перевозку или продажу на другой.",
	"Waste":                "Товарный груз категории отходов. Перед перевозкой учитывайте возможные местные ограничения и соотносите выручку со стоимостью и временем доставки.",
	"Salvage":              "Находка или груз спасательной операции. Происхождение, право собственности и условия миссии могут ограничивать продажу; некоторые предметы передаются специальным контактам вместо обычного рынка.",
	"NonMarketable":        "Служебный расходник, учитываемый в грузовом трюме. Обычный товарный рынок может не предлагать его: проверяйте соответствующую службу станции или флотоносца.",
}
var commodityRussian = map[string]string{
	"aganipperush": "Аганиппский кайф", "airelics": "Реликвии ИИ", "antimattercontainmentunit": "Контейнер антиматерии", "antiquities": "Древности", "basicnarcotics": "Наркотики", "battleweapons": "Боевое оружие", "borasetanipathogenetics": "Боразетанские патогенетики", "combatstabilisers": "Боевые стабилизаторы", "damagedescapepod": "Повреждённая спасательная капсула", "drones": "Дроны", "gilyasignatureweapons": "Фирменное оружие Гильи", "hafnium178": "Гафний-178", "hip118311swarm": "Рой HIP 118311", "hostage": "Заложники", "kamorinhistoricweapons": "Историческое оружие Каморина", "landmines": "Наземные мины", "militaryintelligence": "Военная разведывательная информация", "occupiedcryopod": "Занятая спасательная капсула", "onionheadc": "Луковица · штамм гамма", "pavoniseargrubs": "Ушные личинки Павониса", "personaleffects": "Личные вещи", "personalweapons": "Личное оружие", "politicalprisoner": "Политические заключённые", "prohibitedresearchmaterials": "Запрещённые исследовательские материалы", "slaves": "Рабы", "terramaterbloodbores": "Кровососы Терра Матер", "thargoidpod": "Ксенобиологическая тюремная капсула", "toxicwaste": "Токсичные отходы", "trinketsoffortune": "Безделушки скрытой удачи", "usscargoancientartefact": "Древний артефакт", "usscargoblackbox": "Чёрный ящик", "usscargoexperimentalchemicals": "Экспериментальные химикаты", "usscargomilitaryplans": "Военные планы", "usscargoprototypetech": "Прототип технологий", "usscargorebeltransmissions": "Повстанческие передачи", "usscargotechnicalblueprints": "Технические чертежи", "usscargotradedata": "Торговые данные", "wreckagecomponents": "Компоненты обломков",
}

var commodityDescriptions = map[string]string{
	"tritium":          "Топливо для прыжков флотоносца. Перевозится как обычный товар; топливо в грузовом складе и тритий в топливном баке носителя учитываются раздельно.",
	"gold":             "Драгоценный металл для товарной торговли и добычи. Также используется в отдельных инженерных требованиях и проектах снабжения; способ получения зависит от конкретной задачи.",
	"modularterminals": "Товар, необходимый для открытия мастерской Marco Qwent. Отличается от инженерных материалов: занимает грузовой трюм и передаётся в тоннах.",
	"metaalloys":       "Мета-сплавы, связанные с органическими структурами таргоидов. Одна тонна требуется для открытия мастерской Felicity Farseer; учитываются как товарный груз.",
	"landmines":        "Товар для снабжения и открытия мастерской Liz Ryder. Перевозка может быть запрещена в отдельных юрисдикциях, поэтому перед рейсом проверьте местные правила.",
	"lavianbrandy":     "Редкий товар, необходимый Didi Vatermann для открытия мастерской. Ограниченная поставка может потребовать нескольких закупок и рейсов.",
	"kamitracigars":    "Редкий товар для открытия мастерской Hera Tani. Планируйте объём закупки и грузовой отсек отдельно от инженерных материалов.",
	"fujintea":         "Редкий товар для открытия мастерской Broo Tarquin. Доступный объём у продавца может ограничивать количество за один рейс.",
	"konggaale":        "Редкий товар для открытия мастерской Lori Jameson. Его нужно доставлять как товарный груз после получения доступа к системе инженера.",
	"soontillrelics":   "Редкий товар, необходимый Elvira Martuuk для открытия мастерской. Не относится к коллекции инженерных данных и занимает грузовой отсек.",
}
