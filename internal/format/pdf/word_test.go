package pdf

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SalvioniDigitalSolutions/gopdf"
	"golang.org/x/image/font/gofont/goregular"

	"glance/internal/document"
	"glance/internal/wordlayout"
)

type testResources struct {
	fonts  map[string][]byte
	images map[string][]byte
}

func (r testResources) FontData(key string) ([]byte, error) {
	data, ok := r.fonts[key]
	if !ok {
		return nil, wordlayout.ErrUnknownFont
	}
	return bytes.Clone(data), nil
}

func (r testResources) ImageBytes(id string) ([]byte, error) {
	data, ok := r.images[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return bytes.Clone(data), nil
}

func TestWriteWordPDFReplaysPageGeometryAndSearchableText(t *testing.T) {
	resources := testResources{fonts: map[string][]byte{"font": goregular.TTF}}
	result := wordlayout.Result{
		CanPreview: true,
		Pages: []wordlayout.PageScene{{
			Index: 0,
			Clip:  wordlayout.Rect{WidthPT: 320, HeightPT: 180},
			Text: []wordlayout.TextFragment{{
				Text: "A", RuneEnd: 1, Style: document.WordTextStyle{FontSize: 12, FontColor: "FF0000"},
				XPT: 40, BaselineYPT: 70,
				Glyphs: []wordlayout.PositionedGlyph{{FontKey: "font", RuneEnd: 1, XPT: 40, YPT: 70, AdvanceXPT: 12, Outline: testSquareOutline()}},
			}},
		}},
	}
	var output bytes.Buffer
	if err := WriteWordPDF(context.Background(), &output, result, resources, resources); err != nil {
		t.Fatalf("WriteWordPDF: %v", err)
	}
	reader, err := gopdf.NewReader(output.Bytes())
	if err != nil {
		t.Fatalf("read generated PDF: %v", err)
	}
	if got := reader.NumPages(); got != 1 {
		t.Fatalf("PDF pages = %d, want 1", got)
	}
	size, err := reader.PageSize(0)
	if err != nil {
		t.Fatal(err)
	}
	if size.W != 320 || size.H != 180 {
		t.Fatalf("page size = %.2fx%.2f, want 320x180 points", size.W, size.H)
	}
	text, err := reader.PageText(0)
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if text != "A" {
		t.Fatalf("extracted text = %q, want A", text)
	}
	rendered, _, err := reader.RenderPageDetail(0, gopdf.RenderOpts{DPI: 72, IncludeVector: true})
	if err != nil {
		t.Fatalf("render generated PDF: %v", err)
	}
	got := color.NRGBAModel.Convert(rendered.At(45, 75)).(color.NRGBA)
	if got.R < 240 || got.G > 20 || got.B > 20 {
		t.Fatalf("vector glyph pixel = %v, want visible red fill", got)
	}
}

func TestWriteWordPDFReplaysImageRectangle(t *testing.T) {
	var encoded bytes.Buffer
	fixture := image.NewRGBA(image.Rect(0, 0, 2, 1))
	fixture.Set(0, 0, color.NRGBA{R: 20, G: 40, B: 60, A: 255})
	fixture.Set(1, 0, color.NRGBA{R: 20, G: 40, B: 60, A: 255})
	if err := png.Encode(&encoded, fixture); err != nil {
		t.Fatal(err)
	}
	resources := testResources{images: map[string][]byte{"picture": encoded.Bytes()}}
	result := wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{
		Clip:   wordlayout.Rect{WidthPT: 200, HeightPT: 100},
		Images: []wordlayout.ImagePlacement{{ImageID: "picture", Bounds: wordlayout.Rect{XPT: 11, YPT: 22, WidthPT: 50, HeightPT: 30}}},
	}}}
	var output bytes.Buffer
	if err := WriteWordPDF(context.Background(), &output, result, resources, resources); err != nil {
		t.Fatalf("WriteWordPDF: %v", err)
	}
	reader, err := gopdf.NewReader(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	images, err := reader.PageImages(0)
	if err != nil || len(images) != 1 {
		t.Fatalf("PageImages = %d, err=%v; want one image", len(images), err)
	}
	got := images[0]
	if math.Abs(got.X-11) > 0.01 || math.Abs(got.Y-22) > 0.01 || math.Abs(got.W-50) > 0.01 || math.Abs(got.H-30) > 0.01 {
		t.Fatalf("image bounds = (%.2f, %.2f, %.2f, %.2f), want (11, 22, 50, 30)", got.X, got.Y, got.W, got.H)
	}
}

