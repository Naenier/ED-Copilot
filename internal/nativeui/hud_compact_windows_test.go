//go:build windows

package nativeui

import (
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
