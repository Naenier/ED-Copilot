package carrier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type opportunityMarkets struct {
	at        string
	freshPeak bool
}

func (fixture opportunityMarkets) Do(req *http.Request) (*http.Response, error) {
	reply := M{}
	switch req.URL.Path {
	case "/api/search/systems":
		system := req.URL.Query().Get("q")
		x, id := 0, 101
		if system == tradePeakSystem {
			x, id = 600, 103
		}
		reply["results"] = []M{{"name": system, "id64": id, "x": x, "y": 0, "z": 0}}
	case "/api/station/" + tradePeakMarket:
		// A real price can exist in an old observation. It must not
		// appear as present-day revenue.
		at := fixture.at
		if !fixture.freshPeak {
			at = v.ISO(v.Time(fixture.at).Add(-50 * time.Hour))
		}
		reply = M{"record": M{"name": tradePeakPort, "system_name": tradePeakSystem, "market_id": tradePeakMarket,
			"market_updated_at": at, "market": []M{{"commodity": "Water", "sell_price": 2, "demand": 50000},
				{"commodity": "Wine", "sell_price": 250000, "demand": 50000}, {"commodity": "Liquor", "sell_price": 40000, "demand": 50000}}}}
	case "/api/stations/search":
		var query M
		if err := json.NewDecoder(req.Body).Decode(&query); err != nil {
			return nil, err
		}
		filter := v.Rows(v.At(query, "filters", "market"))[0]
		key, buy := v.Key(filter["name"]), filter["supply"] != nil
		market := func(id int, station, system string, x int, buyPrice, sellPrice, supply, demand int, at string) M {
			return M{"name": station, "system_name": system, "system_x": x, "system_y": 0, "system_z": 0,
				"market_id": id, "market_updated_at": at, "has_large_pad": true, "distance_to_arrival": 123,
				"market": []M{{"symbol": key, "buy_price": buyPrice, "sell_price": sellPrice, "supply": supply, "demand": demand}}}
		}
		rows := []M{}
		switch {
		case key == "wine" && buy:
			rows = append(rows, market(1, "Synthetic Wine A", "Sol", 0, 100, 0, 9000, 0, fixture.at))
			rows = append(rows, market(2, "Synthetic Wine B", "Sol", 0, 120, 0, 17000, 0, fixture.at))
		case key == "liquor" && buy && fixture.freshPeak:
			rows = append(rows, market(7, "Synthetic Liquor", "Sol", 0, 550, 0, 20000, 0, fixture.at))
		case key == "tritium":
			rows = append(rows, market(4, "Synthetic Fuel", "Sol", 0, 40000, 0, 10000, 0, fixture.at))
		case key == "gold" && buy:
			rows = append(rows, market(5, "Synthetic Mine", "Sol", 0, 4000, 0, 30000, 0, fixture.at))
		case key == "gold":
			rows = append(rows, market(6, "Synthetic Buyer", "Synthetic Trade", 100, 0, 19000, 0, 25000, fixture.at))
		}
		reply = M{"count": len(rows), "results": rows}
	default:
		return nil, fmt.Errorf("unexpected request %s", req.URL.Path)
	}
	data, err := json.Marshal(reply)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
}

func opportunityFixture() (M, M) {
	at := v.ISO(time.Now())
	state := M{
		"commander": M{"system": "Sol", "coordinates": []float64{0, 0, 0}, "ship": "Python", "cargoCapacity": 400},
		"pilot":     M{"credits": 2e9},
		"carrier": M{"owned": true, "location": M{"system": "Sol", "coordinates": []float64{0, 0, 0}},
			"space": M{"capacityTons": 25000, "freeTons": 17000, "crewTons": 900, "modulePacksTons": 100, "updatedAt": at},
			"fuel":  M{"tankTons": 1000, "reserveTons": 300, "updatedAt": at}, "warehouse": M{"updatedAt": at}},
	}
	return NewTradePlanner(nil).Snapshot(), state
}

