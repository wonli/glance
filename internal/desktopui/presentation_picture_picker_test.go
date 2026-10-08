package desktopui

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"glance/internal/document"
)

func TestPresentationPictureTabRequiresSupportedSelection(t *testing.T) {
	v := &presentationViewState{snapshot: document.PresentationSnapshot{Slides: []document.PresentationSlide{{Elements: []document.PresentationElement{
		{ID: 1, Kind: document.ElementKindText},
		{ID: 2, Kind: document.ElementKindPicture, CanEditPicture: false},
		{ID: 3, Kind: document.ElementKindPicture, CanEditPicture: true},
	}}}}, activeSlide: 0}
	containsPictureTab := func() bool {
		for _, tab := range presentationRibbonTabs(v) {
			if tab.id == "picture" {
				return true
			}
		}
		return false
	}
	if containsPictureTab() {
		t.Fatal("picture tab appeared without a selected picture")
	}
	for _, id := range []int64{1, 2} {
		v.selectedShape = id
		if containsPictureTab() {
			t.Fatalf("picture tab appeared for unsupported selection %d", id)
		}
	}
	v.selectedShape = 3
	if !containsPictureTab() {
		t.Fatal("editable selected picture did not reveal the contextual picture tab")
	}
	actions := pptxActionsForTab("picture")
	if len(actions) == 0 || actions[0].id != "picture_replace" || actions[0].label != "替换图片" {
		t.Fatalf("picture actions = %+v; want a Replace Picture action", actions)
	}
}

func TestPresentationPicturePickerCancelAndStaleResultAreNoops(t *testing.T) {
	h, picture := newPresentationPicturePickerHarness(t)
	h.app.pptxPicPickerResults = make(chan presentationPicturePickerResult, 1)
	h.app.pptxPicCommandResults = make(chan presentationPictureCommandResult, 1)
	h.app.shutdown = make(chan struct{})
	h.app.pptxPicFilePicker = func() (string, error) { return "", nil }
	h.app.startPresentationPicturePicker()
	result := receivePresentationPicturePicker(t, h.app)
	h.app.handlePresentationPicturePickerResult(result)
	if h.app.pptxPicPickerRunning || h.app.pptxDoc.Dirty() || h.app.pptxDoc.Revision() != 1 {
		t.Fatalf("cancel changed picker/document state: running=%v dirty=%v revision=%d", h.app.pptxPicPickerRunning, h.app.pptxDoc.Dirty(), h.app.pptxDoc.Revision())
	}
	if h.view.selectedShape != picture.ID {
		t.Fatalf("cancel changed selected picture: got %d want %d", h.view.selectedShape, picture.ID)
	}

	imagePath := filepath.Join(t.TempDir(), "replacement.png")
	if err := os.WriteFile(imagePath, pngImageBytes(t, image.NewNRGBA(image.Rect(0, 0, 2, 2))), 0o600); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	h.app.pptxPicFilePicker = func() (string, error) {
		close(started)
		<-release
		return imagePath, nil
	}
	h.app.startPresentationPicturePicker()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("PPTX picture picker did not start asynchronously")
	}
	h.view.selectedShape = 0
	close(release)
	result = receivePresentationPicturePicker(t, h.app)
	h.app.handlePresentationPicturePickerResult(result)
	if h.app.pptxPicPickerRunning || h.app.pptxDoc.Dirty() || h.app.pptxDoc.Revision() != 1 {
		t.Fatalf("stale picker result changed document: running=%v dirty=%v revision=%d", h.app.pptxPicPickerRunning, h.app.pptxDoc.Dirty(), h.app.pptxDoc.Revision())
	}
	select {
	case command := <-h.app.pptxPicCommandResults:
		t.Fatalf("stale picker result unexpectedly queued a replacement: %+v", command)
	default:
	}

	h.view.selectedShape = picture.ID
	h.app.pptxPicFilePicker = func() (string, error) { return imagePath, nil }
	h.app.startPresentationPicturePicker()
	result = receivePresentationPicturePicker(t, h.app)
	h.app.documentGeneration++
	h.app.handlePresentationPicturePickerResult(result)
	if h.app.pptxPicPickerRunning || h.app.pptxDoc.Dirty() || h.app.pptxDoc.Revision() != 1 {
		t.Fatalf("stale generation changed document: running=%v dirty=%v revision=%d", h.app.pptxPicPickerRunning, h.app.pptxDoc.Dirty(), h.app.pptxDoc.Revision())
	}
}

