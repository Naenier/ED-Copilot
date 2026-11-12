package exobiology

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const SchemaVersion = 1
const maxSavedMarks = 4096

type body struct {
	Key, Address, Name, System, UpdatedAt string
	ID                                    any
	Scan                                  M
	Signals, GeologicalSignals            any
	GenusHints                            []M
	Confirmed                             map[string]M
	Mapped, FSSResolved                   bool
	WasFootfalled                         any
	FootfallAt                            string
	OwnFirstFootfall                      bool
	PredictionCache                       []M
	PredictionRevision                    uint64
	Revision                              uint64
	TargetCache                           M
	TargetRevision                        uint64
}
type system struct {
	Name, Address string
	Coordinates   any
	Complete      bool
	BodyCount     any
	BodyIDs       map[string]bool
	Revision      uint64
}

type Service struct {
	mu                                                                 sync.RWMutex
	settings                                                           M
	selected, current, commander                                       string
	currentSystem                                                      string
	bodies                                                             map[string]*body
	systems                                                            map[string]*system
	progress                                                           map[string]M
	pending, sales, discoveries, marks, bookmarks, predictions, alerts []M
	tracker, liveFix                                                   M
	seen                                                               map[string]bool
	settingsRevision                                                   uint64
	predictionRevision                                                 uint64
	totals                                                             M
	firstAt, statsSince                                                string
	lastFixDistance                                                    float64
	fixSeen                                                            time.Time
	targetCache                                                        []M
	targetRevision                                                     uint64
	radarTracks                                                        []radarTrack
	radarPrefs                                                         map[string]radarPreference
	radarShip                                                          M
}

func DefaultSettings() M {
	return M{"includeBacterium": true, "showUnlikely": false, "minValue": 0, "minSignals": 0, "maxArrivalLs": 100000, "radiusLy": 200, "onlyUnwalked": false, "pranavAntalBonus": false, "trackSamples": true, "selectedGenus": "", "searchQuery": "", "predictionAlerts": true}
}

func New(saved M) *Service {
	s := &Service{settings: DefaultSettings(), selected: v.Str(saved["selectedBodyKey"]), statsSince: v.Str(saved["statsSince"])}
	s.restoreRadar(saved)
	_ = s.configure(v.Obj(saved["settings"]))
	for _, row := range v.Rows(saved["surfaceMarks"]) {
		if mark := cleanMark(row); mark != nil {
			s.marks = append(s.marks, mark)
			if len(s.marks) >= maxSavedMarks {
				break
			}
		}
	}
	for _, row := range v.Rows(saved["bookmarks"]) {
		if v.Str(row["bodyKey"]) != "" {
			s.bookmarks = append(s.bookmarks, v.Copy(row))
			if len(s.bookmarks) >= 200 {
				break
			}
		}
	}
	s.reset()
	return s
}

// Reset rebuilds journal-derived observations while preserving positions that
// could only be captured live, explicit settings and user bookmarks.
func (s *Service) Reset() { s.mu.Lock(); defer s.mu.Unlock(); s.reset() }
func (s *Service) reset() {
	s.bodies = map[string]*body{}
	s.systems = map[string]*system{}
	s.progress = map[string]M{}
	s.pending = []M{}
	s.sales = []M{}
	s.discoveries = []M{}
	s.predictions = []M{}
	s.alerts = []M{}
	s.tracker = nil
	s.liveFix = nil
	s.radarShip = nil
	s.current = ""
	s.currentSystem = ""
	s.seen = map[string]bool{}
	s.firstAt = ""
	s.commander = ""
	s.targetCache = nil
	s.totals = M{"salesCount": 0, "soldCredits": 0.0, "unknownSales": 0, "discoveries": 0}
}

func (s *Service) ExportSettings() M {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return M{"schemaVersion": SchemaVersion, "settings": v.Copy(s.settings), "selectedBodyKey": s.selected, "statsSince": s.statsSince, "surfaceMarks": v.Clone(s.marks), "bookmarks": v.Clone(s.bookmarks), "surfaceTracks": v.Clone(s.radarTracks), "radarPreferences": v.Clone(s.radarPrefs)}
}

