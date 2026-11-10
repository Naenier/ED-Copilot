//go:build windows

package nativeui

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/data"
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/walk"
	"io"
	"os"
	"strings"
	"time"
)

func Actions(tab, mode string, state, selected, prefill M) []p.Action {
	return p.CoreActions(tab, mode, state, selected, prefill)
}

type fieldControl struct {
	spec    p.Field
	line    *walk.LineEdit
	text    *walk.TextEdit
	combo   *walk.ComboBox
	check   *walk.CheckBox
	choices []p.Choice
}

func (c *fieldControl) raw() any {
	if c.check != nil {
		return c.check.Checked()
	}
	if c.combo != nil {
		if i := c.combo.CurrentIndex(); i >= 0 && i < len(c.choices) {
			return c.choices[i].Value
		}
		if c.spec.Kind == "combo" {
			return c.combo.Text()
		}
		return ""
	}
	if c.text != nil {
		return c.text.Text()
	}
	if c.line != nil {
		return c.line.Text()
	}
	return nil
}
func (c *fieldControl) setChoices(choices []p.Choice) {
	previous := v.Str(c.raw())
	c.choices = choices
	c.spec.Choices = choices
	names := []string{}
	index := -1
	for i, x := range choices {
		names = append(names, x.Label)
		if x.Value == previous {
			index = i
		}
	}
	_ = c.combo.SetModel(names)
	if index < 0 && len(choices) > 0 {
		index = 0
	}
	_ = c.combo.SetCurrentIndex(index)
}
func (u *Window) form(action p.Action) (M, bool) {
	u.dialogOpen = true
	defer func() { u.dialogOpen = false }()
	dialog, err := walk.NewDialog(u.main)
	if err != nil {
		u.error(err)
		return nil, false
	}
	defer dialog.Dispose()
	_ = dialog.SetTitle(action.Title + " · ED Copilot")
	dialog.SetFont(u.font)
	height := min(850, max(260, len(action.Fields)*76+110))
	if work, ok := monitorRect(u.main.Handle(), true); ok {
		dpi := max(96, u.main.DPI())
		height = min(height, max(260, int(work.Bottom-work.Top)*96/dpi-48))
	}
	_ = dialog.SetSize(walk.Size{Width: 760, Height: height})
	_ = dialog.SetMinMaxSize(walk.Size{Width: 620, Height: 240}, walk.Size{})
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 18, VNear: 16, HFar: 18, VFar: 16})
	_ = dialog.SetLayout(layout)
	scroll, _ := walk.NewScrollView(dialog)
	scrollLayout := walk.NewVBoxLayout()
	scrollLayout.SetMargins(walk.Margins{HNear: 0, VNear: 0, HFar: 12, VFar: 0})
	scrollLayout.SetSpacing(9)
	_ = scroll.SetLayout(scrollLayout)
	_ = layout.SetStretchFactor(scroll, 1)
	controls := map[string]*fieldControl{}
	var firstFocusable walk.Widget
	for _, field := range action.Fields {
		f := field
		entry := &fieldControl{spec: f, choices: f.Choices}
		controls[f.Key] = entry
		group, _ := box(scroll, false)
		if f.Kind == "bool" {
			entry.check, _ = walk.NewCheckBox(group)
			_ = entry.check.SetText(f.Label)
			entry.check.SetChecked(v.Bool(f.Default))
			if firstFocusable == nil {
				firstFocusable = entry.check
			}
		} else {
			caption, _ := label(group, f.Label)
			_ = caption
			switch f.Kind {
			case "select", "combo":
				if f.Kind == "combo" {
					entry.combo, _ = walk.NewComboBox(group)
				} else {
					entry.combo, _ = walk.NewDropDownBox(group)
				}
				names := []string{}
				index := -1
				for i, c := range f.Choices {
					names = append(names, c.Label)
					if c.Value == v.Str(f.Default) {
						index = i
					}
				}
				_ = entry.combo.SetModel(names)
				if index < 0 && f.Kind == "select" && len(names) > 0 {
					index = 0
				}
				_ = entry.combo.SetCurrentIndex(index)
				if index < 0 && f.Kind == "combo" {
					_ = entry.combo.SetText(v.Str(f.Default))
				}
				if firstFocusable == nil {
					firstFocusable = entry.combo
				}
			case "multiline":
				entry.text, _ = walk.NewTextEdit(group)
				_ = entry.text.SetText(p.FieldText(f))
				fixedHeight(entry.text, 75)
				if firstFocusable == nil {
					firstFocusable = entry.text
				}
			default:
				row, _ := box(group, true)
				entry.line, _ = walk.NewLineEdit(row)
				_ = entry.line.SetText(p.FieldText(f))
				_ = row.Layout().(*walk.BoxLayout).SetStretchFactor(entry.line, 1)
				if f.Kind == "password" {
					entry.line.SetPasswordMode(true)
				}
				if f.Kind == "datetime" {
					_ = entry.line.SetCueBanner("ДД.ММ.ГГГГ ЧЧ:ММ")
				}
				if f.Kind == "file" {
					line := entry.line
					_, _ = button(row, "Выбрать…", func() {
						d := walk.FileDialog{Title: f.Label, Filter: "Настройки палитры (*.xml;*.ini)|*.xml;*.ini|Все файлы (*.*)|*.*"}
						ok, e := d.ShowOpen(dialog)
						if e != nil {
							walk.MsgBox(dialog, "ED Copilot", e.Error(), walk.MsgBoxIconWarning)
						} else if ok {
							_ = line.SetText(d.FilePath)
						}
					})
				}
				if firstFocusable == nil {
					firstFocusable = entry.line
				}
			}
		}
		if f.Help != "" {
			help, _ := label(group, f.Help)
			_ = help.SetMinMaxSize(walk.Size{Height: 30}, walk.Size{})
			help.SetTextColor(walk.RGB(70, 85, 100))
		}
	}
	configureEngineeringControls(controls)
	bar, _ := box(dialog, true)
	spacer, _ := label(bar, "")
	_ = bar.Layout().(*walk.BoxLayout).SetStretchFactor(spacer, 1)
	var result M
	save, _ := button(bar, "Применить", func() {
		values := M{}
		for _, f := range action.Fields {
			entry := controls[f.Key]
			value, e := p.ParseField(entry.spec, entry.raw())
			if e != nil {
				walk.MsgBox(dialog, "Проверьте поля", e.Error(), walk.MsgBoxIconWarning)
				return
			}
			values[f.Key] = value
		}
		if _, e := action.Body(values); e != nil {
			walk.MsgBox(dialog, "Проверьте поля", e.Error(), walk.MsgBoxIconWarning)
			return
		}
		result = values
		dialog.Accept()
	})
	cancel, _ := button(bar, "Отмена", func() { dialog.Cancel() })
	_ = dialog.SetDefaultButton(save)
	_ = dialog.SetCancelButton(cancel)
	darkWindow(dialog.Handle())
	// Walk initially sizes dialogs to their minimum layout hint. Apply the
	// requested scrollable viewport after Show has calculated that minimum.
	dialog.Starting().Attach(func() {
		_ = dialog.SetSize(walk.Size{Width: 760, Height: height})
		owner, bounds := u.main.BoundsPixels(), dialog.BoundsPixels()
		bounds.X = owner.X + (owner.Width-bounds.Width)/2
		bounds.Y = owner.Y + (owner.Height-bounds.Height)/2
		_ = dialog.SetBoundsPixels(bounds)
	})
	if firstFocusable != nil {
		firstFocusable.SetFocus()
	}
	accepted := dialog.Run() == walk.DlgCmdOK
	for _, c := range controls {
		if c.spec.Kind == "password" && c.line != nil {
			_ = c.line.SetText("")
		}
	}
	return result, accepted
}
func configureEngineeringControls(controls map[string]*fieldControl) {
	module, blueprint, effect := controls["moduleKey"], controls["blueprintKey"], controls["experimentalKey"]
	if module == nil || blueprint == nil || module.combo == nil || blueprint.combo == nil {
		return
	}
	catalog := data.Object("engineering-data.json")
	lookup := map[string]M{}
	for _, m := range v.Rows(catalog["modules"]) {
		lookup[v.Str(m["key"])] = m
	}
	bps := map[string]M{}
	for _, m := range v.Rows(catalog["blueprints"]) {
		bps[v.Str(m["key"])] = m
	}
	updating := false
	var update func()
	update = func() {
		if updating {
			return
		}
		updating = true
		defer func() { updating = false }()
		m := lookup[v.Str(module.raw())]
		bpChoices := []p.Choice{}
		for _, key := range v.Arr(m["blueprints"]) {
			r := bps[v.Str(key)]
			bpChoices = append(bpChoices, p.Choice{Value: v.Str(key), Label: v.Str(r["name"])})
		}
		blueprint.setChoices(bpChoices)
		if effect != nil && effect.combo != nil {
			allowed := map[string]bool{}
			for _, key := range v.Arr(m["experimentals"]) {
				allowed[v.Str(key)] = true
			}
			effectChoices := []p.Choice{{Value: "", Label: "Без эффекта"}}
			for _, r := range v.Rows(catalog["experimentals"]) {
				if allowed[v.Str(r["key"])] {
					effectChoices = append(effectChoices, p.Choice{Value: v.Str(r["key"]), Label: v.Str(r["name"])})
				}
			}
			effect.setChoices(effectChoices)
		}
		bp := bps[v.Str(blueprint.raw())]
		for i := 1; i <= 5; i++ {
			key := fmt.Sprint("roll", i)
			if c := controls[key]; c != nil && c.line != nil {
				_, valid := v.Obj(bp["grades"])[fmt.Sprint(i)]
				c.line.SetEnabled(valid)
				if !valid {
					_ = c.line.SetText("0")
				}
			}
		}
	}
	module.combo.CurrentIndexChanged().Attach(update)
	blueprint.combo.CurrentIndexChanged().Attach(update)
	update()
}
func (u *Window) runAction(action p.Action) {
	if action.Disabled {
		return
	}
	if action.Confirm != "" && walk.MsgBox(u.main, "ED Copilot", action.Confirm, walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
		return
	}
	switch action.Kind {
	case "navigate":
		if mode := v.Str(action.Payload["mode"]); mode != "" {
			u.modes[action.Path] = mode
		}
		u.navigate(action.Path)
		return
	case "tabs-reset":
		u.tabs = append([]string(nil), p.DefaultTabs...)
		valid := false
		for _, id := range u.tabs {
			if id == u.page {
				valid = true
			}
		}
		if !valid {
			u.page = "overview"
		}
		u.persistTabs()
		u.renderPage(true)
		return
	case "copy":
		text := v.Text(action.Path, action.Payload["text"], action.Payload["system"])
		if text != "" {
			u.error(walk.Clipboard().SetText(text))
			u.footer.SetText("Скопировано: " + text)
		}
		return
	case "url":
		u.error(openExternal(action.Path))
		return
	case "details":
		u.showSelectedDetails()
		return
	case "help":
		u.showHelp()
		return
	case "export":
		u.export(action)
		return
	case "folder":
		d := walk.FileDialog{Title: "Папка игровых журналов", FilePath: v.Str(v.At(u.state, "connection", "journalDir"))}
		ok, e := d.ShowBrowseFolder(u.main)
		if e != nil {
			u.error(e)
		} else if ok {
			u.execute(action, M{"path": d.FilePath})
		}
		return
	case "purchase":
		values := M{}
		if len(action.Fields) > 0 {
			var accepted bool
			values, accepted = u.form(action)
			if !accepted {
				return
			}
		}
		prefill, err := p.PurchasePrefill(action, values)
		if err != nil {
			u.error(err)
			return
		}
		u.prefill = prefill
		u.modes["sale"] = "Купить"
		u.navigate("sale")
		actions := Actions("sale", "Купить", u.state, M{}, u.prefill)
		if len(actions) > 0 {
			u.runAction(actions[0])
		}
		return
	case "import-plan":
		u.importPlan(action)
		return
	}
	values := M{}
	if len(action.Fields) > 0 {
		var accepted bool
		values, accepted = u.form(action)
		if !accepted {
			return
		}
	}
	if action.Kind == "theme-import" {
		u.importTheme(action, values)
		return
	}
	u.execute(action, values)
}
func (u *Window) importPlan(action p.Action) {
	d := walk.FileDialog{Title: "План инженерных улучшений", Filter: "План JSON (*.json)|*.json"}
	ok, err := d.ShowOpen(u.main)
	if err != nil {
		u.error(err)
		return
	}
	if !ok {
		return
	}
	file, err := os.Open(d.FilePath)
	if err != nil {
		u.error(err)
		return
	}
	defer file.Close()
	b, err := io.ReadAll(io.LimitReader(file, 2*1024*1024+1))
	if err != nil || len(b) > 2*1024*1024 {
		u.error(errors.New("Файл плана недоступен или превышает 2 МБ"))
		return
	}
	var plan M
	if err = json.Unmarshal(b, &plan); err != nil {
		u.error(errors.New("Некорректный файл плана JSON"))
		return
	}
	u.execute(p.Action{Title: action.Title, Path: action.Path, Payload: M{"action": "import", "items": plan["items"]}}, nil)
}
func (u *Window) importTheme(action p.Action, values M) {
	body, err := action.Body(values)
	if err != nil {
		u.error(err)
		return
	}
	go func() {
		result, err := u.backend.Do(action.Path, body)
		u.main.Synchronize(func() {
			if err != nil {
				u.error(err)
				return
			}
			theme := v.Obj(result["theme"])
			if u.previewTheme(theme) {
				u.execute(p.Action{Path: "/api/theme", Title: "Сохранить палитру", Payload: M{"settings": theme["settings"]}}, nil)
			}
		})
	}()
}
func (u *Window) previewTheme(theme M) bool {
	u.dialogOpen = true
	defer func() { u.dialogOpen = false }()
	dialog, err := walk.NewDialog(u.main)
	if err != nil {
		u.error(err)
		return false
	}
	defer dialog.Dispose()
	_ = dialog.SetTitle("Цвета из игры · предварительный просмотр")
	_ = dialog.SetSize(walk.Size{Width: 650, Height: 330})
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 18, VNear: 16, HFar: 18, VFar: 16})
	_ = dialog.SetLayout(layout)
	colors := v.Obj(v.At(theme, "settings", "colors"))
	preview, _ := walk.NewCustomWidget(dialog, 0, func(c *walk.Canvas, b walk.Rectangle) error {
		bg, _ := walk.NewSolidColorBrush(color(textColor(colors, "panel", "#0a1622")))
		defer bg.Dispose()
		_ = c.FillRectangle(bg, b)
		_ = c.DrawText("ED COPILOT · КУРС ПРОЛОЖЕН", u.boldFont, color(textColor(colors, "accent", "#28a0f0")), walk.Rectangle{X: 18, Y: 20, Width: 570, Height: 38}, walk.TextSingleLine)
		_ = c.DrawText("Корабль готов к вылету. Следующая система выбрана.", u.font, color(textColor(colors, "text", "#e0e0e0")), walk.Rectangle{X: 18, Y: 64, Width: 570, Height: 35}, walk.TextSingleLine)
		_ = c.DrawText("Проверяйте запас топлива перед прыжком.", u.font, color(textColor(colors, "warn", "#edbd66")), walk.Rectangle{X: 18, Y: 105, Width: 570, Height: 35}, walk.TextSingleLine)
		return nil
	})
	fixedHeight(preview, 165)
	source, _ := label(dialog, "Источник: "+v.Text(v.At(theme, "source", "name"), v.At(theme, "settings", "source", "name"), "Настройки игры"))
	_ = source
	bar, _ := box(dialog, true)
	ok, _ := button(bar, "Сохранить палитру", func() { dialog.Accept() })
	cancel, _ := button(bar, "Отмена", func() { dialog.Cancel() })
	_ = dialog.SetDefaultButton(ok)
	_ = dialog.SetCancelButton(cancel)
	return dialog.Run() == walk.DlgCmdOK
}
func showText(owner walk.Form, title, text string, colors M) {
	dialog, err := walk.NewDialog(owner)
	if err != nil {
		return
	}
	defer dialog.Dispose()
	_ = dialog.SetTitle(title)
	_ = dialog.SetSize(walk.Size{Width: 850, Height: 700})
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 18, VNear: 16, HFar: 18, VFar: 16})
	_ = dialog.SetLayout(layout)
	content, _ := walk.NewTextEdit(dialog)
	_ = content.SetReadOnly(true)
	_ = content.SetText(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n"))
	font, _ := walk.NewFont("Segoe UI", 11, 0)
	defer font.Dispose()
	content.SetFont(font)
	_ = layout.SetStretchFactor(content, 1)
	close, _ := button(dialog, "Закрыть", func() { dialog.Cancel() })
	_ = dialog.SetCancelButton(close)
	darkWindow(dialog.Handle())
	dialog.Run()
}

var _ = time.Now
