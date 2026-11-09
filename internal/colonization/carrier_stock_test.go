package colonization

import (
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/value"
)

func stockFixture(t *testing.T) (*Planner, M, M) {
	t.Helper()
	p := New(nil, WithClock(func() time.Time { return testNow }))
	c := colony(t, p, "Synthetic warehouse colony", 100, 0)
	action(t, p, M{"action": "configure", "includeCarrierWarehouse": true})
	context := M{"commander": M{"fid": "F1", "vehicle": "ship", "cargoForSync": []M{{"Name": "steel", "Count": 10}}}}
	return p, c, context
}

func capiStock(id string, amount float64) M {
	return M{"market": M{"id": id}, "name": M{"callsign": "SYN-001"}, "cargo": []M{{"commodity": "steel", "qty": amount}}}
}

func stockResult(personal, squadron any, at time.Time) M {
	return M{"status": M{"connected": true, "fid": "F1"}, "data": M{"carrier": personal, "squadron": M{"squadronCarrier": squadron, "bank": M{"commodities": M{"steel": []M{{"qty": 99999}}}}}}, "metadata": M{"observedAt": M{"carrier": value.ISO(at), "squadron": value.ISO(at)}}}
}

func steelSupply(t *testing.T, p *Planner, context M) M {
	t.Helper()
	for _, r := range rows(p.Snapshot(context)["aggregateSupplies"]) {
		if str(r["commodity"]) == "steel" {
			return r
		}
	}
	t.Fatal("missing synthetic steel row")
	return nil
}

func TestCarrierStockPersistsObservedLocationSeparatelyFromJump(t *testing.T) {
	p, _, context := stockFixture(t)
	personal := capiStock("42", 30)
	personal["currentStarSystem"] = "Observed Fleet System"
	personal["itinerary"] = M{"currentJump": "Planned Destination"}
	squadron := capiStock("43", 40)
	squadron["currentStarSystem"] = M{"name": "Observed Squadron System"}
	p.ApplyCAPI(stockResult(personal, squadron, testNow), value.Obj(context["commander"]))
	for _, planner := range []*Planner{p, New(p.ExportSettings(), WithClock(func() time.Time { return testNow }))} {
		warehouses := rows(planner.Snapshot(context)["carrierWarehouses"])
		fleetLocation := value.Obj(warehouses[0]["location"])
		squadronLocation := value.Obj(warehouses[1]["location"])
		if fleetLocation["system"] != "Observed Fleet System" || !value.Bool(fleetLocation["jumpPlanned"]) || !value.Time(fleetLocation["updatedAt"]).Equal(testNow) || squadronLocation["system"] != "Observed Squadron System" || value.Bool(squadronLocation["jumpPlanned"]) {
			t.Fatal("wrong carrier identity, location, or jump status", warehouses)
		}
		fleetLocation["system"] = "Caller Mutation"
		if got := value.At(rows(planner.Snapshot(context)["carrierWarehouses"])[0], "location", "system"); got != "Observed Fleet System" {
			t.Fatal("snapshot shared the cached location", got)
		}
		other := planner.Snapshot(M{"commander": M{"fid": "F2"}})
		if value.At(rows(other["carrierWarehouses"])[0], "location") != nil {
			t.Fatal("carrier location exposed to another commander")
		}
	}
}

func TestCarrierStocksOptInAndNoDoubleCounting(t *testing.T) {
	p, colony, context := stockFixture(t)
	p.ApplyCAPI(stockResult(capiStock("42", 30), capiStock("43", 40), testNow), value.Obj(context["commander"]))
	action(t, p, M{"action": "configure", "squadronWarehouseAccessConfirmed": true})
	// The old tracker baseline is another view of the same stock, not extra cargo.
	context["carrier"] = M{"owned": true, "warehouse": M{"complete": true, "manualAt": value.ISO(testNow), "items": []M{{"key": "steel", "count": 9999}}}}
	r := steelSupply(t, p, context)
	if num(r["carrierAvailable"]) != 70 || num(r["cargoAvailable"]) != 10 || num(r["toBuy"]) != 20 || num(r["missing"]) != 100 {
		t.Fatal("stock, ship or manual baseline counted twice", r)
	}
	// Delivered cargo reduces the outstanding bill, never enters the warehouse.
	b := rows(colony["buildings"])[0]
	action(t, p, M{"action": "record-delivery", "projectId": colony["id"], "buildingId": b["id"], "commodity": "steel", "count": 10})
	r = steelSupply(t, p, context)
	if num(r["missing"]) != 90 || num(r["toBuy"]) != 10 || num(r["carrierAvailable"]) != 70 {
		t.Fatal("delivery mixed into stock", r)
	}
	action(t, p, M{"action": "configure", "includeCarrierWarehouse": false})
	r = steelSupply(t, p, context)
	if r["carrierAvailable"] != nil || num(r["toBuy"]) != 80 || value.Bool(r["carrierIncluded"]) {
		t.Fatal("disabled stock still reduces purchase requirement", r)
	}
	if New(nil).Snapshot(nil)["includeCarrierWarehouse"] != false {
		t.Fatal("new warehouse accounting must be explicit")
	}
}

