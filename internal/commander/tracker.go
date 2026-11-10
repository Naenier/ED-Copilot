// Package commander builds an in-memory, owner-scoped view of observations.
// It reads no files or credentials and never treats an absent observation as zero.
package commander

import (
	"encoding/json"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/Naenier/ED-Copilot/internal/ships"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type M = v.M

const (
	maxShips              = 512
	maxModules            = 2048
	maxInventoryRows      = 1024
	maxStatisticsSections = 48
	maxStatisticsFields   = 128
)

type stamp struct{ at, source string }

type Tracker struct {
	mu                         sync.RWMutex
	owner, ownerAt, latest     string
	state                      M
	stamps                     map[string]stamp
	shipPresence, aboardShipID string
}

func New() *Tracker { t := &Tracker{}; t.clear(""); return t }

func unknown() M   { return M{"known": false, "updatedAt": nil, "source": ""} }
func emptyRows() M { return v.Merge(unknown(), M{"entries": []M{}, "count": nil, "truncated": false}) }

func (t *Tracker) clear(owner string) {
	t.owner, t.ownerAt, t.latest = owner, "", ""
	t.stamps = map[string]stamp{}
	t.shipPresence, t.aboardShipID = "unknown", ""
	materials, backpack, locker := M{}, M{}, M{}
	for _, k := range []string{"raw", "manufactured", "encoded"} {
		materials[k] = emptyRows()
	}
	for _, k := range []string{"items", "components", "consumables", "data"} {
		backpack[k], locker[k] = emptyRows(), emptyRows()
	}
	t.state = M{
		"finance":       v.Merge(unknown(), M{"credits": nil, "loan": nil, "netWorth": nil, "rebuy": nil, "hullValue": nil, "modulesValue": nil, "shipValue": nil, "fields": M{}}),
		"fleet":         v.Merge(emptyRows(), M{"complete": false, "scope": "storedShips", "currentShip": v.Merge(unknown(), M{"id": nil, "shipId": nil})}),
		"storedModules": emptyRows(),
		"reputation":    v.Merge(unknown(), M{"federation": nil, "empire": nil, "alliance": nil, "independent": nil, "fields": M{}}),
		"inventory":     M{"materials": materials, "backpack": backpack, "shipLocker": locker},
		"statistics":    v.Merge(unknown(), M{"sections": []M{}}),
	}
}

// Reset discards all derived observations before the journal reader replays.
func (t *Tracker) Reset() { t.mu.Lock(); defer t.mu.Unlock(); t.clear("") }

func timestamp(x any) string {
	at := v.Time(x)
	if at.IsZero() {
		return ""
	}
	return v.ISO(at)
}
func clean(x any) string {
	s, ok := x.(string)
	if !ok {
		return ""
	}
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s))
	r := []rune(s)
	if len(r) > 180 {
		r = r[:180]
	}
	return string(r)
}

// IDs cross the JavaScript boundary as decimal strings, without float conversion.
func exactID(x any) string {
	var s string
	switch n := x.(type) {
	case string:
		s = strings.TrimSpace(n)
	case json.Number:
		s = n.String()
	case uint64:
		s = strconv.FormatUint(n, 10)
	case int:
		if n < 0 {
			return ""
		}
		s = strconv.Itoa(n)
	case int64:
		if n < 0 {
			return ""
		}
		s = strconv.FormatInt(n, 10)
	case float64:
		if n < 0 || n > 9007199254740991 || n != math.Trunc(n) {
			return ""
		}
		s = strconv.FormatFloat(n, 'f', 0, 64)
	default:
		return ""
	}
	if s == "" || len(s) > 20 {
		return ""
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return ""
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return ""
	}
	return strconv.FormatUint(n, 10)
}

func numeric(x any, min, max float64) (any, bool) {
	if _, text := x.(string); text {
		return nil, false
	}
	n, ok := v.Number(x)
	if !ok || n < min || n > max {
		return nil, false
	}
	return x, true
}

