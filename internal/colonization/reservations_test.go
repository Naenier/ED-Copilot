package colonization

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/value"
)

func reservationContext(workspace string, shipStock int) M {
	return M{"planWorkspace": workspace, "commander": M{
		"fid": "F1", "vehicle": "ship", "cargoForSync": []M{{"Name": "steel", "Count": shipStock}},
	}}
}

func reservationRequest(project, building, source string, count int) M {
	return M{"action": "reserve", "projectId": project, "buildingId": building, "source": source, "commodity": "steel", "count": count}
}

func TestSupplyReservationsShareInventoryAndReleaseOnCancel(t *testing.T) {
	p := New(M{
		"includeCarrierWarehouse": true,
		"projects": []M{
			{"id": "alpha", "targetSystem": "Alpha", "buildings": []M{
				{"id": "a", "resourcePlan": []M{{"commodity": "steel", "required": 90}}},
			}},
			{"id": "beta", "targetSystem": "Beta", "buildings": []M{
				{"id": "b", "resourcePlan": []M{{"commodity": "steel", "required": 90}}},
			}},
		},
	}, WithClock(func() time.Time { return testNow }))
	ctx := reservationContext("live-f1", 0)
	p.ApplyCAPI(stockResult(capiStock("42", 100), capiStock("42", 100), testNow), value.Obj(ctx["commander"]))
	action(t, p, M{"action": "configure", "squadronWarehouseAccessConfirmed": true})
	first, err := p.UpdateSupplyReservation(ctx, reservationRequest("alpha", "a", "fleet", 70))
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []M{
		reservationRequest("beta", "b", "fleet", 31),
		reservationRequest("beta", "b", "squadron", 31),
		reservationRequest("alpha", "a", "fleet", 21),
	} {
		if _, err := p.UpdateSupplyReservation(ctx, request); err == nil {
			t.Fatalf("same physical stock or building need was overbooked: %v", request)
		}
	}
	beta := M{"projectIds": []string{"beta"}}
	project, err := p.SupplyPortfolio(ctx, beta)
	if err != nil {
		t.Fatal(err)
	}
	steel := rows(project["supplies"])[0]
	if steel["reservedElsewhere"] != float64(70) || steel["toBuy"] != float64(60) || steel["carrierCovered"] != float64(30) {
		t.Fatal("a different project counted reserved steel", steel)
	}
	free := rows(rows(project["reservable"])[0]["sources"])
	if len(free) != 1 || free[0]["source"] != "fleet" || free[0]["available"] != float64(30) {
		t.Fatal("reservable stock did not match the free physical inventory", free)
	}
	if _, err := p.UpdateSupplyReservation(ctx, reservationRequest("beta", "b", "fleet", 30)); err != nil {
		t.Fatal(err)
	}
	cancel := M{"action": "cancel", "id": first["id"]}
	if _, err := p.UpdateSupplyReservation(reservationContext("legacy-f1", 0), cancel); err == nil {
		t.Fatal("a different branch cancelled the reservation")
	}
	isolated, err := p.SupplyPortfolio(reservationContext("legacy-f1", 0), beta)
	if err != nil || len(rows(isolated["reservations"])) != 0 {
		t.Fatal("another workspace could read the reservations", isolated, err)
	}
	if got, err := p.UpdateSupplyReservation(ctx, cancel); err != nil || got["status"] != "cancelled" {
		t.Fatal("reservation was not cancelled", got, err)
	}
	project, err = p.SupplyPortfolio(ctx, beta)
	if err != nil {
		t.Fatal(err)
	}
	steel = rows(project["supplies"])[0]
	if steel["reservedHere"] != float64(30) || steel["reservedElsewhere"] != float64(0) || steel["toBuy"] != float64(0) {
		t.Fatal("cancelled stock did not return to availability", steel)
	}
}

