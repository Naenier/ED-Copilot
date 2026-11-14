package presentation

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestDealsPageIncludesSystemsStationsBothLegsAndQuoteDetails(t *testing.T) {
	o := trading.RouteOptions(M{"credits": 100000, "cargoCapacity": 100, "roundTrip": true}, M{})
	market := func(id, system, station, key string, price float64) M {
		return M{"id": id, "system": system, "station": station, "pad": "L", "distanceLs": 400, "type": "Coriolis", "coordinates": []float64{0, 0, 0}, "updatedAt": "2026-11-08T12:00:00Z", "quotes": []M{{"key": key, "name": key, "price": price, "supply": 1000, "demand": 1000}}}
	}
	rows := trading.BuildRoutes(map[string]M{"gold": {"buy": []M{market("A", "Alpha", "Source", "gold", 100)}, "sell": []M{market("B", "Beta", "Buyer", "gold", 200)}}, "silver": {"buy": []M{market("B", "Beta", "Buyer", "silver", 100)}, "sell": []M{market("A", "Alpha", "Source", "silver", 200)}}}, o)
	s := M{"tradeRoutes": M{"results": rows}, "commander": M{"system": "Alpha", "cargoCapacity": 100, "cargoTons": 20}, "planWorkspace": M{"key": "owner"}}
	s["goods"] = trading.New(nil).Snapshot(nil)["goods"]
	before := v.Copy(s)
	page := DealsPage("", "", s)
	if len(page.Rows) != 2 || len(page.Columns) != 6 || !strings.Contains(page.Rows[0].Cells[0], "Alpha") || !strings.Contains(page.Rows[0].Cells[1], "Source") || !strings.Contains(page.Rows[0].Details, "Плечо 2") || !strings.Contains(page.Rows[0].Details, "Площадка") {
		t.Fatal("incomplete route presentation", page)
	}
	if len(DealsPage("", "not a match", s).Rows) != 0 {
		t.Fatal("unmatched route query returned unrelated records")
	}
	if !reflect.DeepEqual(before, v.Copy(s)) {
		t.Fatal("presentation changed market observations")
	}
	actions := DealsActions(s, rows[0])
	seen := map[string]Action{}
	for _, a := range actions {
		seen[a.ID] = a
	}
	if seen["deals.copy-buy.0"].Path != "Alpha" || seen["deals.copy-sell.0"].Path != "Beta" || seen["deals.copy-station-buy.0"].Path != "Source" || seen["deals.copy-buy.0"].Kind != "copy" {
		t.Fatal("system and station clipboard actions mixed")
	}
	if v.At(seen["deals.track"].Payload, "routeId") != rows[0]["id"] || seen["deals.save-route"].Path != "/api/trading/saved-routes" {
		t.Fatal("route tracking or full-route persistence command lost")
	}
	search := seen["deals.search"]
	values := M{}
	for _, f := range search.Fields {
		parsed, err := ParseField(f, f.Default)
		if err != nil {
			t.Fatal(f.Key, err)
		}
		values[f.Key] = parsed
	}
	body, err := search.Body(values)
	if err != nil {
		t.Fatal(err)
	}
	if v.Num(body["cargoCapacity"]) != 100 || len(search.Fields) != 15 || body["credits"] != nil || body["resourceKeys"] != nil || !v.Bool(body["roundTrip"]) {
		t.Fatal("unexpected route search fields", body)
	}
}
