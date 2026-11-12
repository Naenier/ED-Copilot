package exploration

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// Snapshot pages the journal index for an explicit desktop request. It is
// not part of the periodic HUD frame.
func (s *Service) Snapshot(owner string, query M) M {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ws := s.workspace(owner, false)
	if ws == nil {
		ws = newWorkspace()
	}
	return ws.snapshot(owner, query)
}

func (ws *workspace) snapshot(owner string, query M) M {
	mode := v.Text(query["mode"], "expeditions")
	result := M{"workspace": owner, "mode": mode}
	system := ws.systemFor(query["systemAddress"], query["systemName"])
	if system != nil {
		result["system"] = ws.systemRow(system)
		if selected := system.Bodies[v.Str(query["bodyId"])]; selected != nil {
			result["body"] = ws.bodyRow(system, selected)
		}
		tree := bodyTree(system)
		first, last, page := pageRange(len(tree), query["bodyPage"])
		rows := []M{}
		for _, item := range tree[first:last] {
			row := ws.bodyRow(system, item.body)
			row["depth"] = item.depth
			rows = append(rows, row)
		}
		result["bodies"] = pageRows(rows, len(tree), page)
		targets := make([]*body, 0)
		for _, item := range tree {
			b := item.body
			if b.PlanetClass != "" && b.MappedAt == "" && estimate(b, true, true) >= 100000 {
				targets = append(targets, b)
			}
		}
		sort.SliceStable(targets, func(i, j int) bool { return estimate(targets[i], true, true) > estimate(targets[j], true, true) })
		valuable := []M{}
		for _, b := range bounded(targets, 5) {
			valuable = append(valuable, ws.bodyRow(system, b))
		}
		result["valuableBodies"] = valuable
	}
	switch mode {
	case "findings":
		ws.findingSnapshot(result, query)
	case "surveys":
		ws.surveySnapshot(result, query)
	default:
		ws.expeditionSnapshot(result, query)
	}
	return result
}

type treeBody struct {
	body  *body
	depth int
}

func bodyTree(sy *system) []treeBody {
	bodies := []*body{}
	children := map[string][]*body{}
	for _, b := range sy.Bodies {
		if b != nil {
			bodies = append(bodies, b)
		}
	}
	sort.Slice(bodies, func(i, j int) bool {
		if bodies[i].Name != bodies[j].Name {
			return bodies[i].Name < bodies[j].Name
		}
		return bodies[i].ID < bodies[j].ID
	})
	for _, b := range bodies {
		parent := b.ParentID
		if sy.Bodies[parent] == nil || parent == b.ID {
			parent = ""
		}
		children[parent] = append(children[parent], b)
	}
	out := make([]treeBody, 0, len(bodies))
	visited := map[string]bool{}
	var visit func(*body, int)
	visit = func(b *body, depth int) {
		if visited[b.ID] {
			return
		}
		visited[b.ID] = true
		out = append(out, treeBody{b, depth})
		for _, child := range children[b.ID] {
			visit(child, depth+1)
		}
	}
	for _, b := range children[""] {
		visit(b, 0)
	}
	for _, b := range bodies {
		visit(b, 0)
	}
	return out
}

