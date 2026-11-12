package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	backupFolder    = "profile-recovery"
	SettingsFile    = "settings-go-v2.json"
	legacyFile      = "settings.json"
	settingsVersion = 1
	backupCount     = 3
	maxSettingsSize = 16 * 1024 * 1024
	maxArchiveSize  = maxSettingsSize + 4096
)

var (
	ErrInvalidSettings            = errors.New("invalid profile settings")
	ErrUnsupportedSettingsVersion = errors.New("unsupported profile settings version")
	ErrGenerationUnavailable      = errors.New("profile backup is unavailable")
)

type Generation struct {
	ID       string         `json:"id"`
	SavedAt  time.Time      `json:"savedAt"`
	Sections []string       `json:"sections"`
	Language string         `json:"language,omitempty"`
	Counts   map[string]int `json:"counts,omitempty"`
}

type RecoveryError struct {
	Cause       error
	Generations []Generation
}

func (e *RecoveryError) Error() string { return e.Cause.Error() }
func (e *RecoveryError) Unwrap() error { return e.Cause }

type settingsSection struct {
	id     string
	fields []string
}

var settingsSections = []settingsSection{
	{"settings", []string{"settings", "appearance", "navigation", "journalDir"}},
	{"goals", []string{"objectives", "bookmarks"}},
	{"trade", []string{"tradePlan", "tradingDestination", "savedTradeRoutes", "manualPrices", "galacticPrices", "statsSince"}},
	{"carrier", []string{"carrier", "carrierCalculator", "carrierRouteGo", "carrierTrade"}},
	{"colonies", []string{"colonization"}},
	{"engineering", []string{"engineering", "shipBuilds", "engineeringItineraries"}},
	{"exploration", []string{"exobiology", "richesRoutes", "exploration", "shipRoutes"}},
	{"notifications", []string{"notifications"}},
	{"learning", []string{"learning"}},
	{"ranks", []string{"ranks"}},
	{"politics", []string{"politics"}},
	{"mining", []string{"mining"}},
}

