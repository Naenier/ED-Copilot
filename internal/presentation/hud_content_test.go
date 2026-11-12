package presentation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/colonization"
	"github.com/Naenier/ED-Copilot/internal/mining"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

var hudFixtureNow = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func hudSection(doc HUDDocument, title string) *HUDSection {
	for i := range doc.Sections {
		if doc.Sections[i].Title == title {
			return &doc.Sections[i]
		}
	}
	return nil
}
func hudHasWarning(doc HUDDocument, part string) bool {
	for _, s := range doc.Sections {
		for _, line := range s.Lines {
			if line.Role == "warn" && strings.Contains(hudLineProse(line), part) {
				return true
			}
		}
	}
	return false
}
func hudTarget(system, body string, distance any, raw bool) M {
	m := M{"kind": "surface", "system": system, "body": body, "distanceLy": distance, "landable": true, "tier": "medium", "confidence": "confirmed", "miningSites": 2}
	if raw {
		m["miningSites"] = 0
		m["materials"] = []M{{"key": "polonium", "name": "Полоний", "grade": 4}}
		m["methods"] = []string{"raw"}
	}
	return m
}

func TestHUDIdleActivitiesDoNotReserveAHeaderPanel(t *testing.T) {
	for _, activity := range Activities {
		t.Run(activity.Value, func(t *testing.T) {
			s := M{"settings": M{"activity": activity.Value}, "commander": M{"system": "Synthetic System", "station": "Synthetic Port", "body": "Synthetic Port", "docked": true, "vehicle": "ship", "ship": "Synthetic Ship Model", "shipName": "Synthetic Personal Name"}, "tradePlan": M{"key": "gold", "quantity": 0, "grossMargin": 0}, "engineering": M{"items": []M{}, "shopping": M{"neededUnits": 0, "missingUnits": 0}}}
			doc := BuildHUD(s, hudFixtureNow)
			text := hudProse(doc)
			if len(doc.Sections) != 0 {
				t.Fatal("idle overlay reserves space without a working panel", doc)
			}
			for _, forbidden := range []string{"Synthetic Ship Model", "Synthetic Personal Name", "Добавьте", "Запустите дрон", "Найдите рынок", "По плану нужно: 0"} {
				if strings.Contains(text, forbidden) {
					t.Fatal("idle guidance obscures the game", activity.Value, forbidden)
				}
			}
		})
	}
}
func TestHUDWorkingPanelRetainsCompactLocationWithoutStaleSpaceBody(t *testing.T) {
	s := M{"settings": M{"activity": "flight"}, "commander": M{"system": "Alpha", "body": "Old Body", "station": "Old Station", "vehicle": "ship", "docked": false}, "pilot": M{"fuel": M{"mainTons": 10}}}
	doc := BuildHUD(s, hudFixtureNow)
	if len(doc.Sections) != 1 || doc.Sections[0].Title != "ТОПЛИВО И ГРУЗ" || doc.Sections[0].Subtitle != "Alpha" || len(doc.Sections[0].Lines) != 1 || strings.Contains(hudProse(doc), "Old") {
		t.Fatal("location was lost or stale space body leaked into working panel", doc)
	}
	for _, vehicle := range []string{"srv", "foot"} {
		v.Obj(s["commander"])["vehicle"] = vehicle
		if panel := BuildHUD(s, hudFixtureNow).Sections[0]; panel.Subtitle != "Alpha · Old Body" {
			t.Fatal("surface location was lost", vehicle, panel)
		}
	}
}
func TestHUDNavigationPrioritizesNextUnvisitedStepAndKnownFuel(t *testing.T) {
	s := M{"settings": M{"activity": "flight"}, "commander": M{"system": "Alpha", "cargoTons": 0, "cargoCapacity": 64}, "pilot": M{"navTarget": M{"system": "Alpha"}, "navRoute": []M{{"system": "Alpha"}, {"system": "Beta", "starClass": "G"}}, "fuel": M{"mainTons": 0, "mainCapacityTons": 16}}, "objectives": []M{{"title": "Unrelated goal", "system": "Gamma", "status": "active", "priority": "high"}}}
	doc := BuildHUD(s, hudFixtureNow)
	text := hudProse(doc)
	if len(doc.Sections) < 2 || doc.Sections[0].Title != "СЛЕДУЮЩИЙ ПРЫЖОК" || strings.Count(text, "Beta") != 1 || strings.Contains(text, "Unrelated goal") || !hudHasWarning(doc, "Топливо: 0 / 16") {
		t.Fatal("flight action priority or zero-fuel warning lost", doc)
	}
	v.Obj(v.At(s, "pilot", "fuel"))["mainTons"] = nil
	doc = BuildHUD(s, hudFixtureNow)
	if strings.Contains(hudProse(doc), "Топливо:") || hudHasWarning(doc, "Топливо") {
		t.Fatal("unknown fuel became empty tank", doc)
	}
}
func TestHUDDockedNavigationModeDoesNotClaimTheShipIsFlying(t *testing.T) {
	s := M{"settings": M{"activity": "flight"}, "commander": M{"system": "Synthetic System", "station": "Synthetic Port", "docked": true, "cargoTons": 0}, "pilot": M{"fuel": M{"mainTons": 12, "mainCapacityTons": 16}}}
	doc := BuildHUD(s, hudFixtureNow)
	if hudSection(doc, "ТОПЛИВО И ГРУЗ") == nil || strings.Contains(hudProse(doc), "В ПОЛЁТЕ") {
		t.Fatal("docked fuel panel claims an active flight", doc)
	}
}

