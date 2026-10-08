package desktopui

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"reflect"
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
			placeholders := make(map[string]document.PresentationElement)
			for _, elem := range snapshot.Slides[0].Elements {
				if elem.PlaceholderType == "ctrTitle" || elem.PlaceholderType == "subTitle" {
					placeholders[elem.PlaceholderType] = elem
				}
			}
			for _, placeholderType := range []string{"ctrTitle", "subTitle"} {
				elem, ok := placeholders[placeholderType]
				if !ok || elem.Kind != document.ElementKindText || elem.PlainText() != "" || elem.Bounds.W <= 0 || elem.Bounds.H <= 0 {
					t.Fatalf("editable blank %s placeholder is missing or has no inherited geometry: found=%t element=%+v", placeholderType, ok, elem)
				}
			}
			pictures := make(map[string]bool)
			for _, elem := range snapshot.Slides[0].InheritedElements {
				if elem.PlainText() == "‹#›" {
					t.Fatal("inherited slide-number field leaked its raw placeholder marker")
				}
				if elem.Kind == document.ElementKindPicture && elem.Image != nil && elem.Bounds.W > 0 && elem.Bounds.H > 0 {
					pictures[elem.AltText] = true
				}
			}
			for _, altText := range []string{
				"090512_118_tk_sr.jpg",
				"vcu-ppt-footer-cover-4-3.eps",
				"bm_UnivRelations_RF_hz_rgb.eps",
				"vcu-ppt-footer-gray.eps",
			} {
				if !pictures[altText] {
					t.Fatalf("inherited picture %q is missing or undecoded; pictures=%v elements=%+v", altText, pictures, snapshot.Slides[0].InheritedElements)
				}
			}
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
			if sample.name == "PPT_RealTest.pptx" {
				slide := view.currentSlide()
				var cover document.PresentationElement
				for _, elem := range slide.InheritedElements {
					if elem.AltText == "vcu-ppt-footer-cover-4-3.eps" {
						cover = elem
						break
					}
				}
				if cover.Image == nil {
					t.Fatal("footer cover EMF did not decode before canvas rendering")
				}
				withCover := renderPresentationTestOps(t, &ops, image.Pt(1200, 700))
				original := slide.InheritedElements
				slide.InheritedElements = make([]document.PresentationElement, 0, len(original)-1)
				for _, elem := range original {
					if elem.AltText != cover.AltText {
						slide.InheritedElements = append(slide.InheritedElements, elem)
					}
				}
				var withoutCoverOps op.Ops
				withoutCoverContext := layout.Context{
					Ops:         &withoutCoverOps,
					Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
					Constraints: layout.Exact(image.Pt(1200, 700)),
				}
				app.layoutPresentationCanvas(withoutCoverContext, view)
				withoutCover := renderPresentationTestOps(t, &withoutCoverOps, image.Pt(1200, 700))
				slide.InheritedElements = original
				box := image.Rect(
					view.canvasCardOrigin.X+int(cover.Bounds.X*view.canvasScale),
					view.canvasCardOrigin.Y+int(cover.Bounds.Y*view.canvasScale),
					view.canvasCardOrigin.X+int((cover.Bounds.X+cover.Bounds.W)*view.canvasScale),
					view.canvasCardOrigin.Y+int((cover.Bounds.Y+cover.Bounds.H)*view.canvasScale),
				).Intersect(withCover.Bounds())
				var changedPixels int
				for y := box.Min.Y; y < box.Max.Y; y++ {
					for x := box.Min.X; x < box.Max.X; x++ {
						if withCover.RGBAAt(x, y) != withoutCover.RGBAAt(x, y) {
							changedPixels++
						}
					}
				}
				if changedPixels < 500 {
					t.Fatalf("footer cover EMF changed only %d canvas pixels in %v", changedPixels, box)
				}
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

func TestPPTXRealTestTitleAndSubtitleEditRoundTrip(t *testing.T) {
	root := os.Getenv("GLANCE_PPTX_TEST_DIR")
	if root == "" {
		root = "/Users/mima1234/Downloads"
	}
	path := filepath.Join(root, "PPT_RealTest.pptx")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("missing sample file %s: %v", path, err)
	}
	doc, err := pptx.Open(path)
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}
	defer doc.Close()
	snapshot := doc.Snapshot()
	ids := make(map[string]int64)
	for _, elem := range snapshot.Slides[0].Elements {
		if elem.PlaceholderType == "ctrTitle" || elem.PlaceholderType == "subTitle" {
			ids[elem.PlaceholderType] = elem.ID
		}
	}
	values := map[string]string{"ctrTitle": "可编辑标题", "subTitle": "可编辑副标题"}
	for _, placeholderType := range []string{"ctrTitle", "subTitle"} {
		id, ok := ids[placeholderType]
		if !ok {
			t.Fatalf("missing %s placeholder in source sample", placeholderType)
		}
		if err := doc.UpdateSlideText(0, id, values[placeholderType]); err != nil {
			t.Fatalf("update %s placeholder: %v", placeholderType, err)
		}
	}
	output := filepath.Join(t.TempDir(), "PPT_RealTest-edited.pptx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatalf("save edited sample copy: %v", err)
	}
	reopened, err := pptx.Open(output)
	if err != nil {
		t.Fatalf("reopen edited sample copy: %v", err)
	}
	defer reopened.Close()
	got := make(map[string]document.PresentationElement)
	for _, elem := range reopened.Snapshot().Slides[0].Elements {
		if elem.PlaceholderType != "" {
			got[elem.PlaceholderType] = elem
		}
	}
	for placeholderType, want := range values {
		elem := got[placeholderType]
		if elem.PlainText() != want {
			t.Errorf("reopened %s text = %q, want %q", placeholderType, elem.PlainText(), want)
		}
		if len(elem.Paragraphs) == 0 || len(elem.Paragraphs[0].Runs) == 0 || elem.Paragraphs[0].Runs[0].SizePT < 30 {
			t.Errorf("reopened %s text did not inherit placeholder-sized typography: %+v", placeholderType, elem.Paragraphs)
		}
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("editing sample placeholders changed the source file: err=%v", err)
	}
}

