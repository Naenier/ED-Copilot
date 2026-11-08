package ranks

type guideSection struct {
	ID         string
	Title      text
	Paragraphs []text
}

// Keep discipline-specific methods here. Unlocks live beside their rank in
// the timeline; unknown progression thresholds are not estimated.
var guides = map[string][]guideSection{
	"combat": {{"method", text{"Бой на корабле", "Ship combat"}, []text{
		{"Боевой ранг повышается в боях на корабле. Ранг цели и обстоятельства боя влияют на продвижение.", "Combat rank advances through ship combat. Target rank and combat circumstances affect progression."},
	}}},
	"trade": {{"method", text{"Торговая прибыль", "Trading profit"}, []text{
		{"Торговый ранг повышается за прибыль, которую учитывает игра. Для торгового рейса прибыль от товара равна выручке от продажи минус затраты на закупку.", "Trade rank advances through profit recognised by the game. A trade trip's commodity profit is sale revenue minus purchase cost."},
	}}},
	"explore": {
		{"method", text{"Звёзды и планеты", "Stars and planets"}, []text{{"Сканируйте звёзды и планеты, картографируйте тела и сохраняйте данные до продажи.", "Scan stars and planets, map bodies and retain the data until sale."}}},
		{"sale", text{"Universal Cartographics", "Universal Cartographics"}, []text{{"Данные астрономической разведки продаются через Universal Cartographics. Продажа генетических образцов относится к отдельному экзобиологическому рангу.", "Sell astronomical exploration data through Universal Cartographics. Genetic sample sales contribute to the separate Exobiologist rank."}}},
	},
	"soldier": {{"method", text{"Пешие бои Odyssey", "Odyssey on-foot combat"}, []text{
		{"Ранг наёмника повышается в пеших боях Odyssey, в том числе в наземных зонах конфликта.", "Mercenary rank advances through Odyssey on-foot combat, including ground conflict zones."},
	}}},
	"exobiologist": {
		{"method", text{"Сбор образцов", "Collecting samples"}, []text{{"Соберите три разнесённые пробы одного вида. Завершённый набор остаётся на борту до продажи.", "Collect three sufficiently separated samples of one species. The completed set remains on board until sold."}}},
		{"sale", text{"Vista Genomics", "Vista Genomics"}, []text{{"Экзобиологический ранг повышается при продаже органических данных в Vista Genomics. Бонус первой высадки подтверждается при продаже.", "Exobiologist rank advances when organic data is sold to Vista Genomics. The first-footfall bonus is confirmed at sale."}}},
	},
	"cqc": {
		{"method", text{"Ранг CQC", "CQC rank"}, []text{{"Карьерный ранг CQC Федерации пилотов включает Elite I–V. У арены есть отдельная шкала уровней 1–50 и сброс Prestige.", "Pilots Federation CQC career rank includes Elite I–V. Arena has a separate level 1–50 progression and Prestige reset."}}},
		{"permit", text{"Разрешение CD-43 11917", "CD-43 11917 permit"}, []text{{"Для разрешения CD-43 11917 требуется Prestige арены после уровня 50. Attilius Orbital предлагает скидки 20% на отдельные корабли и 30% на отдельное вооружение.", "The CD-43 11917 permit requires Arena Prestige after level 50. Attilius Orbital offers 20% off selected ships and 30% off selected weapon modules."}}},
	},
	"federation": {
		{"method", text{"Федеральные фракции", "Federal factions"}, []text{{"Выполняйте задания малых фракций, связанных с Федерацией. Petty Officer открывает Sol, Rear Admiral — покупку Federal Corvette.", "Complete missions for minor factions aligned with the Federation. Petty Officer unlocks Sol; Rear Admiral unlocks Federal Corvette purchase access."}}},
		{"promotion", text{"Повышение при 100%", "Promotion at 100%"}, []text{{"При 100% найдите и выполните задание повышения Federal Navy на корабельной доске миссий.", "At 100%, find and complete a Federal Navy promotion mission on the ship mission board."}}},
	},
	"empire": {
		{"method", text{"Имперские фракции", "Imperial factions"}, []text{{"Выполняйте задания имперских малых фракций. Imperial Courier открывается на Master, Achenar — Squire, Imperial Clipper — Baron, Imperial Cutter — Duke.", "Complete missions for Imperial minor factions. Imperial Courier unlocks at Master, Achenar at Squire, Imperial Clipper at Baron and Imperial Cutter at Duke."}}},
		{"promotion", text{"Повышение при 100%", "Promotion at 100%"}, []text{{"При 100% найдите и выполните задание повышения Imperial Navy на корабельной доске миссий. Продвижение во флотах Империи и Федерации доступно одновременно.", "At 100%, find and complete an Imperial Navy promotion mission on the ship mission board. You can advance in both the Imperial and Federal navies."}}},
	},
}

func guideFor(id, language string) M {
	sections := []M{}
	for _, section := range guides[id] {
		paragraphs := []string{}
		for _, paragraph := range section.Paragraphs {
			if value := paragraph.Get(language); value != "" {
				paragraphs = append(paragraphs, value)
			}
		}
		if len(paragraphs) > 0 {
			sections = append(sections, M{"id": section.ID, "title": section.Title.Get(language), "paragraphs": paragraphs})
		}
	}
	return M{"title": "", "sections": sections}
}
