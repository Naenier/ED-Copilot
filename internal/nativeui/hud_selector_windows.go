//go:build windows

package nativeui

import v "github.com/Naenier/ED-Copilot/internal/value"

// Only the selector summary crosses a live update. Construction queues and
// bills remain in their scoped page response, rather than every status tick.
func overlayColonyUpdate(state M) M {
	colonization := v.Obj(state["colonization"])
	projects := []M{}
	for _, project := range v.Rows(colonization["projects"]) {
		if v.Str(project["id"]) == "" {
			continue
		}
		projects = append(projects, M{"id": project["id"], "name": project["name"], "targetSystem": project["targetSystem"], "status": project["status"]})
	}
	return M{"projects": projects, "selectedProjectId": colonization["selectedProjectId"], "activeProject": M{"id": v.At(colonization, "activeProject", "id")}}
}
