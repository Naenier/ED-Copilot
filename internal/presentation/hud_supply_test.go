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
	return M{"market": M{"id": id}, "name": M{"callsign": "HUD-001"}, "cargo": items}
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

func TestHUDColonyShowsActualCarrierSteelEvenWhenAccountingIsOff(t *testing.T) {
	p, context, now := hudStockPlanner(t, []M{{"Name": "steel", "Name_Localised": "Сталь", "RequiredAmount": 10164, "ProvidedAmount": 0}}, []M{}, []M{{"commodity": "steel", "qty": 12000}}, nil)
	s := hudStockState(p, context)
	before, _ := json.Marshal(s)
	doc := BuildHUD(s, *now)
	text := hudProse(doc)
	for _, want := range []string{"Сталь: осталось 10 164 т · ФН: 12 000 т ✓", "Учёт склада ФН выключен", "Обеспечено: 1/1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "купить") || strings.Contains(text, "на борту 0") {
		t.Fatal("disabled accounting produced a purchase instruction or repeated zeros", text)
	}
	row := v.Rows(v.At(s, "colonization", "activeProject", "operationalSupplies"))[0]
	if row["carrierAvailable"] != nil || v.Num(row["missing"]) != 10164 {
		t.Fatal("informational HUD stock changed the opted-out procurement calculation", row)
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("HUD mutated the source plan or inventory")
	}
}

func TestHUDColonyEnabledStockKeepsDeliveriesAndPurchasesDistinct(t *testing.T) {
	p, context, now := hudStockPlanner(t, []M{{"Name": "steel", "Name_Localised": "Сталь", "RequiredAmount": 100, "ProvidedAmount": 0}, {"Name": "titanium", "Name_Localised": "Титан", "RequiredAmount": 50, "ProvidedAmount": 0}}, []M{}, []M{{"commodity": "steel", "qty": 120}, {"commodity": "titanium", "qty": 10}}, nil)
	if err := p.Update(M{"action": "configure", "includeCarrierWarehouse": true}); err != nil {
		t.Fatal(err)
	}
	text := hudProse(BuildHUD(hudStockState(p, context), *now))
	for _, want := range []string{"Титан: купить 40 т", "Обеспечено: 1/2"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "Сталь:") || strings.Contains(text, "купить 0") || strings.Contains(text, "Учёт склада ФН выключен") {
		t.Fatal(text)
	}
}

func TestHUDColonyShowsTenUncoveredCommoditiesAndCorrectRemainder(t *testing.T) {
	all := []M{{"commodity": "steel", "name": "Сталь", "missing": 100, "cargoAvailable": 0, "carrierIncluded": true, "carrierAvailable": 100, "carrierComplete": true, "toBuy": 0}}
	for index := range 15 {
		all = append(all, M{"commodity": fmt.Sprintf("item%d", index), "name": fmt.Sprintf("Item%02d", index), "missing": 100, "cargoAvailable": 0, "carrierIncluded": true, "carrierAvailable": 0, "carrierComplete": true, "toBuy": 100})
	}
	state := M{"settings": M{"activity": "colonization"}, "colonization": M{"includeCarrierWarehouse": true, "activeProject": M{"name": "Synthetic", "status": "building", "operationalSupplies": all}}}
	before, _ := json.Marshal(state)
	doc := BuildHUD(state, hudFixtureNow)
	text := hudProse(doc)
	if strings.Contains(text, "Сталь:") || strings.Count(text, ": купить 100 т") != 10 || !strings.Contains(text, "Ещё позиций: 5") || !strings.Contains(text, "Обеспечено: 1/16") || len(doc.Sections) != 2 {
		t.Fatal("preview cap/filter/remainder failed", text)
	}
	after, _ := json.Marshal(state)
	if string(before) != string(after) {
		t.Fatal("HUD removed a covered resource from the desktop state")
	}
	v.Obj(state["colonization"])["includeCarrierWarehouse"] = false
	text = hudProse(BuildHUD(state, hudFixtureNow))
	if !strings.Contains(text, "Сталь:") || !strings.Contains(text, "Ещё позиций: 6") {
		t.Fatal("opt-out incorrectly removed carrier-covered cargo", text)
	}
}

