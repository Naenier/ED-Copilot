package exobiology

import (
	"math"
	"sort"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const scanReachM = 1500

type localPoint struct{ e, n float64 }

func localOffset(origin, p M, radius float64) localPoint {
	delta := v.Num(p["longitude"]) - v.Num(origin["longitude"])
	if delta > 180 {
		delta -= 360
	}
	if delta < -180 {
		delta += 360
	}
	return localPoint{delta * math.Pi / 180 * radius * math.Cos(v.Num(origin["latitude"])*math.Pi/180), (v.Num(p["latitude"]) - v.Num(origin["latitude"])) * math.Pi / 180 * radius}
}
func fromOffset(origin M, p localPoint, radius float64) M {
	lat := v.Num(origin["latitude"]) + p.n/radius*180/math.Pi
	lon := v.Num(origin["longitude"])
	cos := math.Cos(v.Num(origin["latitude"]) * math.Pi / 180)
	if cos > 1e-9 {
		lon += p.e / (radius * cos) * 180 / math.Pi
	}
	lon = math.Mod(lon+540, 360) - 180
	return M{"latitude": lat, "longitude": lon}
}

// LocatePlant estimates a plant from independent composition-scanner rays.
// Parallel, backward, overly distant or otherwise ambiguous rays retain the
// scanner origin and explicitly report that weaker basis.
func LocatePlant(scans []M, radius float64) M {
	if len(scans) == 0 {
		return nil
	}
	last := scans[len(scans)-1]
	fallback := M{"latitude": last["latitude"], "longitude": last["longitude"], "locationBasis": "scanned-from", "heading": last["heading"]}
	if radius <= 0 {
		return fallback
	}
	origin := scans[0]
	hits := []localPoint{}
	for i, a := range scans {
		ha, aok := v.Number(a["heading"])
		if !aok {
			continue
		}
		ap := localOffset(origin, a, radius)
		ax, ay := math.Sin(ha*math.Pi/180), math.Cos(ha*math.Pi/180)
		for _, b := range scans[i+1:] {
			hb, bok := v.Number(b["heading"])
			if !bok {
				continue
			}
			angle := math.Abs(math.Mod(ha-hb+540, 360) - 180)
			if angle < 15 || angle > 165 {
				continue
			}
			bp := localOffset(origin, b, radius)
			bx, by := math.Sin(hb*math.Pi/180), math.Cos(hb*math.Pi/180)
			det := ay*bx - ax*by
			if math.Abs(det) < 1e-9 {
				continue
			}
			dx, dy := bp.e-ap.e, bp.n-ap.n
			ta := (dy*bx - dx*by) / det
			tb := (ax*dy - ay*dx) / det
			if ta <= 0 || tb <= 0 || ta > scanReachM || tb > scanReachM {
				continue
			}
			hits = append(hits, localPoint{ap.e + ta*ax, ap.n + ta*ay})
		}
	}
	if len(hits) > 0 {
		es, ns := []float64{}, []float64{}
		for _, p := range hits {
			es = append(es, p.e)
			ns = append(ns, p.n)
		}
		sort.Float64s(es)
		sort.Float64s(ns)
		median := localPoint{es[len(es)/2], ns[len(ns)/2]}
		sum, count := localPoint{}, 0.0
		for _, p := range hits {
			if math.Hypot(p.e-median.e, p.n-median.n) <= 150 {
				sum.e += p.e
				sum.n += p.n
				count++
			}
		}
		point := fromOffset(origin, localPoint{sum.e / count, sum.n / count}, radius)
		point["locationBasis"] = "crossing"
		return point
	}
	for i := len(scans) - 1; i >= 0; i-- {
		scan := scans[i]
		heading, hok := v.Number(scan["heading"])
		height, aok := v.Number(scan["altitudeAboveGroundM"])
		pitch, pok := v.Number(scan["pitch"])
		if !hok || !aok || !pok || height <= 0 || pitch > -3 || pitch <= -90 {
			continue
		}
		distance := height / math.Tan(-pitch*math.Pi/180)
		if distance > scanReachM {
			continue
		}
		p := localOffset(origin, scan, radius)
		p.e += distance * math.Sin(heading*math.Pi/180)
		p.n += distance * math.Cos(heading*math.Pi/180)
		point := fromOffset(origin, p, radius)
		point["locationBasis"] = "ahead"
		return point
	}
	return fallback
}

func (s *Service) addCodexMark(mark M) {
	// Joining nearby same-species scans is deliberately time bounded. Unrelated
	// plants cannot be averaged into one dot just because their species matches.
	radius := v.Num(mark["planetRadiusM"])
	scan := v.Copy(mark)
	scans := []M{scan}
	match := -1
	for i := len(s.marks) - 1; i >= 0; i-- {
		old := s.marks[i]
		if old["kind"] != "codex" || old["bodyKey"] != mark["bodyKey"] || old["speciesId"] != mark["speciesId"] || old["owner"] != mark["owner"] {
			continue
		}
		if old["speciesId"] == nil && old["label"] != mark["label"] {
			continue
		}
		age := v.Time(mark["at"]).Sub(v.Time(old["at"])).Seconds()
		if age < 0 || age > 180 {
			continue
		}
		previous := v.Rows(old["scans"])
		if len(previous) == 0 {
			previous = []M{old}
		}
		located := LocatePlant(append(previous, scan), radius)
		if located["locationBasis"] == "crossing" || GreatCircleDistance(v.Num(old["latitude"]), v.Num(old["longitude"]), v.Num(mark["latitude"]), v.Num(mark["longitude"]), radius) <= 10 {
			scans = append(previous, scan)
			match = i
			break
		}
	}
	if len(scans) > 12 {
		scans = scans[len(scans)-12:]
	}
	located := LocatePlant(scans, radius)
	mark["scans"] = scans
	mark["latitude"], mark["longitude"], mark["locationBasis"] = located["latitude"], located["longitude"], located["locationBasis"]
	if match >= 0 {
		s.marks[match] = mark
	} else {
		s.addMark(mark)
	}
}
