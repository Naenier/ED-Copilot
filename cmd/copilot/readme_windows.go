//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

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
	return &readmeController{engine}, nil
}

func (c *readmeController) Snapshot() v.M {
	return c.fixture(c.Engine.Snapshot())
}
func (c *readmeController) SnapshotForUI(tab string) v.M {
	return c.fixture(c.Engine.SnapshotForUI(tab))
}
func (c *readmeController) SnapshotForHUD() v.M {
	return c.fixture(c.Engine.SnapshotForHUD())
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
