package exobiology

import (
	"math"
	"sort"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// GreatCircleDistance returns the shortest surface distance, including across
// the longitude seam. Radii are metres, journal coordinates are degrees.
func GreatCircleDistance(lat1, lon1, lat2, lon2, radius float64) float64 {
	r := math.Pi / 180
	a := math.Pow(math.Sin((lat2-lat1)*r/2), 2) + math.Cos(lat1*r)*math.Cos(lat2*r)*math.Pow(math.Sin((lon2-lon1)*r/2), 2)
	return 2 * radius * math.Asin(math.Sqrt(math.Max(0, math.Min(1, a))))
}

func bearing(lat1, lon1, lat2, lon2 float64) float64 {
	r := math.Pi / 180
	y := math.Sin((lon2-lon1)*r) * math.Cos(lat2*r)
	x := math.Cos(lat1*r)*math.Sin(lat2*r) - math.Sin(lat1*r)*math.Cos(lat2*r)*math.Cos((lon2-lon1)*r)
	return math.Mod(math.Atan2(y, x)/r+360, 360)
}

func fixFrom(raw M, b *body, at string) M {
	lat, lok := v.Number(v.First(raw["Latitude"], raw["latitude"]))
	lon, ook := v.Number(v.First(raw["Longitude"], raw["longitude"]))
	if !lok || !ook || math.Abs(lat) > 90 || math.Abs(lon) > 180 {
		return nil
	}
	radius, rok := v.Number(v.First(raw["PlanetRadius"], raw["planetRadiusM"], b.Scan["Radius"]))
	if !rok || radius <= 0 {
		return nil
	}
	out := M{"bodyKey": b.Key, "latitude": lat, "longitude": lon, "planetRadiusM": radius, "at": at, "heading": nil, "temperatureK": nil, "gravityG": nil, "elevationM": nil}
	if n, ok := v.Number(raw["Heading"]); ok {
		out["heading"] = math.Mod(n+360, 360)
	}
	if n, ok := v.Number(raw["Temperature"]); ok && n >= 0 {
		out["temperatureK"] = n
	}
	if g, ok := v.Number(raw["Gravity"]); ok && g > 0 {
		out["gravityG"] = g
		if ref, ok := v.Number(b.Scan["SurfaceGravity"]); ok && ref > 0 {
			out["elevationM"] = radius * (math.Sqrt(ref/9.80665/g) - 1)
		}
	}
	if n, ok := v.Number(raw["Altitude"]); ok {
		if flags := uint64(v.Num(raw["Flags"])); flags&(1<<29) != 0 {
			out["elevationM"] = n
		} else {
			out["altitudeAboveGroundM"] = n
		}
	}
	return out
}

func (s *Service) observeStatus(raw, cmd M, fromStatusFile bool) {
	name := v.Text(raw["BodyName"], cmd["body"])
	if name == "" {
		s.liveFix = nil
		return
	}
	b := s.bodies[s.current]
	if b == nil || normal(b.Name) != normal(name) {
		b = nil
		for _, candidate := range s.bodies {
			if normal(candidate.Name) == normal(name) && candidate.Address == v.Str(cmd["systemAddress"]) {
				b = candidate
				break
			}
		}
	}
	if b == nil {
		if normal(v.Str(cmd["body"])) == normal(name) {
			b = s.ensureBody(cmd["systemAddress"], numericBodyID(v.First(cmd["bodyId"], cmd["bodyID"])), name, v.Str(cmd["system"]), v.Str(raw["timestamp"]))
		}
		if b == nil {
			s.liveFix = nil
			return
		}
	}
	if flags, known := v.Number(raw["Flags"]); fromStatusFile && known && uint64(flags)&(1<<21) == 0 {
		s.liveFix = nil
		return
	}
	fix := fixFrom(raw, b, v.Str(raw["timestamp"]))
	if fix == nil {
		s.liveFix = nil
		return
	}
	s.current = b.Key
	s.liveFix = fix
	s.fixSeen = time.Now()
	at := v.Time(fix["at"])
	if fromStatusFile && (at.IsZero() || s.fixSeen.Sub(at) > 15*time.Second || at.Sub(s.fixSeen) > 10*time.Second) {
		s.fixSeen = time.Time{}
	}
	if !s.fixSeen.IsZero() {
		s.recordRadarFix(fix, cmd)
	}
	if s.tracker != nil && s.tracker["phase"] == "tracking" {
		tr := s.trackerSnapshot()
		if tr["nearestSampleMeetsMin"] == true && s.tracker["distanceReadyNotified"] != true {
			s.tracker["distanceReadyNotified"] = true
			s.alerts = appendBounded(s.alerts, M{"id": v.ID(), "kind": "sample-distance-ready", "bodyKey": b.Key, "speciesId": s.tracker["speciesId"], "message": "Генетическое разнообразие: можно искать следующий образец.", "at": raw["timestamp"]}, 100)
		}
	}
}

func (s *Service) updateTracker(event, context M, b *body, p M, identity, at string, entry *Species) {
	kind := strings.ToLower(v.Str(event["ScanType"]))
	if event["WasLogged"] == true && kind != "analyse" {
		s.tracker = nil
		return
	}
	minDistance := any(nil)
	if entry != nil {
		minDistance = entry.MinSampleDistanceM
	}
	t := M{"key": p["key"], "bodyKey": b.Key, "speciesId": p["speciesId"], "name": p["name"], "name_ru": p["name_ru"], "name_en": p["name_en"], "genus": p["genus"], "phase": "tracking", "sampleCount": p["sampleCount"], "requiredSamples": 3, "minSampleDistanceM": minDistance, "startedAt": p["startedAt"], "updatedAt": at}
	t["runKey"] = p["runKey"]
	if kind == "analyse" {
		t["phase"] = "complete"
	}
	// Journal ScanOrganic does not contain coordinates. A live Status fix may
	// place a new scan; replay can only recover an earlier saved mark.
	slot := max(0, min(2, v.Int(p["sampleCount"])-1))
	if !v.Bool(context["replay"]) && s.settings["trackSamples"] == true {
		fix := fixFrom(event, b, at)
		if fix == nil && s.hasLiveFix(b.Key) && fixIsFresh(s.liveFix, at) {
			fix = s.liveFix
		}
		if fix != nil {
			exists := false
			for _, m := range s.marks {
				if m["kind"] == "sample" && m["runKey"] == p["runKey"] && v.Int(m["slot"]) == slot {
					exists = true
					break
				}
			}
			if !exists {
				mark := v.Copy(fix)
				mark["id"], mark["kind"], mark["runKey"], mark["speciesId"], mark["slot"], mark["label"], mark["at"], mark["owner"] = identity, "sample", p["runKey"], p["speciesId"], slot, p["name"], at, s.commander
				s.addMark(mark)
			}
		}
	}
	s.tracker = t
}

func fixIsFresh(fix M, at string) bool {
	a, b := v.Time(fix["at"]), v.Time(at)
	if a.IsZero() || b.IsZero() {
		return false
	}
	return math.Abs(a.Sub(b).Seconds()) <= 15
}

func (s *Service) trackerSnapshot() M {
	if s.tracker == nil {
		return nil
	}
	out := v.Copy(s.tracker)
	if b := s.bodies[v.Str(out["bodyKey"])]; b != nil {
		out["bodyName"], out["system"] = b.Name, b.System
	}
	anchors := []M{}
	for _, m := range s.marks {
		if m["kind"] == "sample" && m["runKey"] == s.tracker["runKey"] && (m["owner"] == nil || m["owner"] == "" || m["owner"] == s.commander) {
			copy := v.Copy(m)
			delete(copy, "owner")
			anchors = append(anchors, copy)
		}
	}
	out["anchors"] = anchors
	out["recoveredSamples"] = max(0, v.Int(out["sampleCount"])-len(anchors))
	out["positionKnown"] = s.hasLiveFix(v.Str(out["bodyKey"]))
	out["nearestSampleDistanceM"], out["remainingDistanceM"], out["nearestSampleMeetsMin"] = nil, nil, nil
	if out["positionKnown"] == true && len(anchors) > 0 {
		nearest := math.Inf(1)
		for _, m := range anchors {
			d := GreatCircleDistance(v.Num(s.liveFix["latitude"]), v.Num(s.liveFix["longitude"]), v.Num(m["latitude"]), v.Num(m["longitude"]), v.Num(s.liveFix["planetRadiusM"]))
			nearest = math.Min(nearest, d)
		}
		out["nearestSampleDistanceM"] = nearest
		if required, ok := v.Number(out["minSampleDistanceM"]); ok {
			out["remainingDistanceM"] = math.Max(0, required-nearest)
			out["nearestSampleMeetsMin"] = nearest >= required
		}
	}
	return out
}

func (s *Service) marksSnapshot(b *body) []M {
	return s.marksSnapshotLimit(b, 200)
}
func (s *Service) marksSnapshotLimit(b *body, limit int) []M {
	out := []M{}
	if b == nil {
		return out
	}
	matching := []M{}
	for _, raw := range s.marks {
		if raw["bodyKey"] != b.Key {
			continue
		}
		if owner := v.Str(raw["owner"]); owner != "" && owner != s.commander {
			continue
		}
		matching = append(matching, raw)
	}
	positionKnown := s.hasLiveFix(b.Key)
	distance := func(m M) float64 {
		return GreatCircleDistance(v.Num(s.liveFix["latitude"]), v.Num(s.liveFix["longitude"]), v.Num(m["latitude"]), v.Num(m["longitude"]), v.Num(s.liveFix["planetRadiusM"]))
	}
	sort.SliceStable(matching, func(i, j int) bool {
		if positionKnown {
			return distance(matching[i]) < distance(matching[j])
		}
		return v.Str(matching[i]["at"]) > v.Str(matching[j]["at"])
	})
	for _, raw := range matching[:min(limit, len(matching))] {
		m := v.Copy(raw)
		delete(m, "owner")
		delete(m, "scans")
		m["distanceM"], m["bearing"] = nil, nil
		m["bodyName"], m["system"], m["scannedFrom"] = b.Name, b.System, m["locationBasis"] == "scanned-from"
		m["confidence"] = "recorded-position"
		if m["locationBasis"] == "crossing" {
			m["confidence"] = "triangulated-estimate"
		}
		if m["locationBasis"] == "ahead" {
			m["confidence"] = "projected-estimate"
		}
		if m["scannedFrom"] == true {
			m["confidence"] = "scanner-origin"
		}
		m["ageSeconds"] = nil
		if at := v.Time(m["at"]); !at.IsZero() {
			m["ageSeconds"] = math.Max(0, time.Since(at).Seconds())
		}
		if positionKnown {
			m["distanceM"] = distance(m)
			m["bearing"] = bearing(v.Num(s.liveFix["latitude"]), v.Num(s.liveFix["longitude"]), v.Num(m["latitude"]), v.Num(m["longitude"]))
		}
		out = append(out, m)
	}
	return out
}

func (s *Service) hasLiveFix(key string) bool {
	return s.liveFix != nil && s.liveFix["bodyKey"] == key && !s.fixSeen.IsZero() && time.Since(s.fixSeen) <= 15*time.Second
}
