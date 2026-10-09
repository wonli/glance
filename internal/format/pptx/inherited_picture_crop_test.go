package pptx

import (
	"bytes"
	"glance/internal/document"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openPictureCropTestDocument(t *testing.T, imageBytes []byte, srcRect string, shared, external bool) (*Document, string) {
	t.Helper()
	parts := pictureCropTestParts(imageBytes, srcRect, shared, external)
	source := writePPTXFixture(t, parts)
	doc, err := Open(source)
	if err != nil {
		t.Fatalf("Open picture crop fixture: %v", err)
	}
	return doc, source
}

func pictureCropTestParts(imageBytes []byte, srcRect string, shared, external bool) map[string][]byte {
	refs := []testPictureRef{{id: 7, rid: "rId2"}}
	if shared {
		refs = append(refs, testPictureRef{id: 8, rid: "rId2"})
	}
	parts := pictureFixtureParts(refs, []testImageRel{{id: "rId2", target: "../media/picture.png", external: external}}, map[string][]byte{"ppt/media/picture.png": imageBytes})
	slideXML := string(parts["ppt/slides/slide1.xml"])
	if srcRect != "" {
		slideXML = strings.Replace(slideXML,
			`<p:blipFill><a:blip r:embed="rId2"/><a:stretch>`,
			`<p:blipFill><a:blip r:embed="rId2"/><a:srcRect `+srcRect+`/><a:stretch>`, 1)
	}
	slideXML = strings.Replace(slideXML, `<p:cNvPr id="7" name="Picture 7"/>`, `<p:cNvPr id="7" name="Picture 7" descr="target alt text"/>`, 1)
	slideXML = strings.Replace(slideXML, `<a:xfrm><a:off x="0" y="0"/>`, `<a:xfrm rot="19110000"><a:off x="0" y="0"/>`, 1)
	parts["ppt/slides/slide1.xml"] = []byte(slideXML)
	return parts
}

func TestUpdateSlidePictureCropValidatesAndPreservesRevisionOnNoop(t *testing.T) {
	original := encodeTestRaster(t, "png", color.NRGBA{G: 220, A: 255})
	doc, _ := openPictureCropTestDocument(t, original, `l="10000" t="20000" r="30000" b="40000"`, false, false)
	defer doc.Close()
	before := doc.Snapshot()
	initial := presentationElementByID(t, before.Slides[0], 7).Crop
	if err := doc.UpdateSlidePictureCrop(0, 7, initial); err != nil {
		t.Fatalf("no-op crop: %v", err)
	}
	if after := doc.Snapshot(); after.Revision != before.Revision || after.Dirty != before.Dirty {
		t.Fatalf("no-op crop changed revision/dirty: before (%d,%t), after (%d,%t)", before.Revision, before.Dirty, after.Revision, after.Dirty)
	}
	for _, crop := range []document.PresentationCrop{
		{Left: -1},
		{Right: 100001},
		{Left: 70000, Right: 30000},
		{Top: 50000, Bottom: 50000},
	} {
		if err := doc.UpdateSlidePictureCrop(0, 7, crop); err == nil {
			t.Fatalf("invalid crop %+v was accepted", crop)
		}
	}
	after := doc.Snapshot()
	if after.Revision != before.Revision || after.Dirty != before.Dirty || presentationElementByID(t, after.Slides[0], 7).Crop != initial {
		t.Fatalf("invalid crops changed document: before %+v after %+v", before, after)
	}
	want := document.PresentationCrop{Left: 12000, Top: 8000, Right: 4000, Bottom: 10000}
	if err := doc.UpdateSlidePictureCrop(0, 7, want); err != nil {
		t.Fatalf("valid crop: %v", err)
	}
	changed := doc.Snapshot()
	if got := presentationElementByID(t, changed.Slides[0], 7).Crop; got != want || !changed.Dirty || changed.Revision != before.Revision+1 {
		t.Fatalf("updated crop/revision/dirty = %+v/%d/%t", got, changed.Revision, changed.Dirty)
	}
}

func TestInvalidExistingPictureCropIsReadOnly(t *testing.T) {
	original := encodeTestRaster(t, "png", color.NRGBA{B: 220, A: 255})
	doc, _ := openPictureCropTestDocument(t, original, `l="70000" t="0" r="40000" b="0"`, false, false)
	defer doc.Close()
	before := doc.Snapshot()
	elem := presentationElementByID(t, before.Slides[0], 7)
	if elem.Kind != document.ElementKindPicture || elem.Image == nil || !bytes.Equal(elem.ImageBytes, original) || elem.CanEditPicture {
		t.Fatalf("invalid-crop picture projection = %+v", elem)
	}
	if err := doc.UpdateSlidePictureCrop(0, 7, document.PresentationCrop{}); err == nil {
		t.Fatal("crop edit accepted an invalid existing crop")
	}
	if err := doc.ReplaceSlidePicture(0, 7, "image/png", encodeTestRaster(t, "png", color.NRGBA{R: 220, A: 255})); err == nil {
		t.Fatal("replacement accepted an invalid existing crop")
	}
	after := doc.Snapshot()
	if after.Dirty != before.Dirty || after.Revision != before.Revision || !bytes.Equal(presentationElementByID(t, after.Slides[0], 7).ImageBytes, original) {
		t.Fatal("invalid crop edit changed committed picture state")
	}
}

func TestPictureCropOverridesTrackSlideAddDelete(t *testing.T) {
	doc, _ := NewBlank()
	defer doc.Close()
	first := document.PresentationCrop{Left: 1000}
	second := document.PresentationCrop{Right: 2000}
	doc.slidePictureCropChanges = []map[uint32]document.PresentationCrop{{7: first}, {8: second}}
	if err := doc.AddSlide(); err != nil {
		t.Fatalf("AddSlide: %v", err)
	}
	if len(doc.slidePictureCropChanges) != 3 || doc.slidePictureCropChanges[0][7] != first || doc.slidePictureCropChanges[1][8] != second {
		t.Fatalf("crop overrides after AddSlide = %+v", doc.slidePictureCropChanges)
	}
	if err := doc.DeleteSlide(0); err != nil {
		t.Fatalf("DeleteSlide: %v", err)
	}
	if len(doc.slidePictureCropChanges) != 2 || doc.slidePictureCropChanges[0][8] != second {
		t.Fatalf("crop overrides after deleting first slide = %+v", doc.slidePictureCropChanges)
	}
}

func TestUpdateSlidePictureCropPersistsAndComposesWithTransform(t *testing.T) {
	original := encodeTestRaster(t, "png", color.NRGBA{B: 220, A: 255})
	doc, source := openPictureCropTestDocument(t, original, `l="10000" t="20000" r="30000" b="40000"`, true, false)
	defer doc.Close()
	sourceBefore, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read source before edits: %v", err)
	}
	wantCrop := document.PresentationCrop{Left: 12000, Top: 8000, Right: 4000, Bottom: 10000}
	wantBounds := document.PresentationRect{X: 14.5, Y: 21.25, W: 83.5, H: 46.75}
	if err := doc.UpdateSlidePictureCrop(0, 7, wantCrop); err != nil {
		t.Fatalf("UpdateSlidePictureCrop: %v", err)
	}
	if err := doc.UpdateSlideElementTransform(0, 7, wantBounds, 42.25); err != nil {
		t.Fatalf("UpdateSlideElementTransform: %v", err)
	}
	pictureOutline, pictureOutlineWidth := "#12ABEF", 2.0
	if err := doc.UpdateSlideElementStyle(0, 7, document.PresentationElementStylePatch{BorderColor: &pictureOutline, BorderWidthPT: &pictureOutlineWidth}); err != nil {
		t.Fatalf("UpdateSlideElementStyle picture outline: %v", err)
	}
	output := filepath.Join(t.TempDir(), "cropped-and-transformed.pptx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatalf("reopen edited picture: %v", err)
	}
	defer reopened.Close()
	got := presentationElementByID(t, reopened.Snapshot().Slides[0], 7)
	if got.Crop != wantCrop || got.Bounds != wantBounds || got.RotationDeg != 42.25 || !bytes.Equal(got.ImageBytes, original) || got.AltText != "target alt text" || got.BorderColor != pictureOutline || math.Abs(got.BorderWidth-pictureOutlineWidth) > 1e-8 {
		t.Fatalf("reopened picture = crop %+v, bounds %+v, rotation %v, outline %q/%.2f, bytes=%d alt=%q", got.Crop, got.Bounds, got.RotationDeg, got.BorderColor, got.BorderWidth, len(got.ImageBytes), got.AltText)
	}
	if after, err := os.ReadFile(source); err != nil || !bytes.Equal(sourceBefore, after) {
		t.Fatalf("SaveAs modified source package: err=%v", err)
	}
}

