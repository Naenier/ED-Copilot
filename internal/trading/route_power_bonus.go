package trading

import (
	"math"
	"strings"
	"sync"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

var routeCommodityCategories = sync.OnceValue(func() map[string]string {
	categories := map[string]string{}
	for _, row := range data.Array("encyclopedia-commodity-metadata.json") {
		categories[v.Key(row["key"])] = v.Str(row["category"])
	}
	return categories
})

// Commodity profit rewards shown on Inara's route cards are applied to the
// price spread (e.g. Torval's +45% for metals and minerals). General trade
// bonds are collected separately in the game and aren't included in this
// projection. See https://inara.cz/elite/powers/ for published rank rewards.
func routePowerPercent(power, key, category string, rare bool) float64 {
	switch v.Key(power) {
	case "zeminatorval":
		if category == "Metals" || category == "Minerals" || key == "imperialslaves" {
			return 45
		}
	case "nakatokaine":
		if category == "Metals" || category == "Minerals" {
			return 50
		}
	case "feliciawinters":
		switch category {
		case "Foods", "Medicines":
			return 60
		case "Salvage":
			return 100
		}
	case "pranavantal":
		if category == "Technology" {
			return 50
		}
	case "edmundmahon":
		if rare {
			return 50
		}
	}
	return 0
}

func AddRoutePowerBonus(route M) {
	type bonus struct {
		perUnit, total float64
	}
	byPower := map[string]bonus{}
	baseProfit := 0.0
	for _, leg := range v.Rows(route["legs"]) {
		margin := n(leg, "sellPrice") - n(leg, "buyPrice")
		quantity := n(leg, "quantity")
		baseProfit += margin * quantity
		sell := v.Obj(leg["sell"])
		power := v.Str(sell["controllingPower"])
		switch v.Key(sell["powerplayState"]) {
		case "exploited", "fortified", "stronghold", "controlled":
		default:
			continue
		}
		key := v.Key(leg["key"])
		category := routeCommodityCategories()[key]
		rare := strings.EqualFold(category, "rare") || v.Bool(leg["rare"])
		perUnit := margin * routePowerPercent(power, key, category, rare) / 100
		if perUnit <= 0 || quantity <= 0 {
			continue
		}
		values := byPower[power]
		values.perUnit += perUnit
		values.total += perUnit * quantity
		byPower[power] = values
	}
	// A commander can only pledge to one Power.
	var bestPower string
	var best bonus
	for power, values := range byPower {
		if values.total > best.total || values.total == best.total && power < bestPower {
			best, bestPower = values, power
		}
	}
	if bestPower != "" {
		route["powerBonusPerUnit"] = math.Round(best.perUnit)
		route["powerBonusProfit"] = math.Round(best.total)
		route["profitWithPowerBonus"] = baseProfit + math.Round(best.total)
		route["powerBonusPower"] = bestPower
	}
}
