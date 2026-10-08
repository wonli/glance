package pptx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
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
