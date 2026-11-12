package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type helperSystem interface {
	lock() (func(), error)
	installed(string) bool
	fileVersion(string) (string, error)
	waitParent(context.Context, int, string) error
	waitDesktop(context.Context) error
	install(context.Context, string, []string) (int, error)
	restart(string, []string) error
}

// AcquireStartupGate serialises normal desktop initialisation with applying an
// update. Call it before creating the desktop instance mutex, then release it
// immediately after that mutex has been acquired. Release on the same goroutine.
func AcquireStartupGate(ctx context.Context) (func(), error) { return platformStartupGate(ctx) }

// HandleHelper must run before flag.Parse, profile loading or the desktop
// instance mutex. Recognised-but-malformed helper invocations are handled too,
// so they cannot accidentally start a second normal application session.
func HandleHelper(args []string) (bool, error) {
	for _, arg := range args {
		if arg == "--apply-update" || strings.HasPrefix(arg, "--apply-update=") {
			if len(args) != 2 || args[0] != "--apply-update" {
				return true, errors.New("invalid update helper arguments")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			executable, err := os.Executable()
			if err != nil {
				return true, err
			}
			return true, runHelper(ctx, args[1], executable, platformHelper{})
		}
	}
	return false, nil
}

func runHelper(ctx context.Context, manifest, helperPath string, system helperSystem) error {
	if !filepath.IsAbs(manifest) || filepath.Base(manifest) != "pending.json" || filepath.Base(filepath.Dir(manifest)) != "updates" {
		return errors.New("invalid update manifest path")
	}
	root := filepath.Dir(manifest)
	m := &Manager{root: root}
	p, err := m.readPending()
	if err != nil {
		return errors.New("update manifest unavailable")
	}
	if !filepath.IsAbs(p.Executable) || !strings.EqualFold(filepath.Base(p.Executable), "ED Copilot.exe") || p.ParentPID <= 4 || p.ParentPID == os.Getpid() {
		return errors.New("invalid original application identity")
	}
	m.opts.Executable = p.Executable
	expectedHelper := filepath.Join(root, p.Version, "update-helper.exe")
	if !samePath(helperPath, expectedHelper) {
		return errors.New("helper is outside its verified staging directory")
	}
	if err = m.verifyPending(p); err != nil {
		return err
	}
	unlock, err := system.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if p.Applied {
		return errors.New("update was already applied")
	}
	if _, blocked := unfinishedRecovery(root, p.Version); blocked {
		return errors.New("this release already has an update recovery record")
	}
	if !system.installed(p.Executable) {
		return errors.New("original installation no longer matches")
	}
	if err = system.waitParent(ctx, p.ParentPID, p.Executable); err != nil {
		return err
	}
	if err = system.waitDesktop(ctx); err != nil {
		return err
	}
	latest, err := m.readPending()
	if err != nil || latest != p {
		return errors.New("staged update changed while waiting")
	}
	// Recheck after waiting: the staged binary may have changed while the old
	// process was still finishing profile writes.
	if err = m.verifyPending(p); err != nil {
		return err
	}
	actualVersion, err := system.fileVersion(p.Executable)
	if err != nil || !newer(p.Version, actualVersion) {
		return errors.New("staged release is not newer than the installed executable")
	}
	recovery, err := prepareRecovery(root, p.Executable, actualVersion, p.Version)
	if err != nil {
		return errors.Join(errors.New("cannot save the previous application and profile"), err)
	}
	setup := filepath.Join(root, p.Version, "setup.exe")
	args := installerArguments(filepath.Dir(p.Executable), filepath.Join(root, p.Version, "install.log"))
	code, err := system.install(ctx, setup, args)
	if (err != nil && code != 3010) || (code != 0 && code != 3010) {
		recovery.Status = recoveryFailed
		return errors.Join(errors.New("quiet update installation did not complete"), writeRecovery(root, recovery))
	}
	actualVersion, err = system.fileVersion(p.Executable)
	if code == 3010 {
		recovery.Status = recoveryReboot
	} else if err != nil || actualVersion != p.Version {
		recovery.Status = recoveryFailed
		return errors.Join(errors.New("installed executable did not reach the verified target version"), writeRecovery(root, recovery))
	} else {
		recovery.Status = recoveryStarting
	}
	if err := writeRecovery(root, recovery); err != nil {
		return err
	}
	p.FromVersion = recovery.FromVersion
	p.Applied = err == nil && actualVersion == p.Version
	if err = writeJSON(manifest, p); err != nil {
		return errors.New("cannot record completed update")
	}
	// A requested OS reboot is never performed here. Only a fully completed
	// non-reboot installation can restart the application.
	if p.Restart && code == 0 {
		return system.restart(p.Executable, []string{"--data-dir", filepath.Dir(root)})
	}
	return nil
}

func installerArguments(installDir, logPath string) []string {
	return []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/SP-", "/NOCLOSEAPPLICATIONS", "/NORESTARTAPPLICATIONS", "/RESTARTEXITCODE=3010", "/DIR=" + installDir, "/LOG=" + logPath}
}
