//go:build windows

package nativeui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/Naenier/ED-Copilot/internal/frontend"
	"github.com/Naenier/ED-Copilot/internal/localization"
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"github.com/wailsapp/go-webview2/pkg/edge"
	"github.com/wailsapp/go-webview2/webviewloader"
)

type modernShell struct {
	owner            *Window
	view             *edge.Chromium
	core             *edge.ICoreWebView2
	document         string
	ready, closed    bool
	focusing         bool
	focusQueued      bool
	resizeQueued     bool
	positionQueued   bool
	visibilityQueued bool
	browserVisible   bool
	uiPost           func(func())
	snapshotMu       sync.Mutex
	pendingState     M
	stateQueued      bool
	hudQueued        bool
	actionMu         sync.Mutex
	desktopMu        sync.Mutex
	desktopPending   M
	desktopDone      chan struct{}
	desktopSaving    bool
	desiredMode      string
	smokeOnce        sync.Once
	smokeFirstView   M
	smokeAfterUI     M
	soak             *nativeSoak
	lastSummary      string
	language         string
	diagnostics      int
	captures         []*modernPreviewCapture
}

func newModernWindow(backend Controller, options Options) (*Window, error) {
	if _, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString(""); err != nil {
		return nil, errors.New("Для интерфейса нужен Microsoft Edge WebView2. Запустите установщик ED Copilot — он предложит добавить этот компонент Windows.")
	}
	u := &Window{backend: backend, options: options, page: "overview", modes: map[string]string{}, queries: map[string]string{}, prefill: M{}, stop: make(chan struct{}), refreshRequested: make(chan struct{}, 1)}
	u.state = nativeHUDSnapshot(backend)
	var err error
	u.main, err = walk.NewMainWindow()
	if err != nil {
		return nil, fmt.Errorf("Создание основного окна: %w", err)
	}
	u.font, _ = walk.NewFont("Segoe UI", 10, 0)
	u.main.SetFont(u.font)
	_ = u.main.SetTitle("ED Copilot · Elite Dangerous")
	if options.Smoke || options.Demo || options.Soak != nil {
		_ = u.main.SetTitle("ED Copilot · Демонстрация")
	}
	_ = u.main.SetSize(walk.Size{Width: 1440, Height: 940})
	_ = u.main.SetMinMaxSize(walk.Size{Width: 900, Height: 700}, walk.Size{})
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{})
	_ = u.main.SetLayout(layout)
	background, _ := walk.NewSolidColorBrush(walk.RGB(25, 25, 25))
	u.brushes = append(u.brushes, background)
	u.main.SetBackground(background)
	if icon, e := walk.NewIconFromResource("APP"); e == nil {
		_ = u.main.SetIcon(icon)
	}
	u.modern = &modernShell{owner: u, uiPost: u.main.Synchronize}
	if options.Soak != nil {
		u.modern.soak = &nativeSoak{webReady: make(chan struct{}), dpi: u.main.DPI()}
	}
	u.hud, err = newHUD(u)
	if err != nil {
		u.dispose()
		return nil, fmt.Errorf("Создание игрового оверлея: %w", err)
	}
	// Keep the native resizing, snapping and system menu, and paint our own
	// non-client header. Windows still owns the window lifecycle and frame.
	var corner int32 = 2
	if dwmSetWindowAttribute.Find() == nil {
		dwmSetWindowAttribute.Call(uintptr(u.main.Handle()), 33, uintptr(unsafe.Pointer(&corner)), 4)
	}
	win.SetWindowPos(u.main.Handle(), 0, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)
	u.main.Closing().Attach(func(cancelled *bool, reason walk.CloseReason) {
		if !u.quitting {
			*cancelled = true
			u.saveBounds()
			u.main.Hide()
		}
	})
	u.tray, err = walk.NewNotifyIcon(u.main)
	if err != nil {
		u.dispose()
		return nil, fmt.Errorf("Создание значка в области уведомлений: %w", err)
	}
	_ = u.tray.SetIcon(u.main.Icon())
	_ = u.tray.SetToolTip("ED Copilot · помощник командира")
	show := walk.NewAction()
	_ = show.SetText("Открыть ED Copilot")
	show.Triggered().Attach(func() { u.main.Show(); u.main.SetFocus() })
	_ = u.tray.ContextMenu().Actions().Add(show)
	hud := walk.NewAction()
	_ = hud.SetText("Показать / скрыть оверлей")
	hud.Triggered().Attach(func() {
		u.execute(p.Action{Path: "/api/settings", Payload: M{"overlayEnabled": !v.Bool(v.At(u.state, "settings", "overlayEnabled"))}}, nil)
	})
	_ = u.tray.ContextMenu().Actions().Add(hud)
	exit := walk.NewAction()
	_ = exit.SetText("Выход")
	exit.Triggered().Attach(u.quit)
	_ = u.tray.ContextMenu().Actions().Add(exit)
	u.tray.MouseDown().Attach(func(x, y int, b walk.MouseButton) {
		if b == walk.LeftButton {
			u.main.Show()
			u.main.SetFocus()
		}
	})
	_ = u.tray.SetVisible(true)
	u.applyNativeLanguage()
	browser := edge.NewChromium()
	u.modern.view = browser
	browser.DataPath = filepath.Join(backend.DataDir(), "WebView2")
	browser.SetGlobalPermission(edge.CoreWebView2PermissionStateDeny)
	browser.SetErrorCallback(func(err error) {
		wrapped := fmt.Errorf("Инициализация или работа WebView2: %w", err)
		nativeErrorLog(wrapped, backend.DataDir())
		fmt.Fprintln(os.Stderr, wrapped)
	})
	browser.MessageCallback = u.modern.message
	// F11 is handled once by the embedded document (preventDefault + the
	// window.toggle-fullscreen bridge command), not by two keyboard handlers.
	// Walk's actual client area is an opaque child Composite. The WebView
	// must be its child rather than a sibling below that panel.
	if !browser.Embed(uintptr(u.main.AsContainerBase().Handle())) {
		u.dispose()
		return nil, errors.New("Не удалось открыть интерфейс ED Copilot")
	}
	u.modern.core, err = browser.GetController().GetCoreWebView2()
	if err != nil {
		u.dispose()
		return nil, fmt.Errorf("Получение WebView2 Core: %w", err)
	}
	browser.NavigationCompletedCallback = func(sender *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
		if u.modern.closed {
			return
		}
		if source, e := sender.GetSource(); e != nil || !trustedWebSource(source) {
			u.modern.ready = false
			u.modern.postUI(func() { browser.NavigateToString(u.modern.document) })
		}
	}
	browser.SetBackgroundColour(25, 25, 25, 255)
	if settings, e := browser.GetSettings(); e == nil {
		_ = settings.PutIsStatusBarEnabled(false)
		_ = settings.PutAreDefaultContextMenusEnabled(false)
		_ = settings.PutAreDevToolsEnabled(false)
		_ = settings.PutIsZoomControlEnabled(false)
		_ = settings.PutAreHostObjectsAllowed(false)
	}
	_ = browser.PutIsGeneralAutofillEnabled(false)
	_ = browser.PutIsPasswordAutosaveEnabled(false)
	var nonce [20]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		u.dispose()
		return nil, err
	}
	u.modern.document = frontend.HTMLMode(hex.EncodeToString(nonce[:]), options.Smoke, options.ReadmeScreenshots)
	browser.NavigateToString(u.modern.document)
	browser.Resize()
	if err := browser.Show(); err != nil {
		u.dispose()
		return nil, fmt.Errorf("Отображение WebView2: %w", err)
	}
	u.modern.browserVisible = true
	u.main.AsContainerBase().SizeChanged().Attach(func() {
		u.modern.resizeBrowser()
	})
	go u.watch()
	return u, nil
}

