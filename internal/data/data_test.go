package data

import (
	"bytes"
	"image/png"
	"sync"
	"testing"
)

func TestAllEmbeddedResourcesAreReadableAndConcurrent(t *testing.T) {
	names := []string{"catalog.json", "commodity-names.json", "commodity-ru.json", "encyclopedia-ships.json", "encyclopedia.json", "engineering-data.json", "help.json"}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, name := range names {
				if value, err := Load(name); err != nil || value == nil {
					t.Errorf("%s: %v", name, err)
				}
			}
		}()
	}
	wg.Wait()
	if _, err := Load("missing.json"); err == nil {
		t.Fatal("missing data silently accepted")
	}
	if len(Array("encyclopedia-ships.json")) < 40 || len(Array("help.json")) < 15 || len(Object("engineering-data.json")) == 0 {
		t.Fatal("incomplete embedded references")
	}
	image, err := Files.ReadFile("copilot.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(image)); err != nil {
		t.Fatal("invalid packaged application icon", err)
	}
}
