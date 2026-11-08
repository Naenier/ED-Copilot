// Package learning provides an offline bilingual learning library and personal
// study progress. Opening a guide never performs a network request.
package learning

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

//go:embed content*.json
var contentFiles embed.FS

var contentJSON, _ = contentFiles.ReadFile("content.json")
var englishContentJSON, englishContentErr = contentFiles.ReadFile("content_en.json")

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
	BasicsSections, PracticeSections, CheckpointSections                  []Section
	BasicsMinutes, PracticeMinutes, CheckpointMinutes                     int
}
type catalogData struct {
	Courses    []Course
	Lessons    []Lesson
	ReviewedAt string
	Language   string
	courseRows []M
	lessonRows []M
	searchText map[string]string
	courseByID map[string]Course
}

var catalogOnce [2]sync.Once
var catalogValue [2]catalogData

func catalog() catalogData {
	return catalogForLanguage("ru")
}

func catalogForLanguage(language string) catalogData {
	index, raw := 0, contentJSON
	if strings.EqualFold(strings.TrimSpace(language), "en") {
		if englishContentErr != nil {
			// During content authoring the English seed can be absent. The
			// release contract test requires both complete paired catalogues.
			return catalog()
		}
		index, raw = 1, englishContentJSON
	}
	catalogOnce[index].Do(func() {
		lang := []string{"ru", "en"}[index]
		value, err := buildCatalog(raw, lang)
		if err != nil {
			panic(fmt.Errorf("learning content (%s): %w", lang, err))
		}
		if index == 1 {
			if err := validateCatalogParity(catalog(), value); err != nil {
				panic(err)
			}
		}
		catalogValue[index] = value
	})
	return catalogValue[index]
}

func buildCatalog(raw []byte, language string) (catalogData, error) {
	var source struct {
		ReviewedAt string       `json:"reviewedAt"`
		Courses    []courseSeed `json:"courses"`
	}
	if err := json.Unmarshal(raw, &source); err != nil {
		return catalogData{}, err
	}
	value := catalogData{ReviewedAt: source.ReviewedAt, Language: language, searchText: map[string]string{}, courseByID: map[string]Course{}}
	text := func(ru, en string) string {
		if language == "en" {
			return en
		}
		return ru
	}
	minutes := func(chosen, fallback int) int {
		if chosen > 0 {
			return chosen
		}
		return fallback
	}
	for _, seed := range source.Courses {
		if seed.BasicsMinutes < 0 || seed.PracticeMinutes < 0 || seed.CheckpointMinutes < 0 {
			return catalogData{}, fmt.Errorf("invalid lesson duration in course %q", seed.ID)
		}
		ids := []string{seed.ID + ".basics", seed.ID + ".practice", seed.ID + ".checkpoint"}
		base := Lesson{CourseID: seed.ID, Difficulty: seed.Difficulty, Tags: seed.Tags, SourceURLs: []string{seed.SourceURL}}
		basics := base
		basics.ID, basics.Title, basics.Summary, basics.Kind, basics.Minutes, basics.Order = ids[0], seed.BasicsTitle, seed.Summary, "reading", minutes(seed.BasicsMinutes, 5), 1
		basics.Sections = append([]Section{{ID: "mechanics", Title: text("Как устроено занятие", "How the activity works"), Paragraphs: seed.Mechanics}}, seed.BasicsSections...)
		practice := base
		practice.ID, practice.Title, practice.Kind, practice.Minutes, practice.Order = ids[1], seed.PracticeTitle, "practice", minutes(seed.PracticeMinutes, 8), 2
		if len(seed.Practice) > 0 {
			practice.Summary = seed.Practice[0]
		}
		practice.Sections = append([]Section{{ID: "workflow", Title: text("Рабочий порядок", "Workflow"), Paragraphs: seed.Practice, AppSection: seed.AppSection, AppAction: text("Открыть нужный раздел", "Open the relevant section")}, {ID: "checklist", Title: text("Проверьте перед выполнением", "Before you begin"), Checklist: seed.Checklist}}, seed.PracticeSections...)
		checkpoint := base
		checkpoint.ID, checkpoint.Title, checkpoint.Summary, checkpoint.Kind, checkpoint.Minutes, checkpoint.Order = ids[2], text("Самопроверка: ", "Self-check: ")+seed.Title, text("Учебный пример, объяснение расчёта и короткие вопросы.", "A worked scenario, calculation walkthrough and short questions."), "checkpoint", minutes(seed.CheckpointMinutes, 5), 3
		checkpoint.Sections = []Section{{ID: "exercise", Title: text("Решите самостоятельно", "Try it yourself"), Paragraphs: []string{seed.Exercise}}, {ID: "solution", Title: text("Разбор учебного примера", "Worked example"), Paragraphs: []string{seed.Solution}}}
		if strings.TrimSpace(seed.Reflection) != "" {
			checkpoint.Sections = append(checkpoint.Sections, Section{ID: "reflection", Title: text("Примените к своей игре", "Apply it to your own game"), Paragraphs: []string{seed.Reflection}})
		}
		checkpoint.Sections = append(checkpoint.Sections, seed.CheckpointSections...)
		checkpoint.Quiz = seed.Quiz
		course := Course{ID: seed.ID, Title: seed.Title, Summary: seed.Summary, Category: seed.Category, Icon: seed.Icon, Difficulty: seed.Difficulty, Minutes: basics.Minutes + practice.Minutes + checkpoint.Minutes, Tags: seed.Tags, LessonIDs: ids, SourceURLs: []string{seed.SourceURL}}
		value.Courses = append(value.Courses, course)
		value.courseByID[course.ID] = course
		value.Lessons = append(value.Lessons, basics, practice, checkpoint)
	}
	if err := validateCatalog(value); err != nil {
		return catalogData{}, err
	}
	for _, course := range value.Courses {
		value.courseRows = append(value.courseRows, mapValue(course))
	}
	for _, lesson := range value.Lessons {
		row := mapValue(lesson)
		row["courseTitle"], row["category"] = value.courseByID[lesson.CourseID].Title, value.courseByID[lesson.CourseID].Category
		value.lessonRows = append(value.lessonRows, row)
		value.searchText[lesson.ID] = lessonSearchText(row)
	}
	return value, nil
}