func LoadSettings(dir string) (v.M, error) {
	writeMu.Lock()
	defer writeMu.Unlock()

	_, settings, err := readSettings(filepath.Join(dir, SettingsFile))
	if err == nil {
		return settings, nil
	}
	generations, hasBackups, listErr := scanGenerations(dir)
	if listErr != nil {
		return nil, errors.Join(err, listErr)
	}
	if !errors.Is(err, os.ErrNotExist) || hasBackups {
		return nil, &RecoveryError{Cause: err, Generations: generations}
	}
	_, settings, err = readSettings(filepath.Join(dir, legacyFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, &RecoveryError{Cause: err}
	}
	return settings, nil
}

func SaveSettings(dir string, settings v.M) error {
	data, err := encodeSettings(settings)
	if err != nil {
		return err
	}
	writeMu.Lock()
	defer writeMu.Unlock()

	file := filepath.Join(dir, SettingsFile)
	current, snapshot, err := readSettings(file)
	if err == nil && bytes.Equal(current, data) {
		// Imported profiles only contain the primary. Seed the first backup
		// on their next save, even when the settings are unchanged.
		if _, statErr := os.Lstat(filepath.Join(dir, backupFolder)); errors.Is(statErr, os.ErrNotExist) {
			return keepBackup(dir, snapshot, "")
		}
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return saveMissingPrimary(dir, file, data, err)
	}
	if err != nil {
		return err
	}
	if err := keepBackup(dir, snapshot, ""); err != nil {
		return err
	}
	return writeAtomic(file, data, replace)
}

func saveMissingPrimary(dir, file string, data []byte, missing error) error {
	_, snapshot, err := readSettings(filepath.Join(dir, legacyFile))
	if errors.Is(err, os.ErrNotExist) {
		snapshot, err = decodeSettings(data)
	}
	if err != nil {
		return err
	}
	generations, hasBackups, err := scanGenerations(dir)
	if err != nil {
		return err
	}
	if hasBackups {
		// A retry is safe when a verified copy of the state we just staged
		// is present. Startup itself always asks before restoring a missing primary.
		snapshotData, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		slots, err := backupSlots(dir)
		if err != nil {
			return err
		}
		for _, slot := range slots {
			if slot.modified.IsZero() {
				continue
			}
			archive, _, err := readArchive(slot.file, backupFormat)
			if err == nil && (bytes.Equal(archive.Settings, data) || bytes.Equal(archive.Settings, snapshotData)) {
				return writeAtomic(file, data, installNew)
			}
		}
		return &RecoveryError{Cause: missing, Generations: generations}
	}
	if err := keepBackup(dir, snapshot, ""); err != nil {
		return err
	}
	return writeAtomic(file, data, installNew)
}

func ListGenerations(dir string) ([]Generation, error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	generations, _, err := scanGenerations(dir)
	return generations, err
}

func scanGenerations(dir string) ([]Generation, bool, error) {
	slots, err := backupSlots(dir)
	if err != nil {
		return nil, false, err
	}
	var generations []Generation
	hasBackups := false
	seen := make(map[string]bool)
	for _, slot := range slots {
		if slot.modified.IsZero() {
			continue
		}
		hasBackups = true
		archive, settings, err := readArchive(slot.file, backupFormat)
		if err != nil {
			continue
		}
		generation := archive.generation(settings)
		if !seen[generation.ID] {
			generations = append(generations, generation)
			seen[generation.ID] = true
		}
	}
	sort.SliceStable(generations, func(i, j int) bool {
		return generations[i].SavedAt.After(generations[j].SavedAt)
	})
	return generations, hasBackups, nil
}

func RestoreGeneration(dir, id string) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	slots, err := backupSlots(dir)
	if err != nil {
		return err
	}
	for _, slot := range slots {
		if slot.modified.IsZero() {
			continue
		}
		archive, settings, err := readArchive(slot.file, backupFormat)
		if err != nil || archive.generation(settings).ID != id {
			continue
		}
		return restoreBackup(dir, slot.file, archive.Settings)
	}
	return ErrGenerationUnavailable
}

func restoreBackup(dir, backup string, data []byte) error {
	file := filepath.Join(dir, SettingsFile)
	current, snapshot, err := readSettings(file)
	switch {
	case err == nil:
		if bytes.Equal(current, data) {
			return nil
		}
		if err := keepBackup(dir, snapshot, backup); err != nil {
			return err
		}
		return writeAtomic(file, data, replace)
	case errors.Is(err, os.ErrNotExist):
		return writeAtomic(file, data, installNew)
	case errors.Is(err, ErrInvalidSettings):
		return writeAtomic(file, data, func(staged, target string) error {
			if err := preserveDamaged(target); err != nil {
				return err
			}
			return installNew(staged, target)
		})
	default:
		return err
	}
}

func preserveDamaged(file string) error {
	info, err := os.Lstat(file)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrInvalidSettings
	}
	archive, err := os.CreateTemp(filepath.Dir(file), "settings-go-v2.damaged-*.json")
	if err != nil {
		return err
	}
	if err = archive.Close(); err == nil {
		err = replace(file, archive.Name())
	}
	if err != nil {
		_ = os.Remove(archive.Name())
	}
	return err
}

type backupSlot struct {
	file     string
	modified time.Time
}

