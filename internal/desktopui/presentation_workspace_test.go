package desktopui

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"golang.org/x/image/math/fixed"

	"glance/internal/document"
	"glance/internal/format/pptx"
)

func TestPresentationNewBlankViaMenuAndHome(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())

	// Test home screen PPTX creation
	app.handleHomeAction("pptx")
	if app.pptxDoc == nil {
		t.Fatalf("expected pptxDoc to be created, got nil")
	}
	if app.path != "" {
		t.Fatalf("expected draft with empty path, got %q", app.path)
	}
	tab := app.tabMeta[app.activeTabID]
	if tab.Title != "新建演示文稿.pptx" {
		t.Fatalf("expected title '新建演示文稿.pptx', got %q", tab.Title)
	}
	if app.pptxDoc.SlideCount() < 1 {
		t.Fatalf("expected at least 1 slide in new presentation, got %d", app.pptxDoc.SlideCount())
	}

	// Close draft
	if err := app.closeDocument(); err != nil {
		t.Fatalf("closeDocument failed: %v", err)
	}
	if app.pptxDoc != nil {
		t.Fatalf("expected pptxDoc to be nil after closeDocument")
	}

	// Test create menu PPTX creation
	app.createNewMenuDocument("pptx")
	if app.pptxDoc == nil {
		t.Fatalf("expected pptxDoc from createNewMenuDocument, got nil")
	}
}

func TestPresentationOpenAndSnapshot(t *testing.T) {
	doc, err := pptx.NewBlank()
	if err != nil {
		t.Fatalf("NewBlank failed: %v", err)
	}
	tmpPath := filepath.Join(t.TempDir(), "sample.pptx")
	if err := doc.SaveAs(tmpPath); err != nil {
		t.Fatalf("SaveAs failed: %v", err)
	}
	_ = doc.Close()

	app := newAppWithRuntime(nil, newDesktopRuntime())
	loaded, err := loadDocument(tmpPath)
	if err != nil {
		t.Fatalf("loadDocument failed: %v", err)
	}
	if loaded.pptxDoc == nil {
		t.Fatalf("expected loaded.pptxDoc, got nil")
	}

	if err := app.installDocument(loaded); err != nil {
		t.Fatalf("installDocument failed: %v", err)
	}

	v := app.ensurePresentationView()
	if v.snapshot.SlideCount < 1 {
		t.Fatalf("expected at least 1 slide in snapshot, got %d", v.snapshot.SlideCount)
	}
	if v.snapshot.WidthPT != 960 || v.snapshot.HeightPT != 540 {
		t.Fatalf("expected 16:9 widescreen 960x540, got %.0fx%.0f", v.snapshot.WidthPT, v.snapshot.HeightPT)
	}
}

