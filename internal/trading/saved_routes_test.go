package trading

import (
	v "github.com/Naenier/ED-Copilot/internal/value"
	"testing"
)

func savedFixture() M {
	return M{"id": "search-one", "legs": []M{{"key": "gold", "name": "Золото", "quantity": 10, "buyPrice": 100, "sellPrice": 200, "buy": M{"system": "Alpha", "station": "Port A", "supply": 100}, "sell": M{"system": "Beta", "station": "Port B", "demand": 1000}}}, "options": M{"minPad": "L"}, "netEstimate": 1000}
}
func TestSavedRoutesPreserveQuotesAcrossRestartWithoutChangingPlan(t *testing.T) {
	s := New(nil)
	route := savedFixture()
	out, err := s.SaveRoute(M{"action": "save-route", "route": route})
	if err != nil {
		t.Fatal(err)
	}
	route["legs"] = nil
	if len(s.Plan) != 0 || len(s.Destination) != 0 {
		t.Fatal("Saving a favourite changed the active plan")
	}
	restored := New(s.ExportSettings())
	rows := restored.SavedRoutesSnapshot()
	if len(rows) != 1 || rows[0]["id"] != out["id"] || v.At(rows[0], "options", "minPad") != "L" || len(v.Rows(rows[0]["legs"])) != 1 {
		t.Fatal("Favourite lost its complete quote", rows)
	}
	v.Obj(v.Rows(rows[0]["legs"])[0]["buy"])["system"] = "mutated"
	if v.At(v.Rows(restored.SavedRoutesSnapshot()[0]["legs"])[0], "buy", "system") != "Alpha" {
		t.Fatal("Snapshot aliases saved state")
	}
	next := savedFixture()
	next["legs"].([]M)[0]["buyPrice"] = 110
	again, err := restored.SaveRoute(M{"action": "save-route", "route": next})
	if err != nil || again["id"] != out["id"] || len(restored.SavedRoutes) != 1 {
		t.Fatal("Saving updated quotes duplicated the route", err)
	}
	if _, err = restored.SaveRoute(M{"action": "delete-route", "id": out["id"]}); err != nil || len(restored.SavedRoutes) != 0 {
		t.Fatal("Delete failed", err)
	}
}
func TestSavedRoutesRejectInvalidSnapshotAtomically(t *testing.T) {
	s := New(nil)
	_, _ = s.SaveRoute(M{"action": "save-route", "route": savedFixture()})
	for _, mutate := range []func(M){func(r M) { r["legs"] = []M{} }, func(r M) { v.Rows(r["legs"])[0]["quantity"] = -1 }, func(r M) { v.Obj(v.Rows(r["legs"])[0]["buy"])["system"] = "" }, func(r M) { v.Rows(r["legs"])[0]["quantity"] = 1.5 }} {
		r := savedFixture()
		mutate(r)
		if _, err := s.SaveRoute(M{"action": "save-route", "route": r}); err == nil {
			t.Fatal("Invalid route accepted", r)
		}
		if len(s.SavedRoutes) != 1 {
			t.Fatal("Invalid update changed favourites")
		}
	}
}

func TestSavedRoutesKeepOwnersSeparateAndAllowReversibleLegacyAssignment(t *testing.T) {
	service := New(nil)
	input := M{"action": "save-route", "route": savedFixture()}
	shared, err := service.SaveRoute(input)
	if err != nil {
		t.Fatal(err)
	}
	setScope := func(id any, owner, scope string) error {
		_, err := service.SaveRouteFor(M{"action": "set-scope", "id": id, "scope": scope}, owner)
		return err
	}
	if err := setScope(shared["id"], "commander-a", "personal"); err != nil {
		t.Fatal(err)
	}
	if rows := service.SavedRoutesFor("commander-b"); len(rows) != 0 {
		t.Fatalf("personal route visible to another commander: %v", rows)
	}
	if _, err := service.SaveRouteFor(M{"action": "delete-route", "id": shared["id"]}, "commander-b"); err == nil {
		t.Fatal("unrelated owner could delete a personal route")
	}
	second, err := service.SaveRouteFor(input, "commander-b")
	if err != nil || second["id"] == shared["id"] {
		t.Fatalf("same route for a different commander replaced the original: %v, %v", second, err)
	}
	if err := setScope(shared["id"], "commander-a", "shared"); err != nil {
		t.Fatal(err)
	}
	if err := setScope(shared["id"], "commander-b", "personal"); err == nil {
		t.Fatal("duplicate ownership conversion silently replaced another route")
	}
	rows := service.SavedRoutesFor("commander-b")
	if len(rows) != 2 || rows[0]["scope"] != "personal" || rows[1]["scope"] != "shared" || rows[0]["planOwner"] != nil || rows[1]["id"] != shared["id"] {
		t.Fatalf("unexpected visible routes after reversible migration: %v", rows)
	}
	if got := service.SavedRoutesFor(""); len(got) != 1 || got[0]["id"] != shared["id"] {
		t.Fatalf("anonymous workspace saw a private route: %v", got)
	}
}
func TestPlanKeepsBothStationQuotesAndRejectsPartialMarket(t *testing.T) {
	s := New(nil)
	leg := v.Rows(savedFixture()["legs"])[0]
	body := v.Merge(v.Copy(leg), M{"action": "plan"})
	if err := s.Update(body); err != nil {
		t.Fatal(err)
	}
	if v.At(s.Plan, "buy", "system") != "Alpha" || v.At(s.Plan, "sell", "station") != "Port B" {
		t.Fatal("Missing market context", s.Plan)
	}
	old := v.Str(v.At(s.Plan, "buy", "system"))
	body["buy"] = M{"system": "Other"}
	if s.Update(body) == nil || v.At(s.Plan, "buy", "system") != old {
		t.Fatal("Partial quote replaced the plan")
	}
}

func TestSelectedPurchaseKeepsItsOwnModeAndFreshnessPolicy(t *testing.T) {
	now := v.Time("2026-11-08T12:00:00Z")
	service := New(nil)
	market := M{"system": "Synthetic A", "station": "Purchase port", "carrier": true, "updatedAt": "2026-11-08T05:00:00Z", "selectionMode": "buy", "quoteOptions": M{"carrierMaxAgeHours": 8}, "quotes": []M{{"key": "gold", "price": 100, "supply": 200, "demand": 0}}}
	if err := service.Update(M{"action": "destination", "destination": market}); err != nil {
		t.Fatal(err)
	}
	projected := v.Obj(service.Snapshot(M{"now": v.ISO(now), "marketOptions": M{"mode": "sell", "carrierMaxAgeHours": 1}})["destination"])
	if projected["selectionMode"] != "buy" || v.Bool(projected["expired"]) {
		t.Fatal("Another search changed the selected purchase", projected)
	}
	restored := New(service.ExportSettings())
	if restored.Destination["selectionMode"] != "buy" || v.Num(v.At(restored.Destination, "quoteOptions", "carrierMaxAgeHours")) != 8 {
		t.Fatal("Purchase context was lost on restart")
	}
}
