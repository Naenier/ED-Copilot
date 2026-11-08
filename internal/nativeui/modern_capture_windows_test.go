//go:build windows

package nativeui

import (
	"image"
	imagecolor "image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestModernPreviewRejectsBlankCorruptAndMissingImages(t *testing.T) {
	dir := t.TempDir()
	blank := filepath.Join(dir, "blank.png")
	f, err := os.Create(blank)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 400, 320))
	for y := 0; y < 320; y++ {
		for x := 0; x < 400; x++ {
			img.SetRGBA(x, y, imagecolor.RGBA{25, 25, 25, 255})
		}
	}
	if err = png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	corrupt := filepath.Join(dir, "corrupt.png")
	if err = os.WriteFile(corrupt, []byte("not a PNG"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{blank, corrupt, filepath.Join(dir, "missing.png")} {
		if _, err := inspectModernPreview(path); err == nil {
			t.Fatal("invalid screenshot accepted", filepath.Base(path))
		}
	}
	for y := 0; y < 320; y++ {
		for x := 0; x < 400; x++ {
			img.SetRGBA(x, y, imagecolor.RGBA{uint8(x), uint8(y), uint8(x + y), 255})
		}
	}
	valid := filepath.Join(dir, "rendered.png")
	f, err = os.Create(valid)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	result, err := inspectModernPreview(valid)
	if err != nil || !v.Bool(result["ok"]) || result["width"] != 400 || result["height"] != 320 {
		t.Fatal(result, err)
	}
}

func TestModernPreviewIsRestrictedAndMissingCapturesFailSmoke(t *testing.T) {
	for _, options := range []Options{{}, {Smoke: true}, {Smoke: true, ReadmeScreenshots: true}} {
		m := &modernShell{owner: &Window{options: options}}
		for _, name := range []string{"../arbitrary", `C:\outside`, "", "overview"} {
			// A shell with no initialized WebView must never create a file.
			if err := m.capturePreview("test", name); err == nil {
				t.Fatal("invalid or unready capture accepted", options, name)
			}
		}
		for _, raw := range []any{nil, "true", 900, M{"width": 900}} {
			if _, err := m.smokeViewport(raw); err == nil {
				t.Fatal("untyped viewport request accepted", options, raw)
			}
		}
		if !options.Smoke {
			if _, err := m.smokeViewport(true); err == nil {
				t.Fatal("production viewport request accepted")
			}
		}
	}
	report := M{"ok": true}
	m := &modernShell{}
	m.smokeVisualState(report)
	if v.Bool(report["ok"]) || len(v.Arr(report["problems"])) != 2 || len(v.Rows(report["screenshots"])) != 0 {
		t.Fatal("missing capture/host geometry was accepted", report)
	}
}
