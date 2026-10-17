package desktopui

import (
	"encoding/json"
	"testing"

	"glance/internal/format/xlsx"
	"glance/internal/workspace"
)

func TestOpeningSpreadsheetKeepsGridUnselectedAcrossSessionRestore(t *testing.T) {
	app := New(nil, "")
	defer close(app.shutdown)
	book := xlsx.NewBlank()
	if err := app.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatal(err)
	}
	if app.selectionSet || app.nameBox.Text() != "" || app.formulaEditor.Text() != "" {
		t.Fatalf("new spreadsheet opened with a visible selection: selected=%t name=%q formula=%q", app.selectionSet, app.nameBox.Text(), app.formulaEditor.Text())
	}
	if app.selectedCell != "A1" || app.cellEdit.Address != "A1" {
		t.Fatalf("initial navigation anchor=%q editor address=%q; want A1 without a visible selection", app.selectedCell, app.cellEdit.Address)
	}

	payload, err := book.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var savedView sessionViewSnapshot
	if err := json.Unmarshal(app.viewStateBytes(), &savedView); err != nil {
		t.Fatal(err)
	}
	if savedView.SelectionSet {
		t.Fatal("unselected spreadsheet view persisted an active selection")
	}
	restored, err := app.restoreSessionState(workspace.Session{Format: "xlsx", Payload: payload, ViewState: app.viewStateBytes()})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.book.Close()
	if restored.selectionSet || restored.nameBox.Text() != "" {
		t.Fatalf("spreadsheet restore selected a cell: selected=%t name=%q", restored.selectionSet, restored.nameBox.Text())
	}
}

func TestFirstSpreadsheetClickCreatesVisibleCellSelection(t *testing.T) {
	app := New(nil, "")
	defer close(app.shutdown)
	book := xlsx.NewBlank()
	if err := app.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatal(err)
	}
	app.updateGridSelection(app.virtualGrid, CellPosition{Row: 2, Column: 3}, false, false)
	if !app.selectionSet || app.selectedCell != "D3" || app.nameBox.Text() != "D3" {
		t.Fatalf("first grid click selection=%t cell=%q name=%q; want D3", app.selectionSet, app.selectedCell, app.nameBox.Text())
	}
}
