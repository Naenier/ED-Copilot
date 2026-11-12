package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/profile"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	recoveryApplying = "applying"
	recoveryFailed   = "installFailed"
	recoveryReboot   = "restartRequired"
	recoveryStarting = "awaitingWorkspace"
	recoveryReady    = "ready"
	recoveryLauncher = "Open previous ED Copilot.cmd"
	recoveryBinary   = "ED Copilot.exe"
	recoveryProfile  = "profile.json"
)

type recoveryRecord struct {
	Slot        string `json:"slot"`
	FromVersion string `json:"fromVersion"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	JournalDir  string `json:"journalDir,omitempty"`
}

func recoveryRecordPath(root string) string           { return filepath.Join(root, "recovery", "state.json") }
func (r recoveryRecord) directory(root string) string { return filepath.Join(root, "recovery", r.Slot) }

func writeRecovery(root string, r recoveryRecord) error {
	return profile.Write(recoveryRecordPath(root), r)
}

func readRecovery(root string) (recoveryRecord, error) {
	var r recoveryRecord
	f, err := os.Open(recoveryRecordPath(root))
	if err != nil {
		return r, err
	}
	defer f.Close()
	const limit = 256 << 10
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || len(data) > limit || json.Unmarshal(data, &r) != nil ||
		(r.Slot != "a" && r.Slot != "b") || !newer(r.Version, r.FromVersion) ||
		!digestPattern.MatchString(r.SHA256) || r.Size <= 0 || r.Size > maxInstaller ||
		(r.JournalDir != "" && !validRecoveryJournalDir(r.JournalDir)) {
		return recoveryRecord{}, errors.New("invalid update recovery record")
	}
	switch r.Status {
	case recoveryApplying, recoveryFailed, recoveryReboot, recoveryStarting, recoveryReady:
		return r, nil
	}
	return recoveryRecord{}, errors.New("invalid update recovery status")
}

func validRecoveryJournalDir(path string) bool {
	return len(path) <= 32767 && filepath.IsAbs(path) && !strings.ContainsRune(path, 0)
}

func recoveryForVersion(root, target string) (recoveryRecord, bool) {
	r, err := readRecovery(root)
	return r, err == nil && r.Version == target
}

func unfinishedRecovery(root, target string) (recoveryRecord, bool) {
	r, matches := recoveryForVersion(root, target)
	return r, matches && r.Status != recoveryReady
}

func (m *Manager) setRecoveryState(s *State, r recoveryRecord) {
	s.Status, s.Staged = "recovery", false
	if r.Status == recoveryReboot {
		s.Status = "restartRequired"
	}
	s.RecoveryPath = filepath.Join(r.directory(m.root), recoveryLauncher)
}

func clearRecoverySlot(dir string) error {
	for _, name := range []string{recoveryLauncher, recoveryBinary, recoveryProfile} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func prepareRecovery(root, executable, fromVersion, targetVersion string) (recoveryRecord, error) {
	previous, err := readRecovery(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return recoveryRecord{}, err
	}
	settings, err := profile.LoadSettings(filepath.Dir(root))
	if err != nil || settings == nil {
		return recoveryRecord{}, errors.New("cannot read a compatible profile for update recovery")
	}
	r := recoveryRecord{Slot: "a", FromVersion: fromVersion, Version: targetVersion, Status: recoveryApplying}
	if path := v.Str(settings["journalDir"]); validRecoveryJournalDir(path) {
		r.JournalDir = path
	}
	if previous.Slot == r.Slot {
		r.Slot = "b"
	}
	dir := r.directory(root)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return recoveryRecord{}, err
	}
	if err := clearRecoverySlot(dir); err != nil {
		return recoveryRecord{}, err
	}
	if err := profile.ExportPortable(filepath.Join(dir, recoveryProfile), settings); err != nil {
		return recoveryRecord{}, err
	}
	r.Size, r.SHA256, err = copyRecoveryExecutable(executable, filepath.Join(dir, recoveryBinary))
	if err != nil {
		return recoveryRecord{}, err
	}
	const launch = "@echo off\r\n\"%~dp0ED Copilot.exe\" --recover-update \"%~dp0.\"\r\n"
	if err := os.WriteFile(filepath.Join(dir, recoveryLauncher), []byte(launch), 0600); err != nil {
		return recoveryRecord{}, err
	}
	if err := writeRecovery(root, r); err != nil {
		return recoveryRecord{}, err
	}
	if previous.Slot != "" {
		_ = clearRecoverySlot(previous.directory(root))
	}
	return r, nil
}

func copyRecoveryExecutable(source, destination string) (int64, string, error) {
	from, err := os.Open(source)
	if err != nil {
		return 0, "", err
	}
	defer from.Close()
	info, err := from.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxInstaller {
		return 0, "", errors.New("previous executable is unavailable or too large")
	}
	to, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return 0, "", err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(to, hash), io.LimitReader(from, maxInstaller+1))
	flushErr := to.Sync()
	closeErr := to.Close()
	if err := errors.Join(copyErr, flushErr, closeErr); err != nil {
		return 0, "", err
	}
	if size != info.Size() {
		return 0, "", errors.New("previous executable changed during backup")
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}

// RecoveryLauncher finds the local manual launch path for a target whose
// installation or first workspace has not been confirmed.
func RecoveryLauncher(dataDir, target string) string {
	root := filepath.Join(dataDir, "updates")
	r, ok := unfinishedRecovery(root, target)
	if !ok {
		return ""
	}
	return filepath.Join(r.directory(root), recoveryLauncher)
}

// LaunchRecovery opens the verified previous executable with a separate copy
// of the pre-update settings. It never installs or writes to the main profile.
func LaunchRecovery(directory string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return launchRecovery(ctx, directory, executable, platformHelper{})
}

func launchRecovery(ctx context.Context, directory, executable string, system helperSystem) error {
	directory = filepath.Clean(directory)
	parent := filepath.Dir(directory)
	root := filepath.Dir(parent)
	if !filepath.IsAbs(directory) || filepath.Base(parent) != "recovery" || filepath.Base(root) != "updates" {
		return errors.New("invalid update recovery directory")
	}
	unlock, err := system.lock()
	if err != nil {
		return err
	}
	defer unlock()
	r, err := readRecovery(root)
	if err != nil || !samePath(r.directory(root), directory) || !samePath(executable, filepath.Join(directory, recoveryBinary)) {
		return errors.New("the saved update recovery copy is no longer active")
	}
	if err := verifyFile(executable, r.Size, r.SHA256); err != nil {
		return errors.New("the saved previous executable has changed")
	}
	if err := system.waitDesktop(ctx); err != nil {
		return err
	}
	working := filepath.Join(filepath.Dir(root), "recovered-updates", r.FromVersion+"-to-"+r.Version)
	settings, err := profile.LoadSettings(working)
	if err != nil {
		return errors.Join(errors.New("the separate recovered profile could not be opened"), err)
	}
	if settings == nil {
		if err := profile.ImportPortable(filepath.Join(directory, recoveryProfile), working); err != nil {
			return err
		}
	}
	args := []string{"--data-dir", working}
	if r.JournalDir != "" && v.Str(settings["journalDir"]) == "" {
		args = append(args, "--journal-dir", r.JournalDir)
	}
	return system.restart(executable, args)
}

// MarkWorkspaceReady is called after the first desktop page has loaded.
func (m *Manager) MarkWorkspaceReady() error {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	r, ok := recoveryForVersion(m.root, m.opts.CurrentVersion)
	if !ok || (r.Status != recoveryStarting && r.Status != recoveryReboot && r.Status != recoveryApplying) || !m.installed(m.opts.Executable) {
		return nil
	}
	p, err := m.readPending()
	if r.Status == recoveryApplying && (err != nil || p.Version != r.Version) {
		return errors.New("interrupted installation has no verified pending update")
	}
	if err == nil && p.Version == r.Version && (!p.Applied || p.FromVersion != r.FromVersion || r.Status == recoveryApplying) {
		if err := m.verifyPending(p); err != nil {
			return err
		}
		p.FromVersion = r.FromVersion
		p.Applied = true
		if err := writeJSON(filepath.Join(m.root, "pending.json"), p); err != nil {
			return err
		}
	}
	r.Status = recoveryReady
	return writeRecovery(m.root, r)
}
