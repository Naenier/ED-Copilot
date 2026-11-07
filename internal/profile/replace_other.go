//go:build !windows

package profile

import "os"

func replace(from, to string) error { return os.Rename(from, to) }