func TestHUDCartographyShowsEveryLocalTargetAndNextStop(t *testing.T) {
	progress := M{
		"currentSystem": "Торговля", "completedJumps": 3, "remainingJumps": 4, "nextSystem": "Станция", "totalMappingCredits": 2750000, "totalScanCredits": 575000,
		"targets": []M{
			{"name": "Торговля A 1", "designation": "A 1", "distanceLs": 550, "mappingCredits": 1250000, "scanCredits": 275000, "completed": true},
			{"name": "Торговля A 2", "designation": "A 2", "mappingCredits": 1500000},
			{"name": "Груз A 3", "mappingCredits": 1750000},
		},
	}
	s := M{
		"settings":  M{"activity": "cartography", "language": "en"},
		"commander": M{"system": "Торговля"},
		"pilot":     M{"navTarget": M{"system": "Unrelated"}, "fuel": M{"mainTons": 5}},
		"riches":    M{"progress": progress},
	}
	doc := BuildHUD(s, hudFixtureNow)
	card := hudSection(doc, "CARTOGRAPHY")
	if card == nil || card.Subtitle != "Торговля" || len(card.Lines) != 3 ||
		card.Lines[0].Text != "✓ A 1 · 550 ls" || card.Lines[0].Role != "muted" || card.Lines[0].Value != "≈ 1 250 000 Cr" ||
		card.Lines[1].Text != "A 2" || card.Lines[2].Text != "Груз A 3" {
		t.Fatalf("cartography lost a current-system target %+v", doc)
	}
	summary := hudSection(doc, "CARTOGRAPHY ROUTE")
	if summary == nil || len(summary.Lines) != 3 || summary.Lines[0].Value != "Станция" ||
		summary.Lines[1].Value != "≈ 2 750 000 Cr" || summary.Lines[2].Value != "3 / ≈ 4" {
		t.Fatalf("incorrect cartography route summary %+v", doc)
	}
	if strings.Contains(hudProse(doc), "Unrelated") || strings.Contains(hudProse(doc), "Fuel") {
		t.Fatalf("unrelated flight content appears in cartography %+v", doc)
	}

	progress["useMappingValue"] = false
	doc = BuildHUD(s, hudFixtureNow)
	if card, route := hudSection(doc, "CARTOGRAPHY"), hudSection(doc, "CARTOGRAPHY ROUTE"); card == nil || route == nil ||
		card.Lines[0].Value != "≈ 275 000 Cr" || route.Lines[1].Value != "≈ 575 000 Cr" {
		t.Fatalf("FSS route should show scan estimates %+v", doc)
	}

	progress["finished"] = true
	delete(progress, "nextSystem")
	doc = BuildHUD(s, hudFixtureNow)
	if len(doc.Sections) != 1 || doc.Sections[0].Title != "CARTOGRAPHY" || len(doc.Sections[0].Lines) != 3 {
		t.Fatalf("target list disappeared on route completion %+v", doc)
	}

	v.Obj(s["settings"])["activity"] = "flight"
	doc = BuildHUD(s, hudFixtureNow)
	if hudSection(doc, "CARTOGRAPHY") != nil || !strings.Contains(hudProse(doc), "Unrelated") || !strings.Contains(hudProse(doc), "Fuel") {
		t.Fatalf("flight HUD did not retain independent navigation %+v", doc)
	}
}

