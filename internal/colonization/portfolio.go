package colonization

import (
	"errors"

	"github.com/Naenier/ED-Copilot/internal/value"
)

// SupplyPortfolio calculates the combined demand of selected projects against
// one stock snapshot. An omitted buildingIds includes all open buildings in
// those projects. Explicitly empty projectIds or buildingIds select no work.
func (p *Planner) SupplyPortfolio(context M, selection M) (M, error) {
	projectIDs, err := portfolioIDs(selection["projectIds"], 100)
	if err != nil {
		return nil, err
	}
	var buildingIDs map[string]bool
	if ids, present := selection["buildingIds"]; present {
		buildingIDs, err = portfolioIDs(ids, 20000)
		if err != nil {
			return nil, err
		}
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	workspace, owner, err := p.supplyOwner(context)
	if err != nil {
		return nil, err
	}
	if owner == "" {
		workspace = ""
	}
	buildings := []M{}
	selected := map[reservationBuilding]bool{}
	unknownBills, projectCount := 0, 0
	for _, project := range p.projects {
		id := str(project["id"])
		if !projectIDs[id] {
			continue
		}
		if !ownsSupplyProject(project, owner) {
			continue
		}
		delete(projectIDs, id)
		projectCount++
		cancelled := str(project["status"]) == "cancelled"
		for _, b := range rows(project["buildings"]) {
			buildingID := str(b["id"])
			if buildingIDs != nil && !buildingIDs[buildingID] {
				continue
			}
			delete(buildingIDs, buildingID)
			status := p.statusOf(b)
			if cancelled || !supplyBuildingOpen(str(b["status"])) || !supplyBuildingOpen(status) {
				continue
			}
			depot := p.depots[str(b["marketId"])]
			requirements := buildingRequirements(project, b, depot, p.buildingContributions(b))
			if len(requirements) == 0 {
				unknownBills++
			}
			selected[reservationBuilding{id, buildingID}] = true
			buildings = append(buildings, M{"id": buildingID, "projectId": id, "name": displayBuildingName(b), "status": status, "requirements": requirements})
		}
	}
	if len(projectIDs) != 0 || len(buildingIDs) != 0 {
		return nil, errors.New("Выбранная колония или объект недоступны.")
	}

	stock := p.readReservationStock(context)
	held := p.activeSupplyHolds(workspace, selected)
	reservable := []M{}
	if workspace != "" && owner != "" {
		reservable = reservableSupply(buildings, stock, held)
	}
	stock.excludeOutsideHolds(held)
	supplies := aggregateResources(buildings, stock.cargo, stock.cargoKnown, stock.warehouse, nil)
	for _, row := range supplies {
		row["cargoCovered"], row["carrierCovered"] = coveredAmounts(row)
		key := str(row["commodity"])
		row["reservedHere"], row["reservedElsewhere"] = held.inside[key], held.outsideByCommodity[key]
		row["inTransit"] = held.insideTransit[key]
	}
	return M{
		"supplies":      supplies,
		"buildingCount": len(buildings),
		"unknownBills":  unknownBills,
		"projectCount":  projectCount,
		"reservations":  p.supplyReservationHistory(workspace),
		"reservable":    reservable,
	}, nil
}

func portfolioIDs(raw any, limit int) (map[string]bool, error) {
	ids := map[string]bool{}
	items := value.Arr(raw)
	switch raw.(type) {
	case []any, []string:
	default:
		return nil, errors.New("Нужен список колоний или объектов.")
	}
	if len(items) > limit {
		return nil, errors.New("Слишком много колоний или объектов.")
	}
	for _, item := range items {
		id, ok := item.(string)
		if !ok || id == "" || len(id) > 320 {
			return nil, errors.New("Недопустимый идентификатор колонии или объекта.")
		}
		ids[id] = true
	}
	return ids, nil
}
