package desktopui

import (
	"runtime"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestNextGridArrowPositionSkipsMergedCells(t *testing.T) {
	merges := []document.MergeRegion{{Range: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2}}}
	for _, test := range []struct {
		name string
		key  key.Name
		want CellPosition
	}{
		{name: "right", key: key.NameRightArrow, want: CellPosition{Row: 0, Column: 2}},
		{name: "down", key: key.NameDownArrow, want: CellPosition{Row: 2, Column: 0}},
		{name: "left boundary", key: key.NameLeftArrow, want: CellPosition{Row: 0, Column: 0}},
		{name: "up boundary", key: key.NameUpArrow, want: CellPosition{Row: 0, Column: 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, handled := nextGridArrowPosition(CellPosition{}, test.key, merges)
			if !handled || got != test.want {
				t.Fatalf("nextGridArrowPosition() = %+v, %t; want %+v, true", got, handled, test.want)
			}
		})
	}
}

func TestRevealGridCellScrollsOnlyAsFarAsNeeded(t *testing.T) {
	viewport := newGridViewport()
	revealGridCell(viewport, CellPosition{Row: 2, Column: 3}, 200, 64)
	if viewport.X != 56 || viewport.Y != 0 {
		t.Fatalf("viewport offset after revealing D3 = (%v,%v); want (56,0)", viewport.X, viewport.Y)
	}
	left, _ := viewport.Columns.Boundary(3)
	right, _ := viewport.Columns.Boundary(4)
	top, _ := viewport.Rows.Boundary(2)
	bottom, _ := viewport.Rows.Boundary(3)
	if left < viewport.X || right > viewport.X+200 || top < viewport.Y || bottom > viewport.Y+64 {
		t.Fatalf("D3 bounds x=%v..%v y=%v..%v remain outside viewport offsets (%v,%v)", left, right, top, bottom, viewport.X, viewport.Y)
	}
}

func TestSpreadsheetGoToSelectsRectangularRangeWithFirstCellActive(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	state := a.documentSessionState
	state.cellEdit.cancel()
	a.nameBox.SetText("B2:D4")

	if !a.goToCell(layout.Context{}) {
		t.Fatalf("Go To rejected rectangular range: status=%q", a.statusText())
	}
	wantBounds := document.CellRange{StartRow: 2, StartColumn: 2, EndRow: 4, EndColumn: 4}
	if got := state.selection.Bounds(); got != wantBounds {
		t.Fatalf("Go To selection = %+v; want %+v", got, wantBounds)
	}
	if a.selectedCell != "B2" || state.selection.Active != (CellPosition{Row: 1, Column: 1}) {
		t.Fatalf("Go To active cell = %q (%+v); want first address B2", a.selectedCell, state.selection.Active)
	}
	if state.selection.Anchor != (CellPosition{Row: 3, Column: 3}) {
		t.Fatalf("Go To anchor = %+v; want opposite endpoint D4", state.selection.Anchor)
	}
	if state.virtualGrid.SelectedRow != 2 || state.virtualGrid.SelectedColumn != 2 {
		t.Fatalf("grid active position = %d,%d; want B2 (2,2)", state.virtualGrid.SelectedRow, state.virtualGrid.SelectedColumn)
	}
}

func TestSpreadsheetGoToDefinedNameActivatesItsWorksheet(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	state := a.documentSessionState
	book := state.book.(*xlsx.Workbook)
	state.cellEdit.cancel()
	if err := book.EditStructure(document.StructureEdit{Kind: document.StructureCreateSheet, NewName: "Data Sheet"}); err != nil {
		t.Fatal(err)
	}
	state.sheets = book.SheetNames()
	if err := book.SetDefinedName(document.SpreadsheetDefinedName{
		Name: "SalesData", Ref: "'data sheet'!$B$2:$D$4", Scope: "Workbook",
	}); err != nil {
		t.Fatal(err)
	}
	a.nameBox.SetText("SalesData")

	if !a.goToCell(layout.Context{}) {
		t.Fatalf("Go To rejected a defined name: status=%q", a.statusText())
	}
	want := document.CellRange{StartRow: 2, StartColumn: 2, EndRow: 4, EndColumn: 4}
	if a.sheet != "Data Sheet" || state.sheet != "Data Sheet" {
		t.Fatalf("Go To defined name sheet = %q / %q; want Data Sheet", a.sheet, state.sheet)
	}
	if got := state.selection.Bounds(); got != want {
		t.Fatalf("Go To defined name selection = %+v; want %+v", got, want)
	}
	if a.selectedCell != "B2" {
		t.Fatalf("Go To defined name active cell = %q; want B2", a.selectedCell)
	}
}

func TestSpreadsheetGoToMissingWorksheetKeepsCurrentSelection(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	state := a.documentSessionState
	state.cellEdit.cancel()
	position := CellPosition{Row: 3, Column: 2}
	state.selection = CellSelection{Anchor: position, Active: position}
	state.selectionSet = true
	state.virtualGrid.SelectedRow, state.virtualGrid.SelectedColumn = 4, 3
	state.selectedCell = "C4"
	a.selectedCell = "C4"
	state.cellEdit.selectCell("C4", "value")
	a.nameBox.SetText("'Missing Sheet'!B2:D4")

	if a.goToCell(layout.Context{}) {
		t.Fatal("Go To accepted a reference to a missing worksheet")
	}
	if state.sheet != "Sheet1" || a.selectedCell != "C4" || state.selection.Bounds() != (document.CellRange{StartRow: 4, StartColumn: 3, EndRow: 4, EndColumn: 3}) {
		t.Fatalf("missing-sheet Go To changed sheet/selection to %q / %s / %+v", state.sheet, a.selectedCell, state.selection.Bounds())
	}
}

