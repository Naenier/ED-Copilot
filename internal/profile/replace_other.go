//go:build !windows

package profile

import "os"

func replace(from, to string) error { return os.Rename(from, to) }

// The temporary file and the destination share a directory.
func installNew(from, to string) error { return os.Link(from, to) }
