package riches

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestPlanDefaultsAndSelectsByRequestedReward(t *testing.T) {
	const fixture = `{"count":5,"reference":{"name":"Origin","x":0,"y":0,"z":0},"results":[
		{"system_name":"Mapped","system_x":2,"system_y":0,"system_z":0,"name":"Mapped 1","subtype":"Water world","distance_to_arrival":50,"estimated_scan_value":200000,"estimated_mapping_value":1300000,"terraforming_state":"Terraformable"},
		{"system_name":"Scanned","system_x":4,"system_y":0,"system_z":0,"name":"Scanned 1","subtype":"Earth-like world","distance_to_arrival":10000,"estimated_scan_value":300000,"estimated_mapping_value":1000000,"terraforming_state":"Terraformed"},
		{"system_name":"Too cheap","system_x":1,"system_y":0,"system_z":0,"name":"Too cheap 1","subtype":"Water world","distance_to_arrival":100,"estimated_scan_value":600000,"estimated_mapping_value":999999},
		{"system_name":"Too distant","system_x":3,"system_y":0,"system_z":0,"name":"Too distant 1","subtype":"Water world","distance_to_arrival":10001,"estimated_scan_value":700000,"estimated_mapping_value":1500000},
		{"system_name":"Unknown value","system_x":5,"system_y":0,"system_z":0,"name":"Unknown value 1","subtype":"Water world","distance_to_arrival":100,"estimated_scan_value":800000}
	]}`
	for _, tc := range []struct {
		mapping     bool
		field, stop string
	}{
		{true, "estimated_mapping_value", "Mapped"},
		{false, "estimated_scan_value", "Scanned"},
	} {
		t.Run(tc.stop, func(t *testing.T) {
			client := &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				var query bodySearch
				if err := json.NewDecoder(req.Body).Decode(&query); err != nil {
					t.Fatal(err)
				}
				if _, ok := query.Sort[0][tc.field]; !ok {
					t.Fatalf("unexpected ranking %+v", query.Sort)
				}
				if mapping, _ := json.Marshal(query.Filters["estimated_mapping_value"]); string(mapping) != `{"comparison":"\u003c=\u003e","value":[1000000,1000000000]}` {
					t.Fatalf("unexpected mapping filter: %s", mapping)
				}
				if arrival, _ := json.Marshal(query.Filters["distance_to_arrival"]); string(arrival) != `{"comparison":"\u003c=\u003e","value":[0,10000]}` {
					t.Fatalf("unexpected arrival filter: %s", arrival)
				}
				return response(fixture)
			})}
			route, err := NewPlanner(client).Plan(context.Background(), Options{
				Origin: "Origin", JumpRangeLY: 5, RadiusLY: 20, MaxResults: 1, UseMappingValue: tc.mapping,
			})
			if err != nil {
				t.Fatal(err)
			}
			if route.MinMappingValue != DefaultMinMappingValue || route.MaxDistanceLS != DefaultMaxDistanceLS ||
				route.MaxResults != 1 || len(route.Stops) != 1 || route.Stops[0].System != tc.stop || !route.Truncated {
				t.Fatalf("unexpected route %+v", route)
			}
			if terra := route.Stops[0].Bodies[0].Terraformable; terra == nil || *terra != tc.mapping {
				t.Fatalf("wrong terraforming flag %v", terra)
			}
		})
	}
}

