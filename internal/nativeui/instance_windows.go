//go:build windows

package nativeui

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

// DesktopInstanceMutex is shared with the installer. Demo and smoke processes
// use disposable profiles and intentionally do not acquire the normal mutex.
const DesktopInstanceMutex = `Local\EDCopilot.Desktop.Session`

func acquireNamedDesktopInstance(name string) (func(), bool, error) {
	handle, _, err := kernelDLL.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(wide(name))))
	if handle == 0 {
		return nil, false, fmt.Errorf("Не удалось защитить сеанс ED Copilot: %w", err)
	}
	if errors.Is(err, syscall.ERROR_ALREADY_EXISTS) {
		closeHandle.Call(handle)
		return func() {}, false, nil
	}
	closed := false
	return func() {
		if !closed {
			closed = true
			closeHandle.Call(handle)
		}
	}, true, nil
}

func AcquireDesktopInstance() (func(), bool, error) {
	return acquireNamedDesktopInstance(DesktopInstanceMutex)
}

func ActivateExistingDesktopInstance() {
	if hwnd := win.FindWindow(nil, wide("ED Copilot · Elite Dangerous")); hwnd != 0 {
		if win.IsIconic(hwnd) {
			win.ShowWindow(hwnd, win.SW_RESTORE)
		} else {
			win.ShowWindow(hwnd, win.SW_SHOW)
		}
		win.SetForegroundWindow(hwnd)
	}
}
