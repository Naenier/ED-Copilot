//go:build windows

package nativeui

import (
	"fmt"
	"math"
	"reflect"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/win"
)

const overlayHotKeyID = 0x4544

var (
	hudClassOnce        sync.Once
	hudClassError       error
	hudClassName        = wide("EDCopilotNativeHUD20")
	hudWindows          = map[win.HWND]*hudWindow{}
	mainHUDWindows      = map[win.HWND]*hudWindow{}
	hudProcedure        = syscall.NewCallback(hudWndProc)
	mainHUDProcedure    = syscall.NewCallback(mainHUDWndProc)
	foregroundProcedure = syscall.NewCallback(hudForegroundEvent)
	fillRectProc        = userDLL.NewProc("FillRect")
)

type hudWindow struct {
	owner                         *Window
	hwnd                          win.HWND
	mainProcedure                 uintptr
	foregroundHook                uintptr
	hotkeyRegistered              bool
	closed, visible               bool
	modeApplied, restoreMaximized bool
	document                      p.HUDDocument
	colors                        M
	dpi                           int
	regular, bold                 win.HFONT
	key                           win.COLORREF
	alpha                         byte
	bounds                        p.HUDRect
}

func newHUD(u *Window) (*hudWindow, error) {
	hudClassOnce.Do(func() {
		class := win.WNDCLASSEX{CbSize: uint32(unsafe.Sizeof(win.WNDCLASSEX{})), LpfnWndProc: hudProcedure, HInstance: win.GetModuleHandle(nil), LpszClassName: hudClassName}
		if win.RegisterClassEx(&class) == 0 {
			hudClassError = fmt.Errorf("Не удалось создать класс игрового оверлея: %v", syscall.GetLastError())
		}
	})
	if hudClassError != nil {
		return nil, hudClassError
	}
	h := &hudWindow{owner: u, colors: M{}, restoreMaximized: v.Bool(v.At(u.state, "desktop", "maximized"))}
	u.normalBounds = u.main.BoundsPixels()
	h.hwnd = win.CreateWindowEx(win.WS_EX_TOPMOST|win.WS_EX_TOOLWINDOW|win.WS_EX_NOACTIVATE|win.WS_EX_LAYERED|win.WS_EX_TRANSPARENT, hudClassName, wide("ED Copilot · игровой оверлей"), win.WS_POPUP, 0, 0, 410, 300, 0, 0, win.GetModuleHandle(nil), nil)
	if h.hwnd == 0 {
		return nil, fmt.Errorf("Не удалось создать игровой оверлей: %v", syscall.GetLastError())
	}
	hudWindows[h.hwnd] = h
	mainHUDWindows[u.main.Handle()] = h
	h.mainProcedure = win.SetWindowLongPtr(u.main.Handle(), win.GWLP_WNDPROC, mainHUDProcedure)
	if h.mainProcedure == 0 {
		h.close()
		return nil, fmt.Errorf("Не удалось подключить горячую клавишу оверлея: %v", syscall.GetLastError())
	}
	// MOD_NOREPEAT prevents a held key from toggling the setting repeatedly.
	ok, _, _ := registerHotKey.Call(uintptr(u.main.Handle()), overlayHotKeyID, 0x0001|0x0002|0x4000, 'H')
	h.hotkeyRegistered = ok != 0
	h.foregroundHook, _, _ = userDLL.NewProc("SetWinEventHook").Call(3, 3, 0, foregroundProcedure, 0, 0, 0)
	// A hotkey already owned by another application does not disable the HUD;
	// its ordinary in-app and tray controls remain available.
	h.refresh()
	return h, nil
}

// WINEVENT_OUTOFCONTEXT is delivered on the installing UI thread, whose Walk
// message loop is running. Polling remains a fallback if the hook is unavailable.
func hudForegroundEvent(hook uintptr, event uint32, hwnd win.HWND, idObject, idChild int32, eventThread, eventTime uint32) uintptr {
	for _, h := range hudWindows {
		if h.foregroundHook == hook && !h.closed {
			h.refresh()
			break
		}
	}
	return 0
}

func mainHUDWndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	h := mainHUDWindows[hwnd]
	if h == nil {
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}
	if msg == smokeCancelMenuMessage && h.owner.options.Smoke {
		userDLL.NewProc("EndMenu").Call()
		return 0
	}
	if msg == win.WM_HOTKEY && wParam == overlayHotKeyID {
		u := h.owner
		if !u.disposed.Load() {
			u.execute(p.Action{Path: "/api/settings", Payload: M{"overlayEnabled": !v.Bool(v.At(u.state, "settings", "overlayEnabled"))}}, nil)
		}
		return 0
	}
	if msg == win.WM_SETFOCUS || (msg == win.WM_ACTIVATE && uint16(wParam) != win.WA_INACTIVE) {
		h.hide()
	}
	result := win.CallWindowProc(h.mainProcedure, hwnd, msg, wParam, lParam)
	if (msg == win.WM_MOVE || msg == win.WM_SIZE) && h.owner.modeValue == "windowed" && !win.IsZoomed(hwnd) && !win.IsIconic(hwnd) {
		b := h.owner.main.BoundsPixels()
		if b.Width >= 900 && b.Height >= 700 {
			h.owner.normalBounds = b
		}
	}
	return result
}

func hudWndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	h := hudWindows[hwnd]
	if h == nil {
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}
	switch msg {
	case win.WM_PAINT:
		h.paint()
		return 0
	case win.WM_ERASEBKGND:
		return 1
	case win.WM_NCHITTEST:
		return ^uintptr(0) // HTTRANSPARENT, including the panel background.
	case win.WM_MOUSEACTIVATE:
		return 3 // MA_NOACTIVATE
	}
	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}

func (h *hudWindow) hide() {
	if h == nil || h.closed || h.hwnd == 0 {
		return
	}
	if h.visible {
		win.ShowWindow(h.hwnd, win.SW_HIDE)
		h.visible = false
	}
}

func gameClientArea(hwnd win.HWND) (p.HUDRect, bool) {
	r, ok := monitorRect(hwnd, false)
	if !ok {
		return p.HUDRect{}, false
	}
	var client win.RECT
	if win.GetClientRect(hwnd, &client) {
		a, b := win.POINT{X: client.Left, Y: client.Top}, win.POINT{X: client.Right, Y: client.Bottom}
		if win.ClientToScreen(hwnd, &a) && win.ClientToScreen(hwnd, &b) {
			r.Left = max(r.Left, a.X)
			r.Top = max(r.Top, a.Y)
			r.Right = min(r.Right, b.X)
			r.Bottom = min(r.Bottom, b.Y)
		}
	}
	view := p.HUDRect{X: int(r.Left), Y: int(r.Top), Width: int(r.Right - r.Left), Height: int(r.Bottom - r.Top)}
	return view, view.Width > 120 && view.Height > 120
}

func (h *hudWindow) refresh() {
	if h == nil || h.closed || h.owner.disposed.Load() {
		return
	}
	s := h.owner.state
	foreground, process := foregroundGame()
	if !p.HUDVisible(v.Bool(v.At(s, "settings", "overlayEnabled")), v.At(s, "connection", "gameRunning"), process) {
		h.hide()
		return
	}
	view, ok := gameClientArea(foreground)
	if !ok {
		h.hide()
		return
	}
	dpi := int(win.GetDpiForWindow(foreground))
	if dpi < 72 || dpi > 480 {
		dpi = 96
	}
	if dpi != h.dpi {
		h.setFonts(dpi)
	}
	colors := v.Obj(v.At(s, "theme", "palette"))
	if len(colors) == 0 {
		colors = h.owner.colors
	}
	// Palette also carries numeric overlayOpacity. Keep only actual color
	// strings here so JSON number canonicalization does not force repainting an
	// otherwise unchanged palette on every poll.
	roles := M{}
	for key, raw := range colors {
		if text, ok := raw.(string); ok {
			roles[key] = text
		}
	}
	colors = roles
	doc := p.BuildHUD(s, time.Now())
	changed := !reflect.DeepEqual(doc, h.document) || !reflect.DeepEqual(colors, h.colors)
	h.document = doc
	h.colors = v.Copy(colors)
	alpha := byte(math.Round(255 * v.Clamp(v.Num(v.First(v.At(s, "theme", "settings", "overlayOpacity"), 0.82)), 0.4, 1)))
	key := win.COLORREF(0x00030201)
	for _, raw := range colors {
		if hex, ok := raw.(string); ok && uint32(color(hex)) == uint32(key) {
			key = win.COLORREF(0x00030202)
			break
		}
	}
	if h.key != key || h.alpha != alpha {
		h.key, h.alpha = key, alpha
		setLayeredAttributes.Call(uintptr(h.hwnd), uintptr(key), uintptr(alpha), 3)
		changed = true
	}
	width := p.HUDGeometry(view, v.Str(v.At(s, "commander", "vehicle")), dpi, 0).Width
	hdc := win.GetDC(h.hwnd)
	height := h.layout(hdc, width, false)
	win.ReleaseDC(h.hwnd, hdc)
	bounds := p.HUDGeometry(view, v.Str(v.At(s, "commander", "vehicle")), dpi, height)
	if bounds.Width <= 0 || bounds.Height <= 0 {
		h.hide()
		return
	}
	if bounds != h.bounds || !h.visible {
		h.bounds = bounds
		win.SetWindowPos(h.hwnd, win.HWND_TOPMOST, int32(bounds.X), int32(bounds.Y), int32(bounds.Width), int32(bounds.Height), win.SWP_NOACTIVATE|win.SWP_SHOWWINDOW)
		h.visible = true
		changed = true
	}
	if changed {
		win.InvalidateRect(h.hwnd, nil, false)
	}
}

