package desktopui

import (
	"image"

	"reflect"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordRightArrowMovesToNextStyledRun(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: "left", Editable: true}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-b", Text: "right", Style: document.WordTextStyle{Bold: true}, Editable: true}},
	}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	_, router, frame := newWordJoinFrame(t, state)
	first := focusWordRunForInteraction(t, state, router, frame, "run-a")
	first.SetCaret(first.Len(), first.Len())
	frame()

	router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press})
	frame()
	next := state.wordRunEditors["run-b"]
	if next == nil || !router.Source().Focused(next) {
		t.Fatal("Right at a run end did not focus the next styled Word run")
	}
	if start, end := next.Selection(); start != 0 || end != 0 {
		t.Fatalf("caret after moving right between runs = %d:%d; want 0:0", start, end)
	}
	if state.selectedWordRunID != "run-b" || state.selectedWordParagraphID != "p0" {
		t.Fatalf("selection after run handoff = %q/%q; want run-b/p0", state.selectedWordRunID, state.selectedWordParagraphID)
	}
}

func TestWordLeftArrowMovesToPreviousParagraphRun(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	first := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: "first", Editable: true}}}}
	second := document.WordParagraph{ID: "p1", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-b", Text: "second", Editable: true}}}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{
		{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &first},
		{ID: "p1", Kind: document.WordParagraphBlock, Paragraph: &second},
	}
	_, router, frame := newWordJoinFrame(t, state)
	secondEditor := focusWordRunForInteraction(t, state, router, frame, "run-b")
	secondEditor.SetCaret(0, 0)
	frame()

	router.Queue(key.Event{Name: key.NameLeftArrow, State: key.Press})
	frame()
	previous := state.wordRunEditors["run-a"]
	if previous == nil || !router.Source().Focused(previous) {
		t.Fatal("Left at a paragraph start did not focus the previous paragraph run")
	}
	if start, end := previous.Selection(); start != previous.Len() || end != previous.Len() {
		t.Fatalf("caret after moving left between paragraphs = %d:%d; want %d:%d", start, end, previous.Len(), previous.Len())
	}
}

func TestWordModifiedArrowKeepsNativeSelectionAndDocumentEdges(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: "selection", Editable: true}}}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	_, router, frame := newWordJoinFrame(t, state)
	editor := focusWordRunForInteraction(t, state, router, frame, "run-a")
	editor.SetCaret(0, 0)
	frame()

	router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press, Modifiers: key.ModShift})
	frame()
	if start, end := editor.Selection(); min(start, end) != 0 || max(start, end) != 1 {
		t.Fatalf("Shift+Right selection = %d:%d; want native selection across offsets 0 and 1", start, end)
	}
	editor.SetCaret(0, 0)
	frame()
	router.Queue(key.Event{Name: key.NameLeftArrow, State: key.Press})
	frame()
	if !router.Source().Focused(editor) {
		t.Fatal("Left at the document start moved focus outside the document")
	}
	if start, end := editor.Selection(); start != 0 || end != 0 {
		t.Fatalf("caret at document start after Left = %d:%d; want 0:0", start, end)
	}
}

func TestWordNavigationRunOrderIncludesEditableTableCellsOnly(t *testing.T) {
	paragraph := func(id, text string, editable bool) document.WordParagraph {
		return document.WordParagraph{ID: id, Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: id + ".run", Text: text, Editable: editable}}}}
	}
	first := paragraph("body", "body", true)
	readOnly := paragraph("locked", "locked", false)
	cellA := document.WordTableCell{ID: "a", Paragraphs: []document.WordParagraph{paragraph("a.p0", "A1", true), paragraph("a.p1", "A2", true)}}
	cellB := document.WordTableCell{ID: "b", Paragraphs: []document.WordParagraph{paragraph("b.p0", "B1", true)}}
	blocks := []document.WordBlock{
		{ID: "body", Kind: document.WordParagraphBlock, Paragraph: &first},
		{ID: "locked", Kind: document.WordParagraphBlock, Paragraph: &readOnly},
		{ID: "table", Kind: document.WordTableBlock, Table: &document.WordTable{Rows: []document.WordTableRow{{Cells: []document.WordTableCell{cellA, cellB}}}}},
	}
	var got []string
	for _, item := range wordNavigationRuns(blocks) {
		got = append(got, item.run.ID)
	}
	want := []string{"body.run", "a.p0.run", "a.p1.run", "b.p0.run"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Word navigation order = %#v; want row-major editable runs %#v", got, want)
	}
}

