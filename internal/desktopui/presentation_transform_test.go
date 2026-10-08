package desktopui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/document"
)

func TestPresentationResizeUsesLocalCoordinates(t *testing.T) {
	bounds := document.PresentationRect{X: 100, Y: 100, W: 100, H: 50}
	original := presentationTransform{Bounds: bounds, RotationDeg: 90}
	// The right-middle handle is at (150,175) after a clockwise quarter turn.
	// Move it down by 20 screen points; that is +20 in the object's local X.
	got := presentationResizePreview(original, presentationHandleRight, f32.Pt(150, 175), f32.Pt(150, 195), f32.Point{}, 1, false)
	want := document.PresentationRect{X: 100, Y: 100, W: 120, H: 50}
	if got.Bounds != want {
		t.Fatalf("90 degree resize bounds = %+v, want %+v", got.Bounds, want)
	}

	for _, angle := range []float64{0, 30, 137.5} {
		transform := presentationTransform{Bounds: bounds, RotationDeg: angle}
		center := f32.Pt(float32(bounds.X+bounds.W/2), float32(bounds.Y+bounds.H/2))
		start := rotateTestPoint(center, f32.Pt(float32(bounds.X+bounds.W), float32(bounds.Y+bounds.H/2)), angle)
		end := rotateTestPoint(center, f32.Pt(float32(bounds.X+bounds.W+25), float32(bounds.Y+bounds.H/2)), angle)
		resized := presentationResizePreview(transform, presentationHandleRight, start, end, f32.Point{}, 1, false)
		if math.Abs(resized.Bounds.W-125) > 0.01 || math.Abs(resized.Bounds.H-50) > 0.01 {
			t.Errorf("%g degree resize = %+v, want width 125 and height 50", angle, resized.Bounds)
		}
	}
}

func TestPresentationResizeHonorsAspectLock(t *testing.T) {
	original := presentationTransform{Bounds: document.PresentationRect{X: 100, Y: 80, W: 100, H: 50}}
	start, end := f32.Pt(200, 130), f32.Pt(250, 150)
	free := presentationResizePreview(original, presentationHandleBottomRight, start, end, f32.Point{}, 1, false)
	locked := presentationResizePreview(original, presentationHandleBottomRight, start, end, f32.Point{}, 1, true)
	if free.Bounds != (document.PresentationRect{X: 100, Y: 80, W: 150, H: 70}) {
		t.Fatalf("unlocked resize = %+v", free.Bounds)
	}
	if locked.Bounds != (document.PresentationRect{X: 100, Y: 80, W: 150, H: 75}) {
		t.Fatalf("locked resize = %+v, want 2:1 aspect", locked.Bounds)
	}
}

func TestPresentationRotateHandleProducesClockwiseAngle(t *testing.T) {
	original := presentationTransform{Bounds: document.PresentationRect{X: 80, Y: 60, W: 40, H: 40}, RotationDeg: 350}
	got := presentationRotatePreview(original, f32.Pt(100, 40), f32.Pt(120, 80), f32.Point{}, 1)
	if got.RotationDeg != 80 {
		t.Fatalf("clockwise rotate preview = %v degrees, want 80", got.RotationDeg)
	}
	if presentationNormalizeAngle(-15) != 345 || presentationNormalizeAngle(720) != 0 {
		t.Fatalf("rotation normalization failed for negative or full-turn values")
	}
}

func TestPresentationTransformAllowsOffSlidePosition(t *testing.T) {
	original := presentationTransform{Bounds: document.PresentationRect{X: 10, Y: 15, W: 40, H: 30}}
	got := presentationMovePreview(original, f32.Pt(100, 100), f32.Pt(50, 45), f32.Point{}, 1)
	want := document.PresentationRect{X: -40, Y: -40, W: 40, H: 30}
	if got.Bounds != want {
		t.Fatalf("off-slide move = %+v, want %+v", got.Bounds, want)
	}
}

func TestPresentationTransformEnforcesMinimumSize(t *testing.T) {
	original := presentationTransform{Bounds: document.PresentationRect{X: 100, Y: 100, W: 40, H: 30}}
	got := presentationResizePreview(original, presentationHandleLeft, f32.Pt(100, 115), f32.Pt(200, 115), f32.Point{}, 1, false)
	if got.Bounds.W != 8 || got.Bounds.X != 132 {
		t.Fatalf("minimum-width resize = %+v, want left 132 and width 8", got.Bounds)
	}
}

