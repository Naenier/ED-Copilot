//go:build windows

package updater

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\EDCopilot.Desktop_is1`

func installedExecutable(executable string) bool {
	if !filepath.IsAbs(executable) || !samePath(filepath.Base(executable), "ED Copilot.exe") {
		return false
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	location, _, err := key.GetStringValue("InstallLocation")
	if err != nil || location == "" {
		return false
	}
	return samePath(filepath.Dir(executable), location)
}

func hiddenCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}
func launchDetached(path string, args []string) error {
	cmd := exec.Command(path, args...)
	cmd.Dir = filepath.Dir(path)
	hiddenCommand(cmd)
	if err := cmd.Start(); err != nil {
		return errors.New("cannot start quiet update helper")
	}
	return cmd.Process.Release()
}

type platformHelper struct{}

func (platformHelper) installed(path string) bool { return installedExecutable(path) }
func (platformHelper) fileVersion(path string) (string, error) {
	var zero windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &zero)
	if err != nil || size == 0 || size > 1<<20 {
		return "", errors.New("executable version resource is unavailable")
	}
	data := make([]byte, size)
	if err = windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&data[0])); err != nil {
		return "", errors.New("cannot read executable version")
	}
	var fixed *windows.VS_FIXEDFILEINFO
	length := uint32(unsafe.Sizeof(windows.VS_FIXEDFILEINFO{}))
	if err = windows.VerQueryValue(unsafe.Pointer(&data[0]), `\`, unsafe.Pointer(&fixed), &length); err != nil || fixed == nil || length < uint32(unsafe.Sizeof(windows.VS_FIXEDFILEINFO{})) {
		return "", errors.New("invalid executable version resource")
	}
	info := *fixed
	runtime.KeepAlive(data)
	if info.Signature != 0xFEEF04BD || info.ProductVersionLS&0xffff != 0 {
		return "", errors.New("unsupported executable product version")
	}
	return fmt.Sprintf("%d.%d.%d", info.ProductVersionMS>>16, info.ProductVersionMS&0xffff, info.ProductVersionLS>>16), nil
}

const startupGateMutex = `Local\EDCopilot.Updater.Apply`

func (platformHelper) lock() (func(), error) { return acquireHelperGate(startupGateMutex) }
func acquireHelperGate(mutexName string) (func(), error) {
	runtime.LockOSThread()
	name, _ := windows.UTF16PtrFromString(mutexName)
	handle, err := windows.CreateMutex(nil, true, name)
	if err != nil {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		runtime.UnlockOSThread()
		return nil, errors.New("another update helper is already active")
	}
	return releaseOwnedMutex(handle), nil
}

func releaseOwnedMutex(handle windows.Handle) func() {
	released := false
	return func() {
		if !released {
			released = true
			_ = windows.ReleaseMutex(handle)
			_ = windows.CloseHandle(handle)
			runtime.UnlockOSThread()
		}
	}
}

func platformStartupGate(ctx context.Context) (func(), error) {
	return acquireNamedStartupGate(ctx, startupGateMutex)
}
func acquireNamedStartupGate(ctx context.Context, mutexName string) (func(), error) {
	runtime.LockOSThread()
	name, _ := windows.UTF16PtrFromString(mutexName)
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		runtime.UnlockOSThread()
		return nil, errors.New("cannot open update startup gate")
	}
	for {
		status, err := windows.WaitForSingleObject(handle, 100)
		if err != nil {
			windows.CloseHandle(handle)
			runtime.UnlockOSThread()
			return nil, errors.New("cannot wait for update startup gate")
		}
		if status == windows.WAIT_OBJECT_0 || status == windows.WAIT_ABANDONED {
			return releaseOwnedMutex(handle), nil
		}
		select {
		case <-ctx.Done():
			windows.CloseHandle(handle)
			runtime.UnlockOSThread()
			return nil, errors.New("update startup gate wait timed out")
		default:
		}
	}
}

func (platformHelper) waitParent(ctx context.Context, pid int, expectedPath string) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil
	} // parent finished before helper opened its handle
	if err != nil {
		return errors.New("cannot verify original application process")
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err = windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil || !samePath(windows.UTF16ToString(buffer[:size]), expectedPath) {
		return errors.New("original process path does not match installation")
	}
	for {
		status, err := windows.WaitForSingleObject(handle, 250)
		if err != nil {
			return errors.New("cannot wait for original application")
		}
		if status == windows.WAIT_OBJECT_0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("original application did not exit in time")
		default:
		}
	}
}

var openMutex = windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenMutexW")

func (platformHelper) waitDesktop(ctx context.Context) error {
	return waitNamedMutex(ctx, DesktopMutex)
}
func waitNamedMutex(ctx context.Context, mutexName string) error {
	name, _ := windows.UTF16PtrFromString(mutexName)
	for {
		handle, _, err := openMutex.Call(windows.SYNCHRONIZE, 0, uintptr(unsafe.Pointer(name)))
		if handle == 0 {
			if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
				return nil
			}
			return errors.New("cannot confirm desktop instance mutex release")
		}
		windows.CloseHandle(windows.Handle(handle))
		select {
		case <-ctx.Done():
			return errors.New("desktop application is still active")
		case <-time.After(250 * time.Millisecond):
		}
	}
}
func (platformHelper) install(ctx context.Context, path string, args []string) (int, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = filepath.Dir(path)
	hiddenCommand(cmd)
	err := cmd.Run()
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode(), err
	}
	return -1, err
}
func (platformHelper) restart(path string, args []string) error {
	cmd := exec.Command(path, args...)
	cmd.Dir = filepath.Dir(path)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