func TestCarrierTradeOnlyPlansPeakAndIgnoresLiveCargoAndFuel(t *testing.T) {
	trade, state := opportunityFixture()
	request := M{"scenario": "custom"}
	contextBefore := TradeOpportunityContext(trade, state, request)
	v.Obj(v.At(state, "carrier", "space"))["freeTons"] = 5
	v.Obj(v.At(state, "carrier", "space"))["cargoTons"] = 23995
	v.Obj(v.At(state, "carrier", "fuel"))["tankTons"] = 2
	v.Obj(v.At(state, "carrier", "fuel"))["reserveTons"] = 2000
	options := v.Obj(trade["options"])
	options["otherCargoTons"], options["cargoTritiumTons"], options["arrivalReserveTons"] = 10000, 7000, 500
	if !reflect.DeepEqual(contextBefore, TradeOpportunityContext(trade, state, request)) {
		t.Fatal("unrelated inventory or legacy fuel settings changed the search")
	}
	result, err := FindTradeOpportunities(context.Background(), opportunityMarkets{at: v.ISO(time.Now())}, trade, state, request, nil)
	rows := v.Rows(result["results"])
	if err != nil || len(rows) == 0 {
		t.Fatalf("Peak route search failed: %v (%d matches, %v)", err, len(rows), result["status"])
	}
	row := rows[0]
	summary := v.Obj(row["summary"])
	fuel := v.Num(summary["fuelTons"])
	reserve := v.Num(v.At(row, "options", "arrivalReserveTons"))
	hold := v.Num(summary["cargoTritiumTons"])
	if reserve != math.Ceil(fuel/10) || v.Num(row["fuelPurchaseTons"]) != fuel+reserve || v.Num(summary["servicesTons"]) != 1000 ||
		v.Num(summary["availableCargoTons"]) != 25000-1000-hold || v.Num(row["quantity"]) != v.Num(summary["availableCargoTons"]) ||
		v.Num(summary["jumpCount"]) != 36 || summary["fuelFits"] != true {
		t.Fatalf("unexpected full-load calculation: %v", row)
	}
	if v.At(result, "options", "scenario") != tradePeakScenario || row["commodityKey"] != "wine" || v.At(row, "sell", "station") != tradePeakPort {
		t.Fatal("legacy ordinary search did not resolve to a Peak trade")
	}
}

func TestCarrierTradeBoozeCruiseCombinesSuppliersAndOmitsStaleProfit(t *testing.T) {
	trade, state := opportunityFixture()
	result, err := FindTradeOpportunities(context.Background(), opportunityMarkets{at: v.ISO(time.Now())}, trade, state, M{"scenario": "booze-cruise"}, nil)
	rows := v.Rows(result["results"])
	if err != nil || len(rows) < 3 {
		t.Fatalf("missing purchase alternatives: %v (%d matches, %v)", err, len(rows), result["status"])
	}
	best := rows[0]
	orders := v.Rows(v.At(best, "buy", "orders"))
	quantity := v.Num(best["quantity"])
	if quantity <= 17000 || len(orders) != 2 || v.Num(orders[0]["quantity"]) != 9000 ||
		v.Num(orders[1]["quantity"]) != quantity-9000 || v.Num(best["purchaseCost"]) != 900000+(quantity-9000)*120 {
		t.Fatalf("purchase did not combine capacity and actual cost: %v", best["buy"])
	}
	if v.At(best, "summary", "profit") != nil || v.At(best, "sell", "price") != nil ||
		v.At(best, "options", "holidayConfirmed") != false || v.Num(v.At(best, "summary", "jumpCount")) != 36 {
		t.Fatalf("stale Peak quote or incorrect route: %v", best["summary"])
	}
	if len(v.Rows(v.At(best, "route", "return"))) != 18 || v.Num(v.At(best, "summary", "fuelCost")) <= 0 {
		t.Fatal("route does not include the fuel cost of the empty return")
	}
}

func TestCarrierTradeBoozeCruiseUsesDistinctFreshSaleQuotes(t *testing.T) {
	trade, state := opportunityFixture()
	result, err := FindTradeOpportunities(context.Background(), opportunityMarkets{at: v.ISO(time.Now()), freshPeak: true}, trade, state, M{"scenario": "booze-cruise"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"wine": 250000, "liquor": 40000}
	seen := map[string]bool{}
	for _, row := range v.Rows(result["results"]) {
		key := v.Str(row["commodityKey"])
		if v.Num(v.At(row, "sell", "price")) != want[key] || v.Num(v.At(row, "summary", "profit")) <= 0 {
			t.Fatalf("incorrect %s sale %v", key, row["sell"])
		}
		seen[key] = true
	}
	if !seen["wine"] || !seen["liquor"] {
		t.Fatalf("missing fresh quotes %v", seen)
	}
}
