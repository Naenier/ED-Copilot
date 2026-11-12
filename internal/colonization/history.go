package colonization

import (
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	historyPeriod     = 3 * time.Hour
	historyRecent     = 240
	historyOlder      = 180
	historyPerColony  = historyRecent + historyOlder
	historyTotal      = 1500
	historyFactions   = 16
	historyMilestones = 200
)

type HistoryTarget struct {
	ProjectID, Name, Address string
}

type systemHistory struct {
	target     HistoryTarget
	samples    []M
	milestones map[string]M
}

func historyTarget(project M) HistoryTarget {
	return HistoryTarget{
		ProjectID: str(project["id"]),
		Name:      str(project["targetSystem"]),
		Address:   historyAddress(project["systemAddress"]),
	}
}

func historyAddress(raw any) string {
	id, err := strconv.ParseUint(str(raw), 10, 64)
	if err != nil || id == 0 {
		return ""
	}
	return strconv.FormatUint(id, 10)
}

func (a HistoryTarget) matches(b HistoryTarget) bool {
	return a.ProjectID == b.ProjectID && a.Name != "" && equal(a.Name, b.Name) &&
		(a.Address == "" || b.Address == "" || a.Address == b.Address)
}

func (p *Planner) HistoryTargets() []HistoryTarget {
	p.mu.RLock()
	defer p.mu.RUnlock()
	targets := []HistoryTarget{}
	for _, project := range p.projects {
		if !p.projectVisible(project) || project["status"] == "cancelled" || project["status"] == "planned" {
			continue
		}
		if target := historyTarget(project); target.Name != "" {
			targets = append(targets, target)
		}
	}
	return targets
}

func (p *Planner) observeJournalSystem(event, ctx M) {
	switch str(event["event"]) {
	case "FSDJump", "Location", "CarrierJump":
	default:
		return
	}
	if !v.Bool(v.At(ctx, "game", "live")) || v.Bool(v.At(ctx, "game", "multicrew")) || v.Bool(ctx["side"]) || v.Bool(event["Multicrew"]) {
		return
	}
	system, address := text(event["StarSystem"], 200), historyAddress(event["SystemAddress"])
	if system == "" {
		return
	}
	project := p.observedProject(system, address)
	if project == nil || !equal(project["targetSystem"], system) {
		return
	}
	target := historyTarget(project)
	if target.Address == "" {
		target.Address = address
	}
	faction := v.Obj(event["SystemFaction"])
	row := M{
		"at": event["timestamp"], "source": "journal", "population": event["Population"],
		"economy": event["SystemEconomy"], "secondEconomy": event["SystemSecondEconomy"],
		"security": event["SystemSecurity"], "government": event["SystemGovernment"], "allegiance": event["SystemAllegiance"],
		"controller": faction["Name"], "state": v.First(faction["FactionState"], event["FactionState"]),
		"power": event["ControllingPower"], "powerState": event["PowerplayState"], "powerControl": event["PowerplayStateControlProgress"],
		"reinforcement": event["PowerplayStateReinforcement"], "undermining": event["PowerplayStateUndermining"],
		"conflicts": optionalHistoryCount(event["Conflicts"]),
	}
	factions := []M{}
	for _, record := range limitedRows(event["Factions"], 64) {
		factions = append(factions, M{"name": record["Name"], "influence": record["Influence"], "state": record["FactionState"], "happiness": record["Happiness"]})
	}
	row["factions"] = factions
	p.addHistoryLocked(target, row)
}

func optionalHistoryCount(raw any) any {
	if items := v.Arr(raw); items != nil {
		return len(items)
	}
	return nil
}

