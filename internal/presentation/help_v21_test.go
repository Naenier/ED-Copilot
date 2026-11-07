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
