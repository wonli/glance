package desktopui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sheetpdf "glance/internal/format/pdf"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetPDFPerformanceSample(t *testing.T) {
	path := os.Getenv("GLANCE_PDF_SAMPLE")
	if path == "" {
		t.Skip("set GLANCE_PDF_SAMPLE for local visual regression")
	}
	book, err := xlsx.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	sheet := book.SheetNames()[0]
	cols, rows, err := book.Dimension(sheet)
	if err != nil {
		t.Fatal(err)
	}
	window, err := book.Window(sheet, 1, 1, rows, cols)
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := newWordTextShaper()
	if err != nil {
		t.Fatal(err)
	}
	dir := os.Getenv("GLANCE_PDF_OUTPUT")
	if dir == "" {
		dir = t.TempDir()
	}
	for _, orientation := range []string{"portrait", "landscape"} {
		t.Run(orientation, func(t *testing.T) {
			opts := sheetpdf.SpreadsheetPDFOptions{SheetName: sheet, Orientation: orientation, FitToPageWidth: true}
			p := sheetpdf.ComputeSpreadsheetPagination(window, opts, nil)
			t.Logf("pages=%d heights=%v", p.TotalPages, p.RowHeights)
			if err := sheetpdf.ExportSpreadsheetPDFFile(context.Background(), filepath.Join(dir, "performance-"+orientation+".pdf"), window, opts, fonts, nil); err != nil {
				t.Fatal(err)
			}
			if p.TotalPages != 1 {
				t.Errorf("pages = %d, want 1", p.TotalPages)
			}
			if len(p.RowHeights) != 14 {
				t.Fatalf("rows = %d, want 14", len(p.RowHeights))
			}
			if p.RowHeights[13] > 100 {
				t.Errorf("instruction row inflated: %v", p.RowHeights[13])
			}
		})
	}
}
