package presentation

import (
	"fmt"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/learning"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func learningState(state M) M {
	if snapshot := v.Obj(state["learning"]); len(snapshot) > 0 {
		return snapshot
	}
	return learning.New(nil).Snapshot()
}

func lessonStatus(lesson M) string {
	if v.Bool(lesson["completed"]) {
		return "Изучено"
	}
	if lesson["status"] == "in-progress" {
		return "В процессе"
	}
	return "Не начато"
}

func lessonKind(raw any) string {
	switch v.Str(raw) {
	case "reading":
		return "Основы"
	case "practice":
		return "Практикум"
	case "checkpoint":
		return "Самопроверка"
	default:
		return "Урок"
	}
}

// LessonText is a plain-text fallback for copy/export and native detail readers.
// The dedicated Academy reader uses the structured sections and quiz fields.
func LessonText(lesson M) string {
	lines := []string{v.Str(lesson["title"]), v.Str(lesson["courseTitle"]), v.Str(lesson["summary"])}
	for _, section := range v.Rows(lesson["sections"]) {
		lines = append(lines, "", v.Str(section["title"]))
		for _, text := range v.Arr(section["paragraphs"]) {
			lines = append(lines, v.Str(text), "")
		}
		for _, text := range v.Arr(section["checklist"]) {
			lines = append(lines, "□ "+v.Str(text))
		}
		if key := v.Str(section["appSection"]); key != "" {
			lines = append(lines, "Раздел приложения: "+Title(key))
		}
	}
	if quiz := v.Rows(lesson["quiz"]); len(quiz) > 0 {
		lines = append(lines, "", "Вопросы для самопроверки")
		for i, question := range quiz {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, v.Str(question["prompt"])))
			for n, choice := range v.Arr(question["choices"]) {
				lines = append(lines, fmt.Sprintf("   %d) %s", n+1, v.Str(choice)))
			}
		}
	}
	lines = append(lines, "", "Дополнительное чтение")
	for _, raw := range v.Arr(lesson["sourceURLs"]) {
		lines = append(lines, v.Str(raw))
	}
	return strings.TrimSpace(strings.Join(lines, "\r\n"))
}

// LearningPage keeps the same underlying lesson IDs for the catalogue, reader,
// bookmarks and progress. Changing a filter never changes the active lesson.
func LearningPage(mode, query string, state M) Page {
	s := learningState(state)
	query = strings.TrimSpace(v.Str(v.At(s, "options", "query")) + " " + query)
	allowed := map[string]bool{}
	for _, lesson := range v.Rows(s["filteredLessons"]) {
		allowed[v.Str(lesson["id"])] = true
	}
	page := Page{Title: "Академия", Description: "Игровые гайды на русском: основы, практика в ED Copilot и самопроверка.", Modes: []string{"Все курсы", "Все уроки", "Продолжить", "Избранное", "Изучено"}, Columns: []string{"Курс", "Тема", "Уроков", "Прогресс"}}
	page.Summary = "Изучено: " + Number(v.At(s, "progress", "completed")) + " из " + Number(v.At(s, "progress", "total")) + " уроков · " + Number(v.At(s, "progress", "percent")) + "%"
	if mode == "" || mode == "Все курсы" {
		for _, course := range v.Rows(s["courses"]) {
			courseHasLesson := false
			for _, lesson := range v.Rows(s["lessons"]) {
				if lesson["courseId"] == course["id"] && allowed[v.Str(lesson["id"])] && learning.Matches(lesson, query) {
					courseHasLesson = true
					break
				}
			}
			if !courseHasLesson {
				continue
			}
			matched := contains(query, course["title"], course["summary"], course["category"])
			if !matched {
				for _, lesson := range v.Rows(s["lessons"]) {
					if lesson["courseId"] == course["id"] && learning.Matches(lesson, query) {
						matched = true
						break
					}
				}
			}
			if !matched {
				continue
			}
			data := v.Copy(course)
			data["kind"], data["lessonId"] = "course", course["nextLessonId"]
			details := []string{v.Str(course["summary"]), "", "Учебный маршрут"}
			for _, lesson := range v.Rows(s["lessons"]) {
				if lesson["courseId"] == course["id"] {
					details = append(details, Number(lesson["order"])+". "+v.Str(lesson["title"])+" · "+lessonStatus(lesson))
				}
			}
			page.Rows = append(page.Rows, row(data, []string{v.Str(course["title"]), v.Str(course["category"]), Number(course["totalLessons"]), Number(course["completedLessons"]) + " / " + Number(course["totalLessons"])}, strings.Join(details, "\r\n")))
		}
		return page
	}
	page.Columns = []string{"Урок", "Курс", "Формат", "Время", "Прогресс"}
	lessons := v.Rows(s["lessons"])
	if mode == "Продолжить" {
		lessons = v.Rows(s["recent"])
	}
	for _, lesson := range lessons {
		if !allowed[v.Str(lesson["id"])] {
			continue
		}
		if mode == "Избранное" && !v.Bool(lesson["bookmarked"]) || mode == "Изучено" && !v.Bool(lesson["completed"]) || mode == "Продолжить" && v.Bool(lesson["completed"]) {
			continue
		}
		if !learning.Matches(lesson, query) {
			continue
		}
		page.Rows = append(page.Rows, row(v.Copy(lesson), []string{v.Str(lesson["title"]), v.Str(lesson["courseTitle"]), lessonKind(lesson["kind"]), Number(lesson["minutes"]) + " мин", lessonStatus(lesson)}, LessonText(lesson)))
	}
	return page
}

