package desktopui

import (
	"image"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/format/xlsx"
)

func TestOfficeSearchShortcutsOpenFindAndReplaceForWordAndExcel(t *testing.T) {
	t.Run("Word", func(t *testing.T) {
		word, err := docx.NewBlank()
		if err != nil {
			t.Fatal(err)
		}
		defer word.Close()
		state := newDocumentSessionState("word-shortcuts.docx")
		state.word = word
		state.setWordBlocks(cloneWordBlockSnapshot(word.WordBlocks()))
		a := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 2)}
		h := newOfficeShortcutHarness(a)

		pressOfficeShortcut(t, h, "F", key.ModShortcut)
		assertOfficeSearchDialog(t, a, false)
		a.modal.Hide()
		pressOfficeShortcut(t, h, "H", replaceShortcutModifier())
		assertOfficeSearchDialog(t, a, true)
	})

	t.Run("Excel", func(t *testing.T) {
		book := xlsx.NewBlank()
		defer book.Close()
		state := newDocumentSessionState("excel-shortcuts.xlsx")
		state.book = book
		state.sheet = book.SheetNames()[0]
		a := &App{documentSessionState: state}
		h := newOfficeShortcutHarness(a)

		pressOfficeShortcut(t, h, "F", key.ModShortcut)
		assertOfficeSearchDialog(t, a, false)
		a.modal.Hide()
		pressOfficeShortcut(t, h, "H", replaceShortcutModifier())
		assertOfficeSearchDialog(t, a, true)
	})
}

func TestSpreadsheetHyperlinkShortcutUsesToolbarFlow(t *testing.T) {
	t.Run("selected cell opens link dialog", func(t *testing.T) {
		h := newFullGridTextHarness(t, false, "")
		app, state := h.app, h.app.documentSessionState
		state.cellEdit.cancel()
		state.selection = CellSelection{Anchor: CellPosition{Row: 1, Column: 1}, Active: CellPosition{Row: 1, Column: 1}}
		state.selectionSet = true
		state.selectedCell = "B2"
		app.selectedCell = "B2"
		h = newOfficeShortcutHarness(app)

		pressOfficeShortcut(t, h, "K", key.ModShortcut)
		if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "插入链接" {
			t.Fatalf("Ctrl/Cmd+K dialog visible=%v title=%q; want Insert Link", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
		}
	})

	t.Run("cell editor keeps shortcut", func(t *testing.T) {
		h := newFullGridTextHarness(t, false, "draft")
		app := h.app
		h = newOfficeShortcutHarness(app)
		pressOfficeShortcut(t, h, "K", key.ModShortcut)
		if app.modal.Visible {
			t.Fatal("Ctrl/Cmd+K opened a worksheet link dialog while the cell editor was active")
		}
	})
}

func TestWordFindShortcutCapturesFocusedEditorDraft(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("word-focused-find.docx")
	state.word = word
	state.setWordBlocks(cloneWordBlockSnapshot(word.WordBlocks()))
	run := word.WordBlocks()[0].Paragraph.Inlines[0].Text
	const draft = "focused editor draft"
	editor := wordRunEditor(state, *run)
	editor.SetText(draft)
	state.wordFocusedRunID = run.ID
	a := &App{documentSessionState: state}
	h := newOfficeShortcutHarness(a)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: editor})
		a.handleOfficeShortcuts(gtx)
	}
	h.frame()
	pressOfficeShortcut(t, h, "F", key.ModShortcut)
	if got := state.wordDraftSnapshot()[run.ID]; got != draft {
		t.Fatalf("focused Word draft captured by Ctrl/Cmd+F = %q; want %q", got, draft)
	}
	if !a.modal.Visible || !a.modal.SearchReplace {
		t.Fatal("Ctrl/Cmd+F did not open Find while a Word editor was focused")
	}
}

func TestSpreadsheetFindShortcutCommitsActiveCellDraft(t *testing.T) {
	book := xlsx.NewBlank()
	defer book.Close()
	sheet := book.SheetNames()[0]
	if err := book.SetCellText(sheet, "A1", "before"); err != nil {
		t.Fatal(err)
	}
	book.RebaseHistory()
	state := newDocumentSessionState("excel-focused-find.xlsx")
	state.book = book
	state.sheet = sheet
	state.selectedCell = "A1"
	state.cellEdit.selectCell("A1", "before")
	state.cellEdit.begin()
	state.cellEditor.SetText("cell edit draft")
	a := &App{documentSessionState: state}
	a.book = book
	h := newOfficeShortcutHarness(a)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
		a.handleOfficeShortcuts(gtx)
	}
	h.frame()
	pressOfficeShortcut(t, h, "F", key.ModShortcut)
	window, err := book.Window(sheet, 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := window.Cells[0][0].Value; got != "cell edit draft" {
		t.Fatalf("A1 after Ctrl/Cmd+F = %q; want the committed editor draft", got)
	}
	if state.cellEdit.Editing {
		t.Fatal("Ctrl/Cmd+F left the spreadsheet cell in edit mode")
	}
	if !a.modal.Visible || !a.modal.SearchReplace {
		t.Fatal("Ctrl/Cmd+F did not open Find after committing the cell editor")
	}
}

func TestSpreadsheetFormatShortcutsToggleSelectedRange(t *testing.T) {
	for _, test := range []struct {
		name string
		key  string
		read func(document.CellStyle) bool
	}{
		{name: "Ctrl+B", key: "B", read: func(style document.CellStyle) bool { return style.Bold }},
		{name: "Ctrl+I", key: "I", read: func(style document.CellStyle) bool { return style.Italic }},
		{name: "Ctrl+U", key: "U", read: func(style document.CellStyle) bool { return style.Underline }},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newFullGridTextHarness(t, false, "value")
			app, state := h.app, h.app.documentSessionState
			book := state.book.(*xlsx.Workbook)
			state.cellEdit.cancel()
			state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 1}}
			state.selectionSet = true
			state.selectedCell = "B1"
			app.selectedCell = "B1"
			state.virtualGrid.WindowRange = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2}
			state.virtualGrid.Window = document.SheetWindow{StartRow: 1, StartColumn: 1, Cells: [][]document.Cell{{{Value: "value"}, {}}}}
			app.formatResults = make(chan spreadsheetFormatResult, 1)
			app.shutdown = make(chan struct{})
			t.Cleanup(func() { close(app.shutdown) })
			h = newOfficeShortcutHarness(app)

			pressOfficeShortcut(t, h, test.key, key.ModShortcut)
			select {
			case result := <-app.formatResults:
				if result.err != nil || !result.applied {
					t.Fatalf("%s format result = %+v", test.name, result)
				}
			case <-time.After(3 * time.Second):
				t.Fatalf("%s did not format the selected range", test.name)
			}
			window, err := book.Window(state.sheet, 1, 1, 1, 2)
			if err != nil {
				t.Fatal(err)
			}
			for column, cell := range window.Cells[0] {
				if !test.read(cell.Style) {
					t.Errorf("cell %s after %s = %+v; expected format enabled", xlsx.CellName(column+1, 1), test.name, cell.Style)
				}
			}
		})
	}
}

