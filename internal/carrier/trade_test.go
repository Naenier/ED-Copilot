package carrier

import (
	"encoding/json"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func tradeDo(t *testing.T, p *TradePlanner, body M) {
	t.Helper()
	if err := p.Update(body); err != nil {
		t.Fatal(err)
	}
}
func tradeExpect(t *testing.T, s M, key string, want float64) {
	t.Helper()
	got, known := v.Number(s[key])
	if !known || math.Abs(got-want) > 0.001 {
		t.Fatalf("%s=%v want %g", key, s[key], want)
	}
}
func tradeGolden(t *testing.T) *TradePlanner {
	t.Helper()
	p := NewTradePlanner(nil)
	tradeDo(t, p, M{"action": "configure", "options": M{"servicesTons": 1000, "otherCargoTons": 500, "weeklyUpkeep": 7000000, "durationHours": 24, "extraCosts": 600000, "tritiumPrice": 50000, "carrierInvestment": 120000000, "haulerPad": "M", "haulerCapacity": 400}, "legs": M{
		"outbound": M{"commodityKey": "gold", "quantity": 10000, "buyPrice": 1000, "sellPrice": 4000, "loadingMode": "haulers", "unloadingMode": "haulers", "loadRewardPerTon": 500, "unloadRewardPerTon": 700, "jumps": 2, "tritiumTons": 100},
		"return":   M{"quantity": 5000, "buyPrice": 2000, "sellPrice": 3000, "jumps": 2, "tritiumTons": 80},
	}})
	return p
}

func TestCarrierTradeWholeTripGolden(t *testing.T) {
	p := tradeGolden(t)
	state := p.Snapshot()
	s := v.Obj(state["summary"])
	for key, want := range map[string]float64{"grossMargin": 35000000, "haulingFees": 12000000, "fuelTons": 180, "fuelCost": 9000000, "jumpCost": 400000, "allocatedUpkeep": 1000000, "totalOperatingCosts": 23000000, "profit": 12000000, "profitPerHour": 500000, "availableCargoTons": 23500, "workingCapital": 74000000, "breakEvenSellPrice": 2800, "breakEvenSpread": 1800, "breakEvenQuantity": 3334, "paybackTrips": 10, "paybackHours": 240} {
		tradeExpect(t, s, key, want)
	}
	legs := v.Rows(state["legs"])
	tradeExpect(t, legs[0], "carrierBuyPrice", 1500)
	tradeExpect(t, legs[0], "carrierSellPrice", 3300)
	tradeExpect(t, legs[0], "haulerSortiesEachWay", 25)
	if s["status"] != "review" {
		t.Fatal("manual prices must remain an unverified scenario", s["status"])
	}
}

func TestCarrierTradeFreshCompleteMarketsProduceReadyPlan(t *testing.T) {
	p := tradeGolden(t)
	for _, leg := range v.Rows(p.Snapshot()["legs"]) {
		for _, side := range []string{"buy", "sell"} {
			tradeDo(t, p, M{"action": "market", "leg": leg["id"], "side": side, "market": M{"station": "Synthetic " + side, "system": "Synthetic " + v.Str(leg["id"]), "pad": "L", "updatedAt": v.ISO(time.Now()), "quotes": []M{{"key": leg["commodityKey"], "price": leg[side+"Price"], "supply": 100000, "demand": 100000}}}})
		}
	}
	snapshot := p.Snapshot()
	if !v.Bool(snapshot["ready"]) || v.At(snapshot, "summary", "status") != "ready" || len(v.Rows(snapshot["issues"])) != 0 {
		t.Fatal("fully specified synthetic plan failed validation", snapshot["issues"], snapshot["missing"])
	}
	tradeExpect(t, v.Obj(snapshot["summary"]), "profit", 12000000)
}

func TestCarrierTradeUnknownIsNotFreeAndEmptyReturnStillCostsFuel(t *testing.T) {
	p := NewTradePlanner(nil)
	if v.At(p.Snapshot(), "summary", "profit") != nil {
		t.Fatal("unknown inputs became free costs")
	}
	p = tradeGolden(t)
	tradeDo(t, p, M{"options": M{"tritiumPrice": nil}})
	if v.At(p.Snapshot(), "summary", "profit") != nil || v.At(p.Snapshot(), "summary", "fuelCost") != nil {
		t.Fatal("unknown tritium price was treated as zero")
	}
	p = tradeGolden(t)
	tradeDo(t, p, M{"legs": M{"return": M{"quantity": 0, "buyPrice": nil, "sellPrice": nil}}})
	s := v.Obj(p.Snapshot()["summary"])
	tradeExpect(t, s, "fuelTons", 180)
	tradeExpect(t, s, "jumpCost", 400000)
	tradeExpect(t, s, "profit", 7000000)
	tradeDo(t, p, M{"options": M{"returnTrip": false}})
	s = v.Obj(p.Snapshot()["summary"])
	tradeExpect(t, s, "fuelTons", 100)
	tradeExpect(t, s, "jumpCost", 200000)
	tradeExpect(t, s, "profit", 11200000)
}

func TestCarrierTradeCapacityUsesHoldTritiumNotDepotAndBothCarrierTypes(t *testing.T) {
	p := tradeGolden(t)
	tradeDo(t, p, M{"options": M{"cargoTritiumTons": 1000, "tankTons": 1000}})
	tradeExpect(t, v.Obj(p.Snapshot()["summary"]), "availableCargoTons", 22500)
	tradeDo(t, p, M{"options": M{"carrierType": "squadron"}})
	s := v.Obj(p.Snapshot()["summary"])
	tradeExpect(t, s, "capacityTons", 60000)
	tradeExpect(t, s, "servicesTons", 3000)
	tradeExpect(t, s, "jumpCostPerJump", 200000)
	tradeExpect(t, s, "availableCargoTons", 55500)
	tradeDo(t, p, M{"legs": M{"outbound": M{"quantity": 60000}}})
	if v.At(p.Snapshot(), "summary", "status") != "blocked" {
		t.Fatal("over-capacity plan was not blocked")
	}
}

func TestBoozeCruiseNeverInventsHolidayPriceAndValidatesMediumPad(t *testing.T) {
	p := NewTradePlanner(nil)
	tradeDo(t, p, M{"action": "preset", "preset": "booze-cruise", "sourceSystem": "Sol"})
	state := p.Snapshot()
	legs := v.Rows(state["legs"])
	if legs[0]["commodityKey"] != "wine" || legs[0]["destinationSystem"] != "HIP 58832" || legs[0]["sellPrice"] != nil || v.At(legs[0], "sellMarket", "pad") != "M" {
		t.Fatal("unsafe Booze Cruise defaults", legs[0])
	}
	tradeDo(t, p, M{"options": M{"haulerPad": "L"}})
	blocked := false
	holiday := false
	for _, row := range v.Rows(p.Snapshot()["issues"]) {
		blocked = blocked || row["code"] == "pad-incompatible"
		holiday = holiday || row["code"] == "holiday-unconfirmed"
	}
	if !blocked || !holiday {
		t.Fatal("large ship/holiday checks missing")
	}
	before, _ := json.Marshal(p.ExportSettings())
	bad := M{"station": "Wrong Market", "system": "Sol", "pad": "L", "quotes": []M{{"key": "wine", "price": 999999, "demand": 100000}}}
	if p.Update(M{"action": "market", "leg": "outbound", "side": "sell", "market": bad}) == nil {
		t.Fatal("wrong station accepted by Booze Cruise")
	}
	after, _ := json.Marshal(p.ExportSettings())
	if string(before) != string(after) {
		t.Fatal("rejected station partially changed the plan")
	}
}

func TestCarrierTradeMarketsCheckAgeDemandShipAndCommodity(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	p := NewTradePlanner(nil, TradeWithClock(func() time.Time { return now }))
	quote := M{"station": "Synthetic Outpost", "system": "Sol", "pad": "M", "updatedAt": v.ISO(now.Add(-48 * time.Hour)), "quotes": []M{{"key": "agronomictreatment", "price": 30000, "demand": 100}}}
	tradeDo(t, p, M{"action": "market", "leg": "outbound", "side": "sell", "market": quote})
	tradeDo(t, p, M{"options": M{"haulerPad": "L"}})
	codes := map[string]bool{}
	for _, row := range v.Rows(p.Snapshot()["issues"]) {
		codes[v.Str(row["code"])] = true
	}
	for _, key := range []string{"pad-incompatible", "quote-stale", "insufficient-market-volume", "bulk-tax-risk"} {
		if !codes[key] {
			t.Fatal("market check missing", key)
		}
	}
	if p.Update(M{"action": "market", "leg": "outbound", "side": "buy", "market": M{"station": "S", "system": "X", "quotes": []M{{"key": "gold", "price": 100}}}}) == nil {
		t.Fatal("another commodity's price was imported")
	}
}

func TestCarrierTradeRouteImportAndPersistence(t *testing.T) {
	p := tradeGolden(t)
	tradeDo(t, p, M{"legs": M{"outbound": M{"sourceSystem": "A", "destinationSystem": "B"}}})
	tradeDo(t, p, M{"action": "route", "leg": "outbound", "sourceSystem": "A", "destinationSystem": "B", "routeSummary": M{"fuelTons": 125, "totalJumps": 3, "totalMinutes": 90}})
	leg := v.Obj(v.Obj(p.ExportSettings()["legs"])["outbound"])
	tradeExpect(t, leg, "tritiumTons", 125)
	tradeExpect(t, leg, "travelHours", 1.5)
	before := p.ExportSettings()
	if p.Update(M{"action": "route", "leg": "outbound", "sourceSystem": "A", "destinationSystem": "Wrong", "routeSummary": M{"fuelTons": 1, "totalJumps": 1, "totalMinutes": 20}}) == nil {
		t.Fatal("wrong route destination accepted")
	}
	if !reflect.DeepEqual(before, p.ExportSettings()) {
		t.Fatal("failed import was not atomic")
	}
	restored := NewTradePlanner(p.ExportSettings())
	if !reflect.DeepEqual(restored.ExportSettings(), p.ExportSettings()) {
		t.Fatal("trip did not survive restart")
	}
	v.Obj(v.Obj(before["legs"])["outbound"])["quantity"] = 1
	if v.At(p.ExportSettings(), "legs", "outbound", "quantity") == 1 {
		t.Fatal("export aliases state")
	}
}

func TestCarrierTradeInvalidAndConcurrentCommands(t *testing.T) {
	p := tradeGolden(t)
	before := p.ExportSettings()
	for _, bad := range []M{{"options": M{"tritiumPrice": math.Inf(1)}}, {"options": M{"weeklyUpkeep": 0}}, {"legs": M{"outbound": M{"quantity": 1.5}}}, {"options": M{"haulerPad": "XL"}}} {
		if p.Update(bad) == nil {
			t.Fatal("invalid command accepted", bad)
		}
		if !reflect.DeepEqual(before, p.ExportSettings()) {
			t.Fatal("invalid command changed settings")
		}
	}
	var wait sync.WaitGroup
	for i := 0; i < 4; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for j := 0; j < 15; j++ {
				if err := p.Update(M{"legs": M{"return": M{"quantity": j}}}); err != nil {
					t.Error(err)
				}
				p.Snapshot()
				p.ExportSettings()
			}
		}()
	}
	wait.Wait()
}

