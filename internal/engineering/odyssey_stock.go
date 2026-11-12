package engineering

import (
	"sort"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

var odysseyInventoryCategories = map[string]string{
	"item": "items", "component": "components", "data": "data", "consumable": "consumables",
}

var odysseyMaterialSymbols = func() map[string]string {
	out := map[string]string{}
	for key, row := range roadmapData.personalMaterials {
		out[v.Key(v.Text(row["symbol"], key))] = key
	}
	return out
}()

type odysseyStockGroup struct {
	known     bool
	updatedAt any
	counts    map[string]int
}

type odysseyStock struct {
	locker, backpack map[string]odysseyStockGroup
}

func readOdysseyStock(observations M) odysseyStock {
	return odysseyStock{
		locker:   odysseyInventory(v.Obj(observations["shipLocker"])),
		backpack: odysseyInventory(v.Obj(observations["backpack"])),
	}
}

func odysseyInventory(raw M) map[string]odysseyStockGroup {
	groups := make(map[string]odysseyStockGroup, len(odysseyInventoryCategories))
	for category, key := range odysseyInventoryCategories {
		group := v.Obj(raw[key])
		s := odysseyStockGroup{
			known:     v.Bool(group["known"]) && !v.Bool(group["truncated"]) && !v.Bool(group["uncertain"]) && v.Arr(group["entries"]) != nil,
			updatedAt: group["updatedAt"], counts: map[string]int{},
		}
		if s.known {
			for _, row := range v.Rows(group["entries"]) {
				mission := v.Str(row["missionId"])
				if mission != "" && mission != "0" && mission != "18446744073709551615" {
					continue
				}
				if material := odysseyMaterialSymbols[v.Key(row["symbol"])]; material != "" &&
					roadmapData.personalMaterials[material]["category"] == category {
					s.counts[material] += v.Int(row["count"])
				}
			}
		}
		groups[category] = s
	}
	return groups
}

func odysseyPickupMaterials(raw any, shared map[string]int) map[string]int {
	out := map[string]int{}
	for index, value := range v.Arr(raw) {
		if index >= 32 {
			break
		}
		if key := odysseyMaterialSymbols[v.Key(value)]; key != "" {
			out[key] = shared[key]
		}
	}
	return out
}

func odysseyMaterialProjection(visible, active, shared map[string]int, stock odysseyStock) []M {
	rows := make([]M, 0, len(visible))
	for _, ingredient := range ingredientTotals(visible) {
		key := v.Str(ingredient["key"])
		label := roadmapData.personalMaterials[key]
		category := v.Str(label["category"])
		locker, backpack := stock.locker[category], stock.backpack[category]
		observed, exact := locker.counts[key]+backpack.counts[key], locker.known && backpack.known
		needed, all := active[key], shared[key]
		reserved := max(0, all-needed)
		maxMissing := max(0, needed-max(0, observed-reserved))
		maxSharedMissing := max(0, all-observed)
		row := M{
			"key": key, "journalKey": v.Key(v.Text(label["symbol"], key)),
			"name": v.Text(label["name"], key), "english": v.Text(label["english"], key),
			"entryId": roadmapMaterialEntry(key, category), "category": category,
			"count": visible[key], "activeCount": needed, "sharedCount": all, "reservedOther": reserved,
			"locker": v.Nullable(float64(locker.counts[key]), locker.known), "lockerObservedAt": locker.updatedAt,
			"backpack": v.Nullable(float64(backpack.counts[key]), backpack.known), "backpackObservedAt": backpack.updatedAt,
			"owned": v.Nullable(float64(observed), exact), "knownOwned": observed,
			"missing": v.Nullable(float64(maxMissing), exact || maxMissing == 0), "missingAtMost": maxMissing,
			"sharedMissing": v.Nullable(float64(maxSharedMissing), exact || maxSharedMissing == 0), "sharedMissingAtMost": maxSharedMissing,
			"spare": v.Nullable(float64(max(0, observed-all)), exact),
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return v.Str(rows[i]["key"]) < v.Str(rows[j]["key"]) })
	return rows
}
