//go:build windows

// Package nativeui hosts the embedded desktop interface in a native Win32 window
// and renders the independent, game-only overlay with Win32/GDI.
package nativeui

import (
	"bytes"
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/data"
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

type M = v.M
type Controller interface {
	Snapshot() M
	Do(string, M) (M, error)
	Updates() <-chan struct{}
	ExportCSV(string) (string, error)
	ExportPlan(string) ([]byte, error)
	Close()
	DataDir() string
}
type Options struct {
	SmokeReport       string
	Smoke             bool
	Demo              bool
	ReadmeScreenshots bool
	SmokeFixture      *SmokeFixture
	Soak              *SoakFixture
}
type Window struct {
	modern                                                 *modernShell
	lastRouteClipboardID                                   string
	routeClipboardWriter                                   func(string) error
	header, identity, toolbar, actionbar                   *walk.Composite
	brand                                                  *walk.Label
	main                                                   *walk.MainWindow
	backend                                                Controller
	options                                                Options
	state                                                  M
	page                                                   string
	modes                                                  map[string]string
	queries                                                map[string]string
	tabs                                                   []string
	nav                                                    *walk.CustomWidget
	navOffset                                              int
	navHits                                                []navHit
	drag                                                   string
	dragX                                                  int
	dragged                                                bool
	contextLabel, titleLabel, description, summary, footer *walk.Label
	activityRow, filterRow                                 *walk.Composite
	activity, colony, overlayColony, mode                  *walk.ComboBox
	query                                                  *walk.LineEdit
	overlayButton                                          *walk.PushButton
	actionButtons                                          []*walk.PushButton
	moreButton                                             *walk.PushButton
	table                                                  *walk.TableView
	details                                                *walk.TextEdit
	model                                                  *tableModel
	actions                                                []p.Action
	columns                                                []string
	refreshing, dialogOpen, quitting                       bool
	stop                                                   chan struct{}
	refreshRequested                                       chan struct{}
	disposed                                               atomic.Bool
	tray                                                   *walk.NotifyIcon
	hud                                                    *hudWindow
	font, titleFont, boldFont                              *walk.Font
	brushes                                                []*walk.SolidColorBrush
	colors                                                 M
	modeValue                                              string
	normalBounds                                           walk.Rectangle
	colonyIDs, overlayColonyIDs                            []string
	lastColonyLabels, lastOverlayLabels                    []string
	prefill                                                M
}
type tableModel struct {
	walk.TableModelBase
	rows   []p.Row
	colors M
	table  *walk.TableView
}

func (m *tableModel) RowCount() int { return len(m.rows) }
func (m *tableModel) Value(row, col int) any {
	if row < 0 || row >= len(m.rows) || col < 0 || col >= len(m.rows[row].Cells) {
		return ""
	}
	return m.rows[row].Cells[col]
}
func (m *tableModel) StyleCell(s *walk.CellStyle) {
	// Draw both colours of the selected cell together. The Windows theme
	// otherwise replaces just one of them for an inactive selection.
	if m.table != nil && s.Row() == m.table.CurrentIndex() {
		if s.Col() >= 0 {
			if canvas := s.Canvas(); canvas != nil {
				brush, _ := walk.NewSolidColorBrush(color(v.Text(m.colors["accentDim"], "#153b50")))
				defer brush.Dispose()
				bounds := s.Bounds()
				_ = canvas.FillRectangle(brush, bounds)
				bounds.X += 6
				bounds.Width = max(0, bounds.Width-10)
				_ = canvas.DrawText(v.Str(m.Value(s.Row(), s.Col())), m.table.Font(), color(v.Text(m.colors["text"], "#e0e0e0")), bounds, walk.TextSingleLine|walk.TextVCenter|walk.TextEndEllipsis)
			}
		}
		return
	}
	s.TextColor = color(v.Text(m.colors["text"], "#e0e0e0"))
	s.BackgroundColor = color(v.Text(m.colors["panel"], "#0a1622"))
	if s.Row()%2 == 1 {
		s.BackgroundColor = color(v.Text(m.colors["bg"], "#07101a"))
	}
}
func color(hex string) walk.Color {
	var r, g, b uint8
	_, _ = fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	return walk.RGB(r, g, b)
}
func fixedHeight(w walk.Widget, h int) {
	_ = w.SetMinMaxSize(walk.Size{Height: h}, walk.Size{Height: h})
}
func label(parent walk.Container, text string) (*walk.Label, error) {
	x, e := walk.NewLabel(parent)
	if e == nil {
		_ = x.SetText(text)
	}
	return x, e
}
func button(parent walk.Container, text string, fn func()) (*walk.PushButton, error) {
	x, e := walk.NewPushButton(parent)
	if e != nil {
		return nil, e
	}
	_ = x.SetText(text)
	if fn != nil {
		x.Clicked().Attach(fn)
	}
	return x, nil
}
func box(parent walk.Container, horizontal bool) (*walk.Composite, error) {
	c, e := walk.NewComposite(parent)
	if e != nil {
		return nil, e
	}
	if horizontal {
		l := walk.NewHBoxLayout()
		l.SetMargins(walk.Margins{})
		l.SetSpacing(10)
		_ = c.SetLayout(l)
	} else {
		l := walk.NewVBoxLayout()
		l.SetMargins(walk.Margins{})
		l.SetSpacing(8)
		_ = c.SetLayout(l)
	}
	return c, nil
}
func Run(backend Controller, options Options) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	u, e := newModernWindow(backend, options)
	if e != nil {
		return e
	}
	defer u.dispose()
	u.main.Show()
	u.applyMode(v.Text(v.At(u.state, "desktop", "displayMode"), "windowed"))
	if options.Soak != nil {
		win.ShowWindow(u.main.Handle(), win.SW_SHOWNOACTIVATE)
		go u.modern.runSoak()
	}
	if options.Smoke || options.Demo {
		// A hidden launcher can supply STARTF_USESHOWWINDOW/SW_HIDE. Its
		// first ShowWindow is consumed by Windows, so make isolated GUI
		// scenarios explicitly visible after the normal first Show.
		win.ShowWindow(u.main.Handle(), win.SW_SHOWNORMAL)
		win.SetForegroundWindow(u.main.Handle())
	}
	if options.Smoke && u.modern == nil {
		go func() { time.Sleep(1100 * time.Millisecond); u.main.Synchronize(func() { u.smoke() }) }()
	}
	u.main.Run()
	return nil
}
func NewWindow(backend Controller, options Options) (*Window, error) {
	u := &Window{backend: backend, options: options, page: "overview", modes: map[string]string{}, queries: map[string]string{}, stop: make(chan struct{}), prefill: M{}}
	var err error
	u.main, err = walk.NewMainWindow()
	if err != nil {
		return nil, err
	}
	u.font, _ = walk.NewFont("Segoe UI", 10, 0)
	u.titleFont, _ = walk.NewFont("Segoe UI", 22, walk.FontBold)
	u.boldFont, _ = walk.NewFont("Segoe UI", 11, walk.FontBold)
	u.main.SetFont(u.font)
	_ = u.main.SetTitle("ED Copilot · Elite Dangerous")
	_ = u.main.SetSize(walk.Size{Width: 1440, Height: 960})
	_ = u.main.SetMinMaxSize(walk.Size{Width: 900, Height: 700}, walk.Size{})
	root := walk.NewVBoxLayout()
	root.SetMargins(walk.Margins{HNear: 20, VNear: 14, HFar: 20, VFar: 10})
	root.SetSpacing(9)
	_ = u.main.SetLayout(root)
	if b, e := data.Files.ReadFile("copilot.png"); e == nil {
		if img, e := png.Decode(bytes.NewReader(b)); e == nil {
			if icon, e := walk.NewIconFromImage(img); e == nil {
				_ = u.main.SetIcon(icon)
			}
		}
	}
	header, err := box(u.main, false)
	u.header = header
	if err != nil {
		return nil, err
	}
	fixedHeight(header, 94)
	identity, _ := box(header, true)
	u.identity = identity
	fixedHeight(identity, 48)
	brand, _ := label(identity, "ED COPILOT")
	u.brand = brand
	brand.SetFont(u.titleFont)
	_ = brand.SetMinMaxSize(walk.Size{Width: 200}, walk.Size{Width: 225})
	u.contextLabel, _ = label(identity, "Читаем журнал полёта")
	_ = identity.Layout().(*walk.BoxLayout).SetStretchFactor(u.contextLabel, 1)
	toolbar, _ := box(header, true)
	u.toolbar = toolbar
	fixedHeight(toolbar, 34)
	_, _ = button(toolbar, "? Помощь", func() { u.showHelp() })
	u.overlayButton, _ = button(toolbar, "Включить оверлей", func() {
		enabled := !v.Bool(v.At(u.state, "settings", "overlayEnabled"))
		u.execute(p.Action{Path: "/api/settings", Payload: M{"overlayEnabled": enabled}}, nil)
	})
	_, _ = button(toolbar, "Обновить", func() { u.execute(p.Action{Path: "/api/rescan"}, nil) })
	_, _ = button(toolbar, "В окно", func() {
		u.applyMode("windowed")
		u.execute(p.Action{Path: "/api/desktop", Payload: M{"displayMode": "windowed"}}, nil)
	})
	toolbarSpace, _ := label(toolbar, "")
	_ = toolbar.Layout().(*walk.BoxLayout).SetStretchFactor(toolbarSpace, 1)
	u.nav, err = walk.NewCustomWidget(u.main, 0, u.paintNavigation)
	if err != nil {
		return nil, err
	}
	fixedHeight(u.nav, 45)
	u.nav.SetInvalidatesOnResize(true)
	u.nav.SetPaintMode(walk.PaintBuffered)
	u.nav.MouseDown().Attach(u.navDown)
	u.nav.MouseMove().Attach(u.navMove)
	u.nav.MouseUp().Attach(u.navUp)
	u.nav.MouseWheel().Attach(func(x, y int, button walk.MouseButton) {
		delta := int(int16(uint32(button) >> 16))
		if delta != 0 {
			u.scrollTabs(-delta)
		}
	})
	u.nav.KeyDown().Attach(func(key walk.Key) {
		if key == walk.KeyLeft {
			u.cycleTab(-1)
		}
		if key == walk.KeyRight {
			u.cycleTab(1)
		}
	})
	u.nav.SizeChanged().Attach(func() { u.keepActiveTabVisible(); u.nav.Invalidate() })
	u.activityRow, _ = box(u.main, true)
	fixedHeight(u.activityRow, 37)
	_, _ = label(u.activityRow, "Занятие для оверлея:")
	u.activity, _ = walk.NewDropDownBox(u.activityRow)
	activityNames := []string{}
	for _, a := range p.Activities {
		activityNames = append(activityNames, a.Label)
	}
	_ = u.activity.SetModel(activityNames)
	_ = u.activity.SetMinMaxSize(walk.Size{Width: 230}, walk.Size{Width: 285})
	u.activity.CurrentIndexChanged().Attach(func() {
		if u.refreshing {
			return
		}
		i := u.activity.CurrentIndex()
		if i >= 0 && i < len(p.Activities) {
			u.execute(p.Action{Path: "/api/settings", Payload: M{"activity": p.Activities[i].Value}}, nil)
		}
	})
	_, _ = label(u.activityRow, "Колония:")
	u.overlayColony, _ = walk.NewDropDownBox(u.activityRow)
	_ = u.overlayColony.SetMinMaxSize(walk.Size{Width: 245}, walk.Size{})
	u.overlayColony.CurrentIndexChanged().Attach(func() {
		if !u.refreshing {
			u.chooseColony(u.overlayColony.CurrentIndex(), u.overlayColonyIDs)
		}
	})
	u.titleLabel, _ = label(u.main, "")
	u.titleLabel.SetFont(u.titleFont)
	fixedHeight(u.titleLabel, 40)
	u.description, _ = label(u.main, "")
	fixedHeight(u.description, 26)
	u.filterRow, _ = box(u.main, true)
	fixedHeight(u.filterRow, 35)
	u.mode, _ = walk.NewDropDownBox(u.filterRow)
	_ = u.mode.SetMinMaxSize(walk.Size{Width: 210}, walk.Size{Width: 260})
	u.mode.CurrentIndexChanged().Attach(func() {
		if u.refreshing {
			return
		}
		if u.mode.CurrentIndex() >= 0 {
			u.modes[u.page] = u.mode.Text()
			u.renderPage(true)
		}
	})
	u.colony, _ = walk.NewDropDownBox(u.filterRow)
	_ = u.colony.SetMinMaxSize(walk.Size{Width: 240}, walk.Size{Width: 340})
	u.colony.CurrentIndexChanged().Attach(func() {
		if !u.refreshing {
			u.chooseColony(u.colony.CurrentIndex(), u.colonyIDs)
		}
	})
	u.query, _ = walk.NewLineEdit(u.filterRow)
	_ = u.query.SetCueBanner("Поиск по текущему разделу")
	_ = u.filterRow.Layout().(*walk.BoxLayout).SetStretchFactor(u.query, 1)
	u.query.TextChanged().Attach(func() {
		if u.refreshing {
			return
		}
		u.queries[u.page] = u.query.Text()
		u.renderPage(false)
	})
	bar, _ := box(u.main, true)
	u.actionbar = bar
	fixedHeight(bar, 37)
	for i := 0; i < 3; i++ {
		index := i
		b, _ := button(bar, "Действие", func() {
			if index < len(u.actions) {
				u.runAction(u.actions[index])
			}
		})
		u.actionButtons = append(u.actionButtons, b)
	}
	u.moreButton, _ = button(bar, "Другие действия…", func() { u.actionMenu() })
	spacer, _ := label(bar, "")
	_ = bar.Layout().(*walk.BoxLayout).SetStretchFactor(spacer, 1)
	u.summary, _ = label(u.main, "")
	fixedHeight(u.summary, 30)
	u.table, err = walk.NewTableView(u.main)
	if err != nil {
		return nil, err
	}
	u.table.SetAlternatingRowBG(false)
	u.table.SetLastColumnStretched(true)
	u.table.SetColumnsOrderable(false)
	u.table.SetSelectionHiddenWithoutFocus(false)
	u.table.SetMultiSelection(false)
	_ = root.SetStretchFactor(u.table, 1)
	u.model = &tableModel{table: u.table}
	_ = u.table.SetModel(u.model)
	u.table.CurrentIndexChanged().Attach(func() {
		if u.refreshing {
			return
		}
		u.updateSelection()
	})
	u.table.ItemActivated().Attach(func() { u.showSelectedDetails() })
	u.details, _ = walk.NewTextEdit(u.main)
	u.details.SetReadOnly(true)
	u.details.SetFont(u.font)
	fixedHeight(u.details, 156)
	u.footer, _ = label(u.main, "ED Copilot · Go · Данные читаются из журнала игры")
	fixedHeight(u.footer, 22)
	u.main.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		if !u.quitting {
			*canceled = true
			u.saveBounds()
			u.main.Hide()
		}
	})
	u.main.KeyDown().Attach(func(key walk.Key) {
		if key == walk.KeyF11 {
			mode := "fullscreen"
			if u.modeValue == "fullscreen" {
				mode = "windowed"
			}
			u.applyMode(mode)
			u.execute(p.Action{Path: "/api/desktop", Payload: M{"displayMode": mode}}, nil)
		} else if key == walk.KeyEscape && u.modeValue != "windowed" {
			u.applyMode("windowed")
			u.execute(p.Action{Path: "/api/desktop", Payload: M{"displayMode": "windowed"}}, nil)
		}
	})
	u.tray, err = walk.NewNotifyIcon(u.main)
	if err != nil {
		return nil, err
	}
	_ = u.tray.SetIcon(u.main.Icon())
	_ = u.tray.SetToolTip("ED Copilot · помощник пилота")
	show := walk.NewAction()
	_ = show.SetText("Открыть ED Copilot")
	show.Triggered().Attach(func() { u.main.Show(); u.main.SetFocus() })
	_ = u.tray.ContextMenu().Actions().Add(show)
	overlay := walk.NewAction()
	_ = overlay.SetText("Показать / скрыть оверлей")
	overlay.Triggered().Attach(func() {
		u.execute(p.Action{Path: "/api/settings", Payload: M{"overlayEnabled": !v.Bool(v.At(u.state, "settings", "overlayEnabled"))}}, nil)
	})
	_ = u.tray.ContextMenu().Actions().Add(overlay)
	exit := walk.NewAction()
	_ = exit.SetText("Выход")
	exit.Triggered().Attach(u.quit)
	_ = u.tray.ContextMenu().Actions().Add(exit)
	u.tray.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		if button == walk.LeftButton {
			u.main.Show()
			u.main.SetFocus()
		}
	})
	_ = u.tray.SetVisible(true)
	u.state = u.backend.Snapshot()
	u.syncRouteClipboard(attachRouteClipboard(u.state, u.backend))
	u.tabs = p.NormalizeTabs(v.At(u.state, "navigation", "tabs"))
	u.applyTheme()
	u.refresh()
	if b := v.Obj(v.At(u.state, "desktop", "windowedBounds")); v.Int(b["width"]) >= 900 && v.Int(b["height"]) >= 700 {
		_ = u.main.SetBoundsPixels(walk.Rectangle{X: v.Int(b["x"]), Y: v.Int(b["y"]), Width: v.Int(b["width"]), Height: v.Int(b["height"])})
	}
	u.hud, err = newHUD(u)
	if err != nil {
		return nil, err
	}
	go u.watch()
	return u, nil
}
func (u *Window) watch() {
	if u.modern != nil {
		u.modern.watch()
		return
	}
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-u.stop:
			return
		case <-u.backend.Updates():
		case <-ticker.C:
		case <-u.refreshRequested:
		}
		if u.disposed.Load() {
			return
		}
		u.main.Synchronize(func() {
			if !u.disposed.Load() {
				if !u.dialogOpen {
					u.refresh()
				} else if u.hud != nil {
					u.hud.refresh()
				}
			}
		})
	}
}
func (u *Window) refresh() {
	if u.modern != nil {
		u.refreshModern()
		return
	}
	u.refreshing = true
	defer func() { u.refreshing = false }()
	u.state = u.backend.Snapshot()
	next := p.NormalizeTabs(v.At(u.state, "navigation", "tabs"))
	if !reflect.DeepEqual(next, u.tabs) {
		u.tabs = next
		u.keepActiveTabVisible()
		u.nav.Invalidate()
	}
	u.contextLabel.SetText(v.Text(v.At(u.state, "commander", "system"), "Ожидание положения") + "\r\n" + p.Ship(v.Obj(u.state["commander"])))
	enabled := v.Bool(v.At(u.state, "settings", "overlayEnabled"))
	u.activityRow.SetVisible(enabled)
	text := "Включить оверлей"
	if enabled {
		text = "Выключить оверлей"
	}
	u.overlayButton.SetText(text)
	activity := v.Str(v.At(u.state, "settings", "activity"))
	for i, a := range p.Activities {
		if a.Value == activity {
			_ = u.activity.SetCurrentIndex(i)
			break
		}
	}
	u.overlayColony.SetVisible(activity == "colonization")
	u.syncColonies()
	u.renderPage(false)
	if u.hud != nil {
		u.hud.refresh()
	}
	status := "Журнал подключён"
	if v.Str(v.At(u.state, "connection", "error")) != "" {
		status = v.Str(v.At(u.state, "connection", "error"))
	}
	u.footer.SetText("ED Copilot " + v.Str(v.At(u.state, "app", "version")) + " · " + status)
	u.applyTheme()
}
func (u *Window) syncColonies() {
	rows := v.Rows(v.At(u.state, "colonization", "projects"))
	names, ids := []string{}, []string{}
	selected := v.Str(v.At(u.state, "colonization", "selectedProjectId"))
	index := -1
	for i, r := range rows {
		ids = append(ids, v.Str(r["id"]))
		names = append(names, v.Text(r["name"], r["targetSystem"]))
		if v.Str(r["id"]) == selected {
			index = i
		}
	}
	if !reflect.DeepEqual(names, u.lastColonyLabels) || !reflect.DeepEqual(ids, u.colonyIDs) {
		_ = u.colony.SetModel(names)
		u.lastColonyLabels = append([]string(nil), names...)
		u.colonyIDs = append([]string(nil), ids...)
	}
	if !reflect.DeepEqual(names, u.lastOverlayLabels) || !reflect.DeepEqual(ids, u.overlayColonyIDs) {
		_ = u.overlayColony.SetModel(names)
		u.lastOverlayLabels = append([]string(nil), names...)
		u.overlayColonyIDs = append([]string(nil), ids...)
	}
	_ = u.colony.SetCurrentIndex(index)
	_ = u.overlayColony.SetCurrentIndex(index)
}
func (u *Window) chooseColony(index int, ids []string) {
	if index >= 0 && index < len(ids) {
		u.execute(p.Action{Path: "/api/colonization", Payload: M{"action": "select-project", "id": ids[index]}}, nil)
	}
}
func (u *Window) renderPage(force bool) {
	was := u.refreshing
	u.refreshing = true
	defer func() { u.refreshing = was }()
	page := p.Build(u.page, u.modes[u.page], u.queries[u.page], u.state)
	u.titleLabel.SetText(page.Title)
	u.description.SetText(page.Description)
	u.summary.SetText(page.Summary)
	u.colony.SetVisible(u.page == "colonization")
	u.mode.SetVisible(len(page.Modes) > 0)
	if !reflect.DeepEqual(page.Modes, modelStrings(u.mode)) {
		_ = u.mode.SetModel(page.Modes)
	}
	mode := u.modes[u.page]
	if mode == "" && len(page.Modes) > 0 {
		mode = page.Modes[0]
		u.modes[u.page] = mode
	}
	for i, s := range page.Modes {
		if s == mode {
			_ = u.mode.SetCurrentIndex(i)
		}
	}
	if u.query.Text() != u.queries[u.page] {
		_ = u.query.SetText(u.queries[u.page])
	}
	selected := ""
	if i := u.table.CurrentIndex(); i >= 0 && i < len(u.model.rows) {
		selected = u.model.rows[i].ID
	}
	if !reflect.DeepEqual(u.columns, page.Columns) {
		u.table.Columns().Clear()
		for i, title := range page.Columns {
			c := walk.NewTableViewColumn()
			_ = c.SetTitle(title)
			width := 155
			if i == 0 {
				width = 260
			}
			_ = c.SetWidth(width)
			_ = u.table.Columns().Add(c)
		}
		u.columns = append([]string(nil), page.Columns...)
	}
	// Live polling must preserve the reader's scroll position and selection.
	// A reset is needed only when the actual rows or columns changed.
	if force || !reflect.DeepEqual(u.model.rows, page.Rows) {
		u.model.rows = page.Rows
		u.model.colors = u.colors
		u.model.PublishRowsReset()
		newIndex := -1
		for i, r := range page.Rows {
			if r.ID == selected && selected != "" {
				newIndex = i
				break
			}
		}
		if newIndex < 0 && len(page.Rows) > 0 {
			newIndex = 0
		}
		_ = u.table.SetCurrentIndex(newIndex)
	}
	u.updateSelection()
	u.nav.Invalidate()
}
func modelStrings(combo *walk.ComboBox) []string {
	if combo == nil {
		return nil
	}
	if a, ok := combo.Model().([]string); ok {
		return a
	}
	return nil
}
func (u *Window) selected() M {
	i := u.table.CurrentIndex()
	if i >= 0 && i < len(u.model.rows) {
		return v.Copy(u.model.rows[i].Data)
	}
	return M{}
}
func (u *Window) updateSelection() {
	i := u.table.CurrentIndex()
	details := "Выберите строку, чтобы увидеть подробности. Если данных ещё нет, используйте действия раздела или откройте помощь."
	if i >= 0 && i < len(u.model.rows) {
		r := u.model.rows[i]
		details = r.Details
	}
	if u.details.Text() != details {
		_ = u.details.SetText(details)
	}
	u.actions = Actions(u.page, u.modes[u.page], u.state, u.selected(), u.prefill)
	for i, b := range u.actionButtons {
		if i < len(u.actions) {
			a := u.actions[i]
			b.SetText(a.Title)
			b.SetEnabled(!a.Disabled)
			b.SetVisible(true)
		} else {
			b.SetVisible(false)
		}
	}
	u.moreButton.SetVisible(len(u.actions) > 3)
}
func (u *Window) showSelectedDetails() {
	i := u.table.CurrentIndex()
	if i < 0 || i >= len(u.model.rows) {
		return
	}
	r := u.model.rows[i]
	if dest := v.Str(r.Data["navigate"]); dest != "" {
		u.navigate(dest)
		return
	}
	showText(u.main, v.Text(r.Data["title"], r.Data["name"], p.Title(u.page)), r.Details, u.colors)
}
func (u *Window) navigate(id string) {
	found := false
	for _, t := range u.tabs {
		if t == id {
			found = true
		}
	}
	if !found {
		u.tabs = append(u.tabs, id)
		u.persistTabs()
	}
	u.page = id
	u.keepActiveTabVisible()
	u.renderPage(true)
}
func (u *Window) persistTabs() {
	_, err := u.backend.Do("/api/navigation", M{"tabs": append([]string(nil), u.tabs...)})
	if err != nil {
		u.error(err)
	}
	u.keepActiveTabVisible()
	u.nav.Invalidate()
}
func (u *Window) showHelp() {
	showText(u.main, "Как пользоваться · "+p.Title(u.page), p.HelpFor(u.page), u.colors)
}
func (u *Window) error(err error) {
	if err != nil {
		walk.MsgBox(u.main, "ED Copilot", err.Error(), walk.MsgBoxIconWarning)
	}
}
func (u *Window) execute(action p.Action, values M) {
	body, err := action.Body(values)
	if err != nil {
		u.error(err)
		return
	}
	if u.modern != nil {
		u.modern.executeAction(action, body)
		return
	}
	if u.footer != nil {
		u.footer.SetText("Выполняем: " + v.Text(action.Title, "действие"))
	}
	go func() {
		result, err := u.backend.Do(action.Path, body)
		delete(body, "apiKey")
		u.main.Synchronize(func() {
			if u.disposed.Load() {
				return
			}
			if err != nil {
				u.error(err)
			} else {
				if raw := v.Str(result["url"]); raw != "" {
					openExternal(raw)
				}
				if action.Path == "/api/desktop" {
					u.applyMode(v.Str(body["displayMode"]))
				}
				u.refresh()
			}
		})
	}()
}
func (u *Window) quit() {
	if u.quitting || u.dialogOpen {
		return
	}
	u.quitting = true
	if u.modern != nil {
		u.modern.flushDesktop()
	}
	u.saveBounds()
	if saver, ok := u.backend.(profileExitSaver); ok {
		if err := saver.CloseWithSave(); err != nil {
			ResolveProfileSaveFailure(u.main, saver, p.Language(u.state), err)
		}
	} else {
		u.backend.Close()
	}
	u.disposed.Store(true)
	close(u.stop)
	if u.hud != nil {
		u.hud.close()
	}
	u.main.Close()
	walk.App().Exit(0)
}
func (u *Window) dispose() {
	if u.modern != nil {
		u.modern.close()
	}
	if !u.disposed.Swap(true) {
		close(u.stop)
		u.backend.Close()
	}
	if u.hud != nil {
		u.hud.close()
	}
	if u.tray != nil {
		u.tray.Dispose()
	}
	if u.main != nil {
		u.main.Dispose()
	}
	for _, b := range u.brushes {
		b.Dispose()
	}
	for _, f := range []*walk.Font{u.font, u.titleFont, u.boldFont} {
		if f != nil {
			f.Dispose()
		}
	}
}
func (u *Window) applyTheme() {
	resolved := v.Obj(v.At(u.state, "theme", "palette"))
	if len(resolved) == 0 {
		resolved = v.Obj(v.At(u.state, "theme", "settings", "colors"))
	}
	// Opacity does not belong to a Win32 colour brush. JSON cloning changes
	// its numeric Go type, so comparing it would allocate a brush every poll.
	colors := M{}
	for key, value := range resolved {
		if text, ok := value.(string); ok {
			colors[key] = text
		}
	}
	if len(colors) == 0 {
		colors = M{"accent": "#28a0f0", "text": "#e0e0e0", "muted": "#8898a8", "bg": "#07101a", "panel": "#0a1622"}
	}
	if reflect.DeepEqual(colors, u.colors) {
		return
	}
	u.colors = v.Copy(colors)
	brush, _ := walk.NewSolidColorBrush(color(v.Str(colors["bg"])))
	u.brushes = append(u.brushes, brush)
	u.main.SetBackground(brush)
	for _, c := range []*walk.Composite{u.header, u.identity, u.toolbar, u.actionbar, u.activityRow, u.filterRow} {
		if c != nil {
			c.SetBackground(brush)
		}
	}
	for _, l := range []*walk.Label{u.brand, u.contextLabel, u.titleLabel, u.description, u.summary, u.footer} {
		if l != nil {
			l.SetTextColor(color(v.Str(colors["text"])))
		}
	}
	u.details.SetTextColor(color(v.Str(colors["text"])))
	u.details.SetBackground(brush)
	u.query.SetTextColor(color(v.Str(colors["text"])))
	u.query.SetBackground(brush)
	u.table.SetBackground(brush)
	for child := win.GetWindow(u.table.Handle(), win.GW_CHILD); child != 0; child = win.GetWindow(child, win.GW_HWNDNEXT) {
		var class [64]uint16
		_, _ = win.GetClassName(child, &class[0], len(class))
		if syscall.UTF16ToString(class[:]) != "SysListView32" {
			continue
		}
		win.SendMessage(child, win.LVM_SETBKCOLOR, 0, uintptr(color(v.Str(colors["panel"]))))
		win.SendMessage(child, win.LVM_SETTEXTBKCOLOR, 0, uintptr(color(v.Str(colors["panel"]))))
		win.SendMessage(child, win.LVM_SETTEXTCOLOR, 0, uintptr(color(v.Str(colors["text"]))))
	}
	u.model.colors = u.colors
	u.model.PublishRowsReset()
	u.nav.Invalidate()
	darkWindow(u.main.Handle())
}
func (u *Window) actionMenu() {
	if len(u.actions) > 3 {
		u.popupActions(u.actions[3:])
	}
}
func (u *Window) popupActions(actions []p.Action) {
	menu, _ := walk.NewMenu()
	defer menu.Dispose()
	for _, a := range actions {
		action := a
		item := walk.NewAction()
		_ = item.SetText(a.Title)
		_ = item.SetEnabled(!a.Disabled)
		item.Triggered().Attach(func() { u.runAction(action) })
		_ = menu.Actions().Add(item)
	}
	var at win.POINT
	win.GetCursorPos(&at)
	previous := u.main.ContextMenu()
	u.main.SetContextMenu(menu)
	win.SendMessage(u.main.Handle(), win.WM_CONTEXTMENU, uintptr(u.main.Handle()), uintptr(uint16(at.X))|uintptr(uint16(at.Y))<<16)
	u.main.SetContextMenu(previous)
}
func (u *Window) export(action p.Action) {
	kind := "statistics"
	switch {
	case strings.Contains(action.Path, "colonization"):
		kind = "colonization"
	case strings.Contains(action.Path, "engineering"):
		kind = "engineering"
	case strings.Contains(action.Path, "route"):
		kind = "route"
	case strings.Contains(action.Path, "trading"):
		kind = "trading"
	}
	isJSON := strings.HasSuffix(action.Path, ".json")
	name := "ED Copilot — " + p.Title(u.page)
	ext := "csv"
	if isJSON {
		ext = "json"
	}
	dialog := walk.FileDialog{Title: "Сохранить " + name, FilePath: name + "." + ext, Filter: strings.ToUpper(ext) + " (*." + ext + ")|*." + ext}
	ok, err := dialog.ShowSave(u.main)
	if err != nil {
		u.error(err)
		return
	}
	if !ok {
		return
	}
	var bytes []byte
	projectID := v.Str(action.Payload["projectId"])
	if isJSON {
		if exporter, ok := u.backend.(interface {
			ExportPlanFor(string, string) ([]byte, error)
		}); ok {
			bytes, err = exporter.ExportPlanFor(kind, projectID)
		} else {
			bytes, err = u.backend.ExportPlan(kind)
		}
	} else {
		var text string
		if exporter, ok := u.backend.(interface {
			ExportCSVFor(string, string) (string, error)
		}); ok {
			text, err = exporter.ExportCSVFor(kind, projectID)
		} else {
			text, err = u.backend.ExportCSV(kind)
		}
		bytes = []byte(text)
	}
	if err == nil {
		err = os.WriteFile(dialog.FilePath, bytes, 0600)
	}
	if err != nil {
		u.error(err)
	} else {
		u.footer.SetText("Сохранено: " + filepath.Base(dialog.FilePath))
	}
}
