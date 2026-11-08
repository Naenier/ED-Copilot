package ranks

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/reference"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const firstAt = "2026-11-08T12:00:00Z"

func row(t *testing.T, tracker *Tracker, id string) M {
	t.Helper()
	for _, item := range v.Rows(tracker.SnapshotForLanguage("en")["disciplines"]) {
		if item["id"] == id {
			return item
		}
	}
	t.Fatal("missing rank discipline", id)
	return nil
}
func apply(tracker *Tracker, kind, at string, values M) {
	tracker.Apply(v.Merge(M{"event": kind, "timestamp": at}, values), M{"commander": M{"fid": "FSYNTHETIC"}})
}

func TestEightDisciplinesKeepMissingRankAndProgressUnknown(t *testing.T) {
	x := New(nil)
	state := x.SnapshotForLanguage("en")
	if len(v.Rows(state["disciplines"])) != 8 || v.Int(v.At(state, "summary", "known")) != 0 {
		t.Fatal("new tracker invented an initial rank")
	}
	for _, item := range v.Rows(state["disciplines"]) {
		if item["rankIndex"] != nil || item["rankName"] != nil || item["progressPercent"] != nil || item["nextRankIndex"] != nil || item["maxed"] != false {
			t.Fatal("unknown data became a zero or a next-rank promise", item)
		}
	}
	apply(x, "Rank", firstAt, M{"Combat": 0, "Federation": 0})
	apply(x, "Progress", firstAt, M{"Combat": 0})
	combat, federal, trade := row(t, x, "combat"), row(t, x, "federation"), row(t, x, "trade")
	if v.Int(combat["rankIndex"]) != 0 || combat["rankName"] != "Harmless" || combat["progressPercent"] == nil || v.Num(combat["progressPercent"]) != 0 || federal["rankName"] != "None" || federal["progressPercent"] != nil || trade["rankIndex"] != nil {
		t.Fatal("known zero, missing percentage and unknown career were conflated")
	}
}

func TestStartupObservationIsNotAPromotionAndPercentDoesNotPromote(t *testing.T) {
	x := New(nil)
	apply(x, "Rank", firstAt, M{"Federation": 3})
	apply(x, "Progress", "2026-11-08T12:01:00Z", M{"Federation": 100})
	federal := row(t, x, "federation")
	if federal["rankName"] != "Midshipman" || federal["nextRankName"] != "Petty Officer" || v.Num(federal["progressPercent"]) != 100 {
		t.Fatal("100% was converted into an unconfirmed naval promotion", federal)
	}
	for _, event := range v.Rows(x.Snapshot()["history"]) {
		if event["kind"] == "promotion" {
			t.Fatal("startup rank fabricated an earned-at promotion date")
		}
	}
	apply(x, "Promotion", "2026-11-08T12:02:00Z", M{"Federation": 4})
	federal = row(t, x, "federation")
	if federal["rankName"] != "Petty Officer" || federal["progressPercent"] != nil || federal["rankUpdatedAt"] != "2026-11-08T12:02:00Z" {
		t.Fatal("promotion retained old-rank percentage or invented zero")
	}
	history := v.Rows(x.Snapshot()["history"])
	if history[0]["kind"] != "promotion" || v.Int(history[0]["fromRankIndex"]) != 3 || v.Int(history[0]["rankIndex"]) != 4 {
		t.Fatal("actual promotion lost its observed transition", history)
	}
}

