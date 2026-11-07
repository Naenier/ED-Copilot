package pilot

import (
	"errors"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"sort"
	"strings"
	"sync"
	"time"
)

type M = v.M

var kinds = map[string]bool{"travel": true, "trade": true, "mining": true, "engineering": true, "exploration": true, "combat": true, "mission": true, "carrier": true, "odyssey": true, "other": true, "colonization": true}
var labels = map[string]string{"LoadGame": "Начало сеанса", "FSDJump": "Гиперпрыжок", "CarrierJump": "Перелёт на флотоносце", "Docked": "Стыковка", "Undocked": "Отстыковка", "Touchdown": "Посадка на поверхность", "Liftoff": "Взлёт с поверхности", "Disembark": "Выход пешком", "Embark": "Посадка на борт", "LaunchSRV": "Выезд на ТРП", "DockSRV": "Возвращение ТРП", "MissionAccepted": "Принято поручение", "MissionCompleted": "Поручение выполнено", "MissionFailed": "Поручение провалено", "MissionAbandoned": "Отказ от поручения", "MissionRedirected": "Новое назначение поручения", "MarketBuy": "Покупка товара", "MarketSell": "Продажа товара", "MaterialTrade": "Обмен материалов", "EngineerCraft": "Инженерное улучшение", "Died": "Потеря корабля", "Resurrect": "Восстановление", "FSSDiscoveryScan": "Обзор системы", "SAAScanComplete": "Картографирование тела", "ScanOrganic": "Исследование органики", "SellOrganicData": "Продажа биологических данных", "SellExplorationData": "Продажа исследовательских данных", "MultiSellExplorationData": "Продажа исследовательских данных", "Bounty": "Награда за цель", "FactionKillBond": "Боевой ваучер", "Shutdown": "Завершение игры", "ColonisationContribution": "Поставка на строительство"}

func NormalizeObjective(input, existing M) (M, error) {
	x := v.Merge(v.Copy(existing), input)
	title := strings.TrimSpace(v.Str(x["title"]))
	if title == "" {
		return nil, errors.New("Укажите название цели")
	}
	kind := v.Text(x["kind"], "travel")
	if !kinds[kind] {
		return nil, errors.New("Выберите тип цели")
	}
	id := v.Text(existing["id"], v.ID())
	status := v.Text(existing["status"], "active")
	r := M{"id": id, "title": title, "kind": kind, "status": status, "createdAt": v.Text(existing["createdAt"], v.ISO(time.Now())), "completedAt": existing["completedAt"]}
	for _, k := range []string{"system", "station", "body", "notes"} {
		r[k] = strings.TrimSpace(v.Str(x[k]))
	}
	r["priority"] = "normal"
	if x["priority"] == "high" {
		r["priority"] = "high"
	}
	return r, nil
}

type Tracker struct {
	mu                       sync.RWMutex
	credits                  any
	creditsAt, creditsSource string
	fuel                     M
	missions                 map[string]M
	recent, route            []M
	target                   M
	routeAt, clearedAt       string
	routeExpected            bool
	system, station          string
	session                  M
	legal                    string
	now                      func() time.Time
}

