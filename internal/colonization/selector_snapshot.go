package colonization

// SelectorSnapshot is sufficient for the global colony selector. Building
// estimates and supply reconciliation are computed only on colony screens.
func (p *Planner) SelectorSnapshot() M {
	p.mu.RLock()
	defer p.mu.RUnlock()
	projects := make([]M, 0, len(p.projects))
	for _, project := range p.projects {
		projects = append(projects, M{"id": project["id"], "name": project["name"], "targetSystem": project["targetSystem"], "status": project["status"]})
	}
	return M{"selectedProjectId": p.selected, "projects": projects}
}
