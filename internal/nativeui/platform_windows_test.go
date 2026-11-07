//go:build windows

package nativeui

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"

	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/walk"
	"github.com/lxn/win"
)

type platformController struct {
	state, last M
	path        string
}

func (c *platformController) Snapshot() M { return v.Copy(c.state) }
func (c *platformController) Do(path string, body M) (M, error) {
	c.path, c.last = path, v.Copy(body)
	return M{}, nil
}
func (c *platformController) Updates() <-chan struct{}          { return nil }
func (c *platformController) ExportCSV(string) (string, error)  { return "", nil }
func (c *platformController) ExportPlan(string) ([]byte, error) { return nil, nil }
func (c *platformController) Close()                            {}
func (c *platformController) DataDir() string                   { return "" }

// The GUI embeds this dependency in its executable manifest. Go's temporary
// test executable needs the same common-controls activation context.
var nativeContextOnce sync.Once
var nativeContext uintptr
var nativeContextErr error
var nativeContextPath string

func TestMain(m *testing.M) {
	code := m.Run()
	if nativeContext != 0 {
		kernelDLL.NewProc("ReleaseActCtx").Call(nativeContext)
	}
	if nativeContextPath != "" {
		_ = os.Remove(nativeContextPath)
		_ = os.Remove(filepath.Dir(nativeContextPath))
	}
	os.Exit(code)
}
func nativeTestControls(t *testing.T) func() {
	t.Helper()
	manifest := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0"><assemblyIdentity version="1.0.0.0" processorArchitecture="*" name="EDCopilot.NativeTests" type="win32"/><dependency><dependentAssembly><assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/></dependentAssembly></dependency></assembly>`
	type actCtx struct {
		Size, Flags                                      uint32
		Source                                           *uint16
		Arch, Language                                   uint16
		AssemblyDirectory, ResourceName, ApplicationName *uint16
		Module                                           uintptr
	}
	nativeContextOnce.Do(func() {
		var dir string
		dir, nativeContextErr = os.MkdirTemp("", "ed-copilot-native-controls-")
		if nativeContextErr != nil {
			return
		}
		nativeContextPath = filepath.Join(dir, "controls.manifest")
		if nativeContextErr = os.WriteFile(nativeContextPath, []byte(manifest), 0600); nativeContextErr != nil {
			return
		}
		ctx := actCtx{Source: wide(nativeContextPath)}
		ctx.Size = uint32(unsafe.Sizeof(ctx))
		var err error
		nativeContext, _, err = kernelDLL.NewProc("CreateActCtxW").Call(uintptr(unsafe.Pointer(&ctx)))
		if nativeContext == ^uintptr(0) {
			nativeContext = 0
			nativeContextErr = err
		}
	})
	if nativeContextErr != nil || nativeContext == 0 {
		t.Fatalf("CreateActCtxW: %v", nativeContextErr)
	}
	var cookie uintptr
	ok, _, err := kernelDLL.NewProc("ActivateActCtx").Call(nativeContext, uintptr(unsafe.Pointer(&cookie)))
	if ok == 0 {
		t.Fatalf("ActivateActCtx: %v", err)
	}
	return func() {
		kernelDLL.NewProc("DeactivateActCtx").Call(0, cookie)
	}
}

