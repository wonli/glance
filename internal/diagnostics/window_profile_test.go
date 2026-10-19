package diagnostics

import (
	"bytes"
	"net/http/httptest"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/google/pprof/profile"
)

func TestWindowProfile(t *testing.T) {
	windowProfiling.Store(true)
	defer windowProfiling.Store(false)
	WindowPhase("layout", func() {
		var stacks bytes.Buffer
		if err := pprof.Lookup("goroutine").WriteTo(&stacks, 1); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stacks.String(), `"window_phase":"layout"`) {
			t.Fatal("missing CPU/goroutine phase label")
		}
		time.Sleep(time.Millisecond)
	})
	r := httptest.NewRecorder()
	serveWindowLatency(r, httptest.NewRequest("GET", "/debug/pprof/window-latency", nil))
	p, err := profile.ParseData(r.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.CheckValid(); err != nil {
		t.Fatal(err)
	}
	if len(p.Sample) == 0 {
		t.Fatal("missing samples")
	}
	found := false
	for _, s := range p.Sample {
		if s.Label["window_phase"][0] == "layout" {
			found = true
			if s.Value[0] < 1 || s.Value[1] < int64(time.Millisecond) {
				t.Fatalf("values=%v", s.Value)
			}
		}
	}
	if !found {
		t.Fatal("missing layout")
	}
}

func TestWindowPhaseDisabled(t *testing.T) {
	windowProfiling.Store(false)
	windowTimings.Lock()
	before := windowTimings.values["layout"]
	windowTimings.Unlock()
	called := false
	WindowPhase("layout", func() { called = true })
	windowTimings.Lock()
	after := windowTimings.values["layout"]
	windowTimings.Unlock()
	if before != after {
		t.Fatal("disabled profiling recorded timing")
	}
	if !called {
		t.Fatal("disabled profiling skipped work")
	}
}
