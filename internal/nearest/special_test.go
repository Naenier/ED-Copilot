package nearest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestNearestFactionsMatchNamedPresenceAndAllActiveStates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/systems/field_values/minor_faction_presences/name":
			if r.URL.Query().Get("q") != "Shadow" {
				t.Error("faction prefix was not requested")
			}
			_ = json.NewEncoder(w).Encode(M{"values": M{"name": []string{"Shadow Alliance", "Other", "Shadows of Space"}}})
		case "/systems/search":
			var input M
			_ = json.NewDecoder(r.Body).Decode(&input)
			filters := v.Rows(v.At(input, "filters", "minor_faction_presences"))
			if len(filters) != 1 || v.Str(v.At(input, "reference_system")) != "Sol" {
				t.Errorf("missing exact provider or additional presence filter: %v", input)
			}
			matches := []M{
				{"name": "Shadow Alliance", "influence": .27, "government": "Anarchy", "state": "Boom", "active_states": []string{"Boom", "Civil Liberty"}},
				{"name": "Shadows of Space", "government": "Anarchy", "state": "Boom", "active_states": []string{"Boom"}},
				{"name": "Other", "government": "Corporate", "state": "Boom", "active_states": []string{"Boom", "Civil Liberty"}},
			}
			_ = json.NewEncoder(w).Encode(M{"count": 3, "results": []M{
				{"name": "System One", "id64": "9007199254740997", "distance": 5.5, "controlling_minor_faction": "Other", "minor_faction_presences": matches},
				{"name": "System Two", "id64": "9007199254740998", "distance": 7.0, "controlling_minor_faction": "Shadow Alliance", "minor_faction_presences": []M{matches[0]}},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := New(server.Client())
	c.BaseURL = server.URL
	result, err := c.Search(context.Background(), Query{Category: "factions", Origin: "Sol", Name: "Shadow", Limit: 2, Filters: M{
		"factionStates": []string{"Boom", "Civil Liberty"}, "factionStateMode": "all", "factionGovernment": "Anarchy",
		"presenceFaction": "Other", "presenceMode": "controlling",
	}})
	// A controlling-faction filter is a top-level field and a faction name
	// filter is a separate nested clause.
	if err != nil || len(result.Rows) != 1 || !result.HasMore {
		t.Fatalf("wrong faction search results: %+v, %v", result, err)
	}
	row := result.Rows[0]
	if row["name"] != "Shadow Alliance" || row["systemAddress"] != "9007199254740997" || row["controlling"] != false ||
		!slices.Equal(spaceValues(row["states"]), []string{"Boom", "Civil Liberty"}) {
		t.Fatalf("wrong normalized faction row: %v", row)
	}
}

func TestNearestSignalsReturnObservedCountsAndKeepUnknownUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bodies/search" {
			t.Errorf("unexpected network request %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var input M
		_ = json.NewDecoder(r.Body).Decode(&input)
		signals := v.Rows(v.At(input, "filters", "signals"))
		if len(signals) != 1 || signals[0]["name"] != "Biological" {
			t.Errorf("unexpected filters: %v", input)
		}
		_ = json.NewEncoder(w).Encode(M{"count": 1, "results": []M{
			{"name": "Example 1", "id64": "9007199254740999", "system_name": "Example", "system_id64": "9007199254740998",
				"distance": 12.5, "distance_to_arrival": 1010.5, "signals": []M{{"name": "Biological", "count": 4}, {"name": "Geological", "count": 2}}},
		}})
	}))
	defer server.Close()
	c := New(server.Client())
	c.BaseURL = server.URL

	result, err := c.Search(context.Background(), Query{Category: "signals", Origin: "Sol", Filters: M{"signal": "biological"}})
	if err != nil || len(result.Rows) != 1 || v.Int(result.Rows[0]["count"]) != 4 || result.Rows[0]["system"] != "Example" {
		t.Fatalf("unexpected signal result: %+v, %v", result, err)
	}
	result, err = c.Search(context.Background(), Query{Category: "signals", Origin: "Sol", Filters: M{"signal": "compromised-nav-beacon"}})
	if err != nil || len(result.Rows) != 0 || result.Note == "" || !strings.Contains(result.Link, "pi30=1") {
		t.Fatalf("unavailable signal must have a relevant external search: %+v, %v", result, err)
	}
}

func TestNearestPublicConflictCannotInventOrRepeatOpponents(t *testing.T) {
	now := time.Date(2026, 11, 18, 13, 50, 0, 0, time.UTC)
	conflict := func(status, date, faction, opponent string) M {
		return M{"type": "CivilWar", "status": status, "updatedAt": date, "faction": M{"name": faction}, "opponentFaction": M{"name": opponent}}
	}
	candidates := map[string]M{
		"s0": {"name": "Local", "id64": "9007199254740997", "distance": 3.1},
		"s1": {"name": "Wrong", "id64": "9007199254740998", "distance": 5.1},
	}
	raw := M{
		"s0": M{"systemAddress": "9007199254740997", "factionConflicts": M{"nodes": []M{
			conflict("Active", "2026-11-18T13:49:55.000", "Civic Party", "Progressive Union"),
			conflict("Active", "2026-11-18T13:49:55.000", "Progressive Union", "Civic Party"),
			conflict("Pending", "2026-11-18T13:49:55.000", "Pending", "Faction"),
			conflict("Active", "2026-11-14T13:49:55.000", "Old", "Conflict"),
			conflict("Active", "2026-11-18T13:49:55.000", "No opponent", ""),
		}}},
		"s1": M{"systemAddress": "9007199254740999"},
	}
	out := readPublicConflicts(raw, candidates, now)
	if len(out) != 1 || len(out["9007199254740997"]) != 1 {
		t.Fatalf("invalid or duplicate pairs leaked: %v", out)
	}
	if row := out["9007199254740997"][0]; row["state"] != "Civil War" || row["opponent"] != "Progressive Union" || row["system"] != "Local" {
		t.Fatalf("wrong conflict pair: %v", row)
	}
}

func TestNearestEngineersHaveKnownDistances(t *testing.T) {
	client := New(nil)
	result, err := client.Search(context.Background(), Query{Category: "misc", Origin: "Sol", Limit: 64, Filters: M{"kind": "engineers"}})
	if err != nil || result.Total != 38 || len(result.Rows) != 38 {
		t.Fatalf("wrong engineer catalogue: %+v, %v", result, err)
	}
	for _, row := range result.Rows {
		if _, ok := v.Number(row["distanceLy"]); !ok || row["referenceId"] == "engineer/" || row["station"] == "" {
			t.Fatalf("engineer without searchable workshop location: %v", row)
		}
	}
}
