package xlsx

import (
	"github.com/xuri/excelize/v2"
	"glance/internal/document"
	"path/filepath"
	"testing"
)

func TestNoFillMetadataDoesNotBecomeBlack(t *testing.T) {
	for _, fill := range []excelize.Fill{
		{Type: "pattern", Pattern: 0, Color: []string{"000000"}},
		{Type: "pattern", Pattern: 0, Color: []string{"FFFFFF"}},
	} {
		if got := neutralCellStyle(&excelize.Style{Fill: fill}).FillColor; got != "" {
			t.Errorf("no fill = %q; want empty", got)
		}
	}
}

func TestWrapAndFontEditsPreserveNoFillAfterReopen(t *testing.T) {
	book := NewBlank()
	defer book.Close()
	sheet := book.SheetNames()[0]
	id, err := book.file.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Pattern: 0, Color: []string{"000000"}}, Alignment: &excelize.Alignment{WrapText: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err = book.file.SetCellStyle(sheet, "A1", "A1", id); err != nil {
		t.Fatal(err)
	}
	if err = book.SetCellText(sheet, "A1", "原本自动换行的中文文字"); err != nil {
		t.Fatal(err)
	}
	bold := true
	if err = book.SetCellTextSelectionFormat(sheet, "A1", "原本自动换行的中文文字", 2, 6, document.CellFormatPatch{Bold: &bold}); err != nil {
		t.Fatal(err)
	}
	size := 18.0
	wrap := false
	for _, patch := range []document.CellFormatPatch{{FontSize: &size}, {WrapText: &wrap}} {
		if err = book.SetCellFormats(sheet, document.CellRange{StartRow: 1, EndRow: 1, StartColumn: 1, EndColumn: 1}, patch); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "no-fill.xlsx")
	if err = book.file.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	window, err := reopened.Window(sheet, 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if style := window.Cells[0][0].Style; style.FillColor != "" || style.WrapText {
		t.Fatalf("style after reopen = %+v", style)
	}
}

func TestVisibleSolidBlackFillSurvivesWrapEdit(t *testing.T) {
	book := NewBlank()
	defer book.Close()
	sheet := book.SheetNames()[0]
	fill := "#000000"
	bounds := document.CellRange{StartRow: 1, EndRow: 1, StartColumn: 1, EndColumn: 1}
	if err := book.SetCellFormats(sheet, bounds, document.CellFormatPatch{FillColor: &fill}); err != nil {
		t.Fatal(err)
	}
	wrap := true
	if err := book.SetCellFormats(sheet, bounds, document.CellFormatPatch{WrapText: &wrap}); err != nil {
		t.Fatal(err)
	}
	window, err := book.Window(sheet, 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if style := window.Cells[0][0].Style; style.FillColor != fill || !style.WrapText {
		t.Fatalf("real black fill changed: %+v", style)
	}
}
