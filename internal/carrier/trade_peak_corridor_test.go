package carrier

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTradePeakCorridorSolAndDirectJoin(t *testing.T) {
	ctx := context.Background()
	sol := knownTradePeakOrigin(" SOL ")
	hops, err := tradePeakCorridor(ctx, nil, sol)
	if err != nil || len(hops) != 18 {
		t.Fatalf("Sol corridor: %d hops, %v", len(hops), err)
	}
	if hops[0].from != "Sol" || hops[0].to != "Wregoe MC-Y b44-0" || hops[len(hops)-1].to != tradePeakSystem {
		t.Fatalf("unexpected corridor endpoints: %+v", hops)
	}
	for i, hop := range hops {
		if hop.distance <= 0 || hop.distance > tradePeakJumpLimit || i > 0 && hops[i-1].to != hop.from {
			t.Fatalf("invalid corridor hop %d: %+v", i, hop)
		}
	}

	join := M{"system": "Synthetic Bubble Market", "coordinates": []float64{0, 10, 0}}
	connected, ok := directTradePeakCorridor(join)
	if !ok || len(connected) != len(hops) || connected[0].from != join["system"] || connected[0].to != hops[0].to || connected[1] != hops[1] {
		t.Fatalf("nearby market did not join the first reachable stop: %+v", connected)
	}
	suffix, err := tradePeakCorridor(ctx, nil, M{"system": "HD 104495"})
	if err != nil || len(suffix) != 2 || suffix[0].to != "HD 105341" || suffix[1].to != tradePeakSystem {
		t.Fatalf("unexpected corridor suffix: %+v, %v", suffix, err)
	}
	atPeak, err := tradePeakCorridor(ctx, nil, M{"system": tradePeakSystem})
	if err != nil || len(atPeak) != 0 {
		t.Fatalf("origin already at the Peak: %+v, %v", atPeak, err)
	}
}

type tradePeakConnectorFixture struct {
	t        *testing.T
	requests int
}

func (fixture *tradePeakConnectorFixture) Do(req *http.Request) (*http.Response, error) {
	fixture.t.Helper()
	fixture.requests++
	if err := req.ParseForm(); err != nil {
		fixture.t.Fatal(err)
	}
	if req.Method != http.MethodPost || req.URL.Path != "/api/fleetcarrier/route" ||
		req.Form.Get("source") != "77" || len(req.Form["destinations"]) != 1 || req.Form.Get("destinations") != "10477373803" {
		fixture.t.Fatalf("provider requested more than the Sol connector: %s %s %v", req.Method, req.URL.Path, req.Form)
	}
	const response = `{"status":"ok","result":{"fuel_loaded":100,"jumps":[
		{"name":"Synthetic Origin","id64":"77","x":0,"y":-650,"z":0,"distance":0,"fuel_used":0,"fuel_in_tank":100,"tritium_in_market":0,"restock_amount":0},
		{"name":"Synthetic Connector","id64":"78","x":0,"y":-300,"z":0,"distance":350,"fuel_used":45,"fuel_in_tank":55,"tritium_in_market":0,"restock_amount":0},
		{"name":"Sol","id64":"10477373803","x":0,"y":0,"z":0,"distance":300,"fuel_used":43,"fuel_in_tank":12,"tritium_in_market":0,"restock_amount":0}
	]}}`
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(response))}, nil
}

func TestTradePeakCorridorOnlyRequestsShortConnector(t *testing.T) {
	route := NewRoutePlanner(nil)
	fixture := &tradePeakConnectorFixture{t: t}
	route.Client = fixture
	origin := M{"system": "Synthetic Origin", "systemAddress": "77", "coordinates": []float64{0, -650, 0}}
	hops, err := tradePeakCorridor(context.Background(), route, origin)
	if err != nil || len(hops) != 20 || fixture.requests != 1 {
		t.Fatalf("connector: %d hops, %d requests, %v", len(hops), fixture.requests, err)
	}
	if hops[0] != (tradeHop{"Synthetic Origin", "Synthetic Connector", 350}) ||
		hops[1] != (tradeHop{"Synthetic Connector", "Sol", 300}) ||
		hops[2].to != "Wregoe MC-Y b44-0" || hops[len(hops)-1].to != tradePeakSystem {
		t.Fatalf("unexpected corridor: %+v", hops)
	}
	origin["coordinates"] = []float64{0, -5000, 0}
	if _, err = tradePeakCorridor(context.Background(), route, origin); err == nil || fixture.requests != 1 {
		t.Fatalf("overlong connector was requested: %d requests, %v", fixture.requests, err)
	}
}
