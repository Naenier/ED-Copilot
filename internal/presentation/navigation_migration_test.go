package presentation

import (
	"reflect"
	"testing"
)

func TestSidebarUpgradePreservesPersonalMenu(t *testing.T) {
	legacy := []string{"overview", "targets", "sale", "encyclopedia", "statistics", "settings"}
	if got := MigrateNavigation(legacy, 0); !reflect.DeepEqual(got, DefaultTabs) {
		t.Fatal("old untouched default did not discover new Academy", got)
	}
	if got := MigrateNavigation(legacy, 2); !reflect.DeepEqual(got, legacy) {
		t.Fatal("explicitly saved visibility changed", got)
	}
	custom := []string{"settings", "carrier-calculator", "learning", "overview", "carrier"}
	if got := MigrateNavigation(custom, 0); !reflect.DeepEqual(got, custom) {
		t.Fatal("custom order changed", got)
	}
	if got := MigrateNavigation(nil, 0); !reflect.DeepEqual(got, DefaultTabs) {
		t.Fatal("new profile default", got)
	}
}