func (m *modernShell) close() {
	if m.closed {
		return
	}
	m.closed = true
	for _, capture := range m.captures {
		capture.closeStream()
	}
	if m.view != nil {
		m.view.ShuttingDown()
		if m.core != nil {
			m.core.Release()
			m.core = nil
		}
		if controller := m.view.GetController(); controller != nil {
			_ = controller.PutIsVisible(false)
			controller.Release()
		}
	}
}

var copyNativeFrameRect = kernelDLL.NewProc("RtlMoveMemory")

func (m *modernShell) focusBrowser() {
	if m.closed || m.focusing || m.focusQueued || m.view == nil || m.view.GetController() == nil {
		return
	}
	m.focusQueued = true
	m.postUI(func() {
		m.focusQueued = false
		if m.focusing || m.view == nil || m.view.GetController() == nil {
			return
		}
		// A delayed WM_SETFOCUS can arrive after WebView already received
		// focus. Repeating MoveFocus here would create a focus-message storm.
		host, focused := m.owner.main.AsContainerBase().Handle(), win.GetFocus()
		if focused != 0 && focused != host && win.IsChild(host, focused) {
			return
		}
		m.focusing = true
		defer func() { m.focusing = false }()
		_ = m.view.GetController().MoveFocus(edge.COREWEBVIEW2_MOVE_FOCUS_REASON_PROGRAMMATIC)
	})
}