// These windows stay hidden for the entire test. No user game, application
// window, foreground selection, keyboard event or real profile is modified.
func TestNativeHiddenHUDStylesFocusAndGDIRendering(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer nativeTestControls(t)()
	main, err := walk.NewMainWindow()
	if err != nil {
		t.Fatal(err)
	}
	defer main.Dispose()
	state := M{"settings": M{"overlayEnabled": true, "activity": "colonization"}, "connection": M{"gameRunning": false}, "commander": M{"ship": "Panther Clipper Mk II", "shipName": "Тест кириллицы"}, "theme": M{"settings": M{"overlayOpacity": 0.8}}, "colonization": M{"activeProject": M{"name": "Тестовая колония", "status": "building", "unknownBills": 2, "nextTask": M{"text": "Проверить строительное депо"}}}}
	u := &Window{main: main, state: state, backend: &platformController{state: state}}
	h, err := newHUD(u)
	if err != nil {
		t.Fatal(err)
	}
	defer h.close()
	u.hud = h
	style := uint32(win.GetWindowLong(h.hwnd, win.GWL_EXSTYLE))
	for _, required := range []uint32{win.WS_EX_TOOLWINDOW, win.WS_EX_NOACTIVATE, win.WS_EX_LAYERED, win.WS_EX_TRANSPARENT, win.WS_EX_TOPMOST} {
		if style&required == 0 {
			t.Errorf("missing extended style %#x in %#x", required, style)
		}
	}
	if h.visible || win.IsWindowVisible(h.hwnd) {
		t.Fatal("HUD appeared with gameRunning=false")
	}
	if result := hudWndProc(h.hwnd, win.WM_NCHITTEST, 0, 0); result != ^uintptr(0) {
		t.Fatalf("HUD intercepted mouse hit: %#x", result)
	}
	if result := hudWndProc(h.hwnd, win.WM_MOUSEACTIVATE, 0, 0); result != 3 {
		t.Fatalf("HUD requested activation: %d", result)
	}
	h.document = p.BuildHUD(state, time.Now())
	h.setFonts(96)
	if h.regular == 0 || h.bold == 0 {
		t.Fatal("Cyrillic HUD fonts could not be created")
	}
	hdc := win.GetDC(h.hwnd)
	height := h.layout(hdc, 410, false)
	win.ReleaseDC(h.hwnd, hdc)
	if height < 120 || height > 1500 {
		t.Fatalf("unexpected measured HUD height: %d", height)
	}
	h.key = win.COLORREF(0x00030201)
	win.SetWindowPos(h.hwnd, 0, 0, 0, 410, int32(height), win.SWP_NOZORDER|win.SWP_NOACTIVATE)
	h.paint()
	if win.IsWindowVisible(h.hwnd) {
		t.Fatal("offscreen GDI rendering showed the HUD")
	}
	h.close()
	h.close()
	if h.hwnd != 0 || h.hotkeyRegistered || h.foregroundHook != 0 || h.regular != 0 || h.bold != 0 {
		t.Fatal("HUD resource cleanup incomplete")
	}
	if _, exists := mainHUDWindows[main.Handle()]; exists {
		t.Fatal("window subclass leaked after close")
	}
}

func TestNativeHiddenWindowModeRoundTripAndPersistedBounds(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer nativeTestControls(t)()
	main, err := walk.NewMainWindow()
	if err != nil {
		t.Fatal(err)
	}
	defer main.Dispose()
	work, ok := monitorRect(main.Handle(), true)
	if !ok {
		t.Fatal("work area unavailable for hidden window test")
	}
	initial := fitWindowedBounds(walk.Rectangle{X: int(work.Left) + 100, Y: int(work.Top) + 100, Width: 1100, Height: 780}, work)
	if err := main.SetBoundsPixels(initial); err != nil {
		t.Fatal(err)
	}
	if got := main.BoundsPixels(); got != initial {
		t.Fatalf("in-workarea initial bounds rejected: expected=%+v actual=%+v work=%+v", initial, got, work)
	}
	backend := &platformController{state: M{}}
	u := &Window{main: main, backend: backend, state: M{"desktop": M{}}, modeValue: "windowed"}
	u.applyMode("fullscreen")
	if style := uint32(win.GetWindowLong(main.Handle(), win.GWL_STYLE)); style&uint32(win.WS_CAPTION) != 0 || style&uint32(win.WS_POPUP) == 0 {
		t.Fatalf("fullscreen style %#x", style)
	}
	if monitor, ok := monitorRect(main.Handle(), false); ok {
		var actual win.RECT
		win.GetWindowRect(main.Handle(), &actual)
		if actual != monitor {
			t.Fatalf("fullscreen bounds=%+v monitor=%+v", actual, monitor)
		}
	}
	u.applyMode("borderless")
	if work, ok := monitorRect(main.Handle(), true); ok {
		var actual win.RECT
		win.GetWindowRect(main.Handle(), &actual)
		if actual != work {
			t.Fatalf("borderless bounds=%+v work=%+v", actual, work)
		}
	}
	u.applyMode("windowed")
	if got := main.BoundsPixels(); got != initial {
		t.Fatalf("normal bounds lost: initial=%+v restored=%+v", initial, got)
	}
	if style := uint32(win.GetWindowLong(main.Handle(), win.GWL_STYLE)); style&uint32(win.WS_OVERLAPPEDWINDOW) != uint32(win.WS_OVERLAPPEDWINDOW) {
		t.Fatalf("window decorations not restored: %#x", style)
	}
	if win.IsWindowVisible(main.Handle()) {
		t.Fatal("mode changes activated/showed the hidden test window")
	}
	u.saveBounds()
	if backend.path != "/api/desktop" || v.Int(v.At(backend.last, "windowedBounds", "width")) != initial.Width || v.Int(v.At(backend.last, "windowedBounds", "height")) != initial.Height || v.Int(v.At(backend.last, "windowedBounds", "x")) != initial.X || v.Int(v.At(backend.last, "windowedBounds", "y")) != initial.Y || v.Bool(backend.last["maximized"]) {
		t.Fatalf("incorrect saved bounds: %#v", backend.last)
	}
	u.applyMode("")
	if u.modeValue != "windowed" {
		t.Fatal("bounds-only persistence changed display mode")
	}
	// A profile saved on a larger or disconnected monitor must restore to the
	// current usable desktop, not the OS-specific maximum tracking rectangle.
	u.applyMode("fullscreen")
	work, ok = monitorRect(main.Handle(), true)
	if !ok {
		t.Fatal("work area unavailable during oversize restore")
	}
	u.normalBounds = walk.Rectangle{X: int(work.Left) - 5000, Y: int(work.Top) - 5000, Width: int(work.Right-work.Left) * 3, Height: int(work.Bottom-work.Top) * 3}
	u.applyMode("windowed")
	want := walk.Rectangle{X: int(work.Left), Y: int(work.Top), Width: int(work.Right - work.Left), Height: int(work.Bottom - work.Top)}
	if got := main.BoundsPixels(); got != want {
		t.Fatalf("oversized/offscreen window not clamped to work area: expected=%+v actual=%+v", want, got)
	}
	if u.normalBounds != want {
		t.Fatalf("normal bounds retained oversize profile: %+v", u.normalBounds)
	}
	u.saveBounds()
	if got := v.Obj(backend.last["windowedBounds"]); v.Int(got["x"]) != want.X || v.Int(got["y"]) != want.Y || v.Int(got["width"]) != want.Width || v.Int(got["height"]) != want.Height {
		t.Fatalf("clamped bounds not persisted: %#v", got)
	}
	if win.IsWindowVisible(main.Handle()) {
		t.Fatal("oversize restore exposed the hidden test window")
	}
}

