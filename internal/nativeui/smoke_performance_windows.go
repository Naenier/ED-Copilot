//go:build windows

package nativeui

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"
	"unsafe"

	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/wailsapp/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// SmokeFixture contains only measurements from a disposable synthetic profile.
type SmokeFixture struct {
	Name           string
	JournalEvents  int
	JournalBytes   int
	Started        time.Time
	BackendElapsed time.Duration
}

func (m *modernShell) recordSmokeFirstView() {
	if m.smokeFirstView != nil {
		return
	}
	m.smokeFirstView = M{
		"milliseconds": smokeMilliseconds(time.Since(m.owner.options.SmokeFixture.Started)),
		"memory":       smokeMemory(),
	}
}

func (m *modernShell) smokePerformance(rawTimings any) M {
	fixture := m.owner.options.SmokeFixture
	result := M{
		"schemaVersion": 1,
		"fixture": M{
			"profile": fixture.Name, "journalEvents": fixture.JournalEvents, "journalBytes": fixture.JournalBytes,
		},
		"host":                     smokeHost(m.owner.main.DPI()),
		"backendStartMilliseconds": smokeMilliseconds(fixture.BackendElapsed),
		"interactions":             summarizeSmokeTimings(rawTimings),
		"endMemory":                smokeMemory(),
	}
	if m.smokeFirstView != nil {
		result["firstView"] = m.smokeFirstView
	}
	if m.smokeAfterUI != nil {
		result["afterInteractionsMemory"] = m.smokeAfterUI
	}
	return result
}

func smokeHost(dpi int) M {
	version := windows.RtlGetVersion()
	webview, _ := webviewloader.GetAvailableCoreWebView2BrowserVersionString("")
	host := M{
		"windows": fmt.Sprintf("%d.%d.%d", version.MajorVersion, version.MinorVersion, version.BuildNumber),
		"go":      runtime.Version(), "architecture": runtime.GOARCH,
		"logicalProcessors": runtime.NumCPU(), "windowDPI": dpi, "webView2": webview,
	}
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		defer key.Close()
		if name, _, err := key.GetStringValue("ProcessorNameString"); err == nil {
			host["cpu"] = strings.TrimSpace(name)
		}
	}
	return host
}

func smokeMilliseconds(elapsed time.Duration) float64 {
	return math.Round(float64(elapsed)/float64(time.Millisecond)*100) / 100
}

func summarizeSmokeTimings(rawTimings any) M {
	samples := map[string][]float64{}
	for _, row := range v.Rows(rawTimings) {
		category := v.String(row, "category")
		if ms, ok := v.Number(row["milliseconds"]); category != "" && ok && ms >= 0 {
			samples[category] = append(samples[category], ms)
		}
	}
	summary := M{}
	for category, values := range samples {
		sort.Float64s(values)
		count := len(values)
		summary[category] = M{
			"count": count, "p50Milliseconds": values[(count-1)/2],
			"p95Milliseconds": values[int(math.Ceil(0.95*float64(count)))-1],
		}
	}
	return summary
}

// processMemoryCounters matches PROCESS_MEMORY_COUNTERS_EX from psapi.h.
type processMemoryCounters struct {
	size, pageFaultCount             uint32
	peakWorkingSet, workingSet       uintptr
	peakPagedPool, pagedPool         uintptr
	peakNonPagedPool, nonPagedPool   uintptr
	pagefileUsage, peakPagefileUsage uintptr
	privateUsage                     uintptr
}

var getProcessMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

func smokeProcessMemory(process windows.Handle) (processMemoryCounters, bool) {
	counters := processMemoryCounters{size: uint32(unsafe.Sizeof(processMemoryCounters{}))}
	ok, _, _ := getProcessMemoryInfo.Call(uintptr(process), uintptr(unsafe.Pointer(&counters)), uintptr(counters.size))
	return counters, ok != 0
}

func smokeMemory() M {
	result := M{}
	if app, ok := smokeProcessMemory(windows.CurrentProcess()); ok {
		result["appWorkingSetBytes"] = uint64(app.workingSet)
		result["appPrivateBytes"] = uint64(app.privateUsage)
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return result
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return result
	}
	children := make(map[uint32][]windows.ProcessEntry32)
	for {
		children[entry.ParentProcessID] = append(children[entry.ParentProcessID], entry)
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}

	queue := []uint32{uint32(os.Getpid())}
	visited := map[uint32]bool{queue[0]: true}
	var workingSet, privateBytes uint64
	var processes, unread int
	for len(queue) != 0 {
		for _, child := range children[queue[0]] {
			if visited[child.ProcessID] {
				continue
			}
			visited[child.ProcessID] = true
			queue = append(queue, child.ProcessID)
			if !strings.EqualFold(windows.UTF16ToString(child.ExeFile[:]), "msedgewebview2.exe") {
				continue
			}
			processes++
			handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, child.ProcessID)
			if err != nil {
				unread++
				continue
			}
			memory, ok := smokeProcessMemory(handle)
			windows.CloseHandle(handle)
			if !ok {
				unread++
				continue
			}
			workingSet += uint64(memory.workingSet)
			privateBytes += uint64(memory.privateUsage)
		}
		queue = queue[1:]
	}
	result["webView2Processes"] = processes
	result["webView2UnreadProcesses"] = unread
	if processes > unread {
		result["webView2WorkingSetBytes"] = workingSet
		result["webView2PrivateBytes"] = privateBytes
	}
	return result
}
