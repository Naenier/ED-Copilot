package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/colonization"
	"github.com/Naenier/ED-Copilot/internal/politics"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestColonyPublicObservationUsesExactAddressWithoutChangingPolitics(t *testing.T) {
	const address = "9007199254740995"
	e, _, _ := fixtureEngine(t, M{"colonization": M{"projects": []M{
		{"id": "target", "targetSystem": "Fictional Colony", "status": "building", "systemAddress": address},
		{"id": "decoy", "targetSystem": "Another Colony", "status": "planned", "systemAddress": "2"},
	}}})
	now := time.Now().UTC().Add(-time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/system/"+address {
			t.Errorf("request must use exact known ID64; got %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"record":{"name":"Fictional Colony","id64":%s,"population":1300,"updated_at":%q,"primary_economy":"High Tech","secondary_economy":"Industrial"}}`, address, v.ISO(now))
	}))
	t.Cleanup(server.Close)
	e.Politics = politics.New(nil, politics.Options{HTTPClient: server.Client(), SpanshURL: server.URL})
	if err := e.Politics.Update(M{"action": "select-system", "system": "Unrelated Politics Selection"}); err != nil {
		t.Fatal(err)
	}
	targets := e.Colonies.HistoryTargets()
	if len(targets) != 1 || targets[0].Address != address {
		t.Fatalf("unexpected history targets %+v", targets)
	}
	e.observeColonyPublicSystem("unrelated-commander", targets[0])
	if rows := v.Rows(v.At(e.SnapshotForUI("colony-history"), "colonyHistory", "samples")); len(rows) != 0 {
		t.Fatalf("stale owner's response became history %+v", rows)
	}
	e.observeColonyPublicSystem(e.planWorkspace.key, targets[0])
	e.observeColonyPublicSystem(e.planWorkspace.key, targets[0])
	scoped := e.SnapshotForUI("colony-history")
	rows := v.Rows(v.At(scoped, "colonyHistory", "samples"))
	if len(rows) != 1 || rows[0]["economy"] != "High Tech" || rows[0]["secondEconomy"] != "Industrial" || v.Num(rows[0]["population"]) != 1300 || rows[0]["at"] != v.ISO(now) {
		t.Fatalf("public observation lost actual fields or duplicated %+v", rows)
	}
	if scoped["engineering"] != nil || e.SnapshotForHUD()["colonyHistory"] != nil {
		t.Fatal("colony history projection leaked onto another surface")
	}
	if got := e.Politics.Snapshot()["selectedSystem"]; got != "Unrelated Politics Selection" {
		t.Fatalf("history changed politics selection %v", got)
	}
}

func TestColonyHistoryPollingChoosesMostOverdueAndPrunesRemovedTargets(t *testing.T) {
	now := time.Now()
	first := colonization.HistoryTarget{ProjectID: "a", Name: "A"}
	second := colonization.HistoryTarget{ProjectID: "b", Name: "B"}
	removed := colonization.HistoryTarget{ProjectID: "c", Name: "C"}
	cadence := map[colonization.HistoryTarget]time.Time{first: now.Add(-time.Minute), removed: now}
	if next, ok := nextColonyObservation([]colonization.HistoryTarget{first, second}, cadence, now); !ok || next != second || len(cadence) != 1 {
		t.Fatalf("next observation or target cleanup wrong: %+v, %t, %+v", next, ok, cadence)
	}
}
