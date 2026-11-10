package trading

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestRegionalRoutesUseAllMarketCommoditiesWithinPageBudget(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	station := func(id, system string, x float64, commodity M) M {
		return M{"market_id": id, "name": system + " port", "system_name": system, "type": "Coriolis",
			"has_large_pad": true, "system_x": x, "system_y": 0, "system_z": 0, "distance_to_arrival": 50,
			"market_updated_at": v.ISO(now), "market": []M{commodity}}
	}
	stations := []M{
		station("A", "Alpha", 0, M{"commodity": "Cobalt", "buy_price": 10, "supply": 100}),
		station("B", "Beta", 10, M{"commodity": "Cobalt", "sell_price": 50, "demand": 100}),
	}
	for _, tc := range []struct {
		name  string
		input M
		want  int
	}{
		{"default all", M{}, 1},
		{"disabled selection ignored", M{"resourceFilter": false, "resourceKeys": []string{"gold"}}, 1},
		{"multiple selected", M{"resourceFilter": true, "resourceKeys": []string{"gold", "cobalt"}}, 1},
		{"unselected excluded", M{"resourceFilter": true, "resourceKeys": []string{"gold"}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := doerFunc(func(request *http.Request) (*http.Response, error) {
				var body M
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if request.Method != "POST" || v.At(body, "filters", "market") != nil || v.Int(body["size"]) != regionalRoutePageSize || v.Int(body["page"]) != calls {
					t.Fatalf("unexpected market discovery request %v", body)
				}
				rows := make([]M, regionalRoutePageSize)
				for i := range rows {
					rows[i] = M{}
				}
				if calls == 0 {
					rows[0] = stations[0]
				}
				if calls == regionalRoutePages-1 {
					rows[0] = stations[1]
				}
				calls++
				return response(M{"count": 999999, "results": rows}), nil
			})
			search := NewSearcher(client)
			search.Now = func() time.Time { return now }
			out, err := search.FindRoutes(context.Background(), v.Merge(M{"cargoCapacity": 10, "credits": 10000}, tc.input), M{"commander": M{"system": "Alpha", "coordinates": []float64{0, 0, 0}}})
			rows := v.Rows(out["results"])
			if err != nil || len(rows) != tc.want || calls != regionalRoutePages || out["coverageLimited"] != true {
				t.Fatalf("regional search got %d routes, %d calls, %v coverage, %v error", len(rows), calls, out["coverageLimited"], err)
			}
			if tc.want > 0 {
				leg := v.Rows(rows[0]["legs"])[0]
				if leg["key"] != "cobalt" || v.At(leg, "buy", "system") != "Alpha" || v.At(leg, "sell", "system") != "Beta" {
					t.Fatalf("wrong regional route: %v", leg)
				}
			}
		})
	}
}

func TestRouteMinimumProfitUsesWholeTripAfterExpenses(t *testing.T) {
	markets := map[string]M{
		"gold":   {"buy": []M{detailsMarket("A", "A", "Source", "gold", 10, 100, 0, 0)}, "sell": []M{detailsMarket("B", "B", "Buyer", "gold", 60, 0, 100, 10)}},
		"silver": {"buy": []M{detailsMarket("B", "B", "Source", "silver", 20, 100, 0, 10)}, "sell": []M{detailsMarket("A", "A", "Buyer", "silver", 70, 0, 100, 0)}},
	}
	options := RouteOptions(M{"cargoCapacity": 10, "credits": 1000, "operatingCostPerLeg": 100, "minNetProfit": 800}, nil)
	if rows := BuildRoutes(markets, options); len(rows) != 0 {
		t.Fatalf("single leg below the minimum profit: %d routes", len(rows))
	}
	options["roundTrip"] = true
	if rows := BuildRoutes(markets, options); len(rows) != 2 || v.Num(rows[0]["netEstimate"]) != 800 {
		t.Fatalf("round-trip profit equals the threshold: %v", rows)
	}
	options["minNetProfit"] = 801
	if rows := BuildRoutes(markets, options); len(rows) != 0 {
		t.Fatalf("round-trip gross spread should not qualify: %d routes", len(rows))
	}
}