func (h *hudWindow) setFonts(dpi int) {
	for _, f := range []win.HFONT{h.regular, h.bold} {
		if f != 0 {
			win.DeleteObject(win.HGDIOBJ(f))
		}
	}
	h.dpi = dpi
	makeFont := func(points, weight int) win.HFONT {
		font := win.LOGFONT{LfHeight: -int32(math.Round(float64(points*dpi) / 72)), LfWeight: int32(weight), LfCharSet: win.DEFAULT_CHARSET, LfQuality: win.CLEARTYPE_QUALITY}
		copy(font.LfFaceName[:], utf16.Encode([]rune("Segoe UI")))
		return win.CreateFontIndirect(&font)
	}
	h.regular = makeFont(10, 400)
	h.bold = makeFont(10, 700)
}

func fillSolid(hdc win.HDC, r win.RECT, c win.COLORREF) {
	brush := win.CreateBrushIndirect(&win.LOGBRUSH{LbStyle: win.BS_SOLID, LbColor: c})
	if brush == 0 {
		return
	}
	fillRectProc.Call(uintptr(hdc), uintptr(unsafe.Pointer(&r)), uintptr(brush))
	win.DeleteObject(win.HGDIOBJ(brush))
}
func (h *hudWindow) ink(role string) win.COLORREF {
	fallback := map[string]string{"accent": "#28a0f0", "text": "#e0e0e0", "muted": "#8898a8", "panel": "#0a1622", "bg": "#07101a", "good": "#63d8bd", "warn": "#edbd66", "danger": "#fa8181"}
	return win.COLORREF(color(v.Text(h.colors[role], fallback[role], fallback["text"])))
}
func hudText(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) > maxRunes {
		return string(r[:maxRunes]) + "…"
	}
	return s
}

