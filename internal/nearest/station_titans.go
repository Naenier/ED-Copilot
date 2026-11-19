package nearest

import (
	"context"
	"sort"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type titanSite struct {
	name, system, address string
	arrivalLs             int
}

// The debris-field systems are confirmed by the Xeno Strike Force field
// guide. Arrival distances are approximate. Cocijo's wreck is in Sol.
// https://xenostrikeforce.com/guides/scavenging-titan-graveyards/
var titanSites = []titanSite{
	{"Cocijo", "Sol", "10477373803", 504},
	{"Hadad", "HIP 30377", "79247165795", 39230},
	{"Indra", "HIP 20567", "1659744799067", 3330},
	{"Leigong", "HIP 8887", "216618994011", 2540},
	{"Oya", "Cephei Sector BV-Y b4", "9465705473417", 5850},
	{"Raijin", "Pegasi Sector IH-U b3-3", "7265876583833", 400},
	{"Taranis", "Hyades Sector FB-N b7-6", "13866167510393", 130},
	{"Thor", "Col 285 Sector IG-O c6-5", "1458443260610", 820},
}

func isStationTitan(name string) bool {
	switch strings.ToLower(name) {
	case "thargoid-titans", "thargoid titan":
		return true
	}
	return false
}

func (c *Client) searchTitanStations(ctx context.Context, q Query) (Result, error) {
	result := Result{Note: "Показаны известные поля обломков титанов. Расстояния от звезды приблизительные."}
	filters := q.Filters
	if stationText(filters["minPad"]) != "" || len(stationValues(filters["services"], 1)) != 0 {
		return Result{Note: "У обломков титанов нет посадочных площадок и станционных услуг."}, nil
	}
	for _, key := range []string{"economy", "government", "ownerAllegiance", "ownerGovernment", "materialTrader", "technologyBroker"} {
		if stationText(filters[key]) != "" {
			return result, nil
		}
	}
	if allegiance := stationText(filters["allegiance"]); allegiance != "" && !strings.EqualFold(allegiance, "Thargoid") {
		return result, nil
	}
	if stationText(filters["owner"]) != "" && filters["ownerMode"] != "not" ||
		v.Bool(filters["ownerPlayerOnly"]) || v.Bool(filters["ownerControllingOnly"]) ||
		len(stationValues(filters["ownerStates"], 1)) != 0 && filters["ownerStateMode"] != "none" {
		return result, nil
	}
	ids := []string{}
	byAddress := map[string]titanSite{}
	for _, site := range titanSites {
		if !matchesName(q.Name, site.name, site.system) {
			continue
		}
		if maximum, known := v.Number(filters["maxArrivalLs"]); known && float64(site.arrivalLs) > maximum {
			continue
		}
		ids = append(ids, site.address)
		byAddress[site.address] = site
	}
	if len(ids) == 0 {
		return result, nil
	}

	systemFilters := spaceSystemFilters(q)
	systemFilters["id64"] = stationValue(ids)
	systemQuery := q
	systemQuery.Sort = ""
	response, err := c.request(ctx, "/systems/search", stationBatch(systemQuery, systemFilters, 0, len(ids)))
	if err != nil {
		return result, err
	}
	for _, row := range v.Rows(response["results"]) {
		site, found := byAddress[v.Str(row["id64"])]
		if !found {
			continue
		}
		result.Rows = append(result.Rows, M{
			"name": site.name, "system": site.system, "systemAddress": site.address, "type": stationTitan,
			"distanceLy": row["distance"], "distanceLs": site.arrivalLs, "allegiance": "Thargoid",
		})
	}
	switch q.Sort {
	case "arrival", "distanceLs":
		sort.SliceStable(result.Rows, func(i, j int) bool { return v.Int(result.Rows[i]["distanceLs"]) < v.Int(result.Rows[j]["distanceLs"]) })
	case "name":
		sort.SliceStable(result.Rows, func(i, j int) bool { return v.Str(result.Rows[i]["name"]) < v.Str(result.Rows[j]["name"]) })
	}
	limit := q.Limit
	if limit < 1 || limit > DefaultLimit {
		limit = DefaultLimit
	}
	result.Total = len(result.Rows)
	start, end := min(q.Page*limit, result.Total), min((q.Page+1)*limit, result.Total)
	result.Rows, result.HasMore = result.Rows[start:end], end < result.Total
	return result, nil
}
