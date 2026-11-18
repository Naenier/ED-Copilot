package nearest

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const stationSystemLimit = 200
const stationReviewLimit = 6
const stationPlayerLookupLimit = 8

//go:embed catalog_prices.json
var stationPriceJSON []byte

var stationPrices = func() struct {
	Ships, Modules map[string]int64
} {
	var prices struct{ Ships, Modules map[string]int64 }
	if err := json.Unmarshal(stationPriceJSON, &prices); err != nil {
		panic(err)
	}
	return prices
}()

type stationOutfit struct {
	kind, name, rating, mount string
	size                      int
	hasSize                   bool
}

type stationCriteria struct {
	items           []stationOutfit
	personal        bool
	discounted      bool
	ownerStates     []string
	ownerStateMode  string
	ownerControlled bool
	ownerPlayer     bool
	ownerAllegiance string
	ownerGovernment string
	stronghold      string
	pledgedPower    string
	minPad          string
	hasSystemJoin   bool
}

func stationValues(raw any, limit int) []string {
	fields := v.Arr(raw)
	if name, ok := raw.(string); ok {
		fields = []any{name}
	}
	names := make([]string, 0, min(len(fields), limit))
	for _, field := range fields {
		name := strings.TrimSpace(v.Str(field))
		if name != "" && validText(name, 150) {
			names = append(names, name)
		}
		if len(names) >= limit {
			break
		}
	}
	return names
}

func stationText(raw any) string {
	if names := stationValues(raw, 1); len(names) != 0 {
		return names[0]
	}
	return ""
}

func stationValue(value any) M { return M{"value": value} }

func stationGroup(value any) M {
	return stationValue(stationValues(value, 30))
}

func stationNumberRange(minimum, maximum float64) M {
	return M{"value": []float64{minimum, maximum}, "comparison": "<=>"}
}