// WebView2 serializes its STA event handlers. A nested Win32 move/resize loop,
// modal dialog or focus callback must start after WebMessageReceived returns.
// Synchronize posts work to the UI message pump; it never runs it inline.
func (m *modernShell) postUI(work func()) {
	if m.owner.disposed.Load() {
		return
	}
	post := m.uiPost
	if post == nil {
		post = m.owner.main.Synchronize
	}
	post(func() {
		if !m.closed && !m.owner.disposed.Load() {
			work()
		}
	})
}

func (m *modernShell) resizeBrowser() {
	if m.closed || m.resizeQueued || m.view == nil || m.view.GetController() == nil {
		return
	}
	m.resizeQueued = true
	m.postUI(func() {
		m.resizeQueued = false
		if m.view != nil && m.view.GetController() != nil {
			m.view.Resize()
			m.logGeometry("client-size")
		}
	})
}

func (m *modernShell) notifyBrowserPosition() {
	if m.closed || m.positionQueued || m.view == nil || m.view.GetController() == nil {
		return
	}
	m.positionQueued = true
	m.postUI(func() {
		m.positionQueued = false
		if m.view != nil && m.view.GetController() != nil {
			_ = m.view.NotifyParentWindowPositionChanged()
		}
	})
}

func (m *modernShell) syncBrowserVisibility() {
	if m.closed || m.visibilityQueued || m.view == nil || m.view.GetController() == nil {
		return
	}
	m.visibilityQueued = true
	m.postUI(func() {
		m.visibilityQueued = false
		visible := win.IsWindowVisible(m.owner.main.Handle()) && !win.IsIconic(m.owner.main.Handle())
		if m.view != nil && m.view.GetController() != nil && visible != m.browserVisible {
			if err := m.view.GetController().PutIsVisible(visible); err == nil {
				m.browserVisible = visible
			}
		}
	})
}

type modernSnapshotGate struct {
	dirty bool
	last  time.Time
}

func (g *modernSnapshotGate) due(now time.Time) bool {
	if now.Sub(g.last) < 400*time.Millisecond || (!g.dirty && now.Sub(g.last) < 5*time.Second) {
		return false
	}
	g.dirty, g.last = false, now
	return true
}

func (m *modernShell) watch() {
	u := m.owner
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	// The initial snapshot precedes browser creation. Refresh it once on the
	// first tick, then remain idle until a real update or the slow fallback.
	gate := modernSnapshotGate{dirty: true, last: time.Now().Add(-400 * time.Millisecond)}
	for {
		select {
		case <-u.stop:
			return
		case <-u.backend.Updates():
			gate.dirty = true
			continue
		case <-u.refreshRequested:
			gate.dirty = true
			continue
		case now := <-ticker.C:
			if u.disposed.Load() {
				return
			}
			if gate.due(now) {
				state := attachRouteClipboard(nativeHUDSnapshot(u.backend), u.backend)
				if u.disposed.Load() {
					return
				}
				m.postSnapshot(state)
			}
			m.postHUDRefresh()
		}
	}
}

func (m *modernShell) postHUDRefresh() {
	m.snapshotMu.Lock()
	if m.hudQueued {
		m.snapshotMu.Unlock()
		return
	}
	m.hudQueued = true
	m.snapshotMu.Unlock()
	m.postUI(func() {
		m.snapshotMu.Lock()
		m.hudQueued = false
		m.snapshotMu.Unlock()
		if m.owner.hud != nil {
			m.owner.hud.refresh()
		}
	})
}

func (m *modernShell) executeAction(action p.Action, body M) {
	go func() {
		m.actionMu.Lock()
		result, err := m.owner.backend.Do(action.Path, body)
		delete(body, "apiKey")
		m.actionMu.Unlock()
		m.postUI(func() {
			if err != nil {
				m.send("notice", M{"message": err.Error()})
				return
			}
			if raw := v.Str(result["url"]); raw != "" {
				if err := openExternal(raw); err != nil {
					m.send("notice", M{"message": err.Error()})
				}
			}
			m.owner.refreshModern()
		})
	}()
}

