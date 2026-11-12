package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/profile"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type fakeHelper struct {
	calls                     []string
	parentError, desktopError error
	code                      int
	afterWait                 func()
	restartArgs               []string
	installedVersion          string
	postVersion               string
	installedCalled           bool
}

func (f *fakeHelper) lock() (func(), error) { return func() {}, nil }

func (f *fakeHelper) installed(string) bool { f.calls = append(f.calls, "installed"); return true }
func (f *fakeHelper) fileVersion(string) (string, error) {
	if f.installedCalled {
		if f.postVersion != "" {
			return f.postVersion, nil
		}
		return "2.7.0", nil
	}
	if f.installedVersion != "" {
		return f.installedVersion, nil
	}
	return "2.6.0", nil
}
func (f *fakeHelper) waitParent(context.Context, int, string) error {
	f.calls = append(f.calls, "parent")
	return f.parentError
}
func (f *fakeHelper) waitDesktop(context.Context) error {
	f.calls = append(f.calls, "mutex")
	if f.afterWait != nil {
		f.afterWait()
	}
	return f.desktopError
}
func (f *fakeHelper) install(_ context.Context, _ string, args []string) (int, error) {
	f.calls = append(f.calls, "install")
	f.installedCalled = true
	for _, arg := range args {
		if strings.HasPrefix(arg, "/TASKS") || strings.HasPrefix(arg, "/MERGETASKS") || arg == "/CLOSEAPPLICATIONS" {
			return -1, errors.New("unsafe installer options")
		}
	}
	return f.code, nil
}
func (f *fakeHelper) restart(_ string, args []string) error {
	f.calls = append(f.calls, "restart")
	f.restartArgs = args
	return nil
}

func stagedHelper(t *testing.T) (*updateFixture, string, string) {
	t.Helper()
	f := fixture(t)
	if err := profile.SaveSettings(f.m.opts.DataDir, v.M{"objectives": []v.M{{"title": "Before the update"}}}); err != nil {
		t.Fatal(err)
	}
	if err := f.m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	p, err := f.m.readPending()
	if err != nil {
		t.Fatal(err)
	}
	p.ParentPID = os.Getpid() + 10000
	manifest := filepath.Join(f.m.root, "pending.json")
	if err = writeJSON(manifest, p); err != nil {
		t.Fatal(err)
	}
	return f, manifest, filepath.Join(f.m.root, "2.7.0", "update-helper.exe")
}

func TestHelperWaitsBeforeInstallAndDoesNotRestartUserExit(t *testing.T) {
	f, manifest, helper := stagedHelper(t)
	system := &fakeHelper{}
	if err := runHelper(context.Background(), manifest, helper, system); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(system.calls, []string{"installed", "parent", "mutex", "install"}) {
		t.Fatal(system.calls)
	}
	p, err := f.m.readPending()
	if err != nil || !p.Applied {
		t.Fatal(p, err)
	}
}
func TestHelperRefusesInvalidPathsAndBusyApplication(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(string, string, *fakeHelper) (string, string)
	}{
		{"relative manifest", func(_, h string, _ *fakeHelper) (string, string) { return "updates/pending.json", h }},
		{"other helper", func(m, _ string, _ *fakeHelper) (string, string) {
			return m, filepath.Join(filepath.Dir(m), "unverified.exe")
		}},
		{"parent still running", func(m, h string, s *fakeHelper) (string, string) { s.parentError = errors.New("busy"); return m, h }},
		{"desktop still running", func(m, h string, s *fakeHelper) (string, string) { s.desktopError = errors.New("busy"); return m, h }},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, m, h := stagedHelper(t)
			s := &fakeHelper{}
			m, h = test.change(m, h, s)
			if err := runHelper(context.Background(), m, h, s); err == nil {
				t.Fatal("invalid helper accepted")
			}
			for _, call := range s.calls {
				if call == "install" {
					t.Fatal(s.calls)
				}
			}
		})
	}
}
func TestHelperRechecksDigestAfterParentExit(t *testing.T) {
	f, manifest, helper := stagedHelper(t)
	s := &fakeHelper{afterWait: func() {
		if err := os.WriteFile(filepath.Join(f.m.root, "2.7.0", "setup.exe"), []byte("changed"), 0600); err != nil {
			t.Fatal(err)
		}
	}}
	if err := runHelper(context.Background(), manifest, helper, s); err == nil {
		t.Fatal("modified setup accepted")
	}
	if !reflect.DeepEqual(s.calls, []string{"installed", "parent", "mutex"}) {
		t.Fatal(s.calls)
	}
}
func TestHelperInstallerFailureDoesNotMarkApplied(t *testing.T) {
	f, manifest, helper := stagedHelper(t)
	s := &fakeHelper{code: 5}
	if err := runHelper(context.Background(), manifest, helper, s); err == nil {
		t.Fatal("failed installer accepted")
	}
	p, err := f.m.readPending()
	if err != nil || p.Applied {
		t.Fatal(p, err)
	}
	recovery, err := readRecovery(f.m.root)
	if err != nil || recovery.Status != recoveryFailed {
		t.Fatalf("failed installation did not keep a recovery record: %v %v", recovery, err)
	}
	second := &fakeHelper{}
	if err := runHelper(context.Background(), manifest, helper, second); err == nil || len(second.calls) != 0 {
		t.Fatalf("failed release was retried: %v %v", second.calls, err)
	}
	if err := f.m.Check(context.Background()); err != nil || f.m.Snapshot().Status != "recovery" || f.m.Snapshot().Staged {
		t.Fatalf("the previous application should offer recovery instead of staging again: %v %v", f.m.Snapshot(), err)
	}
}

