package ranks

// The guides below are original practical workflows. Credit/kill thresholds
// are intentionally not estimated: only the game's observed percentage is used.
var guides = map[string][]text{
	"combat": {
		{"Боевой ранг относится к боям на корабле. Начните с доступного оснащению боя, проверяйте законность цели и сохраняйте деньги на восстановление. Ранг цели и обстоятельства боя влияют на продвижение; одинаковое число уничтожений не гарантирует одинаковый прирост.", "Combat rank covers ship combat. Choose encounters your build can handle, verify target legality and retain a rebuy reserve. Target rank and combat circumstances affect advancement; an equal kill count does not guarantee equal progress."},
		{"Практика: отметьте текущий ранг и процент, проведите ограниченный боевой сеанс, затем сравните новое наблюдение журнала. Если процент не обновился, перезагрузите игровой сеанс для нового Rank/Progress. Copilot не пересчитывает скрытые очки по числу убийств.", "Practice: note your rank and percentage, complete a bounded combat session and compare the next journal observation. If the percentage has not refreshed, start a new game session to obtain Rank/Progress. Copilot does not reconstruct hidden XP from kill counts."},
	},
	"trade": {
		{"Торговое продвижение связано с учитываемой игрой прибылью, а не с общим балансом или оборотом. Покупка флотоносца и перевод собственного груза сами по себе не доказывают рост ранга. Проверяйте результат по игровому проценту.", "Trade advancement follows profit recognised by the game, not your total balance or turnover. Buying a carrier or transferring your own cargo does not itself prove rank progress. Verify the result using the game's percentage."},
		{"Практика: в «Рейсах» выберите выполнимый маршрут, сохраните котировки, выполните сделки и сравните процент до и после сеанса. Отдельно оценивайте прибыль, время и риск. Высокая выручка при дорогой закупке может дать небольшую маржу.", "Practice: choose a feasible trade route, save its quotes, complete the trades and compare progress before and after the session. Evaluate profit, duration and risk separately. High revenue can still produce a small margin when the purchase cost is high."},
	},
	"explore": {
		{"Исследовательский ранг и экзобиология — разные направления. Данные звёзд и планет продаются через Universal Cartographics; генетические образцы — через Vista Genomics. Не складывайте обе выплаты в один расчёт ранга.", "Exploration and exobiology are separate careers. Sell stellar and planetary data through Universal Cartographics and genetic samples through Vista Genomics. Do not combine both payouts into one rank calculation."},
		{"Практика: выберите посильную экспедицию, отмечайте картографированные тела и запланируйте безопасную продажу данных. После продажи проверьте новый процент. До сдачи данных предполагаемая стоимость не является подтверждённым продвижением.", "Practice: choose a manageable expedition, track mapped bodies and plan a safe data sale. Check the next percentage after selling. An estimated data value before sale is not confirmed rank advancement."},
	},
	"soldier": {
		{"Ранг наёмника относится к пешему бою Odyssey и не заменяет корабельный боевой ранг. Отделяйте награды и продвижение, подтверждённое игрой, от примерной доходности выбранной зоны конфликта.", "Mercenary rank covers Odyssey on-foot combat and is separate from ship Combat rank. Distinguish financial rewards and game-confirmed advancement from a conflict zone's estimated earnings."},
		{"Практика: проверьте костюм и оружие в инженерном плане, выберите подходящую сложность и ограничьте сеанс по времени. Сопоставляйте завершённые бои с наблюдениями Rank/Progress; не обещайте себе следующий ранг по фиксированному числу зон.", "Practice: review suit and weapon requirements in your engineering plan, select an appropriate difficulty and set a session time limit. Compare completed battles with Rank/Progress observations rather than promising the next rank after a fixed number of zones."},
	},
	"exobiologist": {
		{"Экзобиологический ранг связан с учтённой игрой продажей органических данных. Три пробы завершают набор, но сами по себе ещё не подтверждают выплату. Предполагаемый бонус первого высаживания не следует считать полученным заранее.", "Exobiologist rank follows organic data sales recognised by the game. Three samples complete a set but do not yet confirm a payout. Do not count a prospective first-footfall bonus as already received."},
		{"Практика: в «Экзобиологии» сохраните планету, соберите три разнесённые пробы, проверьте данные на борту и продайте их в Vista Genomics. Затем сравните подтверждённую продажу с новым процентом. Неизвестные координаты старых проб остаются неизвестными.", "Practice: bookmark a planet in Exobiology, collect three sufficiently separated samples, review data on board and sell it to Vista Genomics. Compare the confirmed sale with the next rank percentage. Unknown coordinates of historical samples remain unknown."},
	},
	"cqc": {
		{"Здесь показан карьерный ранг CQC Федерации пилотов, включая Elite I–V. У арены также есть отдельная шкала уровней 1–50 и сброс Prestige.", "This screen tracks Pilots Federation CQC career rank, including Elite I–V. Arena also has its own level 1–50 progression and Prestige reset."},
		{"Для разрешения CD-43 11917 требуется отдельный Prestige арены после уровня 50. Attilius Orbital предлагает скидки 20% на отдельные корабли и 30% на отдельное вооружение.", "The CD-43 11917 permit requires the separate Arena Prestige option after level 50. Attilius Orbital offers 20% off selected ships and 30% off selected weapon modules."},
	},
	"federation": {
		{"Выберите нужную цель: Sol открывается на Petty Officer, Federal Corvette — на Rear Admiral. Работа выполняется для малых фракций, связанных с Федерацией. Репутация и военно-морское звание — разные шкалы; союзная репутация сама по себе не означает высокий чин.", "Choose a practical target: Sol opens at Petty Officer and the Federal Corvette at Rear Admiral. Work for minor factions aligned with the Federation. Reputation and naval rank are separate scales; Allied reputation does not itself imply a high naval rank."},
		{"Когда шкала достигнет 100%, проверьте предложения повышения Federal Navy на корабельной доске миссий. Прочитайте цель задания и завершите его в игре. Пока Promotion не подтверждено, Copilot сохраняет прежнее звание и показывает 100%, не выдавая его за автоматическое повышение.", "At 100%, look for a Federal Navy promotion opportunity on the ship mission board. Read its requirements and complete it in the game. Until Promotion is confirmed, Copilot retains your existing rank and displays 100% rather than treating it as an automatic promotion."},
	},
	"empire": {
		{"Планируйте продвижение под цель: Imperial Courier — Master, Achenar — Squire, Imperial Clipper — Baron, Imperial Cutter — Duke. Выполняйте подходящие задания имперских малых фракций и сравнивайте репутационную награду с затратами и риском.", "Plan advancement around a target: Imperial Courier at Master, Achenar at Squire, Imperial Clipper at Baron and Imperial Cutter at Duke. Complete suitable missions for Imperial minor factions, weighing reputation rewards against cost and risk."},
		{"При 100% ищите задание повышения Imperial Navy на корабельной доске миссий. Одновременно можно развивать оба флота. Запланируйте бюджет на корпус, модули и восстановление открытого корабля.", "At 100%, look for an Imperial Navy promotion mission on the ship mission board. You can advance in both navies. Budget for the unlocked ship's hull, outfitting and rebuy."},
	},
}

func guideFor(id, language string) M {
	sections := []M{}
	for index, paragraph := range guides[id] {
		title := text{"Как продвигаться", "How to advance"}
		if index > 0 {
			title = text{"Практический следующий шаг", "A practical next step"}
		}
		sections = append(sections, M{"id": []string{"method", "practice"}[index], "title": title.Get(language), "paragraphs": []string{paragraph.Get(language)}})
	}
	return M{"title": text{"Ранг как рабочая цель", "Turn rank into a practical goal"}.Get(language), "sections": sections}
}
