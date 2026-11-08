package exobiology

import (
	"fmt"
	"math"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// The original catalogue's shared histograms, prevalence and conditional body
// priors have a common scale. Similarity against each species' own min/max does
// not, so it must not stand in for this ranking or for presence probability.
func likelihood(sp *Species, scan M, host *body) (float64, int) {
	d := Catalog()
	profile := sp.Habitat
	edges := v.Obj(v.Obj(d.models["histogramEdges"])["edges"])
	parameters := normalizedParameters(scan)
	for _, r := range v.Rows(scan["Materials"]) {
		parameters["body.materials."+v.Str(r["Name"])] = r["Percent"]
	}
	for _, r := range v.Rows(scan["AtmosphereComposition"]) {
		parameters["body.atmosphereComposition."+v.Str(r["Name"])] = r["Percent"]
	}
	for name, raw := range v.Obj(scan["SolidComposition"]) {
		parameters["body.solidComposition."+name] = raw
	}
	sum, atmoSum, terms, atmoTerms := 0.0, 0.0, 0, 0
	for path, raw := range v.Obj(profile["histograms"]) {
		if strings.HasPrefix(path, "body.materials.") {
			continue
		}
		counts := v.Arr(raw)
		binEdges := v.Arr(edges[path])
		if len(counts) != 16 || len(binEdges) != 15 {
			continue
		}
		value, known := v.Number(parameters[path])
		if !known && strings.HasPrefix(path, "body.atmosphereComposition.") {
			value, known = gasShare(scan, strings.TrimPrefix(path, "body.atmosphereComposition."))
		}
		if !known {
			continue
		}
		bin := 0
		for _, edge := range binEdges {
			if value >= v.Num(edge) {
				bin++
			}
		}
		total := 0.0
		for _, count := range counts {
			total += v.Num(count)
		}
		if total <= 0 {
			continue
		}
		term := math.Log((v.Num(counts[bin]) + 0.5) / (total + 8))
		if strings.HasPrefix(path, "body.atmosphereComposition.") {
			atmoSum += term
			atmoTerms++
		} else {
			sum += term
			terms++
		}
	}
	if atmoTerms > 0 {
		sum += 2 * atmoSum / float64(atmoTerms)
		terms += 2
	}
	categorical := map[string]any{"body.subType": scan["PlanetClass"], "body.isLandable": scan["Landable"], "body.atmosphereType": v.First(scan["AtmosphereType"], scan["Atmosphere"]), "body.volcanismType": scan["Volcanism"], "body.rotationalPeriodTidallyLocked": scan["TidalLock"], "body.terraformingState": scan["TerraformState"]}
	if host != nil {
		categorical["exo.host_star_spectral_primary"] = host.Scan["StarType"]
	}
	for path, want := range categorical {
		if want == nil {
			continue
		}
		counts := v.Obj(v.Obj(profile["categorical"])[path])
		if len(counts) == 0 {
			continue
		}
		hit, total, categories := 0.0, 0.0, 0.0
		for label, raw := range counts {
			n := v.Num(raw)
			if n <= 0 {
				continue
			}
			categories++
			total += n
			if categoryBucket(path, label) == categoryBucket(path, want) {
				hit += n
			}
		}
		if total <= 0 {
			continue
		}
		term := math.Log((hit + 0.5) / (total + 0.5*categories))
		if path == "body.volcanismType" {
			term *= 3
		}
		sum += term
		terms++
	}
	prevalence := v.Obj(d.models["prevalence"])
	prior := math.Log((v.Num(v.Obj(prevalence["species"])[sp.ID]) + 1) / (v.Num(prevalence["bodies"]) + 2))
	if conditional, ok := bodyTypePrior(sp.ID, scan); ok {
		prior = 0.48*conditional + 0.52*prior
	}
	return prior + 0.11*sum, terms
}

func categoryBucket(path string, raw any) string {
	switch path {
	case "body.subType":
		return planetName(raw)
	case "body.atmosphereType":
		return strings.TrimSuffix(atmosphereToken(raw), "rich")
	case "body.volcanismType":
		if n := normal(raw); n == "" || n == "none" || n == "no" || n == "novolcanism" {
			return "none"
		}
		return volcanismFamily(raw)
	case "exo.host_star_spectral_primary":
		key := strings.ToUpper(v.Str(raw))
		if key == "NEUTRON STAR" {
			return "N"
		}
		return strings.TrimRight(key, "0123456789.- ")
	}
	return normal(raw)
}
func volcanismFamily(raw any) string {
	if active, _ := activeVolcanism(M{"Volcanism": raw}); !active {
		return "none"
	}
	text := strings.ToLower(v.Str(raw))
	for _, family := range []string{"water", "nitrogen", "ammonia", "methane", "carbon dioxide", "silicate", "metallic", "rocky"} {
		if strings.Contains(text, family) {
			return family
		}
	}
	return "other"
}
func typePlanet(raw any) string {
	key := planetName(raw)
	switch key {
	case "highmetalcontent":
		return "hmc"
	}
	return key
}
func typeVolcanism(raw any) string {
	if active, _ := activeVolcanism(M{"Volcanism": raw}); !active {
		return "none"
	}
	text := strings.ToLower(v.Str(raw))
	text = strings.TrimPrefix(strings.TrimPrefix(text, "minor "), "major ")
	return normal(strings.TrimSuffix(text, "volcanism"))
}

func bodyTypePrior(speciesID string, scan M) (float64, bool) {
	table := v.Obj(Catalog().models["bodyTypePrior"])
	typ := typePlanet(scan["PlanetClass"])
	atmo := strings.TrimSuffix(atmosphereName(scan), "rich")
	volc := typeVolcanism(scan["Volcanism"])
	vb := "volc"
	if volc == "none" {
		vb = "none"
	}
	temp := "?"
	if value, known := v.Number(scan["SurfaceTemperature"]); known {
		index := 0
		for _, edge := range v.Arr(table["tEdges"]) {
			if value >= v.Num(edge) {
				index++
			}
		}
		temp = string(rune('a' + index))
	}
	keys := []string{typ + "|" + atmo + "|" + volc + "|" + temp, typ + "|" + atmo + "|" + volc, typ + "|" + atmo + "|" + vb, typ + "|" + atmo, atmo}
	levels := v.Arr(table["levels"])
	for i, key := range keys {
		if i >= len(levels) {
			break
		}
		cell := v.Obj(v.Obj(levels[i])[key])
		total := 0.0
		categories := 0
		for _, raw := range cell {
			if n := v.Num(raw); n > 0 {
				total += n
				categories++
			}
		}
		if total < 10 {
			continue
		}
		return math.Log((v.Num(cell[speciesID]) + 0.5) / (total + 0.5*float64(max(1, categories)))), true
	}
	return 0, false
}

func genusPrior(scan M, signals any, host *body) M {
	count, known := v.Number(signals)
	if !known || count < 1 {
		return nil
	}
	n := fmt.Sprint(min(5, int(count)))
	typ := typePlanet(scan["PlanetClass"])
	if typ == "rockyice" {
		typ = "rocky-ice"
	}
	if typ == "metalrich" {
		typ = "metal-rich"
	}
	atmo := atmosphereName(scan)
	atmo = mapToken(atmo, map[string]string{"carbondioxide": "carbon dioxide", "sulphurdioxide": "sulphur dioxide", "argonrich": "argon-rich", "neonrich": "neon-rich", "waterrich": "water-rich", "oxygenrich": "oxygen-rich"})
	temp := "?"
	if t, ok := v.Number(scan["SurfaceTemperature"]); ok {
		temp = fmt.Sprint(int(math.Floor(t / 20)))
	}
	star := "?"
	if host != nil {
		star = strings.ToUpper(v.Str(host.Scan["StarType"]))
	}
	volc := volcanismFamily(scan["Volcanism"])
	keys := []string{}
	pressure, pok := numberDiv(scan["SurfacePressure"], 101325)
	gravity, gok := numberDiv(scan["SurfaceGravity"], 9.80665)
	if pok && gok && temp != "?" {
		pb, gb := band(pressure, []float64{0.002, 0.005, 0.01, 0.03, 0.1}), band(gravity, []float64{0.05, 0.1, 0.15, 0.25, 0.4})
		keys = append(keys, strings.Join([]string{"P0", typ, n, atmo, temp, pb, gb, star, volc}, "|"), strings.Join([]string{"P1", typ, n, atmo, temp, pb, gb}, "|"))
	}
	keys = append(keys, strings.Join([]string{"L0", typ, n, atmo, temp, star, volc}, "|"), strings.Join([]string{"L1", typ, n, atmo, temp, star}, "|"), strings.Join([]string{"L2", typ, n, atmo, temp}, "|"), strings.Join([]string{"L3", typ, n, atmo}, "|"), strings.Join([]string{"L4", typ, n}, "|"), "L5|"+n)
	table := v.Obj(Catalog().models["genusPrior"])
	for _, key := range keys {
		cell := v.Obj(v.Obj(table["cells"])[key])
		if v.Num(cell["n"]) >= v.Num(table["min"]) {
			return M{"cell": key, "observations": cell["n"], "rates": cell["p"]}
		}
	}
	return nil
}
func mapToken(key string, m map[string]string) string {
	if value := m[key]; value != "" {
		return value
	}
	return key
}
func band(n float64, edges []float64) string {
	index := 0
	for _, edge := range edges {
		if n >= edge {
			index++
		}
	}
	return fmt.Sprint(index)
}

func applyRanking(rows []M, b *body, host *body) {
	totals := map[string]float64{}
	maxima := map[string]float64{}
	scores := map[string]float64{}
	prior := genusPrior(b.Scan, b.Signals, host)
	for _, row := range rows {
		sp := Catalog().byID[v.Str(row["speciesId"])]
		if sp == nil {
			continue
		}
		score, terms := likelihood(sp, b.Scan, host)
		if terms == 0 {
			continue
		}
		id := sp.GenusID
		scores[sp.ID] = score
		if max, ok := maxima[id]; !ok || score > max {
			maxima[id] = score
		}
		row["rankingEvidenceTerms"] = terms
	}
	for _, row := range rows {
		if score, ok := scores[v.Str(row["speciesId"])]; ok {
			totals[v.Str(row["group"])] += math.Exp(score - maxima[v.Str(row["group"])])
		}
	}
	hinted := map[string]bool{}
	for _, hint := range b.GenusHints {
		hinted[genusID(hint["genus"], hint["name"])] = true
	}
	for _, row := range rows {
		row["relativeSpeciesSharePercent"], row["genusObservedRatePercent"] = nil, nil
		sp := Catalog().byID[v.Str(row["speciesId"])]
		if sp == nil {
			continue
		}
		if score, ok := scores[sp.ID]; ok && totals[sp.GenusID] > 0 {
			row["relativeSpeciesSharePercent"] = 100 * math.Exp(score-maxima[sp.GenusID]) / totals[sp.GenusID]
		}
		if hinted[sp.GenusID] {
			row["genusObservedRatePercent"] = 100
			row["genusRateBasis"] = "dss-confirmed"
		} else if prior != nil {
			key := normal(sp.Genus)
			if len(key) > 6 {
				key = key[:6]
			}
			if rate, ok := v.Number(v.Obj(prior["rates"])[key]); ok {
				row["genusObservedRatePercent"] = rate * 100
				row["genusRateObservations"] = prior["observations"]
				row["genusRateBasis"] = "observed-similar-bodies"
			}
		}
		row["rankingBasis"] = "damped-histograms-and-prevalence"
	}
}
