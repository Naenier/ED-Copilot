package localization

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestDomainErrorTranslationsPreserveFormattingAndInputIdentities(t *testing.T) {
	body, err := files.ReadFile("domain_errors_en.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries map[string]string
	if err = json.Unmarshal(body, &entries); err != nil {
		t.Fatal(err)
	}
	format := regexp.MustCompile(`%(?:\[[0-9]+\])?[-+# 0]*(?:\d+|\*)?(?:\.(?:\d+|\*))?[a-zA-Z%]`)
	for source, target := range entries {
		a, b := format.FindAllString(source, -1), format.FindAllString(target, -1)
		if strings.Join(a, "|") != strings.Join(b, "|") {
			t.Errorf("format arguments changed for %q: %v -> %v", source, a, b)
		}
		if containsCyrillic(target) || strings.TrimSpace(target) == "" {
			t.Errorf("invalid English error translation for %q", source)
		}
	}
	const identity = "Платина"
	raw := fmt.Sprintf("Система «%s» не найдена в Spansh", identity)
	got := TextProtected("en", raw, []string{identity})
	if got != "System ‘Платина’ was not found in Spansh" {
		t.Fatal("rendered error did not translate its template while protecting the system name", got)
	}
	if TextProtected("ru", raw, []string{identity}) != raw {
		t.Fatal("Russian error was altered")
	}
	if T("en", "Сырьевые материалы не продаются на товарном рынке.") != "Raw engineering materials cannot be sold on the commodity market." {
		t.Fatal("literal domain error was not covered")
	}
}

func TestUITranslationDictionariesHaveConsistentDuplicateKeys(t *testing.T) {
	all, err := files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	seen, origins := map[string]string{}, map[string]string{}
	for _, file := range all {
		body, err := files.ReadFile(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		var entries map[string]string
		if err = json.Unmarshal(body, &entries); err != nil {
			t.Fatal(err)
		}
		for source, target := range entries {
			if previous, ok := seen[source]; ok && previous != target {
				t.Errorf("conflicting translation %q: %s=%q, %s=%q", source, origins[source], previous, file.Name(), target)
			}
			seen[source], origins[source] = target, file.Name()
		}
	}
}
