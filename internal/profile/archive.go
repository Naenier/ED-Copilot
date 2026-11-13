package profile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	backupFormat   = "ed-copilot-settings-backup"
	portableFormat = "ed-copilot-portable-profile"
	archiveVersion = 1
)

var (
	ErrInvalidArchive      = errors.New("invalid portable profile archive")
	ErrArchiveVersion      = errors.New("unsupported portable profile archive version")
	ErrDestinationNotEmpty = errors.New("profile destination is not empty")
)

type profileArchive struct {
	Format   string          `json:"format"`
	Version  int             `json:"version"`
	SavedAt  time.Time       `json:"savedAt"`
	Checksum string          `json:"sha256"`
	Settings json.RawMessage `json:"settings"`
}

func newArchive(format string, settings []byte) profileArchive {
	a := profileArchive{Format: format, Version: archiveVersion, SavedAt: time.Now().UTC(), Settings: settings}
	a.Checksum = a.checksum()
	return a
}

func (a profileArchive) checksum() string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(a.SavedAt.UTC().Format(time.RFC3339Nano)))
	_, _ = hash.Write(a.Settings)
	return hex.EncodeToString(hash.Sum(nil))
}

func (a profileArchive) generation(settings v.M) Generation {
	g := Generation{ID: a.Checksum, SavedAt: a.SavedAt}
	if language := v.Str(v.Obj(settings["settings"])["language"]); language == "en" || language == "ru" {
		g.Language = language
	}
	for _, section := range settingsSections {
		for _, key := range section.fields {
			if settings[key] != nil {
				g.Sections = append(g.Sections, section.id)
				break
			}
		}
	}
	g.Counts = map[string]int{
		"goals":       len(v.Rows(settings["objectives"])),
		"colonies":    len(v.Rows(v.At(settings, "colonization", "projects"))),
		"engineering": len(v.Rows(v.At(settings, "engineering", "items"))),
		"trade":       len(v.Rows(settings["savedTradeRoutes"])),
	}
	return g
}

func readArchive(file, format string) (profileArchive, v.M, error) {
	var a profileArchive
	data, err := readLimited(file, maxArchiveSize)
	if err != nil {
		return a, nil, err
	}
	if err := decodeObject(data, &a, true); err != nil || a.Format != format || a.SavedAt.IsZero() {
		return a, nil, ErrInvalidArchive
	}
	if a.Version != archiveVersion {
		return a, nil, ErrArchiveVersion
	}
	var compact bytes.Buffer
	if len(a.Settings) > maxSettingsSize || json.Compact(&compact, a.Settings) != nil {
		return a, nil, ErrInvalidArchive
	}
	a.Settings = compact.Bytes()
	if a.checksum() != a.Checksum {
		return a, nil, ErrInvalidArchive
	}
	settings, err := decodeSettings(a.Settings)
	if err != nil {
		return a, nil, err
	}
	if format == portableFormat {
		// A canonical payload cannot hide overwritten duplicate fields from
		// validation while keeping them in the file written by import.
		canonical, err := json.Marshal(settings)
		if err != nil || !bytes.Equal(canonical, a.Settings) {
			return a, nil, ErrInvalidArchive
		}
		if err := checkPortableSettings(settings); err != nil {
			return a, nil, err
		}
	}
	return a, settings, nil
}

func writeArchive(file string, archive profileArchive) error {
	if len(archive.Settings) > maxSettingsSize {
		return ErrInvalidArchive
	}
	data, err := json.Marshal(archive)
	if err != nil {
		return err
	}
	if len(data) > maxArchiveSize {
		return ErrInvalidArchive
	}
	return writeAtomic(file, data, replace)
}

// ExportPortable saves the current settings snapshot, including unsaved edits.
// An archive carries only the application domains explicitly supported here.
func ExportPortable(file string, settings v.M) error {
	selected := make(v.M)
	for key, value := range settings {
		if portableField(key) {
			selected[key] = value
		}
	}
	data, err := encodeSettings(selected)
	if err != nil {
		return err
	}
	payload, err := decodeSettings(data)
	if err != nil {
		return err
	}
	stripSecrets(payload)
	data, err = encodeSettings(payload)
	if err != nil {
		return err
	}

	return writeArchive(file, newArchive(portableFormat, data))
}

func InspectPortable(file string) (Generation, error) {
	archive, settings, err := readArchive(file, portableFormat)
	if err != nil {
		return Generation{}, err
	}
	return archive.generation(settings), nil
}

// ImportPortable always writes to a separate, empty directory. The final
// install fails if a settings file appeared while the import was in progress.
func ImportPortable(file, newDir string) error {
	if strings.TrimSpace(newDir) == "" {
		return ErrDestinationNotEmpty
	}

	archive, _, err := readArchive(file, portableFormat)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(newDir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(newDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return ErrDestinationNotEmpty
	}
	entries, err := os.ReadDir(newDir)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return ErrDestinationNotEmpty
	}
	return writeAtomic(filepath.Join(newDir, SettingsFile), archive.Settings, installNew)
}

func portableField(key string) bool {
	switch key {
	case "settings", "appearance", "navigation", "objectives", "bookmarks", "tradePlan",
		"tradingDestination", "savedTradeRoutes", "manualPrices", "statsSince", "carrier",
		"carrierCalculator", "carrierRouteGo", "carrierTrade", "colonization", "engineering",
		"exobiology", "richesRoutes", "learning", "ranks", "politics", "mining",
		"shipBuilds", "engineeringItineraries", "currentShipEngineeringPlans", "exploration", "shipRoutes", "surfaceNavigationTargets", "notifications":
		return true
	default:
		return false
	}
}

func checkPortableSettings(settings v.M) error {
	if settings["profileVersion"] == nil {
		return ErrInvalidArchive
	}
	for key := range settings {
		if key != "profileVersion" && !portableField(key) {
			return fmt.Errorf("%w: unsupported settings section", ErrInvalidArchive)
		}
	}
	if stripSecrets(settings) {
		return fmt.Errorf("%w: archive contains private fields", ErrInvalidArchive)
	}
	return nil
}

func stripSecrets(value any) bool {
	stripped := false
	switch rows := value.(type) {
	case v.M:
		for key, child := range rows {
			if privateKey(key) {
				delete(rows, key)
				stripped = true
			} else if stripSecrets(child) {
				stripped = true
			}
		}
	case []any:
		for _, row := range rows {
			if stripSecrets(row) {
				stripped = true
			}
		}
	}
	return stripped
}

func privateKey(key string) bool {
	key = v.Key(key)
	switch key {
	case "apikey", "auth", "authorization", "authentication", "bearer", "browser", "browsercache",
		"browserdata", "clientid", "connection", "cookie", "cookies", "frontier", "inara",
		"integrations", "journal", "journaldir", "oauth", "password", "passphrase", "privatekey", "dpapi", "accounts",
		"secret", "sessionid", "sharedkey", "token":
		return true
	}
	for _, part := range []string{"apikey", "authorization", "credential", "cookie", "oauth", "password", "privatekey", "secret", "token"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}
