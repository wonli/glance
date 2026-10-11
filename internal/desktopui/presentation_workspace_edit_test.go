package desktopui

import (
	"bytes"
	"image"

	"os"
	"path/filepath"

	"testing"

	"gioui.org/f32"

	"glance/internal/document"
	"glance/internal/format/pptx"
)

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
	waitPresentationCommandResult(t, app, v)
	if v.snapshot.SlideCount != initialCount+1 {
		t.Fatalf("expected slide count %d, got %d", initialCount+1, v.snapshot.SlideCount)
	}
	if v.activeSlide != v.snapshot.SlideCount-1 {
		t.Fatalf("expected active slide %d, got %d", v.snapshot.SlideCount-1, v.activeSlide)
	}

	// Delete slide
	app.handlePresentationDeleteSlide(v)
	waitPresentationCommandResult(t, app, v)
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
