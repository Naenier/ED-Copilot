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

// SaveRoute keeps an explicit quote snapshot. It never books a transaction or
// selects a destination; those are separate user actions.
func (s *Service) SaveRoute(body M) (M, error) {
	switch v.Str(body["action"]) {
	case "delete-route":
		id := v.Str(body["id"])
		for i, r := range s.SavedRoutes {
			if r["id"] == id {
				s.SavedRoutes = append(s.SavedRoutes[:i], s.SavedRoutes[i+1:]...)
				return M{"ok": true, "id": id}, nil
			}
		}
		return nil, errors.New("Сохранённый рейс не найден")
	case "save-route":
		route := v.Copy(v.Obj(body["route"]))
		encoded, err := json.Marshal(route)
		if err != nil || len(encoded) > 128*1024 {
			return nil, errors.New("Данные рейса слишком велики или повреждены")
		}
		legs := v.Rows(route["legs"])
		if len(legs) < 1 || len(legs) > 2 {
			return nil, errors.New("Рейс должен содержать одно или два плеча")
		}
		identity := []string{}
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
				identity = append(identity, v.Str(m["system"]), v.Str(m["station"]))
			}
			identity = append(identity, v.Str(leg["key"]))
			for _, key := range []string{"quantity", "buyPrice", "sellPrice"} {
				n, ok := v.Number(leg[key])
				if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 1e12 || key == "quantity" && (n < 1 || n > 25000 || n != math.Floor(n)) {
					return nil, errors.New("Проверьте объём и котировки выбранного рейса")
				}
			}
		}
		hash := sha256.Sum256([]byte(strings.Join(identity, "\x00")))
		id := "saved-" + hex.EncodeToString(hash[:8])
		route["searchID"] = route["id"]
		route["id"] = id
		route["savedAt"] = v.ISO(time.Now())
		route["quoteSnapshot"] = true
		title := strings.TrimSpace(v.Str(body["title"]))
		if title == "" {
			title = v.Str(v.Obj(legs[0]["buy"])["system"]) + " → " + v.Str(v.Obj(legs[0]["sell"])["system"])
		}
		if len([]rune(title)) > 160 {
			return nil, errors.New("Название рейса слишком длинное")
		}
		route["title"] = title
		for i, r := range s.SavedRoutes {
			if r["id"] == id {
				s.SavedRoutes[i] = route
				return M{"ok": true, "id": id}, nil
			}
		}
		if len(s.SavedRoutes) >= 40 {
			return nil, errors.New("Сохранено 40 рейсов. Удалите ненужный рейс перед добавлением нового")
		}
		s.SavedRoutes = append([]M{route}, s.SavedRoutes...)
		return M{"ok": true, "id": id}, nil
	default:
		return nil, errors.New("Неизвестное действие сохранённых рейсов")
	}
}

func (s *Service) SavedRoutesSnapshot() []M { return v.Rows(v.Clone(s.SavedRoutes)) }
