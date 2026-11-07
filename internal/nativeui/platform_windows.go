//go:build windows

package nativeui

import (
	"errors"
	"net/url"
	"strings"
	"syscall"
	"unsafe"

	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/walk"
	"github.com/lxn/win"
)

var (
	userDLL               = syscall.NewLazyDLL("user32.dll")
	kernelDLL             = syscall.NewLazyDLL("kernel32.dll")
	dwmDLL                = syscall.NewLazyDLL("dwmapi.dll")
	setLayeredAttributes  = userDLL.NewProc("SetLayeredWindowAttributes")
	registerHotKey        = userDLL.NewProc("RegisterHotKey")
	unregisterHotKey      = userDLL.NewProc("UnregisterHotKey")
	queryProcessImage     = kernelDLL.NewProc("QueryFullProcessImageNameW")
	openProcess           = kernelDLL.NewProc("OpenProcess")
	closeHandle           = kernelDLL.NewProc("CloseHandle")
	dwmSetWindowAttribute = dwmDLL.NewProc("DwmSetWindowAttribute")
)

func wide(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(strings.ReplaceAll(s, "\x00", ""))
	return p
}

func darkWindow(hwnd win.HWND) {
	if dwmSetWindowAttribute.Find() != nil {
		return
	}
	on := int32(1)
	r, _, _ := dwmSetWindowAttribute.Call(uintptr(hwnd), 20, uintptr(unsafe.Pointer(&on)), unsafe.Sizeof(on))
	if int32(r) < 0 {
		_, _, _ = dwmSetWindowAttribute.Call(uintptr(hwnd), 19, uintptr(unsafe.Pointer(&on)), unsafe.Sizeof(on))
	}
}

func openExternal(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return errors.New("Нужен корректный адрес HTTP или HTTPS.")
	}
	if !win.ShellExecute(0, wide("open"), wide(u.String()), nil, nil, win.SW_SHOWNORMAL) {
		return errors.New("Не удалось открыть адрес в браузере.")
	}
	return nil
}

// foregroundGame identifies the executable owning the foreground window. It
// does not infer focus from a title, a running process or stale journal state.
func foregroundGame() (win.HWND, string) {
	hwnd := win.GetForegroundWindow()
	if hwnd == 0 {
		return 0, ""
	}
	var pid uint32
	win.GetWindowThreadProcessId(hwnd, &pid)
	if pid == 0 {
		return hwnd, ""
	}
	h, _, _ := openProcess.Call(0x1000, 0, uintptr(pid)) // PROCESS_QUERY_LIMITED_INFORMATION
	if h == 0 {
		return hwnd, ""
	}
	defer closeHandle.Call(h)
	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	ok, _, _ := queryProcessImage.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if ok == 0 || size == 0 || size > uint32(len(buf)) {
		return hwnd, ""
	}
	return hwnd, syscall.UTF16ToString(buf[:size])
}

func monitorRect(hwnd win.HWND, work bool) (win.RECT, bool) {
	m := win.MonitorFromWindow(hwnd, win.MONITOR_DEFAULTTONEAREST)
	if m == 0 {
		return win.RECT{}, false
	}
	info := win.MONITORINFO{CbSize: uint32(unsafe.Sizeof(win.MONITORINFO{}))}
	if !win.GetMonitorInfo(m, &info) {
		return win.RECT{}, false
	}
	if work {
		return info.RcWork, true
	}
	return info.RcMonitor, true
}

