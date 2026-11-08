//go:build windows

package nativeui

import (
	"path/filepath"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/app"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestCommanderDomainsReachActualWebBridgeWithoutUnrelatedCatalogues(t *testing.T) {
	root := t.TempDir()
	e, err := app.New(app.Options{DataDir: filepath.Join(root, "profile"), JournalDir: filepath.Join(root, "journals"), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	e.Apply(M{"event": "Commander", "timestamp": "2026-11-08T12:00:00Z", "Name": "Demo Commander", "FID": "FNATIVE-SYNTHETIC"}, nil)
	if err := e.SeedDemoCommanderAndPolitics(); err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"en", "ru"} {
		if _, err := dispatchWeb(e, "settings", M{"values": M{"language": language}}); err != nil {
			t.Fatal(err)
		}
		profile, err := dispatchWeb(e, "view", M{"tab": "commander"})
		if err != nil || v.At(profile, "state", "commanderProfile", "commanderFID") != "FNATIVE-SYNTHETIC" || v.Num(v.At(profile, "state", "commanderProfile", "finance", "netWorth")) != 8734000000 {
			t.Fatal("commander observations were removed at the WebView boundary", err)
		}
		if len(v.Rows(v.At(profile, "state", "commanderProfile", "fleet", "entries"))) != 3 || len(v.Rows(v.At(profile, "state", "commanderProfile", "statistics", "sections"))) < 4 {
			t.Fatal("fleet and game statistics did not reach the actual renderer")
		}
		if v.At(profile, "state", "engineeringInventory") == nil || v.At(profile, "state", "coloniesSelectors") == nil || v.At(profile, "state", "world") != nil {
			t.Fatal("commander did not receive its lightweight supporting domains")
		}
		ranks, err := dispatchWeb(e, "view", M{"tab": "ranks"})
		if err != nil || len(v.Rows(v.At(ranks, "state", "ranks", "disciplines"))) != 8 {
			t.Fatal("rank data was removed at the WebView boundary", err)
		}
		if v.At(ranks, "state", "politics") != nil {
			t.Fatal("political cache was loaded for ranks")
		}
		politics, err := dispatchWeb(e, "view", M{"tab": "politics"})
		if err != nil || v.At(politics, "state", "politics", "activeSystem", "name") != "Demo Political System" {
			t.Fatal("political observations were removed at the WebView boundary", err)
		}
		if v.At(politics, "state", "updates", "version") != app.Version {
			t.Fatal("update status missing from desktop view")
		}
		if v.At(politics, "state", "ranks") != nil {
			t.Fatal("full rank catalogue was loaded for politics")
		}
	}
	if _, err := dispatchWeb(e, "politics", M{"values": M{"action": "select-system", "system": "Demo Political System"}}); err != nil {
		t.Fatal("politics selection route unavailable", err)
	}
	if _, err := dispatchWeb(e, "updates-check", nil); err == nil {
		t.Fatal("isolated bridge accepted real GitHub update check")
	}
	if _, err := dispatchWeb(e, "settings", M{"values": M{"autoUpdateEnabled": false}}); err != nil {
		t.Fatal(err)
	}
	settings, err := dispatchWeb(e, "view", M{"tab": "settings"})
	if err != nil || v.At(settings, "state", "updates", "enabled") != false {
		t.Fatal("update opt-out not visible to WebView", err)
	}
}