func TestSpreadsheetNameBoxEnterSelectsRectangularRange(t *testing.T) {
	h := newFullGridTextHarness(t, true, "value")
	a := h.app
	state := a.documentSessionState
	state.cellEdit.cancel()
	h.draw = func(gtx layout.Context) {
		a.handleOfficeShortcuts(gtx)
		a.layoutGridControls(gtx)
	}
	h.frame()
	h.router.Queue(key.Event{Name: key.NameF5, State: key.Press})
	h.frame()
	if a.nameBox.Text() != "A1" {
		t.Fatalf("F5 Name Box value = %q; want active cell A1", a.nameBox.Text())
	}
	a.nameBox.SetText("B2:D4")
	h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	h.frame()

	want := document.CellRange{StartRow: 2, StartColumn: 2, EndRow: 4, EndColumn: 4}
	if got := state.selection.Bounds(); got != want {
		t.Fatalf("Name Box Enter selection = %+v; want %+v (status=%q)", got, want, a.statusText())
	}
	if a.selectedCell != "B2" {
		t.Fatalf("Name Box Enter active cell = %q; want B2", a.selectedCell)
	}
}

func TestSpreadsheetGoToInvalidRangeKeepsCurrentSelection(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	state := a.documentSessionState
	state.cellEdit.cancel()
	position := CellPosition{Row: 3, Column: 2}
	state.selection = CellSelection{Anchor: position, Active: position}
	state.selectionSet = true
	state.virtualGrid.SelectedRow, state.virtualGrid.SelectedColumn = 4, 3
	state.selectedCell = "C4"
	a.selectedCell = "C4"
	state.cellEdit.selectCell("C4", "value")
	a.nameBox.SetText("B2:XFE4")

	if a.goToCell(layout.Context{}) {
		t.Fatal("Go To accepted an out-of-bounds range")
	}
	if got := state.selection.Bounds(); got != (document.CellRange{StartRow: 4, StartColumn: 3, EndRow: 4, EndColumn: 3}) || a.selectedCell != "C4" {
		t.Fatalf("invalid Go To changed selection to %+v / %s", got, a.selectedCell)
	}
}

func TestSpreadsheetGridArrowKeysMoveAndExtendSelection(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	state := a.documentSessionState
	state.cellEdit.cancel()
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	h.router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press})
	h.frame()
	if a.selectedCell != "B1" {
		t.Fatalf("active cell after Right = %q; want B1", a.selectedCell)
	}
	h.router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press, Modifiers: key.ModShift})
	h.frame()
	if a.selectedCell != "C1" {
		t.Fatalf("active cell after Shift+Right = %q; want C1", a.selectedCell)
	}
	want := document.CellRange{StartRow: 1, StartColumn: 2, EndRow: 1, EndColumn: 3}
	if got := state.selection.Bounds(); got != want {
		t.Fatalf("selection after Shift+Right = %+v; want %+v", got, want)
	}

	h.router.Queue(key.Event{Name: key.NameLeftArrow, State: key.Press})
	h.frame()
	if a.selectedCell != "B1" {
		t.Fatalf("active cell after Left = %q; want B1", a.selectedCell)
	}
	if got := state.selection.Bounds(); got != (document.CellRange{StartRow: 1, StartColumn: 2, EndRow: 1, EndColumn: 2}) {
		t.Fatalf("selection after unmodified Left = %+v; want collapsed B1 selection", got)
	}
}

func TestSpreadsheetGridSpaceShortcutsSelectActiveRowAndColumn(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	state := a.documentSessionState
	state.cellEdit.cancel()
	active := CellPosition{Row: 3, Column: 2}
	state.selection = CellSelection{Anchor: active, Active: active}
	state.selectionSet = true
	state.selectedCell = "C4"
	a.selectedCell = "C4"
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	h.router.Queue(key.Event{Name: key.NameSpace, State: key.Press, Modifiers: key.ModShift})
	h.frame()
	wantRow := document.CellRange{StartRow: 4, EndRow: 4, StartColumn: 1, EndColumn: xlsxColumns}
	if got := state.selection.Bounds(); got != wantRow {
		t.Fatalf("Shift+Space selection = %+v; want entire active row %+v", got, wantRow)
	}

	h.router.Queue(key.Event{Name: key.NameSpace, State: key.Press, Modifiers: key.ModCtrl})
	h.frame()
	wantColumn := document.CellRange{StartRow: 1, EndRow: xlsxRows, StartColumn: 3, EndColumn: 3}
	if got := state.selection.Bounds(); got != wantColumn {
		t.Fatalf("Ctrl+Space selection = %+v; want entire active column %+v", got, wantColumn)
	}
}

func TestSpreadsheetGridSheetShortcutsSwitchSheetsAndRestoreActiveCell(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	state := a.documentSessionState
	book := a.book.(*xlsx.Workbook)
	if err := book.EditStructure(document.StructureEdit{Kind: document.StructureCreateSheet, NewName: "Other"}); err != nil {
		t.Fatal(err)
	}
	state.sheets = book.SheetNames()
	state.cellEdit.cancel()
	state.selectedCell = "C4"
	a.selectedCell = "C4"
	state.selection = CellSelection{Anchor: CellPosition{Row: 3, Column: 2}, Active: CellPosition{Row: 3, Column: 2}}
	state.selectionSet = true
	state.virtualGrid.SelectedRow, state.virtualGrid.SelectedColumn = 4, 3
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	other := state.sheets[1]
	h.router.Queue(key.Event{Name: key.NamePageDown, State: key.Press, Modifiers: key.ModCtrl})
	h.frame()
	if a.sheet != other || a.selectedCell != "A1" {
		t.Fatalf("Ctrl+PageDown switched to sheet %q at %s; want %q at its initial A1", a.sheet, a.selectedCell, other)
	}

	for range 2 {
		h.router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press})
		h.frame()
	}
	if a.selectedCell != "C1" {
		t.Fatalf("active cell on second sheet = %q; want C1 before switching back", a.selectedCell)
	}

	h.router.Queue(key.Event{Name: key.NamePageUp, State: key.Press, Modifiers: key.ModCtrl})
	h.frame()
	if a.sheet != state.sheets[0] || a.selectedCell != "C4" {
		t.Fatalf("Ctrl+PageUp restored sheet %q at %s; want %q at C4", a.sheet, a.selectedCell, state.sheets[0])
	}

	h.router.Queue(key.Event{Name: key.NamePageUp, State: key.Press, Modifiers: key.ModCtrl})
	h.frame()
	if a.sheet != state.sheets[0] || a.selectedCell != "C4" {
		t.Fatalf("Ctrl+PageUp at the first sheet changed to %q at %s", a.sheet, a.selectedCell)
	}

	h.router.Queue(key.Event{Name: key.NamePageDown, State: key.Press, Modifiers: key.ModCtrl})
	h.frame()
	if a.sheet != other || a.selectedCell != "C1" {
		t.Fatalf("Ctrl+PageDown restored sheet %q at %s; want %q at C1", a.sheet, a.selectedCell, other)
	}
	if runtime.GOOS == "darwin" {
		h.router.Queue(key.Event{Name: key.NameLeftArrow, State: key.Press, Modifiers: key.ModAlt})
		h.frame()
		if a.sheet != state.sheets[0] || a.selectedCell != "C4" {
			t.Fatalf("Option+Left restored sheet %q at %s; want %q at C4", a.sheet, a.selectedCell, state.sheets[0])
		}
		h.router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press, Modifiers: key.ModAlt})
		h.frame()
		if a.sheet != other || a.selectedCell != "C1" {
			t.Fatalf("Option+Right restored sheet %q at %s; want %q at C1", a.sheet, a.selectedCell, other)
		}
	}
}