func TestPPTXLayoutMasterSamplesOpenAndRender(t *testing.T) {
	root := os.Getenv("GLANCE_PPTX_TEST_DIR")
	if root == "" {
		root = "/Users/mima1234/Downloads"
	}
	samples := []struct {
		name       string
		slideCount int
		verify     func(*testing.T, document.PresentationSnapshot)
	}{
		{name: "1.pptx", slideCount: 30, verify: func(t *testing.T, snapshot document.PresentationSnapshot) {
			t.Helper()
			if snapshot.WidthPT <= 0 || snapshot.HeightPT <= 0 || len(snapshot.Slides[0].Elements) == 0 {
				t.Fatalf("baseline deck lost dimensions or slide-local content: dimensions=%.1fx%.1f elements=%+v", snapshot.WidthPT, snapshot.HeightPT, snapshot.Slides[0].Elements)
			}
		}},
		{name: "PPT_RealTest.pptx", slideCount: 1, verify: func(t *testing.T, snapshot document.PresentationSnapshot) {
			t.Helper()
			for _, elem := range snapshot.Slides[0].InheritedElements {
				if elem.Kind == document.ElementKindPicture && elem.Image != nil && elem.Bounds.W > 0 && elem.Bounds.H > 0 {
					return
				}
			}
			t.Fatalf("selected layout image is missing or has no geometry: inherited=%+v", snapshot.Slides[0].InheritedElements)
		}},
		{name: "slidetemplate.pptx", slideCount: 3, verify: func(t *testing.T, snapshot document.PresentationSnapshot) {
			t.Helper()
			slide := snapshot.Slides[0]
			if slide.BackgroundImage == nil {
				t.Fatal("explicit slide background image is missing")
			}
			var inheritedPlaceholderBounds int
			for _, elem := range slide.Elements {
				if elem.Kind == document.ElementKindText && elem.Bounds.W > 0 && elem.Bounds.H > 0 {
					inheritedPlaceholderBounds++
				}
			}
			if inheritedPlaceholderBounds < 2 {
				t.Fatalf("title/subtitle placeholder geometry was not inherited: %d text elements, all=%+v", inheritedPlaceholderBounds, slide.Elements)
			}
		}},
		{name: "test-template.pptx", slideCount: 3, verify: func(t *testing.T, snapshot document.PresentationSnapshot) {
			t.Helper()
			background := parsePresentationHexColor(snapshot.Slides[0].BgColorHex, color.NRGBA{A: 255})
			luminance := 0.2126*float64(background.R) + 0.7152*float64(background.G) + 0.0722*float64(background.B)
			if luminance >= 100 {
				t.Fatalf("master theme background is not dark: %s => %#v", snapshot.Slides[0].BgColorHex, background)
			}
		}},
	}

	for _, sample := range samples {
		sample := sample
		t.Run(sample.name, func(t *testing.T) {
			path := filepath.Join(root, sample.name)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Skipf("missing sample file %s: %v", sample.name, err)
			}
			loaded, err := loadDocument(path)
			if err != nil {
				t.Fatalf("loadDocument(%s): %v", sample.name, err)
			}
			app := newAppWithRuntime(nil, newDesktopRuntime())
			app.theme = material.NewTheme()
			t.Cleanup(func() {
				if app.pptxDoc != nil {
					_ = app.closeDocument()
				}
				close(app.shutdown)
			})
			if err := app.installDocument(loaded); err != nil {
				t.Fatalf("installDocument(%s): %v", sample.name, err)
			}
			view := app.ensurePresentationView()
			if view.snapshot.SlideCount != sample.slideCount {
				t.Fatalf("slide count = %d, want %d", view.snapshot.SlideCount, sample.slideCount)
			}
			if len(view.snapshot.Slides) == 0 {
				t.Fatal("snapshot has no first slide")
			}
			sample.verify(t, view.snapshot)

			var ops op.Ops
			gtx := layout.Context{
				Ops:         &ops,
				Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
				Constraints: layout.Exact(image.Pt(1200, 700)),
			}
			dims := app.layoutPresentationCanvas(gtx, view)
			if dims.Size.X <= 0 || dims.Size.Y <= 0 {
				t.Fatalf("first slide did not produce canvas layout: dims=%v", dims.Size)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read sample after open: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("opening/rendering %s modified the source bytes", sample.name)
			}
		})
	}
}

func TestPresentationSlideOperations(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())
	app.handleHomeAction("pptx")
	v := app.ensurePresentationView()

	initialCount := v.snapshot.SlideCount

	// Add slide
	app.handlePresentationAddSlide(v)
	if v.snapshot.SlideCount != initialCount+1 {
		t.Fatalf("expected slide count %d, got %d", initialCount+1, v.snapshot.SlideCount)
	}
	if v.activeSlide != v.snapshot.SlideCount-1 {
		t.Fatalf("expected active slide %d, got %d", v.snapshot.SlideCount-1, v.activeSlide)
	}

	// Delete slide
	app.handlePresentationDeleteSlide(v)
	if v.snapshot.SlideCount != initialCount {
		t.Fatalf("expected slide count %d after deletion, got %d", initialCount, v.snapshot.SlideCount)
	}

	// Attempting to delete the only slide should be prevented
	app.handlePresentationDeleteSlide(v)
	if v.snapshot.SlideCount != initialCount {
		t.Fatalf("expected last slide to remain, got %d", v.snapshot.SlideCount)
	}
}

