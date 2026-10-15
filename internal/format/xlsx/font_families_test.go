package xlsx

import (
	"glance/internal/document"
	"testing"
)

func TestFontFamiliesIncludesOffscreenStyle(t *testing.T) {
	w := NewBlank()
	defer w.Close()
	family := "Offscreen Font"
	if err := w.SetCellFormats("Sheet1", document.CellRange{StartRow: 2000, EndRow: 2000, StartColumn: 3, EndColumn: 3}, document.CellFormatPatch{FontFamily: &family}); err != nil {
		t.Fatal(err)
	}
	names, err := w.FontFamilies()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if n == family {
			return
		}
	}
	t.Fatalf("missing offscreen font: %v", names)
}
