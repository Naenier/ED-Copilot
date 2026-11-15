// Package controls manages local Elite Dangerous Live control profiles.
package controls

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	startFile    = "StartPreset.4.start"
	bindingLimit = 1 << 20
	maxProfiles  = 128
	contextCount = 4
)

var contexts = [contextCount]string{"general", "ship", "srv", "onfoot"}
var localFile = regexp.MustCompile(`(?i)^(.+)\.4\.([0-9]{1,2})\.binds$`)

type Service struct {
	mu                         sync.Mutex
	dir, backupDir, builtinDir string
	gameRunning                func() bool
	builtins                   []*controlProfile
	builtinsRead               bool
}

func New(dir, backupDir string, gameRunning func() bool) *Service {
	return &Service{dir: canonicalPath(dir), backupDir: canonicalPath(backupDir), gameRunning: gameRunning}
}

func (s *Service) WithBuiltIns(dir string) *Service {
	s.mu.Lock()
	s.builtinDir = canonicalPath(dir)
	s.builtins, s.builtinsRead = nil, false
	s.mu.Unlock()
	return s
}

func DefaultDirectory() string {
	if path := os.Getenv("LOCALAPPDATA"); path != "" {
		return filepath.Join(path, "Frontier Developments", "Elite Dangerous", "Options", "Bindings")
	}
	return ""
}

type controlProfile struct {
	id, name, file, revision string
	major, minor             int
	readOnly                 bool
	data                     []byte
	root                     *node
}

type inventory struct {
	localFiles map[string][]byte
	profiles   []*controlProfile
	selected   presets
	presetErr  error
	available  bool
	skipped    int
}

func (s *Service) View(input M) (M, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.load()
	if err != nil {
		return nil, err
	}
	return s.view(items, v.Str(input["profileId"]))
}

func (s *Service) view(items inventory, profileID string) (M, error) {
	profiles := make([]M, 0, len(items.profiles))
	selected := make([]M, 0, contextCount)
	for _, p := range items.profiles {
		profiles = append(profiles, items.summary(p))
	}
	if items.presetErr == nil {
		for index, key := range contexts {
			name := items.selected.names[index]
			selection := M{"context": key, "preset": name}
			if p := items.matchPreset(name); p != nil {
				selection["profileId"] = p.id
			}
			selected = append(selected, selection)
		}
	}
	backups, err := s.backupList()
	if err != nil {
		return nil, err
	}
	result := M{"available": items.available, "gameRunning": s.running(), "profiles": profiles,
		"selections": selected, "selectionValid": items.presetErr == nil, "backups": backups}
	if items.presetErr != nil {
		result["selectionError"] = items.presetErr.Error()
	}
	if items.skipped > 0 {
		result["skippedProfiles"] = items.skipped
	}
	p := items.profile(profileID)
	if p == nil && profileID != "" {
		return nil, errors.New("Профиль управления не найден.")
	}
	if p == nil {
		for _, index := range []int{1, 0, 2, 3} {
			if current := items.matchPreset(items.selected.names[index]); current != nil {
				p = current
				break
			}
		}
	}
	if p == nil && len(items.profiles) != 0 {
		p = items.profiles[0]
	}
	if p != nil {
		summary := items.summary(p)
		summary["actions"] = actionViews(p.root)
		result["profile"] = summary
	}
	return result, nil
}

func (items inventory) summary(p *controlProfile) M {
	active := []string{}
	for index, name := range items.selected.names {
		if selected := items.matchPreset(name); selected == p {
			active = append(active, contexts[index])
		}
	}
	layout := ""
	if field := p.root.child("KeyboardLayout"); field != nil && field.close >= field.open {
		layout = strings.TrimSpace(string(p.data[field.open:field.close]))
	}
	return M{"id": p.id, "name": p.name, "file": p.file, "readOnly": p.readOnly, "revision": p.revision,
		"majorVersion": p.major, "minorVersion": p.minor, "keyboardLayout": layout, "activeContexts": active}
}

func (items inventory) profile(id string) *controlProfile {
	for _, p := range items.profiles {
		if p.id == id {
			return p
		}
	}
	return nil
}

func (items inventory) matchPreset(name string) *controlProfile {
	var best *controlProfile
	for _, p := range items.profiles {
		if !strings.EqualFold(p.name, name) {
			continue
		}
		if best == nil || best.readOnly && !p.readOnly || best.readOnly == p.readOnly && best.minor < p.minor {
			best = p
		}
	}
	return best
}