func TestPresentationTextAndNotesEditing(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())
	doc, err := pptx.NewBlank()
	if err != nil {
		t.Fatalf("NewBlank failed: %v", err)
	}
	tmpPath := filepath.Join(t.TempDir(), "editable.pptx")
	if err := doc.SaveAs(tmpPath); err != nil {
		t.Fatalf("SaveAs failed: %v", err)
	}
	_ = doc.Close()

	loaded, err := loadDocument(tmpPath)
	if err != nil {
		t.Fatalf("loadDocument failed: %v", err)
	}
	if err := app.installDocument(loaded); err != nil {
		t.Fatalf("installDocument failed: %v", err)
	}
	v := app.ensurePresentationView()

	// Update text on first element
	if len(v.snapshot.Slides[0].Elements) > 0 {
		elemID := v.snapshot.Slides[0].Elements[0].ID
		if err := app.pptxDoc.UpdateSlideText(0, elemID, "编辑后的标题"); err != nil {
			t.Fatalf("UpdateSlideText failed: %v", err)
		}
	}

	// Update speaker notes
	if err := app.pptxDoc.UpdateSlideNotes(0, "编辑后的演讲者备注"); err != nil {
		t.Fatalf("UpdateSlideNotes failed: %v", err)
	}

	if !app.isDirty() {
		t.Fatalf("expected document to be dirty after edits")
	}

	// Save
	if err := app.save(); err != nil {
		t.Fatalf("app.save() failed: %v", err)
	}
	if app.isDirty() {
		t.Fatalf("expected document to be clean after save")
	}

	// Reopen and verify
	reloaded, err := pptx.Open(tmpPath)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer reloaded.Close()

	snap := reloaded.Snapshot()
	if snap.Slides[0].Notes != "编辑后的演讲者备注" {
		t.Fatalf("notes mismatch after reopen: got %q", snap.Slides[0].Notes)
	}
}

func TestPresentationTextShapeClickEntersFocusedEditorAndAcceptsTyping(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())
	app.theme = material.NewTheme()
	tmpPath := filepath.Join(t.TempDir(), "editable-copy.pptx")
	seed, err := pptx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.SaveAs(tmpPath); err != nil {
		t.Fatal(err)
	}
	_ = seed.Close()
	loaded, err := loadDocument(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.installDocument(loaded); err != nil {
		t.Fatal(err)
	}
	doc := app.pptxDoc
	defer func() {
		_ = app.closeDocument()
		close(app.shutdown)
	}()
	v := app.ensurePresentationView()
	v.fitMode = false
	v.zoom = 1
	slide := v.currentSlide()
	if slide == nil || len(slide.Elements) == 0 {
		t.Fatal("new presentation has no text element")
	}
	elem := slide.Elements[0]
	if elem.Kind != document.ElementKindText {
		t.Fatalf("first element kind = %q, want text", elem.Kind)
	}

	router := new(input.Router)
	var ops op.Ops
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1200, 700)),
		}
		app.layoutPresentationElement(gtx, v, elem, 1)
		router.Frame(&ops)
	}

	frame()
	point := f32.Pt(float32(int(elem.Bounds.X)+8), float32(int(elem.Bounds.Y)+8))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point})
	frame()
	if v.editingShape != elem.ID {
		t.Fatalf("clicking text shape did not enter edit mode: editing=%d shape=%d", v.editingShape, elem.ID)
	}
	if !router.Source().Focused(&v.shapeEditor) {
		t.Fatal("clicking text shape did not focus the in-place editor")
	}

	router.Queue(key.EditEvent{Text: "X"})
	frame()
	if got := v.shapeEditor.Text(); !strings.Contains(got, "X") {
		t.Fatalf("typing after shape click did not update editor: %q", got)
	}
	frame() // Gio delivers editor input during layout; the draft is captured on the following frame.
	if !strings.Contains(v.shapeDraftText, "X") {
		t.Fatalf("typed text was not retained as an editable draft: %q", v.shapeDraftText)
	}
	if !app.isDirty() {
		t.Fatal("typing into a focused slide text box was not treated as unsaved work")
	}
	var gotText string
	for _, candidate := range doc.Snapshot().Slides[0].Elements {
		if candidate.ID == elem.ID {
			gotText = candidate.PlainText()
		}
	}
	if gotText != elem.PlainText() {
		t.Fatalf("typing changed the package before the save/commit boundary: got=%q want=%q", gotText, elem.PlainText())
	}
	app.startSave()
	select {
	case result := <-app.saveResults:
		if result.err != nil {
			t.Fatalf("toolbar save failed to commit focused text draft: %v", result.err)
		}
		app.saveResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("toolbar save did not finish with a focused text draft")
	}
	reopened, err := pptx.Open(tmpPath)
	if err != nil {
		t.Fatalf("reopen saved copy: %v", err)
	}
	defer reopened.Close()
	for _, candidate := range reopened.Snapshot().Slides[0].Elements {
		if candidate.ID == elem.ID {
			gotText = candidate.PlainText()
			break
		}
	}
	if gotText != v.shapeEditor.Text() {
		t.Fatalf("saved text = %q, want editor text %q", gotText, v.shapeEditor.Text())
	}
	if app.isDirty() {
		t.Fatal("document remained dirty after the toolbar save completed")
	}
}

