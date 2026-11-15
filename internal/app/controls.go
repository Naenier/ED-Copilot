package app

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/controls"
)

const maxControlImport = 1 << 20

func discoverControlDirectories(o *Options) {
	if !o.DisableNetwork && !o.ManualPolling {
		if o.BindingsDir == "" {
			o.BindingsDir = controls.DefaultDirectory()
		}
		if o.ControlSchemesDir == "" {
			o.ControlSchemesDir = eliteControlSchemes()
		}
	}
}

func newControls(o Options) *controls.Service {
	return controls.New(o.BindingsDir, filepath.Join(o.DataDir, "controls-backups"), eliteMayBeRunning).WithBuiltIns(o.ControlSchemesDir)
}

// These paths come from owned system dialogs; the WebView passes only a
// profile identifier, while the bindings service validates imported XML.
func (e *Engine) ImportControlsFile(path string) (M, error) {
	if !strings.EqualFold(filepath.Ext(path), ".binds") {
		return nil, errors.New("Выберите файл .binds.")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxControlImport {
		return nil, errors.New("Файл управления слишком большой или недоступен.")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxControlImport+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxControlImport {
		return nil, errors.New("Файл управления слишком большой или недоступен.")
	}
	return e.controls.Import(data, "")
}

func (e *Engine) ExportControlsFile(id, path string) error {
	if !strings.EqualFold(filepath.Ext(path), ".binds") {
		return errors.New("Выберите файл .binds.")
	}
	// Exporting through a different path must not circumvent the editor's
	// game-running guard or replace a profile in the bindings folder.
	dir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return err
	}
	path = filepath.Join(dir, filepath.Base(path))
	for _, protected := range []string{e.opts.BindingsDir, e.opts.ControlSchemesDir, filepath.Join(e.opts.DataDir, "controls-backups")} {
		if protected != "" && withinControlsDirectory(dir, protected) {
			return errors.New("Сохраните копию за пределами папок управления игры и резервных копий.")
		}
	}
	data, err := e.controls.Export(id)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".copilot-controls-export-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp.Name(), path)
}

func withinControlsDirectory(dir, parent string) bool {
	if resolved, err := filepath.EvalSymlinks(parent); err == nil {
		parent = resolved
	}
	parent, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(strings.ToLower(parent), strings.ToLower(dir))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
