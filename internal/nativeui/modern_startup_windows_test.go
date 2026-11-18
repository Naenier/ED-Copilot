//go:build windows

package nativeui

import (
	"testing"

	"github.com/Naenier/ED-Copilot/internal/app"
)

func BenchmarkWebBootstrapFirstView(b *testing.B) {
	engine, err := app.New(app.Options{DataDir: b.TempDir(), JournalDir: b.TempDir(), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(engine.Close)
	for _, lang := range []string{"ru", "en"} {
		if _, err := engine.Do("/api/settings", M{"language": lang}); err != nil {
			b.Fatal(err)
		}
		b.Run(lang, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				boot, err := dispatchWeb(engine, "boot", nil)
				if err != nil {
					b.Fatal(err)
				}
				if boot["view"] == nil {
					if _, err := dispatchWeb(engine, "view", nil); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

func BenchmarkNativeWindowSnapshot(b *testing.B) {
	engine, err := app.New(app.Options{DataDir: b.TempDir(), JournalDir: b.TempDir(), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(engine.Close)
	if _, err := engine.Do("/api/settings", M{"overlayEnabled": false}); err != nil {
		b.Fatal(err)
	}
	for _, projection := range []struct {
		name     string
		snapshot func(Controller) M
	}{
		{"hud", nativeHUDSnapshot},
		{"window", nativeWindowSnapshot},
		{"idleRefresh", nativeRefreshSnapshot},
	} {
		b.Run(projection.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				projection.snapshot(engine)
			}
		})
	}
}
