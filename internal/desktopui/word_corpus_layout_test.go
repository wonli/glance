package desktopui

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"glance/internal/format/docx"
	"glance/internal/wordlayout"
)

func TestOfficeCorpusDOCXFilesProducePreviewablePageScenes(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "office-corpus", "docx", "*.docx"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("DOCX office corpus is empty")
	}
	shaper, err := newWordTextShaper()
	if err != nil {
		t.Fatal(err)
	}
	engine := wordlayout.NewEngine(shaper)
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			doc, err := docx.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer doc.Close()
			result, err := engine.Layout(context.Background(), wordlayout.Input{
				DocumentID: path, Blocks: doc.WordBlocks(), Layout: doc.WordLayoutData(),
				DefaultPageSetup: defaultWordPageSetup, EditorParagraphInsets: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.CanPreview || len(result.Pages) == 0 {
				t.Errorf("page preview blocked: pages=%d diagnostics=%+v", len(result.Pages), result.Diagnostics)
			}
		})
	}
}

func TestFloatingImageWrapCorpusFlowsTextAlongTightPolygon(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "office-corpus", "docx", "floating-image-wrap-vml.docx")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := docx.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	blocks := doc.WordBlocks()
	floatBlockIndex := -1
	var floatingImageID string
	var floatingGeometry struct {
		widthEMU, heightEMU int64
		x, y, left, right   int64
		points              []struct{ X, Y float64 }
	}
	runBlocks := make(map[string]int)
	for blockIndex, block := range blocks {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID != "" {
				runBlocks[inline.Text.ID] = blockIndex
			}
			if inline.Image == nil || inline.Image.Floating == nil {
				continue
			}
			floatBlockIndex, floatingImageID = blockIndex, inline.Image.ID
			floatingGeometry.widthEMU, floatingGeometry.heightEMU = inline.Image.WidthEMU, inline.Image.HeightEMU
			floatingGeometry.x, floatingGeometry.y = inline.Image.Floating.XOffsetEMU, inline.Image.Floating.YOffsetEMU
			floatingGeometry.left, floatingGeometry.right = inline.Image.Floating.WrapDistanceLeftEMU, inline.Image.Floating.WrapDistanceRightEMU
			for _, point := range inline.Image.Floating.WrapPolygon {
				floatingGeometry.points = append(floatingGeometry.points, struct{ X, Y float64 }{point.X, point.Y})
			}
		}
	}
	if floatBlockIndex < 0 || floatingImageID == "" || len(floatingGeometry.points) < 3 {
		t.Fatal("corpus adapter did not project the tight-wrapped VML float")
	}
	shaper, err := newWordTextShaper()
	if err != nil {
		t.Fatal(err)
	}
	result, err := wordlayout.NewEngine(shaper).Layout(context.Background(), wordlayout.Input{
		DocumentID: path, Blocks: blocks, Layout: doc.WordLayoutData(),
		DefaultPageSetup: defaultWordPageSetup, EditorParagraphInsets: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanPreview || len(result.Pages) == 0 {
		t.Fatalf("corpus layout is not previewable: %+v", result.Diagnostics)
	}
	pageIndex := -1
	var bounds wordlayout.Rect
	imageCount := 0
	for _, page := range result.Pages {
		for _, placement := range page.Images {
			if placement.ImageID == floatingImageID {
				pageIndex, bounds = page.Index, placement.Bounds
				imageCount++
			}
		}
	}
	if imageCount != 1 || pageIndex < 0 {
		t.Fatalf("float placement count=%d page=%d; want one placed image", imageCount, pageIndex)
	}
	wantX := result.Pages[pageIndex].Content.XPT + float64(floatingGeometry.x)/12700
	if math.Abs(bounds.XPT-wantX) > 0.01 || math.Abs(bounds.WidthPT-float64(floatingGeometry.widthEMU)/12700) > 0.01 || math.Abs(bounds.HeightPT-float64(floatingGeometry.heightEMU)/12700) > 0.01 {
		t.Fatalf("corpus float bounds = %+v; want text-relative X %.2fpt and projected extent", bounds, wantX)
	}
	minX, maxX := math.Inf(1), math.Inf(-1)
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, point := range floatingGeometry.points {
		minX, maxX = math.Min(minX, point.X), math.Max(maxX, point.X)
		minY, maxY = math.Min(minY, point.Y), math.Max(maxY, point.Y)
	}
	wrapLeft := bounds.XPT + minX*bounds.WidthPT - float64(floatingGeometry.left)/12700
	wrapRight := bounds.XPT + maxX*bounds.WidthPT + float64(floatingGeometry.right)/12700
	wrapTop := bounds.YPT + minY*bounds.HeightPT
	wrapBottom := bounds.YPT + maxY*bounds.HeightPT
	page := result.Pages[pageIndex]
	seenWrappedText, seenFullWidthText := false, false
	wrappedSidesByBaseline := make(map[float64]uint8)
	for _, fragment := range page.Text {
		blockIndex, found := runBlocks[fragment.RunID]
		if !found || blockIndex < floatBlockIndex || len(fragment.Glyphs) == 0 {
			continue
		}
		fontSize := fragment.Style.FontSize
		textTop, textBottom := fragment.BaselineYPT-fontSize*0.8, fragment.BaselineYPT+fontSize*0.2
		if textBottom > wrapTop && textTop < wrapBottom {
			seenWrappedText = true
			for _, glyph := range fragment.Glyphs {
				glyphRight := glyph.XPT + glyph.AdvanceXPT
				if glyphRight > wrapLeft+0.01 && glyph.XPT < wrapRight-0.01 {
					t.Fatalf("text overlaps the tight wrap exclusion: glyph=%+v, wrap=[%.2f,%.2f] y=[%.2f,%.2f]", glyph, wrapLeft, wrapRight, wrapTop, wrapBottom)
				}
				if glyphRight <= wrapLeft+0.01 {
					wrappedSidesByBaseline[fragment.BaselineYPT] |= 1
				}
				if glyph.XPT >= wrapRight-0.01 {
					wrappedSidesByBaseline[fragment.BaselineYPT] |= 2
				}
			}
		}
		if textTop >= wrapBottom-0.01 && math.Abs(fragment.XPT-page.Content.XPT) < 0.05 {
			seenFullWidthText = true
		}
	}
	seenBothSidesOnOneBaseline := false
	for _, sides := range wrappedSidesByBaseline {
		if sides == 3 {
			seenBothSidesOnOneBaseline = true
			break
		}
	}
	if !seenWrappedText || !seenBothSidesOnOneBaseline || !seenFullWidthText {
		t.Fatalf("corpus flow did not cover both sides of the shape on one baseline and resume below it: wrapped=%v bothSides=%v fullWidthAfter=%v bounds=%+v", seenWrappedText, seenBothSidesOnOneBaseline, seenFullWidthText, bounds)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, after) {
		t.Fatal("opening and laying out the corpus changed the original DOCX bytes")
	}
}
