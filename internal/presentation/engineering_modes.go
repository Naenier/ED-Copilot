package presentation

const EngineeringProgression = "Прокачка"
const EngineeringCurrentShip = "Текущий корабль"
const EngineeringMaterials = "Материалы"
const EngineeringOdyssey = "Снаряжение Odyssey"
const EngineeringShipBuild = "Оснащение корабля"
const EngineeringItinerary = "Поездка к инженерам"
const ColonyHistoryMode = "Динамика системы"

func EngineeringModes() []string {
	return []string{EngineeringProgression, EngineeringCurrentShip, "План", EngineeringOdyssey, EngineeringShipBuild, EngineeringItinerary, EngineeringMaterials, "Инженеры"}
}

func EngineeringMode(mode string) string {
	if mode == "Обмен" {
		return EngineeringMaterials
	}
	return mode
}

func EngineeringMaterialMode(mode string) bool {
	_, _, ok := materialView(mode)
	return ok
}

// SectionSevenCustom marks pages whose renderer owns its controls and rows.
func SectionSevenCustom(tab, mode string) bool {
	switch tab {
	case "ship-route", "exploration", "notifications", "second-screen":
		return true
	case "engineering":
		return mode == EngineeringCurrentShip || mode == EngineeringShipBuild || mode == EngineeringItinerary
	case "exobiology":
		return mode == SurfaceRadarMode
	case "colonization":
		return mode == ColonyHistoryMode
	default:
		return false
	}
}
