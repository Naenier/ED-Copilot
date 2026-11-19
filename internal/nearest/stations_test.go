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

func TestStationSearchVerifiesSystemAndOwnerConditionsTogether(t *testing.T) {
	const systemID = "9334882111829977"
	for _, malformed := range []bool{false, true} {
		name := "verified states"
		if malformed {
			name = "missing provider results"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload M
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				filters := v.Obj(payload["filters"])
				switch r.URL.Path {
				case "/systems/search":
					if ids := stationValues(v.At(filters, "id64", "value"), 20); len(ids) > 0 {
						if !slices.Equal(ids, []string{systemID}) {
							t.Errorf("owner verification changed the system address: %v", ids)
						}
						if malformed {
							_, _ = w.Write([]byte(`{"count":1,"results":{}}`))
							return
						}
						_, _ = w.Write([]byte(`{"count":1,"results":[{"id64":9334882111829977,"name":"Near",
							"minor_faction_presences":[{"name":"Good Owner","state":"Boom","active_states":["War"]},
							{"name":"Wrong Owner","state":"War"}]}]}`))
						return
					}
					linkedStations := v.Rows(filters["stations"])
					if payload["reference_system"] != "Sol" ||
						!slices.Equal(stationValues(v.At(filters, "security", "value"), 20), []string{"High"}) ||
						len(linkedStations) != 1 ||
						!slices.Equal(stationValues(v.At(linkedStations[0], "services", "value"), 20), []string{"Repair"}) {
						t.Errorf("system eligibility dropped a condition: %v", payload)
					}
					_, _ = w.Write([]byte(`{"count":1,"results":[{"id64":9334882111829977,"name":"Near"}]}`))
				case "/stations/search":
					if ids := stationValues(v.At(filters, "system_id64", "value"), 20); !slices.Equal(ids, []string{systemID}) {
						t.Errorf("station query was not restricted to verified systems: %v", payload)
					}
					_, _ = w.Write([]byte(`{"count":2,"results":[
						{"name":"Qualified Port","market_id":1,"system_id64":9334882111829977,"system_name":"Near","controlling_minor_faction":"Good Owner"},
						{"name":"Wrong Owner Port","market_id":2,"system_id64":9334882111829977,"system_name":"Near","controlling_minor_faction":"Wrong Owner"}]}`))
				default:
					t.Errorf("unexpected path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			client := New(server.Client())
			client.BaseURL = server.URL
			out, err := client.Search(t.Context(), Query{
				Category: "stations", Origin: "Sol", Filters: M{
					"security": "High", "services": []string{"Repair"},
					"ownerStates": []string{"Boom", "War"}, "ownerStateMode": "all",
				},
			})
			if malformed {
				if err == nil {
					t.Fatal("missing owner observations looked like a successful empty search")
				}
				return
			}
			if err != nil || out.Total != 1 || out.HasMore || len(out.Rows) != 1 || out.Rows[0]["name"] != "Qualified Port" {
				t.Fatalf("eligible port lost or wrong port included: %+v, %v", out, err)
			}
		})
	}
}

func TestStationTypeFamiliesAndSpecificRoles(t *testing.T) {
	query := Query{Category: "stations", Filters: M{"stationType": "mobile-ports", "ignoreCarriers": true}}
	filter, _, err := stationFilters(query)
	if err != nil || !slices.Equal(stationValues(v.At(filter, "type", "value"), 20), []string{stationMegaship}) {
		t.Fatalf("carrier exclusion within group: %v, %v", filter, err)
	}

	query.Filters = M{"stationType": "orbital-starports", "surface": "no"}
	filter, _, err = stationFilters(query)
	if err != nil || !slices.Equal(stationValues(v.At(filter, "type", "value"), 20), stationOrbitals) {
		t.Fatalf("orbital family does not contain all starport models: %v, %v", filter, err)
	}

	query.Filters = M{"stationType": "scientific-outposts", "economy": "High Tech"}
	filter, criteria, err := stationFilters(query)
	if err != nil || !criteria.kind.role || !slices.Equal(stationValues(v.At(filter, "type", "value"), 20), []string{"Outpost"}) ||
		!slices.Equal(stationValues(v.At(filter, "primary_economy", "value"), 20), []string{"High Tech"}) {
		t.Fatalf("role must be applied through the known outpost type and economy: %v, %v", filter, err)
	}
	query.Filters["economy"] = "Extraction"
	if _, _, err := stationFilters(query); err == nil {
		t.Fatal("conflicting station profiles must not silently override the chosen economy")
	}
	query.Filters = M{"stationType": "odyssey-settlements", "surface": "legacy"}
	if _, _, err := stationFilters(query); err == nil {
		t.Fatal("legacy ground setting includes Odyssey settlements")
	}

	query.Filters = M{"stationType": "dockable-megaships"}
	filter, _, err = stationFilters(query)
	services := v.Rows(filter["services"])
	if err != nil || len(services) != 1 || stationText(v.At(services[0], "name", "value")) != "Dock" {
		t.Fatalf("dockable megaships need the observed docking service: %v, %v", filter, err)
	}
	query.Filters = M{"stationType": "stronghold-carriers"}
	filter, _, err = stationFilters(query)
	if err != nil || v.At(filter, "name", "value") != stationStronghold || filter["type"] != nil {
		t.Fatalf("stronghold search must not depend on its misclassified station type: %v, %v", filter, err)
	}
}

func TestStationTitanWrecksUseSystemPositionAndExcludeDocking(t *testing.T) {
	query := Query{
		Category: "stations", Origin: "Sol", Limit: 10, Sort: "arrival",
		Filters: M{"stationType": "thargoid-titans", "radiusLy": 200, "maxArrivalLs": 600},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/systems/search" {
			t.Errorf("wrong Titan request: %s", r.URL.Path)
		}
		var payload M
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("invalid Titan payload: %v", err)
		}
		ids := stationValues(v.At(payload, "filters", "id64", "value"), 20)
		if payload["reference_system"] != "Sol" || v.At(payload, "filters", "distance", "max") != float64(200) ||
			!slices.Equal(ids, []string{"10477373803", "7265876583833", "13866167510393"}) {
			t.Errorf("incorrect Titan constraint: %v", payload)
		}
		_, _ = w.Write([]byte(`{"results":[{"id64":10477373803,"distance":0},{"id64":7265876583833,"distance":162},{"id64":13866167510393,"distance":168}]}`))
	}))
	defer server.Close()
	client := New(server.Client())
	client.BaseURL = server.URL
	result, err := client.searchStations(context.Background(), query)
	if err != nil || len(result.Rows) != 3 || result.Rows[0]["name"] != "Taranis" || result.Rows[2]["system"] != "Sol" ||
		result.Rows[2]["type"] != stationTitan || result.Rows[2]["pad"] != nil || result.Rows[2]["distanceLy"] == nil {
		t.Fatalf("Titan rows must use known wreck sites, position and no landing pad: %+v, %v", result, err)
	}
	query.Filters["minPad"] = "S"
	result, err = client.searchStations(context.Background(), query)
	if err != nil || len(result.Rows) != 0 || result.Note == "" {
		t.Fatalf("wrecks cannot match a landing-pad requirement: %+v, %v", result, err)
	}
}

