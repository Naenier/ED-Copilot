package exobiology

import (
	"errors"
	"math"
	"sort"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const maxRadarTracks = 12
const maxRadarTrail = 192

var radarScales = [...]float64{100, 250, 500, 1000, 2500, 5000, 10000, 25000}

type radarFix struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	At        string  `json:"at"`
	Break     bool    `json:"break,omitempty"`
}

type radarTrack struct {
	Owner  string     `json:"owner"`
	Body   string     `json:"bodyKey"`
	Points []radarFix `json:"points"`
}

type radarPreference struct {
	Target string  `json:"target,omitempty"`
	Scale  float64 `json:"scaleM,omitempty"`
}

func (s *Service) restoreRadar(saved M) {
	s.radarPrefs = map[string]radarPreference{}
	for owner, item := range v.Obj(saved["radarPreferences"]) {
		if len(s.radarPrefs) >= 32 || owner == "" || len(owner) > 256 {
			continue
		}
		pref := v.Obj(item)
		target := v.Str(pref["target"])
		if len(target) > 256 {
			target = ""
		}
		scale := v.Num(pref["scaleM"])
		if !validRadarScale(scale) {
			scale = 0
		}
		s.radarPrefs[owner] = radarPreference{Target: target, Scale: scale}
	}
	for _, item := range v.Rows(saved["surfaceTracks"]) {
		owner, body := v.Str(item["owner"]), v.Str(item["bodyKey"])
		if owner == "" || body == "" || len(owner) > 256 || len(body) > 200 || len(s.radarTracks) >= maxRadarTracks {
			continue
		}
		row := radarTrack{Owner: owner, Body: body}
		points := v.Rows(item["points"])
		for _, point := range points[max(0, len(points)-maxRadarTrail):] {
			lat, lon, ok := radarCoords(point)
			at := v.Str(point["at"])
			if ok && !v.Time(at).IsZero() {
				row.Points = append(row.Points, radarFix{Latitude: lat, Longitude: lon, At: at, Break: v.Bool(point["break"])})
			}
		}
		if len(row.Points) > 0 {
			s.radarTracks = append(s.radarTracks, row)
		}
	}
}

func validRadarScale(scale float64) bool {
	if scale == 0 {
		return true
	}
	for _, limit := range radarScales {
		if limit == scale {
			return true
		}
	}
	return false
}

func radarCoords(m M) (float64, float64, bool) {
	lat, lok := v.Number(m["latitude"])
	lon, ook := v.Number(m["longitude"])
	return lat, lon, lok && ook && math.Abs(lat) <= 90 && math.Abs(lon) <= 180
}

func (s *Service) trackIndex(owner, body string) int {
	for i, row := range s.radarTracks {
		if row.Owner == owner && row.Body == body {
			return i
		}
	}
	return -1
}

func (s *Service) recordRadarFix(fix, commander M) {
	owner := s.commander
	body := v.Str(fix["bodyKey"])
	vehicle := v.Str(commander["vehicle"])
	if owner == "" || body == "" || v.Bool(commander["docked"]) || vehicle != "" && vehicle != "ship" && vehicle != "srv" && vehicle != "foot" {
		return
	}
	lat, lon, ok := radarCoords(fix)
	at := v.Time(fix["at"])
	if !ok || at.IsZero() {
		return
	}
	point := radarFix{Latitude: lat, Longitude: lon, At: v.Str(fix["at"])}
	i := s.trackIndex(owner, body)
	track := radarTrack{Owner: owner, Body: body}
	if i >= 0 {
		track = s.radarTracks[i]
		last := track.Points[len(track.Points)-1]
		elapsed := at.Sub(v.Time(last.At))
		move := GreatCircleDistance(last.Latitude, last.Longitude, lat, lon, v.Num(fix["planetRadiusM"]))
		if elapsed <= 0 || move < 6 {
			return
		}
		point.Break = elapsed > 3*time.Minute || move > 5000
		copy(s.radarTracks[i:], s.radarTracks[i+1:])
		s.radarTracks = s.radarTracks[:len(s.radarTracks)-1]
	}
	track.Points = append(track.Points, point)
	if len(track.Points) > maxRadarTrail {
		track.Points = track.Points[len(track.Points)-maxRadarTrail:]
	}
	s.radarTracks = append(s.radarTracks, track)
	if len(s.radarTracks) > maxRadarTracks {
		s.radarTracks = s.radarTracks[len(s.radarTracks)-maxRadarTracks:]
	}
}

func (s *Service) SnapshotSurfaceRadar() M {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.surfaceRadar(100, maxRadarTrail)
}