func TestSupplyReservationsReconcileAndPersistWithoutReplayDuplication(t *testing.T) {
	now := testNow
	clock := WithClock(func() time.Time { return now })
	p := New(M{"projects": []M{{
		"id": "alpha", "targetSystem": "Alpha", "status": "building",
		"buildings": []M{{"id": "a", "status": "building", "marketId": 101}},
	}}}, clock)
	ctx := reservationContext("live-f1", 100)
	journal := func(p *Planner, at time.Time, offset int, scope string, event M) {
		event["timestamp"] = value.ISO(at)
		meta := reservationContext(scope, 100)
		meta["file"], meta["offset"] = "Journal.synthetic.log", offset
		p.Apply(event, meta)
	}
	depot := func(provided int) M {
		return M{"event": "ColonisationConstructionDepot", "StarSystem": "Alpha", "MarketID": 101, "ResourcesRequired": []M{
			{"Name": "$steel_name;", "RequiredAmount": 100, "ProvidedAmount": provided},
		}}
	}
	contribution := func() M {
		return M{"event": "ColonisationContribution", "MarketID": 101, "Contributions": []M{{"Name": "$steel_name;", "Amount": 30}}}
	}
	original := testNow.Add(-time.Minute)
	journal(p, original, 1, "live-f1", M{"event": "LoadGame", "FID": "F1"})
	journal(p, original, 2, "live-f1", depot(10))
	action(t, p, M{"action": "record-delivery", "projectId": "alpha", "buildingId": "a", "commodity": "steel", "count": 5})
	var shipment M
	for _, count := range []int{25, 35} {
		var err error
		shipment, err = p.UpdateSupplyReservation(ctx, reservationRequest("alpha", "a", "ship", count))
		if err != nil {
			t.Fatal(err)
		}
	}
	depart := M{"action": "transit", "id": shipment["id"], "inTransit": true}
	if _, err := p.UpdateSupplyReservation(reservationContext("legacy-f1", 100), depart); err == nil {
		t.Fatal("another workspace could dispatch a shipment")
	}
	if sent, err := p.UpdateSupplyReservation(ctx, depart); err != nil || !value.Bool(sent["inTransit"]) || sent["targetSystem"] != "Alpha" {
		t.Fatal("shipment was not marked for its destination", sent, err)
	}
	now = testNow.Add(time.Minute)
	journal(p, now, 3, "live-f1", contribution())
	journal(p, now, 3, "live-f1", contribution())
	journal(p, now, 4, "live-f1", depot(40))
	selection := M{"projectIds": []string{"alpha"}}
	check := func(p *Planner, missing, remaining, fulfilled float64) {
		t.Helper()
		project, err := p.SupplyPortfolio(ctx, selection)
		if err != nil {
			t.Fatal(err)
		}
		history := rows(project["reservations"])
		if len(history) != 2 || history[1]["status"] != "fulfilled" || history[1]["fulfilled"] != float64(25) ||
			history[0]["remaining"] != remaining || history[0]["fulfilled"] != fulfilled {
			t.Fatal("delivery applied more than once or to the wrong batch", history)
		}
		steel := rows(project["supplies"])[0]
		if steel["missing"] != missing || steel["manualDelivered"] != float64(5) || steel["inTransit"] != remaining || steel["reservedHere"] != remaining {
			t.Fatal("construction accounting or manual history was changed", steel)
		}
	}
	check(p, 60, 30, 5)

	payload, err := json.Marshal(p.ExportSettings())
	if err != nil {
		t.Fatal(err)
	}
	var saved M
	if err := json.Unmarshal(payload, &saved); err != nil {
		t.Fatal(err)
	}
	p = New(saved, clock)
	journal(p, original, 1, "live-f1", M{"event": "LoadGame", "FID": "F1"})
	journal(p, original, 2, "live-f1", depot(10))
	journal(p, now, 3, "live-f1", contribution())
	journal(p, now, 4, "live-f1", depot(40))
	check(p, 60, 30, 5)

	now = now.Add(time.Minute)
	journal(p, now, 5, "legacy-f1", depot(55))
	view, err := p.SupplyPortfolio(ctx, selection)
	if err != nil || rows(view["reservations"])[0]["remaining"] != float64(30) {
		t.Fatal("foreign journal branch released Live cargo", view, err)
	}
	journal(p, now, 6, "live-f1", depot(55))
	check(p, 45, 15, 20)
	depart["inTransit"] = false
	if returned, err := p.UpdateSupplyReservation(ctx, depart); err != nil || value.Bool(returned["inTransit"]) || returned["remaining"] != float64(15) {
		t.Fatal("returning a shipment to reserve changed its remaining quantity", returned, err)
	}
	view, err = p.SupplyPortfolio(ctx, selection)
	if err != nil || value.Num(rows(view["supplies"])[0]["inTransit"]) != 0 || value.Num(rows(view["supplies"])[0]["reservedHere"]) != 15 {
		t.Fatal("returned shipment disappeared from reserve", view, err)
	}

	action(t, p, M{"action": "update-building", "projectId": "alpha", "id": "a", "status": "cancelled"})
	view, err = p.SupplyPortfolio(ctx, selection)
	if err != nil || rows(view["reservations"])[0]["status"] != "released" {
		t.Fatal("cancelled building retained its reservation", view, err)
	}
}

