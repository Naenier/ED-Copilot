package carrier

import v "github.com/Naenier/ED-Copilot/internal/value"

func maintenanceProjection(c, finance M) (any, bool) {
	if value, known := v.Number(finance["maintenance"]); known && !v.Time(finance["maintenanceAt"]).Before(v.Time(c["crewChangedAt"])) {
		return value, false
	}
	if upkeep, known := v.Number(finance["weeklyUpkeep"]); known {
		return upkeep, true
	}
	if !v.Bool(c["crewObserved"]) {
		return nil, false
	}
	kind := "fleet"
	if v.Num(v.At(c, "space", "capacityTons")) == 60000 {
		kind = "squadron"
	}
	model, _ := calculatorCatalog(kind)
	cost := model.Upkeep
	for _, service := range model.Services {
		installed := v.Obj(v.At(c, "services", service.Key))
		if !v.Bool(installed["installed"]) {
			continue
		}
		if v.Bool(installed["enabled"]) {
			cost += service.Active
		} else {
			cost += service.Suspended
		}
	}
	return cost, true
}
