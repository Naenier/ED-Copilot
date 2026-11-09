package presentation

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/colonization"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func hudStockCarrier(id string, items ...M) M {
	if items == nil {
		items = []M{}
	}
	return M{"market": M{"id": id}, "name": M{"callsign": "HUD-0" + id}, "cargo": items, "currentStarSystem": "Synthetic HUD Colony"}
}

func hudStockPlanner(t *testing.T, required, cargo, personal, squadron []M) (*colonization.Planner, M, *time.Time) {
	t.Helper()
	now := hudFixtureNow
	planner := colonization.New(nil, colonization.WithClock(func() time.Time { return now }))
	update := func(body M) {
		t.Helper()
		if err := planner.Update(body); err != nil {
			t.Fatal(err)
		}
	}
	update(M{"action": "create-project", "targetSystem": "Synthetic HUD Colony", "status": "building"})
	project := v.Obj(planner.Snapshot(nil)["activeProject"])
	update(M{"action": "add-building", "projectId": project["id"], "typeId": "coriolis", "marketId": "41", "name": "Synthetic Depot"})
	context := M{"commander": M{"fid": "FHUD", "system": "Synthetic HUD Colony", "vehicle": "ship", "cargoForSync": cargo}}
	planner.Apply(M{"event": "ColonisationConstructionDepot", "timestamp": v.ISO(now), "MarketID": "41", "StationName": "Orbital Construction Site: Synthetic Depot", "ResourcesRequired": required}, context)
	result := M{"status": M{"connected": true, "fid": "FHUD"}, "data": M{"carrier": hudStockCarrier("42", personal...), "squadron": M{"squadronCarrier": hudStockCarrier("43", squadron...)}}, "metadata": M{"observedAt": M{"carrier": v.ISO(now), "squadron": v.ISO(now)}}}
	planner.ApplyCAPI(result, v.Obj(context["commander"]))
	return planner, context, &now
}

func hudStockState(planner *colonization.Planner, context M) M {
	return M{"settings": M{"activity": "colonization"}, "colonization": planner.Snapshot(context)}
}

func hudEnableWarehouse(t *testing.T, p *colonization.Planner) {
	t.Helper()
	if err := p.Update(M{"action": "configure", "includeCarrierWarehouse": true}); err != nil {
		t.Fatal(err)
	}
}

func hudCheckAction(t *testing.T, doc HUDDocument, title, name, quantity string) {
	t.Helper()
	action := hudSection(doc, title)
	if action == nil {
		t.Fatalf("missing %s: %#v", title, doc)
	}
	for _, row := range action.Lines {
		if row.Text == name && row.Value == quantity {
			return
		}
	}
	t.Fatalf("missing %s / %s in %#v", name, quantity, action)
}

func TestHUDColonySeparatesShipCarrierAndPurchaseWithoutDoubleCounting(t *testing.T) {
	p, context, now := hudStockPlanner(t, []M{{"Name": "steel", "RequiredAmount": 100, "ProvidedAmount": 0}}, []M{{"Name": "steel", "Count": 10}}, []M{{"commodity": "steel", "qty": 30}}, nil)
	hudEnableWarehouse(t, p)
	state := hudStockState(p, context)
	before, _ := json.Marshal(state)
	doc := BuildHUD(state, *now)
	hudCheckAction(t, doc, "ДОСТАВИТЬ", "Сталь", "10 т")
	hudCheckAction(t, doc, "РАЗГРУЗИТЬ", "Сталь", "30 т")
	hudCheckAction(t, doc, "КУПИТЬ", "Сталь", "60 т")
	if got := hudSection(doc, "РАЗГРУЗИТЬ").Subtitle; got != "HUD-042" {
		t.Fatal("missing carrier identity", doc)
	}
	if doc.Sections[0].Subtitle != "Synthetic HUD Colony" || doc.Sections[2].Subtitle != "" || strings.Contains(hudProse(doc), "ПРОДОЛЖЕНИЕ") {
		t.Fatal("duplicate context or continuation", doc)
	}
	after, _ := json.Marshal(state)
	if string(before) != string(after) {
		t.Fatal("HUD mutated the colony or stock")
	}
	v.Obj(state["settings"])["language"] = "en"
	doc = BuildHUD(state, *now)
	hudCheckAction(t, doc, "DELIVER", "Steel", "10 t")
	hudCheckAction(t, doc, "UNLOAD", "Steel", "30 t")
	hudCheckAction(t, doc, "BUY", "Steel", "60 t")

	if err := p.Update(M{"action": "configure", "includeCarrierWarehouse": false}); err != nil {
		t.Fatal(err)
	}
	doc = BuildHUD(hudStockState(p, context), *now)
	hudCheckAction(t, doc, "КУПИТЬ", "Сталь", "90 т")
	if hudSection(doc, "РАЗГРУЗИТЬ") != nil {
		t.Fatal("unload advice ignored the opt-out", doc)
	}
}

