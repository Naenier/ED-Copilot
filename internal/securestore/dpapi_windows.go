//go:build windows

package securestore

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

var crypt32 = syscall.NewLazyDLL("crypt32.dll")
var protectProc = crypt32.NewProc("CryptProtectData")
var unprotectProc = crypt32.NewProc("CryptUnprotectData")
var localFree = syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree")

type dataBlob struct {
	Size uint32
	Data *byte
}

func available() bool { return protectProc.Find() == nil && unprotectProc.Find() == nil }
func dpapi(input []byte, decrypt bool) ([]byte, error) {
	if len(input) == 0 {
		return nil, errors.New("Пустой защищённый файл.")
	}
	in := dataBlob{Size: uint32(len(input)), Data: &input[0]}
	var out dataBlob
	var result uintptr
	if decrypt {
		result, _, _ = unprotectProc.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	} else {
		result, _, _ = protectProc.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	}
	runtime.KeepAlive(input)
	if result == 0 {
		return nil, errors.New("Защищённое хранилище Windows недоступно для текущего пользователя.")
	}
	defer localFree.Call(uintptr(unsafe.Pointer(out.Data)))
	if out.Size > 16*1024*1024 {
		return nil, errors.New("Некорректный защищённый файл.")
	}
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}
func protect(b []byte) ([]byte, error)   { return dpapi(b, false) }
func unprotect(b []byte) ([]byte, error) { return dpapi(b, true) }
