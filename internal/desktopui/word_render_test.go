package desktopui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordImageFitUsesAvailableWidthAndPreservesAspectRatio(t *testing.T) {
	got := fitWordImageSize(800, 400, 300, 1000)
	if got != image.Pt(300, 150) {
		t.Fatalf("fitted image size = %v; want 300x150", got)
	}
	constrainedHeight := fitWordImageSize(800, 400, 1000, 100)
	if constrainedHeight != image.Pt(200, 100) {
		t.Fatalf("height-constrained image size = %v; want 200x100", constrainedHeight)
	}
	if fitWordImageSize(0, 400, 300, 1000) != (image.Point{}) {
		t.Fatal("invalid source dimensions should produce no preview size")
	}
}

func TestWordImageCacheUsesLRUAndEnforcesDocumentMemoryLimit(t *testing.T) {
	cache := newWordImageCache(20)
	red := image.NewNRGBA(image.Rect(0, 0, 1, 2))
	blue := image.NewNRGBA(image.Rect(0, 0, 1, 2))
	green := image.NewNRGBA(image.Rect(0, 0, 1, 2))
	cache.Put("red", red)
	cache.Put("blue", blue)
	if _, ok := cache.Get("red"); !ok {
		t.Fatal("expected red image in cache")
	}
	cache.Put("green", green)
	if _, ok := cache.Get("blue"); ok {
		t.Fatal("least recently used image was not evicted")
	}
	if got := cache.Bytes(); got > 20 || got != 16 {
		t.Fatalf("cache bytes=%d; want 16 and no more than its 20-byte limit", got)
	}
	tooLarge := image.NewNRGBA(image.Rect(0, 0, 3, 3))
	cache.Put("large", tooLarge)
	if _, ok := cache.Get("large"); ok || cache.Bytes() > 20 {
		t.Fatal("image larger than the cache budget should not remain cached")
	}
}

func TestWordImageDecodeRejectsOversizedAndUnsupportedMedia(t *testing.T) {
	oversized := image.Config{ColorModel: color.NRGBAModel, Width: 32_000_001, Height: 1}
	if err := validateWordPreviewConfig(oversized, "png"); err == nil {
		t.Fatal("preview above 32 megapixels was accepted")
	}
	if err := validateWordPreviewConfig(image.Config{ColorModel: color.NRGBAModel, Width: 1, Height: 1}, "bmp"); err == nil {
		t.Fatal("unsupported preview format was accepted")
	}
}

func TestWordImageGesturePreviewsWithoutMutationUntilRelease(t *testing.T) {
	gesture := wordImageGesture{}
	if !gesture.BeginMove("img-1", "p0") {
		t.Fatal("could not start image move preview")
	}
	gesture.MovePreview("p0", "p0.i2")
	if gesture.CommitMove() {
		t.Fatal("image move committed before a release event")
	}
	if preview := gesture.Preview(); !preview.Active || preview.ImageID != "img-1" || preview.AfterInlineID != "p0.i2" {
		t.Fatalf("active image preview = %#v", preview)
	}
	gesture.ReleaseMove()
	if !gesture.CommitMove() || gesture.Preview().Active {
		t.Fatal("release should commit once and clear the preview")
	}
	gesture.BeginMove("img-1", "p0")
	gesture.MovePreview("p0", "p0.i2")
	gesture.Cancel()
	if gesture.CommitMove() || gesture.Preview().Active {
		t.Fatal("cancelled image drag should not commit or retain a preview")
	}
}

func TestWordImageResizePreviewRetainsRatioAndCommitsOnRelease(t *testing.T) {
	gesture := wordImageGesture{}
	if !gesture.BeginResize("img-1", 914400, 457200) {
		t.Fatal("could not start image resize preview")
	}
	gesture.ResizePreview(1828800)
	preview := gesture.Preview()
	if preview.WidthEMU != 1828800 || preview.HeightEMU != 914400 || !preview.Active {
		t.Fatalf("resize preview = %#v; want 2:1 ratio", preview)
	}
	if gesture.CommitResize() {
		t.Fatal("resize preview committed before release")
	}
	gesture.ReleaseResize()
	if !gesture.CommitResize() {
		t.Fatal("resize should commit when released")
	}
	gesture.BeginResize("img-1", 914400, 457200)
	gesture.ResizePreview(914400)
	gesture.ReleaseResize()
	if gesture.CommitResize() {
		t.Fatal("no-op resize should not request a document mutation")
	}
}

func TestWordUnsupportedImageHasVisibleStatusPlaceholder(t *testing.T) {
	got := wordImagePlaceholder(document.WordUnsupported{Description: "unsupported media"})
	if got == "" || got == "unsupported media" {
		t.Fatalf("unsupported image placeholder should include a visible status: %q", got)
	}
}

func TestWordPreviewCompactsLargeImagesToStayWithinCacheLimit(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 8192, 2))
	preview := compactWordPreview(source)
	if got := preview.Bounds().Size(); got.X != 4096 || got.Y != 1 {
		t.Fatalf("compacted preview size = %v; want 4096x1", got)
	}
	cache := newWordImageCache(wordImageCacheLimit)
	if !cache.Put("large", preview) || cache.Bytes() > wordImageCacheLimit {
		t.Fatalf("compacted preview did not fit cache: %d bytes", cache.Bytes())
	}
}

func TestWordImageDecodeRunsOffFrameAndPopulatesSessionCache(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	paragraph := word.WordBlocks()[0].Paragraph
	imageID, err := word.InsertInlineImage(paragraph.ID, "", "image/png", encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("sample.docx")
	state.word = word
	app := &App{
		documentSessionState: state,
		wordImageResults:     make(chan wordImageDecodeResult, 1),
		wordCommandResults:   make(chan wordCommandResult, 1),
		shutdown:             make(chan struct{}),
	}
	app.requestWordImageDecode(state, imageID)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case result := <-app.wordImageResults:
			if result.err != nil {
				t.Fatalf("asynchronous decode failed: %v", result.err)
			}
			app.wordImageResults <- result
			app.processAsyncResults()
			decoded, ok := state.wordImageCache.Get(imageID)
			if !ok || decoded.Bounds().Size() != image.Pt(3, 2) {
				t.Fatalf("cached decoded image = %v, present=%v", decoded, ok)
			}
			if _, pending := state.wordImagePending[imageID]; pending {
				t.Fatal("completed image decode remained pending")
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for asynchronous image decode")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestWordImageEMUsConvertAt96DPIAndDeviceScale(t *testing.T) {
	if got := wordImageEMUToPixels(914400, 1); got != 96 {
		t.Fatalf("one inch at 1x = %d px; want 96", got)
	}
	if got := wordImageEMUToPixels(914400, 2); got != 192 {
		t.Fatalf("one inch at 2x = %d px; want 192", got)
	}
}
