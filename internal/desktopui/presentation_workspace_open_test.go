package desktopui

import (
	"bytes"
	"image"
	"image/color"

	"os"
	"path/filepath"

	"testing"

	"gioui.org/layout"
	"gioui.org/op"

	"gioui.org/unit"
	"gioui.org/widget/material"

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
