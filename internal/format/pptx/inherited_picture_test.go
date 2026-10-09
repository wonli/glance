package pptx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"glance/internal/document"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func pngConfigOnly(width, height uint32) []byte {
	data := append([]byte(nil), []byte("\x89PNG\r\n\x1a\n")...)
	chunk := make([]byte, 17)
	binary.BigEndian.PutUint32(chunk[0:4], 13)
	copy(chunk[4:8], "IHDR")
	binary.BigEndian.PutUint32(chunk[8:12], width)
	binary.BigEndian.PutUint32(chunk[12:16], height)
	chunk[16] = 8
	chunk = append(chunk, 6, 0, 0, 0)
	crc := crc32.ChecksumIEEE(chunk[4:])
	chunk = append(chunk, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
	data = append(data, chunk...)
	return data
}

func testEMFRectangle() []byte {
	brush := testEMFRecord(39, testEMFWords(1, 0, 255, 0))
	selectBrush := testEMFRecord(37, testEMFWords(1))
	rectangle := testEMFRecord(43, testEMFWords(0, 0, 10, 10))
	eof := testEMFRecord(14, testEMFWords(0, 0, 20))
	return testEMFFile(brush, selectBrush, rectangle, eof)
}

func testEMFRecord(recordType uint32, payload []byte) []byte {
	record := make([]byte, 8+len(payload))
	for i, word := range []uint32{recordType, uint32(len(record))} {
		record[i*4] = byte(word)
		record[i*4+1] = byte(word >> 8)
		record[i*4+2] = byte(word >> 16)
		record[i*4+3] = byte(word >> 24)
	}
	copy(record[8:], payload)
	return record
}

func testEMFWords(words ...uint32) []byte {
	data := make([]byte, len(words)*4)
	for i, word := range words {
		data[i*4] = byte(word)
		data[i*4+1] = byte(word >> 8)
		data[i*4+2] = byte(word >> 16)
		data[i*4+3] = byte(word >> 24)
	}
	return data
}

func testEMFFile(records ...[]byte) []byte {
	const headerSize = 108
	totalSize := headerSize
	for _, record := range records {
		totalSize += len(record)
	}
	header := make([]byte, headerSize)
	for offset, word := range map[int]uint32{
		0: 1, 4: headerSize, 16: 10, 20: 10,
		40: 0x464d4520, 44: 0x00010000, 48: uint32(totalSize), 52: uint32(1 + len(records)),
		72: 100, 76: 100, 80: 26, 84: 26,
	} {
		header[offset] = byte(word)
		header[offset+1] = byte(word >> 8)
		header[offset+2] = byte(word >> 16)
		header[offset+3] = byte(word >> 24)
	}
	header[56] = 2 // handle count
	data := append([]byte(nil), header...)
	for _, record := range records {
		data = append(data, record...)
	}
	return data
}

func firstPictureImage(t *testing.T, parts map[string][]byte) image.Image {
	t.Helper()
	doc, err := Open(writePPTXFixture(t, parts))
	if err != nil {
		t.Fatalf("Open fixture: %v", err)
	}
	defer doc.Close()

	snapshot := doc.Snapshot()
	if len(snapshot.Slides) != 1 {
		t.Fatalf("fixture slide count = %d, want 1", len(snapshot.Slides))
	}
	for _, elem := range snapshot.Slides[0].Elements {
		if elem.Kind == "picture" {
			return elem.Image
		}
	}
	t.Fatalf("fixture contains no picture element: %+v", snapshot.Slides[0].Elements)
	return nil
}

func pictureFixtureParts(refs []testPictureRef, imageRels []testImageRel, images map[string][]byte) map[string][]byte {
	var shapes strings.Builder
	for _, ref := range refs {
		shapes.WriteString(fmt.Sprintf(`<p:pic><p:nvPicPr><p:cNvPr id="%d" name="Picture %d"/><p:cNvPicPr/><p:nvPr/></p:nvPicPr>`+
			`<p:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>`+
			`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="1270000" cy="1270000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`, ref.id, ref.id, ref.rid))
	}

	var relXML strings.Builder
	relXML.WriteString(`<Relationships xmlns="` + testPackageRelNS + `"><Relationship Id="rId1" Type="` + testOfficeRelNS + `/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>`)
	for _, rel := range imageRels {
		targetMode := ""
		if rel.external {
			targetMode = ` TargetMode="External"`
		}
		relXML.WriteString(fmt.Sprintf(`<Relationship Id="%s" Type="%s/image" Target="%s"%s/>`, rel.id, testOfficeRelNS, rel.target, targetMode))
	}
	relXML.WriteString(`</Relationships>`)

	slideXML := `<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` + shapes.String() +
		`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`
	parts := map[string][]byte{
		"ppt/slides/slide1.xml":            []byte(slideXML),
		"ppt/slides/_rels/slide1.xml.rels": []byte(relXML.String()),
	}
	for name, data := range images {
		parts[name] = data
	}
	return parts
}

func encodeTestRaster(t *testing.T, format string, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 3, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&buf, img)
	case "jpeg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100})
	case "gif":
		err = gif.Encode(&buf, img, nil)
	default:
		t.Fatalf("unsupported test image format %q", format)
	}
	if err != nil {
		t.Fatalf("encode %s fixture: %v", format, err)
	}
	return buf.Bytes()
}

