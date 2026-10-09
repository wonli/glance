package docx

import (
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
)

func TestFloatingImageWrapCorpusProjectsVMLImageAndFollowingText(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "office-corpus", "docx", "floating-image-wrap-vml.docx")
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	var projectedImage *document.WordImage
	var imageParagraph *document.WordParagraph
	for _, block := range doc.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Unsupported != nil {
				t.Errorf("unsupported inline %q: %s", inline.Unsupported.ID, inline.Unsupported.Description)
			}
			if inline.Image != nil && (inline.Image.InlineFallback || inline.Image.Floating != nil) {
				projectedImage, imageParagraph = inline.Image, block.Paragraph
			}
		}
	}
	if projectedImage == nil || imageParagraph == nil {
		t.Fatal("floating VML image was not projected")
	}
	if !projectedImage.Resolved || projectedImage.Editable || projectedImage.InlineFallback || projectedImage.WidthEMU != 2828925 || projectedImage.HeightEMU != 1998980 {
		t.Fatalf("VML image = %+v; want resolved read-only float with 222.75×157.4pt extent", projectedImage)
	}
	if projectedImage.Floating == nil {
		t.Fatal("VML image has no floating layout metadata")
	}
	floating := projectedImage.Floating
	if floating.XOffsetEMU != 1533525 || floating.YOffsetEMU != 341630 || floating.WrapType != "tight" {
		t.Fatalf("VML float position/wrap = %+v; want 120.75pt, 26.9pt, tight", floating)
	}
	if floating.WrapDistanceLeftEMU != 114300 || floating.WrapDistanceRightEMU != 114300 {
		t.Fatalf("VML float wrap distances = left %d, right %d; want Microsoft's 9pt defaults", floating.WrapDistanceLeftEMU, floating.WrapDistanceRightEMU)
	}
	if len(floating.WrapPolygon) != 5 {
		t.Fatalf("VML float polygon has %d points; want 5", len(floating.WrapPolygon))
	}
	if floating.WrapPolygon[0].X != -73.0/21600 || floating.WrapPolygon[1].Y != 21497.0/21600 || floating.WrapPolygon[2].X != 1 {
		t.Fatalf("VML float normalized polygon = %+v; want source coordinates divided by coordsize 21600", floating.WrapPolygon)
	}
	imageBytes, err := doc.WordImageBytes(projectedImage.ID)
	if err != nil || len(imageBytes) == 0 {
		t.Fatalf("VML image bytes = %d bytes, error %v", len(imageBytes), err)
	}
	floating.WrapPolygon[0].X = 12
	for _, block := range doc.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Image != nil && inline.Image.ID == projectedImage.ID && inline.Image.Floating.WrapPolygon[0].X == 12 {
				t.Fatal("mutating projected float metadata changed the DOCX adapter's internal model")
			}
		}
	}
	imageIndex := -1
	for index, inline := range imageParagraph.Inlines {
		if inline.Image == projectedImage {
			imageIndex = index
		}
	}
	if imageIndex < 0 || imageIndex+1 >= len(imageParagraph.Inlines) || imageParagraph.Inlines[imageIndex+1].Text == nil || !strings.Contains(imageParagraph.Inlines[imageIndex+1].Text.Text, "Pellentesque porttitor") {
		t.Fatalf("paragraph inlines = %+v; want the adjacent text after the VML image", imageParagraph.Inlines)
	}
}

func TestVMLFloatingImageUnsupportedGeometryFallsBack(t *testing.T) {
	baseShape := `<v:shape style="position:absolute;margin-left:1pt;margin-top:2pt;width:10pt;height:10pt" wrapcoords="0 0 0 21600 21600 21600 21600 0"><v:imagedata r:id="rId4"/>%s</v:shape>`
	cases := []struct {
		name  string
		shape string
	}{
		{name: "omitted wrap type means none", shape: strings.Replace(baseShape, "%s", `<w10:wrap/>`, 1)},
		{name: "page anchor is unsupported", shape: strings.Replace(baseShape, "%s", `<w10:wrap type="tight" anchorx="page"/>`, 1)},
		{name: "single-side wrap is unsupported", shape: strings.Replace(baseShape, "%s", `<w10:wrap type="tight" side="left"/>`, 1)},
		{name: "malformed polygon", shape: strings.Replace(strings.Replace(baseShape, `wrapcoords="0 0 0 21600 21600 21600 21600 0"`, `wrapcoords="0 0 21600"`, 1), "%s", `<w10:wrap type="tight"/>`, 1)},
		{name: "relative position is unsupported", shape: strings.Replace(strings.Replace(baseShape, "position:absolute", "position:relative", 1), "%s", `<w10:wrap type="tight"/>`, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, err := parseWordXML([]byte(`<root xmlns:v="urn:schemas-microsoft-com:vml" xmlns:w10="urn:schemas-microsoft-com:office:word" xmlns:r="urn:schemas.openxmlformats.org/officeDocument/2006/relationships"><v:shapetype id="shape" coordsize="21600,21600"/>` + tc.shape + `</root>`))
			if err != nil {
				t.Fatal(err)
			}
			pict := root
			projector := wordProjector{
				relationships: map[string]wordRelationship{"rId4": {target: "word/media/image.png", image: true}},
				partIndexes:   map[string]int{"word/media/image.png": 0},
				contentTypes:  wordContentTypes{defaults: map[string]string{"png": "image/png"}},
				imageParts:    make(map[string]int),
			}
			inline := projector.vmlImageInline(pict, "fixture-image")
			if inline.Kind != document.WordImageInline || inline.Image == nil || !inline.Image.Resolved || !inline.Image.InlineFallback || inline.Image.Floating != nil {
				t.Fatalf("unsupported VML projection = %+v; want visible resolved inline fallback", inline)
			}
		})
	}
}
