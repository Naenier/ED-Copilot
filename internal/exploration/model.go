// Package exploration keeps commander-scoped cartographic observations and plans.
package exploration

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type M = v.M

const (
	maxWorkspaces   = 12
	maxSystems      = 6000
	maxBodies       = 30000
	maxSystemBodies = 500
	maxExpeditions  = 24
	maxSurveys      = 24
	maxTargets      = 400
	maxTravel       = 2000
	maxFindings     = 800
	maxReceipts     = 400
	pageSize        = 30
)

var workspaceID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

type Service struct {
	mu         sync.RWMutex
	workspaces map[string]*workspace
	now        func() time.Time
}

type workspace struct {
	Expeditions     []*expedition      `json:"expeditions,omitempty"`
	Surveys         []*survey          `json:"surveys,omitempty"`
	SelectedSurvey  string             `json:"selectedSurvey,omitempty"`
	FindingProfiles []findingProfile   `json:"findingProfiles,omitempty"`
	SelectedProfile string             `json:"selectedProfile,omitempty"`
	Notes           map[string]string  `json:"notes,omitempty"`
	Receipts        []receipt          `json:"receipts,omitempty"`
	LastLoss        string             `json:"lastLoss,omitempty"`
	Findings        []finding          `json:"findings,omitempty"`
	Systems         map[string]*system `json:"systems,omitempty"`
	Records         map[string]record  `json:"records,omitempty"`

	names       map[string]string
	order       []string
	oldest      int
	bodies      int
	findings    map[string]bool
	receipts    map[string]bool
	lastAlert   map[string]string
	paidSystems map[string]string
}

type system struct {
	Address     string           `json:"address"`
	Name        string           `json:"name"`
	Coordinates []float64        `json:"coordinates,omitempty"`
	LastAt      string           `json:"lastAt"`
	VisitedAt   string           `json:"visitedAt,omitempty"`
	FirstVisit  string           `json:"firstVisit,omitempty"`
	FSSAt       string           `json:"fssAt,omitempty"`
	SoldAt      string           `json:"soldAt,omitempty"`
	BodyCount   *int             `json:"bodyCount,omitempty"`
	Bodies      map[string]*body `json:"bodies,omitempty"`
}

type ring struct {
	Class string  `json:"class,omitempty"`
	Inner float64 `json:"inner"`
	Outer float64 `json:"outer"`
}

type body struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	PlanetClass   string             `json:"planetClass,omitempty"`
	StarClass     string             `json:"starClass,omitempty"`
	Terraformable bool               `json:"terraformable,omitempty"`
	Landable      bool               `json:"landable,omitempty"`
	Mass          *float64           `json:"mass,omitempty"`
	StellarMass   *float64           `json:"stellarMass,omitempty"`
	Temperature   *float64           `json:"temperature,omitempty"`
	Gravity       *float64           `json:"gravity,omitempty"`
	Radius        *float64           `json:"radius,omitempty"`
	Distance      *float64           `json:"distanceLs,omitempty"`
	Eccentricity  *float64           `json:"eccentricity,omitempty"`
	SemiMajorAxis *float64           `json:"semiMajorAxis,omitempty"`
	ParentID      string             `json:"parentId,omitempty"`
	Rings         []ring             `json:"rings,omitempty"`
	Materials     map[string]float64 `json:"materials,omitempty"`
	VisitedAt     string             `json:"visitedAt,omitempty"`
	ScannedAt     string             `json:"scannedAt,omitempty"`
	ScanFirst     string             `json:"scanFirst,omitempty"`
	ScanPayable   string             `json:"scanPayable,omitempty"`
	MappedAt      string             `json:"mappedAt,omitempty"`
	MapFirst      string             `json:"mapFirst,omitempty"`
	Efficient     bool               `json:"efficient,omitempty"`
}

type target struct {
	System    string  `json:"system"`
	Address   string  `json:"address,omitempty"`
	Note      string  `json:"note,omitempty"`
	Skip      bool    `json:"skip,omitempty"`
	Candidate bool    `json:"candidate,omitempty"`
	Imported  *stages `json:"importedProgress,omitempty"`
}

