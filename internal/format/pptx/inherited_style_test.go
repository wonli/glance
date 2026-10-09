package pptx

import (
	"bytes"

	"math"
	"os"
	"path/filepath"

	"strings"
	"testing"

	"glance/internal/document"
)

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