func (ws *workspace) expeditionSnapshot(result, query M) {
	projects := []M{}
	selectedIndex := ws.expeditionIndex(v.Str(query["expeditionId"]))
	for i, exp := range ws.Expeditions {
		if exp == nil {
			continue
		}
		status := "planned"
		if exp.EndedAt != "" {
			status = "finished"
		} else if exp.StartedAt != "" {
			status = "active"
			if selectedIndex < 0 && query["expeditionId"] != "all" {
				selectedIndex = i
			}
		}
		projects = append(projects, M{"id": exp.ID, "name": exp.Name, "status": status, "visited": len(exp.Travel)})
	}
	result["expeditions"] = projects
	var exp *expedition
	travelSystems := map[string]bool{}
	if selectedIndex >= 0 {
		exp = ws.Expeditions[selectedIndex]
		plan := v.Obj(v.Clone(expeditionPlan(exp)))
		stops := []M{}
		for _, target := range exp.Stops {
			item := v.Obj(v.Clone(target))
			if sy := ws.systemFor(target.Address, target.System); sy != nil {
				item["address"], item["visitedAt"], item["fssAt"] = sy.Address, sy.VisitedAt, sy.FSSAt
			}
			stops = append(stops, item)
		}
		plan["stops"] = stops
		path := []travel{}
		seenTravel := make([]travel, 0, len(exp.Travel))
		for _, visited := range exp.Travel {
			travelSystems[visited.Address] = true
			if len(visited.Coordinates) == 3 {
				seenTravel = append(seenTravel, visited)
			}
		}
		sort.SliceStable(seenTravel, func(i, j int) bool { return newer(seenTravel[j].At, seenTravel[i].At) })
		step := max(1, int(math.Ceil(float64(len(seenTravel))/200)))
		for i, row := range seenTravel {
			if i%step == 0 || i == len(seenTravel)-1 {
				path = append(path, row)
			}
		}
		plan["path"] = v.Clone(path)
		plan["travelCount"] = len(exp.Travel)
		result["expedition"] = plan
	}
	systems := []*system{}
	var onboard, earned, lost int64
	estimated, missing := 0, 0
	filter := normalized(v.Str(query["filter"]))
	for _, sy := range ws.Systems {
		if exp != nil && !travelSystems[sy.Address] && (exp.StartedAt == "" || !within(sy.LastAt, exp)) {
			continue
		}
		if filter == "" || strings.Contains(normalized(sy.Name), filter) {
			systems = append(systems, sy)
		}
		for _, b := range sy.Bodies {
			if b == nil {
				continue
			}
			value, at := ws.pendingAt(sy, b, "")
			if at == "" || exp != nil && !within(at, exp) {
				continue
			}
			if value > 0 {
				onboard += value
				estimated++
			} else {
				missing++
			}
		}
	}
	sort.Slice(systems, func(i, j int) bool {
		if systems[i].LastAt == systems[j].LastAt {
			return systems[i].Address < systems[j].Address
		}
		return newer(systems[i].LastAt, systems[j].LastAt)
	})
	first, last, page := pageRange(len(systems), query["page"])
	rows := []M{}
	for _, sy := range systems[first:last] {
		rows = append(rows, ws.systemRow(sy))
	}
	result["systems"] = pageRows(rows, len(systems), page)
	receipts := []receipt{}
	for _, receipt := range ws.Receipts {
		if exp != nil && receipt.ExpeditionID != exp.ID {
			continue
		}
		receipts = append(receipts, receipt)
		if receipt.Credits != nil {
			earned += *receipt.Credits
		}
		if receipt.Kind == "loss" {
			lost += receipt.Estimated
		}
	}
	sort.SliceStable(receipts, func(i, j int) bool { return newer(receipts[i].At, receipts[j].At) })
	result["receipts"] = v.Clone(bounded(receipts, 20))
	result["wallet"] = M{"onboardEstimate": onboard, "estimatedBodies": estimated, "unpricedBodies": missing, "confirmedCredits": earned, "lostEstimate": lost}
}

func expeditionPlan(exp *expedition) M {
	return M{"id": exp.ID, "name": exp.Name, "goal": exp.Goal, "notes": exp.Notes, "returnSystem": exp.ReturnSystem,
		"days": exp.Days, "createdAt": exp.CreatedAt, "startedAt": exp.StartedAt, "endedAt": exp.EndedAt, "stops": exp.Stops}
}

func within(at string, exp *expedition) bool {
	return at != "" && exp.StartedAt != "" && !newer(exp.StartedAt, at) && (exp.EndedAt == "" || !newer(at, exp.EndedAt))
}

func (ws *workspace) systemRow(sy *system) M {
	scanned, mapped := 0, 0
	var bodyCount any
	if sy.BodyCount != nil {
		bodyCount = *sy.BodyCount
	}
	var estimate int64
	for _, b := range sy.Bodies {
		if b == nil {
			continue
		}
		if b.ScannedAt != "" {
			scanned++
		}
		if b.MappedAt != "" {
			mapped++
		}
		value, _ := ws.pendingAt(sy, b, "")
		estimate += value
	}
	return M{"address": sy.Address, "system": sy.Name, "coordinates": append([]float64(nil), sy.Coordinates...),
		"lastAt": sy.LastAt, "visitedAt": sy.VisitedAt, "fssAt": sy.FSSAt, "bodyCount": bodyCount,
		"scanned": scanned, "mapped": mapped, "estimated": estimate, "note": ws.Notes[sy.Address]}
}

func (ws *workspace) bodyRow(sy *system, b *body) M {
	row := v.Obj(v.Clone(b))
	row["system"], row["systemAddress"], row["kind"] = sy.Name, sy.Address, worldKind(b)
	row["note"] = ws.Notes[sy.Address+"|"+b.ID]
	row["dataState"] = ws.bodyStatus(sy, b)
	if parent := sy.Bodies[b.ParentID]; parent != nil {
		row["parent"] = parent.Name
	}
	if value, _ := ws.pendingAt(sy, b, ""); value > 0 {
		row["estimated"] = value
	}
	if value := estimate(b, true, true); value > 0 && b.PlanetClass != "" {
		row["mappingEstimate"] = value
	}
	return row
}