type stages struct {
	VisitedAt string `json:"visitedAt,omitempty"`
	FSSAt     string `json:"fssAt,omitempty"`
	MappedAt  string `json:"mappedAt,omitempty"`
}

type travel struct {
	Address     string    `json:"address"`
	System      string    `json:"system"`
	At          string    `json:"at"`
	Coordinates []float64 `json:"coordinates,omitempty"`
}

type expedition struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Goal         string   `json:"goal,omitempty"`
	Notes        string   `json:"notes,omitempty"`
	ReturnSystem string   `json:"returnSystem,omitempty"`
	Days         int      `json:"days,omitempty"`
	CreatedAt    string   `json:"createdAt"`
	StartedAt    string   `json:"startedAt,omitempty"`
	EndedAt      string   `json:"endedAt,omitempty"`
	Stops        []target `json:"stops"`
	Travel       []travel `json:"travel,omitempty"`
	travelIDs    map[string]bool
}

type survey struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Notes        string   `json:"notes,omitempty"`
	CreatedAt    string   `json:"createdAt,omitempty"`
	Criterion    string   `json:"criterion"`
	MappingTypes []string `json:"mappingTypes,omitempty"`
	SkipVisited  bool     `json:"skipVisited,omitempty"`
	BoxelSeed    string   `json:"boxelSeed,omitempty"`
	SequenceFrom int      `json:"sequenceFrom"`
	SequenceTo   int      `json:"sequenceTo"`
	Targets      []target `json:"targets"`
}

type receipt struct {
	ID              string   `json:"id"`
	At              string   `json:"at"`
	Kind            string   `json:"kind"`
	Systems         []string `json:"systems,omitempty"`
	Credits         *int64   `json:"credits,omitempty"`
	Estimated       int64    `json:"estimated,omitempty"`
	EstimatedBodies int      `json:"estimatedBodies,omitempty"`
	ExpeditionID    string   `json:"expeditionId,omitempty"`
}

func New(saved M) *Service {
	s := &Service{workspaces: map[string]*workspace{}, now: time.Now}
	keys := make([]string, 0, len(saved))
	for key := range saved {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !workspaceID.MatchString(key) || len(s.workspaces) >= maxWorkspaces {
			continue
		}
		ws := workspace{}
		if raw, err := json.Marshal(saved[key]); err != nil || json.Unmarshal(raw, &ws) != nil {
			continue
		}
		ws.restore()
		s.workspaces[key] = &ws
	}
	return s
}

func newWorkspace() *workspace {
	ws := &workspace{}
	ws.restore()
	return ws
}

