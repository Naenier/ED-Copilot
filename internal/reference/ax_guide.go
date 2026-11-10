package reference

const axGuideID = "article/ax-preparation"

func axStep(title, englishTitle, text, englishText string) Item {
	return Item{Title: title, EnglishTitle: englishTitle, Text: text, EnglishText: englishText}
}

func (s *Store) buildAXGuide() {
	const group = "article-xeno"
	entry := Entry{
		ID: axGuideID, Category: "articles", Group: group, Subtitle: s.groupNames[group],
		Title: "Соло-охота на таргоидов", EnglishTitle: "Solo Thargoid hunting",
		Description:        "Как найти одиночного перехватчика, открыть и уничтожить его сердца, подобрать AX-корабль и заработать на охоте.",
		EnglishDescription: "Find a lone Interceptor, expose and destroy its hearts, fit an AX ship, and turn kills into combat bonds.",
	}
	entry.Sections = append(entry.Sections, axCombatIntroduction()...)
	entry.Sections = append(entry.Sections, axLocationSections()...)
	entry.Sections = append(entry.Sections, axFightSections()...)
	entry.Sections = append(entry.Sections, axBuildSections()...)
	entry.Sections = append(entry.Sections, axRewardSections()...)
	r := s.add(entry, entry.EnglishTitle, entry.EnglishDescription, "AX", "anti-xeno", "антиксено", "торгоид", "циклоп", "базилиск", "медуза", "гидра", "Cyclops", "Basilisk", "Medusa", "Hydra", "Glaive", "Scythe", "Orthrus", "gauss", "cold orbit")
	r.imageKey = "ax-interceptor"
	r.relatedIDs = []string{
		"article/thargoids", "article/farm-guardian-blueprints", "article/farm-thargoid-encounters",
		"article/farm-guardian-components", "article/farm-thargoid-titans", "article/xeno-sites",
	}
	for _, id := range []string{"article/thargoids", "article/farm-thargoid-encounters"} {
		if other := s.records[id]; other != nil {
			other.relatedIDs = append([]string{axGuideID}, other.relatedIDs...)
		}
	}
	s.aliases["article/ax-solo"] = axGuideID
}

