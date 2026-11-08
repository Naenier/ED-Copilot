package app

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/inara"
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestScopedUISnapshotsPreserveAllPagesAndActionForms(t *testing.T) {
	engine, _, _ := fixtureEngine(t, nil)
	doOK(t, engine, "/api/objectives", M{"action": "add", "title": "Synthetic Goal", "system": "Synthetic Origin"})
	doOK(t, engine, "/api/colonization", M{"action": "create-project", "targetSystem": "Synthetic Colony", "originSystem": "Synthetic Origin"})
	project := v.Obj(v.At(engine.Snapshot(), "colonization", "activeProject"))
	doOK(t, engine, "/api/colonization", M{"action": "add-building", "projectId": project["id"], "typeId": "coriolis", "isPrimary": true})
	engine.Apply(testEvent("Scan", M{"BodyID": 2, "BodyName": "Synthetic Origin 2", "Landable": true, "Materials": []M{{"Name": "Polonium", "Percent": 1}}}), M{})
	full := engine.Snapshot()
	for _, tab := range p.Tabs {
		t.Run(tab.ID, func(t *testing.T) {
			scoped := engine.SnapshotForUI(tab.ID)
			if scoped["world"] != nil || v.At(scoped, "mining", "world") != nil {
				t.Fatal("UI snapshot includes explored world")
			}
			page := p.Build(tab.ID, "", "", full)
			modes := page.Modes
			if len(modes) == 0 {
				modes = []string{""}
			}
			for _, mode := range modes {
				want, got := p.Build(tab.ID, mode, "", full), p.Build(tab.ID, mode, "", scoped)
				normalizeScopedPage(&got)
				normalizeScopedPage(&want)
				if tab.ID == "encyclopedia" && mode == "Товары" {
					for _, page := range []*p.Page{&got, &want} {
						sort.SliceStable(page.Rows, func(i, j int) bool {
							return fmt.Sprint(page.Rows[i].Cells[0], page.Rows[i].ID) < fmt.Sprint(page.Rows[j].Cells[0], page.Rows[j].ID)
						})
					}
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("page differs for %q: %s", mode, scopedDifference(reflect.ValueOf(got), reflect.ValueOf(want), "page"))
					continue
				}
				selections := []M{nil}
				if len(want.Rows) > 0 {
					selections = append(selections, want.Rows[0].Data, want.Rows[len(want.Rows)-1].Data)
				}
				for _, selected := range selections {
					wantActions, gotActions := p.CoreActions(tab.ID, mode, full, selected, nil), p.CoreActions(tab.ID, mode, scoped, selected, nil)
					for i := range wantActions {
						wantActions[i].Transform = nil
						normalizeScopedChoices(&wantActions[i])
					}
					for i := range gotActions {
						gotActions[i].Transform = nil
						normalizeScopedChoices(&gotActions[i])
					}
					if !reflect.DeepEqual(gotActions, wantActions) {
						t.Errorf("action/form metadata differs for %q selected=%v: %s", mode, selected["id"], scopedDifference(reflect.ValueOf(gotActions), reflect.ValueOf(wantActions), "actions"))
					}
				}
			}
		})
	}
}

func normalizeScopedPage(page *p.Page) {
	for _, row := range page.Rows {
		if capabilities := v.Rows(row.Data["capabilities"]); capabilities != nil {
			sort.SliceStable(capabilities, func(i, j int) bool {
				return fmt.Sprint(capabilities[i]["moduleKey"], capabilities[i]["blueprintKey"]) < fmt.Sprint(capabilities[j]["moduleKey"], capabilities[j]["blueprintKey"])
			})
			row.Data["capabilities"] = capabilities
		}
	}
	for index := range page.Rows {
		if page.Rows[index].Data["capabilities"] != nil {
			lines := strings.Split(page.Rows[index].Details, "\r\n")
			sort.Strings(lines)
			page.Rows[index].Details = strings.Join(lines, "\r\n")
		}
	}
}

func normalizeScopedChoices(action *p.Action) {
	for index := range action.Fields {
		sort.SliceStable(action.Fields[index].Choices, func(i, j int) bool {
			a, b := action.Fields[index].Choices[i], action.Fields[index].Choices[j]
			if a.Label != b.Label {
				return a.Label < b.Label
			}
			return a.Value < b.Value
		})
	}
}

