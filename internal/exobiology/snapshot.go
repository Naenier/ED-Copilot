package exobiology

import (
	"math"
	"sort"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (s *Service) Snapshot() M       { s.mu.Lock(); defer s.mu.Unlock(); return s.snapshot(false) }
func (s *Service) SnapshotForHUD() M { s.mu.Lock(); defer s.mu.Unlock(); return s.snapshot(true) }
func (s *Service) SelectorSnapshot() M {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return M{"settings": v.Copy(s.settings), "selectedBodyKey": s.selected, "currentBodyKey": s.current, "bodyCount": len(s.bodies)}
}

func (s *Service) snapshot(hud bool) M {
	selected := s.selectedBody()
	current := s.bodies[s.current]
	out := M{"schemaVersion": SchemaVersion, "settings": v.Copy(s.settings), "selectedBodyKey": s.selected, "currentBodyKey": s.current, "selectedBody": s.bodySnapshot(selected, true, hud), "currentBody": nil, "tracker": s.trackerSnapshot(), "summary": s.summary(), "bodyCount": len(s.bodies)}
	out["system"] = s.systemSnapshot()
	if s.tracker == nil {
		out["tracker"] = nil
	}
	if current != nil {
		if selected == current {
			out["currentBody"] = out["selectedBody"]
		} else {
			out["currentBody"] = s.bodySnapshot(current, true, hud)
		}
	}
	if hud {
		out["targets"] = s.targets(5)
		out["surfaceMarks"] = s.marksSnapshotLimit(current, 3)
		out["alerts"] = recent(s.alerts, 3)
		return out
	}
	bodies := []*body{}
	for _, b := range s.bodies {
		if b.Signals != nil || len(b.Confirmed) > 0 || b.Mapped {
			bodies = append(bodies, b)
		}
	}
	sort.Slice(bodies, func(i, j int) bool {
		if bodies[i].UpdatedAt == bodies[j].UpdatedAt {
			return bodies[i].Key < bodies[j].Key
		}
		return bodies[i].UpdatedAt > bodies[j].UpdatedAt
	})
	rows := []M{}
	for _, b := range bodies {
		if len(rows) >= 200 {
			break
		}
		rows = append(rows, s.bodySnapshot(b, false, false))
	}
	out["bodies"], out["targets"], out["pending"], out["sales"], out["discoveries"] = rows, s.targets(200), recent(s.pending, 200), recent(s.sales, 200), recent(s.discoveries, 200)
	out["bookmarks"], out["predictionQuality"], out["alerts"], out["stats"] = v.Clone(s.bookmarks), s.quality(), recent(s.alerts, 100), s.statistics()
	out["surfaceMarks"] = s.marksSnapshot(selected)
	out["position"] = v.Copy(s.liveFix)
	return out
}

func (s *Service) bodySnapshot(b *body, full, hud bool) M {
	if b == nil {
		return nil
	}
	out := M{"key": b.Key, "bodyKey": b.Key, "systemAddress": b.Address, "bodyId": b.ID, "name": b.Name, "system": b.System, "signals": b.Signals, "geologicalSignals": b.GeologicalSignals, "genusHints": v.Clone(b.GenusHints), "mapped": b.Mapped, "firstFootfall": footfallState(b), "wasFootfalled": b.WasFootfalled, "ownFirstFootfall": b.OwnFirstFootfall, "updatedAt": b.UpdatedAt, "confirmed": []M{}, "predictions": []M{}, "valueRange": nil, "progress": nil}
	confirmed := []M{}
	for _, key := range sortedKeys(b.Confirmed) {
		confirmed = append(confirmed, v.Copy(b.Confirmed[key]))
	}
	out["confirmed"] = confirmed
	out["progress"] = s.bodyProgress(b)
	if full {
		rows := s.predict(b)
		out["valueRange"] = s.bodyValueRange(b, rows)
		display := []M{}
		for _, p := range rows {
			if !s.predictionVisible(p) {
				continue
			}
			display = append(display, v.Copy(p))
			if hud && len(display) >= 5 {
				break
			}
		}
		out["predictions"], out["predictionCount"], out["scan"] = display, len(rows), v.Copy(b.Scan)
		if host := s.hostStar(b); host != nil {
			out["hostStar"] = M{"name": host.Name, "starType": host.Scan["StarType"], "luminosity": host.Scan["Luminosity"]}
		}
	} else if b.PredictionCache != nil && b.PredictionRevision == s.bodyPredictionRevision(b) {
		out["valueRange"] = s.bodyValueRange(b, b.PredictionCache)
	} else if b.TargetCache != nil && b.TargetRevision == s.bodyPredictionRevision(b) {
		out["valueRange"] = v.Clone(b.TargetCache["valueRange"])
	}
	return out
}

func (s *Service) predictionVisible(p M) bool {
	if p["confirmed"] == true {
		return true
	}
	if s.settings["showUnlikely"] != true && p["tier"] == "unlikely" {
		return false
	}
	if s.settings["includeBacterium"] != true && p["group"] == "bacterium" {
		return false
	}
	genus := v.Str(s.settings["selectedGenus"])
	if genus != "" && genus != "all" && genus != p["group"] {
		return false
	}
	if v.Num(s.settings["minValue"]) > 0 {
		if n, ok := v.Number(p["estimatedCredits"]); !ok || n < v.Num(s.settings["minValue"]) {
			return false
		}
	}
	query := strings.ToLower(v.Str(s.settings["searchQuery"]))
	return query == "" || strings.Contains(strings.ToLower(v.Str(p["name"])+" "+v.Str(p["name_en"])+" "+v.Str(p["genus"])), query)
}

func (s *Service) bodyProgress(b *body) M {
	rows := []M{}
	seen := map[string]bool{}
	d := Catalog()
	for _, hint := range b.GenusHints {
		id := genusID(hint["genus"], hint["name"])
		if id == "" {
			continue
		}
		seen[id] = true
		g := d.genusByID[id]
		row := M{"genusId": id, "name": g["name"], "name_en": g["name_en"], "stage": "dss", "sampleCount": 0}
		if c := b.Confirmed[normal(g["name_en"])]; c != nil {
			for k, value := range c {
				row[k] = v.Clone(value)
			}
			row["stage"] = progressStage(c)
		}
		rows = append(rows, row)
	}
	for _, key := range sortedKeys(b.Confirmed) {
		c := b.Confirmed[key]
		id := genusID(c["genus"])
		if seen[id] {
			continue
		}
		row := v.Copy(c)
		row["genusId"], row["stage"] = id, progressStage(c)
		rows = append(rows, row)
	}
	complete := 0
	for _, r := range rows {
		if r["stage"] == "complete" || r["stage"] == "sold" {
			complete++
		}
	}
	return M{"genera": rows, "confirmedGenera": len(b.Confirmed), "completeGenera": complete, "signals": b.Signals, "mapped": b.Mapped, "allSamplesComplete": func() bool { n, ok := v.Number(b.Signals); return ok && n > 0 && complete >= int(n) }()}
}
func progressStage(c M) string {
	switch c["phase"] {
	case "sold":
		return "sold"
	case "complete":
		return "complete"
	case "lost":
		return "lost"
	case "codex":
		return "composition"
	}
	if v.Int(c["sampleCount"]) > 0 {
		return "sampling"
	}
	return "seen"
}

// Each biological signal occupies a genus, not every possible species in it.
// Missing prices or an unknown signal count produce null rather than a fake 0.
func (s *Service) bodyValueRange(b *body, predictions []M) M {
	count, known := v.Number(b.Signals)
	if !known && len(b.GenusHints) > 0 {
		count = float64(len(b.GenusHints))
		known = true
	}
	out := M{"minimum": nil, "maximum": nil, "signals": b.Signals, "complete": false, "firstFootfall": footfallState(b), "basis": "candidate-genera-estimate"}
	if !known {
		return out
	}
	if count == 0 {
		out["minimum"], out["maximum"], out["complete"] = 0, 0, true
		return out
	}
	type span struct{ lo, hi float64 }
	genera := map[string]span{}
	mandatory := map[string]bool{}
	for _, confirmation := range b.Confirmed {
		id := genusID(confirmation["genus"])
		if id != "" {
			mandatory[id] = true
		} else {
			return out
		}
	}
	for _, p := range predictions {
		if p["tier"] == "unlikely" && p["confirmed"] != true {
			continue
		}
		price, ok := v.Number(p["estimatedCredits"])
		if !ok {
			continue
		}
		id := v.Str(p["group"])
		prior, found := genera[id]
		if !found {
			genera[id] = span{price, price}
		} else {
			genera[id] = span{min(prior.lo, price), max(prior.hi, price)}
		}
	}
	if len(genera) < int(count) {
		return out
	}
	lows, highs := []float64{}, []float64{}
	low, high := 0.0, 0.0
	for id := range mandatory {
		r, exists := genera[id]
		if !exists {
			return out
		}
		low += r.lo
		high += r.hi
	}
	remaining := max(0, int(count)-len(mandatory))
	for id, r := range genera {
		if mandatory[id] {
			continue
		}
		lows = append(lows, r.lo)
		highs = append(highs, r.hi)
	}
	sort.Float64s(lows)
	sort.Sort(sort.Reverse(sort.Float64Slice(highs)))
	if len(lows) < remaining {
		return out
	}
	for i := 0; i < remaining; i++ {
		low += lows[i]
		high += highs[i]
	}
	out["minimum"], out["maximum"], out["complete"] = low, high, true
	return out
}

func (s *Service) targets(limit int) []M {
	revision := s.predictionRevision + s.settingsRevision
	if s.targetCache != nil && s.targetRevision == revision {
		return cloneRows(s.targetCache[:min(limit, len(s.targetCache))])
	}
	out := []M{}
	for _, b := range s.bodies {
		signals, known := v.Number(b.Signals)
		if known && signals <= 0 && len(b.Confirmed) == 0 {
			continue
		}
		if !known && len(b.Confirmed) == 0 {
			continue
		}
		if signals < v.Num(s.settings["minSignals"]) {
			continue
		}
		if s.settings["onlyUnwalked"] == true && footfallState(b) != "unwalked" {
			continue
		}
		if distance, ok := v.Number(b.Scan["DistanceFromArrivalLS"]); ok && v.Num(s.settings["maxArrivalLs"]) > 0 && distance > v.Num(s.settings["maxArrivalLs"]) {
			continue
		}
		row := s.targetRow(b)
		if row == nil {
			continue
		}
		row["bookmarked"] = false
		for _, bookmark := range s.bookmarks {
			if bookmark["bodyKey"] == b.Key {
				row["bookmarked"] = true
			}
		}
		row["distanceLy"] = nil
		if s.currentSystem != "" {
			from, to := s.systems[s.currentSystem], s.systems[b.Address]
			if from != nil && to != nil {
				distance, ok := coordinatesDistance(from.Coordinates, to.Coordinates)
				if ok {
					if radius := v.Num(s.settings["radiusLy"]); radius > 0 && distance > radius {
						continue
					}
					row["distanceLy"] = distance
				}
			}
		}
		query := strings.ToLower(v.Str(s.settings["searchQuery"]))
		if query != "" && !strings.Contains(strings.ToLower(b.Name+" "+b.System), query) {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a["bookmarked"] != b["bookmarked"] {
			return a["bookmarked"] == true
		}
		if av, bv := v.Num(v.At(a, "valueRange", "maximum")), v.Num(v.At(b, "valueRange", "maximum")); av != bv {
			return av > bv
		}
		if v.Num(a["signals"]) != v.Num(b["signals"]) {
			return v.Num(a["signals"]) > v.Num(b["signals"])
		}
		if v.Num(a["distanceLy"]) != v.Num(b["distanceLy"]) {
			return v.Num(a["distanceLy"]) < v.Num(b["distanceLy"])
		}
		return v.Str(a["bodyKey"]) < v.Str(b["bodyKey"])
	})
	s.targetCache, s.targetRevision = out, revision
	return cloneRows(out[:min(limit, len(out))])
}

func (s *Service) systemSnapshot() M {
	sy := s.systems[s.currentSystem]
	if sy == nil {
		return nil
	}
	scanned, bioBodies, mapped, signalCount, sampled, complete := 0, 0, 0, 0, 0, 0
	for key := range sy.BodyIDs {
		b := s.bodies[key]
		if b == nil {
			continue
		}
		if len(b.Scan) > 0 {
			scanned++
		}
		if v.Num(b.Signals) > 0 || len(b.Confirmed) > 0 {
			bioBodies++
			signalCount += v.Int(b.Signals)
			if b.Mapped {
				mapped++
			}
			progress := s.bodyProgress(b)
			sampled += v.Int(progress["completeGenera"])
			if progress["allSamplesComplete"] == true {
				complete++
			}
		}
	}
	return M{"name": sy.Name, "systemAddress": sy.Address, "coordinates": v.Clone(sy.Coordinates), "bodiesTotal": sy.BodyCount, "scannedBodies": scanned, "fssComplete": sy.Complete, "biologicalBodies": bioBodies, "biologicalSignals": signalCount, "mappedBiologicalBodies": mapped, "sampledGenera": sampled, "fullySampledBodies": complete}
}

func (s *Service) targetRow(b *body) M {
	revision := s.bodyPredictionRevision(b)
	if b.TargetCache != nil && b.TargetRevision == revision {
		if b.TargetCache["filteredOut"] == true {
			return nil
		}
		return v.Copy(b.TargetCache)
	}
	row := s.bodySnapshot(b, false, false)
	filtered := v.Num(s.settings["minValue"]) > 0 || v.Str(s.settings["selectedGenus"]) != "" && s.settings["selectedGenus"] != "all" || s.settings["includeBacterium"] != true
	matching := 0
	if len(b.Scan) > 0 || len(b.Confirmed) > 0 {
		predictions := s.predict(b)
		for _, p := range predictions {
			if s.predictionVisible(p) {
				matching++
			}
		}
		row["valueRange"] = s.bodyValueRange(b, predictions)
		// Hundreds of historic bodies need a small summary, not 126 materialised
		// candidate cards each. Detailed cards remain cached only for active views.
		if b != s.selectedBody() && b != s.bodies[s.current] {
			b.PredictionCache = nil
		}
	}
	row["matchingSpeciesCount"] = matching
	if filtered && matching == 0 {
		row = M{"filteredOut": true}
		b.TargetCache, b.TargetRevision = row, revision
		return nil
	}
	b.TargetCache, b.TargetRevision = v.Copy(row), revision
	return row
}
func cloneRows(rows []M) []M {
	out := make([]M, len(rows))
	for i, row := range rows {
		out[i] = v.Copy(row)
	}
	return out
}
func coordinatesDistance(a, b any) (float64, bool) {
	aa, bb := v.Arr(a), v.Arr(b)
	if len(aa) != 3 || len(bb) != 3 {
		return 0, false
	}
	total := 0.0
	for i := 0; i < 3; i++ {
		x, xok := v.Number(aa[i])
		y, yok := v.Number(bb[i])
		if !xok || !yok {
			return 0, false
		}
		total += (x - y) * (x - y)
	}
	return math.Sqrt(total), true
}

func (s *Service) summary() M {
	credits, unknown := 0.0, 0
	for _, p := range s.pending {
		base, known := v.Number(p["baseCredits"])
		if !known {
			unknown++
			continue
		}
		if p["firstFootfall"] == true {
			base *= 5
		}
		if s.settings["pranavAntalBonus"] == true {
			base *= 1.3
		}
		credits += base
	}
	received, unknownSales := v.Num(s.totals["soldCredits"]), v.Int(s.totals["unknownSales"])
	complete := 0
	for _, p := range s.progress {
		if p["completedAt"] != nil {
			complete++
		}
	}
	return M{"pendingSamples": len(s.pending), "pendingEstimatedCredits": credits, "pendingUnknownPrices": unknown, "soldCredits": received, "salesCount": s.totals["salesCount"], "salesUnknownCredits": unknownSales, "completedSpeciesBodies": complete, "discoveryEvents": s.totals["discoveries"], "bodies": len(s.bodies), "systems": len(s.systems), "firstFootfallBodies": func() int {
		n := 0
		for _, b := range s.bodies {
			if b.OwnFirstFootfall {
				n++
			}
		}
		return n
	}(), "receivedHistoryLimited": len(s.sales) >= 500}
}
func (s *Service) statistics() M {
	out := s.summary()
	out["since"] = v.First(s.statsSince, s.firstAt)
	out["periodCredits"] = 0.0
	out["periodSales"] = 0
	out["periodCompletions"] = 0
	out["periodHistoryLimited"] = v.Int(s.totals["salesCount"]) > len(s.sales)
	since := v.Time(s.statsSince)
	for _, r := range s.sales {
		if since.IsZero() || !v.Time(r["at"]).Before(since) {
			out["periodSales"] = v.Int(out["periodSales"]) + 1
			if n, ok := v.Number(r["credits"]); ok {
				out["periodCredits"] = v.Num(out["periodCredits"]) + n
			}
		}
	}
	for _, p := range s.progress {
		at := v.Time(p["completedAt"])
		if !at.IsZero() && (since.IsZero() || !at.Before(since)) {
			out["periodCompletions"] = v.Int(out["periodCompletions"]) + 1
		}
	}
	return out
}
func (s *Service) quality() M {
	shown, unlikely, absent := 0, 0, 0
	for _, p := range s.predictions {
		for _, o := range v.Rows(p["observations"]) {
			switch o["tier"] {
			case "unlikely":
				unlikely++
			case "absent":
				absent++
			default:
				shown++
			}
		}
	}
	return M{"sealedBodies": len(s.predictions), "observations": shown + unlikely + absent, "shown": shown, "unlikely": unlikely, "absent": absent, "history": recent(s.predictions, 50), "basis": "live-pre-discovery-only", "calibratedProbability": false}
}
func recent(rows []M, limit int) []M {
	start := max(0, len(rows)-limit)
	out := make([]M, 0, len(rows)-start)
	for i := len(rows) - 1; i >= start; i-- {
		out = append(out, v.Copy(rows[i]))
	}
	return out
}