func TestPPTXPictureUsesItsOwningPartRelationship(t *testing.T) {
	red := encodeTestRaster(t, "png", color.NRGBA{R: 240, A: 255})
	blue := encodeTestRaster(t, "png", color.NRGBA{B: 240, A: 255})
	parts := pictureFixtureParts(
		[]testPictureRef{{id: 2, rid: "rId2"}},
		[]testImageRel{
			{id: "rId2", target: "../assets/image2.png"},
			{id: "rId3", target: "../media/unrelated.png"},
		},
		map[string][]byte{
			"ppt/assets/image2.png":   red,
			"ppt/media/unrelated.png": blue,
		},
	)

	img := firstPictureImage(t, parts)
	if img == nil {
		t.Fatal("picture image was not resolved through its owning slide relationship")
	}
	got := color.NRGBAModel.Convert(img.At(img.Bounds().Min.X, img.Bounds().Min.Y)).(color.NRGBA)
	if got.R < 220 || got.B > 20 {
		t.Fatalf("picture resolved wrong media: got %#v, want red image2", got)
	}
}

func TestPPTXPictureWithUnsupportedMediaHasNoImage(t *testing.T) {
	parts := pictureFixtureParts(
		[]testPictureRef{{id: 2, rid: "rId2"}},
		[]testImageRel{{id: "rId2", target: "../media/broken.png"}},
		map[string][]byte{
			"ppt/media/broken.png":    []byte("not a PNG"),
			"ppt/media/unrelated.png": encodeTestRaster(t, "png", color.NRGBA{G: 255, A: 255}),
		},
	)
	if img := firstPictureImage(t, parts); img != nil {
		t.Fatal("unsupported referenced media borrowed an unrelated cached image")
	}
}

