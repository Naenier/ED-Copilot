//go:build windows

package nativeui

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/localization"
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	"github.com/Naenier/ED-Copilot/internal/profile"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
)

func RestoreProfileOnStartup(dir string, recovery *profile.RecoveryError) (bool, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	language := v.Text(recovery.Generations[0].Language, "en")
	t := func(s string) string { return localization.T(language, s) }
	message := t("Настройки не удалось прочитать. Выберите проверенную копию. Повреждённый файл будет сохранён отдельно.")
	if errors.Is(recovery, os.ErrNotExist) {
		message = t("Основной файл настроек отсутствует. Выберите проверенную копию.")
	}
	model := make([]string, 0, len(recovery.Generations))
	for _, g := range recovery.Generations {
		date := g.SavedAt.Local().Format("2006-01-02 15:04:05")
		if language == "ru" {
			date = g.SavedAt.Local().Format("02.01.2006 15:04:05")
		}
		model = append(model, date)
	}

	var dialog *walk.Dialog
	var picker *walk.ComboBox
	var details *walk.TextLabel
	var restore, cancel *walk.PushButton
	config := d.Dialog{
		AssignTo: &dialog, Title: t("Восстановление профиля"), MinSize: d.Size{Width: 640},
		Layout: d.VBox{}, DefaultButton: &restore, CancelButton: &cancel,
		Children: []d.Widget{
			d.TextLabel{Text: message},
			d.ComboBox{AssignTo: &picker, Model: model, CurrentIndex: 0, MinSize: d.Size{Width: 580}},
			d.TextLabel{AssignTo: &details, Text: generationContents(recovery.Generations[0], language)},
			d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
				d.HSpacer{},
				d.PushButton{AssignTo: &cancel, Text: t("Отмена"), OnClicked: func() { dialog.Cancel() }},
				d.PushButton{AssignTo: &restore, Text: t("Восстановить"), OnClicked: func() { dialog.Accept() }},
			}},
		},
	}
	if err := config.Create(nil); err != nil {
		if dialog != nil {
			dialog.Dispose()
		}
		return false, err
	}
	defer dialog.Dispose()
	picker.CurrentIndexChanged().Attach(func() {
		if index := picker.CurrentIndex(); index >= 0 && index < len(recovery.Generations) {
			_ = details.SetText(generationContents(recovery.Generations[index], language))
		}
	})
	if dialog.Run() != walk.DlgCmdOK {
		return false, nil
	}
	index := picker.CurrentIndex()
	if index < 0 || index >= len(recovery.Generations) {
		return false, nil
	}
	if err := profile.RestoreGeneration(dir, recovery.Generations[index].ID); err != nil {
		return false, err
	}
	return true, nil
}

func generationContents(g profile.Generation, language string) string {
	labels := map[string]string{
		"settings": "Настройки", "goals": "Цели", "trade": "Торговля",
		"carrier": "Флотоносец", "colonies": "Колонии", "engineering": "Инженерия",
		"exploration": "Экзобиология", "learning": "Академия", "ranks": "Ранги",
		"politics": "Политика", "mining": "Добыча",
	}
	var contents []string
	for _, section := range g.Sections {
		if label, ok := labels[section]; ok {
			label = localization.T(language, label)
			if n, ok := g.Counts[section]; ok {
				label += " (" + strconv.Itoa(n) + ")"
			}
			contents = append(contents, label)
		}
	}
	return strings.Join(contents, ", ")
}

type profileExporter interface {
	DataDir() string
	ExportProfile(string) error
}

type profileExitSaver interface {
	profileExporter
	CloseWithSave() error
	Save() error
}

func profileExportPath(owner walk.Form, activeDir, language string) (string, error) {
	d := walk.FileDialog{
		Title:    localization.T(language, "Сохранить копию профиля"),
		FilePath: "ED Copilot profile.json",
		Filter:   "JSON (*.json)|*.json",
	}
	ok, err := d.ShowSave(owner)
	if err != nil || !ok {
		return "", err
	}
	if filepath.Ext(d.FilePath) == "" {
		d.FilePath += ".json"
	}
	if err := requireSeparateProfilePath(activeDir, filepath.Dir(d.FilePath)); err != nil {
		return "", err
	}
	return d.FilePath, nil
}

func requireSeparateProfilePath(activeDir, destinationDir string) error {
	active, err := filepath.EvalSymlinks(activeDir)
	if os.IsNotExist(err) {
		active, err = activeDir, nil
	}
	if err != nil {
		return err
	}
	destination, err := filepath.EvalSymlinks(destinationDir)
	if err != nil {
		return err
	}
	active, err = filepath.Abs(active)
	if err != nil {
		return err
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}
	active = strings.ToLower(filepath.Clean(active))
	destination = strings.ToLower(filepath.Clean(destination))
	if destination == active || strings.HasPrefix(destination, strings.TrimSuffix(active, string(filepath.Separator))+string(filepath.Separator)) {
		return errors.New("Выберите папку за пределами текущего профиля")
	}
	return nil
}

