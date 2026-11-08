// Package ranks tracks observed commander ranks without estimating hidden XP.
package ranks

import (
	"math"
	"sort"
	"strings"
	"sync"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type M = v.M

const historyLimit = 512

type observation struct {
	Rank               *int
	Progress           *float64
	RankAt, ProgressAt string
	RankSource         string
	ProgressSource     string
}

type Tracker struct {
	mu      sync.RWMutex
	owner   string
	rows    map[string]*observation
	history []M
}

func New(saved M) *Tracker {
	t := &Tracker{owner: v.Str(saved["commanderFID"]), rows: map[string]*observation{}, history: []M{}}
	for _, definition := range disciplines {
		row := v.Obj(v.At(saved, "observations", definition.ID))
		entry := &observation{RankAt: timestamp(row["rankUpdatedAt"]), ProgressAt: timestamp(row["progressUpdatedAt"]), RankSource: source(row["rankSource"]), ProgressSource: source(row["progressSource"])}
		if n, ok := rankNumber(row["rankIndex"], definition.Max); ok {
			entry.Rank = &n
		}
		if n, ok := progressNumber(row["progressPercent"]); ok {
			entry.Progress = &n
		}
		t.rows[definition.ID] = entry
	}
	for _, row := range v.Rows(saved["history"]) {
		if definition := byID(v.Str(row["disciplineId"])); definition != nil {
			kind := v.Str(row["kind"])
			if kind != "observed" && kind != "promotion" && kind != "progress" {
				continue
			}
			clean := M{"disciplineId": definition.ID, "kind": kind, "at": timestamp(row["at"]), "source": source(row["source"]), "rankIndex": nil, "fromRankIndex": nil, "progressPercent": nil}
			if n, ok := rankNumber(row["rankIndex"], definition.Max); ok {
				clean["rankIndex"] = n
			}
			if n, ok := rankNumber(row["fromRankIndex"], definition.Max); ok {
				clean["fromRankIndex"] = n
			}
			if n, ok := progressNumber(row["progressPercent"]); ok {
				clean["progressPercent"] = n
			}
			if clean["rankIndex"] != nil || clean["progressPercent"] != nil {
				t.appendHistory(clean)
			}
		}
	}
	return t
}

// Reset starts a replay without discarding previously confirmed observations.
// Older journal records are ignored by their field timestamps. An actual game
// reset or a different commander clears the observations through Apply.
func (t *Tracker) Reset() {}

func source(value any) string {
	if value == "capi" {
		return "capi"
	}
	if value == "journal" {
		return "journal"
	}
	return ""
}

func timestamp(value any) string {
	parsed := v.Time(v.Str(value))
	if parsed.IsZero() {
		return ""
	}
	return v.ISO(parsed)
}

func rankNumber(value any, maximum int) (int, bool) {
	if _, text := value.(string); text {
		return 0, false
	}
	n, known := v.Number(value)
	return int(n), known && !math.IsNaN(n) && !math.IsInf(n, 0) && n == math.Trunc(n) && n >= 0 && n <= float64(maximum)
}

func progressNumber(value any) (float64, bool) {
	if _, text := value.(string); text {
		return 0, false
	}
	n, known := v.Number(value)
	return n, known && !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 && n <= 100
}

func fresh(at, previous, incomingSource, previousSource string) bool {
	if previous == "" {
		return true
	}
	if at == "" || v.Time(at).Before(v.Time(previous)) {
		return false
	}
	return at != previous || incomingSource != "capi" || previousSource != "journal"
}

func (t *Tracker) clear(owner string) {
	t.owner, t.rows, t.history = owner, map[string]*observation{}, []M{}
}

func (t *Tracker) latest() string {
	latest := ""
	for _, row := range t.rows {
		for _, at := range []string{row.RankAt, row.ProgressAt} {
			if at > latest {
				latest = at
			}
		}
	}
	return latest
}

func (t *Tracker) Apply(event, context M) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	kind := v.Str(event["event"])
	at := timestamp(event["timestamp"])
	owner := v.Text(event["FID"], v.At(context, "commander", "fid"))
	changed := false
	if owner != "" && owner != t.owner {
		t.clear(owner)
		changed = true
	}
	if kind == "ClearSavedGame" || kind == "NewCommander" {
		if t.latest() == "" || at != "" && !v.Time(at).Before(v.Time(t.latest())) {
			t.clear(owner)
			return true
		}
		return changed
	}
	if kind != "Rank" && kind != "Progress" && kind != "Promotion" {
		return changed
	}
	return t.observe(event, at, "journal", kind) || changed
}