func TestHUDColonyNeverCountsInaccessibleSquadronStock(t *testing.T) {
	p, context, now := hudStockPlanner(t, []M{{"Name": "steel", "Name_Localised": "Сталь", "RequiredAmount": 100, "ProvidedAmount": 0}}, []M{}, []M{}, []M{{"commodity": "steel", "qty": 9999}})
	if err := p.Update(M{"action": "configure", "includeCarrierWarehouse": true}); err != nil {
		t.Fatal(err)
	}
	text := hudProse(BuildHUD(hudStockState(p, context), *now))
	if strings.Contains(text, "9 999") || strings.Contains(text, "✓") || !strings.Contains(text, "купить 100 т") {
		t.Fatal("unconfirmed shared cargo covered the colony bill", text)
	}
	if err := p.Update(M{"action": "configure", "squadronWarehouseAccessConfirmed": true}); err != nil {
		t.Fatal(err)
	}
	text = hudProse(BuildHUD(hudStockState(p, context), *now))
	if !strings.Contains(text, "Обеспечено: 1/1") || strings.Contains(text, "Сталь:") || strings.Contains(text, "купить 100 т") {
		t.Fatal("accessible shared cargo remained invisible", text)
	}
}

func TestHUDColonyStaleWarehouseRemainsUnknownAndBoundsPurchases(t *testing.T) {
	p, context, now := hudStockPlanner(t, []M{{"Name": "steel", "Name_Localised": "Сталь", "RequiredAmount": 100, "ProvidedAmount": 0}}, []M{}, []M{{"commodity": "steel", "qty": 120}}, nil)
	if err := p.Update(M{"action": "configure", "includeCarrierWarehouse": true}); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(16 * time.Minute)
	text := hudProse(BuildHUD(hudStockState(p, context), *now))
	if strings.Contains(text, "✓") || strings.Contains(text, "120 т") || strings.Contains(text, "ФН: 0") || !strings.Contains(text, "Сталь: купить ≤100 т") || !strings.Contains(text, "Обеспечено: ?/1") {
		t.Fatal("stale carrier inventory became a current known zero or a covered bill", text)
	}
}

func TestHUDColonyPartialWarehouseShowsLowerBoundAndUncertainProcurement(t *testing.T) {
	s := M{"settings": M{"activity": "colonization"}, "colonization": M{"includeCarrierWarehouse": true, "carrierWarehouses": []M{{"kind": "fleet", "id": "42", "known": true, "accessible": true, "items": []M{{"key": "steel", "count": 30}}}, {"kind": "squadron", "id": "43", "known": false, "accessible": true}}, "activeProject": M{"name": "Synthetic", "status": "building", "operationalSupplies": []M{{"commodity": "steel", "name": "Steel", "missing": 100, "cargoAvailable": 10, "carrierIncluded": true, "carrierAvailable": 30, "carrierComplete": false, "toBuy": nil, "toBuyAtMost": 60}}}}}
	text := hudProse(BuildHUD(s, hudFixtureNow))
	if strings.Contains(text, "✓") || strings.Contains(text, "купить 60 т") || !strings.Contains(text, "Steel: купить ≤60 т") || !strings.Contains(text, "Обеспечено: ?/1") {
		t.Fatal("partial stock was shown as an exact procurement deficit", text)
	}
	v.Obj(s["settings"])["language"] = "en"
	if english := hudProse(BuildHUD(s, hudFixtureNow)); !strings.Contains(english, "Steel: buy ≤60 t") || !strings.Contains(english, "Stock covers: ?/1") {
		t.Fatal("partial procurement prose was not translated", english)
	}
	v.Obj(s["settings"])["language"] = "ru"
	row := v.Rows(v.At(s, "colonization", "activeProject", "operationalSupplies"))[0]
	row["cargoAvailable"], row["toBuyAtMost"] = nil, nil
	text = hudProse(BuildHUD(s, hudFixtureNow))
	if strings.Contains(text, "на борту 0") || strings.Contains(text, "купить") || strings.Contains(text, "✓") || !strings.Contains(text, "Steel: осталось 100 т · ФН: ≥30 т") {
		t.Fatal("unknown ship cargo became zero or an invented purchase amount", text)
	}
}

func TestHUDColonyKnownEmptyCarrierWithUnknownShipDoesNotBecomeUnknownCarrier(t *testing.T) {
	s := M{"settings": M{"activity": "colonization"}, "colonization": M{"includeCarrierWarehouse": true, "carrierWarehouses": []M{{"id": "42", "known": true, "accessible": true, "items": []M{}}}, "activeProject": M{"name": "Synthetic", "status": "building", "operationalSupplies": []M{{"commodity": "steel", "name": "Steel", "missing": 100, "cargoAvailable": nil, "carrierIncluded": true, "carrierAvailable": 0, "carrierComplete": true}}}}}
	text := hudProse(BuildHUD(s, hudFixtureNow))
	if !strings.Contains(text, "Steel: осталось 100 т") || strings.Contains(text, "ФН: ?") || strings.Contains(text, "купить") || strings.Contains(text, "✓") {
		t.Fatal("unknown ship cargo changed confirmed empty carrier stock", text)
	}
}

