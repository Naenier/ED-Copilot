package nearest

import (
	"context"
	"errors"
	"strconv"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	bodyFactionSystemLimit = 100
	factionStateScanPage   = 100
	factionStateScanPages  = 3
)

func spaceText(input any) string {
	return strings.TrimSpace(v.Str(input))
}

func spaceValues(input any) []string {
	items := v.Arr(input)
	if len(items) == 0 {
		for _, part := range strings.Split(spaceText(input), ",") {
			items = append(items, part)
		}
	}
	values := []string{}
	for _, item := range items {
		label := spaceText(item)
		if label != "" && len(label) <= 180 && len(values) < 15 {
			values = append(values, label)
		}
	}
	return values
}

func setSpaceGroup(filters M, field string, values []string) {
	if len(values) > 0 {
		filters[field] = M{"value": values}
	}
}

func setSpaceRange(filters M, field string, min, max float64) {
	filters[field] = M{"comparison": "<=>", "value": []float64{min, max}}
}

func setSpaceGeometry(filters M, q Query, region string) {
	if q.Region != "" {
		setSpaceGroup(filters, region, []string{q.Region})
	}
	if radius := v.Num(q.Filters["radiusLy"]); radius > 0 && q.Origin != "" {
		filters["distance"] = M{"min": 0, "max": radius}
	}
}

func spaceSystemFilters(q Query) M {
	filters := M{}
	setSpaceGeometry(filters, q, "region")
	for field, key := range map[string]string{
		"allegiance": "systemAllegiance", "government": "systemGovernment", "primary_economy": "systemEconomy",
		"security": "security", "controlling_power": "power", "power_state": "powerState",
	} {
		setSpaceGroup(filters, field, spaceValues(q.Filters[key]))
	}

	low, lowOK := v.Number(q.Filters["populationMin"])
	high, highOK := v.Number(q.Filters["populationMax"])
	const upperPopulationBound = 1e15
	if !highOK {
		high = upperPopulationBound
	}
	switch q.Filters["population"] {
	case "populated":
		low, lowOK = max(1, low), true
	case "unpopulated":
		high, highOK = 0, true
	}
	if lowOK || highOK {
		setSpaceRange(filters, "population", max(0, low), min(upperPopulationBound, high))
	}
	switch q.Filters["colonization"] {
	case "colonized":
		filters["is_colonised"] = M{"value": true}
	case "construction":
		filters["is_being_colonised"] = M{"value": true}
	case "uncolonized":
		filters["is_colonised"] = M{"value": false}
		filters["is_being_colonised"] = M{"value": false}
	}

	if states := spaceValues(q.Filters["factionStates"]); len(states) != 0 {
		logic := "OR"
		switch q.Filters["stateMode"] {
		case "all":
			logic = "AND"
		case "none":
			logic = "NOT"
		}
		filters["controlling_minor_faction_state"] = M{"value": states, "logic": logic}
	}
	if faction := spaceText(q.Filters["presenceFaction"]); faction != "" {
		names := []string{faction}
		presence := M{"name": M{"value": names}}
		switch q.Filters["presenceMode"] {
		case "controlling":
			setSpaceGroup(filters, "controlling_minor_faction", names)
		case "not-controlling":
			filters["controlling_minor_faction"] = M{"value": names, "logic": "NOT"}
			filters["minor_faction_presences"] = []M{presence}
		case "exclude":
			filters["minor_faction_presences"] = []M{{"name": M{"value": names, "logic": "NOT"}}}
		default:
			filters["minor_faction_presences"] = []M{presence}
		}
	}
	return filters
}

func spaceBodyFilters(q Query) M {
	filters := M{}
	setSpaceGeometry(filters, q, "system_region")
	for field, key := range map[string]string{
		"volcanism_type": "volcanism", "atmosphere": "atmosphere", "terraforming_state": "terraforming",
		"reserve_level": "reserve", "system_controlling_power": "power", "system_power_state": "powerState",
	} {
		setSpaceGroup(filters, field, spaceValues(q.Filters[key]))
	}
	var subtypes []string
	subtypes = append(subtypes, spaceValues(q.Filters["starClass"])...)
	subtypes = append(subtypes, spaceValues(q.Filters["planetType"])...)
	setSpaceGroup(filters, "subtype", subtypes)
	if kinds := spaceValues(q.Filters["ringType"]); len(kinds) > 0 {
		field := "rings"
		if q.Filters["ringKind"] == "belt" {
			field = "belts"
		}
		filters[field] = []M{{"type": kinds}}
	}
	if kinds := spaceValues(q.Filters["beltType"]); len(kinds) > 0 {
		filters["belts"] = []M{{"type": kinds}}
	}
	for field, key := range map[string]string{
		"is_landable": "landable", "is_rotational_period_tidally_locked": "tidalLock",
	} {
		if v.Bool(q.Filters[key]) {
			filters[field] = M{"value": true}
		}
	}
	if materials := spaceValues(q.Filters["materials"]); len(materials) > 0 {
		filters["materials"] = []M{}
		rows := make([]M, 0, len(materials))
		for _, name := range materials {
			rows = append(rows, M{"name": name, "share": []float64{0.000001, 100}, "comparison": "<=>"})
		}
		filters["materials"] = rows
	}
	return filters
}