func stationFilters(q Query) (M, stationCriteria, error) {
	input := q.Filters
	criteria := stationCriteria{
		personal: v.Bool(input["personalEquipment"]), discounted: v.Bool(input["discountedOnly"]),
		ownerStates: stationValues(input["ownerStates"], 20), ownerStateMode: stationText(input["ownerStateMode"]),
		ownerControlled: v.Bool(input["ownerControllingOnly"]), ownerPlayer: v.Bool(input["ownerPlayerOnly"]),
		ownerAllegiance: stationText(input["ownerAllegiance"]), ownerGovernment: stationText(input["ownerGovernment"]),
		stronghold: stationText(input["stronghold"]), pledgedPower: stationText(input["pledgedPower"]),
		minPad: strings.ToUpper(stationText(input["minPad"])),
	}
	if v.Bool(input["ignoreStrongholdCarriers"]) {
		criteria.stronghold = "exclude"
	}
	criteria.hasSystemJoin = q.Region != "" ||
		stationText(input["systemAllegiance"]) != "" || stationText(input["systemGovernment"]) != "" ||
		stationText(input["security"]) != "" || stationText(input["presenceFaction"]) != ""
	filters := M{}
	if q.Name != "" {
		filters["name"] = stationValue(q.Name)
	}
	if radius, ok := v.Number(input["radiusLy"]); q.Origin != "" && ok && radius > 0 {
		filters["distance"] = M{"min": 0, "max": min(radius, 100000)}
	}
	if maxArrival, ok := v.Number(input["maxArrivalLs"]); ok && maxArrival > 0 {
		filters["distance_to_arrival"] = stationNumberRange(0, min(maxArrival, 1e8))
	}
	for form, remote := range map[string]string{
		"allegiance": "allegiance", "government": "government", "economy": "primary_economy",
		"systemEconomy": "system_primary_economy", "power": "system_controlling_power",
		"powerState": "system_power_state", "materialTrader": "material_trader", "technologyBroker": "technology_broker",
	} {
		if data := stationValues(input[form], 30); len(data) > 0 {
			filters[remote] = stationValue(data)
		}
	}
	if owner := stationText(input["owner"]); owner != "" {
		term := stationGroup(owner)
		if input["ownerMode"] == "not" {
			term["logic"] = "not"
		}
		filters["controlling_minor_faction"] = term
	}
	minimum, minimumKnown := v.Number(input["populationMin"])
	maximum, maximumKnown := v.Number(input["populationMax"])
	const populationLimit = 1e15
	if !maximumKnown {
		maximum = populationLimit
	}
	switch input["population"] {
	case "populated":
		minimum, minimumKnown = max(1, minimum), true
	case "unpopulated":
		maximum, maximumKnown = 0, true
	}
	if minimumKnown || maximumKnown {
		filters["system_population"] = stationNumberRange(v.Clamp(minimum, 0, populationLimit), v.Clamp(maximum, 0, populationLimit))
	}
	switch input["colonization"] {
	case "colonized":
		filters["system_is_colonised"] = stationValue(true)
	case "construction", "building":
		filters["system_is_being_colonised"] = stationValue(true)
	case "uncolonized":
		filters["system_is_colonised"] = stationValue(false)
		filters["system_is_being_colonised"] = stationValue(false)
	}
	var excluded []string
	if v.Bool(input["ignoreCarriers"]) {
		excluded = append(excluded, "Drake-Class Carrier")
	}
	switch input["surface"] {
	case "no":
		filters["is_planetary"] = stationValue(false)
	case "legacy":
		excluded = append(excluded, "Settlement", "Surface Settlement")
	}
	if kind := stationText(input["stationType"]); kind != "" {
		for _, name := range excluded {
			if strings.EqualFold(kind, name) {
				return nil, criteria, errors.New("Тип станции исключён остальными фильтрами.")
			}
		}
		filters["type"] = stationGroup(kind)
	} else if len(excluded) > 0 {
		filters["type"] = M{"value": excluded, "logic": "not"}
	}
	if criteria.minPad == "L" {
		filters["has_large_pad"] = stationValue(true)
	}
	services := stationValues(input["services"], 24)
	if criteria.personal {
		services = append(services, "Pioneer Supplies")
	}
	if len(services) > 0 {
		groups := make([]M, 0, len(services))
		seen := make(map[string]bool)
		for _, service := range services {
			if !seen[service] {
				groups = append(groups, M{"name": stationGroup(service)})
				seen[service] = true
			}
		}
		filters["services"] = groups
	}
	if q.Category != "outfitting" {
		return filters, criteria, nil
	}
	for _, ship := range stationValues(input["ships"], 30) {
		criteria.items = append(criteria.items, stationOutfit{kind: "ship", name: ship})
	}
	for _, raw := range v.Rows(input["modules"]) {
		name := stationText(raw["name"])
		if name == "" {
			continue
		}
		size, hasSize := v.Number(raw["class"])
		criteria.items = append(criteria.items, stationOutfit{
			kind: "module", name: name, size: int(size), hasSize: hasSize && size >= 0 && size <= 8 && size == float64(int(size)),
			rating: strings.ToUpper(stationText(raw["rating"])), mount: stationMount(stationText(raw["mount"])),
		})
		if len(criteria.items) >= 36 {
			break
		}
	}
	if len(criteria.items) == 0 && !criteria.personal && len(services) == 0 {
		return nil, criteria, errors.New("Выберите корабль, модуль или отдел персонального снаряжения.")
	}
	if criteria.discounted && len(criteria.items) == 0 {
		return nil, criteria, errors.New("Выберите корабль или модуль для поиска скидок.")
	}
	for _, item := range criteria.items {
		field := "ships"
		term := M{"name": stationGroup(item.name)}
		if item.kind == "module" {
			field = "modules"
			if item.hasSize {
				term["class"] = stationValue([]int{item.size})
			}
			if item.rating != "" {
				term["rating"] = stationGroup(item.rating)
			}
		}
		// Combined filters are ANDed across each independently selected item.
		filters[field] = append(v.Rows(filters[field]), term)
	}
	return filters, criteria, nil
}

func stationMount(value string) string {
	switch strings.ToLower(value) {
	case "f", "fixed":
		return "fixed"
	case "g", "gimbal", "gimballed":
		return "gimballed"
	case "t", "turret", "turreted":
		return "turreted"
	}
	return ""
}

func stockMount(raw M) string {
	if mount := stationMount(v.Str(raw["mount"])); mount != "" {
		return mount
	}
	for _, part := range strings.Split(v.Str(raw["ed_symbol"]), "_") {
		if mount := stationMount(part); mount != "" {
			return mount
		}
	}
	return ""
}

