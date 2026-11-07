package theme

import (
	v "github.com/Naenier/ED-Copilot/internal/value"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPaletteValidationAndMatrixOrientation(t *testing.T) {
	for _, bad := range []v.M{{"mode": "unknown"}, {"preset": "unknown"}, {"colors": v.M{"text": "red"}}, {"overlayOpacity": 0.3}, {"overlayOpacity": 1.1}, {"matrix": [][]float64{{1, 0, 0}, {0, 1, 0}}}, {"matrix": [][]float64{{3, 0, 0}, {0, 1, 0}, {0, 0, 1}}}} {
		if _, err := Normalize(bad); err == nil {
			t.Fatal("invalid theme accepted", bad)
		}
	}
	got := Transform("#ff7f00", [][]float64{{0, 1, 0}, {0, 0, 1}, {1, 0, 0}})
	if got != "#00ff7f" {
		t.Fatal("input-row orientation", got)
	}
	palette, err := Palette(v.M{"mode": "matrix", "matrix": [][]float64{{0, 1, 0}, {0, 0, 1}, {1, 0, 0}}})
	if err != nil || palette["accent"] != "#00ff7f" {
		t.Fatal(palette, err)
	}
	for _, key := range append(ColorKeys, "accentBright", "accentDim", "faint", "panelRaised", "line") {
		if len(v.String(palette, key)) != 7 {
			t.Fatal("missing palette role", key)
		}
	}
	presets := Presets()
	v.Obj(presets[0]["colors"])["accent"] = "#000000"
	if v.Str(v.At(Presets()[0], "colors", "accent")) == "#000000" {
		t.Fatal("caller mutated preset globals")
	}
}
func TestXMLAndINIParsingIgnoresExecutableDirectives(t *testing.T) {
	raw := `<GraphicsConfig><GUIColour><Default><MatrixRed>0,1,0</MatrixRed><MatrixGreen>0,0,1</MatrixGreen><MatrixBlue>1,0,0</MatrixBlue></Default><Alternate><MatrixRed>1,0,0</MatrixRed></Alternate></GUIColour></GraphicsConfig>`
	m, err := ParseXML(raw)
	if err != nil || Transform("#ff7f00", m) != "#00ff7f" {
		t.Fatal(m, err)
	}
	if _, err = ParseXML(`<!DOCTYPE config [<!ENTITY external SYSTEM "file:///secret">]><MatrixRed>1,0,0</MatrixRed>`); err == nil {
		t.Fatal("external entity accepted")
	}
	if _, err = ParseXML(`<MatrixRed>1,0,0</MatrixRed>`); err == nil {
		t.Fatal("partial matrix accepted")
	}
	m, err = ParseEDHMMatrix("[Constants]\n x150=0\ny150=1\nz150=0\nx151=0\ny151=0\nz151=1\nx152=1\ny152=0\nz152=0\n[CommandList]\nrun=malicious.exe\n")
	if err != nil || Transform("#ff7f00", m) != "#00ff7f" {
		t.Fatal(m, err)
	}
	if _, err = ParseEDHMMatrix("[Constants]\nx150=1\n"); err == nil {
		t.Fatal("partial INI matrix accepted")
	}
	if m, err = ParseEDHMMatrix("[CommandList]\nx150=1\n"); err != nil || m != nil {
		t.Fatal("nonconstant directives read", err)
	}
}
func TestEDHMGammaRoleImportAndReadOnlyFiles(t *testing.T) {
	dir := t.TempDir()
	game := filepath.Join(dir, "Game")
	profiles := filepath.Join(game, "EDHM-ini")
	os.MkdirAll(profiles, 0700)
	files := map[string]string{"XML-Profile.ini": "[Constants]\nx150=1\ny150=0\nz150=0\nx151=0\ny151=1\nz151=0\nx152=0\ny152=0\nz152=1\n", "Startup-Profile.ini": "[Constants]\nw206=100\nw75=100\ny104=100\ny139=100\n", "Advanced.ini": "[Constants]\nx208=0.5\ny208=0.1\nz208=0\nx77=1\ny77=1\nz77=1\nx172=1\ny172=0.5\nz172=0\nx246=0\ny246=1\nz246=0\nx247=1\ny247=0\nz247=0\n", "ThemeSettings.json": `{"credits":{"theme":"Synthetic ED Theme"}}`}
	for name, body := range files {
		os.WriteFile(filepath.Join(profiles, name), []byte(body), 0600)
	}
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	result, err := Import(ImportOptions{HomeDir: dir, GameDir: game, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if v.Str(v.At(result, "settings", "colors", "accent")) != "#bc5900" || v.Str(v.At(result, "settings", "colors", "text")) != "#ffffff" || v.Str(v.At(result, "source", "name")) != "Synthetic ED Theme" {
		t.Fatal("EDHM roles/gamma", result)
	}
	if v.Str(v.At(result, "source", "importedAt")) != v.ISO(now) {
		t.Fatal("import time")
	}
	for name, want := range files {
		b, _ := os.ReadFile(filepath.Join(profiles, name))
		if string(b) != want {
			t.Fatal("import changed game files", name)
		}
	}
	explicit, err := ReadGamePalette(filepath.Join(profiles, "Advanced.ini"))
	if err != nil || v.At(explicit, "source", "kind") != "edhm" {
		t.Fatal("explicit active file import", err)
	}
	if _, err = ReadGamePalette(filepath.Join(profiles, "run.exe")); err == nil {
		t.Fatal("unsupported file accepted")
	}
	if n := LinearToSRGB(0.5); n < 0.735 || n > 0.736 {
		t.Fatal("linear gamma", n)
	}
}
func TestStandardGraphicsFallbackAndAbsentConfig(t *testing.T) {
	dir := t.TempDir()
	game := filepath.Join(dir, "Game")
	os.MkdirAll(game, 0700)
	file := filepath.Join(game, "GraphicsConfiguration.xml")
	xml := `<GUIColour><Default><MatrixRed>1,0,0</MatrixRed><MatrixGreen>0,1,0</MatrixGreen><MatrixBlue>0,0,1</MatrixBlue></Default></GUIColour>`
	os.WriteFile(file, []byte(xml), 0600)
	result, err := Import(ImportOptions{HomeDir: dir, GameDir: game})
	if err != nil || v.At(result, "source", "kind") != "xml" {
		t.Fatal(result, err)
	}
	if v.Str(v.At(result, "settings", "colors", "accent")) != "#ff7f00" {
		t.Fatal("identity accent")
	}
	os.Remove(file)
	if _, err = Import(ImportOptions{HomeDir: dir, GameDir: game}); err == nil || !strings.Contains(err.Error(), "не найдена") {
		t.Fatal("missing palette", err)
	}
}
