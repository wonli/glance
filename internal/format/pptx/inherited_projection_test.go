package pptx

import (
	"bytes"

	"fmt"

	"image/color"

	"os"
	"path/filepath"

	"strings"
	"testing"

	"glance/internal/document"
)

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
