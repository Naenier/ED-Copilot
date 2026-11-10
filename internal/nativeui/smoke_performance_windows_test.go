//go:build windows

package nativeui

import (
	"reflect"
	"testing"
)

func TestSummarizeSmokeTimings(t *testing.T) {
	var rows []M
	for ms := 20; ms >= 1; ms-- {
		rows = append(rows, M{"category": "navigation", "milliseconds": ms})
	}
	rows = append(rows,
		M{"category": "formOpen", "milliseconds": 7.5},
		M{"category": "navigation", "milliseconds": -1},
		M{"category": "navigation"},
		M{"milliseconds": 50},
	)
	want := M{
		"navigation": M{"count": 20, "p50Milliseconds": 10.0, "p95Milliseconds": 19.0},
		"formOpen":   M{"count": 1, "p50Milliseconds": 7.5, "p95Milliseconds": 7.5},
	}
	if got := summarizeSmokeTimings(rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("summary = %v, want %v", got, want)
	}
}
