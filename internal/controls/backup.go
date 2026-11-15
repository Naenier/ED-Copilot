package controls

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const manifestFile = "backup.json"

var backupName = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[0-9a-f]{32}$`)

type backup struct {
	ID        string            `json:"id"`
	CreatedAt time.Time         `json:"createdAt"`
	Reason    string            `json:"reason"`
	Files     map[string]string `json:"files"`
}

func managedFile(name string) bool {
	_, _, allowed := localProfileName(name)
	return allowed || name == startFile
}

func (s *Service) makeBackup(files map[string][]byte, reason string) (string, error) {
	if err := makeDirectory(s.backupDir); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	info := backup{ID: now.Format("20060102T150405.000000000Z") + "-" + v.ID(),
		CreatedAt: now, Reason: reason, Files: map[string]string{}}
	dir := filepath.Join(s.backupDir, info.ID)
	if err := os.Mkdir(dir, 0o700); err != nil {
		return "", err
	}
	for name, data := range files {
		if !managedFile(name) {
			return "", errors.New("Недопустимое имя файла резервной копии.")
		}
		if err := createFile(filepath.Join(dir, name), data); err != nil {
			return "", err
		}
		info.Files[name] = digest(data)
	}
	manifest, err := json.Marshal(info)
	if err != nil {
		return "", err
	}
	if err = createFile(filepath.Join(dir, manifestFile), manifest); err != nil {
		return "", err
	}
	return info.ID, nil
}

func (s *Service) readBackup(id string) (backup, error) {
	info := backup{}
	if s.backupDir == "" || !backupName.MatchString(id) {
		return info, errors.New("Резервная копия управления не найдена.")
	}
	data, err := readFile(filepath.Join(s.backupDir, id, manifestFile), 64<<10)
	if err != nil {
		return info, err
	}
	if err = json.Unmarshal(data, &info); err != nil {
		return info, err
	}
	if info.ID != id || info.CreatedAt.IsZero() || info.Files == nil || len(info.Files) > maxProfiles+1 {
		return info, errors.New("Резервная копия управления повреждена.")
	}
	seen := map[string]bool{}
	for name, hash := range info.Files {
		folded := strings.ToLower(name)
		if !managedFile(name) || seen[folded] || len(hash) != 64 {
			return info, errors.New("Резервная копия управления повреждена.")
		}
		seen[folded] = true
	}
	return info, nil
}

func (s *Service) backupList() ([]M, error) {
	result := []M{}
	if s.backupDir == "" {
		return result, nil
	}
	entries, err := dirEntries(s.backupDir)
	if errors.Is(err, fs.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() || !backupName.MatchString(entry.Name()) {
			continue
		}
		info, err := s.readBackup(entry.Name())
		if err != nil {
			continue
		}
		names := []string{}
		for file := range info.Files {
			if name, _, ok := localProfileName(file); ok {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		result = append(result, M{"id": info.ID, "createdAt": info.CreatedAt.Format(time.RFC3339Nano),
			"reason": info.Reason, "profileNames": names})
	}
	return result, nil
}

func (s *Service) restoreFiles(id string, items inventory) (map[string][]byte, error) {
	info, err := s.readBackup(id)
	if err != nil {
		return nil, err
	}
	changes := map[string][]byte{}
	for name, hash := range info.Files {
		data, err := readFile(filepath.Join(s.backupDir, id, name), bindingLimit)
		if err != nil {
			return nil, err
		}
		if hash != digest(data) {
			return nil, errors.New("Содержимое резервной копии управления изменилось.")
		}
		for current := range items.localFiles {
			if strings.EqualFold(name, current) {
				name = current
				break
			}
		}
		changes[name] = data
	}
	if _, exists := changes[startFile]; !exists {
		changes[startFile] = nil
	}
	return changes, nil
}

type pendingChange struct {
	name      string
	old, next []byte
	oldPath   string
	nextPath  string
}

func (s *Service) commit(oldFiles, edits map[string][]byte, reason string) error {
	changes := make([]pendingChange, 0, len(edits))
	for name, next := range edits {
		if !managedFile(name) || len(next) > bindingLimit {
			return errors.New("Недопустимое имя или размер файла управления.")
		}
		old := oldFiles[name]
		if (old == nil) == (next == nil) && bytes.Equal(old, next) {
			continue
		}
		changes = append(changes, pendingChange{name: name, old: old, next: next})
	}
	if len(changes) == 0 {
		return nil
	}
	if err := s.writable(); err != nil {
		return err
	}
	if err := makeDirectory(s.dir); err != nil {
		return err
	}
	// Commit the active selection only after all profiles have been written.
	sort.Slice(changes, func(i, j int) bool {
		if (changes[i].name == startFile) != (changes[j].name == startFile) {
			return changes[j].name == startFile
		}
		return changes[i].name < changes[j].name
	})
	for i := range changes {
		c := &changes[i]
		if err := checkPrevious(filepath.Join(s.dir, c.name), c.old); err != nil {
			return err
		}
		var err error
		if c.old != nil {
			c.oldPath, err = stageFile(s.dir, c.old)
			if err != nil {
				return err
			}
			defer os.Remove(c.oldPath)
		}
		if c.next != nil {
			c.nextPath, err = stageFile(s.dir, c.next)
			if err != nil {
				return err
			}
			defer os.Remove(c.nextPath)
		}
	}
	if err := s.checkInventory(oldFiles); err != nil {
		return err
	}
	id, err := s.makeBackup(oldFiles, reason)
	if err != nil {
		return fmt.Errorf("Не удалось сохранить резервную копию управления: %w", err)
	}
	if err := s.writable(); err != nil {
		return err
	}
	if err := s.checkInventory(oldFiles); err != nil {
		return err
	}
	for i, c := range changes {
		path := filepath.Join(s.dir, c.name)
		err = s.writable()
		if err == nil {
			err = checkPrevious(path, c.old)
		}
		if err == nil && c.nextPath == "" {
			err = os.Remove(path)
		} else if err == nil {
			err = replaceFile(c.nextPath, path, c.oldPath != "")
		}
		if err != nil {
			if rollbackErr := s.rollback(changes[:i]); rollbackErr != nil {
				return errors.Join(err, fmt.Errorf("Не удалось отменить изменения. Резервная копия %s: %w", id, rollbackErr))
			}
			return err
		}
	}
	return nil
}

func (s *Service) checkInventory(oldFiles map[string][]byte) error {
	entries, err := dirEntries(s.dir)
	if errors.Is(err, fs.ErrNotExist) && len(oldFiles) == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if !managedFile(name) {
			continue
		}
		count++
		if err := checkPrevious(filepath.Join(s.dir, name), oldFiles[name]); err != nil {
			return err
		}
	}
	if count != len(oldFiles) {
		return errors.New("Список профилей изменился. Откройте профили управления заново.")
	}
	return nil
}

func (s *Service) rollback(changes []pendingChange) error {
	var result error
	for i := len(changes) - 1; i >= 0; i-- {
		c := changes[i]
		path := filepath.Join(s.dir, c.name)
		err := checkPrevious(path, c.next)
		if err == nil && c.oldPath == "" {
			err = os.Remove(path)
		} else if err == nil {
			err = replaceFile(c.oldPath, path, c.nextPath != "")
		}
		result = errors.Join(result, err)
	}
	return result
}

func checkPrevious(path string, expected []byte) error {
	data, err := readFile(path, bindingLimit)
	if errors.Is(err, fs.ErrNotExist) && expected == nil {
		return nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err != nil || expected == nil || !bytes.Equal(data, expected) {
		return changedFile(filepath.Base(path))
	}
	return nil
}
