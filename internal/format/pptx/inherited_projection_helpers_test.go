package pptx

import (
	"fmt"

	"testing"

	"glance/internal/document"
)

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
