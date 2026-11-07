//go:build windows

package main

import (
	"path/filepath"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/app"
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestSmokePlansRetainExplicitOriginInColonyEditForms(t *testing.T) {
	root := t.TempDir()
	if err := smokeJournal(root); err != nil {
		t.Fatal(err)
	}
	engine, err := app.New(app.Options{DataDir: filepath.Join(root, "profile"), JournalDir: root, DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if err := smokePlans(engine); err != nil {
		t.Fatal(err)
	}
	state := engine.Snapshot()
	projects := v.Rows(v.At(state, "colonization", "projects"))
	if len(projects) != 2 {
		t.Fatal("expected two independent synthetic colonies", len(projects))
	}
	for _, project := range projects {
		if project["originSystem"] != "Тестовая исходная система" || project["faction"] != "Тестовая фракция" {
			t.Fatal("smoke colony lost its explicit source", project)
		}
	}
	for _, action := range p.CoreActions("colonization", "Строительство", state, nil, nil) {
		if action.ID != "colony.edit" {
			continue
		}
		for _, field := range action.Fields {
			if field.Key == "originSystem" {
				if field.Default != "Тестовая исходная система" {
					t.Fatal("edit form source does not match the smoke plan", field.Default)
				}
				return
			}
		}
	}
	t.Fatal("colony edit origin field was not available")
}
