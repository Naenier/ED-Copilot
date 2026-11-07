package learning

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type M = v.M
type Option func(*Library)

func WithClock(now func() time.Time) Option {
	return func(l *Library) {
		if now != nil {
			l.now = now
		}
	}
}

type Library struct {
	mu         sync.RWMutex
	now        func() time.Time
	selected   string
	completed  M
	bookmarks  M
	opened     M
	positions  M
	answers    M
	checklists M
	options    M
	lessons    map[string]Lesson
	courses    map[string]Course
}

func New(saved M, options ...Option) *Library {
	l := &Library{now: time.Now, completed: M{}, bookmarks: M{}, opened: M{}, positions: M{}, answers: M{}, checklists: M{}, options: M{"courseId": "", "query": "", "filter": "all"}, lessons: map[string]Lesson{}, courses: map[string]Course{}}
	for _, option := range options {
		option(l)
	}
	for _, lesson := range catalog().Lessons {
		l.lessons[lesson.ID] = lesson
	}
	for _, course := range catalog().Courses {
		l.courses[course.ID] = course
	}
	if id := v.Str(saved["selectedLessonId"]); l.lessons[id].ID != "" {
		l.selected = id
	}
	for id := range l.lessons {
		if at := v.Str(v.At(saved, "completed", id)); !v.Time(at).IsZero() {
			l.completed[id] = at
		}
		if v.Bool(v.At(saved, "bookmarks", id)) {
			l.bookmarks[id] = true
		}
		if at := v.Str(v.At(saved, "opened", id)); !v.Time(at).IsZero() {
			l.opened[id] = at
		}
		if pos := v.Obj(v.At(saved, "positions", id)); len(pos) > 0 {
			if section, ratio, err := l.validPosition(id, pos); err == nil {
				l.positions[id] = M{"sectionId": section, "ratio": ratio}
			}
		}
		if answers := v.Obj(v.At(saved, "answers", id)); len(answers) > 0 {
			valid := M{}
			for _, question := range l.lessons[id].Quiz {
				if choice, known := v.Number(answers[question.ID]); known && choice == math.Trunc(choice) && choice >= 0 && choice < float64(len(question.Choices)) {
					valid[question.ID] = int(choice)
				}
			}
			if len(valid) > 0 {
				l.answers[id] = valid
			}
		}
		checks := M{}
		for _, section := range l.lessons[id].Sections {
			for index := range section.Checklist {
				key := fmt.Sprintf("%s:%d", section.ID, index)
				if v.Bool(v.At(saved, "checklists", id, key)) {
					checks[key] = true
				}
			}
		}
		if len(checks) > 0 {
			l.checklists[id] = checks
		}
	}
	if opts, err := l.normalizeOptions(v.Obj(saved["options"])); err == nil {
		l.options = opts
	}
	return l
}

func (l *Library) normalizeOptions(body M) (M, error) {
	opts := v.Copy(l.options)
	for _, key := range []string{"courseId", "query", "filter"} {
		if raw, exists := body[key]; exists {
			text, ok := raw.(string)
			if !ok {
				return nil, errors.New("Параметры библиотеки должны быть текстом")
			}
			opts[key] = strings.TrimSpace(text)
		}
	}
	if id := v.Str(opts["courseId"]); id != "" && l.courses[id].ID == "" {
		return nil, errors.New("Курс не найден")
	}
	if utf8.RuneCountInString(v.Str(opts["query"])) > 256 {
		return nil, errors.New("Поисковый запрос слишком длинный")
	}
	if f := v.Str(opts["filter"]); f != "all" && f != "in-progress" && f != "completed" && f != "bookmarked" {
		return nil, errors.New("Неизвестный фильтр обучения")
	}
	return opts, nil
}

func (l *Library) lessonID(body M) (string, error) {
	id := v.Text(body["lessonId"], body["id"], l.selected)
	if l.lessons[id].ID == "" {
		return "", errors.New("Выберите существующий урок")
	}
	return id, nil
}

func (l *Library) validPosition(id string, body M) (string, float64, error) {
	sectionID := v.Str(body["sectionId"])
	if sectionID == "" {
		sectionID = l.lessons[id].Sections[0].ID
	}
	found := false
	for _, section := range l.lessons[id].Sections {
		if section.ID == sectionID {
			found = true
			break
		}
	}
	if !found {
		return "", 0, errors.New("Раздел урока не найден")
	}
	ratio := 0.0
	if raw, exists := body["ratio"]; exists {
		var valid bool
		ratio, valid = v.Number(raw)
		if !valid || ratio < 0 || ratio > 1 {
			return "", 0, errors.New("Позиция чтения должна быть от 0 до 1")
		}
	}
	return sectionID, ratio, nil
}

