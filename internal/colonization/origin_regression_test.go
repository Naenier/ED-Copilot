package colonization

import (
	"encoding/json"
	"testing"
)

func TestCreateProjectOriginExplicitAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name, search, configured, want string
		body                           M
	}{
		{"explicit overrides search", "Search Origin", "Configured Origin", "Manual Origin", M{"originSystem": "Manual Origin"}},
		{"explicit blank clears fallback", "Search Origin", "Configured Origin", "", M{"originSystem": ""}},
		{"explicit whitespace clears fallback", "Search Origin", "Configured Origin", "", M{"originSystem": "  "}},
		{"absent prefers search", "Search Origin", "Configured Origin", "Search Origin", M{}},
		{"absent uses configuration", "", "Configured Origin", "Configured Origin", M{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := New(M{"originSystem": tc.configured})
			p.search["origin"] = M{"name": tc.search}
			body := merge(copyM(tc.body), M{"action": "create-project", "targetSystem": "Synthetic Target"})
			action(t, p, body)
			if got := str(active(p)["originSystem"]); got != tc.want {
				t.Fatalf("origin = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIndependentColonyOriginsAndFactionsSurviveExportAndRestart(t *testing.T) {
	p := New(M{"originSystem": "Configured Origin", "faction": "Configured Faction"})
	p.search["origin"] = M{"name": "Unrelated Search Origin"}
	type expected struct{ id, origin, faction string }
	wants := []expected{}
	for _, label := range []string{"Alpha", "Beta"} {
		action(t, p, M{"action": "create-project", "targetSystem": "Synthetic " + label, "originSystem": label + " Origin", "faction": label + " Faction"})
		wants = append(wants, expected{str(active(p)["id"]), label + " Origin", label + " Faction"})
	}
	assertProjects := func(t *testing.T, planner *Planner) {
		t.Helper()
		for _, want := range wants {
			action(t, planner, M{"action": "select-project", "id": want.id})
			project := active(planner)
			if str(project["originSystem"]) != want.origin || str(project["faction"]) != want.faction {
				t.Fatalf("colony %s lost independent origin/faction: %v", want.id, project)
			}
			raw, err := planner.ExportPlan(want.id)
			if err != nil {
				t.Fatal(err)
			}
			var exported M
			if err := json.Unmarshal(raw, &exported); err != nil {
				t.Fatal(err)
			}
			if str(exported["originSystem"]) != want.origin || str(exported["faction"]) != want.faction {
				t.Fatal("export changed the colony origin/faction", exported)
			}
		}
	}
	assertProjects(t, p)
	raw, err := json.Marshal(p.ExportSettings())
	if err != nil {
		t.Fatal(err)
	}
	var saved M
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	restored := New(saved)
	assertProjects(t, restored)
	action(t, restored, M{"action": "update-project", "id": wants[0].id, "originSystem": "Edited Alpha Origin"})
	if str(byID(restored, wants[1].id)["originSystem"]) != wants[1].origin {
		t.Fatal("editing Alpha changed Beta's origin")
	}
}
