package reference

import (
	"fmt"
	"strings"
)

var materialExchangeFallback = map[string]string{
	"militarysupercapacitors": "imperialshielding",
	"improvisedcomponents":    "imperialshielding",
	"pharmaceuticalisolators": "imperialshielding",
	"embeddedfirmware":        "adaptiveencryptors",
	"classifiedscandata":      "adaptiveencryptors",
	"dataminedwake":           "adaptiveencryptors",
	"compactemissionsdata":    "adaptiveencryptors",
	"shieldfrequencydata":     "adaptiveencryptors",
}

func (s *Store) shipMaterialSources(key string) []Item {
	if body, ok := rawMaterialBodies[key]; ok {
		return []Item{body.collectionStep()}
	}
	if step, ok := highGradeSignalMaterials[key]; ok {
		return []Item{step}
	}
	if guide, ok := materialGuideRoutes[key]; ok {
		return s.guideMaterialSteps(guide.guide, guide.steps...)
	}
	for _, source := range shipMaterialDropPools {
		if strings.Contains(" "+source.keys+" ", " "+key+" ") {
			items := []Item{source.step}
			if strings.Contains(davsHopeVerifiedDrops, " "+key+" ") {
				items = append(items, s.guideMaterialSteps("farm-manufactured-missions", "Dav’s Hope · A 5")...)
			}
			return items
		}
	}
	return nil
}

// These drops were individually recorded at Dav's Hope in field runs.
const davsHopeVerifiedDrops = " mechanicalscrap gridresistors heatconductionwiring crystalshards chemicalstorageunits basicconductors wornshieldemitters mechanicalequipment hybridcapacitors heatdispersionplate uncutfocuscrystals shieldemitters galvanisingalloys conductivecomponents chemicalprocessors mechanicalcomponents electrochemicalarrays shieldingsensors highdensitycomposites heatexchangers focuscrystals conductiveceramics chemicaldistillery phasealloys compoundshielding configurablecomponents conductivepolymers refinedfocuscrystals heatvanes "

// Surface composition measured by commanders and recorded in EDSM.
type materialBody struct{ system, body, abundance string }

func (b materialBody) collectionStep() Item {
	return Item{
		Title: b.body, EnglishTitle: b.body, System: b.system,
		Text:        fmt.Sprintf("Доля в составе поверхности: %s%%. Найдите камни волновым сканером ТРП, разбейте выступы или метеориты из турели и подберите обломки грузовым ковшом.", strings.ReplaceAll(b.abundance, ".", ",")),
		EnglishText: fmt.Sprintf("%s%% of the body's surface material composition. Use the SRV wave scanner to find rocks, break outcrops or meteorites with the turret, then scoop the fragments.", b.abundance),
	}
}

var rawMaterialBodies = map[string]materialBody{
	"iron":       {"Deciat", "Deciat 1", "39.94"},
	"nickel":     {"Deciat", "Deciat 1", "30.21"},
	"tin":        {"Deciat", "Deciat 1", "2.65"},
	"zinc":       {"Deciat", "Deciat 1", "10.85"},
	"carbon":     {"Deciat", "Deciat 7 C", "23.86"},
	"sulphur":    {"Deciat", "Deciat 7 C", "28.38"},
	"phosphorus": {"Deciat", "Deciat 7 C", "15.28"},
	"manganese":  {"Diaguandri", "Diaguandri B 1", "15.54"},
	"chromium":   {"Deciat", "Deciat 3", "9.86"},
	"vanadium":   {"Deciat", "Deciat 1", "9.81"},
	"germanium":  {"Frey", "Frey 4 B A", "6.09"},
	"cadmium":    {"Deciat", "Deciat 5", "1.77"},
	"tungsten":   {"Deciat", "Deciat 1", "2.19"},
	"arsenic":    {"Frey", "Frey 4 C A", "2.82"},
	"molybdenum": {"Diaguandri", "Diaguandri B 1", "2.46"},
	"niobium":    {"Diaguandri", "Diaguandri B 1", "2.57"},
	"zirconium":  {"Deciat", "Deciat 7 C", "1.40"},
	"mercury":    {"Diaguandri", "Diaguandri B 2", "1.00"},
}

