package desktopui

import (
	"image"
	"image/color"
	"path/filepath"
	"runtime"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/xuri/excelize/v2"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetConditionalFormatPresetEffectsRenderAtOneAndTwoX(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	path := filepath.Join(t.TempDir(), "conditional-format.xlsx")
	file := excelize.NewFile()
	for _, item := range []struct {
		cell  string
		value any
	}{
		{"A1", -10}, {"A2", 0}, {"A3", 20},
		{"B1", 1}, {"B2", 3}, {"B3", 5},
		{"C1", "apples"}, {"C2", "APPLES"}, {"C3", "pear"},
		{"D1", 1}, {"D2", 3}, {"D3", 5},
	} {
		if err := file.SetCellValue("Sheet1", item.cell, item.value); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	book, err := xlsx.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for _, spec := range []document.SpreadsheetConditionalFormatSpec{
		{Sheet: "Sheet1", Range: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 1}, Type: document.ConditionalFormatDataBar},
		{Sheet: "Sheet1", Range: document.CellRange{StartRow: 1, StartColumn: 2, EndRow: 3, EndColumn: 2}, Type: document.ConditionalFormat3ColorScale},
		{Sheet: "Sheet1", Range: document.CellRange{StartRow: 1, StartColumn: 3, EndRow: 3, EndColumn: 3}, Type: document.ConditionalFormatDuplicate},
		{Sheet: "Sheet1", Range: document.CellRange{StartRow: 1, StartColumn: 4, EndRow: 3, EndColumn: 4}, Type: document.ConditionalFormatAboveAvg},
	} {
		if changed, err := book.SetConditionalFormat(spec); err != nil || !changed {
			t.Fatalf("set %s conditional format changed=%t, err=%v", spec.Type, changed, err)
		}
	}
	window, err := book.Window("Sheet1", 1, 1, 3, 4)
	if err != nil {
		t.Fatal(err)
	}

	for _, scale := range []int{1, 2} {
		t.Run(map[int]string{1: "1x", 2: "2x"}[scale], func(t *testing.T) {
			const cellWidthDP, cellHeightDP = 120, 36
			width, height := 4*cellWidthDP*scale, 3*cellHeightDP*scale
			size := image.Pt(width, height)
			var ops op.Ops
			metric := unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}
			gtx := layout.Context{Ops: &ops, Constraints: layout.Exact(size), Metric: metric}
			theme := material.NewTheme()
			for row := range window.Cells {
				for column, cell := range window.Cells[row] {
					rect := image.Rect(column*cellWidthDP*scale, row*cellHeightDP*scale, (column+1)*cellWidthDP*scale, (row+1)*cellHeightDP*scale)
					paintStyledCell(gtx, theme, cell, rect, paintCellStyle(cell), false)
				}
			}
			renderer, err := headless.NewWindow(width, height)
			if err != nil {
				t.Skipf("headless GPU is unavailable: %v", err)
			}
			defer renderer.Release()
			if err := renderer.Frame(&ops); err != nil {
				t.Fatalf("render conditional-format worksheet: %v", err)
			}
			shot := image.NewRGBA(image.Rect(0, 0, width, height))
			if err := renderer.Screenshot(shot); err != nil {
				t.Fatalf("capture conditional-format worksheet: %v", err)
			}
			pixel := func(row, column, xDP, yDP int) color.NRGBA {
				return color.NRGBAModel.Convert(shot.At((column*cellWidthDP+xDP)*scale, (row*cellHeightDP+yDP)*scale)).(color.NRGBA)
			}
			wantScaleColors := [...]color.NRGBA{
				{R: 248, G: 105, B: 107, A: 255},
				{R: 255, G: 235, B: 132, A: 255},
				{R: 99, G: 190, B: 123, A: 255},
			}
			for row, want := range wantScaleColors {
				if got := pixel(row, 1, 112, 4); got != want {
					t.Errorf("three-color scale pixel at B%d = %v; want %v", row+1, got, want)
				}
			}
			duplicateFill := color.NRGBA{R: 255, G: 199, B: 206, A: 255}
			for _, row := range []int{0, 1} {
				if got := pixel(row, 2, 112, 4); got != duplicateFill {
					t.Errorf("duplicate-value fill at C%d = %v; want %v", row+1, got, duplicateFill)
				}
			}
			if got := pixel(2, 2, 112, 4); got == duplicateFill {
				t.Errorf("unique value C3 unexpectedly has duplicate-value fill %v", got)
			}
			aboveAverageFill := color.NRGBA{R: 198, G: 239, B: 206, A: 255}
			if got := pixel(2, 3, 112, 4); got != aboveAverageFill {
				t.Errorf("above-average fill at D3 = %v; want %v", got, aboveAverageFill)
			}
			for _, row := range []int{0, 1} {
				if got := pixel(row, 3, 112, 4); got == aboveAverageFill {
					t.Errorf("value at D%d is not above average but has fill %v", row+1, got)
				}
			}
			blank := pixel(1, 0, 90, 4)
			if got := pixel(0, 0, 10, 4); got == blank || pixel(0, 0, 90, 4) != blank {
				t.Errorf("negative data bar does not stop at the zero axis: inside=%v outside=%v blank=%v", pixel(0, 0, 10, 4), pixel(0, 0, 90, 4), blank)
			}
			if got := pixel(2, 0, 90, 4); got == blank || pixel(2, 0, 10, 4) != blank {
				t.Errorf("positive data bar does not start at the zero axis: inside=%v outside=%v blank=%v", pixel(2, 0, 90, 4), pixel(2, 0, 10, 4), blank)
			}
		})
	}
}