func TestPresentationRotationRenderAcrossCanvasThumbnailSlideshow(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	slide := h.view.currentSlide()
	shape := document.PresentationElement{
		ID: 73001, Name: "rotated red shape", Kind: document.ElementKindShape,
		Bounds: document.PresentationRect{X: 180, Y: 350, W: 120, H: 40}, RotationDeg: 90,
		FillColorHex: "#FF0000", ShapeType: "rect",
	}
	picture := document.PresentationElement{
		ID: 73002, Name: "rotated two-color picture", Kind: document.ElementKindPicture,
		Bounds: document.PresentationRect{X: 500, Y: 350, W: 120, H: 60}, RotationDeg: 90,
		Crop:  document.PresentationCrop{Left: 50000},
		Image: rotationTestImage(),
	}
	slide.Elements = append(slide.Elements, shape, picture)

	// Main editor canvas.
	h.frame()
	canvasImage := renderPresentationTestOps(t, h.ops, image.Pt(1200, 700))
	canvasPoint := func(x, y float64) image.Point {
		return h.view.canvasCardOrigin.Add(image.Pt(int(x*h.view.canvasScale), int(y*h.view.canvasScale)))
	}
	assertPresentationPixel(t, canvasImage, canvasPoint(240, 315), color.NRGBA{R: 255, A: 255}, "rotated canvas shape")
	assertPresentationPixel(t, canvasImage, canvasPoint(560, 366), color.NRGBA{B: 255, A: 255}, "cropped and rotated canvas picture upper half")
	assertPresentationPixel(t, canvasImage, canvasPoint(560, 394), color.NRGBA{B: 255, A: 255}, "rotated canvas picture bottom half")

	// Static thumbnail uses the same local-coordinate rotation.
	app := newAppWithRuntime(nil, newDesktopRuntime())
	app.theme = h.app.theme
	t.Cleanup(func() { close(app.shutdown) })
	var thumbnailOps op.Ops
	thumbnailContext := layout.Context{Ops: &thumbnailOps, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 225))}
	app.layoutPresentationThumbnail(thumbnailContext, *slide)
	thumbnailImage := renderPresentationTestOps(t, &thumbnailOps, image.Pt(400, 225))
	assertPresentationPixel(t, thumbnailImage, image.Pt(100, 130), color.NRGBA{R: 255, A: 255}, "rotated thumbnail shape")
	assertPresentationPixel(t, thumbnailImage, image.Pt(233, 153), color.NRGBA{B: 255, A: 255}, "cropped and rotated thumbnail picture upper half")
	assertPresentationPixel(t, thumbnailImage, image.Pt(233, 164), color.NRGBA{B: 255, A: 255}, "rotated thumbnail picture bottom half")

	// Slideshow delegates slide content to the same canvas renderer.
	h.view.slideshowMode = true
	h.ops.Reset()
	gtx := layout.Context{Ops: h.ops, Source: h.router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1200, 700))}
	h.app.layoutPresentationSlideshow(gtx, h.view)
	h.router.Frame(h.ops)
	slideshowImage := renderPresentationTestOps(t, h.ops, image.Pt(1200, 700))
	point := func(x, y float64) image.Point {
		return h.view.canvasCardOrigin.Add(image.Pt(int(x*h.view.canvasScale), int(y*h.view.canvasScale)))
	}
	assertPresentationPixel(t, slideshowImage, point(240, 315), color.NRGBA{R: 255, A: 255}, "rotated slideshow shape")
	assertPresentationPixel(t, slideshowImage, point(560, 366), color.NRGBA{B: 255, A: 255}, "cropped and rotated slideshow picture upper half")
	assertPresentationPixel(t, slideshowImage, point(560, 394), color.NRGBA{B: 255, A: 255}, "rotated slideshow picture bottom half")
}

func TestPresentationPictureRendererAppliesCrop(t *testing.T) {
	elem := document.PresentationElement{
		Kind:  document.ElementKindPicture,
		Crop:  document.PresentationCrop{Left: 50000},
		Image: fourColorCropTestImage(),
	}
	img := renderPresentationPictureTest(t, elem, 0, image.Pt(80, 80))
	assertPresentationPixel(t, img, image.Pt(20, 20), color.NRGBA{G: 255, A: 255}, "cropped top source")
	assertPresentationPixel(t, img, image.Pt(60, 60), color.NRGBA{R: 255, G: 255, A: 255}, "cropped bottom source")
}

func TestPresentationPictureRendererCropThenRotation(t *testing.T) {
	elem := document.PresentationElement{
		Kind:  document.ElementKindPicture,
		Crop:  document.PresentationCrop{Left: 50000},
		Image: fourColorCropTestImage(),
	}
	img := renderPresentationPictureTest(t, elem, 90, image.Pt(80, 80))
	assertPresentationPixel(t, img, image.Pt(58, 40), color.NRGBA{G: 255, A: 255}, "rotated upper source on the right")
	assertPresentationPixel(t, img, image.Pt(22, 40), color.NRGBA{R: 255, G: 255, A: 255}, "rotated lower source on the left")
}

func renderPresentationPictureTest(t *testing.T, elem document.PresentationElement, rotation float64, size image.Point) *image.RGBA {
	t.Helper()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
	rotated := pushPresentationElementRotation(gtx, image.Rectangle{Max: size}, rotation)
	drawPresentationPicture(gtx, elem, size)
	rotated.Pop()
	return renderPresentationTestOps(t, &ops, size)
}

func fourColorCropTestImage() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			var c color.NRGBA
			switch {
			case x < 50 && y < 50:
				c = color.NRGBA{R: 255, A: 255}
			case x >= 50 && y < 50:
				c = color.NRGBA{G: 255, A: 255}
			case x < 50 && y >= 50:
				c = color.NRGBA{B: 255, A: 255}
			default:
				c = color.NRGBA{R: 255, G: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func rotateTestPoint(center, point f32.Point, angleDeg float64) f32.Point {
	angle := angleDeg * math.Pi / 180
	dx, dy := float64(point.X-center.X), float64(point.Y-center.Y)
	return f32.Pt(center.X+float32(dx*math.Cos(angle)-dy*math.Sin(angle)), center.Y+float32(dx*math.Sin(angle)+dy*math.Cos(angle)))
}

func rotationTestImage() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 120, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 120; x++ {
			if x < 60 {
				img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 230, A: 255})
			} else {
				img.SetNRGBA(x, y, color.NRGBA{B: 255, A: 255})
			}
		}
	}
	return img
}
