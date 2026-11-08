//go:build windows

package main

import (
	"errors"
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

func newReadmeController(engine *app.Engine) (*readmeController, error) {
	if engine.Commander()["fid"] != "FNATIVE-SYNTHETIC" || !strings.HasPrefix(filepath.Base(filepath.Dir(engine.DataDir())), "ed-copilot-native-smoke-") || engine.Language() != "en" {
		return nil, errors.New("README capture requires the temporary English synthetic profile")
	}
	return &readmeController{engine}, nil
}

func (c *readmeController) Snapshot() v.M {
	return englishReadmeFixture(c.Engine.Snapshot()).(map[string]any)
}
func (c *readmeController) SnapshotForUI(tab string) v.M {
	return englishReadmeFixture(c.Engine.SnapshotForUI(tab)).(map[string]any)
}
func (c *readmeController) SnapshotForHUD() v.M {
	return englishReadmeFixture(c.Engine.SnapshotForHUD()).(map[string]any)
}

var readmeFixtureNames = strings.NewReplacer(
	"Тестовый командир", "Demo Commander",
	"Звёздный попутчик", "Star Voyager",
	"Тестовая колония А", "Demo Colony Alpha",
	"Тестовая колония Б", "Demo Colony Beta",
	"Тестовая исходная система", "Demo Origin",
	"Учебная промежуточная система", "Demo Waypoint",
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
