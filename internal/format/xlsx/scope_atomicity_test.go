package xlsx

import (
	"bytes"
	"glance/internal/document"
	"testing"
)

func TestScopedClearNoOpPreservesPackage(t *testing.T) {
	w := NewBlank()
	defer w.Close()
	before, err := w.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	changed, err := w.SetCellFormatsChanged("Sheet1", document.CellRange{StartRow: 1, EndRow: 1, StartColumn: 1, EndColumn: maxXLSXColumns}, document.CellFormatPatch{ClearFormat: true})
	if err != nil || changed {
		t.Fatalf("clear blank row = %v,%v; want no-op", changed, err)
	}
	after, err := w.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("no-op clear added unused style data to the package")
	}
}

func TestScopedFormatFailureRestoresPackage(t *testing.T) {
	w := NewBlank()
	defer w.Close()
	snapshot, err := w.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	files, target, err := scopePackage(snapshot, "Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	root, err := parseScopeXML(files[target])
	if err != nil {
		t.Fatal(err)
	}
	row := newScopeNode("row")
	row.set("r", "2")
	row.set("s", "999999")
	row.set("customFormat", "1")
	root.child("worksheet").child("sheetData").parts = append(root.child("worksheet").child("sheetData").parts, scopePart{node: row})
	files[target] = root.bytes()
	snapshot, err = replaceScopePackage(snapshot, files)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.restoreSnapshotLocked(snapshot); err != nil {
		t.Fatal(err)
	}
	before, err := w.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	family := "Scoped Atomic Font"
	if _, err = w.SetCellFormatsChanged("Sheet1", document.CellRange{StartRow: 1, EndRow: 2, StartColumn: 1, EndColumn: maxXLSXColumns}, document.CellFormatPatch{FontFamily: &family}); err == nil {
		t.Fatal("invalid source style should fail")
	}
	after, err := w.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed scoped format left newly generated styles in the package")
	}
}