func TestSpreadsheetNumberFormatShortcutsUseExcelControlShiftBindings(t *testing.T) {
	for _, test := range []struct {
		name, key, want string
	}{
		{name: "Number with thousands separator", key: "!", want: "#,##0.00"},
		{name: "Time", key: "@", want: "h:mm AM/PM"},
		{name: "Date", key: "#", want: "m/d/yy"},
		{name: "Currency", key: "$", want: spreadsheetCurrencyNumberFormat},
		{name: "Percentage", key: "%", want: "0%"},
		{name: "Scientific", key: "^", want: "0.00E+00"},
		{name: "General", key: "~", want: "General"},
	} {
		failedBefore := t.Failed()
		t.Run(test.name, func(t *testing.T) {
			h := newFullGridTextHarness(t, false, "1234.5")
			app, state := h.app, h.app.documentSessionState
			book := state.book.(*xlsx.Workbook)
			state.cellEdit.cancel()
			state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 0}}
			state.selectionSet = true
			state.selectedCell = "A1"
			app.selectedCell = "A1"
			if test.want == "General" {
				initial := "0.00"
				if err := book.SetCellFormats(state.sheet, document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, document.CellFormatPatch{NumberFormat: &initial}); err != nil {
					t.Fatal(err)
				}
				state.virtualGrid.Window.Cells[0][0].Style.NumberFormat = initial
			}
			app.formatResults = make(chan spreadsheetFormatResult, 1)
			app.shutdown = make(chan struct{})
			t.Cleanup(func() { close(app.shutdown) })
			h = newOfficeShortcutHarness(app)

			pressOfficeShortcut(t, h, test.key, key.ModCtrl|key.ModShift)
			select {
			case result := <-app.formatResults:
				if result.err != nil || !result.applied {
					t.Fatalf("Ctrl+Shift+%s format result = %+v", test.key, result)
				}
			case <-time.After(3 * time.Second):
				t.Fatalf("Excel Ctrl+Shift+%s did not apply its number format", test.key)
			}
			window, err := book.Window(state.sheet, 1, 1, 1, 1)
			if err != nil {
				t.Fatal(err)
			}
			if got := window.Cells[0][0].Style.NumberFormat; got != test.want {
				t.Fatalf("A1 number format after Ctrl+Shift+%s = %q; want %q", test.key, got, test.want)
			}
			previous := "General"
			if test.want == "General" {
				previous = "0.00"
			}
			if changed, err := book.Undo(); err != nil || !changed {
				t.Fatalf("undo Ctrl+Shift+%s format changed=%t, err=%v", test.key, changed, err)
			}
			window, err = book.Window(state.sheet, 1, 1, 1, 1)
			if err != nil {
				t.Fatal(err)
			}
			if got := window.Cells[0][0].Style.NumberFormat; got != previous {
				t.Fatalf("A1 number format after undo = %q; want %q", got, previous)
			}
			if changed, err := book.Redo(); err != nil || !changed {
				t.Fatalf("redo Ctrl+Shift+%s format changed=%t, err=%v", test.key, changed, err)
			}
		})
		if !failedBefore && t.Failed() {
			return // Keep the RED pass quick while still running all cases once implemented.
		}
	}
}

func TestSpreadsheetNumberFormatShortcutDefersToCellEditor(t *testing.T) {
	h := newFullGridTextHarness(t, false, "1234.5")
	app, state := h.app, h.app.documentSessionState
	book := state.book.(*xlsx.Workbook)
	app.formatResults = make(chan spreadsheetFormatResult, 1)
	h = newOfficeShortcutHarness(app)
	pressOfficeShortcut(t, h, "!", key.ModCtrl|key.ModShift)
	select {
	case result := <-app.formatResults:
		t.Fatalf("Ctrl+Shift+! formatted the worksheet while the cell editor was active: %+v", result)
	default:
	}
	window, err := book.Window(state.sheet, 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := window.Cells[0][0].Style.NumberFormat; got != "General" {
		t.Fatalf("A1 number format while editing = %q; want General", got)
	}
}

func TestWordFormatShortcutsToggleOnlySelectedTextAndKeepSelection(t *testing.T) {
	for _, test := range []struct {
		name string
		key  string
		read func(document.WordTextStyle) bool
	}{
		{name: "Ctrl+B", key: "B", read: func(style document.WordTextStyle) bool { return style.Bold }},
		{name: "Ctrl+I", key: "I", read: func(style document.WordTextStyle) bool { return style.Italic }},
		{name: "Ctrl+U", key: "U", read: func(style document.WordTextStyle) bool { return style.Underline }},
	} {
		t.Run(test.name, func(t *testing.T) {
			word, err := docx.NewBlank()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = word.Close() })
			run := word.WordBlocks()[0].Paragraph.Inlines[0].Text
			if err := word.ReplaceTextRun(run.ID, "甲ABC乙"); err != nil {
				t.Fatal(err)
			}
			run = word.WordBlocks()[0].Paragraph.Inlines[0].Text
			state := newDocumentSessionState("word-selection-format.docx")
			state.word = word
			state.setWordBlocks(cloneWordBlockSnapshot(word.WordBlocks()))
			state.selectedWordRunID = run.ID
			state.selectedWordParagraphID = word.WordBlocks()[0].Paragraph.ID
			state.wordFocusedRunID = run.ID
			state.wordPageTextSelection = &wordPageTextSelection{
				scope: "range", anchor: wordPageCaretPosition{runID: run.ID, runeOffset: 1},
				focus: wordPageCaretPosition{runID: run.ID, runeOffset: 4}, runOrder: map[string]int{run.ID: 0},
			}
			app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 2), shutdown: make(chan struct{})}
			t.Cleanup(func() { close(app.shutdown); app.stopAndWaitManagedBackground(); app.completeQueuedWordSplitsOnClose() })
			h := newOfficeShortcutHarness(app)
			pressOfficeShortcut(t, h, test.key, key.ModShortcut)
			waitForWordSelectionFormatting(t, app, state)
			inlines := state.word.WordBlocks()[0].Paragraph.Inlines
			if len(inlines) != 3 || inlines[0].Text.Text != "甲" || inlines[1].Text.Text != "ABC" || inlines[2].Text.Text != "乙" {
				t.Fatalf("%s changed selection text boundaries: %#v", test.name, inlines)
			}
			if test.read(inlines[0].Text.Style) || !test.read(inlines[1].Text.Style) || test.read(inlines[2].Text.Style) {
				t.Fatalf("%s styles outside/inside selection = %#v / %#v / %#v", test.name, inlines[0].Text.Style, inlines[1].Text.Style, inlines[2].Text.Style)
			}
			if !wordPageTextSelectionActive(state.wordPageTextSelection) {
				t.Fatal("formatting shortcut cleared the text selection")
			}
			if selected, ok := wordPageSelectedText(state); !ok || selected != "ABC" {
				t.Fatalf("selection after %s = %q, %v; want ABC", test.name, selected, ok)
			}
		})
	}
}

