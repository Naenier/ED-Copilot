package riches

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func optionValue[T any](value T) *T { return &value }

func (fn transportFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func response(body string) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestPlanFiltersBodiesAndOrdersTheRoute(t *testing.T) {
	client := &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.String() != searchURL {
			t.Fatalf("unexpected request %s %s", req.Method, req.URL)
		}
		var query bodySearch
		if err := json.NewDecoder(req.Body).Decode(&query); err != nil {
			t.Fatal(err)
		}
		if query.ReferenceSystem != "Source" || query.Size != pageSize || query.Page != 0 {
			t.Fatalf("unexpected query %+v", query)
		}
		if filters, _ := json.Marshal(query.Filters); string(filters) != `{"distance":{"max":20,"min":0},"subtype":{"value":["Earth-like world","Water world"]}}` {
			t.Fatalf("wrong Spansh filters: %s", filters)
		}
		return response(`{
			"count":9,"reference":{"name":"Source","id64":18446744073709551600,"x":0,"y":0,"z":0},
			"results":[
				{"system_name":"Source","system_id64":"18446744073709551600","system_x":0,"system_y":0,"system_z":0,"name":"Home","subtype":"Earth-like world"},
				{"system_name":"Alpha","system_id64":"9223372036854775809","system_x":6,"system_y":0,"system_z":0,"body_id":12,"name":"Alpha A 3","subtype":"Water world","distance_to_arrival":231.5,"estimated_scan_value":100000,"estimated_mapping_value":1230000},
				{"system_name":"Alpha","system_id64":"9223372036854775809","system_x":6,"system_y":0,"system_z":0,"name":"Alpha A 2","subtype":"Earth-like world","distance_to_arrival":150},
				{"system_name":"alpha","system_x":6,"system_y":0,"system_z":0,"name":"alpha a 2","subtype":"Earth-like world","distance_to_arrival":150},
				{"system_name":"Beta","system_x":-7,"system_y":0,"system_z":0,"name":"Beta 3","subtype":"Water world"},
				{"system_name":"Gamma","system_x":14,"system_y":0,"system_z":0,"name":"Gamma 1","subtype":"Water world"},
				{"system_name":"Wrong Type","system_x":2,"system_y":0,"system_z":0,"name":"Wrong Type 2","subtype":"High metal content world"},
				{"system_name":"Too Far","system_x":21,"system_y":0,"system_z":0,"name":"Too Far 2","subtype":"Earth-like world"},
				{"system_name":"Missing Position","system_x":1,"system_y":0,"name":"Missing Position 3","subtype":"Earth-like world"}
			]}`)
	})}
	route, err := NewPlanner(client).Plan(context.Background(), Options{
		Origin: " Source ", JumpRangeLY: 7.5, RadiusLY: 20, MaxDistanceLS: optionValue(0.0), MinMappingValue: optionValue(int64(0)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if route.Origin != "Source" || route.OriginSystemAddress != "18446744073709551600" || len(route.Stops) != 4 || route.Truncated {
		t.Fatalf("unexpected route %+v", route)
	}
	for i, want := range []struct {
		system   string
		distance float64
		jumps    int
	}{{"Source", 0, 0}, {"Alpha", 6, 1}, {"Gamma", 8, 2}, {"Beta", 21, 3}} {
		got := route.Stops[i]
		if got.System != want.system || got.DistanceLY != want.distance || got.EstimatedJumps != want.jumps {
			t.Errorf("stop %d = %+v, want %+v", i, got, want)
		}
	}
	if route.Stops[0].Bodies[0].Designation != "Home" {
		t.Fatal("named body designation was stripped")
	}
	alpha := route.Stops[1]
	if alpha.SystemAddress != "9223372036854775809" || len(alpha.Bodies) != 2 || alpha.Bodies[0].Designation != "A 2" || alpha.Bodies[0].Type != "earthLike" {
		t.Fatalf("unexpected Alpha stop: %+v", alpha)
	}
	b := alpha.Bodies[1]
	if b.Type != "waterWorld" || b.Designation != "A 3" || b.ID == nil || *b.ID != 12 || b.DistanceLS == nil || *b.DistanceLS != 231.5 ||
		b.ScanCredits == nil || *b.ScanCredits != 100000 || b.MappingCredits == nil || *b.MappingCredits != 1230000 {
		t.Fatalf("unexpected mapping target: %+v", b)
	}
}

func TestPlanContinuesPagedSearchForACompleteSystem(t *testing.T) {
	calls := 0
	planet := `{"system_name":"Alpha","system_x":10,"system_y":0,"system_z":0,"name":"Alpha A 1","subtype":"Water world"}`
	client := &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		var query bodySearch
		if err := json.NewDecoder(req.Body).Decode(&query); err != nil {
			t.Fatal(err)
		}
		if query.Page != calls {
			t.Fatalf("requested page %d, expected %d", query.Page, calls)
		}
		calls++
		switch query.Page {
		case 0:
			return response(`{"count":251,"reference":{"name":"Origin","x":0,"y":0,"z":0},"results":[` + strings.Repeat(planet+",", pageSize-1) + planet + `]}`)
		case 1:
			return response(`{"count":251,"results":[` + strings.ReplaceAll(planet, "Alpha A 1", "Alpha A 2") + `]}`)
		default:
			t.Fatalf("unexpected page %d", query.Page)
			return nil, nil
		}
	})}
	route, err := NewPlanner(client).Plan(context.Background(), Options{
		Origin: "Origin", JumpRangeLY: 5, RadiusLY: 100, MaxDistanceLS: optionValue(0.0), MinMappingValue: optionValue(int64(0)),
	})
	if err != nil || calls != 2 || len(route.Stops) != 1 || len(route.Stops[0].Bodies) != 2 || route.Truncated {
		t.Fatalf("expected a complete system after two pages, route=%+v calls=%d err=%v", route, calls, err)
	}
}

func TestPlanResolvesCanonicalSpellingAfterProviderRejectsName(t *testing.T) {
	var queries []string
	client := &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodGet {
			if req.URL.Path != "/api/search/systems" || req.URL.Query().Get("q") != "brestla" {
				t.Fatalf("unexpected system lookup %s", req.URL)
			}
			return response(`{"results":[{"name":"Brestla A"},{"name":"Brestla"}]}`)
		}
		var query bodySearch
		if err := json.NewDecoder(req.Body).Decode(&query); err != nil {
			t.Fatal(err)
		}
		queries = append(queries, query.ReferenceSystem)
		if query.ReferenceSystem == "brestla" {
			return &http.Response{StatusCode: http.StatusBadRequest, Body: http.NoBody}, nil
		}
		return response(`{"count":0,"reference":{"name":"Brestla","x":0,"y":0,"z":0},"results":[]}`)
	})}
	route, err := NewPlanner(client).Plan(context.Background(), Options{Origin: "brestla", JumpRangeLY: 70, RadiusLY: 1})
	if err != nil || route.Origin != "Brestla" || len(route.Stops) != 0 || strings.Join(queries, ",") != "brestla,Brestla" {
		t.Fatalf("route=%+v queries=%v err=%v", route, queries, err)
	}
}