func TestPresentationTextShapeClickPlacesCaretAtClickedText(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.frame()

	// The blank deck's title starts at this slide coordinate. Click well inside
	// the visible glyph run rather than in the much wider placeholder box.
	point := h.textClickPoint(110, 10)
	h.click(point)

	if h.view.editingShape != h.elem.ID || !h.router.Source().Focused(&h.view.shapeEditor) {
		semanticID, semanticHit := h.router.SemanticAt(point)
		t.Fatalf("clicking slide text did not focus its editor: editing=%d focused=%v point=%v semantic=%d/%v clicks=%v", h.view.editingShape, h.router.Source().Focused(&h.view.shapeEditor), point, semanticID, semanticHit, h.view.shapeClick(h.elem.ID).History())
	}
	_, caret := h.view.shapeEditor.Selection()
	length := len([]rune(h.elem.PlainText()))
	if caret <= length/2 || caret >= length {
		t.Fatalf("click near the end of slide text placed the caret at rune %d of %d", caret, length)
	}
}

func TestPresentationTextEditorClickMovesCaret(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.frame()
	h.click(h.textClickPoint(110, 10))
	if h.view.editingShape != h.elem.ID {
		t.Fatal("clicking slide text did not enter edit mode")
	}
	_, initialCaret := h.view.shapeEditor.Selection()
	editorBounds := findPresentationSemanticBounds(h.router.AppendSemantics(nil), semantic.Editor, "")
	if editorBounds.Empty() {
		t.Fatal("focused slide text has no editor bounds")
	}
	point := f32.Pt(float32(editorBounds.Min.X+20), float32(editorBounds.Min.Y+12))
	h.click(point)
	_, clickedCaret := h.view.shapeEditor.Selection()
	if clickedCaret >= initialCaret {
		t.Fatalf("clicking near the start of the editor did not move the caret left: before=%d after=%d bounds=%v point=%v focused=%v", initialCaret, clickedCaret, editorBounds, point, h.router.Source().Focused(&h.view.shapeEditor))
	}
}

