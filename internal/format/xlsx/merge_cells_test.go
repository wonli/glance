package xlsx

import (
	"path/filepath"
	"testing"

	"glance/internal/document"
)

func TestMergeCellsChangedSaveReopenPreservesAnchorAndUntouchedContent(t *testing.T) {
	book := NewBlank()
	t.Cleanup(func() { _ = book.Close() })
	if err := book.SetCellText("Sheet1", "A1", "标题"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellText("Sheet1", "B1", "副标题"); err != nil {
		t.Fatal(err)
	}
	italic := true
	if _, err := book.SetCellFormatsChanged("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, document.CellFormatPatch{Italic: &italic}); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellText("Sheet1", "D5", "保留格式"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellFormula("Sheet1", "E5", "=2+3"); err != nil {
		t.Fatal(err)
	}
	bold := true
	if _, err := book.SetCellFormatsChanged("Sheet1", document.CellRange{StartRow: 5, StartColumn: 4, EndRow: 5, EndColumn: 4}, document.CellFormatPatch{Bold: &bold}); err != nil {
		t.Fatal(err)
	}

	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2}
	preview, err := book.MergeCellsChanged("Sheet1", bounds, "")
	if err != nil || !preview.ConfirmationRequired || preview.DiscardedCellCount != 1 || preview.Changed {
		t.Fatalf("merge preview = %+v, err=%v; want non-mutating content-loss preview", preview, err)
	}
	merged, err := book.MergeCellsChanged("Sheet1", bounds, preview.ConfirmationToken)
	if err != nil || !merged.Changed || merged.ConfirmationRequired {
		t.Fatalf("confirmed merge = %+v, err=%v", merged, err)
	}

	path := filepath.Join(t.TempDir(), "merged.xlsx")
	if err := book.SaveAs(path); err != nil {
		t.Fatalf("save merged workbook: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen merged workbook: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	window, err := reopened.Window("Sheet1", 1, 1, 5, 5)
	if err != nil {
		t.Fatalf("read reopened workbook: %v", err)
	}
	if len(window.Merges) != 1 || window.Merges[0].Range != bounds || window.Merges[0].Anchor.Value != "标题" {
		t.Fatalf("reopened merged cells = %#v; want A1:B1 with the anchor text", window.Merges)
	}
	if style := window.Merges[0].Anchor.Style; style.HorizontalAlignment != "center" || !style.Italic {
		t.Fatalf("reopened merge anchor style = %+v; want centered italic style", style)
	}
	if window.Cells[4][3].Value != "保留格式" || !window.Cells[4][3].Style.Bold {
		t.Fatalf("untouched D5 = %+v; want value and bold style preserved", window.Cells[4][3])
	}
	formula, err := reopened.GetCellFormula("Sheet1", "E5")
	if err != nil || formula != "2+3" {
		t.Fatalf("untouched E5 formula = %q, err=%v; want 2+3", formula, err)
	}
}
