package exobiology

import (
	"fmt"
	"math"
	"sort"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type assessment struct {
	reasons, unknown []string
	checks, passed   int
	excluded         bool
}

func (a *assessment) check(known, pass bool, reason string) {
	if !known {
		a.unknown = append(a.unknown, reason)
		return
	}
	a.checks++
	if pass {
		a.passed++
	} else {
		a.reasons = append(a.reasons, reason)
	}
}

// Prediction evaluates the published habitat constraints and observation
// profiles. Percentages are habitat similarity, never a promise of presence.
func (s *Service) predict(b *body) []M {
	revision := s.bodyPredictionRevision(b)
	if b.PredictionCache != nil && b.PredictionRevision == revision {
		return b.PredictionCache
	}
	d := Catalog()
	rows := []M{}
	sy := s.systems[b.Address]
	host := s.hostStar(b)
	atmo := atmosphereName(b.Scan)
	planet := planetName(b.Scan["PlanetClass"])
	hints := map[string]bool{}
	for _, g := range b.GenusHints {
		hints[genusID(g["genus"], g["name"])] = true
	}
	for _, sp := range d.species {
		confirmed := b.Confirmed[normal(sp.Genus)]
		isConfirmed := confirmed["speciesId"] == sp.ID
		if count, known := v.Number(b.Signals); known && count > 0 && len(b.Confirmed) >= int(count) && confirmed == nil && sp.Conditions["outside_signal_count"] != true {
			continue
		}
		if len(hints) > 0 && !hints[sp.GenusID] && sp.Conditions["outside_signal_count"] != true && !isConfirmed {
			continue
		}
		if n, ok := v.Number(b.Signals); ok && n == 0 && sp.Conditions["outside_signal_count"] != true && !isConfirmed {
			continue
		}
		a := assessment{}
		c := sp.Conditions
		g := d.genusByID[sp.GenusID]
		requirements := v.Obj(g["requirements"])
		if b.Scan["Landable"] != nil {
			a.check(true, b.Scan["Landable"] == true, "Посадка на это тело невозможна")
		}
		types := stringsOf(c["planet_types"])
		if len(types) == 0 {
			types = stringsOf(requirements["planet_types"])
		}
		if len(types) > 0 {
			a.check(planet != "", listHas(types, planet, planetName), "Тип поверхности не соответствует")
		}
		atmos := stringsOf(c["atmosphere"])
		if len(atmos) == 0 {
			atmos = stringsOf(v.First(requirements["atmosphere"], requirements["atmospheres"]))
		}
		anyThin := false
		for _, name := range atmos {
			if strings.Contains(strings.ToLower(name), "any thin") {
				anyThin = true
			}
		}
		atmosphereMatch := len(atmos) == 0 || listHas(atmos, atmo, atmosphereToken)
		active, volKnown := activeVolcanism(b.Scan)
		if anyThin {
			a.check(atmo != "", atmo != "none", "Нужна непустая тонкая атмосфера")
		} else if len(atmos) > 0 {
			allowed := atmosphereMatch
			if c["off_list_atmosphere_needs_volcanism"] == true && active {
				allowed = true
			}
			a.check(atmo != "", allowed, "Состав атмосферы не соответствует")
		}
		if c["atmospherePressureCategory"] == "thin" || anyThin {
			pressure, ok := numberDiv(b.Scan["SurfacePressure"], 101325)
			a.check(ok, pressure > 0 && pressure <= 0.1, "Давление вне диапазона тонкой атмосферы")
		}
		required := stringsOf(v.First(c["required_atmosphere_type"], requirements["required_atmosphere_type"]))
		if len(required) > 0 {
			a.check(atmo != "", listHas(required, atmo, atmosphereToken), "Нужен другой основной газ атмосферы")
		}
		if maxG, ok := v.Number(c["max_gravity"]); ok {
			g, gok := numberDiv(b.Scan["SurfaceGravity"], 9.80665)
			a.check(gok, g <= maxG, "Гравитация выше предела вида")
		}
		temp, tok := v.Number(b.Scan["SurfaceTemperature"])
		if c["temperature_K"] != nil {
			a.check(tok, insideRange(temp, c["temperature_K"]), "Температура вне диапазона вида")
		}
		linked := stringsOf(c["whenAtmosphereLinkedAtmosphereAnyOf"])
		if len(linked) == 0 {
			linked = []string{"Carbon Dioxide"}
		}
		if listHas(linked, atmo, atmosphereToken) {
			if n, ok := v.Number(c["whenAtmosphereLinkedMinTempK"]); ok {
				a.check(tok, temp >= n, "Температура ниже диапазона для этой атмосферы")
			}
			if n, ok := v.Number(c["whenAtmosphereLinkedMaxTempK"]); ok {
				a.check(tok, temp <= n, "Температура выше диапазона для этой атмосферы")
			}
		}
		pressure, pok := numberDiv(b.Scan["SurfacePressure"], 101325)
		if n, ok := v.Number(c["minPressure"]); ok {
			a.check(pok, pressure >= n, "Давление ниже предела вида")
		}
		for gas, raw := range v.Obj(c["maxPressureByAtmosphere"]) {
			if atmosphereToken(gas) == atmo {
				a.check(pok, pressure <= v.Num(raw), "Давление выше предела для этой атмосферы")
			}
		}
		for gas, band := range v.Obj(c["atmosphere_gas_share_pct"]) {
			share, known := gasShare(b.Scan, gas)
			a.check(known, insideRange(share, band), "Доля газа не соответствует: "+gas)
		}
		if c["volcanismActiveRequired"] == true {
			a.check(volKnown, active, "Нужен активный вулканизм")
		}
		if listHas(stringsOf(c["volcanic_only_atmospheres"]), atmo, atmosphereToken) {
			a.check(volKnown, active, "Для этой атмосферы нужен активный вулканизм")
		}
		if volcanoes := stringsOf(c["volcanism"]); len(volcanoes) > 0 {
			actual := normal(b.Scan["Volcanism"])
			match := false
			for _, kind := range volcanoes {
				if strings.Contains(actual, normal(kind)) {
					match = true
				}
			}
			a.check(volKnown, match, "Тип вулканизма не соответствует")
		}
		if c["soft_no_volcanism"] == true {
			a.check(volKnown, !active, "Обычно наблюдается без активного вулканизма")
		}
		if len(v.Arr(c["geologicalSignalIncludes"])) > 0 {
			n, ok := v.Number(b.GeologicalSignals)
			a.check(ok, n > 0, "Геологические сигналы не подтверждены")
		}
		if alternatives := v.Rows(c["presence_any_of"]); len(alternatives) > 0 {
			known, pass := false, false
			allKnown := true
			for _, alt := range alternatives {
				if alt["volcanismActiveRequired"] == true {
					allKnown = allKnown && volKnown
					pass = pass || (volKnown && active)
					known = known || volKnown
				}
				if alt["temperature_K"] != nil {
					allKnown = allKnown && tok
					pass = pass || (tok && insideRange(temp, alt["temperature_K"]))
					known = known || tok
				}
			}
			a.check(pass || allKnown && known, pass, "Нужен вулканизм либо достаточная температура")
		}
		if gate := gateForSpecies(sp.ID); gate != nil {
			s.checkObservedStarGate(&a, b, gate)
		} else if starReq := v.First(c["parent_star_types"], c["parent_star"]); starReq != nil {
			s.checkStar(&a, b, host, starReq)
		}
		needed := stringsOf(c["system_requirements"])
		if len(needed) == 0 {
			needed = stringsOf(requirements["system_requirements"])
		}
		if c["requires_system_bodies"] == true && len(needed) == 0 {
			needed = []string{"Earth-like world", "Gas giant with water-based life"}
		}
		if len(needed) > 0 {
			found := false
			if sy != nil {
				for key := range sy.BodyIDs {
					other := s.bodies[key]
					if other != nil && other.Key != b.Key && listHas(needed, planetName(other.Scan["PlanetClass"]), planetName) {
						found = true
					}
				}
			}
			a.check(found || sy != nil && sy.Complete, found, "Нужные сопутствующие тела системы не обнаружены")
		}
		if ids := v.Arr(c["known_systems"]); len(ids) > 0 {
			match := false
			for _, id := range ids {
				if v.Str(id) == b.Address {
					match = true
				}
			}
			a.check(true, match, "Вид подтверждён только в определённых системах")
			if !match {
				a.excluded = true
			}
		}
		if n, ok := v.Number(v.Obj(c["distance_from_star"])["min_ls"]); ok {
			distance, known := s.starDistance(b, host)
			a.check(known, distance >= n, "Орбита ближе обычного диапазона вида")
		}
		if n, ok := v.Number(v.Obj(c["distance_from_arrival"])["min_ls"]); ok {
			distance, known := v.Number(b.Scan["DistanceFromArrivalLS"])
			a.check(known, distance >= n, "Недостаточное расстояние от точки прибытия")
		}
		if n, ok := v.Number(c["soft_max_semi_major_axis_ls"]); ok {
			distance, known := numberDiv(b.Scan["SemiMajorAxis"], 299792458)
			a.check(known, distance <= n, "Большая полуось выше наблюдаемого диапазона")
		}
		s.checkSpatial(&a, sp, sy)
		similarity := habitatSimilarity(sp, b.Scan)
		tier := "likely"
		if len(a.unknown) > 0 {
			tier = "uncertain"
		}
		if len(a.reasons) > 0 {
			tier = "unlikely"
		}
		if isConfirmed {
			tier = "confirmed"
		}
		if confirmed != nil && !isConfirmed {
			tier = "unlikely"
			a.reasons = append(a.reasons, "В этом роде уже подтверждён другой вид")
		}
		if a.excluded && !isConfirmed {
			continue
		}
		row := sp.summary()
		row["tier"], row["confirmed"], row["reasons"], row["unknownConditions"] = tier, isConfirmed, a.reasons, a.unknown
		row["habitatMatchPercent"], row["probabilityPercent"], row["guaranteed"] = similarity, nil, false
		row["matchedChecks"], row["totalChecks"] = a.passed, a.checks
		row["estimatedCredits"], row["firstFootfall"] = s.estimatedValue(sp.BaseCredits, b), footfallState(b)
		row["potentialFirstFootfallCredits"] = nil
		if b.WasFootfalled == false && !b.OwnFirstFootfall {
			row["potentialFirstFootfallCredits"] = multiply(row["estimatedCredits"], 5)
		}
		if isConfirmed {
			row["progress"] = v.Copy(confirmed)
		}
		row["colourVariants"] = d.predictColours(sp, host, b.Scan)
		rows = append(rows, row)
	}
	rank := map[string]int{"confirmed": 0, "likely": 1, "uncertain": 2, "unlikely": 3}
	applyRanking(rows, b, host)
	sort.SliceStable(rows, func(i, j int) bool {
		a, c := rows[i], rows[j]
		if rank[v.Str(a["tier"])] != rank[v.Str(c["tier"])] {
			return rank[v.Str(a["tier"])] < rank[v.Str(c["tier"])]
		}
		if a["group"] == c["group"] {
			av, aok := v.Number(a["relativeSpeciesSharePercent"])
			bv, bok := v.Number(c["relativeSpeciesSharePercent"])
			if aok && bok && av != bv {
				return av > bv
			}
		} else {
			av, aok := v.Number(a["genusObservedRatePercent"])
			bv, bok := v.Number(c["genusObservedRatePercent"])
			if aok && bok && av != bv {
				return av > bv
			}
		}
		if v.Num(a["habitatMatchPercent"]) != v.Num(c["habitatMatchPercent"]) {
			return v.Num(a["habitatMatchPercent"]) > v.Num(c["habitatMatchPercent"])
		}
		return v.Str(a["name_en"]) < v.Str(c["name_en"])
	})
	b.PredictionCache, b.PredictionRevision = rows, revision
	return rows
}

func (s *Service) bodyPredictionRevision(b *body) uint64 {
	revision := b.Revision + s.settingsRevision
	if sy := s.systems[b.Address]; sy != nil {
		revision += sy.Revision
	}
	return revision
}

func genusID(values ...any) string {
	d := Catalog()
	for _, raw := range values {
		key := normal(raw)
		for alias, id := range d.genusAliases {
			if key == alias || strings.Contains(key, "ent"+alias+"genus") {
				return id
			}
		}
		for _, g := range d.genera {
			for _, name := range []any{g["id"], g["name"], g["name_en"]} {
				id := normal(name)
				if key == id || strings.Contains(key, "genus"+id) || strings.Contains(key, id+"genus") {
					return v.Str(g["id"])
				}
			}
		}
	}
	return ""
}
func numberDiv(raw any, div float64) (float64, bool) { n, ok := v.Number(raw); return n / div, ok }
func insideRange(n float64, raw any) bool {
	lo, hi := any(nil), any(nil)
	if a := v.Arr(raw); len(a) >= 2 {
		lo, hi = a[0], a[1]
	} else {
		r := v.Obj(raw)
		lo, hi = r["min"], r["max"]
	}
	if x, ok := v.Number(lo); ok && n < x {
		return false
	}
	if x, ok := v.Number(hi); ok && x != 999 && n > x {
		return false
	}
	return true
}
func listHas(values []string, actual string, normalize func(any) string) bool {
	for _, raw := range values {
		if normalize(raw) == actual {
			return true
		}
	}
	return false
}
func atmosphereToken(raw any) string {
	name := strings.ToLower(v.Str(raw))
	for _, prefix := range []string{"thin ", "thick ", "hot ", " (thin)", " atmosphere"} {
		name = strings.ReplaceAll(name, prefix, "")
	}
	name = normal(name)
	switch name {
	case "", "none", "noatmosphere":
		return "none"
	case "sulfurdioxide":
		return "sulphurdioxide"
	}
	return name
}
func atmosphereName(scan M) string {
	if raw, ok := scan["AtmosphereType"]; ok {
		return atmosphereToken(raw)
	}
	if raw, ok := scan["Atmosphere"]; ok {
		return atmosphereToken(raw)
	}
	return ""
}
func planetName(raw any) string {
	key := normal(raw)
	switch key {
	case "rocky", "rockybody":
		return "rocky"
	case "highmetalcontent", "highmetalcontentbody", "highmetalcontentworld":
		return "highmetalcontent"
	case "metalrich", "metalrichbody":
		return "metalrich"
	case "icy", "icybody":
		return "icy"
	case "rockyice", "rockyicebody", "rockyiceworld":
		return "rockyice"
	case "earthlikeworld", "earthlikeworlds":
		return "earthlike"
	case "ammoniacworld", "ammoniaworld":
		return "ammoniaworld"
	}
	return key
}
func activeVolcanism(scan M) (bool, bool) {
	raw, exists := scan["Volcanism"]
	if !exists {
		return false, false
	}
	key := normal(raw)
	return key != "" && key != "none" && key != "no" && key != "novolcanism", true
}
func gasShare(scan M, gas string) (float64, bool) {
	rows, exists := scan["AtmosphereComposition"]
	if !exists {
		return 0, false
	}
	for _, r := range v.Rows(rows) {
		if atmosphereToken(r["Name"]) == atmosphereToken(gas) {
			return v.Number(r["Percent"])
		}
	}
	return 0, true
}

func (s *Service) hostStar(b *body) *body {
	sy := s.systems[b.Address]
	if sy == nil {
		return nil
	}
	current := b
	seen := map[string]bool{}
	for depth := 0; depth < 16; depth++ {
		next := (*body)(nil)
		for _, parent := range v.Rows(current.Scan["Parents"]) {
			if id, ok := parent["Star"]; ok {
				candidate := s.bodies[bodyKey(b.Address, id)]
				if candidate != nil {
					return candidate
				}
				return nil
			}
			for kind, id := range parent {
				if kind == "Null" {
					continue
				}
				if candidate := s.bodies[bodyKey(b.Address, id)]; candidate != nil {
					next = candidate
					break
				}
			}
			if next != nil {
				break
			}
		}
		if next == nil || seen[next.Key] {
			break
		}
		seen[next.Key] = true
		current = next
	}
	stars := s.systemStars(b)
	if len(stars) == 1 {
		return stars[0]
	}
	return nil
}
func (s *Service) checkStar(a *assessment, b, host *body, raw any) {
	if host == nil {
		a.check(false, false, "Родительская звезда не определена")
		return
	}
	star := strings.ToUpper(v.Str(host.Scan["StarType"]))
	lum := strings.ToUpper(v.Str(host.Scan["Luminosity"]))
	text := strings.ToLower(v.Str(raw))
	allowed := stringsOf(raw)
	if object := v.Obj(raw); len(object) > 0 {
		allowed = stringsOf(object["required_types"])
	}
	if strings.Contains(text, "any star") {
		sy := s.systems[b.Address]
		pass := false
		if sy != nil {
			for key := range sy.BodyIDs {
				st := v.Str(s.bodies[key].Scan["StarType"])
				if st == "A" || st == "B" {
					pass = true
				}
			}
		}
		a.check(pass || sy != nil && sy.Complete, pass, "В системе нужна звезда A или B")
		return
	}
	if strings.Contains(text, "brown dwarf") {
		a.check(star != "", strings.Contains("AFGKMSLTY", star) && len(star) == 1, "Спектральный класс звезды вне диапазона")
		return
	}
	pass, needsLum := false, false
	for _, rule := range allowed {
		r := strings.ToUpper(rule)
		switch {
		case strings.Contains(r, "NEUTRON"):
			pass = pass || star == "N"
		case strings.Contains(r, "WOLF"):
			pass = pass || strings.HasPrefix(star, "W")
		case strings.Contains(r, "HERBIG"):
			pass = pass || star == "AEBE"
		default:
			base := strings.Fields(r)
			if len(base) == 0 {
				continue
			}
			if base[0] != star {
				continue
			}
			ok := true
			if strings.Contains(r, "I-III") {
				needsLum = true
				ok = lum == "I" || lum == "IA" || lum == "IB" || lum == "IAB" || lum == "II" || lum == "III"
			} else if strings.Contains(r, "IV-V") {
				needsLum = true
				ok = lum == "IV" || lum == "V"
			} else if strings.Contains(r, "VI") {
				needsLum = true
				ok = lum == "VI"
			} else if strings.Contains(r, "CLASS V") {
				needsLum = true
				ok = lum == "V" || lum == "VI" || lum == "VII"
			}
			pass = pass || ok
		}
	}
	a.check(!(needsLum && lum == ""), pass, "Тип или класс светимости звезды не соответствует")
}
func (s *Service) starDistance(b, host *body) (float64, bool) {
	if host == nil {
		return 0, false
	}
	for _, parent := range v.Rows(b.Scan["Parents"]) {
		if parent["Star"] != nil {
			return numberDiv(b.Scan["SemiMajorAxis"], 299792458)
		}
	}
	return 0, false
}

func (s *Service) checkSpatial(a *assessment, sp *Species, sy *system) {
	required := sp.GenusID == "electricae" && strings.Contains(sp.ID, "radialem") || sp.GenusID == "bark_mounds" || sp.GenusID == "brain_trees" || sp.GenusID == "sinuous_tuber" || sp.GenusID == "crystalline_shards"
	if !required {
		return
	}
	if sy == nil || len(v.Arr(sy.Coordinates)) != 3 {
		a.check(false, false, "Галактическое расположение не известно")
		return
	}
	pos := v.Arr(sy.Coordinates)
	x, y, z := v.Num(pos[0]), v.Num(pos[1]), v.Num(pos[2])
	spatial := v.Obj(Catalog().models["spatial"])
	nearest := func(raw any) float64 {
		best := math.Inf(1)
		for _, r := range v.Rows(raw) {
			d := math.Sqrt(math.Pow(x-v.Num(r["x"]), 2) + math.Pow(y-v.Num(r["y"]), 2) + math.Pow(z-v.Num(r["z"]), 2))
			best = math.Min(best, d)
		}
		return best
	}
	switch sp.GenusID {
	case "electricae":
		a.check(true, nearest(spatial["nebulae"]) <= 200 || nearest(spatial["planetary"]) <= 110 || nearest(v.Obj(spatial["records"])["radialem"]) <= 50, "Далеко от известных областей обитания возле туманностей")
	case "bark_mounds":
		a.check(true, nearest(spatial["nebulae"]) <= 300, "Далеко от известных туманностей")
	case "brain_trees":
		a.check(true, nearest(spatial["guardian"]) <= 1000, "Далеко от известных областей Стражей")
	case "sinuous_tuber", "crystalline_shards":
		core := v.Obj(spatial["core"])
		distance := math.Sqrt(math.Pow(x-v.Num(core["x"]), 2) + math.Pow(y-v.Num(core["y"]), 2) + math.Pow(z-v.Num(core["z"]), 2))
		if sp.GenusID == "sinuous_tuber" {
			a.check(true, distance <= 21000, "За пределами наблюдаемой области вокруг центра Галактики")
		} else {
			a.check(true, distance >= 19500, "Слишком близко к центру Галактики для известных скоплений")
		}
	}
}

func habitatSimilarity(sp *Species, scan M) any {
	numerics := v.Obj(sp.Habitat["numerics"])
	values := normalizedParameters(scan)
	sum, weight := 0.0, 0.0
	importance := v.Obj(sp.Habitat["parameterImportance"])
	for key, raw := range values {
		n, known := v.Number(raw)
		profile := v.Obj(numerics[key])
		if !known || len(profile) == 0 {
			continue
		}
		lo, lok := v.Number(profile["min"])
		hi, hok := v.Number(profile["max"])
		if !lok || !hok {
			continue
		}
		w := math.Max(0.1, v.Num(importance[key]))
		match := 1.0
		if n < lo {
			match = 1 / (1 + (lo-n)/math.Max(1, math.Abs(hi-lo)))
		}
		if n > hi {
			match = 1 / (1 + (n-hi)/math.Max(1, math.Abs(hi-lo)))
		}
		sum += match * w
		weight += w
	}
	if weight == 0 {
		return nil
	}
	return math.Round(sum/weight*1000) / 10
}
func normalizedParameters(scan M) M {
	out := M{}
	for target, field := range map[string]string{"body.surfaceTemperature": "SurfaceTemperature", "body.distanceToArrival": "DistanceFromArrivalLS", "body.earthMasses": "MassEM", "body.orbitalEccentricity": "Eccentricity", "body.axialTilt": "AxialTilt", "body.orbitalInclination": "OrbitalInclination"} {
		out[target] = scan[field]
	}
	for target, pair := range map[string]struct {
		field string
		div   float64
	}{"body.gravity": {"SurfaceGravity", 9.80665}, "body.surfacePressure": {"SurfacePressure", 101325}, "body.radius": {"Radius", 1000}, "body.semiMajorAxis": {"SemiMajorAxis", 149597870700}, "body.rotationalPeriod": {"RotationPeriod", 86400}, "body.orbitalPeriod": {"OrbitalPeriod", 86400}} {
		if n, ok := numberDiv(scan[pair.field], pair.div); ok {
			out[target] = n
		}
	}
	return out
}

func (d *Database) predictColours(sp *Species, host *body, scan M) []M {
	configuration := v.Obj(v.Obj(v.Obj(d.models["colourVariants"])["bySpecies"])[strings.ToLower(sp.EnglishName)])
	basis := v.Str(configuration["source"])
	mapping := v.Obj(configuration["map"])
	if len(mapping) == 0 {
		mapping = v.Obj(sp.Colours["mapping"])
	}
	if len(mapping) == 0 {
		mapping = v.Obj(v.Obj(d.genusByID[sp.GenusID]["colours"])["mapping"])
	}
	out := []M{}
	if basis == "material" {
		for _, material := range v.Rows(scan["Materials"]) {
			if v.Num(material["Percent"]) <= 0 {
				continue
			}
			if colour := mapping[strings.ToLower(v.Str(material["Name"]))]; colour != nil {
				out = append(out, M{"name": colour, "basis": "material", "material": material["Name"], "confirmed": false})
			}
		}
	} else if host != nil {
		if colour := mapping[v.Str(host.Scan["StarType"])]; colour != nil {
			out = append(out, M{"name": colour, "basis": "host-star", "confirmed": false})
		}
	}
	if len(out) == 0 {
		for _, key := range sortedKeys(mapping) {
			if mapping[key] != nil {
				out = append(out, M{"name": mapping[key], "basis": v.Text(basis, "catalogue"), "condition": key, "confirmed": false})
			}
		}
	}
	return out
}

func footfallState(b *body) string {
	if b.OwnFirstFootfall {
		return "own-first-footfall"
	}
	if b.WasFootfalled == true {
		return "walked"
	}
	if b.WasFootfalled == false {
		return "unwalked"
	}
	return "unknown"
}
func (s *Service) estimatedValue(base any, b *body) any {
	n, ok := v.Number(base)
	if !ok {
		return nil
	}
	if b.OwnFirstFootfall {
		n *= 5
	}
	if s.settings["pranavAntalBonus"] == true {
		n *= 1.3
	}
	return n
}

func (s *Service) sealPrediction(b *body, at string) {
	for _, r := range s.predictions {
		if r["bodyKey"] == b.Key {
			return
		}
	}
	scores := []M{}
	for _, p := range s.predict(b) {
		scores = append(scores, M{"speciesId": p["speciesId"], "genus": p["genus"], "tier": p["tier"], "habitatMatchPercent": p["habitatMatchPercent"]})
	}
	s.predictions = appendBounded(s.predictions, M{"bodyKey": b.Key, "at": at, "predictions": scores, "observations": []M{}, "basis": "sealed-before-live-discovery"}, 200)
}
func (s *Service) scorePrediction(b *body, speciesID, genus, identity, at string) {
	for _, sealed := range s.predictions {
		if sealed["bodyKey"] != b.Key {
			continue
		}
		observed := v.Rows(sealed["observations"])
		for _, r := range observed {
			if r["speciesId"] == speciesID {
				return
			}
		}
		tier := "absent"
		rank := 0
		for i, p := range v.Rows(sealed["predictions"]) {
			if p["speciesId"] == speciesID {
				tier = v.Str(p["tier"])
				rank = i + 1
				break
			}
		}
		sealed["observations"] = append(observed, M{"speciesId": speciesID, "genus": genus, "tier": tier, "rank": rank, "at": at})
		if tier == "absent" || tier == "unlikely" {
			if s.settings["predictionAlerts"] == true {
				s.alerts = appendBounded(s.alerts, M{"id": identity, "bodyKey": b.Key, "kind": "prediction-miss", "message": fmt.Sprintf("Обнаруженный вид не входил в основной прогноз: %s", speciesID), "at": at}, 100)
			}
		}
		return
	}
}