// Geometry diagnostics belong only to disposable demo/smoke profiles. They do
// not record commander data, credentials, file contents or external addresses.
func (m *modernShell) logGeometry(phase string) {
	if (!m.owner.options.Demo && !m.owner.options.Smoke) || m.closed || m.diagnostics >= 4 || m.view == nil || m.view.GetController() == nil {
		return
	}
	m.diagnostics++
	u := m.owner
	host := u.main.AsContainerBase().Handle()
	var parentRect, hostRect win.RECT
	win.GetClientRect(u.main.Handle(), &parentRect)
	win.GetClientRect(host, &hostRect)
	bounds, err := m.view.GetController().GetBounds()
	detail := M{"phase": phase, "mainClient": parentRect, "hostClient": hostRect, "controllerBounds": bounds, "mainVisible": win.IsWindowVisible(u.main.Handle()), "hostVisible": win.IsWindowVisible(host)}
	if err != nil {
		detail["boundsError"] = err.Error()
	}
	if m.core != nil {
		source, err := m.core.GetSource()
		detail["embeddedDocument"] = err == nil && trustedWebSource(source)
	}
	if raw, err := json.Marshal(detail); err == nil {
		fmt.Fprintf(os.Stderr, "[ED Copilot WebView geometry] %s\n", raw)
	}
}

func (m *modernShell) windowMessage(hwnd win.HWND, msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	if msg == win.WM_NCCALCSIZE && wParam != 0 {
		if lParam != 0 && win.IsZoomed(hwnd) && m.owner.modeValue != "fullscreen" && m.owner.modeValue != "borderless" {
			if work, ok := monitorRect(hwnd, true); ok {
				// WM_NCCALCSIZE supplies a native NCCALCSIZE_PARAMS pointer;
				// its first member is the RECT of the proposed client area.
				// Copy through Win32 instead of treating a foreign uintptr as
				// Go-managed memory, so vet/checkptr remain useful in our code.
				copyNativeFrameRect.Call(lParam, uintptr(unsafe.Pointer(&work)), unsafe.Sizeof(work))
			}
		}
		return 0, true
	}
	if msg == win.WM_SETFOCUS {
		m.focusBrowser()
	}
	if msg == win.WM_MOVE || msg == win.WM_DPICHANGED {
		m.notifyBrowserPosition()
	}
	if msg == win.WM_SIZE {
		m.resizeBrowser()
	}
	if msg == win.WM_SIZE || msg == win.WM_SHOWWINDOW || msg == win.WM_ACTIVATE {
		m.syncBrowserVisibility()
	}
	if msg == win.WM_NCHITTEST && !win.IsZoomed(hwnd) && m.owner.modeValue != "fullscreen" && m.owner.modeValue != "borderless" {
		x, y := int(int16(uint16(lParam))), int(int16(uint16(lParam>>16)))
		var r win.RECT
		win.GetWindowRect(hwnd, &r)
		border := max(5, int(win.GetDpiForWindow(hwnd))*6/96)
		left, right, top, bottom := x < int(r.Left)+border, x >= int(r.Right)-border, y < int(r.Top)+border, y >= int(r.Bottom)-border
		switch {
		case top && left:
			return 13, true
		case top && right:
			return 14, true
		case bottom && left:
			return 16, true
		case bottom && right:
			return 17, true
		case left:
			return 10, true
		case right:
			return 11, true
		case top:
			return 12, true
		case bottom:
			return 15, true
		}
	}
	return 0, false
}

func (u *Window) refreshModern() {
	// Snapshot can wait for journal/domain locks and must not run on the
	// WebView STA. The watch worker coalesces requests and posts its result.
	if u.disposed.Load() {
		return
	}
	select {
	case u.refreshRequested <- struct{}{}:
	default:
	}
}

func (m *modernShell) postSnapshot(state M) {
	if m.soak != nil {
		m.soak.snapshot(v.Str(v.At(state, "commander", "system")))
	}
	m.snapshotMu.Lock()
	m.pendingState = state
	if m.stateQueued {
		m.snapshotMu.Unlock()
		return
	}
	m.stateQueued = true
	m.snapshotMu.Unlock()
	m.postUI(func() {
		m.snapshotMu.Lock()
		next := m.pendingState
		m.pendingState = nil
		m.stateQueued = false
		m.snapshotMu.Unlock()
		if next != nil {
			m.owner.applyModernSnapshot(next)
		}
	})
}