func TestOutfittingSearchFillsPastUnconfirmedProviderPage(t *testing.T) {
	query := Query{Category: "outfitting", Origin: "Sol", Limit: 24, Filters: M{"ships": []string{"Python"}}}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request M
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("bad request: %v", err)
		}
		requests++
		var rows []M
		if v.Int(request["page"]) == 0 {
			for range 48 {
				rows = append(rows, M{"name": "Unconfirmed", "system_name": "Sol"})
			}
		} else {
			rows = []M{{"name": "Confirmed", "system_name": "Sol", "ships": []M{{"name": "Python"}}}}
		}
		_ = json.NewEncoder(w).Encode(M{"count": 49, "results": rows})
	}))
	defer server.Close()
	client := New(server.Client())
	client.BaseURL = server.URL
	result, err := client.searchStations(context.Background(), query)
	if err != nil || len(result.Rows) != 1 || result.Rows[0]["name"] != "Confirmed" || result.HasMore || requests != 2 {
		t.Fatalf("nearest confirmed stock not returned: %+v, %v, %d requests", result, err, requests)
	}
}

func TestStationNameZeroArrivalAndStrongholdExclusion(t *testing.T) {
	filters, criteria, err := stationFilters(Query{Category: "stations", Name: "first", Filters: M{
		"maxArrivalLs": 0, "ignoreCarriers": true,
	}})
	arrival, _ := v.At(filters, "distance_to_arrival", "value").([]float64)
	if err != nil || v.At(filters, "name", "value") != "*first*" ||
		!slices.Equal(arrival, []float64{0, 0}) {
		t.Fatalf("station search fields lost: %v, %v", filters, err)
	}
	if !criteria.reviewsResults() || criteria.matchesStation(M{"type": "Planetary Outpost", "name": stationStronghold}) ||
		!criteria.matchesStation(M{"type": "Planetary Outpost", "name": "Ground Port"}) {
		t.Fatal("carrier exclusion must be based on actual type even if Spansh misclassifies the station")
	}
	query := Query{Category: "outfitting", Filters: M{"modules": []M{{"name": "Heat Sink Launcher", "class": 0}}}}
	_, outfit, err := stationFilters(query)
	if err != nil {
		t.Fatal(err)
	}
	withoutClass := M{"modules": []M{{"name": "Heat Sink Launcher"}}}
	if _, matched := stationStock(withoutClass, outfit); matched {
		t.Fatal("stock with no class cannot confirm a class 0 module")
	}
}
