package nearest

import (
	"context"
	"errors"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/data"
	"github.com/Naenier/ED-Copilot/internal/engineering"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	playerFactionSystemLimit = 6
	conflictSystemLimit      = 8
)

type signalFilter struct {
	field string
	value string
}

var signalFilters = map[string]signalFilter{
	"tourist-beacon":     {"landmarks", "Tourist Beacon"},
	"guardian-beacon":    {"landmarks", "Guardian Beacon"},
	"guardian-ruin":      {"landmarks", "Guardian Ruin"},
	"guardian-structure": {"landmarks", "Guardian Structure"},
	"thargoid-structure": {"landmarks", "Thargoid Structure"},
	"thargoid-spire":     {"landmarks", "Thargoid Spires"},
	"thargoid-barnacle":  {"landmarks", "Thargoid Barnacle"},
	"barnacle-matrix":    {"landmarks", "Thargoid Barnacle Matrix"},
	"crashed-ship":       {"landmarks", "Crashed Ship"},
	"wrecked-ship":       {"landmarks", "Wrecked Ship"},
	"abandoned-base":     {"landmarks", "Abandoned Base"},
	"megaship":           {"station", "Mega ship"},
	"biological":         {"signals", "Biological"},
	"geological":         {"signals", "Geological"},
	"human":              {"signals", "Human"},
	"guardian":           {"signals", "Guardian"},
	"thargoid":           {"signals", "Thargoid"},
	"planetary-mining":   {"signals", "Planetary Mining Location"},
	"major-anomaly":      {"signals", "Major Anomaly"},
}

func (c *Client) searchSpecial(ctx context.Context, q Query) (Result, error) {
	switch q.Category {
	case "factions":
		return c.searchFactions(ctx, q, "")
	case "signals":
		return c.searchSignals(ctx, q)
	}
	switch spaceText(q.Filters["kind"]) {
	case "", "conflicts":
		return c.searchFactions(ctx, q, "conflicts")
	case "multi-state":
		return c.searchFactions(ctx, q, "multi-state")
	case "engineers":
		return c.searchEngineers(ctx, q)
	case "restore", "mining":
		return c.searchMissionCandidates(ctx, q, spaceText(q.Filters["kind"]))
	case "massacre":
		return c.searchMassacre(ctx, q)
	default:
		return Result{}, errors.New("Выберите вид поиска.")
	}
}