func axCombatIntroduction() []Section {
	return []Section{
		{
			ID: "first-hunt", Title: "С чего начать", EnglishTitle: "Your first hunt", Type: "prose",
			Paragraphs: []string{
				"Для первой победы выберите одиночного Cyclops в нечеловеческом источнике сигнала (NHSS). Разведчики подходят для разминки и проверки вооружения. AX-зона конфликта полезна для группового фарма, но там волны и другие цели меняют ход боя.",
				"Соберите Krait Mk II с усиленными AX-мультипушками, если оружие Стражей ещё закрыто. Если хотите освоить технику, которая пригодится на всех четырёх перехватчиках, начните с Alliance Chieftain, обычных пушек Гаусса и холодной орбиты. Сборки и порядок улучшений приведены ниже.",
				"Перед вылетом пополните боезапас, теплоотводы и дроны, оставьте запас на несколько страховок и проложите маршрут прыжка в соседнюю систему. В первом бою цель проста: снять одно сердце, безопасно пережить ответ и повторить цикл. При нехватке охлаждения, повреждённой силовой установке или пробитом фонаре заканчивайте попытку.",
			},
			EnglishParagraphs: []string{
				"Choose a lone Cyclops in a Non-Human Signal Source (NHSS) for your first kill. Scouts are good for warming up and checking your weapons. An AX Conflict Zone is useful for group farming, where waves and other targets change how a fight unfolds.",
				"Build an Enhanced AX Multi-Cannon Krait Mk II if Guardian weapons are still locked. To learn a technique that carries across all four combat Interceptors, start with an Alliance Chieftain, standard Guardian Gauss Cannons and cold orbiting. Both fits and their upgrade order are below.",
				"Restock ammunition, heatsinks and limpets, keep several rebuys in reserve, and plot a jump to a nearby system. The first fight has one goal: take a heart, survive the response, and repeat. Leave if you run short of cooling or your power plant or canopy can no longer sustain the fight.",
			},
		},
		{
			ID: "interceptors", Title: "Четыре боевых перехватчика", EnglishTitle: "The four combat Interceptors", Type: "table",
			figureKey:            "ax-interceptor-variants",
			FigureCaption:        "Схематические силуэты четырёх боевых вариантов. На обложке статьи — художественная иллюстрация перехватчика.",
			EnglishFigureCaption: "Schematic silhouettes of the four combat variants. The cover is an artistic illustration of an Interceptor.",
			Paragraphs:           []string{"Все четыре используют один цикл сердец. Внешность помогает сориентироваться, но перед атакой проверьте название цели сканером. Скорость в таблице — предел перехватчика в обычном полёте, а размер роя относится к бою в космосе."},
			EnglishParagraphs:    []string{"All four share the heart cycle. The outline and colour give you a clue, but confirm the variant with a scanner before opening fire. Speed is the Interceptor's normal top speed and swarm size refers to fights in space."},
			Columns:              []string{"Вариант", "Сердца", "Скорость и рой", "Как распознать и когда идти в соло"},
			EnglishColumns:       []string{"Variant", "Hearts", "Speed and swarm", "Identification and solo difficulty"},
			Rows: [][]string{
				{"Cyclops", "4", "450 м/с · 32 дрона", "Серо-бирюзовый корпус, сравнительно гладкие лепестки. Первая цель для обучения и самый короткий повторяемый бой."},
				{"Basilisk", "5", "530 м/с · 64 дрона", "Красно-коричневые пятнистые лепестки. Идите после уверенных побед над Cyclops. Скорость мешает просто уйти по прямой."},
				{"Medusa", "6", "450 м/с · 96 дронов", "Красно-чёрная окраска, шипы на лепестках. Осваивайте после Basilisk. Ошибки с нагревом, роем и ремонтом стоят значительно дороже."},
				{"Hydra", "8", "450 м/с · 128 дронов", "Ярко-зелёный корпус, второй ряд лепестков и решётка у центра. Задние лепестки закрывают обзор сердец. Соло для опытного пилота с запасом синтеза."},
			},
			EnglishRows: [][]string{
				{"Cyclops", "4", "450 m/s · 32 drones", "Grey and cyan with relatively smooth petals. The first training target and shortest repeatable fight."},
				{"Basilisk", "5", "530 m/s · 64 drones", "Spotted reddish-brown petals. Try it after consistent Cyclops kills. Its speed makes a straight-line retreat difficult."},
				{"Medusa", "6", "450 m/s · 96 drones", "Red-black hull and spines on the petals. Attempt it after Basilisk. Heat, swarm and repair mistakes cost considerably more."},
				{"Hydra", "8", "450 m/s · 128 drones", "Bright green hull, a second row of petals and a cage over the centre. The rear petals obscure the hearts. Experienced solo pilots should bring synthesis reserves."},
			},
		},
		{
			ID: "other-xenos", Title: "Разведчики, охотники и Orthrus", EnglishTitle: "Scouts, Hunters and Orthrus", Type: "notes",
			Items: []Item{
				axStep("Scouts — разведчики", "Scouts", "Небольшие диски без сердец. В одиночку проще всего взять несколько разведчиков усиленными AX-мультипушками с наведением. Berserker усиливает залпы и применяет каустик, Regenerator лечит соседей, Inciter повышает их подвижность, Marauder — обычный боец. Убирайте Berserker и Regenerator первыми, если они мешают добивать остальные цели.", "Small discs without hearts. Gimballed Enhanced AX Multi-Cannons make small groups practical in solo. A Berserker increases damage and uses caustic attacks, a Regenerator heals nearby Scouts, an Inciter improves mobility, and a Marauder is the baseline fighter. Prioritise Berserkers and Regenerators when they keep the rest alive or dangerous."),
				axStep("Glaive — быстрый охотник", "Glaive Hunter", "Скорость до 750 м/с и молния, которая восстанавливает его щит при контакте. Поле разрушает незащищённые модули Стражей. Против него полезнее человеческие AX-мультипушки и обычные блоки усиления модулей. Держите дистанцию от молнии и добивайте корпус после щита. Сердец нет.", "Up to 750 m/s, with lightning that restores its shield on contact. Its field degrades unprotected Guardian modules. Human AX Multi-Cannons and standard module reinforcements suit this fight. Stay out of lightning range and finish the hull after the shield. It has no hearts."),
				axStep("Scythe — охотник за капсулами", "Scythe Hunter", "Охотится за спасательными капсулами и пассажирами и может выпускать дроны-взломщики люков. У него нет щита и сердец. Держите его в секторе наведения AX-мультипушек и работайте по корпусу. Появление зависит от сценария, поэтому не стройте вокруг него стабильный маршрут фарма.", "A pod and passenger hunter that can deploy hatch-breaker limpets. It has no shield or hearts. Keep it in your gimballed AX Multi-Cannons' arc and work on the hull. Spawns depend on the scenario, so do not plan a reliable farm around it."),
				axStep("Orthrus — особый перехватчик", "Orthrus Interceptor", "Бронированный корпус с мощным щитом, без сердец. При атаке пытается уйти и портит незащищённые модули Стражей. Старый фарм на шпилях и вокруг действующих Титанов не следует переносить на сегодняшнюю карту. В бою с ним решает урон по щиту и корпусу до его прыжка.", "An armoured hull under a strong shield, with no hearts. Under attack it tries to escape and degrades unprotected Guardian modules. Historic spire and active Titan farms do not describe the current map. A kill depends on taking down its shield and hull before it jumps."),
			},
		},
	}
}