func (s *Service) configure(values M) error {
	for _, key := range []string{"includeBacterium", "showUnlikely", "onlyUnwalked", "pranavAntalBonus", "trackSamples", "predictionAlerts"} {
		if raw, exists := values[key]; exists {
			b, ok := raw.(bool)
			if !ok {
				return errors.New("Параметр экзобиологии должен быть логическим.")
			}
			s.settings[key] = b
		}
	}
	for key, limit := range map[string]float64{"minValue": 1e12, "minSignals": 50, "maxArrivalLs": 1e9, "radiusLy": 100000} {
		if raw, exists := values[key]; exists {
			n, ok := v.Number(raw)
			if !ok || n < 0 || n > limit {
				return errors.New("Числовой параметр экзобиологии вне допустимого диапазона.")
			}
			s.settings[key] = n
		}
	}
	for _, key := range []string{"selectedGenus", "searchQuery"} {
		if raw, exists := values[key]; exists {
			value := strings.TrimSpace(v.Str(raw))
			if len([]rune(value)) > 160 {
				return errors.New("Слишком длинный фильтр экзобиологии.")
			}
			s.settings[key] = value
		}
	}
	s.settingsRevision++
	return nil
}

func (s *Service) Update(input M) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targetCache = nil
	switch v.Text(input["action"], "configure") {
	case "radar-target", "radar-scale", "radar-clear-track":
		return s.updateRadar(input)
	case "configure":
		before := v.Copy(s.settings)
		if err := s.configure(input); err != nil {
			s.settings = before
			return err
		}
		return nil
	case "select-body":
		key := v.Text(input["bodyKey"], input["id"])
		if s.bodies[key] == nil {
			return errors.New("Тело не найдено в наблюдениях экзобиологии.")
		}
		s.selected = key
		return nil
	case "bookmark":
		key := v.Text(input["bodyKey"], s.selected, s.current)
		b := s.bodies[key]
		if b == nil {
			return errors.New("Сначала выберите известное тело.")
		}
		for _, r := range s.bookmarks {
			if r["bodyKey"] == key {
				return nil
			}
		}
		if len(s.bookmarks) >= 200 {
			return errors.New("Достигнут предел сохранённых мест.")
		}
		if len([]rune(v.Str(input["notes"]))) > 4000 {
			return errors.New("Заметка о месте поиска слишком длинная.")
		}
		s.bookmarks = append(s.bookmarks, M{"bodyKey": key, "name": b.Name, "system": b.System, "systemAddress": b.Address, "bodyId": b.ID, "notes": v.Str(input["notes"])})
		return nil
	case "remove-bookmark":
		out := []M{}
		for _, r := range s.bookmarks {
			if r["bodyKey"] != input["bodyKey"] {
				out = append(out, r)
			}
		}
		s.bookmarks = out
		return nil
	case "clear-marks":
		key := v.Text(input["bodyKey"], s.selected, s.current)
		out := []M{}
		for _, r := range s.marks {
			if r["bodyKey"] != key {
				out = append(out, r)
			}
		}
		s.marks = out
		return nil
	case "dismiss-alert":
		for _, r := range s.alerts {
			if r["id"] == input["id"] {
				r["dismissed"] = true
			}
		}
		return nil
	case "reset-statistics":
		at := v.Time(input["since"])
		if at.IsZero() {
			at = time.Now()
		}
		s.statsSince = v.ISO(at)
		return nil
	case "add-waypoint":
		key := v.Text(input["bodyKey"], s.current)
		b := s.bodies[key]
		lat, lok := v.Number(input["latitude"])
		lon, ook := v.Number(input["longitude"])
		if b == nil || !lok || !ook || math.Abs(lat) > 90 || math.Abs(lon) > 180 {
			return errors.New("Укажите известное тело и корректные координаты.")
		}
		if len([]rune(v.Str(input["label"]))) > 160 {
			return errors.New("Название точки на поверхности слишком длинное.")
		}
		s.addMark(M{"id": v.ID(), "bodyKey": key, "latitude": lat, "longitude": lon, "label": v.Text(input["label"], "Точка на поверхности"), "kind": "waypoint", "at": v.ISO(time.Now()), "owner": s.commander})
		return nil
	case "remove-waypoint":
		out := []M{}
		for _, r := range s.marks {
			if r["id"] != input["id"] {
				out = append(out, r)
			}
		}
		s.marks = out
		return nil
	default:
		return errors.New("Неизвестное действие экзобиологии.")
	}
}

