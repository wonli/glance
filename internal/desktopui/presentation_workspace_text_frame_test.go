package desktopui

import (
	"image"

	"math"

	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/gpu/headless"

	"gioui.org/io/semantic"
	"gioui.org/layout"

	"gioui.org/text"
	"gioui.org/unit"

	"golang.org/x/image/math/fixed"

	"glance/internal/document"
)

func TestPresentationTextEditorFrameUsesObjectBounds(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	elem := &h.view.snapshot.Slides[0].Elements[0]
	elem.Bounds = document.PresentationRect{X: 50, Y: 50, W: 480, H: 220}
	elem.Paragraphs = []document.PresentationParagraph{{Runs: []document.PresentationTextRun{{
		Text:   "Institution",
		Font:   "Arial",
		SizePT: 36,
		Bold:   true,
		Italic: true,
	}}}}
	elem.WrapText = false
	h.elem = *elem
	h.frame()
	point := h.textClickPoint(60, 10)
	h.click(point)
	h.click(point)
	bounds := findPresentationSemanticBounds(h.router.AppendSemantics(nil), semantic.Editor, "")
	if bounds.Empty() {
		t.Fatal("focused slide text has no editor bounds")
	}
	wantFrame := presentationBoundsInPixels(elem.Bounds, h.view.canvasScale)
	if h.view.editingArea != wantFrame {
		t.Fatalf("text editor frame = %v, want exact object bounds %v (semantic=%v)", h.view.editingArea, wantFrame, bounds)
	}
	style := presentationElementEditorFont(*elem)
	if !strings.Contains(string(style.Typeface), "Arial") || style.Weight != font.Bold || style.Style != font.Italic {
		t.Fatalf("editor font does not match the first text run: %+v", style)
	}

	h.view.shapeEditor.SetCaret(len([]rune(h.view.shapeEditor.Text())), len([]rune(h.view.shapeEditor.Text())))
	h.frame()
	window, err := headless.NewWindow(1200, 700)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	if err := window.Frame(h.ops); err != nil {
		t.Fatal(err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, 1200, 700))
	if err := window.Screenshot(screenshot); err != nil {
		t.Fatal(err)
	}
	frame := h.view.editingArea.Add(h.view.canvasCardOrigin)
	handleSize := presentationHandlePaintSize(layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
	for _, corner := range []image.Point{
		frame.Min,
		{X: frame.Max.X - handleSize, Y: frame.Min.Y},
		{X: frame.Min.X, Y: frame.Max.Y - handleSize},
		{X: frame.Max.X - handleSize, Y: frame.Max.Y - handleSize},
	} {
		var accentPixels int
		for y := corner.Y; y < corner.Y+handleSize; y++ {
			for x := corner.X; x < corner.X+handleSize; x++ {
				got := screenshot.RGBAAt(x, y)
				if int(got.B) > int(got.R)+70 && int(got.B) > int(got.G)+70 {
					accentPixels++
				}
			}
		}
		if accentPixels == 0 {
			t.Fatalf("editor boundary corner at %v has no visible control point", corner)
		}
	}
	caret := h.view.shapeEditor.CaretCoords()
	caretX := frame.Min.X + int(presentationEditorInsetX) + int(math.Round(float64(caret.X)))
	caretY := frame.Min.Y + int(presentationEditorInsetY) + int(math.Round(float64(caret.Y)))
	maxCaretRun := 0
	for x := caretX - 2; x <= caretX+2; x++ {
		run := 0
		for y := max(0, caretY-48); y <= min(screenshot.Bounds().Max.Y-1, caretY+12); y++ {
			pixel := screenshot.RGBAAt(x, y)
			if pixel.R < 80 && pixel.G < 100 && pixel.B < 140 {
				run++
				maxCaretRun = max(maxCaretRun, run)
			} else {
				run = 0
			}
		}
	}
	h.app.theme.Shaper.LayoutString(text.Parameters{Font: style, PxPerEm: fixed.I(36)}, h.view.shapeEditor.Text())
	expectedCaretHeight := 0
	for glyph, ok := h.app.theme.Shaper.NextGlyph(); ok; glyph, ok = h.app.theme.Shaper.NextGlyph() {
		glyphHeight := int(math.Ceil(float64(glyph.Ascent)/64)) + int(math.Ceil(float64(glyph.Descent)/64))
		expectedCaretHeight = max(expectedCaretHeight, glyphHeight)
	}
	if expectedCaretHeight == 0 || math.Abs(float64(maxCaretRun-expectedCaretHeight)) > 3 {
		t.Fatalf("caret height does not match the selected text font: pixels=%d font metrics=%d", maxCaretRun, expectedCaretHeight)
	}
}

func TestPresentationTextEditorFrameStaysFixedWithLongText(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.frame()
	point := h.textClickPoint(110, 10)
	h.click(point)
	h.click(point)

	before := findPresentationSemanticBounds(h.router.AppendSemantics(nil), semantic.Editor, "")
	if before.Empty() {
		t.Fatal("focused slide text has no editor bounds")
	}
	h.view.shapeEditor.SetText(strings.Repeat("长", 400))
	h.frame()
	after := findPresentationSemanticBounds(h.router.AppendSemantics(nil), semantic.Editor, "")
	if after != before {
		t.Fatalf("editor viewport changed with draft length: before=%v after=%v", before, after)
	}
	wantFrame := presentationBoundsInPixels(presentationElementByID(t, h.view.currentSlide(), h.elem.ID).Bounds, h.view.canvasScale)
	if h.view.editingArea != wantFrame {
		t.Fatalf("long draft changed the object frame: area=%v want=%v", h.view.editingArea, wantFrame)
	}
}

func TestPresentationBlankCanvasClickCommitsTextAndExitsEditing(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.frame()
	textPoint := h.textClickPoint(110, 10)
	h.click(textPoint)
	h.click(textPoint)
	if h.view.editingShape != h.elem.ID {
		t.Fatal("clicking text did not enter edit mode")
	}
	h.view.shapeEditor.SetText("更新后的标题")
	h.frame()
	h.click(f32.Pt(1050, 600))

	if h.view.editingShape != 0 || h.view.selectedShape != 0 {
		t.Fatalf("blank slide click did not return to read mode: editing=%d selected=%d", h.view.editingShape, h.view.selectedShape)
	}
	if h.router.Source().Focused(&h.view.shapeEditor) {
		t.Fatal("text editor retained keyboard focus after blank click")
	}
	var committed string
	for _, elem := range h.view.currentSlide().Elements {
		if elem.ID == h.elem.ID {
			committed = elem.PlainText()
			break
		}
	}
	if committed != "更新后的标题" {
		t.Fatalf("blank slide click did not commit text: got=%q", committed)
	}
}