func (c *Client) searchFactions(ctx context.Context, q Query, kind string) (Result, error) {
	filterQuery := q
	filterQuery.Filters = v.Copy(q.Filters)
	delete(filterQuery.Filters, "factionStates")
	filters := spaceSystemFilters(filterQuery)
	if filters["population"] == nil {
		setSpaceRange(filters, "population", 1, 1e15)
	}

	factionFilter := M{}
	if q.Name != "" && kind == "" {
		names, err := c.factionNames(ctx, q.Name)
		if err != nil {
			return Result{}, err
		}
		if len(names) == 0 {
			return Result{}, nil
		}
		setSpaceGroup(factionFilter, "name", names)
		if v.Bool(q.Filters["controllerOnly"]) && filters["controlling_minor_faction"] == nil {
			setSpaceGroup(filters, "controlling_minor_faction", names)
		}
	}
	setSpaceGroup(factionFilter, "allegiance", spaceValues(q.Filters["factionAllegiance"]))
	setSpaceGroup(factionFilter, "government", spaceValues(q.Filters["factionGovernment"]))
	if kind == "conflicts" {
		setSpaceGroup(factionFilter, "state", []string{"War", "Civil War"})
	}
	if len(factionFilter) != 0 {
		filters["minor_faction_presences"] = append(v.Rows(filters["minor_faction_presences"]), factionFilter)
	}

	playerOnly := v.Bool(q.Filters["playerOnly"])
	result := Result{}
	if playerOnly {
		if c.EDSM == nil {
			result.Note = "Для отбора фракций игроков нужен каталог EDSM."
			return result, nil
		}
		q.Limit = min(specialPageSize(q), playerFactionSystemLimit)
		result.Note = "Фракции игроков проверяются в EDSM. На странице до шести систем."
	} else if kind == "conflicts" {
		q.Limit = min(specialPageSize(q), conflictSystemLimit)
		result.Note = "Противники указаны по данным EliteHub или недавней записи журнала. Для остальных систем показаны состояния фракций."
	} else if kind == "multi-state" {
		result.Note = "Учитываются одновременно действующие состояния фракций."
	}

	raw, err := c.request(ctx, "/systems/search", spacePayload(q, filters))
	if err != nil {
		return result, err
	}
	systems, err := specialRows(raw)
	if err != nil {
		return result, err
	}
	result.HasMore = specialHasMore(q, raw)
	var conflictsBySystem map[string][]M
	if kind == "conflicts" {
		conflictsBySystem = c.publicConflicts(ctx, systems)
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
	}
	for _, system := range systems {
		name := spaceText(system["name"])
		if name == "" || !matchesSpecialPresence(system, q) {
			continue
		}
		factions := v.Rows(system["minor_faction_presences"])
		if kind == "conflicts" {
			matched := journalConflicts(q.Politics, system)
			if len(matched) == 0 {
				matched = conflictsBySystem[spaceAddress(system["id64"])]
			}
			if len(matched) != 0 {
				for _, row := range matched {
					if matchesName(q.Name, row["name"], row["opponent"], row["system"]) {
						result.Rows = append(result.Rows, row)
					}
				}
				continue
			}
		}
		if len(factions) == 0 || !hasFactionMatch(q, factions, system, kind) {
			continue
		}

		var playerFactions map[string]bool
		if playerOnly {
			observation, err := c.EDSM.Factions(ctx, name)
			if err != nil {
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				result.Note = "EDSM не подтвердил статус фракций игроков для части систем. Показаны подтверждённые результаты."
				continue
			}
			playerFactions = make(map[string]bool)
			for _, f := range v.Rows(observation["factions"]) {
				if v.Bool(f["isPlayer"]) {
					playerFactions[strings.ToLower(spaceText(f["name"]))] = true
				}
			}
		}

		for _, faction := range factions {
			factionName := spaceText(faction["name"])
			if !matchesFaction(q, faction, system, kind) || playerOnly && !playerFactions[strings.ToLower(factionName)] {
				continue
			}
			states := factionStates(faction)
			row := specialSystemRow(system)
			row["name"], row["faction"], row["states"] = factionName, factionName, states
			row["allegiance"], row["government"] = faction["allegiance"], faction["government"]
			row["influence"], row["state"] = faction["influence"], faction["state"]
			row["controlling"] = strings.EqualFold(factionName, spaceText(system["controlling_minor_faction"]))
			if playerOnly {
				row["player"] = true
			}
			if kind == "conflicts" {
				row["state"] = warState(states)
			}
			result.Rows = append(result.Rows, row)
		}
	}
	// The upstream count is by system. A system may contain many matching factions.
	return result, nil
}

func factionStates(faction M) []string {
	states := []string{}
	for _, state := range spaceValues(v.First(faction["active_states"], faction["activeStates"])) {
		if !strings.EqualFold(state, "None") && !containsState(states, state) {
			states = append(states, state)
		}
	}
	primary := spaceText(faction["state"])
	if primary != "" && (len(states) == 0 || !strings.EqualFold(primary, "None")) && !containsState(states, primary) {
		return append(states, primary)
	}
	return states
}

func containsState(states []string, target string) bool {
	for _, state := range states {
		if strings.EqualFold(state, target) {
			return true
		}
	}
	return false
}

func warState(states []string) string {
	for _, state := range []string{"War", "Civil War"} {
		if containsState(states, state) {
			return state
		}
	}
	return ""
}

func matchesState(states, wanted []string, mode string) bool {
	if len(wanted) == 0 {
		return true
	}
	if len(states) == 0 {
		return false
	}
	count := 0
	for _, want := range wanted {
		if containsState(states, want) {
			count++
		}
	}
	switch mode {
	case "none":
		return count == 0
	case "all":
		return count == len(wanted)
	default:
		return count > 0
	}
}

