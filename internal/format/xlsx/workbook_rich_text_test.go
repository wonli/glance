package xlsx

import (
	"path/filepath"
	"testing"

	"glance/internal/document"
)

func TestWorkbookSetCellTextSelectionFormat(t *testing.T) {
	t.Run("bold on plain cell", func(t *testing.T) {
		wb := NewBlank()
		sheet := wb.file.GetSheetName(0)
		cell := "A1"
		text := "Hello World"
		if err := wb.SetCellText(sheet, cell, text); err != nil {
			t.Fatal(err)
		}

		yes := true
		// Format "World" (offsets 6 to 11) with bold
		patch := document.CellFormatPatch{Bold: &yes}
		if err := wb.SetCellTextSelectionFormat(sheet, cell, text, 6, 11, patch); err != nil {
			t.Fatal(err)
		}

		runs, err := wb.GetCellRichText(sheet, cell)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) != 2 {
			t.Fatalf("expected 2 runs, got %d: %#v", len(runs), runs)
		}
		if runs[0].Text != "Hello " || runs[0].Bold {
			t.Errorf("run 0 text=%q bold=%t; want 'Hello ' false", runs[0].Text, runs[0].Bold)
		}
		if runs[1].Text != "World" || !runs[1].Bold {
			t.Errorf("run 1 text=%q bold=%t; want 'World' true", runs[1].Text, runs[1].Bold)
		}
	})

	t.Run("italic and underline in middle of text", func(t *testing.T) {
		wb := NewBlank()
		sheet := wb.file.GetSheetName(0)
		cell := "B2"
		text := "这是测试文字内容"
		if err := wb.SetCellText(sheet, cell, text); err != nil {
			t.Fatal(err)
		}

		yes := true
		// Format "测试" (offsets 2 to 4) with italic and underline
		patch := document.CellFormatPatch{Italic: &yes, Underline: &yes}
		if err := wb.SetCellTextSelectionFormat(sheet, cell, text, 2, 4, patch); err != nil {
			t.Fatal(err)
		}

		runs, err := wb.GetCellRichText(sheet, cell)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) != 3 {
			t.Fatalf("expected 3 runs, got %d: %#v", len(runs), runs)
		}
		if runs[0].Text != "这是" || runs[0].Italic || runs[0].Underline {
			t.Errorf("run 0 text=%q; want '这是'", runs[0].Text)
		}
		if runs[1].Text != "测试" || !runs[1].Italic || !runs[1].Underline {
			t.Errorf("run 1 text=%q italic=%t underline=%t; want '测试' true true", runs[1].Text, runs[1].Italic, runs[1].Underline)
		}
		if runs[2].Text != "文字内容" || runs[2].Italic || runs[2].Underline {
			t.Errorf("run 2 text=%q; want '文字内容'", runs[2].Text)
		}
	})

	t.Run("toggle off bold on existing rich text", func(t *testing.T) {
		wb := NewBlank()
		sheet := wb.file.GetSheetName(0)
		cell := "C3"
		text := "ABCDEF"
		if err := wb.SetCellText(sheet, cell, text); err != nil {
			t.Fatal(err)
		}

		yes := true
		// First, bold "BCDE" (offsets 1 to 5)
		if err := wb.SetCellTextSelectionFormat(sheet, cell, text, 1, 5, document.CellFormatPatch{Bold: &yes}); err != nil {
			t.Fatal(err)
		}

		no := false
		// Then unbold "CD" (offsets 2 to 4)
		if err := wb.SetCellTextSelectionFormat(sheet, cell, text, 2, 4, document.CellFormatPatch{Bold: &no}); err != nil {
			t.Fatal(err)
		}

		runs, err := wb.GetCellRichText(sheet, cell)
		if err != nil {
			t.Fatal(err)
		}
		// Expected runs: "A" (unbold), "B" (bold), "CD" (unbold), "E" (bold), "F" (unbold)
		// Adjacent unbolds should merge: "A" is unbold, but "B" is bold, "CD" unbold, "E" bold, "F" unbold.
		var result []string
		var bolds []bool
		for _, r := range runs {
			result = append(result, r.Text)
			bolds = append(bolds, r.Bold)
		}
		expectedTexts := []string{"A", "B", "CD", "E", "F"}
		expectedBolds := []bool{false, true, false, true, false}
		if len(result) != len(expectedTexts) {
			t.Fatalf("got texts %v (bolds %v); want %v", result, bolds, expectedTexts)
		}
		for i := range result {
			if result[i] != expectedTexts[i] || bolds[i] != expectedBolds[i] {
				t.Errorf("run %d: text=%q bold=%t; want %q %t", i, result[i], bolds[i], expectedTexts[i], expectedBolds[i])
			}
		}
	})

	t.Run("round-trip save and reopen", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "richtext.xlsx")
		wb := NewBlank()
		sheet := wb.file.GetSheetName(0)
		cell := "D4"
		text := "Rich Text Demo"
		if err := wb.SetCellText(sheet, cell, text); err != nil {
			t.Fatal(err)
		}
		yes := true
		if err := wb.SetCellTextSelectionFormat(sheet, cell, text, 5, 9, document.CellFormatPatch{Bold: &yes, Underline: &yes}); err != nil {
			t.Fatal(err)
		}
		wb.SetSourcePath(path)
		if err := wb.Save(); err != nil {
			t.Fatal(err)
		}

		reopened, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()

		runs, err := reopened.GetCellRichText(sheet, cell)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) != 3 {
			t.Fatalf("expected 3 runs, got %d: %#v", len(runs), runs)
		}
		if runs[1].Text != "Text" || !runs[1].Bold || !runs[1].Underline {
			t.Errorf("reopened middle run = %#v; want 'Text' bold and underline", runs[1])
		}
	})

	t.Run("preserves existing style", func(t *testing.T) {
		wb := NewBlank()
		sheet := wb.file.GetSheetName(0)
		cell := "E5"
		text := "红字大号文字"
		if err := wb.SetCellText(sheet, cell, text); err != nil {
			t.Fatal(err)
		}
		// Set font size 18 and red color to whole cell first
		size := 18.0
		color := "#FF0000"
		if err := wb.SetCellFormats(sheet, document.CellRange{StartRow: 5, StartColumn: 5, EndRow: 5, EndColumn: 5}, document.CellFormatPatch{FontSize: &size, FontColor: &color}); err != nil {
			t.Fatal(err)
		}
		origStyleID, _ := wb.file.GetCellStyle(sheet, cell)

		yes := true
		// Bold "大号" (offset 2 to 4)
		if err := wb.SetCellTextSelectionFormat(sheet, cell, text, 2, 4, document.CellFormatPatch{Bold: &yes}); err != nil {
			t.Fatal(err)
		}

		afterStyleID, _ := wb.file.GetCellStyle(sheet, cell)
		t.Logf("origStyleID: %d, afterStyleID: %d", origStyleID, afterStyleID)

		runs, err := wb.GetCellRichText(sheet, cell)
		if err != nil {
			t.Fatal(err)
		}
		for i, r := range runs {
			t.Logf("run %d: text=%q, bold=%t, size=%v, color=%q", i, r.Text, r.Bold, r.FontSize, r.FontColor)
		}

		window, err := wb.Window(sheet, 5, 5, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("cell in window: value=%q, styleID=%d, style=%#v, rich=%#v", window.Cells[0][0].Value, window.Cells[0][0].StyleID, window.Cells[0][0].Style, window.Cells[0][0].RichText)
	})
}