// ObservePublicSystem adds only a timestamped snapshot whose identity matches
// the exact project selected before the asynchronous request started.
func (p *Planner) ObservePublicSystem(requested HistoryTarget, observation M) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if requested.ProjectID == "" {
		return false
	}
	project := p.projectLocked(requested.ProjectID)
	current := historyTarget(project)
	if !current.matches(requested) || project["status"] == "cancelled" || project["status"] == "planned" {
		return false
	}
	observed := HistoryTarget{ProjectID: requested.ProjectID, Name: text(observation["name"], 200), Address: historyAddress(observation["address"])}
	if observed.Address == "" || !current.matches(observed) || !requested.matches(observed) {
		return false
	}
	info, power := v.Obj(observation["info"]), v.Obj(observation["powerplay"])
	row := M{
		"at": info["observedAt"], "source": "spansh", "population": info["population"], "security": info["security"],
		"economy": info["economy"], "secondEconomy": info["secondEconomy"], "government": info["government"],
		"allegiance": info["allegiance"], "controller": info["controllingFaction"], "state": info["state"],
		"power": power["controllingPower"], "powerState": power["state"], "powerControl": power["controlProgress"],
		"reinforcement": power["reinforcement"], "undermining": power["undermining"],
		"factions": v.At(observation, "factions", "entries"),
	}
	return p.addHistoryLocked(observed, row)
}

func normalizedHistorySample(raw M, now time.Time) M {
	at, source := v.Time(raw["at"]), str(raw["source"])
	if at.IsZero() || at.After(now.Add(5*time.Minute)) || source != "journal" && source != "spansh" {
		return nil
	}
	row := M{}
	for _, key := range []string{"population", "powerControl", "reinforcement", "undermining", "conflicts"} {
		n, known := v.Number(raw[key])
		if known && n >= 0 && n <= 9e15 && (key == "powerControl" && n <= 1 || key != "powerControl" && math.Trunc(n) == n) {
			row[key] = n
		}
	}
	for _, key := range []string{"economy", "secondEconomy", "security", "government", "allegiance", "controller", "state", "power", "powerState"} {
		if label := historyLabel(key, text(raw[key], 120)); label != "" {
			row[key] = label
		}
	}
	factions, names := []M{}, map[string]bool{}
	for _, faction := range limitedRows(raw["factions"], 64) {
		name := text(faction["name"], 120)
		if name == "" || names[stringsLower(name)] {
			continue
		}
		item := M{"name": name}
		if influence, ok := v.Number(faction["influence"]); ok && influence >= 0 && influence <= 1 {
			item["influence"] = influence
		}
		for _, key := range []string{"state", "happiness"} {
			if label := historyLabel(key, text(faction[key], 100)); label != "" {
				item[key] = label
			}
		}
		if len(item) > 1 {
			factions = append(factions, item)
			names[stringsLower(name)] = true
		}
	}
	sort.SliceStable(factions, func(i, j int) bool {
		a, b := factions[i], factions[j]
		if a["name"] == row["controller"] && b["name"] != row["controller"] {
			return true
		}
		if b["name"] == row["controller"] && a["name"] != row["controller"] {
			return false
		}
		if v.Num(a["influence"]) != v.Num(b["influence"]) {
			return v.Num(a["influence"]) > v.Num(b["influence"])
		}
		return stringsLower(str(a["name"])) < stringsLower(str(b["name"]))
	})
	if len(factions) > 0 {
		row["factions"] = factions[:min(historyFactions, len(factions))]
	}
	if len(row) == 0 {
		return nil
	}
	row["at"], row["source"] = v.ISO(at), source
	return row
}

func historyLabel(field, label string) string {
	if label == "" {
		return ""
	}
	if field == "controller" || field == "power" {
		return label
	}
	var symbol string
	if strings.HasPrefix(label, "$") && strings.HasSuffix(label, ";") {
		symbol = strings.TrimSuffix(strings.TrimPrefix(label, "$"), ";")
		lower := strings.ToLower(symbol)
		for _, prefix := range []string{"economy_", "system_security_", "government_", "galaxy_map_info_state_", "faction_happinessband"} {
			if strings.HasPrefix(lower, prefix) {
				label = symbol[len(prefix):]
				break
			}
		}
	}
	switch strings.ToLower(strings.ReplaceAll(label, " ", "")) {
	case "agri", "agriculture":
		return "Agriculture"
	case "hightech":
		return "High Tech"
	case "military":
		return "Military"
	case "refinery":
		return "Refinery"
	case "extraction":
		return "Extraction"
	case "industrial":
		return "Industrial"
	case "colony":
		return "Colony"
	case "tourism":
		return "Tourism"
	case "service", "services":
		return "Service"
	case "high", "highsecurity":
		return "High"
	case "medium", "mediumsecurity":
		return "Medium"
	case "low", "lowsecurity":
		return "Low"
	case "lawless":
		return "Lawless"
	case "none":
		return "None"
	case "prisoncolony":
		return "Prison Colony"
	}
	if field == "happiness" {
		switch label {
		case "1":
			return "Elated"
		case "2":
			return "Happy"
		case "3":
			return "Discontented"
		case "4":
			return "Unhappy"
		case "5":
			return "Despondent"
		}
	}
	if symbol != "" && label == "$"+symbol+";" {
		return ""
	}
	return label
}