func TestPictureCropSnapshotBytesRetainsCrop(t *testing.T) {
	original := encodeTestRaster(t, "png", color.NRGBA{G: 220, A: 255})
	replacement := encodeTestRaster(t, "jpeg", color.NRGBA{R: 220, A: 255})
	doc, _ := openPictureCropTestDocument(t, original, `l="10000" t="0" r="0" b="0"`, true, false)
	defer doc.Close()
	if err := doc.ReplaceSlidePicture(0, 7, "image/jpeg", replacement); err != nil {
		t.Fatalf("ReplaceSlidePicture: %v", err)
	}
	wantCrop := document.PresentationCrop{Left: 15000, Top: 5000, Right: 10000, Bottom: 20000}
	if err := doc.UpdateSlidePictureCrop(0, 7, wantCrop); err != nil {
		t.Fatalf("UpdateSlidePictureCrop: %v", err)
	}
	data, err := doc.SnapshotBytes()
	if err != nil {
		t.Fatalf("SnapshotBytes: %v", err)
	}
	recovered, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer recovered.Close()
	got := presentationElementByID(t, recovered.Snapshot().Slides[0], 7)
	if got.Crop != wantCrop || !bytes.Equal(got.ImageBytes, replacement) || got.AltText != "target alt text" {
		t.Fatalf("recovered picture = crop %+v bytes=%d alt=%q", got.Crop, len(got.ImageBytes), got.AltText)
	}
}

