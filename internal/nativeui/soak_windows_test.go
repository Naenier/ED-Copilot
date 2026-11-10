//go:build windows

package nativeui

import (
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestSoakCorrelatesOnlyMatchingSnapshotAndPaint(t *testing.T) {
	started := time.Now()
	probe := &soakProbe{system: "HUD Test 0001", started: started, paint: make(chan soakPaint, 1)}
	s := nativeSoak{probe: probe}
	s.snapshot("Unrelated system")
	s.painted("HUD Test 0000")
	if !probe.snapshot.IsZero() || len(probe.paint) != 0 {
		t.Fatal("a different system satisfied the probe")
	}
	s.snapshot(probe.system)
	first := probe.snapshot
	s.snapshot(probe.system)
	s.painted(probe.system)
	s.painted(probe.system)
	select {
	case got := <-probe.paint:
		if got.snapshot != first || got.snapshot.Before(started) || got.finished.Before(got.snapshot) {
			t.Fatalf("wrong snapshot or paint time: %+v", got)
		}
	default:
		t.Fatal("matching system did not satisfy the probe")
	}
	if len(probe.paint) != 0 {
		t.Fatal("duplicate paint queued another result")
	}
}

func TestSoakSummariesUseSamplesFromTheRequestedWindow(t *testing.T) {
	samples := []M{
		{"elapsedSeconds": 5.0, "journalToPaint": 300.0},
		{"elapsedSeconds": 45.0, "journalToPaint": 1000.0},
		{"elapsedSeconds": 70.0, "journalToPaint": 800.0},
	}
	firstMinute := summarizeSoakLatency(soakWindow(samples, 0, 60))
	lastMinute := summarizeSoakLatency(soakWindow(samples, 40, 100))
	if v.Num(v.At(firstMinute, "journalToPaint", "p50Milliseconds")) != 300 ||
		v.Num(v.At(lastMinute, "journalToPaint", "p50Milliseconds")) != 800 ||
		v.Num(v.At(lastMinute, "journalToPaint", "maxMilliseconds")) != 1000 {
		t.Fatalf("unexpected timing windows: %v, %v", firstMinute, lastMinute)
	}
	peak := peakSoakMemory([]M{
		{"appPrivateBytes": uint64(100), "webView2PrivateBytes": uint64(200)},
		{"appPrivateBytes": uint64(80), "webView2PrivateBytes": uint64(250)},
	})
	if v.Num(peak["appPrivateBytes"]) != 100 || v.Num(peak["webView2PrivateBytes"]) != 250 {
		t.Fatalf("unexpected per-process peaks: %v", peak)
	}
}