func validateCatalogParity(primary, translated catalogData) error {
	if len(primary.Courses) != len(translated.Courses) || len(primary.Lessons) != len(translated.Lessons) {
		return fmt.Errorf("learning languages have different course or lesson counts")
	}
	for i, course := range primary.Courses {
		other := translated.Courses[i]
		if course.ID != other.ID || strings.Join(course.LessonIDs, "\x00") != strings.Join(other.LessonIDs, "\x00") {
			return fmt.Errorf("learning languages disagree on course %q", course.ID)
		}
	}
	for i, lesson := range primary.Lessons {
		other := translated.Lessons[i]
		if lesson.ID != other.ID || lesson.CourseID != other.CourseID || len(lesson.Sections) != len(other.Sections) || len(lesson.Quiz) != len(other.Quiz) {
			return fmt.Errorf("learning languages disagree on lesson %q", lesson.ID)
		}
		for n, section := range lesson.Sections {
			peer := other.Sections[n]
			if section.ID != peer.ID || len(section.Checklist) != len(peer.Checklist) || section.AppSection != peer.AppSection {
				return fmt.Errorf("learning languages disagree on reader position or checklist %q/%q", lesson.ID, section.ID)
			}
		}
		for n, question := range lesson.Quiz {
			peer := other.Quiz[n]
			if question.ID != peer.ID || question.CorrectIndex != peer.CorrectIndex || len(question.Choices) != len(peer.Choices) {
				return fmt.Errorf("learning languages disagree on answer %q/%q", lesson.ID, question.ID)
			}
		}
	}
	return nil
}

func validateCatalog(c catalogData) error {
	seen := map[string]bool{}
	for _, course := range c.Courses {
		if course.ID == "" || seen[course.ID] || course.Title == "" || course.Summary == "" || len(course.LessonIDs) == 0 || course.Minutes <= 0 {
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
		if lesson.ID == "" || lessons[lesson.ID].ID != "" || lesson.Title == "" || lesson.Summary == "" || lesson.Minutes <= 0 || !seen[lesson.CourseID] || len(lesson.Sections) == 0 {
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

// Catalogue templates contain only decoded JSON values and are private and
// immutable. Copy their containers without repeatedly encoding the same text.
func copyCatalogValue(value any) any {
	switch row := value.(type) {
	case M:
		out := make(M, len(row))
		for key, child := range row {
			out[key] = copyCatalogValue(child)
		}
		return out
	case []any:
		out := make([]any, len(row))
		for index, child := range row {
			out[index] = copyCatalogValue(child)
		}
		return out
	default:
		return value
	}
}

func copyCatalogRow(row M) M { return copyCatalogValue(row).(M) }

// Catalog returns independent JSON-shaped copies for UI consumers.
func Catalog() []v.M {
	return CatalogForLanguage("ru")
}

func CatalogForLanguage(language string) []v.M {
	rows := []v.M{}
	for _, lesson := range catalogForLanguage(language).lessonRows {
		rows = append(rows, copyCatalogRow(lesson))
	}
	return rows
}

func CourseCatalog() []v.M {
	return CourseCatalogForLanguage("ru")
}

func CourseCatalogForLanguage(language string) []v.M {
	rows := []v.M{}
	for _, course := range catalogForLanguage(language).courseRows {
		rows = append(rows, copyCatalogRow(course))
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
	return matchesSearchText(lessonSearchText(lesson), query)
}

func lessonSearchText(lesson M) string {
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
		for _, choice := range v.Arr(question["choices"]) {
			parts = append(parts, v.Str(choice))
		}
	}
	return fold(strings.Join(parts, " "))
}

func matchesSearchText(haystack, query string) bool {
	for _, word := range strings.Fields(fold(query)) {
		if !strings.Contains(haystack, word) {
			return false
		}
	}
	return true
}