func TestWordParagraphStyleShortcutsApplyNormalAndHeadingStyles(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	if err := word.SetParagraphStyles([]string{paragraph.ID}, "Heading2"); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("word-style-shortcuts.docx")
	state.word = word
	state.setWordBlocks(cloneWordBlockSnapshot(word.WordBlocks()))
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = paragraph.Inlines[0].Text.ID
	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 1),
		shutdown:             make(chan struct{}),
	}
	t.Cleanup(func() {
		close(app.shutdown)
		app.stopAndWaitManagedBackground()
	})
	h := newOfficeShortcutHarness(app)
	for _, step := range []struct {
		name      string
		key       string
		modifiers key.Modifiers
		style     string
	}{
		{name: "Cmd/Ctrl+Shift+N applies Normal", key: "N", modifiers: key.ModShortcut | key.ModShift, style: "Normal"},
		{name: "Cmd/Ctrl+Alt+1 applies Heading 1", key: "1", modifiers: key.ModShortcut | key.ModAlt, style: "Heading1"},
		{name: "Cmd/Ctrl+Alt+2 applies Heading 2", key: "2", modifiers: key.ModShortcut | key.ModAlt, style: "Heading2"},
	} {
		t.Run(step.name, func(t *testing.T) {
			pressOfficeShortcut(t, h, step.key, step.modifiers)
			waitForWordSelectionFormatting(t, app, state)
			if got := state.word.WordBlocks()[0].Paragraph.Style; got != step.style {
				t.Fatalf("style after %s = %q; want %q", step.name, got, step.style)
			}
		})
	}
}

func TestWordFormatShortcutFlushesPendingDraftBeforeFormatting(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>甲ABC乙</w:t></w:r><w:r><w:t>tail</w:t></w:r></w:p>`)
	p := state.wordBlocks[0].Paragraph
	selected, draftRun := p.Inlines[0].Text, p.Inlines[1].Text
	state.wordPageTextSelection = &wordPageTextSelection{
		scope: "range", anchor: wordPageCaretPosition{runID: selected.ID, runeOffset: 1},
		focus: wordPageCaretPosition{runID: selected.ID, runeOffset: 4}, runOrder: map[string]int{selected.ID: 0, draftRun.ID: 1},
	}
	state.selectedWordRunID = selected.ID
	draftEditor := wordRunEditor(state, *draftRun)
	draftEditor.SetText("tail draft")
	h := newOfficeShortcutHarness(app)
	pressOfficeShortcut(t, h, "B", key.ModShortcut)
	waitForWordSelectionFormatting(t, app, state)
	p = state.word.WordBlocks()[0].Paragraph
	if p.Inlines[0].Text.Text != "甲" || p.Inlines[1].Text.Text != "ABC" || p.Inlines[2].Text.Text != "乙" || p.Inlines[3].Text.Text != "tail draft" {
		t.Fatalf("Ctrl+B did not flush the draft while preserving selection boundaries: %#v", p.Inlines)
	}
	if !p.Inlines[1].Text.Style.Bold {
		t.Fatal("Ctrl+B did not format selected text after flushing drafts")
	}
	if selected, ok := wordPageSelectedText(state); !ok || selected != "ABC" {
		t.Fatalf("selection after draft flush = %q, %v; want ABC", selected, ok)
	}
}

func TestWordParagraphAlignmentShortcutsApplyAndUndo(t *testing.T) {
	for _, tc := range []struct {
		key       string
		alignment document.WordAlignment
	}{
		{key: "L", alignment: document.WordAlignLeft},
		{key: "E", alignment: document.WordAlignCenter},
		{key: "R", alignment: document.WordAlignRight},
		{key: "J", alignment: document.WordAlignJustified},
	} {
		t.Run(tc.key, func(t *testing.T) {
			app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>align me</w:t></w:r></w:p>`)
			paragraph := state.wordBlocks[0].Paragraph
			state.selectedWordParagraphID = paragraph.ID
			state.selectedWordRunID = paragraph.Inlines[0].Text.ID
			h := newOfficeShortcutHarness(app)
			pressOfficeShortcut(t, h, tc.key, key.ModShortcut)
			waitForWordSelectionFormatting(t, app, state)
			if got := state.word.WordBlocks()[0].Paragraph.Alignment; got != tc.alignment {
				t.Fatalf("Cmd/Ctrl+%s alignment = %q; want %q", tc.key, got, tc.alignment)
			}
			history, ok := any(state.word).(document.WordHistory)
			if !ok {
				t.Fatal("Word document does not expose history")
			}
			if applied, err := history.Undo(); err != nil || !applied {
				t.Fatalf("undo alignment shortcut = %v, %v; want true, nil", applied, err)
			}
			if got := state.word.WordBlocks()[0].Paragraph.Alignment; got != document.WordAlignDefault {
				t.Fatalf("alignment after one undo = %q; want default", got)
			}
		})
	}
}

func TestWordParagraphAlignmentShortcutAppliesToEverySelectedParagraphAsOneUndo(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>first paragraph</w:t></w:r></w:p><w:p><w:r><w:t>second paragraph</w:t></w:r></w:p>`)
	paragraphs := allWordParagraphs(state.wordBlocks)
	firstRun := paragraphs[0].Inlines[0].Text
	secondRun := paragraphs[1].Inlines[0].Text
	state.selectedWordParagraphID = paragraphs[1].ID
	state.selectedWordRunID = secondRun.ID
	state.wordPageTextSelection = &wordPageTextSelection{
		scope:    "range",
		anchor:   wordPageCaretPosition{runID: firstRun.ID, runeOffset: 0},
		focus:    wordPageCaretPosition{runID: secondRun.ID, runeOffset: len([]rune(secondRun.Text))},
		runOrder: map[string]int{firstRun.ID: 0, secondRun.ID: 1},
	}
	h := newOfficeShortcutHarness(app)
	pressOfficeShortcut(t, h, "E", key.ModShortcut)
	waitForWordSelectionFormatting(t, app, state)
	for i, block := range state.word.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		if got := block.Paragraph.Alignment; got != document.WordAlignCenter {
			t.Fatalf("paragraph %d alignment = %q; want center", i, got)
		}
	}
	history, ok := any(state.word).(document.WordHistory)
	if !ok {
		t.Fatal("Word document does not expose history")
	}
	if applied, err := history.Undo(); err != nil || !applied {
		t.Fatalf("undo multi-paragraph alignment = %v, %v; want true, nil", applied, err)
	}
	for i, block := range state.word.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		if got := block.Paragraph.Alignment; got != document.WordAlignDefault {
			t.Fatalf("paragraph %d alignment after one undo = %q; want default", i, got)
		}
	}
}

func TestWordAlignmentShortcutFlushesPendingDraftBeforeCommand(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>original text</w:t></w:r></w:p>`)
	paragraph := state.wordBlocks[0].Paragraph
	run := paragraph.Inlines[0].Text
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = run.ID
	captureWordTextDraft(state, run.ID, "updated draft")
	h := newOfficeShortcutHarness(app)
	pressOfficeShortcut(t, h, "E", key.ModShortcut)
	waitForWordSelectionFormatting(t, app, state)
	updated := state.word.WordBlocks()[0].Paragraph
	if got := updated.Inlines[0].Text.Text; got != "updated draft" {
		t.Fatalf("paragraph alignment shortcut dropped pending draft: %q", got)
	}
	if got := updated.Alignment; got != document.WordAlignCenter {
		t.Fatalf("alignment after draft flush = %q; want center", got)
	}
}

