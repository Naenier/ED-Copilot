package presentation

import (
	"strings"
	"testing"
)

func TestEverySectionHasExplicitEnglishHelpAndRussianRemainsOriginal(t *testing.T) {
	for _, tab := range Tabs {
		t.Run(tab.ID, func(t *testing.T) {
			english := HelpForLanguage(tab.ID, "en")
			if len(english) < 160 || referenceHasCyrillic(english) || englishSectionHelp[tab.ID] == "" {
				t.Fatal("missing or mixed-language section help")
			}
			if HelpForLanguage(tab.ID, "ru") != HelpFor(tab.ID) {
				t.Fatal("English help changed existing Russian guidance")
			}
			for _, developerInstruction := range []string{"Client ID", "client_id", "Developer Zone", "Shared key", "redirect_uri"} {
				if strings.Contains(english, developerInstruction) {
					t.Fatal("developer setup leaked into normal user help")
				}
			}
		})
	}
	if HelpForLanguage("unknown", "en") == "" || referenceHasCyrillic(HelpForLanguage("unknown", "en")) {
		t.Fatal("missing English fallback")
	}
}