func TestHUDColonyDuplicateCarrierViewsDoNotCoverOneBillTwice(t *testing.T) {
	warehouse := func(kind string) M {
		return M{"kind": kind, "id": "42", "known": true, "accessible": true, "items": []M{{"key": "steel", "count": 50}}}
	}
	s := M{"settings": M{"activity": "colonization"}, "colonization": M{"includeCarrierWarehouse": true, "carrierWarehouses": []M{warehouse("fleet"), warehouse("squadron")}, "activeProject": M{"name": "Synthetic", "status": "building", "operationalSupplies": []M{{"commodity": "steel", "name": "Steel", "missing": 75, "cargoAvailable": 0, "carrierIncluded": true, "carrierAvailable": 50, "carrierComplete": true, "toBuy": 25}}}}}
	text := hudProse(BuildHUD(s, hudFixtureNow))
	if strings.Contains(text, "✓") || strings.Contains(text, "ФН: 100") || !strings.Contains(text, "Steel: купить 25 т") {
		t.Fatal("the same carrier ID was counted twice", text)
	}
}

func TestHUDColonySupplySummaryIsEnglishWithoutTranslatingUserNames(t *testing.T) {
	p, context, now := hudStockPlanner(t, []M{{"Name": "steel", "Name_Localised": "Сталь", "RequiredAmount": 100, "ProvidedAmount": 0}}, []M{}, []M{{"commodity": "steel", "qty": 100}}, nil)
	s := hudStockState(p, context)
	v.Obj(s["settings"])["language"] = "en"
	text := hudProse(BuildHUD(s, *now))
	for _, want := range []string{"FC: 100 t", "Stock covers: 1/1", "FC stock accounting is off", "Synthetic HUD Colony"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing translated phrase %q: %s", want, text)
		}
	}
	for _, wrong := range []string{"ФН", "Обеспечено", "выключен", "Загрузить"} {
		if strings.Contains(text, wrong) {
			t.Fatal("untranslated HUD supply prose", text)
		}
	}
}

func TestHUDColonyWithoutAnObservedBillDoesNotInventEmptyCoverage(t *testing.T) {
	s := M{"settings": M{"activity": "colonization"}, "colonization": M{"includeCarrierWarehouse": true, "activeProject": M{"name": "Synthetic", "status": "building", "unknownBills": 1}}}
	doc := BuildHUD(s, hudFixtureNow)
	if text := hudProse(doc); strings.Contains(text, "0/0") || strings.Contains(text, "Обеспечено") || !strings.Contains(text, "Сметы неизвестны: 1") {
		t.Fatal("an unread bill became an empty covered bill", text)
	}
}

func TestHUDColonyHidesCarrierCoveredRowsAndTheirOldNextAction(t *testing.T) {
	p, context, now := hudStockPlanner(t, []M{{"Name": "steel", "RequiredAmount": 100, "ProvidedAmount": 0}}, []M{{"Name": "steel", "Count": 5}}, []M{{"commodity": "steel", "qty": 100}}, nil)
	if err := p.Update(M{"action": "configure", "includeCarrierWarehouse": true}); err != nil {
		t.Fatal(err)
	}
	s := hudStockState(p, context)
	v.Obj(v.At(s, "colonization", "activeProject"))["nextTask"] = M{"text": "Steel: load 100"}
	text := hudProse(BuildHUD(s, *now))
	if strings.Contains(text, "Сталь") || strings.Contains(text, "Steel") || !strings.Contains(text, "доставка на стройки ещё нужна") || !strings.Contains(text, "Обеспечено: 1/1") {
		t.Fatal("a suppressed resource leaked into another HUD panel or became delivered", text)
	}
	v.Obj(s["settings"])["language"] = "en"
	text = hudProse(BuildHUD(s, *now))
	if strings.Contains(text, "Ресурсы") || !strings.Contains(text, "construction deliveries still needed") {
		t.Fatal("all-covered HUD not localized", text)
	}
}

func TestHUDColonyKeepsLoadedShipCargoWhenCarrierAloneIsInsufficient(t *testing.T) {
	p, context, now := hudStockPlanner(t, []M{{"Name": "steel", "RequiredAmount": 100, "ProvidedAmount": 0}}, []M{{"Name": "steel", "Count": 65}}, []M{{"commodity": "steel", "qty": 35}}, nil)
	if err := p.Update(M{"action": "configure", "includeCarrierWarehouse": true}); err != nil {
		t.Fatal(err)
	}
	s := hudStockState(p, context)
	text := hudProse(BuildHUD(s, *now))
	if !strings.Contains(text, "Сталь: доставить 65 т") || strings.Contains(text, "купить 0") {
		t.Fatal("combined coverage hid cargo that still needs ship delivery", text)
	}
	v.Obj(s["settings"])["language"] = "en"
	if text = hudProse(BuildHUD(s, *now)); !strings.Contains(text, "Steel: deliver 65 t") {
		t.Fatal("delivery action not localized", text)
	}
}
