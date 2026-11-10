//go:build windows

package nativeui

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/learning"
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type dispatchProbeController struct {
	platformController
	snapshots int
	failure   error
}

func (c *dispatchProbeController) Snapshot() M { c.snapshots++; return c.state }
func (c *dispatchProbeController) Do(path string, body M) (M, error) {
	c.path, c.last = path, v.Copy(body)
	return M{"ok": c.failure == nil}, c.failure
}

type scopedDispatchProbe struct {
	dispatchProbeController
	scopes []string
}

func (c *scopedDispatchProbe) SnapshotForUI(tab string) M {
	c.scopes = append(c.scopes, tab)
	return c.state
}

type engineeringDispatchProbe struct {
	scopedDispatchProbe
	input M
}

func (c *engineeringDispatchProbe) EngineeringRoadmap(input M) (M, error) {
	c.input = v.Copy(input)
	return M{"summary": M{"total": 38}}, nil
}

func TestEngineeringProgressionUsesItsReadOnlyProjection(t *testing.T) {
	backend := &engineeringDispatchProbe{}
	query := M{"engineerIDs": []any{"300100"}, "moduleKey": "fsd"}
	result, err := dispatchWeb(backend, "engineering-roadmap", query)
	if err != nil || v.Int(v.At(result, "summary", "total")) != 38 || !reflect.DeepEqual(query, backend.input) {
		t.Fatalf("wrong roadmap result or filters: %v, %v, %v", result, backend.input, err)
	}
	if backend.snapshots != 0 || len(backend.scopes) != 0 || backend.path != "" {
		t.Fatal("the roadmap built an unrelated snapshot or dispatched a mutation")
	}
	view, err := dispatchWeb(backend, "view", M{"tab": "engineering"})
	page, ok := view["page"].(p.Page)
	if err != nil || !ok || len(page.Modes) == 0 || page.Modes[0] != p.EngineeringProgression ||
		v.At(view, "context", "mode") != p.EngineeringProgression || len(v.Rows(view["actions"])) != 0 {
		t.Fatalf("wrong default engineering view: %v, %v", view, err)
	}
	if !reflect.DeepEqual(backend.scopes, []string{"engineering-roadmap"}) {
		t.Fatalf("unexpected snapshot scopes: %v", backend.scopes)
	}
}

func TestDirectWebCommandsDoNotBuildOrSnapshotDomains(t *testing.T) {
	for _, test := range []struct {
		op, path string
		input    M
		body     M
	}{
		{"navigation", "/api/navigation", M{"tabs": []string{"overview", "learning"}}, M{"tabs": []string{"overview", "learning"}}},
		{"settings", "/api/settings", M{"values": M{"overlayEnabled": false}}, M{"overlayEnabled": false}},
		{"desktop", "/api/desktop", M{"values": M{"sidebarCollapsed": true}}, M{"sidebarCollapsed": true}},
		{"theme-apply", "/api/theme", M{"values": M{"preset": "blue"}}, M{"settings": M{"preset": "blue"}}},
		{"learning", "/api/learning", M{"values": M{"action": "bookmark", "lessonId": "wmm.basics"}}, M{"action": "bookmark", "lessonId": "wmm.basics"}},
		{"calculator", "/api/carrier/calculator", M{"values": M{"jumpsPerWeek": 4}}, M{"jumpsPerWeek": 4}},
		{"select-colony", "/api/colonization", M{"id": "synthetic-colony"}, M{"action": "select-project", "id": "synthetic-colony"}},
	} {
		t.Run(test.op, func(t *testing.T) {
			backend := &dispatchProbeController{}
			result, err := dispatchWeb(backend, test.op, test.input)
			if err != nil || !v.Bool(result["ok"]) || backend.snapshots != 0 || backend.path != test.path || !reflect.DeepEqual(backend.last, v.Copy(test.body)) {
				t.Fatalf("direct command changed contract: result=%v, err=%v, snapshots=%d, path=%s, body=%v", result, err, backend.snapshots, backend.path, backend.last)
			}
			backend.failure = errors.New("synthetic backend validation")
			if _, err := dispatchWeb(backend, test.op, test.input); !errors.Is(err, backend.failure) || backend.snapshots != 0 {
				t.Fatal("backend validation failure was lost or caused a snapshot", err)
			}
		})
	}
	for _, request := range []struct {
		op    string
		input M
	}{
		{"settings", M{"tab": "../../settings", "values": M{"overlayEnabled": false}}},
		{"unknown-operation", M{}},
		{"catalog", M{"tab": "missing"}},
	} {
		backend := &dispatchProbeController{}
		if _, err := dispatchWeb(backend, request.op, request.input); err == nil || backend.snapshots != 0 || backend.path != "" {
			t.Fatal("invalid request reached a domain operation", request, err)
		}
	}
}

