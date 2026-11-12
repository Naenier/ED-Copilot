//go:build windows

package nativeui

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	p "github.com/Naenier/ED-Copilot/internal/presentation"
	"github.com/lxn/walk"
)

func TestNativeHUDStartsWithWorkingPanelAndReclaimsHeaderHeight(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer nativeTestControls(t)()
	main, err := walk.NewMainWindow()
	if err != nil {
		t.Fatal(err)
	}
	defer main.Dispose()
	state := M{"settings": M{"activity": "flight"}, "connection": M{"gameRunning": false}, "commander": M{"system": "Alpha", "vehicle": "ship"}, "pilot": M{"navTarget": M{"system": "Beta", "starClass": "G"}, "fuel": M{"mainTons": 12, "mainCapacityTons": 16}}}
	h, err := newHUD(&Window{main: main, state: state, backend: &platformController{state: state}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.close()
	h.setFonts(96)
	h.document = p.BuildHUD(state, time.Now())
	view := p.HUDRect{Width: 1920, Height: 1080}
	compact := h.arrange(view, "foot", 96, true)
	if len(h.document.Sections) != 2 || len(compact.Panels) != 2 || compact.Hidden != 0 || compact.Panels[0].Bounds.Y != compact.Bands[0].Y {
		t.Fatal("working content lost or phantom header space remains", compact, h.document)
	}
	for _, section := range h.document.Sections {
		if strings.Contains(strings.ToUpper(section.Title), "ED COPILOT") {
			t.Fatal("branding panel remains", section)
		}
	}
	// Compare actual GDI measurements with the former independent header.
	h.document.Sections = append([]p.HUDSection{{Title: "ED COPILOT · ПОЛЁТ И НАВИГАЦИЯ", Lines: []p.HUDLine{{Text: "Alpha", Role: "muted"}}}}, h.document.Sections...)
	legacy := h.arrange(view, "foot", 96, true)
	if legacy.Bounds[0].Height-compact.Bounds[0].Height < 40 {
		t.Fatal("removing branding did not reclaim a whole header panel", compact, legacy)
	}
	h.document = p.BuildHUD(M{"settings": M{"activity": "flight"}, "commander": M{"system": "Alpha"}}, time.Now())
	idle := h.arrange(view, "ship", 96, true)
	if len(idle.Panels) != 0 || idle.Bounds != ([2]p.HUDRect{}) || idle.Hidden != 0 {
		t.Fatal("idle HUD reserves an empty window", idle)
	}
}

func TestNativeHUDCartographyPagesContainAllTargetsWithinSafeBands(t *testing.T) {
	h := &hudWindow{dpi: 96}
	view := p.HUDRect{Width: 1280, Height: 720}
	bands := p.HUDSafeBands(view, "ship", h.dpi)
	targets := p.HUDSection{Kind: "cartography-targets", Title: "CARTOGRAPHY", Subtitle: "Alpha"}
	measured := hudSectionMetrics{ValueWidth: 90}
	for i := 1; i <= 40; i++ {
		targets.Lines = append(targets.Lines, p.HUDLine{Text: fmt.Sprintf("A %d", i), Value: "≈ 1 250 000 Cr"})
		measured.Lines = append(measured.Lines, 17)
	}
	route := p.HUDSection{Kind: "navigation", Title: "CARTOGRAPHY ROUTE"}
	sections := []p.HUDSection{targets, route}
	metrics := []hudSectionMetrics{measured, {Height: 76}}

	fewTargets := targets
	fewTargets.Lines = fewTargets.Lines[:10]
	fewMeasured := measured
	fewMeasured.Lines = fewMeasured.Lines[:10]
	frames := h.cartographyFrames(view, "ship", h.dpi, bands[0].Height, []p.HUDSection{fewTargets, route}, []hudSectionMetrics{fewMeasured, metrics[1]})
	if len(frames) != 1 || len(frames[0].sections) != 3 || frames[0].sections[0].Title != targets.Title ||
		len(frames[0].sections[0].Lines)+len(frames[0].sections[1].Lines) != len(fewTargets.Lines) {
		t.Fatalf("targets that fit across two sides should stay visible simultaneously: %+v", frames)
	}

	frames = h.cartographyFrames(view, "ship", h.dpi, bands[0].Height, sections, metrics)
	if len(frames) < 2 {
		t.Fatalf("long list should paginate: %+v", frames)
	}
	first := time.Unix(0, 0)
	h.cartography = hudCartographyPager{frames: frames, started: first}
	targetIndex := 0
	for i := range frames {
		layout := h.cartographyFrame(first.Add(time.Duration(i) * hudCartographyPageInterval))
		if h.cartography.index != i || layout.Hidden != 0 || len(layout.Panels) != len(h.renderSections) {
			t.Fatalf("invalid cartography page %d: %+v", i, layout)
		}
		for _, panel := range layout.Panels {
			band := bands[panel.Column]
			if panel.Clipped || panel.Bounds.Y < band.Y || panel.Bounds.Y+panel.Bounds.Height > band.Y+band.Height {
				t.Fatalf("panel escaped the safe band on page %d: %+v", i, panel)
			}
			section := h.renderSections[panel.Index]
			if section.Kind == route.Kind {
				continue
			}
			if want := fmt.Sprintf("CARTOGRAPHY · %d/%d", i+1, len(frames)); section.Title != want {
				t.Fatalf("page cue got %q, want %q", section.Title, want)
			}
			for _, row := range section.Lines {
				if targetIndex >= len(targets.Lines) || row != targets.Lines[targetIndex] {
					t.Fatalf("lost target order at %d on page %d: %+v", targetIndex, i, row)
				}
				targetIndex++
			}
		}
	}
	if targetIndex != len(targets.Lines) {
		t.Fatalf("rendered %d of %d targets", targetIndex, len(targets.Lines))
	}
	h.cartographyFrame(first.Add(time.Duration(len(frames)) * hudCartographyPageInterval))
	if h.cartography.index != 0 {
		t.Fatal("cartography paging did not loop")
	}
}
