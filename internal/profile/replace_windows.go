//go:build windows

package profile

import (
	"syscall"
	"unsafe"
)

var moveFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func replace(from, to string) error {
	return moveFile(from, to, 0x1|0x8)
}

func installNew(from, to string) error {
	return moveFile(from, to, 0x8)
}

func moveFile(from, to string, flags uintptr) error {
	f, e := syscall.UTF16PtrFromString(from)
	if e != nil {
		return e
	}
	t, e := syscall.UTF16PtrFromString(to)
	if e != nil {
		return e
	}
	r, _, e := moveFileEx.Call(uintptr(unsafe.Pointer(f)), uintptr(unsafe.Pointer(t)), flags)
	if r == 0 {
		return e
	}
	return nil
}
