package notifications

type Category struct {
	ID                 string `json:"id"`
	Group              string `json:"group"`
	EnglishGroup       string `json:"englishGroup"`
	Title              string `json:"title"`
	EnglishTitle       string `json:"englishTitle"`
	Description        string `json:"description"`
	EnglishDescription string `json:"englishDescription"`
	Priority           int    `json:"-"`
}

var catalog = []Category{
	{"carrierScheduled", "Флотоносец", "Fleet carrier", "Прыжок назначен", "Jump scheduled", "Игра приняла запрос на прыжок вашего флотоносца.", "The game accepted a jump request for your fleet carrier.", 1},
	{"carrier", "Флотоносец", "Fleet carrier", "Скоро прыжок", "Departure soon", "До назначенного вылета вашего флотоносца осталось не больше пяти минут.", "Your fleet carrier is due to depart within five minutes.", 2},
	{"carrierCancelled", "Флотоносец", "Fleet carrier", "Прыжок отменён", "Jump cancelled", "Игра подтвердила отмену прыжка вашего флотоносца.", "The game confirmed cancellation of your fleet carrier's jump.", 2},
	{"carrierArrived", "Флотоносец", "Fleet carrier", "Прыжок выполнен", "Jump complete", "Журнал подтвердил прибытие вашего флотоносца.", "The journal confirmed your fleet carrier's arrival.", 2},
	{"carrierCooldown", "Флотоносец", "Fleet carrier", "Охлаждение завершено", "Cooldown elapsed", "Расчётный интервал охлаждения истёк. Время отсчитывается от прибытия или отметки о начале прыжка.", "The estimated cooldown ended. The timer uses the arrival or confirmed jump start.", 2},
	{"fuel", "Полёт и маршруты", "Flight and routes", "Мало топлива", "Low fuel", "В основном баке корабля осталось 10% топлива или меньше.", "Your ship's main tank has 10% fuel or less.", 3},
	{"shipRouteStop", "Полёт и маршруты", "Flight and routes", "Точка маршрута пройдена", "Route waypoint reached", "Вы прибыли в промежуточную точку активного маршрута корабля.", "You reached an intermediate waypoint on the active ship route.", 1},
	{"shipRouteComplete", "Полёт и маршруты", "Flight and routes", "Маршрут корабля завершён", "Ship route complete", "Вы прибыли в последнюю точку активного маршрута корабля.", "You reached the final waypoint on the active ship route.", 2},
	{"cartographyStop", "Исследования", "Exploration", "Система маршрута картографии достигнута", "Mapping waypoint reached", "Игра подтвердила прибытие в очередную систему маршрута картографии.", "The game confirmed arrival at the next mapping system.", 1},
	{"cartographyComplete", "Исследования", "Exploration", "Маршрут картографии завершён", "Mapping route complete", "Все точки маршрута посещены, и все его планеты отмечены как обследованные.", "All waypoints have been reached and all target bodies are marked surveyed.", 2},
	{"finding", "Исследования", "Exploration", "Находки", "Discoveries", "Обнаружены интересные цели для исследования или картографирования.", "An exploration or mapping target was discovered.", 1},
	{"bioComplete", "Исследования", "Exploration", "Биологический образец исследован", "Organic sample complete", "Успешно завершён анализ серии биологических проб.", "Analysis of an organic sample series succeeded.", 1},
	{"bioSold", "Исследования", "Exploration", "Биологические данные проданы", "Organic data sold", "Журнал подтвердил продажу биологических данных.", "The journal confirmed sale of organic data.", 1},
	{"tradeRound", "Торговля и колонизация", "Trade and colonization", "Торговый круг завершён", "Trade round complete", "Подтверждено возвращение на исходную станцию после измеренного рейса.", "A measured trade round finished with a return to the origin station.", 1},
	{"colonyComplete", "Торговля и колонизация", "Trade and colonization", "Строительство завершено", "Construction complete", "Терминал подтвердил завершение отслеживаемого строительства.", "A construction depot confirmed completion of a tracked project.", 2},
	{"colonyFailed", "Торговля и колонизация", "Trade and colonization", "Строительство не удалось", "Construction failed", "Терминал подтвердил неудачу отслеживаемого строительства.", "A construction depot confirmed failure of a tracked project.", 2},
	{"engineering", "Инженерия", "Engineering", "Материалы для плана собраны", "Engineering materials ready", "Подтверждённых запасов достаточно для активного плана.", "Verified inventory covers the active engineering plan.", 1},
	{"engineerInvited", "Инженерия", "Engineering", "Получено приглашение инженера", "Engineer invitation received", "Журнал подтвердил приглашение к инженеру.", "The journal confirmed an engineer invitation.", 1},
	{"engineerUnlocked", "Инженерия", "Engineering", "Инженер открыт", "Engineer unlocked", "Журнал подтвердил открытие инженера.", "The journal confirmed that an engineer was unlocked.", 2},
	{"task", "Задачи", "Tasks", "Миссия завершена", "Mission completed", "Игра записала успешное завершение миссии.", "The game logged a successful mission completion.", 1},
	{"missionFailed", "Задачи", "Tasks", "Миссия провалена", "Mission failed", "Игра перевела миссию в состояние провала.", "The game marked a mission as failed.", 2},
	{"objective", "Задачи", "Tasks", "Цель выполнена", "Objective completed", "Вы завершили цель в трекере приложения.", "You completed an objective in the app's tracker.", 1},
}

var categories = func() map[string]Category {
	out := make(map[string]Category, len(catalog))
	for _, item := range catalog {
		out[item.ID] = item
	}
	return out
}()