func TestCarrierTradeOverflowCountsServiceExcessEvenWithoutCommodity(t *testing.T) {
	p := tradeGolden(t)
	tradeDo(t, p, M{"options": M{"servicesTons": 30000, "otherCargoTons": 0, "cargoTritiumTons": 0}, "legs": M{"outbound": M{"quantity": 0}, "return": M{"quantity": 0}}})
	s := v.Obj(p.Snapshot()["summary"])
	tradeExpect(t, s, "availableCargoTons", 0)
	tradeExpect(t, s, "overCapacityTons", 5000)
	if s["status"] != "blocked" {
		t.Fatal("services alone overflowed without blocking the trip", s)
	}
}

func TestCarrierTradeManualEditsDoNotInheritVerifiedMarketOrRoute(t *testing.T) {
	p := tradeGolden(t)
	now := time.Now()
	tradeDo(t, p, M{"action": "market", "leg": "outbound", "side": "buy", "market": M{"station": "Synthetic Port", "system": "A", "pad": "L", "updatedAt": v.ISO(now), "quotes": []M{{"key": "gold", "price": 1000, "supply": 100000}}}})
	tradeDo(t, p, M{"legs": M{"outbound": M{"destinationSystem": "B"}}})
	tradeDo(t, p, M{"action": "route", "leg": "outbound", "sourceSystem": "A", "destinationSystem": "B", "routeSummary": M{"fuelTons": 125, "totalJumps": 3, "totalMinutes": 90}})
	tradeDo(t, p, M{"legs": M{"outbound": M{"sourceSystem": "Changed", "buyPrice": 950, "quantity": 11000}}})
	codes := map[string]bool{}
	for _, row := range v.Rows(p.Snapshot()["issues"]) {
		codes[v.Str(row["code"])] = true
	}
	for _, code := range []string{"price-overridden", "market-system-changed", "route-stale", "route-load-changed"} {
		if !codes[code] {
			t.Fatal("manual edit kept stale verification", code, codes)
		}
	}
	restored := NewTradePlanner(p.ExportSettings())
	if !reflect.DeepEqual(p.ExportSettings(), restored.ExportSettings()) {
		t.Fatal("verification context did not survive restart")
	}
}

func TestCarrierTradeBreakEvenPriceNeverRequiresNegativeSale(t *testing.T) {
	p := tradeGolden(t)
	tradeDo(t, p, M{"legs": M{"return": M{"sellPrice": 1000000}}})
	tradeExpect(t, v.Obj(p.Snapshot()["summary"]), "breakEvenSellPrice", 0)
}
