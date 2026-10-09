package desktopui

import (
	"image"

	"math"

	"testing"

	"gioui.org/gpu/headless"

	"gioui.org/layout"
	"gioui.org/op"

	"gioui.org/unit"
)

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
