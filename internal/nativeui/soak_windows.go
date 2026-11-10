//go:build windows

package nativeui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	soakWarmup        = 10 * time.Second
	soakProbeInterval = 3200 * time.Millisecond
	soakProbeTimeout  = 3 * time.Second
	soakMemoryPeriod  = 20 * time.Second
)

// SoakFixture is supplied only by the isolated, offline command-line scenario.
type SoakFixture struct {
	Duration    time.Duration
	JournalFile string
	Fixture     SmokeFixture
}

type nativeSoak struct {
	dpi       int
	readyOnce sync.Once
	webReady  chan struct{}
	mu        sync.Mutex
	probe     *soakProbe
}

type soakProbe struct {
	system   string
	started  time.Time
	snapshot time.Time
	paint    chan soakPaint
}

type soakPaint struct {
	snapshot, finished time.Time
}

func (s *nativeSoak) snapshot(system string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if probe := s.probe; probe != nil && probe.system == system && probe.snapshot.IsZero() {
		probe.snapshot = time.Now()
	}
}

func (s *nativeSoak) painted(system string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if probe := s.probe; probe != nil && probe.system == system {
		select {
		case probe.paint <- soakPaint{snapshot: probe.snapshot, finished: time.Now()}:
		default:
		}
	}
}

func (m *modernShell) runSoak() {
	report := M{
		"ok": false, "schemaVersion": 1, "networkDisabled": true,
		"renderer":   "WebView2 · Go backend · native Win32 HUD",
		"foreground": "synthetic game identity with isolated application window bounds",
		"endpoint":   "matching HUD section blitted and Win32 EndPaint completed",
		"host":       smokeHost(m.soak.dpi),
	}
	if err := m.sampleSoak(report); err != nil {
		report["error"] = err.Error()
	}
	path := m.owner.options.SmokeReport
	b, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0700)
	}
	if err == nil {
		err = os.WriteFile(path, b, 0600)
	}
	if err != nil {
		nativeErrorLog(err, m.owner.backend.DataDir())
	}
	m.postUI(m.owner.quit)
}

func (m *modernShell) sampleSoak(report M) error {
	// This happens on a worker. The UI keeps running ordinary WebView polling
	// while the journal reader discovers the appended records on its own.
	stop := m.owner.stop
	select {
	case <-m.soak.webReady:
	case <-time.After(25 * time.Second):
		return fmt.Errorf("WebView did not request the initial view")
	case <-stop:
		return fmt.Errorf("the isolated window closed before initialization")
	}
	select {
	case <-time.After(soakWarmup):
	case <-stop:
		return fmt.Errorf("the isolated window closed during warmup")
	}

	fixture := m.owner.options.Soak
	file, err := os.OpenFile(fixture.JournalFile, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open synthetic journal: %w", err)
	}
	defer file.Close()

	start := time.Now()
	report["targetSeconds"] = fixture.Duration.Seconds()
	report["warmupSeconds"] = soakWarmup.Seconds()
	report["probeIntervalSeconds"] = soakProbeInterval.Seconds()
	report["memoryIntervalSeconds"] = soakMemoryPeriod.Seconds()
	report["fixture"] = M{"profile": fixture.Fixture.Name, "journalEvents": fixture.Fixture.JournalEvents, "journalBytes": fixture.Fixture.JournalBytes}
	timing := time.NewTicker(soakProbeInterval)
	memory := time.NewTicker(soakMemoryPeriod)
	end := time.NewTimer(fixture.Duration)
	defer timing.Stop()
	defer memory.Stop()
	defer end.Stop()
	memoryRows := []M{soakMemorySample(start)}
	samples := []M{}
	defer func() {
		memoryRows = append(memoryRows, soakMemorySample(start))
		report["elapsedSeconds"] = time.Since(start).Seconds()
		report["memory"] = memoryRows
		report["samples"] = samples
		report["latency"] = summarizeSoakLatency(samples)
		// The initial and final minutes help expose deterioration even when
		// the aggregate percentiles are stable.
		report["latencyFirstMinute"] = summarizeSoakLatency(soakWindow(samples, 0, 60))
		report["latencyLastMinute"] = summarizeSoakLatency(soakWindow(samples, fixture.Duration.Seconds()-60, fixture.Duration.Seconds()))
		report["peakSampledMemory"] = peakSoakMemory(memoryRows)
	}()

	for {
		select {
		case <-stop:
			return fmt.Errorf("the isolated window closed during measurement")
		case <-end.C:
			report["ok"] = len(samples) > 0
			return nil
		case <-memory.C:
			memoryRows = append(memoryRows, soakMemorySample(start))
		case <-timing.C:
			row, err := m.soak.measure(file, stop, len(samples)+1)
			if err != nil {
				return err
			}
			row["elapsedSeconds"] = time.Since(start).Seconds()
			samples = append(samples, row)
		}
	}
}

