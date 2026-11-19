package nearest

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// Directive 47 publishes a rolling EDDN FSS index. Fetch it only for an
// orbital-signal search. The underlying sightings are public game data and
// the index format is documented in https://github.com/dseelinger/d47.
const orbitalSignalFeed = "https://github.com/dseelinger/d47/releases/download/signals-1/signals.tsv.gz"
const orbitalSignalHeader = "system_address\tsystem_name\tx\ty\tz\tsignal_type\tsignal_name\tlast_seen"
const maxCompressedSignals = 24 * 1024 * 1024
const maxDecodedSignals = 160 * 1024 * 1024
const orbitalSystemJoinLimit = 200

var orbitalSignalLabels = map[string]string{
	"navigation-beacon":        "Nav Beacon",
	"compromised-nav-beacon":   "Compromised Nav Beacon",
	"resource-extraction":      "Resource Extraction Site",
	"resource-extraction-low":  "Low Resource Extraction Site",
	"resource-extraction-high": "High Resource Extraction Site",
	"resource-extraction-haz":  "Hazardous Resource Extraction Site",
	"any-resource-extraction":  "Resource Extraction Site",
	"colonization-beacon":      "Colonisation Beacon",
	"orbital-tourist-beacon":   "Orbital Tourist Beacon",
	"installation":             "Installation",
	"scannable-megaship":       "Scannable Megaship",
	"thargoid-titan":           "Thargoid Titan",
	"listening-post":           "Listening Post",
	"unregistered-beacon":      "Unregistered Communications Beacon",
	"stellar-phenomena":        "Notable Stellar Phenomena",
}

var signalScenario = map[string]string{
	"$MULTIPLAYER_SCENARIO42_TITLE;": "navigation-beacon",
	"$MULTIPLAYER_SCENARIO80_TITLE;": "compromised-nav-beacon",
	"$MULTIPLAYER_SCENARIO14_TITLE;": "resource-extraction",
	"$MULTIPLAYER_SCENARIO77_TITLE;": "resource-extraction-low",
	"$MULTIPLAYER_SCENARIO78_TITLE;": "resource-extraction-high",
	"$MULTIPLAYER_SCENARIO79_TITLE;": "resource-extraction-haz",
}

func classifyOrbitalSignal(signalType, name string) string {
	symbol := strings.ToUpper(name)
	if kind := signalScenario[symbol]; kind != "" {
		return kind
	}
	switch {
	case strings.HasPrefix(symbol, "$EXT_PANEL_COLONISATIONBEACON_"):
		return "colonization-beacon"
	case strings.HasPrefix(symbol, "$LISTENINGPOST"):
		return "listening-post"
	case strings.HasPrefix(symbol, "$NUMBERSTATION"):
		return "unregistered-beacon"
	case strings.EqualFold(signalType, "Codex") && symbol == "$FIXED_EVENT_LIFE_CLOUD;":
		return "stellar-phenomena"
	}
	switch strings.ToLower(signalType) {
	case "navbeacon":
		switch strings.ToLower(name) {
		case "nav beacon":
			return "navigation-beacon"
		case "compromised nav beacon":
			return "compromised-nav-beacon"
		}
	case "touristbeacon":
		return "orbital-tourist-beacon"
	case "installation":
		if !strings.HasPrefix(symbol, "PLANETARY CONSTRUCTION SITE:") {
			return "installation"
		}
	case "megaship":
		return "scannable-megaship"
	case "titan":
		return "thargoid-titan"
	}
	return ""
}

var sharedOrbitalSignals struct {
	sync.Mutex
	downloaded time.Time
	gzip       []byte
}

func (c *Client) orbitalSignals(ctx context.Context) ([]byte, bool, error) {
	if c.signalFeed != "" {
		data, err := c.fetchOrbitalSignals(ctx, c.signalFeed)
		return data, false, err
	}
	sharedOrbitalSignals.Lock()
	cached, downloaded := sharedOrbitalSignals.gzip, sharedOrbitalSignals.downloaded
	sharedOrbitalSignals.Unlock()
	if len(cached) != 0 && time.Since(downloaded) < 20*time.Hour {
		return cached, false, nil
	}
	data, err := c.fetchOrbitalSignals(ctx, orbitalSignalFeed)
	if err != nil {
		if len(cached) != 0 && ctx.Err() == nil {
			return cached, true, nil
		}
		return nil, false, err
	}
	sharedOrbitalSignals.Lock()
	sharedOrbitalSignals.gzip, sharedOrbitalSignals.downloaded = data, time.Now()
	sharedOrbitalSignals.Unlock()
	return data, false, nil
}