func TestWordCutShortcutDeletesOnlySelectedRange(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>甲AB</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>CD乙</w:t></w:r></w:p>`)
	p := state.wordBlocks[0].Paragraph
	start, end := p.Inlines[0].Text, p.Inlines[1].Text
	state.wordPageTextSelection = &wordPageTextSelection{
		scope: "range", anchor: wordPageCaretPosition{runID: start.ID, runeOffset: 1},
		focus: wordPageCaretPosition{runID: end.ID, runeOffset: 2}, runOrder: map[string]int{start.ID: 0, end.ID: 1},
	}
	state.selectedWordRunID = end.ID
	h := newOfficeShortcutHarness(app)
	pressOfficeShortcut(t, h, "X", key.ModShortcut)
	waitForWordSelectionFormatting(t, app, state)
	p = state.word.WordBlocks()[0].Paragraph
	if len(p.Inlines) != 2 || p.Inlines[0].Text.Text != "甲" || p.Inlines[1].Text.Text != "乙" {
		t.Fatalf("Ctrl+X changed text outside the selection: %#v", p.Inlines)
	}
	if !p.Inlines[0].Text.Style.Bold || !p.Inlines[1].Text.Style.Italic {
		t.Fatalf("Ctrl+X lost surrounding run formatting: %#v", p.Inlines)
	}
	if wordPageTextSelectionActive(state.wordPageTextSelection) {
		t.Fatal("Ctrl+X left the deleted selection active")
	}
	if applied, err := any(state.word).(document.WordHistory).Undo(); err != nil || !applied {
		t.Fatalf("undo cut = %v, %v", applied, err)
	}
	p = state.word.WordBlocks()[0].Paragraph
	if p.Inlines[0].Text.Text != "甲AB" || p.Inlines[1].Text.Text != "CD乙" {
		t.Fatalf("undo cut restored %q/%q", p.Inlines[0].Text.Text, p.Inlines[1].Text.Text)
	}
}

func TestWordToolbarCutDeletesOnlySelectedRange(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>甲ABC乙</w:t></w:r></w:p>`)
	run := state.wordBlocks[0].Paragraph.Inlines[0].Text
	state.wordPageTextSelection = &wordPageTextSelection{
		scope: "range", anchor: wordPageCaretPosition{runID: run.ID, runeOffset: 1},
		focus: wordPageCaretPosition{runID: run.ID, runeOffset: 4}, runOrder: map[string]int{run.ID: 0},
	}
	state.selectedWordRunID = run.ID
	app.executeWordToolbarAction("cut")
	waitForWordSelectionFormatting(t, app, state)
	text := state.word.WordBlocks()[0].Paragraph.Inlines[0].Text.Text
	if text != "甲乙" {
		t.Fatalf("Ribbon Cut left %q; want it to remove only ABC", text)
	}
}

func TestWordRibbonPasteReplacesSelectedRangeAcrossRuns(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>甲A</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>B乙</w:t></w:r></w:p>`)
	p := state.wordBlocks[0].Paragraph
	start, end := p.Inlines[0].Text, p.Inlines[1].Text
	state.selectedWordRunID = end.ID
	state.wordPageTextSelection = &wordPageTextSelection{
		scope: "range", anchor: wordPageCaretPosition{runID: start.ID, runeOffset: 1},
		focus: wordPageCaretPosition{runID: end.ID, runeOffset: 1}, runOrder: map[string]int{start.ID: 0, end.ID: 1},
	}
	h := newOfficeShortcutHarness(app)
	requested := false
	h.draw = func(gtx layout.Context) {
		if !requested {
			app.executeOfficeRibbonMenuCommand(gtx, officeRibbonMenuWord, "paste-text")
			requested = true
		}
		app.handleWordClipboardEvents(gtx)
	}
	h.frame()
	h.router.Queue(transfer.DataEvent{
		Type: "application/text",
		Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("新🌱")) },
	})
	h.frame()
	waitForWordSelectionFormatting(t, app, state)
	p = state.word.WordBlocks()[0].Paragraph
	if len(p.Inlines) != 2 || p.Inlines[0].Text.Text != "甲新🌱" || p.Inlines[1].Text.Text != "乙" {
		t.Fatalf("Ribbon Paste replaced more than the selected AB: %q/%q status=%q pending=%+v intent=%+v", p.Inlines[0].Text.Text, p.Inlines[1].Text.Text, app.statusText(), state.wordPendingPasteSelection, state.wordSelectionEditIntent)
	}
	if !p.Inlines[0].Text.Style.Bold || !p.Inlines[1].Text.Style.Italic {
		t.Fatalf("Ribbon Paste lost surrounding run properties: %#v", p.Inlines)
	}
	if wordPageTextSelectionActive(state.wordPageTextSelection) {
		t.Fatal("Ribbon Paste left the replaced selection active")
	}
	if applied, err := any(state.word).(document.WordHistory).Undo(); err != nil || !applied {
		t.Fatalf("undo Ribbon Paste = %v, %v", applied, err)
	}
	p = state.word.WordBlocks()[0].Paragraph
	if p.Inlines[0].Text.Text != "甲A" || p.Inlines[1].Text.Text != "B乙" {
		t.Fatalf("undo Ribbon Paste restored %q/%q; want the original selected text", p.Inlines[0].Text.Text, p.Inlines[1].Text.Text)
	}
}

func TestWordPasteTextMenuReplacesLocalEditorSelectionWithoutReplacingRun(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>甲乙丙</w:t></w:r></w:p>`)
	run := state.wordBlocks[0].Paragraph.Inlines[0].Text
	state.selectedWordRunID = run.ID
	state.wordFocusedRunID = run.ID
	editor := wordRunEditor(state, *run)
	editor.SetText("甲乙丙")
	editor.SetCaret(1, 2)
	captureWordTextDraft(state, run.ID, editor.Text())
	h := newOfficeShortcutHarness(app)
	requested := false
	h.draw = func(gtx layout.Context) {
		if !requested {
			app.executeOfficeRibbonMenuCommand(gtx, officeRibbonMenuWord, "paste-text")
			requested = true
		}
		app.handleWordClipboardEvents(gtx)
	}
	h.frame()
	h.router.Queue(transfer.DataEvent{
		Type: "application/text",
		Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("新")) },
	})
	h.frame()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		app.processAsyncResults()
		if !state.formatRunning.Load() && !state.wordFlushRequested.Load() && len(app.wordCommandResults) == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if got := editor.Text(); got != "甲新丙" {
		t.Fatalf("Paste Text replaced the whole run or ignored the caret selection: editor=%q model=%q", got, state.word.WordBlocks()[0].Paragraph.Inlines[0].Text.Text)
	}
	if start, end := editor.Selection(); start != 2 || end != 2 {
		t.Fatalf("caret after Paste Text = %d:%d; want 2:2", start, end)
	}
	if draft := state.wordDraftSnapshot()[run.ID]; draft != "甲新丙" {
		t.Fatalf("Word run draft after Paste Text = %q; want %q", draft, "甲新丙")
	}
}

