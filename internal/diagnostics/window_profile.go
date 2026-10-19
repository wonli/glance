package diagnostics

import (
	"context"
	"net/http"
	"runtime/pprof"
	"runtime/trace"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/pprof/profile"
)

var windowProfiling atomic.Bool
var windowTimings = struct {
	sync.Mutex
	values map[string][2]int64
}{values: make(map[string][2]int64)}

// WindowPhase measures application wall time, including waits, when diagnostics
// are enabled. Phase names must be fixed constants, never document/user data.
// It does not measure GPU execution or identify native move/resize sessions.
func WindowPhase(phase string, work func()) {
	if !windowProfiling.Load() {
		work()
		return
	}
	pprof.Do(context.Background(), pprof.Labels("window_phase", phase), func(ctx context.Context) {
		region := trace.StartRegion(ctx, "window."+phase)
		start := time.Now()
		defer func() {
			elapsed := time.Since(start).Nanoseconds()
			region.End()
			windowTimings.Lock()
			v := windowTimings.values[phase]
			v[0]++
			v[1] += elapsed
			windowTimings.values[phase] = v
			windowTimings.Unlock()
		}()
		work()
	})
}

// serveWindowLatency emits cumulative totals. Compare snapshots with pprof
// -base to isolate an interaction; completed phases from all windows aggregate.
func serveWindowLatency(w http.ResponseWriter, r *http.Request) {
	p := &profile.Profile{
		SampleType:        []*profile.ValueType{{Type: "calls", Unit: "count"}, {Type: "wall", Unit: "nanoseconds"}},
		DefaultSampleType: "wall",
		TimeNanos:         time.Now().UnixNano(),
	}
	windowTimings.Lock()
	values := make(map[string][2]int64, len(windowTimings.values))
	for k, v := range windowTimings.values {
		values[k] = v
	}
	windowTimings.Unlock()
	for phase, v := range values {
		id := uint64(len(p.Function) + 1)
		fn := &profile.Function{ID: id, Name: "window." + phase, SystemName: "window." + phase}
		loc := &profile.Location{ID: id, Line: []profile.Line{{Function: fn}}}
		p.Function = append(p.Function, fn)
		p.Location = append(p.Location, loc)
		p.Sample = append(p.Sample, &profile.Sample{Location: []*profile.Location{loc}, Value: []int64{v[0], v[1]}, Label: map[string][]string{"window_phase": {phase}}})
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_ = p.Write(w)
}
