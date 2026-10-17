package xlsx

import (
	"testing"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

func TestFillDownChangedRepeatsTopRowValuesFormulasAndStylesAsOneUndo(t *testing.T) {
	book := NewBlank()
	defer book.Close()
	sheet := book.SheetNames()[0]
	for cell, value := range map[string]string{"A1": "10", "C1": "label", "D1": "5", "A2": "old-a2", "B2": "old-b2", "C2": "old-c2", "A3": "old-a3", "B3": "old-b3", "C3": "old-c3"} {
		if err := book.SetCellText(sheet, cell, value); err != nil {
			t.Fatalf("SetCellText(%s): %v", cell, err)
		}
	}
	if err := book.SetCellFormula(sheet, "B1", "=A1*2+$D$1"); err != nil {
		t.Fatalf("SetCellFormula(B1): %v", err)
	}
	bold, italic, fill := true, true, "#FFF2CC"
	for _, formatted := range []struct {
		cell   string
		bounds document.CellRange
		patch  document.CellFormatPatch
	}{
		{"A1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, document.CellFormatPatch{Bold: &bold}},
		{"B1", document.CellRange{StartRow: 1, StartColumn: 2, EndRow: 1, EndColumn: 2}, document.CellFormatPatch{Italic: &italic}},
		{"C1", document.CellRange{StartRow: 1, StartColumn: 3, EndRow: 1, EndColumn: 3}, document.CellFormatPatch{FillColor: &fill}},
	} {
		if err := book.SetCellFormats(sheet, formatted.bounds, formatted.patch); err != nil {
			t.Fatalf("SetCellFormats(%s): %v", formatted.cell, err)
		}
	}
	source, err := book.ReadRange(sheet, document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 3})
	if err != nil {
		t.Fatalf("ReadRange(source row): %v", err)
	}
	filler, ok := any(book).(interface {
		FillDownChanged(string, document.CellRange) (bool, error)
	})
	if !ok {
		t.Fatal("XLSX workbook does not implement bounded fill down")
	}
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 3}
	changed, err := filler.FillDownChanged(sheet, bounds)
	if err != nil || !changed {
		t.Fatalf("FillDownChanged changed=%t, err=%v; want true, nil", changed, err)
	}
	for _, cell := range []struct{ address, value, formula string }{
		{"A1", "10", ""}, {"A2", "10", ""}, {"A3", "10", ""},
		{"B1", "", "A1*2+$D$1"}, {"B2", "", "A2*2+$D$1"}, {"B3", "", "A3*2+$D$1"},
		{"C1", "label", ""}, {"C2", "label", ""}, {"C3", "label", ""},
	} {
		if cell.formula == "" {
			if got, err := book.Value(sheet, cell.address); err != nil || got != cell.value {
				t.Errorf("%s value after fill down = %q, %v; want %q", cell.address, got, err, cell.value)
			}
		} else if got, err := book.GetCellFormula(sheet, cell.address); err != nil || got != cell.formula {
			t.Errorf("%s formula after fill down = %q, %v; want %q", cell.address, got, err, cell.formula)
		}
	}
	filled, err := book.ReadRange(sheet, bounds)
	if err != nil {
		t.Fatalf("ReadRange(filled range): %v", err)
	}
	for row := 1; row < len(filled); row++ {
		for column := range source[0] {
			if got, want := filled[row][column].StyleID, source[0][column].StyleID; got != want {
				t.Errorf("style at row %d column %d = %d; want copied top-row style %d", row+1, column+1, got, want)
			}
		}
	}
	if changed, err := filler.FillDownChanged(sheet, bounds); err != nil || changed {
		t.Fatalf("repeated FillDownChanged changed=%t, err=%v; want no-op", changed, err)
	}
	if changed, err := book.Undo(); err != nil || !changed {
		t.Fatalf("Undo fill down = changed %t, err %v; want one undoable batch", changed, err)
	}
	for address, want := range map[string]string{"A2": "old-a2", "B2": "old-b2", "C2": "old-c2", "A3": "old-a3", "B3": "old-b3", "C3": "old-c3"} {
		if got, err := book.Value(sheet, address); err != nil || got != want {
			t.Errorf("%s after undo = %q, %v; want %q", address, got, err, want)
		}
	}
}

func TestFillDownRejectsUnsafeFormulaTranslationWithoutMutation(t *testing.T) {
	book := NewBlank()
	defer book.Close()
	if err := book.SetCellFormula("Sheet1", "B1", "=A1048576"); err != nil {
		t.Fatalf("SetCellFormula(B1): %v", err)
	}
	if err := book.SetCellText("Sheet1", "B2", "keep"); err != nil {
		t.Fatalf("SetCellText(B2): %v", err)
	}
	book.RebaseHistory()
	historyBefore := len(book.undo)
	changed, err := book.FillDownChanged("Sheet1", document.CellRange{StartRow: 1, StartColumn: 2, EndRow: 2, EndColumn: 2})
	if err == nil || changed {
		t.Fatalf("out-of-bounds fill changed=%v err=%v; want atomic rejection", changed, err)
	}
	if got, err := book.GetCellFormula("Sheet1", "B1"); err != nil || got != "A1048576" {
		t.Errorf("source formula after rejected fill=%q, %v; want unchanged", got, err)
	}
	if got, err := book.Value("Sheet1", "B2"); err != nil || got != "keep" {
		t.Errorf("destination after rejected fill=%q, %v; want unchanged", got, err)
	}
	if len(book.undo) != historyBefore {
		t.Errorf("rejected fill changed undo history from %d to %d entries", historyBefore, len(book.undo))
	}
}

func TestFillDownRejectsMergedAndFormulaGroupRanges(t *testing.T) {
	t.Run("merged cells", func(t *testing.T) {
		book := NewBlank()
		defer book.Close()
		if err := book.file.MergeCell("Sheet1", "A1", "B2"); err != nil {
			t.Fatal(err)
		}
		if changed, err := book.FillDownChanged("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 2}); err == nil || changed {
			t.Fatalf("fill into merged range changed=%v err=%v; want rejection", changed, err)
		}
	})
	t.Run("array formula group", func(t *testing.T) {
		book := NewBlank()
		defer book.Close()
		kind, formulaRange := excelize.STCellFormulaTypeArray, "A1:A2"
		if err := book.file.SetCellFormula("Sheet1", "A1", "ROW(A1:A2)", excelize.FormulaOpts{Type: &kind, Ref: &formulaRange}); err != nil {
			t.Fatal(err)
		}
		if changed, err := book.FillDownChanged("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 1}); err == nil || changed {
			t.Fatalf("fill into array formula group changed=%v err=%v; want rejection", changed, err)
		}
	})
}
