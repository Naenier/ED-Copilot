//go:build windows

package nativeui

import (
	"encoding/json"
	"strings"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestOverlayColonyLiveSummaryKeepsSelectionAndOmitsQueues(t *testing.T) {
	state := M{"colonization": M{"selectedProjectId": "B", "activeProject": M{"id": "B", "supplies": []M{{"name": "SECRET-QUEUE"}}}, "projects": []M{{"id": "A", "name": "Alpha", "targetSystem": "System A", "status": "building", "objects": []M{{"name": "SECRET-OBJECT"}}}, {"id": "B", "name": "Beta", "status": "complete"}}}}
	got := overlayColonyUpdate(state)
	if got["selectedProjectId"] != "B" || v.At(got, "activeProject", "id") != "B" || len(v.Rows(got["projects"])) != 2 {
		t.Fatal(got)
	}
	encoded, err := json.Marshal(got)
	if err != nil || strings.Contains(string(encoded), "SECRET-") || strings.Contains(string(encoded), "supplies") || strings.Contains(string(encoded), "objects") {
		t.Fatal(string(encoded), err)
	}
	v.Rows(got["projects"])[0]["name"] = "Changed"
	if v.Rows(v.At(state, "colonization", "projects"))[0]["name"] != "Alpha" {
		t.Fatal("live summary aliases original projects")
	}
	if len(v.Rows(overlayColonyUpdate(M{})["projects"])) != 0 {
		t.Fatal("empty selector is not an explicit list")
	}
}
