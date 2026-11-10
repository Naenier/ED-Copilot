package presentation

const EngineeringProgression = "Прокачка"
const EngineeringMaterials = "Материалы"

func EngineeringModes() []string {
	return []string{EngineeringProgression, "План", EngineeringMaterials, "Инженеры"}
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
