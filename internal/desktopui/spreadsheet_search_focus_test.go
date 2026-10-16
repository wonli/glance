package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetFindDismissalReturnsFocusToGrid(t *testing.T) {
	for _, dismiss := range []struct {
		name string
		act  func(*gridTextHarness, func())
	}{
		{name: "Escape", act: func(h *gridTextHarness, frame func()) {
			h.router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
			frame()
		}},
		{name: "close button", act: func(h *gridTextHarness, frame func()) {
			position := f32.Pt(650, 104)
			h.router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position, Time: time.Second})
			frame()
			h.router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position, Time: time.Second + time.Millisecond})
			frame()
			frame()
		}},
	} {
		t.Run(dismiss.name, func(t *testing.T) {
			h := newFullGridTextHarness(t, false, "value")
			app, state := h.app, h.app.documentSessionState
			state.cellEdit.cancel()
			app.selectedCell = "A1"
			state.selectedCell = "A1"
			app.showSpreadsheetFindDialog()

			focusGrid := true
			frame := func() {
				h.ops.Reset()
				gtx := layout.Context{
					Source:      h.router.Source(),
					Ops:         &h.ops,
					Now:         time.Now(),
					Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
					Constraints: layout.Exact(image.Pt(820, 500)),
				}
				if focusGrid {
					gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
					focusGrid = false
				}
				app.handleOverlayEscape(gtx)
				app.layoutVirtualGrid(gtx)
				app.layoutModalDialog(gtx)
				h.router.Frame(&h.ops)
			}
			frame()
			if !h.router.Source().Focused(&app.modal.Input) {
				t.Fatal("Find query did not receive focus while the dialog was open")
			}

			dismiss.act(h, frame)
			if app.modal.Visible {
				t.Fatal("dismiss action left the Find dialog open")
			}
			if !h.router.Source().Focused(&state.virtualGrid.PointerTag) {
				t.Fatal("closing Find did not return keyboard focus to the grid")
			}
			if app.selectedCell != "A1" {
				t.Fatalf("active cell after closing Find = %q; want A1", app.selectedCell)
			}
			h.router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press})
			frame()
			if app.selectedCell != "B1" {
				t.Fatalf("active cell after Right Arrow = %q; want B1", app.selectedCell)
			}
		})
	}
}

func TestSpreadsheetFindPreviousCyclesFromActiveCell(t *testing.T) {
	for _, test := range []struct {
		name      string
		activeRow int
		wantCell  string
	}{
		{name: "previous result", activeRow: 2, wantCell: "A1"},
		{name: "wrap from first result", activeRow: 0, wantCell: "A3"},
	} {
		t.Run(test.name, func(t *testing.T) {
			book := xlsx.NewBlank()
			defer book.Close()
			sheet := book.SheetNames()[0]
			for _, address := range []string{"A1", "A3"} {
				if err := book.SetCellText(sheet, address, "needle"); err != nil {
					t.Fatal(err)
				}
			}
			state := newDocumentSessionState("find-previous.xlsx")
			state.book = book
			state.sheet = sheet
			state.selectedCell = xlsx.CellName(1, test.activeRow+1)
			state.selection = CellSelection{Anchor: CellPosition{Row: test.activeRow}, Active: CellPosition{Row: test.activeRow}}
			state.selectionSet = true
			viewport := newGridViewport()
			viewport.WindowSheet = sheet
			viewport.WindowRange = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 1}
			viewport.Window = document.SheetWindow{StartRow: 1, StartColumn: 1, Cells: [][]document.Cell{{{Value: "needle"}}, {{Value: ""}}, {{Value: "needle"}}}}
			state.virtualGrid = viewport
			app := &App{documentSessionState: state}
			app.virtualGrid = viewport
			app.selectedCell = state.selectedCell

			app.findPreviousSpreadsheetText(book, "needle")

			if app.selectedCell != test.wantCell {
				t.Fatalf("previous Find selected %s; want %s", app.selectedCell, test.wantCell)
			}
		})
	}
}
