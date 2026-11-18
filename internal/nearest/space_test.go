package nearest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestSpaceSystemFiltersSendPositionAndFactionMode(t *testing.T) {
	q := Query{
		Category: "systems", Origin: "Sol", Region: "Inner Orion Spur", Sort: "population", Page: 2, Limit: 24,
		Filters: M{
			"radiusLy": 40, "population": "populated", "populationMax": 1000, "colonization": "uncolonized",
			"presenceFaction": "Independent Test", "presenceMode": "not-controlling",
			"systemAllegiance": "Federation", "factionStates": []string{"Boom", "Expansion"}, "stateMode": "any",
		},
	}
	payload := spacePayload(q, spaceSystemFilters(q))
	want := M{
		"reference_system": "Sol", "size": 24, "page": 2,
		"sort": []M{
			{"population": M{"direction": "desc"}},
			{"distance": M{"direction": "asc"}},
			{"name": M{"direction": "asc"}},
		},
		"filters": M{
			"distance": M{"min": 0, "max": float64(40)}, "region": M{"value": []string{"Inner Orion Spur"}},
			"allegiance":                      M{"value": []string{"Federation"}},
			"controlling_minor_faction_state": M{"value": []string{"Boom", "Expansion"}, "logic": "OR"},
			"minor_faction_presences":         []M{{"name": M{"value": []string{"Independent Test"}}}},
			"controlling_minor_faction":       M{"value": []string{"Independent Test"}, "logic": "NOT"},
			"population":                      M{"comparison": "<=>", "value": []float64{1, 1000}},
			"is_colonised":                    M{"value": false}, "is_being_colonised": M{"value": false},
		},
	}
	if !reflect.DeepEqual(payload, want) {
		t.Fatalf("got %#v, want %#v", payload, want)
	}
}

func TestSpaceBodiesRestrictsToIndexedFactionSystemsAndKeepsExactIDs(t *testing.T) {
	const firstID = "9007199254740999"
	const bodyID = "9007199254741009"
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		var payload M
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if payload["reference_system"] != "Sol" {
			t.Errorf("missing reference system: %#v", payload)
		}
		switch r.URL.Path {
		case "/systems/search":
			rows := v.Rows(v.At(payload, "filters", "minor_faction_presences"))
			if len(rows) != 1 || v.Arr(v.At(rows[0], "name", "value"))[0] != "Independent Test" {
				t.Errorf("missing presence filter: %#v", payload)
			}
			_, _ = w.Write([]byte(`{"count":101,"results":[{"id64":9007199254740999,"name":"Test"}]}`))
		case "/bodies/search":
			if got := v.Arr(v.At(payload, "filters", "system_id64", "value")); !reflect.DeepEqual(got, []any{firstID}) {
				t.Errorf("body search omitted or changed system IDs: %#v", got)
			}
			sortBy := v.Rows(payload["sort"])
			sortMaterials := v.Rows(sortBy[0]["materials"])
			if len(sortMaterials) != 2 || sortMaterials[0]["name"] != "Polonium" || sortMaterials[1]["name"] != "Yttrium" {
				t.Errorf("changed user material priority: %#v", sortBy)
			}
			rings := v.Rows(v.At(payload, "filters", "rings"))
			if len(rings) != 1 || !reflect.DeepEqual(v.Arr(rings[0]["type"]), []any{"Metallic"}) {
				t.Errorf("missing ring filter: %#v", payload)
			}
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id64":9007199254741009,"system_id64":9007199254740999,"system_name":"Test","name":"Test 1","subtype":"High metal content world","distance":2.1,"distance_to_arrival":120,"is_landable":true,"materials":[{"name":"Polonium","share":1.16}]}]}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := New(server.Client())
	client.BaseURL = server.URL
	out, err := client.searchSpace(t.Context(), Query{
		Category: "bodies", Origin: "Sol", Sort: "materials",
		Filters: M{"presenceFaction": "Independent Test", "materials": []string{"Polonium", "Yttrium"}, "ringType": "Metallic", "landable": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 1 || out.Rows[0]["systemAddress"] != firstID || out.Rows[0]["bodyAddress"] != bodyID || v.Num(out.Rows[0]["distanceLs"]) != 120 ||
		!v.Bool(out.Rows[0]["landable"]) || out.HasMore || out.Note == "" || out.Total != 1 {
		t.Fatalf("invalid results: %#v", out)
	}
	close(requests)
	if paths := []string{<-requests, <-requests}; !reflect.DeepEqual(paths, []string{"/systems/search", "/bodies/search"}) {
		t.Fatalf("unexpected calls: %#v", paths)
	}
}

func TestSpaceMatchesActiveStatesOfOnlyTheControllingFaction(t *testing.T) {
	system := M{
		"controlling_minor_faction": "Test controller", "controlling_minor_faction_state": "Boom",
		"minor_faction_presences": []M{
			{"name": "Test neighbor", "state": "War", "active_states": []string{"War"}},
			{"name": "Test controller", "state": "Boom", "active_states": []string{"Boom", "Civil Liberty"}, "pending_states": []string{"Holiday"}},
		},
	}
	for _, example := range []struct {
		mode   string
		states []string
		want   bool
	}{
		{"all", []string{"Boom", "Civil Liberty"}, true},
		{"all", []string{"Boom", "War"}, false},
		{"any", []string{"War", "Civil Liberty"}, true},
		{"none", []string{"Boom", "Holiday"}, false},
		{"none", []string{"War", "Holiday"}, true},
	} {
		if got := spaceMatchesFactionStates(system, example.states, example.mode); got != example.want {
			t.Errorf("%s %v: got %v, want %v", example.mode, example.states, got, example.want)
		}
	}
}
