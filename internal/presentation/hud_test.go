package presentation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func hudProse(doc HUDDocument) string {
	parts := []string{}
	for _, section := range doc.Sections {
		parts = append(parts, section.Title)
		if section.Subtitle != "" {
			parts = append(parts, section.Subtitle)
		}
		for _, line := range section.Lines {
			parts = append(parts, hudLineProse(line))
		}
	}
	return strings.Join(parts, "\n")
}
func hudLineProse(line HUDLine) string {
	if line.Value != "" {
		return line.Text + ": " + line.Value
	}
	return line.Text
}
func TestHUDFocusPolicyRequiresActualGameExecutable(t *testing.T) {
	for _, tc := range []struct {
		enabled bool
		running any
		process string
		want    bool
	}{
		{true, true, `C:\Games\Elite\EliteDangerous64.exe`, true},
		{true, nil, "EliteDangerous.exe", true},
		{true, true, "ELITEDANGEROUS32.EXE", true},
		{false, true, "EliteDangerous64.exe", false},
		{true, false, "EliteDangerous64.exe", false},
		{true, true, "ED Copilot.exe", false},
		{true, true, "EDLaunch.exe", false},
		{true, true, "", false},
		{true, true, "EliteDangerous64.exe.bak", false},
		{true, true, "MyEliteDangerous64.exe", false},
		{true, true, "notepad.exe", false},
	} {
		if got := HUDVisible(tc.enabled, tc.running, tc.process); got != tc.want {
			t.Errorf("%#v: visible=%v", tc, got)
		}
	}
}
func TestHUDAllActivitiesExcludeBrandingAndPermanentShipRows(t *testing.T) {
	s := M{"settings": M{}, "commander": M{"system": "Sol", "ship": "Panther Clipper Mk II", "shipType": "panthermkii", "shipName": "Звёздный путь", "shipIdent": "RU-42", "cargoTons": 12, "cargoCapacity": 128}, "connection": M{}}
	for _, a := range Activities {
		v.Obj(s["settings"])["activity"] = a.Value
		doc := BuildHUD(s, time.Now())
		text := hudProse(doc)
		if doc.Activity != a.Value || strings.Contains(strings.ToUpper(text), "ED COPILOT") || strings.Contains(text, "Panther") || strings.Contains(text, "Звёздный путь") || strings.Contains(text, "panthermkii") {
			t.Fatalf("%s: %s", a.Value, text)
		}
	}
}
func TestHUDColoniesSelectedOnlyUnknownBillsAndInactive(t *testing.T) {
	a := M{"id": "a", "name": "Альфа", "targetSystem": "Colony A", "status": "building", "unknownBills": 1, "totalMissing": 0, "nextTask": M{"text": "Посетить депо Альфа"}, "operationalSupplies": []M{{"commodity": "steel", "name": "Сталь", "missing": 50, "cargoAvailable": 20, "authoritative": true}}, "supplies": []M{{"name": "Устаревшая смета", "missing": 9999}}}
	b := M{"id": "b", "name": "Бета", "targetSystem": "Colony B", "status": "building", "nextTask": M{"text": "Доставить Бета"}, "operationalSupplies": []M{{"name": "Титан", "missing": 72, "cargoAvailable": nil}}}
	s := M{"settings": M{"activity": "colonization"}, "colonization": M{"activeProject": a, "projects": []M{a, b}, "aggregateSupplies": []M{{"name": "Чужой груз", "missing": 123}}}}
	text := hudProse(BuildHUD(s, time.Now()))
	for _, want := range []string{"Альфа", "Сметы неизвестны: 1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	hudCheckAction(t, BuildHUD(s, time.Now()), "ДОСТАВИТЬ", "Сталь", "20 т")
	hudCheckAction(t, BuildHUD(s, time.Now()), "КУПИТЬ", "Сталь", "30 т")
	for _, not := range []string{"Бета", "Титан", "Чужой груз", "Устаревшая смета", ": 0 т"} {
		if strings.Contains(text, not) {
			t.Fatalf("leaked/misleading %q: %s", not, text)
		}
	}
	v.Obj(s["colonization"])["activeProject"] = b
	text = hudProse(BuildHUD(s, time.Now()))
	hudCheckAction(t, BuildHUD(s, time.Now()), "КУПИТЬ", "Титан", "≤72 т")
	if strings.Contains(text, "Альфа") || !strings.Contains(text, "Проверьте груз корабля") {
		t.Fatal(text)
	}
	for _, status := range []string{"complete", "cancelled"} {
		b["status"] = status
		text = hudProse(BuildHUD(s, time.Now()))
		if strings.Contains(text, "Доставить Бета") || strings.Contains(text, "Титан") || strings.Contains(text, "СНАБЖЕНИЕ КОЛОНИИ") {
			t.Fatal(text)
		}
	}
	v.Obj(s["colonization"])["activeProject"] = nil
	if text = hudProse(BuildHUD(s, time.Now())); strings.Contains(text, "ВЫБРАННАЯ КОЛОНИЯ") || strings.Contains(text, "Выберите колонию") {
		t.Fatal(text)
	}
}
func TestHUDProspectorTradeQuoteAndNoStateMutation(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	s := M{"settings": M{"activity": "asteroid"}, "commander": M{"cargoTons": 35, "cargoCapacity": 128}, "goods": []M{{"key": "monazite", "name": "Монацит"}}, "prospect": M{"decision": "mine", "remaining": 70, "motherlode": "monazite", "materials": []M{{"name": "Платина", "percent": 31}, {"name": "Празеодим", "percent": 10}}, "reasons": []string{"Состав подходит"}}, "trading": M{"destination": M{"station": "Prospect", "system": "Col 285", "distanceLy": 42, "updatedAt": "2026-11-07T10:00:00Z", "quotes": []M{{"name": "Платина", "price": 250000, "demand": 4000}}}}}
	before, _ := json.Marshal(s)
	text := hudProse(BuildHUD(s, now))
	for _, want := range []string{"Можно добывать", "Платина: 31 %", "Ядро: Монацит", "Prospect", "250 000 кр./т · спрос 4 000 т", "2 ч назад"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("HUD modified source snapshot")
	}
	v.Obj(s["settings"])["activity"] = "raw"
	text = hudProse(BuildHUD(s, now))
	if strings.Contains(text, "СКАН АСТЕРОИДА") || strings.Contains(text, "ПРОДАЖА ГРУЗА") {
		t.Fatal(text)
	}
}
func TestHUDCarrierArrivalAndUnknownFuel(t *testing.T) {
	s := M{"settings": M{"activity": "carrier"}, "carrier": M{"owned": true, "name": "Ковчег", "callsign": "NAT-001", "fuel": M{"tankTons": nil, "reserveTons": nil}}, "carrierRoute": M{"progress": M{"nextIndex": 1}, "route": M{"stops": []M{{"name": "A"}, {"name": "B", "distanceLy": 450, "fuelUsedTons": 92, "restockTons": 50}}}, "summary": M{"remainingJumps": 3, "totalMinutes": 63}}}
	text := hudProse(BuildHUD(s, time.Now()))
	for _, want := range []string{"Запас трития неизвестен", "\nB\n", "Дальность и расход: 450 св. л. · 92 т", "Заправка после прыжка: 50 т", "Прыжков осталось: 3"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
}

func TestHUDTradeUsesDomainMarginAndPreservesQuoteWarnings(t *testing.T) {
	s := M{"settings": M{"activity": "trade"}, "tradePlan": M{"key": "gold", "name": "Золото", "quantity": 40, "grossMargin": 1200000}, "trading": M{"destination": M{"station": "Market", "system": "Sol", "expired": true, "cargoChanged": true, "bulkTaxRisk": true, "quotes": []M{{"name": "Золото", "price": 100000, "demand": 20}}}}}
	text := hudProse(BuildHUD(s, time.Now()))
	for _, want := range []string{"Плановая маржа: 1 200 000 кр.", "Котировка устарела", "Состав груза изменился", "Низкий спрос"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	v.Obj(s["settings"])["activity"] = "raw"
	s["engineering"] = M{"items": []M{{"moduleName": "FSD"}}, "materials": []M{{"key": "polonium", "name": "Полоний", "category": "raw", "needed": 10, "owned": nil, "missing": nil}}, "shopping": M{"knownCategories": M{"raw": false}}}
	text = hudProse(BuildHUD(s, time.Now()))
	if !strings.Contains(text, "Запасы сырья известны не полностью") || strings.Contains(text, "нет дефицита") {
		t.Fatal(text)
	}
}
