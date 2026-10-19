package desktopui

import (
	"strings"
	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetReplaceAllReplacesAllTextMatchesAsOneUndoableEdit(t *testing.T) {
	book := xlsx.NewBlank()
	defer book.Close()
	sheet := book.SheetNames()[0]
	if err := book.EditStructure(document.StructureEdit{Kind: document.StructureCreateSheet, NewName: "Other"}); err != nil {
		t.Fatal(err)
	}
	for cell, value := range map[string]string{
		"A1": "23 apples and 23 pears",
		"A2": "pre23post",
	} {
		if err := book.SetCellText(sheet, cell, value); err != nil {
			t.Fatal(err)
		}
	}
	valueEditor, ok := any(book).(document.SpreadsheetCellValueEditor)
	if !ok {
		t.Fatal("XLSX adapter does not expose typed cell writes")
	}
	if _, err := valueEditor.SetCellValueChanged(sheet, "B1", document.CellValue{Kind: document.CellValueNumber, Number: 23}); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellFormula(sheet, "B2", "=23"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellText("Other", "A1", "23"); err != nil {
		t.Fatal(err)
	}
	book.RebaseHistory()
	state := newDocumentSessionState("replace-all.xlsx")
	state.book = book
	state.sheet = sheet
	a := &App{documentSessionState: state, formatResults: make(chan spreadsheetFormatResult, 4)}
	a.book = book

	a.showSpreadsheetReplaceDialog()
	if !a.modal.Visible || !a.modal.SearchReplaceReplaceTab {
		t.Fatal("Ctrl+H entry did not open the Replace tab")
	}
	hasReplaceAll := false
	for _, action := range a.modal.Actions {
		if action.ID == "replace-all" {
			hasReplaceAll = true
		}
	}
	if !hasReplaceAll {
		t.Fatal("Excel Replace tab does not provide Replace All")
	}
	a.modal.Input.SetText("23")
	a.modal.SecondInput.SetText("X")
	a.modal.OnAction("replace-all")
	waitForSpreadsheetReplaceAll(t, a)

	assertCell := func(row, column int, want string) {
		t.Helper()
		window, err := book.Window(sheet, row, column, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got := window.Cells[0][0].Value; got != want {
			t.Fatalf("cell (%d,%d) = %q; want %q", row, column, got, want)
		}
	}
	assertCell(1, 1, "X apples and X pears")
	assertCell(2, 1, "preXpost")
	assertCell(1, 2, "23")
	formula, err := book.GetCellFormula(sheet, "B2")
	if err != nil || formula != "23" {
		t.Fatalf("formula B2 = %q, err=%v; want unchanged expression 23", formula, err)
	}
	other, err := book.Window("Other", 1, 1, 1, 1)
	if err != nil || other.Cells[0][0].Value != "23" {
		t.Fatalf("other worksheet A1 = %+v, err=%v; want untouched value 23", other, err)
	}
	if got := a.messageText(a.modal.SearchReplaceSummaryRef, a.modal.SearchReplaceSummary); !strings.HasPrefix(got, "已替换 2 个单元格") {
		t.Fatalf("Replace All summary = %q; want replaced-cell count and optional skipped-cell details", got)
	}
	if undone, err := book.Undo(); err != nil || !undone {
		t.Fatalf("undo Replace All = %v, err=%v; want one undoable batch", undone, err)
	}
	assertCell(1, 1, "23 apples and 23 pears")
	assertCell(2, 1, "pre23post")
}

func waitForSpreadsheetReplaceAll(t *testing.T, a *App) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case result := <-a.formatResults:
			if result.err != nil {
				t.Fatalf("Replace All failed: %v", result.err)
			}
			operation := result.operation
			a.formatResults <- result
			a.processSpreadsheetFormatResult()
			if operation == "替换全部" {
				return
			}
		case <-deadline.C:
			t.Fatal("Replace All did not complete")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
