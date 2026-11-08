package reference

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestCatalogCoverage(t *testing.T) {
	s := Catalog()
	want := map[string]int{"articles": 96, "ships": 49, "engineers": 38, "modules": 39, "materials": 137, "blueprints": 36, "experimentals": 60, "commodities": 412, "systems": 38}
	for _, category := range s.categories {
		t.Logf("%s=%d", category.ID, category.Count)
		if n, ok := want[category.ID]; ok && n != category.Count {
			t.Errorf("%s: got %d want %d", category.ID, category.Count, n)
		}
		if category.Count == 0 {
			t.Errorf("empty category %s", category.ID)
		}
	}
	for _, raw := range objectRows("encyclopedia-ships.json") {
		id := "ship/" + v.Str(raw["key"])
		t.Run(id, func(t *testing.T) {
			e, ok := s.Entry(id)
			if !ok || len(e.Sections) < 2 || e.Description == "" || e.Image == nil {
				t.Fatalf("incomplete ship %s: found=%v", id, ok)
			}
			checkJPEG(t, e.Image, 640, 420, 150000)
		})
	}
	for _, raw := range v.Rows(data.Object("engineering-data.json")["engineers"]) {
		id := "engineer/" + v.Str(raw["key"])
		t.Run(id, func(t *testing.T) {
			e, ok := s.Entry(id)
			if !ok || e.Image == nil || len(e.Sections) != 3 || len(e.Sections[0].Items) < 3 || len(e.Sections[1].Rows) == 0 {
				t.Fatalf("incomplete ship engineer %s", id)
			}
			checkJPEG(t, e.Image, 400, 400, 150000)
		})
	}
}

