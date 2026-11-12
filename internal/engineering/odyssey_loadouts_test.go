package engineering

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func saveOdysseyTestKit(t *testing.T, p *Planner, scope M, name string, reserve bool, items ...M) M {
	t.Helper()
	state, err := p.UpdateOdysseyLoadout(M{"action": "save", "name": name, "reserve": reserve, "items": items}, scope)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func odysseyTestItem(model string, grade int, mods ...string) M {
	return M{"itemKey": model, "targetGrade": grade, "modKeys": mods}
}

func TestOdysseyLoadoutsShareReserveAndKeepBackpackAndLockerSeparate(t *testing.T) {
	p := New(nil)
	stamp := "2026-11-13T10:00:00Z"
	shipLocker := M{"items": M{"known": true, "updatedAt": stamp, "entries": []M{
		{"symbol": "suitschematic", "count": 2},
		{"symbol": "$suitschematic_name;", "count": 10, "missionId": "123456789"},
	}}}
	backpack := M{"items": M{"known": true, "updatedAt": stamp, "entries": []M{{"symbol": "suitschematic", "count": 1}}}}
	scope := M{"planWorkspace": "live-test", "shipLocker": shipLocker, "backpack": backpack}
	first := saveOdysseyTestKit(t, p, scope, "Scavenger", true, odysseyTestItem("utilitysuit", 3))
	saveOdysseyTestKit(t, p, scope, "Combat", true, odysseyTestItem("tacticalsuit", 2))
	saveOdysseyTestKit(t, p, scope, "Preview", false, odysseyTestItem("explorationsuit", 2))
	state, err := p.UpdateOdysseyLoadout(M{"action": "select", "id": first["selectedId"]}, scope)
	if err != nil {
		t.Fatal(err)
	}
	row := byKey(v.Rows(state["materials"]))["suitschematic"]
	for key, expected := range map[string]int{
		"count": 3, "sharedCount": 4, "reservedOther": 1, "locker": 2, "backpack": 1, "owned": 3, "missing": 1, "sharedMissing": 1,
	} {
		if row[key] == nil || v.Int(row[key]) != expected {
			t.Errorf("%s = %v, expected %d", key, row[key], expected)
		}
	}
	if row["lockerObservedAt"] != stamp || row["backpackObservedAt"] != stamp {
		t.Fatal("missing inventory observation dates", row)
	}
	backpack["items"] = M{"known": true, "truncated": true, "updatedAt": stamp, "entries": []M{{"symbol": "suitschematic", "count": 1}}}
	row = byKey(v.Rows(p.OdysseyLoadouts(scope)["materials"]))["suitschematic"]
	if row["backpack"] != nil || row["owned"] != nil || row["missing"] != nil || row["knownOwned"] != 2 || row["missingAtMost"] != 2 {
		t.Fatal("an incomplete inventory was treated as empty or authoritative", row)
	}
	backpack["items"] = M{"known": true, "updatedAt": stamp, "entries": []M{}}
	row = byKey(v.Rows(p.OdysseyLoadouts(scope)["materials"]))["suitschematic"]
	if row["backpack"] == nil || v.Int(row["backpack"]) != 0 || v.Int(row["missing"]) != 2 {
		t.Fatal("observed zero was treated as unknown", row)
	}
	if state := p.OdysseyLoadouts(M{"planWorkspace": "other"}); state["activeLoadout"] != nil || len(v.Rows(state["loadouts"])) != 0 {
		t.Fatal("another commander could read the kits", state)
	}
}

func TestOdysseyOwnedProgressIsDeduplicatedAndRestored(t *testing.T) {
	const id = "9007199254740993"
	const selectedMod = "suit_increasedmeleedamage"
	base := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	p := New(nil)
	scope := M{"planWorkspace": "live-f1"}
	suit := func(kind string, seconds, grade int, mods any) M {
		row := M{"event": kind, "timestamp": v.ISO(base.Add(time.Duration(seconds) * time.Second)), "SuitID": json.Number(id), "Class": grade}
		if kind == "SuitLoadout" {
			row["SuitName"] = "UtilitySuit_Class1"
		} else {
			row["Name"] = "UtilitySuit_Class1"
		}
		if mods != nil {
			row["SuitMods"] = mods
		}
		return row
	}
	p.Apply(suit("SuitLoadout", 1, 1, []string{}), scope)
	item := odysseyTestItem("utilitysuit", 3, selectedMod)
	item["equipmentKey"] = "suit:" + id
	first := saveOdysseyTestKit(t, p, scope, "Scavenging", true, item)
	saveOdysseyTestKit(t, p, scope, "Light combat", true, item)
	state := p.OdysseyLoadouts(scope)
	if row := byKey(v.Rows(state["portfolioMaterials"]))["suitschematic"]; v.Int(row["sharedCount"]) != 3 {
		t.Fatal("the same physical suit was upgraded twice in the reserve", row)
	}
	p.Apply(suit("UpgradeSuit", 2, 2, nil), scope)
	p.Apply(suit("UpgradeSuit", 2, 2, nil), scope)
	state = p.OdysseyLoadouts(scope)
	if row := byKey(v.Rows(state["materials"]))["suitschematic"]; v.Int(row["count"]) != 2 {
		t.Fatal("grade replay incorrectly deducted or repeated the step", row)
	}
	p.Apply(suit("SuitLoadout", 3, 2, []string{selectedMod}), scope)
	p.Apply(suit("UpgradeSuit", 4, 3, nil), scope)
	state, err := p.UpdateOdysseyLoadout(M{"action": "select", "id": first["selectedId"]}, scope)
	if err != nil {
		t.Fatal(err)
	}
	check := func(p *Planner) {
		t.Helper()
		state := p.OdysseyLoadouts(scope)
		active := v.Rows(v.At(state, "activeLoadout", "items"))[0]
		if state["selectedId"] != first["selectedId"] || v.Int(active["currentGrade"]) != 3 || !contains(active["installedMods"], selectedMod) ||
			len(v.Rows(state["materials"])) != 0 || len(v.Rows(state["portfolioMaterials"])) != 0 {
			t.Fatal("selection or confirmed equipment progress lost", state["activeLoadout"], state["materials"])
		}
	}
	check(p)
	payload, err := json.Marshal(p.ExportSettings())
	if err != nil {
		t.Fatal(err)
	}
	saved := M{}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&saved); err != nil {
		t.Fatal(err)
	}
	p = New(saved)
	check(p)
	p.Apply(suit("SuitLoadout", 1, 1, []string{}), scope)
	p.Apply(suit("UpgradeSuit", 2, 2, nil), scope)
	p.Apply(suit("UpgradeSuit", 4, 3, nil), scope)
	check(p)
	p.Apply(M{"event": "SellSuit", "timestamp": v.ISO(base.Add(time.Minute)), "SuitID": json.Number(id)}, scope)
	p = New(p.ExportSettings())
	state = p.OdysseyLoadouts(scope)
	active := v.Rows(v.At(state, "activeLoadout", "items"))[0]
	if active["currentGrade"] != nil || !contains(active["conditions"], "equipment-unavailable") || len(v.Rows(state["ownedEquipment"])) != 0 {
		t.Fatal("a sold suit was treated as available after restart", active)
	}
}

