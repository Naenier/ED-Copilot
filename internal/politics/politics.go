// Package politics keeps dated public political observations separate from
// personally observed journal progress. Snapshot and Apply never use the network.
package politics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Naenier/ED-Copilot/internal/freshness"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type M = v.M

const MaxSystems = 40
const refreshInterval = 5 * time.Minute

type Options struct {
	HTTPClient          *http.Client
	DisableNetwork      bool
	Now                 func() time.Time
	SpanshURL, GoalsURL string // Endpoint overrides are for deterministic tests.
}

type Service struct {
	mu                                                             sync.RWMutex
	opts                                                           Options
	selected, current, commander                                   string
	systems                                                        map[string]M
	publicGoals, personalGoals, powerIndex, search, pledge, status M
	lastRequests                                                   map[string]time.Time
	failedRequests                                                 map[string]requestFailure
	activeRequest                                                  context.Context
	requestID, selectionID                                         uint64
}

func New(saved M, opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{}
	}
	client := *opts.HTTPClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	opts.HTTPClient = &client
	if opts.SpanshURL == "" {
		opts.SpanshURL = "https://spansh.co.uk/api"
	}
	if opts.GoalsURL == "" {
		opts.GoalsURL = "https://api.orerve.net/2.0/website/initiatives/list"
	}
	s := &Service{opts: opts, systems: map[string]M{}, lastRequests: map[string]time.Time{}, failedRequests: map[string]requestFailure{}, publicGoals: M{}, personalGoals: M{}, powerIndex: M{}, search: M{}, pledge: M{}, status: M{"running": false}}
	s.selected = clean(saved["selectedSystem"], 200)
	// Only bounded normalized snapshots are persisted. They remain observations,
	// not evidence that a system or goal still has the same state today.
	for _, row := range boundedRows(saved["systems"], MaxSystems) {
		if name := clean(row["name"], 200); name != "" {
			s.systems[key(name)] = v.Copy(row)
		}
	}
	for name, target := range map[string]*M{"publicGoals": &s.publicGoals, "personalGoals": &s.personalGoals, "powerIndex": &s.powerIndex, "search": &s.search} {
		if raw := v.Obj(saved[name]); len(raw) > 0 {
			*target = v.Copy(raw)
		}
	}
	// Discard oversized imported state rather than making startup unbounded.
	if len(v.Rows(s.publicGoals["entries"])) > 100 {
		s.publicGoals = M{}
	}
	if len(v.Rows(s.personalGoals["entries"])) > 100 {
		s.personalGoals = M{}
	}
	if len(v.Rows(s.search["entries"])) > 10 {
		s.search = M{}
	}
	return s
}

func clean(raw any, limit int) string {
	text := strings.TrimSpace(v.Str(raw))
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > limit {
		return ""
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return ""
		}
	}
	return text
}
func key(name string) string { return strings.ToLower(strings.Join(strings.Fields(name), " ")) }
func boundedRows(raw any, max int) []M {
	rows := v.Rows(raw)
	if len(rows) > max {
		rows = rows[:max]
	}
	return rows
}
func number(raw any) any {
	if n, ok := v.Number(raw); ok {
		return n
	}
	return nil
}
func fraction(raw any) any {
	if n, ok := v.Number(raw); ok && n >= 0 && n <= 1 {
		return n
	}
	return nil
}
func date(raw any) string {
	if t := v.Time(raw); !t.IsZero() {
		return v.ISO(t)
	}
	return ""
}
func (s *Service) now() string { return v.ISO(s.opts.Now()) }
func meta(source, observed, fetched, url string) M {
	return M{"source": source, "observedAt": observed, "fetchedAt": fetched, "sourceURL": url, "known": true}
}

func (s *Service) trimLocked() {
	if len(s.systems) <= MaxSystems {
		return
	}
	rows := make([]M, 0, len(s.systems))
	for _, row := range s.systems {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return v.Str(rows[i]["updatedAt"]) > v.Str(rows[j]["updatedAt"]) })
	keep := map[string]M{}
	for _, row := range rows[:MaxSystems] {
		keep[key(v.Str(row["name"]))] = row
	}
	s.systems = keep
}

func (s *Service) mergeLocked(row M) {
	name := clean(row["name"], 200)
	if name == "" {
		return
	}
	id := key(name)
	old := s.systems[id]
	if old == nil {
		old = M{"name": name}
	}
	if row["address"] != nil && v.Str(row["address"]) != "" {
		old["address"] = row["address"]
	}
	if at := v.Time(row["detailsFetchedAt"]); at.After(v.Time(old["detailsFetchedAt"])) {
		old["detailsFetchedAt"] = v.ISO(at)
	}
	for _, domain := range []string{"info", "factions", "conflicts", "powerplay", "thargoid"} {
		next := v.Obj(row[domain])
		if len(next) == 0 {
			continue
		}
		previous := v.Obj(old[domain])
		// Unknown upstream observation time must not silently supersede a dated
		// journal record. Each domain retains its own provenance and timestamp.
		if len(previous) > 0 && date(previous["observedAt"]) != "" && (date(next["observedAt"]) == "" || v.Time(next["observedAt"]).Before(v.Time(previous["observedAt"]))) {
			continue
		}
		old[domain] = v.Copy(next)
	}
	old["updatedAt"] = s.now()
	s.systems[id] = old
	s.trimLocked()
}