func (u *Window) applyMode(mode string) {
	if mode != "windowed" && mode != "fullscreen" && mode != "borderless" {
		return
	}
	if u.main == nil || u.disposed.Load() {
		return
	}
	if u.modeValue == mode {
		return
	}
	hwnd := u.main.Handle()
	if u.modeValue == "windowed" || (u.modeValue == "" && mode != "windowed") {
		if win.IsZoomed(hwnd) {
			var placement win.WINDOWPLACEMENT
			placement.Length = uint32(unsafe.Sizeof(placement))
			if win.GetWindowPlacement(hwnd, &placement) {
				r := placement.RcNormalPosition
				u.normalBounds = walk.Rectangle{X: int(r.Left), Y: int(r.Top), Width: int(r.Right - r.Left), Height: int(r.Bottom - r.Top)}
			}
		} else {
			u.normalBounds = u.main.BoundsPixels()
		}
	}
	// Restore before removing a maximized caption; otherwise Windows retains a
	// maximized placement which prevents correct monitor-sized borderless mode.
	wasMaximized := win.IsZoomed(hwnd)
	if mode != "windowed" && wasMaximized {
		win.ShowWindow(hwnd, win.SW_RESTORE)
	}
	style := uint32(win.GetWindowLong(hwnd, win.GWL_STYLE))
	if mode == "windowed" {
		style = (style &^ uint32(win.WS_POPUP)) | uint32(win.WS_OVERLAPPEDWINDOW)
		win.SetWindowLong(hwnd, win.GWL_STYLE, int32(style))
		u.modeValue = mode
		b := u.normalBounds
		if b.Width < 900 || b.Height < 700 {
			if saved := v.Obj(v.At(u.state, "desktop", "windowedBounds")); v.Int(saved["width"]) >= 900 && v.Int(saved["height"]) >= 700 {
				b = walk.Rectangle{X: v.Int(saved["x"]), Y: v.Int(saved["y"]), Width: v.Int(saved["width"]), Height: v.Int(saved["height"])}
			}
		}
		if b.Width < 900 || b.Height < 700 {
			b = u.main.BoundsPixels()
		}
		win.SetWindowPos(hwnd, 0, int32(b.X), int32(b.Y), int32(b.Width), int32(b.Height), win.SWP_FRAMECHANGED|win.SWP_NOZORDER|win.SWP_NOACTIVATE)
		if u.hud != nil && u.hud.restoreMaximized {
			win.ShowWindow(hwnd, win.SW_MAXIMIZE)
			u.hud.restoreMaximized = false
		} else if !wasMaximized && v.Bool(v.At(u.state, "desktop", "maximized")) && u.hud != nil && !u.hud.modeApplied {
			win.ShowWindow(hwnd, win.SW_MAXIMIZE)
		}
	} else {
		if u.hud != nil && u.modeValue == "windowed" {
			u.hud.restoreMaximized = wasMaximized
		}
		style = (style &^ uint32(win.WS_OVERLAPPEDWINDOW)) | uint32(win.WS_POPUP)
		win.SetWindowLong(hwnd, win.GWL_STYLE, int32(style))
		u.modeValue = mode
		if r, ok := monitorRect(hwnd, mode == "borderless"); ok {
			win.SetWindowPos(hwnd, 0, r.Left, r.Top, r.Right-r.Left, r.Bottom-r.Top, win.SWP_FRAMECHANGED|win.SWP_NOZORDER|win.SWP_NOACTIVATE)
		}
	}
	if u.hud != nil {
		u.hud.modeApplied = true
		u.hud.refresh()
	}
}

func (u *Window) saveBounds() {
	if u.main == nil || u.disposed.Load() {
		return
	}
	b := u.main.BoundsPixels()
	maximized := false
	if u.modeValue == "windowed" || u.modeValue == "" {
		maximized = win.IsZoomed(u.main.Handle())
		if !maximized && !win.IsIconic(u.main.Handle()) && b.Width >= 900 && b.Height >= 700 {
			u.normalBounds = b
		}
	} else if u.hud != nil {
		maximized = u.hud.restoreMaximized
	}
	normal := u.normalBounds
	if normal.Width < 900 || normal.Height < 700 {
		normal = b
	}
	if normal.Width < 900 || normal.Height < 700 {
		return
	}
	mapRect := func(r walk.Rectangle) M { return M{"x": r.X, "y": r.Y, "width": r.Width, "height": r.Height} }
	_, _ = u.backend.Do("/api/desktop", M{"windowedBounds": mapRect(normal), "bounds": mapRect(b), "maximized": maximized})
}
