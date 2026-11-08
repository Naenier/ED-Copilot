package presentation

import (
	"testing"
	"unicode"

	"github.com/Naenier/ED-Copilot/internal/exobiology"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestExobiologyReferenceEnglishRenderedContent(t *testing.T) {
	check := func(id string, raw any) {
		t.Helper()
		if text, ok := raw.(string); ok {
			for _, r := range text {
				if unicode.Is(unicode.Cyrillic, r) {
					t.Errorf("%s untranslated rendered text: %s", id, text)
					return
				}
			}
		}
	}
	catalogue := exobiology.Catalog()
	for offset := 0; offset < 126; offset += 48 {
		for _, summary := range v.Rows(catalogue.List(exobiology.Query{Offset: offset, Limit: 48})["entries"]) {
			id := v.Str(summary["id"])
			entry, _ := catalogue.Entry(id)
			english := LocalizeReference("en", entry)
			for _, field := range []string{"title", "description", "summary"} {
				check(id, english[field])
			}
			properties := func(raw any) {
				for _, r := range v.Rows(raw) {
					check(id, r["label"])
					check(id, r["value"])
				}
			}
			properties(english["properties"])
			for _, section := range v.Rows(english["sections"]) {
				check(id, section["title"])
				properties(section["properties"])
				if section["type"] == "facts" {
					properties(section["items"])
				}
				for _, p := range v.Arr(section["paragraphs"]) {
					check(id, p)
				}
			}
		}
	}
}