// Update validates a whole command before changing personal progress.
func (l *Library) Update(body M) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	action := v.Text(body["action"], "configure")
	if action == "configure" {
		opts, err := l.normalizeOptions(body)
		if err != nil {
			return err
		}
		l.options = opts
		return nil
	}
	if action == "reset-progress" {
		courseID := v.Str(body["courseId"])
		if courseID != "" && l.courses[courseID].ID == "" {
			return errors.New("Курс не найден")
		}
		for id, lesson := range l.lessons {
			if courseID == "" || lesson.CourseID == courseID {
				delete(l.completed, id)
				delete(l.opened, id)
				delete(l.positions, id)
				delete(l.answers, id)
				delete(l.checklists, id)
			}
		}
		return nil
	}
	id, err := l.lessonID(body)
	if err != nil {
		return err
	}
	switch action {
	case "select":
		var position M
		_, sectionSpecified := body["sectionId"]
		_, ratioSpecified := body["ratio"]
		if sectionSpecified || ratioSpecified {
			section, ratio, err := l.validPosition(id, body)
			if err != nil {
				return err
			}
			position = M{"sectionId": section, "ratio": ratio}
		}
		l.selected, l.opened[id] = id, v.ISO(l.now())
		if position != nil {
			l.positions[id] = position
		}
		return nil
	case "complete":
		complete := true
		if raw, exists := body["completed"]; exists {
			var ok bool
			complete, ok = raw.(bool)
			if !ok {
				return errors.New("Отметка выполнения должна быть логическим значением")
			}
		}
		if complete {
			if l.completed[id] == nil {
				l.completed[id] = v.ISO(l.now())
			}
		} else {
			delete(l.completed, id)
		}
		return nil
	case "bookmark":
		bookmark := !v.Bool(l.bookmarks[id])
		if raw, exists := body["bookmarked"]; exists {
			var ok bool
			bookmark, ok = raw.(bool)
			if !ok {
				return errors.New("Отметка избранного должна быть логическим значением")
			}
		}
		if bookmark {
			l.bookmarks[id] = true
		} else {
			delete(l.bookmarks, id)
		}
		return nil
	case "position":
		section, ratio, err := l.validPosition(id, body)
		if err != nil {
			return err
		}
		l.positions[id] = M{"sectionId": section, "ratio": ratio}
		// A debounced scroll save from the previous lesson may arrive after
		// another lesson is selected. Preserve that newer reading history.
		if l.selected == id || l.opened[id] == nil {
			l.opened[id] = v.ISO(l.now())
		}
		return nil
	case "answer":
		questionID := v.Str(body["questionId"])
		for _, question := range l.lessons[id].Quiz {
			if question.ID != questionID {
				continue
			}
			choice, valid := v.Number(body["choiceIndex"])
			if !valid || choice != math.Trunc(choice) || choice < 0 || choice >= float64(len(question.Choices)) {
				return errors.New("Выберите существующий вариант ответа")
			}
			answers := v.Obj(l.answers[id])
			answers[questionID] = int(choice)
			l.answers[id] = answers
			return nil
		}
		return errors.New("Вопрос урока не найден")
	case "checklist":
		sectionID := v.Str(body["sectionId"])
		index, valid := v.Number(body["index"])
		checked, known := body["checked"].(bool)
		if !valid || index != math.Trunc(index) || !known {
			return errors.New("Некорректная отметка чек-листа")
		}
		for _, section := range l.lessons[id].Sections {
			if section.ID != sectionID {
				continue
			}
			if index < 0 || index >= float64(len(section.Checklist)) {
				return errors.New("Пункт чек-листа не найден")
			}
			checks := v.Obj(l.checklists[id])
			key := fmt.Sprintf("%s:%d", section.ID, int(index))
			if checked {
				checks[key] = true
			} else {
				delete(checks, key)
			}
			l.checklists[id] = checks
			return nil
		}
		return errors.New("Раздел чек-листа не найден")
	default:
		return errors.New("Неизвестное действие Академии")
	}
}

func (l *Library) ExportSettings() M {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return v.Copy(M{"version": 1, "selectedLessonId": l.selected, "completed": l.completed, "bookmarks": l.bookmarks, "opened": l.opened, "positions": l.positions, "answers": l.answers, "checklists": l.checklists, "options": l.options})
}

