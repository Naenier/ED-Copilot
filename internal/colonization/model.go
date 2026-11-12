// Package colonization plans independent colonies and reconciles journal depot observations.
package colonization

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Naenier/ED-Copilot/internal/data"
	"github.com/Naenier/ED-Copilot/internal/value"
)

type M = value.M

//go:embed catalog.json
var assets embed.FS
var types []M
var typesByID map[string]M
var sources []M
var names map[string]string
var english map[string]any
var projectStatuses = []string{"planned", "claimed", "building", "complete", "cancelled"}
var buildingStatuses = []string{"planned", "building", "complete", "cancelled"}
var economies = []M{{"id": "refinery", "name": "Переработка"}, {"id": "industrial", "name": "Промышленность"}, {"id": "extraction", "name": "Добыча"}, {"id": "agriculture", "name": "Сельское хозяйство"}, {"id": "hightech", "name": "Высокие технологии"}, {"id": "military", "name": "Военная"}, {"id": "tourism", "name": "Туризм"}, {"id": "service", "name": "Услуги"}, {"id": "mixed", "name": "Смешанная"}}
var marketPattern = regexp.MustCompile(`^\d{1,20}$`)

func init() {
	var catalogData M
	b, _ := assets.ReadFile("catalog.json")
	if err := json.Unmarshal(b, &catalogData); err != nil {
		panic(err)
	}
	types = value.Rows(catalogData["catalog"])
	sources = value.Rows(catalogData["sources"])
	types = enrichDevelopmentCatalog(types)
	typesByID = map[string]M{}
	for _, row := range types {
		typesByID[str(row["id"])] = row
	}
	names = map[string]string{}
	for k, v := range data.Object("commodity-ru.json") {
		names[k] = value.Str(v)
	}
	english = data.Object("commodity-names.json")
}

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}
type Option func(*Planner)

func WithHTTPClient(client HTTPClient) Option {
	return func(p *Planner) {
		if client != nil {
			p.client = client
		}
	}
}
func WithBaseURL(url string) Option {
	return func(p *Planner) { p.baseURL = strings.TrimRight(url, "/") }
}
func WithClock(now func() time.Time) Option {
	return func(p *Planner) {
		if now != nil {
			p.now = now
		}
	}
}

type Planner struct {
	mu                 sync.RWMutex
	config             M
	projects           []M
	selected           string
	depots             map[string]M
	contributions      []M
	supplyReservations []supplyReservation
	claims             map[string]M
	stations           map[string]M
	seen               map[string]bool
	sequence           int64
	commander          string
	carrierOwner       string
	carrierStocks      M
	carrierMovementAt  M
	developmentBodies  map[string]M
	history            map[string]*systemHistory
	search             M
	searchSerial       uint64
	cancel             context.CancelFunc
	client             HTTPClient
	baseURL            string
	now                func() time.Time
}