func TestSpreadsheetGridArrowKeysStayWithinXLSXBounds(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	state := a.documentSessionState
	state.cellEdit.cancel()
	state.selection = CellSelection{Anchor: CellPosition{}, Active: CellPosition{}}
	state.selectionSet = true
	state.selectedCell = "A1"
	a.selectedCell = "A1"
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()
	h.router.Queue(key.Event{Name: key.NameLeftArrow, State: key.Press})
	h.frame()
	h.router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press})
	h.frame()
	if a.selectedCell != "A1" {
		t.Fatalf("active cell at top-left boundary = %q; want A1", a.selectedCell)
	}
}

func TestSpreadsheetGridEnterAndTabMoveTheActiveCell(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	state := a.documentSessionState
	book := state.book.(*xlsx.Workbook)
	state.cellEditor.SetText("edited")
	h.draw = func(gtx layout.Context) {
		if state.cellEdit.Editing {
			gtx.Execute(key.FocusCmd{Tag: &state.cellEditor})
		} else {
			gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		}
		a.layoutVirtualGrid(gtx)
	}
	h.frame()
	h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	h.frame()
	if a.selectedCell != "A2" {
		t.Fatalf("active cell after Enter = %q; want A2", a.selectedCell)
	}
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "edited" {
		t.Fatalf("A1 after Enter = %q, %v; want committed value edited", got, err)
	}
	h.router.Queue(key.Event{Name: key.NameTab, State: key.Press})
	h.frame()
	if a.selectedCell != "B2" {
		t.Fatalf("active cell after Tab = %q; want B2", a.selectedCell)
	}
	state.cellEdit.begin()
	state.cellEditor.SetText("right")
	h.frame()
	h.router.Queue(key.Event{Name: key.NameTab, State: key.Press})
	h.frame()
	if a.selectedCell != "C2" {
		t.Fatalf("active cell after Tab from cell editor = %q; want C2", a.selectedCell)
	}
	if got, err := book.Value(state.sheet, "B2"); err != nil || got != "right" {
		t.Fatalf("B2 after Tab = %q, %v; want committed value right", got, err)
	}
	h.router.Queue(key.Event{Name: key.NameTab, State: key.Press, Modifiers: key.ModShift})
	h.frame()
	if a.selectedCell != "B2" {
		t.Fatalf("active cell after Shift+Tab = %q; want B2", a.selectedCell)
	}
	state.cellEdit.begin()
	state.cellEditor.SetText("below")
	h.frame()
	h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press, Modifiers: key.ModShift})
	h.frame()
	if a.selectedCell != "B1" {
		t.Fatalf("active cell after Shift+Enter = %q; want B1", a.selectedCell)
	}
	if got, err := book.Value(state.sheet, "B2"); err != nil || got != "below" {
		t.Fatalf("B2 after Shift+Enter = %q, %v; want committed value below", got, err)
	}
}

func TestSpreadsheetFormulaBarEnterMovesTheActiveCellDown(t *testing.T) {
	h := newFullGridTextHarness(t, true, "value")
	a := h.app
	state := a.documentSessionState
	book := state.book.(*xlsx.Workbook)
	state.formulaEditor.SetText("formula bar edit")
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.formulaEditor})
		a.layoutGridControls(gtx)
	}
	h.frame()
	h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	h.frame()
	if a.selectedCell != "A2" {
		t.Fatalf("active cell after formula-bar Enter = %q; want A2", a.selectedCell)
	}
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "formula bar edit" {
		t.Fatalf("A1 after formula-bar Enter = %q, %v; want committed text", got, err)
	}
}

func TestSpreadsheetGridTabCommitsQueuedEditorTextBeforeMoving(t *testing.T) {
	h := newFullGridTextHarness(t, false, "")
	a := h.app
	state := a.documentSessionState
	book := state.book.(*xlsx.Workbook)
	state.cellEdit.begin()
	state.cellEditor.SetText("")
	h.draw = func(gtx layout.Context) {
		if state.cellEdit.Editing {
			gtx.Execute(key.FocusCmd{Tag: &state.cellEditor})
		} else {
			gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		}
		a.layoutVirtualGrid(gtx)
	}
	h.frame()
	h.router.Queue(key.EditEvent{Text: "豆豆", Range: key.Range{Start: 0, End: 0}})
	h.router.Queue(key.Event{Name: key.NameTab, State: key.Press})
	h.frame()
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "豆豆" {
		t.Fatalf("A1 after Chinese input then Tab = %q, %v; want committed value 豆豆", got, err)
	}
	if a.selectedCell != "B1" {
		t.Fatalf("active cell after typing then Tab = %q; want B1", a.selectedCell)
	}
}

func TestSpreadsheetGridTypingReplacesSelectedCellAndWaitsForCommit(t *testing.T) {
	h := newFullGridTextHarness(t, false, "existing value")
	a := h.app
	state := a.documentSessionState
	book := state.book.(*xlsx.Workbook)
	if err := book.SetCellText(state.sheet, "A1", "existing value"); err != nil {
		t.Fatalf("SetCellText(A1): %v", err)
	}
	state.cellEdit.cancel()
	state.cellEditor.SetText("")
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	h.router.Queue(key.EditEvent{Text: "replacement", Range: key.Range{Start: 0, End: 0}})
	h.frame()
	if !state.cellEdit.Editing || state.cellEditor.Text() != "replacement" {
		t.Fatalf("cell edit after typing = editing %t, text %q; want an active replacement draft", state.cellEdit.Editing, state.cellEditor.Text())
	}
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "existing value" {
		t.Fatalf("A1 before commit = %q, %v; want the original value", got, err)
	}

	h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	h.frame()
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "replacement" {
		t.Fatalf("A1 after Enter = %q, %v; want replacement", got, err)
	}
}

