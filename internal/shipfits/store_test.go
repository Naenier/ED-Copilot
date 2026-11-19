package shipfits

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func libraryFit(name string) Record {
	build, _ := json.Marshal(map[string]any{"version": 1, "shipId": "python", "code": "synthetic", "padding": strings.Repeat("a", 48<<10)})
	return Record{Name: name, Ship: "python", Build: build}
}

func TestFittingLibraryAllExportPagesCanBeImported(t *testing.T) {
	dir := t.TempDir()
	source := New(filepath.Join(dir, "source.db"))
	restore := New(filepath.Join(dir, "restored.db"))
	for batch := 0; batch < 2; batch++ {
		fits := make([]Record, 10)
		for i := range fits {
			fits[i] = libraryFit(fmt.Sprintf("Fit %02d", batch*10+i))
		}
		if _, err := source.Import(fits); err != nil {
			t.Fatal(err)
		}
	}
	var after, revision string
	exported := map[string]bool{}
	pages := 0
	for {
		page, err := source.Export(after, revision)
		if err != nil {
			t.Fatal(err)
		}
		if page.Revision == "" || len(page.Fits) == 0 {
			t.Fatalf("export page unexpectedly empty or unversioned: %d", len(page.Fits))
		}
		for _, fit := range page.Fits {
			if len(fit.Build) == 0 || exported[fit.Name] {
				t.Fatalf("missing build or duplicate fit: %s", fit.Name)
			}
			exported[fit.Name] = true
		}
		if _, err := restore.Import(page.Fits); err != nil {
			t.Fatalf("our own export page cannot be imported: %v", err)
		}
		pages++
		if page.Next == "" {
			break
		}
		after, revision = page.Next, page.Revision
	}
	restored, err := restore.List()
	if err != nil || len(restored) != 20 || len(exported) != 20 || pages < 2 {
		t.Fatalf("expected a complete multi-volume library: %d pages, %d exported, %d restored, %v", pages, len(exported), len(restored), err)
	}
	first, err := source.Export("", "")
	if err != nil || first.Next == "" {
		t.Fatalf("expected a continuation: %q, %v", first.Next, err)
	}
	if _, err := source.Save(libraryFit("Concurrent edit")); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Export(first.Next, first.Revision); !errors.Is(err, errExportPage) {
		t.Fatalf("export read a different library revision: %v", err)
	}
}

func TestInvalidFitDoesNotPartiallyImportBatch(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "library.db"))
	invalid := libraryFit("Invalid")
	invalid.Ship = "anaconda"
	if _, err := store.Import([]Record{libraryFit("Valid"), invalid}); !errors.Is(err, errInvalidFit) {
		t.Fatalf("expected validation failure: %v", err)
	}
	if rows, err := store.List(); err != nil || len(rows) != 0 {
		t.Fatalf("an invalid batch was partially imported: %d rows, %v", len(rows), err)
	}
}

func TestFittingLibraryUpdatesTheSameRecordAndExcludesDeletedBuilds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	store := New(path)
	first, err := store.Save(libraryFit("Original"))
	if err != nil {
		t.Fatal(err)
	}
	replacement := libraryFit("Renamed")
	replacement.ID = first.ID
	updated, err := store.Save(replacement)
	if err != nil || updated.ID != first.ID {
		t.Fatalf("update created a second fit: %+v, %v", updated, err)
	}

	reopened := New(path)
	stored, err := reopened.Get(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	var build map[string]any
	if err = json.Unmarshal(stored.Build, &build); err != nil || stored.Name != "Renamed" || build["name"] != stored.Name {
		t.Fatalf("the exported build name does not match the record: %q, %v", stored.Name, err)
	}
	if err = reopened.Delete(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.Get(first.ID); !errors.Is(err, errMissingFit) {
		t.Fatalf("deleted fit can still be opened: %v", err)
	}
	if _, err = reopened.Save(replacement); !errors.Is(err, errMissingFit) {
		t.Fatalf("saved an update after the original fit was deleted: %v", err)
	}
	page, err := reopened.Export("", "")
	if err != nil || len(page.Fits) != 0 || page.Next != "" {
		t.Fatalf("deleted fit still appears in export: %+v, %v", page, err)
	}
}
