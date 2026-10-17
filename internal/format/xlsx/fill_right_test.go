package xlsx

import (
	"testing"

	"glance/internal/document"
)

func TestFillRightChangedRepeatsLeftColumnValuesFormulasAndStylesAsOneUndo(t *testing.T) {
	book := NewBlank()
	defer book.Close()
	sheet := book.SheetNames()[0]
	for cell, value := range map[string]string{"A1": "10", "A2": "label", "B1": "old-b1", "C1": "old-c1", "B2": "old-b2", "C2": "old-c2"} {
		if err := book.SetCellText(sheet, cell, value); err != nil {
			t.Fatalf("SetCellText(%s): %v", cell, err)
		}
	}
	if err := book.SetCellFormula(sheet, "A1", "=B1*2+$D$1"); err != nil {
		t.Fatalf("SetCellFormula(A1): %v", err)
	}
	bold, fill := true, "#FFF2CC"
	for _, formatted := range []struct {
		cell   string
		bounds document.CellRange
		patch  document.CellFormatPatch
	}{
		{"A1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, document.CellFormatPatch{Bold: &bold}},
		{"A2", document.CellRange{StartRow: 2, StartColumn: 1, EndRow: 2, EndColumn: 1}, document.CellFormatPatch{FillColor: &fill}},
	} {
		if err := book.SetCellFormats(sheet, formatted.bounds, formatted.patch); err != nil {
			t.Fatalf("SetCellFormats(%s): %v", formatted.cell, err)
		}
	}
	source, err := book.ReadRange(sheet, document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 1})
	if err != nil {
		t.Fatalf("ReadRange(source column): %v", err)
	}
	filler, ok := any(book).(interface {
		FillRightChanged(string, document.CellRange) (bool, error)
	})
	if !ok {
		t.Fatal("XLSX workbook does not implement bounded fill right")
	}
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 3}
	changed, err := filler.FillRightChanged(sheet, bounds)
	if err != nil || !changed {
		t.Fatalf("FillRightChanged changed=%t, err=%v; want true, nil", changed, err)
	}
	for _, cell := range []struct{ address, value, formula string }{
		{"A1", "", "B1*2+$D$1"}, {"B1", "", "C1*2+$D$1"}, {"C1", "", "D1*2+$D$1"},
		{"A2", "label", ""}, {"B2", "label", ""}, {"C2", "label", ""},
	} {
		if cell.formula == "" {
			if got, err := book.Value(sheet, cell.address); err != nil || got != cell.value {
				t.Errorf("%s value after fill right = %q, %v; want %q", cell.address, got, err, cell.value)
			}
		} else if got, err := book.GetCellFormula(sheet, cell.address); err != nil || got != cell.formula {
			t.Errorf("%s formula after fill right = %q, %v; want %q", cell.address, got, err, cell.formula)
		}
	}
	fillerRange, err := book.ReadRange(sheet, bounds)
	if err != nil {
		t.Fatalf("ReadRange(filled range): %v", err)
	}
	for row := range source {
		for column := 1; column < len(fillerRange[row]); column++ {
			if got, want := fillerRange[row][column].StyleID, source[row][0].StyleID; got != want {
				t.Errorf("style at row %d column %d = %d; want copied left-column style %d", row+1, column+1, got, want)
			}
		}
	}
	snapshot, err := book.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot after fill right: %v", err)
	}
	reopened, err := OpenSnapshot(snapshot)
	if err != nil {
		t.Fatalf("OpenSnapshot after fill right: %v", err)
	}
	defer reopened.Close()
	for address, want := range map[string]string{"A1": "B1*2+$D$1", "B1": "C1*2+$D$1", "C1": "D1*2+$D$1"} {
		if got, err := reopened.GetCellFormula(sheet, address); err != nil || got != want {
			t.Errorf("%s formula after snapshot reopen = %q, %v; want %q", address, got, err, want)
		}
	}
	for _, cell := range []struct {
		address string
		row     int
	}{
		{"B1", 0}, {"C1", 0}, {"B2", 1}, {"C2", 1},
	} {
		styleID, err := reopened.file.GetCellStyle(sheet, cell.address)
		if err != nil || styleID != source[cell.row][0].StyleID {
			t.Errorf("%s style after snapshot reopen = %d, %v; want %d", cell.address, styleID, err, source[cell.row][0].StyleID)
		}
	}
	if changed, err := filler.FillRightChanged(sheet, bounds); err != nil || changed {
		t.Fatalf("repeated FillRightChanged changed=%t, err=%v; want no-op", changed, err)
	}
	if changed, err := book.Undo(); err != nil || !changed {
		t.Fatalf("Undo fill right = changed %t, err %v; want one undoable batch", changed, err)
	}
	for address, want := range map[string]string{"B1": "old-b1", "C1": "old-c1", "B2": "old-b2", "C2": "old-c2"} {
		if got, err := book.Value(sheet, address); err != nil || got != want {
			t.Errorf("%s after undo = %q, %v; want %q", address, got, err, want)
		}
	}
}

func TestFillRightRejectsUnsafeFormulaTranslationAndMergedRanges(t *testing.T) {
	t.Run("formula outside worksheet", func(t *testing.T) {
		book := NewBlank()
		defer book.Close()
		if err := book.SetCellFormula("Sheet1", "A1", "=XFD1"); err != nil {
			t.Fatalf("SetCellFormula(A1): %v", err)
		}
		if err := book.SetCellText("Sheet1", "B1", "keep"); err != nil {
			t.Fatalf("SetCellText(B1): %v", err)
		}
		book.RebaseHistory()
		filler, ok := any(book).(interface {
			FillRightChanged(string, document.CellRange) (bool, error)
		})
		if !ok {
			t.Fatal("XLSX workbook does not implement bounded fill right")
		}
		changed, err := filler.FillRightChanged("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2})
		if err == nil || changed {
			t.Fatalf("out-of-bounds fill changed=%v err=%v; want atomic rejection", changed, err)
		}
		if got, err := book.Value("Sheet1", "B1"); err != nil || got != "keep" {
			t.Errorf("destination after rejected fill = %q, %v; want unchanged", got, err)
		}
	})
	t.Run("merged cells", func(t *testing.T) {
		book := NewBlank()
		defer book.Close()
		if err := book.file.MergeCell("Sheet1", "A1", "B2"); err != nil {
			t.Fatal(err)
		}
		filler, ok := any(book).(interface {
			FillRightChanged(string, document.CellRange) (bool, error)
		})
		if !ok {
			t.Fatal("XLSX workbook does not implement bounded fill right")
		}
		if changed, err := filler.FillRightChanged("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 3}); err == nil || changed {
			t.Fatalf("fill into merged range changed=%v err=%v; want rejection", changed, err)
		}
	})
}