func soakWindow(samples []M, from, to float64) []M {
	var selected []M
	for _, row := range samples {
		elapsed := v.Num(row["elapsedSeconds"])
		if elapsed >= from && elapsed <= to {
			selected = append(selected, row)
		}
	}
	return selected
}

func summarizeSoakLatency(samples []M) M {
	stages := []string{"journalToSnapshot", "snapshotToPaint", "journalToPaint"}
	timings := make([]M, 0, len(samples)*len(stages))
	peaks := make(map[string]float64, len(stages))
	for _, row := range samples {
		for _, stage := range stages {
			if elapsed, ok := v.Number(row[stage]); ok && elapsed >= 0 {
				timings = append(timings, M{"category": stage, "milliseconds": elapsed})
				peaks[stage] = max(peaks[stage], elapsed)
			}
		}
	}
	result := summarizeSmokeTimings(timings)
	for stage, peak := range peaks {
		v.Obj(result[stage])["maxMilliseconds"] = peak
	}
	return result
}

func peakSoakMemory(samples []M) M {
	peak := M{}
	for _, row := range samples {
		for _, key := range []string{"appPrivateBytes", "webView2PrivateBytes", "appWorkingSetBytes", "webView2WorkingSetBytes", "goHeapAllocBytes", "goHeapInUseBytes"} {
			if bytes := v.Num(row[key]); bytes > v.Num(peak[key]) {
				peak[key] = bytes
			}
		}
	}
	return peak
}

func (s *nativeSoak) measure(file *os.File, stop <-chan struct{}, sequence int) (M, error) {
	system := fmt.Sprintf("HUD Test %04d", sequence)
	event := M{
		"event": "Location", "timestamp": v.ISO(time.Now()),
		"StarSystem": system, "SystemAddress": 990000000 + sequence,
		"StarPos": []int{sequence, 0, 0}, "Docked": false,
	}
	b, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	probe := &soakProbe{system: system, paint: make(chan soakPaint, 1)}
	s.mu.Lock()
	probe.started = time.Now()
	s.probe = probe
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.probe = nil
		s.mu.Unlock()
	}()
	if _, err := file.Write(append(b, '\n')); err != nil {
		return nil, fmt.Errorf("append synthetic journal record: %w", err)
	}
	select {
	case paint := <-probe.paint:
		if paint.snapshot.IsZero() {
			return nil, fmt.Errorf("HUD sample %d painted without matching snapshot", sequence)
		}
		return M{
			"sequence":          sequence,
			"journalToSnapshot": smokeMilliseconds(paint.snapshot.Sub(probe.started)),
			"snapshotToPaint":   smokeMilliseconds(paint.finished.Sub(paint.snapshot)),
			"journalToPaint":    smokeMilliseconds(paint.finished.Sub(probe.started)),
		}, nil
	case <-time.After(soakProbeTimeout):
		return nil, fmt.Errorf("HUD sample %d was not painted within %s", sequence, soakProbeTimeout)
	case <-stop:
		return nil, fmt.Errorf("the isolated window closed before HUD sample %d", sequence)
	}
}

func soakMemorySample(start time.Time) M {
	memory := smokeMemory()
	var heap runtime.MemStats
	runtime.ReadMemStats(&heap)
	memory["goHeapAllocBytes"] = heap.HeapAlloc
	memory["goHeapInUseBytes"] = heap.HeapInuse
	memory["goHeapObjects"] = heap.HeapObjects
	memory["goGCCycles"] = heap.NumGC
	memory["elapsedSeconds"] = time.Since(start).Seconds()
	return memory
}
