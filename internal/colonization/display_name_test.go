package colonization

import (
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/value"
)

func TestJournalConstructionDisplayNamePreservesIdentityAndUserNames(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"$EXT_PANEL_ColonisationShip; SYN-123", "Колонизационный корабль · SYN-123"},
		{"$EXT_PANEL_ColonisationShip; : Synthetic Port", "Колонизационный корабль · Synthetic Port"},
		{"Planetary Construction Site: Synthetic Port", "Планетарная стройплощадка · Synthetic Port"},
		{"Orbital Construction Site: Synthetic Ring", "Орбитальная стройплощадка · Synthetic Ring"},
		{"Surface Construction Site: Synthetic Surface", "Поверхностная стройплощадка · Synthetic Surface"},
		{"$EXT_PANEL_Unknown; Synthetic Identity", "Synthetic Identity"},
		{"$EXT_PANEL_Unknown;", "Стройплощадка из журнала"},
		{"Собственное название — Синтетическая база", "Собственное название — Синтетическая база"},
	} {
		if got := displayBuildingName(M{"name": tc.raw, "journalDiscovered": true}); got != tc.want {
			t.Fatalf("display=%q want=%q", got, tc.want)
		}
	}
	raw := "$EXT_PANEL_ColonisationShip; SYN-123"
	if got := displayBuildingName(M{"name": raw, "journalDiscovered": false}); got != raw {
		t.Fatal("display formatting changed an explicit manual name")
	}
	if got := displayBuildingName(M{"name": "Мой новый заголовок", "journalDiscovered": true, "journalStationName": raw, "journalDisplayName": "Локализованное имя станции"}); got != "Мой новый заголовок" {
		t.Fatal("localized station data overwrote a user-renamed building")
	}
}

func TestLocalizedJournalConstructionDisplaySurvivesRestartWithoutChangingLinkage(t *testing.T) {
	p := New(nil)
	discoveryClaim(p, "Synthetic Localized Colony", "12345", "2026-11-01T12:00:00Z")
	const raw = "Planetary Construction Site: Synthetic Base"
	const localized = "Планетарная стройплощадка · Синтетическая база"
	discoveryEvent(p, "Docked", "2026-11-01T12:01:00Z", M{"StarSystem": "Synthetic Localized Colony", "MarketID": "201", "StationName": raw, "StationName_Localised": localized, "StationType": "PlanetaryConstructionDepot"}, nil)
	discoveryDepot(p, "Synthetic Localized Colony", "12345", "201", "2026-11-01T12:02:00Z", false)
	before := rows(rows(p.ExportSettings()["projects"])[0]["buildings"])[0]
	if before["name"] != raw || p.stations["201"]["name"] != raw || p.depots["201"]["station"] != raw {
		t.Fatal("raw identity was changed by display normalization")
	}
	for _, planner := range []*Planner{p, New(p.ExportSettings())} {
		building := rows(active(planner)["buildings"])[0]
		if building["name"] != localized || building["displayName"] != localized || building["id"] != before["id"] || building["marketId"] != "201" {
			t.Fatal("localized label or stable linkage was lost")
		}
	}
	if task := str(value.Obj(active(p)["nextTask"])["text"]); strings.Contains(task, "$EXT_PANEL_") {
		t.Fatal("raw localization token leaked into the next task")
	}
	action(t, p, M{"action": "update-building", "projectId": active(p)["id"], "id": before["id"], "name": "Моя очередь снабжения"})
	if rows(active(p)["buildings"])[0]["name"] != "Моя очередь снабжения" {
		t.Fatal("user override was not kept in display output")
	}
}

func TestLegacyManualLinkedRawConstructionNameFormatsWithoutChangingPlan(t *testing.T) {
	const raw = "Planetary Construction Site: Synthetic Legacy Site"
	p := New(M{"selectedProjectId": "legacy-plan", "projects": []M{{"id": "legacy-plan", "targetSystem": "Synthetic Legacy Colony", "systemAddress": "12345", "status": "planned", "buildings": []M{{"id": "legacy-site", "typeId": "custom", "name": raw, "marketId": "201", "status": "planned", "notes": "Keep my plan"}}, "supplyOrder": []string{"legacy-site"}}}})
	discoveryClaim(p, "Synthetic Legacy Colony", "12345", "2026-11-01T12:00:00Z")
	discoveryEvent(p, "Docked", "2026-11-01T12:01:00Z", M{"StarSystem": "Synthetic Legacy Colony", "SystemAddress": "12345", "MarketID": "201", "StationName": raw, "StationType": "PlanetaryConstructionDepot"}, nil)
	discoveryDepot(p, "Synthetic Legacy Colony", "12345", "201", "2026-11-01T12:02:00Z", false)
	for _, planner := range []*Planner{p, New(p.ExportSettings())} {
		project := rows(planner.ExportSettings()["projects"])[0]
		stored := rows(project["buildings"])[0]
		shown := rows(active(planner)["buildings"])[0]
		if shown["name"] != "Планетарная стройплощадка · Synthetic Legacy Site" || stored["name"] != raw || stored["id"] != "legacy-site" || stored["marketId"] != "201" || stored["status"] != "planned" || stored["journalDiscovered"] != false || stored["notes"] != "Keep my plan" || len(stringsOf(project["supplyOrder"])) != 1 || stringsOf(project["supplyOrder"])[0] != "legacy-site" {
			t.Fatal("legacy display formatting changed the saved name, identity, flag or plan")
		}
	}
	action(t, p, M{"action": "update-building", "projectId": "legacy-plan", "id": "legacy-site", "name": "Моя отдельная стройка"})
	for _, planner := range []*Planner{p, New(p.ExportSettings())} {
		if rows(active(planner)["buildings"])[0]["name"] != "Моя отдельная стройка" {
			t.Fatal("an explicit user rename was replaced by the observed station name")
		}
	}
}
