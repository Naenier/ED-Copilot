package controls

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func canonicalPath(path string) string {
	if path == "" || strings.ContainsRune(path, 0) {
		return ""
	}
	result, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return filepath.Clean(result)
}

func pathsOverlap(a, b string) bool {
	if runtime.GOOS == "windows" {
		a, b = strings.ToLower(a), strings.ToLower(b)
	}
	inside := func(base, target string) bool {
		rel, err := filepath.Rel(base, target)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	return inside(a, b) || inside(b, a)
}

func plainPath(path string) (os.FileInfo, error) {
	parent := filepath.Dir(path)
	if parent != path {
		if _, err := plainPath(parent); err != nil {
			return nil, err
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if linkedPath(path, info) {
		return nil, errors.New("Ссылки и точки подключения в папке управления не поддерживаются.")
	}
	return info, nil
}

func makeDirectory(path string) error {
	info, err := plainPath(path)
	if err == nil && info.IsDir() {
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return err
		}
		return errors.New("Указанный путь не является папкой.")
	}
	if err = makeDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if err = os.Mkdir(path, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return makeDirectory(path)
}

func dirEntries(path string) ([]os.DirEntry, error) {
	info, err := plainPath(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("Указанный путь не является папкой.")
	}
	return os.ReadDir(path)
}

func readFile(path string, limit int) ([]byte, error) {
	info, err := plainPath(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > int64(limit) {
		return nil, errors.New("Недопустимый размер или тип файла управления.")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if len(data) > limit {
		return nil, errors.New("Файл управления слишком велик.")
	}
	return data, err
}

func saveFile(f *os.File, data []byte) error {
	_, err := f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}

func createFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	return saveFile(f, data)
}

func stageFile(dir string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, ".ed-copilot-bindings-*")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err = saveFile(f, data); err != nil {
		return "", errors.Join(err, os.Remove(path))
	}
	return path, nil
}

func changedFile(name string) error {
	return fmt.Errorf("Файл %s изменился. Откройте профили управления заново.", name)
}