func TestWordPasteTextMenuInsertsAtLocalCaret(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>甲乙丙</w:t></w:r></w:p>`)
	run := state.wordBlocks[0].Paragraph.Inlines[0].Text
	state.selectedWordRunID = run.ID
	state.wordFocusedRunID = run.ID
	editor := wordRunEditor(state, *run)
	editor.SetText("甲乙丙")
	editor.SetCaret(2, 2)
	captureWordTextDraft(state, run.ID, editor.Text())
	h := newOfficeShortcutHarness(app)
	requested := false
	h.draw = func(gtx layout.Context) {
		if !requested {
			app.executeOfficeRibbonMenuCommand(gtx, officeRibbonMenuWord, "paste-text")
			requested = true
		}
		app.handleWordClipboardEvents(gtx)
	}
	h.frame()
	h.router.Queue(transfer.DataEvent{
		Type: "application/text",
		Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("新")) },
	})
	h.frame()
	if got := editor.Text(); got != "甲乙新丙" {
		t.Fatalf("Paste Text did not insert at the caret: editor=%q", got)
	}
	if start, end := editor.Selection(); start != 3 || end != 3 {
		t.Fatalf("caret after Paste Text = %d:%d; want 3:3", start, end)
	}
}

func TestWordPasteTextMenuLeavesNewerEditorDraftUntouched(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>甲乙丙</w:t></w:r></w:p>`)
	run := state.wordBlocks[0].Paragraph.Inlines[0].Text
	state.selectedWordRunID = run.ID
	state.wordFocusedRunID = run.ID
	editor := wordRunEditor(state, *run)
	editor.SetText("甲乙丙")
	editor.SetCaret(1, 1)
	captureWordTextDraft(state, run.ID, editor.Text())
	h := newOfficeShortcutHarness(app)
	requested := false
	h.draw = func(gtx layout.Context) {
		if !requested {
			app.executeOfficeRibbonMenuCommand(gtx, officeRibbonMenuWord, "paste-text")
			requested = true
		}
		app.handleWordClipboardEvents(gtx)
	}
	h.frame()
	editor.SetText("甲X乙丙")
	captureWordTextDraft(state, run.ID, editor.Text())
	h.router.Queue(transfer.DataEvent{
		Type: "application/text",
		Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("新")) },
	})
	h.frame()
	if got := editor.Text(); got != "甲X乙丙" {
		t.Fatalf("Paste Text overwrote newer editor text: %q", got)
	}
	if draft := state.wordDraftSnapshot()[run.ID]; draft != "甲X乙丙" {
		t.Fatalf("newer Word draft after rejected stale paste = %q; want %q", draft, "甲X乙丙")
	}
}

func TestWordEditorCtrlVPastesOverCrossRunDocumentSelection(t *testing.T) {
	app, state, router, frame := newWordSelectionPageTest(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>甲A</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>B乙</w:t></w:r></w:p>`)
	p := state.wordBlocks[0].Paragraph
	first, last := p.Inlines[0].Text, p.Inlines[1].Text
	state.selectedWordRunID = last.ID
	state.wordPageTextSelection = &wordPageTextSelection{
		scope: "range", anchor: wordPageCaretPosition{runID: first.ID, runeOffset: 1},
		focus: wordPageCaretPosition{runID: last.ID, runeOffset: 1}, runOrder: map[string]int{first.ID: 0, last.ID: 1},
	}
	editor := wordRunEditor(state, *last)
	router.Source().Execute(key.FocusCmd{Tag: editor})
	frame()
	if !router.Source().Focused(editor) {
		t.Fatal("Word run editor did not receive focus for Ctrl/Cmd+V")
	}
	router.Queue(key.Event{Name: key.Name("V"), State: key.Press, Modifiers: key.ModShortcut})
	frame()
	if !router.ClipboardRequested() {
		t.Fatal("Ctrl/Cmd+V did not request clipboard text from the focused Word editor")
	}
	router.Queue(transfer.DataEvent{
		Type: "application/text",
		Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("新")) },
	})
	frame()
	waitWordReplaceCompletion(t, app, state)
	p = state.word.WordBlocks()[0].Paragraph
	if p.Inlines[0].Text.Text != "甲新" || p.Inlines[1].Text.Text != "乙" {
		t.Fatalf("Ctrl/Cmd+V replaced %q/%q; want it to replace only the cross-run selection", p.Inlines[0].Text.Text, p.Inlines[1].Text.Text)
	}
}

func TestWordRibbonPasteUsesSelectionCapturedAtRequest(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>甲A</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>B乙</w:t></w:r></w:p>`)
	p := state.wordBlocks[0].Paragraph
	first, last := p.Inlines[0].Text, p.Inlines[1].Text
	state.wordPageTextSelection = &wordPageTextSelection{
		scope: "range", anchor: wordPageCaretPosition{runID: first.ID, runeOffset: 1},
		focus: wordPageCaretPosition{runID: last.ID, runeOffset: 1}, runOrder: map[string]int{first.ID: 0, last.ID: 1},
	}
	state.selectedWordRunID = last.ID
	h := newOfficeShortcutHarness(app)
	requested := false
	h.draw = func(gtx layout.Context) {
		if !requested {
			app.requestWordClipboardRead(gtx, state)
			requested = true
		}
		app.handleWordClipboardEvents(gtx)
	}
	h.frame()
	if !h.router.ClipboardRequested() {
		t.Fatal("Ribbon Paste did not request clipboard data")
	}
	state.wordPageTextSelection = &wordPageTextSelection{
		scope: "range", anchor: wordPageCaretPosition{runID: first.ID, runeOffset: 0},
		focus: wordPageCaretPosition{runID: first.ID, runeOffset: 1}, runOrder: map[string]int{first.ID: 0, last.ID: 1},
	}
	h.router.Queue(transfer.DataEvent{
		Type: "application/text",
		Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("新")) },
	})
	h.frame()
	waitForWordSelectionFormatting(t, app, state)
	p = state.word.WordBlocks()[0].Paragraph
	if p.Inlines[0].Text.Text != "甲新" || p.Inlines[1].Text.Text != "乙" {
		t.Fatalf("Ribbon Paste followed a later selection and produced %q/%q; want it to use the requested AB range", p.Inlines[0].Text.Text, p.Inlines[1].Text.Text)
	}
}