func (s *Service) Update(body M) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch v.Text(body["action"], "select-system") {
	case "select-system":
		name := clean(body["system"], 200)
		if name == "" {
			return errors.New("Enter a system name")
		}
		s.setSelection(name)
		for _, row := range v.Rows(s.search["entries"]) {
			if key(v.Str(row["name"])) == key(name) {
				s.mergeLocked(row)
				break
			}
		}
	case "select-current":
		if s.current == "" {
			return errors.New("Current system is not observed yet")
		}
		s.setSelection("")
	default:
		return errors.New("Unknown politics action")
	}
	return nil
}

func (s *Service) setSelection(name string) {
	if key(s.selected) != key(name) {
		s.selectionID++
	}
	s.selected = name
}

func (s *Service) ResetObserved() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = ""
	s.selectionID++
	s.commander = ""
	s.personalGoals = M{}
	s.pledge = M{}
	// Public cache survives journal replay. Remove journal-only domains so replay
	// reconstructs the actual current commander's observations without leakage.
	for id, row := range s.systems {
		for _, domain := range []string{"info", "factions", "conflicts", "powerplay", "thargoid"} {
			if v.At(row, domain, "source") == "journal" {
				delete(row, domain)
			}
		}
		if len(row) <= 3 {
			delete(s.systems, id)
		}
	}
}

func (s *Service) Snapshot() M {
	s.mu.RLock()
	defer s.mu.RUnlock()
	name := s.selected
	if name == "" {
		name = s.current
	}
	active := v.Copy(s.systems[key(name)])
	if len(active) == 0 && name != "" {
		active = M{"name": name}
	}
	systems := []M{}
	for _, row := range s.systems {
		systems = append(systems, v.Copy(row))
	}
	sort.Slice(systems, func(i, j int) bool { return v.Str(systems[i]["name"]) < v.Str(systems[j]["name"]) })
	now := s.opts.Now()
	for _, domain := range []string{"info", "factions", "conflicts", "powerplay", "thargoid"} {
		if raw := v.Obj(active[domain]); len(raw) > 0 {
			active[domain] = describeObservation(raw, now)
		}
	}
	return M{"selectedSystem": name, "currentSystem": s.current, "activeSystem": active, "systems": systems, "communityGoals": describeObservation(s.publicGoals, now), "personalGoals": describeObservation(s.personalGoals, now), "powerIndex": describeObservation(s.powerIndex, now), "search": describeObservation(s.search, now), "pledge": describeObservation(s.pledge, now), "status": v.Merge(v.Copy(s.status), M{"offline": s.opts.DisableNetwork}), "limits": M{"systems": MaxSystems, "searchPageSize": 10, "cacheMinutes": refreshInterval.Minutes()}}
}

func describeObservation(raw M, now time.Time) M {
	out := v.Copy(raw)
	known := v.Bool(raw["known"])
	coverage := freshness.Complete
	if v.Bool(raw["partial"]) {
		coverage = freshness.Partial
	} else if entries, exists := raw["entries"]; known && exists && len(v.Rows(entries)) == 0 {
		coverage = freshness.Empty
	} else if !known {
		coverage = ""
	}
	quality := freshness.Describe(now, 30*time.Minute, known, coverage, v.Time(raw["observedAt"]), v.Time(raw["fetchedAt"]))
	out["quality"] = quality
	out["stale"] = quality.State != freshness.Fresh
	return out
}

func (s *Service) ExportSettings() M {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := []M{}
	for _, row := range s.systems {
		rows = append(rows, row)
	}
	return v.Copy(M{"version": 1, "selectedSystem": s.selected, "systems": rows, "publicGoals": s.publicGoals, "personalGoals": s.personalGoals, "powerIndex": s.powerIndex, "search": s.search})
}