func TestPresentationTextEditorFrameFitsShortText(t *testing.T) {
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
	h.click(h.textClickPoint(60, 10))
	bounds := findPresentationSemanticBounds(h.router.AppendSemantics(nil), semantic.Editor, "")
	if bounds.Empty() {
		t.Fatal("focused slide text has no editor bounds")
	}
	if h.view.editingArea.Dx() >= 300 || h.view.editingArea.Dy() >= 100 {
		t.Fatalf("short text editor extended beyond its content: semantic=%v area=%v", bounds, h.view.editingArea)
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
	handleSize := 6
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
	caretX := frame.Min.X + 8 + int(math.Round(float64(caret.X)))
	caretY := frame.Min.Y + 4 + int(math.Round(float64(caret.Y)))
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

type presentationCanvasHarness struct {
	app    *App
	view   *presentationViewState
	elem   document.PresentationElement
	router *input.Router
	ops    *op.Ops
	frame  func()
}

func newPresentationCanvasHarness(t *testing.T) presentationCanvasHarness {
	t.Helper()
	app := newAppWithRuntime(nil, newDesktopRuntime())
	app.theme = material.NewTheme()
	tmpPath := filepath.Join(t.TempDir(), "editable-copy.pptx")
	seed, err := pptx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.SaveAs(tmpPath); err != nil {
		t.Fatal(err)
	}
	_ = seed.Close()
	loaded, err := loadDocument(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.installDocument(loaded); err != nil {
		t.Fatal(err)
	}
	view := app.ensurePresentationView()
	view.fitMode = false
	view.zoom = 1
	slide := view.currentSlide()
	if slide == nil || len(slide.Elements) == 0 || slide.Elements[0].Kind != document.ElementKindText {
		t.Fatal("new presentation has no text element on its first slide")
	}
	router := new(input.Router)
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1200, 700)),
		}
		app.layoutPresentationCanvas(gtx, view)
		router.Frame(&ops)
	}
	t.Cleanup(func() {
		_ = app.closeDocument()
		close(app.shutdown)
	})
	return presentationCanvasHarness{app: app, view: view, elem: slide.Elements[0], router: router, ops: &ops, frame: frame}
}

func (h presentationCanvasHarness) click(point f32.Point) {
	h.router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point, Time: time.Duration(time.Now().UnixNano())})
	h.frame()
	h.router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point, Time: time.Duration(time.Now().UnixNano())})
	h.frame()
}

func (h presentationCanvasHarness) textClickPoint(offsetX, offsetY int) f32.Point {
	area := h.view.shapeHitAreas[h.elem.ID]
	origin := h.view.canvasCardOrigin.Add(area.elementOrigin).Add(area.textOrigin)
	return f32.Pt(float32(origin.X+offsetX), float32(origin.Y+offsetY))
}

func findPresentationSemanticBounds(nodes []input.SemanticNode, class semantic.ClassOp, description string) image.Rectangle {
	var bounds image.Rectangle
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Class == class && (description == "" || node.Desc.Description == description || node.Desc.Label == description) {
				bounds = node.Desc.Bounds
				return
			}
			visit(node.Children)
		}
	}
	visit(nodes)
	return bounds
}

func TestPresentationTextEditorFrameExpandsWithLongText(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.frame()
	point := h.textClickPoint(110, 10)
	h.click(point)

	before := findPresentationSemanticBounds(h.router.AppendSemantics(nil), semantic.Editor, "")
	if before.Empty() {
		t.Fatal("focused slide text has no editor bounds")
	}
	h.view.shapeEditor.SetText(strings.Repeat("长", 400))
	h.frame()
	after := findPresentationSemanticBounds(h.router.AppendSemantics(nil), semantic.Editor, "")
	if after.Dx() <= before.Dx() || after.Dy() <= before.Dy() {
		t.Fatalf("editor viewport did not grow with its text: before=%v after=%v", before, after)
	}

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
	borderY := h.view.canvasCardOrigin.Y + h.view.editingArea.Min.Y
	var expandedBorderPixels int
	for y := max(0, borderY-1); y <= min(screenshot.Bounds().Max.Y-1, borderY+1); y++ {
		for x := max(0, before.Max.X+5); x < min(screenshot.Bounds().Max.X, after.Max.X+5); x++ {
			c := screenshot.RGBAAt(x, y)
			if int(c.B) > int(c.R)+70 && int(c.B) > int(c.G)+70 {
				expandedBorderPixels++
			}
		}
	}
	if expandedBorderPixels < after.Max.X-before.Max.X {
		t.Fatalf("expanded editor has no visible border across its added width: before=%v after=%v", before, after)
	}
}