func New() *Tracker { t := &Tracker{now: time.Now}; t.reset(); return t }
func (t *Tracker) reset() {
	t.credits = nil
	t.creditsAt = ""
	t.creditsSource = ""
	t.fuel = M{"mainTons": nil, "mainCapacityTons": nil, "updatedAt": nil}
	t.missions = map[string]M{}
	t.recent = nil
	t.route = nil
	t.target = nil
	t.routeAt = ""
	t.clearedAt = ""
	t.routeExpected = false
	t.system = ""
	t.station = ""
	t.session = M{"since": nil, "jumps": 0, "distanceLy": 0}
	t.legal = ""
}
func (t *Tracker) Reset() { t.mu.Lock(); defer t.mu.Unlock(); t.reset() }
func fresh(at, prev string) bool {
	return prev == "" || (!v.Time(at).IsZero() && !v.Time(at).Before(v.Time(prev)))
}
func (t *Tracker) creditsSet(x any, at, source string) {
	if n, ok := v.Number(x); ok && n >= 0 && fresh(at, t.creditsAt) {
		t.credits = n
		t.creditsAt = at
		t.creditsSource = source
	}
}
func (t *Tracker) fuelSet(x any, at string) {
	if n, ok := v.Number(x); ok && n >= 0 && fresh(at, v.Str(t.fuel["updatedAt"])) {
		t.fuel["mainTons"] = n
		t.fuel["updatedAt"] = at
	}
}
func (t *Tracker) routeSet(event, commander M) {
	if event["Route"] == nil {
		return
	}
	at := v.Str(event["timestamp"])
	if t.clearedAt != "" && (at == "" || v.Time(at).Before(v.Time(t.clearedAt)) || (at == t.clearedAt && !t.routeExpected)) {
		return
	}
	if t.routeAt != "" && at != "" && v.Time(at).Before(v.Time(t.routeAt)) {
		return
	}
	t.routeAt = at
	t.route = nil
	current := v.Text(commander["system"], t.system)
	index := -1
	for _, r := range v.Rows(event["Route"]) {
		if name := v.Str(r["StarSystem"]); name != "" {
			t.route = append(t.route, M{"system": name, "systemAddress": r["SystemAddress"], "coordinates": r["StarPos"], "starClass": r["StarClass"], "visited": false})
			if strings.EqualFold(name, current) {
				index = len(t.route) - 1
			}
		}
		if len(t.route) >= 1000 {
			break
		}
	}
	for i := 0; i <= index; i++ {
		t.route[i]["visited"] = true
	}
	t.retarget()
}
func (t *Tracker) retarget() {
	t.target = nil
	remaining := 0
	for _, r := range t.route {
		if !v.Bool(r["visited"]) {
			remaining++
			if t.target == nil {
				t.target = M{"system": r["system"]}
			}
		}
	}
	if t.target != nil {
		t.target["remainingJumps"] = remaining
	}
}
func missionName(event, prev M) string {
	return v.Text(event["LocalisedName"], event["Name_Localised"], prev["name"], "Поручение №"+v.Str(event["MissionID"]))
}
func (t *Tracker) Apply(event, context M) {
	t.mu.Lock()
	defer t.mu.Unlock()
	kind, at := v.Str(event["event"]), v.Str(event["timestamp"])
	commander := v.Obj(context["commander"])
	switch kind {
	case "LoadGame":
		t.session = M{"since": at, "jumps": 0, "distanceLy": 0}
		t.creditsSet(event["Credits"], at, "Журнал: начало сеанса")
		t.fuelSet(event["FuelLevel"], at)
		if n, ok := v.Number(event["FuelCapacity"]); ok {
			t.fuel["mainCapacityTons"] = n
		}
	case "Loadout":
		if n, ok := v.Number(v.First(v.Obj(event["FuelCapacity"])["Main"], event["FuelCapacity"])); ok {
			t.fuel["mainCapacityTons"] = n
		}
	case "Location", "FSDJump", "CarrierJump":
		t.system = v.Text(event["StarSystem"], t.system)
		t.station = ""
		if kind == "Location" && v.Bool(event["Docked"]) {
			t.station = v.Str(event["StationName"])
		}
		t.fuelSet(event["FuelLevel"], at)
		if kind == "FSDJump" {
			t.session["jumps"] = v.Int(t.session["jumps"]) + 1
			t.session["distanceLy"] = v.Num(t.session["distanceLy"]) + max(0, v.Num(event["JumpDist"]))
			for _, r := range t.route {
				if !v.Bool(r["visited"]) {
					if strings.EqualFold(v.Str(r["system"]), t.system) {
						r["visited"] = true
					}
					break
				}
			}
			t.retarget()
		}
	case "Docked":
		t.station = v.Str(event["StationName"])
	case "Undocked":
		t.station = ""
	case "FuelScoop":
		t.fuelSet(event["Total"], at)
	case "Status":
		t.creditsSet(event["Balance"], at, "Status.json")
		t.fuelSet(v.Obj(event["Fuel"])["FuelMain"], at)
		if s := v.Str(event["LegalState"]); s != "" {
			t.legal = s
		}
	case "NavRoute":
		t.routeExpected = true
		t.routeSet(event, commander)
	case "NavRouteClear":
		t.route = nil
		t.target = nil
		t.routeAt = at
		t.clearedAt = at
		t.routeExpected = false
	case "FSDTarget":
		t.target = M{"system": event["Name"], "systemAddress": event["SystemAddress"], "remainingJumps": event["RemainingJumpsInRoute"], "starClass": event["StarClass"]}
	case "MissionAccepted":
		id := v.Str(event["MissionID"])
		if id != "" {
			t.missions[id] = M{"id": id, "name": missionName(event, t.missions[id]), "status": "active", "acceptedAt": at, "destinationSystem": v.Text(event["NewDestinationSystem"], event["DestinationSystem"]), "destinationStation": v.Text(event["NewDestinationStation"], event["DestinationStation"], event["DestinationSettlement"]), "expiry": event["Expiry"], "reward": event["Reward"], "faction": event["Faction"], "cargo": event["Commodity_Localised"], "count": event["Count"], "updatedAt": at}
		}
	case "MissionRedirected":
		id := v.Str(event["MissionID"])
		if id != "" {
			p := t.missions[id]
			if p == nil {
				p = M{"id": id, "name": missionName(event, nil), "status": "active"}
			}
			v.Merge(p, M{"destinationSystem": event["NewDestinationSystem"], "destinationStation": event["NewDestinationStation"], "updatedAt": at})
			t.missions[id] = p
		}
	case "MissionCompleted", "MissionFailed", "MissionAbandoned":
		id := v.Str(event["MissionID"])
		if id != "" {
			p := t.missions[id]
			if p == nil {
				p = M{"id": id, "name": missionName(event, nil)}
			}
			p["status"] = map[string]string{"MissionCompleted": "completed", "MissionFailed": "failed", "MissionAbandoned": "abandoned"}[kind]
			p["updatedAt"] = at
			t.missions[id] = p
		}
	case "Missions":
		if event["Active"] != nil {
			present := map[string]bool{}
			for _, f := range []string{"Active", "Failed", "Complete"} {
				for _, r := range v.Rows(event[f]) {
					present[v.Str(r["MissionID"])] = true
				}
			}
			for id, m := range t.missions {
				if (m["status"] == "active" || m["status"] == "ready") && !present[id] {
					m["status"] = "untracked"
				}
			}
			for field, status := range map[string]string{"Active": "active", "Failed": "failed", "Complete": "ready"} {
				for _, row := range v.Rows(event[field]) {
					id := v.Str(row["MissionID"])
					p := t.missions[id]
					if p == nil {
						p = M{}
					}
					var expiry any = p["expiry"]
					if n, ok := v.Number(row["Expires"]); ok && !v.Time(at).IsZero() {
						expiry = v.ISO(v.Time(at).Add(time.Duration(max(0, n)) * time.Second))
					}
					t.missions[id] = v.Merge(v.Copy(p), M{"id": id, "name": missionName(row, p), "status": status, "expiry": expiry, "updatedAt": at})
				}
			}
		}
	}
	if label := labels[kind]; label != "" {
		detail := v.Text(event["Type_Localised"], event["BodyName"], event["Body"], event["ShipName"])
		if strings.HasPrefix(kind, "Mission") {
			detail = missionName(event, t.missions[v.Str(event["MissionID"])])
		}
		t.recent = append([]M{{"timestamp": at, "event": kind, "label": label, "system": v.Text(t.system, commander["system"]), "station": t.station, "detail": detail}}, t.recent...)
		if len(t.recent) > 40 {
			t.recent = t.recent[:40]
		}
	}
}
func (t *Tracker) Snapshot() M {
	t.mu.RLock()
	defer t.mu.RUnlock()
	missions := []M{}
	for _, raw := range t.missions {
		if raw["status"] == "active" || raw["status"] == "ready" {
			r := v.Copy(raw)
			expiry := v.Time(r["expiry"])
			r["expired"] = !expiry.IsZero() && expiry.Before(t.now())
			missions = append(missions, r)
		}
	}
	sort.Slice(missions, func(i, j int) bool {
		a, b := v.Time(missions[i]["expiry"]), v.Time(missions[j]["expiry"])
		if a.IsZero() {
			return false
		}
		if b.IsZero() {
			return true
		}
		return a.Before(b)
	})
	return v.Copy(M{"credits": t.credits, "creditsAt": t.creditsAt, "creditsSource": t.creditsSource, "fuel": t.fuel, "missions": missions, "navRoute": t.route, "navTarget": t.target, "routeAt": t.routeAt, "recent": t.recent, "session": t.session, "legalState": t.legal})
}
