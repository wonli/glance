package desktopui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
)

func TestPresentationCanvasPaintsBackgroundInheritedAndSlideOrder(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	slide := h.view.currentSlide()
	slide.BgColorHex = "#102030"
	slide.BackgroundImage = presentationLayerTestImage()
	slide.InheritedElements = []document.PresentationElement{{
		ID: -1, Name: "master decoration", Kind: document.ElementKindShape,
		Bounds: document.PresentationRect{X: 100, Y: 350, W: 110, H: 100}, FillColorHex: "#0000FF", ShapeType: "rect",
	}}
	slide.Elements = append(slide.Elements, document.PresentationElement{
		ID: 987654, Name: "slide local overlay", Kind: document.ElementKindShape,
		Bounds: document.PresentationRect{X: 130, Y: 380, W: 60, H: 50}, FillColorHex: "#00FF00", ShapeType: "rect",
	})
	h.frame()
	screenshot := renderPresentationTestOps(t, h.ops, image.Pt(1200, 700))
	point := func(x, y float64) image.Point {
		return h.view.canvasCardOrigin.Add(image.Pt(int(x*h.view.canvasScale), int(y*h.view.canvasScale)))
	}
	assertPresentationPixel(t, screenshot, point(700, 100), color.NRGBA{R: 16, G: 32, B: 48, A: 255}, "slide background color")
	assertPresentationPixel(t, screenshot, point(100, 100), color.NRGBA{R: 255, G: 220, B: 0, A: 255}, "slide background image")
	assertPresentationPixel(t, screenshot, point(110, 360), color.NRGBA{B: 255, A: 255}, "inherited master decoration")
	assertPresentationPixel(t, screenshot, point(140, 390), color.NRGBA{G: 255, A: 255}, "slide-local shape above inherited decoration")
}

func TestPresentationThumbnailPaintsBackgroundAndInheritedLayer(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())
	app.theme = material.NewTheme()
	t.Cleanup(func() { close(app.shutdown) })
	slide := document.PresentationSlide{
		WidthPT: 960, HeightPT: 540, BgColorHex: "#102030", BackgroundImage: presentationLayerTestImage(),
		InheritedElements: []document.PresentationElement{{
			ID: -1, Kind: document.ElementKindShape, Bounds: document.PresentationRect{X: 100, Y: 350, W: 100, H: 100}, FillColorHex: "#0000FF", ShapeType: "rect",
		}},
	}
	size := image.Pt(400, 225)
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
	app.layoutPresentationThumbnail(gtx, slide)
	screenshot := renderPresentationTestOps(t, &ops, size)
	assertPresentationPixel(t, screenshot, image.Pt(300, 42), color.NRGBA{R: 16, G: 32, B: 48, A: 255}, "thumbnail background color")
	assertPresentationPixel(t, screenshot, image.Pt(40, 42), color.NRGBA{R: 255, G: 220, B: 0, A: 255}, "thumbnail background image")
	assertPresentationPixel(t, screenshot, image.Pt(50, 150), color.NRGBA{B: 255, A: 255}, "thumbnail inherited decoration")
}

func TestPresentationInheritedVisualsAreNotInteractive(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	slide := h.view.currentSlide()
	slide.InheritedElements = []document.PresentationElement{{
		ID: -1000001, Name: "read-only inherited shape", Kind: document.ElementKindShape,
		Bounds: document.PresentationRect{X: 50, Y: 400, W: 100, H: 80}, FillColorHex: "#0000FF", ShapeType: "rect",
	}}
	h.frame()
	clickablesBefore := len(h.view.shapeClicks)
	point := h.view.canvasCardOrigin.Add(image.Pt(int(75*h.view.canvasScale), int(420*h.view.canvasScale)))
	h.click(f32Point(point))
	if h.view.selectedShape != 0 || h.view.editingShape != 0 {
		t.Fatalf("clicking inherited visual selected or edited it: selected=%d editing=%d", h.view.selectedShape, h.view.editingShape)
	}
	if len(h.view.shapeClicks) != clickablesBefore {
		t.Fatalf("inherited visual registered a clickable: before=%d after=%d", clickablesBefore, len(h.view.shapeClicks))
	}
	if _, ok := h.view.shapeClicks[-1000001]; ok {
		t.Fatal("inherited visual has a slide interaction clickable")
	}
	h.click(h.textClickPoint(60, 10))
	if h.view.editingShape != h.elem.ID {
		t.Fatalf("clicking slide-local text did not enter edit mode: editing=%d want %d", h.view.editingShape, h.elem.ID)
	}
}

