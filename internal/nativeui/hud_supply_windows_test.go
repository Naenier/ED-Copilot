//go:build windows

package nativeui

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	p "github.com/Naenier/ED-Copilot/internal/presentation"
	"github.com/lxn/walk"
	"github.com/lxn/win"
)

func TestNativeColonySupplyPreviewFitsSafeBandsAtNormalFont(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer nativeTestControls(t)()
	main, err := walk.NewMainWindow()
	if err != nil {
		t.Fatal(err)
	}
	defer main.Dispose()
	supplies := []M{
		{"commodity": "steel", "name": "Сталь", "missing": 10164, "cargoAvailable": 0, "carrierIncluded": true, "carrierAvailable": 12000, "carrierComplete": true, "toBuy": 0, "authoritative": true},
		{"commodity": "aluminium", "name": "Алюминий", "missing": 7047, "cargoAvailable": 1232, "carrierIncluded": true, "carrierAvailable": 0, "carrierComplete": true, "toBuy": 5815, "authoritative": true},
		{"commodity": "ceramiccomposites", "name": "СММ-композит", "missing": 6776, "cargoAvailable": 0, "carrierIncluded": true, "carrierAvailable": 1000, "carrierComplete": true, "toBuy": 5776, "authoritative": true},
	}
	for i := range 23 {
		supplies = append(supplies, M{"commodity": fmt.Sprintf("synthetic%d", i), "name": fmt.Sprintf("Resource %02d", i), "missing": 100, "cargoAvailable": 0, "carrierIncluded": true, "carrierAvailable": 0, "carrierComplete": true, "toBuy": 100})
	}
	state := M{"settings": M{"activity": "colonization"}, "connection": M{"gameRunning": false}, "commander": M{"system": "Synthetic Origin", "station": "Synthetic Orbital Port", "docked": true, "vehicle": "ship"}, "colonization": M{"includeCarrierWarehouse": true, "activeProject": M{"name": "Synthetic Colony", "status": "building", "operationalSupplies": supplies}}}
	h, err := newHUD(&Window{main: main, state: state, backend: &platformController{state: state}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.close()
	for _, language := range []string{"ru", "en"} {
		state["settings"].(map[string]any)["language"] = language
		h.document = p.BuildHUD(state, time.Now())
		for _, dpi := range []int{96, 144} {
			h.setFonts(dpi)
			view := p.HUDRect{Width: 1920, Height: 1080}
			plan := h.arrange(view, "ship", dpi, true)
			if len(plan.Panels) != 2 || plan.Hidden != 0 {
				t.Fatal("a 26-commodity bill expanded or hid the compact two-panel HUD", language, dpi, plan)
			}
			for _, panel := range plan.Panels {
				if panel.Clipped {
					t.Fatal("normal-font supply preview was truncated", language, dpi, panel)
				}
				band := plan.Bands[panel.Column]
				if panel.Bounds.X < band.X || panel.Bounds.Y < band.Y || panel.Bounds.X+panel.Bounds.Width > band.X+band.Width || panel.Bounds.Y+panel.Bounds.Height > band.Y+band.Height {
					t.Fatal("measured text escaped the cockpit-safe band", language, dpi, panel, band)
				}
			}
			text := ""
			for _, section := range h.document.Sections {
				for _, line := range section.Lines {
					text += line.Text + "\n"
				}
			}
			if strings.Contains(text, "12 000") || strings.Count(text, ": купить ")+strings.Count(text, ": buy ") != 10 || !strings.Contains(text, "15") || !strings.Contains(text, "1/26") {
				t.Fatal("ten uncovered resources or the exact remainder was lost", text)
			}
		}
	}
}

func TestNativeTenLongColonyActionsRebalanceAtActualGDIScale(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer nativeTestControls(t)()
	main, err := walk.NewMainWindow()
	if err != nil {
		t.Fatal(err)
	}
	defer main.Dispose()
	names := []string{"Титан", "СММ-композит", "Жидкий кислород", "Керамокомпозиты", "Конструкционные регуляторы", "Полимеры", "Стабилизаторы поверхности", "Медь", "Строительные синтезаторы", "Эвакуационное убежище"}
	supplies := []M{}
	for i, name := range names {
		supplies = append(supplies, M{"commodity": fmt.Sprintf("longsynthetic%d", i), "name": name, "missing": 6776, "cargoAvailable": 0, "carrierIncluded": true, "carrierAvailable": 0, "carrierComplete": true, "toBuy": 6776, "authoritative": true})
	}
	state := M{"settings": M{"activity": "colonization"}, "connection": M{"gameRunning": false}, "commander": M{"system": "Synthetic Colony", "vehicle": "ship"}, "colonization": M{"includeCarrierWarehouse": true, "activeProject": M{"name": "Synthetic Colony", "status": "building", "operationalSupplies": supplies}}}
	h, err := newHUD(&Window{main: main, state: state, backend: &platformController{state: state}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.close()
	for _, language := range []string{"ru", "en"} {
		state["settings"].(map[string]any)["language"] = language
		h.document = p.BuildHUD(state, time.Now())
		original, _ := json.Marshal(h.document)
		if h.document.ColonySupplyActions != 10 || h.document.ColonySupplyContext != 1 {
			t.Fatal("bounded action/context metadata lost", h.document)
		}
		for _, dpi := range []int{96, 144} {
			h.setFonts(dpi)
			view := p.HUDRect{X: -1920, Y: 100, Width: 1920, Height: 1080}
			dc := win.GetDC(h.hwnd)
			heights := []int{}
			for _, section := range h.document.Sections {
				heights = append(heights, h.measureSection(dc, p.HUDGeometry(view, "ship", dpi, 0).Width, section).Height)
			}
			win.ReleaseDC(h.hwnd, dc)
			fixed := p.LayoutHUDSections(view, "ship", dpi, heights)
			plan := h.arrange(view, "ship", dpi, true)
			t.Log(language, dpi, "sourceheights", heights, "metrics", h.metrics)
			if len(plan.Panels) != 2 || plan.Hidden != 0 || plan.Panels[0].Column != 0 {
				t.Fatal("ten actions were hidden or right priority lost", language, dpi, plan)
			}
			for _, panel := range plan.Panels {
				band := plan.Bands[panel.Column]
				if panel.Clipped || panel.Bounds.X < band.X || panel.Bounds.Y < band.Y || panel.Bounds.X+panel.Bounds.Width > band.X+band.Width || panel.Bounds.Y+panel.Bounds.Height > band.Y+band.Height {
					t.Fatal("long commodity action clipped or covered cockpit instruments", language, dpi, panel, band)
				}
			}
			if len(h.renderSections) != 2 || len(h.metrics) != 2 {
				t.Fatal("paint sections do not match measured layout")
			}
			flatten := func(sections []p.HUDSection) string {
				text := ""
				for _, section := range sections {
					for _, line := range section.Lines {
						text += line.Text + "\n"
					}
				}
				return text
			}
			if flatten(h.renderSections) != flatten(h.document.Sections) {
				t.Fatal("rebalancing lost, duplicated or reordered context/actions/footer", language, dpi, h.renderSections)
			}
			after, _ := json.Marshal(h.document)
			if string(after) != string(original) {
				t.Fatal("native fitting mutated source document")
			}
			fixedClipped := false
			for _, panel := range fixed.Panels {
				fixedClipped = fixedClipped || panel.Clipped
			}
			if fixedClipped && len(h.renderSections[0].Lines) == len(h.document.Sections[0].Lines) {
				t.Fatal("measured clipping did not rebalance action boundary", language, dpi)
			}
			t.Log(language, dpi, "fixed", heights, "balanced", h.metrics[0].Height, h.metrics[1].Height, "first actions", len(h.renderSections[0].Lines)-h.document.ColonySupplyContext)
		}
	}
}
