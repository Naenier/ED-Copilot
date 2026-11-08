package ranks

import "strings"

type text struct{ RU, EN string }

func (x text) Get(language string) string {
	if normalizedLanguage(language) == "en" {
		return x.EN
	}
	return x.RU
}
func normalizedLanguage(language string) string {
	if strings.EqualFold(strings.TrimSpace(language), "en") {
		return "en"
	}
	return "ru"
}

type discipline struct {
	ID, Journal, Icon string
	Title             text
	Names             []text
	Max               int
}

func names(ru, en string, elite bool) []text {
	a, b, out := strings.Split(ru, "|"), strings.Split(en, "|"), []text{}
	for i := range a {
		out = append(out, text{a[i], b[i]})
	}
	if elite {
		for _, suffix := range []string{"", " I", " II", " III", " IV", " V"} {
			out = append(out, text{"Элита" + suffix, "Elite" + suffix})
		}
	}
	return out
}

// Names use the Journal appendix and current game nomenclature (Trailblazer).
// Career CQC Elite I–V is distinct from Arena level 1–50 and its reset Prestige.
var disciplines = []discipline{
	{"combat", "Combat", "target", text{"Боевой ранг", "Combat"}, names("Безвредный|В целом безвредный|Новичок|Специалист|Эксперт|Мастер|Опасный|Смертоносный", "Harmless|Mostly Harmless|Novice|Competent|Expert|Master|Dangerous|Deadly", true), 13},
	{"trade", "Trade", "chart-no-axes-combined", text{"Торговый ранг", "Trade"}, names("Нищий|Полунищий|Мелкий торговец|Агент|Коммерсант|Брокер|Предприниматель|Магнат", "Penniless|Mostly Penniless|Peddler|Dealer|Merchant|Broker|Entrepreneur|Tycoon", true), 13},
	{"explore", "Explore", "compass", text{"Исследовательский ранг", "Exploration"}, names("Бесцельный|Почти бесцельный|Разведчик|Изыскатель|Путешественник|Следопыт|Скиталец|Первооткрыватель", "Aimless|Mostly Aimless|Scout|Surveyor|Trailblazer|Pathfinder|Ranger|Pioneer", true), 13},
	{"soldier", "Soldier", "shield-check", text{"Наёмник", "Mercenary"}, names("Беззащитный|В целом беззащитный|Новичок|Солдат|Стрелок|Воин|Гладиатор|Меткий глаз", "Defenceless|Mostly Defenceless|Rookie|Soldier|Gunslinger|Warrior|Gladiator|Deadeye", true), 13},
	{"exobiologist", "Exobiologist", "leaf", text{"Экзобиолог", "Exobiologist"}, names("Беспомощный|Практически беспомощный|Компилятор|Коллекционер|Каталогизатор|Таксономист|Эколог|Генетик", "Directionless|Mostly Directionless|Compiler|Collector|Cataloguer|Taxonomist|Ecologist|Geneticist", true), 13},
	{"cqc", "CQC", "target", text{"CQC", "CQC"}, names("Беспомощный|Почти беспомощный|Любитель|Полупрофессионал|Профессионал|Чемпион|Герой|Легенда", "Helpless|Mostly Helpless|Amateur|Semi Professional|Professional|Champion|Hero|Legend", true), 13},
	{"federation", "Federation", "shield-check", text{"Флот Федерации", "Federal Navy"}, names("Нет звания|Новобранец|Кадет|Гардемарин|Старшина|Главный старшина|Уорент-офицер|Энсин|Лейтенант|Капитан-лейтенант|Начальник гарнизона|Командир корабля|Контр-адмирал|Вице-адмирал|Адмирал", "None|Recruit|Cadet|Midshipman|Petty Officer|Chief Petty Officer|Warrant Officer|Ensign|Lieutenant|Lieutenant Commander|Post Commander|Post Captain|Rear Admiral|Vice Admiral|Admiral", false), 14},
	{"empire", "Empire", "shield-check", text{"Флот Империи", "Imperial Navy"}, names("Нет звания|Чужак|Крепостной|Мастер|Оруженосец|Рыцарь|Лорд|Барон|Виконт|Граф|Эрл|Маркиз|Герцог|Принц|Король", "None|Outsider|Serf|Master|Squire|Knight|Lord|Baron|Viscount|Count|Earl|Marquis|Duke|Prince|King", false), 14},
}

func byID(id string) *discipline {
	for i := range disciplines {
		if disciplines[i].ID == id {
			return &disciplines[i]
		}
	}
	return nil
}

type reward struct {
	ID, Kind, Entity, System string
	Title, Note              text
}