func TestWordUpDownKeepsPreferredVisualColumnAcrossVisualLines(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	text := strings.Repeat("abcdefghij", 5) + "\n" + strings.Repeat("abcdefghij", 3) + "\n" + strings.Repeat("abc", 20)
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: text, Editable: true}}}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(620, 700)}}
		app.layoutWord(gtx)
		router.Frame(&ops)
	}
	editor := focusWordRunForInteraction(t, state, router, frame, "run-a")
	editor.SetCaret(8, 8)
	frame()
	geometry := state.wordNavLayouts["run-a"]
	start := editor.CaretCoords()
	startY := geometry.y + int(start.Y)

	for move := 0; move < 2; move++ {
		router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
		frame()
		coords := editor.CaretCoords()
		if gotY := geometry.y + int(coords.Y); gotY <= startY {
			t.Fatalf("Down %d left caret on the same visual line: y=%d, start=%d, caret=%v geometry=%+v", move+1, gotY, startY, coords, geometry)
		}
		if !state.wordPreferredVerticalXSet {
			t.Fatal("Up/Down did not retain a preferred horizontal column")
		}
		visualX := geometry.x + int(coords.X)
		if distance := absWordInt(visualX - state.wordPreferredVerticalX); distance > 10 {
			t.Fatalf("Down %d moved to x=%d; preferred visual x=%d (distance %d)", move+1, visualX, state.wordPreferredVerticalX, distance)
		}
		startY = geometry.y + int(coords.Y)
		geometry = state.wordNavLayouts["run-a"]
	}
}

func TestWordUpDownCrossesParagraphsAtPreferredVisualColumn(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	text := strings.Repeat("0123456789", 10)
	first := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: text, Editable: true}}}}
	second := document.WordParagraph{ID: "p1", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-b", Text: text, Editable: true}}}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &first}, {ID: "p1", Kind: document.WordParagraphBlock, Paragraph: &second}}
	_, router, frame := newWordJoinFrame(t, state)
	firstEditor := focusWordRunForInteraction(t, state, router, frame, "run-a")
	firstEditor.SetCaret(100, 100)
	frame()
	preferred := state.wordNavLayouts["run-a"].x + int(firstEditor.CaretCoords().X)

	router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
	frame()
	frame()
	secondEditor := state.wordRunEditors["run-b"]
	if secondEditor == nil || !router.Source().Focused(secondEditor) {
		t.Fatalf("Down at the last visual line did not focus the next paragraph: selected=%q/%q pending=%q/%q preferred=%v/%d sourceFocusedA=%v bounds=%#v", state.selectedWordRunID, state.selectedWordParagraphID, state.wordPendingFocusRunID, state.wordPendingCaretRunID, state.wordPreferredVerticalXSet, state.wordPreferredVerticalX, router.Source().Focused(firstEditor), state.wordNavLayouts)
	}
	if !state.wordPreferredVerticalXSet {
		t.Fatal("vertical caret column was cleared while moving to the next paragraph")
	}
	secondX := state.wordNavLayouts["run-b"].x + int(secondEditor.CaretCoords().X)
	if distance := absWordInt(secondX - preferred); distance > 10 {
		t.Fatalf("next paragraph caret x=%d; preferred x=%d (distance %d)", secondX, preferred, distance)
	}

	router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press})
	frame()
	frame()
	if !router.Source().Focused(firstEditor) {
		t.Fatal("Up at the first visual line did not return to the previous paragraph")
	}
	firstX := state.wordNavLayouts["run-a"].x + int(firstEditor.CaretCoords().X)
	if distance := absWordInt(firstX - preferred); distance > 10 {
		t.Fatalf("previous paragraph caret x=%d; preferred x=%d (distance %d)", firstX, preferred, distance)
	}
}