func TestCarrierStocksUnknownZeroAndSquadronAccess(t *testing.T) {
	p, _, context := stockFixture(t)
	r := steelSupply(t, p, context)
	if r["carrierAvailable"] != nil || r["toBuy"] != nil || num(r["toBuyAtMost"]) != 90 {
		t.Fatal("unread stock treated as zero", r)
	}
	p.ApplyCAPI(stockResult(capiStock("42", 0), capiStock("43", 40), testNow), value.Obj(context["commander"]))
	r = steelSupply(t, p, context)
	if r["carrierAvailable"] == nil || num(r["carrierAvailable"]) != 0 || num(r["toBuy"]) != 90 {
		t.Fatal("confirmed zero or squadron exclusion lost", r)
	}
	warehouses := rows(p.Snapshot(context)["carrierWarehouses"])
	if !value.Bool(warehouses[1]["known"]) || value.Bool(warehouses[1]["included"]) || num(warehouses[1]["knownTons"]) != 40 {
		t.Fatal("observed shared stock must remain visible but excluded", warehouses)
	}
	action(t, p, M{"action": "configure", "squadronWarehouseAccessConfirmed": true})
	if r = steelSupply(t, p, context); num(r["carrierAvailable"]) != 40 || num(r["toBuy"]) != 50 {
		t.Fatal("confirmed accessible squadron stock not counted", r)
	}
	// A successful response without squadronCarrier.cargo is unknown, not the bank.
	p.ApplyCAPI(stockResult(capiStock("42", 20), nil, testNow.Add(time.Second)), value.Obj(context["commander"]))
	p.now = func() time.Time { return testNow.Add(time.Second) }
	r = steelSupply(t, p, context)
	if num(r["carrierAvailable"]) != 20 || r["toBuy"] != nil || num(r["toBuyAtMost"]) != 70 || value.Bool(r["carrierComplete"]) {
		t.Fatal("unknown shared hold replaced by bank or zero", r)
	}
}

func TestCarrierStocksLegalityIdentityAgeAndMalformedCargo(t *testing.T) {
	p, _, context := stockFixture(t)
	personal := capiStock("42", 20)
	personal["cargo"] = []M{{"commodity": "steel", "qty": 20}, {"commodity": "steel", "qty": 30, "stolen": true}, {"commodity": "steel", "qty": 40, "mission": true}, {"commodity": "steel", "qty": 5}}
	p.ApplyCAPI(stockResult(personal, nil, testNow), value.Obj(context["commander"]))
	if r := steelSupply(t, p, context); num(r["carrierAvailable"]) != 25 || num(r["toBuy"]) != 65 {
		t.Fatal("illegal, mission or duplicate cargo handling", r)
	}
	wrong := value.Copy(context)
	value.Obj(wrong["commander"])["fid"] = "F2"
	if r := steelSupply(t, p, wrong); r["carrierAvailable"] != nil {
		t.Fatal("another commander's stock exposed", r)
	}
	if warehouse := rows(p.Snapshot(wrong)["carrierWarehouses"])[0]; warehouse["id"] != nil || warehouse["callsign"] != nil || warehouse["receivedAt"] != nil {
		t.Fatal("another commander's warehouse identity exposed", warehouse)
	}
	p.ApplyCAPI(stockResult(capiStock("42", 999), nil, testNow.Add(time.Minute)), value.Obj(wrong["commander"]))
	if r := steelSupply(t, p, context); num(r["carrierAvailable"]) != 25 {
		t.Fatal("mismatching CAPI identity accepted", r)
	}
	p.now = func() time.Time { return testNow.Add(16 * time.Minute) }
	if r := steelSupply(t, p, context); r["carrierAvailable"] != nil || r["toBuy"] != nil {
		t.Fatal("expired stock counted", r)
	}
	for _, invalid := range []any{nil, M{"steel": 1}, []M{{"commodity": "steel", "qty": -1}}, []any{"invalid"}} {
		raw := capiStock("42", 10)
		raw["cargo"] = invalid
		if readCarrierStock(raw, value.ISO(testNow)) != nil {
			t.Fatalf("malformed cargo became complete: %#v", invalid)
		}
	}
}

