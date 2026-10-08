package desktopui

import (
	"context"
	"math"
	"os"
	"testing"

	"glance/internal/format/docx"
	pdfexport "glance/internal/format/pdf"
	"glance/internal/wordlayout"
)

// Private fixtures stay outside the repository and ordinary test runs.
func TestLocalWordCoverLogo(t *testing.T) {
	path := os.Getenv("GLANCE_WORD_COVER_DOCX")
	if path == "" {
		t.Skip("set GLANCE_WORD_COVER_DOCX to run the private cover regression")
	}
	doc, err := docx.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	shaper, err := newWordTextShaper()
	if err != nil {
		t.Fatal(err)
	}
	result, err := wordlayout.NewEngine(shaper).Layout(context.Background(), wordlayout.Input{Blocks: doc.WordBlocks(), Layout: doc.WordLayoutData()})
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanPreview {
		t.Fatalf("layout blocked: %+v", result.Diagnostics)
	}
	if len(result.Pages) == 0 || len(result.Pages[0].Images) != 1 {
		t.Fatal("cover logo must remain on page 1")
	}
	if len(result.Pages) < 2 || len(result.Pages[1].Text) == 0 {
		t.Fatal("an empty page separates the cover from the next section")
	}
	logo := result.Pages[0].Images[0]
	if math.Abs(logo.Bounds.WidthPT-152) > 0.01 || math.Abs(logo.Bounds.HeightPT-31.95) > 0.01 {
		t.Fatalf("logo was resized: %+v", logo.Bounds)
	}
	if logo.Bounds.YPT < 72 || logo.Bounds.YPT+logo.Bounds.HeightPT > 769.9 {
		t.Fatalf("logo exceeds cover content: %+v", logo.Bounds)
	}
	t.Logf("cover logo: page 1, y=%.2f, size %.2fx%.2fpt", logo.Bounds.YPT, logo.Bounds.WidthPT, logo.Bounds.HeightPT)
	if output := os.Getenv("GLANCE_WORD_COVER_PDF"); output != "" {
		file, err := os.Create(output)
		if err != nil {
			t.Fatal(err)
		}
		result.Pages = result.Pages[:2]
		err = pdfexport.WriteWordPDF(context.Background(), file, result, shaper, wordPDFImageSource{document: doc})
		closeErr := file.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
}
