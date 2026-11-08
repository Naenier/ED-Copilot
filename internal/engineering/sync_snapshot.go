package engineering

import (
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// InventorySnapshot contains observed inventories and engineer ranks used by
// synchronization. It deliberately avoids recipes, trade offers and the large
// engineer capability catalogue, which are only needed by the engineering UI.
func (p *Planner) InventorySnapshot() M {
	p.mu.RLock()
	defer p.mu.RUnlock()
	rows := make([]M, 0, len(materialRows))
	for _, material := range materialRows {
		key, group := v.Str(material["key"]), v.Str(material["category"])
		var owned any
		if p.known[group] && !p.uncertain[key] {
			owned = p.stock[key]
		}
		rows = append(rows, M{"key": key, "symbol": material["symbol"], "name": material["name"], "category": group, "owned": owned, "updatedAt": p.at[group]})
	}
	statuses := make([]M, 0, len(engineers))
	for _, engineer := range v.Rows(catalog["engineers"]) {
		key := v.Str(engineer["key"])
		observed := p.progress[key]
		status := strings.ToLower(v.Text(observed["status"], "unknown"))
		if unlocked, manual := p.manual[key].(bool); manual {
			status = "locked"
			if unlocked {
				status = "unlocked"
			}
		}
		statuses = append(statuses, M{"key": key, "english": engineer["english"], "journalStatus": observed["status"], "status": status, "rank": observed["rank"], "updatedAt": observed["timestamp"]})
	}
	known := M{}
	for category, value := range p.known {
		known[category] = value
	}
	return M{"materials": rows, "shopping": M{"knownCategories": known, "updatedAt": v.Copy(p.at)}, "engineers": statuses}
}
