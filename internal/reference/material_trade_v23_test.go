package reference

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestSpecialMaterialsNeverRecommendAnOrdinaryMaterialTrader(t *testing.T) {
	s := Catalog()
	special := 0
	for _, raw := range v.Rows(data.Object("engineering-data.json")["materials"]) {
		if raw["tradeable"] != false {
			continue
		}
		special++
		key := v.Str(raw["key"])
		t.Run(key, func(t *testing.T) {
			e, found := s.Entry("material/" + key)
			if !found {
				t.Fatal("missing special material")
			}
			status := ""
			for _, p := range e.Properties {
				if p.Label == "Обмен у торговца" {
					status = p.Value
				}
			}
			if status != "Недоступен" {
				t.Fatal("nontradeable property missing", status)
			}
			encoded, _ := json.Marshal(e)
			text := string(encoded)
			if strings.Contains(text, "Обменник своей категории") || strings.Contains(text, "остаток можно использовать для обмена") || !strings.Contains(text, "нельзя получить или сдать") {
				t.Fatal("ordinary trader advice leaked into special material", e.Title)
			}
			if e.Sections[0].Paragraphs[0] == "" || strings.Contains(e.Sections[0].Paragraphs[0], "Собирайте данные сканером следов FSD у высокоэнергетических следов уходящих кораблей") {
				t.Fatal("ordinary wake collection leaked into special data")
			}
		})
	}
	if special != 29 {
		t.Fatalf("special-material coverage changed unexpectedly: %d", special)
	}
}

func TestOrdinaryMaterialStillExplainsItsValidTraderAndWakeCollection(t *testing.T) {
	e, found := Catalog().Entry("material/iron")
	if !found {
		t.Fatal("missing normal raw material")
	}
	status := ""
	for _, p := range e.Properties {
		if p.Label == "Обмен у торговца" {
			status = p.Value
		}
	}
	if status != "Доступен" || e.Sections[1].Items[0].Title != "Обменник своей категории" {
		t.Fatal("valid normal exchange was removed")
	}
	for _, raw := range v.Rows(data.Object("engineering-data.json")["materials"]) {
		if raw["tradeable"] == true && strings.Contains(v.Str(raw["key"]), "wake") {
			e, _ := Catalog().Entry("material/" + v.Str(raw["key"]))
			if !strings.Contains(e.Sections[0].Paragraphs[0], "сканером следов FSD") {
				t.Fatal("normal wake guidance lost")
			}
			return
		}
	}
	t.Fatal("fixture no longer covers an ordinary wake material")
}
