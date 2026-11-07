//go:build windows

package nativeui

import (
	"encoding/json"
	"fmt"
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// smoke uses only the disposable CLI fixture with networking disabled.
// Dialogs are inspected and cancelled, never submitted; no browser is opened.
func (u *Window) smoke() {
	if u.options.Smoke {
		go u.smokeRun()
	}
}
func (u *Window) smokeRun() {
	report := M{"ok": true, "native": true, "renderer": "Win32/GDI", "webview": false, "networkDisabled": true, "scenarios": []M{}}
	problems := []string{}
	add := func(name string, ok bool) {
		report["scenarios"] = append(v.Rows(report["scenarios"]), M{"name": name, "ok": ok})
		if !ok {
			problems = append(problems, name)
		}
	}
	dir := filepath.Dir(u.options.SmokeReport)
	if dir == "" {
		dir = "."
	}
	_ = os.MkdirAll(dir, 0700)
	step := func(name string, fn func()) {
		_ = os.WriteFile(filepath.Join(dir, "native-smoke-progress.txt"), []byte(name), 0600)
		done := make(chan struct{})
		u.main.Synchronize(func() { fn(); close(done) })
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			panic("native smoke UI timeout: " + name)
		}
	}
	step("show fixture", func() {
		report["version"] = v.At(u.state, "app", "version")
		win.ShowWindow(u.main.Handle(), win.SW_SHOWNORMAL)
	})
	time.Sleep(150 * time.Millisecond)
	seenForms := map[string]bool{}
	forms := []p.Action{}
	for _, tab := range p.Tabs {
		modes := []string{}
		step("open "+tab.ID, func() { u.navigate(tab.ID); modes = p.Build(tab.ID, "", "", u.state).Modes })
		if len(modes) == 0 {
			modes = []string{""}
		}
		for _, mode := range modes {
			step("render "+tab.ID+" / "+mode, func() {
				u.modes[tab.ID] = mode
				u.renderPage(true)
				add(tab.ID+" / "+mode, u.table.Columns().Len() > 0 && u.titleLabel.Text() != "")
				for _, action := range Actions(tab.ID, mode, u.state, u.selected(), u.prefill) {
					if len(action.Fields) > 0 && !seenForms[action.ID] && !action.Disabled {
						seenForms[action.ID] = true
						forms = append(forms, action)
					}
				}
			})
			time.Sleep(25 * time.Millisecond)
		}
	}
	for _, action := range forms {
		step("form "+action.ID, func() {
			success, counts := u.smokeForm(action, dir)
			add("Native form: "+action.ID, success)
			report["forms"] = append(v.Rows(report["forms"]), M{"id": action.ID, "fields": len(action.Fields), "controls": counts, "cancelled": success})
		})
	}
	step("reset tabs", func() {
		u.tabs = append([]string(nil), p.DefaultTabs...)
		u.page = "overview"
		u.persistTabs()
		u.renderPage(true)
	})
	time.Sleep(100 * time.Millisecond)
	step("native plus popup", func() {
		var seen atomic.Bool
		thread := win.GetWindowThreadProcessId(u.main.Handle(), nil)
		go func() {
			time.Sleep(100 * time.Millisecond)
			for _, hwnd := range smokeWindowsOnThread(thread) {
				if smokeClass(hwnd) == "#32768" {
					seen.Store(true)
				}
			}
			win.PostMessage(u.main.Handle(), smokeCancelMenuMessage, 0, 0)
		}()
		u.addTabMenu()
		add("Plus menu opens a native popup", seen.Load())
	})
	step("optional tab open close", func() {
		u.navigate("carrier")
		add("Optional tab opens", u.page == "carrier" && smokeContains(u.tabs, "carrier"))
		u.closeTab("carrier")
		add("Optional tab closes", !smokeContains(u.tabs, "carrier"))
		before := strings.Join(u.tabs, "|")
		u.closeTab("overview")
		u.closeTab("settings")
		add("Required tabs cannot close", strings.Join(u.tabs, "|") == before)
		u.navigate("targets")
	})
	time.Sleep(100 * time.Millisecond)
	step("tab reorder", func() {
		smokePaint(u.nav.Handle())
		if len(u.navHits) > 1 {
			id := u.navHits[1].ID
			u.drag = id
			u.dragX = u.navHits[1].Bounds.X + 10
			u.dragged = true
			u.navUp(2, 20, walk.LeftButton)
			saved := v.Arr(v.At(u.backend.Snapshot(), "navigation", "tabs"))
			add("Tabs can be reordered and persisted", len(u.tabs) > 0 && u.tabs[0] == id && len(saved) > 0 && v.Str(saved[0]) == id)
		}
	})
	for _, mode := range []string{"fullscreen", "borderless", "windowed"} {
		step("mode "+mode, func() {
			u.applyMode(mode)
			style := uint32(win.GetWindowLong(u.main.Handle(), win.GWL_STYLE))
			ok := u.modeValue == mode
			if mode == "windowed" {
				ok = ok && style&uint32(win.WS_OVERLAPPEDWINDOW) == uint32(win.WS_OVERLAPPEDWINDOW)
			} else {
				ok = ok && style&uint32(win.WS_CAPTION) == 0
			}
			add("Window mode: "+mode, ok)
		})
		time.Sleep(100 * time.Millisecond)
	}
	step("native identity and focus", func() {
		add("All sections use native HWND controls", u.main.Handle() != 0 && u.table.Handle() != 0 && u.details.Handle() != 0)
		add("Cyrillic system font", u.font.Family() == "Segoe UI")
		cmd := v.Obj(u.state["commander"])
		if v.Str(cmd["shipType"]) != "" {
			add("Ship model is displayed separately", v.Str(cmd["ship"]) != "" && v.Str(cmd["ship"]) != v.Str(cmd["shipType"]))
		}
		if u.hud != nil {
			u.hud.refresh()
			add("HUD is hidden while Copilot has focus", !win.IsWindowVisible(u.hud.hwnd))
		}
	})
	samples := []struct {
		tab, mode, file string
		width           int
	}{{"colonization", "Строительство", "native-colonization.png", 1100}, {"overview", "Планы", "native-overview.png", 1100}, {"engineering", "Материалы", "native-engineering.png", 1100}, {"settings", "", "native-settings.png", 1100}, {"colonization", "Снабжение", "native-colonization-900.png", 900}, {"colonization", "Строительство", "native-colonization-1440.png", 1440}}
	for _, sample := range samples {
		step("layout "+sample.file, func() {
			u.main.SetSize(walk.Size{Width: sample.width, Height: 900})
			u.navigate(sample.tab)
			u.modes[sample.tab] = sample.mode
			u.renderPage(true)
		})
		time.Sleep(250 * time.Millisecond)
		step("capture "+sample.file, func() {
			smokePaint(u.main.Handle())
			err := smokeCapture(u.main.Handle(), filepath.Join(dir, sample.file))
			add("Painted capture: "+sample.file, err == nil)
		})
	}
	report["problems"] = problems
	report["ok"] = len(problems) == 0
	bytes, _ := json.MarshalIndent(report, "", "  ")
	if u.options.SmokeReport != "" {
		_ = os.WriteFile(u.options.SmokeReport, bytes, 0600)
	} else {
		fmt.Println(string(bytes))
	}
	step("completed", u.quit)
}

