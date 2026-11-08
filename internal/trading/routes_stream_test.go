package trading

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestFindRoutesProgressNeverPublishesChildMarketPages(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	pageCalls := 0
	client := doerFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != "POST" {
			t.Fatalf("reference coordinates should be known: %s", request.URL)
			return nil, nil
		}
		var body M
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		commodity := v.Rows(v.At(body, "filters", "market"))[0]
		if v.Key(commodity["name"]) != "gold" {
			return &http.Response{StatusCode: 400, Body: response(M{}).Body, Header: http.Header{}}, nil
		}
		pageCalls++
		buy := commodity["supply"] != nil
		side := "sell"
		if buy {
			side = "buy"
		}
		page := v.Int(body["page"])
		first, last := 0, 60
		if page == 1 {
			first, last = 60, 61
		}
		stations := []M{}
		for i := first; i < last; i++ {
			x := float64(i + 1)
			if !buy {
				x += 100
			}
			stations = append(stations, M{"market_id": fmt.Sprintf("%s-%03d", side, i), "name": fmt.Sprintf("Synthetic %s %03d", side, i), "system_name": fmt.Sprintf("Synthetic system %s %03d", side, i), "type": "Coriolis", "has_large_pad": true, "system_x": x, "system_y": 0, "system_z": 0, "distance_to_arrival": 100, "market_updated_at": v.ISO(now), "market": []M{{"commodity": "Gold", "buy_price": 100, "sell_price": 200, "supply": 1000, "demand": 1000}}})
		}
		return response(M{"count": 61, "results": stations}), nil
	})
	search := NewSearcher(client)
	search.Now = func() time.Time { return now }
	updates := []M{}
	search.OnUpdate = func(state M) { updates = append(updates, v.Copy(state)) }
	result, err := search.FindRoutes(context.Background(), M{"system": "Synthetic Origin", "resourceKeys": []string{"gold", "silver", "polonium"}, "cargoCapacity": 100, "credits": 100000, "radiusLy": 500, "maxLegLy": 500}, M{"commander": M{"system": "Synthetic Origin", "coordinates": []float64{0, 0, 0}}})
	if err != nil {
		t.Fatal(err)
	}
	if pageCalls != 4 {
		t.Fatal("expected two station pages per gold side", pageCalls)
	}
	if len(updates) != 7 {
		t.Fatal("expected initial state and six completed subsearches, without child station publications", len(updates))
	}
	for index, state := range updates {
		if v.Int(state["checked"]) != index || v.Int(state["total"]) != 6 || v.Int(v.At(state, "progress", "checked")) != index {
			t.Fatal("route progress missing or not monotonic", index, state["checked"], state["total"])
		}
		for _, route := range v.Rows(state["results"]) {
			legs := v.Rows(route["legs"])
			if len(legs) < 1 || len(legs) > 2 || v.Str(v.At(legs[0], "buy", "system")) == "" || route["station"] != nil {
				t.Fatal("raw market escaped into route stream", index, route)
			}
		}
		if len(v.Rows(state["results"])) > 60 {
			t.Fatal("route cap lost during partial update", index)
		}
	}
	if len(v.Rows(result["results"])) != 60 || v.Int(result["checked"]) != 6 || !strings.Contains(v.Str(result["error"]), "400") || !strings.Contains(strings.ToLower(v.Str(result["error"])), "сырь") {
		t.Fatal("partial failures lost final route DTO or completion", result["checked"], len(v.Rows(result["results"])), result["error"])
	}
}
