package reference

import (
	"encoding/json"
	"testing"
)

func TestSoloAXGuideIsSearchableAndIllustrated(t *testing.T) {
	store := Catalog()
	for _, query := range []string{"соло-охота", "solo thargoid", "торгоид"} {
		list := store.List(Query{Category: "articles", Query: query})
		found := false
		for _, entry := range list.Entries {
			found = found || entry.ID == axGuideID
		}
		if !found {
			t.Errorf("AX guide missing for search %q", query)
		}
	}
	entry, ok := store.Entry("article/ax-solo")
	if !ok || entry.ID != axGuideID || entry.Group != "article-xeno" || entry.Image == nil {
		t.Fatal("the guide must resolve from its old and short addresses with an offline cover")
	}
	checkJPEG(t, entry.Image, 1600, 1000, 250000)
	figures, systems := 0, 0
	ids := map[string]bool{}
	for _, section := range entry.Sections {
		if section.ID == "" || ids[section.ID] || section.Title == "" || section.EnglishTitle == "" {
			t.Errorf("missing localized title or repeated ID in %q", section.ID)
		}
		ids[section.ID] = true
		if len(section.Paragraphs) != len(section.EnglishParagraphs) || len(section.Columns) != len(section.EnglishColumns) || len(section.Rows) != len(section.EnglishRows) {
			t.Errorf("locale content differs in %q", section.ID)
		}
		for i, row := range section.Rows {
			if i < len(section.EnglishRows) && (len(row) != len(section.Columns) || len(section.EnglishRows[i]) != len(section.EnglishColumns)) {
				t.Errorf("misaligned row in %q", section.ID)
			}
		}
		for _, row := range section.CellLinks {
			for _, id := range row {
				if id != "" {
					if _, ok := store.Entry(id); !ok {
						t.Errorf("missing AX reference link %s", id)
					}
				}
			}
		}
		for _, item := range section.Items {
			if item.Title == "" || item.EnglishTitle == "" || item.Text == "" || item.EnglishText == "" {
				t.Errorf("missing localized guidance in %q", section.ID)
			}
			if section.Type == "route" && item.System != "" {
				systems++
			}
		}
		if section.Figure != nil {
			figures++
			if section.FigureCaption == "" || section.EnglishFigureCaption == "" {
				t.Error("diagram is missing a bilingual legend")
			}
			checkJPEG(t, section.Figure, 1600, 1000, 250000)
		}
	}
	raw, err := json.Marshal(entry)
	if err != nil || len(raw) > 850000 || figures != 2 || systems < 4 {
		t.Fatalf("unusable AX entry: %d bytes, %d figures, %d copyable systems, %v", len(raw), figures, systems, err)
	}
}
