package desktopui

import (
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordFlowKeepsParagraphAndTableCellOrder(t *testing.T) {
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "p0.i0", Text: "Before"}},
		{Kind: document.WordImageInline, Image: &document.WordImage{ID: "img-1"}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "p0.i2", Text: "After"}},
	}}
	cellA := document.WordTableCell{ID: "cell-a", Paragraphs: []document.WordParagraph{{ID: "cell-a.p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "cell-a.p0.i0", Text: "Cell A"}}}}}}
	cellB := document.WordTableCell{ID: "cell-b", Paragraphs: []document.WordParagraph{{ID: "cell-b.p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "cell-b.p0.i0", Text: "Cell B"}}}}}}
	blocks := []document.WordBlock{
		{ID: "b0", Kind: document.WordParagraphBlock, Paragraph: &paragraph},
		{ID: "b1", Kind: document.WordTableBlock, Table: &document.WordTable{Rows: []document.WordTableRow{{Cells: []document.WordTableCell{cellA, cellB}}}}},
		{ID: "b2", Kind: document.WordUnsupportedBlock, Unsupported: &document.WordUnsupported{ID: "b2.unsupported", Description: "Retained content"}},
	}
	order := wordContentOrder(blocks)
	want := []string{"p0.i0", "img-1", "p0.i2", "cell-a.p0.i0", "cell-b.p0.i0", "b2.unsupported"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("Word content order = %#v; want %#v", order, want)
	}
}

func TestWordRunSelectionHandsTextToStableMultilineEditor(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)
	state := newDocumentSessionState("sample.docx")
	state.word = word
	blocks := word.WordBlocks()
	paragraph := blocks[0].Paragraph
	run := paragraph.Inlines[0].Text
	if !selectWordRun(state, paragraph.ID, run.ID) {
		t.Fatal("could not select an editable Word run")
	}
	if state.selectedWordRunID != run.ID || state.selectedWordParagraphID != paragraph.ID || state.wordEditor.Text() != run.Text {
		t.Fatalf("Word selection/editor handoff = run %q paragraph %q text %q", state.selectedWordRunID, state.selectedWordParagraphID, state.wordEditor.Text())
	}
	if state.wordEditor.SingleLine {
		t.Fatal("Word editor should support multiline text")
	}
	first := wordRunEditor(state, *run)
	if first == nil || first != wordRunEditor(state, *run) {
		t.Fatal("Word in-place editor must persist across layout frames")
	}
}

func TestWordFlowLayoutRendersWithStableVerticalListAndRibbon(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("sample.docx")
	state.word = word
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Constraints: layout.Constraints{Max: image.Pt(900, 700)}, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	dimensions := app.layoutWord(gtx)
	if dimensions.Size != image.Pt(900, 700) {
		t.Fatalf("Word flow dimensions = %v; want viewport size", dimensions.Size)
	}
	if state.wordListState.List.Axis != layout.Vertical || state.wordToolbarActionList.List.Axis != layout.Horizontal {
		t.Fatalf("Word widget axes = flow %v toolbar %v", state.wordListState.List.Axis, state.wordToolbarActionList.List.Axis)
	}
	if state.wordToolbarActionButtons["bold"] == nil {
		t.Fatal("Word Ribbon did not retain its action widgets after layout")
	}
}

func TestWordUnsupportedPlaceholderUsesMeasuredHeight(t *testing.T) {
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Constraints{Max: image.Pt(640, 720)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	dimensions := wordReadOnlyPlaceholder(gtx, material.NewTheme(), "Unsupported content remains visible and read-only")
	if dimensions.Size.Y <= 0 || dimensions.Size.Y >= gtx.Constraints.Max.Y/2 {
		t.Fatalf("unsupported-content placeholder height = %d; want its measured label height, independent of the %dpx viewport", dimensions.Size.Y, gtx.Constraints.Max.Y)
	}
}

func TestWordTableCellParagraphsExposeIndependentSelections(t *testing.T) {
	cell := document.WordTableCell{ID: "a", Paragraphs: []document.WordParagraph{
		{ID: "a.p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "a.p0.i0", Text: "First", Editable: true}}}},
		{ID: "a.p1", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "a.p1.i0", Text: "Second", Editable: true}}}},
	}}
	table := document.WordTable{Rows: []document.WordTableRow{{Cells: []document.WordTableCell{cell}}}}
	blocks := []document.WordBlock{{ID: "b0", Kind: document.WordTableBlock, Table: &table}}
	paragraphs := allWordParagraphs(blocks)
	if len(paragraphs) != 2 || paragraphs[0].ID == paragraphs[1].ID {
		t.Fatalf("table cell paragraph projections = %#v", paragraphs)
	}
	if _, ok := selectedWordParagraph(blocks, "a.p1"); !ok {
		t.Fatal("second table-cell paragraph is not independently selectable")
	}
}

func TestWordSelectionStyleCanBeReadFromSelectedRun(t *testing.T) {
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "p0.i0", Style: document.WordTextStyle{Bold: true, FontColor: "2196F3"}}}}}
	blocks := []document.WordBlock{{ID: "b0", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	if run, ok := selectedWordRun(blocks, "p0.i0"); !ok || !run.Style.Bold || run.Style.FontColor != "2196F3" {
		t.Fatalf("selected Word run style = %#v, %v", run, ok)
	}
}

func TestWordRunEditCommitsThroughAsyncCommandWithoutLosingSelection(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("sample.docx")
	state.word = word
	paragraph := word.WordBlocks()[0].Paragraph
	run := paragraph.Inlines[0].Text
	if !selectWordRun(state, paragraph.ID, run.ID) {
		t.Fatal("could not select the blank Word run")
	}
	value := "edited content"
	state.wordEditor.SetText(value)
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	app.runWordCommand("编辑文字", func(editor document.WordTextEditor) error {
		return editor.ReplaceTextRun(run.ID, value)
	})
	deadline := time.After(3 * time.Second)
	for {
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("async Word run edit failed: %v", result.err)
			}
			app.wordCommandResults <- result
			app.processAsyncResults()
			if state.formatRunning {
				t.Fatal("Word format operation remained marked as running")
			}
			updated, ok := selectedWordRun(word.WordBlocks(), run.ID)
			if !ok || updated.Text != "edited content" {
				t.Fatalf("edited run = %#v, exists=%v", updated, ok)
			}
			if state.selectedWordRunID != run.ID || state.wordEditor.Text() != "edited content" {
				t.Fatalf("selection draft after edit = run %q text %q", state.selectedWordRunID, state.wordEditor.Text())
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for asynchronous Word edit")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestWordHistoryRefreshesProjectionAndInvalidatesImageCache(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	imageID, err := word.InsertInlineImage(paragraph.ID, "", "image/png", tinyToolbarPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("sample.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordImageID = imageID
	state.wordImageCache.Put(imageID, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	app := &App{documentSessionState: state, documentGeneration: 0, wordCommandResults: make(chan wordCommandResult, 1)}

	applyAndWait := func(undo bool) {
		t.Helper()
		app.applyWordHistory(undo)
		deadline := time.After(3 * time.Second)
		for {
			select {
			case result := <-app.wordCommandResults:
				if result.err != nil {
					t.Fatalf("Word history operation failed: %v", result.err)
				}
				app.wordCommandResults <- result
				app.processAsyncResults()
				return
			case <-deadline:
				t.Fatal("timed out waiting for Word history operation")
			default:
				time.Sleep(time.Millisecond)
			}
		}
	}

	applyAndWait(true)
	if len(state.wordBlocks) == 0 || findWordImage(state.wordBlocks, imageID) != nil {
		t.Fatal("undo did not refresh the Word projection without the inserted image")
	}
	if state.wordImageCache.Bytes() != 0 || state.selectedWordImageID != "" {
		t.Fatalf("undo kept image cache/selection: bytes=%d selection=%q", state.wordImageCache.Bytes(), state.selectedWordImageID)
	}

	applyAndWait(false)
	if findWordImage(state.wordBlocks, imageID) == nil {
		t.Fatal("redo did not restore the image in the Word projection")
	}
	if state.wordImageCache.Bytes() != 0 {
		t.Fatalf("redo should invalidate decoded image previews; got %d bytes", state.wordImageCache.Bytes())
	}
}