func TestWebReadsPreferScopedSnapshotsAndKeepLegacyControllers(t *testing.T) {
	state := M{"commander": M{"name": "Synthetic"}, "engineering": M{"catalog": M{"marker": "synthetic-catalog"}}}
	backend := &scopedDispatchProbe{dispatchProbeController: dispatchProbeController{platformController: platformController{state: state}}}
	for _, request := range []struct {
		op, tab, scope string
	}{
		{"boot", "overview", "overview"},
		{"view", "learning", "learning"},
		{"action", "overview", "overview"},
		{"catalog", "overview", "engineering"},
	} {
		_, err := dispatchWeb(backend, request.op, M{"tab": request.tab, "actionId": "not-an-action"})
		if (err != nil && request.op != "action") || len(backend.scopes) == 0 || backend.scopes[len(backend.scopes)-1] != request.scope || backend.snapshots != 0 {
			t.Fatal("wrong snapshot scope", request, backend.scopes, backend.snapshots, err)
		}
	}
	legacy := &dispatchProbeController{platformController: platformController{state: state}}
	if _, err := dispatchWeb(legacy, "boot", M{}); err != nil || legacy.snapshots != 1 {
		t.Fatal("legacy controller fallback was lost", err, legacy.snapshots)
	}
}

func TestWebStateCopiesAndRedactsTypedNestedJSONValues(t *testing.T) {
	type namedBytes []byte
	type typedRow struct {
		Key    string `json:"api-key"`
		Nested M      `json:"nested"`
	}
	pointed := M{"password": "secret-pointer", "safe": "pointer-value"}
	state := M{
		"commander": M{"name": "Synthetic"},
		"integrations": M{
			"apiKeyConfigured": true,
			"typedRows":        []M{{"Refresh_Token": "secret-row", "safe": M{"name": "row"}}},
			"nestedSlices":     [][]M{{{"Code-Verifier": "secret-nested", "safe": "nested-value"}}},
			"arrays":           [1]any{[]any{M{"client_secret": "secret-array", "safe": "array-value"}}},
			"typedMaps":        map[string]map[string]string{"row": {"Access-Token": "secret-map", "safe": "map-value"}},
			"typedStruct":      typedRow{Key: "secret-struct", Nested: M{"password": "secret-field", "safe": "struct-value"}},
			"pointer":          &pointed,
			"tags":             []string{"one", "two"},
			"bytes":            []byte{1, 2, 3},
			"numbers":          []uint64{18446744073709551615},
			"address":          json.Number("18446744073709551615"),
			"rawJSON":          json.RawMessage(`{"access_token":"secret-raw","safe":"raw-value"}`),
			"namedBytes":       namedBytes{1, 2, 3},
			"nullElement":      []M{nil},
			"emptyRows":        []M{},
			"nilRows":          []M(nil),
			"emptyStrings":     []string{},
			"nilStrings":       []string(nil),
		},
	}
	original, _ := json.Marshal(state)
	result := webState(state)
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(string(encoded), "secret-") || !strings.Contains(string(encoded), `"apiKeyConfigured":true`) || !strings.Contains(string(encoded), "18446744073709551615") {
		t.Fatal("typed or nested payload leaked credentials or lost precision", string(encoded), err)
	}
	for _, shape := range []string{`"emptyRows":[]`, `"nilRows":null`, `"emptyStrings":[]`, `"nilStrings":null`, `"bytes":"AQID"`, `"namedBytes":"AQID"`, `"nullElement":[null]`} {
		if !strings.Contains(string(encoded), shape) {
			t.Fatal("JSON shape changed", shape, string(encoded))
		}
	}
	integration := v.Obj(result["integrations"])
	if address, ok := integration["address"].(json.Number); !ok || address.String() != "18446744073709551615" {
		t.Fatal("JSON number type or identifier precision changed", integration["address"])
	}
	v.Obj(result["commander"])["name"] = "changed"
	v.Obj(v.Rows(integration["typedRows"])[0]["safe"])["name"] = "changed"
	v.Rows(v.Arr(integration["nestedSlices"])[0])[0]["safe"] = "changed"
	v.Obj(v.Arr(v.Arr(integration["arrays"])[0])[0])["safe"] = "changed"
	v.Obj(v.Obj(integration["typedMaps"])["row"])["safe"] = "changed"
	v.Obj(v.Obj(integration["typedStruct"])["nested"])["safe"] = "changed"
	v.Obj(integration["pointer"])["safe"] = "changed"
	integration["tags"].([]string)[0] = "changed"
	integration["bytes"].([]byte)[0] = 9
	v.Arr(integration["numbers"])[0] = 0
	v.Obj(integration["rawJSON"])["safe"] = "changed"
	integration["namedBytes"].(namedBytes)[0] = 9
	after, _ := json.Marshal(state)
	if string(original) != string(after) {
		t.Fatal("render-state copy retained a mutable alias to engine state")
	}
}

