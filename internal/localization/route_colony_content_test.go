package localization

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestColonyAdviceHasCompleteEnglishParagraphs(t *testing.T) {
	body, err := os.ReadFile("../colonization/advice.json")
	if err != nil {
		t.Fatal(err)
	}
	var source any
	if err := json.Unmarshal(body, &source); err != nil {
		t.Fatal(err)
	}
	var visit func(any)
	count := 0
	visit = func(value any) {
		switch row := value.(type) {
		case map[string]any:
			for key, child := range row {
				if key == "title" || key == "text" {
					text, _ := child.(string)
					translated := T("en", text)
					if translated == text || containsCyrillic(translated) || strings.TrimSpace(translated) == "" {
						t.Errorf("untranslated colony advice %s: %q", key, text)
					}
					count++
				} else {
					visit(child)
				}
			}
		case []any:
			for _, child := range row {
				visit(child)
			}
		}
	}
	visit(source)
	if count < 20 {
		t.Fatal("advice coverage did not visit the development paths", count)
	}
}

func TestRouteTimingAndColonyNextTasksComposeEnglishWithoutChangingIdentities(t *testing.T) {
	for _, source := range []string{
		"Подготовка 15 мин, перезарядка 5 мин между прыжками, прочее 1 мин. Очередь и задержки могут быть больше.",
		"По текущей очереди перед этим этапом не хватает 4 очк. T2. Поставьте раньше подходящие объекты T1 и завершите их. Затем обновите текущий баланс по экрану архитектора. Не расходуйте одни и те же очки на параллельные стройки.",
		"Ещё 2 этапов имеют расчётный дефицит очков. Начните с первого предупреждения в очереди и пересчитайте план после изменения порядка.",
	} {
		if got := Text("en", source); containsCyrillic(got) || got == source {
			t.Fatal("formatted operational advice remains partly Russian", got)
		}
	}
	const identity = "Личная станция"
	for _, source := range []string{
		identity + ": доставить Aluminium — 40 т",
		identity + ": посетите депо для точного списка товаров.",
		identity + ": проверьте завершение строительства в игре.",
		"Проверьте заявку на " + identity + " у контакта колонизации.",
	} {
		got := TextProtected("en", source, []string{identity})
		if !strings.Contains(got, identity) || containsCyrillic(strings.ReplaceAll(got, identity, "")) {
			t.Fatal("next task did not preserve the user's name or translated only part of the instructions", got)
		}
	}
}
