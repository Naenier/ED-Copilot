package profile

import (
	v "github.com/Naenier/ED-Copilot/internal/value"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAtomicWritesAndFailedSavePreservesPrevious(t *testing.T) {
	file := filepath.Join(t.TempDir(), "nested", "settings.json")
	if e := Write(file, v.M{"value": 1}); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if e := Write(file, v.M{"value": n}); e != nil {
				t.Error(e)
			}
		}(i)
	}
	wg.Wait()
	before, e := Read(file)
	if e != nil {
		t.Fatal(e)
	}
	if e = Write(file, make(chan int)); e == nil {
		t.Fatal("invalid value written")
	}
	after, e := Read(file)
	if e != nil || v.Int(before["value"]) != v.Int(after["value"]) {
		t.Fatal("failed save corrupted previous settings")
	}
	entries, _ := os.ReadDir(filepath.Dir(file))
	if len(entries) != 1 {
		t.Fatal("temporary files left behind")
	}
	if e = os.WriteFile(file, []byte("\xef\xbb\xbf{\"id\":18446744073709551614}"), 0600); e != nil {
		t.Fatal(e)
	}
	got, e := Read(file)
	if e != nil || v.Str(got["id"]) != "18446744073709551614" {
		t.Fatal("BOM or 64-bit precision failed")
	}
}