func spaceSort(q Query) []M {
	order := []M{}
	switch q.Sort {
	case "population":
		if q.Category == "systems" {
			order = append(order, M{"population": M{"direction": "desc"}})
		}
	case "arrival":
		if q.Category == "bodies" {
			order = append(order, M{"distance_to_arrival": M{"direction": "asc"}})
		}
	case "materials":
		if q.Category == "bodies" {
			sorts := []M{}
			for _, name := range spaceValues(q.Filters["materials"]) {
				sorts = append(sorts, M{"name": name, "direction": "desc"})
			}
			if len(sorts) > 0 {
				order = append(order, M{"materials": sorts})
			}
		}
	}
	if q.Origin != "" {
		order = append(order, M{"distance": M{"direction": "asc"}})
	}
	return append(order, M{"name": M{"direction": "asc"}})
}

func spacePayload(q Query, filters M) M {
	size := q.Limit
	if size < 1 {
		size = 24
	}
	out := M{"filters": filters, "sort": spaceSort(q), "size": size, "page": q.Page}
	if q.Origin != "" {
		out["reference_system"] = q.Origin
	}
	return out
}

func (c *Client) searchSpace(ctx context.Context, q Query) (Result, error) {
	result := Result{Rows: []M{}}
	filters := spaceSystemFilters(q)
	normalize := spaceSystemRow
	if q.Category == "bodies" {
		filters = spaceBodyFilters(q)
		normalize = spaceBodyRow
		if spaceText(q.Filters["presenceFaction"]) != "" {
			ids, partial, err := c.spaceBodyFactionIDs(ctx, q)
			if err != nil {
				return result, err
			}
			if partial {
				result.Note = "Поиск тел охватывает первые 100 систем, подходящих по фракции."
			}
			if len(ids) == 0 {
				return result, nil
			}
			filters["system_id64"] = M{"value": ids}
		}
	}
	if q.Name != "" {
		filters["name"] = M{"value": q.Name}
	}
	if states := spaceValues(q.Filters["factionStates"]); q.Category == "systems" && len(states) > 0 {
		return c.searchSpaceFactionStates(ctx, q, filters, states)
	}
	payload := spacePayload(q, filters)
	raw, err := c.request(ctx, "/"+q.Category+"/search", payload)
	if err != nil {
		return result, err
	}
	rows := v.Arr(raw["results"])
	if rows == nil {
		return result, errors.New("Spansh не вернул результаты поиска.")
	}
	for _, value := range rows {
		if row := normalize(v.Obj(value)); row != nil {
			result.Rows = append(result.Rows, row)
		}
	}
	result.Total = max(len(result.Rows), v.Int(raw["count"]))
	result.HasMore = len(rows) == v.Int(payload["size"]) && (q.Page+1)*v.Int(payload["size"]) < result.Total
	return result, nil
}

// Spansh indexes one primary state per controlling faction. Read the actual
// active states returned with each system when matching all/any/none.
func (c *Client) searchSpaceFactionStates(ctx context.Context, q Query, filters M, selected []string) (Result, error) {
	delete(filters, "controlling_minor_faction_state")
	result := Result{Rows: []M{}}
	request := q
	request.Limit = factionStateScanPage
	matches := []M{}
	mode := v.Str(q.Filters["stateMode"])
	for page := 0; page < factionStateScanPages; page++ {
		request.Page = page
		raw, err := c.request(ctx, "/systems/search", spacePayload(request, filters))
		if err != nil {
			return result, err
		}
		rows := v.Arr(raw["results"])
		if rows == nil {
			return result, errors.New("Spansh не вернул результаты поиска.")
		}
		for _, item := range rows {
			row := v.Obj(item)
			if !spaceMatchesFactionStates(row, selected, mode) {
				continue
			}
			if normalized := spaceSystemRow(row); normalized != nil {
				matches = append(matches, normalized)
			}
		}
		if len(rows) < factionStateScanPage || (page+1)*factionStateScanPage >= v.Int(raw["count"]) {
			break
		}
		if page == factionStateScanPages-1 {
			result.Note = "Состояния фракций проверены в первых 300 системах, подходящих по остальным фильтрам. Сузьте радиус или добавьте критерий."
		}
	}
	limit := q.Limit
	if limit < 1 {
		limit = DefaultLimit
	}
	result.Total = len(matches)
	start := min(max(0, q.Page)*limit, result.Total)
	end := min(start+limit, result.Total)
	result.Rows, result.HasMore = matches[start:end], end < result.Total
	return result, nil
}