func matchesSpecialPresence(system M, q Query) bool {
	target := spaceText(q.Filters["presenceFaction"])
	if target == "" {
		return true
	}
	controlling := strings.EqualFold(target, spaceText(system["controlling_minor_faction"]))
	present := controlling
	for _, faction := range v.Rows(system["minor_faction_presences"]) {
		present = present || strings.EqualFold(target, spaceText(faction["name"]))
	}
	switch q.Filters["presenceMode"] {
	case "controlling":
		return controlling
	case "not-controlling":
		return present && !controlling
	case "exclude":
		return !present
	default:
		return present
	}
}

func matchesFaction(q Query, faction, system M, kind string) bool {
	name := spaceText(faction["name"])
	if name == "" {
		return false
	}
	if kind == "" && !matchesName(q.Name, name) {
		return false
	}
	if kind != "" && !matchesName(q.Name, name, system["name"]) {
		return false
	}
	for field, key := range map[string]string{"allegiance": "factionAllegiance", "government": "factionGovernment"} {
		if values := spaceValues(q.Filters[key]); len(values) != 0 && !containsState(values, spaceText(faction[field])) {
			return false
		}
	}
	if v.Bool(q.Filters["controllerOnly"]) && !strings.EqualFold(name, spaceText(system["controlling_minor_faction"])) {
		return false
	}
	states := factionStates(faction)
	switch kind {
	case "conflicts":
		return warState(states) != ""
	case "multi-state":
		minimum := max(2, min(10, v.Int(q.Filters["minimumStates"])))
		return len(states) >= minimum
	default:
		mode := spaceText(v.First(q.Filters["factionStateMode"], q.Filters["stateMode"]))
		return matchesState(states, spaceValues(q.Filters["factionStates"]), mode)
	}
}

func hasFactionMatch(q Query, factions []M, system M, kind string) bool {
	for _, faction := range factions {
		if matchesFaction(q, faction, system, kind) {
			return true
		}
	}
	return false
}

func journalConflicts(observations []M, system M) []M {
	out := []M{}
	for _, observation := range observations {
		if systemID := spaceAddress(system["id64"]); systemID != "" && spaceAddress(observation["address"]) != "" {
			if systemID != spaceAddress(observation["address"]) {
				continue
			}
		} else if !strings.EqualFold(spaceText(observation["name"]), spaceText(system["name"])) {
			continue
		}
		conflicts := v.Obj(observation["conflicts"])
		at := v.Time(conflicts["observedAt"])
		if conflicts["source"] != "journal" || at.IsZero() || time.Since(at) > 30*time.Minute || time.Until(at) > time.Minute {
			continue
		}
		for _, conflict := range v.Rows(conflicts["entries"]) {
			state := map[string]string{"war": "War", "civilwar": "Civil War"}[v.Key(conflict["type"])]
			if state == "" || !strings.EqualFold(spaceText(conflict["status"]), "active") {
				continue
			}
			faction, opponent := spaceText(v.At(conflict, "faction1", "name")), spaceText(v.At(conflict, "faction2", "name"))
			if faction == "" || opponent == "" {
				continue
			}
			row := specialSystemRow(system)
			row["name"], row["faction"], row["opponent"], row["state"] = faction, faction, opponent, state
			row["updatedAt"] = conflicts["observedAt"]
			out = append(out, row)
		}
	}
	return out
}