func TestCarrierStocksTransfersReplayPersistenceAndSameCarrierDedupe(t *testing.T) {
	p, _, context := stockFixture(t)
	initial := stockResult(capiStock("42", 30), capiStock("42", 30), testNow)
	p.ApplyCAPI(initial, value.Obj(context["commander"]))
	action(t, p, M{"action": "configure", "squadronWarehouseAccessConfirmed": true})
	if num(steelSupply(t, p, context)["carrierAvailable"]) != 30 {
		t.Fatal("one physical carrier counted through two views")
	}
	p.now = func() time.Time { return testNow.Add(time.Minute) }
	event := M{"event": "CargoTransfer", "timestamp": value.ISO(testNow.Add(time.Minute)), "CarrierID": "42", "Transfers": []M{{"Type": "steel", "Count": 5, "Direction": "toship"}}}
	meta := M{"file": "Journal.synthetic.log", "offset": 99}
	p.Apply(event, meta)
	value.Obj(context["commander"])["cargoForSync"] = []M{{"Name": "steel", "Count": 15}}
	check := func(planner *Planner) {
		t.Helper()
		r := steelSupply(t, planner, context)
		if num(r["carrierAvailable"]) != 25 || num(r["cargoAvailable"]) != 15 || num(r["toBuy"]) != 60 {
			t.Fatal("transfer counted twice or lost", r)
		}
	}
	check(p)
	p.Apply(event, meta)
	p.ApplyCAPI(initial, value.Obj(context["commander"])) // Same cached response cannot undo a transfer.
	check(p)
	p.Reset()
	p.Apply(M{"event": "LoadGame", "FID": "F1"}, M{})
	p.Apply(event, meta)
	check(p)
	restored := New(p.ExportSettings(), WithClock(p.now))
	restored.Apply(event, meta)
	check(restored)
	restored.Apply(M{"event": "LoadGame", "FID": "F2"}, M{})
	if r := steelSupply(t, restored, context); r["carrierAvailable"] != nil {
		t.Fatal("commander switch kept stock", r)
	}
}

func TestCarrierStockConfigurationRejectsInvalidValuesAtomically(t *testing.T) {
	p := New(nil)
	if err := p.Update(M{"action": "configure", "includeCarrierWarehouse": true, "squadronWarehouseAccessConfirmed": "yes"}); err == nil || value.Bool(p.ExportSettings()["includeCarrierWarehouse"]) {
		t.Fatal("invalid checkbox update partially applied")
	}
}

func TestSquadronOnlyAccountDoesNotInventUnknownPersonalCargo(t *testing.T) {
	p, _, context := stockFixture(t)
	p.ApplyCAPI(stockResult(nil, capiStock("43", 40), testNow), value.Obj(context["commander"]))
	action(t, p, M{"action": "configure", "squadronWarehouseAccessConfirmed": true})
	if r := steelSupply(t, p, context); num(r["carrierAvailable"]) != 40 || num(r["toBuy"]) != 50 || !value.Bool(r["carrierComplete"]) {
		t.Fatal("successful personal 204 prevents squadron-only planning", r)
	}
	restored := New(p.ExportSettings(), WithClock(p.now))
	if r := steelSupply(t, restored, context); num(r["toBuy"]) != 50 {
		t.Fatal("confirmed personal absence lost on restore", r)
	}
}

func TestSquadronAccessIsBoundToCommanderAndCarrier(t *testing.T) {
	p, _, context := stockFixture(t)
	p.ApplyCAPI(stockResult(capiStock("42", 30), capiStock("43", 40), testNow), value.Obj(context["commander"]))
	action(t, p, M{"action": "configure", "squadronWarehouseAccessConfirmed": true, "squadronWarehouseAccessScope": "F1|43"})
	if p.config["squadronWarehouseAccessFID"] != "F1" || p.config["squadronWarehouseAccessCarrierID"] != "43" {
		t.Fatal("confirmation lacks an identity binding", p.config)
	}
	p.now = func() time.Time { return testNow.Add(time.Second) }
	p.ApplyCAPI(stockResult(capiStock("42", 30), capiStock("43", 35), p.now()), value.Obj(context["commander"]))
	if !p.squadronAccessConfirmed() || num(steelSupply(t, p, context)["carrierAvailable"]) != 65 {
		t.Fatal("unchanged carrier lost confirmation")
	}
	restored := New(p.ExportSettings(), WithClock(p.now))
	if !restored.squadronAccessConfirmed() {
		t.Fatal("bound confirmation was not restored")
	}
	legacy := p.ExportSettings()
	delete(legacy, "squadronWarehouseAccessFID")
	delete(legacy, "squadronWarehouseAccessCarrierID")
	if value.Bool(New(legacy, WithClock(p.now)).ExportSettings()["squadronWarehouseAccessConfirmed"]) {
		t.Fatal("old global true silently authorized shared stock")
	}
	p.now = func() time.Time { return testNow.Add(2 * time.Second) }
	p.ApplyCAPI(stockResult(capiStock("42", 30), capiStock("44", 99), p.now()), value.Obj(context["commander"]))
	if p.squadronAccessConfirmed() || value.Bool(p.config["squadronWarehouseAccessConfirmed"]) {
		t.Fatal("new squadron carrier inherited access")
	}
	if err := p.Update(M{"action": "configure", "squadronWarehouseAccessConfirmed": true, "squadronWarehouseAccessScope": "F1|43"}); err == nil {
		t.Fatal("stale UI authorized a different carrier")
	}
	action(t, p, M{"action": "configure", "squadronWarehouseAccessConfirmed": true, "squadronWarehouseAccessScope": "F1|44"})
	p.Apply(M{"event": "LoadGame", "FID": "F2"}, M{})
	nextCommander := M{"fid": "F2", "vehicle": "ship", "cargoForSync": []M{}}
	next := stockResult(capiStock("45", 1), capiStock("44", 99), p.now().Add(time.Second))
	value.Obj(next["status"])["fid"] = "F2"
	p.ApplyCAPI(next, nextCommander)
	if p.squadronAccessConfirmed() || value.Bool(p.config["squadronWarehouseAccessConfirmed"]) {
		t.Fatal("new commander inherited access even to the same carrier")
	}
}