func spaceMatchesFactionStates(system M, selected []string, mode string) bool {
	states := map[string]bool{}
	if primary := v.Key(system["controlling_minor_faction_state"]); primary != "" {
		states[primary] = true
	}
	faction := v.Str(system["controlling_minor_faction"])
	for _, presence := range v.Rows(system["minor_faction_presences"]) {
		if faction != "" && strings.EqualFold(faction, v.Str(presence["name"])) {
			for _, state := range v.Arr(presence["active_states"]) {
				states[v.Key(state)] = true
			}
			break
		}
	}
	matches := 0
	for _, state := range selected {
		if states[v.Key(state)] {
			matches++
		}
	}
	switch mode {
	case "all":
		return matches == len(selected)
	case "none":
		return matches == 0
	default:
		return matches > 0
	}
}
func (c *Client) spaceBodyFactionIDs(ctx context.Context, q Query) ([]string, bool, error) {
	q.Category, q.Page, q.Limit, q.Sort = "systems", 0, bodyFactionSystemLimit, ""
	raw, err := c.request(ctx, "/systems/search", spacePayload(q, spaceSystemFilters(q)))
	if err != nil {
		return nil, false, err
	}
	rows := v.Arr(raw["results"])
	if rows == nil {
		return nil, false, errors.New("Spansh не вернул системы с выбранной фракцией.")
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, system := range v.Rows(raw["results"]) {
		id := spaceAddress(system["id64"])
		if id != "" && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids, v.Int(raw["count"]) > len(rows), nil
}

func spaceAddress(value any) string {
	id := v.Str(value)
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return ""
	}
	return id
}

func spaceRowFields(row, raw M, fields map[string]string) {
	for key, field := range fields {
		if value, exists := raw[field]; exists && value != nil {
			row[key] = value
		}
	}
}

func spaceSystemRow(raw M) M {
	name, address := v.Str(raw["name"]), spaceAddress(raw["id64"])
	if name == "" || address == "" {
		return nil
	}
	row := M{"name": name, "system": name, "systemAddress": address, "url": "https://spansh.co.uk/system/" + address}
	spaceRowFields(row, raw, map[string]string{
		"x": "x", "y": "y", "z": "z", "distanceLy": "distance", "population": "population", "region": "region",
		"allegiance": "allegiance", "government": "government", "economy": "primary_economy", "security": "security",
		"faction": "controlling_minor_faction", "state": "controlling_minor_faction_state", "power": "controlling_power",
		"powerState": "power_state", "updatedAt": "updated_at", "bodyCount": "body_count", "permitRequired": "needs_permit",
		"colonized": "is_colonised", "construction": "is_being_colonised",
	})
	return row
}

func spaceBodyRow(raw M) M {
	name, system := v.Str(raw["name"]), v.Str(raw["system_name"])
	address, bodyAddress := spaceAddress(raw["system_id64"]), spaceAddress(raw["id64"])
	if name == "" || system == "" || address == "" {
		return nil
	}
	row := M{"name": name, "system": system, "systemAddress": address, "type": v.Text(raw["subtype"], raw["type"])}
	if bodyAddress != "" {
		row["bodyAddress"], row["url"] = bodyAddress, "https://spansh.co.uk/body/"+bodyAddress
	}
	spaceRowFields(row, raw, map[string]string{
		"x": "system_x", "y": "system_y", "z": "system_z", "bodyId": "body_id", "distanceLy": "distance",
		"distanceLs": "distance_to_arrival", "region": "system_region", "landable": "is_landable", "gravity": "gravity",
		"atmosphere": "atmosphere", "terraforming": "terraforming_state", "reserve": "reserve_level", "volcanism": "volcanism_type",
		"tidalLock": "is_rotational_period_tidally_locked", "power": "system_controlling_power", "powerState": "system_power_state",
		"estimatedScanValue": "estimated_scan_value", "estimatedMappingValue": "estimated_mapping_value", "updatedAt": "updated_at",
		"materials": "materials", "rings": "rings", "belts": "belts", "signals": "signals", "signalCount": "signal_count",
	})
	return row
}
