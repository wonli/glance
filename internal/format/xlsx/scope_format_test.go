package xlsx

import (
	"github.com/xuri/excelize/v2"
	"glance/internal/document"
	"path/filepath"
	"testing"
)

func TestScopedFormattingDefaultsAndHistory(t *testing.T) {
	for _, tc := range []struct {
		name           string
		bounds         document.CellRange
		blank, outside string
	}{
		{"row", document.CellRange{StartRow: 3, EndRow: 3, StartColumn: 1, EndColumn: 16384}, "Z3", "Z4"},
		{"column", document.CellRange{StartRow: 1, EndRow: 1048576, StartColumn: 2, EndColumn: 2}, "B900000", "C900000"},
		{"sheet", document.CellRange{StartRow: 1, EndRow: 1048576, StartColumn: 1, EndColumn: 16384}, "Z900000", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := NewBlank()
			defer w.Close()
			w.file.SetCellValue("Sheet1", "B3", "keep")
			id, _ := w.file.NewStyle(&excelize.Style{Font: &excelize.Font{Italic: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FF0000"}}})
			w.file.SetCellStyle("Sheet1", "B3", "B3", id)
			w.RebaseHistory()
			family := "Synthetic Scoped Font"
			changed, err := w.SetCellFormatsChanged("Sheet1", tc.bounds, document.CellFormatPatch{FontFamily: &family})
			if err != nil || !changed {
				t.Fatalf("format changed=%v err=%v", changed, err)
			}
			if len(w.undo) != 1 {
				t.Fatalf("undo entries=%d", len(w.undo))
			}
			path := filepath.Join(t.TempDir(), "scope.xlsx")
			if err = w.SaveAs(path); err != nil {
				t.Fatal(err)
			}
			r, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			id, err = r.file.GetCellStyle("Sheet1", tc.blank)
			if err != nil {
				t.Fatal(err)
			}
			style, _ := r.file.GetStyle(id)
			if style.Font == nil || style.Font.Family != family {
				t.Fatalf("blank inherited style=%+v", style.Font)
			}
			id, _ = r.file.GetCellStyle("Sheet1", "B3")
			style, _ = r.file.GetStyle(id)
			if style.Font == nil || !style.Font.Italic || style.Font.Family != family || style.Fill.Pattern != 1 {
				t.Fatalf("existing format lost %+v", style)
			}
			if tc.outside != "" {
				id, _ = r.file.GetCellStyle("Sheet1", tc.outside)
				style, _ = r.file.GetStyle(id)
				if style.Font != nil && style.Font.Family == family {
					t.Fatal("outside changed")
				}
			}
			changed, err = w.SetCellFormatsChanged("Sheet1", tc.bounds, document.CellFormatPatch{FontFamily: &family})
			if err != nil || changed {
				t.Fatalf("repeat changed=%v err=%v", changed, err)
			}
			if ok, err := w.Undo(); err != nil || !ok || w.Dirty() {
				t.Fatalf("undo=%v dirty=%v err=%v", ok, w.Dirty(), err)
			}
		})
	}
}

func TestClearCellRangeSparsePreservesStyleAndUndo(t *testing.T) {
	w := NewBlank()
	defer w.Close()
	w.file.SetCellValue("Sheet1", "A1", "remove")
	w.file.SetCellFormula("Sheet1", "B900000", "1+2")
	w.file.SetCellValue("Sheet1", "C1", "outside")
	id, _ := w.file.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	w.file.SetCellStyle("Sheet1", "A1", "A1", id)
	w.RebaseHistory()
	editor, ok := any(w).(interface {
		ClearCellRange(string, document.CellRange) (bool, error)
	})
	if !ok {
		t.Fatal("missing sparse range clear capability")
	}
	bounds := document.CellRange{StartRow: 1, EndRow: 1048576, StartColumn: 1, EndColumn: 2}
	changed, err := editor.ClearCellRange("Sheet1", bounds)
	if err != nil || !changed {
		t.Fatalf("clear=%v err=%v", changed, err)
	}
	if v, _ := w.Value("Sheet1", "A1"); v != "" {
		t.Fatal(v)
	}
	if f, _ := w.file.GetCellFormula("Sheet1", "B900000"); f != "" {
		t.Fatal(f)
	}
	got, _ := w.file.GetCellStyle("Sheet1", "A1")
	if got != id {
		t.Fatal("style lost")
	}
	if v, _ := w.Value("Sheet1", "C1"); v != "outside" {
		t.Fatal(v)
	}
	if changed, err = editor.ClearCellRange("Sheet1", bounds); err != nil || changed {
		t.Fatalf("repeat=%v err=%v", changed, err)
	}
	if len(w.undo) != 1 {
		t.Fatal(len(w.undo))
	}
	w.Undo()
	if v, _ := w.Value("Sheet1", "A1"); v != "remove" {
		t.Fatal(v)
	}
}

func TestScopedFormattingPreservesCrossAxisDefaults(t *testing.T) {
	w := NewBlank()
	defer w.Close()
	id, _ := w.file.NewStyle(&excelize.Style{Font: &excelize.Font{Italic: true}})
	w.file.SetColStyle("Sheet1", "D:D", id)
	family := "Cross Axis Font"
	row := document.CellRange{StartRow: 3, EndRow: 3, StartColumn: 1, EndColumn: 16384}
	if _, err := w.SetCellFormatsChanged("Sheet1", row, document.CellFormatPatch{FontFamily: &family}); err != nil {
		t.Fatal(err)
	}
	id, _ = w.file.GetCellStyle("Sheet1", "D3")
	style, _ := w.file.GetStyle(id)
	if style.Font == nil || !style.Font.Italic || style.Font.Family != family {
		t.Fatalf("column inheritance lost: %+v", style.Font)
	}
	bold := true
	col := document.CellRange{StartRow: 1, EndRow: 1048576, StartColumn: 5, EndColumn: 5}
	if _, err := w.SetCellFormatsChanged("Sheet1", col, document.CellFormatPatch{Bold: &bold}); err != nil {
		t.Fatal(err)
	}
	id, _ = w.file.GetCellStyle("Sheet1", "E3")
	style, _ = w.file.GetStyle(id)
	if style.Font == nil || !style.Font.Bold || style.Font.Family != family {
		t.Fatalf("row inheritance lost: %+v", style.Font)
	}
	if _, err := w.SetCellFormatsChanged("Sheet1", row, document.CellFormatPatch{ClearFormat: true}); err != nil {
		t.Fatal(err)
	}
	id, _ = w.file.GetCellStyle("Sheet1", "D3")
	style, _ = w.file.GetStyle(id)
	if style.Font != nil && (style.Font.Italic || style.Font.Family == family) {
		t.Fatalf("row clear inherited column style: %+v", style.Font)
	}
}

func TestClearCellRangePreservesSharedFormulasOutsideSelection(t *testing.T) {
	w := NewBlank()
	defer w.Close()
	kind, ref := excelize.STCellFormulaTypeShared, "B1:B3"
	if err := w.file.SetCellFormula("Sheet1", "B1", "A1+1", excelize.FormulaOpts{Type: &kind, Ref: &ref}); err != nil {
		t.Fatal(err)
	}
	editor := any(w).(document.SpreadsheetRangeContentEditor)
	if _, err := editor.ClearCellRange("Sheet1", document.CellRange{StartRow: 1, EndRow: 1, StartColumn: 1, EndColumn: 16384}); err != nil {
		t.Fatal(err)
	}
	f, err := w.file.GetCellFormula("Sheet1", "B2")
	if err != nil || f != "A2+1" {
		t.Fatalf("outside shared formula=%q err=%v", f, err)
	}
}
