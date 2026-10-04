package desktopui

import (
	"image"
	"strings"
	"testing"

	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordReadingModeSimplifiesChromeAndRestoresViewState(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)

	state := newDocumentSessionState("reading.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.wordBlocks[0].Paragraph.Inlines = append(state.wordBlocks[0].Paragraph.Inlines, document.WordInline{
		Kind:  document.WordImageInline,
		Image: &document.WordImage{ID: "reader-image", WidthEMU: 914400, HeightEMU: 914400},
	})
	state.wordImageCache.Put("reader-image", image.NewRGBA(image.Rect(0, 0, 1, 1)))
	state.wordLayoutPendingKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevision}
	state.wordToolbarTab = wordTabView
	state.wordLayoutZoom = 1.25
	state.wordGridlinesVisible = true
	state.wordListState.Position.First = 2
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	action := findWordViewTestAction(t, "view-reading")
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("View > Reading Mode should be enabled for a Word document")
	}
	if wordToolbarActionActive(state, action) {
		t.Fatal("Reading Mode should be inactive initially")
	}
	normalSemantics := wordGridlineSemantics(app)
	for _, visible := range []string{"Word 功能区", "Word 水平标尺", "Word 垂直标尺", "Word 网格线", "Word 状态栏"} {
		if !strings.Contains(normalSemantics, visible) {
			t.Fatalf("normal view is missing %q: %s", visible, normalSemantics)
		}
	}
	previousPosition := state.wordListState.Position

	app.executeWordToolbarAction(action.id)
	if !wordToolbarActionActive(state, action) {
		t.Fatal("Reading Mode did not become active")
	}
	if state.wordViewMode != wordViewContinuous || state.wordToolbarTab != wordTabView || state.wordLayoutZoom != 1.25 || state.wordListState.Position != previousPosition || !state.wordGridlinesVisible {
		t.Fatalf("entering Reading Mode changed the saved view, scroll, zoom, tab, or gridline state: position=%+v want=%+v", state.wordListState.Position, previousPosition)
	}
	readingSemantics := wordGridlineSemantics(app)
	state.wordObjectButtons["reader-image"].Click()
	wordGridlineSemantics(app)
	if state.selectedWordImageID != "" {
		t.Fatal("clicking an image in Reading Mode selected it")
	}
	for _, hidden := range []string{"Word 功能区", "Word 水平标尺", "Word 垂直标尺", "Word 网格线", "单击此处继续输入"} {
		if strings.Contains(readingSemantics, hidden) {
			t.Errorf("Reading Mode still exposes %q: %s", hidden, readingSemantics)
		}
	}
	for _, visible := range []string{"退出阅读模式", "Word 状态栏"} {
		if !strings.Contains(readingSemantics, visible) {
			t.Errorf("Reading Mode is missing %q: %s", visible, readingSemantics)
		}
	}
	blocks := wordBlocksForState(state)
	if len(blocks) == 0 || blocks[0].Paragraph == nil {
		t.Fatal("blank DOCX has no paragraph to verify read-only mode")
	}
	runID := blocks[0].Paragraph.Inlines[0].Text.ID
	if editor := state.wordRunEditors[runID]; editor == nil || !editor.ReadOnly {
		t.Fatalf("Reading Mode body editor = %#v; want read-only editor", editor)
	}
	if word.Dirty() {
		t.Fatal("entering Reading Mode changed DOCX dirty state")
	}

	state.wordReadingExitButton.Click()
	wordGridlineSemantics(app)
	if wordToolbarActionActive(state, action) {
		t.Fatal("exit Reading Mode button did not restore the previous view")
	}
	restoredSemantics := wordGridlineSemantics(app)
	if state.selectedWordImageID != "" {
		t.Fatal("a Reading Mode image click was replayed after exiting")
	}
	for _, visible := range []string{"Word 功能区", "Word 水平标尺", "Word 垂直标尺", "Word 网格线", "Word 状态栏"} {
		if !strings.Contains(restoredSemantics, visible) {
			t.Errorf("restored view is missing %q: %s", visible, restoredSemantics)
		}
	}
	if state.wordViewMode != wordViewContinuous || state.wordToolbarTab != wordTabView || state.wordLayoutZoom != 1.25 || state.wordListState.Position != previousPosition || !state.wordGridlinesVisible {
		t.Errorf("exiting Reading Mode did not restore the exact prior view state: position=%+v want=%+v", state.wordListState.Position, previousPosition)
	}
	if word.Dirty() {
		t.Fatal("exiting Reading Mode changed DOCX dirty state")
	}
}
