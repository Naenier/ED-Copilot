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
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"github.com/wailsapp/go-webview2/pkg/edge"
	"github.com/wailsapp/go-webview2/webviewloader"
)

type modernShell struct {
	owner         *Window
	view          *edge.Chromium
	core          *edge.ICoreWebView2
	document      string
	ready, closed bool
	focusing      bool
	smokeOnce     sync.Once
	lastSummary   string
	diagnostics   int
	captures      []*modernPreviewCapture
}

func newModernWindow(backend Controller, options Options) (*Window, error) {
	if _, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString(""); err != nil {
		return nil, errors.New("Для интерфейса нужен Microsoft Edge WebView2. Запустите установщик ED Copilot — он предложит добавить этот компонент Windows.")
	}
	u := &Window{backend: backend, options: options, page: "overview", modes: map[string]string{}, queries: map[string]string{}, prefill: M{}, stop: make(chan struct{})}
	u.state = backend.Snapshot()
	var err error
	u.main, err = walk.NewMainWindow()
	if err != nil {
		return nil, fmt.Errorf("Создание основного окна: %w", err)
	}
	u.font, _ = walk.NewFont("Segoe UI", 10, 0)
	u.main.SetFont(u.font)
	_ = u.main.SetTitle("ED Copilot · Elite Dangerous")
	if options.Smoke || options.Demo {
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
	u.modern = &modernShell{owner: u}
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
			browser.NavigateToString(u.modern.document)
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
	u.modern.document = frontend.HTML(hex.EncodeToString(nonce[:]), options.Smoke)
	browser.NavigateToString(u.modern.document)
	browser.Resize()
	if err := browser.Show(); err != nil {
		u.dispose()
		return nil, fmt.Errorf("Отображение WebView2: %w", err)
	}
	u.main.AsContainerBase().SizeChanged().Attach(func() {
		if !u.modern.closed && browser.GetController() != nil {
			browser.Resize()
			u.modern.logGeometry("client-size")
		}
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
	if m.closed || m.focusing || m.view == nil || m.view.GetController() == nil {
		return
	}
	m.focusing = true
	defer func() { m.focusing = false }()
	_ = m.view.GetController().MoveFocus(edge.COREWEBVIEW2_MOVE_FOCUS_REASON_PROGRAMMATIC)
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
	if (msg == win.WM_MOVE || msg == win.WM_DPICHANGED) && m.view != nil && !m.closed {
		_ = m.view.NotifyParentWindowPositionChanged()
	}
	if msg == win.WM_SIZE && m.view != nil && !m.closed && m.view.GetController() != nil {
		m.view.Resize()
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
	u.state = u.backend.Snapshot()
	mode := v.Text(v.At(u.state, "desktop", "displayMode"), "windowed")
	if u.modeValue != mode {
		u.applyMode(mode)
	}
	if u.hud != nil {
		u.hud.refresh()
	}
	if u.modern.ready && !u.modern.closed && win.IsWindowVisible(u.main.Handle()) && !win.IsIconic(u.main.Handle()) {
		u.modern.send("update", M{"revision": u.state["revision"], "connection": u.state["connection"], "settings": u.state["settings"], "commander": u.state["commander"]})
	}
}

func (m *modernShell) send(kind string, payload any) {
	if m.closed || m.owner.disposed.Load() || m.view == nil || m.view.GetController() == nil || m.core == nil {
		return
	}
	if source, err := m.core.GetSource(); err != nil || !trustedWebSource(source) {
		return
	}
	b, e := json.Marshal(redactWebSecrets(M{"event": kind, "data": payload}))
	if e == nil {
		m.view.Eval("window.copilotReceive&&window.copilotReceive(" + string(b) + ")")
	}
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
		result, e := m.nativeAction(req.Params)
		m.reply(req.ID, result, e)
		return
	}
	go func() {
		result, e := dispatchWeb(m.owner.backend, req.Op, req.Params)
		if m.owner.disposed.Load() {
			return
		}
		m.owner.main.Synchronize(func() {
			if m.closed {
				return
			}
			m.reply(req.ID, result, e)
			if e == nil {
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
	r := M{"id": id, "result": result}
	if err != nil {
		r["error"] = err.Error()
	}
	m.send("response", r)
}

func (m *modernShell) windowAction(action string) (M, error) {
	u := m.owner
	switch action {
	case "drag":
		win.ReleaseCapture()
		var cursor win.POINT
		win.GetCursorPos(&cursor)
		win.SendMessage(u.main.Handle(), win.WM_NCLBUTTONDOWN, 2, uintptr(uint16(cursor.X))|uintptr(uint16(cursor.Y))<<16)
	case "minimize":
		win.ShowWindow(u.main.Handle(), win.SW_MINIMIZE)
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

func (m *modernShell) changeWindowMode(mode string) (M, error) {
	if mode != "fullscreen" && mode != "windowed" && mode != "borderless" {
		return nil, errors.New("Неизвестный режим окна")
	}
	u := m.owner
	u.saveBounds()
	result, err := u.backend.Do("/api/desktop", M{"displayMode": mode})
	if err != nil {
		return nil, err
	}
	u.state = u.backend.Snapshot()
	u.applyMode(mode)
	u.saveBounds()
	return v.Merge(v.Copy(result), M{"ok": true, "displayMode": mode, "maximized": win.IsZoomed(u.main.Handle())}), nil
}

func (m *modernShell) nativeAction(params M) (M, error) {
	u := m.owner
	u.dialogOpen = true
	defer func() { u.dialogOpen = false }()
	switch v.Str(params["action"]) {
	case "open":
		return M{"ok": true}, openExternal(v.Str(params["url"]))
	case "copy":
		return M{"ok": true}, walk.Clipboard().SetText(v.Str(params["text"]))
	case "folder":
		d := walk.FileDialog{Title: "Папка игровых журналов", FilePath: v.Str(v.At(u.state, "connection", "journalDir"))}
		ok, e := d.ShowBrowseFolder(u.main)
		if e != nil {
			return nil, e
		}
		if !ok {
			return M{"cancelled": true}, nil
		}
		return u.backend.Do("/api/journal-dir", M{"path": d.FilePath})
	case "import-plan":
		d := walk.FileDialog{Title: "План инженерных улучшений", Filter: "JSON (*.json)|*.json"}
		ok, e := d.ShowOpen(u.main)
		if e != nil {
			return nil, e
		}
		if !ok {
			return M{"cancelled": true}, nil
		}
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
	case "export":
		kind := v.Str(params["kind"])
		asJSON := v.Bool(params["json"])
		extension := "csv"
		if asJSON {
			extension = "json"
		}
		d := walk.FileDialog{Title: "Сохранить план или таблицу", FilePath: "ED Copilot." + extension, Filter: strings.ToUpper(extension) + " (*." + extension + ")|*." + extension}
		ok, e := d.ShowSave(u.main)
		if e != nil {
			return nil, e
		}
		if !ok {
			return M{"cancelled": true}, nil
		}
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
	m.smokeVisualState(report)
	if m.owner.hud != nil {
		m.owner.hud.refresh()
		report["hudHidden"] = !win.IsWindowVisible(m.owner.hud.hwnd)
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

// The renderer receives serializable views and action descriptions. Raw engine
// methods, API keys and arbitrary filesystem paths are not exposed to scripts.
func dispatchWeb(backend Controller, op string, input M) (M, error) {
	state := backend.Snapshot()
	tab, mode, query := v.Text(input["tab"], "overview"), v.Str(input["mode"]), v.Str(input["query"])
	valid := false
	for _, t := range p.Tabs {
		if t.ID == tab {
			valid = true
		}
	}
	if !valid {
		return nil, errors.New("Раздел не найден")
	}
	page := p.Build(tab, mode, query, state)
	selected := M{}
	selectedDetails, selectedTitle := "", page.Title
	for _, row := range page.Rows {
		if row.ID == v.Str(input["selected"]) {
			selected = v.Copy(row.Data)
			selectedDetails = row.Details
			selectedTitle = v.Text(row.Data["title"], row.Data["name"], page.Title)
			break
		}
	}
	actions := p.CoreActions(tab, mode, state, selected, v.Obj(input["prefill"]))
	switch op {
	case "boot":
		return M{"app": state["app"], "tabs": p.Tabs, "navigation": state["navigation"], "activities": p.Activities, "state": webState(state)}, nil
	case "view":
		return M{"page": page, "actions": webActions(actions), "state": webState(state), "help": p.HelpFor(tab)}, nil
	case "catalog":
		return M{"engineering": v.At(state, "engineering", "catalog")}, nil
	case "navigation":
		return backend.Do("/api/navigation", M{"tabs": input["tabs"]})
	case "settings":
		return backend.Do("/api/settings", v.Obj(input["values"]))
	case "desktop":
		return backend.Do("/api/desktop", v.Obj(input["values"]))
	case "theme-apply":
		return backend.Do("/api/theme", M{"settings": v.Obj(input["values"])})
	case "learning":
		return backend.Do("/api/learning", v.Obj(input["values"]))
	case "calculator":
		return backend.Do("/api/carrier/calculator", v.Obj(input["values"]))
	case "select-colony":
		return backend.Do("/api/colonization", M{"action": "select-project", "id": input["id"]})
	case "action":
		for _, a := range actions {
			if a.ID != v.Str(input["actionId"]) {
				continue
			}
			if a.Disabled {
				return nil, errors.New("Сначала выберите подходящую строку")
			}
			values := M{}
			for _, f := range a.Fields {
				raw, present := v.Obj(input["values"])[f.Key]
				if !present {
					raw = f.Default
				}
				parsed, e := p.ParseField(f, raw)
				if e != nil {
					return nil, e
				}
				values[f.Key] = parsed
			}
			body, e := a.Body(values)
			if e != nil {
				return nil, e
			}
			switch a.Kind {
			case "purchase":
				prefill, e := p.PurchasePrefill(a, values)
				return M{"navigate": "sale", "mode": "Купить", "prefill": prefill}, e
			case "navigate":
				return M{"navigate": a.Path, "mode": v.Str(a.Payload["mode"])}, nil
			case "help":
				return M{"help": p.HelpFor(tab)}, nil
			case "details":
				return M{"details": selectedDetails, "title": selectedTitle}, nil
			case "copy":
				return M{"copy": v.Text(a.Path, a.Payload["text"], a.Payload["system"])}, nil
			case "url":
				return M{"url": a.Path}, nil
			case "export":
				return M{"export": a.Path, "projectId": a.Payload["projectId"]}, nil
			case "folder", "import-plan":
				return M{"native": a.Kind}, nil
			case "engineering-trade":
				result, e := backend.Do(a.Path, body)
				if e != nil {
					return nil, e
				}
				return v.Merge(v.Copy(result), M{"navigate": "engineering", "mode": "Обмен"}), nil
			case "theme-import":
				result, e := backend.Do(a.Path, body)
				if e != nil {
					return nil, e
				}
				return M{"themePreview": v.Copy(v.Obj(result["theme"]))}, nil
			}
			result, e := backend.Do(a.Path, body)
			delete(body, "apiKey")
			return result, e
		}
		return nil, errors.New("Действие больше недоступно. Обновите раздел.")
	}
	return nil, errors.New("Неизвестный запрос интерфейса")
}

func webState(state M) M {
	out := M{}
	for _, key := range []string{"app", "commander", "connection", "settings", "desktop", "theme", "navigation", "tradePlan", "carrier", "carrierRoute", "colonization", "pilot", "mining", "stats", "learning", "carrierCalculator", "frontier", "integrations", "notices", "objectives"} {
		out[key] = state[key]
	}
	return redactWebSecrets(v.Obj(v.Clone(out)))
}

func redactWebSecrets(input M) M {
	result := M{}
	for key, value := range input {
		normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key))
		switch normalized {
		case "apikey", "accesstoken", "refreshtoken", "clientsecret", "codeverifier", "password":
			continue
		}
		switch item := value.(type) {
		case map[string]any:
			result[key] = redactWebSecrets(item)
		case []any:
			out := make([]any, len(item))
			for i, entry := range item {
				if row, ok := entry.(map[string]any); ok {
					out[i] = redactWebSecrets(row)
				} else {
					out[i] = entry
				}
			}
			result[key] = out
		default:
			result[key] = value
		}
	}
	return result
}
func webActions(actions []p.Action) []M {
	out := []M{}
	for _, a := range actions {
		fields := []M{}
		for _, f := range a.Fields {
			defaultValue := f.Default
			if f.Kind == "password" {
				defaultValue = nil
			}
			fields = append(fields, M{"key": f.Key, "label": f.Label, "kind": f.Kind, "help": f.Help, "default": defaultValue, "choices": f.Choices, "required": f.Required, "min": f.Min, "max": f.Max})
		}
		out = append(out, M{"id": a.ID, "title": a.Title, "kind": a.Kind, "path": a.Path, "confirm": a.Confirm, "disabled": a.Disabled, "fields": fields})
	}
	return out
}