func checkJPEG(t *testing.T, image *Image, maxWidth, maxHeight, maxBytes int) {
	t.Helper()
	if !strings.HasPrefix(image.Src, "data:image/jpeg;base64,") {
		t.Fatal("nonembedded image")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(image.Src, "data:image/jpeg;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width < 48 || cfg.Height < 32 || cfg.Width > maxWidth || cfg.Height > maxHeight || len(raw) > maxBytes {
		t.Fatalf("bad image %dx%d %d bytes", cfg.Width, cfg.Height, len(raw))
	}
}

func TestRecipeParity(t *testing.T) {
	s := Catalog()
	c := data.Object("engineering-data.json")
	for _, raw := range v.Rows(c["blueprints"]) {
		key := v.Str(raw["key"])
		t.Run("blueprint/"+key, func(t *testing.T) {
			e, ok := s.Entry("blueprint/" + key)
			if !ok {
				t.Fatal("missing alias")
			}
			var section *Section
			for i := range e.Sections {
				if e.Sections[i].ID == "recipe-"+key {
					section = &e.Sections[i]
				}
			}
			if section == nil {
				t.Fatal("recipe was lost during grouping")
			}
			grades := v.Obj(raw["grades"])
			if len(section.Grades) != len(grades) {
				t.Fatal("grade count changed")
			}
			for _, g := range section.Grades {
				want := v.Rows(grades[fmt.Sprint(g.Grade)])
				if len(g.Materials) != len(want) {
					t.Fatal("material count changed")
				}
				for i, m := range g.Materials {
					if m.EntryID != "material/"+v.Str(want[i]["key"]) || m.Count != v.Int(want[i]["count"]) {
						t.Fatalf("ingredient changed: %+v vs %+v", m, want[i])
					}
				}
			}
		})
	}
	for _, raw := range v.Rows(c["experimentals"]) {
		key := v.Str(raw["key"])
		t.Run("experimental/"+key, func(t *testing.T) {
			e, ok := s.Entry("experimental/" + key)
			if !ok {
				t.Fatal("missing effect alias")
			}
			for _, section := range e.Sections {
				if section.ID == "recipe-"+key {
					want := v.Rows(raw["materials"])
					if len(section.Grades) != 1 || len(section.Grades[0].Materials) != len(want) {
						t.Fatal("recipe length changed")
					}
					for i, m := range section.Grades[0].Materials {
						if m.EntryID != "material/"+v.Str(want[i]["key"]) || m.Count != v.Int(want[i]["count"]) {
							t.Fatal("ingredient changed")
						}
					}
					return
				}
			}
			t.Fatal("recipe was lost during grouping")
		})
	}
}

func TestLinksAndAliases(t *testing.T) {
	s := Catalog()
	for _, r := range s.ordered {
		for _, id := range r.relatedIDs {
			if _, ok := s.Entry(id); !ok {
				t.Errorf("%s refers to missing %s", r.entry.ID, id)
			}
		}
		for _, section := range r.entry.Sections {
			for _, item := range section.Items {
				if item.EntryID != "" {
					if _, ok := s.Entry(item.EntryID); !ok {
						t.Errorf("missing item %s", item.EntryID)
					}
				}
			}
			for _, grade := range section.Grades {
				for _, m := range grade.Materials {
					if _, ok := s.Entry(m.EntryID); !ok {
						t.Errorf("missing material %s", m.EntryID)
					}
				}
			}
		}
	}
	for _, id := range []string{"ship/Panther Clipper Mk II", "ship/Sidewinder", "engineer/Felicity Farseer", "engineer/Фелисити Фарсир", "module/bsg", "module/psg", "module/advmc", "module/amr", "system/Shinrarta%20Dezhra", "commodity/Platinum"} {
		if _, ok := s.Entry(id); !ok {
			t.Errorf("alias missing: %s", id)
		}
	}
}

func TestListBoundsAndImmutability(t *testing.T) {
	s := Catalog()
	all := s.List(Query{Category: "all", Limit: 999999, Offset: -2})
	if len(all.Entries) != 48 || all.Limit != 48 || all.Offset != 0 {
		t.Fatal("unbounded list")
	}
	end := s.List(Query{Category: "ships", Offset: 999999})
	if end.Offset != 49 || len(end.Entries) != 0 {
		t.Fatal("bad terminal page")
	}
	if result := s.List(Query{Category: "ships", Query: "Panther Clipper", Limit: 1}); result.Total != 1 || len(result.Entries) != 1 {
		t.Fatal("search failed")
	}
	original, _ := s.Entry("engineer/300100")
	changed, _ := s.Entry(original.ID)
	changed.Properties[0].Value = "mutated"
	changed.Sections[0].Items[0].Text = "mutated"
	changed.Image.Src = "mutated"
	if len(changed.Related) > 0 {
		changed.Related[0].Title = "mutated"
	}
	again, _ := s.Entry(original.ID)
	if !reflect.DeepEqual(original, again) {
		t.Fatal("caller mutated immutable records")
	}
	list := s.List(Query{Category: "ships"})
	list.Categories[0].Title = "mutated"
	list.Entries[0].Meta[0].Value = "mutated"
	list.Entries[0].Image.Src = "mutated"
	next := s.List(Query{Category: "ships"})
	if next.Categories[0].Title == "mutated" || next.Entries[0].Image.Src == "mutated" || next.Entries[0].Meta[0].Value == "mutated" {
		t.Fatal("mutable summary shared")
	}
}

func TestSummaryPayloadAndOfflineMedia(t *testing.T) {
	s := Catalog()
	for _, category := range []string{"ships", "engineers"} {
		list := s.List(Query{Category: category})
		raw, err := json.Marshal(list)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > 300000 {
			t.Fatalf("list payload too big: %s %d", category, len(raw))
		}
		for _, e := range list.Entries {
			if e.Image != nil {
				checkJPEG(t, e.Image, 160, 120, 12000)
			}
		}
	}
	for _, r := range s.ordered {
		e, _ := s.Entry(r.entry.ID)
		raw, _ := json.Marshal(e)
		for _, forbidden := range []string{"sourceIds", "sourceUrl", "verifiedAt", "https://", "http://"} {
			if bytes.Contains(raw, []byte(forbidden)) {
				t.Errorf("source metadata leaked into %s", r.entry.ID)
			}
		}
	}
	for _, e := range s.List(Query{Category: "commodities", Limit: 48}).Entries {
		full, _ := s.Entry(e.ID)
		if len(full.Actions) != 2 || full.Actions[0].CommodityKey != full.CommodityKey || full.Actions[0].Mode != "buy" || full.Actions[1].Mode != "sell" {
			t.Errorf("bad trade actions %s", e.ID)
		}
	}
}

func TestParallelReaders(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = Catalog().List(Query{Category: "ships", Query: "a"})
				_, _ = Catalog().Entry("engineer/300100")
			}
		}()
	}
	wg.Wait()
}

