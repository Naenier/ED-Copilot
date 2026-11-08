package localization

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCleanedReferenceContentHasCompleteEnglishText(t *testing.T) {
	body, err := os.ReadFile("../data/encyclopedia.json")
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Articles []struct {
			ID, Title, Summary string
			Body               []string
		}
	}
	if err = json.Unmarshal(body, &source); err != nil {
		t.Fatal(err)
	}
	cleaned := map[string]bool{"operations": true, "game-modes": true, "teams": true, "squadrons": true, "ranks": true, "galnet": true, "external-data": true}
	count := 0
	for _, article := range source.Articles {
		if !cleaned[article.ID] {
			continue
		}
		for _, text := range append([]string{article.Title, article.Summary}, article.Body...) {
			translated := Text("en", text)
			if translated == "" || containsCyrillic(translated) {
				t.Fatal("cleaned reference text not fully translated", article.ID, text, translated)
			}
		}
		count++
	}
	if count != len(cleaned) {
		t.Fatal("reference article IDs changed", count)
	}
	if translated := Text("en", "Запреты зависят от юрисдикции; проверяйте статус груза в пункте назначения."); containsCyrillic(translated) {
		t.Fatal("legality note untranslated", translated)
	}
}