func TestHUDColonyUsesShipCargoBeforeCarrierEvenWhenCarrierCoversEverything(t *testing.T) {
	p, context, now := hudStockPlanner(t, []M{{"Name": "steel", "RequiredAmount": 100, "ProvidedAmount": 0}}, []M{{"Name": "steel", "Count": 65}}, []M{{"commodity": "steel", "qty": 120}}, nil)
	hudEnableWarehouse(t, p)
	doc := BuildHUD(hudStockState(p, context), *now)
	hudCheckAction(t, doc, "ДОСТАВИТЬ", "Сталь", "65 т")
	hudCheckAction(t, doc, "РАЗГРУЗИТЬ", "Сталь", "35 т")
	if hudSection(doc, "КУПИТЬ") != nil {
		t.Fatal("a covered item appeared in purchases", doc)
	}
}

func TestHUDColonyCarrierLocationRequiresFreshConfirmedIdentity(t *testing.T) {
	const system = "Synthetic HUD Colony"
	p, context, now := hudStockPlanner(t, []M{{"Name": "steel", "RequiredAmount": 100, "ProvidedAmount": 0}}, []M{}, []M{{"commodity": "steel", "qty": 30}}, nil)
	hudEnableWarehouse(t, p)
	state := hudStockState(p, context)
	warehouse := v.Rows(v.At(state, "colonization", "carrierWarehouses"))[0]
	for _, tc := range []struct {
		name, observed, tracked string
		id, address             string
		departure               time.Time
		planned, old, unload    bool
	}{
		{name: "fresh CAPI", observed: system, unload: true},
		{name: "remote stock", observed: "Elsewhere"},
		{name: "stale location", observed: system, old: true},
		{name: "owned tracker fallback", tracked: system, id: "42", unload: true},
		{name: "unrelated tracker", tracked: system, id: "43"},
		{name: "address mismatch", tracked: system, id: "42", address: "999"},
		{name: "pending target is not position", observed: "Elsewhere", id: "42", departure: now.Add(time.Minute)},
		{name: "departure still ahead", observed: system, id: "42", departure: now.Add(time.Minute), planned: true, unload: true},
		{name: "elapsed departure", observed: system, id: "42", departure: now.Add(-time.Second)},
		{name: "CAPI itinerary with unknown departure", observed: system, planned: true},
		{name: "untracked departure does not reuse prior journal location", observed: system, tracked: system, id: "42", planned: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := *now
			if tc.old {
				at = at.Add(-hudCarrierLocationMaxAge - time.Second)
			}
			warehouse["location"] = M{"system": tc.observed, "updatedAt": v.ISO(at), "jumpPlanned": tc.planned}
			v.Obj(v.At(state, "colonization", "activeProject"))["systemAddress"] = "123"
			carrier := M{"id": tc.id, "location": M{"system": tc.tracked, "systemAddress": tc.address, "updatedAt": v.ISO(*now)}}
			if !tc.departure.IsZero() {
				carrier["jump"] = M{"pending": M{"system": system, "departureTime": v.ISO(tc.departure)}}
			}
			state["carrier"] = carrier
			doc := BuildHUD(state, *now)
			hudCheckAction(t, doc, "КУПИТЬ", "Сталь", "70 т")
			if got := hudSection(doc, "РАЗГРУЗИТЬ"); (got != nil) != tc.unload {
				t.Fatalf("unexpected unload: %#v", doc)
			}
			if tc.unload {
				hudCheckAction(t, doc, "РАЗГРУЗИТЬ", "Сталь", "30 т")
			} else {
				hudCheckAction(t, doc, "КОЛОНИЯ", "Забрать с флотоносца", "30 т")
			}
		})
	}
}