func New(saved M, opts ...Option) *Planner {
	p := &Planner{config: M{"originSystem": text(saved["originSystem"], 200), "faction": text(saved["faction"], 200), "cargoCapacity": natural(saved["cargoCapacity"], 100000), "includeCarrierWarehouse": value.Bool(saved["includeCarrierWarehouse"]), "squadronWarehouseAccessConfirmed": value.Bool(saved["squadronWarehouseAccessConfirmed"])}, projects: []M{}, client: &http.Client{Timeout: 25 * time.Second}, baseURL: "https://spansh.co.uk/api", now: time.Now}
	for _, opt := range opts {
		opt(p)
	}
	for _, row := range limitedRows(saved["projects"], 100) {
		p.projects = append(p.projects, cleanProject(row))
	}
	p.restoreSupplyReservations(saved["supplyReservations"])
	p.selected = str(saved["selectedProjectId"])
	if p.projectLocked(p.selected) == nil {
		if len(p.projects) > 0 {
			p.selected = str(p.projects[0]["id"])
		} else {
			p.selected = ""
		}
	}
	p.search = initialSearch()
	p.search["options"] = merge(M{"originSystem": "", "faction": "", "rangeLy": 15}, p.config)
	p.resetLocked()
	p.restoreSystemHistory(saved["systemHistory"])
	p.restoreCarrierStocks(value.Obj(saved["carrierInventoryCache"]))
	accessFID, accessID := text(saved["squadronWarehouseAccessFID"], 100), marketID(saved["squadronWarehouseAccessCarrierID"])
	currentCarrier := str(value.At(p.carrierStocks, "squadron", "id"))
	if value.Bool(saved["squadronWarehouseAccessConfirmed"]) && accessFID != "" && accessFID == p.carrierOwner && accessID != "" && (currentCarrier == "" || accessID == currentCarrier) {
		p.config["squadronWarehouseAccessFID"], p.config["squadronWarehouseAccessCarrierID"] = accessFID, accessID
	} else {
		p.clearSquadronAccess()
	}
	return p
}
func Catalog() []M { return cloneRows(types) }
func Rules() M {
	return M{"rangeLy": 15, "claimCost": 25000000, "beaconHours": 24, "primaryDays": 28, "maxConcurrent": 5}
}
func (p *Planner) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Journal replay does not re-create CAPI observations. Retain the bounded
	// cache, but count it only after the journal confirms the same FID again.
	owner, stocks := p.carrierOwner, p.carrierStocks
	p.resetLocked()
	p.carrierOwner, p.carrierStocks = owner, stocks
}
func (p *Planner) resetLocked() {
	p.depots = map[string]M{}
	p.contributions = []M{}
	p.claims = map[string]M{}
	p.stations = map[string]M{}
	p.seen = map[string]bool{}
	p.sequence = 0
	p.commander = ""
	p.carrierOwner = ""
	p.carrierStocks = M{}
	p.carrierMovementAt = M{}
	p.developmentBodies = map[string]M{}
}
func (p *Planner) ExportSettings() M {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return merge(copyM(p.config), M{"projects": cloneRows(p.projects), "selectedProjectId": p.selected, "supplyReservations": p.savedSupplyReservations(), "carrierInventoryCache": M{"fid": p.carrierOwner, "warehouses": value.Copy(p.carrierStocks)}, "systemHistory": p.saveSystemHistory()})
}
func (p *Planner) projectLocked(id string) M {
	if id == "" {
		id = p.selected
	}
	for _, r := range p.projects {
		if str(r["id"]) == id && p.projectVisible(r) {
			return r
		}
	}
	return nil
}
func building(project M, id string) M {
	for _, r := range rows(project["buildings"]) {
		if str(r["id"]) == id {
			return r
		}
	}
	return nil
}
func initialSearch() M {
	return M{"running": false, "status": "idle", "error": "", "origin": nil, "origins": []M{}, "factions": []M{}, "claimStations": []M{}, "results": []M{}, "options": M{"originSystem": "", "faction": "", "rangeLy": 15}, "completeness": M{"complete": false, "providerTotal": nil, "fetched": 0, "pages": 0, "reason": "Поиск ещё не выполнялся."}, "warnings": []string{}}
}
func cleanBuilding(v M) M {
	t := typesByID[str(v["typeId"])]
	if t == nil {
		t = typesByID["custom"]
	}
	status := str(v["status"])
	if !has(buildingStatuses, status) {
		status = "planned"
	}
	id := text(v["id"], 80)
	if id == "" {
		id = value.ID()
	}
	tier := natural(v["tierOverride"], 3)
	if num(tier) < 1 {
		tier = nil
	}
	bill, _ := resourceRows(v["resourcePlan"])
	journalStatus := str(v["journalStatus"])
	if !has([]string{"building", "complete", "failed"}, journalStatus) || validDate(v["journalUpdatedAt"]) == nil {
		journalStatus = ""
	}
	journalSource := str(v["journalSource"])
	if !has([]string{"station", "depot"}, journalSource) {
		journalSource = ""
	}
	return M{"id": id, "typeId": t["id"], "name": firstText(v["name"], t["name"]), "status": status, "body": text(v["body"], 200), "isPrimary": value.Bool(v["isPrimary"]) && value.Bool(t["canBePrimary"]), "marketId": marketID(v["marketId"]), "notes": text(v["notes"], 4000), "dependsOn": uniqueStrings(v["dependsOn"], 100), "tierOverride": tier, "pointCostOverride": natural(v["pointCostOverride"], 100000), "pointRewardOverride": natural(v["pointRewardOverride"], 100000), "resourcePlan": bill, "sourceNote": text(v["sourceNote"], 1000), "journalDiscovered": value.Bool(v["journalDiscovered"]), "journalStatus": journalStatus, "journalUpdatedAt": validDate(v["journalUpdatedAt"]), "journalStationName": text(v["journalStationName"], 200), "journalDisplayName": text(v["journalDisplayName"], 200), "journalStationType": text(v["journalStationType"], 200), "journalSource": journalSource}
}
func cleanProject(v M) M {
	id := text(v["id"], 80)
	if id == "" {
		id = value.ID()
	}
	status := str(v["status"])
	if !has(projectStatuses, status) {
		status = "planned"
	}
	bs := []M{}
	for _, r := range limitedRows(v["buildings"], 200) {
		bs = append(bs, cleanBuilding(r))
	}
	ds := []M{}
	dr := rows(v["deliveries"])
	if len(dr) > 5000 {
		dr = dr[len(dr)-5000:]
	}
	for _, r := range dr {
		key := value.Key(r["commodity"])
		count := natural(r["count"], 1e9)
		if key != "" && num(count) > 0 {
			ds = append(ds, M{"id": firstText(r["id"], value.ID()), "buildingId": text(r["buildingId"], 80), "commodity": key, "count": count, "timestamp": validDate(r["timestamp"]), "notes": text(r["notes"], 1000), "source": "manual"})
		}
	}
	project := M{"id": id, "name": firstText(v["name"], v["targetSystem"], "Новая колония"), "originSystem": text(v["originSystem"], 200), "faction": text(v["faction"], 200), "targetSystem": text(v["targetSystem"], 200), "coordinates": coordinates(v["coordinates"]), "systemAddress": marketID(v["systemAddress"]), "status": status, "goalEconomy": text(v["goalEconomy"], 80), "claimedAt": validDate(v["claimedAt"]), "deadline": validDate(v["deadline"]), "notes": text(v["notes"], 4000), "points": M{"tier2": point(value.Obj(v["points"])["tier2"]), "tier3": point(value.Obj(v["points"])["tier3"])}, "externalPortCount": natural(v["externalPortCount"], 1000), "cargoCapacity": natural(v["cargoCapacity"], 100000), "supplyOrder": uniqueStrings(v["supplyOrder"], 200), "buildings": bs, "deliveries": ds, "journalOwnership": cleanJournalOwnership(v["journalOwnership"])}
	pruneShipBuildings(project, "")
	return project
}
func resourceRows(v any) ([]M, error) {
	if v == nil {
		return []M{}, nil
	}
	a, ok := v.([]any)
	if !ok {
		if r, yes := v.([]M); yes {
			a = value.Arr(r)
		} else {
			return nil, errors.New("Нужен список ресурсов.")
		}
	}
	if len(a) > 500 {
		return nil, errors.New("В списке ресурсов не более 500 строк.")
	}
	out := []M{}
	by := map[string]M{}
	for _, x := range a {
		r := value.Obj(x)
		key := value.Key(firstText(r["commodity"], r["Name"], r["name"]))
		raw := r["required"]
		if raw == nil {
			raw = r["RequiredAmount"]
		}
		n, valid := value.Number(raw)
		if key == "" || len(key) > 100 || !valid || n < 0 || math.Trunc(n) != n || n > 1e9 {
			return nil, errors.New("Укажите товар и неотрицательное целое количество до миллиарда.")
		}
		if old := by[key]; old != nil {
			old["required"] = math.Min(1e9, num(old["required"])+n)
		} else {
			row := M{"commodity": key, "name": commodityName(key, firstText(r["Name_Localised"], r["name"])), "required": n}
			out = append(out, row)
			by[key] = row
		}
	}
	return out, nil
}
func commodityName(key, supplied string) string {
	if n := names[key+"name"]; n != "" {
		return n
	}
	if supplied != "" {
		return supplied
	}
	if a := value.Arr(english[key]); len(a) > 0 {
		return firstText(a[0], key)
	}
	return firstText(value.Obj(english[key])["english"], key)
}
func (p *Planner) uniqueDepot(id string, except M) error {
	if id == "" {
		return nil
	}
	if IsColonisationShip(p.depots[id]) || IsColonisationShip(p.stations[id]) {
		return errors.New("Колонизационный корабль снабжения не является зданием или стройплощадкой.")
	}
	exceptID := str(except["id"])
	for _, project := range p.projects {
		for _, b := range rows(project["buildings"]) {
			if str(b["id"]) != exceptID && str(b["marketId"]) == id {
				return errors.New("Это депо уже связано с другим объектом.")
			}
		}
	}
	return nil
}
func (p *Planner) uniqueColony(system, address string, except M) error {
	for _, project := range p.projects {
		if !p.projectVisible(project) {
			continue
		}
		if str(project["id"]) != str(except["id"]) && (equal(project["targetSystem"], system) || address != "" && str(project["systemAddress"]) == address) {
			return errors.New("Эта колония уже есть в списке. Выберите её в поле «Колония».")
		}
	}
	return nil
}
func validateDependencies(project M) error {
	all := map[string]M{}
	for _, b := range rows(project["buildings"]) {
		id := str(b["id"])
		if all[id] != nil {
			return errors.New("Идентификаторы объектов должны быть уникальны.")
		}
		all[id] = b
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return errors.New("Зависимости объектов образуют цикл.")
		}
		if done[id] {
			return nil
		}
		if all[id] == nil {
			return errors.New("Объект зависимости не найден в этой колонии.")
		}
		visiting[id] = true
		for _, d := range stringsOf(all[id]["dependsOn"]) {
			if e := visit(d); e != nil {
				return e
			}
		}
		delete(visiting, id)
		done[id] = true
		return nil
	}
	for id := range all {
		if e := visit(id); e != nil {
			return e
		}
	}
	return nil
}
func supplyBuildings(project M, bs []M) []M {
	by := map[string]M{}
	for _, b := range bs {
		by[str(b["id"])] = b
	}
	ids := stringsOf(project["supplyOrder"])
	for _, b := range bs {
		if !has(ids, str(b["id"])) {
			ids = append(ids, str(b["id"]))
		}
	}
	out := []M{}
	for _, id := range ids {
		if b := by[id]; b != nil {
			out = append(out, b)
		}
	}
	return out
}
func str(v any) string          { return value.Str(v) }
func num(v any) float64         { return value.Num(v) }
func rows(v any) []M            { return value.Rows(v) }
func copyM(v M) M               { return value.Copy(v) }
func cloneRows(v []M) []M       { return rows(value.Clone(v)) }
func merge(dst M, src ...M) M   { return value.Merge(dst, src...) }
func firstText(v ...any) string { return text(value.Text(v...), 4000) }
func text(v any, max int) string {
	s := strings.Map(func(r rune) rune {
		if r < 32 {
			return ' '
		}
		return r
	}, str(v))
	r := []rune(strings.TrimSpace(s))
	if len(r) > max {
		r = r[:max]
	}
	return string(r)
}
func equal(a, b any) bool {
	return strings.EqualFold(strings.TrimSpace(str(a)), strings.TrimSpace(str(b)))
}
func has(v []string, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}
func stringsOf(v any) []string {
	out := []string{}
	for _, x := range value.Arr(v) {
		out = append(out, str(x))
	}
	return out
}
func uniqueStrings(v any, max int) []string {
	out := []string{}
	for _, x := range value.Arr(v) {
		s := text(x, 80)
		if s != "" && !has(out, s) {
			out = append(out, s)
		}
		if len(out) >= max {
			break
		}
	}
	return out
}
func natural(v any, max float64) any {
	n, ok := value.Number(v)
	if !ok {
		return nil
	}
	return math.Max(0, math.Min(max, math.Trunc(n)))
}
func point(v any) any {
	n, ok := value.Number(v)
	if !ok {
		return nil
	}
	return math.Max(-100000, math.Min(100000, math.Trunc(n)))
}
func marketID(v any) string {
	s := str(v)
	if marketPattern.MatchString(s) {
		return s
	}
	if n, ok := value.Number(v); ok && n >= 0 && n == math.Trunc(n) {
		s = fmt.Sprintf("%.0f", n)
		if marketPattern.MatchString(s) {
			return s
		}
	}
	return ""
}
func validDate(v any) any {
	t := value.Time(v)
	if t.IsZero() {
		return nil
	}
	return value.ISO(t)
}
func coordinates(v any) M {
	m := value.Obj(v)
	out := M{}
	for _, key := range []string{"x", "y", "z"} {
		n, ok := value.Number(m[key])
		if !ok {
			return nil
		}
		out[key] = n
	}
	return out
}
func dist(a, b M) any {
	if a == nil || b == nil {
		return nil
	}
	return math.Sqrt(math.Pow(num(a["x"])-num(b["x"]), 2) + math.Pow(num(a["y"])-num(b["y"]), 2) + math.Pow(num(a["z"])-num(b["z"]), 2))
}
func limitedRows(v any, max int) []M {
	r := rows(v)
	if len(r) > max {
		return r[:max]
	}
	return r
}
func eventID(file any, offset any) string {
	sum := sha256.Sum256([]byte(str(file) + ":" + str(offset)))
	return hex.EncodeToString(sum[:16])
}
func sortedKeys(m map[string]M) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
