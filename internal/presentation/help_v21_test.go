package presentation

import (
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestHelpMatchesSidebarReaderAndModernScreenNavigation(t *testing.T) {
	topics := map[string]string{}
	for _, topic := range data.Array("help.json") {
		body := v.Str(topic["body"])
		topics[v.Str(topic["id"])] = body
		for _, obsolete := range []string{"Другие действия", "вкладку сверху", "вкладку «Колонизация» через «+»", "Нажмите «+», чтобы добавить", "выпадающий список над таблицей выбирает экран", "подробности появятся под ней", "подробности показываются в нижней части окна"} {
			if strings.Contains(body, obsolete) {
				t.Fatalf("%v contains obsolete 2.0 navigation: %s", topic["id"], obsolete)
			}
		}
	}
	for _, word := range []string{"боковом меню", "Академия", "Энциклопедия", "Помощь с приложением", "Ещё"} {
		if !strings.Contains(topics["start"], word) {
			t.Fatal("first-run help omitted current control", word)
		}
	}
	for _, word := range []string{"перетаскиванием", "Ctrl+K", "подробности", "Режим чтения", "Escape"} {
		if !strings.Contains(topics["interface"], word) {
			t.Fatal("interface help omitted control", word)
		}
	}
}

func TestFrontierHelpDescribesAutomaticRefreshWithoutRemovedButtons(t *testing.T) {
	for _, topic := range data.Array("help.json") {
		id := v.Str(topic["id"])
		if id != "frontier" && id != "carrier" {
			continue
		}
		body := v.Str(topic["body"])
		if !strings.Contains(body, "автоматически") {
			t.Fatalf("%s does not explain automatic observations", id)
		}
		for _, field := range []string{"body", "search"} {
			text := strings.ToLower(v.Str(topic[field]))
			for _, removed := range []string{"«обновить данные»", "«склад из frontier»"} {
				if strings.Contains(text, removed) {
					t.Fatalf("%s.%s still instructs the user to click a removed button", id, field)
				}
			}
		}
	}
	for _, tab := range []string{"account", "carrier"} {
		if !strings.Contains(HelpForLanguage(tab, "en"), "automatically") {
			t.Fatalf("%s English help does not explain automatic observations", tab)
		}
	}
}
