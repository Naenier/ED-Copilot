package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/edsm"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func galaxyFixture(t *testing.T, transport economyRoundTripper) *Engine {
	t.Helper()
	client := &http.Client{Transport: transport}
	return &Engine{ctx: t.Context(), opts: Options{HTTPClient: client}, EDSM: edsm.NewClient(client)}
}

func galaxyHTTP(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestGalaxyProjectsEDSMWithoutFillingMissingFacts(t *testing.T) {
	engine := galaxyFixture(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "www.edsm.net" || request.URL.Query().Get("systemName") != "Pilot's Test" {
			t.Errorf("unexpected request %s", request.URL)
		}
		switch request.URL.Path {
		case "/api-v1/system":
			return galaxyHTTP(200, `{"name":"Pilot's Test","id":9,"id64":18446744073709551615,"coords":{"x":0,"y":-2,"z":3},"requirePermit":false,"information":{"population":0},"primaryStar":{"type":"G","isScoopable":true}}`), nil
		case "/api-system-v1/bodies":
			return galaxyHTTP(200, `{"name":"Pilot's Test","bodies":[{"name":"Test A 1","type":"Planet","gravity":0.4,"isLandable":true,"materials":{"Carbon":10.5},"rings":[{"name":"A ring","type":"Icy","innerRadius":19}]}]}`), nil
		case "/api-system-v1/stations":
			return galaxyHTTP(200, `{"name":"Pilot's Test","stations":[{"name":"First Station","body":{"name":"Test A 1"},"haveMarket":false,"otherServices":["Refuel"],"updateTime":{"information":"2026-11-09 09:00:00"}}]}`), nil
		case "/api-system-v1/factions":
			return galaxyHTTP(200, `{"name":"Pilot's Test","factions":[{"name":"None Found","influence":0},{"name":"Testing Group","influence":0.75,"activeStates":[{"state":"Boom"}],"lastUpdate":7218038504}]}`), nil
		default:
			t.Fatalf("unexpected API path %s", request.URL.Path)
			return nil, context.Canceled
		}
	})
	load := func(section string) M {
		t.Helper()
		result := engine.Galaxy("galaxy.system", M{"name": " Pilot's Test ", "section": section})
		if result["status"] != "ok" {
			t.Fatalf("%s: %#v", section, result)
		}
		return result
	}
	overview := v.Obj(load("summary")["system"])
	if overview["address"] != "18446744073709551615" || overview["x"] != 0.0 || overview["population"] != 0.0 || overview["permitRequired"] != false || overview["government"] != nil {
		t.Fatalf("overview %#v", overview)
	}
	body := v.Rows(load("bodies")["rows"])[0]
	if body["landable"] != true || body["temperature"] != nil || v.Rows(body["materials"])[0]["percent"] != 10.5 || v.Rows(body["rings"])[0]["type"] != "Icy" {
		t.Fatalf("body %#v", body)
	}
	station := v.Rows(load("stations")["rows"])[0]
	if station["market"] != false || station["shipyard"] != nil || station["body"] != "Test A 1" || station["updatedAt"] != "2026-11-09T09:00:00Z" {
		t.Fatalf("station %#v", station)
	}
	faction := v.Rows(load("factions")["rows"])[0]
	if faction["name"] != "Testing Group" || faction["influence"] != 75.0 || faction["updatedAt"] != nil || v.Arr(faction["activeStates"])[0] != "Boom" {
		t.Fatalf("faction %#v", faction)
	}
}

func TestGalaxyNearbyFallbackKeepsSourceDistanceAndCoverage(t *testing.T) {
	engine := galaxyFixture(t, func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case "www.edsm.net":
			if request.URL.Path == "/api-v1/sphere-systems" {
				if request.URL.Query().Get("showInformation") != "1" {
					t.Error("nearby omitted summary metadata")
				}
				return galaxyHTTP(503, "{}"), nil
			}
			return galaxyHTTP(200, `{"name":"Sol","coords":{"x":0,"y":0,"z":0}}`), nil
		case "spansh.co.uk":
			var data M
			if request.Method != http.MethodPost || request.URL.Path != "/api/systems/search" || json.NewDecoder(request.Body).Decode(&data) != nil || v.At(data, "filters", "distance", "max") != 12.0 {
				t.Errorf("unexpected nearby fallback %s %#v", request.URL, data)
			}
			return galaxyHTTP(200, `{"count":81,"results":[{"name":"Nearby","id64":18446744073709551615,"x":3,"y":4,"z":0,"population":7},{"name":"Too far","id64":2,"x":13,"y":0,"z":0}]}`), nil
		default:
			t.Fatalf("unexpected host %s", request.URL.Host)
			return nil, context.Canceled
		}
	})
	if bad := engine.Galaxy("galaxy.nearby", M{"name": "Sol", "radius": 100.5}); bad["status"] != "invalid" {
		t.Fatalf("invalid radius %#v", bad)
	}
	result := engine.Galaxy("galaxy.nearby", M{"name": "Sol", "radius": 12})
	rows := v.Rows(result["rows"])
	if result["status"] != "ok" || result["source"] != "Spansh" || result["truncated"] != true || result["total"] != 81.0 || len(rows) != 1 {
		t.Fatalf("fallback %#v", result)
	}
	if rows[0]["name"] != "Nearby" || rows[0]["distance"] != 5.0 || rows[0]["address"] != "18446744073709551615" {
		t.Fatalf("nearby %#v", rows[0])
	}
}