func TestWordRibbonPasteRebasesRequestedSelectionAfterPendingReplacementInput(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>甲A</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>B乙</w:t></w:r></w:p>`)
	p := state.wordBlocks[0].Paragraph
	first, last := p.Inlines[0].Text, p.Inlines[1].Text
	state.wordPageTextSelection = &wordPageTextSelection{
		scope: "range", anchor: wordPageCaretPosition{runID: first.ID, runeOffset: 1},
		focus: wordPageCaretPosition{runID: last.ID, runeOffset: 1}, runOrder: map[string]int{first.ID: 0, last.ID: 1},
	}
	state.selectedWordRunID = last.ID
	editor := wordRunEditor(state, *last)
	editor.SetText(last.Text)
	if beginWordSelectionInputDraft(state, last.ID, editor, true) || state.wordSelectionInputDraft == nil {
		t.Fatal("selected range did not begin tracking input in its focused run")
	}
	editor.SetText("新")
	h := newOfficeShortcutHarness(app)
	requested := false
	h.draw = func(gtx layout.Context) {
		if !requested {
			app.requestWordClipboardRead(gtx, state)
			requested = true
		}
		app.handleWordClipboardEvents(gtx)
	}
	h.frame()
	if !app.finishWordSelectionInput(state, last.ID, editor) {
		t.Fatal("pending selected-text input was not queued before clipboard data arrived")
	}
	waitForWordSelectionFormatting(t, app, state)
	h.router.Queue(transfer.DataEvent{
		Type: "application/text",
		Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("文")) },
	})
	h.frame()
	waitForWordSelectionFormatting(t, app, state)
	p = state.word.WordBlocks()[0].Paragraph
	if p.Inlines[0].Text.Text != "甲新文" || p.Inlines[1].Text.Text != "乙" {
		t.Fatalf("Ribbon Paste did not follow the caret after pending selected-text input: %q/%q status=%q", p.Inlines[0].Text.Text, p.Inlines[1].Text.Text, app.statusText())
	}
}

func TestWordRibbonPasteFlushesOtherRunDraftsBeforeReplacingSelection(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>甲A</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>B乙</w:t></w:r><w:r><w:t>外</w:t></w:r></w:p>`)
	p := state.wordBlocks[0].Paragraph
	first, last, unrelated := p.Inlines[0].Text, p.Inlines[1].Text, p.Inlines[2].Text
	state.wordPageTextSelection = &wordPageTextSelection{
		scope: "range", anchor: wordPageCaretPosition{runID: first.ID, runeOffset: 1},
		focus: wordPageCaretPosition{runID: last.ID, runeOffset: 1}, runOrder: map[string]int{first.ID: 0, last.ID: 1},
	}
	state.selectedWordRunID = last.ID
	draftEditor := wordRunEditor(state, *unrelated)
	draftEditor.SetText("外改")
	captureWordTextDraft(state, unrelated.ID, "外改")
	h := newOfficeShortcutHarness(app)
	requested := false
	h.draw = func(gtx layout.Context) {
		if !requested {
			app.requestWordClipboardRead(gtx, state)
			requested = true
		}
		app.handleWordClipboardEvents(gtx)
	}
	h.frame()
	h.router.Queue(transfer.DataEvent{
		Type: "application/text",
		Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("新")) },
	})
	h.frame()
	waitForWordSelectionFormatting(t, app, state)
	p = state.word.WordBlocks()[0].Paragraph
	if p.Inlines[0].Text.Text != "甲新" || p.Inlines[1].Text.Text != "乙" || p.Inlines[2].Text.Text != "外改" {
		t.Fatalf("Ribbon Paste did not flush the unrelated run draft before replacing the selected range: %q/%q/%q", p.Inlines[0].Text.Text, p.Inlines[1].Text.Text, p.Inlines[2].Text.Text)
	}
}

