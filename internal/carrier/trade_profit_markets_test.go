package carrier

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type bulkTradeFixture func(*http.Request) (*http.Response, error)

func (f bulkTradeFixture) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestBulkTradeMarketsSearchAndCombineSuppliers(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	origin := M{"system": "Sol", "coordinates": []float64{0, 0, 0}}
	station := func(id, name, system, kind string, x float64, at time.Time, buy, supply, sell, demand int) M {
		largePads := 1
		if id == "5" || id == "9" {
			largePads = 0
		}
		return M{"market_id": id, "name": name, "system_name": system, "type": kind,
			"system_x": x, "system_y": 0, "system_z": 0, "large_pads": largePads, "medium_pads": 1, "market_updated_at": v.ISO(at),
			"market": []M{{"commodity": "Beryllium", "buy_price": buy, "supply": supply, "sell_price": sell, "demand": demand}}}
	}
	var calls atomic.Int32
	client := bulkTradeFixture(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		var payload M
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			return nil, err
		}
		if req.Method != http.MethodPost || req.URL.Path != "/api/stations/search" || v.At(payload, "filters", "market") != nil {
			t.Error("bulk search should retrieve all goods through station search")
		}
		var rows []M
		count := 0
		switch v.Int(v.At(payload, "filters", "distance", "min")) {
		case 0:
			supply := station("1", "Supplier A", "Alpha", "Coriolis Starport", 30, now, 10000, 14000, 0, 0)
			rows = []M{
				supply, supply,
				station("2", "Supplier B", "Alpha", "Coriolis Starport", 30, now, 11000, 11000, 0, 0),
				station("3", "Insufficient isolated supply", "Beta", "Coriolis Starport", 40, now, 37000, 24000, 0, 0),
				station("4", "Excluded carrier", "Beta", "Drake-Class Carrier", 40, now, 1000, 50000, 0, 0),
				station("9", "Too small for the loading ship", "Beta", "Outpost", 40, now, 1000, 50000, 0, 0),
			}
			count = 1800
		case 80:
			rows = []M{
				station("5", "Buyer", "Gamma", "Coriolis Starport", 120, now, 0, 0, 38000, 50000),
				station("6", "Old quote", "Gamma", "Coriolis Starport", 120, now.Add(-48*time.Hour), 0, 0, 90000, 50000),
				station("7", "Insufficient demand", "Gamma", "Coriolis Starport", 120, now, 0, 0, 80000, 24000),
				station("8", "Low margin", "Gamma", "Coriolis Starport", 120, now, 0, 0, 12000, 50000),
			}
			count = len(rows)
		}
		body, err := json.Marshal(M{"results": rows, "count": count})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body))}, err
	})
	markets, limited, err := findBulkTradeMarkets(context.Background(), *trading.NewSearcher(client), origin, "M", 24, now)
	if err != nil || !limited || calls.Load() != 3 {
		t.Fatalf("bulk search calls=%d limited=%v err=%v", calls.Load(), limited, err)
	}
	books := markets["beryllium"]
	if len(books.buys) != 4 || len(books.sells) != 3 {
		t.Fatalf("bulk market indexing lost valid offers or kept invalid offers: %d buyers, %d sellers", len(books.buys), len(books.sells))
	}
	options, _ := tradeDefaults()
	options["haulerPad"], options["haulerCapacity"], options["tritiumPrice"] = "L", 750, 50000
	options["unloadHaulerPad"], options["unloadHaulerCapacity"] = "M", 400
	options["plannedFullLoad"] = true
	plan := tradeOpportunityInputs{options: options, origin: origin, system: "Sol", capacity: 25000, budget: 1e9}
	rows := plan.localCandidates(markets, nil, now)
	if len(rows) != 1 || rows[0]["commodityKey"] != "beryllium" || v.Num(rows[0]["quantity"]) != 25000 ||
		v.At(rows[0], "sell", "station") != "Buyer" || len(v.Rows(v.At(rows[0], "buy", "orders"))) != 2 ||
		v.Num(v.At(rows[0], "summary", "profit")) < netOpportunityMinimum {
		t.Fatalf("want a single full-load Beryllium trip with combined suppliers, got %v", rows)
	}
}

func TestBulkTradeMarketsCanFillUsingThreeSystems(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	station := func(id, name, system string, x float64) M {
		return M{"marketId": id, "station": name, "system": system, "coordinates": []float64{x, 0, 0}, "pad": "L", "updatedAt": v.ISO(now)}
	}
	options, _ := tradeDefaults()
	options["haulerPad"], options["haulerCapacity"], options["tritiumPrice"] = "L", 750, 50000
	options["unloadHaulerCapacity"] = 750
	options["plannedFullLoad"] = true
	origin := M{"system": "Sol", "coordinates": []float64{0, 0, 0}}
	plan := tradeOpportunityInputs{options: options, origin: origin, system: "Sol", capacity: 25000, budget: 1e9}
	markets := map[string]bulkTradeMarkets{"cobalt": {
		buys: []bulkTradeOffer{
			{station("1", "Supplier A", "Alpha", 30), 10000, 9000},
			{station("2", "Supplier B", "Beta", 40), 11000, 9000},
			{station("3", "Supplier C", "Gamma", 80), 12000, 9000},
			// One cannot buy and sell on the same market to complete a route.
			{station("4", "Buyer", "Delta", 100), 1, 9000},
		},
		sells: []bulkTradeOffer{{station("4", "Buyer", "Delta", 100), 38000, 50000}},
	}}
	rows := plan.localCandidates(markets, nil, now)
	if len(rows) == 0 {
		t.Fatal("no route found using all three pickup systems")
	}
	for _, row := range rows {
		orders := v.Rows(v.At(row, "buy", "orders"))
		hops := v.Rows(v.At(row, "route", "loaded"))
		if len(orders) != 3 || len(hops) != 3 || v.Num(row["quantity"]) != 25000 {
			t.Fatalf("want a full cargo hold and all three pickup systems, got %v", row)
		}
		for i, order := range orders {
			if order["marketId"] == "4" || order["system"] != hops[i]["from"] {
				t.Fatal("purchase orders do not match the travel order")
			}
		}
	}
}
