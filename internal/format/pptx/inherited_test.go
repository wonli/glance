package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"glance/internal/document"
)

const (
	testOfficeRelNS  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	testPackageRelNS = "http://schemas.openxmlformats.org/package/2006/relationships"
)

type testPictureRef struct {
	id  int
	rid string
}

type testImageRel struct {
	id       string
	target   string
	external bool
}

func TestResolveOPCTargetNormalizesOwnerRelativeAndRootRelativePaths(t *testing.T) {
	rels := parsePresentationRels([]byte(`<Relationships xmlns="` + testPackageRelNS + `">` +
		`<Relationship Id="rId1" Target="../ppt/slides/slide17.xml"/>` +
		`<Relationship Id="rId2" Target="/ppt/slides/slide2.xml"/>` +
		`<Relationship Id="rId3" Target="../../outside/slide3.xml"/>` +
		`</Relationships>`))
	presentation := []byte(`<p:presentation xmlns:r="` + testOfficeRelNS + `"><p:sldIdLst>` +
		`<p:sldId id="1" r:id="rId1"/><p:sldId id="2" r:id="rId2"/><p:sldId id="3" r:id="rId3"/>` +
		`</p:sldIdLst></p:presentation>`)

	got := parsePresentationSlideFiles(presentation, rels)
	want := []string{"ppt/slides/slide17.xml", "ppt/slides/slide2.xml"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("normalized slide targets = %v, want %v", got, want)
	}
}

func TestOPCPackageRejectsExternalImageRelationships(t *testing.T) {
	parts := pictureFixtureParts(
		[]testPictureRef{{id: 2, rid: "rId2"}},
		[]testImageRel{{id: "rId2", target: "../media/unrelated.png", external: true}},
		map[string][]byte{"ppt/media/unrelated.png": encodeTestRaster(t, "png", color.NRGBA{B: 255, A: 255})},
	)
	if img := firstPictureImage(t, parts); img != nil {
		t.Fatal("external relationship unexpectedly borrowed a local media image")
	}
}