func (u *Window) profileExportAction() (any, error) {
	exporter, ok := u.backend.(profileExporter)
	if !ok {
		return nil, errors.New("Копирование профиля недоступно")
	}
	file, err := profileExportPath(u.main, exporter.DataDir(), p.Language(u.state))
	if err != nil {
		return nil, err
	}
	if file == "" {
		return M{"cancelled": true}, nil
	}
	return func() (M, error) { return M{"ok": true}, exporter.ExportProfile(file) }, nil
}

func (u *Window) profileImportAction() (any, error) {
	language := p.Language(u.state)
	file := walk.FileDialog{Title: localization.T(language, "Открыть копию профиля"), Filter: "JSON (*.json)|*.json"}
	ok, err := file.ShowOpen(u.main)
	if err != nil {
		return nil, err
	}
	if !ok {
		return M{"cancelled": true}, nil
	}
	folder := walk.FileDialog{Title: localization.T(language, "Выберите пустую папку для восстановления")}
	ok, err = folder.ShowBrowseFolder(u.main)
	if err != nil {
		return nil, err
	}
	if !ok {
		return M{"cancelled": true}, nil
	}
	return func() (M, error) {
		if err := requireSeparateProfilePath(u.backend.DataDir(), folder.FilePath); err != nil {
			return nil, err
		}
		if err := profile.ImportPortable(file.FilePath, folder.FilePath); err != nil {
			switch {
			case errors.Is(err, profile.ErrDestinationNotEmpty):
				return nil, errors.New("Папка восстановления должна быть пустой")
			case errors.Is(err, profile.ErrInvalidArchive):
				return nil, errors.New("Файл копии профиля повреждён или имеет неверный формат")
			case errors.Is(err, profile.ErrArchiveVersion), errors.Is(err, profile.ErrUnsupportedSettingsVersion):
				return nil, errors.New("Эта версия ED Copilot не поддерживает формат выбранной копии")
			default:
				return nil, err
			}
		}
		return M{"ok": true, "folder": folder.FilePath}, nil
	}, nil
}

func ResolveProfileSaveFailure(owner walk.Form, backend profileExitSaver, language string, cause error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t := func(s string) string { return localization.T(language, s) }
	title := t("Не удалось сохранить профиль")
	for {
		action := profileSaveFailureChoice(owner, title, cause, t)
		switch action {
		case "retry":
			cause = backend.Save()
			if cause == nil {
				return
			}
		case "export":
			file, err := profileExportPath(owner, backend.DataDir(), language)
			if err == nil && file != "" {
				err = backend.ExportProfile(file)
				if err == nil {
					return
				}
			}
			if err != nil {
				walk.MsgBox(owner, t("Не удалось сохранить копию профиля"), localization.Text(language, err.Error()), walk.MsgBoxOK|walk.MsgBoxIconError)
			}
		case "discard":
			if walk.MsgBox(owner, title, t("Выйти без сохранения последних изменений?"), walk.MsgBoxYesNo|walk.MsgBoxIconWarning|walk.MsgBoxDefButton2) == walk.DlgCmdYes {
				return
			}
		}
	}
}

func profileSaveFailureChoice(owner walk.Form, title string, cause error, t func(string) string) string {
	var dialog *walk.Dialog
	var retry *walk.PushButton
	choice := ""
	choose := func(action string) func() { return func() { choice = action; dialog.Accept() } }
	config := d.Dialog{
		AssignTo: &dialog, Title: title, MinSize: d.Size{Width: 590}, Layout: d.VBox{}, DefaultButton: &retry,
		Children: []d.Widget{
			d.TextLabel{Text: t("Повторите сохранение или выгрузите последние изменения в отдельную копию.")},
			d.TextLabel{Text: cause.Error()},
			d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
				d.HSpacer{},
				d.PushButton{Text: t("Выйти без сохранения"), OnClicked: choose("discard")},
				d.PushButton{Text: t("Сохранить копию"), OnClicked: choose("export")},
				d.PushButton{AssignTo: &retry, Text: t("Повторить"), OnClicked: choose("retry")},
			}},
		},
	}
	if err := config.Create(owner); err != nil {
		if dialog != nil {
			dialog.Dispose()
		}
		switch walk.MsgBox(owner, title, t("Да — повторить сохранение. Нет — выгрузить копию. Отмена — перейти к выходу."), walk.MsgBoxYesNoCancel|walk.MsgBoxIconWarning) {
		case walk.DlgCmdYes:
			return "retry"
		case walk.DlgCmdNo:
			return "export"
		default:
			return "discard"
		}
	}
	defer dialog.Dispose()
	dialog.Run()
	return choice
}
