package reference

func axLocationSections() []Section {
	return []Section{
		{
			ID: "ax-hunting-grounds", Title: "Где искать таргоидов в Live", EnglishTitle: "Where to hunt in Live", Type: "route",
			Paragraphs: []string{
				"После гибели Кокиджо в декабре 2024 года охота на живых титанов закончилась. Для одиночных боёв ищите нечеловеческие источники сигналов (Non-Human Signal Sources, NHSS), прежде всего в Плеядах. Маршруты относятся к Live 4.0 и Odyssey. Активность сверена по наблюдениям пилотов осенью 2026 года, состав каждого сигнала меняется.",
			},
			EnglishParagraphs: []string{
				"Live Titan assaults ended after Cocijo fell in December 2024. For solo fights, look for Non-Human Signal Sources (NHSS), especially in the Pleiades. These routes cover Live 4.0 and Odyssey. Locations were checked against commander reports in autumn 2026, and individual signal contents vary.",
			},
			Items: []Item{
				{Title: "Asterope · первая дуэль", EnglishTitle: "Asterope · first duel", System: "Asterope",
					Text:        "Выбирайте NHSS 5 с цилиндрическим значком обломков, чтобы искать одиночного «Циклопа». Для тренировки по разведчикам берите NHSS 4. Перед вылетом отметьте станцию с ремонтом, боеприпасами и площадкой вашего размера.",
					EnglishText: "Choose a threat 5 NHSS with the cylindrical salvage icon to look for a lone Cyclops. Try threat 4 for scout practice. Before launching, bookmark a station with repairs, restock and a pad that fits your ship."},
				{Title: "Merope · запасной район Плеяд", EnglishTitle: "Merope · another Pleiades option", System: "Merope",
					Text:        "Проверьте новые сигналы здесь, если в Asterope нет подходящего. Старая точка The Zoo (Pleiades Sector MI-S B4-0) после колонизации почти перестала давать NHSS.",
					EnglishText: "Check for fresh signals here if Asterope has no suitable contacts. The old Zoo (Pleiades Sector MI-S B4-0) now produces very few NHSS after colonisation."},
				{Title: "Sterope II · поиск «Гидры»", EnglishTitle: "Sterope II · Hydra hunting", System: "Sterope II",
					Text:        "Ищите одиночные сигналы восьмого уровня. Подходит для опытных пилотов, которые уверенно проходят «Медузу». Если встреча смешанная, выберите другой сигнал.",
					EnglishText: "Look for lone contacts in threat 8 signals. This is for experienced pilots who can reliably beat a Medusa. If the encounter is mixed, pick another signal."},
				{Title: "Угольный Мешок · сигналы высокой угрозы", EnglishTitle: "Coalsack · higher-threat signals", System: "Musca Dark Region HM-V c2-17",
					Text:        "Здесь часто встречаются сигналы повышенной угрозы. Заранее проверьте доступ к ремонту и пополнению боекомплекта. Личный или доступный флотоносец сокращает время между вылетами.",
					EnglishText: "Higher-threat signals are common here. Check access to repairs and restock before departing. A fleet carrier you can use cuts turnaround time."},
				{Title: "California Sector BA-A e6 · бои волнами", EnglishTitle: "California Sector BA-A e6 · wave combat", System: "California Sector BA-A e6",
					Text:        "AX-зона высокой интенсивности у тела 4 использовалась пилотами осенью 2026 года. Зоны в California Sector HR-W d1-28 у тела 3 и California Sector JH-V c2-12 у тела 6 также отмечены сообществом. Проверяйте, что сценарий и противники появляются после входа. Это тренировка по нескольким целям, для первой дуэли удобнее отдельный NHSS.",
					EnglishText: "Pilots used the high-intensity AX conflict zone at body 4 in autumn 2026. The community also reports zones at body 3 in California Sector HR-W d1-28 and body 6 in California Sector JH-V c2-12. Check that the scenario starts and enemies spawn after dropping. These are multi-target encounters. Use a separate NHSS for your first duel."},
			},
		},
		{
			ID: "ax-select-instance", Title: "Как выбрать сигнал и уйти из неудачной встречи", EnglishTitle: "Choose a signal and leave a bad instance", Type: "route",
			Paragraphs: []string{
				"Постоянный сигнал бедствия в HR 1185 и сценарии у повреждённых мегакораблей в Deciat или Gree дают другие встречи, иногда сразу с несколькими кораблями. В районе Туманности Головы Ведьмы тоже бывают NHSS, но игроки Odyssey сообщают о проблемах с производительностью. Гиперперехват может привести к встрече с несколькими перехватчиками и импульсу отключения, поэтому для первой победы используйте обычный сигнал.",
			},
			EnglishParagraphs: []string{
				"The permanent distress call in HR 1185 and damaged megaship scenarios in Deciat or Gree offer other encounters, sometimes with several ships. The Witch Head region also has NHSS, though Odyssey players report performance issues. Hyperdictions can bring multiple Interceptors and a shutdown pulse, so use a regular signal for your first kill.",
			},
			Items: []Item{
				axStep("В суперкруизе", "In supercruise",
					"Откройте полнополосный сканер FSS или просканируйте навигационный маяк, если он есть. Выберите «Нечеловеческий источник сигнала» в навигационной панели. Справа в полосе NHSS чаще всего одиночные перехватчики или пустые встречи, слева бывают разведчики или смешанные группы. Для дуэли ищите цилиндрический значок обломков у сигнала 5–8.",
					"Open the Full Spectrum System Scanner (FSS) or scan the navigation beacon if available. Select a Non-Human Signal Source in the navigation panel. The right side of the NHSS band favours lone Interceptors or empty instances. The left side has scouts or mixed groups. For a duel, look for the cylindrical salvage icon on threat 5–8 signals."),
				axStep("Уровень угрозы подсказывает цель", "Threat level narrows the search",
					"Уровень 4 подходит для разведчиков. Для одиночных перехватчиков 5 обычно означает «Циклопа», 6 — «Василиска», 7 — «Медузу», 8 — «Гидру», если регион её даёт. Без значка обломков в уровне 5 могут быть только разведчики, а в 6–8 — разведчики и перехватчик классом ниже. Уровень 9 оставьте для боя с группой противников.",
					"Threat 4 is suitable for scouts. For lone Interceptors, threat 5 usually means Cyclops, 6 Basilisk, 7 Medusa and 8 Hydra where the region spawns it. Without the salvage icon, threat 5 may contain only scouts, and 6–8 may hold scouts and an Interceptor one tier lower. Save threat 9 for fighting several opponents."),
				axStep("После входа сначала опознайте цель", "Identify the target before firing",
					"Для первого боя нужен один «Циклоп» без эскорта. В одиночном сигнале он обычно сначала сканирует корабль. Оставьте на станции таргоидские зонды, датчики и грузовые артефакты Стражей, которые могут спровоцировать нападение. Установленные модули Стражей не считаются таким грузом. При наличии разведчиков цель может сразу стать враждебной.",
					"For your first fight, choose one Cyclops without an escort. A lone Interceptor normally starts by scanning you. Leave Thargoid probes, sensors and Guardian cargo artefacts behind, as they can provoke an attack. Fitted Guardian modules are separate from cargo. Scouts can cause the Interceptor to become hostile immediately."),
				axStep("Если цель не та или корабль повреждён", "If the target is wrong or your ship is damaged",
					"Заранее выберите соседнюю систему. Если в сигнале несколько противников, закончились теплоотводы, корпус быстро теряет прочность или повреждены двигатели и FSD, прекращайте атаку, пока можете маневрировать. Убирайте орудия, набирайте дистанцию ускорением и прыгайте в соседнюю систему. Если услышали предупреждение об энергетическом всплеске, переведите четыре деления в SYS. Примерно через секунду после предупреждения включите нейтрализатор, если он установлен, и удерживайте кнопку до прохождения волны. Прыжок в другую систему помогает избежать задержки разгона из-за близкого корабля.",
					"Preselect a nearby system. If several opponents are present, heat sinks run out, hull is failing quickly or thrusters and FSD are taking damage, disengage while you can still manoeuvre. Retract hardpoints, boost away and jump to the other system. If you hear the energy surge warning, put four pips into SYS. Start your shutdown neutraliser about one second after the warning if fitted and hold it until the pulse passes. A hyperspace jump avoids the charging delay caused by a nearby ship."),
			},
		},
	}
}

