package presentation

import (
	"sort"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// The physical warehouse view remains useful when procurement accounting is
// off. It never turns warehouse cargo into a delivered construction supply.
type hudColonyStock struct {
	items    map[string]float64
	known    bool
	complete bool
}

func hudPhysicalColonyStock(colony M) hudColonyStock {
	out := hudColonyStock{items: map[string]float64{}, complete: true}
	seen := map[string]bool{}
	for _, warehouse := range v.Rows(colony["carrierWarehouses"]) {
		// The source sets accessible only after checking the active FID and
		// the explicit, identity-bound permission to unload shared cargo.
		if !v.Bool(warehouse["accessible"]) {
			continue
		}
		if !v.Bool(warehouse["known"]) {
			out.complete = false
			continue
		}
		if id := v.Str(warehouse["id"]); id != "" {
			if seen[id] {
				continue
			}
			seen[id] = true
		}
		out.known = true
		for _, item := range v.Rows(warehouse["items"]) {
			key := v.Key(v.First(item["key"], item["commodity"]))
			if key != "" && hudKnown(item["count"]) {
				out.items[key] += v.Num(item["count"])
			}
		}
	}
	out.complete = out.complete && out.known
	return out
}

type hudColonySupply struct {
	row             M
	carrier         any
	carrierComplete bool
	covered         bool
	uncertain       bool
}

func hudColonySupplies(colony, project M) []hudColonySupply {
	stock := hudPhysicalColonyStock(colony)
	result := []hudColonySupply{}
	for _, row := range v.Rows(project["operationalSupplies"]) {
		missing := v.Num(row["missing"])
		if missing <= 0 {
			continue
		}
		var carrier any
		complete := stock.complete
		if amount, ok := stock.items[v.Key(row["commodity"])]; ok {
			carrier = amount
		} else if stock.complete {
			carrier = float64(0)
		} else if v.Bool(row["carrierIncluded"]) && hudKnown(row["carrierAvailable"]) {
			// A scoped precomputed supply DTO can carry the same verified
			// observation without including the full warehouse listing.
			carrier, complete = row["carrierAvailable"], v.Bool(row["carrierComplete"])
		}
		ship := row["cargoAvailable"]
		covered := v.Num(ship)+v.Num(carrier) >= missing
		uncertain := !covered && (!hudKnown(ship) || carrier == nil || !complete)
		result = append(result, hudColonySupply{row, carrier, complete, covered, uncertain})
	}
	priority := func(item hudColonySupply) int {
		if v.Num(item.row["cargoAvailable"]) > 0 {
			return 3
		}
		if v.Num(item.carrier) > 0 {
			if item.covered {
				return 2
			}
			return 1
		}
		return 0
	}
	sort.SliceStable(result, func(i, j int) bool { return priority(result[i]) > priority(result[j]) })
	return result
}

func hudColonySupplyLines(colony, project M, supplies []hudColonySupply) []HUDLine {
	const limit = 10
	rows := []HUDLine{}
	covered, uncertain, warehouseVisible := 0, false, false
	for _, item := range supplies {
		if item.covered {
			covered++
		}
		uncertain = uncertain || item.uncertain
		warehouseVisible = warehouseVisible || v.Num(item.carrier) > 0
	}
	pending := hudPendingColonySupplies(colony, supplies)
	for _, item := range hudVisibleColonySupplies(pending, limit) {
		r := item.row
		text := v.Text(r["name"], r["commodity"]) + ": осталось " + Number(r["missing"]) + " т"
		// With warehouse accounting on, the overlay is the next shopping or
		// delivery action. The desktop retains the full bill and stock columns.
		if v.Bool(colony["includeCarrierWarehouse"]) && v.Bool(r["carrierIncluded"]) {
			switch {
			case hudKnown(r["toBuy"]) && v.Num(r["toBuy"]) > 0:
				text = v.Text(r["name"], r["commodity"]) + ": купить " + Number(r["toBuy"]) + " т"
			case r["toBuy"] == nil && hudKnown(r["toBuyAtMost"]) && v.Num(r["toBuyAtMost"]) > 0:
				text = v.Text(r["name"], r["commodity"]) + ": купить ≤" + Number(r["toBuyAtMost"]) + " т"
			case item.covered && v.Num(r["cargoAvailable"]) > 0:
				text = v.Text(r["name"], r["commodity"]) + ": доставить " + Number(min(v.Num(r["cargoAvailable"]), v.Num(r["missing"]))) + " т"
			default:
				if v.Num(item.carrier) > 0 {
					qualifier := ""
					if !item.carrierComplete {
						qualifier = "≥"
					}
					text += " · ФН: " + qualifier + Number(item.carrier) + " т"
				} else if item.carrier == nil || !item.carrierComplete {
					text += " · ФН: ?"
				}
			}
			role := "text"
			if item.covered {
				role = "good"
			}
			if !v.Bool(r["authoritative"]) && strings.Contains(v.Str(r["source"]), "manual") {
				text += " (оценка)"
				role = "warn"
			}
			rows = append(rows, HUDLine{text, role})
			continue
		}
		if hudKnown(r["cargoAvailable"]) && v.Num(r["cargoAvailable"]) > 0 {
			text += " · на борту " + Number(r["cargoAvailable"]) + " т"
		}
		if v.Num(item.carrier) > 0 {
			qualifier := ""
			if !item.carrierComplete {
				qualifier = "≥"
			}
			text += " · ФН: " + qualifier + Number(item.carrier) + " т"
		} else if v.Bool(r["carrierIncluded"]) && item.carrier == nil {
			text += " · ФН: ?"
		}
		role := "text"
		if item.covered {
			text += " ✓"
			role = "good"
		} else if v.Bool(r["carrierIncluded"]) {
			if hudKnown(r["toBuy"]) && v.Num(r["toBuy"]) > 0 {
				text += " · купить " + Number(r["toBuy"]) + " т"
			} else if r["toBuy"] == nil && hudKnown(r["toBuyAtMost"]) && v.Num(r["toBuyAtMost"]) > 0 {
				text += " · купить ≤" + Number(r["toBuyAtMost"]) + " т"
			}
		}
		if !v.Bool(r["authoritative"]) && strings.Contains(v.Str(r["source"]), "manual") {
			text += " (оценка)"
			role = "warn"
		}
		rows = append(rows, HUDLine{text, role})
	}
	if len(supplies) > 0 && (len(pending) > limit || warehouseVisible || v.Bool(colony["includeCarrierWarehouse"])) {
		parts := []string{}
		if len(pending) > limit {
			parts = append(parts, "Ещё позиций: "+Number(len(pending)-limit))
		}
		if uncertain && covered == 0 {
			parts = append(parts, "Обеспечено: ?/"+Number(len(supplies)))
		} else {
			qualifier := ""
			if uncertain {
				qualifier = "≥"
			}
			parts = append(parts, "Обеспечено: "+qualifier+Number(covered)+"/"+Number(len(supplies)))
		}
		rows = append(rows, hudMuted(strings.Join(parts, " · ")))
	}
	if warehouseVisible && !v.Bool(colony["includeCarrierWarehouse"]) {
		rows = append(rows, hudMuted("Учёт склада ФН выключен"))
	}
	if n := v.Int(project["unknownBills"]); n > 0 {
		rows = append(rows, hudWarn("Сметы неизвестны: "+Number(n)))
	}
	return rows
}

// A confirmed carrier quantity can remove an item from this task list only
// when the commander opted into warehouse accounting. Unknown/stale cargo
// remains actionable. Ship cargo still needs delivery, so it stays visible.
func hudPendingColonySupplies(colony M, all []hudColonySupply) []hudColonySupply {
	if !v.Bool(colony["includeCarrierWarehouse"]) {
		return all
	}
	pending := make([]hudColonySupply, 0, len(all))
	for _, item := range all {
		if hudKnown(item.carrier) && v.Num(item.carrier) >= v.Num(item.row["missing"]) {
			continue
		}
		pending = append(pending, item)
	}
	return pending
}

// The stable source order already prioritizes loaded ship cargo and partial
// carrier stock. Bound the working preview without reordering domain state.
func hudVisibleColonySupplies(all []hudColonySupply, limit int) []hudColonySupply {
	return all[:min(len(all), max(0, limit))]
}
