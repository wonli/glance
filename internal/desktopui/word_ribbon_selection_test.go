package desktopui

import (
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
)

func TestWordTextClickKeepsSelectedRibbonTab(t *testing.T) {
	word, state, app, router, frame := newWordJoinInteraction(t, "first", "second")
	defer word.Close()
	state.wordToolbarTab = wordTabView
	runID := state.wordBlocks[0].Paragraph.Inlines[0].Text.ID

	focusWordRunForInteraction(t, state, router, frame, runID)
	if state.wordToolbarTab != wordTabView {
		t.Fatalf("Ribbon tab after clicking flow text = %v, want View", state.wordToolbarTab)
	}
	var ops op.Ops
	app.focusWordPageRun(layout.Context{Ops: &ops}, state, runID, 0, 0)
	if state.wordToolbarTab != wordTabView {
		t.Fatalf("Ribbon tab after clicking page text = %v, want View", state.wordToolbarTab)
	}
}

func TestClearingWordImageSelectionKeepsRegularRibbonTab(t *testing.T) {
	state := newDocumentSessionState("selection.docx")
	state.wordToolbarTab = wordTabView
	state.selectedWordImageID = "image"

	clearWordImageSelection(state)
	if state.wordToolbarTab != wordTabView {
		t.Fatalf("Ribbon tab after clearing image selection = %v, want View", state.wordToolbarTab)
	}
	state.wordToolbarTab = wordTabPicture
	state.selectedWordImageID = "image"
	clearWordImageSelection(state)
	if state.wordToolbarTab != wordTabHome {
		t.Fatalf("Ribbon tab after leaving Picture = %v, want Home", state.wordToolbarTab)
	}
}