func (ws *workspace) findingSnapshot(result, query M) {
	profiles := []M{}
	for _, profile := range ws.FindingProfiles {
		profiles = append(profiles, M{"id": profile.ID, "name": profile.Name})
	}
	result["profiles"], result["profile"] = profiles, v.Clone(ws.profile())
	rules := []M{}
	byRule := map[string]ruleDefinition{}
	for _, d := range ruleDefinitions {
		byRule[d.key] = d
		rules = append(rules, M{"key": d.key, "title": d.title, "english": d.english, "unit": d.unit, "max": d.maximum, "default": d.value})
	}
	result["rules"] = rules
	filter := v.Str(query["rule"])
	matching := make([]finding, 0, len(ws.Findings))
	for _, entry := range ws.Findings {
		if filter == "" || entry.Rule == filter {
			matching = append(matching, entry)
		}
	}
	sort.SliceStable(matching, func(i, j int) bool { return newer(matching[i].ObservedAt, matching[j].ObservedAt) })
	first, last, page := pageRange(len(matching), query["page"])
	rows := []M{}
	for _, entry := range matching[first:last] {
		row := v.Obj(v.Clone(entry))
		d := byRule[entry.Rule]
		row["title"], row["englishTitle"], row["note"] = d.title, d.english, ws.Notes[entry.SystemAddress+"|"+entry.BodyID]
		rows = append(rows, row)
	}
	result["findings"] = pageRows(rows, len(matching), page)
	records := []record{}
	for _, key := range []string{"hot", "cold", "gravity", "low-gravity", "radius", "small-radius", "mass", "eccentric"} {
		if row, exists := ws.Records[key]; exists {
			records = append(records, row)
		}
	}
	result["records"] = v.Clone(records)
	result["expeditions"] = []M{}
	for _, e := range ws.Expeditions {
		if e != nil {
			result["expeditions"] = append(v.Rows(result["expeditions"]), M{"id": e.ID, "name": e.Name})
		}
	}
}

func (ws *workspace) surveySnapshot(result, query M) {
	projects := []M{}
	for _, p := range ws.Surveys {
		if p != nil {
			projects = append(projects, M{"id": p.ID, "name": p.Name, "criterion": p.Criterion, "targets": len(p.Targets)})
		}
	}
	result["surveys"] = projects
	index := ws.surveyIndex(v.Str(query["surveyId"]))
	if index < 0 {
		index = ws.surveyIndex(ws.SelectedSurvey)
	}
	if index < 0 && len(projects) > 0 {
		index = ws.surveyIndex(v.Str(projects[0]["id"]))
	}
	if index >= 0 {
		p := ws.Surveys[index]
		project := v.Obj(v.Clone(p))
		rows := []M{}
		done, skipped := 0, 0
		var next M
		first, last, page := pageRange(len(p.Targets), query["page"])
		for i, item := range p.Targets {
			progress, relevant, complete, skip, sy := ws.targetStages(item, p)
			if skip {
				skipped++
			} else if complete {
				done++
			}
			if i < first || i >= last {
				if next != nil || skip || complete {
					continue
				}
			}
			row := v.Obj(v.Clone(item))
			row["progress"], row["relevantBodies"], row["complete"], row["skipped"] = v.Clone(progress), relevant, complete, skip
			if sy != nil {
				row["address"], row["candidate"] = sy.Address, false
			}
			if next == nil && !skip && !complete {
				next = row
			}
			if i >= first && i < last {
				rows = append(rows, row)
			}
		}
		var nextTarget any
		if next != nil {
			nextTarget = next
		}
		project["results"], project["completed"], project["skipped"], project["next"] = pageRows(rows, len(p.Targets), page), done, skipped, nextTarget
		if box, ok := parseBoxel(p.BoxelSeed); ok {
			project["boxel"] = box.projection(ws.systemFor(nil, p.BoxelSeed))
		}
		result["survey"] = project
	}
	type world struct {
		sy *system
		b  *body
	}
	worlds := []world{}
	filter := normalized(v.Str(query["worldFilter"]))
	for _, sy := range ws.Systems {
		for _, b := range sy.Bodies {
			if b == nil || b.ScannedAt == "" || !b.Terraformable && worldKind(b) != "earthlike" {
				continue
			}
			if filter == "earthlike" && worldKind(b) != "earthlike" || filter == "terraformable" && !b.Terraformable {
				continue
			}
			worlds = append(worlds, world{sy, b})
		}
	}
	sort.Slice(worlds, func(i, j int) bool {
		if worlds[i].b.ScannedAt == worlds[j].b.ScannedAt {
			return worlds[i].b.Name < worlds[j].b.Name
		}
		return newer(worlds[i].b.ScannedAt, worlds[j].b.ScannedAt)
	})
	first, last, page := pageRange(len(worlds), query["worldPage"])
	rows := []M{}
	for _, world := range worlds[first:last] {
		rows = append(rows, ws.bodyRow(world.sy, world.b))
	}
	result["worlds"] = pageRows(rows, len(worlds), page)
}

