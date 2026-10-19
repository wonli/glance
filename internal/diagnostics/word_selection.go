package diagnostics

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const wordSelectionTraceCapacity = 512

// WordSelectionEvent contains geometry and opaque source IDs, never document text.
// Pointer and paint records share the same chronological trace.
type WordSelectionEvent struct {
	Time             time.Time `json:"time"`
	NativeThread     uint32    `json:"native_thread,omitempty"`
	WindowThread     uint32    `json:"window_thread,omitempty"`
	NativeClientX    float32   `json:"native_client_x,omitempty"`
	NativeClientY    float32   `json:"native_client_y,omitempty"`
	ViewportOriginX  int       `json:"viewport_origin_x,omitempty"`
	ViewportOriginY  int       `json:"viewport_origin_y,omitempty"`
	NativeNormalized bool      `json:"native_normalized,omitempty"`
	Phase            string    `json:"phase"`
	Page             int       `json:"page"`
	ViewMode         int       `json:"view_mode"`
	ReadingMode      bool      `json:"reading_mode"`
	PointerID        uint16    `json:"pointer_id,omitempty"`
	LocalX           float32   `json:"local_x"`
	LocalY           float32   `json:"local_y"`
	ViewportX        float32   `json:"viewport_x"`
	ViewportY        float32   `json:"viewport_y"`
	PageX            float32   `json:"page_x"`
	PageY            float32   `json:"page_y"`
	OriginX          int       `json:"origin_x"`
	OriginY          int       `json:"origin_y"`
	PixelsPerPoint   float64   `json:"pixels_per_point"`
	AnchorRun        string    `json:"anchor_run,omitempty"`
	AnchorOffset     int       `json:"anchor_offset"`
	FocusRun         string    `json:"focus_run,omitempty"`
	FocusOffset      int       `json:"focus_offset"`
	EditorStart      int       `json:"editor_start"`
	EditorEnd        int       `json:"editor_end"`
	Dragging         bool      `json:"dragging"`
	HitRun           string    `json:"hit_run,omitempty"`
	HitOffset        int       `json:"hit_offset"`
}

var wordSelectionTrace struct {
	enabled     atomic.Bool
	mu          sync.Mutex
	events      [wordSelectionTraceCapacity]WordSelectionEvent
	next, count int
}

// WordSelectionDiagnosticsEnabled lets the UI skip snapshot work when disabled.
func WordSelectionDiagnosticsEnabled() bool { return wordSelectionTrace.enabled.Load() }

// RecordWordSelection does no work unless the profiling listener was enabled.
func RecordWordSelection(event WordSelectionEvent) {
	if !wordSelectionTrace.enabled.Load() {
		return
	}
	event.Time = time.Now().UTC()
	wordSelectionTrace.mu.Lock()
	wordSelectionTrace.events[wordSelectionTrace.next] = event
	wordSelectionTrace.next = (wordSelectionTrace.next + 1) % wordSelectionTraceCapacity
	wordSelectionTrace.count = min(wordSelectionTrace.count+1, wordSelectionTraceCapacity)
	wordSelectionTrace.mu.Unlock()
}

func registerWordSelectionHandlers(mux *http.ServeMux) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		wordSelectionTrace.mu.Lock()
		events := make([]WordSelectionEvent, wordSelectionTrace.count)
		start := (wordSelectionTrace.next - wordSelectionTrace.count + wordSelectionTraceCapacity) % wordSelectionTraceCapacity
		for i := range events {
			events[i] = wordSelectionTrace.events[(start+i)%wordSelectionTraceCapacity]
		}
		wordSelectionTrace.mu.Unlock()
		writeDiagnosticJSON(w, struct {
			Capacity int                  `json:"capacity"`
			Events   []WordSelectionEvent `json:"events"`
		}{wordSelectionTraceCapacity, events})
	}
	mux.HandleFunc("GET /debug/word-selection", handler)
	mux.HandleFunc("GET /debug/pprof/word-selection", handler)
}