func TestElitePrestigeAndNavalBoundariesAreDisciplineSpecific(t *testing.T) {
	x := New(nil)
	apply(x, "Rank", firstAt, M{"Combat": 13, "Trade": 8, "Explore": 9, "Soldier": 13, "Exobiologist": 12, "CQC": 13, "Federation": 14, "Empire": 12})
	for _, id := range []string{"combat", "soldier", "cqc"} {
		item := row(t, x, id)
		if item["rankName"] != "Elite V" || item["maxed"] != true || item["nextRankIndex"] != nil || len(v.Rows(item["timeline"])) != 14 {
			t.Fatal("career prestige boundary is incorrect", id, item)
		}
	}
	if row(t, x, "explore")["rankName"] != "Elite I" || row(t, x, "federation")["rankName"] != "Admiral" || row(t, x, "empire")["nextRankName"] != "Prince" {
		t.Fatal("career prestige and naval title ladders crossed")
	}
	before := x.ExportSettings()
	apply(x, "Rank", "2026-11-08T13:00:00Z", M{"CQC": 14, "Combat": 14, "Empire": 15})
	if !reflect.DeepEqual(before, x.ExportSettings()) {
		t.Fatal("unsupported rank levels were accepted")
	}
}

func TestStaleEventsAndMalformedNumbersCannotRewriteKnownObservations(t *testing.T) {
	x := New(nil)
	apply(x, "Rank", firstAt, M{"Combat": 4})
	apply(x, "Progress", firstAt, M{"Combat": 25})
	before := x.ExportSettings()
	for _, event := range []M{
		{"event": "Rank", "timestamp": "2026-11-07T12:00:00Z", "Combat": 8},
		{"event": "Progress", "timestamp": "2026-11-07T12:00:00Z", "Combat": 90},
		{"event": "Progress", "Combat": 90}, {"event": "Rank", "Combat": 8},
		{"event": "Rank", "timestamp": "2026-11-09T12:00:00Z", "Combat": 3},
		{"event": "Rank", "Combat": 4.5}, {"event": "Rank", "Combat": "4"}, {"event": "Progress", "Combat": true},
		{"event": "Progress", "Combat": -1}, {"event": "Progress", "Combat": 101}, {"event": "Progress", "Combat": math.Inf(1)}, {"event": "Progress", "Combat": math.NaN()},
	} {
		x.Apply(event, nil)
		if !reflect.DeepEqual(before, x.ExportSettings()) {
			t.Fatal("bad input altered a valid rank", event)
		}
	}
}

func TestProgressBeforeRankAndMissingTimestampRemainHonest(t *testing.T) {
	x := New(nil)
	apply(x, "Progress", firstAt, M{"Exobiologist": 0})
	if item := row(t, x, "exobiologist"); item["rankIndex"] != nil || item["progressPercent"] == nil {
		t.Fatal("early progress invented a rank or lost a known zero")
	}
	apply(x, "Rank", firstAt, M{"Exobiologist": 3})
	if row(t, x, "exobiologist")["progressPercent"] == nil {
		t.Fatal("same-startup percentage was discarded")
	}
	apply(x, "Rank", "", M{"Empire": 7})
	if item := row(t, x, "empire"); item["rankName"] != "Baron" || item["rankUpdatedAt"] != "" {
		t.Fatal("missing historical timestamp became a fabricated date")
	}
}

func TestCommanderSwitchAndActualSaveResetNeverLeakPreviousRanks(t *testing.T) {
	x := New(nil)
	apply(x, "Rank", firstAt, M{"Trade": 13})
	x.Apply(M{"event": "Commander", "FID": "FSECOND"}, nil)
	if row(t, x, "trade")["rankIndex"] != nil || len(v.Rows(x.Snapshot()["history"])) != 0 {
		t.Fatal("another commander inherited Elite V")
	}
	x.Apply(M{"event": "Rank", "timestamp": firstAt, "Trade": 2}, M{"commander": M{"fid": "FSECOND"}})
	x.Apply(M{"event": "ClearSavedGame", "timestamp": "2026-11-09T12:00:00Z", "FID": "FSECOND"}, nil)
	if row(t, x, "trade")["rankIndex"] != nil {
		t.Fatal("actual clear-save retained previous rank")
	}
}