func validName(name string) bool {
	if name == "" || len([]rune(name)) > 48 || strings.TrimSpace(name) != name ||
		strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") || strings.Contains(name, "..") {
		return false
	}
	for _, c := range name {
		if !unicode.IsLetter(c) && !unicode.IsNumber(c) && c != ' ' && c != '-' && c != '_' && c != '.' {
			return false
		}
	}
	first, _, _ := strings.Cut(name, ".")
	folded := strings.ToUpper(strings.TrimSpace(first))
	switch folded {
	case "CON", "NUL", "AUX", "PRN":
		return false
	}
	runes := []rune(folded)
	return !(len(runes) == 4 && (strings.HasPrefix(folded, "COM") || strings.HasPrefix(folded, "LPT")) &&
		strings.ContainsRune("0123456789¹²³", runes[3]))
}

func localProfileName(name string) (string, int, bool) {
	parts := localFile.FindStringSubmatch(name)
	if len(parts) != 3 || !validName(parts[1]) {
		return "", 0, false
	}
	minor, _ := strconv.Atoi(parts[2])
	return parts[1], minor, true
}

func newProfile(file string, data []byte, builtin bool) (*controlProfile, error) {
	root, err := parseXML(data)
	if err != nil {
		return nil, err
	}
	name := root.attr("PresetName")
	major, _ := strconv.Atoi(root.attr("MajorVersion"))
	minor, _ := strconv.Atoi(root.attr("MinorVersion"))
	prefix := "builtin:"
	if !builtin {
		fileName, fileMinor, valid := localProfileName(file)
		if !valid || !strings.EqualFold(name, fileName) || major != 4 || minor != fileMinor {
			return nil, errors.New("Имя или версия профиля не совпадает с его файлом.")
		}
		prefix = "local:"
	}
	return &controlProfile{id: prefix + file, name: name, file: file, revision: digest(data),
		major: major, minor: minor, readOnly: builtin, data: data, root: root}, nil
}