func (u *Window) applyModernSnapshot(state M) {
	u.state = state
	u.applyNativeLanguage()
	u.syncRouteClipboard(state)
	if u.dialogOpen {
		if u.hud != nil {
			u.hud.refresh()
		}
		return
	}
	mode := v.Text(v.At(u.state, "desktop", "displayMode"), "windowed")
	if desired := u.modern.desiredMode; desired != "" {
		if mode == desired {
			u.modern.desiredMode = ""
		} else {
			mode = desired
		}
	}
	if u.modeValue != mode {
		u.applyMode(mode)
	}
	if u.hud != nil {
		u.hud.refresh()
	}
	if u.modern.ready && !u.modern.closed && win.IsWindowVisible(u.main.Handle()) && !win.IsIconic(u.main.Handle()) {
		update := M{"revision": u.state["revision"], "connection": u.state["connection"], "settings": u.state["settings"], "commander": u.state["commander"]}
		if v.At(u.state, "settings", "activity") == "colonization" {
			update["overlayColonies"] = overlayColonyUpdate(u.state)
		}
		u.modern.send("update", update)
	}
}

func (u *Window) applyNativeLanguage() {
	if u.modern == nil {
		return
	}
	language := p.Language(u.state)
	if u.modern.language == language {
		return
	}
	u.modern.language = language
	if u.tray != nil {
		_ = u.tray.SetToolTip(localization.T(language, "ED Copilot · помощник командира"))
		actions := u.tray.ContextMenu().Actions()
		for i, text := range []string{"Открыть ED Copilot", "Показать / скрыть оверлей", "Выход"} {
			if i < actions.Len() {
				_ = actions.At(i).SetText(localization.T(language, text))
			}
		}
	}
	if u.options.Demo || u.options.Smoke {
		_ = u.main.SetTitle(localization.T(language, "ED Copilot · Демонстрация"))
	}
}

func (m *modernShell) send(kind string, payload any) {
	wire, err := encodeWebEvent(kind, payload)
	if err == nil {
		m.sendEncoded(wire)
	}
}

func encodeWebEvent(kind string, payload any) (string, error) {
	wire, err := json.Marshal(redactWebSecrets(M{"event": kind, "data": payload}))
	return string(wire), err
}

func (m *modernShell) sendEncoded(wire string) {
	if m.closed || m.owner.disposed.Load() || m.view == nil || m.view.GetController() == nil || m.core == nil {
		return
	}
	if source, err := m.core.GetSource(); err != nil || !trustedWebSource(source) {
		return
	}
	m.view.Eval("window.copilotReceive&&window.copilotReceive(" + wire + ")")
}

type webRequest struct {
	ID     string `json:"id"`
	Op     string `json:"op"`
	Params M      `json:"params"`
}

func trustedWebSource(source string) bool { return strings.SplitN(source, "#", 2)[0] == "about:blank" }

func decodeWebRequest(source, raw string) (webRequest, error) {
	var req webRequest
	if !trustedWebSource(source) || len(raw) > 2*1024*1024 {
		return req, errors.New("Недопустимый источник интерфейса")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&req); err != nil {
		return req, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return req, errors.New("Некорректный запрос интерфейса")
	}
	if len(req.ID) == 0 || len(req.ID) > 80 || len(req.Op) == 0 || len(req.Op) > 64 {
		return req, errors.New("Некорректный запрос интерфейса")
	}
	if req.Params == nil {
		req.Params = M{}
	}
	return req, nil
}

func (m *modernShell) message(raw string, sender *edge.ICoreWebView2, args *edge.ICoreWebView2WebMessageReceivedEventArgs) {
	source, err := args.GetSource()
	if err != nil || m.closed || m.owner.disposed.Load() {
		return
	}
	if m.core == nil {
		return
	}
	if current, e := m.core.GetSource(); e != nil || !trustedWebSource(current) {
		return
	}
	req, err := decodeWebRequest(source, raw)
	if err != nil {
		return
	}
	m.postUI(func() { m.handleRequest(req) })
}