func (c *Client) searchSignals(ctx context.Context, q Query) (Result, error) {
	signal := spaceText(q.Filters["signal"])
	if signal == "" {
		signal = "tourist-beacon"
	}
	if signal == "titan-wreck" {
		return c.searchTitanStations(ctx, q)
	}
	if _, ok := orbitalSignalLabels[signal]; ok {
		return c.searchOrbitalSignals(ctx, q, signal)
	}
	entry, ok := signalFilters[signal]
	if !ok {
		return Result{}, errors.New("Выберите тип сигнала.")
	}
	filters := M{}
	var systemFilters M
	resource := "/bodies/search"
	if entry.field == "station" {
		resource = "/stations/search"
		setSpaceGeometry(filters, q, "system_region")
		delete(filters, "system_region")
		setSpaceGroup(filters, "type", []string{entry.value})
		filters["services"] = []M{{"name": stationGroup("Dock")}}
		systemFilters = M{"stations": []M{{"type": filters["type"], "services": stationGroup("Dock")}}}
	} else {
		filters = spaceBodyFilters(q)
		if entry.field == "signals" {
			filters["signals"] = []M{{"name": entry.value, "value": []int{1, 10000}, "comparison": "<=>"}}
		} else {
			filters["landmarks"] = []M{{"subtype": M{"value": []string{entry.value}}}}
			if signal != "guardian-beacon" && signal != "thargoid-spire" {
				filters["landmarks"] = []M{{"type": M{"value": []string{entry.value}}}}
			}
			systemFilters = M{"landmarks": filters["landmarks"]}
		}
	}
	result := Result{}
	if hasSpecialSystemFilters(q) || entry.field == "station" && q.Region != "" {
		ids, partial, err := c.specialSystemIDs(ctx, q, systemFilters)
		if err != nil {
			return result, err
		}
		if partial {
			result.Note = "Поиск сигналов охватывает первые 100 систем, подходящих по дополнительным условиям."
		}
		if len(ids) == 0 {
			return result, nil
		}
		setSpaceGroup(filters, "system_id64", ids)
	}
	if q.Name != "" {
		filters["name"] = nameCondition(q.Name)
	}
	raw, err := c.request(ctx, resource, spacePayload(q, filters))
	if err != nil {
		return result, err
	}
	rows, err := specialRows(raw)
	if err != nil {
		return result, err
	}
	for _, item := range rows {
		row := signalRow(item, entry)
		if row == nil {
			continue
		}
		result.Rows = append(result.Rows, row)
	}
	result.Total = v.Int(raw["count"])
	result.HasMore = specialHasMore(q, raw)
	return result, nil
}

func hasSpecialSystemFilters(q Query) bool {
	filters := spaceSystemFilters(q)
	delete(filters, "region")
	delete(filters, "distance")
	return len(filters) > 0
}

func signalRow(raw M, signal signalFilter) M {
	name, system := spaceText(raw["name"]), spaceText(raw["system_name"])
	if system == "" || name == "" {
		return nil
	}
	row := M{
		"name": name, "system": system, "systemAddress": spaceAddress(raw["system_id64"]),
		"distanceLy": raw["distance"], "distanceLs": raw["distance_to_arrival"], "type": signal.value,
		"updatedAt": raw["updated_at"],
	}
	if signal.field == "station" {
		id := spaceAddress(raw["market_id"])
		row["station"] = name
		if id != "" {
			row["url"] = "https://spansh.co.uk/station/" + id
		}
		return row
	}
	row["body"] = name
	if id := spaceAddress(raw["id64"]); id != "" {
		row["url"] = "https://spansh.co.uk/body/" + id
	}
	for _, item := range v.Rows(raw[signal.field]) {
		if signal.field == "signals" && strings.EqualFold(spaceText(item["name"]), signal.value) {
			row["count"] = item["count"]
			row["updatedAt"] = v.First(raw["signals_updated_at"], raw["updated_at"])
			return row
		}
		if strings.EqualFold(spaceText(item["type"]), signal.value) || strings.EqualFold(spaceText(item["subtype"]), signal.value) {
			row["count"] = v.Int(row["count"]) + max(1, v.Int(item["count"]))
			if row["latitude"] == nil && item["latitude"] != nil && item["longitude"] != nil {
				row["latitude"], row["longitude"] = item["latitude"], item["longitude"]
			}
		}
	}
	if v.Int(row["count"]) > 0 {
		return row
	}
	return nil
}

func specialRows(raw M) ([]M, error) {
	if v.Arr(raw["results"]) == nil {
		return nil, errors.New("Spansh не вернул результаты поиска.")
	}
	return v.Rows(raw["results"]), nil
}