// layout uses GDI's actual Cyrillic glyph metrics for both measurement and
// drawing. Each section is a separate translucent panel with a clear gap.
func (h *hudWindow) layout(hdc win.HDC, width int, draw bool) int {
	if hdc == 0 {
		return 0
	}
	scale := float64(h.dpi) / 96
	px := func(n int) int { return max(1, int(math.Round(float64(n)*scale))) }
	pad, gap := px(12), px(8)
	y := 0
	oldFont := win.SelectObject(hdc, win.HGDIOBJ(h.regular))
	defer win.SelectObject(hdc, oldFont)
	win.SetBkMode(hdc, win.TRANSPARENT)
	textHeight := func(text string, bold bool) int {
		f := h.regular
		if bold {
			f = h.bold
		}
		win.SelectObject(hdc, win.HGDIOBJ(f))
		rect := win.RECT{Left: int32(pad), Top: 0, Right: int32(max(pad+10, width-pad)), Bottom: 0}
		win.DrawTextEx(hdc, wide(text), -1, &rect, win.DT_WORDBREAK|win.DT_NOPREFIX|win.DT_CALCRECT, nil)
		return min(max(px(17), int(rect.Bottom)), px(52))
	}
	for _, section := range h.document.Sections {
		title := hudText(section.Title, 100)
		titleH := textHeight(title, true)
		heights := make([]int, len(section.Lines))
		sectionH := pad + titleH + px(7) + pad
		for i, line := range section.Lines {
			heights[i] = textHeight(hudText(line.Text, 280), false)
			sectionH += heights[i] + px(3)
		}
		if draw {
			fillSolid(hdc, win.RECT{Left: 0, Top: int32(y), Right: int32(width), Bottom: int32(y + sectionH)}, h.ink("panel"))
			fillSolid(hdc, win.RECT{Left: 0, Top: int32(y), Right: int32(px(2)), Bottom: int32(y + sectionH)}, h.ink("accent"))
			win.SelectObject(hdc, win.HGDIOBJ(h.bold))
			win.SetTextColor(hdc, h.ink("accent"))
			r := win.RECT{Left: int32(pad), Top: int32(y + pad), Right: int32(width - pad), Bottom: int32(y + pad + titleH)}
			win.DrawTextEx(hdc, wide(title), -1, &r, win.DT_WORDBREAK|win.DT_NOPREFIX|win.DT_END_ELLIPSIS, nil)
			lineY := y + pad + titleH + px(7)
			win.SelectObject(hdc, win.HGDIOBJ(h.regular))
			for i, line := range section.Lines {
				win.SetTextColor(hdc, h.ink(line.Role))
				r := win.RECT{Left: int32(pad), Top: int32(lineY), Right: int32(width - pad), Bottom: int32(lineY + heights[i])}
				win.DrawTextEx(hdc, wide(hudText(line.Text, 280)), -1, &r, win.DT_WORDBREAK|win.DT_NOPREFIX|win.DT_END_ELLIPSIS, nil)
				lineY += heights[i] + px(3)
			}
		}
		y += sectionH + gap
	}
	return max(1, y-gap)
}

func (h *hudWindow) paint() {
	var ps win.PAINTSTRUCT
	target := win.BeginPaint(h.hwnd, &ps)
	if target == 0 {
		return
	}
	defer win.EndPaint(h.hwnd, &ps)
	var client win.RECT
	if !win.GetClientRect(h.hwnd, &client) {
		return
	}
	width, height := client.Right-client.Left, client.Bottom-client.Top
	if width <= 0 || height <= 0 {
		return
	}
	hdc := win.CreateCompatibleDC(target)
	if hdc == 0 {
		return
	}
	defer win.DeleteDC(hdc)
	bitmap := win.CreateCompatibleBitmap(target, width, height)
	if bitmap == 0 {
		return
	}
	defer win.DeleteObject(win.HGDIOBJ(bitmap))
	old := win.SelectObject(hdc, win.HGDIOBJ(bitmap))
	defer win.SelectObject(hdc, old)
	fillSolid(hdc, client, h.key)
	h.layout(hdc, int(width), true)
	win.BitBlt(target, 0, 0, width, height, hdc, 0, 0, win.SRCCOPY)
}

func (h *hudWindow) close() {
	if h == nil || h.closed {
		return
	}
	h.closed = true
	if h.foregroundHook != 0 {
		userDLL.NewProc("UnhookWinEvent").Call(h.foregroundHook)
		h.foregroundHook = 0
	}
	if h.owner != nil && h.owner.main != nil {
		main := h.owner.main.Handle()
		if h.hotkeyRegistered {
			unregisterHotKey.Call(uintptr(main), overlayHotKeyID)
			h.hotkeyRegistered = false
		}
		if h.mainProcedure != 0 {
			win.SetWindowLongPtr(main, win.GWLP_WNDPROC, h.mainProcedure)
		}
		delete(mainHUDWindows, main)
	}
	if h.hwnd != 0 {
		delete(hudWindows, h.hwnd)
		win.DestroyWindow(h.hwnd)
		h.hwnd = 0
	}
	for _, font := range []win.HFONT{h.regular, h.bold} {
		if font != 0 {
			win.DeleteObject(win.HGDIOBJ(font))
		}
	}
	h.regular, h.bold = 0, 0
	h.visible = false
}
