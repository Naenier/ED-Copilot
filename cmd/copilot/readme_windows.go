//go:build windows

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/app"
	"github.com/Naenier/ED-Copilot/internal/localization"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// The photography fixture is never available on the ordinary desktop path.
// Only fictional fixture strings are adapted; no real profile or journal is
// changed, copied, or rendered. The domain observations and calculations remain
// those of the actual Go engine.
type readmeController struct{ *app.Engine }

func readmeCaptureLanguage(enabled bool, requested string) (string, error) {
	if !enabled && requested != "" {
		return "", errors.New("--readme-language is available only with --readme-screenshots")
	}
	if requested == "" {
		return "en", nil
	}
	if requested != "en" && requested != "ru" {
		return "", errors.New("--readme-language must be en or ru")
	}
	return requested, nil
}

func newReadmeController(engine *app.Engine) (*readmeController, error) {
	root := filepath.Dir(engine.DataDir())
	temporary := strings.EqualFold(filepath.Clean(filepath.Dir(root)), filepath.Clean(os.TempDir()))
	if engine.Commander()["fid"] != "FNATIVE-SYNTHETIC" || !temporary || !strings.HasPrefix(filepath.Base(root), "ed-copilot-native-smoke-") || (engine.Language() != "en" && engine.Language() != "ru") {
		return nil, errors.New("README capture requires the isolated temporary synthetic profile in English or Russian")
	}
	c := &readmeController{engine}
	c.seedFitting()
	return c, nil
}

func (c *readmeController) Snapshot() v.M {
	return c.fixture(c.Engine.Snapshot())
}
func (c *readmeController) SnapshotForUI(tab string) v.M {
	state := c.Engine.SnapshotForUI(tab)
	if tab == "riches" {
		state["riches"] = readmeCartography()
	}
	return c.fixture(state)
}
func (c *readmeController) SnapshotForHUD() v.M {
	return c.fixture(c.Engine.SnapshotForHUD())
}
func (c *readmeController) SnapshotForWindow() v.M {
	return c.fixture(c.Engine.SnapshotForWindow())
}
func (c *readmeController) SnapshotForNativeRefresh() v.M {
	return c.fixture(c.Engine.SnapshotForNativeRefresh())
}
func (c *readmeController) SnapshotForOverview() v.M {
	return c.fixture(c.Engine.SnapshotForOverview())
}

func (c *readmeController) Do(path string, body v.M) (v.M, error) {
	result, err := c.Engine.Do(path, body)
	if err == nil && strings.HasPrefix(path, "/api/ship-fits/fleet") {
		result = c.fixture(result)
	}
	return result, err
}

// Apply a full synthetic ship observation so the capture uses the same fleet,
// build calculations and materials interfaces as the ordinary renderer.
func (c *readmeController) seedFitting() {
	at := time.Now().UTC().Add(time.Minute)
	c.Engine.Apply(v.M{
		"event": "Loadout", "timestamp": v.ISO(at), "Ship": "panthermkii", "ShipID": 1,
		"ShipName": "Звёздный попутчик", "ShipIdent": "CP-200",
		"Modules": []v.M{
			{"Slot": "Armour", "Item": "panthermkii_armour_grade2", "Engineering": v.M{"BlueprintName": "Armour_HeavyDuty", "Level": 5, "Quality": 1}},
			{"Slot": "PowerPlant", "Item": "int_powerplant_size8_class5"},
			{"Slot": "MainEngines", "Item": "int_engine_size8_class5"},
			{"Slot": "FrameShiftDrive", "Item": "int_hyperdrive_overcharge_size7_class5", "Engineering": v.M{"BlueprintName": "FSD_LongRange", "Level": 5, "Quality": 1, "ExperimentalEffect": "special_fsd_cooled"}},
			{"Slot": "LifeSupport", "Item": "int_lifesupport_size5_class4"},
			{"Slot": "PowerDistributor", "Item": "int_powerdistributor_size7_class5"},
			{"Slot": "Radar", "Item": "int_sensors_size5_class4"},
			{"Slot": "FuelTank", "Item": "int_fueltank_size7_class3"},
			{"Slot": "CargoHatch", "Item": "modularcargobaydoor"},
			{"Slot": "Cargo01", "Item": "int_cargorack_size8_class1"},
			{"Slot": "Cargo02", "Item": "int_cargorack_size7_class1"},
			{"Slot": "Slot01_Size8", "Item": "int_cargorack_size8_class1"},
			{"Slot": "Slot02_Size7", "Item": "int_shieldgenerator_size7_class5"},
			{"Slot": "PlanetaryApproachSuite", "Item": "int_planetapproachsuite_advanced"},
		},
	}, nil)
	c.Engine.Apply(v.M{
		"event": "StoredShips", "timestamp": v.ISO(at.Add(time.Second)), "StarSystem": "Тестовая колония А",
		"StationName": "Строительный склад Альфа",
		"ShipsHere": []v.M{
			{"ShipID": 1, "ShipType": "panthermkii", "Name": "Звёздный попутчик", "ShipIdent": "CP-200"},
			{"ShipID": 2, "ShipType": "python", "Name": "Полярная звезда", "ShipIdent": "CP-201"},
			{"ShipID": 3, "ShipType": "mandalay", "Name": "Горизонт", "ShipIdent": "CP-202"},
		},
		"ShipsRemote": []v.M{{"ShipID": 4, "ShipType": "sidewinder", "Name": "Первый полёт", "ShipIdent": "CP-203",
			"StarSystem": "Тестовая колония Б", "StationName": "Демонстрационный порт Бета"}},
	}, nil)
	for _, entry := range []struct {
		component, event string
		err              error
	}{
		{"engineering", "ship-fit.materials", nil},
		{"search", "stations.lookup", context.DeadlineExceeded},
		{"journal", "replay.cancel", context.Canceled},
		{"carrier", "report.open", os.ErrNotExist},
	} {
		c.Engine.RecordOperation(entry.component, entry.event, time.Time{}, nil, entry.err)
	}
}