func TestSupplyReservationCreditsExplicitContributionBeforeDepot(t *testing.T) {
	p := New(M{"projects": []M{{
		"id": "alpha", "targetSystem": "Alpha", "buildings": []M{{
			"id": "a", "status": "building", "marketId": 101,
			"resourcePlan": []M{{"commodity": "steel", "required": 100}},
		}},
	}}}, WithClock(func() time.Time { return testNow }))
	ctx := reservationContext("live-f1", 100)
	if _, err := p.UpdateSupplyReservation(ctx, reservationRequest("alpha", "a", "ship", 40)); err != nil {
		t.Fatal(err)
	}
	journal := func(p *Planner, event M, at time.Time, offset int) {
		meta := reservationContext("live-f1", 100)
		meta["file"], meta["offset"] = "Journal.synthetic.log", offset
		event["timestamp"] = value.ISO(at)
		p.Apply(event, meta)
	}
	contribution := func(market, amount int) M {
		return M{"event": "ColonisationContribution", "MarketID": market, "Contributions": []M{{"Name": "steel", "Amount": amount}}}
	}
	depot := func(provided int) M {
		return M{"event": "ColonisationConstructionDepot", "StarSystem": "Alpha", "MarketID": 101, "ResourcesRequired": []M{
			{"Name": "steel", "RequiredAmount": 100, "ProvidedAmount": provided},
		}}
	}
	at := testNow.Add(time.Minute)
	journal(p, contribution(102, 50), at, 1)
	journal(p, contribution(101, 10), at, 2)
	journal(p, contribution(101, 10), at, 2)
	check := func(p *Planner, remaining float64) {
		t.Helper()
		view, err := p.SupplyPortfolio(ctx, M{"projectIds": []string{"alpha"}})
		if err != nil {
			t.Fatal(err)
		}
		history := rows(view["reservations"])
		if len(history) != 1 || history[0]["remaining"] != remaining || history[0]["fulfilled"] != 40-remaining {
			t.Fatal("explicit contribution released the wrong amount", history)
		}
	}
	check(p, 30)
	p = New(p.ExportSettings())
	journal(p, depot(20), testNow.Add(-time.Minute), 1)
	journal(p, contribution(101, 10), at, 2)
	journal(p, depot(30), at, 3)
	check(p, 30)
	journal(p, contribution(101, 8), at, 4)
	check(p, 22)
	p = New(p.ExportSettings())
	journal(p, contribution(101, 8), at, 4)
	check(p, 22)
}
