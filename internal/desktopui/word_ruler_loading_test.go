package desktopui

import (
	"bytes"
	"image"
	"image/color"
	"runtime"
	"testing"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/wordlayout"
)

func TestWordRulerLoadingDoesNotExposeLegacyControls(t *testing.T) {
	state, closeDoc := newLongWordFlowState(t, 1)
	defer closeDoc()
	state.wordCanvasWidthDP = 900
	state.selectedWordParagraphID = state.wordBlocks[0].Paragraph.ID
	app := &App{documentSessionState: state, theme: material.NewTheme(), wordLayoutResults: make(chan wordLayoutResult, 1)}
	var ops op.Ops
	var router input.Router
	for _, phase := range []string{"before-pagination", "pending", "failed", "empty", "unsupported"} {
		state.wordLayoutResult = nil
		state.wordLayoutPendingKey = nil
		state.wordLayoutError = ""
		switch phase {
		case "pending":
			state.wordLayoutPendingKey = &wordLayoutKey{}
		case "failed":
			state.wordLayoutError = "failed"
		case "empty":
			state.wordLayoutResult = &wordlayout.Result{CanPreview: true}
		case "unsupported":
			state.wordLayoutResult = &wordlayout.Result{Pages: []wordlayout.PageScene{{}}}
		}
		ops.Reset()
		dims := app.layoutWordHorizontalRuler(layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1100, 28))}, state)
		router.Frame(&ops)
		if dims.Size != image.Pt(1100, 28) {
			t.Fatalf("%s changed reserved ruler geometry: %v", phase, dims.Size)
		}
		var visit func([]input.SemanticNode)
		visit = func(nodes []input.SemanticNode) {
			for _, node := range nodes {
				if node.Desc.Description == app.textID("word.accessibility.resize_ruler") || node.Desc.Description == "拖动调整段落首行缩进" {
					t.Errorf("%s exposed ruler control %q before page geometry was ready", phase, node.Desc.Description)
				}
				visit(node.Children)
			}
		}
		visit(router.AppendSemantics(nil))
	}
}

func TestWordRulerLoadingToReadyVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, density := range []float32{1, 2} {
		state, closeDoc := newLongWordFlowState(t, 1)
		defer closeDoc()
		state.selectedWordParagraphID = state.wordBlocks[0].Paragraph.ID
		state.wordCanvasWidthDP = 900
		app := &App{documentSessionState: state, theme: material.NewTheme(), wordLayoutResults: make(chan wordLayoutResult, 1)}
		data, ok := wordLayoutDataForState(state)
		if !ok {
			t.Fatal("missing layout metadata")
		}
		key := wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevisionAtomic.Load(), DraftEpoch: state.wordDraftEpoch.Load(), PageSetupKey: wordLayoutPageSetupKey(data, defaultWordPageSetup)}
		state.wordLayoutPendingKey = &key
		var ops op.Ops
		var router input.Router
		size := image.Pt(int(1100*density), int(28*density))
		frame := func() *image.RGBA {
			ops.Reset()
			app.layoutWordHorizontalRuler(layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: density, PxPerSp: density}, Constraints: layout.Exact(size)}, state)
			router.Frame(&ops)
			return renderPresentationTestOps(t, &ops, size)
		}
		loading := frame()
		for y := 0; y < size.Y; y++ {
			for x := 0; x < size.X; x++ {
				if got := color.NRGBAModel.Convert(loading.At(x, y)).(color.NRGBA); got != wordWorkspaceBackground {
					t.Fatalf("%vx loading frame paints provisional ruler at %d,%d: %v", density, x, y, got)
				}
			}
		}
		scene := wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Clip: wordlayout.Rect{WidthPT: 600, HeightPT: 800}, Content: wordlayout.Rect{XPT: 60, WidthPT: 480}}}}
		app.applyWordLayoutResult(wordLayoutResult{state: state, key: key, result: scene, isPartial: true})
		first := frame()
		if bytes.Equal(first.Pix, loading.Pix) {
			t.Fatal("accepted first page did not draw ruler")
		}
		app.applyWordLayoutResult(wordLayoutResult{state: state, key: key, result: scene})
		complete := frame()
		stable := frame()
		if !bytes.Equal(first.Pix, complete.Pix) || !bytes.Equal(complete.Pix, stable.Pix) {
			t.Fatal("ruler changed between accepted first page, completed layout and stable frame")
		}
	}
}

