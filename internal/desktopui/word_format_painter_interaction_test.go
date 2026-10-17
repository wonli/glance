package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/document"
)

func TestWordPageFormatPainterAppliesOnlyDraggedTargetRange(t *testing.T) {
	app, state, router, release := newWordSelectionPageTest(t, `<w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>Source</w:t></w:r></w:p><w:p><w:pPr><w:jc w:val="right"/></w:pPr><w:r><w:rPr><w:i/></w:rPr><w:t>前目标后</w:t></w:r></w:p>`)
	defer release()
	sourceParagraph, targetParagraph := state.wordBlocks[0].Paragraph, state.wordBlocks[1].Paragraph
	source, target := sourceParagraph.Inlines[0].Text, targetParagraph.Inlines[0].Text
	state.wordFormatPainterActive = true
	state.wordFormatPainterSourceRunID = source.ID
	var ops op.Ops
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(600, 500)}}
		app.layoutWordPages(gtx, state, wordPageContinuous)
		app.processWordFormatPainterTargetIntent()
		router.Frame(&ops)
	}
	frame()
	interaction := state.wordPageVisibleInteractions[0]
	start := interaction.Transform.PageToScreen(f32.Pt(20.1, 60))
	end := interaction.Transform.PageToScreen(f32.Pt(40.1, 60))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 72, Buttons: pointer.ButtonPrimary, Position: start, Time: time.Second})
	frame()
	if state.wordPageSelectionDrag == nil || state.wordFormatPainterPendingRunID != target.ID || !state.wordFormatPainterActive {
		t.Fatalf("format painter did not wait for page selection release: drag=%v pending=%q active=%v", state.wordPageSelectionDrag != nil, state.wordFormatPainterPendingRunID, state.wordFormatPainterActive)
	}
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 72, Buttons: pointer.ButtonPrimary, Position: end, Time: time.Second + 20*time.Millisecond})
	frame()
	if selected, ok := wordPageSelectedText(state); !ok || selected != "目标" {
		t.Fatalf("page selection = %q, %v; want target substring", selected, ok)
	}
	if !state.wordFormatPainterActive || state.formatRunning.Load() {
		t.Fatal("format painter applied before the page selection was released")
	}
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 72, Position: end, Time: time.Second + 40*time.Millisecond})
	frame()
	if state.wordPageSelectionDrag != nil {
		t.Fatal("page selection drag remained active after release")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatal(result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for selected format-painter operation")
	}
	painted, ok := selectedWordParagraph(state.wordBlocks, targetParagraph.ID)
	if !ok || len(painted.Inlines) != 3 {
		t.Fatalf("painted target paragraph = %#v; want prefix/selected/suffix fragments", painted)
	}
	prefix, selectedRun, suffix := painted.Inlines[0].Text, painted.Inlines[1].Text, painted.Inlines[2].Text
	if prefix.Text != "前" || selectedRun.Text != "目标" || suffix.Text != "后" || !prefix.Style.Italic || prefix.Style.Bold || !selectedRun.Style.Bold || !suffix.Style.Italic || suffix.Style.Bold {
		t.Fatalf("format painter did not limit character formatting to selected text: %#v", painted.Inlines)
	}
	if painted.Alignment != document.WordAlignCenter {
		t.Fatalf("painted target paragraph alignment = %q; want source alignment %q", painted.Alignment, document.WordAlignCenter)
	}
}