// The signal owner's allegiance and state determine which pools can appear.
// Where pools overlap, the in-game contacts list confirms the actual drop.
func highGradeSignal(title, titleEN, details, detailsEN string) Item {
	return Item{
		Title: title, EnglishTitle: titleEN,
		Text:        details + " Просканируйте навигационный маяк, вернитесь в суперкруиз и выберите сигнал High Grade Emissions (HGE). На месте проверьте состав во вкладке «Контакты» и соберите нужное дронами.",
		EnglishText: detailsEN + " Scan the nav beacon, return to supercruise and select a High Grade Emissions (HGE) signal. Check Contacts on arrival and collect the right salvage with limpets.",
	}
}

var highGradeSignalMaterials = map[string]Item{
	"imperialshielding": highGradeSignal(
		"Империя · без активного состояния или выборы", "Empire · no active state or Election",
		"Ищите имперский сигнал в населённой системе Империи. У фракции сигнала желательно отсутствие активного состояния или выборы.",
		"Look for a signal owned by an Imperial faction in a populated Imperial system. No active state or Election is a good starting point."),
	"fedcorecomposites": highGradeSignal(
		"Федерация · без активного состояния или выборы", "Federation · no active state or Election",
		"Ищите федеральный сигнал в населённой системе Федерации. В том же сигнале встречаются фирменные композиты G4.",
		"Look for a signal owned by a Federal faction in a populated Federal system. The same signal can contain Proprietary Composites G4."),
	"fedproprietarycomposites": highGradeSignal(
		"Федерация · вместе с композитами G5", "Federation · alongside Core Dynamics Composites",
		"Ищите федеральный сигнал от фракции без активного состояния или на выборах. Фирменные композиты G4 встречаются рядом с композитами Core Dynamics G5.",
		"Look for a Federal signal with no active state or Election. Proprietary Composites G4 can appear alongside Core Dynamics Composites G5."),
	"protoheatradiators": highGradeSignal(
		"Подъём · независимые системы или Альянс", "Boom · Independent or Alliance systems",
		"Ищите фракцию сигнала в состоянии «Подъём» (Boom). Другая точка с тем же состоянием может содержать проторадиационные сплавы.",
		"Look for a signal faction in Boom. Another signal in the same state may contain Proto Radiolic Alloys."),
	"protoradiolicalloys": highGradeSignal(
		"Подъём · независимые системы или Альянс", "Boom · Independent or Alliance systems",
		"Ищите фракцию сигнала в состоянии «Подъём» (Boom). Протолёгкие сплавы G4 могут лежать рядом, а другая точка может содержать проторадиаторы.",
		"Look for a signal faction in Boom. Proto Light Alloys G4 can appear alongside the G5 salvage. A different signal may contain Proto Heat Radiators."),
	"protolightalloys": highGradeSignal(
		"Подъём · вместе со сплавом G5", "Boom · alongside Proto Radiolic Alloys",
		"В независимой системе или системе Альянса ищите сигнал фракции в состоянии «Подъём» (Boom), который содержит проторадиационные сплавы G5.",
		"In an Independent or Alliance system, look for a Boom signal that contains Proto Radiolic Alloys G5."),
	"militarygradealloys": highGradeSignal(
		"Война или гражданская война · независимые системы или Альянс", "War or Civil War · Independent or Alliance systems",
		"Проверьте, что сама фракция сигнала воюет. В этом наборе добычи встречаются и военные суперконденсаторы.",
		"Check that the faction owning the signal is at War or Civil War. Military Supercapacitors share this drop pool."),
	"militarysupercapacitors": highGradeSignal(
		"Война или гражданская война · независимые системы или Альянс", "War or Civil War · Independent or Alliance systems",
		"Проверьте, что сама фракция сигнала воюет. Если сигналы в районе дают только военные сплавы, используйте обмен из другого семейства.",
		"Check that the faction owning the signal is at War or Civil War. If the local signals only yield Military Grade Alloys, use the cross-family trade above."),
	"improvisedcomponents": highGradeSignal(
		"Гражданские волнения · независимые системы или Альянс", "Civil Unrest · Independent or Alliance systems",
		"Ищите фракцию сигнала в состоянии «Гражданские волнения» (Civil Unrest). Если таких сигналов поблизости нет, соберите имперские экраны для обмена.",
		"Look for a signal faction in Civil Unrest. If there are no suitable signals nearby, collect Imperial Shielding to trade."),
	"pharmaceuticalisolators": highGradeSignal(
		"Эпидемия · независимые системы или Альянс", "Outbreak · Independent or Alliance systems",
		"Ищите фракцию сигнала в состоянии «Эпидемия» (Outbreak). Если подходящих сигналов нет, соберите имперские экраны для обмена.",
		"Look for a signal faction in Outbreak. If no suitable signals are available, collect Imperial Shielding to trade."),
}

