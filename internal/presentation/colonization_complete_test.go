package presentation

import "testing"

func TestCompletedStationNeedsNoFurtherDeliveryWithoutAConstructionBill(t *testing.T) {
	state := M{"colonization": M{"activeProject": M{"id": "synthetic-colony", "buildings": []M{
		{"id": "ready-port", "name": "Synthetic Ready Port", "status": "complete"},
		{"id": "unobserved-site", "name": "Synthetic Unknown Site", "status": "building"},
		{"id": "observed-site", "name": "Synthetic Observed Site", "status": "building", "requirements": []M{{"missing": 25}}},
	}}}}
	page := Build("colonization", "Строительство", "", state)
	if len(page.Rows) != 3 {
		t.Fatal("ready station or independent construction sites disappeared")
	}
	for i, want := range []string{"0", "Неизвестно", "25"} {
		if got := page.Rows[i].Cells[5]; got != want {
			t.Fatalf("delivery remaining row %d=%q, want %q", i, got, want)
		}
	}
}