func (ws *workspace) restore() {
	if ws.Notes == nil {
		ws.Notes = map[string]string{}
	}
	if ws.Systems == nil {
		ws.Systems = map[string]*system{}
	}
	ws.names, ws.findings, ws.receipts = map[string]string{}, map[string]bool{}, map[string]bool{}
	ws.lastAlert, ws.paidSystems = map[string]string{}, map[string]string{}
	if ws.Records == nil {
		ws.Records = map[string]record{}
	}
	ws.order, ws.oldest, ws.bodies = nil, 0, 0
	ws.restoreFindingProfiles()
	ws.Expeditions = bounded(ws.Expeditions, maxExpeditions)
	ws.Surveys = bounded(ws.Surveys, maxSurveys)
	ws.Findings = tail(ws.Findings, maxFindings)
	ws.Receipts = tail(ws.Receipts, maxReceipts)
	for _, exp := range ws.Expeditions {
		if exp == nil {
			continue
		}
		exp.Stops = bounded(exp.Stops, maxTargets)
		exp.Travel = bounded(exp.Travel, maxTravel)
		exp.travelIDs = map[string]bool{}
		for _, visit := range exp.Travel {
			exp.travelIDs[visit.Address+"|"+visit.At] = true
		}
	}
	for _, row := range ws.Surveys {
		if row != nil {
			row.Targets = bounded(row.Targets, maxTargets)
		}
	}
	for _, row := range ws.Findings {
		ws.findings[row.ID] = true
		if row.NotifiedAt != "" {
			ws.lastAlert[row.Rule] = latest(row.NotifiedAt, ws.lastAlert[row.Rule])
		}
	}
	for _, row := range ws.Receipts {
		ws.indexReceipt(row)
	}
	for id, sy := range ws.Systems {
		if sy == nil || address(id) == "" || sy.Address != id {
			delete(ws.Systems, id)
			continue
		}
		ws.order = append(ws.order, id)
	}
	sort.Slice(ws.order, func(i, j int) bool { return ws.Systems[ws.order[i]].LastAt < ws.Systems[ws.order[j]].LastAt })
	for _, id := range ws.order {
		sy := ws.Systems[id]
		ws.names[normalized(sy.Name)] = id
		if sy.Bodies == nil {
			sy.Bodies = map[string]*body{}
		}
		ws.bodies += len(sy.Bodies)
		for _, b := range sy.Bodies {
			if b != nil {
				ws.updateRecords(sy, b)
			}
		}
	}
	for len(ws.Systems) > maxSystems || ws.bodies > maxBodies {
		if !ws.evict("") {
			break
		}
	}
}

func (ws *workspace) evict(exclude string) bool {
	for index := 0; index < len(ws.order); index++ {
		pos := (ws.oldest + index) % len(ws.order)
		id := ws.order[pos]
		if id == "" || id == exclude {
			continue
		}
		if sy := ws.Systems[id]; sy != nil {
			if key := normalized(sy.Name); ws.names[key] == id {
				delete(ws.names, key)
			}
			ws.bodies -= len(sy.Bodies)
			delete(ws.Systems, id)
		}
		ws.order[pos] = ""
		ws.oldest = (pos + 1) % len(ws.order)
		return true
	}
	return false
}

func (s *Service) workspace(owner string, create bool) *workspace {
	if !workspaceID.MatchString(owner) {
		return nil
	}
	if ws := s.workspaces[owner]; ws != nil || !create {
		return ws
	}
	if len(s.workspaces) >= maxWorkspaces {
		return nil
	}
	ws := newWorkspace()
	s.workspaces[owner] = ws
	return ws
}

func address(raw any) string {
	if n, ok := raw.(float64); ok {
		if n <= 0 || n >= 1<<53 || math.Trunc(n) != n {
			return ""
		}
		return strconv.FormatUint(uint64(n), 10)
	}
	s := v.Str(raw)
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil || id == 0 {
		return ""
	}
	return strconv.FormatUint(id, 10)
}

func bodyID(raw any) string {
	if n, ok := raw.(float64); ok {
		if n < 0 || n > math.MaxUint32 || math.Trunc(n) != n {
			return ""
		}
		return strconv.FormatUint(uint64(n), 10)
	}
	s := v.Str(raw)
	id, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return ""
	}
	return strconv.FormatUint(id, 10)
}

func normalized(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
func text(raw any, maxRunes int) string {
	s := strings.TrimSpace(v.Str(raw))
	i := 0
	for at := range s {
		if i >= maxRunes {
			return s[:at]
		}
		i++
	}
	return s
}

func atTime(raw any) string {
	at := v.Time(raw)
	if at.IsZero() {
		return ""
	}
	return v.ISO(at)
}
func newer(a, b string) bool { return a != "" && (b == "" || v.Time(a).After(v.Time(b))) }
func latest(a, b string) string {
	if newer(a, b) {
		return a
	}
	return b
}
func earliest(a, b string) string {
	if a != "" && (b == "" || newer(b, a)) {
		return a
	}
	return b
}
func bounded[T any](a []T, limit int) []T { return a[:min(len(a), limit)] }
func tail[T any](a []T, limit int) []T    { return a[max(0, len(a)-limit):] }