func TestReplayKeepsConfirmedHistoryAndRestartKeepsLocalizedProgress(t *testing.T) {
	x := New(nil)
	apply(x, "Rank", firstAt, M{"Trade": 7})
	apply(x, "Promotion", "2026-11-08T12:01:00Z", M{"Trade": 8})
	apply(x, "Progress", "2026-11-08T12:02:00Z", M{"Trade": 7})
	before := x.ExportSettings()
	x.Reset()
	apply(x, "NewCommander", "2020-01-01T00:00:00Z", M{"FID": "FSYNTHETIC"})
	apply(x, "Rank", firstAt, M{"Trade": 7})
	apply(x, "Progress", firstAt, M{"Trade": 99})
	if !reflect.DeepEqual(before, x.ExportSettings()) {
		t.Fatal("rescan erased current progress or historical promotion")
	}
	restored := New(before)
	if !reflect.DeepEqual(x.SnapshotForLanguage("en"), restored.SnapshotForLanguage("en")) || row(t, restored, "trade")["rankName"] != "Elite" {
		t.Fatal("rank history did not survive a restart")
	}
	if v.Str(v.Rows(restored.Snapshot()["disciplines"])[1]["rankName"]) != "Элита" {
		t.Fatal("Russian display was not independent of stored progress")
	}
}

func TestCAPIRequiresIdentityAndCannotDowngradeOrReplaceNewerJournal(t *testing.T) {
	x := New(nil)
	profile := M{"commander": M{"name": "Synthetic Pilot", "rank": M{"combat": 8, "federation": 4}}}
	context := M{"commander": M{"fid": "FSYNTHETIC", "name": "Synthetic Pilot"}, "observedAt": firstAt}
	if x.ApplyCAPI(profile, context) {
		t.Fatal("unverified CAPI identity was trusted")
	}
	context["identityVerified"] = true
	if !x.ApplyCAPI(profile, context) || row(t, x, "combat")["rankName"] != "Elite" || row(t, x, "combat")["progressPercent"] != nil {
		t.Fatal("verified CAPI did not supplement rank without inventing a percentage")
	}
	apply(x, "Rank", "2026-11-08T12:01:00Z", M{"Combat": 9})
	before := x.ExportSettings()
	x.ApplyCAPI(profile, context)
	context["commander"] = M{"fid": "FOTHER", "name": "Synthetic Pilot"}
	x.ApplyCAPI(profile, context)
	if !reflect.DeepEqual(before, x.ExportSettings()) {
		t.Fatal("stale or foreign CAPI replaced current journal data")
	}
}

func TestRewardMilestonesAndCQCPrestigeAreNotConflated(t *testing.T) {
	checks := map[string]map[int][]string{"federation": {3: {"federal-dropship"}, 4: {"sol"}, 5: {"federal-assault-ship", "vega", "beta-hydri"}, 6: {"plx695"}, 7: {"federal-gunship", "ross128"}, 8: {"exbeur"}, 10: {"hors"}, 12: {"federal-corvette"}}, "empire": {3: {"imperial-courier"}, 4: {"achenar"}, 7: {"imperial-clipper", "summerland"}, 10: {"facece"}, 12: {"imperial-cutter"}}}
	for id, milestones := range checks {
		for rank, ids := range milestones {
			got := rewardsFor(id, rank, "en")
			if len(got) != len(ids) {
				t.Fatal("missing rank-specific unlocks", id, rank, got)
			}
			for i, key := range ids {
				if got[i]["id"] != key {
					t.Fatal("wrong milestone", id, rank, got)
				}
			}
		}
	}
	for _, id := range []string{"combat", "trade", "explore", "soldier", "exobiologist"} {
		found := false
		for _, r := range rewardsFor(id, 8, "en") {
			found = found || r["id"] == "founders-world"
		}
		if !found || len(rewardsFor(id, 9, "en")) != 0 {
			t.Fatal("base Elite reward missing or prestige reward fabricated", id)
		}
	}
	for _, reward := range rewardsFor("cqc", 8, "en") {
		if reward["kind"] == "permit" {
			t.Fatal("CQC career rank was converted to an Arena Prestige or Shinrarta permit")
		}
	}
	if !strings.Contains(v.Str(v.Rows(guideFor("cqc", "en")["sections"])[1]["paragraphs"]), "level 50") {
		t.Fatal("Arena Prestige requirement not explained")
	}
}