func digest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func (s *Service) load() (inventory, error) {
	items := inventory{localFiles: map[string][]byte{}}
	if s.dir != "" {
		entries, err := dirEntries(s.dir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return items, err
		}
		items.available = err == nil
		for _, entry := range entries {
			name := entry.Name()
			_, _, bind := localProfileName(name)
			if !bind && name != startFile {
				continue
			}
			if len(items.localFiles) >= maxProfiles+1 {
				return items, errors.New("Слишком много локальных профилей управления.")
			}
			data, err := readFile(filepath.Join(s.dir, name), bindingLimit)
			if err != nil {
				return items, err
			}
			items.localFiles[name] = data
			if name == startFile {
				continue
			}
			p, err := newProfile(name, data, false)
			if err != nil {
				items.skipped++
				continue
			}
			items.profiles = append(items.profiles, p)
		}
	}
	items.selected, items.presetErr = parsePresets(items.localFiles[startFile])
	if !s.builtinsRead && s.builtinDir != "" {
		s.builtinsRead = true
		entries, _ := dirEntries(s.builtinDir)
		for _, entry := range entries {
			if len(s.builtins) >= maxProfiles {
				break
			}
			name := entry.Name()
			if !strings.EqualFold(filepath.Ext(name), ".binds") || !validName(strings.TrimSuffix(name, filepath.Ext(name))) {
				continue
			}
			data, err := readFile(filepath.Join(s.builtinDir, name), bindingLimit)
			if err != nil {
				continue
			}
			p, err := newProfile(name, data, true)
			if err != nil {
				continue
			}
			s.builtins = append(s.builtins, p)
		}
	}
	items.profiles = append(items.profiles, s.builtins...)
	sort.SliceStable(items.profiles, func(i, j int) bool {
		a, b := items.profiles[i], items.profiles[j]
		if a.readOnly != b.readOnly {
			return !a.readOnly
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	})
	return items, nil
}

func (s *Service) running() bool { return s.gameRunning == nil || s.gameRunning() }

func (s *Service) writable() error {
	if s.running() {
		return errors.New("Закройте Elite Dangerous перед изменением управления.")
	}
	return s.validDirectories()
}

func (s *Service) validDirectories() error {
	if s.dir == "" || s.backupDir == "" || pathsOverlap(s.dir, s.backupDir) ||
		s.builtinDir != "" && (pathsOverlap(s.dir, s.builtinDir) || pathsOverlap(s.backupDir, s.builtinDir)) {
		return errors.New("Папки управления и резервных копий недоступны или пересекаются.")
	}
	return nil
}

func (s *Service) Update(input M) (M, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	action := v.Str(input["action"])
	check := s.writable
	if action == "backup" {
		check = s.validDirectories
	}
	if err := check(); err != nil {
		return nil, err
	}
	items, err := s.load()
	if err != nil {
		return nil, err
	}
	var (
		changes  map[string][]byte
		id       = v.Str(input["profileId"])
		backupID string
	)
	p := items.profile(id)
	if id != "" && p == nil {
		return nil, errors.New("Профиль управления не найден.")
	}
	switch action {
	case "backup":
		if err = s.checkInventory(items.localFiles); err == nil {
			backupID, err = s.makeBackup(items.localFiles, "manual")
		}
	case "restore":
		changes, err = s.restoreFiles(v.Str(input["backupId"]), items)
	case "activate":
		changes, err = activationChanges(items, p, input)
	case "edit", "clone":
		if p == nil {
			return nil, errors.New("Профиль управления не найден.")
		}
		if expected, supplied := input["expectedRevision"]; supplied && expected != p.revision {
			return nil, errors.New("Профиль управления изменился. Откройте его заново.")
		}
		if action == "edit" && p.readOnly {
			return nil, errors.New("Для изменения встроенного профиля создайте его копию.")
		}
		raw := v.Arr(input["changes"])
		rows := v.Rows(input["changes"])
		if len(raw) != len(rows) || input["changes"] != nil && raw == nil {
			return nil, errors.New("Некорректный список изменений управления.")
		}
		next, e := patchProfile(p.data, p.root, rows)
		if e != nil {
			return nil, e
		}
		if action == "clone" {
			changes, id, err = cloneChanges(items, p, next, strings.TrimSpace(v.Str(input["name"])))
		} else {
			if _, err = newProfile(p.file, next, false); err != nil {
				return nil, err
			}
			changes = map[string][]byte{p.file: next}
		}
	default:
		return nil, errors.New("Неизвестное действие управления.")
	}
	if err != nil {
		return nil, err
	}
	if len(changes) != 0 {
		if err = s.commit(items.localFiles, changes, action); err != nil {
			return nil, err
		}
		items, err = s.load()
		if err != nil {
			return nil, err
		}
	}
	result, err := s.view(items, id)
	if err == nil {
		result["ok"] = true
		if backupID != "" {
			result["backupId"] = backupID
		}
	}
	return result, err
}

func cloneChanges(items inventory, p *controlProfile, data []byte, name string) (map[string][]byte, string, error) {
	if !validName(name) {
		return nil, "", errors.New("Введите имя профиля до 48 символов. Можно использовать буквы, цифры, пробелы, точки, дефисы и подчёркивания.")
	}
	if items.nameTaken(name) {
		return nil, "", errors.New("Профиль управления с таким именем уже существует.")
	}
	minor := p.minor
	if p.readOnly {
		minor = 2
	}
	file := fmt.Sprintf("%s.4.%d.binds", name, minor)
	root, err := parseXML(data)
	if err != nil {
		return nil, "", err
	}
	next := rewriteName(data, root, name, p.readOnly)
	if _, err = newProfile(file, next, false); err != nil {
		return nil, "", err
	}
	return map[string][]byte{file: next}, "local:" + file, nil
}

func (items inventory) nameTaken(name string) bool {
	if items.matchPreset(name) != nil {
		return true
	}
	for _, preset := range items.selected.names {
		if strings.EqualFold(name, preset) {
			return true
		}
	}
	for candidate := range items.localFiles {
		existing, _, valid := localProfileName(candidate)
		if valid && strings.EqualFold(name, existing) {
			return true
		}
	}
	return false
}

func (s *Service) Import(data []byte, name string) (M, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writable(); err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > bindingLimit {
		return nil, errors.New("Недопустимый размер файла управления.")
	}
	root, err := parseXML(data)
	if err != nil {
		return nil, err
	}
	if root.attr("MajorVersion") != "4" || root.attr("MinorVersion") == "" {
		return nil, errors.New("Импорт поддерживает профили управления Elite Dangerous Live версии 4.")
	}
	minor, err := strconv.Atoi(root.attr("MinorVersion"))
	if err != nil || minor < 0 || minor > 99 {
		return nil, errors.New("Неизвестная версия файла управления.")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = root.attr("PresetName")
	}
	items, err := s.load()
	if err != nil {
		return nil, err
	}
	if items.nameTaken(name) {
		base := []rune(name)
		for number := 2; number <= maxProfiles+1; number++ {
			suffix := fmt.Sprintf(" %d", number)
			name = strings.TrimRight(string(base[:min(len(base), 48-len(suffix))]), " .") + suffix
			if !items.nameTaken(name) {
				break
			}
		}
	}
	p := &controlProfile{minor: minor}
	files, id, err := cloneChanges(items, p, data, name)
	if err != nil {
		return nil, err
	}
	if err = s.commit(items.localFiles, files, "import"); err != nil {
		return nil, err
	}
	items, err = s.load()
	if err != nil {
		return nil, err
	}
	return s.view(items, id)
}

func (s *Service) Export(profileID string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.load()
	if err != nil {
		return nil, err
	}
	if p := items.profile(profileID); p != nil {
		return bytes.Clone(p.data), nil
	}
	return nil, errors.New("Профиль управления не найден.")
}
