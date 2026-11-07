// Package learning provides an offline Russian learning library and personal
// study progress. Opening a guide never performs a network request.
package learning

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

//go:embed content.json
var contentJSON []byte

type Course struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Category   string   `json:"category"`
	Icon       string   `json:"icon"`
	Difficulty string   `json:"difficulty"`
	Minutes    int      `json:"minutes"`
	Tags       []string `json:"tags"`
	LessonIDs  []string `json:"lessonIds"`
	SourceURLs []string `json:"sourceURLs"`
}

type Section struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Paragraphs []string `json:"paragraphs,omitempty"`
	Checklist  []string `json:"checklist,omitempty"`
	AppSection string   `json:"appSection,omitempty"`
	AppAction  string   `json:"appAction,omitempty"`
}

type Question struct {
	ID           string   `json:"id"`
	Prompt       string   `json:"prompt"`
	Choices      []string `json:"choices"`
	CorrectIndex int      `json:"correctIndex"`
	Explanation  string   `json:"explanation"`
}

type Lesson struct {
	ID         string     `json:"id"`
	CourseID   string     `json:"courseId"`
	Title      string     `json:"title"`
	Summary    string     `json:"summary"`
	Kind       string     `json:"kind"`
	Difficulty string     `json:"difficulty"`
	Minutes    int        `json:"minutes"`
	Order      int        `json:"order"`
	Tags       []string   `json:"tags"`
	SourceURLs []string   `json:"sourceURLs"`
	Sections   []Section  `json:"sections"`
	Quiz       []Question `json:"quiz,omitempty"`
}

type courseSeed struct {
	ID, Title, Summary, Category, Icon, Difficulty, AppSection, SourceURL string
	Tags                                                                  []string
	BasicsTitle, PracticeTitle, Exercise, Solution, Reflection            string
	Mechanics, Practice, Checklist                                        []string
	Quiz                                                                  []Question
}
type catalogData struct {
	Courses    []Course
	Lessons    []Lesson
	ReviewedAt string
}

var catalogOnce sync.Once
var catalogValue catalogData

func catalog() catalogData {
	catalogOnce.Do(func() {
		var source struct {
			ReviewedAt string       `json:"reviewedAt"`
			Courses    []courseSeed `json:"courses"`
		}
		if err := json.Unmarshal(contentJSON, &source); err != nil {
			panic(fmt.Errorf("learning content: %w", err))
		}
		catalogValue.ReviewedAt = source.ReviewedAt
		for _, seed := range source.Courses {
			ids := []string{seed.ID + ".basics", seed.ID + ".practice", seed.ID + ".checkpoint"}
			base := Lesson{CourseID: seed.ID, Difficulty: seed.Difficulty, Tags: seed.Tags, SourceURLs: []string{seed.SourceURL}}
			basics := base
			basics.ID, basics.Title, basics.Summary, basics.Kind, basics.Minutes, basics.Order = ids[0], seed.BasicsTitle, seed.Summary, "reading", 5, 1
			basics.Sections = []Section{{ID: "mechanics", Title: "Как устроено занятие", Paragraphs: seed.Mechanics}, {ID: "outcome", Title: "Результат урока", Paragraphs: []string{"Объясните своими словами, какие исходные условия нужно проверить до начала. Затем переходите к практикуму: там вы превратите решение в собственный план ED Copilot."}}}
			practice := base
			practice.ID, practice.Title, practice.Summary, practice.Kind, practice.Minutes, practice.Order = ids[1], seed.PracticeTitle, "Практика в ED Copilot: свой план, контрольные точки и разбор результата.", "practice", 8, 2
			practice.Sections = []Section{{ID: "workflow", Title: "Рабочий порядок", Paragraphs: seed.Practice, AppSection: seed.AppSection, AppAction: "Открыть нужный раздел"}, {ID: "checklist", Title: "Проверьте перед выполнением", Checklist: seed.Checklist}}
			checkpoint := base
			checkpoint.ID, checkpoint.Title, checkpoint.Summary, checkpoint.Kind, checkpoint.Minutes, checkpoint.Order = ids[2], "Самопроверка: "+seed.Title, "Учебный пример, объяснение расчёта и короткие вопросы.", "checkpoint", 5, 3
			checkpoint.Sections = []Section{{ID: "exercise", Title: "Решите самостоятельно", Paragraphs: []string{seed.Exercise}}, {ID: "solution", Title: "Разбор учебного примера", Paragraphs: []string{seed.Solution}}, {ID: "reflection", Title: "Примените к своей игре", Paragraphs: []string{seed.Reflection}}}
			checkpoint.Quiz = seed.Quiz
			catalogValue.Courses = append(catalogValue.Courses, Course{ID: seed.ID, Title: seed.Title, Summary: seed.Summary, Category: seed.Category, Icon: seed.Icon, Difficulty: seed.Difficulty, Minutes: basics.Minutes + practice.Minutes + checkpoint.Minutes, Tags: seed.Tags, LessonIDs: ids, SourceURLs: []string{seed.SourceURL}})
			catalogValue.Lessons = append(catalogValue.Lessons, basics, practice, checkpoint)
		}
		if err := validateCatalog(catalogValue); err != nil {
			panic(err)
		}
	})
	return catalogValue
}

