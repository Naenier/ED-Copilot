//go:build windows

package nativeui

import (
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"math"
	"unicode/utf8"
)

type navHit struct {
	ID            string
	Bounds, Close walk.Rectangle
}

func inside(r walk.Rectangle, x, y int) bool {
	return x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height
}
func navWidth(id string) int { return min(245, max(105, utf8.RuneCountInString(p.Title(id))*7+48)) }
func (u *Window) scrollTabs(delta int) {
	width := 0
	for _, id := range u.tabs {
		width += navWidth(id) + 3
	}
	u.navOffset = min(max(0, width-max(0, u.nav.ClientBounds().Width-48)), max(0, u.navOffset+delta))
	u.nav.Invalidate()
}
func (u *Window) cycleTab(delta int) {
	for i, id := range u.tabs {
		if id == u.page {
			u.navigate(u.tabs[(i+delta+len(u.tabs))%len(u.tabs)])
			return
		}
	}
}
func (u *Window) keepActiveTabVisible() {
	if u.nav == nil {
		return
	}
	width := u.nav.ClientBounds().Width - 48
	if width < 1 {
		return
	}
	position := 0
	for _, id := range u.tabs {
		w := navWidth(id)
		if id == u.page {
			if position-u.navOffset < 0 {
				u.navOffset = position
			}
			if position+w-u.navOffset > width {
				u.navOffset = position + w - width
			}
			break
		}
		position += w + 3
	}
	u.navOffset = max(0, u.navOffset)
}
func (u *Window) paintNavigation(canvas *walk.Canvas, update walk.Rectangle) error {
	bounds := u.nav.ClientBounds()
	bg, _ := walk.NewSolidColorBrush(color(textColor(u.colors, "bg", "#07101a")))
	defer bg.Dispose()
	_ = canvas.FillRectangle(bg, bounds)
	u.navHits = nil
	x := -u.navOffset
	right := max(0, bounds.Width-43)
	for _, id := range u.tabs {
		width := navWidth(id)
		rect := walk.Rectangle{X: x, Y: 3, Width: width, Height: 40}
		close := walk.Rectangle{X: x + width - 25, Y: 14, Width: 16, Height: 16}
		u.navHits = append(u.navHits, navHit{id, rect, close})
		if x+width >= 0 && x < right {
			drawRect := rect
			if drawRect.X < 0 {
				drawRect.Width += drawRect.X
				drawRect.X = 0
			}
			drawRect.Width = min(drawRect.Width, right-drawRect.X)
			if drawRect.Width > 0 {
				background := "panel"
				if id != u.page {
					background = "bg"
				}
				brush, _ := walk.NewSolidColorBrush(color(textColor(u.colors, background, "#0a1622")))
				_ = canvas.FillRectangle(brush, drawRect)
				brush.Dispose()
				textRect := walk.Rectangle{X: x + 12, Y: 4, Width: width - 43, Height: 36}
				if textRect.X >= 0 && textRect.X+textRect.Width <= right {
					_ = canvas.DrawText(p.Title(id), u.font, color(textColor(u.colors, "text", "#e0e0e0")), textRect, walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
				}
				if id == u.page {
					pen, _ := walk.NewCosmeticPen(walk.PenSolid, color(textColor(u.colors, "accent", "#28a0f0")))
					_ = canvas.DrawLine(pen, walk.Point{X: max(0, x), Y: 43}, walk.Point{X: min(right, x+width), Y: 43})
					pen.Dispose()
				}
				required := id == "overview" || id == "settings"
				if !required && close.X >= 0 && close.X+close.Width <= right {
					pen, _ := walk.NewCosmeticPen(walk.PenSolid, color(textColor(u.colors, "muted", "#8898a8")))
					_ = canvas.DrawLine(pen, walk.Point{X: close.X + 4, Y: close.Y + 4}, walk.Point{X: close.X + 12, Y: close.Y + 12})
					_ = canvas.DrawLine(pen, walk.Point{X: close.X + 12, Y: close.Y + 4}, walk.Point{X: close.X + 4, Y: close.Y + 12})
					pen.Dispose()
				}
			}
		}
		x += width + 3
	}
	plusRect := walk.Rectangle{X: bounds.Width - 40, Y: 3, Width: 38, Height: 40}
	_ = canvas.FillRectangle(bg, plusRect)
	pen, _ := walk.NewCosmeticPen(walk.PenSolid, color(textColor(u.colors, "accent", "#28a0f0")))
	defer pen.Dispose()
	cx, cy := bounds.Width-20, 23
	_ = canvas.DrawLine(pen, walk.Point{X: cx - 5, Y: cy}, walk.Point{X: cx + 5, Y: cy})
	_ = canvas.DrawLine(pen, walk.Point{X: cx, Y: cy - 5}, walk.Point{X: cx, Y: cy + 5})
	return nil
}
func textColor(colors M, key, fallback string) string {
	if s, ok := colors[key].(string); ok && s != "" {
		return s
	}
	return fallback
}
func (u *Window) navDown(x, y int, button walk.MouseButton) {
	if button != walk.LeftButton {
		return
	}
	if x >= u.nav.ClientBounds().Width-43 {
		u.addTabMenu()
		return
	}
	for _, hit := range u.navHits {
		if inside(hit.Bounds, x, y) {
			if inside(hit.Close, x, y) && hit.ID != "overview" && hit.ID != "settings" {
				u.closeTab(hit.ID)
				return
			}
			u.drag = hit.ID
			u.dragX = x
			u.dragged = false
			win.SetCapture(u.nav.Handle())
			u.nav.SetFocus()
			return
		}
	}
}
func (u *Window) navMove(x, y int, button walk.MouseButton) {
	if u.drag != "" && math.Abs(float64(x-u.dragX)) > 7 {
		u.dragged = true
		u.nav.Invalidate()
	}
}
func (u *Window) navUp(x, y int, button walk.MouseButton) {
	if button != walk.LeftButton || u.drag == "" {
		return
	}
	win.ReleaseCapture()
	id := u.drag
	u.drag = ""
	if !u.dragged {
		u.navigate(id)
		return
	}
	next := []string{}
	for _, tab := range u.tabs {
		if tab != id {
			next = append(next, tab)
		}
	}
	index := len(next)
	for _, hit := range u.navHits {
		if hit.ID == id {
			continue
		}
		if x < hit.Bounds.X+hit.Bounds.Width/2 {
			for i, key := range next {
				if key == hit.ID {
					index = i
					break
				}
			}
			break
		}
	}
	next = append(next, "")
	copy(next[index+1:], next[index:])
	next[index] = id
	u.tabs = next
	u.persistTabs()
	u.keepActiveTabVisible()
}
func (u *Window) closeTab(id string) {
	if id == "overview" || id == "settings" {
		return
	}
	index := -1
	next := []string{}
	for i, tab := range u.tabs {
		if tab == id {
			index = i
		} else {
			next = append(next, tab)
		}
	}
	if index < 0 {
		return
	}
	if u.page == id {
		u.page = next[min(index, len(next)-1)]
	}
	u.tabs = next
	u.persistTabs()
	u.renderPage(true)
}
func (u *Window) addTabMenu() {
	actions := []p.Action{}
	opened := map[string]bool{}
	for _, id := range u.tabs {
		opened[id] = true
	}
	for _, tab := range p.Tabs {
		if !opened[tab.ID] {
			actions = append(actions, p.Action{ID: "tab." + tab.ID, Title: tab.Title, Kind: "navigate", Path: tab.ID})
		}
	}
	// An overflowing strip still gives access to every already-open section.
	width := 0
	for _, id := range u.tabs {
		width += navWidth(id) + 3
	}
	if width > u.nav.ClientBounds().Width-48 {
		for _, id := range u.tabs {
			actions = append(actions, p.Action{ID: "switch." + id, Title: "Перейти: " + p.Title(id), Kind: "navigate", Path: id})
		}
	}
	actions = append(actions, p.Action{ID: "tabs.reset", Title: "Восстановить основные вкладки", Kind: "tabs-reset"})
	u.popupActions(actions)
}
