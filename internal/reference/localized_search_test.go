package reference

import (
	"testing"
)

func TestReferenceSearchFindsEnglishDisplayTitlesAndKeepsRussian(t *testing.T) {
	for _, q := range []Query{{Category: "modules", Query: "Frame Shift Drive"}, {Category: "articles", Query: "Material traders"}} {
		result := Catalog().List(q)
		if result.Total == 0 {
			t.Fatalf("display title cannot be searched: %+v", q)
		}
		original := Catalog().List(Query{Category: q.Category, Query: result.Entries[0].Title})
		if original.Total == 0 {
			t.Fatal("original catalogue title no longer searchable")
		}
	}
}
