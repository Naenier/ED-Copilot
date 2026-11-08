package presentation

import (
	"reflect"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestEnglishPresentationProtectsIdentitiesAndCanonicalActions(t *testing.T) {
	state := M{"settings": M{"language": "en"}, "commander": M{"system": "Настройки", "station": "Колонизация", "shipName": "Топливо"}}
	page := Page{Title: "Обзор", Modes: []string{"Состояние"}, Columns: []string{"Параметр", "Значение"}, Rows: []Row{{ID: "station", Data: M{"station": "Колонизация"}, Cells: []string{"Станция", "Колонизация"}}}}
	before := v.Clone(page)
	out := LocalizePage("overview", page, state)
	if out.Title != "Overview" || out.Modes[0] != "Состояние" || out.Rows[0].Cells[0] != "Station" || out.Rows[0].Cells[1] != "Колонизация" {
		t.Fatal(out)
	}
	if !reflect.DeepEqual(v.Clone(page), before) {
		t.Fatal("presentation mutated source")
	}
	actions := CoreActions("settings", "", state, nil, nil)
	if actions[0].ID != "settings.language" || actions[0].Fields[0].Key != "language" || actions[0].Fields[0].Choices[1].Value != "en" || actions[0].Fields[0].Label != "Interface language" {
		t.Fatal(actions[0])
	}
	if got := localizedNumber("1 234,5 Cr"); got != "1,234.5 Cr" {
		t.Fatal(got)
	}
}

func TestEnglishTableAndModelNamesStaySeparate(t *testing.T) {
	state := M{"settings": M{"language": "en"}, "colonization": M{"activeProject": M{"id": "c", "buildings": []M{{"id": "b", "name": "Настройки", "status": "building"}}}}}
	page := Build("colonization", "Строительство", "", state)
	if page.Title != "Colonisation" || page.Rows[0].Cells[1] != "Настройки" || page.Rows[0].Data["name"] != "Настройки" {
		t.Fatal(page)
	}
	state["goods"] = []M{{"key": "gold", "name": "Золото", "english": "Gold"}}
	copy := v.Obj(v.Clone(state))
	LocalizeWebState(copy)
	if v.Rows(copy["goods"])[0]["name"] != "Gold" || v.Rows(state["goods"])[0]["name"] != "Золото" {
		t.Fatal("canonical game name projection changed stored model")
	}
}