func (c *Client) fetchOrbitalSignals(ctx context.Context, address string) ([]byte, error) {
	client := *c.HTTP
	client.CheckRedirect = func(req *http.Request, chain []*http.Request) error {
		switch req.URL.Host {
		case "release-assets.githubusercontent.com", "objects.githubusercontent.com":
			if req.URL.Scheme == "https" && req.URL.User == nil && len(chain) == 1 {
				return nil
			}
		}
		return http.ErrUseLastResponse
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ED Copilot")
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > maxCompressedSignals {
		return nil, errors.New("Каталог орбитальных сигналов недоступен.")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxCompressedSignals+1))
	if err != nil || len(data) > maxCompressedSignals {
		return nil, errors.New("Каталог орбитальных сигналов недоступен.")
	}
	scanner, reader, err := signalScanner(data)
	if err != nil {
		return nil, err
	}
	_ = reader.Close()
	if scanner.Err() != nil {
		return nil, scanner.Err()
	}
	return data, nil
}

func signalScanner(data []byte) (*bufio.Scanner, *gzip.Reader, error) {
	decoded, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	scanner := bufio.NewScanner(io.LimitReader(decoded, maxDecodedSignals+1))
	scanner.Buffer(make([]byte, 8192), 8192)
	if !scanner.Scan() || scanner.Text() != orbitalSignalHeader {
		_ = decoded.Close()
		return nil, nil, errors.New("Каталог орбитальных сигналов имеет неизвестный формат.")
	}
	return scanner, decoded, nil
}