func TestModifierCoverageAndDescriptions(t *testing.T) {
	c := data.Object("engineering-data.json")
	stats := data.Object("encyclopedia-engineering-stats.json")
	for _, raw := range v.Rows(c["blueprints"]) {
		key := v.Str(raw["key"])
		if lookupDescription(blueprintDescriptions, v.Str(raw["english"])) == "" {
			t.Errorf("generic blueprint description %s", key)
		}
		for grade := range v.Obj(raw["grades"]) {
			t.Run(key+"/G"+grade, func(t *testing.T) {
				features := v.Obj(v.Obj(v.Obj(stats["blueprints"])[key])[grade])
				out := blueprintModifiers(stats, key, v.Int(grade))
				if len(features) == 0 || len(features) != len(out) {
					t.Fatalf("lost feature: %d -> %d", len(features), len(out))
				}
				for _, property := range out {
					if property.Label == "" || property.Value == "" {
						t.Fatal("empty translated modifier")
					}
				}
			})
		}
	}
	for _, raw := range v.Rows(c["experimentals"]) {
		if lookupDescription(effectDescriptions, v.Str(raw["english"])) == "" {
			t.Errorf("generic effect description %s", v.Str(raw["english"]))
		}
	}
	properties := blueprintModifiers(stats, "FSD_LongRange", 5)
	for _, property := range properties {
		if property.Label == "Оптимальная масса" && property.Value != "+45% → +55%" {
			t.Errorf("wrong FSD G5 range: %s", property.Value)
		}
	}
}

func TestNewShipDetailsAndModuleFamilies(t *testing.T) {
	s := Catalog()
	for _, key := range []string{"corsair", "explorernx", "smallcombat01nx", "mediumtransport01", "anacondamkii"} {
		e, _ := s.Entry("ship/" + key)
		found := false
		for _, section := range e.Sections {
			if section.ID == "mounts" && len(section.Rows) >= 6 {
				found = true
			}
		}
		if !found {
			t.Errorf("new ship lacks layout %s", key)
		}
	}
	for _, pair := range [][2]string{{"bsg", "sg"}, {"psg", "sg"}, {"advmc", "mc"}, {"amr", "mr"}} {
		a, _ := s.Entry("module/" + pair[0])
		b, _ := s.Entry("module/" + pair[1])
		if a.ID != b.ID {
			t.Errorf("duplicate family %v", pair)
		}
	}
	e, _ := s.Entry("module/csl")
	if !strings.Contains(e.Title, "каустических") {
		t.Fatal("caustic sink mislabeled")
	}
	if _, ok := s.Entry("ship/does-not-exist"); ok {
		t.Fatal("unknown entity invented")
	}
	unknown := UnknownSystem("\x01Some System\n")
	if unknown.Title != "Some System" || len(unknown.Sections) == 0 {
		t.Fatal("unsafe unknown system")
	}
}

func TestEngineerCapabilityCellLinksAlignAndRemainIndependent(t *testing.T) {
	s := Catalog()
	for _, summary := range s.List(Query{Category: "engineers", Limit: 48}).Entries {
		entry, _ := s.Entry(summary.ID)
		if !strings.HasPrefix(summary.ID, "engineer/3") {
			continue
		}
		for _, section := range entry.Sections {
			if section.ID != "modifications" {
				continue
			}
			if len(section.Rows) == 0 || len(section.CellLinks) != len(section.Rows) {
				t.Fatalf("links lost for %s", entry.ID)
			}
			for index, row := range section.Rows {
				links := section.CellLinks[index]
				if len(links) != len(row) || len(links) != 3 {
					t.Fatal("cell link alignment", entry.ID, index)
				}
				for column, category := range []string{"modules", "blueprints"} {
					target, found := s.Entry(links[column])
					if !found || target.Category != category {
						t.Fatalf("invalid capability link %s", links[column])
					}
				}
				if links[2] != "" {
					t.Fatal("grade must not link to an unrelated page")
				}
			}
		}
		if entry.Actions[0].Type != "copy-system" || entry.Actions[0].System != entry.Properties[0].Value || entry.Properties[0].EntryID == "" {
			t.Fatal("engineer lost system navigation/copy", entry.ID)
		}
	}
	first, _ := s.Entry("engineer/300100")
	again, _ := s.Entry(first.ID)
	first.Sections[1].CellLinks[0][0] = "tampered"
	unchanged, _ := s.Entry(first.ID)
	if !reflect.DeepEqual(again, unchanged) {
		t.Fatal("cell link DTO aliases catalogue")
	}
	system, _ := s.Entry("system/deciat")
	destinations := system.Sections[0]
	if len(destinations.CellLinks) != len(destinations.Rows) || destinations.CellLinks[0][1] != "engineer/300100" {
		t.Fatal("system workshop engineer link lost")
	}
}

func BenchmarkListShips(b *testing.B) {
	s := Catalog()
	s.List(Query{Category: "ships"})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.List(Query{Category: "ships"})
	}
}
func BenchmarkEntryEngineer(b *testing.B) {
	s := Catalog()
	s.Entry("engineer/300100")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.Entry("engineer/300100")
	}
}