func smokeContains(values []string, want string) bool {
	for _, s := range values {
		if s == want {
			return true
		}
	}
	return false
}

var smokeEnumResult []win.HWND
var smokeEnumMutex sync.Mutex

const smokeCancelMenuMessage = win.WM_APP + 0x79

var smokeEnumCallback = syscall.NewCallback(func(hwnd win.HWND, param uintptr) uintptr { smokeEnumResult = append(smokeEnumResult, hwnd); return 1 })

func smokeThreadWindows() []win.HWND {
	thread, _, _ := kernelDLL.NewProc("GetCurrentThreadId").Call()
	return smokeWindowsOnThread(uint32(thread))
}
func smokeWindowsOnThread(thread uint32) []win.HWND {
	smokeEnumMutex.Lock()
	defer smokeEnumMutex.Unlock()
	smokeEnumResult = nil
	userDLL.NewProc("EnumThreadWindows").Call(uintptr(thread), smokeEnumCallback, 0)
	return append([]win.HWND(nil), smokeEnumResult...)
}
func smokeClass(hwnd win.HWND) string {
	buf := make([]uint16, 256)
	n, _ := win.GetClassName(hwnd, &buf[0], len(buf))
	if n <= 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}
func smokeTitle(hwnd win.HWND) string {
	buf := make([]uint16, 1024)
	n := win.SendMessage(hwnd, win.WM_GETTEXT, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	return syscall.UTF16ToString(buf[:min(n, uintptr(len(buf)))])
}
func (u *Window) smokeForm(action p.Action, dir string) (bool, M) {
	counts := M{}
	found := false
	passwords := 0
	for _, f := range action.Fields {
		if f.Kind == "password" {
			passwords++
		}
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		u.main.Synchronize(func() {
			for _, hwnd := range smokeThreadWindows() {
				if win.GetWindow(hwnd, win.GW_OWNER) != u.main.Handle() || !strings.HasPrefix(smokeTitle(hwnd), action.Title) {
					continue
				}
				found = true
				win.EnumChildWindows(hwnd, syscall.NewCallback(func(child win.HWND, param uintptr) uintptr {
					class := smokeClass(child)
					counts[class] = v.Int(counts[class]) + 1
					if class == "Edit" && win.SendMessage(child, win.EM_GETPASSWORDCHAR, 0, 0) != 0 {
						counts["password"] = v.Int(counts["password"]) + 1
					}
					return 1
				}), 0)
				if action.ID == "engineering.add" || action.ID == "settings.palette" || action.ID == "colony.build.add" {
					smokePaint(hwnd)
					_ = smokeCapture(hwnd, filepath.Join(dir, "native-form-"+strings.ReplaceAll(action.ID, ".", "-")+".png"))
				}
				win.PostMessage(hwnd, win.WM_CLOSE, 0, 0)
			}
		})
	}()
	_, accepted := u.form(action)
	inputs := v.Int(counts["Edit"]) + v.Int(counts["ComboBox"]) + v.Int(counts["Button"])
	return found && !accepted && inputs >= len(action.Fields) && v.Int(counts["password"]) >= passwords, counts
}
func smokePaint(hwnd win.HWND) {
	win.RedrawWindow(hwnd, nil, 0, win.RDW_INVALIDATE|win.RDW_ERASE|win.RDW_ALLCHILDREN|win.RDW_FRAME|win.RDW_UPDATENOW)
	if flush := dwmDLL.NewProc("DwmFlush"); flush.Find() == nil {
		flush.Call()
	}
}

// WM_PRINT is not implemented by every custom-painted Walk child. Capture the
// repainted own window DC; GDI's undefined alpha is made opaque in the PNG.
func smokeCapture(hwnd win.HWND, file string) error {
	var r win.RECT
	if !win.GetWindowRect(hwnd, &r) {
		return fmt.Errorf("GetWindowRect")
	}
	var client win.RECT
	var origin win.POINT
	if !win.GetClientRect(hwnd, &client) || !win.ClientToScreen(hwnd, &origin) {
		return fmt.Errorf("GetClientRect")
	}
	width, height := client.Right-client.Left, client.Bottom-client.Top
	if width <= 0 || height <= 0 || int64(width)*int64(height) > 50000000 {
		return fmt.Errorf("invalid window size")
	}
	raw, _, _ := userDLL.NewProc("GetWindowDC").Call(uintptr(hwnd))
	source := win.HDC(raw)
	if source == 0 {
		return fmt.Errorf("GetWindowDC")
	}
	defer win.ReleaseDC(hwnd, source)
	dc := win.CreateCompatibleDC(source)
	if dc == 0 {
		return fmt.Errorf("CreateCompatibleDC")
	}
	defer win.DeleteDC(dc)
	bitmap := win.CreateCompatibleBitmap(source, width, height)
	if bitmap == 0 {
		return fmt.Errorf("CreateCompatibleBitmap")
	}
	defer win.DeleteObject(win.HGDIOBJ(bitmap))
	old := win.SelectObject(dc, win.HGDIOBJ(bitmap))
	ok := win.BitBlt(dc, 0, 0, width, height, source, origin.X-r.Left, origin.Y-r.Top, win.SRCCOPY)
	win.SelectObject(dc, old)
	if !ok {
		return fmt.Errorf("BitBlt")
	}
	info := win.BITMAPINFO{}
	info.BmiHeader.BiSize = uint32(unsafe.Sizeof(info.BmiHeader))
	info.BmiHeader.BiWidth = width
	info.BmiHeader.BiHeight = -height
	info.BmiHeader.BiPlanes = 1
	info.BmiHeader.BiBitCount = 32
	info.BmiHeader.BiCompression = win.BI_RGB
	buf := make([]byte, int(width)*int(height)*4)
	if win.GetDIBits(dc, bitmap, 0, uint32(height), &buf[0], &info, win.DIB_RGB_COLORS) == 0 {
		return fmt.Errorf("GetDIBits")
	}
	img := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	for i := 0; i < len(buf); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = buf[i+2], buf[i+1], buf[i], 255
	}
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
func nativeErrorLog(err error, dir string) {
	if err != nil {
		_ = os.MkdirAll(dir, 0700)
		_ = os.WriteFile(filepath.Join(dir, "native-startup-error.txt"), []byte(strings.TrimSpace(err.Error())), 0600)
	}
}