func TestPPTXReferencedRasterDecoderSupportsPNGJPEGGIF(t *testing.T) {
	formats := []string{"png", "jpeg", "gif"}
	refs := make([]testPictureRef, 0, len(formats))
	rels := make([]testImageRel, 0, len(formats))
	images := make(map[string][]byte, len(formats))
	for i, format := range formats {
		id := i + 2
		rid := fmt.Sprintf("rId%d", id)
		filename := fmt.Sprintf("image%d.%s", id, format)
		refs = append(refs, testPictureRef{id: id, rid: rid})
		rels = append(rels, testImageRel{id: rid, target: "../assets/" + filename})
		images["ppt/assets/"+filename] = encodeTestRaster(t, format, color.NRGBA{R: uint8(40 * id), G: 70, B: 90, A: 255})
	}
	parts := pictureFixtureParts(refs, rels, images)
	doc, err := Open(writePPTXFixture(t, parts))
	if err != nil {
		t.Fatalf("Open fixture: %v", err)
	}
	defer doc.Close()

	snapshot := doc.Snapshot()
	if len(snapshot.Slides) != 1 || len(snapshot.Slides[0].Elements) != len(formats) {
		t.Fatalf("fixture pictures = %+v, want %d picture elements", snapshot.Slides, len(formats))
	}
	for index, elem := range snapshot.Slides[0].Elements {
		if elem.Image == nil {
			t.Errorf("%s relationship resolved no image", formats[index])
		}
	}
}

func TestPictureProjectionIncludesBytesAndCrop(t *testing.T) {
	original := encodeTestRaster(t, "png", color.NRGBA{G: 220, A: 255})
	doc, _ := openPictureCropTestDocument(t, original, `l="10000" t="20000" r="30000" b="40000"`, true, false)
	defer doc.Close()

	slide := doc.Snapshot().Slides[0]
	target := presentationElementByID(t, slide, 7)
	if target.Kind != document.ElementKindPicture || !bytes.Equal(target.ImageBytes, original) || target.Image == nil {
		t.Fatalf("picture source projection = kind %q bytes=%d image=%t", target.Kind, len(target.ImageBytes), target.Image != nil)
	}
	if target.Crop != (document.PresentationCrop{Left: 10000, Top: 20000, Right: 30000, Bottom: 40000}) {
		t.Fatalf("picture crop = %+v", target.Crop)
	}
	if !target.CanEditPicture {
		t.Fatal("valid slide-local PNG picture is not marked editable")
	}
}

func TestReplaceSlidePicturePreservesIdentityAndResetsCrop(t *testing.T) {
	original := encodeTestRaster(t, "png", color.NRGBA{B: 220, A: 255})
	replacement := encodeTestRaster(t, "jpeg", color.NRGBA{R: 220, A: 255})
	doc, _ := openPictureCropTestDocument(t, original, `l="10000" t="0" r="0" b="0"`, true, false)
	defer doc.Close()

	before := doc.Snapshot().Slides[0]
	targetBefore := presentationElementByID(t, before, 7)
	siblingBefore := presentationElementByID(t, before, 8)
	if err := doc.ReplaceSlidePicture(0, 7, "image/jpeg", replacement); err != nil {
		t.Fatalf("ReplaceSlidePicture: %v", err)
	}
	after := doc.Snapshot().Slides[0]
	targetAfter := presentationElementByID(t, after, 7)
	siblingAfter := presentationElementByID(t, after, 8)
	if targetAfter.ID != targetBefore.ID || targetAfter.Bounds != targetBefore.Bounds || targetAfter.RotationDeg != targetBefore.RotationDeg || targetAfter.AltText != "target alt text" {
		t.Fatalf("replacement changed target identity/frame/rotation/alt text: before %+v after %+v", targetBefore, targetAfter)
	}
	if targetAfter.Crop != (document.PresentationCrop{}) || !bytes.Equal(targetAfter.ImageBytes, replacement) {
		t.Fatalf("replacement bytes/crop = bytes %d crop %+v", len(targetAfter.ImageBytes), targetAfter.Crop)
	}
	if !bytes.Equal(siblingAfter.ImageBytes, original) || siblingAfter.ID != siblingBefore.ID {
		t.Fatalf("replacement changed sibling sharing source media: before %+v after %+v", siblingBefore, siblingAfter)
	}
	if !doc.IsDirty() {
		t.Fatal("replacement did not mark document dirty")
	}
}

