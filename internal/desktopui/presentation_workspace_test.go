package desktopui

import (
	"image"
	"math"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

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