func (t *Tracker) fresh(key, at, source string) bool {
	if at == "" || t.ownerAt != "" && v.Time(at).Before(v.Time(t.ownerAt)) {
		return false
	}
	prev := t.stamps[key]
	if prev.at != "" && (v.Time(at).Before(v.Time(prev.at)) || at == prev.at && source == "capi" && prev.source == "journal") {
		return false
	}
	return true
}
func (t *Tracker) mark(key, at, source string, section M) {
	t.stamps[key] = stamp{at, source}
	if t.latest == "" || v.Time(at).After(v.Time(t.latest)) {
		t.latest = at
	}
	section["known"] = true
	if prev := v.Str(section["updatedAt"]); prev == "" || !v.Time(at).Before(v.Time(prev)) {
		section["updatedAt"], section["source"] = at, source
	}
}

func (t *Tracker) field(sectionName, name string, value any, at, source string, min, max float64) bool {
	x, ok := numeric(value, min, max)
	if !ok || !t.fresh(sectionName+"."+name, at, source) {
		return false
	}
	section := v.Obj(t.state[sectionName])
	old := section[name]
	fields := v.Obj(section["fields"])
	changed := !sameObservation(old, x) || v.At(fields, name, "updatedAt") != at || v.At(fields, name, "source") != source
	section[name] = x
	fields[name] = M{"updatedAt": at, "source": source}
	section["fields"] = fields
	t.mark(sectionName+"."+name, at, source, section)
	return changed
}

// Apply requires a known FID, supplied by Commander/LoadGame or context.commander.
// An event explicitly belonging to another FID is rejected. Only an identity
// event can switch an already established owner, and an older event cannot do so.
func (t *Tracker) Apply(event, context M) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	kind, at := v.Str(event["event"]), timestamp(event["timestamp"])
	explicit, scoped := clean(event["FID"]), clean(v.At(context, "commander", "fid"))
	if explicit != "" && scoped != "" && explicit != scoped {
		return false
	}
	owner := v.Text(explicit, scoped)
	if owner == "" {
		owner = t.owner
	}
	if owner == "" || at == "" {
		return false
	}
	identity := kind == "Commander" || kind == "LoadGame" || kind == "NewCommander" || kind == "ClearSavedGame"
	changed := false
	if t.owner != owner {
		if t.owner != "" && (!identity || t.latest != "" && v.Time(at).Before(v.Time(t.latest))) {
			return false
		}
		t.clear(owner)
		t.ownerAt = at
		t.latest = at
		changed = true
	}
	if identity && (t.ownerAt == "" || v.Time(at).After(v.Time(t.ownerAt))) {
		t.ownerAt = at
		if t.latest == "" || v.Time(at).After(v.Time(t.latest)) {
			t.latest = at
		}
	}
	if kind == "ClearSavedGame" || kind == "NewCommander" {
		if t.latest != "" && v.Time(at).Before(v.Time(t.latest)) {
			return changed
		}
		t.clear(owner)
		t.ownerAt, t.latest = at, at
		return true
	}
	switch kind {
	case "LoadGame":
		changed = t.field("finance", "credits", event["Credits"], at, "journal", 0, 1e18) || changed
		changed = t.field("finance", "loan", event["Loan"], at, "journal", 0, 1e18) || changed
		changed = t.currentShip(event, at, "journal", context) || changed
	case "Status":
		changed = t.field("finance", "credits", event["Balance"], at, "journal", 0, 1e18) || changed
	case "Loadout", "ShipyardSwap", "ShipyardNew", "SetUserShipName":
		changed = t.currentShip(event, at, "journal", context) || changed
	case "StoredShips":
		changed = t.storedShips(event, at) || changed
	case "StoredModules":
		changed = t.modules(event, at) || changed
	case "Statistics":
		changed = t.statistics(event, at) || changed
	case "Reputation":
		for field, key := range map[string]string{"Federation": "federation", "Empire": "empire", "Alliance": "alliance", "Independent": "independent"} {
			changed = t.field("reputation", key, event[field], at, "journal", -100, 100) || changed
		}
	case "Materials":
		changed = t.inventory(event, "materials", map[string]string{"Raw": "raw", "Manufactured": "manufactured", "Encoded": "encoded"}, at) || changed
	case "Backpack", "ShipLocker", "ShipLockerMaterials":
		container := "backpack"
		if kind != "Backpack" {
			container = "shipLocker"
		}
		changed = t.inventory(event, container, map[string]string{"Items": "items", "Components": "components", "Consumables": "consumables", "Data": "data"}, at) || changed
	}
	changed = t.observeShipPresence(event, context, at) || changed
	changed = t.observeShipLocation(event, context, at) || changed
	return t.invalidateChanges(event, at) || changed
}