func TestReplaceSlidePictureRejectsUnsupportedTargets(t *testing.T) {
	imageBytes := encodeTestRaster(t, "png", color.NRGBA{R: 220, A: 255})
	tests := []struct {
		name string
		open func(*testing.T) *Document
		id   int64
	}{
		{name: "missing id", open: func(t *testing.T) *Document {
			doc, _ := openPictureCropTestDocument(t, imageBytes, "", false, false)
			return doc
		}, id: 999},
		{name: "external relationship", open: func(t *testing.T) *Document {
			doc, _ := openPictureCropTestDocument(t, imageBytes, "", false, true)
			return doc
		}, id: 7},
		{name: "non-picture shape", open: func(t *testing.T) *Document {
			doc, _ := openTransformTestDocument(t, transformTestShapeXML(7, `<a:xfrm><a:off x="0" y="0"/><a:ext cx="1270000" cy="1270000"/></a:xfrm>`))
			return doc
		}, id: 7},
		{name: "duplicate picture id", open: func(t *testing.T) *Document {
			parts := pictureFixtureParts(
				[]testPictureRef{{id: 7, rid: "rId2"}, {id: 7, rid: "rId2"}},
				[]testImageRel{{id: "rId2", target: "../media/picture.png"}},
				map[string][]byte{"ppt/media/picture.png": imageBytes},
			)
			doc, err := Open(writePPTXFixture(t, parts))
			if err != nil {
				t.Fatalf("Open duplicate ID fixture: %v", err)
			}
			return doc
		}, id: 7},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := test.open(t)
			defer doc.Close()
			before := doc.Snapshot()
			if err := doc.ReplaceSlidePicture(0, test.id, "image/png", imageBytes); err == nil {
				t.Fatal("unsupported replacement unexpectedly succeeded")
			}
			after := doc.Snapshot()
			if after.Revision != before.Revision || after.Dirty != before.Dirty {
				t.Fatalf("rejected replacement changed state: before (%d,%t), after (%d,%t)", before.Revision, before.Dirty, after.Revision, after.Dirty)
			}
		})
	}
}

func TestReplaceSlidePictureRejectsInvalidImageData(t *testing.T) {
	original := encodeTestRaster(t, "png", color.NRGBA{B: 220, A: 255})
	doc, _ := openPictureCropTestDocument(t, original, "", false, false)
	defer doc.Close()
	before := doc.Snapshot()
	for _, test := range []struct {
		name      string
		mediaType string
		data      []byte
		wantError string
	}{
		{name: "corrupt", mediaType: "image/png", data: []byte("broken"), wantError: "损坏"},
		{name: "format mismatch", mediaType: "image/png", data: encodeTestRaster(t, "jpeg", color.NRGBA{R: 220, A: 255}), wantError: "与实际"},
		{name: "over edge limit", mediaType: "image/png", data: pngConfigOnly(32769, 1), wantError: "尺寸"},
		{name: "over pixel limit", mediaType: "image/png", data: pngConfigOnly(8192, 8193), wantError: "尺寸"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := doc.ReplaceSlidePicture(0, 7, test.mediaType, test.data); err == nil {
				t.Fatal("invalid image data was accepted")
			} else if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("replacement error = %q, want it to identify %q", err, test.wantError)
			}
			after := doc.Snapshot()
			if after.Revision != before.Revision || after.Dirty != before.Dirty || !bytes.Equal(presentationElementByID(t, after.Slides[0], 7).ImageBytes, original) {
				t.Fatal("rejected image data changed committed picture state")
			}
		})
	}
}