func TestWindowedBoundsFitWorkArea(t *testing.T) {
	for _, tc := range []struct {
		name   string
		bounds walk.Rectangle
		work   win.RECT
		want   walk.Rectangle
	}{
		{"unchanged", walk.Rectangle{X: 40, Y: 40, Width: 1000, Height: 720}, win.RECT{Left: 0, Top: 0, Right: 1920, Bottom: 1040}, walk.Rectangle{X: 40, Y: 40, Width: 1000, Height: 720}},
		{"runner1024", walk.Rectangle{X: 100, Y: 100, Width: 1100, Height: 780}, win.RECT{Left: 0, Top: 0, Right: 1024, Bottom: 728}, walk.Rectangle{X: 0, Y: 0, Width: 1024, Height: 728}},
		{"negative monitor", walk.Rectangle{X: -5000, Y: -5000, Width: 4000, Height: 2000}, win.RECT{Left: -1280, Top: 40, Right: 0, Bottom: 760}, walk.Rectangle{X: -1280, Y: 40, Width: 1280, Height: 720}},
		{"right bottom offscreen", walk.Rectangle{X: 10000, Y: 10000, Width: 1000, Height: 710}, win.RECT{Left: -1280, Top: 40, Right: 0, Bottom: 760}, walk.Rectangle{X: -1000, Y: 50, Width: 1000, Height: 710}},
		{"below minimum desktop", walk.Rectangle{X: 50, Y: 50, Width: 1100, Height: 780}, win.RECT{Left: 100, Top: -500, Right: 900, Bottom: 100}, walk.Rectangle{X: 100, Y: -500, Width: 800, Height: 600}},
		{"minimum normal dimensions", walk.Rectangle{X: 0, Y: 0, Width: 20, Height: 20}, win.RECT{Left: 0, Top: 0, Right: 1920, Bottom: 1040}, walk.Rectangle{X: 0, Y: 0, Width: 900, Height: 700}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := fitWindowedBounds(tc.bounds, tc.work); got != tc.want {
				t.Fatalf("fit=%+v expected=%+v", got, tc.want)
			}
		})
	}
}

func TestExternalURLRejectsCommandsWithoutLaunching(t *testing.T) {
	for _, raw := range []string{"", "file:///C:/Windows/System32/cmd.exe", "javascript:alert(1)", "edcopilot://private", "https://user:password@example.com/", "https:///missing-host"} {
		if openExternal(raw) == nil {
			t.Fatalf("accepted non-browser URL %q", raw)
		}
	}
}
