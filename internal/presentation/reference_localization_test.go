package presentation

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unicode"

	"github.com/Naenier/ED-Copilot/internal/reference"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func referenceDTO(t *testing.T, id string) M {
	t.Helper()
	entry, ok := reference.Catalog().Entry(id)
	if !ok {
		t.Fatal("reference fixture not found", id)
	}
	return v.Obj(v.Clone(entry))
}
func referenceHasCyrillic(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}
func TestSoloAXGuideUsesCompleteEnglishText(t *testing.T) {
	source := referenceDTO(t, "article/ax-solo")
	english := LocalizeReference("en", source)
	if english["title"] != "Solo Thargoid hunting" || source["title"] != "Соло-охота на таргоидов" {
		t.Fatal("the reader must show the right title in both languages")
	}
	check := func(value any) {
		t.Helper()
		if referenceHasCyrillic(v.Str(value)) {
			t.Errorf("untranslated AX guide content: %q", value)
		}
	}
	for _, section := range v.Rows(english["sections"]) {
		check(section["title"])
		check(section["figureCaption"])
		check(v.At(section, "figure", "alt"))
		for _, paragraph := range v.Arr(section["paragraphs"]) {
			check(paragraph)
		}
		for _, column := range v.Arr(section["columns"]) {
			check(column)
		}
		for _, row := range v.Arr(section["rows"]) {
			for _, cell := range v.Arr(row) {
				check(cell)
			}
		}
		for _, item := range v.Rows(section["items"]) {
			check(item["title"])
			check(item["text"])
		}
	}
}