func specialPageSize(q Query) int {
	if q.Limit <= 0 {
		return DefaultLimit
	}
	return q.Limit
}

func specialHasMore(q Query, raw M) bool {
	return (q.Page+1)*specialPageSize(q) < v.Int(raw["count"])
}

func specialSystemRow(raw M) M {
	system := spaceText(raw["name"])
	id := spaceAddress(raw["id64"])
	row := M{"system": system, "systemAddress": id, "distanceLy": raw["distance"], "updatedAt": raw["updated_at"]}
	if id != "" {
		row["url"] = "https://spansh.co.uk/system/" + id
	}
	return row
}

func matchesName(query string, names ...any) bool {
	if query == "" {
		return true
	}
	query = strings.ToLower(query)
	for _, name := range names {
		if strings.Contains(strings.ToLower(spaceText(name)), query) {
			return true
		}
	}
	return false
}

func (c *Client) specialOriginCoordinates(ctx context.Context, q Query) ([]float64, error) {
	origin := q.OriginCoords
	if len(origin) != 3 {
		origin = engineering.WorkshopCoordinates(q.Origin)
	}
	if len(origin) != 3 && q.Origin != "" {
		index, err := c.request(ctx, "/systems/field_values/system_names?q="+url.QueryEscape(q.Origin), nil)
		if err != nil {
			return nil, err
		}
		for _, candidate := range v.Rows(index["min_max"]) {
			if strings.EqualFold(spaceText(candidate["name"]), q.Origin) {
				if position := specialCoordinates(candidate); len(position) == 3 {
					origin = position
					break
				}
			}
		}
		if len(origin) != 3 {
			return nil, errors.New("Не найдены координаты исходной системы.")
		}
	}
	return origin, nil
}

func (c *Client) searchEngineers(ctx context.Context, q Query) (Result, error) {
	origin, err := c.specialOriginCoordinates(ctx, q)
	if err != nil {
		return Result{}, err
	}
	catalog, err := data.Load("engineering-roadmap.json")
	if err != nil {
		return Result{}, err
	}
	rows := []M{}
	for _, engineer := range v.Rows(v.At(v.Obj(catalog), "engineers")) {
		name, system, station := spaceText(engineer["name"]), spaceText(engineer["system"]), spaceText(engineer["base"])
		if name == "" || system == "" || !matchesName(q.Name, name, engineer["english"], system, station) {
			continue
		}
		row := M{
			"name": name, "nameEn": engineer["english"], "system": system, "station": station,
			"systemAddress": spaceAddress(engineer["systemAddress"]), "referenceId": "engineer/" + spaceText(engineer["key"]),
			"type": engineer["kind"],
		}
		if coordinates := engineering.WorkshopCoordinates(system); len(coordinates) == 3 && len(origin) == 3 {
			distance := math.Hypot(math.Hypot(coordinates[0]-origin[0], coordinates[1]-origin[1]), coordinates[2]-origin[2])
			if radius := v.Num(q.Filters["radiusLy"]); radius > 0 && distance > radius {
				continue
			}
			row["distanceLy"] = distance
		} else if v.Num(q.Filters["radiusLy"]) > 0 {
			continue
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, aKnown := v.Number(rows[i]["distanceLy"])
		b, bKnown := v.Number(rows[j]["distanceLy"])
		if aKnown != bKnown {
			return aKnown
		}
		if a != b {
			return a < b
		}
		return spaceText(rows[i]["name"]) < spaceText(rows[j]["name"])
	})
	from := min(q.Page*specialPageSize(q), len(rows))
	to := min(from+specialPageSize(q), len(rows))
	return Result{Rows: rows[from:to], Total: len(rows), HasMore: to < len(rows), Link: "https://inara.cz/elite/engineers/"}, nil
}

func specialCoordinates(raw M) []float64 {
	x, xKnown := v.Number(raw["x"])
	y, yKnown := v.Number(raw["y"])
	z, zKnown := v.Number(raw["z"])
	if !xKnown || !yKnown || !zKnown {
		return nil
	}
	return []float64{x, y, z}
}