func TestUpdateRecoveryRequiresWorkspaceAndPreservesNewerProfile(t *testing.T) {
	f, manifest, helper := stagedHelper(t)
	if err := runHelper(context.Background(), manifest, helper, &fakeHelper{}); err != nil {
		t.Fatal(err)
	}
	recovery, err := readRecovery(f.m.root)
	if err != nil || recovery.Status != recoveryStarting || RecoveryLauncher(f.m.opts.DataDir, "2.7.0") == "" {
		t.Fatalf("installer completion counted as desktop startup: %v %v", recovery, err)
	}
	latest, err := New(Options{CurrentVersion: "2.7.0", DataDir: f.m.opts.DataDir, Executable: f.m.opts.Executable})
	if err != nil {
		t.Fatal(err)
	}
	latest.installed = f.m.installed
	if notice, err := latest.ConsumeNotice(); err != nil || notice != nil {
		t.Fatalf("notice before a usable workspace: %v %v", notice, err)
	}
	if err := profile.SaveSettings(f.m.opts.DataDir, v.M{"objectives": []v.M{{"title": "After the update"}}}); err != nil {
		t.Fatal(err)
	}
	previous := &fakeHelper{}
	backup := filepath.Join(recovery.directory(f.m.root), recoveryBinary)
	if err := launchRecovery(context.Background(), recovery.directory(f.m.root), backup, previous); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(previous.calls, []string{"mutex", "restart"}) || len(previous.restartArgs) != 2 || previous.restartArgs[0] != "--data-dir" {
		t.Fatalf("manual recovery ran installation or used the main profile: %v %v", previous.calls, previous.restartArgs)
	}
	check := func(dir, want string) {
		t.Helper()
		settings, err := profile.LoadSettings(dir)
		goals := v.Rows(settings["objectives"])
		if err != nil || len(goals) != 1 || goals[0]["title"] != want {
			t.Fatalf("wrong recovery contents: %v %v", goals, err)
		}
	}
	check(previous.restartArgs[1], "Before the update")
	check(f.m.opts.DataDir, "After the update")
	if err := latest.MarkWorkspaceReady(); err != nil {
		t.Fatal(err)
	}
	if current, err := readRecovery(f.m.root); err != nil || current.Status != recoveryReady {
		t.Fatalf("successful first workspace did not mark the update ready: %v %v", current, err)
	}
	if notice, err := latest.ConsumeNotice(); err != nil || notice == nil || notice.Version != "2.7.0" {
		t.Fatalf("missing update notice after workspace ready: %v %v", notice, err)
	}
}

func TestUpdateRecoveryHandlesPendingWindowsRestart(t *testing.T) {
	f, manifest, helper := stagedHelper(t)
	system := &fakeHelper{code: 3010, postVersion: "2.6.0"}
	if err := runHelper(context.Background(), manifest, helper, system); err != nil {
		t.Fatal(err)
	}
	recovery, err := readRecovery(f.m.root)
	p, pendingErr := f.m.readPending()
	if err != nil || pendingErr != nil || recovery.Status != recoveryReboot || p.Applied || len(system.restartArgs) != 0 {
		t.Fatalf("unsafe handling of a pending reboot: %v %v %v %v", recovery, p, err, pendingErr)
	}
	f.m.opts.CurrentVersion = "2.7.0" // The new executable opened after Windows restarted.
	if err := f.m.MarkWorkspaceReady(); err != nil {
		t.Fatal(err)
	}
	if p, err := f.m.readPending(); err != nil || !p.Applied {
		t.Fatalf("successful launch after reboot did not complete the update: %v %v", p, err)
	}
}