func axFightSections() []Section {
	return []Section{
		{
			ID: "scan-and-hearts", Title: "Сканирование и открытое сердце", EnglishTitle: "Scanning and the exerted heart", Type: "route",
			figureKey:            "ax-heart-diagram",
			FigureCaption:        "1 — сканирование. 2 — урон по корпусу. 3 — попадания по открытому сердцу. Подсвеченный лепесток — пример, его положение меняется при развороте перехватчика.",
			EnglishFigureCaption: "1 — scan. 2 — damage the hull. 3 — hit the exerted heart. The highlighted petal is an example and its position changes as the Interceptor turns.",
			Items: []Item{
				axStep("Опознайте цель", "Identify the target", "Выберите корабль и дождитесь пассивного опознания. Для выбора сердец удерживайте цель перед носом, назначьте ксеносканер на группу огня и держите кнопку до завершения активного скана. Дальность обычного сканера — 500 м, усиленного Enhanced Xeno Scanner — 2 км. Если одиночный перехватчик сам подлетел сканировать вас, это удобное окно. При наличии разведчиков он обычно атакует сразу.", "Target the ship and let passive identification complete. For heart subtargeting, keep the target ahead, assign the Xeno Scanner to a fire group and hold the trigger until the active scan completes. The standard scanner reaches 500 m and the Enhanced Xeno Scanner reaches 2 km. A lone Interceptor approaching to scan you offers a good opportunity. With Scouts present it usually attacks immediately."),
				axStep("Нанесите урон по корпусу", "Exert a heart through hull damage", "Стреляйте AX-оружием в корпус без активного щита. Корпус регенерирует, поэтому редкие одиночные попадания могут не накопить нужный урон. Когда сердце откроется, один внешний лепесток ярко загорится красным. У Hydra открытый участок может выглядеть дымящимся. У первого сердца ориентир — около 20% запаса корпуса, затем пороги снижаются. Сигнал к переносу огня — свечение лепестка.", "Fire AX weapons into the unshielded hull. The hull regenerates, so occasional hits may fail to build enough damage. An outer petal glows bright red when a heart is exerted. On a Hydra the exposed area can look smoky. Roughly 20% of hull is a first-heart benchmark and later thresholds are lower. The glowing petal is your cue to switch targets."),
				axStep("Выбейте сердце, пока оно открыто", "Destroy the heart while it is exposed", "После полного скана выберите Exerted Heart в списке подсистем. Так мультипушки с наведением берут на прицел открытое сердце. Фиксированным Гауссом можно попасть в красное пятно вручную без скана. Цельтесь в лепесток. Ночное видение помогает не потерять силуэт при развороте. Окно обычно около 45 секунд. Если свечение погасло, снова откройте то же сердце уроном по корпусу. Уже нанесённый сердцу урон сохранится.", "After a full scan, choose Exerted Heart in the subtarget list. This lets gimballed Multi-Cannons lock onto the exposed heart. Fixed Gauss can manually hit the red spot without a scan. Aim at the petal. Night Vision helps track the silhouette as it turns. The window is usually about 45 seconds. If the glow fades, exert the same heart again with hull damage. Damage already dealt to the heart is retained."),
				axStep("Разорвите дистанцию и повторите", "Make space and repeat", "После разрушения сердца перехватчик поднимает временный щит. Отойдите, проверьте рой, охлаждение и модули. После спада щита открывайте следующее сердце. После последнего сердца корпус больше не регенерирует. Попробуйте добить его сразу. Если щит успел подняться, дождитесь его спада и завершите бой. При взрыве уходите от облака.", "The Interceptor raises a temporary shield after a heart breaks. Make space and check the swarm, cooling and modules. Exert the next heart after the shield drops. After the final heart the hull stops regenerating. Try to finish it immediately. If its shield comes up, let it decay and finish the fight. Clear the explosion cloud when the hull goes."),
			},
		},
		{
			ID: "cold-orbit", Title: "Холодная орбита и огонь", EnglishTitle: "Cold orbiting and weapon timing", Type: "route",
			Items: []Item{
				axStep("Держите цель сбоку от вектора движения", "Move across the Interceptor's nose", "Для холодной орбиты нужны одновременно нагрев ниже 20% и заметная боковая или вертикальная скорость относительно перехватчика. Прямой заход на холодном корабле не защищает. Тренируйте Flight Assist Off вне боя и осваивайте орбиту вокруг Cyclops. Для Гаусса удобное рабочее расстояние около 0,9–1,5 км, когда нет опасности молнии.", "Cold orbiting needs both heat below 20% and clear lateral or vertical motion relative to the Interceptor. Flying straight at it while cold will not protect you. Practise Flight Assist Off outside combat and learn the orbit on a Cyclops. About 0.9–1.5 km is a practical Gauss distance when lightning is not a threat."),
				axStep("Охлаждайтесь до залпа", "Cool before the volley", "Сбросьте теплоотвод перед серией Гаусса и разбейте четыре пушки на две пары. Чередуйте пары, чтобы сгладить тепловой пик, держите заряд в SYS для теплоотводов, в WEP для стрельбы и в ENG для манёвра. Луч с Thermal Vent охлаждает только пока попадает. Используйте его между сериями, а при промахах отпустите спуск.", "Deploy a heatsink before firing Gauss and split four guns into two pairs. Alternate the pairs to smooth heat spikes. Keep charge in SYS for sinks, WEP for firing and ENG for manoeuvring. A Thermal Vent beam only cools when it hits. Use it between volleys and release the trigger if you miss."),
				axStep("Используйте время щита", "Use the shield interval", "В соло экономьте боезапас: щит сам ослабевает примерно за 1 мин 40 с у Cyclops, 3 мин у Basilisk, 4 мин у Medusa и 5 мин 20 с у Hydra. Зелёная вспышка подсказывает, что щит спал. Пока он активен, охлаждайтесь, разберите рой или ремонтируйтесь на безопасной дистанции. Если после уничтожения сердца прошло слишком много времени, перехватчик входит в ярость. Ориентиры для четырёх вариантов — 6, 7, 7 и 8 минут. Таймер обновляется при уничтожении сердца.", "Conserve ammo in solo. A full shield decays in about 1 min 40 s on Cyclops, 3 min on Basilisk, 4 min on Medusa and 5 min 20 s on Hydra. A green flash marks shield loss. Use that interval to cool, clear the swarm or repair at a safe distance. If the fight stalls after a heart, the Interceptor enrages. Approximate timers for the four variants are 6, 7, 7 and 8 minutes. Destroying a heart resets the timer."),
			},
		},
		{
			ID: "countermeasures", Title: "Рой, молния, каустик и отключение", EnglishTitle: "Swarm, lightning, caustic and shutdown", Type: "notes",
			Items: []Item{
				axStep("Рой таргонов", "Thargon swarm", "В космосе выберите рой отдельной целью. Для Remote Release Flak удерживайте спуск после выстрела и отпускайте, когда индикатор и звуковой сигнал отмечают сближение с роем. Для чистого выстрела сначала разведите рой и перехватчика по дистанции. Рой чаще идёт прямо с 4–6 км. Если дроны Medusa или Hydra выстроились кольцом, сместите прицел на край строя. Пролёт сквозь рой опасен: дроны могут перейти к ракетной атаке. В обычном бою прямо у поверхности перехватчики не разворачивают рой.", "In space, select the swarm as a separate target. For Remote Release Flak, hold the trigger after firing and release when the indicator and audio cue mark proximity to the swarm. Separate the swarm from the Interceptor to get a clear shot. A swarm often flies straight from 4–6 km. If Medusa or Hydra drones form a ring, offset your aim towards the rim. Flying through it can provoke a missile attack. Interceptors do not deploy their swarm during ordinary surface engagements."),
				axStep("Молния после первого сердца", "Lightning after the first heart", "Сразу откройте дистанцию. Перехватчик преследует вас около 30–40 секунд. Если дать ему приблизиться на 700–800 м, молния затормозит корабль и повредит модули. После погони он сохраняет способность бить молнией вблизи. Basilisk быстрее многих сборок, поэтому заранее приготовьте боковой выход и заряд ENG, а не надейтесь только на скорость буста.", "Open distance as soon as the first heart breaks. The Interceptor gives chase for about 30–40 seconds. Let it reach roughly 700–800 m and lightning can slow you and damage modules. It can still use lightning at close range after the chase. Basilisk outruns many fits, so plan a lateral exit and keep ENG charge instead of relying only on boost speed."),
				axStep("Каустик после второго сердца", "Caustic after the second heart", "Держите холод или отойдите за 3 км до попытки запуска ракет. Caustic Sink Launcher поглощает загрязнение, заполненный заряд выбрасывают вручную из группы огня. Если устройство не установлено, отойдите и используйте дрон деконтаминации или состыкуйтесь. Выжигание каустика высоким нагревом повреждает корабль и годится только как аварийный приём.", "Stay cold or move beyond 3 km before it attempts to fire. A Caustic Sink Launcher absorbs contamination, and you eject the loaded sink manually from its fire group. Without one, make space and use a decontamination limpet or dock. Burning off caustic at high heat damages your ship and is an emergency technique."),
				axStep("Импульс после предпоследнего сердца", "Shutdown after the penultimate heart", "Заранее держитесь холоднее 20% или за 3 км, чтобы снизить шанс запуска. Если слышите предупреждение об энергетическом всплеске, переведите энергию в SYS и удерживайте Shutdown Field Neutraliser до прохождения волны. Когда импульс уже запущен, холод его не отменит. Без нейтрализатора спасает дистанция за пределами поля либо заранее набранный дрейф, пока корабль отключён.", "Stay below 20% heat or beyond 3 km beforehand to avoid triggering the attack. On the energy surge warning, put power into SYS and hold the Shutdown Field Neutraliser until the wave passes. Cold will not cancel a pulse already launched. Without the neutraliser, get outside its range or set up a safe drift before your ship shuts down."),
			},
		},
	}
}
