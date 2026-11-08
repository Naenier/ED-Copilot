//go:build windows

package nativeui

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/reference"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func referenceSectionV23(entry M, id string) M {
	for _, s := range v.Rows(entry["sections"]) {
		if s["id"] == id {
			return s
		}
	}
	return nil
}

func TestReferenceSystemMergePreservesGuideAndRealJournalBodies(t *testing.T) {
	engine := newBridgeEngine(t)
	engine.Apply(M{"timestamp": "2026-11-08T00:00:00Z", "event": "Location", "StarSystem": "Deciat", "SystemAddress": json.Number("123456789"), "StarPos": []float64{0, 0, 0}}, M{})
	engine.Apply(M{"timestamp": "2026-11-08T00:00:01Z", "event": "Scan", "BodyID": 5, "BodyName": "Deciat 5", "PlanetClass": "Rocky body", "Landable": true, "Rings": []M{{"Name": "Deciat 5 A Ring", "RingClass": "eRingClass_Rocky"}}}, M{})
	static, ok := reference.Catalog().Entry("system/deciat")
	if !ok {
		t.Fatal("missing static workshop system")
	}
	original := v.Obj(v.Clone(static))
	before := v.Copy(original)
	merged := mergeReferenceSystem(engine, original)
	if merged["id"] != "system/deciat" || merged["title"] != "Deciat" || referenceSectionV23(merged, "destinations") == nil || referenceSectionV23(merged, "bodies") == nil {
		t.Fatal("static guide or actual observations lost", merged)
	}
	rows := v.Arr(referenceSectionV23(merged, "bodies")["rows"])
	if len(rows) != 1 || v.Str(v.Arr(rows[0])[0]) != "Deciat 5" || v.Str(v.Arr(rows[0])[3]) != "1" {
		t.Fatal("journal body and ring count absent", rows)
	}
	if len(v.Rows(merged["related"])) == 0 || !reflect.DeepEqual(before, original) {
		t.Fatal("guide links lost or input mutated")
	}
	v.Rows(merged["properties"])[0]["value"] = "changed"
	if !reflect.DeepEqual(before, original) {
		t.Fatal("merged DTO aliases immutable guide")
	}
}

type referenceProviderV23 struct {
	Controller
	result M
	calls  int
}

func (p *referenceProviderV23) ReferenceSystem(string) M { p.calls++; return p.result }

func TestReferenceSystemMergeUnknownMismatchAndNonSystems(t *testing.T) {
	static, _ := reference.Catalog().Entry("system/deciat")
	entry := v.Obj(v.Clone(static))
	before := v.Copy(entry)
	unknown := &referenceProviderV23{result: M{"title": "Deciat", "description": "No observations", "sections": []M{{"id": "next", "type": "prose"}}, "actions": []M{{"type": "copy-system", "system": "Deciat"}}}}
	out := mergeReferenceSystem(unknown, entry)
	if out["description"] != entry["description"] || referenceSectionV23(out, "next") != nil || len(v.Rows(out["actions"])) != 1 {
		t.Fatal("unknown provider replaced known guide")
	}
	mismatch := &referenceProviderV23{result: M{"title": "Sol", "properties": []M{{"label": "Известно тел", "value": "9"}}}}
	if got := mergeReferenceSystem(mismatch, entry); !reflect.DeepEqual(before, got) {
		t.Fatal("another system leaked")
	}
	ship := M{"category": "ships", "title": "Sidewinder"}
	if got := mergeReferenceSystem(unknown, ship); !reflect.DeepEqual(got, ship) || unknown.calls != 1 {
		t.Fatal("non-system calls observation provider")
	}
}