func TestPPTXRealTestPlaceholdersRenderPromptsAndAcceptText(t *testing.T) {
	root := os.Getenv("GLANCE_PPTX_TEST_DIR")
	if root == "" {
		root = "/Users/mima1234/Downloads"
	}
	path := filepath.Join(root, "PPT_RealTest.pptx")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("missing sample file %s: %v", path, err)
	}
	h := newPresentationCanvasHarnessFromPath(t, path)
	placeholders := make(map[string]document.PresentationElement)
	for _, elem := range h.view.currentSlide().Elements {
		if elem.PlaceholderType == "ctrTitle" || elem.PlaceholderType == "subTitle" {
			placeholders[elem.PlaceholderType] = elem
		}
	}
	wantPrompts := map[string]string{
		"ctrTitle": "单击此处添加标题",
		"subTitle": "单击此处添加副标题",
	}
	for placeholderType := range wantPrompts {
		if _, ok := placeholders[placeholderType]; !ok {
			t.Fatalf("real sample has no %s placeholder", placeholderType)
		}
	}

	h.frame()
	withPrompts := renderPresentationTestOps(t, h.ops, image.Pt(1200, 700))
	for placeholderType, label := range wantPrompts {
		elem := placeholders[placeholderType]
		if got := presentationElementDisplayParagraphs(elem)[0].Runs[0].Text; got != label {
			t.Fatalf("%s prompt = %q, want %q", placeholderType, got, label)
		}
		for i := range h.view.currentSlide().Elements {
			if h.view.currentSlide().Elements[i].ID == elem.ID {
				h.view.currentSlide().Elements[i].PlaceholderType = ""
			}
		}
	}
	h.frame()
	withoutPrompts := renderPresentationTestOps(t, h.ops, image.Pt(1200, 700))
	for placeholderType, elem := range placeholders {
		box := image.Rect(
			h.view.canvasCardOrigin.X+int(elem.Bounds.X*h.view.canvasScale),
			h.view.canvasCardOrigin.Y+int(elem.Bounds.Y*h.view.canvasScale),
			h.view.canvasCardOrigin.X+int((elem.Bounds.X+elem.Bounds.W)*h.view.canvasScale),
			h.view.canvasCardOrigin.Y+int((elem.Bounds.Y+elem.Bounds.H)*h.view.canvasScale),
		).Intersect(withPrompts.Bounds())
		var changed int
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				if withPrompts.RGBAAt(x, y) != withoutPrompts.RGBAAt(x, y) {
					changed++
				}
			}
		}
		if changed < 10 {
			t.Fatalf("%s prompt changed only %d rendered pixels in %v", placeholderType, changed, box)
		}
		for i := range h.view.currentSlide().Elements {
			if h.view.currentSlide().Elements[i].ID == elem.ID {
				h.view.currentSlide().Elements[i].PlaceholderType = placeholderType
			}
		}
	}
	h.frame()

	for placeholderType, text := range map[string]string{"ctrTitle": "真实封面标题", "subTitle": "真实封面副标题"} {
		elem := placeholders[placeholderType]
		area := h.view.shapeHitAreas[elem.ID]
		point := h.view.canvasCardOrigin.Add(area.elementOrigin).Add(area.textOrigin).Add(image.Pt(12, 12))
		h.click(f32Point(point))
		if h.view.editingShape != elem.ID || h.view.shapeEditor.Text() != "" {
			t.Fatalf("clicking real %s placeholder did not open its empty editor: editing=%d draft=%q", placeholderType, h.view.editingShape, h.view.shapeEditor.Text())
		}
		h.view.shapeEditor.SetText(text)
		h.frame()
	}
	h.click(f32.Pt(1050, 650))
	gotText := make(map[string]string)
	for _, elem := range h.view.currentSlide().Elements {
		if elem.PlaceholderType == "ctrTitle" || elem.PlaceholderType == "subTitle" {
			gotText[elem.PlaceholderType] = elem.PlainText()
		}
	}
	if gotText["ctrTitle"] != "真实封面标题" || gotText["subTitle"] != "真实封面副标题" {
		t.Fatalf("real placeholder text after canvas edits = %v", gotText)
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

func TestPresentationTextEditorKeepsVisibleDefaultAlignment(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	elem := &h.view.snapshot.Slides[0].Elements[0]
	elem.Bounds = document.PresentationRect{X: 100, Y: 100, W: 480, H: 220}
	elem.PlaceholderType = "title"
	elem.WrapText = false
	elem.Paragraphs = []document.PresentationParagraph{{Runs: []document.PresentationTextRun{{
		Text: "Left aligned title",
	}}}}
	h.elem = *elem
	h.frame()

	point := h.textClickPoint(20, 10)
	h.click(point)
	h.click(point)
	if h.view.editingShape != h.elem.ID {
		t.Fatal("clicking visible title text did not enter edit mode")
	}
	if got := h.view.shapeEditor.Alignment; got != text.Start {
		t.Fatalf("editor alignment = %v, want %v to match the left-aligned preview", got, text.Start)
	}
}

func TestPresentationEnteringEditDoesNotChangeTextStyleOrGeometry(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	before := h.app.pptxDoc.Snapshot()
	beforeElement := presentationElementByID(t, &before.Slides[0], h.elem.ID)
	revision := h.app.pptxDoc.Revision()
	dirty := h.app.pptxDoc.IsDirty()
	h.view.beginPresentationTextEdit(beforeElement)
	if h.view.editingShape != h.elem.ID {
		t.Fatal("text object did not enter edit mode")
	}
	after := h.app.pptxDoc.Snapshot()
	afterElement := presentationElementByID(t, &after.Slides[0], h.elem.ID)
	if !reflect.DeepEqual(afterElement.Paragraphs, beforeElement.Paragraphs) || afterElement.Bounds != beforeElement.Bounds || afterElement.RotationDeg != beforeElement.RotationDeg {
		t.Fatalf("entering edit mode changed text style or geometry: before=%+v after=%+v", beforeElement, afterElement)
	}
	if h.app.pptxDoc.Revision() != revision || h.app.pptxDoc.IsDirty() != dirty {
		t.Fatalf("entering edit mode changed dirty/revision state: before=%t/%d after=%t/%d", dirty, revision, h.app.pptxDoc.IsDirty(), h.app.pptxDoc.Revision())
	}
}

func TestPresentationStyleCommandsKeepEditDraftAndOnlyChangeRequestedStyle(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.view.selectedShape = h.elem.ID
	h.view.editingShape = h.elem.ID
	h.view.shapeEditor.SetText("尚未提交的草稿")
	h.view.shapeEditor.SetCaret(3, 3)
	h.view.shapeDraftText = h.view.shapeEditor.Text()
	selectionStart, selectionEnd := h.view.shapeEditor.Selection()
	caretBefore := [2]int{selectionStart, selectionEnd}
	before := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	if len(before.Paragraphs) == 0 || len(before.Paragraphs[0].Runs) == 0 {
		t.Fatal("selected fixture has no paragraph or run")
	}
	revision := h.app.pptxDoc.Revision()
	h.app.handlePresentationRibbonAction(h.view, "home_bold")
	if got := h.app.pptxDoc.Revision(); got != revision+1 {
		t.Fatalf("bold command revision = %d, want %d", got, revision+1)
	}
	styled := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	for _, paragraph := range styled.Paragraphs {
		for _, run := range paragraph.Runs {
			if !run.Bold {
				t.Fatalf("bold command left run unstyled: %+v", run)
			}
		}
	}
	selectionStart, selectionEnd = h.view.shapeEditor.Selection()
	if h.view.editingShape != h.elem.ID || h.view.selectedShape != h.elem.ID || h.view.shapeEditor.Text() != "尚未提交的草稿" || [2]int{selectionStart, selectionEnd} != caretBefore {
		t.Fatalf("style command disturbed edit state: selected=%d editing=%d draft=%q selection=%v want=%v", h.view.selectedShape, h.view.editingShape, h.view.shapeEditor.Text(), [2]int{selectionStart, selectionEnd}, caretBefore)
	}
	spacingBefore, spacingAfter := styled.Paragraphs[0].LineSpacing, before.Paragraphs[0].LineSpacing
	spacingMatches := spacingBefore == nil && spacingAfter == nil || spacingBefore != nil && spacingAfter != nil && *spacingBefore == *spacingAfter
	if styled.Paragraphs[0].Runs[0].SizePT != before.Paragraphs[0].Runs[0].SizePT || styled.Paragraphs[0].Align != before.Paragraphs[0].Align || !spacingMatches {
		t.Fatalf("bold command changed unrelated text metrics: before=%+v after=%+v", before.Paragraphs[0], styled.Paragraphs[0])
	}
	h.app.handlePresentationRibbonAction(h.view, "home_font_color")
	if h.view.stylePopover != "text_color" || len(h.app.presentationStyleOptions(h.view)) == 0 {
		t.Fatalf("font color action did not open its persistent palette: popover=%q", h.view.stylePopover)
	}
	h.app.handlePresentationRibbonAction(h.view, "home_line_spacing")
	spacingOptions := h.app.presentationStyleOptions(h.view)
	if h.view.stylePopover != "line_spacing" || len(spacingOptions) < 3 {
		t.Fatalf("line spacing action did not open its presets: popover=%q options=%d", h.view.stylePopover, len(spacingOptions))
	}
	if !h.app.applyPresentationStylePatch(h.view, spacingOptions[2].patch) {
		t.Fatal("applying the 1.5x line-spacing preset failed")
	}
	spaced := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	if spacing := spaced.Paragraphs[0].LineSpacing; spacing == nil || *spacing != (document.PresentationSpacing{Kind: "pct", Value: 1.5}) {
		t.Fatalf("line spacing preset = %+v, want 1.5x", spacing)
	}
	if metrics := presentationParagraphMetrics(spaced.Paragraphs[0], presentationParagraphBaseFontSize(spaced.Paragraphs[0])); metrics.lineHeightScale != 1.5 {
		t.Fatalf("edit preview metrics = %+v, want 1.5x", metrics)
	}
	selectionStart, selectionEnd = h.view.shapeEditor.Selection()
	if [2]int{selectionStart, selectionEnd} != caretBefore || h.view.shapeEditor.Text() != "尚未提交的草稿" {
		t.Fatalf("line spacing changed the active editor draft or caret: selection=%v draft=%q", [2]int{selectionStart, selectionEnd}, h.view.shapeEditor.Text())
	}
}

func TestPresentationTextEditorUsesVisibleFontMetricsAndInsets(t *testing.T) {
	elem := document.PresentationElement{
		PlaceholderType: "title",
		Paragraphs: []document.PresentationParagraph{{Runs: []document.PresentationTextRun{
			{Text: "", Font: "Arial", SizePT: 11},
			{Text: "Visible title", Font: "Arial", SizePT: 32, Bold: true},
		}}},
	}
	if got := presentationElementFontSize(elem); got != 32 {
		t.Fatalf("editor base font size = %.1f pt, want first visible preview run at 32 pt", got)
	}
	if presentationEditorInsetX != presentationTextInsetX || presentationEditorInsetY != presentationTextInsetY {
		t.Fatalf("editor insets %g/%g dp differ from preview insets %g/%g dp", presentationEditorInsetX, presentationEditorInsetY, presentationTextInsetX, presentationTextInsetY)
	}
}

func TestPresentationTextEditorUsesProjectedTextColor(t *testing.T) {
	elem := document.PresentationElement{Paragraphs: []document.PresentationParagraph{{Runs: []document.PresentationTextRun{{Text: "colored", ColorHex: "#12ABEF"}}}}}
	if got, want := presentationElementEditorColor(elem), (color.NRGBA{R: 18, G: 171, B: 239, A: 255}); got != want {
		t.Fatalf("editor color = %+v, want projected run color %+v", got, want)
	}
}

func TestPresentationEditorUsesParagraphLineAndOuterSpacing(t *testing.T) {
	paragraph := document.PresentationParagraph{
		Runs:        []document.PresentationTextRun{{Text: "line one\nline two", SizePT: 20}},
		LineSpacing: &document.PresentationSpacing{Kind: "pct", Value: 1.5},
		SpaceBefore: &document.PresentationSpacing{Kind: "pts", Value: 6},
		SpaceAfter:  &document.PresentationSpacing{Kind: "pct", Value: 0.5},
	}
	if !presentationParagraphNeedsRichLayout(paragraph) {
		t.Fatal("paragraph with explicit spacing used the plain-label path, which drops PPTX metrics")
	}
	metrics := presentationParagraphMetrics(paragraph, 20)
	if metrics.lineHeightScale != 1.5 || metrics.lineHeightPT != 0 {
		t.Fatalf("percentage line spacing metrics = %+v, want 1.5x line height", metrics)
	}
	if metrics.spaceBeforePT != 6 || metrics.spaceAfterPT != 10 {
		t.Fatalf("paragraph spacing metrics = %+v, want 6pt before and 10pt after", metrics)
	}

	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(500, 500)},
	}
	style := material.EditorStyle{}
	elem := document.PresentationElement{Paragraphs: []document.PresentationParagraph{paragraph}}
	applyPresentationEditorParagraphMetrics(&style, gtx, elem, 1)
	if style.LineHeightScale != 1 || style.LineHeight != unit.Sp(30) {
		t.Fatalf("editor line metrics = scale %.2f, absolute %g, want fixed 30sp", style.LineHeightScale, style.LineHeight)
	}
	inset := presentationEditorLayoutInset(gtx, elem, 1)
	if gtx.Dp(inset.Top) != 7 || gtx.Dp(inset.Bottom) != 11 {
		t.Fatalf("editor vertical insets = %d/%d px, want 7/11 including PPTX spacing", gtx.Dp(inset.Top), gtx.Dp(inset.Bottom))
	}

	base := paragraph
	base.LineSpacing, base.SpaceBefore, base.SpaceAfter = nil, nil, nil
	baseDims := layoutPresentationRichParagraph(gtx, material.NewTheme(), base, false, 1)
	spacedDims := layoutPresentationRichParagraph(gtx, material.NewTheme(), paragraph, false, 1)
	if spacedDims.Size.Y <= baseDims.Size.Y+16 {
		t.Fatalf("preview line and paragraph spacing height = %d, base %d; want explicit paragraph spacing plus a larger inter-line gap", spacedDims.Size.Y, baseDims.Size.Y)
	}

	paragraph.LineSpacing = &document.PresentationSpacing{Kind: "pts", Value: 30}
	metrics = presentationParagraphMetrics(paragraph, 20)
	if metrics.lineHeightPT != 30 || metrics.lineHeightScale != 0 {
		t.Fatalf("absolute line spacing metrics = %+v, want 30pt baseline distance", metrics)
	}
	elem.Paragraphs[0] = paragraph
	style = material.EditorStyle{}
	applyPresentationEditorParagraphMetrics(&style, gtx, elem, 1)
	if style.LineHeight != unit.Sp(30) || style.LineHeightScale != 1 {
		t.Fatalf("editor absolute line metrics = scale %.2f, absolute %g, want 30sp", style.LineHeightScale, style.LineHeight)
	}
}

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