func TestWordRulerReadyResultsDoNotWaitForUserInput(t *testing.T) {
	state, closeDoc := newLongWordFlowState(t, 1)
	defer closeDoc()
	app := &App{documentSessionState: state, theme: material.NewTheme(), wordLayoutResults: make(chan wordLayoutResult, 2)}
	data, ok := wordLayoutDataForState(state)
	if !ok {
		t.Fatal("missing metadata")
	}
	key := wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevisionAtomic.Load(), DraftEpoch: state.wordDraftEpoch.Load(), PageSetupKey: wordLayoutPageSetupKey(data, defaultWordPageSetup)}
	state.wordLayoutPendingKey = &key
	page := wordlayout.PageScene{Clip: wordlayout.Rect{WidthPT: 600, HeightPT: 800}, Content: wordlayout.Rect{XPT: 60, WidthPT: 480}}
	app.wordLayoutResults <- wordLayoutResult{state: state, key: key, result: wordlayout.Result{Pages: []wordlayout.PageScene{page}}, isPartial: true}
	app.wordLayoutResults <- wordLayoutResult{state: state, key: key, result: wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{page}}}
	app.processAsyncResults()
	if state.wordLayoutAcceptedKey == nil || state.wordLayoutResult == nil || !state.wordLayoutResult.CanPreview || len(app.wordLayoutResults) != 0 {
		t.Fatal("ready scene remains queued until another frame/input")
	}
	if state.selectedWordRunID != "" {
		t.Fatal("pagination should not need text selection")
	}
}

func TestWordRulerControlsVisibleWithoutTextSelection(t *testing.T) {
	state, closeDoc := newLongWordFlowState(t, 1)
	defer closeDoc()
	state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Clip: wordlayout.Rect{WidthPT: 600, HeightPT: 800}, Content: wordlayout.Rect{XPT: 60, WidthPT: 480}}}}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	var ops op.Ops
	var router input.Router
	app.layoutWordHorizontalRuler(layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1100, 28))}, state)
	router.Frame(&ops)
	found := map[string]bool{}
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, n := range nodes {
			if !n.Desc.Bounds.Empty() {
				found[n.Desc.Description] = true
			}
			visit(n.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	for _, name := range []string{"拖动调整段落首行缩进", "拖动调整段落悬挂缩进", "拖动调整段落左缩进", "拖动调整段落右缩进"} {
		if !found[name] {
			t.Errorf("initial ruler missing %s without clicking text", name)
		}
	}
	if state.selectedWordRunID != "" || state.selectedWordParagraphID != "" {
		t.Fatal("painting ruler changed text selection")
	}
}

func TestWordRulerKeepsContextAfterTextSelectionClears(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, density := range []float32{1, 2} {
		state, closeDoc := newLongWordFlowState(t, 2)
		defer closeDoc()
		state.wordBlocks[1].Paragraph.LeftIndentEMU = wordEMUPerCM
		state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Clip: wordlayout.Rect{WidthPT: 600, HeightPT: 800}, Content: wordlayout.Rect{XPT: 60, WidthPT: 480}}}}
		app := &App{documentSessionState: state, theme: material.NewTheme()}
		var ops op.Ops
		var router input.Router
		size := image.Pt(int(1100*density), int(28*density))
		frame := func() *image.RGBA {
			ops.Reset()
			app.layoutWordHorizontalRuler(layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: density, PxPerSp: density}, Constraints: layout.Exact(size)}, state)
			router.Frame(&ops)
			return renderPresentationTestOps(t, &ops, size)
		}
		initial := frame()
		state.selectedWordParagraphID = state.wordBlocks[0].Paragraph.ID
		selected := frame()
		if !bytes.Equal(initial.Pix, selected.Pix) {
			t.Fatal("first click changed the initial ruler")
		}
		state.selectedWordParagraphID = state.wordBlocks[1].Paragraph.ID
		second := frame()
		if bytes.Equal(selected.Pix, second.Pix) {
			t.Fatal("ruler failed to follow active paragraph")
		}
		state.selectedWordParagraphID = ""
		state.selectedWordRunID = ""
		blurred := frame()
		if !bytes.Equal(second.Pix, blurred.Pix) {
			t.Fatal("ruler lost paragraph markers on blur")
		}
		if state.selectedWordParagraphID != "" || state.selectedWordRunID != "" {
			t.Fatal("ruler rendering changed text selection")
		}
	}
}
