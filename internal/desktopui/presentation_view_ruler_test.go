package desktopui

import (
	"encoding/json"
	"testing"

	gioapp "gioui.org/app"
)

func TestPresentationViewRulerRibbonUsesSharedViewIntent(t *testing.T) {
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	app := newAppWithRuntime(nil, runtime)
	app.window = &gioapp.Window{}
	app.windowID = 1
	app.handleHomeAction("pptx")
	view := app.ensurePresentationView()
	beforeRevision := app.pptxDoc.Snapshot().Revision

	app.handlePresentationRibbonAction(view, "view_ruler")
	raw, err := marshalPresentationViewState(app.windowID, view, false)
	if err != nil {
		t.Fatalf("marshal presentation view: %v", err)
	}
	var state struct {
		RulerVisible bool `json:"ruler_visible"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("decode presentation view: %v", err)
	}
	if !state.RulerVisible {
		t.Fatalf("view ruler Ribbon did not enable the ruler through the view intent: %s", raw)
	}

	app.handlePresentationRibbonAction(view, "view_ruler")
	raw, err = marshalPresentationViewState(app.windowID, view, false)
	if err != nil {
		t.Fatalf("marshal presentation view after toggle: %v", err)
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("decode presentation view after toggle: %v", err)
	}
	if state.RulerVisible {
		t.Fatalf("second view ruler click should hide ruler: %s", raw)
	}
	if got := app.pptxDoc.Snapshot().Revision; got != beforeRevision {
		t.Fatalf("ruler visibility changed document revision from %d to %d", beforeRevision, got)
	}
}