func TestPresentationBlankCanvasClickCommitsTextAndExitsEditing(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.frame()
	textPoint := h.textClickPoint(110, 10)
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

func TestPresentationSaveAsAndExtensionValidation(t *testing.T) {
	if err := validateSaveAsExtension("pptx", "presentation.pptx"); err != nil {
		t.Fatalf("validateSaveAsExtension pptx failed: %v", err)
	}
	if err := validateSaveAsExtension("pptx", "presentation.docx"); err == nil {
		t.Fatalf("expected error for mismatching extension, got nil")
	}

	doc, err := pptx.NewBlank()
	if err != nil {
		t.Fatalf("NewBlank failed: %v", err)
	}
	defer doc.Close()

	tmpPath := filepath.Join(t.TempDir(), "savedas.pptx")
	if err := doc.SaveAs(tmpPath); err != nil {
		t.Fatalf("SaveAs failed: %v", err)
	}
	if _, err := os.Stat(tmpPath); err != nil {
		t.Fatalf("file not found after SaveAs: %v", err)
	}
}

func TestPresentationLayoutRendering(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())
	app.handleHomeAction("pptx")
	v := app.ensurePresentationView()

	// 1440x900 viewport matching Pen frame C2TtCZ
	size := image.Pt(1440, 900)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	gtx := layout.Context{Ops: &ops, Metric: metric, Constraints: layout.Exact(size)}

	// 1. Render default Home ribbon & workspace
	dims := app.layoutPresentationDocument(gtx)
	if dims.Size.X != 1440 || dims.Size.Y != 900 {
		t.Fatalf("expected layout dims 1440x900, got %dx%d", dims.Size.X, dims.Size.Y)
	}

	// 2. Render each ribbon tab
	tabs := []string{"file", "insert", "design", "slideshow", "view", "home"}
	for _, tab := range tabs {
		v.activeRibbonTab = tab
		ops.Reset()
		dims := app.layoutPresentationDocument(gtx)
		if dims.Size.X != 1440 || dims.Size.Y != 900 {
			t.Fatalf("tab %s: expected dims 1440x900, got %dx%d", tab, dims.Size.X, dims.Size.Y)
		}
	}

	// 3. Render with notes panel expanded
	v.notesExpanded = true
	ops.Reset()
	dims = app.layoutPresentationDocument(gtx)
	if dims.Size.X != 1440 || dims.Size.Y != 900 {
		t.Fatalf("notes expanded: expected dims 1440x900, got %dx%d", dims.Size.X, dims.Size.Y)
	}

	// 4. Render slideshow mode
	v.slideshowMode = true
	ops.Reset()
	dims = app.layoutPresentationDocument(gtx)
	if dims.Size.X != 1440 || dims.Size.Y != 900 {
		t.Fatalf("slideshow mode: expected dims 1440x900, got %dx%d", dims.Size.X, dims.Size.Y)
	}

	// 5. Exit slideshow mode
	v.slideshowMode = false
	ops.Reset()
	dims = app.layoutPresentationDocument(gtx)
	if dims.Size.X != 1440 || dims.Size.Y != 900 {
		t.Fatalf("exit slideshow: expected dims 1440x900, got %dx%d", dims.Size.X, dims.Size.Y)
	}
}

