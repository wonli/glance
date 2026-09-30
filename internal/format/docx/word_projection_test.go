package docx

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"glance/internal/document"
)

func projectionPNG(t *testing.T, value color.NRGBA) []byte {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	pixels.SetNRGBA(0, 0, value)
	var buf bytes.Buffer
	if err := png.Encode(&buf, pixels); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func projectionFixture(t *testing.T, documentXML, relationshipsXML string, media map[string][]byte) *Document {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	parts := map[string][]byte{
		"[Content_Types].xml": []byte(`<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`),
		"word/document.xml":   []byte(documentXML),
	}
	if relationshipsXML != "" {
		parts["word/_rels/document.xml.rels"] = []byte(relationshipsXML)
	}
	for name, data := range media {
		parts[name] = data
	}
	for name, data := range parts {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	doc, err := OpenSnapshot(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	return doc
}

func projectionRelationships() string {
	return `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rIdPicture1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/one.png"/><Relationship Id="rIdPicture2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/two.png"/></Relationships>`
}

func mixedContentProjectionFixture(t *testing.T) (*Document, []byte) {
	t.Helper()
	imageBytes := projectionPNG(t, color.NRGBA{R: 40, G: 120, B: 220, A: 255})
	documentXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">
<w:body>
 <w:p><w:pPr><w:pStyle w:val="Heading1"/><w:jc w:val="center"/></w:pPr>
  <w:r><w:rPr><w:b/><w:i w:val="0"/><w:color w:val="0080FF"/><w:rFonts w:ascii="Aptos"/><w:sz w:val="24"/></w:rPr><w:t xml:space="preserve"> before </w:t></w:r>
  <w:r><w:drawing><wp:inline><wp:extent cx="914400" cy="457200"/><wp:docPr id="1" name="First picture"/><a:graphic><a:graphicData><pic:pic><pic:blipFill><a:blip r:embed="rIdPicture1"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>
  <w:r><w:t> after</w:t></w:r>
 </w:p>
 <w:tbl><w:tr><w:tc>
  <w:p><w:r><w:t>Cell first paragraph</w:t></w:r></w:p>
  <w:p><w:r><w:t>Cell before</w:t></w:r><w:r><w:drawing><wp:inline><wp:extent cx="457200" cy="457200"/><wp:docPr id="2" name="Second picture"/><a:graphic><a:graphicData><pic:pic><pic:blipFill><a:blip r:embed="rIdPicture2"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r><w:r><w:t>Cell after</w:t></w:r></w:p>
 </w:tc></w:tr></w:tbl>
 <w:p><w:r><w:t>Floating follows</w:t><w:drawing><wp:anchor><wp:extent cx="1" cy="1"/><wp:docPr id="3" name="Floating"/></wp:anchor></w:drawing></w:r></w:p>
 <w:sectPr/>
</w:body></w:document>`
	doc := projectionFixture(t, documentXML, projectionRelationships(), map[string][]byte{
		"word/media/one.png": imageBytes,
		"word/media/two.png": projectionPNG(t, color.NRGBA{R: 230, G: 80, B: 40, A: 255}),
	})
	return doc, imageBytes
}

func requireWordContentReader(t *testing.T, doc *Document) document.WordContentReader {
	t.Helper()
	reader, ok := any(doc).(document.WordContentReader)
	if !ok {
		t.Fatal("DOCX adapter does not expose the neutral ordered Word content reader")
	}
	return reader
}

func TestDOCXProjectionPreservesBodyAndTableInlineOrderAndStyles(t *testing.T) {
	doc, _ := mixedContentProjectionFixture(t)
	reader := requireWordContentReader(t, doc)
	blocks := reader.WordBlocks()
	if len(blocks) != 3 {
		t.Fatalf("top-level blocks = %d; want paragraph, table, paragraph", len(blocks))
	}
	first := blocks[0].Paragraph
	if first == nil || blocks[0].Kind != document.WordParagraphBlock {
		t.Fatalf("first block = %#v; want paragraph", blocks[0])
	}
	if first.Style != "Heading1" || first.Alignment != document.WordAlignCenter {
		t.Fatalf("first paragraph style/alignment = %q/%q", first.Style, first.Alignment)
	}
	if len(first.Inlines) != 3 || first.Inlines[0].Kind != document.WordTextInline || first.Inlines[1].Kind != document.WordImageInline || first.Inlines[2].Kind != document.WordTextInline {
		t.Fatalf("first paragraph inline order = %#v; want text, image, text", first.Inlines)
	}
	if got := first.Inlines[0].Text; got == nil || got.Text != " before " {
		t.Fatalf("first text inline = %#v; want preserved boundary spaces", got)
	}
	style := first.Inlines[0].Text.Style
	if !style.Bold || style.Italic || style.FontFamily != "Aptos" || style.FontSize != 12 || style.FontColor != "0080FF" {
		t.Fatalf("text style = %#v; want bold Aptos 12pt blue", style)
	}
	image := first.Inlines[1].Image
	if image == nil || !image.Resolved || !image.Editable || image.MediaType != "image/png" || image.WidthEMU != 914400 || image.HeightEMU != 457200 {
		t.Fatalf("first inline image = %#v", image)
	}
	if got := first.Inlines[2].Text; got == nil || got.Text != " after" {
		t.Fatalf("trailing text inline = %#v", got)
	}

	table := blocks[1].Table
	if blocks[1].Kind != document.WordTableBlock || table == nil || len(table.Rows) != 1 || len(table.Rows[0].Cells) != 1 {
		t.Fatalf("table block = %#v", blocks[1])
	}
	cell := table.Rows[0].Cells[0]
	if len(cell.Paragraphs) != 2 || len(cell.Paragraphs[1].Inlines) != 3 {
		t.Fatalf("table cell paragraphs = %#v", cell.Paragraphs)
	}
	if cell.Paragraphs[1].Inlines[0].Text.Text != "Cell before" || cell.Paragraphs[1].Inlines[1].Kind != document.WordImageInline || cell.Paragraphs[1].Inlines[2].Text.Text != "Cell after" {
		t.Fatalf("table cell inline order = %#v", cell.Paragraphs[1].Inlines)
	}
}

func TestDOCXProjectionResolvesImageBytesAndKeepsFloatingObjectsReadOnly(t *testing.T) {
	doc, imageBytes := mixedContentProjectionFixture(t)
	reader := requireWordContentReader(t, doc)
	blocks := reader.WordBlocks()
	imageID := blocks[0].Paragraph.Inlines[1].Image.ID
	got, err := reader.WordImageBytes(imageID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, imageBytes) {
		t.Fatal("resolved image bytes differ from the relationship target")
	}
	last := blocks[2].Paragraph
	if last == nil || len(last.Inlines) != 2 || last.Inlines[0].Text == nil || last.Inlines[1].Kind != document.WordUnsupportedInline || last.Inlines[1].Unsupported == nil {
		t.Fatalf("floating drawing projection = %#v; want text plus read-only placeholder", last)
	}
	if last.Inlines[1].Unsupported.Description == "" {
		t.Fatal("floating drawing placeholder has no explanation")
	}
}

func TestDOCXProjectionMarksMissingAndUnsupportedImageRelationships(t *testing.T) {
	makeImageDocument := func(relID, target, mediaType string) *Document {
		documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><w:body><w:p><w:r><w:drawing><wp:inline><wp:extent cx="100" cy="50"/><a:graphic><a:graphicData><a:blip r:embed="` + relID + `"/></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p></w:body></w:document>`
		rels := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="` + relID + `" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="` + target + `"/></Relationships>`
		if relID == "missing" {
			rels = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`
		}
		ct := `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Default Extension="svg" ContentType="image/svg+xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`
		if mediaType == "image/svg+xml" {
			return projectionFixtureWithContentTypes(t, documentXML, rels, ct, map[string][]byte{"word/media/picture.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)})
		}
		return projectionFixtureWithContentTypes(t, documentXML, rels, ct, nil)
	}
	for _, test := range []struct {
		name      string
		relID     string
		target    string
		mediaType string
	}{
		{name: "missing relationship", relID: "missing", target: "media/missing.png", mediaType: "image/png"},
		{name: "unsupported media", relID: "rIdSvg", target: "media/picture.svg", mediaType: "image/svg+xml"},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc := makeImageDocument(test.relID, test.target, test.mediaType)
			reader := requireWordContentReader(t, doc)
			inline := reader.WordBlocks()[0].Paragraph.Inlines[0]
			if inline.Kind != document.WordUnsupportedInline || inline.Unsupported == nil || inline.Unsupported.Description == "" {
				t.Fatalf("unsupported image = %#v; want visible read-only placeholder", inline)
			}
			if _, err := doc.Snapshot(); err != nil {
				t.Fatalf("snapshot after projection: %v", err)
			}
		})
	}
}

func projectionFixtureWithContentTypes(t *testing.T, documentXML, relationshipsXML, contentTypesXML string, media map[string][]byte) *Document {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	parts := map[string][]byte{
		"[Content_Types].xml":          []byte(contentTypesXML),
		"word/document.xml":            []byte(documentXML),
		"word/_rels/document.xml.rels": []byte(relationshipsXML),
	}
	for name, data := range media {
		parts[name] = data
	}
	for name, data := range parts {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	doc, err := OpenSnapshot(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	return doc
}

func TestWordProjectionIDsAreUniqueAndImageBytesAreDefensiveCopies(t *testing.T) {
	doc, _ := mixedContentProjectionFixture(t)
	reader := requireWordContentReader(t, doc)
	blocks := reader.WordBlocks()
	blocks[0].Paragraph.Inlines[0].Text.Text = "changed only in caller data"
	if got := reader.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != " before " {
		t.Fatalf("mutating projected view changed adapter state: %q", got)
	}
	blocks = reader.WordBlocks()
	seen := make(map[string]bool)
	visit := func(id string) {
		t.Helper()
		if id == "" || seen[id] {
			t.Fatalf("empty or duplicate object id %q", id)
		}
		seen[id] = true
	}
	for _, block := range blocks {
		visit(block.ID)
		var visitParagraph func(document.WordParagraph)
		visitParagraph = func(paragraph document.WordParagraph) {
			visit(paragraph.ID)
			for _, inline := range paragraph.Inlines {
				switch inline.Kind {
				case document.WordTextInline:
					visit(inline.Text.ID)
				case document.WordImageInline:
					visit(inline.Image.ID)
				case document.WordUnsupportedInline:
					visit(inline.Unsupported.ID)
				}
			}
		}
		if block.Paragraph != nil {
			visitParagraph(*block.Paragraph)
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					visit(cell.ID)
					for _, paragraph := range cell.Paragraphs {
						visitParagraph(paragraph)
					}
				}
			}
		}
	}
	data, err := reader.WordImageBytes(blocks[0].Paragraph.Inlines[1].Image.ID)
	if err != nil {
		t.Fatal(err)
	}
	data[0] = 0
	again, err := reader.WordImageBytes(blocks[0].Paragraph.Inlines[1].Image.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again[0] == 0 {
		t.Fatal("caller mutation changed adapter-owned image bytes")
	}
}
