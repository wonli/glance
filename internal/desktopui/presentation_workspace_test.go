package desktopui

import (
	"image"

	"path/filepath"

	"strings"
	"testing"
	"time"

	"gioui.org/f32"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"

	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/pptx"
)

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
	point := f32.Pt(float32(int(elem.Bounds.X)+110), float32(int(elem.Bounds.Y)+10))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point})
	frame()
	if v.editingShape == 0 {
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point})
		frame()
	}
	if v.editingShape != elem.ID {
		t.Fatalf("selecting then clicking text shape did not enter edit mode: editing=%d selected=%d shape=%d history=%+v hit=%+v", v.editingShape, v.selectedShape, elem.ID, v.shapeClick(elem.ID).History(), v.shapeHitAreas[elem.ID])
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
	point := h.textClickPoint(110, 10)
	h.click(point)
	h.click(point)
	if h.view.editingShape != h.elem.ID {
		t.Fatal("clicking slide text did not enter edit mode")
	}
	_, initialCaret := h.view.shapeEditor.Selection()
	editorBounds := findPresentationSemanticBounds(h.router.AppendSemantics(nil), semantic.Editor, "")
	if editorBounds.Empty() {
		t.Fatal("focused slide text has no editor bounds")
	}
	point = f32.Pt(float32(editorBounds.Min.X+20), float32(editorBounds.Min.Y+12))
	h.click(point)
	_, clickedCaret := h.view.shapeEditor.Selection()
	if clickedCaret >= initialCaret {
		t.Fatalf("clicking near the start of the editor did not move the caret left: before=%d after=%d bounds=%v point=%v focused=%v", initialCaret, clickedCaret, editorBounds, point, h.router.Source().Focused(&h.view.shapeEditor))
	}
}

func TestPresentationCenteredTextEditorClickPlacesCaretByVisibleGlyph(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	elem := &h.view.snapshot.Slides[0].Elements[0]
	elem.Bounds = document.PresentationRect{X: 100, Y: 100, W: 480, H: 220}
	elem.PlaceholderType = ""
	elem.WrapText = false
	elem.Paragraphs = []document.PresentationParagraph{{Align: "center", Runs: []document.PresentationTextRun{{Text: "Center aligned title", SizePT: 36}}}}
	h.elem = *elem
	h.frame()

	// The displayed text is centered in the object. Clicking its visual center
	// while editing should place the caret near the middle of the text, too.
	center := presentationCanvasPoint(h.view, 340, 130)
	h.click(center)
	h.click(center)
	if h.view.editingShape != h.elem.ID {
		t.Fatal("centered PPTX text did not enter edit mode")
	}
	h.click(center)
	_, caret := h.view.shapeEditor.Selection()
	textRunes := len([]rune(h.view.shapeEditor.Text()))
	if caret < textRunes/2-1 || caret > textRunes/2+1 {
		t.Fatalf("clicking the visible center placed caret at rune %d of %d", caret, textRunes)
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
	tmpPath := filepath.Join(t.TempDir(), "editable-copy.pptx")
	seed, err := pptx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.SaveAs(tmpPath); err != nil {
		t.Fatal(err)
	}
	_ = seed.Close()
	return newPresentationCanvasHarnessFromPath(t, tmpPath)
}

func newPresentationCanvasHarnessFromPath(t *testing.T, path string) presentationCanvasHarness {
	t.Helper()
	app := newAppWithRuntime(nil, newDesktopRuntime())
	app.theme = material.NewTheme()
	loaded, err := loadDocument(path)
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