func (t *Tracker) currentShip(event M, at, source string, context M) bool {
	if !t.fresh("currentShip", at, source) {
		return false
	}
	if previousAt := t.stamps["shipLocation"].at; previousAt != "" && v.Time(at).Before(v.Time(previousAt)) {
		return false
	}
	if raw := v.Text(event["Ship"], event["ShipType"]); raw != "" && ships.VehicleKind(raw) != "ship" {
		return false
	}
	fleet := v.Obj(t.state["fleet"])
	prev := v.Obj(fleet["currentShip"])
	next := ships.ApplyEvent(prev, event)
	if v.Str(next["shipType"]) == "" || ships.VehicleKind(v.Str(next["shipType"])) != "ship" {
		return false
	}
	id := exactID(v.First(next["shipId"], event["NewShipID"]))
	if id == "" {
		return false
	}
	next["id"], next["shipId"], next["known"], next["updatedAt"], next["source"] = id, id, true, at, source
	changed := !reflect.DeepEqual(prev, next)
	if previousID := v.Str(prev["shipId"]); previousID != "" && previousID != id {
		for _, key := range []string{"system", "systemAddress", "station", "marketId", "docked", "locationUpdatedAt", "locationSource", "locationKnown"} {
			delete(next, key)
		}
		t.shipPresence, t.aboardShipID = "unknown", ""
		delete(t.stamps, "shipLocation")
		t.stamps["shipPresence"] = stamp{at, source}
		finance := v.Obj(t.state["finance"])
		fields := v.Obj(finance["fields"])
		for _, name := range []string{"rebuy", "hullValue", "modulesValue", "shipValue"} {
			finance[name] = nil
			delete(fields, name)
			delete(t.stamps, "finance."+name)
		}
	}
	fleet["currentShip"] = next
	t.stamps["currentShip"] = stamp{at, source}
	if t.latest == "" || v.Time(at).After(v.Time(t.latest)) {
		t.latest = at
	}
	if event["event"] == "Loadout" {
		for field, name := range map[string]string{"HullValue": "hullValue", "ModulesValue": "modulesValue", "Rebuy": "rebuy"} {
			changed = t.field("finance", name, event[field], at, source, 0, 1e18) || changed
		}
		if h, ok := numeric(event["HullValue"], 0, 1e18); ok {
			if m, ok := numeric(event["ModulesValue"], 0, 1e18); ok {
				changed = t.field("finance", "shipValue", v.Num(h)+v.Num(m), at, source, 0, 1e18) || changed
			}
		}
	}
	return changed
}

func (t *Tracker) Snapshot() M {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return v.Copy(v.Merge(M{"commanderFID": t.owner}, t.state))
}

func (t *Tracker) ShipLockerSnapshot(fid string) M {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if fid == "" || t.owner != fid {
		return nil
	}
	return v.Copy(v.Obj(v.At(t.state, "inventory", "shipLocker")))
}

func sortedKeys(m M) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func label(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(s, "_", " "), "-", " ")), " ")
}
