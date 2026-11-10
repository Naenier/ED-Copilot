package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"os"
	"path/filepath"
	"sync"
)

var writeMu sync.Mutex

func Read(file string) (v.M, error) {
	b, e := os.ReadFile(file)
	if e != nil {
		return nil, e
	}
	if len(b) > maxSettingsSize {
		return nil, errors.New("Файл настроек слишком большой")
	}
	b = bytes.TrimPrefix(b, []byte{239, 187, 191})
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var out v.M
	if e = d.Decode(&out); e != nil {
		return nil, fmt.Errorf("Настройки %s: %w", filepath.Base(file), e)
	}
	return out, nil
}
func Write(file string, value any) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	b, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return e
	}
	return writeAtomic(file, b, replace)
}

func writeAtomic(file string, b []byte, install func(string, string) error) error {
	if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
		return e
	}
	tmp, e := os.CreateTemp(filepath.Dir(file), ".copilot-save-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, e = tmp.Write(b); e == nil {
		e = tmp.Sync()
	}
	closeErr := tmp.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return install(name, file)
}
func DefaultDir() string {
	if x := os.Getenv("ED_COPILOT_DATA_DIR"); x != "" {
		return x
	}
	if x := os.Getenv("LOCALAPPDATA"); x != "" {
		return filepath.Join(x, "ED Copilot")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ed-copilot")
}
func DefaultJournals() string {
	if x := os.Getenv("ED_JOURNAL_DIR"); x != "" {
		return x
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Saved Games", "Frontier Developments", "Elite Dangerous")
}
