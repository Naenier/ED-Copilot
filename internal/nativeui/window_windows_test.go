//go:build windows

package nativeui

import (
	"runtime"
	"testing"

	p "github.com/Naenier/ED-Copilot/internal/presentation"
	"github.com/lxn/win"
)

func TestHiddenNativeShellResolvedPaletteAndStableLiveTable(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer nativeTestControls(t)()
	state := M{
		"app": M{"version": "2.0.0"}, "navigation": M{"tabs": p.DefaultTabs},
		"commander":  M{"ship": "Panther Clipper Mk II", "shipName": "Звёздный попутчик", "system": "Тестовая система"},
		"settings":   M{"overlayEnabled": false, "activity": "flight"},
		"theme":      M{"palette": M{"bg": "#101820", "panel": "#15202b", "text": "#eeeeee", "accent": "#f09050", "muted": "#8899aa", "overlayOpacity": 0.82}, "settings": M{"colors": M{"accent": "#0000ff"}}},
		"objectives": []M{{"id": "synthetic-1", "title": "Первая задача", "status": "active"}, {"id": "synthetic-2", "title": "Вторая задача", "status": "active"}},
	}
	backend := &platformController{state: state}
	u, err := NewWindow(backend, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer u.dispose()
	if win.IsWindowVisible(u.main.Handle()) {
		t.Fatal("hidden test exposed a window")
	}
	if u.colors["accent"] != "#f09050" {
		t.Fatal("window ignored resolved matrix/game palette", u.colors)
	}
	u.navigate("targets")
	if len(u.model.rows) < 2 {
		t.Fatal("native rows were not bound")
	}
	_ = u.table.SetCurrentIndex(1)
	resets := 0
	brushes := len(u.brushes)
	u.model.RowsReset().Attach(func() { resets++ })
	for range 3 {
		u.renderPage(false)
		u.applyTheme()
	}
	if resets != 0 || u.table.CurrentIndex() != 1 {
		t.Fatal("unchanged live polling reset the table", resets, u.table.CurrentIndex())
	}
	if len(u.brushes) != brushes {
		t.Fatal("unchanged palette leaked GDI brushes", brushes, len(u.brushes))
	}
	if u.activityRow.Visible() {
		t.Fatal("disabled overlay kept activity controls visible")
	}
	u.tabs = nil
	for _, tab := range p.Tabs {
		u.tabs = append(u.tabs, tab.ID)
	}
	u.scrollTabs(100000)
	if u.navOffset <= 0 {
		t.Fatal("overflowing tabs cannot be scrolled")
	}
	u.scrollTabs(-100000)
	if u.navOffset != 0 {
		t.Fatal("tab scroll failed to clamp")
	}
	u.page = u.tabs[0]
	u.cycleTab(-1)
	if u.page != u.tabs[len(u.tabs)-1] {
		t.Fatal("keyboard tab navigation failed to wrap")
	}
}
