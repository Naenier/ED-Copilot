package presentation

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/exobiology"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func exobiologyPresentationFixture() M {
	service := exobiology.New(nil)
	ctx := M{"commander": M{"system": "Synthetic Bio", "systemAddress": "123", "coordinates": []float64{0, 0, 0}}}
	apply := func(kind string, r M) {
		r["event"], r["timestamp"], r["SystemAddress"] = kind, "2026-11-08T12:00:00Z", "123"
		service.Apply(r, ctx)
	}
	apply("LoadGame", M{"FID": "synthetic"})
	for _, id := range []int{1, 2} {
		name := fmt.Sprintf("Synthetic Bio %d", id)
		apply("Location", M{"StarSystem": "Synthetic Bio", "BodyID": id, "BodyName": name, "BodyType": "Planet", "StarPos": []float64{0, 0, 0}})
		apply("Scan", M{"BodyID": id, "BodyName": name, "Landable": true, "PlanetClass": "High metal content body", "AtmosphereType": "Ammonia", "SurfacePressure": 1700, "SurfaceTemperature": 175, "SurfaceGravity": 1, "Radius": 1000000, "DistanceFromArrivalLS": 50})
		apply("SAASignalsFound", M{"BodyID": id, "BodyName": name, "Signals": []M{{"Type": "$SAA_SignalType_Biological;", "Count": 1}}, "Genuses": []M{{"Genus": "$Codex_Ent_Stratum_Genus_Name;"}}})
		apply("ScanOrganic", M{"Body": id, "ScanType": "Analyse", "Species": "$Codex_Ent_Stratum_01_Name;", "Variant": "$Codex_Ent_Stratum_01_m_Name;", "Genus": "$Codex_Ent_Stratum_Genus_Name;"})
		if id == 1 {
			apply("SellOrganicData", M{"MarketID": 1, "BioData": []M{{"Species": "$Codex_Ent_Stratum_01_Name;", "Variant": "$Codex_Ent_Stratum_01_m_Name;", "Value": 19010800, "Bonus": 0}}})
		}
	}
	return service.Snapshot()
}

func TestExobiologyHUDUnknownPositionAndCurrentBodyIsolation(t *testing.T) {
	state := M{"settings": M{"activity": "exobiology"}, "commander": M{"system": "Test System", "vehicle": "foot"}, "exobiology": M{"currentBody": M{"key": "current", "name": "Current Planet", "system": "Test System", "signals": 3, "firstFootfall": "unknown", "predictions": []M{{"name": "Possible species", "tier": "uncertain", "estimatedCredits": nil}, {"name": "Excluded species", "tier": "unlikely"}}}, "selectedBody": M{"key": "other", "name": "Other Selected Planet"}, "tracker": M{"bodyKey": "old", "bodyName": "Old Planet", "name": "Stratum Tectonicas", "sampleCount": 1, "minSampleDistanceM": 500, "phase": "tracking"}, "summary": M{"pendingSamples": 0}}}
	doc := BuildHUD(state, time.Now())
	text := hudProse(doc)
	for _, s := range []string{"Current Planet", "Old Planet", "1 / 3", "Между пробами: ≥ 500 м", "Possible species"} {
		if !strings.Contains(text, s) {
			t.Fatal("missing useful biological HUD information", s, text)
		}
	}
	for _, s := range []string{"Other Selected Planet", "Excluded species", "Нет актуальной координаты", "0 кр."} {
		if strings.Contains(text, s) {
			t.Fatal("fabricated/stale HUD fact", s, text)
		}
	}
	if len(doc.Sections) > 4 {
		t.Fatal("overcrowded HUD", doc)
	}
	tr := v.Obj(v.At(state, "exobiology", "tracker"))
	tr["nearestSampleDistanceM"], tr["remainingDistanceM"], tr["nearestSampleMeetsMin"] = 550, 0, true
	if strings.Contains(hudProse(BuildHUD(state, time.Now())), "Можно брать пробу") {
		t.Fatal("old body sample distance shown as ready")
	}
	tr["bodyKey"] = "current"
	if text := hudProse(BuildHUD(state, time.Now())); !strings.Contains(text, "Можно брать пробу") || strings.Contains(text, "Possible species") {
		t.Fatal("active sampling on current planet must take priority over predictions", text)
	}
}

func TestExobiologyActionsStableAndIdentityPreservedInEnglish(t *testing.T) {
	state := M{"settings": M{"language": "en"}, "exobiology": M{"settings": M{}, "selectedBodyKey": "123:1", "bodies": []M{{"key": "123:1", "name": "Настройки", "system": "Обзор", "signals": 2, "firstFootfall": "unknown"}}}}
	page := Build("exobiology", "Планеты", "", state)
	if len(page.Rows) != 1 || page.Rows[0].Cells[0] != "Настройки" || page.Rows[0].Cells[1] != "Обзор" {
		t.Fatal("planet/system identities translated", page.Rows)
	}
	for _, a := range CoreActions("exobiology", "Обзор", state, M{"key": "123:1", "system": "Обзор"}, nil) {
		if a.ID == "exobiology.configure" {
			body, err := a.Body(M{"includeBacterium": false})
			if err != nil || body["action"] != "configure" || body["includeBacterium"] != false {
				t.Fatal("localization changed domain action", body, err)
			}
			return
		}
	}
	t.Fatal("missing configuration")
}