func TestPresentationRibbonIconsLoadValid(t *testing.T) {
	allTabs := []string{"home", "file", "insert", "design", "transitions", "animations", "slideshow", "review", "view"}
	color := pptxTextSecondary

	for _, tab := range allTabs {
		actions := pptxActionsForTab(tab)
		if len(actions) == 0 {
			t.Fatalf("tab %s has no actions", tab)
		}
		for _, action := range actions {
			img, ok := pptxToolbarIcon(action.icon, color)
			if !ok || img == nil {
				t.Fatalf("tab %s action %s (%s) failed to load icon %q", tab, action.id, action.label, action.icon)
			}
			if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
				t.Fatalf("tab %s action %s icon %q has zero bounds", tab, action.id, action.icon)
			}
		}
	}
}

func TestPresentationAddSlideButtonLayoutAndClick(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())
	app.handleHomeAction("pptx")
	v := app.ensurePresentationView()

	initialSlides := app.pptxDoc.SlideCount()
	if initialSlides < 1 {
		t.Fatalf("expected at least 1 slide initially, got %d", initialSlides)
	}

	size := image.Pt(248, 900)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	gtx := layout.Context{Ops: &ops, Metric: metric, Constraints: layout.Exact(size)}

	// 1. Render sidebar
	dims := app.layoutPresentationSidebar(gtx, v)
	if dims.Size.X != 248 || dims.Size.Y != 900 {
		t.Fatalf("expected sidebar dims 248x900, got %dx%d", dims.Size.X, dims.Size.Y)
	}

	// 2. Click "+ 新建" button
	v.addSlideButton.Click()
	ops.Reset()
	app.layoutPresentationSidebar(gtx, v)

	if got := app.pptxDoc.SlideCount(); got != initialSlides+1 {
		t.Fatalf("expected slide count %d after clicking add slide button, got %d", initialSlides+1, got)
	}
}

