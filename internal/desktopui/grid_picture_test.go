package desktopui

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"glance/internal/document"
)

func TestGridPictureGeometryUsesAnchorsScrollScaleAndViewportClip(t *testing.T) {
	v := newGridViewport()
	_ = v.Rows.SetSize(0, 40)
	_ = v.Rows.SetSize(1, 30)
	_ = v.Columns.SetSize(0, 100)
	_ = v.Columns.SetSize(1, 120)
	v.Scale = 2
	v.X, v.Y = 30, 20
	picture := document.SpreadsheetImage{ID: "p1", Row: 2, Column: 2, OffsetX: 5, OffsetY: 7, WidthPx: 50, HeightPx: 25}

	content := gridPictureContentGeometry(v, picture)
	wantContent := gridPictureGeometry{X: 110, Y: 54, Width: 100, Height: 50}
	if content != wantContent {
		t.Fatalf("content geometry=%+v; want %+v", content, wantContent)
	}
	screen := gridPictureScreenRect(v, content, 42, 32)
	if screen != image.Rect(122, 66, 222, 116) {
		t.Fatalf("screen bounds=%v; want (122,66)-(222,116)", screen)
	}
	clipped := gridPictureClipRect(screen, image.Rect(42, 32, 200, 100))
	if clipped != image.Rect(122, 66, 200, 100) {
		t.Fatalf("clipped bounds=%v; want (122,66)-(200,100)", clipped)
	}
	placement, err := gridPicturePlacementAt(v, picture, gridPictureGeometry{X: 350, Y: 92, Width: 140, Height: 70})
	if err != nil {
		t.Fatal(err)
	}
	if placement.Row != 3 || placement.Column != 4 || placement.OffsetX != 20 || placement.OffsetY != 11 || placement.WidthPx != 70 {
		t.Fatalf("geometry placement=%+v; want row=3 column=4 offset=(20,11) width=70", placement)
	}
}

func TestGridPictureHitTestingPrioritizesResizeHandleThenTopPicture(t *testing.T) {
	v := newGridViewport()
	images := []document.SpreadsheetImage{
		{ID: "back", Row: 1, Column: 1, WidthPx: 120, HeightPx: 80, Editable: true},
		{ID: "front", Row: 1, Column: 1, OffsetX: 20, OffsetY: 10, WidthPx: 70, HeightPx: 60, Editable: true},
	}
	handle := gridPictureHandleRect(image.Rect(62, 42, 132, 102), 10)
	viewport := image.Rect(42, 32, 240, 180)
	got := gridPictureHitAt(v, images, "front", handle.Min.Add(image.Pt(2, 2)), viewport, 42, 32, 10)
	if got.ImageID != "front" || !got.ResizeHandle {
		t.Fatalf("handle hit=%+v; want front resize handle", got)
	}
	got = gridPictureHitAt(v, images, "front", image.Pt(90, 80), viewport, 42, 32, 10)
	if got.ImageID != "front" || got.ResizeHandle {
		t.Fatalf("overlap body hit=%+v; want front picture", got)
	}
	got = gridPictureHitAt(v, images, "front", image.Pt(53, 50), viewport, 42, 32, 10)
	if got.ImageID != "back" || got.ResizeHandle {
		t.Fatalf("underlap body hit=%+v; want back picture", got)
	}
	got = gridPictureHitAt(v, images, "front", image.Pt(10, 10), viewport, 42, 32, 10)
	if got.ImageID != "" {
		t.Fatalf("outside grid hit=%+v; want no picture", got)
	}
}

