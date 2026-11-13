package engineering

import (
	"math"
	"sort"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type shipPlanStop struct {
	engineer M
	tasks    []M
	steps    []M
	deps     []string
	grade    int
	referral M
}

func (p *Planner) shipPlanVisit(stops map[string]*shipPlanStop, engineer, observations M) *shipPlanStop {
	key := v.Str(engineer["key"])
	if stop := stops[key]; stop != nil {
		return stop
	}
	stop := &shipPlanStop{engineer: engineer, tasks: []M{}, steps: []M{}}
	stops[key] = stop
	for _, requirement := range v.Rows(roadmapData.engineers[key]["requirements"]) {
		step := p.roadmapRequirement(key, engineer, requirement, observations)
		dependency := v.Str(step["engineerKey"])
		if step["status"] == "done" || dependency != "" && step["status"] == "available" {
			continue
		}
		stop.steps = append(stop.steps, step)
		if dependency != "" && dependency != key && roadmapData.engineers[dependency] != nil {
			prerequisite := p.shipPlanVisit(stops, p.roadmapEngineerState(dependency), observations)
			stop.deps = append(stop.deps, dependency)
			if v.Int(step["rank"]) > 0 && shipPlanReferralRank(step) > shipPlanReferralRank(prerequisite.referral) {
				prerequisite.referral = step
			}
		}
	}
	sort.Strings(stop.deps)
	return stop
}

func shipPlanReferralRank(step M) float64 {
	return float64(v.Int(step["rank"])) + v.Num(step["progress"])/100
}

func shipPlanRankStatus(stop *shipPlanStop) string {
	switch stop.engineer["status"] {
	case "unlocked":
		if stop.referral != nil || v.Int(stop.engineer["rank"]) < stop.grade {
			return "rank-required"
		}
		return "ready"
	case "barred":
		return "blocked"
	case "unknown":
		return "access-unknown"
	default:
		return "access-required"
	}
}

func (p *Planner) shipPlanStopRow(stop *shipPlanStop) M {
	key := v.Str(stop.engineer["key"])
	row := workshopStop(stop.engineer, "upgrade")
	row["status"], row["tasks"] = shipPlanRankStatus(stop), stop.tasks
	if row["status"] == "access-required" || row["status"] == "access-unknown" {
		row["action"] = "unlock"
	}
	row["systemAddress"] = roadmapData.engineers[key]["systemAddress"]
	dependencies := make([]string, 0, len(stop.deps))
	for _, dependency := range stop.deps {
		dependencies = append(dependencies, "engineer:"+dependency)
	}
	row["dependsOn"] = dependencies
	steps := append([]M{}, stop.steps...)
	// If a referral requires a higher rank, the referral step already
	// contains the threshold and any fractional reputation requirement.
	if stop.grade > v.Int(stop.engineer["rank"]) && stop.grade > v.Int(stop.referral["rank"]) {
		step := p.roadmapRankStep(key, stop.engineer)
		step["target"] = stop.grade
		if stop.grade != 5 {
			step["text"] = "Поднимите репутацию до G" + v.Str(stop.grade) + ", выполняя модификации лично в мастерской."
			step["english"] = "Reach Grade " + v.Str(stop.grade) + " reputation by crafting in person at the workshop."
		}
		steps = append(steps, step)
	}
	if stop.referral != nil {
		steps = append(steps, stop.referral)
	}
	row["steps"] = steps
	return row
}

func (p *Planner) orderShipPlanStops(stops map[string]*shipPlanStop, observations M) []M {
	ordered := []M{}
	remaining := make(map[string]int, len(stops))
	dependents := make(map[string][]string, len(stops))
	rows := make(map[string]M, len(stops))
	for key, stop := range stops {
		remaining[key] = len(stop.deps)
		rows[key] = p.shipPlanStopRow(stop)
		for _, dep := range stop.deps {
			dependents[dep] = append(dependents[dep], key)
		}
	}
	origin, system := observations["position"], observations["system"]
	if len(v.Arr(origin)) != 3 {
		origin = shipPlanCoordinates(v.Str(system), observations)
	}
	for len(remaining) > 0 {
		var best M
		bestKey := ""
		for key, degree := range remaining {
			if degree != 0 {
				continue
			}
			row := rows[key]
			destination := v.Str(row["system"])
			row["distance"] = nil
			if row["status"] != "blocked" {
				row["distance"] = shipPlanDistance(origin, shipPlanCoordinates(destination, observations), system, destination)
			}
			if best == nil || shipPlanStopLess(row, best) {
				best, bestKey = row, key
			}
		}
		if best == nil {
			// Leave circular prerequisite data out of the proposed route.
			break
		}
		ordered = append(ordered, best)
		delete(remaining, bestKey)
		for _, next := range dependents[bestKey] {
			remaining[next]--
			if best["status"] == "blocked" {
				rows[next]["status"] = "blocked"
			}
		}
		if best["status"] != "blocked" {
			system = best["system"]
			origin = shipPlanCoordinates(v.Str(system), observations)
		}
	}
	return ordered
}

func shipPlanStopLess(a, b M) bool {
	if (a["status"] == "blocked") != (b["status"] == "blocked") {
		return a["status"] != "blocked"
	}
	da, oka := v.Number(a["distance"])
	db, okb := v.Number(b["distance"])
	if oka != okb {
		return oka
	}
	if oka && da != db {
		return da < db
	}
	if (a["status"] == "ready") != (b["status"] == "ready") {
		return a["status"] == "ready"
	}
	return v.Str(a["id"]) < v.Str(b["id"])
}

func shipPlanCoordinates(system string, observations M) any {
	positions := v.Obj(observations["engineerPositions"])
	if position := v.First(positions[system], positions[v.Key(system)]); len(v.Arr(position)) == 3 {
		return position
	}
	return workshopCoordinates(system)
}

func shipPlanDistance(from, to, fromSystem, toSystem any) any {
	if key := v.Key(fromSystem); key != "" && key == v.Key(toSystem) {
		return 0
	}
	origin, destination := v.Arr(from), v.Arr(to)
	if len(origin) != 3 || len(destination) != 3 {
		return nil
	}
	sum := float64(0)
	for i, raw := range origin {
		x, xOK := v.Number(raw)
		y, yOK := v.Number(destination[i])
		if !xOK || !yOK {
			return nil
		}
		sum += (x - y) * (x - y)
	}
	if distance := math.Sqrt(sum); !math.IsInf(distance, 0) {
		return distance
	}
	return nil
}

func (p *Planner) shipPlanUnlockSupplies(stops map[string]*shipPlanStop, budget map[string]int, observations M) []M {
	supplies := map[string]M{}
	for _, stop := range stops {
		for _, step := range stop.steps {
			if step["status"] == "blocked" || v.Bool(step["combined"]) || step["kind"] != "unlock" && step["kind"] != "donation" {
				continue
			}
			for _, row := range v.Rows(step["items"]) {
				category, key := v.Str(row["category"]), v.Str(row["key"])
				count := max(0, v.Int(row["count"])-v.Int(row["donated"]))
				if key == "" || count == 0 {
					continue
				}
				if category == "raw" || category == "encoded" || category == "manufactured" {
					budget[key] += count
					continue
				}
				id := category + ":" + key
				if existing := supplies[id]; existing != nil {
					existing["count"] = v.Int(existing["count"]) + count
				} else {
					supplies[id] = v.Copy(row)
					supplies[id]["count"] = count
				}
			}
		}
	}
	rows := make([]M, 0, len(supplies))
	for _, row := range supplies {
		delete(row, "donated")
		delete(row, "missing")
		delete(row, "owned")
		if owned, known := p.roadmapStock(v.Str(row["key"]), v.Str(row["category"]), observations); known {
			row["owned"], row["missing"] = owned, max(0, v.Int(row["count"])-owned)
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i]["category"] == rows[j]["category"] {
			return v.Str(rows[i]["key"]) < v.Str(rows[j]["key"])
		}
		return v.Str(rows[i]["category"]) < v.Str(rows[j]["category"])
	})
	return rows
}