func TestPresentationAddSlideButtonInkAlignment(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())
	app.handleHomeAction("pptx")
	v := app.ensurePresentationView()

	size := image.Pt(248, 900)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	gtx := layout.Context{Ops: &ops, Metric: metric, Constraints: layout.Exact(size)}

	app.layoutPresentationSidebar(gtx, v)

	window, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	if err := window.Frame(&ops); err != nil {
		t.Fatal(err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	if err := window.Screenshot(screenshot); err != nil {
		t.Fatal(err)
	}

	// Scan only header area (x: 160..235, y: 10..46)
	var btnMinY, btnMaxY int = 999, -1
	var btnMinX, btnMaxX int = 999, -1
	var textMinY, textMaxY int = 999, -1
	var plusMinY, plusMaxY int = 999, -1
	var labelMinY, labelMaxY int = 999, -1 // "幻灯片" at x: 16..70

	for y := 10; y <= 45; y++ {
		// Scan "幻灯片" label (x: 16..70)
		for x := 16; x <= 70; x++ {
			c := screenshot.RGBAAt(x, y)
			if int(c.B) > 180 && int(c.B) > int(c.R)+40 {
				if y < labelMinY {
					labelMinY = y
				}
				if y > labelMaxY {
					labelMaxY = y
				}
			}
		}

		// Scan button (x: 160..235)
		for x := 160; x <= 235; x++ {
			c := screenshot.RGBAAt(x, y)
			// Border color pptxBorderColor (R: 217, G: 221, B: 227) or button interior (R: 255)
			if (c.R > 250 && c.G > 250 && c.B > 250) || (c.R > 210 && c.R < 225 && c.G > 215 && c.G < 230 && c.B > 220) {
				if y < btnMinY {
					btnMinY = y
				}
				if y > btnMaxY {
					btnMaxY = y
				}
				if x < btnMinX {
					btnMinX = x
				}
				if x > btnMaxX {
					btnMaxX = x
				}
			}
			// Purple ink in button
			if int(c.B) > 180 && int(c.B) > int(c.R)+40 {
				if x < 190 {
					if y < plusMinY {
						plusMinY = y
					}
					if y > plusMaxY {
						plusMaxY = y
					}
				} else {
					if y < textMinY {
						textMinY = y
					}
					if y > textMaxY {
						textMaxY = y
					}
				}
			}
		}
	}

	t.Logf("Button bounds: Y=[%d, %d] (H=%d, Center=%.1f), X=[%d, %d] (W=%d)",
		btnMinY, btnMaxY, btnMaxY-btnMinY+1, float64(btnMinY+btnMaxY)/2.0, btnMinX, btnMaxX, btnMaxX-btnMinX+1)
	t.Logf("'幻灯片' ink:   Y=[%d, %d] (H=%d, Center=%.1f)",
		labelMinY, labelMaxY, labelMaxY-labelMinY+1, float64(labelMinY+labelMaxY)/2.0)
	t.Logf("'+' plus ink:  Y=[%d, %d] (H=%d, Center=%.1f)",
		plusMinY, plusMaxY, plusMaxY-plusMinY+1, float64(plusMinY+plusMaxY)/2.0)
	t.Logf("'新建' ink:    Y=[%d, %d] (H=%d, Center=%.1f)",
		textMinY, textMaxY, textMaxY-textMinY+1, float64(textMinY+textMaxY)/2.0)

	btnCenter := float64(btnMinY+btnMaxY) / 2.0
	labelCenter := float64(labelMinY+labelMaxY) / 2.0
	plusCenter := float64(plusMinY+plusMaxY) / 2.0
	textCenter := float64(textMinY+textMaxY) / 2.0

	// Assert that '幻灯片' ink, button center, '+' ink, and '新建' ink all align within 2 pixels
	if math.Abs(btnCenter-textCenter) > 2.0 {
		t.Fatalf("button center (%.1f) and '新建' text center (%.1f) deviate by more than 2px", btnCenter, textCenter)
	}
	if math.Abs(plusCenter-textCenter) > 2.0 {
		t.Fatalf("'+' center (%.1f) and '新建' text center (%.1f) deviate by more than 2px", plusCenter, textCenter)
	}
	if math.Abs(labelCenter-btnCenter) > 2.5 {
		t.Fatalf("'幻灯片' center (%.1f) and button center (%.1f) deviate by more than 2.5px", labelCenter, btnCenter)
	}
}

func TestPresentationRenderSample1PPTX(t *testing.T) {
	samplePath := "/Users/mima1234/Downloads/1.pptx"
	if _, err := os.Stat(samplePath); err != nil {
		t.Skipf("1.pptx not found: %v", err)
	}

	app := newAppWithRuntime(nil, newDesktopRuntime())
	loaded, err := loadDocument(samplePath)
	if err != nil {
		t.Fatalf("loadDocument 1.pptx failed: %v", err)
	}
	if err := app.installDocument(loaded); err != nil {
		t.Fatalf("installDocument failed: %v", err)
	}
	v := app.ensurePresentationView()
	v.activeSlide = 0

	// 1. Layout Slide 0 ("Title")
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Constraints: layout.Exact(image.Pt(1440, 900)),
		Metric:      unit.Metric{PxPerDp: 1.0, PxPerSp: 1.0},
	}
	dims := app.layoutPresentationDocument(gtx)
	if dims.Size.X != 1440 || dims.Size.Y != 900 {
		t.Fatalf("Slide 0 render dims expected 1440x900, got %dx%d", dims.Size.X, dims.Size.Y)
	}

	// 2. Layout Slide 1 ("Outline")
	v.activeSlide = 1
	ops.Reset()
	dims = app.layoutPresentationDocument(gtx)
	if dims.Size.X != 1440 || dims.Size.Y != 900 {
		t.Fatalf("Slide 1 render dims expected 1440x900, got %dx%d", dims.Size.X, dims.Size.Y)
	}

	// 3. Layout Slide 5 (Animation with pause & meanwhile)
	v.activeSlide = 5
	ops.Reset()
	dims = app.layoutPresentationDocument(gtx)
	if dims.Size.X != 1440 || dims.Size.Y != 900 {
		t.Fatalf("Slide 5 render dims expected 1440x900, got %dx%d", dims.Size.X, dims.Size.Y)
	}
}
