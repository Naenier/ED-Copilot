package exobiology

import (
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type SiteQuery struct {
	Query, SpeciesID, Region string
	Coordinates              []float64
	RadiusLy                 float64
	Offset, Limit            int
}

var sitesOnce sync.Once
var knownSites []M

// Sites searches a bundled, public observation index. It is not a live game
// database. Coordinates in this index are boxel estimates and omit Y: distances
// must therefore be labelled as lower bounds, never navigation-grade distances.
func (d *Database) Sites(q SiteQuery) M {
	sitesOnce.Do(func() {
		raw := data.Object("exobiology-sites.json")
		types := v.Arr(raw["types"])
		byKey := map[string]M{}
		for region, value := range v.Obj(raw["regions"]) {
			for _, rawRow := range v.Arr(v.Obj(value)["bio"]) {
				row := v.Arr(rawRow)
				if len(row) < 5 {
					continue
				}
				address := v.Str(row[0])
				key := region + "|" + address
				site := byKey[key]
				if site == nil {
					site = M{"systemAddress": address, "system": row[1], "region": region, "approximateX": row[2], "approximateZ": row[3], "approximate": true, "coordinatesKnown": false, "observations": []M{}}
					byKey[key] = site
				}
				for _, index := range v.Arr(row[4]) {
					i := v.Int(index)
					if i < 0 || i >= len(types) {
						continue
					}
					t := v.Arr(types[i])
					if len(t) < 2 {
						continue
					}
					sp := d.resolve(t[1])
					if sp == nil {
						continue
					}
					site["observations"] = append(v.Rows(site["observations"]), M{"speciesId": sp.ID, "name": sp.Name, "name_ru": sp.Name, "name_en": sp.EnglishName, "variant": t[1], "codexId": t[0]})
				}
			}
		}
		for _, key := range sortedKeys(byKey) {
			site := byKey[key]
			if len(v.Rows(site["observations"])) > 0 {
				knownSites = append(knownSites, site)
			}
		}
	})
	limit := q.Limit
	if limit <= 0 {
		limit = 24
	}
	limit = min(limit, 48)
	rows := []M{}
	regions := map[string]int{}
	requested := d.resolve(q.SpeciesID)
	needle := strings.ToLower(strings.TrimSpace(q.Query))
	for _, site := range knownSites {
		regions[v.Str(site["region"])]++
		if q.Region != "" && q.Region != "all" && q.Region != site["region"] {
			continue
		}
		match := q.SpeciesID == ""
		for _, o := range v.Rows(site["observations"]) {
			if requested != nil && o["speciesId"] == requested.ID || normal(o["codexId"]) == normal(q.SpeciesID) {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		if needle != "" {
			text := strings.ToLower(v.Str(site["system"]) + " " + v.Str(site["region"]))
			for _, o := range v.Rows(site["observations"]) {
				text += " " + strings.ToLower(v.Str(o["name"])+" "+v.Str(o["name_en"])+" "+v.Str(o["variant"]))
			}
			if !strings.Contains(text, needle) {
				continue
			}
		}
		row := v.Copy(site)
		row["distanceLy"], row["distanceLowerBoundLy"] = nil, nil
		if len(q.Coordinates) == 3 {
			dx, dz := q.Coordinates[0]-v.Num(site["approximateX"]), q.Coordinates[2]-v.Num(site["approximateZ"])
			distance := math.Sqrt(dx*dx + dz*dz)
			if q.RadiusLy > 0 && distance > q.RadiusLy {
				continue
			}
			row["distanceLowerBoundLy"] = distance
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if a, b := v.Num(rows[i]["distanceLowerBoundLy"]), v.Num(rows[j]["distanceLowerBoundLy"]); a != b {
			return a < b
		}
		return v.Str(rows[i]["system"]) < v.Str(rows[j]["system"])
	})
	total := len(rows)
	offset := min(max(0, q.Offset), total)
	groups := []M{}
	for _, region := range sortedKeys(regions) {
		groups = append(groups, M{"id": region, "title": region, "count": regions[region]})
	}
	return M{"entries": rows[offset:min(offset+limit, total)], "total": total, "offset": offset, "limit": limit, "regions": groups, "live": false, "distanceBasis": "approximate-xz-lower-bound", "observationDate": "2026-10-01"}
}