func TestEmptySlidePlaceholderShowsPromptAndCanBeEdited(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	placeholder := document.PresentationElement{
		ID: 24680, Name: "title placeholder", Kind: document.ElementKindText, PlaceholderType: "ctrTitle",
		Bounds: document.PresentationRect{X: 220, Y: 120, W: 500, H: 90}, WrapText: true,
	}
	h.view.currentSlide().Elements = append(h.view.currentSlide().Elements, placeholder)
	h.frame()

	window, err := headless.NewWindow(1200, 700)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	if err := window.Frame(h.ops); err != nil {
		t.Fatalf("render placeholder prompt: %v", err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, 1200, 700))
	if err := window.Screenshot(screenshot); err != nil {
		t.Fatal(err)
	}
	box := image.Rectangle{
		Min: h.view.canvasCardOrigin.Add(image.Pt(int(placeholder.Bounds.X*h.view.canvasScale), int(placeholder.Bounds.Y*h.view.canvasScale))),
		Max: h.view.canvasCardOrigin.Add(image.Pt(int((placeholder.Bounds.X+placeholder.Bounds.W)*h.view.canvasScale), int((placeholder.Bounds.Y+placeholder.Bounds.H)*h.view.canvasScale))),
	}
	var darkPixels int
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			c := color.NRGBAModel.Convert(screenshot.At(x, y)).(color.NRGBA)
			if c.A > 200 && c.R < 180 && c.G < 180 && c.B < 180 {
				darkPixels++
			}
		}
	}
	if darkPixels < 10 {
		t.Fatalf("empty title placeholder has no visible prompt text in %v: dark pixels=%d", box, darkPixels)
	}

	point := h.view.canvasCardOrigin.Add(image.Pt(int(placeholder.Bounds.X*h.view.canvasScale+20), int(placeholder.Bounds.Y*h.view.canvasScale+20)))
	h.click(f32Point(point))
	if h.view.editingShape != placeholder.ID || h.view.shapeEditor.Text() != "" {
		t.Fatalf("empty placeholder did not open an empty editor: editing=%d draft=%q", h.view.editingShape, h.view.shapeEditor.Text())
	}
}

func presentationLayerTestImage() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 220, B: 0, A: 255})
		}
	}
	return img
}

func renderPresentationTestOps(t *testing.T, ops *op.Ops, size image.Point) *image.RGBA {
	t.Helper()
	window, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	if err := window.Frame(ops); err != nil {
		t.Fatalf("render presentation frame: %v", err)
	}
	screenshot := image.NewRGBA(image.Rectangle{Max: size})
	if err := window.Screenshot(screenshot); err != nil {
		t.Fatalf("capture presentation frame: %v", err)
	}
	return screenshot
}

func assertPresentationPixel(t *testing.T, img *image.RGBA, point image.Point, want color.NRGBA, label string) {
	t.Helper()
	if !point.In(img.Bounds()) {
		t.Fatalf("%s sample point %v outside screenshot bounds %v", label, point, img.Bounds())
	}
	got := color.NRGBAModel.Convert(img.At(point.X, point.Y)).(color.NRGBA)
	if math.Abs(float64(int(got.R)-int(want.R))) > 8 || math.Abs(float64(int(got.G)-int(want.G))) > 8 || math.Abs(float64(int(got.B)-int(want.B))) > 8 || got.A < 245 {
		t.Fatalf("%s pixel at %v = %#v, want approximately %#v", label, point, got, want)
	}
}

func f32Point(point image.Point) f32.Point {
	return f32.Pt(float32(point.X), float32(point.Y))
}