func TestPlanDestinationUsesCorridorAndEndsAtDestination(t *testing.T) {
	client := &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		var query bodySearch
		if err := json.NewDecoder(req.Body).Decode(&query); err != nil {
			t.Fatal(err)
		}
		if query.ReferenceSystem != "" || query.ReferenceRoute == nil ||
			query.ReferenceRoute.Source != "Origin" || query.ReferenceRoute.Destination != "Destination" {
			t.Fatalf("unexpected corridor query %+v", query)
		}
		return response(`{"count":3,"reference":[{"name":"Origin","x":0,"y":0,"z":0},
			{"name":"Destination","id64":"18446744073709551600","x":100,"y":0,"z":0}],"results":[
			{"system_name":"Along","system_x":75,"system_y":10,"system_z":0,"name":"Along 1","subtype":"Water world","distance_to_arrival":10,"estimated_mapping_value":1100000},
			{"system_name":"Too far sideways","system_x":50,"system_y":30,"system_z":0,"name":"Too far sideways 1","subtype":"Water world","distance_to_arrival":10,"estimated_mapping_value":1100000},
			{"system_name":"Beyond corridor","system_x":125,"system_y":0,"system_z":0,"name":"Beyond corridor 1","subtype":"Water world","distance_to_arrival":10,"estimated_mapping_value":1100000}
		]}`)
	})}
	route, err := NewPlanner(client).Plan(context.Background(), Options{
		Origin: "Origin", Destination: "Destination", JumpRangeLY: 20, RadiusLY: 20, Loop: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if route.Destination != "Destination" || route.DestinationSystemAddress != "18446744073709551600" ||
		len(route.Stops) != 2 || route.Stops[0].System != "Along" || route.Stops[0].EstimatedJumps != 4 ||
		route.Stops[1].System != route.Destination || !route.Stops[1].Transit || len(route.Stops[1].Bodies) != 0 ||
		route.Stops[1].EstimatedJumps != 2 {
		t.Fatalf("unexpected destination route %+v", route)
	}
}

func TestPlanLoopFiltersKnownThargoidSystems(t *testing.T) {
	requests := []string{}
	client := &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case thargoidStatesURL:
			requests = append(requests, "states")
			return response(`{"values":["None","Invasion"]}`)
		case unsafeSearchURL:
			var query bodySearch
			if err := json.NewDecoder(req.Body).Decode(&query); err != nil {
				t.Fatal(err)
			}
			if _, ok := query.Filters["allegiance"]; ok {
				requests = append(requests, "allegiance")
				return response(`{"count":1,"results":[{"name":"Controlled","id64":"200"}]}`)
			}
			if _, ok := query.Filters["thargoid_war_state"]; ok {
				requests = append(requests, "war")
				return response(`{"count":1,"results":[{"name":"Old name","id64":"300"}]}`)
			}
			t.Fatalf("unexpected system filter: %+v", query.Filters)
		case searchURL:
			requests = append(requests, "bodies")
			return response(`{"count":4,"reference":{"name":"Origin","id64":100,"x":0,"y":0,"z":0},"results":[
				{"system_name":"Origin","system_id64":100,"system_x":0,"system_y":0,"system_z":0,"name":"Origin 1","subtype":"Earth-like world","distance_to_arrival":50,"estimated_mapping_value":1000000},
				{"system_name":"controlled","system_x":3,"system_y":0,"system_z":0,"name":"Controlled 1","subtype":"Water world","distance_to_arrival":50,"estimated_mapping_value":1100000},
				{"system_name":"Invaded","system_id64":300,"system_x":4,"system_y":0,"system_z":0,"name":"Invaded 1","subtype":"Water world","distance_to_arrival":50,"estimated_mapping_value":1100000},
				{"system_name":"Peaceful","system_x":7,"system_y":0,"system_z":0,"name":"Peaceful 1","subtype":"Water world","distance_to_arrival":50,"estimated_mapping_value":1100000}
			]}`)
		default:
			t.Fatalf("unexpected URL %s", req.URL)
		}
		return nil, nil
	})}
	opts := Options{Origin: "Origin", JumpRangeLY: 5, RadiusLY: 10, Loop: true, AvoidThargoids: true}
	route, err := NewPlanner(client).Plan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(requests, []string{"bodies", "states", "allegiance", "war"}) ||
		len(route.Stops) != 3 || route.Stops[0].System != "Origin" || route.Stops[1].System != "Peaceful" ||
		route.Stops[2].System != "Origin" || route.Stops[2].EstimatedJumps != 2 || !route.Stops[2].Transit ||
		len(route.Stops[2].Bodies) != 0 {
		t.Fatalf("unexpected loop %+v; requests=%v", route, requests)
	}

	requests = nil
	opts.AvoidThargoids = false
	route, err = NewPlanner(client).Plan(context.Background(), opts)
	if err != nil || len(route.Stops) != 5 || strings.Join(requests, ",") != "bodies" {
		t.Fatalf("unchecked avoidance unexpectedly affected route %+v; requests=%v; err=%v", route, requests, err)
	}
}