func validateCatalog(c catalogData) error {
	seen := map[string]bool{}
	for _, course := range c.Courses {
		if course.ID == "" || seen[course.ID] || course.Title == "" || len(course.LessonIDs) == 0 {
			return fmt.Errorf("invalid learning course %q", course.ID)
		}
		seen[course.ID] = true
		for _, raw := range course.SourceURLs {
			u, err := url.Parse(raw)
			if err != nil || u.Scheme != "https" || u.Host != "pilotstradenetwork.com" || !strings.HasPrefix(u.Path, "/guides/") {
				return fmt.Errorf("invalid course source %q", raw)
			}
		}
	}
	lessons := map[string]Lesson{}
	for _, lesson := range c.Lessons {
		if lesson.ID == "" || lessons[lesson.ID].ID != "" || !seen[lesson.CourseID] || len(lesson.Sections) == 0 {
			return fmt.Errorf("invalid learning lesson %q", lesson.ID)
		}
		sections := map[string]bool{}
		for _, section := range lesson.Sections {
			if section.ID == "" || sections[section.ID] || section.Title == "" || len(section.Paragraphs)+len(section.Checklist) == 0 {
				return fmt.Errorf("invalid lesson section %q/%q", lesson.ID, section.ID)
			}
			sections[section.ID] = true
		}
		questions := map[string]bool{}
		for _, question := range lesson.Quiz {
			if question.ID == "" || questions[question.ID] || question.Prompt == "" || len(question.Choices) < 2 || question.CorrectIndex < 0 || question.CorrectIndex >= len(question.Choices) || question.Explanation == "" {
				return fmt.Errorf("invalid lesson question %q/%q", lesson.ID, question.ID)
			}
			questions[question.ID] = true
		}
		lessons[lesson.ID] = lesson
	}
	for _, course := range c.Courses {
		for _, id := range course.LessonIDs {
			if lessons[id].CourseID != course.ID {
				return fmt.Errorf("course %q references missing lesson %q", course.ID, id)
			}
		}
	}
	return nil
}

func mapValue(value any) v.M {
	bytes, _ := json.Marshal(value)
	decoder := json.NewDecoder(strings.NewReader(string(bytes)))
	decoder.UseNumber()
	var out v.M
	_ = decoder.Decode(&out)
	return out
}

// Catalog returns independent JSON-shaped copies for UI consumers.
func Catalog() []v.M {
	rows := []v.M{}
	for _, lesson := range catalog().Lessons {
		rows = append(rows, mapValue(lesson))
	}
	return rows
}

func CourseCatalog() []v.M {
	rows := []v.M{}
	for _, course := range catalog().Courses {
		rows = append(rows, mapValue(course))
	}
	return rows
}

func fold(text string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(text)), "ё", "е")
}

// Matches searches complete lesson text using all query words, including tags.
func Matches(lesson v.M, query string) bool {
	if strings.TrimSpace(query) == "" {
		return true
	}
	parts := []string{v.Str(lesson["title"]), v.Str(lesson["summary"]), v.Str(lesson["courseTitle"]), v.Str(lesson["category"])}
	for _, tag := range v.Arr(lesson["tags"]) {
		parts = append(parts, v.Str(tag))
	}
	for _, section := range v.Rows(lesson["sections"]) {
		parts = append(parts, v.Str(section["title"]))
		for _, field := range []string{"paragraphs", "checklist"} {
			for _, text := range v.Arr(section[field]) {
				parts = append(parts, v.Str(text))
			}
		}
	}
	for _, question := range v.Rows(lesson["quiz"]) {
		parts = append(parts, v.Str(question["prompt"]), v.Str(question["explanation"]))
	}
	haystack := fold(strings.Join(parts, " "))
	for _, word := range strings.Fields(fold(query)) {
		if !strings.Contains(haystack, word) {
			return false
		}
	}
	return true
}
