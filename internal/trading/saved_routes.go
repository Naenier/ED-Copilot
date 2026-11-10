package trading

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"strings"
	"time"
)

const savedRouteOwner = "planOwner"

// SaveRoute keeps an explicit quote snapshot. It never books a transaction or
// selects a destination; those are separate user actions.
func (s *Service) SaveRoute(body M) (M, error) {
	return s.SaveRouteFor(body, "")
}

func (s *Service) SaveRouteFor(body M, activeOwner string) (M, error) {
	switch v.Str(body["action"]) {
	case "delete-route", "set-scope":
		id := v.Str(body["id"])
		for i, r := range s.SavedRoutes {
			owner := v.Str(r[savedRouteOwner])
			if r["id"] != id || owner != "" && owner != activeOwner {
				continue
			}
			if body["action"] == "delete-route" {
				s.SavedRoutes = append(s.SavedRoutes[:i], s.SavedRoutes[i+1:]...)
				return M{"ok": true, "id": id}, nil
			}
			next, err := routeOwner(body, activeOwner, owner)
			if err != nil {
				return nil, err
			}
			if next != owner {
				key := savedRouteKey(v.Rows(r["legs"]))
				for _, other := range s.SavedRoutes {
					if v.Str(other[savedRouteOwner]) == next && savedRouteKey(v.Rows(other["legs"])) == key {
						return nil, errors.New("У этого владельца уже есть такой рейс.")
					}
				}
				setSavedRouteOwner(r, next)
			}
			return M{"ok": true, "id": id}, nil
		}
		return nil, errors.New("Сохранённый рейс не найден")
	case "save-route":
		owner, err := routeOwner(body, activeOwner, activeOwner)
		if err != nil {
			return nil, err
		}
		route := v.Obj(body["route"])
		encoded, err := json.Marshal(route)
		if err != nil || len(encoded) > 128*1024 {
			return nil, errors.New("Данные рейса слишком велики или повреждены")
		}
		legs := v.Rows(route["legs"])
		if len(legs) < 1 || len(legs) > 2 {
			return nil, errors.New("Рейс должен содержать одно или два плеча")
		}
		for _, leg := range legs {
			for _, field := range []string{"key", "name"} {
				if len(v.Str(leg[field])) > 200 {
					return nil, errors.New("Некорректное название товара")
				}
			}
			if v.Str(leg["key"]) == "" {
				return nil, errors.New("Не выбран товар рейса")
			}
			for _, side := range []string{"buy", "sell"} {
				m := v.Obj(leg[side])
				for _, key := range []string{"system", "station"} {
					if strings.TrimSpace(v.Str(m[key])) == "" || len([]rune(v.Str(m[key]))) > 150 {
						return nil, errors.New("Укажите систему и станцию обеих точек рейса")
					}
				}
			}
			for _, key := range []string{"quantity", "buyPrice", "sellPrice"} {
				n, ok := v.Number(leg[key])
				if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 1e12 || key == "quantity" && (n < 1 || n > 25000 || n != math.Floor(n)) {
					return nil, errors.New("Проверьте объём и котировки выбранного рейса")
				}
			}
		}
		title := strings.TrimSpace(v.Str(body["title"]))
		if title == "" {
			title = v.Str(v.Obj(legs[0]["buy"])["system"]) + " → " + v.Str(v.Obj(legs[0]["sell"])["system"])
		}
		if len([]rune(title)) > 160 {
			return nil, errors.New("Название рейса слишком длинное")
		}
		route = v.Obj(v.Clone(route))
		key := savedRouteKey(legs)
		id := "saved-" + key
		route["searchID"] = route["id"]
		route["savedAt"] = v.ISO(time.Now())
		route["quoteSnapshot"] = true
		route["title"] = title
		delete(route, "scope")
		setSavedRouteOwner(route, owner)
		for i, r := range s.SavedRoutes {
			if v.Str(r[savedRouteOwner]) == owner && savedRouteKey(v.Rows(r["legs"])) == key {
				id = v.Str(r["id"])
				route["id"] = id
				s.SavedRoutes[i] = route
				return M{"ok": true, "id": id}, nil
			}
			if r["id"] == id {
				id = "saved-" + v.ID()
			}
		}
		if len(s.SavedRoutes) >= 40 {
			return nil, errors.New("Сохранено 40 рейсов. Удалите ненужный рейс перед добавлением нового")
		}
		route["id"] = id
		s.SavedRoutes = append([]M{route}, s.SavedRoutes...)
		return M{"ok": true, "id": id}, nil
	default:
		return nil, errors.New("Неизвестное действие сохранённых рейсов")
	}
}

func (s *Service) SavedRoutesSnapshot() []M { return v.Rows(v.Clone(s.SavedRoutes)) }

func (s *Service) SavedRoutesFor(owner string) []M {
	visible := make([]M, 0, len(s.SavedRoutes))
	for _, r := range s.SavedRoutes {
		key := v.Str(r[savedRouteOwner])
		if key == "" || key == owner {
			visible = append(visible, r)
		}
	}
	rows := v.Rows(v.Clone(visible))
	for _, row := range rows {
		key := v.Str(row[savedRouteOwner])
		delete(row, savedRouteOwner)
		row["scope"] = "shared"
		if key != "" {
			row["scope"] = "personal"
		}
	}
	return rows
}

func routeOwner(body M, active, fallback string) (string, error) {
	raw, present := body["scope"]
	if !present {
		return fallback, nil
	}
	switch raw {
	case "shared":
		return "", nil
	case "personal":
		if active != "" {
			return active, nil
		}
		return "", errors.New("Для личного плана нужен командир из журнала игры.")
	default:
		return "", errors.New("Выберите область плана.")
	}
}

func setSavedRouteOwner(row M, owner string) {
	if owner == "" {
		delete(row, savedRouteOwner)
	} else {
		row[savedRouteOwner] = owner
	}
}

func savedRouteKey(legs []M) string {
	var parts []string
	for _, leg := range legs {
		for _, side := range []string{"buy", "sell"} {
			point := v.Obj(leg[side])
			parts = append(parts, v.Str(point["system"]), v.Str(point["station"]))
		}
		parts = append(parts, v.Str(leg["key"]))
	}
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:8])
}