func (ws *workspace) targetStages(item target, p *survey) (stages, int, bool, bool, *system) {
	var result stages
	firstVisit := ""
	if item.Imported != nil {
		result = *item.Imported
		firstVisit = result.VisitedAt
	}
	sy := ws.systemFor(item.Address, item.System)
	relevant := 0
	if sy != nil {
		result.VisitedAt = latest(sy.VisitedAt, result.VisitedAt)
		firstVisit = earliest(v.Text(sy.FirstVisit, sy.VisitedAt), firstVisit)
		result.FSSAt = latest(sy.FSSAt, result.FSSAt)
		totalScanned, completed := 0, 0
		var lastMap string
		for _, b := range sy.Bodies {
			if b == nil || b.ScannedAt == "" {
				continue
			}
			totalScanned++
			if b.PlanetClass == "" || !matchesWorld(b, p.MappingTypes) {
				continue
			}
			relevant++
			if b.MappedAt != "" {
				completed++
				lastMap = latest(b.MappedAt, lastMap)
			}
		}
		if sy.FSSAt != "" && sy.BodyCount != nil && totalScanned >= *sy.BodyCount && completed == relevant {
			result.MappedAt = latest(latest(sy.FSSAt, lastMap), result.MappedAt)
		}
	}
	complete := result.VisitedAt != ""
	switch p.Criterion {
	case "fss":
		complete = result.FSSAt != ""
	case "mapping":
		complete = result.MappedAt != ""
	}
	skip := item.Skip || p.SkipVisited && !complete && firstVisit != "" && p.CreatedAt != "" && !newer(firstVisit, p.CreatedAt)
	return result, relevant, complete, skip, sy
}

func matchesWorld(b *body, types []string) bool {
	for _, kind := range types {
		if kind == "all" || kind == "terraformable" && b.Terraformable || kind == worldKind(b) {
			return true
		}
	}
	return false
}

func pageRange(total int, raw any) (int, int, int) {
	page := min(max(0, v.Int(raw)), max(0, (total-1)/pageSize))
	return page * pageSize, min(total, (page+1)*pageSize), page
}
func pageRows(rows []M, total, page int) M {
	return M{"rows": rows, "total": total, "page": page, "pageSize": pageSize}
}

// ExportSettings persists plans and a bounded recent observation index.
// Older observations remain available through replay of the original logs.
func (s *Service) ExportSettings() M {
	s.mu.RLock()
	defer s.mu.RUnlock()
	saved := M{}
	perWorkspace := (2 << 20) / max(1, len(s.workspaces))
	owners := make([]string, 0, len(s.workspaces))
	for owner := range s.workspaces {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	for _, owner := range owners {
		ws := s.workspaces[owner]
		copy := *ws
		copy.Systems = map[string]*system{}
		findings := append([]finding(nil), ws.Findings...)
		sort.SliceStable(findings, func(i, j int) bool { return newer(findings[j].ObservedAt, findings[i].ObservedAt) })
		copy.Findings = fitRecent(findings, perWorkspace/5)
		receipts := append([]receipt(nil), ws.Receipts...)
		sort.SliceStable(receipts, func(i, j int) bool { return newer(receipts[j].At, receipts[i].At) })
		copy.Receipts = fitRecent(receipts, perWorkspace/5)
		copy.Expeditions = make([]*expedition, 0, len(ws.Expeditions))
		travelShare := perWorkspace / 5 / max(1, len(ws.Expeditions))
		for _, exp := range ws.Expeditions {
			if exp == nil {
				continue
			}
			entry := *exp
			entry.Travel = fitRecent(exp.Travel, travelShare)
			copy.Expeditions = append(copy.Expeditions, &entry)
		}
		systems := make([]*system, 0, len(ws.Systems))
		for _, sy := range ws.Systems {
			systems = append(systems, sy)
		}
		sort.Slice(systems, func(i, j int) bool { return newer(systems[i].LastAt, systems[j].LastAt) })
		share := perWorkspace * 2 / 5
		for _, sy := range bounded(systems, 700) {
			raw, err := json.Marshal(sy)
			if err != nil || len(raw) > share {
				continue
			}
			share -= len(raw)
			copy.Systems[sy.Address] = sy
		}
		saved[owner] = &copy
	}
	return v.Copy(saved)
}

func fitRecent[T any](values []T, budget int) []T {
	end := len(values)
	first := end
	for first > 0 {
		raw, err := json.Marshal(values[first-1])
		if err != nil || len(raw)+1 > budget {
			break
		}
		budget -= len(raw) + 1
		first--
	}
	return values[first:end]
}