func TestGridPictureGesturePreviewReleaseCancelAndRebase(t *testing.T) {
	original := gridPictureGeometry{X: 100, Y: 60, Width: 80, Height: 40}
	gesture := gridPictureGesture{}
	if !gesture.begin("p1", gridPictureGestureMove, original, f32.Pt(150, 80)) {
		t.Fatal("move gesture did not start")
	}
	gesture.update(f32.Pt(170, 95))
	if want := (gridPictureGeometry{X: 120, Y: 75, Width: 80, Height: 40}); gesture.Preview != want {
		t.Fatalf("move preview=%+v; want %+v", gesture.Preview, want)
	}
	gesture.rebase(f32.Pt(170, 95))
	gesture.update(f32.Pt(180, 100))
	if want := (gridPictureGeometry{X: 130, Y: 80, Width: 80, Height: 40}); gesture.Preview != want {
		t.Fatalf("rebased move preview=%+v; want %+v", gesture.Preview, want)
	}
	committed, changed := gesture.finish()
	if !changed || committed != gesturePreview(130, 80, 80, 40) || gesture.Mode != gridPictureGestureNone {
		t.Fatalf("move finish=(%+v,%v), active=%v", committed, changed, gesture.Mode)
	}

	if !gesture.begin("p1", gridPictureGestureResize, original, f32.Pt(180, 100)) {
		t.Fatal("resize gesture did not start")
	}
	gesture.update(f32.Pt(220, 100))
	if want := (gridPictureGeometry{X: 100, Y: 60, Width: 120, Height: 60}); gesture.Preview != want {
		t.Fatalf("ratio-locked preview=%+v; want %+v", gesture.Preview, want)
	}
	gesture.cancel()
	if gesture.Mode != gridPictureGestureNone || gesture.Preview != (gridPictureGeometry{}) {
		t.Fatalf("cancel left preview state: %+v", gesture)
	}
	if !gesture.begin("p1", gridPictureGestureMove, original, f32.Pt(180, 100)) {
		t.Fatal("second move gesture did not start")
	}
	gesture.update(f32.Pt(180, 100))
	if _, changed := gesture.finish(); changed {
		t.Fatal("zero-delta release should not commit")
	}
}

func TestGridPictureLayerRendersAndClipsInsideScrollableGridViewport(t *testing.T) {
	state := newDocumentSessionState("pictures.xlsx")
	state.sheet = "Sheet1"
	state.gridPicturesSheet = "Sheet1"
	state.gridPictures = []document.SpreadsheetImage{
		{ID: "blue", Sheet: "Sheet1", Cell: "A1", Row: 1, Column: 1, WidthPx: 40, HeightPx: 30, MediaType: "image/png", Editable: true},
		{ID: "orange", Sheet: "Sheet1", Cell: "B1", Row: 1, Column: 2, WidthPx: 20, HeightPx: 20, MediaType: "image/png", Editable: true},
		{ID: "magenta", Sheet: "Sheet1", Cell: "C1", Row: 1, Column: 3, WidthPx: 50, HeightPx: 20, MediaType: "image/png", Editable: true},
	}
	state.gridPictureCache.Put("blue", solidGridPicture(color.NRGBA{R: 45, G: 110, B: 190, A: 255}, 40, 30))
	state.gridPictureCache.Put("orange", solidGridPicture(color.NRGBA{R: 240, G: 170, B: 55, A: 255}, 20, 20))
	state.gridPictureCache.Put("magenta", solidGridPicture(color.NRGBA{R: 200, G: 40, B: 170, A: 255}, 50, 20))
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	v := newGridViewport()
	v.X, v.Y = 20, 10
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Constraints: layout.Constraints{Max: image.Pt(220, 160)}, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	paintVirtualGrid(gtx, app, v)

	window, err := headless.NewWindow(220, 160)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	if err := window.Frame(&ops); err != nil {
		t.Fatalf("render spreadsheet picture layer: %v", err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, 220, 160))
	if err := window.Screenshot(screenshot); err != nil {
		t.Fatalf("capture spreadsheet picture layer: %v", err)
	}
	blue := color.NRGBA{R: 45, G: 110, B: 190, A: 255}
	orange := color.NRGBA{R: 240, G: 170, B: 55, A: 255}
	magenta := color.NRGBA{R: 200, G: 40, B: 170, A: 255}
	if got := pixelAt(screenshot, image.Pt(46, 36)); got != blue {
		t.Fatalf("blue picture data pixel=%v; want %v", got, blue)
	}
	if got := pixelAt(screenshot, image.Pt(120, 35)); got != orange {
		t.Fatalf("orange picture data pixel=%v; want %v", got, orange)
	}
	if got := pixelAt(screenshot, image.Pt(210, 36)); got != magenta {
		t.Fatalf("partially visible magenta image pixel=%v; want %v", got, magenta)
	}
	for _, point := range []image.Point{{30, 40}, {50, 20}, {210, 20}} {
		got := pixelAt(screenshot, point)
		if got == blue || got == orange || got == magenta {
			t.Errorf("image color %v escaped data viewport into header at %v", got, point)
		}
	}
}

