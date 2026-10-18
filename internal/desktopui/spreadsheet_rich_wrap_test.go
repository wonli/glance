package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/xuri/excelize/v2"
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"image"
	"path/filepath"
	"testing"
)

func TestRichTextCellHonorsWrapWidth(t *testing.T) {
	for _, scale := range []int{1, 2} {
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Constraints: layout.Constraints{Max: image.Pt(60*scale, 400*scale)}, Metric: unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}}
		style := document.CellStyle{WrapText: true}
		dims := paintRichTextCell(gtx, material.NewTheme(), []document.CellRichTextRun{{Text: "自动换行"}, {Text: "加粗后应该继续自动换行", Bold: true}}, style, paintStyle(style), 1)
		if dims.Size.X > 60*scale || dims.Size.Y < 40*scale {
			t.Fatalf("%dx wrapped dimensions = %v", scale, dims.Size)
		}
	}
}

func TestSpreadsheetNoFillMetadataKeepsRibbonInactive(t *testing.T) {
	file := excelize.NewFile()
	defer file.Close()
	sheet := file.GetSheetName(0)
	id, err := file.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Pattern: 0, Color: []string{"000000"}}, Alignment: &excelize.Alignment{WrapText: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err = file.SetCellStyle(sheet, "A1", "A1", id); err != nil {
		t.Fatal(err)
	}
	if err = file.SetCellValue(sheet, "A1", "无填充但自动换行"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "no-fill.xlsx")
	if err = file.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	book, err := xlsx.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	state := newDocumentSessionState(path)
	state.book, state.sheet, state.selectedCell, state.selectionSet = book, sheet, "A1", true
	state.virtualGrid = newGridViewport()
	state.virtualGrid.Window, err = book.Window(sheet, 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	state.virtualGrid.WindowSheet = sheet
	state.virtualGrid.WindowRange = document.CellRange{StartRow: 1, EndRow: 1, StartColumn: 1, EndColumn: 1}
	app := New(nil, "")
	app.documentSessionState = state
	style := app.selectedCellStyle()
	if formatStyleActive(8, style) {
		t.Fatalf("no fill incorrectly activates ribbon: %+v", style)
	}
	if !formatStyleActive(toolbarActionWrapText, style) {
		t.Fatal("wrap ribbon lost active state")
	}
}
