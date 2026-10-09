package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"glance/internal/document"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func transformTestShapeXML(id int, transform string) string {
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="editable shape"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr>%s<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:rPr lang="en-US" sz="1800"/><a:t>content</a:t></a:r></a:p></p:txBody></p:sp>`, id, transform)
}

func openTransformTestDocument(t *testing.T, shape string) (*Document, string) {
	t.Helper()
	source := writePPTXFixture(t, inheritedFixtureParts(inheritedSlideXML("", shape), inheritedLayoutXML("", ""), inheritedMasterXML(""), nil))
	doc, err := Open(source)
	if err != nil {
		t.Fatalf("Open transform fixture: %v", err)
	}
	return doc, source
}

func presentationElementByID(t *testing.T, slide document.PresentationSlide, id int64) document.PresentationElement {
	t.Helper()
	for _, element := range slide.Elements {
		if element.ID == id {
			return element
		}
	}
	t.Fatalf("slide has no element with ID %d: %+v", id, slide.Elements)
	return document.PresentationElement{}
}

func transformTestShapeNode(t *testing.T, path string, id uint32) pptxXMLNode {
	t.Helper()
	data := readPPTXPartForTest(t, path, "ppt/slides/slide1.xml")
	var root pptxXMLNode
	if err := xml.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse slide XML: %v", err)
	}
	tree := shapeTree(root)
	if tree != nil {
		for _, shape := range tree.Children {
			if shape.XMLName.Local != "sp" && shape.XMLName.Local != "pic" {
				continue
			}
			if actual := slideShapeID(shape); actual == id {
				return shape
			}
		}
	}
	t.Fatalf("slide XML has no shape ID %d", id)
	return pptxXMLNode{}
}

func slideShapeID(shape pptxXMLNode) uint32 {
	nv := shape.child("nvSpPr")
	if nv == nil {
		nv = shape.child("nvPicPr")
	}
	if nv == nil {
		return 0
	}
	cNvPr := nv.child("cNvPr")
	if cNvPr == nil {
		return 0
	}
	id, _ := strconv.ParseUint(cNvPr.attr("id"), 10, 32)
	return uint32(id)
}

func readPPTXPartForTest(t *testing.T, path, name string) []byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open package %s: %v", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatalf("stat package %s: %v", path, err)
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		t.Fatalf("open package ZIP %s: %v", path, err)
	}
	for _, part := range reader.File {
		if part.Name == name {
			data, err := readOPCPart(part)
			if err != nil {
				t.Fatalf("read package part %s: %v", name, err)
			}
			return data
		}
	}
	t.Fatalf("package %s is missing part %s", path, name)
	return nil
}

func TestUpdateSlideElementTransformPersistsBoundsAndRotation(t *testing.T) {
	doc, source := openTransformTestDocument(t, transformTestShapeXML(7, `<a:xfrm><a:off x="127000" y="254000"/><a:ext cx="1016000" cy="635000"/></a:xfrm>`))
	defer doc.Close()

	wantBounds := document.PresentationRect{X: -2.25, Y: 7.5, W: 91.25, H: 40.5}
	if err := doc.UpdateSlideElementTransform(0, 7, wantBounds, 37.5); err != nil {
		t.Fatalf("UpdateSlideElementTransform: %v", err)
	}
	if !doc.IsDirty() {
		t.Fatal("transform update did not mark document dirty")
	}

	output := filepath.Join(t.TempDir(), "transformed.pptx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatalf("SaveAs transformed presentation: %v", err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatalf("reopen transformed presentation: %v", err)
	}
	defer reopened.Close()
	element := presentationElementByID(t, reopened.Snapshot().Slides[0], 7)
	if element.Bounds != wantBounds || element.RotationDeg != 37.5 {
		t.Fatalf("reopened transform = bounds %+v, rotation %v; want %+v, 37.5", element.Bounds, element.RotationDeg, wantBounds)
	}
	if after, err := os.ReadFile(source); err != nil || len(after) == 0 {
		t.Fatalf("source fixture became unreadable: err=%v", err)
	}
}

func TestUpdateSlidePictureTransformPersists(t *testing.T) {
	parts := pictureFixtureParts(
		[]testPictureRef{{id: 7, rid: "rId2"}},
		[]testImageRel{{id: "rId2", target: "../media/picture.png"}},
		map[string][]byte{"ppt/media/picture.png": encodeTestRaster(t, "png", color.NRGBA{R: 220, A: 255})},
	)
	source := writePPTXFixture(t, parts)
	doc, err := Open(source)
	if err != nil {
		t.Fatalf("Open picture fixture: %v", err)
	}
	defer doc.Close()
	element := presentationElementByID(t, doc.Snapshot().Slides[0], 7)
	if element.Kind != document.ElementKindPicture {
		t.Fatalf("element kind = %q, want picture", element.Kind)
	}
	want := document.PresentationRect{X: 47.25, Y: 13.5, W: 63.75, H: 52.25}
	if err := doc.UpdateSlideElementTransform(0, 7, want, 319.5); err != nil {
		t.Fatalf("UpdateSlideElementTransform picture: %v", err)
	}
	output := filepath.Join(t.TempDir(), "picture-transformed.pptx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatalf("SaveAs picture transform: %v", err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatalf("reopen picture transform: %v", err)
	}
	defer reopened.Close()
	got := presentationElementByID(t, reopened.Snapshot().Slides[0], 7)
	if got.Bounds != want || got.RotationDeg != 319.5 || got.Image == nil {
		t.Fatalf("reopened picture = bounds %+v rotation %v image=%t, want %+v 319.5 with image", got.Bounds, got.RotationDeg, got.Image != nil, want)
	}
}

func TestUpdateSlideElementTransformNoopPreservesRevision(t *testing.T) {
	doc, _ := openTransformTestDocument(t, transformTestShapeXML(7, `<a:xfrm rot="900000"><a:off x="139700" y="152400"/><a:ext cx="1016000" cy="635000"/></a:xfrm>`))
	defer doc.Close()

	before := doc.Snapshot()
	element := presentationElementByID(t, before.Slides[0], 7)
	if err := doc.UpdateSlideElementTransform(0, 7, element.Bounds, element.RotationDeg); err != nil {
		t.Fatalf("no-op UpdateSlideElementTransform: %v", err)
	}
	after := doc.Snapshot()
	if after.Revision != before.Revision || after.Dirty != before.Dirty {
		t.Fatalf("no-op changed revision/dirty: before (%d,%t), after (%d,%t)", before.Revision, before.Dirty, after.Revision, after.Dirty)
	}
}

func TestUpdateSlideElementTransformRejectsInvalidGeometry(t *testing.T) {
	doc, _ := openTransformTestDocument(t, transformTestShapeXML(7, `<a:xfrm><a:off x="127000" y="254000"/><a:ext cx="1016000" cy="635000"/></a:xfrm>`))
	defer doc.Close()

	before := doc.Snapshot()
	valid := document.PresentationRect{X: 2, Y: 3, W: 40, H: 30}
	for _, test := range []struct {
		name   string
		slide  int
		id     int64
		bounds document.PresentationRect
		angle  float64
	}{
		{name: "nan x", slide: 0, id: 7, bounds: document.PresentationRect{X: math.NaN(), Y: 0, W: 20, H: 20}},
		{name: "infinite rotation", slide: 0, id: 7, bounds: valid, angle: math.Inf(1)},
		{name: "too narrow", slide: 0, id: 7, bounds: document.PresentationRect{W: 7.99, H: 20}},
		{name: "stale shape id", slide: 0, id: 99, bounds: valid},
		{name: "invalid slide", slide: 1, id: 7, bounds: valid},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := doc.UpdateSlideElementTransform(test.slide, test.id, test.bounds, test.angle); err == nil {
				t.Fatal("invalid transform was accepted")
			}
		})
	}
	after := doc.Snapshot()
	beforeElement := presentationElementByID(t, before.Slides[0], 7)
	afterElement := presentationElementByID(t, after.Slides[0], 7)
	if after.Revision != before.Revision || after.Dirty != before.Dirty || afterElement.Bounds != beforeElement.Bounds || afterElement.RotationDeg != beforeElement.RotationDeg {
		t.Fatalf("rejected transforms changed document state: before %+v, after %+v", before, after)
	}
}

func TestTransformPatchPreservesLocksAndFlips(t *testing.T) {
	shape := transformTestShapeXML(7, `<a:xfrm rot="300000" flipH="1" flipV="1" extLst="preserve-me"><a:off x="127000" y="254000"/><a:ext cx="1016000" cy="635000"/></a:xfrm>`)
	shape = strings.Replace(shape, `<p:cNvSpPr/>`, `<p:cNvSpPr><a:spLocks noChangeAspect="1"/></p:cNvSpPr>`, 1)
	doc, _ := openTransformTestDocument(t, shape)
	defer doc.Close()
	element := presentationElementByID(t, doc.Snapshot().Slides[0], 7)
	if !element.AspectRatioLocked {
		t.Fatal("explicit noChangeAspect lock was not projected")
	}
	want := document.PresentationRect{X: 13.25, Y: 19.5, W: 80, H: 45}
	if err := doc.UpdateSlideElementTransform(0, 7, want, 112.25); err != nil {
		t.Fatalf("UpdateSlideElementTransform: %v", err)
	}
	output := filepath.Join(t.TempDir(), "preserved-flags.pptx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	shapeNode := transformTestShapeNode(t, output, 7)
	xfrm := shapeNode.child("spPr").child("xfrm")
	if xfrm.attr("flipH") != "1" || xfrm.attr("flipV") != "1" || xfrm.attr("extLst") != "preserve-me" {
		t.Fatalf("transform attributes were not preserved: %+v", xfrm.Attrs)
	}
	locks := shapeNode.child("nvSpPr").child("cNvSpPr").child("spLocks")
	if locks == nil || locks.attr("noChangeAspect") != "1" {
		t.Fatalf("aspect-ratio lock was not preserved: %+v", locks)
	}
}

func TestTransformSnapshotBytesRetainsGeometry(t *testing.T) {
	doc, _ := openTransformTestDocument(t, transformTestShapeXML(7, `<a:xfrm><a:off x="127000" y="254000"/><a:ext cx="1016000" cy="635000"/></a:xfrm>`))
	defer doc.Close()
	want := document.PresentationRect{X: 33.5, Y: -4.25, W: 77.5, H: 25.25}
	if err := doc.UpdateSlideElementTransform(0, 7, want, 221.75); err != nil {
		t.Fatalf("UpdateSlideElementTransform: %v", err)
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
	element := presentationElementByID(t, recovered.Snapshot().Slides[0], 7)
	if element.Bounds != want || element.RotationDeg != 221.75 {
		t.Fatalf("recovered transform = %+v, %v; want %+v, 221.75", element.Bounds, element.RotationDeg, want)
	}
}

func TestTransformSaveFailurePreservesOriginal(t *testing.T) {
	doc, source := openTransformTestDocument(t, transformTestShapeXML(7, `<a:xfrm><a:off x="127000" y="254000"/><a:ext cx="1016000" cy="635000"/></a:xfrm>`))
	defer doc.Close()
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read source before edit: %v", err)
	}
	if err := doc.UpdateSlideElementTransform(0, 7, document.PresentationRect{X: 1, Y: 2, W: 20, H: 20}, 15); err != nil {
		t.Fatalf("UpdateSlideElementTransform: %v", err)
	}
	revision := doc.Revision()
	blockedTarget := filepath.Join(t.TempDir(), "directory-target")
	if err := os.Mkdir(blockedTarget, 0755); err != nil {
		t.Fatalf("create directory target: %v", err)
	}
	if err := doc.SaveAs(blockedTarget); err == nil {
		t.Fatal("SaveAs to a directory unexpectedly succeeded")
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("failed save changed source bytes: err=%v", err)
	}
	if !doc.IsDirty() || doc.Revision() != revision {
		t.Fatalf("failed save lost dirty state or changed revision: dirty=%t revision=%d want %d", doc.IsDirty(), doc.Revision(), revision)
	}
}

func TestInheritedPlaceholderTransformMaterializesLocally(t *testing.T) {
	slideXML := inheritedSlideXML("", inheritedEmptyPlaceholderXML(10, "title", "ctrTitle", ""))
	layoutShape := inheritedPlaceholderXML(31, "layout title", "ctrTitle", "", `<a:xfrm rot="900000" flipH="1"><a:off x="1270000" y="1397000"/><a:ext cx="6350000" cy="1016000"/></a:xfrm>`)
	parts := inheritedFixtureParts(slideXML, inheritedLayoutXML("", layoutShape), inheritedMasterXML(""), nil)
	source := writePPTXFixture(t, parts)
	layoutBefore := readPPTXPartForTest(t, source, "ppt/slideLayouts/slideLayout1.xml")
	masterBefore := readPPTXPartForTest(t, source, "ppt/slideMasters/slideMaster1.xml")
	doc, err := Open(source)
	if err != nil {
		t.Fatalf("Open placeholder fixture: %v", err)
	}
	defer doc.Close()
	placeholder := presentationElementByID(t, doc.Snapshot().Slides[0], 10)
	if placeholder.Bounds != (document.PresentationRect{X: 100, Y: 110, W: 500, H: 80}) || placeholder.RotationDeg != 15 {
		t.Fatalf("inherited placeholder transform = %+v, %v", placeholder.Bounds, placeholder.RotationDeg)
	}
	want := document.PresentationRect{X: 85.5, Y: 42.25, W: 420, H: 73.5}
	if err := doc.UpdateSlideElementTransform(0, 10, want, 42.75); err != nil {
		t.Fatalf("UpdateSlideElementTransform: %v", err)
	}
	if err := doc.Save(); err != nil {
		t.Fatalf("Save materialized placeholder transform: %v", err)
	}
	reopened, err := Open(source)
	if err != nil {
		t.Fatalf("reopen materialized placeholder: %v", err)
	}
	defer reopened.Close()
	materialized := presentationElementByID(t, reopened.Snapshot().Slides[0], 10)
	if materialized.Bounds != want || materialized.RotationDeg != 42.75 {
		t.Fatalf("reopened placeholder transform = %+v, %v; want %+v, 42.75", materialized.Bounds, materialized.RotationDeg, want)
	}
	shapeNode := transformTestShapeNode(t, source, 10)
	materializedXfrm := shapeNode.child("spPr").child("xfrm")
	if materializedXfrm == nil {
		t.Fatal("slide-local placeholder has no materialized transform")
	}
	if materializedXfrm.attr("flipH") != "1" {
		t.Fatalf("materialized transform lost inherited horizontal flip: %+v", materializedXfrm.Attrs)
	}
	if !bytes.Equal(layoutBefore, readPPTXPartForTest(t, source, "ppt/slideLayouts/slideLayout1.xml")) || !bytes.Equal(masterBefore, readPPTXPartForTest(t, source, "ppt/slideMasters/slideMaster1.xml")) {
		t.Fatal("materializing a placeholder modified its layout or master part")
	}
}
