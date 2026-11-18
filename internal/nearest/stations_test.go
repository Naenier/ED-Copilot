package nearest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestStationSearchFiltersAndNormalizeStocks(t *testing.T) {
	query := Query{
		Category: "outfitting", Origin: "Sol", Limit: 24,
		Filters: M{
			"minPad": "L", "maxArrivalLs": 5000, "surface": "no", "ignoreCarriers": true,
			"services":       []string{"Material Trader", "Technology Broker"},
			"materialTrader": "Raw", "owner": "Mother Gaia", "ownerMode": "not",
			"ships":   []string{"Python"},
			"modules": []M{{"name": "Frame Shift Drive (SCO)", "class": 5, "rating": "A"}},
		},
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/stations/search" || r.Method != "POST" {
			t.Errorf("bad station request %s %s", r.Method, r.URL.Path)
		}
		var payload M
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode station request: %v", err)
		}
		filters := v.Obj(payload["filters"])
		if payload["reference_system"] != "Sol" || v.At(filters, "has_large_pad", "value") != true ||
			v.At(filters, "is_planetary", "value") != false || v.At(filters, "type", "logic") != "not" ||
			v.At(filters, "controlling_minor_faction", "logic") != "not" {
			t.Errorf("station constraints missing: %v", payload)
		}
		groups := v.Rows(filters["services"])
		if len(groups) != 2 || v.Arr(v.At(groups[0], "name", "value"))[0] != "Material Trader" ||
			v.Arr(v.At(groups[1], "name", "value"))[0] != "Technology Broker" {
			t.Errorf("service groups must all match: %v", groups)
		}
		modules := v.Rows(filters["modules"])
		if len(modules) != 1 || v.Arr(v.At(modules[0], "class", "value"))[0] != float64(5) ||
			v.Arr(v.At(modules[0], "rating", "value"))[0] != "A" {
			t.Errorf("module class and rating lost: %v", filters)
		}
		_, _ = w.Write([]byte(`{"count":1,"results":[{
			"name":"Test Dock","system_name":"Test System","system_id64":9334882111829977,"market_id":123,
			"distance":22.25,"distance_to_arrival":700,"has_large_pad":true,"material_trader":"Raw",
			"services":[{"name":"Material Trader"},{"name":"Technology Broker"}],
			"ships":[{"name":"Python","price":50000000}],
			"modules":[{"name":"Frame Shift Drive (SCO)","class":5,"rating":"A","ed_symbol":"Int_Hyperdrive_Overcharge_Size5_Class5","price":6100000}]
		}]}`))
	}))
	defer server.Close()
	client := New(server.Client())
	client.BaseURL = server.URL
	result, err := client.searchStations(context.Background(), query)
	if err != nil || len(result.Rows) != 1 || requests != 1 || result.Total != 1 || result.HasMore {
		t.Fatalf("outfitting result: %+v, %v, %d requests", result, err, requests)
	}
	row := result.Rows[0]
	if row["systemAddress"] != "9334882111829977" || row["pad"] != "L" || row["materialTrader"] != "Raw" ||
		!slices.Contains(row["services"].([]string), "Technology Broker") || len(v.Rows(row["items"])) != 2 {
		t.Errorf("unexpected row %v", row)
	}
	item := v.Rows(row["items"])[0]
	if item["name"] != "Python" || item["discounted"] != true || v.Num(item["basePrice"]) < 50_000_000 {
		t.Errorf("ship stock and base price must be separate: %+v", item)
	}
}

func TestStationSecondaryStateAndMount(t *testing.T) {
	criteria := stationCriteria{
		ownerStates: []string{"Boom", "War"}, ownerStateMode: "all",
		ownerGovernment: "Democracy", ownerControlled: true,
		items: []stationOutfit{{kind: "module", name: "Beam Laser", size: 2, hasSize: true, mount: "gimballed"}},
	}
	row := M{"controlling_minor_faction": "Faction", "modules": []M{
		{"name": "Beam Laser", "class": 2, "ed_symbol": "Hpt_BeamLaser_Fixed_Medium"},
		{"name": "Beam Laser", "class": 2, "ed_symbol": "Hpt_BeamLaser_Gimbal_Medium"},
	}}
	owner := M{"name": "Faction", "government": "Democracy", "state": "Boom", "active_states": []string{"Boom", "War"}}
	system := M{"controlling_minor_faction": "Faction", "minor_faction_presences": []M{owner}}
	if !stationFactionMatches(row, system, criteria) {
		t.Fatal("multiple observed faction states should match")
	}
	criteria.ownerStateMode = "none"
	if stationFactionMatches(row, system, criteria) {
		t.Fatal("excluded faction state matched")
	}
	items, matched := stationStock(row, criteria)
	if !matched || len(items) != 1 || items[0]["mount"] != "gimballed" {
		t.Errorf("mount must come from outfitting stock: %+v", items)
	}
}
