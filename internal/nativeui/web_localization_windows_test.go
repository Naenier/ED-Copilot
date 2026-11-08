//go:build windows

package nativeui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/localization"
)

func TestLocalizedErrorKeepsRequestSystemAndUserName(t *testing.T) {
	input := M{"values": M{"sourceSystem": "Платина", "name": "Настройки", "apiKey": "sensitive-value", "language": "en"}}
	identities := webInputIdentities(input)
	for _, s := range identities {
		if s == "sensitive-value" || s == "en" {
			t.Fatal("non-identity added to error protection")
		}
	}
	translated := localization.TextProtected("en", "Система не найдена: Платина", identities)
	if !strings.Contains(translated, "Платина") || strings.Contains(translated, "Система не найдена") {
		t.Fatal("error caption or system name mistranslated", translated)
	}
	many := M{}
	for i := 0; i < 100; i++ {
		many[fmt.Sprintf("x%dSystem", i)] = "Система"
	}
	if len(webInputIdentities(many)) > 64 {
		t.Fatal("unbounded error identity mask")
	}
}