func TestReferenceCommodityCaptionsCanonicalNameAndActionModes(t *testing.T) {
	source := referenceDTO(t, "commodity/platinum")
	source["personal"] = M{"commander": M{"name": "Платина", "system": "Кислород", "station": "Материалы"}, "notes": "Копировать систему"}
	before, _ := json.Marshal(source)
	english := LocalizeReference("en", source)
	if english["title"] != "Platinum" || english["commodityKey"] != "platinum" || english["id"] != "commodity/platinum" {
		t.Fatal("commodity identity/canonical English changed", english["title"])
	}
	properties := v.Rows(english["properties"])
	if properties[0]["label"] != "Storage" || properties[0]["value"] != "Cargo hold · tonnes" || properties[1]["label"] != "Category" {
		t.Fatal("commodity UI properties were not translated", properties)
	}
	actions := v.Rows(english["actions"])
	if actions[0]["label"] != "Buy" || actions[0]["mode"] != "buy" || actions[1]["label"] != "Sell" || actions[1]["mode"] != "sell" || actions[0]["commodityKey"] != "platinum" {
		t.Fatal("localisation changed action protocol or omitted captions", actions)
	}
	if !reflect.DeepEqual(english["personal"], source["personal"]) || english["description"] != source["description"] {
		t.Fatal("user identity or article content was translated")
	}
	after, _ := json.Marshal(source)
	if string(before) != string(after) {
		t.Fatal("localisation mutated the source/cache DTO")
	}
	v.Rows(english["actions"])[0]["mode"] = "mutated"
	fresh := referenceDTO(t, "commodity/platinum")
	if v.Rows(source["actions"])[0]["mode"] != "buy" || fresh["title"] != "Платина" {
		t.Fatal("translated payload aliases source or shared reference cache")
	}
}
func TestReferenceAmbiguousNamesResolveByRecipeAndCellEntityIDs(t *testing.T) {
	source := M{"id": "blueprint/high-charge-capacity", "category": "blueprints", "title": "Большая ёмкость", "subtitle": "High charge capacity", "sections": []M{{"id": "recipe-PowerDistributor_HighCapacity", "title": "Распределитель питания", "type": "grades", "grades": []M{{"grade": 1, "title": "Класс G1: один ролл", "materials": []M{{"name": "Осколки кристаллов", "count": 3, "entryId": "material/crystalshards"}, {"name": "Железо", "count": 2, "entryId": "material/iron"}}, "effects": []M{{"label": "Ёмкость ENG", "value": "+10% → +20%"}}}}}, {"id": "capabilities", "title": "Чертежи и максимальные классы", "type": "table", "columns": []string{"Чертёж", "Экспериментальный эффект", "Система"}, "rows": [][]string{{"Большая ёмкость", "Большая ёмкость", "Платина"}}, "cellLinks": [][]string{{"blueprint/high-charge-capacity", "experimental/hi-cap", "system/Платина"}}}}}
	english := LocalizeReference("en", source)
	if english["title"] != "High charge capacity" {
		t.Fatal("blueprint meaning was guessed from ambiguous Russian label", english["title"])
	}
	sections := v.Rows(english["sections"])
	grade := v.Rows(sections[0]["grades"])[0]
	materials := v.Rows(grade["materials"])
	if sections[0]["title"] != "Power Distributor" || grade["title"] != "Grade G1: one roll" || materials[0]["name"] != "Crystal Shards" || materials[1]["name"] != "Iron" || materials[0]["entryId"] != "material/crystalshards" || v.Int(materials[0]["count"]) != 3 {
		t.Fatal("canonical recipe name, caption, ID or amount changed", grade)
	}
	if v.Rows(grade["effects"])[0]["label"] != "ENG capacity" {
		t.Fatal("modifier label not translated")
	}
	row := v.Arr(v.Arr(sections[1]["rows"])[0])
	if row[0] != "High charge capacity" || row[1] != "Hi-Cap" || row[2] != "Платина" {
		t.Fatal("linked blueprint/effect meaning or system identity lost", row)
	}
	if !reflect.DeepEqual(sections[1]["cellLinks"], v.Clone(v.Rows(source["sections"])[1]["cellLinks"])) {
		t.Fatal("canonical cell links changed")
	}
}
func TestReferenceListGroupsEngineerNamesAndRelatedEntries(t *testing.T) {
	list := v.Obj(v.Clone(reference.Catalog().List(reference.Query{Category: "engineers", Limit: 48})))
	english := LocalizeReference("en", list)
	if v.Rows(english["categories"])[0]["title"] != "Game mechanics" || len(v.Rows(english["groups"])) == 0 {
		t.Fatal("category/group captions missing")
	}
	for _, group := range v.Rows(english["groups"]) {
		if referenceHasCyrillic(v.Str(group["title"])) {
			t.Fatal("English engineer group has a Russian caption", group)
		}
	}
	for _, entry := range v.Rows(english["entries"]) {
		if referenceHasCyrillic(v.Str(entry["title"])) {
			t.Fatal("canonical engineer name was not selected", entry["id"], entry["title"])
		}
	}
	engineer := referenceDTO(t, "engineer/300100")
	en := LocalizeReference("en", engineer)
	if en["title"] != "Felicity Farseer" {
		t.Fatal("canonical engineer name missing", en["title"])
	}
	oldProperties, newProperties := v.Rows(engineer["properties"]), v.Rows(en["properties"])
	if newProperties[0]["label"] != "System" || newProperties[0]["value"] != oldProperties[0]["value"] || newProperties[1]["value"] != oldProperties[1]["value"] {
		t.Fatal("engineer system/base identity was translated")
	}
	for i, related := range v.Rows(en["related"]) {
		old := v.Rows(engineer["related"])[i]
		if related["id"] != old["id"] || related["category"] != old["category"] {
			t.Fatal("related navigation IDs changed")
		}
	}
}
func TestReferenceSystemUserPropertiesAndBodiesAreProtected(t *testing.T) {
	source := M{"id": "system/synthetic-user", "category": "systems", "title": "Платина", "subtitle": "Звёздная система", "description": "Не переводить пользовательский текст", "properties": []M{{"label": "Станция", "value": "Железо"}, {"label": "Фракция", "value": "Материалы"}, {"label": "Заметки", "value": "Большая ёмкость"}, {"label": "Посадка", "value": "Не подтверждён"}}, "sections": []M{{"id": "bodies", "title": "Исследованные тела", "items": []M{{"title": "Никель", "text": "Сохранено пользователем"}}}, {"id": "observations", "title": "Наблюдения журнала", "type": "table", "columns": []string{"Тело", "Площадка"}, "rows": [][]string{{"Малая", "Малая"}}}}, "actions": []M{{"type": "copy-system", "label": "Копировать систему", "system": "Платина", "mode": "original-mode"}}}
	en := LocalizeReference("en", source)
	if en["title"] != "Платина" || en["subtitle"] != "Star system" || en["description"] != source["description"] {
		t.Fatal("system name/content was altered")
	}
	properties := v.Rows(en["properties"])
	if properties[0]["value"] != "Железо" || properties[1]["value"] != "Материалы" || properties[2]["value"] != "Большая ёмкость" || properties[3]["value"] != "Unconfirmed" {
		t.Fatal("protected identities or known enum translation incorrect", properties)
	}
	sections := v.Rows(en["sections"])
	if sections[0]["title"] != "Scanned bodies" || v.Rows(sections[0]["items"])[0]["title"] != "Никель" {
		t.Fatal("body identity changed or section caption omitted")
	}
	row := v.Arr(v.Arr(sections[1]["rows"])[0])
	if row[0] != "Малая" || row[1] != "Small" {
		t.Fatal("identity column versus enum column was confused", row)
	}
	a := v.Rows(en["actions"])[0]
	if a["system"] != "Платина" || a["mode"] != "original-mode" || a["label"] != "Copy system" {
		t.Fatal("action payload or UI caption incorrect", a)
	}
}
func TestReferenceKeepsArticleParagraphsAndRussianCopiesIndependent(t *testing.T) {
	source := referenceDTO(t, "blueprint/AFM_Shielded")
	before, _ := json.Marshal(source)
	english := LocalizeReference("en", source)
	russian := LocalizeReference("ru", source)
	for i, section := range v.Rows(english["sections"]) {
		original := v.Rows(source["sections"])[i]
		if !reflect.DeepEqual(section["paragraphs"], original["paragraphs"]) {
			t.Fatal("article paragraphs were partially translated")
		}
	}
	russian["title"] = "Mutated"
	after, _ := json.Marshal(source)
	if string(before) != string(after) {
		t.Fatal("Russian/English copy aliases shared catalogue")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			language := "ru"
			if i%2 == 0 {
				language = "en"
			}
			r := LocalizeReference(language, source)
			if language == "ru" && r["title"] != source["title"] {
				t.Error("English global state leaked into Russian request")
			}
		}(i)
	}
	wg.Wait()
}
func TestGeneratedGameNameDictionaryUsesTrustedNamesAndOmitsAmbiguities(t *testing.T) {
	raw, err := os.ReadFile("../localization/game_names_en.json")
	if err != nil {
		t.Fatal(err)
	}
	var names map[string]string
	if err = json.Unmarshal(raw, &names); err != nil {
		t.Fatal(err)
	}
	if names["Платина"] != "Platinum" || names["Полоний"] != "Polonium" || names["Алеоид Arcus"] != "Aleoida arcus" || names["Фелисити Фарсир"] != "Felicity Farseer" {
		t.Fatal("trusted commodity/material/species/engineer names missing")
	}
	for _, name := range []string{"Амфора", "Большая ёмкость", "Быстрая зарядка", "Осколки кристаллов", "Опал бездны", "Бастнезит"} {
		if _, ok := names[name]; ok {
			t.Fatal("ambiguous label received a global English meaning", name)
		}
	}
	for _, english := range names {
		if strings.TrimSpace(english) == "" || referenceHasCyrillic(english) {
			t.Fatal("invalid canonical English name", english)
		}
	}
}
