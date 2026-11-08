package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func referenceProperty(entry M, label string) any {
	for _, property := range v.Rows(entry["properties"]) {
		if property["label"] == label {
			return property["value"]
		}
	}
	return nil
}
func referenceSection(entry M, id string) M {
	for _, section := range v.Rows(entry["sections"]) {
		if section["id"] == id {
			return section
		}
	}
	return nil
}
func referenceRows(entry M) [][]string {
	rows, _ := referenceSection(entry, "bodies")["rows"].([][]string)
	return rows
}

func TestReferenceSystemObservedUnknownAndNameValidation(t *testing.T) {
	engine, _, _ := fixtureEngine(t, nil)
	observed := engine.ReferenceSystem("  synthetic ORIGIN  ")
	if observed["category"] != "systems" || observed["title"] != "synthetic ORIGIN" || referenceProperty(observed, "Адрес системы") != "18446744073709551615" {
		t.Fatal("case-insensitive observed system lost ID64 or identity", observed)
	}
	if referenceProperty(observed, "Известно тел") != "1" || referenceProperty(observed, "От текущей системы") != "0 св. л." || len(referenceRows(observed)) != 1 {
		t.Fatal("observed system properties or bodies were not rendered", observed)
	}
	unknown := engine.ReferenceSystem("Unknown Synthetic System")
	if unknown["id"] != "system/Unknown%20Synthetic%20System" || referenceSection(unknown, "bodies") != nil || !strings.Contains(v.Str(unknown["description"]), "пока нет") || referenceProperty(unknown, "Известно тел") != nil {
		t.Fatal("unknown system invented local observations", unknown)
	}
	if v.Rows(unknown["actions"])[0]["system"] != "Unknown Synthetic System" {
		t.Fatal("copy action does not preserve the requested system")
	}
	for _, invalid := range []string{"", " \t\r\n ", strings.Repeat("Ж", 151)} {
		if got := engine.ReferenceSystem(invalid); got != nil {
			t.Fatal("invalid name accepted", invalid, got)
		}
	}
	if engine.ReferenceSystem(strings.Repeat("Ж", 150)) == nil {
		t.Fatal("150 Cyrillic characters rejected by a byte-length limit")
	}
}

func TestReferenceSystemBodyProjectionBoundedSortedAndDoesNotAliasDomain(t *testing.T) {
	engine, _, _ := fixtureEngine(t, nil)
	engine.Apply(testEvent("Location", M{"StarSystem": "Synthetic Large", "SystemAddress": json.Number("18446744073709551613"), "StarPos": []float64{3, 4, 0}}), M{})
	for index := 140; index > 0; index-- {
		engine.Apply(testEvent("Scan", M{"BodyID": index, "BodyName": fmt.Sprintf("Synthetic Large %03d", index), "PlanetClass": "Rocky body", "Landable": index%2 == 0, "Rings": []M{{"Name": "Synthetic Ring", "RingClass": "eRingClass_Rocky"}}}), M{})
	}
	engine.Apply(testEvent("Location", M{"StarSystem": "Synthetic Origin", "SystemAddress": json.Number("18446744073709551615"), "StarPos": []float64{0, 0, 0}}), M{})
	entry := engine.ReferenceSystem("Synthetic Large")
	section := referenceSection(entry, "bodies")
	rows := referenceRows(entry)
	if len(rows) != 128 || referenceProperty(entry, "Известно тел") != "140" || referenceProperty(entry, "От текущей системы") != "5 св. л." || referenceSection(entry, "body-limit") == nil {
		t.Fatal("projection did not distinguish display cap from actual body count", entry)
	}
	if rows[0][0] != "Synthetic Large 001" || rows[127][0] != "Synthetic Large 128" || rows[0][2] != "Нет" || rows[1][2] != "Да" || rows[0][3] != "1" {
		t.Fatal("stable order, landing or ring count lost", rows[0], rows[1], rows[127])
	}
	before := v.Copy(entry)
	v.Rows(entry["properties"])[0]["value"] = "tampered"
	v.Rows(entry["actions"])[0]["system"] = "tampered"
	rows[0][0] = "tampered"
	section["columns"].([]string)[0] = "tampered"
	if !reflect.DeepEqual(before, v.Copy(engine.ReferenceSystem("Synthetic Large"))) {
		t.Fatal("mutating renderer data changed a subsequent system projection")
	}
	if len(referenceRows(engine.ReferenceSystem("Synthetic Origin"))) != 1 {
		t.Fatal("opening another system changed the original system's observations")
	}
}