func TestSpreadsheetF2StartsCellEditingAtEnd(t *testing.T) {
	h := newFullGridTextHarness(t, false, "existing value")
	a, state := h.app, h.app.documentSessionState
	book := state.book.(*xlsx.Workbook)
	if err := book.SetCellText(state.sheet, "A1", "existing value"); err != nil {
		t.Fatalf("SetCellText(A1): %v", err)
	}
	state.cellEdit.cancel()
	state.cellEditor.SetText("")
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()
	h.router.Queue(key.Event{Name: key.NameF2, State: key.Press})
	h.frame()
	start, end := state.cellEditor.Selection()
	if !state.cellEdit.Editing || state.cellEditor.Text() != "existing value" || start != len("existing value") || end != start {
		t.Fatalf("F2 edit state = editing %t, text %q, selection (%d,%d); want value and caret at end", state.cellEdit.Editing, state.cellEditor.Text(), start, end)
	}
	if !h.router.Source().Focused(&state.cellEditor) {
		t.Fatal("F2 did not focus the in-cell editor")
	}
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "existing value" {
		t.Fatalf("A1 after F2 = %q, %v; want unchanged until commit", got, err)
	}
}

func TestSpreadsheetFormulaReferenceShortcutCyclesReferenceAtCaret(t *testing.T) {
	h := newFullGridTextHarness(t, true, "=SUM(A1:$B$2)")
	a, state := h.app, h.app.documentSessionState
	state.formulaEditor.SetText("=SUM(A1:$B$2)")
	state.formulaEditor.SetCaret(6, 6)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.formulaEditor})
		a.layoutGridControls(gtx)
	}
	h.frame()
	cycle := key.Event{Name: key.NameF4, State: key.Press}
	if runtime.GOOS == "darwin" {
		cycle.Name, cycle.Modifiers = key.Name("T"), key.ModShortcut
	}
	h.router.Queue(cycle)
	h.frame()
	if got, want := state.formulaEditor.Text(), "=SUM($A$1:$B$2)"; got != want {
		t.Fatalf("formula after reference-cycle shortcut = %q; want %q", got, want)
	}
	start, end := state.formulaEditor.Selection()
	if start != end || start < 5 || start > len("=SUM($A$1") {
		t.Fatalf("formula caret after reference cycle = (%d,%d); want an insertion point inside the cycled reference", start, end)
	}
	if got, err := state.book.(*xlsx.Workbook).GetCellFormula(state.sheet, "A1"); err != nil || got != "" {
		t.Fatalf("A1 formula before commit = %q, %v; want unchanged", got, err)
	}
}

func TestSpreadsheetFillDownShortcutCopiesTopRowAndKeepsSelection(t *testing.T) {
	h := newFullGridTextHarness(t, false, "10")
	a, state := h.app, h.app.documentSessionState
	book := state.book.(*xlsx.Workbook)
	for address, value := range map[string]string{"A1": "10", "A2": "old-a2", "A3": "old-a3", "B2": "old-b2", "B3": "old-b3"} {
		if err := book.SetCellText(state.sheet, address, value); err != nil {
			t.Fatalf("SetCellText(%s): %v", address, err)
		}
	}
	if err := book.SetCellFormula(state.sheet, "B1", "=A1*2"); err != nil {
		t.Fatalf("SetCellFormula(B1): %v", err)
	}
	state.selection = CellSelection{Anchor: CellPosition{Row: 2, Column: 1}, Active: CellPosition{Row: 0, Column: 0}}
	state.selectionSet = true
	state.selectedCell = "A1"
	a.selectedCell = "A1"
	state.cellEdit.cancel()
	state.cellEditor.SetText("")
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()
	h.router.Queue(key.Event{Name: key.Name("D"), State: key.Press, Modifiers: key.ModShortcut})
	h.frame()
	if !state.formatRunning.Load() {
		t.Fatal("Ctrl/Cmd+D did not start a fill-down operation")
	}
	select {
	case result := <-a.formatResults:
		if result.err != nil || !result.applied || result.operation != "向下填充" {
			t.Fatalf("fill-down result = %+v; want one successful fill operation", result)
		}
		a.formatResults <- result
		a.processSpreadsheetFormatResult()
	case <-time.After(5 * time.Second):
		t.Fatal("Ctrl/Cmd+D did not finish filling down")
	}
	for address, want := range map[string]string{"A1": "10", "A2": "10", "A3": "10"} {
		if got, err := book.Value(state.sheet, address); err != nil || got != want {
			t.Errorf("%s value after fill down = %q, %v; want %q", address, got, err, want)
		}
	}
	for address, want := range map[string]string{"B1": "A1*2", "B2": "A2*2", "B3": "A3*2"} {
		if got, err := book.GetCellFormula(state.sheet, address); err != nil || got != want {
			t.Errorf("%s formula after fill down = %q, %v; want %q", address, got, err, want)
		}
	}
	if got, want := state.selection.Bounds(), (document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 2}); got != want {
		t.Fatalf("selection after fill down = %+v; want %+v", got, want)
	}
}

func TestSpreadsheetGridTypingStartsEditingBlankCell(t *testing.T) {
	h := newFullGridTextHarness(t, false, "")
	a := h.app
	state := a.documentSessionState
	state.cellEdit.cancel()
	state.cellEditor.SetText("")
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	h.router.Queue(key.EditEvent{Text: "值", Range: key.Range{Start: 0, End: 0}})
	h.frame()
	if !state.cellEdit.Editing || state.cellEditor.Text() != "值" {
		t.Fatalf("blank-cell entry = editing %t, text %q; want active draft 值", state.cellEdit.Editing, state.cellEditor.Text())
	}
	h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	h.frame()
	if got, err := state.book.(*xlsx.Workbook).Value(state.sheet, "A1"); err != nil || got != "值" {
		t.Fatalf("A1 after Enter = %q, %v; want 值", got, err)
	}
}

