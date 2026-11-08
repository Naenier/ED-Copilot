package frontier

import (
	v "github.com/Naenier/ED-Copilot/internal/value"
	"testing"
)

func TestVerifiedCAPIMarketPreservesGalacticAverageApartFromLocalOffer(t *testing.T) {
	result := v.M{"status": v.M{"connected": true, "commander": "Synthetic"}, "data": v.M{"profile": v.M{"commander": v.M{"name": "Synthetic", "docked": true}, "lastStarport": v.M{"id": 42}}, "market": v.M{"id": 42, "commodities": []v.M{{"name": "steel", "buyPrice": 1000, "sellPrice": 900, "meanPrice": 4179, "stock": 10000, "demand": 0, "stockBracket": 3, "demandBracket": 0}}}}, "metadata": v.M{"source": "Frontier CAPI", "observedAt": v.M{"market": "2026-11-07T12:00:00Z"}}}
	market := MarketSnapshot(result)
	rows := v.Rows(market["Items"])
	if len(rows) != 1 || v.Num(rows[0]["MeanPrice"]) != 4179 || v.Num(rows[0]["SellPrice"]) != 900 {
		t.Fatal(market)
	}
}