func stationStock(row M, criteria stationCriteria) ([]M, bool) {
	out := make([]M, 0, len(criteria.items)+1)
	discount := false
	for _, item := range criteria.items {
		field, reported := "ships", row["shipyard_updated_at"]
		if item.kind == "module" {
			field, reported = "modules", row["outfitting_updated_at"]
		}
		var matches []M
		for _, entry := range v.Rows(row[field]) {
			if !strings.EqualFold(item.name, v.Str(entry["name"])) {
				continue
			}
			mount := stockMount(entry)
			if item.kind == "module" && (item.hasSize && v.Int(entry["class"]) != item.size ||
				item.rating != "" && entry["rating"] != item.rating || item.mount != "" && mount != item.mount) {
				continue
			}
			var normalPrice int64
			symbol := v.Text(entry["ed_symbol"], entry["symbol"])
			if item.kind == "ship" {
				normalPrice = stationPrices.Ships[v.Key(entry["name"])]
			} else {
				normalPrice = stationPrices.Modules[v.Key(symbol)]
			}
			price, hasPrice := v.Number(entry["price"])
			itemDiscount := hasPrice && price > 0 && normalPrice > 0 && price+1 < float64(normalPrice)
			discount = discount || itemDiscount
			matches = append(matches, M{"kind": item.kind, "name": entry["name"], "class": entry["class"], "rating": entry["rating"],
				"mount": mount, "symbol": symbol, "price": entry["price"], "basePrice": v.Nullable(float64(normalPrice), normalPrice > 0),
				"discounted": itemDiscount, "updatedAt": reported})
		}
		if len(matches) == 0 {
			return nil, false
		}
		out = append(out, matches[:min(8, len(matches))]...)
	}
	if criteria.personal {
		out = append(out, M{"kind": "service", "name": "Pioneer Supplies"})
	}
	return out, !criteria.discounted || discount
}

func stationPadClass(raw M) string {
	if v.Bool(raw["has_large_pad"]) || v.Num(raw["large_pads"]) > 0 {
		return "L"
	}
	if v.Num(raw["medium_pads"]) > 0 {
		return "M"
	}
	if v.Num(raw["small_pads"]) > 0 {
		return "S"
	}
	return ""
}

func stationRow(raw M, items []M) M {
	name, system := v.Str(raw["name"]), v.Str(raw["system_name"])
	if name == "" || system == "" {
		return nil
	}
	market := v.Text(raw["market_id"], raw["id"])
	services := []string{}
	for _, entry := range v.Rows(raw["services"]) {
		if text := v.Str(entry["name"]); text != "" {
			services = append(services, text)
		}
	}
	row := M{"name": name, "station": name, "system": system, "systemAddress": v.Str(raw["system_id64"]), "marketId": market,
		"type": raw["type"], "distanceLy": raw["distance"], "distanceLs": raw["distance_to_arrival"], "pad": stationPadClass(raw),
		"faction": raw["controlling_minor_faction"], "state": raw["controlling_minor_faction_state"], "government": raw["government"],
		"allegiance": raw["allegiance"], "economy": raw["primary_economy"], "isPlanetary": raw["is_planetary"], "updatedAt": raw["updated_at"],
		"services": services, "materialTrader": raw["material_trader"], "technologyBroker": raw["technology_broker"],
		"power": raw["system_controlling_power"], "powerState": raw["system_power_state"],
		"body": raw["body_name"], "items": items}
	if _, err := strconv.ParseUint(market, 10, 64); err == nil {
		row["url"] = "https://spansh.co.uk/station/" + url.PathEscape(market)
	}
	return row
}

func (filter stationCriteria) needsFactionDetails() bool {
	return filter.ownerControlled || filter.ownerAllegiance != "" || filter.ownerGovernment != "" || len(filter.ownerStates) > 0
}

func (filter stationCriteria) reviewsResults() bool {
	if filter.needsFactionDetails() || filter.ownerPlayer || filter.discounted ||
		filter.minPad == "S" || filter.minPad == "M" || filter.stronghold == "exclude" || filter.stronghold == "friendly" {
		return true
	}
	for _, item := range filter.items {
		if item.mount != "" {
			return true
		}
	}
	return false
}

func (filter stationCriteria) matchesStation(raw M) bool {
	padRank := map[string]int{"S": 1, "M": 2, "L": 3}
	if padRank[stationPadClass(raw)] < padRank[filter.minPad] {
		return false
	}
	if strings.EqualFold(v.Str(raw["name"]), "Stronghold Carrier") {
		if filter.stronghold == "exclude" || filter.stronghold == "friendly" &&
			(filter.pledgedPower == "" || !strings.EqualFold(filter.pledgedPower, v.Str(raw["system_controlling_power"]))) {
			return false
		}
	}
	return true
}

