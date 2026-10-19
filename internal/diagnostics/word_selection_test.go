package diagnostics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWordSelectionDiagnosticsBoundedAndObservational(t *testing.T) {
	wordSelectionTrace.enabled.Store(true)
	defer wordSelectionTrace.enabled.Store(false)
	wordSelectionTrace.mu.Lock()
	wordSelectionTrace.next = 0
	wordSelectionTrace.count = 0
	wordSelectionTrace.mu.Unlock()
	for i := 0; i < wordSelectionTraceCapacity+3; i++ {
		RecordWordSelection(WordSelectionEvent{Phase: "drag", FocusOffset: i, ReadingMode: true})
	}
	mux := http.NewServeMux()
	registerWordSelectionHandlers(mux)
	for _, path := range []string{"/debug/word-selection", "/debug/pprof/word-selection"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		var got struct {
			Events []WordSelectionEvent `json:"events"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Events) != wordSelectionTraceCapacity || got.Events[0].FocusOffset != 3 || got.Events[len(got.Events)-1].FocusOffset != wordSelectionTraceCapacity+2 {
			t.Fatalf("unexpected ring snapshot: %+v", got)
		}
		if !got.Events[0].ReadingMode {
			t.Fatal("missing reading mode")
		}
	}
	wordSelectionTrace.enabled.Store(false)
	RecordWordSelection(WordSelectionEvent{FocusOffset: -1})
	wordSelectionTrace.mu.Lock()
	defer wordSelectionTrace.mu.Unlock()
	if wordSelectionTrace.count != wordSelectionTraceCapacity {
		t.Fatal("disabled trace changed count")
	}
	if wordSelectionTrace.events[(wordSelectionTrace.next+wordSelectionTraceCapacity-1)%wordSelectionTraceCapacity].FocusOffset < 0 {
		t.Fatal("disabled trace recorded an event")
	}
}