func TestWordFormatShortcutKeepsSelectionAcrossRuns(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>甲A</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>B🌱乙</w:t></w:r></w:p>`)
	p := state.wordBlocks[0].Paragraph
	start, end := p.Inlines[0].Text, p.Inlines[1].Text
	state.wordPageTextSelection = &wordPageTextSelection{
		scope: "range", anchor: wordPageCaretPosition{runID: start.ID, runeOffset: 1},
		focus: wordPageCaretPosition{runID: end.ID, runeOffset: 2}, runOrder: map[string]int{start.ID: 0, end.ID: 1},
	}
	state.selectedWordRunID = start.ID
	h := newOfficeShortcutHarness(app)
	pressOfficeShortcut(t, h, "B", key.ModShortcut)
	waitForWordSelectionFormatting(t, app, state)
	p = state.word.WordBlocks()[0].Paragraph
	if len(p.Inlines) != 4 || p.Inlines[0].Text.Text != "甲" || p.Inlines[1].Text.Text != "A" || p.Inlines[2].Text.Text != "B🌱" || p.Inlines[3].Text.Text != "乙" {
		t.Fatalf("Ctrl+B changed cross-run text boundaries: %#v", p.Inlines)
	}
	if p.Inlines[0].Text.Style.Bold || !p.Inlines[1].Text.Style.Bold || !p.Inlines[2].Text.Style.Bold || p.Inlines[3].Text.Style.Bold || !p.Inlines[2].Text.Style.Italic {
		t.Fatalf("Ctrl+B cross-run styles = %#v", p.Inlines)
	}
	if selected, ok := wordPageSelectedText(state); !ok || selected != "AB🌱" {
		t.Fatalf("selection after Ctrl+B = %q, %v; want AB🌱", selected, ok)
	}
}

func waitForWordSelectionFormatting(t *testing.T, app *App, state *documentSessionState) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		app.processAsyncResults()
		if state.wordSelectionEditIntent == nil && state.wordPendingPasteSelection == nil && !state.formatRunning.Load() && len(app.wordCommandResults) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("Word selection formatting did not complete: intent=%+v running=%v status=%q", state.wordSelectionEditIntent, state.formatRunning.Load(), app.statusText())
}

func TestSpreadsheetFormatShortcutDoesNotFormatWhileEditingCellText(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	app, state := h.app, h.app.documentSessionState
	book := state.book.(*xlsx.Workbook)
	state.cellEdit.begin()
	state.cellEditor.SetText("editing Ctrl+B text")
	app.formatResults = make(chan spreadsheetFormatResult, 1)
	app.shutdown = make(chan struct{})
	t.Cleanup(func() { close(app.shutdown) })
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.cellEditor})
		app.handleOfficeShortcuts(gtx)
	}
	h.frame()
	pressOfficeShortcut(t, h, "B", key.ModShortcut)
	select {
	case result := <-app.formatResults:
		t.Fatalf("Ctrl/Cmd+B formatted the cell while its editor was active: %+v", result)
	default:
	}
	window, err := book.Window(state.sheet, 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if window.Cells[0][0].Style.Bold {
		t.Fatal("cell formatting changed while Ctrl/Cmd+B belonged to the text editor")
	}
}

func TestSpreadsheetFormatShortcutDoesNotInterceptNameBox(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	app, state := h.app, h.app.documentSessionState
	book := state.book.(*xlsx.Workbook)
	state.cellEdit.cancel()
	app.nameBox.SetText("A1")
	app.formatResults = make(chan spreadsheetFormatResult, 1)
	app.shutdown = make(chan struct{})
	t.Cleanup(func() { close(app.shutdown) })
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &app.nameBox})
		app.handleOfficeShortcuts(gtx)
	}
	h.frame()
	pressOfficeShortcut(t, h, "B", key.ModShortcut)
	select {
	case result := <-app.formatResults:
		t.Fatalf("Ctrl/Cmd+B formatted the grid while the Name Box was focused: %+v", result)
	default:
	}
	window, err := book.Window(state.sheet, 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if window.Cells[0][0].Style.Bold {
		t.Fatal("cell formatting changed while the Name Box was focused")
	}
}

func TestOfficePrintShortcutOpensExistingPrintFlows(t *testing.T) {
	t.Run("Word unavailable printer", func(t *testing.T) {
		word, err := docx.NewBlank()
		if err != nil {
			t.Fatal(err)
		}
		defer word.Close()
		state := newDocumentSessionState("word-print-shortcut.docx")
		state.word = word
		a := &App{documentSessionState: state, nativePrinter: &testWordPrinter{available: false}}
		h := newOfficeShortcutHarness(a)
		pressOfficeShortcut(t, h, "P", key.ModShortcut)
		if a.statusText() != "当前平台不支持原生打印" {
			t.Fatalf("Word Ctrl/Cmd+P status = %q; want the existing native-print availability response", a.statusText())
		}
	})

	t.Run("Excel confirmation", func(t *testing.T) {
		book := xlsx.NewBlank()
		defer book.Close()
		sheet := book.SheetNames()[0]
		if err := book.SetCellText(sheet, "A1", "打印内容"); err != nil {
			t.Fatal(err)
		}
		state := newDocumentSessionState("excel-print-shortcut.xlsx")
		state.book = book
		state.sheet = sheet
		state.selectedCell = "A1"
		state.cellEdit.selectCell("A1", "打印内容")
		state.cellEdit.begin()
		state.cellEditor.SetText("待打印的新内容")
		a := &App{documentSessionState: state, nativePrinter: &darwinTestPrinter{}}
		a.book = book
		h := newOfficeShortcutHarness(a)
		pressOfficeShortcut(t, h, "P", key.ModShortcut)
		if !a.modal.Visible || a.messageText(a.modal.TitleRef, a.modal.Title) != "打印工作表" {
			t.Fatalf("Excel Ctrl/Cmd+P dialog = visible %v, title %q; want print confirmation", a.modal.Visible, a.messageText(a.modal.TitleRef, a.modal.Title))
		}
		window, err := book.Window(sheet, 1, 1, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got := window.Cells[0][0].Value; got != "待打印的新内容" {
			t.Fatalf("A1 when print confirmation opened = %q; want committed cell draft", got)
		}
		if state.cellEdit.Editing {
			t.Fatal("Excel Ctrl/Cmd+P left the cell editor active")
		}
	})
}

func TestOfficeReplaceShortcutPreservesMacOSHideBinding(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Cmd+H is the macOS Hide Application shortcut")
	}
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("word-shortcuts.docx")
	state.word = word
	state.setWordBlocks(cloneWordBlockSnapshot(word.WordBlocks()))
	a := &App{documentSessionState: state}
	h := newOfficeShortcutHarness(a)

	pressOfficeShortcut(t, h, "H", key.ModShortcut)
	if a.modal.Visible {
		t.Fatal("Cmd+H opened Replace and conflicts with macOS Hide Application")
	}
	pressOfficeShortcut(t, h, "H", key.ModCtrl)
	assertOfficeSearchDialog(t, a, true)
}

func TestOfficeSearchCloseButtonClosesDialogsOpenedByShortcuts(t *testing.T) {
	tests := []struct {
		name  string
		setup func() (*App, func())
	}{
		{
			name: "Word Find",
			setup: func() (*App, func()) {
				word, err := docx.NewBlank()
				if err != nil {
					t.Fatal(err)
				}
				state := newDocumentSessionState("word-close.docx")
				state.word = word
				state.setWordBlocks(cloneWordBlockSnapshot(word.WordBlocks()))
				a := &App{documentSessionState: state, theme: material.NewTheme()}
				a.showWordFindDialog()
				return a, func() { _ = word.Close() }
			},
		},
		{
			name: "Excel Find",
			setup: func() (*App, func()) {
				book := xlsx.NewBlank()
				state := newDocumentSessionState("excel-close.xlsx")
				state.book = book
				state.sheet = book.SheetNames()[0]
				a := &App{documentSessionState: state, theme: material.NewTheme()}
				a.showSpreadsheetFindDialog()
				return a, func() { _ = book.Close() }
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, cleanup := test.setup()
			defer cleanup()
			if !a.modal.Visible || !a.modal.SearchReplace {
				t.Fatal("Office Find did not open its search dialog")
			}
			var router input.Router
			var ops op.Ops
			frame := func() {
				ops.Reset()
				gtx := layout.Context{
					Ops:         &ops,
					Source:      router.Source(),
					Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
					Constraints: layout.Exact(image.Pt(820, 500)),
				}
				a.layoutModalDialog(gtx)
				router.Frame(&ops)
			}
			frame()
			cardHeight := float32(360)
			if a.modal.SearchReplaceScope != "" {
				cardHeight = 384
			}
			y := (500-cardHeight)/2 + 18 + 16
			pos := f32.Pt(650, y)
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: pos})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: pos})
			frame()
			if a.modal.Visible {
				t.Fatal("the close control on the shortcut-opened Office dialog did not close it")
			}
		})
	}
}

func TestSearchReplaceCloseButtonDismissesEvenIfCallbackIgnoresClose(t *testing.T) {
	a := &App{theme: material.NewTheme()}
	closeCalled := false
	a.modal.ShowInput("查找和替换", "", "词语", "查找内容", func(action string) {
		closeCalled = action == "close"
	})
	a.modal.SearchReplace = true
	a.modal.SearchReplaceHasReplace = true
	clickOfficeSearchReplaceClose(t, a, 360)
	if !closeCalled {
		t.Fatal("close button did not notify the modal callback")
	}
	if a.modal.Visible {
		t.Fatal("close button left the modal open because its callback did not hide it")
	}
}

func TestOfficeSaveShortcutPersistsWordAndExcelEdits(t *testing.T) {
	t.Run("Word", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "shortcut-save.docx")
		word, err := docx.NewBlank()
		if err != nil {
			t.Fatal(err)
		}
		defer word.Close()
		if err := word.SaveAsDocument(path); err != nil {
			t.Fatal(err)
		}
		run := word.WordBlocks()[0].Paragraph.Inlines[0].Text
		const want = "saved with the Word shortcut"
		state := newDocumentSessionState(path)
		state.word = word
		state.setWordBlocks(cloneWordBlockSnapshot(word.WordBlocks()))
		editor := wordRunEditor(state, *run)
		editor.SetText(want)
		state.wordFocusedRunID = run.ID
		a := &App{documentSessionState: state, saveResults: make(chan saveResult, 1), wordCommandResults: make(chan wordCommandResult, 1), shutdown: make(chan struct{})}
		a.word = word
		h := newOfficeShortcutHarness(a)
		h.draw = func(gtx layout.Context) {
			gtx.Execute(key.FocusCmd{Tag: editor})
			a.handleOfficeShortcuts(gtx)
		}
		h.frame()
		pressOfficeShortcut(t, h, "S", key.ModShortcut)
		waitForOfficeShortcutSave(t, a)
		reopened, err := docx.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		if got := reopened.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != want {
			t.Fatalf("Word text after Ctrl/Cmd+S = %q; want %q", got, want)
		}
	})

	t.Run("Excel", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "shortcut-save.xlsx")
		book := xlsx.NewBlank()
		defer book.Close()
		if err := book.SaveAsDocument(path); err != nil {
			t.Fatal(err)
		}
		sheet := book.SheetNames()[0]
		const want = "saved with the Excel shortcut"
		if err := book.SetCellText(sheet, "A1", want); err != nil {
			t.Fatal(err)
		}
		state := newDocumentSessionState(path)
		state.book = book
		state.sheet = sheet
		a := &App{documentSessionState: state, saveResults: make(chan saveResult, 1), shutdown: make(chan struct{})}
		a.book = book
		h := newOfficeShortcutHarness(a)
		pressOfficeShortcut(t, h, "S", key.ModShortcut)
		waitForOfficeShortcutSave(t, a)
		reopened, err := xlsx.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		window, err := reopened.Window(sheet, 1, 1, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got := window.Cells[0][0].Value; got != want {
			t.Fatalf("Excel A1 after Ctrl/Cmd+S = %q; want %q", got, want)
		}
	})
}

func TestSpreadsheetAutoSumShortcutUsesPlatformBindingAndRibbonFlow(t *testing.T) {
	for _, tc := range []struct {
		goos      string
		name      key.Name
		modifiers key.Modifiers
	}{
		{goos: "windows", name: key.Name("="), modifiers: key.ModAlt},
		{goos: "linux", name: key.Name("="), modifiers: key.ModAlt},
		{goos: "darwin", name: key.Name("T"), modifiers: key.ModShortcut | key.ModShift},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			name, modifiers := spreadsheetAutoSumShortcutBinding(tc.goos)
			if name != tc.name || modifiers != tc.modifiers {
				t.Fatalf("AutoSum shortcut = %q/%v; want %q/%v", name, modifiers, tc.name, tc.modifiers)
			}
		})
	}

	name, modifiers := spreadsheetAutoSumShortcutBinding(runtime.GOOS)
	book := &mockSearcherBook{toolbarTestBook: toolbarTestBook{editable: true}}
	state := newDocumentSessionState("autosum-shortcut.xlsx")
	state.book = book
	state.sheet = "Sheet1"
	state.selectionSet = true
	state.selection = CellSelection{Anchor: CellPosition{Row: 2, Column: 0}, Active: CellPosition{Row: 2, Column: 0}}
	app := &App{documentSessionState: state, formatResults: make(chan spreadsheetFormatResult, 2)}
	h := newOfficeShortcutHarness(app)
	pressOfficeShortcut(t, h, string(name), modifiers)
	select {
	case result := <-app.formatResults:
		if result.err != nil {
			t.Fatalf("AutoSum shortcut result: %v", result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("AutoSum shortcut did not use the Ribbon formula insertion flow")
	}
	if len(book.lastFormulas) != 1 || book.lastFormulas[0] != (document.CellFormulaEdit{Row: 3, Column: 1, Formula: "=SUM(A1:A2)"}) {
		t.Fatalf("AutoSum shortcut formulas = %+v; want A3 =SUM(A1:A2)", book.lastFormulas)
	}
}

func TestSpreadsheetAutoSumShortcutDoesNotInterceptCellEditor(t *testing.T) {
	name, modifiers := spreadsheetAutoSumShortcutBinding(runtime.GOOS)
	book := &mockSearcherBook{toolbarTestBook: toolbarTestBook{editable: true}}
	state := newDocumentSessionState("autosum-editor-shortcut.xlsx")
	state.book = book
	state.sheet = "Sheet1"
	state.selectionSet = true
	state.selection = CellSelection{Anchor: CellPosition{Row: 2, Column: 0}, Active: CellPosition{Row: 2, Column: 0}}
	app := &App{documentSessionState: state}
	h := newOfficeShortcutHarness(app)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &app.cellEditor})
		app.handleOfficeShortcuts(gtx)
	}
	h.frame()
	pressOfficeShortcut(t, h, string(name), modifiers)
	if len(book.lastFormulas) != 0 || state.formatRunning.Load() {
		t.Fatal("AutoSum shortcut intercepted input while the cell editor was focused")
	}
}

func newOfficeShortcutHarness(a *App) *gridTextHarness {
	h := &gridTextHarness{app: a}
	h.draw = func(gtx layout.Context) { a.handleOfficeShortcuts(gtx) }
	h.frame()
	return h
}

func pressOfficeShortcut(t *testing.T, h *gridTextHarness, name string, modifiers key.Modifiers) {
	t.Helper()
	h.router.Queue(key.Event{Name: key.Name(name), State: key.Press, Modifiers: modifiers})
	h.now += 30 * time.Millisecond
	h.frame()
}

func replaceShortcutModifier() key.Modifiers {
	if runtime.GOOS == "darwin" {
		return key.ModCtrl
	}
	return key.ModShortcut
}

func assertOfficeSearchDialog(t *testing.T, a *App, replaceTab bool) {
	t.Helper()
	if !a.modal.Visible || !a.modal.SearchReplace {
		t.Fatal("Office search shortcut did not open the search dialog")
	}
	if !a.modal.SearchReplaceHasReplace || !a.modal.HasSecondInput {
		t.Fatal("Office search dialog did not expose its supported Replace input")
	}
	if a.modal.SearchReplaceReplaceTab != replaceTab {
		t.Fatalf("Replace tab active = %v; want %v", a.modal.SearchReplaceReplaceTab, replaceTab)
	}
}

func clickOfficeSearchReplaceClose(t *testing.T, a *App, cardHeight float32) {
	t.Helper()
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(820, 500)),
		}
		a.layoutModalDialog(gtx)
		router.Frame(&ops)
	}
	frame()
	pos := f32.Pt(650, (500-cardHeight)/2+18+16)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: pos})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: pos})
	frame()
}

func waitForOfficeShortcutSave(t *testing.T, a *App) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case commandResult := <-a.wordCommandResults:
			a.wordCommandResults <- commandResult
			a.processAsyncResults()
		case result := <-a.saveResults:
			if result.err != nil {
				t.Fatalf("Office save shortcut failed: %v", result.err)
			}
			if a.documentSessionState.operationGeneration == 0 {
				t.Fatal("Office save shortcut did not begin a document save operation")
			}
			a.stopAndWaitManagedBackground()
			return
		case <-deadline.C:
			t.Fatal("Office save shortcut did not finish")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