func TestSpreadsheetControlEnterFillsSelectedRangeInOneUndoableEdit(t *testing.T) {
	h := newFullGridTextHarness(t, false, "original")
	a, state := h.app, h.app.documentSessionState
	book := state.book.(*xlsx.Workbook)
	for address, value := range map[string]string{"A1": "original", "B1": "right", "A2": "below", "B2": "corner"} {
		if err := book.SetCellText(state.sheet, address, value); err != nil {
			t.Fatalf("SetCellText(%s): %v", address, err)
		}
	}
	bold, italic, fill, numberFormat := true, true, "#FFF2CC", "0.00"
	for _, formatted := range []struct {
		cell   string
		bounds document.CellRange
		patch  document.CellFormatPatch
	}{
		{"A1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, document.CellFormatPatch{Bold: &bold}},
		{"B1", document.CellRange{StartRow: 1, StartColumn: 2, EndRow: 1, EndColumn: 2}, document.CellFormatPatch{Italic: &italic}},
		{"A2", document.CellRange{StartRow: 2, StartColumn: 1, EndRow: 2, EndColumn: 1}, document.CellFormatPatch{FillColor: &fill}},
		{"B2", document.CellRange{StartRow: 2, StartColumn: 2, EndRow: 2, EndColumn: 2}, document.CellFormatPatch{NumberFormat: &numberFormat}},
	} {
		if err := book.SetCellFormats(state.sheet, formatted.bounds, formatted.patch); err != nil {
			t.Fatalf("SetCellFormats(%s): %v", formatted.cell, err)
		}
	}
	stylesBefore, err := book.ReadRange(state.sheet, document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2})
	if err != nil {
		t.Fatalf("ReadRange styles before Ctrl+Enter: %v", err)
	}
	book.RebaseHistory()
	state.selection = CellSelection{Anchor: CellPosition{Row: 1, Column: 1}, Active: CellPosition{Row: 0, Column: 0}}
	state.selectionSet = true
	state.selectedCell = "A1"
	a.selectedCell = "A1"
	state.cellEdit.selectCell("A1", "original")
	state.cellEdit.begin()
	state.cellEditor.SetText("shared")
	state.cellEdit.setDraft("shared")
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.cellEditor})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press, Modifiers: key.ModCtrl})
	h.frame()
	select {
	case result := <-a.formatResults:
		if result.err != nil || !result.applied || result.operation != "填充选区" {
			t.Fatalf("Ctrl+Enter result = %+v; want one successful fill operation", result)
		}
		a.formatResults <- result
		a.processSpreadsheetFormatResult()
	case <-time.After(5 * time.Second):
		t.Fatal("Ctrl+Enter did not fill the selected range")
	}
	for _, address := range []string{"A1", "B1", "A2", "B2"} {
		if got, err := book.Value(state.sheet, address); err != nil || got != "shared" {
			t.Errorf("%s after Ctrl+Enter = %q, %v; want shared", address, got, err)
		}
	}
	stylesAfter, err := book.ReadRange(state.sheet, document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2})
	if err != nil {
		t.Fatalf("ReadRange styles after Ctrl+Enter: %v", err)
	}
	for row := range stylesBefore {
		for column := range stylesBefore[row] {
			if got, want := stylesAfter[row][column].StyleID, stylesBefore[row][column].StyleID; got != want {
				t.Errorf("cell style at row %d column %d after Ctrl+Enter = %d; want preserved style %d", row, column, got, want)
			}
		}
	}
	if got := state.selection.Bounds(); got != (document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2}) {
		t.Fatalf("selection after Ctrl+Enter = %+v; want A1:B2", got)
	}
	if changed, err := book.Undo(); err != nil || !changed {
		t.Fatalf("Undo Ctrl+Enter = changed %t, err %v; want one undoable edit", changed, err)
	}
	for address, want := range map[string]string{"A1": "original", "B1": "right", "A2": "below", "B2": "corner"} {
		if got, err := book.Value(state.sheet, address); err != nil || got != want {
			t.Errorf("%s after undo = %q, %v; want %q", address, got, err, want)
		}
	}
}

func TestSpreadsheetControlEnterFillsSelectedRangeFormulas(t *testing.T) {
	h := newFullGridTextHarness(t, true, "=SUM(A1:A2)")
	a, state := h.app, h.app.documentSessionState
	book := state.book.(*xlsx.Workbook)
	book.RebaseHistory()
	state.selection = CellSelection{Anchor: CellPosition{Row: 1, Column: 1}, Active: CellPosition{Row: 0, Column: 0}}
	state.selectionSet = true
	state.selectedCell = "A1"
	a.selectedCell = "A1"
	state.cellEdit.selectCell("A1", "=SUM(A1:A2)")
	state.cellEdit.begin()
	state.cellEditor.SetText("=SUM(A1:A2)")
	state.formulaEditor.SetText("=SUM(A1:A2)")
	state.cellEdit.setDraft("=SUM(A1:A2)")
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.formulaEditor})
		a.layoutGridControls(gtx)
	}
	h.frame()

	h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press, Modifiers: key.ModCtrl})
	h.frame()
	select {
	case result := <-a.formatResults:
		if result.err != nil || !result.applied || result.operation != "填充选区" {
			t.Fatalf("Ctrl+Enter formula result = %+v; want one successful fill operation", result)
		}
		a.formatResults <- result
		a.processSpreadsheetFormatResult()
	case <-time.After(5 * time.Second):
		t.Fatalf("Ctrl+Enter did not fill the selected range with formulas: focus=%t running=%t status=%q", h.router.Source().Focused(&state.formulaEditor), state.formatRunning.Load(), a.statusText())
	}
	for address, want := range map[string]string{
		"A1": "SUM(A1:A2)", "B1": "SUM(B1:B2)",
		"A2": "SUM(A2:A3)", "B2": "SUM(B2:B3)",
	} {
		if got, err := book.GetCellFormula(state.sheet, address); err != nil || got != want {
			t.Errorf("%s formula after Ctrl+Enter = %q, %v; want %s", address, got, err, want)
		}
	}
	if changed, err := book.Undo(); err != nil || !changed {
		t.Fatalf("Undo Ctrl+Enter formula batch = changed %t, err %v; want one undoable edit", changed, err)
	}
	for _, address := range []string{"A1", "B1", "A2", "B2"} {
		if got, err := book.GetCellFormula(state.sheet, address); err != nil || got != "" {
			t.Errorf("%s formula after undo = %q, %v; want empty", address, got, err)
		}
	}
}

