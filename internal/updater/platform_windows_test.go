//go:build windows

package updater

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestUpdaterProcessFixture(t *testing.T) {
	if os.Getenv("ED_COPILOT_UPDATER_PROCESS_FIXTURE") != "1" {
		return
	}
	fmt.Println("ready")
	time.Sleep(250 * time.Millisecond)
	os.Exit(0)
}

func fixtureProcess(t *testing.T) (*exec.Cmd, string) {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(path, "-test.run=^TestUpdaterProcessFixture$")
	cmd.Env = append(os.Environ(), "ED_COPILOT_UPDATER_PROCESS_FIXTURE=1")
	hiddenCommand(cmd)
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if line, err := bufio.NewReader(output).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatal(line, err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })
	return cmd, path
}

func TestWindowsWaitChecksProcessImageAndNaturalExit(t *testing.T) {
	cmd, path := fixtureProcess(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := (platformHelper{}).waitParent(ctx, cmd.Process.Pid, path); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsWaitRejectsDifferentProcessImage(t *testing.T) {
	cmd, path := fixtureProcess(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := (platformHelper{}).waitParent(ctx, cmd.Process.Pid, filepath.Join(filepath.Dir(path), "wrong-application.exe")); err == nil {
		t.Fatal("different process image accepted")
	}
}

func TestWindowsWaitForUniqueMutexRelease(t *testing.T) {
	name := fmt.Sprintf(`Local\EDCopilot.Updater.Test.%d.%d`, os.Getpid(), time.Now().UnixNano())
	wide, _ := windows.UTF16PtrFromString(name)
	handle, err := windows.CreateMutex(nil, false, wide)
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() { time.Sleep(40 * time.Millisecond); windows.CloseHandle(handle); close(released) }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = waitNamedMutex(ctx, name); err != nil {
		t.Fatal(err)
	}
	<-released
}

func TestWindowsVersionResourceRejectsNonExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-release.exe")
	if err := os.WriteFile(path, []byte("synthetic non-PE fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := (platformHelper{}).fileVersion(path); err == nil {
		t.Fatal("non-PE accepted as an installed version")
	}
}

func TestWindowsVersionResourceReadsExistingBuildWithoutLaunching(t *testing.T) {
	path := filepath.Join("..", "..", "dist", "ED Copilot.exe")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("release executable has not been built yet")
	}
	v, err := (platformHelper{}).fileVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = version(v); err != nil {
		t.Fatal(v, err)
	}
}

func TestWindowsStartupGateBlocksWhileUpdateOwnsIt(t *testing.T) {
	name := fmt.Sprintf(`Local\EDCopilot.Updater.StartupTest.%d.%d`, os.Getpid(), time.Now().UnixNano())
	ready, finish, done := make(chan error, 1), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		release, err := acquireHelperGate(name)
		ready <- err
		if err == nil {
			<-finish
			release()
		}
	}()
	if err := <-ready; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	release, err := acquireNamedStartupGate(ctx, name)
	cancel()
	if err == nil {
		release()
		close(finish)
		<-done
		t.Fatal("normal startup passed an active update")
	}
	close(finish)
	<-done
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	release, err = acquireNamedStartupGate(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if unlock, err := acquireHelperGate(name); err == nil {
		unlock()
		t.Fatal("helper ignored existing startup gate")
	}
}
