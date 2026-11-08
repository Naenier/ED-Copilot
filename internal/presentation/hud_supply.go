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
	const limit = 3
	rows := []HUDLine{}
	covered, uncertain, warehouseVisible := 0, false, false
	for _, item := range supplies {
		if item.covered {
			covered++
		}
		uncertain = uncertain || item.uncertain
		warehouseVisible = warehouseVisible || v.Num(item.carrier) > 0
	}
	for _, item := range hudVisibleColonySupplies(supplies, limit) {
		r := item.row
		text := v.Text(r["name"], r["commodity"]) + ": осталось " + Number(r["missing"]) + " т"
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
	if len(supplies) > 0 && (len(supplies) > limit || warehouseVisible || v.Bool(colony["includeCarrierWarehouse"])) {
		parts := []string{}
		if len(supplies) > limit {
			parts = append(parts, "Ещё позиций: "+Number(len(supplies)-limit))
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

// Represent the next three different actions when the bill is longer than
// the HUD: deliver loaded cargo, load an already covered carrier commodity,
// and obtain an uncovered commodity. Three loaded rows must not hide the
// useful fact that a fourth commodity is already aboard the carrier.
func hudVisibleColonySupplies(all []hudColonySupply, limit int) []hudColonySupply {
	if len(all) <= limit {
		return all
	}
	selected, used := []hudColonySupply{}, map[int]bool{}
	pick := func(matches func(hudColonySupply) bool) {
		for index, item := range all {
			if len(selected) == limit {
				return
			}
			if !used[index] && matches(item) {
				selected, used[index] = append(selected, item), true
				return
			}
		}
	}
	pick(func(item hudColonySupply) bool { return v.Num(item.row["cargoAvailable"]) > 0 })
	pick(func(item hudColonySupply) bool {
		return item.covered && v.Num(item.carrier) > 0 && v.Num(item.row["cargoAvailable"]) < v.Num(item.row["missing"])
	})
	pick(func(item hudColonySupply) bool { return !item.covered })
	for len(selected) < limit {
		pick(func(hudColonySupply) bool { return true })
	}
	return selected
}
