package engineering

import (
	"math"
	"strconv"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// ShipFitMaterials budgets a design from unmodified equipment, using the
// current Live G5 workshop rule. The fitter does not identify a specific
// owned module, so a journal loadout cannot safely reduce a design's cost.
func (p *Planner) ShipFitMaterials(parts []M, stockReady bool) M {
	totals := map[string]int{}
	unavailable := []M{}
	estimated := false
	for _, part := range parts {
		blueprint, experimental := v.Str(part["blueprint"]), v.Str(part["experimental"])
		if !v.Bool(part["engineeringLocked"]) && blueprint != "" {
			grades := v.Obj(blueprints[journalUpgradeIndex().blueprintKeys[v.Key(blueprint)]]["grades"])
			grade := v.Int(part["grade"])
			if grade >= 1 && grade <= 5 && len(grades) != 0 {
				quality := 1.0
				if value, known := v.Number(part["quality"]); known {
					quality = v.Clamp(value, 0, 1)
				}
				for g := 1; g <= grade; g++ {
					rows := v.Rows(grades[strconv.Itoa(g)])
					if len(rows) == 0 {
						unavailable = append(unavailable, M{"slot": part["slot"], "blueprint": blueprint, "grade": g, "reason": "blueprint"})
						continue
					}
					rolls := g
					if g == grade {
						// A design stops at the first whole craft that reaches the
						// requested quality. Coriolis can round fractional inputs.
						rolls = max(1, int(math.Ceil(float64(g)*quality-1e-8)))
					}
					accumulateIngredients(totals, rows, rolls)
					estimated = true
				}
			} else {
				unavailable = append(unavailable, M{"slot": part["slot"], "blueprint": blueprint, "grade": part["grade"], "reason": "blueprint"})
			}
		}
		if experimental != "" && v.Key(experimental) != v.Key(part["factoryExperimental"]) {
			rows := v.Rows(effects[journalUpgradeIndex().effectKeys[v.Key(experimental)]]["materials"])
			if len(rows) == 0 {
				unavailable = append(unavailable, M{"slot": part["slot"], "experimental": experimental, "reason": "experimental"})
			} else {
				accumulateIngredients(totals, rows, 1)
			}
		}
	}

	p.mu.Lock()
	rows := p.ingredientRows(ingredientTotals(totals), 1, false, nil)
	p.mu.Unlock()
	groups := []M{}
	for _, category := range []string{"raw", "manufactured", "encoded"} {
		entries := []M{}
		required, missing, unknown := 0, 0, 0
		for _, row := range rows {
			if row["category"] != category {
				continue
			}
			row["required"] = row["count"]
			delete(row, "count")
			if !stockReady {
				delete(row, "owned")
				delete(row, "missing")
			}
			required += v.Int(row["required"])
			if row["owned"] == nil {
				unknown++
			} else {
				missing += v.Int(row["missing"])
			}
			entries = append(entries, row)
		}
		groups = append(groups, M{"category": category, "rows": entries, "required": required, "missing": missing, "unknown": unknown})
	}
	currencies := []M{}
	for _, row := range rows {
		if row["category"] == "currency" {
			row["required"] = row["count"]
			delete(row, "count")
			currencies = append(currencies, row)
		}
	}
	return M{"groups": groups, "currencies": currencies, "unavailable": unavailable, "estimated": estimated}
}
