package csv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestOpenAndReadCSV(t *testing.T) {
	content := "Name,Age,Active,Code\nAlice,30,true,007\nBob,25.5,false,0123\n"
	tmp := filepath.Join(t.TempDir(), "test.csv")
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	doc, err := Open(tmp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	if doc.Dirty() {
		t.Errorf("expected clean document upon open")
	}

	sheets := doc.SheetNames()
	if len(sheets) == 0 {
		t.Fatalf("expected at least one sheet")
	}

	window, err := doc.Window(sheets[0], 1, 1, 3, 4)
	if err != nil {
		t.Fatalf("Window: %v", err)
	}

	if len(window.Cells) < 3 || len(window.Cells[0]) < 4 {
		t.Fatalf("unexpected window size: %d x %d", len(window.Cells), len(window.Cells[0]))
	}

	// Header row
	if window.Cells[0][0].Value != "Name" || window.Cells[0][1].Value != "Age" ||
		window.Cells[0][2].Value != "Active" || window.Cells[0][3].Value != "Code" {
		t.Errorf("unexpected header values: %v, %v, %v, %v",
			window.Cells[0][0].Value, window.Cells[0][1].Value, window.Cells[0][2].Value, window.Cells[0][3].Value)
	}

	// Alice row
	if window.Cells[1][0].Value != "Alice" || window.Cells[1][1].Value != "30" ||
		window.Cells[1][2].Value != "TRUE" || window.Cells[1][3].Value != "007" {
		t.Errorf("unexpected Alice values: %v, %v, %v, %v",
			window.Cells[1][0].Value, window.Cells[1][1].Value, window.Cells[1][2].Value, window.Cells[1][3].Value)
	}

	// Bob row (float + false + preserved leading zero)
	if window.Cells[2][0].Value != "Bob" || window.Cells[2][1].Value != "25.5" ||
		window.Cells[2][2].Value != "FALSE" || window.Cells[2][3].Value != "0123" {
		t.Errorf("unexpected Bob values: %v, %v, %v, %v",
			window.Cells[2][0].Value, window.Cells[2][1].Value, window.Cells[2][2].Value, window.Cells[2][3].Value)
	}
}

func TestCSVWithBOMAndCRLF(t *testing.T) {
	content := "\xef\xbb\xbfItem,Price\r\nApple,5\r\nBanana,3.5\r\n"
	tmp := filepath.Join(t.TempDir(), "bom.csv")
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	doc, err := Open(tmp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	if !doc.hasBOM {
		t.Errorf("expected hasBOM to be true")
	}
	if !doc.useCRLF {
		t.Errorf("expected useCRLF to be true")
	}

	// Modify a cell and save
	if err := doc.SetCellText(doc.SheetNames()[0], "B2", "6"); err != nil {
		t.Fatalf("SetCellText: %v", err)
	}
	if !doc.Dirty() {
		t.Errorf("expected document to be dirty after edit")
	}

	if err := doc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if doc.Dirty() {
		t.Errorf("expected document to be clean after save")
	}

	savedBytes, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}

	if !strings.HasPrefix(string(savedBytes), "\xef\xbb\xbf") {
		t.Errorf("saved file missing UTF-8 BOM")
	}
	if !strings.Contains(string(savedBytes), "\r\n") {
		t.Errorf("saved file missing CRLF line endings")
	}
}

func TestCSVDelimiters(t *testing.T) {
	// TSV test
	tsvContent := "Col1\tCol2\tCol3\nA\tB\tC\n"
	tmpTSV := filepath.Join(t.TempDir(), "tab.csv")
	if err := os.WriteFile(tmpTSV, []byte(tsvContent), 0o600); err != nil {
		t.Fatalf("write TSV: %v", err)
	}

	doc, err := Open(tmpTSV)
	if err != nil {
		t.Fatalf("Open TSV: %v", err)
	}
	defer doc.Close()

	if doc.comma != '\t' {
		t.Errorf("expected comma to be tab, got %q", doc.comma)
	}

	window, err := doc.Window(doc.SheetNames()[0], 1, 1, 2, 3)
	if err != nil {
		t.Fatalf("Window: %v", err)
	}
	if window.Cells[0][1].Value != "Col2" || window.Cells[1][2].Value != "C" {
		t.Errorf("unexpected cell values from TSV")
	}

	// Semicolon test
	semiContent := "A;B;C\n1;2;3\n"
	tmpSemi := filepath.Join(t.TempDir(), "semi.csv")
	if err := os.WriteFile(tmpSemi, []byte(semiContent), 0o600); err != nil {
		t.Fatalf("write semicolon CSV: %v", err)
	}

	docSemi, err := Open(tmpSemi)
	if err != nil {
		t.Fatalf("Open Semicolon CSV: %v", err)
	}
	defer docSemi.Close()

	if docSemi.comma != ';' {
		t.Errorf("expected comma to be ';', got %q", docSemi.comma)
	}
}

func TestCSVQuotingAndNewlines(t *testing.T) {
	content := "Title,Description\n\"Comma, Field\",\"Line1\nLine2\"\n\"Quote \"\"Inside\"\"\",Normal\n"
	tmp := filepath.Join(t.TempDir(), "quotes.csv")
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	doc, err := Open(tmp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	sheet := doc.SheetNames()[0]
	window, err := doc.Window(sheet, 1, 1, 3, 2)
	if err != nil {
		t.Fatalf("Window: %v", err)
	}

	if window.Cells[1][0].Value != "Comma, Field" {
		t.Errorf("got %q, want %q", window.Cells[1][0].Value, "Comma, Field")
	}
	if window.Cells[1][1].Value != "Line1\nLine2" {
		t.Errorf("got %q, want %q", window.Cells[1][1].Value, "Line1\nLine2")
	}
	if window.Cells[2][0].Value != "Quote \"Inside\"" {
		t.Errorf("got %q, want %q", window.Cells[2][0].Value, "Quote \"Inside\"")
	}

	// Save and reopen
	if err := doc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reopened, err := Open(tmp)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer reopened.Close()

	reWindow, err := reopened.Window(reopened.SheetNames()[0], 1, 1, 3, 2)
	if err != nil {
		t.Fatalf("Window on reopen: %v", err)
	}
	if reWindow.Cells[1][0].Value != "Comma, Field" || reWindow.Cells[1][1].Value != "Line1\nLine2" || reWindow.Cells[2][0].Value != "Quote \"Inside\"" {
		t.Errorf("round trip failed: %+v", reWindow.Cells)
	}
}

func TestEditAndSave(t *testing.T) {
	content := "A,B\n1,2\n"
	tmp := filepath.Join(t.TempDir(), "edit.csv")
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	doc, err := Open(tmp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	sheet := doc.SheetNames()[0]
	if err := doc.SetCellText(sheet, "B2", "200"); err != nil {
		t.Fatalf("SetCellText: %v", err)
	}
	if !doc.Dirty() {
		t.Errorf("expected dirty after edit")
	}

	if err := doc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if doc.Dirty() {
		t.Errorf("expected clean after save")
	}

	reopened, err := Open(tmp)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer reopened.Close()

	window, err := reopened.Window(sheet, 1, 1, 2, 2)
	if err != nil {
		t.Fatalf("Window: %v", err)
	}
	if window.Cells[1][1].Value != "200" {
		t.Errorf("got %q, want %q", window.Cells[1][1].Value, "200")
	}
}

func TestSaveAsXLSXAndCSV(t *testing.T) {
	content := "City,Population\nTokyo,37000000\nParis,2161000\n"
	tmpCSV := filepath.Join(t.TempDir(), "cities.csv")
	if err := os.WriteFile(tmpCSV, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	doc, err := Open(tmpCSV)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	// SaveAs another CSV
	copyCSV := filepath.Join(t.TempDir(), "cities_copy.csv")
	if err := doc.SaveAs(copyCSV); err != nil {
		t.Fatalf("SaveAs CSV: %v", err)
	}

	copyDoc, err := Open(copyCSV)
	if err != nil {
		t.Fatalf("Open copy CSV: %v", err)
	}
	defer copyDoc.Close()

	// SaveAs XLSX
	xlsxPath := filepath.Join(t.TempDir(), "cities.xlsx")
	if err := doc.SaveAs(xlsxPath); err != nil {
		t.Fatalf("SaveAs XLSX: %v", err)
	}

	xlsxDoc, err := xlsx.Open(xlsxPath)
	if err != nil {
		t.Fatalf("Open converted XLSX: %v", err)
	}
	defer xlsxDoc.Close()

	window, err := xlsxDoc.Window(xlsxDoc.SheetNames()[0], 1, 1, 3, 2)
	if err != nil {
		t.Fatalf("XLSX Window: %v", err)
	}
	if window.Cells[1][0].Value != "Tokyo" || window.Cells[2][1].Value != "2161000" {
		t.Errorf("unexpected XLSX content: %+v", window.Cells)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	doc := NewBlank()
	defer doc.Close()

	sheet := doc.SheetNames()[0]
	if err := doc.SetCellText(sheet, "A1", "SnapshotTest"); err != nil {
		t.Fatalf("SetCellText: %v", err)
	}
	if err := doc.SetCellText(sheet, "B1", "42"); err != nil {
		t.Fatalf("SetCellText: %v", err)
	}

	snap, err := doc.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	restored, err := OpenSnapshot(snap)
	if err != nil {
		t.Fatalf("OpenSnapshot: %v", err)
	}
	defer restored.Close()

	window, err := restored.Window(restored.SheetNames()[0], 1, 1, 1, 2)
	if err != nil {
		t.Fatalf("Window: %v", err)
	}
	if window.Cells[0][0].Value != "SnapshotTest" || window.Cells[0][1].Value != "42" {
		t.Errorf("snapshot restored unexpected values: %+v", window.Cells)
	}
}

func TestUndoRedo(t *testing.T) {
	content := "Value\n100\n"
	tmp := filepath.Join(t.TempDir(), "undo.csv")
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	doc, err := Open(tmp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	sheet := doc.SheetNames()[0]
	history, ok := any(doc).(document.SpreadsheetHistory)
	if !ok {
		t.Fatalf("doc does not implement SpreadsheetHistory")
	}

	if err := doc.SetCellText(sheet, "A2", "999"); err != nil {
		t.Fatalf("SetCellText: %v", err)
	}

	// Undo
	undoChanged, err := history.Undo()
	if err != nil || !undoChanged {
		t.Fatalf("Undo failed: changed=%v, err=%v", undoChanged, err)
	}

	window, err := doc.Window(sheet, 1, 1, 2, 1)
	if err != nil {
		t.Fatalf("Window: %v", err)
	}
	if window.Cells[1][0].Value != "100" {
		t.Errorf("after undo got %q, want %q", window.Cells[1][0].Value, "100")
	}

	// Redo
	redoChanged, err := history.Redo()
	if err != nil || !redoChanged {
		t.Fatalf("Redo failed: changed=%v, err=%v", redoChanged, err)
	}

	window, err = doc.Window(sheet, 1, 1, 2, 1)
	if err != nil {
		t.Fatalf("Window: %v", err)
	}
	if window.Cells[1][0].Value != "999" {
		t.Errorf("after redo got %q, want %q", window.Cells[1][0].Value, "999")
	}
}