func TestInterruptedUpdateRequiresVerifiedTargetAndActualWorkspace(t *testing.T) {
	f, manifest, helper := stagedHelper(t)
	recovery, err := prepareRecovery(f.m.root, f.m.opts.Executable, "2.6.1", "2.7.0")
	if err != nil || recovery.Status != recoveryApplying {
		t.Fatal(recovery, err)
	}
	system := &fakeHelper{}
	if err := runHelper(context.Background(), manifest, helper, system); err == nil || len(system.calls) != 0 {
		t.Fatalf("incomplete installation was restarted: %v %v", system.calls, err)
	}
	if err := f.m.MarkWorkspaceReady(); err != nil {
		t.Fatal(err)
	}
	if current, _ := readRecovery(f.m.root); current.Status != recoveryApplying {
		t.Fatal("old workspace confirmed the new version", current)
	}
	f.m.opts.CurrentVersion = "2.7.0"
	if err := f.m.MarkWorkspaceReady(); err != nil {
		t.Fatal(err)
	}
	if notice, err := f.m.ConsumeNotice(); err != nil || notice == nil || notice.FromVersion != "2.6.1" {
		t.Fatalf("verified workspace could not complete an interrupted install: %v %v", notice, err)
	}
}
func TestMalformedHelperFlagsAreHandledWithoutNormalStartup(t *testing.T) {
	for _, args := range [][]string{{"--apply-update"}, {"--apply-update="}, {"--apply-update", "one", "two"}, {"--other", "--apply-update", "one"}} {
		if handled, err := HandleHelper(args); !handled || err == nil {
			t.Fatal(args, handled, err)
		}
	}
	if handled, err := HandleHelper([]string{"--version"}); handled || err != nil {
		t.Fatal(handled, err)
	}
}

func TestHelperCannotDowngradeAfterManualUpdate(t *testing.T) {
	f, manifest, helper := stagedHelper(t)
	s := &fakeHelper{installedVersion: "2.8.0"}
	if err := runHelper(context.Background(), manifest, helper, s); err == nil {
		t.Fatal("downgrade allowed")
	}
	for _, call := range s.calls {
		if call == "install" {
			t.Fatal(s.calls)
		}
	}
	p, _ := f.m.readPending()
	if p.Applied {
		t.Fatal("unexecuted update marked applied")
	}
}
func TestInstallerSuccessMustChangeActualProductVersion(t *testing.T) {
	f, manifest, helper := stagedHelper(t)
	s := &fakeHelper{postVersion: "2.6.0"}
	if err := runHelper(context.Background(), manifest, helper, s); err == nil {
		t.Fatal("unchanged binary accepted")
	}
	p, _ := f.m.readPending()
	if p.Applied {
		t.Fatal("wrong installed version marked applied")
	}
}
func TestPendingReplacementWhileWaitingAbortsStaleHelper(t *testing.T) {
	f, manifest, helper := stagedHelper(t)
	s := &fakeHelper{afterWait: func() {
		p, _ := f.m.readPending()
		p.ParentPID++
		if err := writeJSON(manifest, p); err != nil {
			t.Fatal(err)
		}
	}}
	if err := runHelper(context.Background(), manifest, helper, s); err == nil {
		t.Fatal("stale helper continued")
	}
	for _, call := range s.calls {
		if call == "install" {
			t.Fatal(s.calls)
		}
	}
}
func TestInstallerArgumentsPreserveUserChoices(t *testing.T) {
	args := installerArguments(`C:\Users\A Person\Programs\ED Copilot`, `C:\Users\A Person\profile\install.log`)
	joined := strings.Join(args, "|")
	for _, required := range []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/NOCLOSEAPPLICATIONS", "/NORESTARTAPPLICATIONS", "/DIR=C:\\Users\\A Person\\Programs\\ED Copilot"} {
		if !strings.Contains(joined, required) {
			t.Fatal(required, args)
		}
	}
	if strings.Contains(joined, "/TASKS") || strings.Contains(joined, "/INSTALLWEBVIEW2") {
		t.Fatal(args)
	}
}