type materialGuideRoute struct {
	guide string
	steps []string
}

func farmSteps(guide string, steps ...string) materialGuideRoute {
	return materialGuideRoute{guide, steps}
}

var (
	obeliskMaterialRoute   = farmSteps("farm-guardian-obelisks", "Dedicated stop — 1 B", "Target an illuminated obelisk", "Check all five counters")
	sentinelMaterialRoute  = farmSteps("farm-guardian-components", "Combine with a blueprint — B 3", "Sentinel Weapon Parts and Wreckage Components")
	thargoidSurfaceSalvage = farmSteps("farm-thargoid-encounters", "Surface structure on A 2", "Scavenger salvage")
	thargoidSurfaceScan    = farmSteps("farm-thargoid-encounters", "Surface structure on A 2")
	thargoidShipScan       = farmSteps("farm-thargoid-encounters", "Find a non-human signal", "Signature, flight and systems data")
	titanMaterialRoute     = farmSteps("farm-thargoid-titans", "Cocijo near Earth", "Collect materials through Contacts", "Alternative for the four parts")
)

var materialGuideRoutes = map[string]materialGuideRoute{
	"antimony":   farmSteps("farm-raw-nearby", "Antimony and Selenium · A 4 B", "Target the right growth"),
	"tellurium":  farmSteps("farm-raw-nearby", "Tellurium · A 6 A", "Target the right growth"),
	"yttrium":    farmSteps("farm-raw-nearby", "Yttrium and Selenium · A 7 B", "Target the right growth"),
	"ruthenium":  farmSteps("farm-raw-nearby", "Ruthenium · A 7 F A", "Target the right growth"),
	"technetium": farmSteps("farm-raw-nearby", "Technetium and Selenium · A 8 A", "Target the right growth"),
	"polonium":   farmSteps("farm-raw-nearby", "Polonium · A 9 A", "Target the right growth"),
	"selenium":   farmSteps("farm-raw-nearby", "HR 3230 · 3 A A", "Target the right growth"),

	"biotechconductors":      farmSteps("farm-manufactured-missions", "Biotech Conductors G5", "Robigo Mines → Sothis"),
	"exquisitefocuscrystals": farmSteps("farm-manufactured-missions", "Exquisite Focus Crystals G5", "Robigo Mines → Sothis"),
	"adaptiveencryptors":     farmSteps("farm-encoded-jameson", "HIP 12099 · 1 B", "Work around the wreck"),
	"encryptionarchives":     farmSteps("farm-encoded-jameson", "HIP 12099 · 1 B", "Work around the wreck"),
	"dataminedwake":          farmSteps("farm-encoded-direct", "Fit a wake scanner", "Datamined Wake Exceptions G5"),

	"guardianpowercell":                  farmSteps("farm-guardian-components", "Combine with a blueprint — B 3", "Guardian Power Cells"),
	"guardianpowerconduit":               farmSteps("farm-guardian-components", "Combine with a blueprint — B 3", "Guardian Power Conduits"),
	"guardiantechcomponent":              farmSteps("farm-guardian-components", "Stock up on Technology Components — C 3", "Guardian Technology Components"),
	"guardiansentinelweaponparts":        sentinelMaterialRoute,
	"guardiansentinelwreckagecomponents": sentinelMaterialRoute,
	"ancientbiologicaldata":              obeliskMaterialRoute,
	"ancientculturaldata":                obeliskMaterialRoute,
	"ancienthistoricaldata":              obeliskMaterialRoute,
	"ancientlanguagedata":                obeliskMaterialRoute,
	"ancienttechnologicaldata":           obeliskMaterialRoute,
	"guardianmoduleblueprint":            farmSteps("farm-guardian", "Main site — B 3", "Relic and reconnaissance", "Charge six pylons", "Deposit and scan"),
	"guardianweaponblueprint":            farmSteps("farm-guardian-blueprints", "Main site — B 1"),
	"guardianvesselblueprint":            farmSteps("farm-guardian-blueprints", "Guardian Beacon near body 4", "Three nodes and one key", "Site A 1", "Six pylons, key, scan"),

	"unknownenergysource":         farmSteps("farm-thargoid-encounters", "Crash site on Halla", "Unlock quantities"),
	"unknowncarapace":             thargoidSurfaceSalvage,
	"unknownenergycell":           thargoidSurfaceSalvage,
	"unknownorganiccircuitry":     thargoidSurfaceSalvage,
	"unknowntechnologycomponents": thargoidSurfaceSalvage,
	"tgcompositiondata":           thargoidSurfaceScan,
	"tgresiduedata":               thargoidSurfaceScan,
	"tgstructuraldata":            thargoidSurfaceScan,
	"tgshipflightdata":            thargoidShipScan,
	"tgshipsystemsdata":           thargoidShipScan,
	"unknownshipsignature":        thargoidShipScan,
	"unknownwakedata":             farmSteps("farm-thargoid-encounters", "Find a non-human signal", "Thargoid Wake Data"),
	"tgbiomechanicalconduits":     titanMaterialRoute,
	"tgpropulsionelement":         titanMaterialRoute,
	"tgweaponparts":               titanMaterialRoute,
	"tgwreckagecomponents":        titanMaterialRoute,
	"tg_causticcrystal":           farmSteps("farm-thargoid-titans", "Cocijo near Earth", "Collect materials through Contacts", "Cost per module"),
	"tg_abrasion03":               farmSteps("farm-thargoid-titans", "How to read older routes", "Cost per module"),
	"mercenarycoin":               farmSteps("farm-merc-coins", "Join an Operation", "Compare scenarios", "Account for the weekly cycle"),
}