func TestPPTXCanvasTransformSaveReopenPreservesUntouchedContent(t *testing.T) {
	fixturePath := filepath.Join("testdata", "pptx-transform-fixture.pptx")
	fixtureBytes, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read transform fixture: %v", err)
	}
	sourcePath := filepath.Join(t.TempDir(), "source.pptx")
	if err := os.WriteFile(sourcePath, fixtureBytes, 0o600); err != nil {
		t.Fatalf("copy transform fixture: %v", err)
	}
	h := newPresentationCanvasHarnessFromPath(t, sourcePath)
	slide := h.view.currentSlide()
	findByName := func(name string) document.PresentationElement {
		t.Helper()
		for _, elem := range slide.Elements {
			if elem.Name == name {
				return elem
			}
		}
		t.Fatalf("fixture element %q not found", name)
		return document.PresentationElement{}
	}
	textElem := findByName("Transform text")
	shapeElem := findByName("Transform shape")
	var pictureElem document.PresentationElement
	for _, elem := range slide.Elements {
		if elem.Kind == document.ElementKindPicture {
			pictureElem = elem
			break
		}
	}
	if textElem.Kind != document.ElementKindText || shapeElem.Kind != document.ElementKindShape || pictureElem.ID == 0 || pictureElem.Kind != document.ElementKindPicture {
		t.Fatalf("fixture object kinds = text:%q shape:%q picture:%q", textElem.Kind, shapeElem.Kind, pictureElem.Kind)
	}
	untouched := findByName("Untouched reference")
	emptyTextBox := findByName("Empty text box")
	if emptyTextBox.PlainText() != "" {
		t.Fatalf("fixture empty text box contains unexpected text %q", emptyTextBox.PlainText())
	}
	originalPictureBytes := append([]byte(nil), pictureElem.ImageBytes...)

	want := make(map[int64]presentationTransform, 3)
	for _, id := range []int64{textElem.ID, shapeElem.ID, pictureElem.ID} {
		before, found := presentationCanvasElementByID(h.view, id)
		if !found {
			t.Fatalf("canvas element %d disappeared before transform", id)
		}
		h.view.selectedShape = id
		h.frame()

		moveStart := presentationCanvasPoint(h.view, before.Bounds.X+before.Bounds.W/2, before.Bounds.Y+before.Bounds.H/2)
		moveEnd := moveStart.Add(f32.Pt(22, 14))
		beginPresentationCanvasDrag(h, moveStart)
		movePresentationCanvasDrag(h, moveEnd)
		finishPresentationCanvasDrag(h, moveEnd)
		afterMove := presentationElementByID(t, h.view.currentSlide(), id)
		if math.Abs(afterMove.Bounds.X-(before.Bounds.X+22)) > 0.02 || math.Abs(afterMove.Bounds.Y-(before.Bounds.Y+14)) > 0.02 {
			t.Fatalf("canvas move for element %d = %+v, want +22,+14 from %+v", id, afterMove.Bounds, before.Bounds)
		}

		h.frame()
		resizeRect := presentationBoundsInPixels(afterMove.Bounds, h.view.canvasScale)
		resizePoint := h.view.canvasCardOrigin.Add(presentationTransformHandleCenter(resizeRect, presentationHandleBottomRight))
		resizeStart := f32.Pt(float32(resizePoint.X), float32(resizePoint.Y))
		resizeEnd := resizeStart.Add(f32.Pt(18, 14))
		beginPresentationCanvasDrag(h, resizeStart)
		movePresentationCanvasDrag(h, resizeEnd)
		finishPresentationCanvasDrag(h, resizeEnd)
		afterResize := presentationElementByID(t, h.view.currentSlide(), id)
		if afterResize.Bounds.W <= afterMove.Bounds.W || afterResize.Bounds.H <= afterMove.Bounds.H {
			t.Fatalf("canvas resize for element %d = %+v, did not expand %+v", id, afterResize.Bounds, afterMove.Bounds)
		}

		h.frame()
		rotateRect := presentationBoundsInPixels(afterResize.Bounds, h.view.canvasScale)
		center := image.Pt(rotateRect.Min.X+rotateRect.Dx()/2, rotateRect.Min.Y+rotateRect.Dy()/2)
		rotationPoint := presentationRotationHandleCenter(rotateRect, 24)
		rotateStart := h.view.canvasCardOrigin.Add(rotationPoint)
		rotateEnd := h.view.canvasCardOrigin.Add(image.Pt(center.X+40, center.Y))
		beginPresentationCanvasDrag(h, f32.Pt(float32(rotateStart.X), float32(rotateStart.Y)))
		movePresentationCanvasDrag(h, f32.Pt(float32(rotateEnd.X), float32(rotateEnd.Y)))
		finishPresentationCanvasDrag(h, f32.Pt(float32(rotateEnd.X), float32(rotateEnd.Y)))
		afterRotate := presentationElementByID(t, h.view.currentSlide(), id)
		if afterRotate.RotationDeg == afterResize.RotationDeg {
			t.Fatalf("canvas rotation for element %d did not change angle: %+v", id, afterRotate)
		}
		want[id] = presentationTransform{Bounds: afterRotate.Bounds, RotationDeg: afterRotate.RotationDeg}
	}
	if !h.app.isDirty() {
		t.Fatal("canvas object transforms did not mark the source presentation dirty")
	}

	outputPath := filepath.Join(t.TempDir(), "transformed.pptx")
	if err := h.app.pptxDoc.SaveAs(outputPath); err != nil {
		t.Fatalf("save transformed PPTX copy: %v", err)
	}
	reopened, err := pptx.Open(outputPath)
	if err != nil {
		t.Fatalf("reopen transformed PPTX: %v", err)
	}
	defer reopened.Close()
	reopenedSlide := reopened.Snapshot().Slides[0]
	for id, expected := range want {
		got := presentationElementByID(t, &reopenedSlide, id)
		if math.Abs(got.Bounds.X-expected.Bounds.X) > 0.001 || math.Abs(got.Bounds.Y-expected.Bounds.Y) > 0.001 || math.Abs(got.Bounds.W-expected.Bounds.W) > 0.001 || math.Abs(got.Bounds.H-expected.Bounds.H) > 0.001 || math.Abs(got.RotationDeg-expected.RotationDeg) > 1.0/60000 {
			t.Errorf("reopened element %d transform = bounds %+v rotation %.8f, want %+v %.8f", id, got.Bounds, got.RotationDeg, expected.Bounds, expected.RotationDeg)
		}
	}
	gotText := presentationElementByID(t, &reopenedSlide, textElem.ID)
	if gotText.PlainText() != "Editable title" {
		t.Errorf("transformed text changed during save: %q", gotText.PlainText())
	}
	gotUntouched := presentationElementByID(t, &reopenedSlide, untouched.ID)
	if gotUntouched.PlainText() != untouched.PlainText() || gotUntouched.Bounds != untouched.Bounds || gotUntouched.RotationDeg != untouched.RotationDeg {
		t.Errorf("untouched text changed: before=%+v after=%+v", untouched, gotUntouched)
	}
	gotEmpty := presentationElementByID(t, &reopenedSlide, emptyTextBox.ID)
	if gotEmpty.PlainText() != "" {
		t.Errorf("empty text box changed during transform save: %q", gotEmpty.PlainText())
	}
	gotPicture := presentationElementByID(t, &reopenedSlide, pictureElem.ID)
	if !bytes.Equal(gotPicture.ImageBytes, originalPictureBytes) {
		t.Error("picture bytes changed while applying its transform")
	}
	if after, err := os.ReadFile(sourcePath); err != nil || !bytes.Equal(after, fixtureBytes) {
		t.Fatalf("source package changed during SaveAs: err=%v", err)
	}
	if after, err := os.ReadFile(fixturePath); err != nil || !bytes.Equal(after, fixtureBytes) {
		t.Fatalf("checked-in transform fixture changed: err=%v", err)
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
	allTabs := []string{"home", "file", "insert", "design", "transitions", "animations", "slideshow", "review", "view", "picture", "shape"}
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