func readOrbitalSignals(ctx context.Context, data []byte, q Query, kind string, origin []float64, now time.Time) ([]M, error) {
	scanner, decoded, err := signalScanner(data)
	if err != nil {
		return nil, err
	}
	defer decoded.Close()
	radius := v.Num(q.Filters["radiusLy"])
	rows, bytesRead := []M{}, 0
	type beaconObservation struct {
		day, kind string
		conflict  bool
	}
	beacons := map[string]beaconObservation{}
	beaconSearch := kind == "navigation-beacon" || kind == "compromised-nav-beacon"
	cutoff := now.UTC().AddDate(0, 0, -90).Format(time.DateOnly)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		bytesRead += len(scanner.Bytes()) + 1
		if bytesRead >= maxDecodedSignals {
			return nil, errors.New("Каталог орбитальных сигналов превышает допустимый размер.")
		}
		parts := strings.SplitN(scanner.Text(), "\t", 9)
		if len(parts) != 8 || parts[7] < cutoff {
			continue
		}
		signal := classifyOrbitalSignal(parts[5], parts[6])
		isBeacon := beaconSearch && (signal == "navigation-beacon" || signal == "compromised-nav-beacon")
		if signal != kind && !isBeacon && !(kind == "any-resource-extraction" && strings.HasPrefix(signal, "resource-extraction")) {
			continue
		}
		address, system, name := spaceAddress(parts[0]), parts[1], parts[6]
		if address == "" || !validText(system, 150) || !validText(name, 180) || system == "" || name == "" {
			continue
		}
		if strings.HasPrefix(name, "$") {
			name = orbitalSignalLabels[signal]
		}
		at, err := time.Parse(time.DateOnly, parts[7])
		if err != nil || at.After(now.Add(24*time.Hour)) {
			continue
		}
		if isBeacon {
			old := beacons[address]
			switch {
			case parts[7] > old.day:
				beacons[address] = beaconObservation{day: parts[7], kind: signal}
			case parts[7] == old.day && signal != old.kind:
				old.conflict = true
				beacons[address] = old
			}
			if signal != kind {
				continue
			}
		}
		if !matchesName(q.Name, system, name) {
			continue
		}
		row := M{"name": name, "system": system, "systemAddress": address, "type": orbitalSignalLabels[signal], "updatedAt": v.ISO(at)}
		if len(origin) == 3 || q.Region != "" {
			var point [3]float64
			valid := true
			for i := range point {
				n, err := strconv.ParseFloat(parts[i+2], 64)
				valid = valid && err == nil && !math.IsNaN(n) && !math.IsInf(n, 0)
				point[i] = n
			}
			if !valid {
				continue
			}
			if q.Region != "" && !sameOrbitalRegion(q.Region, orbitalRegion(point[0], point[2])) {
				continue
			}
			if len(origin) == 3 {
				distance := math.Hypot(math.Hypot(point[0]-origin[0], point[1]-origin[1]), point[2]-origin[2])
				if radius > 0 && distance > radius {
					continue
				}
				row["distanceLy"] = distance
			}
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if beaconSearch {
		unique := map[string]bool{}
		filtered := rows[:0]
		for _, row := range rows {
			address := v.Str(row["systemAddress"])
			latest := beacons[address]
			if !latest.conflict && latest.kind == kind && strings.HasPrefix(v.Str(row["updatedAt"]), latest.day) && !unique[address] {
				unique[address] = true
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if a, b := v.Num(rows[i]["distanceLy"]), v.Num(rows[j]["distanceLy"]); a != b {
			return a < b
		}
		if a, b := v.Str(rows[i]["system"]), v.Str(rows[j]["system"]); a != b {
			return a < b
		}
		return v.Str(rows[i]["name"]) < v.Str(rows[j]["name"])
	})
	return rows, nil
}

func (c *Client) searchOrbitalSignals(ctx context.Context, q Query, kind string) (Result, error) {
	result := Result{Note: "Орбитальные сигналы по наблюдениям EDDN из каталога Directive 47.", Link: InaraLink(q)}
	index, stale, err := c.orbitalSignals(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		result.Note = "Каталог орбитальных сигналов Directive 47 временно недоступен."
		return result, nil
	}
	position, err := c.specialOriginCoordinates(ctx, q)
	if err != nil {
		return result, err
	}
	rows, err := readOrbitalSignals(ctx, index, q, kind, position, time.Now())
	if err != nil {
		return result, err
	}
	if stale {
		result.Note = "Обновление каталога Directive 47 недоступно. Используются последние сохранённые наблюдения EDDN."
	}
	if hasSpecialSystemFilters(q) {
		var partial bool
		rows, partial, err = c.filterOrbitalSignals(ctx, q, rows)
		if err != nil {
			return result, err
		}
		if partial {
			result.Note = "Каталог Directive 47. Дополнительные условия проверены для первых 200 систем с сигналами."
		}
	}
	if q.Sort == "name" {
		sort.SliceStable(rows, func(i, j int) bool {
			return strings.ToLower(v.Str(rows[i]["name"])) < strings.ToLower(v.Str(rows[j]["name"]))
		})
	}
	size := specialPageSize(q)
	from := min(q.Page*size, len(rows))
	to := min(from+size, len(rows))
	result.Rows, result.Total, result.HasMore = rows[from:to], len(rows), to < len(rows)
	return result, nil
}

func (c *Client) filterOrbitalSignals(ctx context.Context, q Query, rows []M) ([]M, bool, error) {
	addresses, partial := []string{}, false
	seen := map[string]bool{}
	for _, row := range rows {
		id := v.Str(row["systemAddress"])
		if seen[id] {
			continue
		}
		if len(addresses) == orbitalSystemJoinLimit {
			partial = true
			break
		}
		seen[id] = true
		addresses = append(addresses, id)
	}
	matched := make(map[string]bool, len(addresses))
	for from := 0; from < len(addresses); from += 100 {
		ids := addresses[from:min(from+100, len(addresses))]
		filters := spaceSystemFilters(q)
		setSpaceGroup(filters, "id64", ids)
		search := q
		search.Page, search.Limit = 0, len(ids)
		raw, err := c.request(ctx, "/systems/search", spacePayload(search, filters))
		if err != nil {
			return nil, partial, err
		}
		systems, err := specialRows(raw)
		if err != nil {
			return nil, partial, err
		}
		for _, system := range systems {
			id := spaceAddress(system["id64"])
			if seen[id] && matchesSpecialPresence(system, q) {
				matched[id] = true
			}
		}
	}
	filtered := rows[:0]
	for _, row := range rows {
		if matched[v.Str(row["systemAddress"])] {
			filtered = append(filtered, row)
		}
	}
	return filtered, partial, nil
}
