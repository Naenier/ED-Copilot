//go:build windows

package nativeui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileActionsRequireSeparateDestination(t *testing.T) {
	root := t.TempDir()
	active := filepath.Join(root, "Profile")
	child := filepath.Join(active, "child")
	sibling := filepath.Join(root, "Profile Backup")
	for _, dir := range []string{child, sibling} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, unsafe := range []string{active, filepath.Join(root, "profile"), child} {
		if err := requireSeparateProfilePath(active, unsafe); err == nil {
			t.Fatalf("accepted destination inside the active profile: %s", unsafe)
		}
	}
	if err := requireSeparateProfilePath(active, sibling); err != nil {
		t.Fatal(err)
	}
	if err := requireSeparateProfilePath(filepath.Join(root, "missing"), sibling); err != nil {
		t.Fatalf("cannot export a backup when the active folder is missing: %v", err)
	}
}