func TestHUDCartographyCanShowAnUpcomingSystemWithoutLocalTargets(t *testing.T) {
	s := M{
		"settings": M{"activity": "cartography", "language": "en"},
		"riches":   M{"progress": M{"nextSystem": "Beta", "targets": []M{}}},
	}
	doc := BuildHUD(s, hudFixtureNow)
	card := hudSection(doc, "CARTOGRAPHY")
	if card == nil || len(doc.Sections) != 1 || len(card.Lines) != 1 || card.Lines[0].Value != "Beta" {
		t.Fatalf("waypoint navigation was lost while in transit %+v", doc)
	}
}
func TestHUDAsteroidCurrentContextValuableCompositionAndPreparation(t *testing.T) {
	s := M{"settings": M{"activity": "asteroid", "minTier": "medium"}, "commander": M{"system": "Alpha", "body": "Alpha 1", "cargoTons": 0, "cargoCapacity": 100}, "mining": M{"cargo": M{"limpets": nil}, "current": M{"refinedTons": 0, "tonsPerHour": 0}}, "prospect": M{"system": "Alpha", "body": "Alpha 1", "decision": "mine", "remaining": 80, "materials": []M{{"name": "Cheap water", "tier": "low", "percent": 70}, {"name": "Useful ore", "tier": "high", "percent": 5}}}}
	doc := BuildHUD(s, hudFixtureNow)
	scan := hudSection(doc, "СКАН АСТЕРОИДА")
	if scan == nil || doc.Sections[0].Title != scan.Title || !strings.Contains(hudProse(doc), "Useful ore") || strings.Contains(hudProse(doc), "Cheap water") || hudHasWarning(doc, "Дронов") || strings.Contains(hudProse(doc), "За заход: 0") {
		t.Fatal("current useful scan or unknown prep was distorted", doc)
	}
	v.Obj(v.At(s, "mining", "cargo"))["limpets"] = 0
	if !hudHasWarning(BuildHUD(s, hudFixtureNow), "Дронов в трюме нет") {
		t.Fatal("known missing limpets did not trigger preparation warning")
	}
	v.Obj(s["prospect"])["remaining"] = 0
	scan = hudSection(BuildHUD(s, hudFixtureNow), "СКАН АСТЕРОИДА")
	if scan == nil || scan.Lines[0].Role != "warn" || strings.Contains(scan.Lines[0].Text, "Можно добывать") {
		t.Fatal("exhausted asteroid still recommended", scan)
	}
	for _, change := range []M{{"docked": true}, {"system": "Other"}, {"body": "Other body"}} {
		copy := v.Copy(s)
		v.Merge(v.Obj(copy["commander"]), change)
		if hudSection(BuildHUD(copy, hudFixtureNow), "СКАН АСТЕРОИДА") != nil {
			t.Fatal("out-of-context old scan remains visible", change)
		}
	}
}
func TestHUDMiningDoesNotShowUnrelatedOrEmptyCargoDestination(t *testing.T) {
	s := M{"settings": M{"activity": "asteroid"}, "commander": M{"cargoTons": 5, "cargo": []M{{"key": "drones", "count": 5}}}, "trading": M{"destination": M{"station": "Buyer", "system": "Remote", "updatedAt": v.ISO(hudFixtureNow), "quotes": []M{{"key": "silver", "name": "Uncarried silver", "price": 200, "demand": 100}, {"key": "gold", "name": "Carried gold", "price": 100, "demand": 100}}}}}
	if strings.Contains(hudProse(BuildHUD(s, hudFixtureNow)), "Buyer") {
		t.Fatal("limpets were treated as matching sell cargo")
	}
	v.Obj(s["commander"])["cargo"] = []M{{"key": "gold", "count": 5}}
	if text := hudProse(BuildHUD(s, hudFixtureNow)); !strings.Contains(text, "Buyer") || !strings.Contains(text, "Carried gold") || strings.Contains(text, "Uncarried silver") {
		t.Fatal("mining sale card failed to isolate carried goods", text)
	}
	v.Obj(v.At(s, "trading", "destination"))["selectionMode"] = "buy"
	if strings.Contains(hudProse(BuildHUD(s, hudFixtureNow)), "Buyer") {
		t.Fatal("unrelated buying station polluted mining overlay")
	}
}
func TestHUDPreservesAssessedUncrackedCoreWithZeroLaserRemainder(t *testing.T) {
	prospect := mining.AssessProspect(M{"remaining": 0, "motherlode": "monazite", "materials": []M{}}, M{"method": "core", "minTier": "medium"}, trading.Resources())
	if prospect["decision"] != "mine" {
		t.Fatal("fixture does not represent an assessed mineable core", prospect)
	}
	s := M{"settings": M{"activity": "asteroid"}, "goods": []M{{"key": "monazite", "name": "Монацит"}}, "prospect": prospect}
	text := hudProse(BuildHUD(s, hudFixtureNow))
	if !strings.Contains(text, "Ядро: Монацит") || strings.Contains(text, "Астероид истощён") {
		t.Fatal("laser depletion overrode a valid core-mining assessment", text)
	}
}
func TestHUDSurfaceChoosesCurrentSiteAndUsesSRVCargoThenNearbyMiningOnly(t *testing.T) {
	s := M{"settings": M{"activity": "surface", "radiusLy": 50}, "commander": M{"system": "Alpha", "body": "Alpha 1", "vehicle": "srv", "cargoTons": 3, "cargoCapacity": 128, "srvCargoCapacity": 4}, "surface": M{"site": "Current worksite", "system": "Alpha", "body": "Alpha 1", "lastRefined": []M{{"name": "Ore", "count": 3}}}, "targets": []M{hudTarget("Far", "Far planet", 9500, false), hudTarget("Nearby", "Raw-only planet", 10, true), hudTarget("Nearby", "Useful mining planet", 25, false), hudTarget("Unknown", "Unknown distance", nil, false)}}
	doc := BuildHUD(s, hudFixtureNow)
	text := hudProse(doc)
	if hudSection(doc, "ТЕКУЩИЙ УЧАСТОК") == nil || strings.Contains(text, "МЕСТА ПОБЛИЗОСТИ") || !strings.Contains(text, "3 / 4 т") || strings.Contains(text, "128") {
		t.Fatal("current site or SRV cargo is wrong", doc)
	}
	v.Obj(s["surface"])["system"] = "Elsewhere"
	doc = BuildHUD(s, hudFixtureNow)
	text = hudProse(doc)
	if !strings.Contains(text, "Useful mining planet") || strings.Contains(text, "Current worksite") || strings.Contains(text, "Far planet") || strings.Contains(text, "Raw-only planet") || strings.Contains(text, "Unknown distance") {
		t.Fatal("surface target context/distance/type filter failed", doc)
	}
}
func TestHUDRawNeedsKnownStockAndTargetsMatchingTheDeficit(t *testing.T) {
	s := M{"settings": M{"activity": "raw", "radiusLy": 40}, "commander": M{"system": "Alpha"}, "engineering": M{"items": []M{{"moduleKey": "fsd"}}, "materials": []M{{"key": "polonium", "name": "Полоний", "category": "raw", "needed": 10, "owned": 0, "missing": 10}, {"key": "zirconium", "name": "Unknown stock", "category": "raw", "needed": 100, "owned": nil, "missing": nil}}, "shopping": M{"knownCategories": M{"raw": true}}}, "targets": []M{hudTarget("Far", "Far raw body", 9500, true), hudTarget("Close", "Needed raw body", 20, true), hudTarget("Close", "Commodity site", 10, false)}}
	wrong := hudTarget("Close", "Wrong material", 5, true)
	wrong["materials"] = []M{{"key": "carbon", "name": "Углерод"}}
	unknown := hudTarget("Close", "Unknown stock needed body", 15, true)
	unknown["materials"] = []M{{"key": "zirconium", "name": "Цирконий"}}
	s["targets"] = append(v.Rows(s["targets"]), wrong, unknown)
	doc := BuildHUD(s, hudFixtureNow)
	text := hudProse(doc)
	if !strings.Contains(text, "Полоний: 10") || !hudHasWarning(doc, "Unknown stock: ≤ 100") || !hudHasWarning(doc, "Запасы сырья") || !strings.Contains(text, "Needed raw body") || !strings.Contains(text, "Unknown stock needed body") || strings.Contains(text, "Far raw body") || strings.Contains(text, "Wrong material") || strings.Contains(text, "Commodity site") {
		t.Fatal("raw stock or useful local targets distorted", doc)
	}
	v.Obj(s["engineering"])["materials"] = []M{{"category": "manufactured", "needed": 10, "missing": 10, "owned": 0}}
	if hudSection(BuildHUD(s, hudFixtureNow), "НУЖНОЕ СЫРЬЁ") != nil {
		t.Fatal("raw mode reported completion/unknown stocks for unrelated manufactured plan")
	}
}
func TestHUDNearbyTargetsPreferCurrentSystemThenDistanceScoreAndStableID(t *testing.T) {
	current := hudTarget("Alpha", "Current body", nil, false)
	current["score"] = 1
	nearLow := hudTarget("Close", "Lower score", 5, false)
	nearLow["score"], nearLow["id"] = 50, "b"
	nearBest := hudTarget("Close", "Higher score stable first", 5, false)
	nearBest["score"], nearBest["id"] = 90, "a"
	nearTie := hudTarget("Close", "Higher score stable second", 5, false)
	nearTie["score"], nearTie["id"] = 90, "z"
	farHigh := hudTarget("Farther", "Further high score", 15, false)
	farHigh["score"] = 100
	cheap := hudTarget("Alpha", "Low value site", 0, false)
	cheap["tier"] = "low"
	// Confirmed surface-mining sites can carry an unknown/non-landable catalog
	// flag; the observed activity, unlike raw gathering, is sufficient evidence.
	current["landable"] = false
	s := M{"settings": M{"activity": "surface", "minTier": "low"}, "commander": M{"system": "Alpha"}, "targets": []M{farHigh, nearTie, nearLow, current, cheap, nearBest}}
	doc := BuildHUD(s, hudFixtureNow)
	section := hudSection(doc, "МЕСТА ПОБЛИЗОСТИ")
	if section == nil || len(section.Lines) != 2 || section.Lines[0].Text != "Current body" || !strings.Contains(section.Lines[1].Text, "Higher score stable first") || !strings.Contains(section.Lines[1].Value, "5 св. л.") || strings.Contains(hudProse(doc), "Low value site") {
		t.Fatal("nearby target priority/value boundary changed", doc)
	}
}
func TestHUDRawCompletionDoesNotClaimUnfinishedManufacturedPlanIsComplete(t *testing.T) {
	s := M{"settings": M{"activity": "raw"}, "engineering": M{"items": []M{{"moduleKey": "fsd"}}, "materials": []M{{"key": "polonium", "category": "raw", "needed": 10, "owned": 10, "missing": 0}, {"key": "chemicalprocessors", "category": "manufactured", "needed": 5, "owned": 0, "missing": 5}}, "shopping": M{"neededUnits": 15, "missingUnits": 5, "knownCategories": M{"raw": true}}}}
	text := hudProse(BuildHUD(s, hudFixtureNow))
	if !strings.Contains(text, "Сырьё для плана собрано") || strings.Contains(text, "Материалы плана собраны") {
		t.Fatal("raw completion overstated the whole engineering plan", text)
	}
}
func TestHUDKnownAbsentCargoBayIsNotReportedAsFull(t *testing.T) {
	s := M{"settings": M{"activity": "asteroid"}, "commander": M{"cargoTons": 0, "cargoCapacity": 0}}
	doc := BuildHUD(s, hudFixtureNow)
	if !hudHasWarning(doc, "Грузовой трюм отсутствует") || strings.Contains(hudProse(doc), "Трюм полон") {
		t.Fatal("absent bay was confused with a full loaded hold", doc)
	}
}
func TestHUDTradeArrivalKeepsOneStationAndSelectedCommodityWarnings(t *testing.T) {
	s := M{"settings": M{"activity": "trade"}, "commander": M{"system": "Alpha", "station": "Buyer", "body": "Buyer", "docked": true}, "tradePlan": M{"key": "gold", "quantity": 40, "name": "Золото", "grossMargin": nil}, "trading": M{"destination": M{"station": "Buyer", "system": "Alpha", "selectionMode": "buy", "cargoChanged": true, "updatedAt": nil, "quotes": []M{{"key": "silver", "name": "Silver", "price": 100, "supply": 1000}, {"key": "copper", "name": "Copper", "price": 200, "supply": 1000}, {"key": "gold", "name": "Золото", "price": nil, "supply": 0, "demand": 999}}}}}
	doc := BuildHUD(s, hudFixtureNow)
	text := hudProse(doc)
	if strings.Count(text, "Buyer") != 1 || !strings.Contains(text, "НА МЕСТЕ") || !strings.Contains(text, "Золото: запас 0 т") || strings.Contains(text, "Copper") || strings.Contains(text, "спрос 999") || strings.Contains(text, "Плановая маржа: 0") {
		t.Fatal("buy/arrival/current plan quote context failed", doc)
	}
	if !hudHasWarning(doc, "Время цены неизвестно") || !hudHasWarning(doc, "Состав груза изменился") {
		t.Fatal("quote/cargo risks were hidden", doc)
	}
}
func TestHUDCarrierPendingJumpFuelSafetyAndStalePlanIsolation(t *testing.T) {
	s := M{"settings": M{"activity": "carrier"}, "commander": M{"system": "Alpha"}, "carrier": M{"owned": true, "callsign": "TEST-1", "location": M{"system": "Alpha"}, "fuel": M{"tankTons": 0, "reserveTons": 100}, "jump": M{"pending": M{"system": "Beta", "departureTime": v.ISO(hudFixtureNow.Add(5 * time.Minute))}}}, "carrierRoute": M{"progress": M{"nextIndex": 1}, "route": M{"stops": []M{{"name": "Alpha", "visited": true}, {"name": "Beta", "fuelUsedTons": 92, "distanceLy": 450}}}, "summary": M{"remainingJumps": 1}}}
	doc := BuildHUD(s, hudFixtureNow)
	if strings.Count(hudProse(doc), "Beta") != 1 || doc.Sections[0].Title != "ПРЫЖОК ФЛОТОНОСЦА" || !strings.Contains(hudProse(doc), "До отправления: 5 мин") || !hudHasWarning(doc, "Перелить в бак: 92") {
		t.Fatal("pending action, de-duplication or known fuel prep failed", doc)
	}
	doc = BuildHUD(s, hudFixtureNow.Add(6*time.Minute))
	if hudHasWarning(doc, "Перелить") || !strings.Contains(hudProse(doc), "Ожидаем подтверждение прибытия") {
		t.Fatal("past departure kept advising a transfer for the pending jump", doc)
	}
	v.Obj(v.At(s, "carrier", "fuel"))["tankTons"] = nil
	if hudHasWarning(BuildHUD(s, hudFixtureNow), "Перелить") {
		t.Fatal("unknown tank generated a guessed transfer")
	}
	v.Obj(v.At(s, "carrier", "fuel"))["tankTons"] = 0
	v.Obj(v.At(s, "carrier", "fuel"))["needsRefresh"] = true
	doc = BuildHUD(s, hudFixtureNow)
	if hudHasWarning(doc, "Перелить") || !hudHasWarning(doc, "Ожидаем данные о баке") || strings.Contains(hudProse(doc), "Тритий в баке") {
		t.Fatal("stale inventory was treated as fresh fuel")
	}
	v.Obj(v.At(s, "carrier", "fuel"))["needsRefresh"] = false
	v.Obj(s["carrierRoute"])["error"] = "Synthetic route failure"
	doc = BuildHUD(s, hudFixtureNow)
	if hudHasWarning(doc, "Перелить") || strings.Contains(hudProse(doc), "Дальность и расход") || !hudHasWarning(doc, "Synthetic route failure") {
		t.Fatal("failed route gave instructions from an old plan", doc)
	}
}