var navyRewards = map[string]map[int][]reward{
	"federation": {
		3:  {{"federal-dropship", "ship", "ship/federationdropship", "", text{"Покупка Federal Dropship", "Federal Dropship purchase access"}, text{}}},
		4:  {{"sol", "permit", "system/Sol", "Sol", text{"Разрешение Sol", "Sol permit"}, text{"Солнечная система с Землёй; столица Федерации находится на Марсе. Здесь работает инженер Bris Dekker, база Dekker’s Yard. У Daedalus действуют местные скидки: Eagle Mk II — 20%, Federal Dropship и Vulture — 10%.", "Our Solar System contains Earth and the Federation’s capital on Mars. Engineer Bris Dekker works here at Dekker’s Yard. Daedalus offers local discounts: Eagle Mk II 20%, Federal Dropship and Vulture 10%."}}},
		5:  {{"federal-assault-ship", "ship", "ship/federationdropshipmkii", "", text{"Покупка Federal Assault Ship", "Federal Assault Ship purchase access"}, text{}}, {"vega", "permit", "system/Vega", "Vega", text{"Разрешение Vega", "Vega permit"}, text{"Taylor City — место покупки редкого товара Vega Slimweed.", "Taylor City is the source of the rare commodity Vega Slimweed."}}, {"beta-hydri", "permit", "system/Beta%20Hydri", "Beta Hydri", text{"Разрешение Beta Hydri", "Beta Hydri permit"}, text{}}},
		6:  {{"plx695", "permit", "system/PLX%20695", "PLX 695", text{"Разрешение PLX 695", "PLX 695 permit"}, text{}}},
		7:  {{"federal-gunship", "ship", "ship/federationgunship", "", text{"Покупка Federal Gunship", "Federal Gunship purchase access"}, text{}}, {"ross128", "permit", "system/Ross%20128", "Ross 128", text{"Разрешение Ross 128", "Ross 128 permit"}, text{"Тюремная система со станцией Warren Prison Mine.", "A prison system containing Warren Prison Mine."}}},
		8:  {{"exbeur", "permit", "system/Exbeur", "Exbeur", text{"Разрешение Exbeur", "Exbeur permit"}, text{}}},
		10: {{"hors", "permit", "system/Hors", "Hors", text{"Разрешение Hors", "Hors permit"}, text{}}},
		12: {{"federal-corvette", "ship", "ship/federationcorvette", "", text{"Покупка Federal Corvette", "Federal Corvette purchase access"}, text{}}},
	},
	"empire": {
		3:  {{"imperial-courier", "ship", "ship/empirecourier", "", text{"Покупка Imperial Courier", "Imperial Courier purchase access"}, text{}}},
		4:  {{"achenar", "permit", "system/Achenar", "Achenar", text{"Разрешение Achenar", "Achenar permit"}, text{"Столичная система Империи: императорский дворец и Сенат находятся на Capitol. Здесь расположена Fortune’s Loss — мастерская Tiana Fortune, специалиста по сенсорам, сканерам и контроллерам дронов.", "The Empire’s capital system: Capitol houses the Imperial Palace and Senate. Fortune’s Loss is the workshop of Tiana Fortune, who specialises in sensors, scanners and limpet controllers."}}},
		7:  {{"imperial-clipper", "ship", "ship/empiretrader", "", text{"Покупка Imperial Clipper", "Imperial Clipper purchase access"}, text{}}, {"summerland", "permit", "system/Summerland", "Summerland", text{"Разрешение Summerland", "Summerland permit"}, text{"В Henry O’Hare’s Hangar действует местная скидка 10% на Imperial Eagle, Imperial Courier, Imperial Clipper и Imperial Cutter.", "Henry O’Hare’s Hangar offers a local 10% discount on Imperial Eagle, Imperial Courier, Imperial Clipper and Imperial Cutter."}}},
		10: {{"facece", "permit", "system/Facece", "Facece", text{"Разрешение Facece", "Facece permit"}, text{"Военный оплот Империи и штаб Имперского флота.", "An Imperial military bastion and the headquarters of the Imperial Navy."}}},
		12: {{"imperial-cutter", "ship", "ship/cutter", "", text{"Покупка Imperial Cutter", "Imperial Cutter purchase access"}, text{}}},
	},
}

func rewardsFor(id string, index int, language string) []M {
	items := append([]reward{}, navyRewards[id][index]...)
	if index == 8 && id != "federation" && id != "empire" {
		if id != "cqc" {
			items = append(items, reward{"founders-world", "permit", "system/Shinrarta%20Dezhra", "Shinrarta Dezhra", text{"Разрешение Shinrarta Dezhra", "Shinrarta Dezhra permit"}, text{"Jameson Memorial: местная скидка 10% на корабли и модули.", "Jameson Memorial offers a local 10% ship and module discount."}})
		}
		if id == "combat" || id == "trade" || id == "explore" || id == "cqc" {
			items = append(items, reward{"elite-discount", "discount", "", "", text{"Скидка Elite 2,5%", "Elite discount: 2.5%"}, text{"Скидка на корабли и модули сочетается с местными скидками; проценты скидок не складываются.", "The ship and module discount stacks with local discounts; the percentages are not added together."}})
		}
	}
	if (id == "soldier" || id == "exobiologist") && index <= 8 {
		parts := []text{{"Базовый костюм", "Body suit"}, {"Торс, руки и ноги", "Torso, arms and legs"}, {"Шлем", "Helmet"}}
		colors := []text{{"Бронза", "Bronze"}, {"Серебро", "Silver"}, {"Золото", "Gold"}}
		suit := "Dominator"
		if id == "exobiologist" {
			suit, colors = "Artemis", []text{{"Ярко-зелёный", "Vibrant Green"}, {"Ярко-синий", "Vibrant Blue"}, {"Ярко-красный", "Vibrant Red"}}
		}
		items = append(items, reward{"suit-livery", "cosmetic", "", "", text{suit + ": " + parts[index%3].RU + " · " + colors[index/3].RU, suit + ": " + parts[index%3].EN + " · " + colors[index/3].EN}, text{}})
	}
	rows := []M{}
	for _, item := range items {
		rows = append(rows, M{"id": item.ID, "kind": item.Kind, "title": item.Title.Get(language), "note": item.Note.Get(language), "entryId": item.Entity, "system": item.System})
	}
	return rows
}