func (s *Service) Fetch(ctx context.Context, body M) (M, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, field := range []string{"kind", "system", "power", "state"} {
		if raw, exists := body[field]; exists {
			value, ok := raw.(string)
			limit := 100
			if field == "system" {
				limit = 200
			}
			if !ok || strings.ContainsAny(value, "\r\n\t") || (strings.TrimSpace(value) != "" && clean(value, limit) == "") {
				return nil, fmt.Errorf("Invalid politics %s", field)
			}
		}
	}
	if raw, exists := body["page"]; exists {
		page, ok := v.Number(raw)
		if !ok || page != math.Trunc(page) || page < 0 || page > 100 {
			return nil, errors.New("Page must be a whole number between 0 and 100")
		}
	}
	kind := v.Text(body["kind"], "system")
	if kind != "system" && kind != "goals" && kind != "powerplay" && kind != "thargoid" {
		return nil, errors.New("Unknown political data request")
	}
	language := "en"
	if raw, exists := body["language"]; exists {
		value, ok := raw.(string)
		if kind != "goals" || !ok || (value != "en" && value != "ru") {
			return nil, errors.New("Invalid community goal language")
		}
		language = value
	}
	keepSelection, _ := body["keepSelection"].(bool)
	s.mu.Lock()
	if s.activeRequest != nil && s.activeRequest.Err() == nil {
		s.mu.Unlock()
		return nil, errors.New("A politics refresh is already running")
	}
	name := clean(body["system"], 200)
	if name == "" {
		name = s.selected
	}
	if name == "" {
		name = s.current
	}
	if kind == "system" && name == "" {
		s.mu.Unlock()
		return nil, errors.New("Enter a system name")
	}
	power, state := clean(body["power"], 100), clean(body["state"], 100)
	if raw, exists := body["system"]; exists && clean(raw, 200) == "" && v.Str(raw) != "" {
		s.mu.Unlock()
		return nil, errors.New("Invalid system name")
	}
	page := v.Int(body["page"])
	if page < 0 || page > 100 {
		s.mu.Unlock()
		return nil, errors.New("Page must be between 0 and 100")
	}
	systemKey := ""
	if kind == "system" {
		systemKey = key(name)
	}
	requestKey := fmt.Sprintf("%s|%s|%s|%s|%d|%s", kind, systemKey, power, state, page, language)
	params := M{"system": name, "language": language, "power": power, "state": state, "page": page}
	if s.opts.DisableNetwork {
		s.status = failedStatus(kind, params, requestFailure{reason: freshness.Offline, description: "offline", failedAt: s.opts.Now()})
		s.mu.Unlock()
		return M{"status": "offline"}, nil
	}
	if failure, exists := s.failedRequests[requestKey]; exists && s.opts.Now().Before(failure.retryAt) {
		s.status = failedStatus(kind, params, failure)
		s.mu.Unlock()
		return nil, errors.New("Wait one minute before retrying this public request")
	}
	cacheAvailable := kind == "system" && s.systems[key(name)] != nil ||
		kind == "goals" && v.Bool(s.publicGoals["known"]) && v.Text(s.publicGoals["language"], "en") == language ||
		(kind == "powerplay" || kind == "thargoid") && s.search["requestKey"] == requestKey
	if at := s.lastRequests[requestKey]; cacheAvailable && !at.IsZero() && s.opts.Now().Sub(at) < refreshInterval {
		if kind == "system" && !keepSelection {
			s.setSelection(name)
		}
		s.mu.Unlock()
		return M{"status": "cached", "kind": kind}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	s.requestID++
	requestID, selectionID := s.requestID, s.selectionID
	s.activeRequest = ctx
	s.status = M{"running": true, "kind": kind, "parameters": params, "startedAt": s.now()}
	s.mu.Unlock()
	result, err := s.fetch(ctx, kind, name, power, state, language, page)
	s.mu.Lock()
	defer s.mu.Unlock()
	if requestID != s.requestID {
		return nil, context.Canceled
	}
	s.activeRequest = nil
	if errors.Is(ctx.Err(), context.Canceled) || kind == "system" && selectionID != s.selectionID {
		s.status = M{"running": false}
		return nil, context.Canceled
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		if index := v.Obj(result["index"]); kind == "powerplay" && len(index) > 0 {
			s.powerIndex = index
		}
		failure := failureDetails(err, s.opts.Now())
		s.status = failedStatus(kind, params, failure)
		s.failedRequests[requestKey] = failure
		if len(s.failedRequests) > 128 {
			s.failedRequests = map[string]requestFailure{requestKey: failure}
		}
		return nil, err
	}
	s.lastRequests[requestKey] = s.opts.Now()
	delete(s.failedRequests, requestKey)
	if len(s.lastRequests) > 128 {
		s.lastRequests = map[string]time.Time{requestKey: s.opts.Now()}
	}
	switch kind {
	case "system":
		s.mergeLocked(v.Obj(result["system"]))
		if !keepSelection {
			s.setSelection(name)
		}
	case "goals":
		s.publicGoals = v.Obj(result["goals"])
	case "powerplay":
		s.powerIndex = v.Obj(result["index"])
		s.search = v.Obj(result["search"])
		s.search["requestKey"] = requestKey
	case "thargoid":
		s.search = v.Obj(result["search"])
		s.search["requestKey"] = requestKey
	}
	s.status = M{"running": false, "kind": kind, "parameters": params, "updatedAt": s.now()}
	return M{"status": "updated", "kind": kind}, nil
}
