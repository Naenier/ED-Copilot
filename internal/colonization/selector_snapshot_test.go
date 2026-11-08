package colonization

import "testing"

func TestSelectorSnapshotKeepsIndependentColonySelectionWithoutSupplyExpansion(t *testing.T) {
	p := New(nil)
	a := colony(t, p, "Alpha", 100, 101)
	b := colony(t, p, "Beta", 200, 202)
	action(t, p, M{"action": "select-project", "id": a["id"]})
	selector := p.SelectorSnapshot()
	if selector["selectedProjectId"] != a["id"] || len(rows(selector["projects"])) != 2 {
		t.Fatal(selector)
	}
	for _, row := range rows(selector["projects"]) {
		if row["buildings"] != nil || row["supplies"] != nil {
			t.Fatal("selector expanded colony supplies", row)
		}
	}
	rows(selector["projects"])[0]["name"] = "external mutation"
	if active(p)["name"] == "external mutation" {
		t.Fatal("selector aliases a colony")
	}
	action(t, p, M{"action": "select-project", "id": b["id"]})
	if p.SelectorSnapshot()["selectedProjectId"] != b["id"] {
		t.Fatal("selector retained old colony")
	}
}
