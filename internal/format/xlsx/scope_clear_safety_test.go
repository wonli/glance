package xlsx

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

func TestClearRangePreservesCalculationChainSheetInheritance(t *testing.T) {
	w := NewBlank()
	defer w.Close()
	if _, err := w.file.NewSheet("Other"); err != nil {
		t.Fatal(err)
	}
	for _, cell := range []string{"B1", "B2"} {
		w.file.SetCellFormula("Sheet1", cell, "1+1")
	}
	w.file.SetCellFormula("Other", "B2", "2+2")
	w.file.SetCellFormula("Other", "C2", "3+3")
	w.file.Pkg.Store("xl/calcChain.xml", []byte(`<calcChain xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><c r="B2" i="2"/><c r="B1" i="1"/><c r="B2"/><c r="C2" i="2"/></calcChain>`))
	w.RebaseHistory()
	if changed, err := w.ClearCellRange("Sheet1", document.CellRange{StartRow: 1, EndRow: 1, StartColumn: 1, EndColumn: 16384}); err != nil || !changed {
		t.Fatalf("clear=%v err=%v", changed, err)
	}
	snapshot, err := w.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := scopePackage(snapshot, "Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	root, err := parseScopeXML(files["xl/calcChain.xml"])
	if err != nil {
		t.Fatal(err)
	}
	nodes := root.child("calcChain").children("c")
	if len(nodes) != 3 {
		t.Fatalf("chain retains deleted entry: %s", files["xl/calcChain.xml"])
	}
	for i, want := range []struct{ ref, sheet string }{{"B2", "2"}, {"B2", "1"}, {"C2", "2"}} {
		if nodes[i].attrs["r"] != want.ref || nodes[i].attrs["i"] != want.sheet {
			t.Fatalf("chain entry %d=%v want %+v", i, nodes[i].attrs, want)
		}
	}
	if ok, err := w.Undo(); err != nil || !ok {
		t.Fatalf("undo=%v err=%v", ok, err)
	}
	snapshot, _ = w.Snapshot()
	files, _, _ = scopePackage(snapshot, "Sheet1")
	if !bytes.Contains(files["xl/calcChain.xml"], []byte(`r="B1"`)) {
		t.Fatal("undo did not restore chain")
	}
}

func TestClearRangeRemovesEmptyCalculationChain(t *testing.T) {
	w := NewBlank()
	defer w.Close()
	w.file.SetCellFormula("Sheet1", "B1", "1+1")
	w.file.Pkg.Store("xl/calcChain.xml", []byte(`<calcChain xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><c r="B1" i="1"/></calcChain>`))
	seed, err := w.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	parts, _, err := scopePackage(seed, "Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	parts["xl/_rels/workbook.xml.rels"] = bytes.Replace(parts["xl/_rels/workbook.xml.rels"], []byte(`</Relationships>`), []byte(`<Relationship Id="rIdCalcChain" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/calcChain" Target="calcChain.xml"/></Relationships>`), 1)
	parts["[Content_Types].xml"] = bytes.Replace(parts["[Content_Types].xml"], []byte(`</Types>`), []byte(`<Override PartName="/xl/calcChain.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.calcChain+xml"/></Types>`), 1)
	seed, err = replaceScopePackage(seed, parts)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.restoreSnapshotLocked(seed); err != nil {
		t.Fatal(err)
	}
	if changed, err := w.ClearCellRange("Sheet1", document.CellRange{StartRow: 1, EndRow: 1, StartColumn: 1, EndColumn: 16384}); err != nil || !changed {
		t.Fatalf("clear=%v err=%v", changed, err)
	}
	snapshot, _ := w.Snapshot()
	files, _, err := scopePackage(snapshot, "Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := files["xl/calcChain.xml"]; exists {
		t.Fatal("empty calculation chain remains in package")
	}
	if bytes.Contains(files["xl/_rels/workbook.xml.rels"], []byte("/calcChain")) || bytes.Contains(files["[Content_Types].xml"], []byte("calcChain.xml")) {
		t.Fatal("removed calculation chain retains relationship or content-type declaration")
	}
}

func TestClearRangeSharedAnchorSurvivesSaveAndHistory(t *testing.T) {
	w := NewBlank()
	defer w.Close()
	kind, ref := excelize.STCellFormulaTypeShared, "B1:B3"
	if err := w.file.SetCellFormula("Sheet1", "B1", "A1+1", excelize.FormulaOpts{Type: &kind, Ref: &ref}); err != nil {
		t.Fatal(err)
	}
	w.RebaseHistory()
	if changed, err := w.ClearCellRange("Sheet1", document.CellRange{StartRow: 1, EndRow: 1, StartColumn: 1, EndColumn: 16384}); err != nil || !changed {
		t.Fatalf("clear=%v err=%v", changed, err)
	}
	path := filepath.Join(t.TempDir(), "shared.xlsx")
	if err := w.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for cell, want := range map[string]string{"B1": "", "B2": "A2+1", "B3": "A3+1"} {
		if got, err := r.file.GetCellFormula("Sheet1", cell); err != nil || got != want {
			t.Fatalf("saved %s=%q want %q err=%v", cell, got, want, err)
		}
	}
	if ok, err := w.Undo(); err != nil || !ok {
		t.Fatalf("undo=%v err=%v", ok, err)
	}
	if got, _ := w.file.GetCellFormula("Sheet1", "B1"); got != "A1+1" {
		t.Fatalf("undo B1=%q", got)
	}
	if ok, err := w.Redo(); err != nil || !ok {
		t.Fatalf("redo=%v err=%v", ok, err)
	}
	if got, _ := w.file.GetCellFormula("Sheet1", "B3"); got != "A3+1" {
		t.Fatalf("redo B3=%q", got)
	}
}

func TestClearRangeRejectsPartialArrayAtomically(t *testing.T) {
	for _, row := range []int{2, 3} {
		t.Run(CellName(2, row), func(t *testing.T) {
			w := NewBlank()
			defer w.Close()
			kind, ref := excelize.STCellFormulaTypeArray, "B2:B4"
			if err := w.file.SetCellFormula("Sheet1", "B2", "A2:A4+1", excelize.FormulaOpts{Type: &kind, Ref: &ref}); err != nil {
				t.Fatal(err)
			}
			w.file.SetCellValue("Sheet1", CellName(1, row), "keep")
			w.RebaseHistory()
			changed, err := w.ClearCellRange("Sheet1", document.CellRange{StartRow: row, EndRow: row, StartColumn: 1, EndColumn: 16384})
			if err == nil || changed {
				t.Fatalf("partial array clear=%v err=%v", changed, err)
			}
			if got, _ := w.Value("Sheet1", CellName(1, row)); got != "keep" {
				t.Fatalf("other cell changed=%q", got)
			}
			if got, _ := w.file.GetCellFormula("Sheet1", "B2"); got != "A2:A4+1" {
				t.Fatalf("array changed=%q", got)
			}
			if w.Dirty() || len(w.undo) != 0 {
				t.Fatal("rejected clear changed dirty/history")
			}
		})
	}
}

func TestClearRangeAllowsWholeArray(t *testing.T) {
	w := NewBlank()
	defer w.Close()
	kind, ref := excelize.STCellFormulaTypeArray, "B2:B4"
	if err := w.file.SetCellFormula("Sheet1", "B2", "A2:A4+1", excelize.FormulaOpts{Type: &kind, Ref: &ref}); err != nil {
		t.Fatal(err)
	}
	if changed, err := w.ClearCellRange("Sheet1", document.CellRange{StartRow: 1, EndRow: 1048576, StartColumn: 2, EndColumn: 2}); err != nil || !changed {
		t.Fatalf("whole array clear=%v err=%v", changed, err)
	}
}