func (m *modernShell) handleRequest(req webRequest) {
	if m.owner.quitting {
		return
	}
	if m.soak != nil && req.Op == "view" {
		m.soak.readyOnce.Do(func() { close(m.soak.webReady) })
	}
	if req.Op == "window" {
		result, e := m.windowAction(v.Str(req.Params["action"]))
		m.reply(req.ID, result, e)
		return
	}
	if req.Op == "smoke-result" && m.owner.options.Smoke {
		m.receiveSmoke(req.Params)
		m.reply(req.ID, M{"ok": true}, nil)
		return
	}
	if req.Op == "smoke-performance" && m.owner.options.Smoke && m.owner.options.SmokeFixture != nil {
		switch v.Str(req.Params["checkpoint"]) {
		case "firstView":
			m.recordSmokeFirstView()
		case "afterInteractions":
			if m.smokeAfterUI == nil {
				m.smokeAfterUI = smokeMemory()
			}
		}
		m.reply(req.ID, M{"ok": true}, nil)
		return
	}
	if req.Op == "smoke-capture" {
		if err := m.capturePreview(req.ID, v.Str(req.Params["name"])); err != nil {
			m.reply(req.ID, nil, err)
		}
		return
	}
	if req.Op == "smoke-viewport" {
		result, err := m.smokeViewport(req.Params["compact"])
		m.reply(req.ID, result, err)
		return
	}
	if req.Op == "native" {
		if m.owner.dialogOpen {
			m.reply(req.ID, nil, errors.New("Сначала закройте открытое системное окно"))
			return
		}
		result, e := m.nativeAction(req.Params)
		if work, ok := result.(func() (M, error)); e == nil && ok {
			m.runAsyncResponse(req.ID, work)
			return
		}
		m.reply(req.ID, result, e)
		return
	}
	if req.Op == "system-search" || req.Op == "system-search-cancel" || req.Op == "exo.sites" || req.Op == "engineering-roadmap" {
		m.runReadOnlyResponse(req.ID, func() (M, error) { return dispatchWeb(m.owner.backend, req.Op, req.Params) }, webInputIdentities(req.Params)...)
		return
	}
	m.runAsyncResponse(req.ID, func() (M, error) { return dispatchWeb(m.owner.backend, req.Op, req.Params) }, webInputIdentities(req.Params)...)
}

func (m *modernShell) runAsyncResponse(id string, work func() (M, error), identities ...string) {
	m.runResponse(id, work, true, identities...)
}

func (m *modernShell) runReadOnlyResponse(id string, work func() (M, error), identities ...string) {
	m.runResponse(id, work, false, identities...)
}

func (m *modernShell) runResponse(id string, work func() (M, error), refresh bool, identities ...string) {
	language := "ru"
	if provider, ok := m.owner.backend.(interface{ Language() string }); ok {
		language = provider.Language()
	}
	go func() {
		result, e := work()
		if e != nil {
			e = errors.New(localization.TextProtected(language, e.Error(), identities))
		}
		packet := webResponse(id, result, e)
		wire, marshalErr := encodeWebEvent("response", packet)
		if marshalErr != nil {
			wire, _ = encodeWebEvent("response", webResponse(id, nil, marshalErr))
		}
		if m.owner.disposed.Load() {
			return
		}
		m.postUI(func() {
			m.sendEncoded(wire)
			if e == nil && refresh {
				m.owner.refreshModern()
			}
		})
	}()
}

func (m *modernShell) smokeViewport(raw any) (M, error) {
	compact, valid := raw.(bool)
	if !m.owner.options.Smoke || !valid || m.closed {
		return nil, errors.New("Размер проверочного окна доступен только в изолированной проверке")
	}
	size := walk.Size{Width: 1440, Height: 940}
	if compact {
		size = walk.Size{Width: 900, Height: 700}
	}
	if win.IsZoomed(m.owner.main.Handle()) || win.IsIconic(m.owner.main.Handle()) {
		win.ShowWindow(m.owner.main.Handle(), win.SW_RESTORE)
	}
	if err := m.owner.main.SetSize(size); err != nil {
		return nil, err
	}
	return M{"ok": true, "compact": compact, "width": size.Width, "height": size.Height}, nil
}
func (m *modernShell) reply(id string, result any, err error) {
	m.send("response", webResponse(id, result, err))
}

func webResponse(id string, result any, err error) M {
	r := M{"id": id, "result": result}
	if err != nil {
		r["error"] = err.Error()
	}
	return r
}