func TestPPTXBackgroundReferenceUsesThemeFillStyle(t *testing.T) {
	red := encodeTestRaster(t, "png", color.NRGBA{R: 255, A: 255})
	theme := func(fillStyles, bgFillStyles string) []byte {
		return []byte(`<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `"><a:themeElements><a:clrScheme name="test"><a:dk1><a:srgbClr val="111111"/></a:dk1><a:lt1><a:srgbClr val="FFFFFF"/></a:lt1><a:accent1><a:srgbClr val="445566"/></a:accent1></a:clrScheme><a:fmtScheme name="test"><a:fillStyleLst>` + fillStyles + `</a:fillStyleLst><a:lnStyleLst/><a:effectStyleLst/><a:bgFillStyleLst>` + bgFillStyles + `</a:bgFillStyleLst></a:fmtScheme></a:themeElements></a:theme>`)
	}
	partsFor := func(idx, referenceColor, fillStyles, bgFillStyles string, imageRel bool) map[string][]byte {
		parts := backgroundFixtureParts(`<p:bg><p:bgRef idx="`+idx+`"><a:schemeClr val="`+referenceColor+`"/></p:bgRef></p:bg>`, "", "", nil, nil)
		parts["ppt/theme/theme1.xml"] = theme(fillStyles, bgFillStyles)
		if imageRel {
			parts["ppt/theme/_rels/theme1.xml.rels"] = []byte(`<Relationships xmlns="` + testPackageRelNS + `"><Relationship Id="rIdImage" Type="` + testOfficeRelNS + `/image" Target="../assets/theme-bg.png"/></Relationships>`)
			parts["ppt/assets/theme-bg.png"] = red
		}
		return parts
	}

	t.Run("solid fill style wins over reference color", func(t *testing.T) {
		parts := partsFor("1", "accent1", `<a:solidFill><a:srgbClr val="123456"/></a:solidFill>`, "", false)
		if got := openFirstPPTXSlide(t, parts).BgColorHex; got != "#123456" {
			t.Fatalf("background = %q, want selected fill style #123456", got)
		}
	})
	t.Run("placeholder color is substituted", func(t *testing.T) {
		parts := partsFor("1", "accent1", `<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>`, "", false)
		if got := openFirstPPTXSlide(t, parts).BgColorHex; got != "#445566" {
			t.Fatalf("background = %q, want reference placeholder color #445566", got)
		}
	})
	t.Run("background image style uses theme relationship", func(t *testing.T) {
		parts := partsFor("1001", "accent1", "", `<a:blipFill><a:blip r:embed="rIdImage"/><a:stretch><a:fillRect/></a:stretch></a:blipFill>`, true)
		got := openFirstPPTXSlide(t, parts)
		if got.BackgroundImage == nil {
			t.Fatal("theme background image was not projected")
		}
		pixel := color.NRGBAModel.Convert(got.BackgroundImage.At(0, 0)).(color.NRGBA)
		if pixel.R < 220 || pixel.B > 20 {
			t.Fatalf("theme image pixel = %#v, want red image resolved from theme relationship", pixel)
		}
	})
	t.Run("zero index is no fill", func(t *testing.T) {
		parts := partsFor("0", "accent1", `<a:solidFill><a:srgbClr val="123456"/></a:solidFill>`, "", false)
		if got := openFirstPPTXSlide(t, parts).BgColorHex; got != "#FFFFFF" {
			t.Fatalf("background = %q, want default white for no-fill style", got)
		}
	})
}

func TestPPTXInheritedShapeProjectsCustomPathAndLinearGradient(t *testing.T) {
	custom := `<p:sp><p:nvSpPr><p:cNvPr id="51" name="master custom path"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr>` + inheritedTransformXML(10, 20, 80, 60) +
		`<a:custGeom><a:avLst/><a:gdLst/><a:ahLst/><a:cxnLst/><a:rect l="l" t="t" r="r" b="b"/><a:pathLst><a:path w="100" h="100" fill="norm" stroke="0"><a:moveTo><a:pt x="0" y="0"/></a:moveTo><a:lnTo><a:pt x="100" y="0"/></a:lnTo><a:cubicBezTo><a:pt x="100" y="25"/><a:pt x="75" y="100"/><a:pt x="0" y="100"/></a:cubicBezTo><a:close/></a:path></a:pathLst></a:custGeom>` +
		`<a:gradFill><a:gsLst><a:gs pos="0"><a:srgbClr val="FF0000"><a:alpha val="50000"/></a:srgbClr></a:gs><a:gs pos="100000"><a:schemeClr val="accent1"/></a:gs></a:gsLst><a:lin ang="5400000" scaled="1"/></a:gradFill></p:spPr></p:sp>`
	slide := openFirstPPTXSlide(t, inheritedFixtureParts(inheritedSlideXML("", ""), inheritedLayoutXML("", ""), inheritedMasterXML(custom), nil))
	if len(slide.InheritedElements) != 1 {
		t.Fatalf("inherited elements = %+v, want custom path shape", slide.InheritedElements)
	}
	elem := slide.InheritedElements[0]
	if len(elem.Paths) != 1 {
		t.Fatalf("custom paths = %+v, want one parsed DrawingML path", elem.Paths)
	}
	path := elem.Paths[0]
	if path.Width != 100 || path.Height != 100 || path.Fill != "norm" || path.Stroke != "0" {
		t.Errorf("custom path metadata = %+v", path)
	}
	wantCommands := []string{"moveTo", "lnTo", "cubicBezTo", "close"}
	if len(path.Commands) != len(wantCommands) {
		t.Fatalf("custom path commands = %+v", path.Commands)
	}
	for index, want := range wantCommands {
		if path.Commands[index].Kind != want {
			t.Errorf("path command %d = %q, want %q", index, path.Commands[index].Kind, want)
		}
	}
	if elem.FillGradient == nil {
		t.Fatal("linear fill gradient was not projected")
	}
	if elem.FillGradient.AngleDeg != 90 || len(elem.FillGradient.Stops) != 2 {
		t.Fatalf("gradient = %+v, want 90 degree two-stop gradient", elem.FillGradient)
	}
	if stop := elem.FillGradient.Stops[0]; stop.Position != 0 || stop.ColorHex != "#FF0000" || stop.Alpha != 0.5 {
		t.Errorf("first gradient stop = %+v, want translucent red at 0", stop)
	}
	if stop := elem.FillGradient.Stops[1]; stop.Position != 100000 || stop.ColorHex != "#4472C4" {
		t.Errorf("second gradient stop = %+v, want mapped accent at 100000", stop)
	}
}

func TestPPTXInheritedLinearGradientDefaultsMissingAngleToZero(t *testing.T) {
	shape := `<p:sp><p:nvSpPr><p:cNvPr id="52" name="master default-angle gradient"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr>` + inheritedTransformXML(10, 20, 80, 60) +
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:gradFill><a:gsLst><a:gs pos="0"><a:srgbClr val="FF0000"/></a:gs><a:gs pos="100000"><a:srgbClr val="0000FF"/></a:gs></a:gsLst><a:lin/></a:gradFill></p:spPr></p:sp>`
	slide := openFirstPPTXSlide(t, inheritedFixtureParts(inheritedSlideXML("", ""), inheritedLayoutXML("", ""), inheritedMasterXML(shape), nil))
	if len(slide.InheritedElements) != 1 || slide.InheritedElements[0].FillGradient == nil {
		t.Fatalf("inherited gradient = %+v, want gradient with default angle", slide.InheritedElements)
	}
	if got := slide.InheritedElements[0].FillGradient.AngleDeg; got != 0 {
		t.Fatalf("gradient angle = %v, want omitted angle default 0", got)
	}
}

func TestPPTXInheritedUnsupportedCustomPathIsOmitted(t *testing.T) {
	shape := `<p:sp><p:nvSpPr><p:cNvPr id="53" name="unsupported inherited path"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr>` + inheritedTransformXML(10, 20, 80, 60) +
		`<a:custGeom><a:pathLst><a:path w="100" h="100"><a:moveTo><a:pt x="0" y="0"/></a:moveTo><a:arcTo wR="50" hR="50" stAng="0" swAng="5400000"/></a:path></a:pathLst></a:custGeom><a:solidFill><a:srgbClr val="0000FF"/></a:solidFill></p:spPr></p:sp>`
	slide := openFirstPPTXSlide(t, inheritedFixtureParts(inheritedSlideXML("", ""), inheritedLayoutXML("", ""), inheritedMasterXML(shape), nil))
	if len(slide.InheritedElements) != 0 {
		t.Fatalf("inherited elements = %+v, want unsupported custom path omitted", slide.InheritedElements)
	}
}

func TestPPTXPackagePreservesPresentationSlideRelationshipOrder(t *testing.T) {
	rels := parsePresentationRels([]byte(`<Relationships xmlns="` + testPackageRelNS + `">` +
		`<Relationship Id="rIdZ" Target="slides/slide17.xml"/>` +
		`<Relationship Id="rIdA" Target="slides/slide2.xml"/>` +
		`</Relationships>`))
	presentation := []byte(`<p:presentation xmlns:r="` + testOfficeRelNS + `"><p:sldIdLst>` +
		`<p:sldId id="1" r:id="rIdZ"/><p:sldId id="2" r:id="rIdA"/>` +
		`</p:sldIdLst></p:presentation>`)

	got := parsePresentationSlideFiles(presentation, rels)
	want := []string{"ppt/slides/slide17.xml", "ppt/slides/slide2.xml"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("slide parts = %v, want presentation order %v", got, want)
	}
}

func TestPPTXMissingOrMalformedRelationshipsAreSafe(t *testing.T) {
	for name, data := range map[string][]byte{
		"missing":   nil,
		"malformed": []byte(`<Relationships><Relationship Id="rId1" Target="../media/image.png"`),
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("relationship parsing panicked: %v", recovered)
				}
			}()
			rels := parsePresentationRels(data)
			if got := parsePresentationSlideFiles([]byte(`<p:presentation><p:sldIdLst><p:sldId id="1" r:id="rId1"/></p:sldIdLst></p:presentation>`), rels); len(got) != 0 {
				t.Fatalf("malformed relationship resolved slide parts %v", got)
			}
		})
	}
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

func TestPPTXBackgroundUsesSlideThenLayoutThenMasterFallback(t *testing.T) {
	red := encodeTestRaster(t, "png", color.NRGBA{R: 255, A: 255})
	parts := backgroundFixtureParts(
		backgroundImageFill("rId2"),
		backgroundSolidFill("00FF00"),
		backgroundSolidFill("0000FF"),
		[]testImageRel{{id: "rId2", target: "../assets/slide-background.png"}},
		map[string][]byte{"ppt/assets/slide-background.png": red},
	)
	slide := openFirstPPTXSlide(t, parts)
	if img := slide.BackgroundImage; img == nil {
		t.Fatal("explicit slide background image was not projected")
	}

	parts = backgroundFixtureParts("", backgroundSolidFill("00FF00"), backgroundSolidFill("0000FF"), nil, nil)
	slide = openFirstPPTXSlide(t, parts)
	if slide.BgColorHex != "#00FF00" {
		t.Fatalf("layout fallback background = %q, want #00FF00", slide.BgColorHex)
	}

	parts = backgroundFixtureParts("", "", backgroundSolidFill("0000FF"), nil, nil)
	slide = openFirstPPTXSlide(t, parts)
	if slide.BgColorHex != "#0000FF" {
		t.Fatalf("master fallback background = %q, want #0000FF", slide.BgColorHex)
	}

	parts = backgroundFixtureParts(backgroundSolidFill("112233"), backgroundSolidFill("00FF00"), backgroundSolidFill("0000FF"), nil, nil)
	slide = openFirstPPTXSlide(t, parts)
	if slide.BgColorHex != "#112233" {
		t.Fatalf("explicit slide background = %q, want #112233", slide.BgColorHex)
	}
}

func TestPPTXBackgroundWithUnsupportedMediaHasNoImage(t *testing.T) {
	parts := backgroundFixtureParts(
		backgroundImageFill("rId2"), "", "",
		[]testImageRel{{id: "rId2", target: "../media/broken.png"}},
		map[string][]byte{
			"ppt/media/broken.png":    []byte("not a PNG"),
			"ppt/media/unrelated.png": encodeTestRaster(t, "png", color.NRGBA{G: 255, A: 255}),
		},
	)
	if img := openFirstPPTXSlide(t, parts).BackgroundImage; img != nil {
		t.Fatal("unsupported background media borrowed an unrelated cached image")
	}
}

func TestPPTXBackgroundResolvesThemeTx1AndBg1ThroughMasterClrMap(t *testing.T) {
	parts := backgroundFixtureParts(
		backgroundSolidFill("scheme", "tx1"),
		backgroundReferenceFill("bg1"),
		"",
		nil,
		nil,
	)
	master := string(parts["ppt/slideMasters/slideMaster1.xml"])
	defaultColorMap := `<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>`
	customColorMap := `<p:clrMap bg1="dk1" tx1="accent1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>`
	if !strings.Contains(master, defaultColorMap) {
		t.Fatal("master fixture did not contain the default color map")
	}
	parts["ppt/slideMasters/slideMaster1.xml"] = []byte(strings.Replace(master, defaultColorMap, customColorMap, 1))

	slide := openFirstPPTXSlide(t, parts)
	if slide.BgColorHex != "#4472C4" {
		t.Fatalf("tx1 theme mapping = %q, want #4472C4 through master clrMap", slide.BgColorHex)
	}

	parts = backgroundFixtureParts(backgroundReferenceFill("bg1"), "", "", nil, nil)
	master = string(parts["ppt/slideMasters/slideMaster1.xml"])
	parts["ppt/slideMasters/slideMaster1.xml"] = []byte(strings.Replace(master, defaultColorMap, customColorMap, 1))
	slide = openFirstPPTXSlide(t, parts)
	if slide.BgColorHex != "#000000" {
		t.Fatalf("bg1 background reference mapping = %q, want #000000 through master clrMap", slide.BgColorHex)
	}
}

func TestPPTXBackgroundUsesExactOwningPartRelationship(t *testing.T) {
	red := encodeTestRaster(t, "png", color.NRGBA{R: 245, A: 255})
	blue := encodeTestRaster(t, "png", color.NRGBA{B: 245, A: 255})
	parts := backgroundFixtureParts(
		backgroundImageFill("rId2"), "", "",
		[]testImageRel{
			{id: "rId2", target: "../assets/background.png"},
			{id: "rId3", target: "../media/unrelated.png"},
		},
		map[string][]byte{
			"ppt/assets/background.png": red,
			"ppt/media/unrelated.png":   blue,
		},
	)
	img := openFirstPPTXSlide(t, parts).BackgroundImage
	if img == nil {
		t.Fatal("background image was not resolved through its owning slide relationship")
	}
	got := color.NRGBAModel.Convert(img.At(img.Bounds().Min.X, img.Bounds().Min.Y)).(color.NRGBA)
	if got.R < 220 || got.B > 20 {
		t.Fatalf("background resolved wrong media: got %#v, want red background.png", got)
	}
}

func TestPPTXInheritedVisualsRenderMasterThenLayout(t *testing.T) {
	parts := inheritedFixtureParts(
		inheritedSlideXML("", inheritedShapeXML(20, "slide local", 3, 4, 5, 6, "00FF00", "")),
		inheritedLayoutXML("", inheritedShapeXML(30, "layout decoration", 10, 10, 20, 20, "0000FF", "")),
		inheritedMasterXML(inheritedShapeXML(40, "master decoration", 0, 0, 30, 30, "FF0000", "")),
		nil,
	)
	slide := openFirstPPTXSlide(t, parts)
	if len(slide.InheritedElements) != 2 {
		t.Fatalf("inherited elements = %+v, want master and layout decorations", slide.InheritedElements)
	}
	if got := []string{slide.InheritedElements[0].Name, slide.InheritedElements[1].Name}; fmt.Sprint(got) != "[master decoration layout decoration]" {
		t.Fatalf("inherited order = %v, want master then layout", got)
	}
	if len(slide.Elements) != 1 || slide.Elements[0].Name != "slide local" {
		t.Fatalf("slide-local elements = %+v, want existing local shape retained", slide.Elements)
	}
}

func TestAddSlideKeepsDefaultLayoutBackgroundAndInheritedArtwork(t *testing.T) {
	parts := inheritedFixtureParts(
		inheritedSlideXML("", ""),
		inheritedLayoutXML("", inheritedShapeXML(30, "default layout decoration", 10, 10, 20, 20, "0000FF", "")),
		inheritedMasterXML(inheritedShapeXML(40, "default master decoration", 0, 0, 30, 30, "FF0000", "")),
		nil,
	)
	layout := string(parts["ppt/slideLayouts/slideLayout1.xml"])
	layout = strings.Replace(layout, `<p:cSld name="Blank">`, `<p:cSld name="Blank">`+backgroundSolidFill("123456"), 1)
	parts["ppt/slideLayouts/slideLayout1.xml"] = []byte(layout)
	parts["ppt/slideMasters/slideMaster1.xml"] = inheritedMasterXML(inheritedShapeXML(40, "default master decoration", 0, 0, 30, 30, "FF0000", ""))
	doc, err := Open(writePPTXFixture(t, parts))
	if err != nil {
		t.Fatalf("Open fixture: %v", err)
	}
	defer doc.Close()
	if err := doc.AddSlide(); err != nil {
		t.Fatalf("AddSlide: %v", err)
	}
	snapshot := doc.Snapshot()
	if len(snapshot.Slides) != 2 {
		t.Fatalf("slides = %d, want 2", len(snapshot.Slides))
	}
	added := snapshot.Slides[1]
	if added.BgColorHex != "#123456" {
		t.Errorf("new slide background = %q, want default layout background #123456", added.BgColorHex)
	}
	if len(added.InheritedElements) != 2 || added.InheritedElements[0].Name != "default master decoration" || added.InheritedElements[1].Name != "default layout decoration" {
		t.Errorf("new slide inherited artwork = %+v, want default master then layout", added.InheritedElements)
	}
}

func TestAddSlideReusesDecodedDefaultLayoutBackgroundImage(t *testing.T) {
	red := encodeTestRaster(t, "png", color.NRGBA{R: 245, A: 255})
	parts := inheritedFixtureParts(
		inheritedSlideXML("", ""),
		inheritedLayoutXML("", ""),
		inheritedMasterXML(""),
		map[string][]byte{
			"ppt/slideLayouts/_rels/slideLayout1.xml.rels": []byte(`<Relationships xmlns="` + testPackageRelNS + `"><Relationship Id="rId1" Type="` + testOfficeRelNS + `/slideMaster" Target="../slideMasters/slideMaster1.xml"/><Relationship Id="rId9" Type="` + testOfficeRelNS + `/image" Target="../media/layout-bg.png"/></Relationships>`),
			"ppt/media/layout-bg.png":                      red,
		},
	)
	layout := string(parts["ppt/slideLayouts/slideLayout1.xml"])
	layout = strings.Replace(layout, `<p:cSld name="Blank">`, `<p:cSld name="Blank">`+backgroundImageFill("rId9"), 1)
	parts["ppt/slideLayouts/slideLayout1.xml"] = []byte(layout)
	doc, err := Open(writePPTXFixture(t, parts))
	if err != nil {
		t.Fatalf("Open fixture: %v", err)
	}
	defer doc.Close()
	if err := doc.AddSlide(); err != nil {
		t.Fatalf("AddSlide: %v", err)
	}
	snapshot := doc.Snapshot()
	if len(snapshot.Slides) != 2 || snapshot.Slides[0].BackgroundImage == nil || snapshot.Slides[1].BackgroundImage == nil {
		t.Fatalf("background images on existing/new slides = %+v, want both present", snapshot.Slides)
	}
	if snapshot.Slides[0].BackgroundImage != snapshot.Slides[1].BackgroundImage {
		t.Fatal("existing and added slides do not share the decoded immutable layout background image")
	}
}

func TestInheritedVisualsRespectShowMasterSpFlags(t *testing.T) {
	for _, test := range []struct {
		name       string
		slideFlag  string
		layoutFlag string
	}{
		{name: "slide hides master", slideFlag: ` showMasterSp="0"`},
		{name: "layout hides master", layoutFlag: ` showMasterSp="0"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			parts := inheritedFixtureParts(
				inheritedSlideXML(test.slideFlag, ""),
				inheritedLayoutXML(test.layoutFlag, inheritedShapeXML(30, "layout decoration", 0, 0, 10, 10, "0000FF", "")),
				inheritedMasterXML(inheritedShapeXML(40, "master decoration", 0, 0, 10, 10, "FF0000", "")),
				nil,
			)
			slide := openFirstPPTXSlide(t, parts)
			if len(slide.InheritedElements) != 1 || slide.InheritedElements[0].Name != "layout decoration" {
				t.Fatalf("showMasterSp projection = %+v, want layout decoration only", slide.InheritedElements)
			}
		})
	}
}

func TestPlaceholderGeometryUsesLocalThenUniqueTypeAndIndexFallback(t *testing.T) {
	slideXML := inheritedSlideXML("", strings.Join([]string{
		inheritedPlaceholderXML(10, "title", "ctrTitle", "", ""),
		inheritedPlaceholderXML(11, "ambiguous", "body", "1", ""),
		inheritedPlaceholderXML(12, "local", "body", "2", inheritedTransformXML(50, 60, 70, 80)),
		inheritedPlaceholderXML(13, "default object", "", "", ""),
		inheritedPlaceholderXML(14, "master fallback", "sldNum", "9", ""),
	}, ""))
	layoutXML := inheritedLayoutXML("", strings.Join([]string{
		inheritedPlaceholderXML(31, "title layout", "ctrTitle", "", inheritedTransformXML(100, 110, 120, 130)),
		inheritedPlaceholderXML(32, "body layout one", "body", "1", inheritedTransformXML(200, 210, 220, 230)),
		inheritedPlaceholderXML(33, "body layout two", "body", "1", inheritedTransformXML(300, 310, 320, 330)),
		inheritedPlaceholderXML(34, "local layout", "body", "2", inheritedTransformXML(400, 410, 420, 430)),
		inheritedPlaceholderXML(35, "default layout", "obj", "0", inheritedTransformXML(700, 710, 720, 730)),
	}, ""))
	masterXML := inheritedMasterXML(strings.Join([]string{
		inheritedPlaceholderXML(41, "title master", "ctrTitle", "", inheritedTransformXML(500, 510, 520, 530)),
		inheritedPlaceholderXML(42, "ambiguous master", "body", "1", inheritedTransformXML(600, 610, 620, 630)),
		inheritedPlaceholderXML(43, "number master", "sldNum", "9", inheritedTransformXML(800, 810, 820, 830)),
	}, ""))
	slide := openFirstPPTXSlide(t, inheritedFixtureParts(slideXML, layoutXML, masterXML, nil))
	byName := make(map[string]document.PresentationElement)
	for _, elem := range slide.Elements {
		byName[elem.Name] = elem
	}
	wantTitle := document.PresentationRect{X: 100, Y: 110, W: 120, H: 130}
	if got := byName["title"].Bounds; got != wantTitle {
		t.Errorf("unique layout placeholder bounds = %+v, want %+v", got, wantTitle)
	}
	if got := byName["ambiguous"].Bounds; got != (document.PresentationRect{}) {
		t.Errorf("ambiguous placeholder bounds = %+v, want zero bounds", got)
	}
	wantLocal := document.PresentationRect{X: 50, Y: 60, W: 70, H: 80}
	if got := byName["local"].Bounds; got != wantLocal {
		t.Errorf("local placeholder bounds = %+v, want %+v", got, wantLocal)
	}
	wantDefault := document.PresentationRect{X: 700, Y: 710, W: 720, H: 730}
	if got := byName["default object"].Bounds; got != wantDefault {
		t.Errorf("default type/index placeholder bounds = %+v, want %+v", got, wantDefault)
	}
	wantMaster := document.PresentationRect{X: 800, Y: 810, W: 820, H: 830}
	if got := byName["master fallback"].Bounds; got != wantMaster {
		t.Errorf("master fallback placeholder bounds = %+v, want %+v", got, wantMaster)
	}

}

func TestEmptySlidePlaceholderProjectsAsTypedEditableText(t *testing.T) {
	slideXML := inheritedSlideXML("", inheritedEmptyPlaceholderXML(10, "title", "ctrTitle", ""))
	layoutXML := inheritedLayoutXML("", inheritedPlaceholderXML(31, "title layout", "ctrTitle", "", inheritedTransformXML(100, 110, 500, 80)))
	parts := inheritedFixtureParts(slideXML, layoutXML, inheritedMasterXML(""), nil)
	source := writePPTXFixture(t, parts)
	sourceBytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read fixture before edit: %v", err)
	}
	doc, err := Open(source)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	slide := doc.Snapshot().Slides[0]
	if len(slide.Elements) != 1 {
		t.Fatalf("slide elements = %d, want one empty title placeholder", len(slide.Elements))
	}
	elem := slide.Elements[0]
	if elem.Kind != document.ElementKindText || elem.PlaceholderType != "ctrTitle" || elem.PlainText() != "" {
		t.Fatalf("empty title placeholder projection = %+v, want editable empty ctrTitle text", elem)
	}
	wantBounds := document.PresentationRect{X: 100, Y: 110, W: 500, H: 80}
	if elem.Bounds != wantBounds {
		t.Fatalf("empty title placeholder bounds = %+v, want inherited bounds %+v", elem.Bounds, wantBounds)
	}
	if err := doc.UpdateSlideText(0, elem.ID, "新标题"); err != nil {
		t.Fatalf("type into empty title placeholder: %v", err)
	}
	output := filepath.Join(t.TempDir(), "placeholder-edited.pptx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatalf("save placeholder edit: %v", err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatalf("reopen edited placeholder: %v", err)
	}
	defer reopened.Close()
	if got := reopened.Snapshot().Slides[0].Elements[0].PlainText(); got != "新标题" {
		t.Fatalf("reopened placeholder text = %q, want %q", got, "新标题")
	}
	if after, err := os.ReadFile(source); err != nil || !bytes.Equal(sourceBytes, after) {
		t.Fatalf("editing placeholder modified source package: err=%v", err)
	}
}

func TestProjectionPreservesParagraphSpacing(t *testing.T) {
	shape := inheritedShapeXML(61, "spaced text", 0, 0, 400, 120, "FFFFFF", "line spacing")
	shape = strings.Replace(shape, `<a:p><a:r>`, `<a:p><a:pPr><a:lnSpc><a:spcPct val="150000"/></a:lnSpc><a:spcBef><a:spcPts val="600"/></a:spcBef><a:spcAft><a:spcPct val="50000"/></a:spcAft></a:pPr><a:r>`, 1)
	source := writePPTXFixture(t, inheritedFixtureParts(inheritedSlideXML("", shape), inheritedLayoutXML("", ""), inheritedMasterXML(""), nil))
	doc, err := Open(source)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer doc.Close()

	slide := doc.Snapshot().Slides[0]
	if len(slide.Elements) != 1 || len(slide.Elements[0].Paragraphs) != 1 {
		t.Fatalf("projected spaced paragraphs = %+v", slide.Elements)
	}
	paragraph := slide.Elements[0].Paragraphs[0]
	if paragraph.LineSpacing == nil || paragraph.LineSpacing.Kind != "pct" || paragraph.LineSpacing.Value != 1.5 {
		t.Errorf("line spacing = %+v, want 150%%", paragraph.LineSpacing)
	}
	if paragraph.SpaceBefore == nil || paragraph.SpaceBefore.Kind != "pts" || paragraph.SpaceBefore.Value != 6 {
		t.Errorf("space before = %+v, want 6pt", paragraph.SpaceBefore)
	}
	if paragraph.SpaceAfter == nil || paragraph.SpaceAfter.Kind != "pct" || paragraph.SpaceAfter.Value != 0.5 {
		t.Errorf("space after = %+v, want 50%%", paragraph.SpaceAfter)
	}
}

func TestUpdateSlideElementStyleProjectsAndNoops(t *testing.T) {
	doc, _ := openTransformTestDocument(t, transformTestShapeXML(7, `<a:xfrm><a:off x="127000" y="254000"/><a:ext cx="1016000" cy="635000"/></a:xfrm>`))
	defer doc.Close()
	fontFamily, textColor := "Aptos", "#AABBCC"
	fontSize, borderWidth := 22.0, 1.25
	bold, italic, underline := true, false, true
	align, fillColor, borderColor := "center", "#123456", "#654321"
	lineSpacing := document.PresentationSpacing{Kind: "pct", Value: 1.5}
	spaceBefore := document.PresentationSpacing{Kind: "pts", Value: 6}
	patch := document.PresentationElementStylePatch{
		FontFamily: &fontFamily, FontSizePT: &fontSize, Bold: &bold, Italic: &italic, Underline: &underline,
		TextColorHex: &textColor, ParagraphAlignment: &align, LineSpacing: &lineSpacing, SpaceBefore: &spaceBefore,
		FillColorHex: &fillColor, BorderColor: &borderColor, BorderWidthPT: &borderWidth,
	}
	before := doc.revision
	if err := doc.UpdateSlideElementStyle(0, 7, patch); err != nil {
		t.Fatalf("UpdateSlideElementStyle: %v", err)
	}
	if doc.revision != before+1 || !doc.dirty {
		t.Fatalf("style revision/dirty = %d/%t, want %d/true", doc.revision, doc.dirty, before+1)
	}
	elem := presentationElementByID(t, doc.Snapshot().Slides[0], 7)
	if len(elem.Paragraphs) != 1 || len(elem.Paragraphs[0].Runs) != 1 {
		t.Fatalf("styled element paragraphs = %+v", elem.Paragraphs)
	}
	paragraph, run := elem.Paragraphs[0], elem.Paragraphs[0].Runs[0]
	if run.Font != fontFamily || run.SizePT != fontSize || !run.Bold || run.Italic || !run.Underline || run.ColorHex != textColor {
		t.Errorf("styled run = %+v", run)
	}
	if paragraph.Align != align || paragraph.LineSpacing == nil || *paragraph.LineSpacing != lineSpacing || paragraph.SpaceBefore == nil || *paragraph.SpaceBefore != spaceBefore {
		t.Errorf("styled paragraph = %+v", paragraph)
	}
	if elem.FillColorHex != fillColor || elem.BorderColor != borderColor || elem.BorderWidth != borderWidth {
		t.Errorf("styled shape fill/border = %q/%q/%.2f", elem.FillColorHex, elem.BorderColor, elem.BorderWidth)
	}
	if err := doc.UpdateSlideElementStyle(0, 7, patch); err != nil {
		t.Fatalf("repeat no-op style update: %v", err)
	}
	if doc.revision != before+1 {
		t.Fatalf("identical style update advanced revision to %d", doc.revision)
	}
}

func TestUpdateSlideElementStyleNoopsAgainstOriginalStyle(t *testing.T) {
	doc, _ := openTransformTestDocument(t, transformTestShapeXML(7, `<a:xfrm><a:off x="127000" y="254000"/><a:ext cx="1016000" cy="635000"/></a:xfrm>`))
	defer doc.Close()
	elem := presentationElementByID(t, doc.Snapshot().Slides[0], 7)
	run := elem.Paragraphs[0].Runs[0]
	revision, dirty := doc.Revision(), doc.IsDirty()
	if err := doc.UpdateSlideElementStyle(0, 7, document.PresentationElementStylePatch{FontSizePT: &run.SizePT, Bold: &run.Bold}); err != nil {
		t.Fatal(err)
	}
	if doc.Revision() != revision || doc.IsDirty() != dirty {
		t.Fatal("selecting the object's original style advanced dirty/revision state")
	}
}

func TestUpdateSlideElementStyleRejectsInvalidPatch(t *testing.T) {
	doc, _ := openTransformTestDocument(t, transformTestShapeXML(7, `<a:xfrm><a:off x="127000" y="254000"/><a:ext cx="1016000" cy="635000"/></a:xfrm>`))
	defer doc.Close()
	badColor, tooLarge, badAlignment, badLineWidth := "not-a-color", 401.0, "distributed", -1.0
	tests := []struct {
		name  string
		patch document.PresentationElementStylePatch
	}{
		{name: "color", patch: document.PresentationElementStylePatch{TextColorHex: &badColor}},
		{name: "font size", patch: document.PresentationElementStylePatch{FontSizePT: &tooLarge}},
		{name: "alignment", patch: document.PresentationElementStylePatch{ParagraphAlignment: &badAlignment}},
		{name: "line width", patch: document.PresentationElementStylePatch{BorderWidthPT: &badLineWidth}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := doc.revision
			if err := doc.UpdateSlideElementStyle(0, 7, test.patch); err == nil {
				t.Fatal("invalid style patch unexpectedly succeeded")
			}
			if doc.revision != before || doc.dirty {
				t.Fatalf("invalid style changed revision/dirty state: %d/%t", doc.revision, doc.dirty)
			}
		})
	}
	bold := true
	if err := doc.UpdateSlideElementStyle(0, 99, document.PresentationElementStylePatch{Bold: &bold}); err == nil {
		t.Fatal("missing style target unexpectedly succeeded")
	}
}

func TestUpdateSlideElementStylePersistsAndComposesWithTransform(t *testing.T) {
	shape := transformTestShapeXML(7, `<a:xfrm><a:off x="127000" y="254000"/><a:ext cx="1016000" cy="635000"/></a:xfrm>`)
	shape = strings.Replace(shape, `<a:p><a:r>`, `<a:p><a:pPr algn="l"><a:lnSpc><a:spcPct val="100000"/></a:lnSpc><a:extLst><a:ext uri="keep-me"/></a:extLst></a:pPr><a:r>`, 1)
	shape = strings.Replace(shape, `<a:rPr lang="en-US" sz="1800"/>`, `<a:rPr lang="en-US" sz="1800" customAttr="keep"><a:latin typeface="Old Latin"/><a:extLst><a:ext uri="keep-run"/></a:extLst></a:rPr>`, 1)
	doc, source := openTransformTestDocument(t, shape)
	defer doc.Close()
	fontFamily, textColor, fillColor, borderColor := `Aptos "Display"`, "#AABBCC", "#123456", "#654321"
	fontSize, borderWidth := 22.0, 1.25
	bold, italic, underline := true, false, true
	align := "center"
	lineSpacing := document.PresentationSpacing{Kind: "pct", Value: 1.5}
	spaceBefore := document.PresentationSpacing{Kind: "pts", Value: 6}
	spaceAfter := document.PresentationSpacing{Kind: "pts", Value: 3}
	patch := document.PresentationElementStylePatch{
		FontFamily: &fontFamily, FontSizePT: &fontSize, Bold: &bold, Italic: &italic, Underline: &underline,
		TextColorHex: &textColor, ParagraphAlignment: &align, LineSpacing: &lineSpacing, SpaceBefore: &spaceBefore,
		SpaceAfter: &spaceAfter, FillColorHex: &fillColor, BorderColor: &borderColor, BorderWidthPT: &borderWidth,
	}
	if err := doc.UpdateSlideElementStyle(0, 7, patch); err != nil {
		t.Fatalf("UpdateSlideElementStyle: %v", err)
	}
	wantBounds := document.PresentationRect{X: 8, Y: 9, W: 90, H: 45}
	if err := doc.UpdateSlideElementTransform(0, 7, wantBounds, 17.5); err != nil {
		t.Fatalf("UpdateSlideElementTransform: %v", err)
	}
	output := filepath.Join(t.TempDir(), "styled.pptx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatalf("SaveAs style and transform: %v", err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatalf("Open saved style: %v", err)
	}
	defer reopened.Close()
	element := presentationElementByID(t, reopened.Snapshot().Slides[0], 7)
	if err := validateProjectedPresentationStyle(element, patch); err != nil {
		t.Fatalf("reopened style: %v, element %+v", err, element)
	}
	if !samePresentationRect(element.Bounds, wantBounds) || math.Abs(element.RotationDeg-17.5) > 1e-8 {
		t.Fatalf("reopened geometry = %+v rotation %.2f, want %+v rotation 17.5", element.Bounds, element.RotationDeg, wantBounds)
	}
	patchedSlide := string(readPPTXPartForTest(t, output, "ppt/slides/slide1.xml"))
	for _, preserved := range []string{`customAttr="keep"`, `uri="keep-run"`, `uri="keep-me"`, `algn="ctr"`, `sz="2200"`, `u="sng"`} {
		if !strings.Contains(patchedSlide, preserved) {
			t.Errorf("saved slide XML lost expected style or unknown content %q", preserved)
		}
	}
	sourceBytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read source fixture: %v", err)
	}
	outputBytes, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output fixture: %v", err)
	}
	if bytes.Equal(sourceBytes, outputBytes) {
		t.Fatal("SaveAs did not produce an edited PPTX")
	}
	if doc.IsDirty() {
		t.Fatal("successful SaveAs left the document dirty")
	}
	snapshotBytes, err := doc.SnapshotBytes()
	if err != nil {
		t.Fatalf("SnapshotBytes with style overlay: %v", err)
	}
	snapshot, err := OpenBytes(snapshotBytes)
	if err != nil {
		t.Fatalf("OpenBytes style snapshot: %v", err)
	}
	defer snapshot.Close()
	snapshotElement := presentationElementByID(t, snapshot.Snapshot().Slides[0], 7)
	if err := validateProjectedPresentationStyle(snapshotElement, patch); err != nil {
		t.Fatalf("recovered style snapshot: %v", err)
	}
}

func TestUpdateSlideElementStyleCanExplicitlyClearFillAndLine(t *testing.T) {
	doc, _ := openTransformTestDocument(t, transformTestShapeXML(7, `<a:xfrm><a:off x="127000" y="254000"/><a:ext cx="1016000" cy="635000"/></a:xfrm>`))
	defer doc.Close()
	none := ""
	if err := doc.UpdateSlideElementStyle(0, 7, document.PresentationElementStylePatch{FillColorHex: &none, BorderColor: &none}); err != nil {
		t.Fatalf("clear shape fill and line: %v", err)
	}
	output := filepath.Join(t.TempDir(), "no-fill.pptx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatalf("SaveAs explicit no fill/line: %v", err)
	}
	xml := string(readPPTXPartForTest(t, output, "ppt/slides/slide1.xml"))
	if strings.Contains(xml, `<a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill>`) || !strings.Contains(xml, `<a:noFill/>`) {
		t.Fatalf("saved XML did not represent explicit no fill: %s", xml)
	}
	if !strings.Contains(xml, `<a:ln><a:noFill/></a:ln>`) {
		t.Fatalf("saved XML did not represent explicit no line: %s", xml)
	}
}

func TestUpdateSlideElementStyleRejectsUnsupportedTextStructure(t *testing.T) {
	shape := transformTestShapeXML(7, `<a:xfrm><a:off x="127000" y="254000"/><a:ext cx="1016000" cy="635000"/></a:xfrm>`)
	shape = strings.Replace(shape, `</a:p></p:txBody>`, `</a:p><a:p><a:r><a:rPr/><a:t>second paragraph</a:t></a:r></a:p></p:txBody>`, 1)
	doc, _ := openTransformTestDocument(t, shape)
	defer doc.Close()
	align := "center"
	before := doc.Snapshot()
	if err := doc.UpdateSlideElementStyle(0, 7, document.PresentationElementStylePatch{ParagraphAlignment: &align}); err == nil {
		t.Fatal("multi-paragraph text style edit unexpectedly succeeded")
	}
	after := doc.Snapshot()
	if after.Dirty != before.Dirty || after.Revision != before.Revision {
		t.Fatalf("unsupported text style edit changed dirty/revision state: before=%t/%d after=%t/%d", before.Dirty, before.Revision, after.Dirty, after.Revision)
	}
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

func inheritedFixtureParts(slideXML, layoutXML, masterXML []byte, extras map[string][]byte) map[string][]byte {
	parts := map[string][]byte{
		"ppt/slides/slide1.xml":                        slideXML,
		"ppt/slides/_rels/slide1.xml.rels":             []byte(`<Relationships xmlns="` + testPackageRelNS + `"><Relationship Id="rId1" Type="` + testOfficeRelNS + `/slideLayout" Target="../slideLayouts/slideLayout1.xml"/></Relationships>`),
		"ppt/slideLayouts/slideLayout1.xml":            layoutXML,
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels": []byte(`<Relationships xmlns="` + testPackageRelNS + `"><Relationship Id="rId1" Type="` + testOfficeRelNS + `/slideMaster" Target="../slideMasters/slideMaster1.xml"/></Relationships>`),
		"ppt/slideMasters/slideMaster1.xml":            masterXML,
	}
	for name, data := range extras {
		parts[name] = data
	}
	return parts
}

func inheritedSlideXML(rootAttributes, shapes string) []byte {
	return []byte(`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"` + rootAttributes + `><p:cSld><p:spTree>` + inheritedTreePrefix() + shapes + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`)
}

func inheritedLayoutXML(rootAttributes, shapes string) []byte {
	return []byte(`<p:sldLayout xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"` + rootAttributes + `><p:cSld name="Blank"><p:spTree>` + inheritedTreePrefix() + shapes + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`)
}

func inheritedMasterXML(shapes string) []byte {
	return []byte(`<p:sldMaster xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree>` + inheritedTreePrefix() + shapes + `</p:spTree></p:cSld><p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/><p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst></p:sldMaster>`)
}

func inheritedTreePrefix() string {
	return `<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>`
}

func inheritedShapeXML(id int, name string, x, y, w, h int, fill, text string) string {
	textBody := ""
	if text != "" {
		textBody = `<p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:rPr lang="en-US" sz="1800"/><a:t>` + text + `</a:t></a:r></a:p></p:txBody>`
	}
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr>%s<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="%s"/></a:solidFill></p:spPr>%s</p:sp>`, id, name, inheritedTransformXML(x, y, w, h), fill, textBody)
}

func inheritedPlaceholderXML(id int, name, typ, idx, transform string) string {
	typeAttribute := ""
	if typ != "" {
		typeAttribute = ` type="` + typ + `"`
	}
	index := ""
	if idx != "" {
		index = ` idx="` + idx + `"`
	}
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr/><p:nvPr><p:ph%s%s/></p:nvPr></p:nvSpPr><p:spPr>%s<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:rPr lang="en-US"/><a:t>placeholder</a:t></a:r></a:p></p:txBody></p:sp>`, id, name, typeAttribute, index, transform)
}

func inheritedEmptyPlaceholderXML(id int, name, typ, transform string) string {
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr/><p:nvPr><p:ph type="%s"/></p:nvPr></p:nvSpPr><p:spPr>%s<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr/></a:p></p:txBody></p:sp>`, id, name, typ, transform)
}

func inheritedTransformXML(x, y, w, h int) string {
	return fmt.Sprintf(`<a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`, x*12700, y*12700, w*12700, h*12700)
}

func inheritedPictureXML(id int, name, relID string, x, y, w, h int) string {
	return fmt.Sprintf(`<p:pic><p:nvPicPr><p:cNvPr id="%d" name="%s"/><p:cNvPicPr/><p:nvPr/></p:nvPicPr><p:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></p:blipFill><p:spPr>%s<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`, id, name, relID, inheritedTransformXML(x, y, w, h))
}

func openFirstPPTXSlide(t *testing.T, parts map[string][]byte) document.PresentationSlide {
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
	return snapshot.Slides[0]
}

func backgroundFixtureParts(slideBG, layoutBG, masterBG string, imageRels []testImageRel, images map[string][]byte) map[string][]byte {
	parts := pictureFixtureParts(nil, imageRels, images)
	parts["ppt/slides/slide1.xml"] = emptySlideWithBackground(slideBG)
	parts["ppt/slideLayouts/slideLayout1.xml"] = emptyLayoutWithBackground(layoutBG)
	parts["ppt/slideMasters/slideMaster1.xml"] = emptyMasterWithBackground(masterBG)
	return parts
}

func emptySlideWithBackground(bg string) []byte {
	return []byte(`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld>` + bg +
		`<p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>` +
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`)
}

func emptyLayoutWithBackground(bg string) []byte {
	return []byte(`<p:sldLayout xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld name="Blank">` + bg +
		`<p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>` +
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`)
}

func emptyMasterWithBackground(bg string) []byte {
	return []byte(`<p:sldMaster xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld>` + bg +
		`<p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>` +
		`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
		`<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst></p:sldMaster>`)
}

func backgroundSolidFill(rgb string, scheme ...string) string {
	var colorElement string
	if rgb == "scheme" && len(scheme) > 0 {
		colorElement = `<a:schemeClr val="` + scheme[0] + `"/>`
	} else {
		colorElement = `<a:srgbClr val="` + rgb + `"/>`
	}
	return `<p:bg><p:bgPr><a:solidFill>` + colorElement + `</a:solidFill></p:bgPr></p:bg>`
}

func backgroundReferenceFill(scheme string) string {
	return `<p:bg><p:bgRef idx="1001"><a:schemeClr val="` + scheme + `"/></p:bgRef></p:bg>`
}

func backgroundImageFill(relID string) string {
	return `<p:bg><p:bgPr><a:blipFill><a:blip r:embed="` + relID + `"/><a:stretch><a:fillRect/></a:stretch></a:blipFill></p:bgPr></p:bg>`
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

func writePPTXFixture(t *testing.T, replacements map[string][]byte) string {
	t.Helper()
	seed, err := NewBlank()
	if err != nil {
		t.Fatalf("create seed presentation: %v", err)
	}
	seedPath := filepath.Join(t.TempDir(), "seed.pptx")
	if err := seed.SaveAs(seedPath); err != nil {
		_ = seed.Close()
		t.Fatalf("save seed presentation: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed presentation: %v", err)
	}

	seedFile, err := os.Open(seedPath)
	if err != nil {
		t.Fatalf("open seed package: %v", err)
	}
	info, err := seedFile.Stat()
	if err != nil {
		_ = seedFile.Close()
		t.Fatalf("stat seed package: %v", err)
	}
	reader, err := zip.NewReader(seedFile, info.Size())
	if err != nil {
		_ = seedFile.Close()
		t.Fatalf("read seed package: %v", err)
	}

	targetPath := filepath.Join(t.TempDir(), "fixture.pptx")
	targetFile, err := os.Create(targetPath)
	if err != nil {
		_ = seedFile.Close()
		t.Fatalf("create fixture package: %v", err)
	}
	writer := zip.NewWriter(targetFile)
	seen := make(map[string]bool, len(replacements))
	for _, part := range reader.File {
		rc, err := part.Open()
		if err != nil {
			t.Fatalf("open seed part %s: %v", part.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read seed part %s: %v", part.Name, err)
		}
		if replacement, ok := replacements[part.Name]; ok {
			data = replacement
			seen[part.Name] = true
		}
		if part.Name == "[Content_Types].xml" {
			data = addImageContentTypes(data)
		}
		header := &zip.FileHeader{Name: part.Name, Method: zip.Deflate}
		out, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("write fixture part %s: %v", part.Name, err)
		}
		if _, err := out.Write(data); err != nil {
			t.Fatalf("write fixture part %s bytes: %v", part.Name, err)
		}
	}
	for name, data := range replacements {
		if seen[name] {
			continue
		}
		out, err := writer.Create(name)
		if err != nil {
			t.Fatalf("add fixture part %s: %v", name, err)
		}
		if _, err := out.Write(data); err != nil {
			t.Fatalf("write fixture part %s bytes: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close fixture ZIP: %v", err)
	}
	if err := targetFile.Close(); err != nil {
		t.Fatalf("close fixture package: %v", err)
	}
	if err := seedFile.Close(); err != nil {
		t.Fatalf("close seed package: %v", err)
	}
	return targetPath
}

func addImageContentTypes(data []byte) []byte {
	var defaults strings.Builder
	for _, item := range []struct{ ext, contentType string }{
		{"png", "image/png"}, {"jpg", "image/jpeg"}, {"jpeg", "image/jpeg"}, {"gif", "image/gif"},
	} {
		if !bytes.Contains(data, []byte(`Extension="`+item.ext+`"`)) {
			defaults.WriteString(`<Default Extension="` + item.ext + `" ContentType="` + item.contentType + `"/>`)
		}
	}
	if defaults.Len() == 0 {
		return data
	}
	return bytes.Replace(data, []byte(`</Types>`), append([]byte(defaults.String()), []byte(`</Types>`)...), 1)
}
