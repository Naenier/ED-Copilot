package trading

import (
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

var tradeEpoch = v.Time("2026-11-14T10:00:00Z")

func liveRouteFixture(roundTrip bool) M {
	a := M{"system": "Alpha", "systemAddress": "10", "station": "Alpha Port", "marketId": "101"}
	b := M{"system": "Beta", "systemAddress": "20", "station": "Beta Port", "marketId": "202"}
	legs := []M{{"buy": a, "sell": b, "key": "indite", "name": "Индит", "quantity": 10, "buyPrice": 10, "sellPrice": 30}}
	if roundTrip {
		legs = append(legs, M{"buy": b, "sell": a, "key": "bauxite", "name": "Боксит", "quantity": 10, "buyPrice": 8, "sellPrice": 18})
	}
	return M{"id": "test-route", "legs": legs}
}

func liveJournal(kind string, offset int, values M) M {
	return v.Merge(M{"event": kind, "timestamp": v.ISO(tradeEpoch.Add(time.Duration(offset) * time.Second))}, values)
}

func livePoint(side string, station bool) M {
	if side == "A" {
		if station {
			return M{"StarSystem": "Alpha", "SystemAddress": "10", "StationName": "Alpha Port", "MarketID": "101"}
		}
		return M{"StarSystem": "Alpha", "SystemAddress": "10"}
	}
	if station {
		return M{"StarSystem": "Beta", "SystemAddress": "20", "StationName": "Beta Port", "MarketID": "202"}
	}
	return M{"StarSystem": "Beta", "SystemAddress": "20"}
}

func TestLiveTradeRouteCopiesOnArrivalAndMeasuresClockOnExactDocking(t *testing.T) {
	store := NewLiveRoutes(nil)
	if copy, err := store.Select("pilot", liveRouteFixture(true), M{"docked": true, "system": "Alpha", "station": "Alpha Port", "marketId": "101"}, tradeEpoch); err != nil || copy != "Beta" {
		t.Fatalf("initial destination = %q, %v", copy, err)
	}
	view := store.Snapshot("pilot")
	if v.At(view, "target", "system") != "Beta" || v.At(view, "atStation", "buy", "key") != "indite" || view["profitPerHour"] != nil {
		t.Fatalf("unexpected initial projection: %v", view)
	}
	apply := func(kind string, seconds int, values M) string {
		return store.Observe("pilot", liveJournal(kind, seconds, values), M{})
	}
	// Two fills with the same second must both count, yet replaying either
	// journal identity must never duplicate the fill.
	buy := v.Merge(livePoint("A", true), M{"Type": "Indite", "Count": 5, "TotalCost": 50})
	buyEvent := liveJournal("MarketBuy", 1, buy)
	for _, offset := range []int{104, 109, 104} {
		store.Observe("pilot", buyEvent, M{"file": "Journal.test.01.log", "offset": offset})
	}
	if value := apply("Undocked", 10, livePoint("A", true)); value != "" {
		t.Fatalf("unexpected copy on origin departure: %q", value)
	}
	if value := apply("FSDJump", 60, livePoint("B", false)); value != "Alpha" {
		t.Fatalf("reverse clipboard = %q", value)
	}
	if value := apply("FSDJump", 60, livePoint("B", false)); value != "" {
		t.Fatalf("duplicate jump resubmitted clipboard: %q", value)
	}
	wrong := v.Merge(livePoint("B", true), M{"MarketID": "303", "StationName": "Wrong Port"})
	if value := apply("Docked", 80, wrong); value != "" || store.Snapshot("pilot")["phase"] != "outbound" {
		t.Fatalf("incorrect market advanced the route: %v, %q", store.Snapshot("pilot"), value)
	}
	if value := apply("Docked", 90, livePoint("B", true)); value != "" {
		t.Fatalf("docking duplicated reverse clipboard: %q", value)
	}
	view = store.Snapshot("pilot")
	if v.At(view, "target", "system") != "Alpha" || v.At(view, "atStation", "buy", "key") != "bauxite" ||
		v.At(view, "atStation", "sell", "key") != "indite" || view["profitPerHour"] != nil {
		t.Fatalf("destination station actions were lost: %v", view)
	}
	apply("MarketSell", 91, v.Merge(livePoint("B", true), M{"Type": "$Indite_Name;", "Count": 10, "SellPrice": 32}))
	apply("MarketBuy", 92, v.Merge(livePoint("B", true), M{"Type": "Bauxite", "Count": 8, "TotalCost": 80}))
	apply("Undocked", 100, livePoint("B", true))
	apply("FSDJump", 300, livePoint("A", false))
	if view = store.Snapshot("pilot"); view["profitPerHour"] != nil {
		t.Fatalf("system entry completed the lap too soon: %v", view)
	}
	if value := apply("Docked", 310, livePoint("A", true)); value != "" {
		t.Fatalf("docking duplicated outward clipboard: %q", value)
	}
	view = store.Snapshot("pilot")
	if view["completedRounds"] != 1 || view["lastRoundSeconds"] != 300.0 || view["profitPerHour"] != 3600.0 ||
		view["actualProfitPerHour"] != nil || v.At(view, "atStation", "sell", "key") != "bauxite" {
		t.Fatalf("incorrect timed lap: %v", view)
	}
	apply("MarketSell", 311, v.Merge(livePoint("A", true), M{"Type": "Bauxite", "Count": 8, "TotalSale": 200}))
	view = store.Snapshot("pilot")
	if view["actualRoundProfit"] != 340.0 || view["actualProfitPerHour"] != 4080.0 {
		t.Fatalf("actual settled margin: %v", view)
	}
	restored := NewLiveRoutes(store.Export())
	if other := restored.Snapshot("another-pilot"); other != nil || restored.Snapshot("pilot")["actualRoundProfit"] != 340.0 {
		t.Fatalf("profile did not scope or retain measured route: %v", restored.Export())
	}
	if value := restored.Observe("pilot", liveJournal("MarketSell", 311, v.Merge(livePoint("A", true),
		M{"Type": "Bauxite", "Count": 8, "TotalSale": 200})), M{"replay": true}); value != "" || restored.Snapshot("pilot")["actualRoundProfit"] != 340.0 {
		t.Fatal("replayed the persisted sale")
	}
}

func TestLiveRouteReplayDoesNotFabricateFirstRound(t *testing.T) {
	store := NewLiveRoutes(nil)
	apply := func(kind string, offset int, values M, replay bool) string {
		return store.Observe("pilot", liveJournal(kind, offset, values), M{"replay": replay})
	}
	if _, err := store.Select("pilot", liveRouteFixture(true), nil, tradeEpoch); err != nil {
		t.Fatal(err)
	}
	if text := apply("Docked", 15, livePoint("B", true), true); text != "" || store.Snapshot("pilot")["phase"] != "return" {
		t.Fatalf("replay failed to catch up silently: %q", text)
	}
	apply("Docked", 60, livePoint("A", true), true)
	view := store.Snapshot("pilot")
	if view["completedRounds"] != 0 || view["lastRoundSeconds"] != nil || v.At(view, "atStation", "sell", "key") != "bauxite" {
		t.Fatalf("a missing departure was considered a recorded lap: %v", view)
	}
}

func TestLiveRouteMeasuresAnUnloadedReturn(t *testing.T) {
	store := NewLiveRoutes(nil)
	if text, err := store.Select("pilot", liveRouteFixture(false), nil, tradeEpoch); err != nil || text != "Beta" {
		t.Fatalf("initial clipboard: %q, %v", text, err)
	}
	apply := func(kind string, seconds int, values M) string {
		return store.Observe("pilot", liveJournal(kind, seconds, values), M{})
	}
	apply("Docked", 15, livePoint("A", true))
	apply("MarketBuy", 16, v.Merge(livePoint("A", true), M{"Type": "Indite", "Count": 10, "TotalCost": 110}))
	apply("Undocked", 20, livePoint("A", true))
	if text := apply("FSDJump", 25, livePoint("B", false)); text != "Alpha" {
		t.Fatalf("unloaded return system was not copied: %q", text)
	}
	if text := apply("Docked", 30, livePoint("B", true)); text != "" {
		t.Fatalf("clipboard duplicated at station: %q", text)
	}
	view := store.Snapshot("pilot")
	if view["phase"] != "return" || v.At(view, "target", "system") != "Alpha" || v.At(view, "target", "sell") != nil ||
		v.At(view, "atStation", "sell", "key") != "indite" || v.At(view, "atStation", "buy") != nil || view["profitPerHour"] != nil {
		t.Fatalf("incorrect unloaded return: %v", view)
	}
	apply("MarketSell", 32, v.Merge(livePoint("B", true), M{"Type": "Indite", "Count": 10, "TotalSale": 380}))
	apply("Undocked", 36, livePoint("B", true))
	if text := apply("FSDJump", 70, livePoint("A", false)); text != "Beta" {
		t.Fatalf("outgoing system was not copied for next round: %q", text)
	}
	apply("Docked", 80, livePoint("A", true))
	view = store.Snapshot("pilot")
	if view["phase"] != "outbound" || view["lastRoundSeconds"] != 60.0 || view["actualRoundProfit"] != 270.0 ||
		view["actualProfitPerHour"] != 16200.0 || store.Export()["pilot"] == nil {
		t.Fatalf("unloaded return was not timed or reconciled: %v", view)
	}
	restored := NewLiveRoutes(store.Export())
	if restored.Snapshot("pilot")["actualProfitPerHour"] != 16200.0 {
		t.Fatal("unloaded route failed to survive reload")
	}
}

func TestLiveRouteSameSystemUsesDockingForClipboard(t *testing.T) {
	route := liveRouteFixture(true)
	legs := v.Rows(route["legs"])
	for _, b := range []M{v.Obj(legs[0]["sell"]), v.Obj(legs[1]["buy"])} {
		b["system"], b["systemAddress"] = "Alpha", "10"
	}
	store := NewLiveRoutes(nil)
	if _, err := store.Select("pilot", route, M{}, tradeEpoch); err != nil {
		t.Fatal(err)
	}
	store.Observe("pilot", liveJournal("Docked", 10, livePoint("A", true)), nil)
	store.Observe("pilot", liveJournal("Undocked", 11, livePoint("A", true)), nil)
	if text := store.Observe("pilot", liveJournal("FSDJump", 20, livePoint("A", false)), nil); text != "" {
		t.Fatalf("same-system route copied before market arrival: %q", text)
	}
	b := v.Merge(livePoint("B", true), M{"StarSystem": "Alpha", "SystemAddress": "10"})
	if text := store.Observe("pilot", liveJournal("Docked", 30, b), nil); text != "Alpha" {
		t.Fatalf("same-system dock did not copy: %q", text)
	}
}
