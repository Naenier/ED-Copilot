package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
