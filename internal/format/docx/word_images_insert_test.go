package docx

import (
	"bytes"
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
)

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

func TestInsertInlineImageConstrainsOversizedImage(t *testing.T) {
	doc := imageOperationFixture(t,
		imageDocRoot(`<w:p><w:r><w:t>text</w:t></w:r></w:p>`),
		imageRelationshipRoot(), imageContentTypes(), nil)
	defer doc.Close()
	paragraph := doc.WordBlocks()[0].Paragraph

	largeImageBytes := imageFixtureBytes(t, 2000, 3000, color.NRGBA{R: 50, G: 150, B: 250, A: 255})
	imageID, err := requireWordImageEditor(t, doc).InsertInlineImage(paragraph.ID, "", "image/png", largeImageBytes)
	if err != nil {
		t.Fatal(err)
	}
	updated := doc.WordBlocks()[0].Paragraph
	var img *document.WordImage
	for _, inline := range updated.Inlines {
		if inline.Image != nil && inline.Image.ID == imageID {
			img = inline.Image
			break
		}
	}
	if img == nil {
		t.Fatalf("inserted image %q not found in inlines: %+v", imageID, updated.Inlines)
	}
	const maxDefaultWidthEMU = int64(5.5 * 914400)
	const maxDefaultHeightEMU = int64(7.5 * 914400)
	if img.WidthEMU > maxDefaultWidthEMU || img.HeightEMU > maxDefaultHeightEMU {
		t.Fatalf("oversized image dimensions = %dx%d; want <= %dx%d", img.WidthEMU, img.HeightEMU, maxDefaultWidthEMU, maxDefaultHeightEMU)
	}
}
