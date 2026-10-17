package desktopui

import (
	"testing"
	"time"

	"gioui.org/layout"
	"glance/internal/document"
)

type structureEditRecordingBook struct {
	toolbarTestBook
	edits chan document.StructureEdit
}

func (book *structureEditRecordingBook) EditStructure(edit document.StructureEdit) error {
	book.edits <- edit
	return nil
}

func TestSpreadsheetInsertRowsUsesWholeRowSelectionCount(t *testing.T) {
	book := &structureEditRecordingBook{toolbarTestBook: toolbarTestBook{editable: true}, edits: make(chan document.StructureEdit, 1)}
	state := newDocumentSessionState("insert-selected-rows.xlsx")
	state.book = book
	state.sheet = "Sheet1"
	state.selectionSet = true
	state.selection = CellSelection{Anchor: CellPosition{Row: 1}, Active: CellPosition{Row: 3}, Scope: selectionRows}
	state.toolbarTab = toolbarTabHome
	app := &App{documentSessionState: state, formatResults: make(chan spreadsheetFormatResult, 1)}
	t.Cleanup(app.stopAndWaitManagedBackground)
	state.toolbarActionButtons[12].Click()
	app.handleSpreadsheetToolbarEvents(layout.Context{})

	got := waitForStructureEdit(t, book.edits)
	want := document.StructureEdit{Kind: document.StructureInsertRows, Sheet: "Sheet1", Index: 2, Count: 3}
	if got != want {
		t.Fatalf("Insert on selected rows = %+v; want %+v", got, want)
	}
}

func TestSpreadsheetDeleteRowsAndColumnsUseWholeSelectionCount(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		scope   cellSelectionScope
		anchor  CellPosition
		active  CellPosition
		kind    document.StructureEditKind
		index   int
		count   int
	}{
		{name: "rows", command: "excel-delete-row", scope: selectionRows, anchor: CellPosition{Row: 1}, active: CellPosition{Row: 3}, kind: document.StructureDeleteRows, index: 2, count: 3},
		{name: "columns", command: "excel-delete-column", scope: selectionColumns, anchor: CellPosition{Column: 2}, active: CellPosition{Column: 4}, kind: document.StructureDeleteColumns, index: 3, count: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			book := &structureEditRecordingBook{toolbarTestBook: toolbarTestBook{editable: true}, edits: make(chan document.StructureEdit, 1)}
			state := newDocumentSessionState("delete-selected-" + tc.name + ".xlsx")
			state.book = book
			state.sheet = "Sheet1"
			state.selectionSet = true
			state.selection = CellSelection{Anchor: tc.anchor, Active: tc.active, Scope: tc.scope}
			app := &App{documentSessionState: state, formatResults: make(chan spreadsheetFormatResult, 1)}
			t.Cleanup(app.stopAndWaitManagedBackground)
			app.executeOfficeRibbonMenuCommand(layout.Context{}, officeRibbonMenuSpreadsheet, tc.command)

			got := waitForStructureEdit(t, book.edits)
			want := document.StructureEdit{Kind: tc.kind, Sheet: "Sheet1", Index: tc.index, Count: tc.count}
			if got != want {
				t.Fatalf("%s on whole selection = %+v; want %+v", tc.command, got, want)
			}
		})
	}
}

func TestSpreadsheetCellSelectionStructureActionRemainsSingleRowOrColumn(t *testing.T) {
	book := &structureEditRecordingBook{toolbarTestBook: toolbarTestBook{editable: true}, edits: make(chan document.StructureEdit, 1)}
	state := newDocumentSessionState("insert-active-row.xlsx")
	state.book = book
	state.sheet = "Sheet1"
	state.selectionSet = true
	state.selection = CellSelection{Anchor: CellPosition{Row: 1, Column: 2}, Active: CellPosition{Row: 4, Column: 5}}
	state.toolbarTab = toolbarTabHome
	app := &App{documentSessionState: state, formatResults: make(chan spreadsheetFormatResult, 1)}
	t.Cleanup(app.stopAndWaitManagedBackground)
	state.toolbarActionButtons[12].Click()
	app.handleSpreadsheetToolbarEvents(layout.Context{})

	got := waitForStructureEdit(t, book.edits)
	want := document.StructureEdit{Kind: document.StructureInsertRows, Sheet: "Sheet1", Index: 5, Count: 1}
	if got != want {
		t.Fatalf("Insert on cell selection = %+v; want %+v", got, want)
	}
}

func waitForStructureEdit(t *testing.T, edits <-chan document.StructureEdit) document.StructureEdit {
	t.Helper()
	select {
	case edit := <-edits:
		return edit
	case <-time.After(time.Second):
		t.Fatal("spreadsheet structure edit was not dispatched")
		return document.StructureEdit{}
	}
}
