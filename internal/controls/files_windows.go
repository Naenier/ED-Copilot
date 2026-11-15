//go:build windows

package controls

import (
	"os"

	"golang.org/x/sys/windows"
)

func linkedPath(path string, info os.FileInfo) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return true
	}
	attrs, err := windows.GetFileAttributes(name)
	return err != nil || attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

func replaceFile(from, to string, overwrite bool) error {
	src, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	dst, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	flags := uint32(windows.MOVEFILE_WRITE_THROUGH)
	if overwrite {
		flags |= windows.MOVEFILE_REPLACE_EXISTING
	}
	return windows.MoveFileEx(src, dst, flags)
}
