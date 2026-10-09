package docx

import (
	"bytes"
	"glance/internal/document"
	"image/color"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotateInlineImageSnapshotUndoRedo(t *testing.T) {
	doc := imageOperationFixture(t,
		imageDocRoot(`<w:p><w:r><w:t>text</w:t></w:r></w:p>`),
		imageRelationshipRoot(), imageContentTypes(), nil)
	defer doc.Close()
	paragraph := doc.WordBlocks()[0].Paragraph
	imageBytes := imageFixtureBytes(t, 2, 1, color.NRGBA{R: 30, G: 120, B: 210, A: 255})
	imageID, err := requireWordImageEditor(t, doc).InsertInlineImage(paragraph.ID, "", "image/png", imageBytes)
	if err != nil {
		t.Fatal(err)
	}
	rotator, ok := any(doc).(document.WordImageRotator)
	if !ok {
		t.Fatal("DOCX adapter does not expose inline image rotation")
	}
	if err := rotator.SetInlineImageRotation(imageID, 90); err != nil {
		t.Fatal(err)
	}
	projected := projectedWordImage(doc, imageID)
	if projected == nil || projected.RotationDegrees != 90 {
		t.Fatalf("projected image rotation = %#v; want 90 degrees", projected)
	}
	snapshot, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	parts := readZipParts(t, snapshot)
	if !strings.Contains(string(parts["word/document.xml"]), `rot="5400000"`) {
		t.Fatalf("DrawingML rotation was not encoded as 90 degrees: %s", parts["word/document.xml"])
	}
	output := filepath.Join(t.TempDir(), "rotated.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedImage := projectedWordImage(reopened, imageID)
	if reopenedImage == nil || reopenedImage.RotationDegrees != 90 {
		t.Fatalf("rotation after save/reopen = %#v; want 90 degrees", reopenedImage)
	}
	reopenedBytes, err := reopened.WordImageBytes(imageID)
	if err != nil || !bytes.Equal(reopenedBytes, imageBytes) {
		t.Fatalf("image bytes changed after rotation: %v", err)
	}
	if applied, err := requireWordHistory(t, doc).Undo(); err != nil || !applied {
		t.Fatalf("undo image rotation = %v, %v", applied, err)
	}
	if got := projectedWordImage(doc, imageID).RotationDegrees; got != 0 {
		t.Fatalf("rotation after undo = %v; want 0", got)
	}
	if applied, err := requireWordHistory(t, doc).Redo(); err != nil || !applied {
		t.Fatalf("redo image rotation = %v, %v", applied, err)
	}
	if got := projectedWordImage(doc, imageID).RotationDegrees; got != 90 {
		t.Fatalf("rotation after redo = %v; want 90", got)
	}
}

func TestReplacingOneImageDoesNotChangeSharedMediaUsedByAnother(t *testing.T) {
	shared := imageFixtureBytes(t, 1, 1, color.NRGBA{R: 200, G: 40, B: 30, A: 255})
	doc := imageOperationFixture(t,
		imageDocRoot(`<w:p>`+wordImageDrawing(1, "rId1", 914400, 457200)+wordImageDrawing(2, "rId2", 914400, 457200)+`</w:p>`),
		imageRelationshipRoot(
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/shared.png"/>`,
			`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/shared.png"/>`,
		), imageContentTypes(), map[string][]byte{"word/media/shared.png": shared})
	defer doc.Close()
	images := imageIDsInParagraph(*doc.WordBlocks()[0].Paragraph)
	if len(images) != 2 || images[0] == images[1] {
		t.Fatalf("shared image fixture projection IDs = %#v", images)
	}
	replacement := imageFixtureJPEG(t, 2, 1, color.NRGBA{R: 10, G: 180, B: 80, A: 255})
	if err := requireWordImageEditor(t, doc).ReplaceInlineImage(images[0], "image/jpeg", replacement); err != nil {
		t.Fatal(err)
	}
	blocks := doc.WordBlocks()[0].Paragraph.Inlines
	if blocks[0].Image.MediaType != "image/jpeg" || blocks[1].Image.MediaType != "image/png" {
		t.Fatalf("replacement media types = %q, %q", blocks[0].Image.MediaType, blocks[1].Image.MediaType)
	}
	untouched, err := doc.WordImageBytes(images[1])
	if err != nil || !bytes.Equal(untouched, shared) {
		t.Fatalf("second image bytes changed: %v", err)
	}
	output := filepath.Join(t.TempDir(), "replaced.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.WordBlocks()[0].Paragraph.Inlines[0].Image.MediaType != "image/jpeg" {
		t.Fatal("replacement did not survive save/reopen")
	}
	reopenedUntouched, err := reopened.WordImageBytes(images[1])
	if err != nil || !bytes.Equal(reopenedUntouched, shared) {
		t.Fatalf("shared source image did not survive save/reopen: %v", err)
	}
	partsData, err := reopened.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	parts := readZipParts(t, partsData)
	if !bytes.Equal(parts["word/media/shared.png"], shared) {
		t.Fatal("replacement mutated shared media bytes")
	}
	if !strings.Contains(string(parts["[Content_Types].xml"]), `Extension="jpg" ContentType="image/jpeg"`) {
		t.Fatal("replacement did not add the JPEG content type")
	}
	contentTypes := string(parts["[Content_Types].xml"])
	if strings.Index(contentTypes, `<Default Extension="jpg"`) > strings.Index(contentTypes, `<Override PartName=`) {
		t.Fatalf("JPEG Default must precede Content Type Overrides: %s", contentTypes)
	}
}

func TestMoveResizeDeleteTableImageRoundTripsAndUndoRestoresMedia(t *testing.T) {
	doc, originalImage := mixedContentProjectionFixture(t)
	defer doc.Close()
	blocks := doc.WordBlocks()
	imageID := blocks[0].Paragraph.Inlines[1].Image.ID
	targetParagraph := blocks[1].Table.Rows[0].Cells[0].Paragraphs[1]
	afterTextID := targetParagraph.Inlines[0].Text.ID
	editor := requireWordImageEditor(t, doc)
	if err := editor.MoveInlineImage(imageID, targetParagraph.ID, afterTextID); err != nil {
		t.Fatal(err)
	}
	if err := editor.ResizeInlineImage(imageID, 1828800); err != nil {
		t.Fatal(err)
	}
	moved := doc.WordBlocks()[1].Table.Rows[0].Cells[0].Paragraphs[1].Inlines
	if len(moved) != 4 || moved[0].Text.Text != "Cell before" || moved[1].Image.ID != imageID || moved[2].Image == nil || moved[3].Text.Text != "Cell after" {
		t.Fatalf("moved table image order = %#v", moved)
	}
	if moved[1].Image.WidthEMU != 1828800 || moved[1].Image.HeightEMU != 914400 {
		t.Fatalf("resized image extent = %dx%d; want 1828800x914400", moved[1].Image.WidthEMU, moved[1].Image.HeightEMU)
	}
	output := filepath.Join(t.TempDir(), "moved-resized.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	image := reopened.WordBlocks()[1].Table.Rows[0].Cells[0].Paragraphs[1].Inlines[1].Image
	if image.ID != imageID || image.WidthEMU != 1828800 || image.HeightEMU != 914400 {
		t.Fatalf("reopened moved image = %#v", image)
	}
	imageBytes, err := reopened.WordImageBytes(imageID)
	if err != nil || !bytes.Equal(imageBytes, originalImage) {
		t.Fatalf("moved image media changed: %v", err)
	}
	if err := requireWordImageEditor(t, reopened).DeleteInlineImage(imageID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.WordImageBytes(imageID); err == nil {
		t.Fatal("deleted image remains addressable")
	}
	deletedSnapshot, err := reopened.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	deletedParts := readZipParts(t, deletedSnapshot)
	if _, exists := deletedParts["word/media/one.png"]; exists || strings.Contains(string(deletedParts["word/_rels/document.xml.rels"]), `Id="rIdPicture1"`) {
		t.Fatal("deleting the last drawing did not prune its now-unreferenced relationship and media")
	}
	if applied, err := requireWordHistory(t, reopened).Undo(); err != nil || !applied {
		t.Fatalf("undo image delete = %v, %v", applied, err)
	}
	restoredBytes, err := reopened.WordImageBytes(imageID)
	if err != nil || !bytes.Equal(restoredBytes, originalImage) {
		t.Fatalf("undo did not restore deleted media bytes: %v", err)
	}
	if applied, err := requireWordHistory(t, reopened).Redo(); err != nil || !applied {
		t.Fatalf("redo image delete = %v, %v", applied, err)
	}
	if _, err := reopened.WordImageBytes(imageID); err == nil {
		t.Fatal("redo did not remove deleted image")
	}
	_ = reopened.Close()
}

func TestRotateInlineImagePersistsInDrawingMLAndHistory(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	paragraph := doc.WordBlocks()[0].Paragraph
	imageBytes := imageFixtureBytes(t, 3, 2, color.NRGBA{R: 20, G: 110, B: 230, A: 255})
	imageID, err := requireWordImageEditor(t, doc).InsertInlineImage(paragraph.ID, "", "image/png", imageBytes)
	if err != nil {
		t.Fatal(err)
	}
	type wordImageRotationEditor interface {
		SetInlineImageRotation(imageID string, degrees float64) error
	}
	rotator, ok := any(doc).(wordImageRotationEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose inline image rotation")
	}
	if err := rotator.SetInlineImageRotation(imageID, 90); err != nil {
		t.Fatalf("rotate inline image clockwise: %v", err)
	}
	snapshot, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	documentXML := string(readZipPart(t, snapshot, "word/document.xml"))
	if !strings.Contains(documentXML, `<a:xfrm rot="5400000">`) {
		t.Fatalf("90 degree rotation was not written in DrawingML units: %s", documentXML)
	}
	reopened, err := OpenSnapshot(snapshot)
	if err != nil {
		t.Fatalf("reopen rotated image: %v", err)
	}
	reopenedImage, err := reopened.WordImageBytes(imageID)
	if err != nil || !bytes.Equal(reopenedImage, imageBytes) {
		t.Fatalf("rotating image changed the media bytes: %v", err)
	}
	_ = reopened.Close()
	history := requireWordHistory(t, doc)
	if applied, err := history.Undo(); err != nil || !applied {
		t.Fatalf("undo image rotation = %v, %v", applied, err)
	}
	undone, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(readZipPart(t, undone, "word/document.xml")), `rot="5400000"`) {
		t.Fatal("undo left the image rotated")
	}
	if applied, err := history.Redo(); err != nil || !applied {
		t.Fatalf("redo image rotation = %v, %v", applied, err)
	}
	redone, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readZipPart(t, redone, "word/document.xml")), `rot="5400000"`) {
		t.Fatal("redo did not restore the image rotation")
	}
}

func TestInvalidImagePayloadAndSizeLeavePackageAndHistoryUnchanged(t *testing.T) {
	doc, _ := mixedContentProjectionFixture(t)
	defer doc.Close()
	imageID := doc.WordBlocks()[0].Paragraph.Inlines[1].Image.ID
	before, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	editor := requireWordImageEditor(t, doc)
	validPNG := imageFixtureBytes(t, 1, 1, color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	for _, test := range []struct {
		name string
		fn   func() error
	}{
		{name: "empty data", fn: func() error { return editor.ReplaceInlineImage(imageID, "image/png", nil) }},
		{name: "mismatched media type", fn: func() error { return editor.ReplaceInlineImage(imageID, "image/jpeg", validPNG) }},
		{name: "unsupported media type", fn: func() error { return editor.ReplaceInlineImage(imageID, "image/gif", validPNG) }},
		{name: "zero width", fn: func() error { return editor.ResizeInlineImage(imageID, 0) }},
		{name: "negative width", fn: func() error { return editor.ResizeInlineImage(imageID, -10) }},
		{name: "extreme width", fn: func() error { return editor.ResizeInlineImage(imageID, 914400000) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.fn(); err == nil {
				t.Fatal("invalid image operation succeeded")
			}
			if doc.Dirty() || requireWordHistory(t, doc).CanUndo() {
				t.Fatal("invalid image operation changed dirty/history state")
			}
		})
	}
	after, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	beforeParts := readZipParts(t, before)
	afterParts := readZipParts(t, after)
	for name, data := range beforeParts {
		if !bytes.Equal(data, afterParts[name]) {
			t.Errorf("invalid operation changed package part %q", name)
		}
	}
}
