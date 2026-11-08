package reference

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMissingReferenceFactsDoNotProduceBoilerplate(t *testing.T) {
	unknown := UnknownSystem("Synthetic unknown system")
	if unknown.Description != "" || len(unknown.Sections) != 0 {
		t.Fatal("unknown system filled with generic travel advice", unknown)
	}
	if blueprintDescription("Synthetic unknown blueprint") != "" || experimentalDescription("Synthetic unknown effect") != "" {
		t.Fatal("unknown effect received a fabricated generic description")
	}
	catalog := Catalog()
	computers, ok := catalog.Entry("commodity/computercomponents")
	if !ok || computers.Description != "" || len(computers.Actions) != 2 || computers.CommodityKey != "computercomponents" || len(computers.Badges) == 0 {
		t.Fatal("commodity identity/actions lost or generic description returned", computers)
	}
	for _, id := range []string{"commodity/metaalloys", "commodity/slaves", "commodity/occupiedcryopod"} {
		entry, ok := catalog.Entry(id)
		if !ok || entry.Description == "" {
			t.Fatal("specific use or legality information lost", id)
		}
	}
	entry, ok := catalog.Entry("system/Deciat")
	if !ok || len(entry.Sections) == 0 || entry.Sections[0].ID != "destinations" || len(entry.Sections[0].Rows) == 0 {
		t.Fatal("known engineer destination removed", entry)
	}
	for _, record := range catalog.ordered {
		body, _ := json.Marshal(record.entry)
		for _, filler := range []string{"В этой справочной карточке нет текущей рыночной цены", "Выгодность рейса определяется конкретными предложениями", "Карточка системы ", "Совместимые модули и их рецепты приведены ниже"} {
			if strings.Contains(string(body), filler) {
				t.Fatal("repeated reference filler returned", record.entry.ID, filler)
			}
		}
	}
}