func TestWriteWordPDFImageRotationMatchesClockwiseSceneRotation(t *testing.T) {
	fixture := image.NewRGBA(image.Rect(0, 0, 2, 1))
	fixture.Set(0, 0, color.NRGBA{R: 255, A: 255})
	fixture.Set(1, 0, color.NRGBA{B: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, fixture); err != nil {
		t.Fatal(err)
	}
	resources := testResources{images: map[string][]byte{"picture": encoded.Bytes()}}
	result := wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{
		Clip:   wordlayout.Rect{WidthPT: 100, HeightPT: 100},
		Images: []wordlayout.ImagePlacement{{ImageID: "picture", Bounds: wordlayout.Rect{XPT: 11, YPT: 22, WidthPT: 50, HeightPT: 30}, RotationDegrees: 90}},
	}}}
	var output bytes.Buffer
	if err := WriteWordPDF(context.Background(), &output, result, resources, resources); err != nil {
		t.Fatalf("WriteWordPDF: %v", err)
	}
	reader, err := gopdf.NewReader(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	images, err := reader.PageImages(0)
	if err != nil || len(images) != 1 {
		t.Fatalf("PageImages = %d, err=%v", len(images), err)
	}
	if math.Abs(images[0].W-30) > 0.01 || math.Abs(images[0].H-50) > 0.01 {
		t.Fatalf("rotated image bounds = %.2fx%.2f; want 30x50", images[0].W, images[0].H)
	}
	rendered, _, err := reader.RenderPageDetail(0, gopdf.RenderOpts{DPI: 72, IncludeRasterImages: true})
	if err != nil {
		t.Fatal(err)
	}
	top := color.NRGBAModel.Convert(rendered.At(36, 25)).(color.NRGBA)
	if top.R < 200 || top.B > 20 {
		t.Fatalf("rotated top pixel = %v; want original left/red half at top", top)
	}
}

func TestWriteWordPDFExtractsMixedCJKLatinTextWhenSystemFontIsAvailable(t *testing.T) {
	const fontPath = "/Library/Fonts/Arial Unicode.ttf"
	fontData, err := os.ReadFile(fontPath)
	if err != nil {
		t.Skip("host CJK font fixture is unavailable")
	}
	resources := testResources{fonts: map[string][]byte{"font": fontData}}
	result := wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{
		Clip: wordlayout.Rect{WidthPT: 320, HeightPT: 180},
		Text: []wordlayout.TextFragment{{
			Text: "A中B", RuneEnd: 3, Style: document.WordTextStyle{FontSize: 11}, XPT: 20, BaselineYPT: 40,
			Glyphs: []wordlayout.PositionedGlyph{
				{FontKey: "font", RuneStart: 0, RuneEnd: 1, XPT: 20, YPT: 40, AdvanceXPT: 8},
				{FontKey: "font", RuneStart: 1, RuneEnd: 2, XPT: 28, YPT: 40, AdvanceXPT: 11},
				{FontKey: "font", RuneStart: 2, RuneEnd: 3, XPT: 39, YPT: 40, AdvanceXPT: 8},
			},
		}},
	}}}
	var output bytes.Buffer
	if err := WriteWordPDF(context.Background(), &output, result, resources, resources); err != nil {
		t.Fatalf("WriteWordPDF mixed text: %v", err)
	}
	reader, err := gopdf.NewReader(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	text, err := reader.PageText(0)
	if err != nil || text != "A中B" {
		t.Fatalf("extracted mixed text = %q, err=%v; want A中B", text, err)
	}
}

func TestWriteWordPDFBlocksUnsupportedLayoutAndCanceledWork(t *testing.T) {
	base := wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Clip: wordlayout.Rect{WidthPT: 100, HeightPT: 100}}}}
	base.Diagnostics = []wordlayout.Diagnostic{{Severity: wordlayout.DiagnosticError, Code: "unsupported", Detail: "cannot render"}}
	if err := WriteWordPDF(context.Background(), &bytes.Buffer{}, base, testResources{}, testResources{}); err == nil || !strings.Contains(err.Error(), "cannot render") {
		t.Fatalf("blocking layout diagnostic error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := WriteWordPDF(ctx, &bytes.Buffer{}, wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Clip: wordlayout.Rect{WidthPT: 100, HeightPT: 100}}}}, testResources{}, testResources{}); err == nil || err != context.Canceled {
		t.Fatalf("canceled export error = %v, want context.Canceled", err)
	}
}

