package nearest

import (
	_ "embed"
	"encoding/json"
	"math"
	"strings"
	"sync"
)

// Region grid from https://github.com/klightspeed/EliteDangerousRegionMap
// at 6c1191a58e1e593966f44f16235ab39d1ad24d84, under the included MIT license.
//
//go:embed codex-regions.json
var codexRegionData []byte

var codexRegions = sync.OnceValue(func() struct {
	Regions   []string
	RegionMap [][][2]int
} {
	var atlas struct {
		Regions   []string
		RegionMap [][][2]int
	}
	if err := json.Unmarshal(codexRegionData, &atlas); err != nil {
		panic(err)
	}
	return atlas
})

func orbitalRegion(x, z float64) string {
	column := int(math.Floor((x + 49985) * 83 / 4096))
	row := int(math.Floor((z + 24105) * 83 / 4096))
	atlas := codexRegions()
	if column < 0 || row < 0 || row >= len(atlas.RegionMap) {
		return ""
	}
	for _, span := range atlas.RegionMap[row] {
		if column < span[0] {
			if span[1] > 0 && span[1] < len(atlas.Regions) {
				return atlas.Regions[span[1]]
			}
			return ""
		}
		column -= span[0]
	}
	return ""
}

var orbitalRegionAliases = strings.NewReplacer("’", "'", "Fomorian", "Formorian")

func sameOrbitalRegion(one, other string) bool {
	return strings.EqualFold(orbitalRegionAliases.Replace(one), orbitalRegionAliases.Replace(other))
}