func TestSquadronAccessRequiresIdentityAndKeepsUnknownStockUnknown(t *testing.T) {
	p, _, context := stockFixture(t)
	if p.Update(M{"action": "configure", "squadronWarehouseAccessConfirmed": true}) == nil {
		t.Fatal("unread carrier can be confirmed")
	}
	p.ApplyCAPI(stockResult(capiStock("42", 0), capiStock("43", 10), testNow), value.Obj(context["commander"]))
	action(t, p, M{"action": "configure", "squadronWarehouseAccessConfirmed": true})
	p.now = func() time.Time { return testNow.Add(time.Second) }
	unknown := capiStock("43", 0)
	delete(unknown, "cargo")
	p.ApplyCAPI(stockResult(capiStock("42", 0), unknown, p.now()), value.Obj(context["commander"]))
	if !p.squadronAccessConfirmed() || steelSupply(t, p, context)["toBuy"] != nil {
		t.Fatal("unknown cargo was changed to zero or access moved")
	}
	action(t, p, M{"action": "configure", "includeCarrierWarehouse": false, "squadronWarehouseAccessConfirmed": true})
}

func TestCarrierResponseRequestedBeforeTransferCannotRollbackStock(t *testing.T) {
	for _, withBaseline := range []bool{false, true} {
		t.Run(map[bool]string{false: "first snapshot", true: "existing snapshot"}[withBaseline], func(t *testing.T) {
			p, _, context := stockFixture(t)
			if withBaseline {
				p.ApplyCAPI(stockResult(capiStock("42", 30), nil, testNow), value.Obj(context["commander"]))
			}
			started := testNow.Add(30 * time.Second)
			moved := testNow.Add(time.Minute)
			p.now = func() time.Time { return moved }
			p.Apply(M{"event": "CargoTransfer", "timestamp": value.ISO(moved), "CarrierID": "42", "Transfers": []M{{"Type": "steel", "Count": 5, "Direction": "toship"}}}, M{"file": "Journal.synthetic.log", "offset": 101, "commander": context["commander"]})
			value.Obj(context["commander"])["cargoForSync"] = []M{{"Name": "steel", "Count": 15}}
			received := testNow.Add(2 * time.Minute)
			p.now = func() time.Time { return received }
			response := stockResult(capiStock("42", 30), nil, received)
			value.Obj(response["metadata"])["requestStartedAt"] = M{"carrier": value.ISO(started), "squadron": value.ISO(started)}
			p.ApplyCAPI(response, value.Obj(context["commander"]))
			r := steelSupply(t, p, context)
			if withBaseline && (num(r["carrierAvailable"]) != 25 || num(r["toBuy"]) != 60) {
				t.Fatal("late response rolled back the known transfer", r)
			}
			if !withBaseline && (r["carrierAvailable"] != nil || r["toBuy"] != nil) {
				t.Fatal("first stale response manufactured current stock", r)
			}
			freshAt := testNow.Add(3 * time.Minute)
			p.now = func() time.Time { return freshAt }
			fresh := stockResult(capiStock("42", 25), nil, freshAt)
			value.Obj(fresh["metadata"])["requestStartedAt"] = M{"carrier": value.ISO(freshAt.Add(-time.Second))}
			p.ApplyCAPI(fresh, value.Obj(context["commander"]))
			if r := steelSupply(t, p, context); num(r["carrierAvailable"]) != 25 || num(r["toBuy"]) != 60 {
				t.Fatal("later unambiguous snapshot was rejected", r)
			}
		})
	}
}
