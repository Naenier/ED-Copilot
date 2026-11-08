package app

import (
	"sort"

	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const maxGalacticPrices = 1000

// Galactic averages survive market changes and journal replays. In particular,
// carrier markets often report zero: that never erases a positive observation.
func restoreGalacticPrices(saved M) M {
	out := M{}
	keys := make([]string, 0, len(saved))
	for key := range saved {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		row := v.Obj(saved[key])
		price, ok := v.Number(row["price"])
		if key != v.Key(key) || len(key) > 100 || !ok || price <= 0 || price > 1e9 || v.Time(row["observedAt"]).IsZero() {
			continue
		}
		source := v.Str(row["source"])
		if source != "Market.json" && source != "Frontier CAPI" {
			continue
		}
		out[key] = M{"price": price, "observedAt": row["observedAt"], "source": source}
		if len(out) >= maxGalacticPrices {
			break
		}
	}
	return out
}

// Called with Engine.mu held, together with the local market observations.
func (e *Engine) observeGalacticPricesLocked(event M) {
	at := v.Time(event["timestamp"])
	if at.IsZero() || v.Bool(e.game["isBeta"]) || v.Str(e.game["version"]) != "" && !v.Bool(e.game["live"]) {
		return
	}
	source := "Market.json"
	if event["Source"] == "Frontier CAPI" {
		source = "Frontier CAPI"
	}
	for _, row := range v.Rows(event["Items"]) {
		good := trading.Good(row["Name"], row["Name_Localised"])
		price, ok := v.Number(row["MeanPrice"])
		if good == nil || !ok || price <= 0 || price > 1e9 {
			continue
		}
		key := v.Str(good["key"])
		if key == "" || len(key) > 100 {
			continue
		}
		prior := v.Obj(e.galacticPrices[key])
		if old := v.Time(prior["observedAt"]); !old.IsZero() && !at.After(old) {
			continue
		}
		if _, exists := e.galacticPrices[key]; !exists && len(e.galacticPrices) >= maxGalacticPrices {
			continue
		}
		e.galacticPrices[key] = M{"price": price, "observedAt": event["timestamp"], "source": source}
	}
}