func TestExportWordPDFFilePreservesDestinationOnCancelAndFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.pdf")
	if err := os.WriteFile(path, []byte("keep-me"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Clip: wordlayout.Rect{WidthPT: 100, HeightPT: 100}}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ExportWordPDFFile(ctx, path, result, testResources{}, testResources{}); err != context.Canceled {
		t.Fatalf("canceled export error = %v, want context.Canceled", err)
	}
	result.CanPreview = false
	if err := ExportWordPDFFile(context.Background(), path, result, testResources{}, testResources{}); err == nil {
		t.Fatal("expected invalid layout export failure")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "keep-me" {
		t.Fatalf("existing destination after failure = %q, want keep-me", data)
	}
}

func TestExportWordPDFFileCommitGuardPreservesDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.pdf")
	if err := os.WriteFile(path, []byte("keep-me"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Clip: wordlayout.Rect{WidthPT: 100, HeightPT: 100}}}}
	err := ExportWordPDFFileWithCommitCheck(context.Background(), path, result, testResources{}, testResources{}, func() error {
		return errors.New("stale revision")
	})
	if err == nil || !strings.Contains(err.Error(), "stale revision") {
		t.Fatalf("stale commit guard error = %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil || string(data) != "keep-me" {
		t.Fatalf("destination after stale export = %q, err=%v; want keep-me", data, readErr)
	}
}

func TestExportWordPDFFileAtomicallyReplacesDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.pdf")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Clip: wordlayout.Rect{WidthPT: 100, HeightPT: 100}}}}
	if err := ExportWordPDFFile(context.Background(), path, result, testResources{}, testResources{}); err != nil {
		t.Fatalf("ExportWordPDFFile: %v", err)
	}
	reader, err := gopdf.Open(path)
	if err != nil {
		t.Fatalf("open exported PDF: %v", err)
	}
	if reader.NumPages() != 1 {
		t.Fatalf("exported page count = %d, want 1", reader.NumPages())
	}
}

func TestValidateFontEmbeddingRejectsRestrictedAndNoSubsetFonts(t *testing.T) {
	for _, test := range []struct {
		name string
		flag uint16
	}{
		{name: "restricted", flag: 0x0002},
		{name: "no subsetting", flag: 0x0100},
		{name: "bitmap only", flag: 0x0200},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := withFSType(t, goregular.TTF, test.flag)
			if err := validateFontEmbedding(data, "font"); err == nil {
				t.Fatal("expected font embedding permission error")
			}
		})
	}
}

func testSquareOutline() []wordlayout.GlyphPathCommand {
	point := func(x, y float64) [3]wordlayout.GlyphPathPoint {
		return [3]wordlayout.GlyphPathPoint{{XPT: x, YPT: y}}
	}
	return []wordlayout.GlyphPathCommand{
		{Op: wordlayout.GlyphPathMoveTo, Points: point(0, 0)},
		{Op: wordlayout.GlyphPathLineTo, Points: point(10, 0)},
		{Op: wordlayout.GlyphPathLineTo, Points: point(10, 10)},
		{Op: wordlayout.GlyphPathLineTo, Points: point(0, 10)},
	}
}

func withFSType(t *testing.T, original []byte, flag uint16) []byte {
	t.Helper()
	data := bytes.Clone(original)
	if len(data) < 12 {
		t.Fatal("font fixture is too small")
	}
	tables := int(binary.BigEndian.Uint16(data[4:6]))
	for i := 0; i < tables; i++ {
		record := 12 + 16*i
		if record+16 > len(data) {
			t.Fatal("invalid font table directory")
		}
		if string(data[record:record+4]) != "OS/2" {
			continue
		}
		offset := int(binary.BigEndian.Uint32(data[record+8 : record+12]))
		length := int(binary.BigEndian.Uint32(data[record+12 : record+16]))
		if length < 10 || offset+length > len(data) {
			t.Fatal("invalid OS/2 table")
		}
		binary.BigEndian.PutUint16(data[offset+8:offset+10], flag)
		return data
	}
	t.Fatal("font fixture has no OS/2 table")
	return nil
}
