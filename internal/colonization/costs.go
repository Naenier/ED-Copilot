package colonization

import (
	"math"
	"time"

	"github.com/Naenier/ED-Copilot/internal/freshness"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const maxEstimateAge = 7 * 24 * time.Hour

// EnrichCosts annotates an owned desktop snapshot. The planner and its saved
// resource quantities are untouched. Prices are game MeanPrice observations,
// not local market offers, manually entered prices or recorded expenditure.
// Callers deliberately omit this work for the in-game HUD and sync snapshots.
func EnrichCosts(snapshot, prices M) {
	enrichCostsAt(snapshot, prices, time.Now())
}

func enrichCostsAt(snapshot, prices M, now time.Time) {
	project := v.Obj(snapshot["activeProject"])
	if len(project) == 0 {
		return
	}
	summary := newCostSummary()
	unknownBills, planned, building, manual := 0, 0, 0, 0
	for _, b := range rows(project["buildings"]) {
		status := str(b["status"])
		if status != "planned" && status != "building" {
			continue
		}
		if status == "planned" {
			planned++
		} else {
			building++
		}
		requirements := rows(b["requirements"])
		if len(requirements) == 0 {
			unknownBills++
		}
		if str(b["billSource"]) == "manual" {
			manual++
		}
		own := newCostSummary()
		for _, row := range requirements {
			annotatePrice(row, prices)
			for _, field := range []string{"required", "delivered", "missing"} {
				amount := row[field]
				if field == "delivered" {
					amount = math.Min(num(row["required"]), num(amount))
				}
				row[field+"Cost"] = resourceCost(amount, row["galacticAveragePrice"])
				addCostMetric(v.Obj(own[field]), amount, row["galacticAveragePrice"])
			}
		}
		for _, field := range []string{"required", "delivered", "missing"} {
			finishCostMetric(v.Obj(own[field]), len(requirements) == 0)
		}
		own["unknownBills"] = boolInt(len(requirements) == 0)
		b["costSummary"] = own
	}
	priced, stalePrices, undatedPrices, commodityCount := 0, 0, 0, 0
	var oldest, latest time.Time
	for _, row := range rows(project["supplies"]) {
		annotatePrice(row, prices)
		commodityCount++
		price := row["galacticAveragePrice"]
		stamp := v.Time(row["galacticAveragePriceAt"])
		observed, fetched := stamp, time.Time{}
		// The current CAPI market metadata records receipt. The actual market
		// observation time is only available for journal prices.
		if row["galacticAveragePriceSource"] == "Frontier CAPI" {
			observed, fetched = time.Time{}, stamp
		}
		quality := freshness.Describe(now, maxEstimateAge, price != nil, "", observed, fetched)
		row["priceQuality"] = quality
		if price != nil {
			priced++
			switch quality.State {
			case freshness.Stale:
				stalePrices++
			case freshness.Undated:
				undatedPrices++
			}
			if oldest.IsZero() || stamp.Before(oldest) {
				oldest = stamp
			}
			if stamp.After(latest) {
				latest = stamp
			}
		}
		cargo, carrier := coveredAmounts(row)
		row["cargoCovered"], row["carrierCovered"] = cargo, carrier
		quantities := M{"required": row["required"], "delivered": math.Min(num(row["required"]), num(row["delivered"])), "missing": row["missing"], "cargoCovered": cargo, "carrierCovered": carrier, "toBuy": row["toBuy"], "toBuyAtMost": row["toBuyAtMost"]}
		for _, field := range costFields {
			amount := quantities[field]
			row[field+"Cost"] = resourceCost(amount, price)
			addCostMetric(v.Obj(summary[field]), amount, price)
		}
	}
	for _, field := range costFields {
		finishCostMetric(v.Obj(summary[field]), unknownBills > 0)
	}
	summary["unknownBills"], summary["plannedBuildings"], summary["buildingBuildings"], summary["manualBills"] = unknownBills, planned, building, manual
	summary["pricedCommodityCount"], summary["commodityCount"] = priced, commodityCount
	summary["stalePriceCount"], summary["undatedPriceCount"] = stalePrices, undatedPrices
	summary["oldestPriceAt"], summary["latestPriceAt"] = nil, nil
	if !oldest.IsZero() {
		summary["oldestPriceAt"] = v.ISO(oldest)
	}
	if !latest.IsZero() {
		summary["latestPriceAt"] = v.ISO(latest)
	}
	summary["basis"] = "game-galactic-average"
	project["costSummary"] = summary
}

var costFields = []string{"required", "delivered", "missing", "cargoCovered", "carrierCovered", "toBuy", "toBuyAtMost"}

func newCostSummary() M {
	out := M{}
	for _, field := range costFields {
		out[field] = M{"value": nil, "knownValue": float64(0), "complete": false, "unpricedCommodityCount": 0, "unknownQuantityCount": 0}
	}
	return out
}

func annotatePrice(row, prices M) {
	price := v.Obj(prices[v.Key(row["commodity"])])
	row["galacticAveragePrice"], row["galacticAveragePriceAt"], row["galacticAveragePriceSource"] = nil, nil, nil
	if n, ok := v.Number(price["price"]); ok && n > 0 && n <= 1e9 && !v.Time(price["observedAt"]).IsZero() {
		row["galacticAveragePrice"], row["galacticAveragePriceAt"], row["galacticAveragePriceSource"] = n, price["observedAt"], price["source"]
	}
}

func resourceCost(quantity, price any) any {
	q, ok := v.Number(quantity)
	if !ok || q < 0 {
		return nil
	}
	if q == 0 {
		return float64(0)
	}
	p, ok := v.Number(price)
	if !ok || p <= 0 || p > 1e9 {
		return nil
	}
	result := q * p
	if math.IsInf(result, 0) || math.IsNaN(result) {
		return nil
	}
	return result
}

func addCostMetric(metric M, quantity, price any) {
	q, ok := v.Number(quantity)
	if !ok || q < 0 {
		metric["unknownQuantityCount"] = v.Int(metric["unknownQuantityCount"]) + 1
		return
	}
	amount := resourceCost(q, price)
	if amount == nil {
		metric["unpricedCommodityCount"] = v.Int(metric["unpricedCommodityCount"]) + 1
		return
	}
	metric["knownValue"] = num(metric["knownValue"]) + num(amount)
}

func finishCostMetric(metric M, unknownBill bool) {
	complete := !unknownBill && v.Int(metric["unpricedCommodityCount"]) == 0 && v.Int(metric["unknownQuantityCount"]) == 0
	metric["complete"] = complete
	if complete {
		metric["value"] = metric["knownValue"]
	}
}

func coveredAmounts(row M) (any, any) {
	missing := num(row["missing"])
	ship, known := v.Number(row["cargoAvailable"])
	if missing <= 0 {
		return float64(0), float64(0)
	}
	if !known || ship < 0 {
		return nil, nil
	}
	cargo := math.Min(missing, ship)
	remaining := missing - cargo
	if !v.Bool(row["carrierIncluded"]) || remaining == 0 {
		return cargo, float64(0)
	}
	carrier, known := v.Number(row["carrierAvailable"])
	if !known || carrier < 0 {
		return cargo, nil
	}
	if !v.Bool(row["carrierComplete"]) && carrier < remaining {
		return cargo, nil
	}
	return cargo, math.Min(remaining, carrier)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
