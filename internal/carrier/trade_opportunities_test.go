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
	mixedPeak bool
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
		filters := v.Rows(v.At(query, "filters", "market"))
		if len(filters) == 0 {
			reply = M{"count": 0, "results": []M{}}
			break
		}
		filter := filters[0]
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
			secondSystem, x := "Sol", 0
			if fixture.mixedPeak {
				secondSystem, x = "Synthetic Supplier", 25
			}
			rows = append(rows, market(2, "Synthetic Wine B", secondSystem, x, 120, 0, 17000, 0, fixture.at))
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

func TestCarrierTradeProfitSearchIgnoresLiveCargoAndFuel(t *testing.T) {
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
	result, err := FindTradeOpportunities(context.Background(), opportunityMarkets{at: v.ISO(time.Now()), freshPeak: true}, trade, state, request, nil)
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
	if v.At(result, "options", "scenario") != TradeOpportunityScenario || row["commodityKey"] != "wine" || v.At(row, "sell", "station") != tradePeakPort ||
		v.Num(summary["profit"]) < netOpportunityMinimum {
		t.Fatal("search did not return the fully funded Peak trade")
	}
}

func TestCarrierTradeBoozeCruiseCombinesSuppliersAndOmitsStaleProfit(t *testing.T) {
	trade, state := opportunityFixture()
	result, err := FindTradeOpportunities(context.Background(), opportunityMarkets{at: v.ISO(time.Now()), freshPeak: true}, trade, state, M{"scenario": "booze-cruise"}, nil)
	rows := v.Rows(result["results"])
	if err != nil || len(rows) == 0 {
		t.Fatalf("missing purchase alternatives: %v (%d matches, %v)", err, len(rows), result["status"])
	}
	best := rows[0]
	orders := v.Rows(v.At(best, "buy", "orders"))
	quantity := v.Num(best["quantity"])
	if quantity <= 17000 || len(orders) != 2 || v.Num(orders[0]["quantity"]) != 9000 ||
		v.Num(orders[1]["quantity"]) != quantity-9000 || v.Num(best["purchaseCost"]) != 900000+(quantity-9000)*120 {
		t.Fatalf("purchase did not combine capacity and actual cost: %v", best["buy"])
	}
	if v.At(best, "options", "holidayConfirmed") != false || v.Num(v.At(best, "summary", "jumpCount")) != 36 {
		t.Fatalf("incorrect route: %v", best["summary"])
	}
	if len(v.Rows(v.At(best, "route", "return"))) != 18 || v.Num(v.At(best, "summary", "fuelCost")) <= 0 {
		t.Fatal("route does not include the fuel cost of the empty return")
	}
	stale, err := FindTradeOpportunities(context.Background(), opportunityMarkets{at: v.ISO(time.Now())}, trade, state, nil, nil)
	if err != nil || len(v.Rows(stale["results"])) != 0 {
		t.Fatalf("stale sale price produced a route: %v, %v", stale, err)
	}
}

func TestCarrierTradeBoozeCruiseCombinesSystemsAndEstimatesEachShuttle(t *testing.T) {
	trade, state := opportunityFixture()
	options := v.Obj(trade["options"])
	options["haulerPad"], options["haulerCapacity"] = "L", 1300
	result, err := FindTradeOpportunities(context.Background(), opportunityMarkets{at: v.ISO(time.Now()), freshPeak: true, mixedPeak: true}, trade, state, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	foundMixed := false
	for _, row := range v.Rows(result["results"]) {
		if row["commodityKey"] == "liquor" {
			t.Fatal("a fresh high price does not qualify if its supplier cannot fill the hold")
		}
		orders := v.Rows(v.At(row, "buy", "orders"))
		if len(orders) != 2 || orders[0]["system"] == orders[1]["system"] {
			continue
		}
		foundMixed = true
		quantity := v.Num(row["quantity"])
		summary := v.Obj(row["summary"])
		loading := math.Ceil(v.Num(orders[0]["quantity"])/1300) + math.Ceil(v.Num(orders[1]["quantity"])/1300)
		unloading := math.Ceil(quantity / 400)
		if v.Num(summary["loadingSorties"]) != loading || v.Num(summary["unloadingSorties"]) != unloading ||
			v.Num(summary["jumpCount"]) <= 36 || v.Num(summary["profit"]) < netOpportunityMinimum {
			t.Fatalf("mixed route or shuttle estimate is wrong: %v", summary)
		}
		total := v.Num(summary["travelHours"]) + v.Num(summary["loadingHours"]) +
			v.Num(summary["unloadingHours"]) + v.Num(summary["fuelLoadingHours"])
		if math.Abs(total-v.Num(summary["workingHours"])) > .000001 {
			t.Fatalf("working time excludes a route stage: %v", summary)
		}
	}
	if !foundMixed {
		t.Fatal("no fully loaded route using both systems")
	}
}

func TestCarrierTradeRequiresFullLoadAndMinimumNetProfit(t *testing.T) {
	trade, state := opportunityFixture()
	plan, err := newTradeOpportunityInputs(trade, state, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan.options["durationHours"] = 0
	now := time.Now()
	quantity := plan.capacity - plan.services
	buy := M{"system": "Sol", "station": "Supplier", "price": 1, "supply": quantity, "pad": "L"}
	sell := M{"system": "Sol", "station": "Buyer", "price": 1 + float64(netOpportunityMinimum)/quantity, "demand": 30000, "pad": "L"}
	find := func() M {
		supply := v.Num(buy["supply"])
		return plan.candidate("gold", []M{buy}, sell, supply, supply, nil, nil, nil, nil, now)
	}
	if row := find(); v.Num(v.At(row, "summary", "profit")) != netOpportunityMinimum {
		t.Fatalf("route at the minimum net profit was excluded: %v", row)
	}
	plan.options["extraCosts"] = 1
	if find() != nil {
		t.Fatal("gross margin passed even though net profit was under the threshold")
	}
	plan.options["extraCosts"] = 0
	sell["price"] = 500000
	buy["supply"] = quantity - 1
	if find() != nil {
		t.Fatal("profitable partial load was included")
	}
	buy["supply"] = quantity
	plan.budget = quantity - 1
	if find() != nil {
		t.Fatal("unfunded full load was included")
	}
}
