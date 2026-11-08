package frontend

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/Naenier/ED-Copilot/internal/data"
)

var rankAssetCache sync.Map

// RankInsignia returns a bounded selection of offline game art. Discipline and
// numeric rank select only known embedded files; no caller-supplied path is read.
func RankInsignia(input map[string]any) (map[string]any, error) {
	var entries []any
	switch raw := input["entries"].(type) {
	case []any:
		entries = raw
	case []map[string]any:
		for _, entry := range raw {
			entries = append(entries, entry)
		}
	default:
		return nil, errors.New("rank insignia entries must be an array")
	}
	if len(entries) == 0 || len(entries) > 16 {
		return nil, errors.New("rank insignia requires between 1 and 16 entries")
	}
	images := make(map[string]any, len(entries))
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("rank insignia entry must be an object")
		}
		id, ok := entry["id"].(string)
		maximum, navy := 13, false
		switch id {
		case "combat", "trade", "explore", "cqc", "soldier", "exobiologist":
		case "federation", "empire":
			maximum, navy = 14, true
		default:
			return nil, errors.New("unknown rank insignia discipline")
		}
		index, valid := insigniaRankIndex(entry["rankIndex"])
		if !ok || !valid || index < 0 || index > maximum {
			return nil, errors.New("rank insignia index is outside its discipline")
		}
		key, file := fmt.Sprintf("%s/%d", id, index), fmt.Sprintf("rank-insignia/%s-%d.png", id, index)
		if navy {
			key, file = id, "rank-insignia/"+id+".png"
		}
		if cached, found := rankAssetCache.Load(file); found {
			images[key] = cached
			continue
		}
		bytes, err := data.Files.ReadFile(file)
		if err != nil {
			return nil, errors.New("embedded rank insignia is unavailable")
		}
		uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(bytes)
		cached, _ := rankAssetCache.LoadOrStore(file, uri)
		images[key] = cached
	}
	return map[string]any{"images": images}, nil
}

func insigniaRankIndex(value any) (int, bool) {
	var number float64
	switch value := value.(type) {
	case int:
		number = float64(value)
	case int64:
		number = float64(value)
	case float64:
		number = value
	case json.Number:
		var err error
		number, err = value.Float64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || number != math.Trunc(number) || number < 0 || number > 14 {
		return 0, false
	}
	return int(number), true
}