func readmeCartography() v.M {
	options := v.M{"origin": "Тестовая колония А", "destination": "Учебная система Галилей", "jumpRangeLy": 68.8, "radiusLy": 200,
		"maxResults": 100, "maxDistanceLs": 10000, "minMappingValue": 1000000, "useMappingValue": true, "avoidThargoids": true, "loop": false}
	earth := v.M{"name": "Учебная система Атлас 3", "designation": "3", "type": "earthLike", "distanceLs": 242, "scanCredits": 265842, "mappingCredits": 1151985, "terraformable": false}
	water := v.M{"name": "Учебная система Атлас 4", "designation": "4", "type": "waterWorld", "distanceLs": 448, "scanCredits": 342889, "mappingCredits": 1492600, "terraformable": true}
	return v.M{
		"route": v.M{"origin": options["origin"], "destination": options["destination"], "options": options, "stops": []v.M{
			{"system": "Учебная система Атлас", "estimatedJumps": 2, "bodies": []v.M{earth, water}},
			{"system": "Учебная система Кеплер", "estimatedJumps": 1, "bodies": []v.M{{"name": "Учебная система Кеплер 2", "designation": "2", "type": "earthLike", "distanceLs": 538, "scanCredits": 282400, "mappingCredits": 1475288}}},
			{"system": "Учебная система Галилей", "estimatedJumps": 1, "transit": true},
		}},
		"progress": v.M{"nextSystem": "Учебная система Атлас", "nextTarget": earth, "completedJumps": 0, "remainingJumps": 4,
			"completedStops": 0, "totalStops": 3, "totalMappingCredits": 4119873, "useMappingValue": true},
		"search": v.M{"status": "Маршрут построен"},
	}
}

func (c *readmeController) SearchNearest(input v.M) (v.M, error) {
	if input["category"] != "stations" {
		return c.Engine.SearchNearest(input)
	}
	return c.fixture(v.M{"status": "ok", "total": 5, "rows": []v.M{
		{"name": "Демонстрационный порт Альфа", "system": "Тестовая колония А", "type": "Coriolis", "distanceLy": 0, "distanceLs": 345, "pad": "L", "faction": "Тестовая фракция", "economy": "Industrial", "services": []string{"Market", "Refuel", "Repair"}},
		{"name": "Демонстрационный порт Бета", "system": "Учебная система Атлас", "type": "Orbis", "distanceLy": 5.24, "distanceLs": 614, "pad": "L", "economy": "High Tech", "services": []string{"Shipyard", "Outfitting", "Repair"}},
		{"name": "Учебный торговый порт", "system": "Учебная система Лагранж", "type": "Ocellus", "distanceLy": 7.88, "distanceLs": 819, "pad": "L", "economy": "Refinery", "services": []string{"Market", "Repair"}},
		{"name": "Строительный склад Бета", "system": "Тестовая колония Б", "type": "Outpost", "distanceLy": 10, "distanceLs": 1238, "pad": "M", "economy": "Colony", "services": []string{"Market", "Refuel"}},
		{"name": "Демонстрационный терминал", "system": "Учебная система Кеплер", "type": "Coriolis", "distanceLy": 12.45, "distanceLs": 1492, "pad": "L", "economy": "Agriculture", "services": []string{"Refuel", "Repair"}},
	}}), nil
}