func TestHUDEnglishCarrierKeepsTargetIdentityAndLocalizesPairedFacts(t *testing.T) {
	system := "Станция"
	s := M{"settings": M{"activity": "carrier", "language": "en"},
		"carrier":      M{"owned": true, "fuel": M{"tankTons": 0, "reserveTons": 100}, "jump": M{"pending": M{"system": system, "departureTime": v.ISO(hudFixtureNow.Add(5 * time.Minute))}}},
		"carrierRoute": M{"progress": M{"nextIndex": 1}, "route": M{"stops": []M{{"name": "Alpha"}, {"name": system, "fuelUsedTons": 92, "distanceLy": 450}}}}}
	doc := BuildHUD(s, hudFixtureNow)
	got := hudProse(doc)
	for _, want := range []string{"CARRIER JUMP", system, "Departure in: 5 min", "Transfer to tank: 92 t", "Distance and fuel use: 450 ly · 92 t"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestHUDCarrierAccountFiguresStayWithFuelDuringRoute(t *testing.T) {
	s := M{"settings": M{"activity": "carrier"}, "carrier": M{"owned": true,
		"fuel":    M{"tankTons": 742, "reserveTons": 2079},
		"finance": M{"balance": -1200, "weeklyMaintenance": 8300000, "weeklyMaintenanceEstimated": true},
		"space":   M{"capacityTons": 25000, "freeTons": 18000}},
		"carrierRoute": M{"progress": M{"nextIndex": 1}, "route": M{"stops": []M{{"name": "Sothis"}, {"name": "HIP 19510", "fuelUsedTons": 87}}}}}
	doc := BuildHUD(s, hudFixtureNow)
	panel := hudSection(doc, "ФЛОТОНОСЕЦ")
	if len(doc.Sections) != 2 || panel == nil || len(panel.Lines) != 5 {
		t.Fatalf("expected five lines in the carrier card: %#v", doc)
	}
	for index, want := range []string{"Тритий в баке: 742 т", "Тритий на складе: 2 079 т", "Баланс носителя: -1 200 кр.", "Содержание в неделю: ≈ 8 300 000 кр.", "Вместимость: 7 000 / 25 000 т"} {
		if got := hudLineProse(panel.Lines[index]); got != want {
			t.Errorf("line %d = %q, want %q", index, got, want)
		}
	}
}
func TestHUDColonyPrioritizesCargoAndNeverAggregatesOtherProjects(t *testing.T) {
	first := M{"name": "High-priority empty item", "commodity": "steel", "missing": 500, "cargoAvailable": 0, "authoritative": true}
	loaded := M{"name": "Already loaded item", "commodity": "titanium", "missing": 20, "cargoAvailable": 10, "authoritative": true}
	p := M{"id": "chosen", "name": "Chosen colony", "targetSystem": "New Alpha", "status": "building", "unknownBills": 2, "nextTask": M{"text": "Buy 500 units first"}, "operationalSupplies": []M{first, loaded}}
	s := M{"settings": M{"activity": "colonization"}, "colonization": M{"activeProject": p, "projects": []M{p, {"name": "Other colony", "operationalSupplies": []M{{"name": "Other supply", "missing": 9999}}}}, "aggregateSupplies": []M{{"name": "Global supplies", "missing": 5000}}}}
	before, _ := json.Marshal(s)
	doc := BuildHUD(s, hudFixtureNow)
	text := hudProse(doc)
	hudCheckAction(t, doc, "ДОСТАВИТЬ", "Already loaded item", "10 т")
	if doc.Sections[0].Kind != "delivery" || strings.Contains(text, "Buy 500 units first") || strings.Contains(text, "Other colony") || strings.Contains(text, "Global supplies") || !hudHasWarning(doc, "Сметы неизвестны: 2") {
		t.Fatal("colony cargo priority or isolation failed", doc)
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("HUD reordered source supply priorities")
	}
	p["status"] = "complete"
	doc = BuildHUD(s, hudFixtureNow)
	if len(doc.Sections) != 0 {
		t.Fatal("completed colony still advertises active deliveries", doc)
	}
}
func TestHUDEngineeringDisplaysOnlyPlannedDeficitsAndFeasibleExchange(t *testing.T) {
	s := M{"settings": M{"activity": "engineering"}, "engineering": M{"items": []M{{"moduleKey": "fsd"}}, "materials": []M{{"name": "Small deficit", "needed": 2, "owned": 0, "missing": 2}, {"name": "Large deficit", "needed": 50, "owned": 10, "missing": 40}, {"name": "Unrelated zero", "needed": 0, "owned": 0, "missing": 0}}, "shopping": M{"neededUnits": 52}, "trader": M{"offers": []M{{"fromName": "Impossible", "toName": "Target", "requiredFrom": 6, "received": 1, "affordable": true, "storageFits": false}, {"fromName": "Available", "toName": "Needed", "requiredFrom": 6, "received": 1, "affordable": true, "storageFits": true}}}}}
	doc := BuildHUD(s, hudFixtureNow)
	section := hudSection(doc, "МАТЕРИАЛЫ ДЛЯ УЛУЧШЕНИЙ")
	if section == nil || !strings.Contains(section.Lines[0].Text, "Large deficit") || strings.Contains(hudProse(doc), "Unrelated zero") || strings.Contains(hudProse(doc), "Impossible") || !strings.Contains(hudProse(doc), "Available") {
		t.Fatal("engineering deficit/feasibility priority failed", doc)
	}
	v.Obj(s["engineering"])["items"] = []M{}
	if len(BuildHUD(s, hudFixtureNow).Sections) != 0 {
		t.Fatal("unconfigured engineering plan still occupies the overlay")
	}
}
func TestHUDColonyPrimaryDeadlineRespectsOwnedClaimCompletionEvidence(t *testing.T) {
	claimAt := hudFixtureNow.Add(-48 * time.Hour)
	beaconAt := claimAt.Add(time.Hour)
	completionAt := claimAt.Add(24 * time.Hour)
	deadline := v.ISO(claimAt.Add(28 * 24 * time.Hour))
	project := M{"id": "synthetic-owned", "status": "building", "name": "Synthetic growing colony", "claimedAt": v.ISO(claimAt), "deadline": deadline, "journalOwnership": M{"source": "journal-claim", "claimedAt": v.ISO(claimAt), "beaconAt": v.ISO(beaconAt), "status": "beacon"}, "claimObservation": M{"claimedAt": v.ISO(claimAt), "beaconAt": v.ISO(beaconAt), "status": "beacon"}, "buildings": []M{{"typeId": "custom", "journalStationName": "Orbital Construction Site: Synthetic Initial Port", "status": "complete", "effectiveStatus": "complete", "journalDiscovered": true, "journalStatus": "complete", "journalUpdatedAt": v.ISO(completionAt)}, {"typeId": "custom", "status": "building", "journalDiscovered": true, "journalStatus": "building", "journalUpdatedAt": v.ISO(hudFixtureNow)}}}
	state := func(p M) M {
		return M{"settings": M{"activity": "colonization"}, "colonization": M{"activeProject": p}}
	}
	hasDeadline := func(p M) bool {
		return strings.Contains(hudProse(BuildHUD(state(p), hudFixtureNow)), "Срок первичного порта:")
	}
	if hasDeadline(project) {
		t.Fatal("completed owned initial phase retained the primary deadline during secondary construction")
	}
	for name, change := range map[string]func(M){
		"unfinished initial depot": func(p M) {
			b := v.Rows(p["buildings"])[0]
			b["status"], b["effectiveStatus"], b["journalStatus"] = "building", "building", "building"
		},
		"completion predates current claim": func(p M) {
			v.Rows(p["buildings"])[0]["journalUpdatedAt"] = v.ISO(claimAt.Add(-time.Hour))
		},
		"explicit primary observation predates current claim": func(p M) {
			b := v.Rows(p["buildings"])[0]
			b["isPrimary"], b["journalUpdatedAt"] = true, v.ISO(claimAt.Add(-time.Hour))
		},
		"completion predates current beacon": func(p M) {
			v.Rows(p["buildings"])[0]["journalUpdatedAt"] = v.ISO(claimAt.Add(30 * time.Minute))
		},
		"no own claim evidence": func(p M) { p["journalOwnership"] = nil },
		"manual secondary completion": func(p M) {
			v.Rows(p["buildings"])[0]["journalDiscovered"] = false
		},
		"no journal completion confirmation": func(p M) {
			v.Rows(p["buildings"])[0]["journalStatus"] = "building"
		},
		"unknown completion date": func(p M) {
			v.Rows(p["buildings"])[0]["journalUpdatedAt"] = nil
		},
		"generic completed depot": func(p M) {
			v.Rows(p["buildings"])[0]["journalStationName"] = "Synthetic Unidentified Depot"
		},
		"colonisation ship": func(p M) {
			b := v.Rows(p["buildings"])[0]
			b["journalStationName"], b["isPrimary"] = "$EXT_PANEL_ColonisationShip; Synthetic Ship", true
		},
	} {
		t.Run(name, func(t *testing.T) {
			copy := v.Copy(project)
			change(copy)
			if !hasDeadline(copy) {
				t.Fatal("unconfirmed/old/unrelated completion hid an unfinished primary deadline")
			}
		})
	}
	manual := v.Copy(project)
	manual["journalOwnership"], manual["buildings"] = nil, []M{{"status": "complete", "isPrimary": true}, {"status": "building"}}
	if hasDeadline(manual) {
		t.Fatal("explicit completed primary port retained a deadline")
	}
}
func TestHUDDiscoveredInitialDepotCompletionRemovesDeadlineDuringNextPhase(t *testing.T) {
	p := colonization.New(nil)
	claimAt := hudFixtureNow.Add(-48 * time.Hour)
	ctx := M{"commander": M{"system": "Synthetic HUD Colony", "systemAddress": "32100001"}}
	p.Apply(M{"event": "ColonisationSystemClaim", "timestamp": v.ISO(claimAt), "SystemName": "Synthetic HUD Colony", "SystemAddress": "32100001"}, ctx)
	p.Apply(M{"event": "ColonisationBeaconDeployed", "timestamp": v.ISO(claimAt.Add(time.Hour))}, ctx)
	depot := func(id string, at time.Time, complete bool) {
		p.Apply(M{"event": "ColonisationConstructionDepot", "timestamp": v.ISO(at), "MarketID": id, "StationName": "Orbital Construction Site: Synthetic " + id, "ConstructionComplete": complete, "ResourcesRequired": []M{{"Name": "steel", "RequiredAmount": 100, "ProvidedAmount": 20}}}, ctx)
	}
	hud := func() HUDDocument {
		return BuildHUD(M{"settings": M{"activity": "colonization"}, "colonization": p.Snapshot(nil)}, hudFixtureNow)
	}
	depot("32100101", claimAt.Add(2*time.Hour), false)
	if !strings.Contains(hudProse(hud()), "Срок первичного порта:") {
		t.Fatal("real discovery DTO lost the unfinished first-port deadline")
	}
	depot("32100101", claimAt.Add(24*time.Hour), true)
	depot("32100102", claimAt.Add(25*time.Hour), false)
	for _, planner := range []*colonization.Planner{p, colonization.New(p.ExportSettings())} {
		project := v.Obj(planner.Snapshot(nil)["activeProject"])
		buildings := v.Rows(project["buildings"])
		if project["status"] != "building" || len(buildings) != 2 || v.Bool(buildings[0]["isPrimary"]) || !v.Bool(buildings[0]["journalDiscovered"]) || buildings[0]["journalStatus"] != "complete" {
			t.Fatal("fixture no longer reproduces a completed untyped initial port plus another active construction")
		}
		doc := BuildHUD(M{"settings": M{"activity": "colonization"}, "colonization": M{"activeProject": project}}, hudFixtureNow)
		text := hudProse(doc)
		if strings.Contains(text, "Срок первичного порта:") ||
			!(strings.Contains(text, "Synthetic 32100102: посетите депо") || strings.Contains(text, "Сталь: ≤80 т")) {
			t.Fatal("completed discovered phase kept its deadline or hid subsequent construction supply", doc)
		}
	}
}
func TestHUDColonySupplyOmitsRepeatedZeroWithoutConvertingUnknownStock(t *testing.T) {
	supplies := []M{{"name": "Loaded", "missing": 20, "cargoAvailable": 4}, {"name": "Empty one", "missing": 30, "cargoAvailable": 0}, {"name": "Unknown cargo", "missing": 40, "cargoAvailable": nil}}
	s := M{"settings": M{"activity": "colonization"}, "colonization": M{"activeProject": M{"status": "building", "name": "Synthetic colony", "operationalSupplies": supplies}}}
	doc := BuildHUD(s, hudFixtureNow)
	hudCheckAction(t, doc, "ДОСТАВИТЬ", "Loaded", "4 т")
	hudCheckAction(t, doc, "КУПИТЬ", "Empty one", "30 т")
	hudCheckAction(t, doc, "КУПИТЬ", "Unknown cargo", "≤40 т")
	if !hudHasWarning(doc, "Проверьте груз корабля") {
		t.Fatal("supply rows repeated empty cargo or invented unknown stock", doc)
	}
}
