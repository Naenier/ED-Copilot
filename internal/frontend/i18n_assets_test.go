package frontend

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOfflineHTMLInitializesSharedTranslationsBeforeUIAndAutocomplete(t *testing.T) {
	document := HTML("0123456789abcdef0123456789abcdef01234567", false)
	const prefix = "window.CopilotTranslations="
	first := strings.Index(document, prefix)
	translator := strings.Index(document, "window.CopilotI18n = {")
	controller := strings.Index(document, "const C={t,locale,tab:")
	autocomplete := strings.Index(document, "window.SystemSearchUI = {")
	if first < 0 || translator <= first || autocomplete <= translator || controller <= autocomplete {
		t.Fatal("offline catalogue, translator and system search must initialize before the controller")
	}
	raw := strings.SplitN(document[first+len(prefix):], ";\n", 2)[0]
	var catalogue map[string]string
	if err := json.Unmarshal([]byte(raw), &catalogue); err != nil {
		t.Fatalf("embedded translations are not a JSON object: %v", err)
	}
	for _, key := range []string{"Настройки", "Калькулятор флотоносца", "Колонизация", "Скрыть подробности", "Экзобиология"} {
		if catalogue[key] == "" || catalogue[key] == key {
			t.Errorf("missing English translation for %q", key)
		}
	}
	if strings.Contains(raw, "<") || strings.Count(document, "</script>") != 1 {
		t.Fatal("translation text must not be able to close the executable script")
	}
	for _, dependency := range []string{"system-search.css", "i18n.js", "system-search.js", "exobiology.js", "exobiology.css"} {
		if data, err := Files.ReadFile(dependency); err != nil || len(data) == 0 {
			t.Errorf("required offline locale/autocomplete asset %s is absent: %v", dependency, err)
		}
	}
}