func TestShipUnlockLinksResolveToTheActualReferenceCatalogue(t *testing.T) {
	for _, id := range []string{"federation", "empire"} {
		for rank := 0; rank <= 14; rank++ {
			for _, reward := range rewardsFor(id, rank, "en") {
				if reward["kind"] != "ship" {
					continue
				}
				entry, exists := reference.Catalog().Entry(v.Str(reward["entryId"]))
				if !exists || entry.Category != "ships" || !strings.Contains(v.Str(reward["title"]), entry.Title) {
					t.Fatal("rank unlock points at an unavailable or unrelated ship", id, rank, reward)
				}
			}
		}
	}
}

func TestUsefulPermitNotesAreBilingualAndKeepDestinationIdentities(t *testing.T) {
	for _, language := range []string{"ru", "en"} {
		for _, check := range []struct {
			discipline string
			rank       int
			id, place  string
		}{
			{"federation", 4, "sol", "Daedalus"},
			{"federation", 5, "vega", "Taylor City"},
			{"federation", 7, "ross128", "Warren Prison Mine"},
			{"empire", 4, "achenar", "Tiana Fortune"},
			{"empire", 10, "facece", ""},
		} {
			found := false
			for _, reward := range rewardsFor(check.discipline, check.rank, language) {
				if reward["id"] != check.id {
					continue
				}
				found = true
				note := v.Str(reward["note"])
				if note == "" || !strings.Contains(note, check.place) {
					t.Fatal("permit lacks practical destination information", language, reward)
				}
				if language == "en" && strings.ContainsAny(note, "АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯабвгдеёжзийклмнопрстуфхцчшщъыьэюя") {
					t.Fatal("English permit note contains untranslated prose", reward)
				}
			}
			if !found {
				t.Fatal("permit milestone missing", check.id)
			}
		}
	}
}

func TestHistoriesAreBoundedAndSnapshotsCannotMutateTracker(t *testing.T) {
	x := New(nil)
	apply(x, "Rank", firstAt, M{"Combat": 3})
	for i := 0; i < 600; i++ {
		apply(x, "Progress", v.ISO(v.Time(firstAt).Add(time.Duration(i)*time.Minute)), M{"Combat": i % 100})
	}
	if len(v.Rows(x.Snapshot()["history"])) != historyLimit {
		t.Fatal("unbounded rank history")
	}
	before := x.ExportSettings()
	state := x.SnapshotForLanguage("en")
	v.Rows(state["disciplines"])[0]["rankName"] = "changed"
	v.Rows(v.Rows(state["disciplines"])[0]["timeline"])[0]["rankName"] = "changed"
	v.Rows(state["history"])[0]["kind"] = "changed"
	v.Obj(v.At(before, "observations", "combat"))["rankIndex"] = 13
	if row(t, x, "combat")["rankName"] != "Competent" || v.Rows(x.Snapshot()["history"])[0]["kind"] == "changed" {
		t.Fatal("mutable view or export escaped into rank state")
	}
}

func TestConcurrentJournalAndBilingualSnapshotAccess(t *testing.T) {
	x := New(nil)
	apply(x, "Rank", firstAt, M{"Trade": 8})
	var group sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			for step := 0; step < 20; step++ {
				if worker == 0 {
					apply(x, "Progress", fmt.Sprintf("2026-11-08T12:%02d:00Z", step), M{"Trade": step})
				} else {
					language := []string{"ru", "en"}[worker%2]
					if state := x.SnapshotForLanguage(language); state["language"] != language || len(v.Rows(state["disciplines"])) != 8 {
						t.Error("concurrent locale state crossed readers")
					}
					_ = x.ExportSettings()
				}
			}
		}(worker)
	}
	group.Wait()
}
