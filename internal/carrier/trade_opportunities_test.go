package carrier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	case "/api/fleetcarrier/route":
		hop := func(name string, id, x, distance, used, tank int) M {
			return M{"name": name, "id64": id, "x": x, "y": 0, "z": 0, "distance": distance,
				"fuel_used": used, "fuel_in_tank": tank, "tritium_in_market": 0, "restock_amount": 0}
		}
		reply = M{"status": "ok", "result": M{"fuel_loaded": 100,
			"jumps": []M{hop("Sol", 101, 0, 0, 0, 100), hop("Synthetic Transit", 102, 300, 300, 44, 56), hop(tradePeakSystem, 103, 600, 300, 43, 13)}}}
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
			rows = append(rows, market(2, "Synthetic Wine B", "Sol", 0, 120, 0, 10000, 0, fixture.at))
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
			"space": M{"capacityTons": 25000, "freeTons": 17000, "updatedAt": at},
			"fuel":  M{"tankTons": 1000, "reserveTons": 300, "updatedAt": at}, "warehouse": M{"updatedAt": at}},
	}
	return NewTradePlanner(nil).Snapshot(), state
}

func TestCarrierTradeOpportunitiesCompleteLocalTrip(t *testing.T) {
	trade, state := opportunityFixture()
	result, err := FindTradeOpportunities(context.Background(), opportunityMarkets{at: v.ISO(time.Now())}, trade, state, M{"scenario": "custom"}, nil)
	rows := v.Rows(result["results"])
	if err != nil || len(rows) != 1 {
		t.Fatalf("local route search failed: %v (%d matches, %v)", err, len(rows), result["status"])
	}
	row := rows[0]
	if v.Num(row["quantity"]) < 17000 || v.Num(v.At(row, "summary", "jumpCount")) != 2 ||
		v.Num(v.At(row, "summary", "fuelTons")) <= 0 || v.Num(v.At(row, "summary", "profit")) <= 0 ||
		v.Bool(row["estimatedCarrier"]) || v.At(row, "summary", "fuelFits") != true {
		t.Fatalf("local trade did not use carrier data and an empty return: quantity %v, summary %v", row["quantity"], row["summary"])
	}
	if v.At(row, "buy", "station") != "Synthetic Mine" || v.At(row, "sell", "station") != "Synthetic Buyer" {
		t.Fatal("station identity lost")
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
	if v.Num(best["quantity"]) != 17000 || len(orders) != 2 || v.Num(orders[0]["quantity"]) != 9000 ||
		v.Num(orders[1]["quantity"]) != 8000 || v.Num(best["purchaseCost"]) != 1860000 {
		t.Fatalf("purchase did not combine capacity and actual cost: %v", best["buy"])
	}
	if v.At(best, "summary", "profit") != nil || v.At(best, "sell", "price") != nil ||
		v.At(best, "options", "holidayConfirmed") != false || v.Num(v.At(best, "summary", "jumpCount")) != 4 {
		t.Fatalf("stale Peak quote or incorrect route: %v", best["summary"])
	}
	if len(v.Rows(v.At(best, "route", "return"))) != 2 || v.Num(v.At(best, "summary", "fuelCost")) <= 0 {
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