func (s *Service) updateRadar(input M) error {
	if s.commander == "" {
		return errors.New("Сначала подключите журнал командира.")
	}
	if body := v.Str(input["bodyKey"]); body != "" && body != s.current {
		return errors.New("Радар уже показывает другое тело.")
	}
	pref := s.radarPrefs[s.commander]
	switch input["action"] {
	case "radar-scale":
		scale, ok := v.Number(input["scaleM"])
		if !ok || !validRadarScale(scale) {
			return errors.New("Выберите масштаб радара из списка.")
		}
		pref.Scale = scale
	case "radar-target":
		id := v.Str(input["id"])
		if id != "" {
			found := false
			for _, mark := range s.radarMarks() {
				if mark["id"] == id {
					found = true
					break
				}
			}
			if !found {
				return errors.New("Точка находится за пределами текущего тела.")
			}
		}
		pref.Target = id
	case "radar-clear-track":
		if i := s.trackIndex(s.commander, s.current); i >= 0 {
			s.radarTracks = append(s.radarTracks[:i], s.radarTracks[i+1:]...)
		}
		return nil
	}
	if _, exists := s.radarPrefs[s.commander]; !exists && len(s.radarPrefs) >= 32 {
		return errors.New("Достигнут предел профилей радара.")
	}
	s.radarPrefs[s.commander] = pref
	return nil
}

func (s *Service) radarMarks() []M {
	var out []M
	for _, mark := range s.marks {
		if mark["bodyKey"] != s.current || mark["kind"] == "ship" {
			continue
		}
		if owner := v.Str(mark["owner"]); owner != "" && owner != s.commander {
			continue
		}
		out = append(out, mark)
	}
	if s.radarShip != nil && s.radarShip["bodyKey"] == s.current && s.radarShip["owner"] == s.commander {
		out = append(out, s.radarShip)
	}
	return out
}

func radarProjection(lat, lon, latitude, longitude, radius, heading float64) M {
	d := GreatCircleDistance(lat, lon, latitude, longitude, radius)
	course := bearing(lat, lon, latitude, longitude)
	relative := (course - heading) * math.Pi / 180
	return M{"x": math.Round(d * math.Sin(relative)), "y": math.Round(-d * math.Cos(relative)), "distanceM": math.Round(d), "bearing": math.Mod(math.Round(course), 360)}
}

func (s *Service) surfaceRadar(pointLimit, trailLimit int) M {
	b := s.bodies[s.current]
	if b == nil || s.liveFix == nil || s.liveFix["bodyKey"] != b.Key {
		return M{"available": false}
	}
	lat, lon, ok := radarCoords(s.liveFix)
	radius := v.Num(s.liveFix["planetRadiusM"])
	if !ok || radius <= 0 {
		return M{"available": false}
	}
	heading, hasHeading := v.Number(s.liveFix["heading"])
	pref := s.radarPrefs[s.commander]
	marks := s.radarMarks()
	points := make([]M, 0, len(marks))
	nearest := math.Inf(1)
	var target M
	for _, mark := range marks {
		mlat, mlon, ok := radarCoords(mark)
		if !ok {
			continue
		}
		point := radarProjection(lat, lon, mlat, mlon, radius, heading)
		point["id"], point["kind"], point["label"] = mark["id"], mark["kind"], mark["label"]
		point["locationBasis"] = mark["locationBasis"]
		switch mark["locationBasis"] {
		case "ahead", "crossing", "scanned-from":
			point["uncertaintyM"] = v.First(mark["uncertaintyM"], scanReachM)
		}
		if mark["kind"] == "sample" && s.tracker["bodyKey"] == b.Key && s.tracker["phase"] == "tracking" && mark["runKey"] == s.tracker["runKey"] {
			point["exclusionM"] = s.tracker["minSampleDistanceM"]
		}
		point["target"] = pref.Target != "" && mark["id"] == pref.Target
		if point["target"] == true {
			target = point
		}
		if mark["kind"] != "ship" {
			nearest = math.Min(nearest, math.Max(v.Num(point["distanceM"]), v.Num(point["exclusionM"])))
		}
		points = append(points, point)
	}
	sort.SliceStable(points, func(i, j int) bool {
		a, b := points[i], points[j]
		if a["target"] != b["target"] {
			return a["target"] == true
		}
		if (a["kind"] == "ship") != (b["kind"] == "ship") {
			return a["kind"] == "ship"
		}
		if v.Num(a["distanceM"]) != v.Num(b["distanceM"]) {
			return v.Num(a["distanceM"]) < v.Num(b["distanceM"])
		}
		return v.Str(a["id"]) < v.Str(b["id"])
	})
	scale := pref.Scale
	if scale == 0 {
		if target != nil {
			nearest = v.Num(target["distanceM"])
		}
		scale = 500
		if !math.IsInf(nearest, 1) {
			for _, candidate := range radarScales {
				scale = candidate
				if candidate >= nearest*1.25 {
					break
				}
			}
		}
	}
	trail := []M{}
	if i := s.trackIndex(s.commander, b.Key); i >= 0 {
		entries := s.radarTracks[i].Points
		for _, fix := range entries[max(0, len(entries)-trailLimit):] {
			p := radarProjection(lat, lon, fix.Latitude, fix.Longitude, radius, heading)
			p["break"] = fix.Break
			trail = append(trail, p)
		}
	}
	return M{"available": true, "body": b.Name, "bodyKey": b.Key, "stale": !s.hasLiveFix(b.Key), "heading": s.liveFix["heading"],
		"headingUp": hasHeading, "scaleM": scale, "selectedScaleM": pref.Scale, "target": target, "latitude": lat, "longitude": lon,
		"points": points[:min(pointLimit, len(points))], "trail": trail, "at": s.liveFix["at"]}
}