func (p *Planner) addHistoryLocked(target HistoryTarget, input M) bool {
	history := p.historyForLocked(target)
	if history == nil {
		return false
	}
	at := v.Time(input["at"])
	if at.IsZero() {
		return false
	}
	bucket := at.Truncate(historyPeriod)
	index := sort.Search(len(history.samples), func(i int) bool {
		return !v.Time(history.samples[i]["at"]).Truncate(historyPeriod).Before(bucket)
	})
	if index == 0 && len(history.samples) == historyPerColony && v.Time(history.samples[0]["at"]).Truncate(historyPeriod).After(bucket) {
		return false
	}
	if index < len(history.samples) && v.Time(history.samples[index]["at"]).Truncate(historyPeriod).Equal(bucket) && at.Before(v.Time(history.samples[index]["at"])) {
		return false
	}
	row := normalizedHistorySample(input, p.now())
	if row == nil {
		return false
	}
	if history.target.Address == "" {
		history.target.Address = target.Address
	}
	p.history[target.ProjectID] = history
	if index < len(history.samples) {
		previous := history.samples[index]
		previousAt := v.Time(previous["at"])
		if previousAt.Truncate(historyPeriod).Equal(bucket) {
			if at.Before(previousAt) {
				return false
			}
			if at.Equal(previousAt) {
				// Duplicate reports of the same sample do not replace preferred
				// journal fields. Fill fields only when timestamps really agree.
				if previous["source"] == "journal" || previous["source"] == row["source"] {
					row = merge(copyM(row), previous)
				} else {
					row = merge(copyM(previous), row)
				}
				if reflect.DeepEqual(previous, row) {
					return false
				}
			}
			history.samples[index] = row
			return true
		}
	}
	history.samples = append(history.samples, nil)
	copy(history.samples[index+1:], history.samples[index:])
	history.samples[index] = row
	if len(history.samples) > historyPerColony {
		history.samples[0] = nil
		history.samples = history.samples[1:]
	}
	compactHistory(history)
	p.trimHistoryLocked()
	return true
}

func compactHistory(history *systemHistory) {
	rows := history.samples
	if len(rows) <= historyRecent {
		return
	}
	recent := v.Time(rows[len(rows)-1]["at"]).Add(-30 * 24 * time.Hour)
	write := 0
	for i, row := range rows {
		at := v.Time(row["at"])
		if at.Before(recent) && i+1 < len(rows) {
			next := v.Time(rows[i+1]["at"])
			if next.Before(recent) && at.Truncate(24*time.Hour).Equal(next.Truncate(24*time.Hour)) {
				continue
			}
		}
		rows[write] = row
		write++
	}
	clear(rows[write:])
	history.samples = rows[:write]
}

func (p *Planner) historyForLocked(target HistoryTarget) *systemHistory {
	if target.ProjectID == "" || target.Name == "" {
		return nil
	}
	if p.history == nil {
		p.history = map[string]*systemHistory{}
	}
	history := p.history[target.ProjectID]
	if history == nil {
		history = &systemHistory{target: target, milestones: map[string]M{}}
	}
	if !history.target.matches(target) {
		return nil
	}
	return history
}

func (p *Planner) recordConstructionMilestone(project, depot M) {
	if depot["complete"] != true || depot["failed"] == true {
		return
	}
	target := historyTarget(project)
	if target.Address == "" {
		target.Address = historyAddress(depot["systemAddress"])
	}
	kind := "completion"
	if depot["source"] == "station" {
		kind = "first-visit"
	}
	p.addMilestoneLocked(target, M{
		"at": depot["historyAt"], "marketId": depot["marketId"], "kind": kind,
		"name": firstText(depot["stationDisplayName"], depot["station"]),
	})
}

