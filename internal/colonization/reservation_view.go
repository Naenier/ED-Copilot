package colonization

import (
	"math"

	"github.com/Naenier/ED-Copilot/internal/value"
)

var supplySources = [...]string{"ship", "fleet", "squadron"}

type reservationStock struct {
	sourceIDs  map[string]string
	items      map[string]map[string]float64
	cargo      map[string]float64
	cargoKnown bool
	warehouse  *stock
}

func inventoryKey(source, id string) string {
	if source == "ship" {
		return source
	}
	if id != "" {
		return "carrier:" + id
	}
	return ""
}

func (p *Planner) readReservationStock(context M) reservationStock {
	cargo, known := AvailableCargo(value.Obj(context["commander"]))
	warehouses, warehouse := p.carrierWarehouses(context)
	state := reservationStock{
		sourceIDs: map[string]string{}, items: map[string]map[string]float64{},
		cargo: cargo, cargoKnown: known, warehouse: warehouse,
	}
	if known {
		state.items["ship"] = cargo
	}
	for _, row := range warehouses {
		if !value.Bool(row["known"]) || !value.Bool(row["included"]) || !value.Bool(row["accessible"]) {
			continue
		}
		source, id := str(row["kind"]), str(row["id"])
		key := inventoryKey(source, id)
		if key == "" || state.items[key] != nil {
			continue
		}
		state.sourceIDs[source] = id
		state.items[key] = map[string]float64{}
		for _, item := range rows(row["items"]) {
			state.items[key][str(item["key"])] = num(item["count"])
		}
	}
	return state
}

type reservationBuilding struct{ project, building string }
type reservationItem struct{ inventory, commodity string }

type supplyHolds struct {
	targets            map[reservationTarget]float64
	stock              map[reservationItem]float64
	outside            map[reservationItem]float64
	inside             map[string]float64
	insideTransit      map[string]float64
	outsideByCommodity map[string]float64
}

func (p *Planner) activeSupplyHolds(workspace string, selection map[reservationBuilding]bool) supplyHolds {
	held := supplyHolds{
		targets: map[reservationTarget]float64{}, stock: map[reservationItem]float64{},
		outside: map[reservationItem]float64{}, inside: map[string]float64{},
		insideTransit:      map[string]float64{},
		outsideByCommodity: map[string]float64{},
	}
	if workspace == "" {
		return held
	}
	for _, r := range p.supplyReservations {
		if r.workspace != workspace || r.status != "active" {
			continue
		}
		item := reservationItem{inventoryKey(r.source, r.sourceID), r.target.commodity}
		held.targets[r.target] += r.remaining
		held.stock[item] += r.remaining
		if selection[reservationBuilding{r.target.project, r.target.building}] {
			held.inside[r.target.commodity] += r.remaining
			if !r.transitAt.IsZero() {
				held.insideTransit[r.target.commodity] += r.remaining
			}
		} else {
			held.outside[item] += r.remaining
			held.outsideByCommodity[r.target.commodity] += r.remaining
		}
	}
	return held
}

func (s reservationStock) available(source, commodity string, held supplyHolds) float64 {
	key := inventoryKey(source, s.sourceIDs[source])
	return max0(s.items[key][commodity] - held.stock[reservationItem{key, commodity}])
}

func (s reservationStock) excludeOutsideHolds(held supplyHolds) {
	for item, reserved := range held.outside {
		amount := math.Min(s.items[item.inventory][item.commodity], reserved)
		if amount <= 0 {
			continue
		}
		if item.inventory == "ship" {
			s.cargo[item.commodity] -= amount
		} else {
			s.warehouse.items[item.commodity] = max0(s.warehouse.items[item.commodity] - amount)
		}
	}
}

func (p *Planner) supplyReservationHistory(workspace string) []M {
	list := []M{}
	systems := map[string]string{}
	for _, project := range p.projects {
		systems[str(project["id"])] = str(project["targetSystem"])
	}
	for i := len(p.supplyReservations) - 1; i >= 0; i-- {
		r := p.supplyReservations[i]
		if r.workspace == workspace {
			row := r.public()
			row["targetSystem"] = firstText(systems[r.target.project], r.system)
			list = append(list, row)
		}
	}
	return list
}

func reservableSupply(buildings []M, stock reservationStock, held supplyHolds) []M {
	options := []M{}
	for _, b := range buildings {
		for _, row := range rows(b["requirements"]) {
			missing := num(row["missing"])
			if missing == 0 {
				continue
			}
			target := reservationTarget{str(b["projectId"]), str(b["id"]), str(row["commodity"])}
			reserved := held.targets[target]
			sources := []M{}
			for _, source := range supplySources {
				if available := math.Min(max0(missing-reserved), stock.available(source, target.commodity, held)); available > 0 {
					sources = append(sources, M{"source": source, "available": available})
				}
			}
			if len(sources) == 0 {
				continue
			}
			options = append(options, M{
				"projectId": target.project, "buildingId": target.building, "buildingName": b["name"],
				"commodity": target.commodity, "name": row["name"], "missing": missing,
				"reserved": reserved, "sources": sources,
			})
		}
	}
	return options
}