func TestPPTXLayoutPictureUsesLayoutRelationship(t *testing.T) {
	red := encodeTestRaster(t, "png", color.NRGBA{R: 245, A: 255})
	blue := encodeTestRaster(t, "png", color.NRGBA{B: 245, A: 255})
	picture := inheritedPictureXML(35, "layout picture", "rId9", 0, 0, 10, 10)
	parts := inheritedFixtureParts(
		inheritedSlideXML("", ""),
		inheritedLayoutXML("", picture),
		inheritedMasterXML(""),
		map[string][]byte{
			"ppt/slideLayouts/_rels/slideLayout1.xml.rels": []byte(`<Relationships xmlns="` + testPackageRelNS + `"><Relationship Id="rId1" Type="` + testOfficeRelNS + `/slideMaster" Target="../slideMasters/slideMaster1.xml"/><Relationship Id="rId9" Type="` + testOfficeRelNS + `/image" Target="../assets/layout-picture.png"/></Relationships>`),
			"ppt/assets/layout-picture.png":                red,
			"ppt/media/unrelated.png":                      blue,
		},
	)
	slide := openFirstPPTXSlide(t, parts)
	if len(slide.InheritedElements) != 1 || slide.InheritedElements[0].Kind != document.ElementKindPicture || slide.InheritedElements[0].Image == nil {
		t.Fatalf("layout picture projection = %+v, want one decoded picture", slide.InheritedElements)
	}
	img := slide.InheritedElements[0].Image
	got := color.NRGBAModel.Convert(img.At(img.Bounds().Min.X, img.Bounds().Min.Y)).(color.NRGBA)
	if got.R < 220 || got.B > 20 {
		t.Fatalf("layout picture resolved wrong image: got %#v, want red layout-picture.png", got)
	}
}

func TestPPTXInheritedEMFPictureIsDecoded(t *testing.T) {
	picture := inheritedPictureXML(35, "layout EMF", "rId9", 0, 0, 10, 10)
	parts := inheritedFixtureParts(
		inheritedSlideXML("", ""),
		inheritedLayoutXML("", picture),
		inheritedMasterXML(""),
		map[string][]byte{
			"ppt/slideLayouts/_rels/slideLayout1.xml.rels": []byte(`<Relationships xmlns="` + testPackageRelNS + `"><Relationship Id="rId1" Type="` + testOfficeRelNS + `/slideMaster" Target="../slideMasters/slideMaster1.xml"/><Relationship Id="rId9" Type="` + testOfficeRelNS + `/image" Target="../assets/layout-picture.emf"/></Relationships>`),
			"ppt/assets/layout-picture.emf":                testEMFRectangle(),
		},
	)
	slide := openFirstPPTXSlide(t, parts)
	if len(slide.InheritedElements) != 1 || slide.InheritedElements[0].Kind != document.ElementKindPicture {
		t.Fatalf("inherited EMF picture = %+v, want one decoded picture", slide.InheritedElements)
	}
	img := slide.InheritedElements[0].Image
	if img == nil {
		t.Fatal("decoded EMF picture has no image")
	}
	if img.Bounds().Dx() != 11 || img.Bounds().Dy() != 11 {
		t.Fatalf("decoded EMF image bounds = %v, want 11x11", img.Bounds())
	}
	got := color.NRGBAModel.Convert(img.At(5, 5)).(color.NRGBA)
	if got.R < 200 || got.G > 30 || got.B > 30 || got.A < 200 {
		t.Fatalf("decoded EMF center pixel = %#v, want opaque red", got)
	}
}

func TestEMFValidationRejectsOversizedPointCounts(t *testing.T) {
	badPolyline := testEMFRecord(emfPolylineTo16, testEMFWords(0, 0, 0, 0, ^uint32(0)))
	data := testEMFFile(badPolyline, testEMFRecord(emfEndRecord, testEMFWords(0, 0, 20)))
	if _, err := decodeEMFImage(data); err == nil {
		t.Fatal("EMF with a point count larger than its record was accepted")
	}
}

func TestEMFValidationRejectsOversizedBounds(t *testing.T) {
	data := testEMFRectangle()
	data[16], data[17], data[18], data[19] = 0xff, 0xff, 0xff, 0x7f
	if _, err := decodeEMFImage(data); err == nil {
		t.Fatal("EMF with oversized image bounds was accepted")
	}
}