func TestWebRedactionSharedNormalizerIsConcurrentAndIndependent(t *testing.T) {
	input := M{"rows": []M{{"API-KEY": "secret", "keep": []string{"original"}}}}
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 20 {
				out := redactWebSecrets(input)
				row := v.Rows(out["rows"])[0]
				if _, leaked := row["API-KEY"]; leaked {
					t.Error("shared normalizer leaked a credential")
				}
				row["keep"].([]string)[0] = "independent"
			}
		}()
	}
	workers.Wait()
	if v.Rows(input["rows"])[0]["keep"].([]string)[0] != "original" {
		t.Fatal("concurrent copies shared mutable state")
	}
}

func TestWebViewReturnsCanonicalContextAndRowSpecificActionsInOneSnapshot(t *testing.T) {
	state := M{"objectives": []M{
		{"id": "first-goal", "title": "Первая цель", "kind": "other", "status": "active"},
		{"id": "second-goal", "title": "Вторая цель", "kind": "trade", "status": "active"},
	}}
	for _, test := range []struct{ selected, expected string }{{"", "first-goal"}, {"stale-goal", "first-goal"}, {"second-goal", "second-goal"}} {
		backend := &dispatchProbeController{platformController: platformController{state: state}}
		result, err := dispatchWeb(backend, "view", M{"tab": "overview", "selected": test.selected})
		if err != nil || backend.snapshots != 1 || v.At(result, "context", "mode") != "Обзор" || v.At(result, "context", "selected") != test.expected {
			t.Fatalf("wrong initial context: %v err=%v snapshots=%d", result["context"], err, backend.snapshots)
		}
		page := result["page"].(p.Page)
		var row p.Row
		for _, candidate := range page.Rows {
			if candidate.ID == test.expected {
				row = candidate
			}
		}
		expected := webActions(p.CoreActions("overview", "Обзор", state, row.Data, M{}))
		if !reflect.DeepEqual(result["actions"], expected) {
			t.Fatal("actions do not match the returned canonical selection", test)
		}
	}
	backend := &dispatchProbeController{platformController: platformController{state: state}}
	result, err := dispatchWeb(backend, "view", M{"tab": "overview", "mode": "Корабль и груз"})
	if err != nil || v.At(result, "context", "mode") != "Корабль и груз" || v.At(result, "context", "selected") != "" {
		t.Fatal("explicit mode or empty table changed", result["context"], err)
	}
	for _, selected := range []string{"", "stale-goal"} {
		backend := &dispatchProbeController{platformController: platformController{state: state}}
		if _, err := dispatchWeb(backend, "action", M{"tab": "overview", "mode": "Обзор", "selected": selected, "actionId": "goal.complete"}); err == nil || backend.path != "" {
			t.Fatal("action fell back to the first record without an explicit valid selection", selected, err, backend.path)
		}
	}
	backend = &dispatchProbeController{platformController: platformController{state: state}}
	if _, err := dispatchWeb(backend, "action", M{"tab": "overview", "mode": "Обзор", "selected": "second-goal", "actionId": "goal.complete"}); err != nil || backend.last["id"] != "second-goal" {
		t.Fatal("explicit action selected a different record", backend.last, err)
	}
}

func TestWebViewKeepsTheExplicitAcademyReadingLesson(t *testing.T) {
	library := learning.New(nil)
	state := M{"learning": library.Snapshot()}
	backend := &dispatchProbeController{platformController: platformController{state: state}}
	result, err := dispatchWeb(backend, "view", M{"tab": "learning", "selected": "wmm.practice"})
	if err != nil || v.At(result, "context", "mode") != "Все курсы" || v.At(result, "context", "selected") != "wmm.practice" {
		t.Fatal("course catalogue replaced the explicit reader lesson", result["context"], err)
	}
	var lesson M
	for _, row := range v.Rows(v.At(state, "learning", "lessons")) {
		if row["id"] == "wmm.practice" {
			lesson = row
		}
	}
	if !reflect.DeepEqual(result["actions"], webActions(p.CoreActions("learning", "Все курсы", state, lesson, M{}))) {
		t.Fatal("reader actions target another lesson")
	}
}
