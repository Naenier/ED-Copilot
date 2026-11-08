//go:build windows

package nativeui

import (
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"strings"
	"testing"
)

func TestReferenceBridgeLoadsBoundedCatalogueAndFullEntriesWithoutWholeWorld(t *testing.T) {
	backend := &scopedDispatchProbe{}
	backend.state = M{"engineering": M{"engineers": []M{{"key": "farseer", "status": "unlocked", "rank": 5}}}}
	for _, category := range []string{"articles", "ships", "engineers", "commodities", "modules", "materials", "blueprints", "experimentals", "systems"} {
		result, err := dispatchWeb(backend, "reference.list", M{"category": category, "limit": 100000})
		rows := v.Rows(result["entries"])
		if err != nil || len(rows) == 0 || len(rows) > 48 {
			t.Fatal(category, len(rows), err)
		}
		id := v.Str(rows[0]["id"])
		entry, err := dispatchWeb(backend, "reference.entry", M{"id": id})
		if err != nil || entry["id"] != id || v.Str(entry["title"]) == "" || len(v.Arr(entry["sections"])) == 0 {
			t.Fatal(category, id, err)
		}
		if _, ok := entry["world"]; ok {
			t.Fatal("whole-world payload on a detail request")
		}
	}
	if backend.snapshots != 0 {
		t.Fatal("reference requested full snapshot", backend.snapshots)
	}
	for _, scope := range backend.scopes {
		if scope != "sync" {
			t.Fatal("unrelated domain requested", scope)
		}
	}
	if _, err := dispatchWeb(backend, "reference.entry", M{"id": "../../private"}); err == nil {
		t.Fatal("unknown ID accepted")
	}
}

func TestReferenceScreensDoNotBuildLegacyTablesAndCarrierTradeUsesValidatedCommands(t *testing.T) {
	for _, tab := range []string{"encyclopedia", "engineering", "targets"} {
		backend := &scopedDispatchProbe{}
		backend.state = M{}
		mode := "Инженеры"
		if tab == "encyclopedia" {
			mode = "Статьи"
		}
		result, err := dispatchWeb(backend, "view", M{"tab": tab, "mode": mode})
		if err != nil {
			t.Fatal(err)
		}
		page := result["page"].(p.Page)
		if len(page.Rows) != 0 || len(page.Columns) != 0 || len(v.Rows(result["actions"])) != 0 {
			t.Fatal("legacy inspector table still built", tab)
		}
		if len(backend.scopes) != 1 || backend.scopes[0] != "reference" {
			t.Fatal(backend.scopes)
		}
	}
	backend := &dispatchProbeController{}
	_, err := dispatchWeb(backend, "carrier-trade", M{"values": M{"action": "preset", "preset": "booze-cruise"}})
	if err != nil || backend.snapshots != 0 || backend.path != "/api/carrier/trade" {
		t.Fatal(backend.path, err)
	}
	_, err = dispatchWeb(backend, "carrier-trade-markets", M{"values": M{"mode": "sell", "minPad": "M"}})
	if err != nil || !strings.HasSuffix(backend.path, "/trading/search") || backend.last["minPad"] != "M" {
		t.Fatal(backend.path, backend.last, err)
	}
}
