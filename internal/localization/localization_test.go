package localization

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestLanguageCatalogAndPhraseBoundaries(t *testing.T) {
	if Normalize("EN") != "en" || Normalize("invalid") != "ru" {
		t.Fatal("language normalization")
	}
	for _, tc := range []struct{ source, want string }{{"Настройки", "Settings"}, {"  Настройки ", "  Settings "}, {"ТОПЛИВО", "FUEL"}, {"Лазер: 100 т/ч", "Laser: 100 t/h"}, {"НеизвестноеСлово", "НеизвестноеСлово"}} {
		if got := Text("en", tc.source); got != tc.want {
			t.Fatalf("%q -> %q want %q", tc.source, got, tc.want)
		}
		if got := Text("ru", tc.source); got != tc.source {
			t.Fatal("Russian source changed")
		}
	}
	var dictionary map[string]string
	if json.Unmarshal([]byte(CatalogJSON()), &dictionary) != nil || dictionary["Настройки"] != "Settings" {
		t.Fatal("shared catalog invalid")
	}
}

func TestFormattedUITranslationAndUserIdentities(t *testing.T) {
	if got := Text("en", "Открыть: Synthetic Port"); got != "Open: Synthetic Port" {
		t.Fatal(got)
	}
	if got := TextProtected("en", "Система: Настройки · Товар: Тест", []string{"Настройки", "Тест"}); got != "System: Настройки · Commodity: Тест" {
		t.Fatal(got)
	}
	if got := Text("en", "Укажите целое число от 1 до 10 или оставьте поле пустым."); got != "Enter a whole number from 1 to 10, or leave the field blank." {
		t.Fatal(got)
	}
	if got := TextProtected("en", "Название: \ue000 Настройки", []string{"\ue000 Настройки"}); got != "Name: \ue000 Настройки" {
		t.Fatal(got)
	}
}

func TestExplicitLocalesAreIndependentDuringConcurrentSnapshots(t *testing.T) {
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				if T("en", "Система") != "System" || T("ru", "Система") != "Система" {
					t.Error("locale leaked between snapshots")
				}
			}
		}()
	}
	wg.Wait()
}

func BenchmarkComposedUITranslation(b *testing.B) {
	value := strings.Repeat("Тритий: 1 000 т · Свободно: 500 т · ", 4)
	T("en", "Настройки")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		Text("en", value)
	}
}