// The ordinary Galaxy bridge stays offline in this fixture. These invented
// results exercise the same tables and system reader used by the live bridge.
func (c *readmeController) Galaxy(operation string, input v.M) v.M {
	switch operation {
	case "galaxy.nearby":
		return c.fixture(v.M{"status": "ok", "source": "EDSM", "total": 7, "rows": []v.M{
			{"name": "Тестовая колония А", "distance": 0, "allegiance": "Independent", "government": "Cooperative", "economy": "Colony", "security": "Medium", "population": 14200},
			{"name": "Учебная система Атлас", "distance": 5.24, "allegiance": "Federation", "government": "Democracy", "economy": "High Tech", "security": "High", "population": 2684000},
			{"name": "Учебная система Лагранж", "distance": 7.88, "allegiance": "Alliance", "government": "Corporate", "economy": "Refinery", "security": "Medium", "population": 183400},
			{"name": "Тестовая колония Б", "distance": 10, "allegiance": "Independent", "government": "Cooperative", "economy": "Industrial", "security": "Low", "population": 9600},
			{"name": "Учебная система Кеплер", "distance": 12.45, "allegiance": "Empire", "government": "Feudal", "economy": "Agriculture", "security": "Medium", "population": 84700},
			{"name": "Учебная система Галилей", "distance": 15.61, "allegiance": "Independent", "government": "Anarchy", "security": "Lawless", "population": 0},
			{"name": "Учебная промежуточная система", "distance": 19.2, "population": 0},
		}})
	case "galaxy.system":
		system := v.Str(input["name"])
		switch input["section"] {
		case "summary":
			return c.fixture(v.M{"status": "ok", "system": v.M{
				"x": 4.625, "y": -2.375, "z": 0.625, "allegiance": "Federation", "government": "Democracy",
				"economy": "High Tech", "secondEconomy": "Industrial", "security": "High", "population": 2684000,
				"faction": "Тестовая фракция", "factionState": "Boom", "permitRequired": false,
				"starName": system, "starType": "G2 V", "scoopable": true,
			}})
		case "bodies":
			return c.fixture(v.M{"status": "ok", "total": 3, "rows": []v.M{
				{"name": system + " A", "type": "Star", "subType": "G2 V", "distance": 0, "landable": false},
				{"name": system + " A 1", "type": "Planet", "distance": 426.7, "landable": true,
					"gravity": 0.46, "temperature": 184, "materials": []v.M{{"name": "Железо", "percent": 23.5}, {"name": "Никель", "percent": 16.2}, {"name": "Углерод", "percent": 12.4}, {"name": "Полоний", "percent": 0.9}}},
				{"name": system + " A 2", "type": "Planet", "distance": 1880.3, "landable": false,
					"rings": []v.M{{"name": system + " A 2 A Ring", "type": "Metallic", "innerRadius": 91000, "outerRadius": 156000}}},
			}})
		}
	}
	return v.M{"status": "not-found"}
}

func (c *readmeController) fixture(state v.M) v.M {
	if c.Engine.Language() == "en" {
		return englishReadmeFixture(state).(map[string]any)
	}
	return state
}

var readmeFixtureNames = strings.NewReplacer(
	"Снабжение новой станции", "Supply a new station",
	"Тестовый командир", "Demo Commander",
	"Звёздный попутчик", "Star Voyager",
	"Полярная звезда", "Polaris",
	"Горизонт", "Horizon",
	"Первый полёт", "First Flight",
	"Тестовая колония А", "Demo Colony Alpha",
	"Тестовая колония Б", "Demo Colony Beta",
	"Тестовая исходная система", "Demo Origin",
	"Учебная промежуточная система", "Demo Waypoint",
	"Учебная система Атлас", "Demo Atlas",
	"Учебная система Лагранж", "Demo Lagrange",
	"Учебная система Кеплер", "Demo Kepler",
	"Учебная система Галилей", "Demo Galileo",
	"Строительный склад Альфа", "Alpha Construction Depot",
	"Строительный склад Бета", "Beta Construction Depot",
	"Тестовая фракция", "Demo Cooperative",
	"Демонстрационный порт Альфа", "Demo Port Alpha",
	"Демонстрационный порт Бета", "Demo Port Beta",
	"Учебный торговый порт", "Demo Trade Port",
	"Демонстрационный терминал", "Demo Terminal",
	"Основной порт", "Primary Port",
	"Доставить алюминий в первую колонию", "Deliver aluminium to Colony Alpha",
	"Ковчег", "Ark",
	"Демонстрационные данные: системы, координаты и расход трития вымышлены. Не используйте этот маршрут в игре.", "Demonstration: systems, coordinates and fuel values are fictional. Do not use this route in the game.",
	"Демонстрационный рейс. Цены вымышлены.", "Demonstration route. Prices are fictional.",
	"Демонстрационные предложения", "Demonstration offers",
	"Демонстрационные данные", "Demonstration data",
	"Синтетический пример. Не является рыночной котировкой.", "Synthetic example, not a market quote.",
)

func englishReadmeFixture(raw any) any {
	switch value := raw.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, item := range value {
			out[key] = englishReadmeFixture(item)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = englishReadmeFixture(item)
		}
		return out
	case []v.M:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = englishReadmeFixture(item)
		}
		return out
	case []string:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = englishReadmeFixture(item)
		}
		return out
	case string:
		return localization.Text("en", readmeFixtureNames.Replace(value))
	default:
		return value
	}
}