func TestPictureCropPatchPreservesSharedMediaAndUntouchedParts(t *testing.T) {
	original := encodeTestRaster(t, "png", color.NRGBA{B: 220, A: 255})
	parts := pictureCropTestParts(original, `l="10000" t="0" r="0" b="0"`, true, false)
	parts["ppt/custom/protected.bin"] = []byte{0, 1, 2, 0xfe, 0xff}
	source := writePPTXFixture(t, parts)
	doc, err := Open(source)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()
	wantCrop := document.PresentationCrop{Left: 20000, Top: 5000, Right: 10000, Bottom: 5000}
	if err := doc.UpdateSlidePictureCrop(0, 7, wantCrop); err != nil {
		t.Fatalf("UpdateSlidePictureCrop: %v", err)
	}
	output := filepath.Join(t.TempDir(), "crop-preserves-package.pptx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	slide := reopened.Snapshot().Slides[0]
	target := presentationElementByID(t, slide, 7)
	sibling := presentationElementByID(t, slide, 8)
	if target.Crop != wantCrop || sibling.Crop != (document.PresentationCrop{}) || !bytes.Equal(target.ImageBytes, original) || !bytes.Equal(sibling.ImageBytes, original) {
		t.Fatalf("shared picture results = target %+v, sibling %+v", target, sibling)
	}
	if got := readPPTXPartForTest(t, output, "ppt/custom/protected.bin"); !bytes.Equal(got, parts["ppt/custom/protected.bin"]) {
		t.Fatalf("untouched package part changed: %v", got)
	}
	if got := readPPTXPartForTest(t, output, "ppt/slides/_rels/slide1.xml.rels"); !bytes.Equal(got, readPPTXPartForTest(t, source, "ppt/slides/_rels/slide1.xml.rels")) {
		t.Fatal("crop patch changed picture relationships")
	}
}

func TestPictureCropSaveFailurePreservesOriginalAndDirtyState(t *testing.T) {
	original := encodeTestRaster(t, "png", color.NRGBA{G: 220, A: 255})
	doc, source := openPictureCropTestDocument(t, original, `l="10000" t="0" r="0" b="0"`, false, false)
	defer doc.Close()
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read source before edit: %v", err)
	}
	if err := doc.UpdateSlidePictureCrop(0, 7, document.PresentationCrop{Left: 15000, Right: 5000}); err != nil {
		t.Fatalf("UpdateSlidePictureCrop: %v", err)
	}
	revision := doc.Revision()
	blockedTarget := filepath.Join(t.TempDir(), "directory-target")
	if err := os.Mkdir(blockedTarget, 0755); err != nil {
		t.Fatalf("create directory target: %v", err)
	}
	if err := doc.SaveAs(blockedTarget); err == nil {
		t.Fatal("SaveAs to a directory unexpectedly succeeded")
	}
	if after, err := os.ReadFile(source); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("failed save changed source bytes: err=%v", err)
	}
	if !doc.IsDirty() || doc.Revision() != revision {
		t.Fatalf("failed save lost dirty state or changed revision: dirty=%t revision=%d want %d", doc.IsDirty(), doc.Revision(), revision)
	}
}
