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
			_, _ = w.Write([]byte(`{"count":101,"results":[{"id64":9007199254740999,"name":"Test",
				"minor_faction_presences":[{"name":"Independent Test"}]}]}`))
		case "/bodies/search":
			if got := v.Arr(v.At(payload, "filters", "system_id64", "value")); !reflect.DeepEqual(got, []any{firstID}) {
				t.Errorf("body search omitted or changed system IDs: %#v", got)
			}
			sortBy := v.Rows(payload["sort"])
			sortMaterials := v.Rows(sortBy[0]["materials"])
			if len(sortMaterials) != 2 || sortMaterials[0]["name"] != "Polonium" || sortMaterials[1]["name"] != "Yttrium" {
				t.Errorf("changed user material priority: %#v", sortBy)
			}
			materials := v.Rows(v.At(payload, "filters", "materials"))
			if len(materials) != 2 || materials[0]["name"] != "Polonium" || len(v.Arr(materials[0]["value"])) != 2 {
				t.Errorf("wrong range field for body materials: %#v", materials)
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

func TestSystemSearchPaginatesByVerifiedControllingFactionStates(t *testing.T) {
	const response = `{"count":101,"results":[
		{"id64":11,"name":"Wrong","controlling_minor_faction":"Other",
		 "minor_faction_presences":[{"name":"Other","state":"Boom"},{"name":"Neighbour","state":"War"}]},
		{"id64":12,"name":"First","controlling_minor_faction":"Controller","controlling_minor_faction_state":"Boom",
		 "minor_faction_presences":[{"name":"Controller","active_states":["War"]}]}]}`
	const continuation = `{"count":101,"results":[
		{"id64":13,"name":"Second","controlling_minor_faction":"Controller",
		 "minor_faction_presences":[{"name":"Controller","active_states":["Boom","War"]}]}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload M
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Path != "/systems/search" || v.Int(payload["size"]) != factionStateScanPage ||
			payload["reference_system"] != "Sol" || v.At(payload, "filters", "controlling_minor_faction_state") != nil {
			t.Errorf("faction-state matching depended on the primary provider state: %v", payload)
		}
		switch v.Int(payload["page"]) {
		case 0:
			_, _ = w.Write([]byte(response))
		case 1:
			_, _ = w.Write([]byte(continuation))
		default:
			t.Errorf("unexpected provider page: %v", payload["page"])
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	client := New(server.Client())
	client.BaseURL = server.URL
	query := Query{Category: "systems", Origin: "Sol", Limit: 1,
		Filters: M{"factionStates": []string{"Boom", "War"}, "stateMode": "all"}}
	for page, want := range []string{"First", "Second"} {
		query.Page = page
		result, err := client.Search(t.Context(), query)
		if err != nil || result.Total != 2 || len(result.Rows) != 1 || result.Rows[0]["system"] != want || result.HasMore != (page == 0) {
			t.Fatalf("page %d = %+v, %v", page, result, err)
		}
	}
}

func TestSpaceBodyFiltersKeepConcreteTypesAndSeparateRingAndBeltQueries(t *testing.T) {
	q := Query{
		Category: "bodies", Origin: "Sol", Sort: "mapping",
		Filters: M{
			"bodyKind": "Planet", "planetType": "Class III gas giant", "ringType": "Icy", "beltType": "any",
			"minGravity": 1.0, "maxGravity": 2.3, "maxTemperature": 220, "maxArrivalLs": 30000,
			"volcanism": "No volcanism", "atmosphere": "Thick Helium", "landable": true,
		},
	}
	got := spaceBodyFilters(q)
	want := M{
		"type": M{"value": []string{"Planet"}}, "subtype": M{"value": []string{"Class III gas giant"}},
		"rings":               []M{{"type": []string{"Icy"}}},
		"belts":               []M{{"type": []string{"Icy", "Metal Rich", "Metallic", "Rocky"}}},
		"gravity":             M{"comparison": "<=>", "value": []float64{1, 2.3}},
		"surface_temperature": M{"comparison": "<=>", "value": []float64{0, 220}},
		"distance_to_arrival": M{"comparison": "<=>", "value": []float64{0, 30000}},
		"volcanism_type":      M{"value": []string{"No volcanism"}}, "atmosphere": M{"value": []string{"Thick Helium"}},
		"is_landable": M{"value": true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	if sort := spaceSort(q); !reflect.DeepEqual(sort[0], M{"estimated_mapping_value": M{"direction": "desc"}}) {
		t.Fatalf("mapping value sort omitted: %v", sort)
	}
}

func TestSpaceSystemPermitAndSecondaryEconomyRemainIndependent(t *testing.T) {
	q := Query{
		Category: "systems", Origin: "Sol", Sort: "name",
		Filters: M{"systemEconomy": "Industrial", "secondaryEconomy": "Service", "permit": "open", "populationMin": 10000.0},
	}
	got := spaceSystemFilters(q)
	want := M{
		"primary_economy": M{"value": []string{"Industrial"}}, "secondary_economy": M{"value": []string{"Service"}},
		"needs_permit": M{"value": false}, "population": M{"comparison": "<=>", "value": []float64{10000, populationLimit}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	if sort := spaceSort(q); !reflect.DeepEqual(sort, []M{{"name": M{"direction": "asc"}}}) {
		t.Fatalf("name order overridden by distance: %v", sort)
	}
}

func TestNearestRejectsInvalidPageAndContradictoryRanges(t *testing.T) {
	bad := []M{
		{"page": 1.5}, {"page": 100}, {"page": "last"},
		{"filters": M{"radiusLy": 0}}, {"filters": M{"maxArrivalLs": -1}},
		{"filters": M{"minGravity": 2, "maxGravity": 1}},
		{"filters": M{"population": "unpopulated", "populationMin": 1}},
		{"filters": M{"population": "populated", "populationMax": 0}},
	}
	for _, input := range bad {
		input["category"] = "systems"
		if _, err := FromInput(input, "Sol"); err == nil {
			t.Errorf("accepted invalid criteria: %v", input)
		}
	}
	q, err := FromInput(M{"category": "stations", "filters": M{"maxArrivalLs": 0}}, "Sol")
	arrival, known := v.Number(q.Filters["maxArrivalLs"])
	if err != nil || !known || arrival != 0 {
		t.Fatalf("valid zero arrival limit was lost: %v, %v", q, err)
	}
}

func TestKnownReferencePositionAndLastNearestPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input M
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		coords := v.Obj(input["reference_coords"])
		if len(coords) != 3 || v.Num(coords["x"]) != 22.5 || input["reference_system"] != nil {
			t.Errorf("known position not used: %v", input)
		}
		_, _ = w.Write([]byte(`{"count":3000,"results":[{"name":"Example","id64":1}]}`))
	}))
	defer server.Close()
	c := New(server.Client())
	c.BaseURL = server.URL
	q := Query{Category: "systems", Origin: "Not indexed yet", OriginCoords: []float64{22.5, 0, 0}, Page: maxPages - 2, Limit: DefaultLimit}
	first, err := c.Search(t.Context(), q)
	if err != nil || !first.HasMore {
		t.Fatalf("provider count should preserve pagination: %+v, %v", first, err)
	}
	q.Page++
	last, err := c.Search(t.Context(), q)
	if err != nil || last.HasMore || last.Note == "" {
		t.Fatalf("last page offers an invalid continuation: %+v, %v", last, err)
	}
}
