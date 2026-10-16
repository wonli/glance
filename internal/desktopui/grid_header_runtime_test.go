package desktopui

import (
	"context"
	"image"
	"image/color"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

// This crosses the pointer -> UI command -> registered session -> XLSX boundary.
// Routing only the active cell, dropping the command's full bounds, or using the
// small-selection clear fallback must fail for at least one header scope.
func TestGridHeaderRegisteredRuntimeFormattingAndDelete(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		x, y                    float32
		futureRow, futureColumn int
		futureCell              string
		all                     bool
	}{
		{"row", 20, 108, 2, 26, "Z2", false},
		{"column", 437, 15, 100, 2, "B100", false},
		{"sheet", 20, 15, 100, 26, "Z100", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := headerSelectionHarness(t)
			a := h.app
			book := a.book.(*xlsx.Workbook)
			for _, cell := range []string{"A1", "B2", "C3"} {
				if err := book.SetCellText(a.sheet, cell, cell); err != nil {
					t.Fatal(err)
				}
			}
			italic, fill := true, "#FF0000"
			if err := book.SetCellFormats(a.sheet, document.CellRange{StartRow: 2, EndRow: 2, StartColumn: 2, EndColumn: 2}, document.CellFormatPatch{Italic: &italic, FillColor: &fill}); err != nil {
				t.Fatal(err)
			}
			book.RebaseHistory()
			desktop := newDesktopRuntime()
			t.Cleanup(desktop.shutdown)
			a.desktopRuntime = desktop
			a.documentID = workspaceSessionID(filepath.Join(t.TempDir(), "headers.xlsx"))
			desktop.registerSession(a.sharedDocumentSession)
			a.formatResults = make(chan spreadsheetFormatResult, 1)
			if !a.applicationSessionRegistered(a.documentID) {
				t.Fatal("workbook not registered")
			}
			events := make(chan application.Event, 16)
			cancel := desktop.applicationRuntime.On(context.Background(), application.EventFilter{Types: []string{application.EventDocumentChanged}, DocumentID: a.documentID}, func(event application.Event) { events <- event })
			defer cancel()
			wait := func(command string) {
				t.Helper()
				select {
				case result := <-a.formatResults:
					if result.err != nil {
						t.Fatalf("%s: %v", command, result.err)
					}
					if !result.applied {
						t.Fatalf("%s did not change workbook", command)
					}
					a.formatResults <- result
					a.processSpreadsheetFormatResult()
				case <-time.After(5 * time.Second):
					t.Fatalf("%s did not finish", command)
				}
				select {
				case event := <-events:
					if event.CommandID != command || event.Source != application.OperatorHuman {
						t.Fatalf("command event=%+v; want human %s", event, command)
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("%s did not publish registered command event", command)
				}
			}
			read := func(w *xlsx.Workbook, row, col int) document.Cell {
				t.Helper()
				window, err := w.Window(a.sheet, row, col, 1, 1)
				if err != nil {
					t.Fatal(err)
				}
				return window.Cells[0][0]
			}
			initial := read(book, 2, 2).Style
			outside := read(book, 1, 1).Style
			futureInitial := read(book, tc.futureRow, tc.futureColumn).Style
			h.point(pointer.Press, tc.x, tc.y)
			h.point(pointer.Release, tc.x, tc.y)
			if !h.router.Source().Focused(&a.virtualGrid.PointerTag) {
				t.Fatal("header did not focus grid")
			}
			family, size := "Synthetic Header Font", float64(22)
			a.applyCellFormat(document.CellFormatPatch{FontFamily: &family})
			wait("spreadsheet.range.format.set")
			familyOnly := read(book, tc.futureRow, tc.futureColumn).Style
			a.applyCellFormat(document.CellFormatPatch{FontSize: &size})
			wait("spreadsheet.range.format.set")
			checkFormatted := func(w *xlsx.Workbook) {
				t.Helper()
				for _, pos := range [][2]int{{2, 2}, {tc.futureRow, tc.futureColumn}} {
					cell := read(w, pos[0], pos[1])
					if cell.Style.FontFamily != family || cell.Style.FontSize != size {
						t.Fatalf("cell %v font=%+v; want %q/%v", pos, cell.Style, family, size)
					}
				}
				got := read(w, 2, 2).Style
				if !got.Italic || got.FillColor != initial.FillColor {
					t.Fatalf("existing style lost: %+v", got)
				}
				if !tc.all && read(w, 1, 1).Style != outside {
					t.Fatal("font changed outside selected header")
				}
			}
			checkFormatted(book)
			a.applyHistory(true)
			wait("spreadsheet.history.apply")
			if got := read(book, tc.futureRow, tc.futureColumn).Style; got != familyOnly {
				t.Fatalf("undo size=%+v", got)
			}
			a.applyHistory(true)
			wait("spreadsheet.history.apply")
			if got := read(book, 2, 2).Style; got != initial {
				t.Fatalf("undo family=%+v; want %+v", got, initial)
			}
			if got := read(book, tc.futureRow, tc.futureColumn).Style; got != futureInitial {
				t.Fatalf("undo blank default=%+v; want %+v", got, futureInitial)
			}
			for i := 0; i < 2; i++ {
				a.applyHistory(false)
				wait("spreadsheet.history.apply")
			}
			checkFormatted(book)
			h.router.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
			h.frame()
			wait("spreadsheet.range.clear")
			checkCleared := func(w *xlsx.Workbook) {
				t.Helper()
				for _, c := range []struct {
					name     string
					row, col int
				}{{"A1", 1, 1}, {"B2", 2, 2}, {"C3", 3, 3}} {
					want := c.name
					if tc.all || c.name == "B2" {
						want = ""
					}
					if got := read(w, c.row, c.col).Value; got != want {
						t.Fatalf("%s=%q; want %q", c.name, got, want)
					}
				}
				checkFormatted(w)
			}
			checkCleared(book)
			if a.virtualGrid.Rows.sizeAt(0) != 60 || a.virtualGrid.Columns.sizeAt(0) != 350 {
				t.Fatal("Delete changed grid dimensions")
			}
			a.applyHistory(true)
			wait("spreadsheet.history.apply")
			for _, c := range []struct {
				name     string
				row, col int
			}{{"A1", 1, 1}, {"B2", 2, 2}, {"C3", 3, 3}} {
				if got := read(book, c.row, c.col).Value; got != c.name {
					t.Fatalf("undo clear %s=%q", c.name, got)
				}
			}
			a.applyHistory(false)
			wait("spreadsheet.history.apply")
			target := filepath.Join(t.TempDir(), "headers.xlsx")
			if _, err := desktop.applicationRuntime.SaveDocumentAs(context.Background(), a.documentID, target); err != nil {
				t.Fatal(err)
			}
			reopened, err := xlsx.Open(target)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			checkCleared(reopened)
			if err := reopened.SetCellText(a.sheet, tc.futureCell, "future value"); err != nil {
				t.Fatal(err)
			}
			if got := read(reopened, tc.futureRow, tc.futureColumn); got.Value != "future value" || got.Style.FontFamily != family || got.Style.FontSize != size {
				t.Fatalf("future cell did not inherit font: %+v", got)
			}
		})
	}
}

// Retina hit coordinates and the selected header's visible fill must scale together.
func TestGridHeaderSelectionPaintsSelectedHeadersRetina(t *testing.T) {
	for _, tc := range []struct {
		name  string
		x, y  float32
		pixel image.Point
	}{
		{"row", 20, 108, image.Pt(5, 108)},
		{"column", 437, 15, image.Pt(400, 5)},
		{"sheet", 20, 15, image.Pt(5, 5)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := headerSelectionHarness(t)
			h.size = image.Pt(1200, 480)
			draw := h.draw
			h.draw = func(gtx layout.Context) { gtx.Metric = unit.Metric{PxPerDp: 2, PxPerSp: 2}; draw(gtx) }
			h.frame()
			h.point(pointer.Press, tc.x*2, tc.y*2)
			h.point(pointer.Release, tc.x*2, tc.y*2)
			img := renderPresentationTestOps(t, &h.ops, h.size)
			got := color.NRGBAModel.Convert(img.At(tc.pixel.X*2, tc.pixel.Y*2)).(color.NRGBA)
			if got != selectedFill {
				t.Fatalf("Retina header fill=%v; want %v", got, selectedFill)
			}
		})
	}
}