func stationFactionMatches(station, system M, filter stationCriteria) bool {
	if !filter.needsFactionDetails() {
		return true
	}
	owner := v.Str(station["controlling_minor_faction"])
	if owner == "" || filter.ownerControlled && !strings.EqualFold(owner, v.Str(system["controlling_minor_faction"])) {
		return false
	}
	var faction M
	for _, entry := range v.Rows(system["minor_faction_presences"]) {
		if strings.EqualFold(owner, v.Str(entry["name"])) {
			faction = entry
			break
		}
	}
	if filter.ownerAllegiance != "" && !strings.EqualFold(filter.ownerAllegiance, v.Str(faction["allegiance"])) ||
		filter.ownerGovernment != "" && !strings.EqualFold(filter.ownerGovernment, v.Str(faction["government"])) {
		return false
	}
	if len(filter.ownerStates) == 0 {
		return true
	}
	states := make(map[string]bool)
	for _, state := range stationValues(faction["active_states"], 30) {
		states[strings.ToLower(state)] = true
	}
	if state := v.Text(faction["state"], station["controlling_minor_faction_state"]); state != "" {
		states[strings.ToLower(state)] = true
	}
	if len(states) == 0 {
		return false
	}
	matched := 0
	for _, state := range filter.ownerStates {
		if states[strings.ToLower(state)] {
			matched++
		}
	}
	switch filter.ownerStateMode {
	case "any":
		return matched > 0
	case "none":
		return matched == 0
	default:
		return matched == len(filter.ownerStates)
	}
}

func stationBatch(q Query, filters M, page, limit int) M {
	field := "distance"
	switch q.Sort {
	case "arrival", "distanceLs":
		field = "distance_to_arrival"
	case "name":
		field = "name"
	default:
		if q.Origin == "" {
			field = "name"
		}
	}
	payload := M{"filters": filters, "sort": []M{{field: M{"direction": "asc"}}}, "page": page, "size": limit}
	if len(q.OriginCoords) == 3 {
		payload["reference_coords"] = M{"x": q.OriginCoords[0], "y": q.OriginCoords[1], "z": q.OriginCoords[2]}
	} else if q.Origin != "" {
		payload["reference_system"] = q.Origin
	}
	return payload
}