func axRewardSections() []Section {
	return []Section{
		{
			ID: "ax-bonds", Title: "Выплаты за цели", EnglishTitle: "Bonds by target", Type: "table",
			Paragraphs: []string{
				"Таблица показывает базовые боевые облигации за уничтожение цели в Live. После победы проверьте «Транзакции», затем сдайте ваучеры через «Услуги станции → Контакты → Боевые облигации». Награды задания или AX-зоны могут начисляться отдельно.",
			},
			EnglishParagraphs: []string{
				"The table shows base combat bonds per kill in Live. After the fight, check Transactions and redeem your vouchers through Starport Services → Contacts → Combat Bond Contact. A mission or an AX conflict zone can pay additional rewards separately.",
			},
			Columns:        []string{"Цель", "Выплата", "Что учитывать"},
			EnglishColumns: []string{"Target", "Bond", "Planning note"},
			Rows: [][]string{
				{"Разведчик, любой вариант", "80 000 кр.", "Доступная тренировка и сбор материалов. 100 разведчиков дают 8 млн кр."},
				{"Cyclops / «Циклоп»", "8 млн кр.", "Первый и наиболее удобный перехватчик для повторных соло-вылетов."},
				{"Basilisk / «Василиск»", "24 млн кр.", "Быстрее «Циклопа». Выгоден, если стабильно берёте сердца и переживаете выход из атаки."},
				{"Medusa / «Медуза»", "40 млн кр.", "Дольше бой и выше расход материалов, больше требований к ремонту."},
				{"Hydra / «Гидра»", "60 млн кр.", "Высокая выплата за победу. Сравнивайте с доходом за серию более быстрых вылетов."},
				{"Glaive и Scythe", "4,5 млн кр. каждый", "Охотники. У этих типов нет надёжного маршрута для регулярного фарма после войны."},
				{"Orthrus / «Орф»", "15 млн кр.", "Историческая выплата. Активные шпили и устойчивый фарм этой цели после войны не подтверждены."},
			},
			EnglishRows: [][]string{
				{"Scout, any variant", "80,000 Cr", "Accessible practice and material collection. One hundred scouts pay 8 million Cr."},
				{"Cyclops", "8 million Cr", "The first and most convenient Interceptor for repeat solo sorties."},
				{"Basilisk", "24 million Cr", "Faster than Cyclops. Worth hunting once you can reliably take hearts and disengage after a pass."},
				{"Medusa", "40 million Cr", "Longer fight with higher material and repair demands."},
				{"Hydra", "60 million Cr", "High payout per kill. Compare with the proceeds from several shorter sorties."},
				{"Glaive and Scythe", "4.5 million Cr each", "Hunters. There is no reliable postwar farming route for either type."},
				{"Orthrus", "15 million Cr", "Historical payout. Active spires and consistent postwar farming are unconfirmed."},
			},
		},
		{
			ID: "ax-net-profit", Title: "Доход за вылет и материалы с обломков", EnglishTitle: "Sortie earnings and salvage", Type: "route",
			Items: []Item{
				axStep("Считайте полный цикл", "Measure the full loop",
					"Если за 45 минут от выхода со станции до сдачи ваучеров вы победили трёх «Циклопов», получите 24 млн кр. до расходов, или 32 млн кр./ч. Для своего результата вычтите боеприпасы, ремонт и страховые выкупы, а время на перелёт, поиск сигналов и сбор ингредиентов синтеза включите в расчёт. Одна неудачная встреча может изменить лучший выбор цели.",
					"Three Cyclops kills in 45 minutes, measured from undocking to handing in the bonds, pay 24 million Cr before costs, or 32 million Cr per hour. Subtract ammunition, repairs and rebuys for your own result. Include travel, signal hunting and gathering synthesis ingredients in the time. One failed encounter can change which target is most profitable for you."),
				axStep("Что собирать после разведчиков", "What to collect after killing scouts",
					"Из разведчиков в сигналах 4–5 можно получить части оружия, компоненты обломков и биомеханические проводники. Они нужны для покупки предмодифицированных AX-мультипушек Azimuth. Это инженерные материалы, они хранятся отдельно от груза.",
					"Scouts in threat 4–5 signals can drop Weapon Parts, Wreckage Components and Bio-Mechanical Conduits. These materials buy pre-engineered Azimuth AX Multi-Cannons. They go into material storage separately from cargo."),
				axStep("Как забрать сердце", "How to recover a heart",
					"После взрыва перехватчика отойдите примерно на 4,5 км и дождитесь, пока исчезнет едкое облако. Сердце — коррозионно-активный груз. Для перевозки нужен стойкий к коррозии грузовой стеллаж, а для подбора удобен коллектор. Сердце и 10 единиц метасплавов можно обменять у человеческого технологического брокера на декоративную фигурку таргоида.",
					"After an Interceptor explodes, move about 4.5 km away and wait for the caustic cloud to clear. A Thargoid Heart is corrosive cargo. Use a corrosion-resistant cargo rack to carry it and a collector limpet to retrieve it. A Human Technology Broker exchanges one heart and ten Meta-Alloys for the cosmetic Thargoid bobblehead."),
			},
		},
	}
}
