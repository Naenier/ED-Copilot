package carrier

import v "github.com/Naenier/ED-Copilot/internal/value"

// ClipboardProgress is a small projection; journal processing does not copy
// the entire Spansh route to determine its next destination.
func (p *RoutePlanner) ClipboardProgress() M {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := M{"completedJumps": v.At(p.state, "progress", "completedJumps")}
	route := v.Obj(p.state["route"])
	stops := v.Rows(route["stops"])
	if raw := route["activeIndex"]; raw != nil {
		index := v.Int(raw)
		if index >= 0 && index < len(stops) && !v.Bool(stops[index]["visited"]) {
			result["system"] = stops[index]["name"]
		}
	}
	return result
}