func (l *Library) Snapshot() M {
	l.mu.RLock()
	defer l.mu.RUnlock()
	courses, lessons, recent := []M{}, []M{}, []M{}
	for _, course := range catalog().Courses {
		row := mapValue(course)
		completed := 0
		for _, id := range course.LessonIDs {
			if l.completed[id] != nil {
				completed++
			}
		}
		row["completedLessons"], row["totalLessons"] = completed, len(course.LessonIDs)
		row["completed"] = completed == len(course.LessonIDs)
		row["firstLessonId"] = course.LessonIDs[0]
		row["percent"] = int(math.Round(float64(completed) * 100 / float64(len(course.LessonIDs))))
		row["nextLessonId"] = course.LessonIDs[0]
		for _, id := range course.LessonIDs {
			if l.completed[id] == nil {
				row["nextLessonId"] = id
				break
			}
		}
		courses = append(courses, row)
	}
	active := M{}
	for index, lesson := range catalog().Lessons {
		row := mapValue(lesson)
		row["courseTitle"], row["category"] = l.courses[lesson.CourseID].Title, l.courses[lesson.CourseID].Category
		row["completed"], row["completedAt"], row["bookmarked"], row["openedAt"] = l.completed[lesson.ID] != nil, l.completed[lesson.ID], v.Bool(l.bookmarks[lesson.ID]), l.opened[lesson.ID]
		row["position"] = v.Copy(v.Obj(l.positions[lesson.ID]))
		row["checklistState"] = v.Copy(v.Obj(l.checklists[lesson.ID]))
		status := "not-started"
		if l.opened[lesson.ID] != nil || len(v.Obj(l.positions[lesson.ID])) > 0 {
			status = "in-progress"
		}
		if l.completed[lesson.ID] != nil {
			status = "complete"
		}
		row["status"] = status
		row["previousLessonId"], row["nextLessonId"] = "", ""
		if index > 0 && catalog().Lessons[index-1].CourseID == lesson.CourseID {
			row["previousLessonId"] = catalog().Lessons[index-1].ID
		}
		if index+1 < len(catalog().Lessons) && catalog().Lessons[index+1].CourseID == lesson.CourseID {
			row["nextLessonId"] = catalog().Lessons[index+1].ID
		}
		answers, correct := v.Copy(v.Obj(l.answers[lesson.ID])), 0
		for _, question := range lesson.Quiz {
			if raw, exists := answers[question.ID]; exists && v.Int(raw) == question.CorrectIndex {
				correct++
			}
		}
		row["quizResult"] = M{"answers": answers, "answered": len(answers), "total": len(lesson.Quiz), "correct": correct, "passed": len(lesson.Quiz) > 0 && correct == len(lesson.Quiz)}
		lessons = append(lessons, row)
		if l.opened[lesson.ID] != nil {
			recent = append(recent, v.Copy(row))
		}
		if lesson.ID == l.selected {
			active = v.Copy(row)
		}
	}
	sort.SliceStable(recent, func(i, j int) bool { return v.Str(recent[i]["openedAt"]) > v.Str(recent[j]["openedAt"]) })
	if len(recent) > 12 {
		recent = recent[:12]
	}
	filtered := []M{}
	for _, lesson := range lessons {
		if course := v.Str(l.options["courseId"]); course != "" && lesson["courseId"] != course {
			continue
		}
		filter := v.Str(l.options["filter"])
		if filter == "bookmarked" && !v.Bool(lesson["bookmarked"]) || filter == "completed" && !v.Bool(lesson["completed"]) || filter == "in-progress" && lesson["status"] != "in-progress" {
			continue
		}
		if Matches(lesson, v.Str(l.options["query"])) {
			filtered = append(filtered, v.Copy(lesson))
		}
	}
	completed, started, inProgress := len(l.completed), 0, 0
	for _, lesson := range lessons {
		if lesson["status"] != "not-started" {
			started++
		}
		if lesson["status"] == "in-progress" {
			inProgress++
		}
	}
	percent := 0
	if len(lessons) > 0 {
		percent = int(math.Round(float64(completed) * 100 / float64(len(lessons))))
	}
	return M{"courses": courses, "lessons": lessons, "filteredLessons": filtered, "selectedLessonId": l.selected, "activeLesson": active, "recent": recent, "progress": M{"completed": completed, "total": len(lessons), "percent": percent, "started": started, "inProgress": inProgress, "bookmarked": len(l.bookmarks)}, "options": v.Copy(l.options), "reviewedAt": catalog().ReviewedAt}
}
