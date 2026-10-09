package desktopui

import (
	"context"
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"glance/internal/wordlayout"
)

func officeCorpusPath(relative string) string {
	return filepath.Join("..", "..", "testdata", "office-corpus", filepath.FromSlash(relative))
}

func renderCorpusDocument(t *testing.T, relative string) (*HeadlessDocument, []HeadlessTarget) {
	t.Helper()
	doc, err := OpenHeadlessDocument(officeCorpusPath(relative))
	if err != nil {
		t.Fatalf("open %s: %v", relative, err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	targets, err := doc.Render(context.Background(), HeadlessRenderOptions{Width: 1280, Height: 800, Scale: 1})
	if errors.Is(err, ErrHeadlessUnavailable) {
		t.Skipf("Gio headless is unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("render %s: %v", relative, err)
	}
	return doc, targets
}

func assertHeadlessTarget(t *testing.T, target HeadlessTarget, name string) {
	t.Helper()
	if target.Name != name {
		t.Fatalf("target name = %q, want %q", target.Name, name)
	}
	if target.Image == nil {
		t.Fatal("target image is nil")
	}
	if got := target.Image.Bounds().Size(); got.X != 1280 || got.Y != 800 {
		t.Fatalf("target dimensions = %v, want 1280x800", got)
	}
	var painted int
	for y := target.Image.Bounds().Min.Y; y < target.Image.Bounds().Max.Y; y += 8 {
		for x := target.Image.Bounds().Min.X; x < target.Image.Bounds().Max.X; x += 8 {
			pixel := color.NRGBAModel.Convert(target.Image.At(x, y)).(color.NRGBA)
			if pixel.A != 0 && pixel != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
				painted++
			}
		}
	}
	if painted < 10 {
		t.Fatalf("screenshot contains only %d sampled non-white pixels", painted)
	}
}

func TestHeadlessRenderXLSXFirstWorksheet(t *testing.T) {
	_, targets := renderCorpusDocument(t, "xlsx/formula-example.xlsx")
	if len(targets) != 1 {
		t.Fatalf("target count = %d, want one worksheet viewport", len(targets))
	}
	assertHeadlessTarget(t, targets[0], "worksheet-1")
}

func TestHeadlessRenderDOCXFirstViewport(t *testing.T) {
	_, targets := renderCorpusDocument(t, "docx/sample.docx")
	if len(targets) != 1 {
		t.Fatalf("target count = %d, want one Word viewport", len(targets))
	}
	assertHeadlessTarget(t, targets[0], "viewport-1")
}

func TestHeadlessRenderPPTXEverySlide(t *testing.T) {
	doc, targets := renderCorpusDocument(t, "pptx/sample-show.pptx")
	want := doc.app.pptxDoc.Snapshot().SlideCount
	if len(targets) != want {
		t.Fatalf("target count = %d, want one target per slide (%d)", len(targets), want)
	}
	for index, target := range targets {
		assertHeadlessTarget(t, target, "slide-"+strconv.Itoa(index+1))
	}
}

func TestHeadlessCloseClosesPresentationAdapter(t *testing.T) {
	doc, err := OpenHeadlessDocument(officeCorpusPath("pptx/sample-show.pptx"))
	if err != nil {
		t.Fatal(err)
	}
	presentation := doc.app.pptxDoc
	if presentation == nil {
		t.Fatal("opened PPTX has no presentation adapter")
	}
	if err := doc.Close(); err != nil {
		t.Fatalf("close headless document: %v", err)
	}
	if err := presentation.AddSlide(); err == nil {
		t.Fatal("presentation adapter remained open after closing headless document")
	}
}

func TestHeadlessRenderEachStreamsPPTXSlides(t *testing.T) {
	doc, err := OpenHeadlessDocument(officeCorpusPath("pptx/sample-show.pptx"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	var names []string
	err = doc.RenderEach(context.Background(), HeadlessRenderOptions{Width: 1280, Height: 800, Scale: 1}, func(target HeadlessTarget) error {
		if target.Image == nil {
			t.Fatal("streamed target image is nil")
		}
		names = append(names, target.Name)
		return nil
	})
	if errors.Is(err, ErrHeadlessUnavailable) {
		t.Skipf("Gio headless is unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("stream PPTX targets: %v", err)
	}
	want := doc.app.pptxDoc.Snapshot().SlideCount
	if len(names) != want {
		t.Fatalf("streamed %d targets, want %d", len(names), want)
	}
	for index, name := range names {
		if name != "slide-"+strconv.Itoa(index+1) {
			t.Errorf("target %d = %q", index+1, name)
		}
	}
}

func TestHeadlessRenderMalformedDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "malformed.docx")
	if err := os.WriteFile(path, []byte("not an Office document"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenHeadlessDocument(path); err == nil {
		t.Fatal("OpenHeadlessDocument accepted malformed DOCX data")
	}
}

func TestHeadlessDocumentDiagnosticsExposeCodesWithoutDocumentText(t *testing.T) {
	doc := &HeadlessDocument{app: &App{documentSessionState: &documentSessionState{
		sharedDocumentSession: &sharedDocumentSession{wordLayoutResult: &wordlayout.Result{Diagnostics: []wordlayout.Diagnostic{{
			Severity: wordlayout.DiagnosticWarning,
			SourceID: "run-secret",
			Code:     "unsupported-floating-object",
			Detail:   "private document phrase",
		}}}},
	}}}
	got := doc.Diagnostics()
	if len(got) != 1 || got[0] != "warning:unsupported-floating-object" {
		t.Fatalf("headless diagnostics = %v", got)
	}
}
