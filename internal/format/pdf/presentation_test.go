package pdf

import (
	"bytes"
	"context"
	"testing"

	"github.com/SalvioniDigitalSolutions/gopdf"

	"glance/internal/document"
	"glance/internal/wordfont"
)

func TestPresentationPDFLeavesUnstyledTextElementInvisible(t *testing.T) {
	snapshot := document.PresentationSnapshot{
		SlideCount: 1,
		Slides: []document.PresentationSlide{{
			WidthPT: 960, HeightPT: 540,
			Elements: []document.PresentationElement{{
				ID: 1, Kind: document.ElementKindText,
				Bounds:    document.PresentationRect{X: 80, Y: 80, W: 300, H: 80},
				ShapeType: "rect",
			}},
		}},
	}
	var output bytes.Buffer
	if err := WritePresentationPDF(context.Background(), &output, snapshot, "landscape", nil); err != nil {
		t.Fatalf("WritePresentationPDF: %v", err)
	}
	reader, err := gopdf.NewReader(output.Bytes())
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	image, err := reader.RenderPage(0, gopdf.RenderOpts{DPI: 36, IncludeVector: true, IncludeText: true})
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}
	for y := image.Bounds().Min.Y; y < image.Bounds().Max.Y; y++ {
		for x := image.Bounds().Min.X; x < image.Bounds().Max.X; x++ {
			r, g, b, _ := image.At(x, y).RGBA()
			if r < 0xf000 || g < 0xf000 || b < 0xf000 {
				t.Fatalf("unstyled text element painted pixel (%d,%d) = #%04X%04X%04X", x, y, r, g, b)
			}
		}
	}
}

func TestPresentationPDFKeepsWideTextboxTextOnOneLine(t *testing.T) {
	fonts, err := wordfont.NewSystem()
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	snapshot := document.PresentationSnapshot{
		SlideCount: 1,
		Slides: []document.PresentationSlide{{
			WidthPT: 960, HeightPT: 540,
			Elements: []document.PresentationElement{{
				ID: 1, Kind: document.ElementKindText,
				Bounds: document.PresentationRect{X: 72, Y: 72, W: 360, H: 72},
				Paragraphs: []document.PresentationParagraph{{Runs: []document.PresentationTextRun{{
					Text: "季度报告导出验证", SizePT: 18, ColorHex: "#1F2937",
				}}}},
			}},
		}},
	}
	var output bytes.Buffer
	if err := WritePresentationPDF(context.Background(), &output, snapshot, "landscape", fonts); err != nil {
		t.Fatalf("WritePresentationPDF: %v", err)
	}
	reader, err := gopdf.NewReader(output.Bytes())
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	fragments, err := reader.PageTextFragments(0)
	if err != nil {
		t.Fatalf("PageTextFragments: %v", err)
	}
	if len(fragments) == 0 {
		t.Fatal("PDF contains no text fragments")
	}
	baseline := fragments[0].Y
	for _, fragment := range fragments[1:] {
		if fragment.Y != baseline {
			t.Fatalf("wide textbox text wrapped across baselines %.2f and %.2f", baseline, fragment.Y)
		}
	}
}