func (p *Planner) addMilestoneLocked(target HistoryTarget, input M) {
	id, name, kind := historyAddress(input["marketId"]), text(input["name"], 200), str(input["kind"])
	at := v.Time(input["at"])
	if id == "" || name == "" || at.IsZero() || at.After(p.now().Add(5*time.Minute)) || kind != "completion" && kind != "first-visit" {
		return
	}
	history := p.historyForLocked(target)
	if history == nil {
		return
	}
	previous := history.milestones[id]
	if previous != nil {
		if previous["kind"] == "completion" && kind == "first-visit" ||
			previous["kind"] == kind && !at.Before(v.Time(previous["at"])) {
			return
		}
	} else if len(history.milestones) >= historyMilestones {
		var earliestID string
		var earliest time.Time
		for key, row := range history.milestones {
			if timestamp := v.Time(row["at"]); earliest.IsZero() || timestamp.Before(earliest) {
				earliest, earliestID = timestamp, key
			}
		}
		if !at.After(earliest) {
			return
		}
		delete(history.milestones, earliestID)
	}
	if history.target.Address == "" {
		history.target.Address = target.Address
	}
	history.milestones[id] = M{"marketId": id, "name": name, "at": v.ISO(at), "kind": kind}
	p.history[target.ProjectID] = history
}

func (h *systemHistory) sortedMilestones() []M {
	rows := make([]M, 0, len(h.milestones))
	for _, row := range h.milestones {
		rows = append(rows, copyM(row))
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := v.Time(rows[i]["at"]), v.Time(rows[j]["at"])
		if a.Equal(b) {
			return str(rows[i]["marketId"]) < str(rows[j]["marketId"])
		}
		return a.Before(b)
	})
	return rows
}

func (p *Planner) trimHistoryLocked() {
	total := 0
	for _, history := range p.history {
		total += len(history.samples)
	}
	for total > historyTotal {
		var earliest *systemHistory
		for _, history := range p.history {
			if len(history.samples) > 1 && (earliest == nil || v.Time(history.samples[0]["at"]).Before(v.Time(earliest.samples[0]["at"]))) {
				earliest = history
			}
		}
		if earliest == nil {
			return
		}
		earliest.samples[0] = nil
		earliest.samples = earliest.samples[1:]
		total--
	}
}

func (p *Planner) SystemHistorySnapshot() M {
	p.mu.RLock()
	defer p.mu.RUnlock()
	projects := []M{}
	for _, project := range p.projects {
		if p.projectVisible(project) {
			projects = append(projects, M{"id": project["id"], "name": project["name"], "targetSystem": project["targetSystem"]})
		}
	}
	result := M{"projects": projects, "selectedProjectId": p.selected, "samples": []M{}}
	project := p.projectLocked("")
	if project == nil {
		return result
	}
	target := historyTarget(project)
	result["system"] = target.Name
	if history := p.history[target.ProjectID]; history != nil && history.target.matches(target) {
		result["samples"] = cloneRows(history.samples)
		result["milestones"] = history.sortedMilestones()
	}
	return result
}

func (p *Planner) saveSystemHistory() []M {
	entries := []M{}
	for _, project := range p.projects {
		target := historyTarget(project)
		history := p.history[target.ProjectID]
		if history != nil && history.target.matches(target) && (len(history.samples) > 0 || len(history.milestones) > 0) {
			entries = append(entries, M{"projectId": target.ProjectID, "name": target.Name, "address": history.target.Address, "samples": cloneRows(history.samples), "milestones": history.sortedMilestones()})
		}
	}
	return entries
}

func (p *Planner) restoreSystemHistory(raw any) {
	projects := map[string]HistoryTarget{}
	for _, project := range p.projects {
		projects[str(project["id"])] = historyTarget(project)
	}
	for _, row := range limitedRows(raw, 100) {
		saved := HistoryTarget{ProjectID: text(row["projectId"], 80), Name: text(row["name"], 200), Address: historyAddress(row["address"])}
		if !projects[saved.ProjectID].matches(saved) {
			continue
		}
		for _, sample := range limitedRows(row["samples"], historyPerColony) {
			p.addHistoryLocked(saved, sample)
		}
		for _, milestone := range limitedRows(row["milestones"], historyMilestones) {
			p.addMilestoneLocked(saved, milestone)
		}
	}
}