func TestSpreadsheetControlEnterRejectsRangesOverCellLimit(t *testing.T) {
	h := newFullGridTextHarness(t, false, "original")
	a, state := h.app, h.app.documentSessionState
	if err := state.book.(*xlsx.Workbook).SetCellText(state.sheet, "A1", "original"); err != nil {
		t.Fatal(err)
	}
	state.selection = CellSelection{Anchor: CellPosition{Row: maxSpreadsheetRangeEntryCells, Column: 0}, Active: CellPosition{Row: 0, Column: 0}}
	state.selectionSet = true
	if err := a.enterSpreadsheetValueInSelection("shared"); err == nil {
		t.Fatal("Ctrl+Enter accepted more than the bounded selection size")
	}
	if got, err := state.book.(*xlsx.Workbook).Value(state.sheet, "A1"); err != nil || got != "original" {
		t.Fatalf("A1 after oversized fill = %q, %v; want original", got, err)
	}
}

func TestSpreadsheetControlEnterDoesNotCommitDuringIMEComposition(t *testing.T) {
	h := newFullGridTextHarness(t, false, "original")
	a, state := h.app, h.app.documentSessionState
	book := state.book.(*xlsx.Workbook)
	if err := book.SetCellText(state.sheet, "A1", "original"); err != nil {
		t.Fatalf("SetCellText(A1): %v", err)
	}
	state.selection = CellSelection{Anchor: CellPosition{Row: 1, Column: 1}, Active: CellPosition{Row: 0, Column: 0}}
	state.selectionSet = true
	state.cellEdit.cancel()
	state.cellEditor.SetText("")
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	h.router.Queue(key.CompositionEvent{Start: 0, End: 2}, key.EditEvent{Text: "ni", Range: key.Range{Start: 0, End: 0}})
	h.frame()
	if !state.gridInputComposing || state.cellEditor.Text() != "ni" {
		t.Fatalf("preedit state = composing %t, text %q; want composing draft ni", state.gridInputComposing, state.cellEditor.Text())
	}
	h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press, Modifiers: key.ModCtrl})
	h.frame()
	if !state.cellEdit.Editing || state.cellEditor.Text() != "ni" {
		t.Fatalf("cell edit after Ctrl+Enter during composition = editing %t, text %q; want active preedit draft ni", state.cellEdit.Editing, state.cellEditor.Text())
	}
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "original" {
		t.Fatalf("A1 after Ctrl+Enter during composition = %q, %v; want original", got, err)
	}
	select {
	case result := <-a.formatResults:
		t.Fatalf("Ctrl+Enter during composition started an operation: %+v", result)
	default:
	}
}

func TestSpreadsheetControlEnterUnchangedRangeDoesNotCreateHistory(t *testing.T) {
	h := newFullGridTextHarness(t, false, "same")
	a, state := h.app, h.app.documentSessionState
	book := state.book.(*xlsx.Workbook)
	edits := []document.CellEdit{
		{Row: 1, Column: 1, Value: "same"}, {Row: 1, Column: 2, Value: "same"},
		{Row: 2, Column: 1, Value: "same"}, {Row: 2, Column: 2, Value: "same"},
	}
	if err := book.SetCells(state.sheet, edits); err != nil {
		t.Fatalf("SetCells: %v", err)
	}
	book.RebaseHistory()
	state.selection = CellSelection{Anchor: CellPosition{Row: 1, Column: 1}, Active: CellPosition{Row: 0, Column: 0}}
	state.selectionSet = true
	state.cellEdit.selectCell("A1", "same")
	state.cellEdit.begin()
	state.cellEditor.SetText("same")
	state.cellEdit.setDraft("same")
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	if err := a.enterSpreadsheetValueInSelection("same"); err != nil {
		t.Fatalf("enterSpreadsheetValueInSelection: %v", err)
	}
	select {
	case result := <-a.formatResults:
		if result.err != nil || !result.noOp || result.applied {
			t.Fatalf("unchanged Ctrl+Enter result = %+v; want successful no-op", result)
		}
		if changed, err := book.Undo(); err != nil || !changed {
			t.Fatalf("Undo after unchanged Ctrl+Enter = changed %t, err %v; want to undo only the seed edit", changed, err)
		}
		for _, address := range []string{"A1", "B1", "A2", "B2"} {
			if got, err := book.Value(state.sheet, address); err != nil || got != "" {
				t.Errorf("%s after undo = %q, %v; want empty (no Ctrl+Enter history step)", address, got, err)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("unchanged Ctrl+Enter did not finish")
	}
}

func TestSpreadsheetGridBackspaceEditsActiveCellOnMac(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Excel for Mac uses Backspace to edit the active cell")
	}
	h := newFullGridTextHarness(t, false, "existing value")
	a := h.app
	state := a.documentSessionState
	book := state.book.(*xlsx.Workbook)
	if err := book.SetCellText(state.sheet, "A1", "existing value"); err != nil {
		t.Fatalf("SetCellText(A1): %v", err)
	}
	state.cellEdit.cancel()
	state.cellEditor.SetText("")
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	h.router.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
	h.frame()
	if !state.cellEdit.Editing || state.cellEditor.Text() != "existing valu" {
		t.Fatalf("cell edit after Backspace = editing %t, text %q; want an active draft with the preceding character deleted", state.cellEdit.Editing, state.cellEditor.Text())
	}
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "existing value" {
		t.Fatalf("A1 before Enter = %q, %v; want existing value", got, err)
	}
	h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	h.frame()
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "existing valu" {
		t.Fatalf("A1 after Enter = %q, %v; want existing valu", got, err)
	}
}

func TestSpreadsheetGridIMECompositionStaysDraftUntilCommitted(t *testing.T) {
	h := newFullGridTextHarness(t, false, "original")
	a := h.app
	state := a.documentSessionState
	book := state.book.(*xlsx.Workbook)
	if err := book.SetCellText(state.sheet, "A1", "original"); err != nil {
		t.Fatalf("SetCellText(A1): %v", err)
	}
	state.cellEdit.cancel()
	state.cellEditor.SetText("")
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	h.router.Queue(key.CompositionEvent{Start: 0, End: 2}, key.EditEvent{Text: "ni", Range: key.Range{Start: 0, End: 0}})
	h.frame()
	if !state.cellEdit.Editing || state.cellEditor.Text() != "ni" {
		t.Fatalf("preedit draft = editing %t, text %q; want active draft ni", state.cellEdit.Editing, state.cellEditor.Text())
	}
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "original" {
		t.Fatalf("A1 during composition = %q, %v; want original", got, err)
	}
	h.router.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
	h.frame()
	if state.cellEditor.Text() != "n" {
		t.Fatalf("preedit after Backspace = %q; want n", state.cellEditor.Text())
	}
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "original" {
		t.Fatalf("A1 after preedit Backspace = %q, %v; want original", got, err)
	}

	h.router.Queue(key.EditEvent{Text: "你", Range: key.Range{Start: 0, End: 1}}, key.CompositionEvent{Start: -1, End: -1})
	h.frame()
	if state.cellEditor.Text() != "你" {
		t.Fatalf("committed IME text = %q; want 你", state.cellEditor.Text())
	}
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "original" {
		t.Fatalf("A1 before Enter = %q, %v; want original", got, err)
	}

	h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	h.frame()
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "你" {
		t.Fatalf("A1 after Enter = %q, %v; want 你", got, err)
	}
}

func TestSpreadsheetGridEscapeCancelsDirectIMEEntry(t *testing.T) {
	h := newFullGridTextHarness(t, false, "original")
	a := h.app
	state := a.documentSessionState
	book := state.book.(*xlsx.Workbook)
	if err := book.SetCellText(state.sheet, "A1", "original"); err != nil {
		t.Fatalf("SetCellText(A1): %v", err)
	}
	state.cellEdit.cancel()
	state.cellEditor.SetText("")
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	h.router.Queue(key.CompositionEvent{Start: 0, End: 2}, key.EditEvent{Text: "ni", Range: key.Range{Start: 0, End: 0}})
	h.frame()
	h.router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	h.frame()
	if state.cellEdit.Editing || state.cellEditor.Text() != "original" {
		t.Fatalf("cell edit after Escape = editing %t, text %q; want canceled original", state.cellEdit.Editing, state.cellEditor.Text())
	}
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "original" {
		t.Fatalf("A1 after Escape = %q, %v; want original", got, err)
	}
}

func TestSpreadsheetGridArrowKeysDeferToCellEditor(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	state := a.documentSessionState
	state.cellEditor.SetCaret(0, 0)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.cellEditor})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()
	h.router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press})
	h.frame()
	if a.selectedCell != "A1" {
		t.Fatalf("active cell after Right in cell editor = %q; want A1", a.selectedCell)
	}
	start, end := state.cellEditor.Selection()
	if start != end || start == 0 {
		t.Fatalf("cell editor caret after Right = (%d,%d); want it to move within the editor", start, end)
	}
}

