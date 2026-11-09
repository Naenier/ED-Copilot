package presentation

import (
	"reflect"
	"strings"
	"testing"
	"unicode"
)

func TestGuideHasCompleteEnglishContentAndStableTopics(t *testing.T) {
	russian := Build("help", "", "", M{"settings": M{"language": "ru"}})
	english := Build("help", "", "", M{"settings": M{"language": "en"}})
	if english.Title != "Guide" || !reflect.DeepEqual(english.Columns, []string{"Section", "Topic"}) ||
		len(english.Rows) != 26 || len(english.Rows) != len(russian.Rows) {
		t.Fatalf("unexpected guide shape: title %q, columns %v, EN %d, RU %d", english.Title, english.Columns, len(english.Rows), len(russian.Rows))
	}
	for i, row := range english.Rows {
		if row.ID != russian.Rows[i].ID || len(row.Cells) != 2 || len(row.Details) < 100 {
			t.Fatalf("incomplete topic %q", row.ID)
		}
		if row.Data["body"] != nil || row.Data["search"] != nil || russian.Rows[i].Data["body"] != nil || russian.Rows[i].Data["search"] != nil {
			t.Fatalf("topic %q duplicates guide content in row data", row.ID)
		}
		for _, field := range append(append([]string(nil), row.Cells...), row.Details) {
			if strings.IndexFunc(field, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) >= 0 {
				t.Fatalf("topic %q contains untranslated content", row.ID)
			}
		}
	}
}

func TestGuideSearchMatchesTheBodyAndBothLanguages(t *testing.T) {
	for _, tc := range []struct {
		language string
		query    string
		wantID   string
	}{
		{"en", "HUD colours", "appearance"},
		{"en", "outstanding procurement", "colonization"},
		{"en", "Шрифт, цвета и палитра HUD", "appearance"},
		{"ru", "outstanding procurement", "colonization"},
	} {
		page := Build("help", "", tc.query, M{"settings": M{"language": tc.language}})
		if len(page.Rows) != 1 || page.Rows[0].ID != tc.wantID {
			t.Fatalf("%q in %s returned %d topics, wanted %q", tc.query, tc.language, len(page.Rows), tc.wantID)
		}
	}
}