func backupSlots(dir string) ([]backupSlot, error) {
	folder := filepath.Join(dir, backupFolder)
	if info, err := os.Lstat(folder); err == nil && !info.IsDir() {
		return nil, fmt.Errorf("%w: profile-recovery is not a directory", ErrInvalidSettings)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	slots := make([]backupSlot, 0, backupCount)
	for i := 1; i <= backupCount; i++ {
		slot := backupSlot{file: filepath.Join(folder, "settings-"+strconv.Itoa(i)+".json")}
		info, err := os.Lstat(slot.file)
		switch {
		case err == nil && info.Mode().IsRegular():
			slot.modified = info.ModTime()
		case err == nil:
			return nil, fmt.Errorf("%w: backup slot %d is not a regular file", ErrInvalidSettings, i)
		case !errors.Is(err, os.ErrNotExist):
			return nil, err
		}
		slots = append(slots, slot)
	}
	sort.SliceStable(slots, func(i, j int) bool { return slots[i].modified.Before(slots[j].modified) })
	return slots, nil
}

func keepBackup(dir string, settings v.M, protectedFile string) error {
	compact, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	slots, err := backupSlots(dir)
	if err != nil {
		return err
	}
	newest := slots[len(slots)-1]
	if !newest.modified.IsZero() {
		archive, _, err := readArchive(newest.file, backupFormat)
		if err == nil && bytes.Equal(archive.Settings, compact) {
			return nil
		}
	}
	archive := newArchive(backupFormat, compact)
	for _, slot := range slots {
		if slot.file != protectedFile {
			return writeArchive(slot.file, archive)
		}
	}
	return ErrGenerationUnavailable
}

func readSettings(file string) ([]byte, v.M, error) {
	data, err := readLimited(file, maxSettingsSize)
	if err != nil {
		return nil, nil, err
	}
	settings, err := decodeSettings(data)
	return data, settings, err
}

func readLimited(file string, limit int64) ([]byte, error) {
	info, err := os.Lstat(file)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", ErrInvalidSettings, filepath.Base(file))
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("%w: %s exceeds the size limit", ErrInvalidSettings, filepath.Base(file))
	}
	reader, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: %s exceeds the size limit", ErrInvalidSettings, filepath.Base(file))
	}
	return data, err
}

func decodeSettings(data []byte) (v.M, error) {
	var settings v.M
	if err := decodeObject(data, &settings, false); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSettings, err)
	}
	if version, ok := settings["profileVersion"]; ok {
		number, valid := version.(json.Number)
		if !valid {
			return nil, ErrInvalidSettings
		}
		if number != json.Number(strconv.Itoa(settingsVersion)) {
			return nil, ErrUnsupportedSettingsVersion
		}
	}
	hasFields := false
	for _, section := range settingsSections {
		for _, key := range section.fields {
			value, exists := settings[key]
			if !exists {
				continue
			}
			switch key {
			case "journalDir", "statsSince":
				if _, valid := value.(string); !valid {
					return nil, invalidField(key)
				}
			case "objectives", "bookmarks", "savedTradeRoutes":
				if value == nil {
					continue
				}
				rows, valid := value.([]any)
				if !valid {
					return nil, invalidField(key)
				}
				for _, row := range rows {
					if _, valid := row.(v.M); !valid {
						return nil, invalidField(key)
					}
				}
			default:
				if _, valid := value.(v.M); !valid {
					return nil, invalidField(key)
				}
			}
			hasFields = true
		}
	}
	if !hasFields {
		return nil, ErrInvalidSettings
	}
	return settings, nil
}

func invalidField(name string) error {
	return fmt.Errorf("%w: invalid %s", ErrInvalidSettings, name)
}

func encodeSettings(settings v.M) ([]byte, error) {
	snapshot := make(v.M, len(settings)+1)
	for key, value := range settings {
		snapshot[key] = value
	}
	snapshot["profileVersion"] = settingsVersion
	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	if len(data) > maxSettingsSize {
		return nil, ErrInvalidSettings
	}
	_, err = decodeSettings(data)
	return data, err
}

func decodeObject(data []byte, out any, strict bool) error {
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte{239, 187, 191}))
	if len(data) == 0 || data[0] != '{' || !utf8.Valid(data) {
		return ErrInvalidSettings
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if strict {
		d.DisallowUnknownFields()
	}
	if err := d.Decode(out); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return ErrInvalidSettings
	}
	return nil
}
