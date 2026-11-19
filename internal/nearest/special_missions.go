package nearest

import (
	"context"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const edToolsMissionURL = "https://edtools.cc"

var missionTable = regexp.MustCompile(`(?is)<table\b[^>]*\bid\s*=\s*["']?sys_tbl["']?\s*[^>]*>(.*?)</table>`)
var missionRows = regexp.MustCompile(`(?is)<tr\b[^>]*>(.*?)</tr>`)
var missionCells = regexp.MustCompile(`(?is)<td\b[^>]*>(.*?)</td>`)
var missionMarkup = regexp.MustCompile(`(?s)<[^>]+>`)

func (c *Client) edToolsMissions(ctx context.Context, page string, parameters url.Values) ([][]string, error) {
	base := edToolsMissionURL
	if c.missionSearchURL != "" {
		base = c.missionSearchURL
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+page+"?"+parameters.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ED Copilot (+https://github.com/Naenier/ED-Copilot)")
	response, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	const maxDocument = 4 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(response.Body, maxDocument+1))
	if err != nil || response.StatusCode != http.StatusOK || len(data) > maxDocument {
		return nil, errors.New("ED Tools не вернул результаты поиска заданий.")
	}
	table := missionTable.FindSubmatch(data)
	if len(table) == 0 {
		return nil, errors.New("ED Tools не вернул результаты поиска заданий.")
	}
	rows := [][]string{}
	for _, row := range missionRows.FindAllSubmatch(table[1], -1) {
		cells := missionCells.FindAllSubmatch(row[1], -1)
		if len(cells) == 0 {
			continue
		}
		columns := make([]string, len(cells))
		for i, cell := range cells {
			columns[i] = strings.Join(strings.Fields(html.UnescapeString(missionMarkup.ReplaceAllString(string(cell[1]), " "))), " ")
		}
		rows = append(rows, columns)
	}
	return rows, nil
}

func (c *Client) searchMassacre(ctx context.Context, q Query) (Result, error) {
	limit := v.Num(q.Filters["radiusLy"])
	if limit <= 0 {
		limit = 100
	}
	sources := v.Int(q.Filters["minimumSources"])
	if sources < 1 || sources > 5 {
		sources = 1
	}
	large := q.Filters["missionPad"] == "L"
	params := url.Values{"s": {q.Origin}, "md": {strconv.Itoa(int(max(30, min(250, limit))))}, "sc": {strconv.Itoa(sources)}}
	if large {
		params.Set("lo", "on")
	}
	observed, err := c.edToolsMissions(ctx, "/pve", params)
	if err != nil {
		return missionSourceUnavailable(ctx, q)
	}
	rows := []M{}
	for _, cells := range observed {
		if len(cells) != 11 {
			continue
		}
		distance, known := missionNumber(cells[0])
		system, target := cells[1], cells[9]
		target, count, _ := strings.Cut(target, " / ")
		if !known || distance > limit || large && cells[2] == "" ||
			!validText(system, 150) || !validText(target, 150) || system == "" || target == "" ||
			!matchesName(q.Name, system, target) {
			continue
		}
		row := M{"system": system, "distanceLy": distance, "destinationSystem": target, "huntingSites": cells[10]}
		for i, pad := range []string{"L", "M", "S"} {
			if cells[i+2] != "" {
				row["pad"] = pad
				if arrival, hasArrival := missionNumber(cells[i+2]); hasArrival {
					row["distanceLs"] = arrival
				}
				break
			}
		}
		if supporters, ok := missionNumber(strings.Trim(count, "[] ")); ok {
			if supporters < float64(sources) {
				continue
			}
			row["sourceSystems"] = supporters
		} else if sources > 1 {
			continue
		}
		rows = append(rows, row)
	}
	note := "Системы выдачи заданий и цели по каталогу ED Tools. Наличие заданий зависит от доски миссий."
	if limit > 250 {
		note = "ED Tools проверяет системы в пределах 250 св. л. Наличие заданий зависит от доски миссий."
	}
	return pageMissions(q, rows, note), nil
}

func (c *Client) searchMissionCandidates(ctx context.Context, q Query, kind string) (Result, error) {
	if kind == "restore" {
		return c.searchRestorationCandidates(ctx, q)
	}
	params := url.Values{"s": {q.Origin}}
	if q.Filters["missionPad"] == "L" {
		params.Set("lp", "on")
	}
	observed, err := c.edToolsMissions(ctx, "/minmis", params)
	if err != nil {
		return missionSourceUnavailable(ctx, q)
	}
	rows := []M{}
	var system string
	var distance float64
	for _, cells := range observed {
		if len(cells) != 8 {
			continue
		}
		if cells[1] != "" {
			system = ""
			if n, known := missionNumber(cells[0]); known && validText(cells[1], 150) {
				system, distance = cells[1], n
			}
		}
		station, pad := cells[2], strings.ToUpper(cells[4])
		if system == "" || station == "" || !validText(station, 180) || !matchesName(q.Name, system, station) ||
			(v.Num(q.Filters["radiusLy"]) > 0 && distance > v.Num(q.Filters["radiusLy"])) {
			continue
		}
		if q.Filters["missionPad"] == "L" && pad != "L" {
			continue
		}
		row := M{"system": system, "name": station, "station": station, "pad": pad, "type": cells[6], "economy": cells[5], "distanceLy": distance}
		if arrival, ok := missionNumber(cells[3]); ok {
			row["distanceLs"] = arrival
		}
		state, _, _ := strings.Cut(cells[7], " (")
		row["states"] = spaceValues(state)
		rows = append(rows, row)
	}
	return pageMissions(q, rows, "Кандидаты по каталогу ED Tools. Наличие заданий зависит от доски миссий."), nil
}

func missionNumber(text string) (float64, bool) {
	number, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", ""), 64)
	return number, err == nil && number >= 0 && number < 1e11
}

func pageMissions(q Query, rows []M, note string) Result {
	size := specialPageSize(q)
	from := min(q.Page*size, len(rows))
	to := min(from+size, len(rows))
	return Result{Rows: rows[from:to], Total: len(rows), HasMore: to < len(rows), Note: note}
}

func missionSourceUnavailable(ctx context.Context, q Query) (Result, error) {
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	return Result{Note: "Каталог заданий ED Tools временно недоступен.", Link: InaraLink(q)}, nil
}

func (c *Client) searchRestorationCandidates(ctx context.Context, q Query) (Result, error) {
	// In Live, a negative operating state can leave an Odyssey settlement
	// unpowered; settlements involved in active wars serve ground combat.
	// This query finds plausible settlements to check on a mission board.
	negative := []string{"Infrastructure Failure", "Civil Unrest", "Bust", "Lockdown", "Famine", "Outbreak", "Drought", "Blight", "Natural Disaster", "Terrorist Attack", "Pirate Attack"}
	filters := spaceSystemFilters(q)
	filters["minor_faction_presences"] = []M{{"state": M{"value": negative}}}
	raw, err := c.request(ctx, "/systems/search", spacePayload(q, filters))
	if err != nil {
		return Result{}, err
	}
	systems, err := specialRows(raw)
	if err != nil {
		return Result{}, err
	}
	result := Result{HasMore: specialHasMore(q, raw), Note: "Поселения с неблагополучным состоянием владельца по данным Spansh. Подтверждение задания доступно на доске миссий."}
	for _, system := range systems {
		for _, faction := range v.Rows(system["minor_faction_presences"]) {
			name, states := spaceText(faction["name"]), factionStates(faction)
			if name == "" || warState(states) != "" || !matchesState(states, negative, "any") {
				continue
			}
			for _, station := range v.Rows(system["stations"]) {
				stationName := spaceText(station["name"])
				if stationName == "" || !containsStationType(stationSettlements, v.Str(station["type"])) ||
					!strings.EqualFold(v.Str(station["controlling_minor_faction"]), name) ||
					!matchesName(q.Name, name, stationName, system["name"]) {
					continue
				}
				row := specialSystemRow(system)
				row["station"], row["name"], row["states"], row["faction"] = stationName, stationName, states, name
				row["distanceLs"], row["power"] = station["distance_to_arrival"], system["controlling_power"]
				if id := spaceAddress(station["market_id"]); id != "" {
					row["url"] = "https://spansh.co.uk/station/" + id
				}
				result.Rows = append(result.Rows, row)
			}
		}
	}
	return result, nil
}
