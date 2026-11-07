package securestore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicReplaceAndFailurePreservesOtherFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "nested", "state.bin")
	if err := AtomicWrite(file, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(file, []byte("replacement")); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(file)
	if err != nil || string(actual) != "replacement" {
		t.Fatal("replacement", string(actual), err)
	}
	entries, err := os.ReadDir(filepath.Dir(file))
	if err != nil || len(entries) != 1 {
		t.Fatal("temporary files leaked", entries, err)
	}
	if err = AtomicWrite(filepath.Join(file, "invalid-child.bin"), []byte("bad")); err == nil {
		t.Fatal("regular-file ancestor accepted")
	}
	actual, _ = os.ReadFile(file)
	if string(actual) != "replacement" {
		t.Fatal("failed update altered existing file")
	}
}