func (m *modernShell) windowAction(action string) (M, error) {
	u := m.owner
	switch action {
	case "status":
		return M{"ok": true, "maximized": win.IsZoomed(u.main.Handle()), "minimized": win.IsIconic(u.main.Handle()), "displayMode": u.modeValue}, nil
	case "drag":
		if state, _, _ := userDLL.NewProc("GetAsyncKeyState").Call(win.VK_LBUTTON); uint16(state)&0x8000 == 0 {
			return M{"ok": true, "cancelled": true}, nil
		}
		win.ReleaseCapture()
		var cursor win.POINT
		win.GetCursorPos(&cursor)
		win.SendMessage(u.main.Handle(), win.WM_NCLBUTTONDOWN, 2, uintptr(uint16(cursor.X))|uintptr(uint16(cursor.Y))<<16)
	case "minimize":
		win.ShowWindow(u.main.Handle(), win.SW_MINIMIZE)
	case "restore":
		win.ShowWindow(u.main.Handle(), win.SW_RESTORE)
		m.focusBrowser()
		u.saveBounds()
	case "maximize":
		if win.IsZoomed(u.main.Handle()) {
			win.ShowWindow(u.main.Handle(), win.SW_RESTORE)
		} else {
			win.ShowWindow(u.main.Handle(), win.SW_MAXIMIZE)
		}
		u.saveBounds()
	case "hide":
		u.saveBounds()
		u.main.Hide()
	case "quit":
		go func() { time.Sleep(40 * time.Millisecond); u.main.Synchronize(u.quit) }()
	case "fullscreen", "windowed", "borderless":
		return m.changeWindowMode(action)
	case "toggle-fullscreen":
		mode := "fullscreen"
		if u.modeValue == mode {
			mode = "windowed"
		}
		return m.changeWindowMode(mode)
	case "ready":
		m.ready = true
		m.focusBrowser()
		m.logGeometry("document-ready")
	default:
		return nil, errors.New("Неизвестная команда окна")
	}
	return M{"ok": true, "maximized": win.IsZoomed(u.main.Handle())}, nil
}

func nativeHUDSnapshot(backend Controller) M {
	if scoped, ok := backend.(interface{ SnapshotForHUD() M }); ok {
		return scoped.SnapshotForHUD()
	}
	return backend.Snapshot()
}

func (m *modernShell) changeWindowMode(mode string) (M, error) {
	if mode != "fullscreen" && mode != "windowed" && mode != "borderless" {
		return nil, errors.New("Неизвестный режим окна")
	}
	u := m.owner
	u.saveBounds()
	u.applyMode(mode)
	m.desiredMode = mode
	body := u.boundsPayload()
	if body == nil {
		body = M{}
	}
	body["displayMode"] = mode
	m.persistDesktop(body)
	return M{"ok": true, "displayMode": mode, "maximized": win.IsZoomed(u.main.Handle())}, nil
}

// Geometry and mode changes are captured on the UI thread, while persistence
// runs serially in a worker. Rapid window actions replace pending values instead
// of queueing unbounded Save/Snapshot calls on the WebView message pump.
func (m *modernShell) persistDesktop(body M) {
	if m.owner.disposed.Load() {
		return
	}
	m.desktopMu.Lock()
	if m.desktopPending == nil {
		m.desktopPending = M{}
	}
	v.Merge(m.desktopPending, body)
	if m.desktopSaving {
		m.desktopMu.Unlock()
		return
	}
	m.desktopSaving = true
	m.desktopDone = make(chan struct{})
	m.desktopMu.Unlock()
	go func() {
		for {
			m.desktopMu.Lock()
			next := m.desktopPending
			m.desktopPending = nil
			if next == nil {
				m.desktopSaving = false
				close(m.desktopDone)
				m.desktopMu.Unlock()
				return
			}
			m.desktopMu.Unlock()
			if _, err := m.owner.backend.Do("/api/desktop", next); err != nil {
				m.postUI(func() {
					if v.Str(next["displayMode"]) == m.desiredMode {
						m.desiredMode = ""
					}
					m.send("notice", M{"message": err.Error()})
				})
			}
			m.owner.refreshModern()
		}
	}()
}

func (m *modernShell) flushDesktop() {
	m.desktopMu.Lock()
	done := m.desktopDone
	m.desktopMu.Unlock()
	if done != nil {
		<-done
	}
}

