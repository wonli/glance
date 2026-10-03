package desktopui

import (
	"testing"

	"gioui.org/widget/material"

	"glance/internal/format/docx"
	"glance/internal/wordlayout"
)

func TestWordSinglePageActionFitsAcceptedPageAndManualZoomExitsFitMode(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)

	state := newDocumentSessionState("single-page.docx")
	state.word = word
	state.wordLayoutResult = &wordlayout.Result{
		DocumentID: state.documentID,
		Revision:   state.wordRevision,
		CanPreview: true,
		Pages:      []wordlayout.PageScene{{Index: 0, Clip: wordlayout.Rect{WidthPT: 595, HeightPT: 842}}},
	}
	state.wordLayoutAcceptedKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevision}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	action := findWordViewTestAction(t, "view-single-page")
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("View > Single Page should be enabled for Word")
	}

	app.executeWordToolbarAction(action.id)
	if state.wordViewMode != wordViewPrintLayout || !state.wordPageFitMode || !wordToolbarActionActive(state, action) {
		t.Fatalf("single-page action state = mode %d / fit %t / active %t", state.wordViewMode, state.wordPageFitMode, wordToolbarActionActive(state, action))
	}
	wordGridlineSemantics(app)
	if state.wordLayoutZoom <= wordStatusZoomMinimum || state.wordLayoutZoom >= 1 {
		t.Fatalf("fitted portrait-page zoom = %.3f; want a value between the minimum and 100%%", state.wordLayoutZoom)
	}
	if word.Dirty() {
		t.Fatal("entering Single Page changed DOCX dirty state")
	}

	state.wordStatusZoomInButton.Click()
	wordGridlineSemantics(app)
	if state.wordPageFitMode {
		t.Fatal("manual status-bar zoom did not exit responsive page-fit mode")
	}
	if word.Dirty() {
		t.Fatal("manual view zoom changed DOCX dirty state")
	}
}

func TestWordPagePresetsStartAtSelectedCaretPageAndPreserveItAcrossModes(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)
	state := newDocumentSessionState("page-preset-position.docx")
	state.word = word
	state.selectedWordRunID = "run"
	pages := make([]wordlayout.PageScene, 5)
	for index := range pages {
		pages[index] = wordlayout.PageScene{Index: index, Clip: wordlayout.Rect{WidthPT: 300, HeightPT: 400}}
	}
	state.wordLayoutResult = &wordlayout.Result{
		DocumentID: state.documentID, Revision: state.wordRevision, CanPreview: true,
		Pages: pages, Locations: map[wordlayout.SourcePosition]wordlayout.Position{{RunID: "run", RuneOffset: 0}: {PageIndex: 3}},
	}
	state.wordLayoutAcceptedKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevision}
	app := &App{documentSessionState: state}
	app.selectWordSinglePageView()
	if state.wordPageListState.Position.First != 3 {
		t.Fatalf("Single Page started at page index %d; want the selected caret page 3", state.wordPageListState.Position.First)
	}
	app.selectWordMultiplePagesView()
	if state.wordPageListState.Position.First != 1 || wordCurrentPageIndex(state) != 2 {
		t.Fatalf("Multiple Pages position = row %d / page index %d; want row 1 / page index 2", state.wordPageListState.Position.First, wordCurrentPageIndex(state))
	}
	if word.Dirty() {
		t.Fatal("switching page view presets changed DOCX dirty state")
	}
}

func TestWordPageFitZoomUsesSmallerViewportRatioAndClampsToStatusRange(t *testing.T) {
	page := wordlayout.PageScene{Clip: wordlayout.Rect{WidthPT: 720, HeightPT: 480}}
	got := wordPageFitZoom(page, 800, 600, 1.5)
	want := float64(800) / (720 * 1.5 * 4.0 / 3.0)
	if got != want {
		t.Fatalf("page fit zoom = %.6f; want width-limited %.6f", got, want)
	}
	if got := wordPageFitZoom(wordlayout.PageScene{Clip: wordlayout.Rect{WidthPT: 100, HeightPT: 100}}, 800, 600, 1); got != wordStatusZoomMaximum {
		t.Fatalf("oversized fit zoom = %.3f; want max %.3f", got, wordStatusZoomMaximum)
	}
	if got := wordPageFitZoom(wordlayout.PageScene{Clip: wordlayout.Rect{WidthPT: 720, HeightPT: 1200}}, 200, 200, 1); got != wordStatusZoomMinimum {
		t.Fatalf("undersized fit zoom = %.3f; want min %.3f", got, wordStatusZoomMinimum)
	}
}