const eliteHubConflictURL = "https://vault.elitehub.eu/graphql"

// EliteHub exposes observed conflict parties by exact system address. Limiting
// the batch to the current search page keeps this one read request per page.
func (c *Client) publicConflicts(ctx context.Context, systems []M) map[string][]M {
	variables := M{}
	declarations, selections := []string{}, []string{}
	candidates := make(map[string]M)
	for _, system := range systems {
		id := spaceAddress(system["id64"])
		if id == "" || len(candidates) >= conflictSystemLimit {
			continue
		}
		alias := "s" + strconv.Itoa(len(candidates))
		variables[alias] = id
		declarations = append(declarations, "$"+alias+":BigInt!")
		selections = append(selections, alias+":systemBySystemAddress(systemAddress:$"+alias+
			"){systemAddress factionConflicts(first:10,orderBy:UPDATED_AT_DESC){nodes{type status updatedAt factionWonDays opponentWonDays faction{name} opponentFaction{name}}}}")
		candidates[alias] = system
	}
	if len(candidates) == 0 {
		return nil
	}
	query := "query NearbyWars(" + strings.Join(declarations, ",") + "){" + strings.Join(selections, " ") + "}"
	raw, err := trading.FetchJSON(ctx, c.HTTP, "POST", eliteHubConflictURL, M{"query": query, "variables": variables})
	if err != nil {
		return nil
	}
	return readPublicConflicts(v.Obj(v.At(v.Obj(raw), "data")), candidates, time.Now())
}

func readPublicConflicts(raw M, candidates map[string]M, now time.Time) map[string][]M {
	out := map[string][]M{}
	for alias, system := range candidates {
		address := spaceAddress(system["id64"])
		record := v.Obj(raw[alias])
		if address == "" || address != spaceAddress(record["systemAddress"]) {
			continue
		}
		seen := map[[2]string]bool{}
		for _, conflict := range v.Rows(v.At(record, "factionConflicts", "nodes")) {
			state := map[string]string{"war": "War", "civilwar": "Civil War"}[v.Key(conflict["type"])]
			if state == "" || !strings.EqualFold(spaceText(conflict["status"]), "active") {
				continue
			}
			at := v.Time(conflict["updatedAt"])
			if at.IsZero() {
				// The API expresses UTC timestamps without a timezone suffix.
				at, _ = time.Parse("2006-01-02T15:04:05.999999999", spaceText(conflict["updatedAt"]))
			}
			if at.IsZero() || now.Sub(at) > 48*time.Hour || at.Sub(now) > time.Minute {
				continue
			}
			faction, opponent := spaceText(v.At(conflict, "faction", "name")), spaceText(v.At(conflict, "opponentFaction", "name"))
			if faction == "" || opponent == "" || strings.EqualFold(faction, opponent) {
				continue
			}
			left, right := strings.ToLower(faction), strings.ToLower(opponent)
			key := [2]string{min(left, right), max(left, right)}
			if seen[key] {
				continue
			}
			seen[key] = true
			row := specialSystemRow(system)
			row["name"], row["faction"], row["opponent"], row["state"] = faction, faction, opponent, state
			row["updatedAt"], row["source"] = v.ISO(at), "EliteHub"
			row["wonDays"], row["opponentWonDays"] = conflict["factionWonDays"], conflict["opponentWonDays"]
			out[address] = append(out[address], row)
		}
	}
	return out
}

func (c *Client) factionNames(ctx context.Context, query string) ([]string, error) {
	raw, err := c.request(ctx, "/systems/field_values/minor_faction_presences/name?q="+url.QueryEscape(query), nil)
	if err != nil {
		return nil, err
	}
	values := v.Arr(v.At(raw, "values", "name"))
	if values == nil {
		return nil, errors.New("Spansh не вернул названия фракций.")
	}
	names := []string{}
	for _, value := range values {
		name := spaceText(value)
		if strings.EqualFold(name, query) {
			return []string{name}, nil
		}
		if len(names) < 20 && matchesName(query, name) {
			names = append(names, name)
		}
	}
	return names, nil
}