// ApplyCAPI supplements ranks from an already identity-verified Frontier
// profile. It does not infer percentages or promotion dates from that snapshot.
func (t *Tracker) ApplyCAPI(profile, context M) bool {
	if context["identityVerified"] != true {
		return false
	}
	owner := v.Str(v.At(context, "commander", "fid"))
	at := timestamp(context["observedAt"])
	name, expected := v.Str(v.At(profile, "commander", "name")), v.Str(v.At(context, "commander", "name"))
	if owner == "" || at == "" || name == "" || expected != "" && !strings.EqualFold(name, expected) {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.owner != "" && t.owner != owner {
		return false
	}
	t.owner = owner
	input := M{}
	for _, definition := range disciplines {
		if value, exists := v.Obj(v.At(profile, "commander", "rank"))[definition.ID]; exists {
			input[definition.Journal] = value
		}
	}
	return t.observe(input, at, "capi", "Rank")
}

func (t *Tracker) observe(event M, at, origin, kind string) bool {
	changed := false
	for _, definition := range disciplines {
		value, exists := event[definition.Journal]
		if !exists {
			continue
		}
		row := t.rows[definition.ID]
		if row == nil {
			row = &observation{}
			t.rows[definition.ID] = row
		}
		if kind == "Progress" {
			n, valid := progressNumber(value)
			if !valid || !fresh(at, row.ProgressAt, origin, row.ProgressSource) || row.RankAt != "" && (at == "" || v.Time(at).Before(v.Time(row.RankAt))) {
				continue
			}
			if row.Progress == nil || *row.Progress != n {
				t.appendHistory(M{"disciplineId": definition.ID, "kind": "progress", "at": at, "source": origin, "rankIndex": row.Rank, "progressPercent": n})
			}
			changed = changed || row.Progress == nil || *row.Progress != n || row.ProgressAt != at
			row.Progress, row.ProgressAt, row.ProgressSource = &n, at, origin
			continue
		}
		n, valid := rankNumber(value, definition.Max)
		if !valid || !fresh(at, row.RankAt, origin, row.RankSource) || row.Rank != nil && n < *row.Rank {
			continue
		}
		if row.Rank == nil || n != *row.Rank {
			historyKind := "observed"
			if kind == "Promotion" {
				historyKind = "promotion"
			}
			t.appendHistory(M{"disciplineId": definition.ID, "kind": historyKind, "at": at, "source": origin, "rankIndex": n, "fromRankIndex": row.Rank})
			if row.Rank != nil || row.ProgressAt != "" && at != "" && v.Time(row.ProgressAt).Before(v.Time(at)) {
				row.Progress, row.ProgressAt, row.ProgressSource = nil, "", ""
			}
			changed = true
		}
		changed = changed || row.RankAt != at
		row.Rank, row.RankAt, row.RankSource = &n, at, origin
	}
	return changed
}

func (t *Tracker) appendHistory(row M) {
	row = v.Copy(row)
	for _, key := range []string{"rankIndex", "fromRankIndex", "progressPercent"} {
		if _, exists := row[key]; !exists {
			row[key] = nil
		}
	}
	if len(t.history) > 0 {
		last := t.history[len(t.history)-1]
		if v.Str(last["disciplineId"]) == v.Str(row["disciplineId"]) && v.Str(last["kind"]) == v.Str(row["kind"]) && v.Str(last["at"]) == v.Str(row["at"]) && v.Str(last["rankIndex"]) == v.Str(row["rankIndex"]) && v.Str(last["progressPercent"]) == v.Str(row["progressPercent"]) {
			return
		}
	}
	t.history = append(t.history, row)
	if len(t.history) > historyLimit {
		t.history = t.history[len(t.history)-historyLimit:]
	}
}

func (t *Tracker) ExportSettings() M {
	t.mu.RLock()
	defer t.mu.RUnlock()
	rows := M{}
	for id, row := range t.rows {
		rows[id] = M{"rankIndex": row.Rank, "progressPercent": row.Progress, "rankUpdatedAt": row.RankAt, "progressUpdatedAt": row.ProgressAt, "rankSource": row.RankSource, "progressSource": row.ProgressSource}
	}
	return v.Copy(M{"version": 1, "commanderFID": t.owner, "observations": rows, "history": t.history})
}

func (t *Tracker) Snapshot() M { return t.SnapshotForLanguage("ru") }

func (t *Tracker) SnapshotForLanguage(language string) M {
	t.mu.RLock()
	defer t.mu.RUnlock()
	rows, history := []M{}, []M{}
	known, elite := 0, 0
	for _, definition := range disciplines {
		row := t.rows[definition.ID]
		if row == nil {
			row = &observation{}
		}
		x := M{"id": definition.ID, "title": definition.Title.Get(language), "icon": definition.Icon, "rankIndex": row.Rank, "rankName": nil, "rankNameEnglish": nil, "progressPercent": row.Progress, "rankUpdatedAt": row.RankAt, "progressUpdatedAt": row.ProgressAt, "rankSource": row.RankSource, "progressSource": row.ProgressSource, "nextRankIndex": nil, "nextRankName": nil, "maxed": false, "timeline": []M{}, "guide": guideFor(definition.ID, language)}
		if row.Rank != nil {
			known++
			x["rankName"], x["rankNameEnglish"] = definition.Names[*row.Rank].Get(language), definition.Names[*row.Rank].EN
			x["maxed"] = *row.Rank == definition.Max
			if *row.Rank < definition.Max {
				x["nextRankIndex"], x["nextRankName"] = *row.Rank+1, definition.Names[*row.Rank+1].Get(language)
			}
			if definition.Max == 13 && *row.Rank >= 8 {
				elite++
			}
		}
		timeline := []M{}
		for index, name := range definition.Names {
			status := "unknown"
			if row.Rank != nil {
				status = "locked"
				if index < *row.Rank {
					status = "reached"
				} else if index == *row.Rank {
					status = "current"
				}
			}
			timeline = append(timeline, M{"rankIndex": index, "rankName": name.Get(language), "rankNameEnglish": name.EN, "status": status, "rewards": rewardsFor(definition.ID, index, language)})
		}
		x["timeline"] = timeline
		rows = append(rows, x)
	}
	for _, event := range t.history {
		x := v.Copy(event)
		if definition := byID(v.Str(x["disciplineId"])); definition != nil {
			x["disciplineTitle"] = definition.Title.Get(language)
			if index, valid := rankNumber(x["rankIndex"], definition.Max); valid {
				x["rankName"] = definition.Names[index].Get(language)
			}
		}
		history = append(history, x)
	}
	sort.SliceStable(history, func(i, j int) bool { return v.Str(history[i]["at"]) > v.Str(history[j]["at"]) })
	return v.Copy(M{"disciplines": rows, "history": history, "summary": M{"known": known, "total": len(disciplines), "eliteDisciplines": elite}, "language": normalizedLanguage(language)})
}