func eventIdentity(event, context M) string {
	if context["file"] != nil && context["offset"] != nil {
		return v.Str(context["file"]) + ":" + v.Str(context["offset"])
	}
	b, _ := json.Marshal(event)
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:])
}
func bodyKey(address, id any) string {
	if v.Str(address) == "" || id == nil {
		return ""
	}
	return v.Str(address) + ":" + v.Str(id)
}
func (s *Service) ensureBody(address, id any, name, systemName, at string) *body {
	key := bodyKey(address, id)
	if key == "" {
		return nil
	}
	b := s.bodies[key]
	if b == nil {
		b = &body{Key: key, Address: v.Str(address), ID: id, Name: name, System: systemName, Scan: M{}, Confirmed: map[string]M{}, Revision: 1}
		s.bodies[key] = b
	}
	if name != "" {
		b.Name = name
	}
	if b.Name == "" {
		b.Name = "Тело " + v.Str(id)
	}
	if systemName != "" {
		b.System = systemName
	}
	if at != "" {
		b.UpdatedAt = at
	}
	sy := s.systems[b.Address]
	if sy == nil {
		sy = &system{Name: systemName, Address: b.Address, BodyIDs: map[string]bool{}}
		s.systems[b.Address] = sy
	}
	sy.BodyIDs[key] = true
	return b
}
func (s *Service) selectedBody() *body {
	if b := s.bodies[s.selected]; b != nil {
		return b
	}
	return s.bodies[s.current]
}
func (s *Service) addMark(mark M) {
	s.marks = append(s.marks, mark)
	if len(s.marks) > maxSavedMarks {
		s.marks = s.marks[len(s.marks)-maxSavedMarks:]
	}
}
func cleanMark(raw M) M {
	lat, lok := v.Number(raw["latitude"])
	lon, ook := v.Number(raw["longitude"])
	if v.Str(raw["bodyKey"]) == "" || !lok || !ook || math.Abs(lat) > 90 || math.Abs(lon) > 180 {
		return nil
	}
	out := M{}
	for _, key := range []string{"id", "bodyKey", "kind", "runKey", "speciesId", "slot", "at", "owner", "label", "planetRadiusM", "heading", "temperatureK", "gravityG", "elevationM", "altitudeAboveGroundM", "pitch", "locationBasis", "uncertaintyM"} {
		if value, exists := raw[key]; exists {
			if text, ok := value.(string); ok && len([]rune(text)) > 1000 {
				continue
			}
			out[key] = v.Clone(value)
		}
	}
	if scans := v.Rows(raw["scans"]); len(scans) > 0 {
		clean := []M{}
		for _, scan := range scans[max(0, len(scans)-12):] {
			lat, lok := v.Number(scan["latitude"])
			lon, ook := v.Number(scan["longitude"])
			if !lok || !ook || math.Abs(lat) > 90 || math.Abs(lon) > 180 {
				continue
			}
			row := M{"latitude": lat, "longitude": lon}
			for _, key := range []string{"heading", "altitudeAboveGroundM", "pitch"} {
				if n, ok := v.Number(scan[key]); ok {
					row[key] = n
				}
			}
			row["at"] = v.Str(scan["at"])
			clean = append(clean, row)
		}
		out["scans"] = clean
	}
	out["latitude"], out["longitude"] = lat, lon
	return out
}