func TestPresentationPicturePickerValidatesDecodeLimits(t *testing.T) {
	directory := t.TempDir()
	valid := filepath.Join(directory, "valid.png")
	validBytes := pngImageBytes(t, image.NewNRGBA(image.Rect(0, 0, 3, 2)))
	if err := os.WriteFile(valid, validBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadPresentationPictureFile(valid)
	if err != nil || loaded.mediaType != "image/png" || !bytes.Equal(loaded.data, validBytes) {
		t.Fatalf("valid PNG loaded as media=%q bytes=%d err=%v", loaded.mediaType, len(loaded.data), err)
	}

	misnamed := filepath.Join(directory, "misnamed.jpg")
	if err := os.WriteFile(misnamed, validBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPresentationPictureFile(misnamed); err == nil {
		t.Fatal("extension and actual image format mismatch was accepted")
	}
	corrupt := filepath.Join(directory, "corrupt.png")
	if err := os.WriteFile(corrupt, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPresentationPictureFile(corrupt); err == nil {
		t.Fatal("corrupt image was accepted")
	}
	tooWide := filepath.Join(directory, "too-wide.png")
	if err := os.WriteFile(tooWide, pngConfigHeader(32769, 1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPresentationPictureFile(tooWide); err == nil {
		t.Fatal("image wider than 32768 pixels was accepted")
	}
	tooManyPixels := filepath.Join(directory, "too-many-pixels.png")
	if err := os.WriteFile(tooManyPixels, pngConfigHeader(8193, 8192), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPresentationPictureFile(tooManyPixels); err == nil {
		t.Fatal("image larger than 64 megapixels was accepted")
	}
	tooLarge := filepath.Join(directory, "too-large.png")
	file, err := os.Create(tooLarge)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxPresentationPickerPictureBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPresentationPictureFile(tooLarge); err == nil {
		t.Fatal("image file over 128 MiB was accepted")
	}
}

func TestPresentationPictureReplacementUpdatesPreviewAndDirtyState(t *testing.T) {
	h, picture := newPresentationPicturePickerHarness(t)
	original := picture
	replacement := pngImageBytes(t, image.NewNRGBA(image.Rect(0, 0, 7, 4)))
	imagePath := filepath.Join(t.TempDir(), "replacement.png")
	if err := os.WriteFile(imagePath, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	h.app.pptxPicPickerResults = make(chan presentationPicturePickerResult, 1)
	h.app.pptxPicCommandResults = make(chan presentationPictureCommandResult, 1)
	h.app.shutdown = make(chan struct{})
	h.app.pptxPicFilePicker = func() (string, error) { return imagePath, nil }
	initialRevision := h.app.pptxDoc.Revision()
	h.app.startPresentationPicturePicker()
	pickerResult := receivePresentationPicturePicker(t, h.app)
	h.app.handlePresentationPicturePickerResult(pickerResult)
	commandResult := receivePresentationPictureCommand(t, h.app)
	h.app.applyPresentationPictureCommandResult(commandResult)

	if !h.app.pptxDoc.Dirty() || h.app.pptxDoc.Revision() != initialRevision+1 {
		t.Fatalf("replacement dirty/revision = %v/%d; want true/%d", h.app.pptxDoc.Dirty(), h.app.pptxDoc.Revision(), initialRevision+1)
	}
	snapshot := h.app.pptxDoc.Snapshot()
	got := presentationElementByID(t, &snapshot.Slides[0], original.ID)
	if !bytes.Equal(got.ImageBytes, replacement) || got.Image == nil || got.Bounds != original.Bounds || got.RotationDeg != original.RotationDeg || got.AltText != original.AltText {
		t.Fatalf("replacement projection lost image or identity/frame metadata: %+v", got)
	}
	if h.view.selectedShape != original.ID {
		t.Fatalf("replacement changed selection: got %d want %d", h.view.selectedShape, original.ID)
	}
}

func newPresentationPicturePickerHarness(t *testing.T) (presentationCanvasHarness, document.PresentationElement) {
	t.Helper()
	fixturePath := filepath.Join("testdata", "pptx-transform-fixture.pptx")
	fixtureBytes, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(t.TempDir(), "picture-picker.pptx")
	if err := os.WriteFile(sourcePath, fixtureBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	h := newPresentationCanvasHarnessFromPath(t, sourcePath)
	for _, elem := range h.view.currentSlide().Elements {
		if elem.Kind == document.ElementKindPicture && elem.CanEditPicture {
			h.view.selectedShape = elem.ID
			return h, elem
		}
	}
	t.Fatal("transform fixture has no supported slide-local picture")
	return presentationCanvasHarness{}, document.PresentationElement{}
}

func receivePresentationPicturePicker(t *testing.T, app *App) presentationPicturePickerResult {
	t.Helper()
	select {
	case result := <-app.pptxPicPickerResults:
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for PPTX picture picker result")
		return presentationPicturePickerResult{}
	}
}

func receivePresentationPictureCommand(t *testing.T, app *App) presentationPictureCommandResult {
	t.Helper()
	select {
	case result := <-app.pptxPicCommandResults:
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for PPTX picture command result")
		return presentationPictureCommandResult{}
	}
}