func LearningActions(mode string, state, selected M) []Action {
	s := learningState(state)
	id := v.Text(selected["lessonId"], selected["id"], s["selectedLessonId"])
	lesson := M{}
	for _, row := range v.Rows(s["lessons"]) {
		if row["id"] == id {
			lesson = row
			break
		}
	}
	has := len(lesson) > 0
	open := Action{ID: "learning.open", Title: "Открыть урок", Path: "/api/learning", Payload: M{"action": "select", "lessonId": id}, Disabled: !has}
	completeTitle := "Отметить изученным"
	if v.Bool(lesson["completed"]) {
		completeTitle = "Вернуть в обучение"
	}
	bookmarkTitle := "В избранное"
	if v.Bool(lesson["bookmarked"]) {
		bookmarkTitle = "Убрать из избранного"
	}
	actions := []Action{open, {ID: "learning.complete", Title: completeTitle, Path: "/api/learning", Payload: M{"action": "complete", "lessonId": id, "completed": !v.Bool(lesson["completed"])}, Disabled: !has}, {ID: "learning.bookmark", Title: bookmarkTitle, Path: "/api/learning", Payload: M{"action": "bookmark", "lessonId": id, "bookmarked": !v.Bool(lesson["bookmarked"])}, Disabled: !has}}
	if next := v.Str(lesson["nextLessonId"]); next != "" {
		actions = append(actions, Action{ID: "learning.next", Title: "Следующий урок", Path: "/api/learning", Payload: M{"action": "select", "lessonId": next}})
	}
	if previous := v.Str(lesson["previousLessonId"]); previous != "" {
		actions = append(actions, Action{ID: "learning.previous", Title: "Предыдущий урок", Path: "/api/learning", Payload: M{"action": "select", "lessonId": previous}})
	}
	if has {
		actions = append(actions, Action{ID: "learning.copy", Title: "Копировать текст урока", Kind: "copy", Path: LessonText(lesson)})
		for i, raw := range v.Arr(lesson["sourceURLs"]) {
			actions = append(actions, Action{ID: fmt.Sprintf("learning.source.%d", i), Title: "Открыть материал PTN", Kind: "url", Path: v.Str(raw)})
		}
		for _, section := range v.Rows(lesson["sections"]) {
			if key := v.Str(section["appSection"]); key != "" {
				actions = append(actions, Action{ID: "learning.tool." + key, Title: "Открыть: " + Title(key), Kind: "navigate", Path: key})
			}
		}
	}
	courseChoices := []Choice{{Value: "", Label: "Все курсы"}}
	for _, course := range v.Rows(s["courses"]) {
		courseChoices = append(courseChoices, Choice{Value: v.Str(course["id"]), Label: v.Str(course["title"])})
	}
	actions = append(actions, Action{ID: "learning.filter", Title: "Настроить библиотеку", Path: "/api/learning", Payload: M{"action": "configure"}, Fields: []Field{selectField("courseId", "Курс", v.At(s, "options", "courseId"), courseChoices), selectField("filter", "Показывать", v.At(s, "options", "filter"), ChoicePairs("all", "Все уроки", "in-progress", "В процессе", "completed", "Изученные", "bookmarked", "Избранное")), textField("query", "Поиск по материалам", v.At(s, "options", "query"))}})
	actions = append(actions, Action{ID: "learning.reset", Title: "Сбросить прогресс обучения", Path: "/api/learning", Confirm: "Сбросить отметки изучения, ответы и позицию чтения всех уроков? Избранное сохранится.", Payload: M{"action": "reset-progress"}})
	return actions
}
