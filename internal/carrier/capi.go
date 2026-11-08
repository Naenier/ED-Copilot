package carrier

import (
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"strconv"
	"time"
)

// ApplyCAPI replaces the observed physical hold, never adds market orders or
// the previous manual baseline to it. Colony procurement independently filters
// stolen/mission cargo; this warehouse is the complete physical hold.
// The caller serializes tracker access with journal application.
func (t *Tracker) ApplyCAPI(result, commander M) bool {
	status := v.Obj(result["status"])
	if v.Str(v.At(result, "metadata", "source")) != "Frontier CAPI" || !v.Bool(status["connected"]) || v.Bool(status["commanderMismatch"]) || v.Str(commander["fid"]) == "" || v.Str(status["fid"]) != v.Str(commander["fid"]) {
		return false
	}
	raw, present := v.Obj(result["data"])["carrier"]
	if !present || raw == nil {
		return false
	}
	record := v.Obj(raw)
	id := v.Str(v.At(record, "market", "id"))
	if _, err := strconv.ParseUint(id, 10, 64); err != nil || id == "0" {
		return false
	}
	c := t.Current()
	if c == nil || id != t.OwnerID || v.Str(c["callsign"]) == "" || v.Str(v.At(record, "name", "callsign")) != v.Str(c["callsign"]) {
		return false
	}
	at, started := v.Time(v.At(result, "metadata", "observedAt", "carrier")), v.Time(v.At(result, "metadata", "requestStartedAt", "carrier"))
	if at.IsZero() || started.IsZero() || at.Before(started) {
		return false
	}
	if previous := v.Time(t.StockBaseline["timestamp"]); !previous.IsZero() && (!at.After(previous) || previous.After(started)) {
		return false
	}
	for _, move := range v.Rows(c["movements"]) {
		if v.Time(move["timestamp"]).After(started) {
			return false
		}
	}
	var rows []M
	switch cargo := record["cargo"].(type) {
	case []M:
		rows = cargo
	case []any:
		rows = []M{}
		for _, entry := range cargo {
			row, ok := entry.(M)
			if !ok {
				return false
			}
			rows = append(rows, row)
		}
	case M:
		rows = []M{}
		for _, entry := range cargo {
			row, ok := entry.(M)
			if !ok {
				return false
			}
			rows = append(rows, row)
		}
	default:
		return false
	}
	if len(rows) > 512 {
		return false
	}
	groups := map[string]M{}
	total := float64(0)
	for _, row := range rows {
		key := v.Key(row["commodity"])
		count, ok := v.Number(row["qty"])
		if key == "" || !ok || math.IsNaN(count) || math.IsInf(count, 0) || count < 0 || math.Trunc(count) != count || count > 25000 {
			return false
		}
		for _, flag := range []string{"stolen", "mission"} {
			if raw, exists := row[flag]; exists && raw != nil {
				if _, valid := raw.(bool); !valid {
					return false
				}
			}
		}
		if groups[key] == nil {
			groups[key] = M{"key": key, "name": trading.Name(key, row["locName"]), "count": float64(0)}
		}
		groups[key]["count"] = v.Num(groups[key]["count"]) + count
		total += count
		if total > 25000 {
			return false
		}
	}
	// A later HTTP response can still contain the provider's pre-transfer
	// cargo. Compare commodities changed after the known baseline before
	// replacing it; adding their deltas to the response could double count.
	// Protect each recent movement for at most 15 minutes after the latest
	// movement, so an ordinary poll near an old baseline's expiry cannot erase
	// a transfer made moments ago. This does not refresh the baseline itself.
	if v.Str(t.StockBaseline["carrierId"]) == id && t.StockBaseline["complete"] != false {
		pending := map[string]bool{}
		baselineAt := v.Time(t.StockBaseline["timestamp"])
		latestPending := baselineAt
		for _, move := range v.Rows(c["movements"]) {
			if moved := v.Time(move["timestamp"]); moved.After(baselineAt) {
				pending[v.Str(move["key"])] = true
				if moved.After(latestPending) {
					latestPending = moved
				}
			}
		}
		if len(pending) > 0 && at.Before(latestPending.Add(15*time.Minute)) {
			for _, row := range v.Rows(t.warehouse(c)["items"]) {
				key := v.Str(row["key"])
				if pending[key] && v.Num(row["count"]) != v.Num(groups[key]["count"]) {
					return false
				}
			}
		}
	}
	items := []M{}
	for _, row := range groups {
		items = append(items, row)
	}
	t.StockBaseline = M{"carrierId": id, "timestamp": v.ISO(at), "items": items, "complete": true, "source": "Frontier CAPI"}
	return true
}