func (m *modernShell) nativeAction(params M) (any, error) {
	u := m.owner
	u.dialogOpen = true
	defer func() { u.dialogOpen = false }()
	switch v.Str(params["action"]) {
	case "profile-export":
		return u.profileExportAction()
	case "profile-import":
		return u.profileImportAction()
	case "open":
		return M{"ok": true}, openExternal(v.Str(params["url"]))
	case "copy":
		return M{"ok": true}, walk.Clipboard().SetText(v.Str(params["text"]))
	case "folder":
		d := walk.FileDialog{Title: localization.T(p.Language(u.state), "Папка игровых журналов"), FilePath: v.Str(v.At(u.state, "connection", "journalDir"))}
		ok, e := d.ShowBrowseFolder(u.main)
		if e != nil {
			return nil, e
		}
		if !ok {
			return M{"cancelled": true}, nil
		}
		return func() (M, error) { return u.backend.Do("/api/journal-dir", M{"path": d.FilePath}) }, nil
	case "import-plan":
		d := walk.FileDialog{Title: localization.T(p.Language(u.state), "План инженерных улучшений"), Filter: "JSON (*.json)|*.json"}
		ok, e := d.ShowOpen(u.main)
		if e != nil {
			return nil, e
		}
		if !ok {
			return M{"cancelled": true}, nil
		}
		return func() (M, error) {
			file, e := os.Open(d.FilePath)
			if e != nil {
				return nil, e
			}
			defer file.Close()
			plan, e := readWebEngineeringPlan(file)
			if e != nil {
				return nil, e
			}
			return u.backend.Do("/api/engineering", M{"action": "import", "items": plan["items"]})
		}, nil
	case "export":
		kind := v.Str(params["kind"])
		asJSON := v.Bool(params["json"])
		extension := "csv"
		if asJSON {
			extension = "json"
		}
		d := walk.FileDialog{Title: localization.T(p.Language(u.state), "Сохранить план или таблицу"), FilePath: "ED Copilot." + extension, Filter: strings.ToUpper(extension) + " (*." + extension + ")|*." + extension}
		ok, e := d.ShowSave(u.main)
		if e != nil {
			return nil, e
		}
		if !ok {
			return M{"cancelled": true}, nil
		}
		return func() (M, error) {
			var content []byte
			if asJSON {
				if exp, ok := u.backend.(interface {
					ExportPlanFor(string, string) ([]byte, error)
				}); ok {
					content, e = exp.ExportPlanFor(kind, v.Str(params["projectId"]))
				} else {
					content, e = u.backend.ExportPlan(kind)
				}
			} else {
				var text string
				if exp, ok := u.backend.(interface {
					ExportCSVFor(string, string) (string, error)
				}); ok {
					text, e = exp.ExportCSVFor(kind, v.Str(params["projectId"]))
				} else {
					text, e = u.backend.ExportCSV(kind)
				}
				content = []byte(text)
			}
			if e == nil {
				e = os.WriteFile(d.FilePath, content, 0600)
			}
			return M{"ok": e == nil, "file": filepath.Base(d.FilePath)}, e
		}, nil
	}
	return nil, errors.New("Неизвестное действие приложения")
}

func readWebEngineeringPlan(reader io.Reader) (M, error) {
	content, err := io.ReadAll(io.LimitReader(reader, 2*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(content) > 2*1024*1024 {
		return nil, errors.New("План превышает 2 МБ")
	}
	var plan M
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.UseNumber()
	if decoder.Decode(&plan) != nil {
		return nil, errors.New("Не удалось прочитать план JSON")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("Не удалось прочитать план JSON")
	}
	if _, ok := plan["items"].([]any); !ok {
		return nil, errors.New("В файле отсутствует список улучшений items")
	}
	return plan, nil
}

func (m *modernShell) receiveSmoke(report M) {
	if !m.owner.options.Smoke {
		return
	}
	m.smokeOnce.Do(func() { m.finishSmoke(report) })
}
func (m *modernShell) finishSmoke(report M) {
	report = v.Copy(report)
	report["renderer"] = "WebView2 · Go backend · native Win32 HUD"
	report["version"] = v.At(m.owner.state, "app", "version")
	report["networkDisabled"] = true
	if m.owner.options.SmokeFixture != nil {
		report["performance"] = m.smokePerformance(report["interactionTimings"])
	}
	m.smokeVisualState(report)
	if m.owner.hud != nil {
		m.owner.hud.refresh()
		report["hudHidden"] = !win.IsWindowVisible(m.owner.hud.hwnd) && !win.IsWindowVisible(m.owner.hud.leftHwnd)
		if !v.Bool(report["hudHidden"]) {
			report["ok"] = false
			report["problems"] = append(v.Arr(report["problems"]), "Оверлей виден при фокусе приложения")
		}
	}
	path := m.owner.options.SmokeReport
	go func() {
		time.Sleep(250 * time.Millisecond)
		if m.owner.disposed.Load() {
			return
		}
		m.owner.main.Synchronize(func() {
			if m.closed {
				return
			}
			b, _ := json.MarshalIndent(report, "", "  ")
			_ = os.MkdirAll(filepath.Dir(path), 0700)
			if err := os.WriteFile(path, b, 0600); err != nil {
				nativeErrorLog(err, m.owner.backend.DataDir())
			}
			m.owner.quit()
		})
	}()
}