func TestOdysseyRejectsIncompatibleEquipmentAndPlansSharedModsConservatively(t *testing.T) {
	p := New(nil)
	scope := M{"planWorkspace": "live-f1"}
	for _, items := range [][]M{
		{odysseyTestItem("utilitysuit", 2), odysseyTestItem("wpn_m_assaultrifle_kinetic_fauto", 2), odysseyTestItem("wpn_m_assaultrifle_laser_fauto", 2)},
		{odysseyTestItem("wpn_m_assaultrifle_kinetic_fauto", 2, "weapon_range_laser")},
		{odysseyTestItem("utilitysuit", 2, "suit_increasedmeleedamage", "suit_improvedarmourrating")},
	} {
		if _, err := p.UpdateOdysseyLoadout(M{"action": "save", "name": "Invalid", "items": items}, scope); err == nil {
			t.Fatal("incompatible equipment, slots or mod accepted", items)
		}
	}
	p.Apply(M{"event": "SuitLoadout", "timestamp": "2026-11-13T10:00:00Z", "SuitID": "777", "SuitName": "utilitysuit_class2", "SuitMods": []string{}}, scope)
	for i, mod := range []string{"suit_increasedmeleedamage", "suit_improvedarmourrating"} {
		item := odysseyTestItem("utilitysuit", 2, mod)
		item["equipmentKey"] = "suit:777"
		state := saveOdysseyTestKit(t, p, scope, mod, true, item)
		if i == 0 {
			continue
		}
		active := v.Rows(v.At(state, "activeLoadout", "items"))[0]
		if state["portfolioComplete"] != false || !contains(active["conditions"], "incompatible-goals") ||
			v.Int(byKey(v.Rows(state["portfolioMaterials"]))["microthrusters"]["count"]) != 0 {
			t.Fatal("one owned suit was reserved for incompatible modifications", state["portfolioMaterials"], active)
		}
	}
	scope["recentPickupKeys"] = []string{"titaniumplating", "graphene"}
	hud := p.OdysseyHUD(scope)
	if hud["equipment"] != nil || hud["ownedEquipment"] != nil || v.At(hud, "activeLoadout", "items") != nil ||
		len(v.Rows(hud["portfolioMaterials"])) != 2 {
		t.Fatal("overlay got editor data or did not get the pickup classification", hud)
	}
}
