//go:build !windows

package controls

import "os"

func linkedPath(_ string, info os.FileInfo) bool { return info.Mode()&os.ModeSymlink != 0 }

func replaceFile(from, to string, overwrite bool) error {
	if overwrite {
		return os.Rename(from, to)
	}
	return os.Link(from, to)
}
