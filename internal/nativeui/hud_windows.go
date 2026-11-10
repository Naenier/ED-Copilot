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
	leftHwnd                      win.HWND
	mainProcedure                 uintptr
	foregroundHook                uintptr
	hotkeyRegistered              bool
	closed, visible               bool
	leftVisible                   bool
	modeApplied, restoreMaximized bool
	document                      p.HUDDocument
	renderSections                []p.HUDSection
	colors                        M
	dpi                           int
	regular, bold                 win.HFONT
	key                           win.COLORREF
	alpha                         byte
	bounds                        p.HUDRect
	leftBounds                    p.HUDRect
	plan                          p.HUDLayout
	metrics                       []hudSectionMetrics
	layoutView                    p.HUDRect
	layoutVehicle                 string
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
	h.hwnd = win.CreateWindowEx(win.WS_EX_TOPMOST|win.WS_EX_TOOLWINDOW|win.WS_EX_NOACTIVATE|win.WS_EX_LAYERED|win.WS_EX_TRANSPARENT, hudClassName, wide("ED Copilot · игровой оверлей"), win.WS_POPUP, 0, 0, 330, 300, 0, 0, win.GetModuleHandle(nil), nil)
	if h.hwnd == 0 {
		return nil, fmt.Errorf("Не удалось создать игровой оверлей: %v", syscall.GetLastError())
	}
	hudWindows[h.hwnd] = h
	h.leftHwnd = win.CreateWindowEx(win.WS_EX_TOPMOST|win.WS_EX_TOOLWINDOW|win.WS_EX_NOACTIVATE|win.WS_EX_LAYERED|win.WS_EX_TRANSPARENT, hudClassName, wide("ED Copilot · дополнительные панели"), win.WS_POPUP, 0, 0, 330, 300, 0, 0, win.GetModuleHandle(nil), nil)
	if h.leftHwnd == 0 {
		createErr := syscall.GetLastError()
		h.close()
		return nil, fmt.Errorf("Не удалось создать дополнительную колонку оверлея: %v", createErr)
	}
	hudWindows[h.leftHwnd] = h
	mainHUDWindows[u.main.Handle()] = h
	h.mainProcedure = win.SetWindowLongPtr(u.main.Handle(), win.GWLP_WNDPROC, mainHUDProcedure)
	if h.mainProcedure == 0 {
		h.close()
		return nil, fmt.Errorf("Не удалось подключить горячую клавишу оверлея: %v", syscall.GetLastError())
	}
	// MOD_NOREPEAT prevents a held key from toggling the setting repeatedly.
	if u.options.Soak == nil {
		ok, _, _ := registerHotKey.Call(uintptr(u.main.Handle()), overlayHotKeyID, 0x0001|0x0002|0x4000, 'H')
		h.hotkeyRegistered = ok != 0
		h.foregroundHook, _, _ = userDLL.NewProc("SetWinEventHook").Call(3, 3, 0, foregroundProcedure, 0, 0, 0)
	}
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
	if h.owner.modern != nil {
		if value, handled := h.owner.modern.windowMessage(hwnd, msg, wParam, lParam); handled {
			return value
		}
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
		h.paintWindow(hwnd)
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
	if h.leftVisible {
		win.ShowWindow(h.leftHwnd, win.SW_HIDE)
		h.leftVisible = false
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
	var foreground win.HWND
	var process string
	if h.owner.options.Soak != nil {
		// Only the isolated synthetic run supplies a game window identity.
		foreground, process = h.owner.main.Handle(), "EliteDangerous64.exe"
	} else {
		foreground, process = foregroundGame()
	}
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
	fontsChanged := dpi != h.dpi
	if fontsChanged {
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
	documentChanged := !reflect.DeepEqual(doc, h.document)
	changed := fontsChanged || documentChanged || !reflect.DeepEqual(colors, h.colors)
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
		setLayeredAttributes.Call(uintptr(h.leftHwnd), uintptr(key), uintptr(alpha), 3)
		changed = true
	}
	vehicle := v.Str(v.At(s, "commander", "vehicle"))
	plan := h.arrange(view, vehicle, dpi, fontsChanged || documentChanged)
	if plan.Bounds[0].Height <= 0 && plan.Bounds[1].Height <= 0 {
		h.layoutView = p.HUDRect{}
		h.hide()
		return
	}
	changed = changed || !reflect.DeepEqual(plan, h.plan)
	h.plan = plan
	applyColumn := func(hwnd win.HWND, next p.HUDRect, previous *p.HUDRect, visible *bool) {
		if next.Width <= 0 || next.Height <= 0 {
			if *visible {
				win.ShowWindow(hwnd, win.SW_HIDE)
				*visible = false
			}
			*previous = next
			return
		}
		if next != *previous || !*visible {
			*previous = next
			win.SetWindowPos(hwnd, win.HWND_TOPMOST, int32(next.X), int32(next.Y), int32(next.Width), int32(next.Height), win.SWP_NOACTIVATE|win.SWP_SHOWWINDOW)
			*visible = true
			changed = true
		}
	}
	applyColumn(h.hwnd, plan.Bounds[0], &h.bounds, &h.visible)
	applyColumn(h.leftHwnd, plan.Bounds[1], &h.leftBounds, &h.leftVisible)
	if changed {
		win.InvalidateRect(h.hwnd, nil, false)
		win.InvalidateRect(h.leftHwnd, nil, false)
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
	h.bold = makeFont(10, 650)
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

func (h *hudWindow) paint() { h.paintWindow(h.hwnd) }

func (h *hudWindow) paintWindow(hwnd win.HWND) {
	var ps win.PAINTSTRUCT
	target := win.BeginPaint(hwnd, &ps)
	if target == 0 {
		return
	}
	var rendered string
	defer func() {
		win.EndPaint(hwnd, &ps)
		if rendered != "" {
			h.owner.modern.soak.painted(rendered)
		}
	}()
	var client win.RECT
	if !win.GetClientRect(hwnd, &client) {
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
	column, bounds := 0, h.bounds
	if hwnd == h.leftHwnd {
		column, bounds = 1, h.leftBounds
	}
	sections := h.renderSections
	if sections == nil {
		sections = h.document.Sections
	}
	for _, panel := range h.plan.Panels {
		if panel.Column != column || panel.Index >= len(sections) {
			continue
		}
		var metrics hudSectionMetrics
		if panel.Index < len(h.metrics) {
			metrics = h.metrics[panel.Index]
		} else {
			metrics = h.measureSection(hdc, int(width), sections[panel.Index])
		}
		h.drawSection(hdc, int(width), panel.Bounds.Y-bounds.Y, panel.Bounds.Height, sections[panel.Index], metrics, panel.Clipped)
		if h.owner.options.Soak != nil && panel.Index == 0 {
			rendered = sections[panel.Index].Subtitle
		}
	}
	if column == 1 && h.plan.Hidden > 0 && h.plan.Overflow.Height > 0 {
		r := h.plan.Overflow
		h.drawOverflow(hdc, int(width), r.Y-bounds.Y, r.Height, h.plan.Hidden)
	}
	if !win.BitBlt(target, 0, 0, width, height, hdc, 0, 0, win.SRCCOPY) {
		rendered = ""
	}
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
	if h.leftHwnd != 0 {
		delete(hudWindows, h.leftHwnd)
		win.DestroyWindow(h.leftHwnd)
		h.leftHwnd = 0
	}
	for _, font := range []win.HFONT{h.regular, h.bold} {
		if font != 0 {
			win.DeleteObject(win.HGDIOBJ(font))
		}
	}
	h.regular, h.bold = 0, 0
	h.visible = false
	h.leftVisible = false
}
