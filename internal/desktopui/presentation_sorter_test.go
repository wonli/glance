package desktopui

import (
	"encoding/json"
	"errors"
	"image"
	"testing"

	gioapp "gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/application"
)

func TestPresentationSorterRibbonUsesSharedViewIntent(t *testing.T) {
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	app := newAppWithRuntime(nil, runtime)
	app.window = &gioapp.Window{}
	app.windowID = 1
	app.handleHomeAction("pptx")
	view := app.ensurePresentationView()
	if err := app.pptxDoc.AddSlide(); err != nil {
		t.Fatalf("add second slide fixture: %v", err)
	}
	view.snapshot = app.pptxDoc.Snapshot()
	active := view.currentSlide()
	view.editingShape = active.Elements[0].ID
	view.shapeEditor.SetText(active.Elements[0].PlainText())
	app.markPresentationTextEditorOpen(view, true)

	app.handlePresentationRibbonAction(view, "view_sorter")
	raw, err := marshalPresentationViewState(app.windowID, view, false)
	if err != nil {
		t.Fatalf("marshal presentation view: %v", err)
	}
	var state struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("decode presentation view: %v", err)
	}
	if state.Mode != "sorter" {
		t.Fatalf("view sorter Ribbon mode = %q, want sorter; data=%s", state.Mode, raw)
	}
	if view.editingShape != 0 || app.documentSessionState.hasOpenPresentationTextEditor() {
		t.Fatalf("sorter mode kept the hidden text editor active: editingShape=%d open=%v", view.editingShape, app.documentSessionState.hasOpenPresentationTextEditor())
	}
	secondSlideID := view.snapshot.Slides[1].ID
	view.sorterButton(secondSlideID).Click()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1200, 700))}
	app.layoutPresentationSorter(gtx, view)
	if view.sorterMode || view.activeSlide != 1 {
		t.Fatalf("selecting sorter slide should return to normal view on slide 2: sorter=%v activeSlide=%d", view.sorterMode, view.activeSlide)
	}
	app.handlePresentationRibbonAction(view, "view_normal")
	raw, err = marshalPresentationViewState(app.windowID, view, false)
	if err != nil {
		t.Fatalf("marshal normal view: %v", err)
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("decode normal view: %v", err)
	}
	if state.Mode != "normal" {
		t.Fatalf("view normal Ribbon mode = %q, want normal; data=%s", state.Mode, raw)
	}
}

func TestPresentationSorterResolvesStableSlideIDAgainstCurrentSnapshot(t *testing.T) {
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	app := newAppWithRuntime(nil, runtime)
	app.window = &gioapp.Window{}
	app.handleHomeAction("pptx")
	view := app.ensurePresentationView()
	for range 3 {
		if err := app.pptxDoc.AddSlide(); err != nil {
			t.Fatalf("add slide fixture: %v", err)
		}
	}
	view.snapshot = app.pptxDoc.Snapshot()
	targetID := view.snapshot.Slides[2].ID
	if err := app.pptxDoc.DeleteSlide(0); err != nil {
		t.Fatalf("delete slide before sorter selection: %v", err)
	}
	view.sorterMode = true

	app.selectPresentationSorterSlide(view, targetID)
	current := app.pptxDoc.Snapshot()
	if view.sorterMode || view.activeSlide >= len(current.Slides) || current.Slides[view.activeSlide].ID != targetID {
		t.Fatalf("sorter selection did not resolve the stable slide ID: sorter=%v activeSlide=%d target=%d current=%+v", view.sorterMode, view.activeSlide, targetID, current.Slides)
	}
}

type testPresentationsorterOperationModeArguments struct {
	Operation string `json:"operation"`
	Mode      string `json:"mode"`
}

func TestPresentationSorterRejectsPendingTextDraft(t *testing.T) {
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	app := newAppWithRuntime(nil, runtime)
	app.window = &gioapp.Window{}
	app.handleHomeAction("pptx")
	view := app.ensurePresentationView()
	slide := view.currentSlide()
	if slide == nil || len(slide.Elements) == 0 {
		t.Fatal("blank presentation fixture has no editable text element")
	}
	view.editingShape = slide.Elements[0].ID
	view.shapeEditor.SetText(slide.Elements[0].PlainText() + " draft")

	raw, err := json.Marshal(testPresentationsorterOperationModeArguments{Operation: "set_mode", Mode: "sorter"})
	if err != nil {
		t.Fatalf("marshal sorter intent: %v", err)
	}
	if _, err := app.applyPresentationViewIntent("presentation.view.set", raw); !errors.Is(err, application.ErrPendingEditorDraft) {
		t.Fatalf("sorter mode with pending text draft error = %v, want pending editor draft", err)
	}
}
