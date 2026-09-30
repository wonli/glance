package docx

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"glance/internal/document"
)

func imageFixtureBytes(t *testing.T, width, height int, colorValue color.NRGBA) []byte {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixels.SetNRGBA(x, y, colorValue)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, pixels); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func imageFixtureJPEG(t *testing.T, width, height int, colorValue color.NRGBA) []byte {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixels.SetNRGBA(x, y, colorValue)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, pixels, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func wordImageDrawing(docPrID int, relationshipID string, widthEMU, heightEMU int64) string {
	return `<w:r><w:drawing><wp:inline><wp:extent cx="` + strconvInt64(widthEMU) + `" cy="` + strconvInt64(heightEMU) + `"/><wp:docPr id="` + strconvInt(docPrID) + `" name="Picture ` + strconvInt(docPrID) + `"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:blipFill><a:blip r:embed="` + relationshipID + `"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`
}

func imageOperationFixture(t *testing.T, documentXML, relationshipsXML, contentTypesXML string, media map[string][]byte) *Document {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	parts := map[string][]byte{
		"[Content_Types].xml":          []byte(contentTypesXML),
		"word/document.xml":            []byte(documentXML),
		"word/_rels/document.xml.rels": []byte(relationshipsXML),
		"custom/untouched.bin":         []byte("keep this package part byte-for-byte"),
	}
	for name, data := range media {
		parts[name] = data
	}
	for name, data := range parts {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	doc, err := OpenSnapshot(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	return doc
}

func imageContentTypes() string {
	return `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`
}

func imageRelationshipRoot(children ...string) string {
	return `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(children, "") + `</Relationships>`
}

func imageDocRoot(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:body>` + body + `</w:body></w:document>`
}

func requireWordImageEditor(t *testing.T, doc *Document) document.WordImageEditor {
	t.Helper()
	editor, ok := any(doc).(document.WordImageEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose inline image editing")
	}
	return editor
}

func TestInsertInlineImageAfterTextRoundTripsAndUsesCollisionFreeParts(t *testing.T) {
	doc := imageOperationFixture(t,
		imageDocRoot(`<w:p><w:r><w:t>before</w:t></w:r><w:r><w:t>after</w:t></w:r></w:p>`),
		imageRelationshipRoot(`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`, `<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>`),
		imageContentTypes(), nil)
	defer doc.Close()
	original, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	paragraph := doc.WordBlocks()[0].Paragraph
	imageID, err := requireWordImageEditor(t, doc).InsertInlineImage(paragraph.ID, paragraph.Inlines[0].Text.ID, "image/png", imageFixtureBytes(t, 2, 1, color.NRGBA{R: 10, G: 80, B: 200, A: 255}))
	if err != nil {
		t.Fatal(err)
	}
	updated := doc.WordBlocks()[0].Paragraph
	if len(updated.Inlines) != 3 || updated.Inlines[0].Text.Text != "before" || updated.Inlines[1].Image == nil && updated.Inlines[1].Unsupported == nil || updated.Inlines[2].Text.Text != "after" {
		t.Fatalf("inserted inline order = %#v", updated.Inlines)
	}
	if updated.Inlines[1].Image == nil {
		t.Fatalf("inserted image projected as unsupported: %s", updated.Inlines[1].Unsupported.Description)
	}
	if updated.Inlines[1].Image.ID != imageID {
		t.Fatalf("inserted image id = %q; want %q", updated.Inlines[1].Image.ID, imageID)
	}
	if updated.Inlines[1].Image.WidthEMU != 19050 || updated.Inlines[1].Image.HeightEMU != 9525 {
		t.Fatalf("inserted image extent = %dx%d; want 19050x9525 EMU", updated.Inlines[1].Image.WidthEMU, updated.Inlines[1].Image.HeightEMU)
	}
	if !doc.Dirty() {
		t.Fatal("image insert did not mark document dirty")
	}
	output := filepath.Join(t.TempDir(), "inserted.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedImage := reopened.WordBlocks()[0].Paragraph.Inlines[1].Image
	if reopenedImage.ID != imageID {
		t.Fatalf("image ID after reopen = %q; want %q", reopenedImage.ID, imageID)
	}
	imageBytes, err := reopened.WordImageBytes(imageID)
	if err != nil || len(imageBytes) == 0 {
		t.Fatalf("read inserted image after reopen: bytes=%d err=%v", len(imageBytes), err)
	}
	snapshot, err := reopened.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	parts := readZipParts(t, snapshot)
	if !bytes.Equal(parts["custom/untouched.bin"], []byte("keep this package part byte-for-byte")) {
		t.Fatal("image insertion changed an unrelated package part")
	}
	relationships := string(parts["word/_rels/document.xml.rels"])
	if !strings.Contains(relationships, `Id="rId3"`) || !strings.Contains(relationships, `Target="media/glance-image-1.png"`) {
		t.Fatalf("image relationship did not skip existing rId collisions: %s", relationships)
	}
	contentTypes := string(parts["[Content_Types].xml"])
	if !strings.Contains(contentTypes, `Extension="png" ContentType="image/png"`) {
		t.Fatalf("missing PNG content type: %s", contentTypes)
	}
	assertImageUntouchedParts(t, original, snapshot, map[string]bool{"word/document.xml": true, "word/_rels/document.xml.rels": true, "[Content_Types].xml": true, "word/media/glance-image-1.png": true})
	if applied, err := requireWordHistory(t, doc).Undo(); err != nil || !applied {
		t.Fatalf("undo image insertion = %v, %v", applied, err)
	}
	if len(doc.WordBlocks()[0].Paragraph.Inlines) != 2 || !requireWordHistory(t, doc).CanRedo() {
		t.Fatal("undo did not remove inserted image or keep redo")
	}
	if applied, err := requireWordHistory(t, doc).Redo(); err != nil || !applied {
		t.Fatalf("redo image insertion = %v, %v", applied, err)
	}
	if len(doc.WordBlocks()[0].Paragraph.Inlines) != 3 {
		t.Fatal("redo did not restore inserted image")
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

func TestMissingImageRelationshipRemainsVisibleAndCannotBeMutated(t *testing.T) {
	doc := projectionFixtureWithContentTypes(t,
		imageDocRoot(`<w:p>`+wordImageDrawing(1, "rIdMissing", 914400, 457200)+`</w:p>`),
		imageRelationshipRoot(), imageContentTypes(), nil)
	defer doc.Close()
	block := doc.WordBlocks()[0].Paragraph.Inlines[0]
	if block.Kind != document.WordUnsupportedInline || block.Unsupported == nil {
		t.Fatalf("missing relationship projection = %#v", block)
	}
	if err := requireWordImageEditor(t, doc).DeleteInlineImage("img-1"); err == nil {
		t.Fatal("operation accepted an image with an unresolved relationship")
	}
	if doc.Dirty() {
		t.Fatal("missing relationship edit changed dirty state")
	}
}

func imageIDsInParagraph(paragraph document.WordParagraph) []string {
	var ids []string
	for _, inline := range paragraph.Inlines {
		if inline.Image != nil {
			ids = append(ids, inline.Image.ID)
		}
	}
	return ids
}

func assertImageUntouchedParts(t *testing.T, original, after []byte, changed map[string]bool) {
	t.Helper()
	beforeParts := readZipParts(t, original)
	afterParts := readZipParts(t, after)
	for name, want := range beforeParts {
		if changed[name] {
			continue
		}
		if !bytes.Equal(afterParts[name], want) {
			t.Errorf("untouched image package part %q changed", name)
		}
	}
}

func strconvInt(value int) string { return strconv.Itoa(value) }

func strconvInt64(value int64) string { return strconv.FormatInt(value, 10) }