// Materials in one observed loot pool share the same collection action.
// More specific sources appear before broader combat or salvage pools.
var shipMaterialDropPools = []struct {
	keys string
	step Item
}{
	{"rhenium lead", Item{
		Title: "Побочная добыча в ледяных кольцах", EnglishTitle: "Ice-ring mining by-product",
		Text:        "В ледяном кольце выберите астероиды с высоким содержанием сырья по данным дрона-разведчика. Используйте добывающий лазер и собирайте выпавшие обломки ковшом или дронами. Для быстрого пополнения запаса обменяйте старший элемент семейства.",
		EnglishText: "In an icy ring, use a prospector to find asteroids with high material content. Laser-mine the rock and collect the fragments with limpets or the scoop. For a quick refill, trade down from the top grade in this family.",
	}},
	{"boron", Item{
		Title: "Побочная добыча лазером в кольцах", EnglishTitle: "Laser mining by-product",
		Text:        "Бор может выпасть при лазерной добыче трития в ледяном кольце. Выбирайте астероиды с высоким содержанием сырья по данным дрона-разведчика и проверяйте обломки в «Контактах». Если нужен только бор, обменяйте сурьму G4.",
		EnglishText: "Boron can appear while laser-mining Tritium in icy rings. Prospect for high material content and check the fragments in Contacts. If you only need Boron, trade down from Antimony G4.",
	}},
	{"salvagedalloys uncutfocuscrystals mechanicalcomponents", Item{
		Title: "Обломки после боя · Combat Aftermath", EnglishTitle: "Combat Aftermath wreckage",
		Text:        "В населённой системе просканируйте навигационный маяк и проверьте в суперкруизе сигналы Combat Aftermath. Войдите в сигнал и подберите нужный обломок из списка «Контакты».",
		EnglishText: "Scan the nav beacon in a populated system and check supercruise for Combat Aftermath signals. Drop in and recover the matching fragment from Contacts.",
	}},
	{"compoundshielding configurablecomponents", Item{
		Title: "Закодированные выбросы · Encoded Emissions", EnglishTitle: "Encoded Emissions wreckage",
		Text:        "В населённой системе просканируйте навигационный маяк, вернитесь в суперкруиз и войдите в сигнал Encoded Emissions. Ищите физические обломки в «Контактах» и подбирайте их ковшом или дронами.",
		EnglishText: "Scan the nav beacon in a populated system, return to supercruise and drop into Encoded Emissions. Find physical salvage in Contacts and collect it with the scoop or limpets.",
	}},
	{"conductivecomponents conductiveceramics electrochemicalarrays heatexchangers", Item{
		Title: "Анархия · Degraded Emissions", EnglishTitle: "Anarchy · Degraded Emissions",
		Text:        "Выберите населённую систему с правительством «Анархия». Просканируйте навигационный маяк и проверьте сигналы Degraded Emissions в суперкруизе. Нужный компонент лежит среди обломков и подбирается ковшом или дронами.",
		EnglishText: "Choose a populated system with an Anarchy government. Scan the nav beacon and look for Degraded Emissions in supercruise. Collect the component from the physical debris with a scoop or limpets.",
	}},
	{"refinedfocuscrystals", Item{
		Title: "Сигнал «Обнаружен огонь» · Weapons Fire Detected", EnglishTitle: "Weapons Fire Detected signal",
		Text:        "Начните с населённой системы с высоким уровнем безопасности. После сканирования навигационного маяка проверьте сигналы Weapons Fire Detected и ищите нужные обломки в «Контактах». Для пополнения запаса проще взять награду с кристаллами G5 и обменять её.",
		EnglishText: "Start in a populated high-security system. Scan the nav beacon, check Weapons Fire Detected signals and look for the salvage in Contacts. To stock up, take a named G5 crystal mission reward and trade down.",
	}},
	{"chemicalmanipulators polymercapacitors", Item{
		Title: "Обломки конвоя или Dav’s Hope", EnglishTitle: "Convoy salvage or Dav’s Hope", System: "Hyades Sector DR-V c2-23",
		Text:        "Проверьте обломки конвоя в сигнале Convoy Dispersal Pattern. Без боевого корабля отправляйтесь в Dav’s Hope на A 5 и подбирайте случайные компоненты вдоль внешней дороги на ТРП.",
		EnglishText: "Check convoy wreckage at a Convoy Dispersal Pattern signal. Without a combat ship, visit Dav’s Hope on A 5 and collect mixed salvage around the outer loop in an SRV.",
	}},
	{"compactcomposites gridresistors hybridcapacitors filamentcomposites heatresistantceramics highdensitycomposites precipitatedalloys", Item{
		Title: "Трофеи военных кораблей", EnglishTitle: "Military ship salvage",
		Text:        "В системе со статусом «Война» или «Гражданская война» найдите Conflict Zone, выберите сторону и подбирайте обломки уничтоженных кораблей. Используйте дроны-сборщики после боя или когда рядом нет противников.",
		EnglishText: "Enter a Conflict Zone in a system at War or Civil War, choose a side and collect destroyed ships’ salvage. Deploy collector limpets after the battle or when you are clear of enemies.",
	}},
	{"wornshieldemitters crystalshards temperedalloys galvanisingalloys shieldemitters shieldingsensors focuscrystals phasealloys thermicalloys", Item{
		Title: "Обломки пиратов в зоне добычи", EnglishTitle: "Pirate wreckage at a Resource Extraction Site",
		Text:        "Войдите в зону добычи ресурсов (RES) в кольце, дождитесь отметки «Разыскивается» на пирате и уничтожьте его. Отбирайте нужные обломки в «Контактах» и подбирайте ковшом или дронами.",
		EnglishText: "Enter a ring’s Resource Extraction Site (RES), wait for a pirate to show Wanted and destroy it. Pick out the matching salvage in Contacts and collect it with a scoop or limpets.",
	}},
	{"basicconductors chemicalstorageunits heatconductionwiring mechanicalscrap chemicalprocessors mechanicalequipment heatdispersionplate chemicaldistillery conductivepolymers heatvanes", Item{
		Title: "Обломки грузовых кораблей", EnglishTitle: "Hauler and freighter wreckage",
		Text:        "Уничтоженные грузовые NPC, например Hauler, Adder и Type-6/7/9, оставляют этот вид компонентов. Подбирайте обломки разыскиваемых грузовиков или работайте в зонах без юрисдикции. Для большого запаса выгоднее обменять материал G5 по ссылке выше.",
		EnglishText: "Destroyed cargo NPCs such as Hauler, Adder and Type-6/7/9 can drop these components. Recover them from wanted freighters or where there is no jurisdiction. To stock up, trade down from the linked G5 material.",
	}},
	{"disruptedwakeechoes fsdtelemetry wakesolutions hyperspacetrajectories", Item{
		Title: "Высокоэнергетические следы у загруженного порта", EnglishTitle: "High-energy wakes outside a busy port",
		Text:        "Установите сканер следов FSD и назначьте его в группу огня. Держитесь за пределами зоны запрета стрельбы станции и на дальности сканера от высокоэнергетических следов уходящих кораблей. Сканируйте следы один за другим, материал выпадает случайно.",
		EnglishText: "Fit a Frame Shift Wake Scanner and assign it to a fire group. Stay clear of the station’s no-fire zone and within scanner range of departing ships’ High Energy Wakes. Scan successive wakes for a chance of receiving this data.",
	}},
	{"bulkscandata scanarchives scandatabanks shieldcyclerecordings shieldsoakanalysis", Item{
		Title: "Обычный скан грузовых NPC у станции", EnglishTitle: "Standard scan of cargo NPCs near a station",
		Text:        "На подлёте к занятой станции по очереди берите грузовые NPC в цель и держите их в секторе корабельных сенсоров до окончания опознания. Сканер следов здесь не нужен. По одному и тому же кораблю повторно данные не начисляются.",
		EnglishText: "Approach a busy station and target freighters one by one. Keep each ship in the sensor arc until the standard identification scan finishes. A wake scanner is unnecessary. Scan a new ship for each chance at a drop.",
	}},
	{"compactemissionsdata", Item{
		Title: "Закрытый маяк в Encoded Emissions", EnglishTitle: "Private beacon at Encoded Emissions",
		Text:        "В сигнале Encoded Emissions выберите Private Data Beacon в «Контактах» и просканируйте его сканером канала данных. Ещё один способ — обычный скан боевых NPC в зоне добычи ресурсов.",
		EnglishText: "In Encoded Emissions, target a Private Data Beacon from Contacts and use the Data Link Scanner. Standard scans of combat NPCs in a Resource Extraction Site can also drop this data.",
	}},
	{"emissiondata shielddensityreports shieldpatternanalysis decodedemissiondata shieldfrequencydata", Item{
		Title: "Обычный скан боевых NPC в RES", EnglishTitle: "Standard scan of combat NPCs in a RES",
		Text:        "В зоне добычи ресурсов (RES) по очереди наводитесь на пиратские боевые корабли и ждите окончания обычного опознания. Для данных достаточно сканирования. Если конкретный редкий материал не выпадает, используйте обмен.",
		EnglishText: "In a Resource Extraction Site, target pirate combat ships one by one and wait for standard identification to finish. The scan itself yields the data. Trade for the exact item if rare direct drops are slow.",
	}},
	{"classifiedscandata", Item{
		Title: "Скан военных кораблей и службы безопасности", EnglishTitle: "Scan military or security ships",
		Text:        "Берите в цель корабли службы безопасности у навигационного маяка или военные корабли в Conflict Zone. Дождитесь окончания обычного скана. На доске заданий отдельно проверяйте награды, где этот материал назван явно.",
		EnglishText: "Target security ships at a nav beacon or military ships in a Conflict Zone and let each standard scan finish. Check mission boards for rewards that explicitly name this item.",
	}},
	{"embeddedfirmware", Item{
		Title: "Награда за задание или сеть точек данных", EnglishTitle: "Mission reward or settlement data network",
		Text:        "На доске заданий выбирайте только предложения, где этот материал указан в награде. Прямой сбор на поверхности: на ТРП последовательно отсканируйте сеть точек данных в поселении средней или высокой охраны до истечения таймера.",
		EnglishText: "Choose missions that explicitly list this firmware as a reward. For surface collection, drive an SRV through the data-point network at a medium- or high-security settlement and scan the full sequence before the timer expires.",
	}},
	{"industrialfirmware encodedscandata securityfirmware", Item{
		Title: "Сеть точек данных на поверхности", EnglishTitle: "Surface data-point network",
		Text:        "Возьмите ТРП и ищите сеть точек данных в наземном поселении средней или высокой охраны. Назначьте сканер канала данных, проверьте доступ и завершите сканирование всей цепочки в отведённое время.",
		EnglishText: "Bring an SRV to a medium- or high-security surface settlement with a data-point network. Assign the Data Link Scanner, check access and scan the full chain within its time limit.",
	}},
	{"scrambledemissiondata legacyfirmware encryptedfiles archivedemissiondata consumerfirmware encryptioncodes symmetrickeys", Item{
		Title: "Маяк в сигнале обломков", EnglishTitle: "Beacon in a wreckage signal",
		Text:        "В населённой системе проверьте сигналы Encoded Emissions или Degraded Emissions после сканирования навигационного маяка. Если среди обломков есть Private Data Beacon, выберите его в «Контактах» и используйте сканер канала данных.",
		EnglishText: "After scanning a nav beacon in a populated system, check Encoded Emissions or Degraded Emissions. If a Private Data Beacon is present, target it through Contacts and scan it with the Data Link Scanner.",
	}},
}
