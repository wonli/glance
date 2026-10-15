package desktopui

import (
	"strings"
	"testing"

	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/fontcatalog"
	"glance/internal/format/docx"
	"glance/internal/wordlayout"
)

func TestWordMultiplePagesShowsPairsFromSharedLayoutWithoutDirtyingDocument(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)
	state := newDocumentSessionState("multiple-pages.docx")
	state.word = word
	state.wordLayoutResult = &wordlayout.Result{
		DocumentID: state.documentID, Revision: state.wordRevision, CanPreview: true,
		Pages: []wordlayout.PageScene{
			{Index: 0, Clip: wordlayout.Rect{WidthPT: 595, HeightPT: 842}},
			{Index: 1, Clip: wordlayout.Rect{WidthPT: 595, HeightPT: 842}},
			{Index: 2, Clip: wordlayout.Rect{WidthPT: 595, HeightPT: 842}},
		},
	}
	state.wordLayoutAcceptedKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevision, FontVersion: fontcatalog.Default().Version}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	action := findWordViewTestAction(t, "view-multiple-pages")
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("View > Multiple Pages should be enabled for Word")
	}

	app.executeWordToolbarAction(action.id)
	semantics := wordGridlineSemantics(app)
	if state.wordViewMode != wordViewPrintLayout || !state.wordMultiplePagesMode || !state.wordPageFitMode || !wordToolbarActionActive(state, action) {
		t.Fatalf("multiple-page action state = mode %d / multiple %t / fit %t / active %t", state.wordViewMode, state.wordMultiplePagesMode, state.wordPageFitMode, wordToolbarActionActive(state, action))
	}
	if state.wordPageListState.List.Axis != layout.Vertical {
		t.Fatalf("multiple-page rows use list axis %d; want vertical pairs", state.wordPageListState.List.Axis)
	}
	if state.wordLayoutZoom >= 1 {
		t.Fatalf("two-page fit zoom = %.3f; want pages reduced to fit side by side", state.wordLayoutZoom)
	}
	if !strings.Contains(semantics, "Word 第 1 页") || !strings.Contains(semantics, "Word 第 2 页") {
		t.Fatalf("the first page pair is missing from the view: %s", semantics)
	}
	if word.Dirty() {
		t.Fatal("entering Multiple Pages changed DOCX dirty state")
	}

	state.wordStatusZoomInButton.Click()
	wordGridlineSemantics(app)
	if state.wordMultiplePagesMode || state.wordPageFitMode {
		t.Fatal("manual zoom did not return from the two-page fit preset")
	}
	if word.Dirty() {
		t.Fatal("manual view zoom changed DOCX dirty state")
	}
}

func TestWordPageFitZoomForTwoColumnsFitsWithinHalfWidth(t *testing.T) {
	page := wordlayout.PageScene{Clip: wordlayout.Rect{WidthPT: 720, HeightPT: 480}}
	got := wordPageFitZoomForColumns(page, 1600, 700, 1.5, 2)
	want := float64(1600/2) / (720 * 1.5 * 4.0 / 3.0)
	if got != want {
		t.Fatalf("two-column fit zoom = %.6f; want %.6f", got, want)
	}
}

func TestWordMultiplePagesStatusTracksLaterVisibleRow(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("multiple-pages-scroll.docx")
	state.word = word
	state.wordViewMode = wordViewPrintLayout
	state.wordMultiplePagesMode = true
	state.wordPageListState.Position.First = 2
	state.selectedWordRunID = "run"
	state.wordRunEditors["run"] = &widget.Editor{}
	pages := make([]wordlayout.PageScene, 10)
	for index := range pages {
		pages[index] = wordlayout.PageScene{Index: index, Clip: wordlayout.Rect{WidthPT: 300, HeightPT: 300}}
	}
	state.wordLayoutResult = &wordlayout.Result{
		CanPreview: true,
		Pages:      pages,
		Locations: map[wordlayout.SourcePosition]wordlayout.Position{
			{RunID: "run", RuneOffset: 0}: {PageIndex: 0},
		},
	}
	state.wordLayoutAcceptedKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevision, FontVersion: fontcatalog.Default().Version}
	app := &App{documentSessionState: state, theme: material.NewTheme()}

	semantics := wordGridlineSemantics(app)
	if state.wordPageListState.Position.First != 2 || state.wordLayoutActivePage != 4 {
		t.Fatalf("visible row=%d active page=%d; want row 2 / page index 4", state.wordPageListState.Position.First, state.wordLayoutActivePage)
	}
	if got := wordPaginationStatusText(state); !strings.Contains(got, "第 5 页") {
		t.Fatalf("print status = %q; want the first page in the visible later spread (accepted=%+v pending=%+v)", got, state.wordLayoutAcceptedKey, state.wordLayoutPendingKey)
	}
	if !strings.Contains(semantics, "Word 第 5 页，共 10 页") {
		t.Fatalf("the later page spread was not laid out: %s", semantics)
	}
}
