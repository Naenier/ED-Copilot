package colonization

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/value"
)

func sourceEligibilityClient(stations []M) HTTPClient {
	return mockDoer(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "system_names") {
			return responseJSON(M{"min_max": []M{{"name": "Origin", "id64": "1"}}}), nil
		}
		if strings.Contains(r.URL.Path, "/system/") {
			return responseJSON(M{"record": M{"name": "Origin", "id64": "1", "x": 0, "y": 0, "z": 0, "population": 100, "minor_faction_presences": []M{{"name": "Source Faction"}, {"name": "FleetCarrier"}, {"name": "Independent Carrier Traders"}}, "stations": stations}}), nil
		}
		return responseJSON(M{"count": 1, "results": []M{candidate("2", "Synthetic Candidate", 5)}}), nil
	})
}

func TestClaimSourceSearchRetainsFullOriginAndRejectsCarrierDepotAndExplicitAbsence(t *testing.T) {
	provider := []M{
		{"market_id": 1, "name": "Confirmed port", "type": "Coriolis", "controlling_minor_faction": "Source Faction", "services": []string{"systemcolonisation"}},
		{"market_id": 2, "name": "Unknown contact port", "type": "Outpost", "controlling_minor_faction": "Other Faction", "services": []string{"market"}},
		{"market_id": 3, "name": "Explicit absent port", "type": "Orbis", "controlling_minor_faction": "Source Faction", "hasColonisationContact": false, "services": []string{"systemcolonisation"}},
		{"market_id": 4, "name": "Provider carrier", "type": "FleetCarrier", "controlling_minor_faction": "FleetCarrier", "services": []string{"systemcolonisation"}},
		{"market_id": 5, "name": "Orbital building site", "type": "OrbitalConstructionSite", "controlling_minor_faction": "Builder Corporation", "services": []string{"systemcolonisation"}},
		{"market_id": 6, "name": "Construction depot", "type": "ColonisationConstructionDepot", "controlling_minor_faction": "Builder Corporation", "services": []string{"systemcolonisation"}},
		{"market_id": 7, "name": "Provider squadron carrier", "type": "SquadronCarrier", "controlling_minor_faction": "Source Faction", "services": []string{"systemcolonisation"}},
		{"market_id": 8, "name": "Depot Named Regular Port", "type": "Orbis", "controlling_minor_faction": "Source Faction", "services": []string{"systemcolonisation"}},
	}
	p := New(nil, WithHTTPClient(sourceEligibilityClient(provider)))
	dock := func(id int, name, typ, faction string, services []string) {
		p.Apply(M{"event": "Docked", "timestamp": "2026-11-08T00:00:00Z", "MarketID": id, "StarSystem": "Origin", "StationName": name, "StationType": typ, "StationFaction": M{"Name": faction}, "StationServices": services}, nil)
	}
	// Unknown journal services/type must not erase known provider facts.
	dock(1, "Confirmed port", "", "Source Faction", []string{"market"})
	dock(3, "Explicit absent port", "", "Source Faction", []string{"market"})
	// Covers both the old dropped-type journal path and the new typed path.
	dock(101, "Legacy journal carrier", "", "FleetCarrier", []string{"market"})
	dock(102, "Typed journal carrier", "SquadronCarrier", "Source Faction", []string{"systemcolonisation"})
	dock(103, "Journal regular port", "Coriolis", "Source Faction", []string{"systemcolonisation"})
	dock(104, "Known active depot", "", "Source Faction", []string{"systemcolonisation"})
	p.Apply(M{"event": "ColonisationConstructionDepot", "timestamp": "2026-11-08T00:00:01Z", "MarketID": 104, "StarSystem": "Origin", "StationName": "Known active depot", "ResourcesRequired": []M{}}, nil)
	result, err := p.Search(context.Background(), M{"originSystem": "Origin"})
	if err != nil {
		t.Fatal(err)
	}
	all := rows(value.Obj(result["origin"])["stations"])
	if len(all) != 12 {
		t.Fatalf("full station reference was filtered: %d", len(all))
	}
	byID := map[string]M{}
	for _, s := range all {
		byID[str(s["marketId"])] = s
		if placeholderFaction(str(s["faction"])) {
			t.Fatal("station placeholder exposed as faction", s)
		}
	}
	if !value.Bool(byID["1"]["hasColonisationContact"]) || byID["1"]["type"] != "Coriolis" || byID["3"]["hasColonisationContact"] != false {
		t.Fatal("journal unknown erased confirmed source facts")
	}
	if byID["102"]["type"] != "SquadronCarrier" {
		t.Fatal("journal station type was lost")
	}
	claims := rows(result["claimStations"])
	want := map[string]bool{"1": true, "2": true, "8": true, "103": true}
	if len(claims) != len(want) {
		t.Fatalf("bad claim sources: %+v", claims)
	}
	for _, s := range claims {
		if !want[str(s["marketId"])] {
			t.Fatal("ineligible source offered", s)
		}
	}
	if byID["2"]["hasColonisationContact"] != nil || !value.Bool(byID["2"]["claimSourceEligible"]) {
		t.Fatal("missing service was guessed as a negative contact")
	}
	factions := map[string]bool{}
	for _, f := range rows(result["factions"]) {
		factions[str(f["name"])] = true
		if placeholderFaction(str(f["name"])) {
			t.Fatal("placeholder faction suggestion", f)
		}
	}
	if !factions["Source Faction"] || !factions["Independent Carrier Traders"] {
		t.Fatal("real faction names removed")
	}
	filtered, err := p.Search(context.Background(), M{"originSystem": "Origin", "faction": "Source Faction"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows(filtered["claimStations"])) != 3 || len(rows(value.Obj(filtered["origin"])["stations"])) != 12 {
		t.Fatal("faction filter changed full origin reference or offered wrong sources")
	}
}

func TestClaimContactIsTriStateAndKnownCompletedDepotDoesNotBlockOrdinaryPort(t *testing.T) {
	for _, tc := range []struct {
		name     string
		row      M
		services any
		want     any
	}{{"missing", M{}, nil, nil}, {"services without new contact", M{}, []string{"market", "repair"}, nil}, {"positive service", M{}, []string{"system_colonisation"}, true}, {"explicit false", M{"hasColonisationContact": false}, []string{"systemcolonisation"}, false}, {"explicit true", M{"hasColonizationContact": true}, nil, true}} {
		t.Run(tc.name, func(t *testing.T) {
			if got := colonisationContact(tc.row, tc.services); got != tc.want {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
		})
	}
	provider := []M{{"market_id": 1, "name": "Completed regular port", "type": "Coriolis", "controlling_minor_faction": "Source Faction", "services": []string{"systemcolonisation"}}}
	p := New(nil, WithHTTPClient(sourceEligibilityClient(provider)))
	p.depots["1"] = M{"complete": true}
	result, err := p.Search(context.Background(), M{"originSystem": "Origin"})
	if err != nil || len(rows(result["claimStations"])) != 1 {
		t.Fatal("historical completed depot blocked a now ordinary port", err, result)
	}
}

func TestUnknownContactSourceStaysSelectableWithWarningAndPlaceholderFactionCannotBeSavedBySearch(t *testing.T) {
	p := New(M{"faction": "Source Faction"}, WithHTTPClient(sourceEligibilityClient([]M{{"market_id": 1, "name": "Unknown contact port", "type": "Outpost", "controlling_minor_faction": "Source Faction"}})))
	result, err := p.Search(context.Background(), M{"originSystem": "Origin", "faction": ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows(result["claimStations"])) != 1 || rows(result["claimStations"])[0]["hasColonisationContact"] != nil {
		t.Fatal("unknown contact source lost")
	}
	if !strings.Contains(strings.Join(stringsOf(result["warnings"]), " "), "Наличие контакта колонизации не подтверждено") {
		t.Fatal("unknown contact warning absent")
	}
	before := str(p.config["faction"])
	for _, name := range []string{"FleetCarrier", "Fleet Carrier", "SquadronCarrier", "Construction Depot"} {
		if _, err := p.Search(context.Background(), M{"originSystem": "Origin", "faction": name}); err == nil {
			t.Fatalf("placeholder accepted: %s", name)
		}
		if str(p.config["faction"]) != before {
			t.Fatal("invalid faction mutated settings")
		}
	}
}