func TestSpreadsheetGridControlArrowJumpsAndExtendsSelection(t *testing.T) {
	h := newFullGridTextHarness(t, false, "one")
	a := h.app
	state := a.documentSessionState
	book := state.book.(*xlsx.Workbook)
	for cell, value := range map[string]string{"A1": "one", "B1": "two", "D1": "four", "E1": "five"} {
		if err := book.SetCellText(state.sheet, cell, value); err != nil {
			t.Fatalf("SetCellText(%s): %v", cell, err)
		}
	}
	state.cellEdit.cancel()
	state.selection = CellSelection{Anchor: CellPosition{}, Active: CellPosition{}}
	state.selectionSet = true
	state.selectedCell = "A1"
	a.selectedCell = "A1"
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	press := func(modifiers key.Modifiers, want string) {
		t.Helper()
		h.router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press, Modifiers: modifiers})
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			h.frame()
			if a.selectedCell == want {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("active cell after Ctrl/Cmd+Right with modifiers %v = %q; want %s", modifiers, a.selectedCell, want)
	}
	press(key.ModShortcut, "B1")
	press(key.ModShortcut|key.ModShift, "E1")
	if got := state.selection.Bounds(); got != (document.CellRange{StartRow: 1, StartColumn: 2, EndRow: 1, EndColumn: 5}) {
		t.Fatalf("selection after Ctrl/Cmd+Shift+Right = %+v; want B1:E1", got)
	}
}

func TestSpreadsheetGridHomeAndControlHomeMoveAndExtendSelection(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	state := a.documentSessionState
	state.cellEdit.cancel()
	setActive := func(position CellPosition) {
		state.selection = CellSelection{Anchor: position, Active: position}
		state.selectionSet = true
		state.selectedCell = xlsx.CellName(position.Column+1, position.Row+1)
		a.selectedCell = state.selectedCell
	}
	setActive(CellPosition{Row: 2, Column: 2}) // C3
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	h.router.Queue(key.Event{Name: key.NameHome, State: key.Press})
	h.frame()
	if a.selectedCell != "A3" {
		t.Fatalf("active cell after Home = %q; want A3", a.selectedCell)
	}
	setActive(CellPosition{Row: 2, Column: 2})
	h.frame()
	h.router.Queue(key.Event{Name: key.NameHome, State: key.Press, Modifiers: key.ModShift})
	h.frame()
	if got, want := state.selection.Bounds(), (document.CellRange{StartRow: 3, StartColumn: 1, EndRow: 3, EndColumn: 3}); got != want {
		t.Fatalf("selection after Shift+Home = %+v; want A3:C3 (%+v)", got, want)
	}

	homeEndModifier := key.ModShortcut
	if runtime.GOOS == "darwin" {
		homeEndModifier = key.ModCtrl
		setActive(CellPosition{Row: 2, Column: 2})
		h.frame()
		h.router.Queue(key.Event{Name: key.NameHome, State: key.Press, Modifiers: key.ModShortcut})
		h.frame()
		if a.selectedCell != "C3" {
			t.Fatalf("Cmd+Home on macOS changed active cell to %q; want C3 because Excel uses Ctrl+Home", a.selectedCell)
		}
	}
	setActive(CellPosition{Row: 2, Column: 2})
	h.frame()
	h.router.Queue(key.Event{Name: key.NameHome, State: key.Press, Modifiers: homeEndModifier})
	h.frame()
	if a.selectedCell != "A1" {
		t.Fatalf("active cell after Ctrl+Home = %q; want A1", a.selectedCell)
	}
	setActive(CellPosition{Row: 2, Column: 2})
	h.frame()
	h.router.Queue(key.Event{Name: key.NameHome, State: key.Press, Modifiers: homeEndModifier | key.ModShift})
	h.frame()
	if got, want := state.selection.Bounds(), (document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 3}); got != want {
		t.Fatalf("selection after Ctrl+Shift+Home = %+v; want A1:C3 (%+v)", got, want)
	}
}

