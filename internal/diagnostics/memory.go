package diagnostics

import (
	"encoding/json"
	"glance/internal/fontdiag"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

type processMemory struct {
	ResidentBytes          uint64 `json:"resident_bytes,omitempty"`
	PhysicalFootprintBytes uint64 `json:"physical_footprint_bytes,omitempty"`
	Error                  string `json:"error,omitempty"`
}

type goMemory struct {
	HeapAlloc             uint64 `json:"heap_alloc"`
	HeapInuse             uint64 `json:"heap_inuse"`
	HeapIdle              uint64 `json:"heap_idle"`
	HeapReleased          uint64 `json:"heap_released"`
	HeapUnused            uint64 `json:"heap_inuse_unused"`
	HeapIdleUnreleased    uint64 `json:"heap_idle_unreleased"`
	Sys                   uint64 `json:"sys"`
	RuntimeMappedEstimate uint64 `json:"runtime_mapped_estimate"`
	StackInuse            uint64 `json:"stack_inuse"`
	OtherSys              uint64 `json:"other_sys"`
	NumGC                 uint32 `json:"num_gc"`
}

type memorySnapshot struct {
	Time     time.Time     `json:"time"`
	PID      int           `json:"pid"`
	Platform string        `json:"platform"`
	Go       goMemory      `json:"go"`
	Process  processMemory `json:"process"`
}

func readMemorySnapshot() memorySnapshot {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return memorySnapshot{
		Time: time.Now().UTC(), PID: os.Getpid(), Platform: runtime.GOOS,
		Go: goMemory{
			HeapAlloc: m.HeapAlloc, HeapInuse: m.HeapInuse, HeapIdle: m.HeapIdle, HeapReleased: m.HeapReleased,
			HeapUnused: m.HeapInuse - m.HeapAlloc, HeapIdleUnreleased: m.HeapIdle - m.HeapReleased,
			Sys: m.Sys, RuntimeMappedEstimate: m.Sys - m.HeapReleased,
			StackInuse: m.StackInuse, OtherSys: m.OtherSys, NumGC: m.NumGC,
		}, Process: readProcessMemory(),
	}
}

func writeDiagnosticJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(value)
}

// These routes exist only on the explicitly enabled profiling listener. GET is
// observational; the intrusive GC/scavenge experiment requires a separate POST.
func registerMemoryHandlers(mux *http.ServeMux) {
	var releaseMu sync.Mutex
	mux.HandleFunc("GET /debug/fonts", func(w http.ResponseWriter, r *http.Request) { writeDiagnosticJSON(w, fontdiag.Read()) })
	mux.HandleFunc("GET /debug/memory", func(w http.ResponseWriter, r *http.Request) {
		writeDiagnosticJSON(w, readMemorySnapshot())
	})
	mux.HandleFunc("POST /debug/memory/release", func(w http.ResponseWriter, r *http.Request) {
		if !releaseMu.TryLock() {
			http.Error(w, "memory release already running", http.StatusConflict)
			return
		}
		defer releaseMu.Unlock()
		before := readMemorySnapshot()
		start := time.Now()
		// Free workspace SQLite memory before OS memory reclamation.
		invokeWorkspaceShrinker()
		// FreeOSMemory already performs a GC, then returns idle heap pages to the OS.
		// The HTTP worker runs this, never the Gio frame/event loop.
		debug.FreeOSMemory()
		after := readMemorySnapshot()
		writeDiagnosticJSON(w, struct {
			Before     memorySnapshot `json:"before"`
			After      memorySnapshot `json:"after"`
			DurationMS int64          `json:"duration_ms"`
		}{before, after, time.Since(start).Milliseconds()})
	})
}