func TestHUDColonyOnlyUsesSquadronStockWithMatchingPermission(t *testing.T) {
	p, context, now := hudStockPlanner(t, []M{{"Name": "steel", "RequiredAmount": 100, "ProvidedAmount": 0}}, []M{}, []M{{"commodity": "steel", "qty": 15}}, []M{{"commodity": "steel", "qty": 9999}})
	hudEnableWarehouse(t, p)
	doc := BuildHUD(hudStockState(p, context), *now)
	hudCheckAction(t, doc, "КУПИТЬ", "Сталь", "85 т")
	if strings.Contains(hudProse(doc), "HUD-043") {
		t.Fatal("inaccessible carrier stock leaked", doc)
	}
	if err := p.Update(M{"action": "configure", "squadronWarehouseAccessConfirmed": true}); err != nil {
		t.Fatal(err)
	}
	doc = BuildHUD(hudStockState(p, context), *now)
	unloads := []HUDSection{}
	for _, section := range doc.Sections {
		if section.Kind == "unload" {
			unloads = append(unloads, section)
		}
	}
	if len(unloads) != 2 || unloads[0].Lines[0].Value != "15 т" || unloads[1].Subtitle != "HUD-043" || unloads[1].Lines[0].Value != "85 т" || hudSection(doc, "КУПИТЬ") != nil {
		t.Fatal("accessible warehouses were conflated, overcounted, or treated as purchase", doc)
	}
	state := hudStockState(p, context)
	warehouses := v.Rows(v.At(state, "colonization", "carrierWarehouses"))
	warehouses[1]["id"] = warehouses[0]["id"]
	doc = BuildHUD(state, *now)
	hudCheckAction(t, doc, "КУПИТЬ", "Сталь", "85 т")
}

func TestHUDColonyStaleWarehouseProducesOnlyAnUpperPurchaseBound(t *testing.T) {
	p, context, now := hudStockPlanner(t, []M{{"Name": "steel", "RequiredAmount": 100, "ProvidedAmount": 0}}, []M{}, []M{{"commodity": "steel", "qty": 120}}, nil)
	hudEnableWarehouse(t, p)
	*now = now.Add(16 * time.Minute)
	doc := BuildHUD(hudStockState(p, context), *now)
	hudCheckAction(t, doc, "КУПИТЬ", "Сталь", "≤100 т")
	if hudSection(doc, "РАЗГРУЗИТЬ") != nil || strings.Contains(hudProse(doc), "120 т") {
		t.Fatal("stale carrier stock appeared as current", doc)
	}
	state := hudStockState(p, context)
	row := v.Rows(v.At(state, "colonization", "activeProject", "operationalSupplies"))[0]
	row["cargoAvailable"] = nil
	doc = BuildHUD(state, *now)
	hudCheckAction(t, doc, "КУПИТЬ", "Сталь", "≤100 т")
	if hudSection(doc, "ДОСТАВИТЬ") != nil || !hudHasWarning(doc, "Проверьте груз корабля") {
		t.Fatal("unobserved ship cargo was treated as zero", doc)
	}
}

func TestHUDColonyUnknownShipHoldBoundsKnownLocalCarrierAndPurchase(t *testing.T) {
	p, context, now := hudStockPlanner(t, []M{{"Name": "steel", "RequiredAmount": 100, "ProvidedAmount": 0}}, nil, []M{{"commodity": "steel", "qty": 30}}, nil)
	hudEnableWarehouse(t, p)
	v.Obj(context["commander"])["cargoForSync"] = nil
	doc := BuildHUD(hudStockState(p, context), *now)
	hudCheckAction(t, doc, "РАЗГРУЗИТЬ", "Сталь", "≤30 т")
	hudCheckAction(t, doc, "КУПИТЬ", "Сталь", "≤70 т")
	if hudSection(doc, "ДОСТАВИТЬ") != nil || !hudHasWarning(doc, "Проверьте груз корабля") {
		t.Fatal("unknown hold produced an exact delivery", doc)
	}
}

func TestHUDColonyPreviewKeepsGroupRemaindersWithoutContinuation(t *testing.T) {
	var rows []M
	for index := range 15 {
		rows = append(rows, M{"commodity": fmt.Sprintf("item%d", index), "name": fmt.Sprintf("Item%02d", index), "missing": 100, "cargoAvailable": 0})
	}
	state := M{"settings": M{"activity": "colonization"}, "colonization": M{"activeProject": M{"name": "Synthetic", "status": "building", "operationalSupplies": rows}}}
	doc := BuildHUD(state, hudFixtureNow)
	if len(doc.Sections) != 1 || doc.Sections[0].More != 8 || len(doc.Sections[0].Lines) != 7 {
		t.Fatal("single purchase preview cap/remainder", doc)
	}
	for _, row := range rows {
		row["cargoAvailable"] = 10
	}
	doc = BuildHUD(state, hudFixtureNow)
	delivery, purchase := hudSection(doc, "ДОСТАВИТЬ"), hudSection(doc, "КУПИТЬ")
	if len(doc.Sections) != 2 || delivery == nil || purchase == nil || len(delivery.Lines)+len(purchase.Lines) != 10 || delivery.More+len(delivery.Lines) != 15 || purchase.More+len(purchase.Lines) != 15 {
		t.Fatal("combined preview cap/remainders", doc)
	}
}