func scopedDifference(a, b reflect.Value, path string) string {
	if !a.IsValid() || !b.IsValid() {
		return fmt.Sprint(path, " missing value")
	}
	if reflect.DeepEqual(a.Interface(), b.Interface()) {
		return ""
	}
	if a.Type() != b.Type() {
		return fmt.Sprintf("%s type %s != %s", path, a.Type(), b.Type())
	}
	switch a.Kind() {
	case reflect.Interface, reflect.Pointer:
		return scopedDifference(a.Elem(), b.Elem(), path)
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			if diff := scopedDifference(a.Field(i), b.Field(i), path+"."+a.Type().Field(i).Name); diff != "" {
				return diff
			}
		}
	case reflect.Slice, reflect.Array:
		if a.Len() != b.Len() {
			return fmt.Sprintf("%s length %d != %d", path, a.Len(), b.Len())
		}
		for i := 0; i < a.Len(); i++ {
			if diff := scopedDifference(a.Index(i), b.Index(i), fmt.Sprintf("%s[%d]", path, i)); diff != "" {
				return diff
			}
		}
	case reflect.Map:
		if a.Len() != b.Len() {
			return fmt.Sprintf("%s keys %d != %d", path, a.Len(), b.Len())
		}
		keys := a.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface()) })
		for _, key := range keys {
			if diff := scopedDifference(a.MapIndex(key), b.MapIndex(key), path+"."+fmt.Sprint(key.Interface())); diff != "" {
				return diff
			}
		}
	}
	return fmt.Sprintf("%s got=%.160v want=%.160v", path, a.Interface(), b.Interface())
}

func TestScopedHUDPreservesEveryActivityAndGlobalColonySelector(t *testing.T) {
	engine, _, _ := fixtureEngine(t, nil)
	engine.Apply(testEvent("Scan", M{"BodyID": 2, "BodyName": "Synthetic Origin 2", "Landable": true, "Materials": []M{{"Name": "Polonium", "Percent": 1}}}), M{})
	doOK(t, engine, "/api/colonization", M{"action": "create-project", "targetSystem": "Synthetic Colony"})
	now := time.Now()
	for _, activity := range p.Activities {
		doOK(t, engine, "/api/settings", M{"activity": activity.Value})
		want, got := p.BuildHUD(engine.Snapshot(), now), p.BuildHUD(engine.SnapshotForHUD(), now)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("HUD changed for %s: got=%+v want=%+v", activity.Value, got, want)
		}
	}
	doOK(t, engine, "/api/settings", M{"activity": "colonization"})
	for _, tab := range []string{"settings", "carrier-calculator", "learning", "account"} {
		state := engine.SnapshotForUI(tab)
		if len(v.Rows(v.At(state, "colonization", "projects"))) != 1 || v.At(state, "colonization", "selectedProjectId") == nil {
			t.Fatal("global colony selector missing on", tab)
		}
	}
}

func TestScopedSyncPreservesInaraPayloadWithoutWorldOrLessons(t *testing.T) {
	engine, _, _ := fixtureEngine(t, nil)
	stamp := v.ISO(time.Now().Add(-time.Second))
	engine.Apply(M{"event": "Materials", "timestamp": stamp, "Raw": []M{{"Name": "carbon", "Count": 12}}, "Manufactured": []M{}, "Encoded": []M{}}, M{})
	full, scoped := engine.Snapshot(), engine.SnapshotForSync()
	context := M{"gameVersion": "4.4.2", "live": true, "loan": 0}
	want, got := inara.BuildSnapshotEvents(full, context, time.Now()), inara.BuildSnapshotEvents(scoped, context, time.Now())
	if !reflect.DeepEqual(got, want) {
		t.Fatal("scoped synchronization changed observations", got, want)
	}
	for _, key := range []string{"world", "mining", "targets", "learning", "carrierCalculator", "colonization"} {
		if scoped[key] != nil {
			t.Fatal("sync computed unrelated domain", key)
		}
	}
	if v.At(scoped, "engineering", "catalog") != nil {
		t.Fatal("sync includes full engineering catalogue")
	}
}