func TestSpreadsheetGridControlEndMovesToUsedRangeAndExtendsSelection(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	state := a.documentSessionState
	book := state.book.(*xlsx.Workbook)
	if err := book.SetCellText(state.sheet, "D5", "last used"); err != nil {
		t.Fatal(err)
	}
	bold := true
	if err := book.SetCellFormats(state.sheet, document.CellRange{StartRow: 7, StartColumn: 6, EndRow: 7, EndColumn: 6}, document.CellFormatPatch{Bold: &bold}); err != nil {
		t.Fatal(err)
	}
	if maxColumn, maxRow, err := book.Dimension(state.sheet); err != nil || maxColumn != 6 || maxRow != 7 {
		t.Fatalf("Dimension() = (%d,%d,%v); want (6,7,nil), including styled blank F7", maxColumn, maxRow, err)
	}
	state.cellEdit.cancel()
	setActive := func(position CellPosition) {
		state.selection = CellSelection{Anchor: position, Active: position}
		state.selectionSet = true
		state.selectedCell = xlsx.CellName(position.Column+1, position.Row+1)
		a.selectedCell = state.selectedCell
	}
	setActive(CellPosition{}) // A1
	homeEndModifier := key.ModShortcut
	if runtime.GOOS == "darwin" {
		homeEndModifier = key.ModCtrl
	}
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	pressUntil := func(modifiers key.Modifiers, check func() bool) {
		t.Helper()
		h.router.Queue(key.Event{Name: key.NameEnd, State: key.Press, Modifiers: modifiers})
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			h.frame()
			if check() {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("Ctrl/Cmd+End with modifiers %v did not produce the expected selection; active=%q bounds=%+v status=%q", modifiers, a.selectedCell, state.selection.Bounds(), a.statusText())
	}
	pressUntil(homeEndModifier, func() bool { return a.selectedCell == "F7" })
	setActive(CellPosition{})
	h.frame()
	pressUntil(homeEndModifier|key.ModShift, func() bool {
		return state.selection.Bounds() == (document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 7, EndColumn: 6})
	})
}

func TestSpreadsheetGridEndModeJumpsOnceAndTogglesOff(t *testing.T) {
	h := newFullGridTextHarness(t, false, "one")
	a := h.app
	state := a.documentSessionState
	book := state.book.(*xlsx.Workbook)
	for cell, value := range map[string]string{"B1": "two", "D1": "four", "E1": "five"} {
		if err := book.SetCellText(state.sheet, cell, value); err != nil {
			t.Fatalf("SetCellText(%s): %v", cell, err)
		}
	}
	state.cellEdit.cancel()
	setActive := func(address string) {
		row, column, ok := parseCellAddress(address)
		if !ok {
			t.Fatalf("invalid test cell address %q", address)
		}
		position := CellPosition{Row: row - 1, Column: column - 1}
		state.selection = CellSelection{Anchor: position, Active: position}
		state.selectionSet = true
		state.selectedCell = address
		a.selectedCell = address
	}
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}

	pressUntil := func(name key.Name, want string) {
		t.Helper()
		h.router.Queue(key.Event{Name: name, State: key.Press})
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			h.frame()
			if a.selectedCell == want {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("active cell after %s = %q; want %s", name, a.selectedCell, want)
	}

	setActive("B1")
	h.frame()
	h.router.Queue(key.Event{Name: key.NameEnd, State: key.Press})
	h.frame()
	if !state.gridEndMode {
		t.Fatal("End did not enable End mode")
	}
	pressUntil(key.NameRightArrow, "D1")
	if state.gridEndMode {
		t.Fatal("End mode remained enabled after its arrow key")
	}
	pressUntil(key.NameRightArrow, "E1")

	setActive("B1")
	h.frame()
	h.router.Queue(key.Event{Name: key.NameEnd, State: key.Press})
	h.frame()
	if !state.gridEndMode {
		t.Fatal("End did not enable End mode before toggle-off")
	}
	h.router.Queue(key.Event{Name: key.NameEnd, State: key.Press})
	h.frame()
	if state.gridEndMode {
		t.Fatal("pressing End twice did not turn End mode off")
	}
	pressUntil(key.NameRightArrow, "C1")

	setActive("B1")
	h.frame()
	h.router.Queue(key.Event{Name: key.NameEnd, State: key.Press})
	h.frame()
	if !state.gridEndMode {
		t.Fatal("End did not enable End mode before Escape")
	}
	h.router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	h.frame()
	if state.gridEndMode {
		t.Fatal("Escape did not cancel End mode")
	}
}

func TestSpreadsheetGridShiftEndExtendsToLastColumnOfRow(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	state := a.documentSessionState
	state.cellEdit.cancel()
	position := CellPosition{Row: 2, Column: 2} // C3
	state.selection = CellSelection{Anchor: position, Active: position}
	state.selectionSet = true
	state.selectedCell = "C3"
	a.selectedCell = "C3"
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()
	h.router.Queue(key.Event{Name: key.NameEnd, State: key.Press, Modifiers: key.ModShift})
	h.frame()
	if want := "XFD3"; a.selectedCell != want {
		t.Fatalf("active cell after Shift+End = %q; want %s", a.selectedCell, want)
	}
	if got, want := state.selection.Bounds(), (document.CellRange{StartRow: 3, StartColumn: 3, EndRow: 3, EndColumn: 16_384}); got != want {
		t.Fatalf("selection after Shift+End = %+v; want C3:XFD3 (%+v)", got, want)
	}
}
