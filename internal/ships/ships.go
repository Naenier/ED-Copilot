package ships

import (
	"embed"
	"encoding/json"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"strings"
	"unicode"
)

//go:embed ship-names.json
var files embed.FS

type Model struct {
	ID      string   `json:"id"`
	Symbol  string   `json:"symbol"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Aliases []string `json:"aliases"`
}

var models map[string]Model

func init() {
	b, _ := files.ReadFile("ship-names.json")
	var data struct {
		Ships []Model `json:"ships"`
	}
	if err := json.Unmarshal(b, &data); err != nil {
		panic(err)
	}
	models = map[string]Model{}
	for _, row := range data.Ships {
		for _, name := range append([]string{row.ID, row.Symbol, row.Name}, row.Aliases...) {
			if name != "" {
				models[key(name)] = row
			}
		}
	}
}
func clean(x any) string {
	s := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, v.Str(x))
	r := []rune(strings.TrimSpace(s))
	if len(r) > 180 {
		r = r[:180]
	}
	return string(r)
}
func key(s string) string {
	s = strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(s), "$shipname_"), ";")
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}
func DisplayShip(raw string, localized ...string) string {
	if m, ok := models[key(raw)]; ok {
		return m.Name
	}
	for _, s := range localized {
		if clean(s) != "" {
			return clean(s)
		}
	}
	return clean(raw)
}
func VehicleKind(raw string) string {
	k := key(raw)
	if m, ok := models[k]; ok {
		return m.Kind
	}
	if strings.Contains(k, "srv") || strings.Contains(k, "buggy") {
		return "srv"
	}
	if k == "onfoot" {
		return "foot"
	}
	return "ship"
}
func Identity(raw, localized string, id any, name, ident string) v.M {
	return v.M{"ship": DisplayShip(raw, localized), "shipType": clean(raw), "shipId": id, "shipName": clean(name), "shipIdent": clean(ident)}
}
func CAPI(ship v.M) v.M {
	return Identity(v.String(ship, "name"), v.String(ship, "locName"), ship["id"], v.String(ship, "shipName"), v.String(ship, "shipID"))
}
func ApplyEvent(current, event v.M) v.M {
	next := v.Copy(current)
	typ := v.String(event, "event")
	raw := v.Text(event["Ship"], event["ShipType"])
	switch typ {
	case "LoadGame", "Loadout", "ShipyardSwap", "ShipyardNew":
		if raw == "" {
			return next
		}
		kind := VehicleKind(raw)
		if typ == "LoadGame" {
			next["vehicle"] = kind
		}
		if kind != "ship" {
			next["vehicleType"] = raw
			next["vehicleName"] = DisplayShip(raw, v.String(event, "Ship_Localised"))
			next["vehicleId"] = event["ShipID"]
			return next
		}
		id := event["ShipID"]
		if typ == "ShipyardNew" {
			id = v.First(event["NewShipID"], id)
		}
		changed := key(raw) != key(v.String(current, "shipType")) || (id != nil && current["shipId"] != nil && v.Str(id) != v.Str(current["shipId"]))
		if id == nil && !changed {
			id = current["shipId"]
		}
		name, ident := v.String(current, "shipName"), v.String(current, "shipIdent")
		if changed {
			name = ""
			ident = ""
		}
		if value, ok := event["ShipName"]; ok {
			name = clean(value)
		}
		if value, ok := event["ShipIdent"]; ok {
			ident = clean(value)
		}
		v.Merge(next, Identity(raw, v.String(event, "Ship_Localised"), id, name, ident))
		if typ != "Loadout" {
			next["vehicle"] = "ship"
			next["vehicleName"] = ""
			next["vehicleType"] = ""
			next["vehicleId"] = nil
		}
	case "SetUserShipName":
		if raw != "" && VehicleKind(raw) != "ship" || event["ShipID"] != nil && current["shipId"] != nil && v.Str(event["ShipID"]) != v.Str(current["shipId"]) || raw != "" && v.String(current, "shipType") != "" && key(raw) != key(v.String(current, "shipType")) {
			return next
		}
		if raw != "" && v.String(current, "shipType") == "" {
			v.Merge(next, Identity(raw, v.String(event, "Ship_Localised"), event["ShipID"], "", ""))
		}
		if x, ok := event["UserShipName"]; ok {
			next["shipName"] = clean(x)
		}
		if x, ok := event["UserShipId"]; ok {
			next["shipIdent"] = clean(x)
		} else if x, ok := event["UserShipID"]; ok {
			next["shipIdent"] = clean(x)
		}
	}
	return next
}