func (c *Client) stationEligibleSystems(ctx context.Context, q Query, stationFields M) ([]string, bool, error) {
	// The station index has only selected system fields. Apply the remaining
	// conditions against the system index before querying stations by address.
	q.Name, q.Category, q.Sort = "", "systems", ""
	fields := spaceSystemFilters(q)
	station := M{}
	for _, field := range []string{"name", "type", "distance_to_arrival", "controlling_minor_faction"} {
		if condition, ok := stationFields[field]; ok {
			station[field] = condition
		}
	}
	if len(station) == 0 {
		// Restrict the bounded join to systems containing a known port.
		station["type"] = stationValue([]string{
			"Asteroid base", "Coriolis Starport", "Dockable Planet Station", "Dodec Starport",
			"Drake-Class Carrier", "Mega ship", "Ocellus Starport", "Orbis Starport", "Outpost",
			"Planetary Construction Depot", "Planetary Outpost", "Planetary Port",
			"Settlement", "Space Construction Depot", "Surface Settlement",
		})
	}
	fields["stations"] = []M{station}
	response, err := c.request(ctx, "/systems/search", stationBatch(q, fields, 0, stationSystemLimit))
	if err != nil {
		return nil, false, err
	}
	rows := v.Rows(response["results"])
	ids := make([]string, 0, len(rows))
	for _, system := range rows {
		if id := v.Str(system["id64"]); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, v.Int(response["count"]) > len(rows), nil
}

func (c *Client) stationFactionView(ctx context.Context, raw []M, cached map[string]M) error {
	ids := make([]string, 0, len(raw))
	seen := make(map[string]bool)
	for _, row := range raw {
		id := v.Str(row["system_id64"])
		if id != "" && !seen[id] && cached[id] == nil {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if len(ids) == 0 {
		return nil
	}
	response, err := c.request(ctx, "/systems/search", M{"filters": M{"id64": stationValue(ids)}, "size": len(ids), "page": 0})
	if err != nil {
		return err
	}
	for _, row := range v.Rows(response["results"]) {
		if id := v.Str(row["id64"]); id != "" {
			cached[id] = row
		}
	}
	return nil
}

func (c *Client) searchStations(ctx context.Context, q Query) (Result, error) {
	filters, criteria, err := stationFilters(q)
	if err != nil {
		return Result{Note: err.Error()}, nil
	}
	notes := make([]string, 0, 4)
	if criteria.personal {
		notes = append(notes, "Показаны станции с магазином Pioneer Supplies. Наличие конкретных костюмов и оружия в открытом каталоге неизвестно.")
	}
	if criteria.discounted {
		notes = append(notes, "Скидки рассчитаны по опубликованным ценам и базовой стоимости выбранных кораблей и модулей.")
	}
	if criteria.stronghold == "friendly" && criteria.pledgedPower == "" {
		notes = append(notes, "Укажите державу командира для поиска её носителей укрепления.")
	}
	if criteria.hasSystemJoin {
		ids, limited, err := c.stationEligibleSystems(ctx, q, filters)
		if err != nil {
			return Result{}, err
		}
		if len(ids) == 0 {
			return Result{Note: strings.Join(notes, " ")}, nil
		}
		filters["system_id64"] = stationValue(ids)
		if limited {
			notes = append(notes, "Проверены станции в первых 200 системах, подходящих по дополнительным фильтрам.")
		}
	}
	limit := q.Limit
	if limit < 1 || limit > DefaultLimit {
		limit = DefaultLimit
	}
	if !criteria.reviewsResults() {
		response, err := c.request(ctx, "/stations/search", stationBatch(q, filters, q.Page, limit))
		if err != nil {
			return Result{}, err
		}
		rows := []M{}
		for _, raw := range v.Rows(response["results"]) {
			if items, matched := stationStock(raw, criteria); matched {
				if row := stationRow(raw, items); row != nil {
					rows = append(rows, row)
				}
			}
		}
		total := v.Int(response["count"])
		return Result{Rows: rows, Total: total, HasMore: (q.Page+1)*limit < total, Note: strings.Join(notes, " ")}, nil
	}

	// The consolidated provider page can include several mounts or multiple
	// active faction states. Validate those observations before slicing pages.
	target, checked, remoteCount := (q.Page+1)*limit+1, 0, 0
	systems, players := make(map[string]M), make(map[string]map[string]bool)
	all := []M{}
	lookupLimited := false
	for page := 0; page < stationReviewLimit && len(all) < target; page++ {
		response, err := c.request(ctx, "/stations/search", stationBatch(q, filters, page, 48))
		if err != nil {
			if page == 0 || ctx.Err() != nil {
				return Result{}, err
			}
			break
		}
		raw := v.Rows(response["results"])
		remoteCount = v.Int(response["count"])
		checked += len(raw)
		if criteria.needsFactionDetails() {
			if err := c.stationFactionView(ctx, raw, systems); err != nil {
				return Result{}, err
			}
		}
		for _, entry := range raw {
			if !criteria.matchesStation(entry) || !stationFactionMatches(entry, systems[v.Str(entry["system_id64"])], criteria) {
				continue
			}
			items, matched := stationStock(entry, criteria)
			if !matched {
				continue
			}
			if criteria.ownerPlayer {
				system, owner := v.Str(entry["system_name"]), strings.ToLower(v.Str(entry["controlling_minor_faction"]))
				if players[system] == nil {
					if len(players) >= stationPlayerLookupLimit || c.EDSM == nil {
						lookupLimited = true
						continue
					}
					players[system] = make(map[string]bool)
					factions, err := c.EDSM.Factions(ctx, system)
					if err != nil {
						if ctx.Err() != nil {
							return Result{}, ctx.Err()
						}
						lookupLimited = true
						continue
					}
					for _, faction := range v.Rows(factions["factions"]) {
						players[system][strings.ToLower(v.Str(faction["name"]))] = v.Bool(faction["isPlayer"])
					}
				}
				if !players[system][owner] {
					continue
				}
			}
			if row := stationRow(entry, items); row != nil {
				all = append(all, row)
			}
		}
		if len(raw) < 48 || checked >= remoteCount {
			break
		}
	}
	if lookupLimited {
		notes = append(notes, "Статус фракции игроков подтверждён через EDSM для части систем.")
	}
	if checked < remoteCount && len(all) < target {
		notes = append(notes, "Дополнительные условия проверены по ограниченной выборке ближайших станций.")
	}
	start, end := min(q.Page*limit, len(all)), min((q.Page+1)*limit, len(all))
	return Result{Rows: all[start:end], Total: len(all), HasMore: len(all) > end, Note: strings.Join(notes, " ")}, nil
}
