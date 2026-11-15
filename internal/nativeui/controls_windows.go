//go:build windows

package nativeui

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/localization"
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/walk"
)

func (u *Window) controlsFileAction(params M) (any, error) {
	language := p.Language(u.state)
	dialog := walk.FileDialog{Filter: "Elite Dangerous (*.binds)|*.binds"}
	if params["action"] == "controls-import" {
		importer, ok := u.backend.(interface{ ImportControlsFile(string) (M, error) })
		if !ok {
			return nil, errors.New("Импорт настроек управления недоступен.")
		}
		dialog.Title = localization.T(language, "Импорт профиля управления")
		accepted, err := dialog.ShowOpen(u.main)
		if err != nil || !accepted {
			return M{"cancelled": !accepted}, err
		}
		return func() (M, error) { return importer.ImportControlsFile(dialog.FilePath) }, nil
	}

	exporter, ok := u.backend.(interface{ ExportControlsFile(string, string) error })
	if !ok {
		return nil, errors.New("Экспорт настроек управления недоступен.")
	}
	profileID := v.Str(params["profileId"])
	if profileID == "" {
		return nil, errors.New("Выберите профиль управления.")
	}
	name := v.Str(params["filename"])
	if name != filepath.Base(name) || !strings.EqualFold(filepath.Ext(name), ".binds") || strings.ContainsAny(name, "<>:\"/\\|?*\r\n") {
		name = "Elite Dangerous.binds"
	}
	dialog.Title = localization.T(language, "Экспорт профиля управления")
	dialog.FilePath = name
	accepted, err := dialog.ShowSave(u.main)
	if err != nil || !accepted {
		return M{"cancelled": !accepted}, err
	}
	if filepath.Ext(dialog.FilePath) == "" {
		dialog.FilePath += ".binds"
	}
	return func() (M, error) {
		return M{"ok": true}, exporter.ExportControlsFile(profileID, dialog.FilePath)
	}, nil
}