func TestSpreadsheetPicturePreviewDecodesPNGJPEGAndGIF(t *testing.T) {
	source := solidGridPicture(color.NRGBA{R: 40, G: 130, B: 210, A: 255}, 12, 6)
	for _, format := range []string{"png", "jpeg", "gif"} {
		t.Run(format, func(t *testing.T) {
			var encoded bytes.Buffer
			var err error
			switch format {
			case "png":
				err = png.Encode(&encoded, source)
			case "jpeg":
				err = jpeg.Encode(&encoded, source, &jpeg.Options{Quality: 90})
			case "gif":
				err = gif.Encode(&encoded, source, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			preview, err := decodeSpreadsheetPicturePreview(encoded.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if preview.Bounds().Dx() != 12 || preview.Bounds().Dy() != 6 {
				t.Fatalf("decoded %s preview size=%v; want 12x6", format, preview.Bounds().Size())
			}
		})
	}
	if _, err := decodeSpreadsheetPicturePreview([]byte("corrupt image bytes")); err == nil {
		t.Fatal("corrupt picture bytes were accepted")
	}
}

func TestGridPictureAsyncResultsRejectStaleWorkbookGenerations(t *testing.T) {
	state := newDocumentSessionState("stale.xlsx")
	state.sheet = "Sheet1"
	state.gridPicturesLoading = true
	state.gridPicturesRequestedSheet = "Sheet1"
	app := &App{documentSessionState: state, documentGeneration: 5}
	app.applyGridPictureMetadata(gridPictureMetadataResult{state: state, generation: 4, sheet: "Sheet1", images: []document.SpreadsheetImage{{ID: "old"}}})
	if state.gridPicturesLoading || state.gridPicturesRequestedSheet != "" || state.gridPicturesSheet != "" || len(state.gridPictures) != 0 {
		t.Fatalf("stale metadata changed active picture state: loading=%v request=%q sheet=%q pictures=%#v", state.gridPicturesLoading, state.gridPicturesRequestedSheet, state.gridPicturesSheet, state.gridPictures)
	}
	state.gridPicturesSheet = "Sheet1"
	state.gridPictures = []document.SpreadsheetImage{{ID: "current", Sheet: "Sheet1"}}
	state.gridPictureVersions["current"] = 2
	state.gridPicturePending["current"] = 2
	app.applyGridPicturePreview(gridPicturePreviewResult{state: state, generation: 4, sheet: "Sheet1", id: "current", version: 1, preview: solidGridPicture(color.NRGBA{R: 255, A: 255}, 1, 1)})
	if state.gridPictureCache.Bytes() != 0 || state.gridPicturePending["current"] != 2 {
		t.Fatalf("stale preview changed current decode state: cache=%d pending=%v", state.gridPictureCache.Bytes(), state.gridPicturePending)
	}
}

func solidGridPicture(fill color.NRGBA, width, height int) image.Image {
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			result.SetNRGBA(x, y, fill)
		}
	}
	return result
}

func pixelAt(source image.Image, point image.Point) color.NRGBA {
	return color.NRGBAModel.Convert(source.At(point.X, point.Y)).(color.NRGBA)
}

func gesturePreview(x, y, width, height float64) gridPictureGeometry {
	return gridPictureGeometry{X: x, Y: y, Width: width, Height: height}
}
