package docx

import (
	"archive/zip"
	"bytes"
	"glance/internal/document"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strconv"
	"strings"
	"testing"
)

func imageFixtureBytes(t *testing.T, width, height int, colorValue color.NRGBA) []byte {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixels.SetNRGBA(x, y, colorValue)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, pixels); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func imageFixtureJPEG(t *testing.T, width, height int, colorValue color.NRGBA) []byte {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixels.SetNRGBA(x, y, colorValue)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, pixels, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func wordImageDrawing(docPrID int, relationshipID string, widthEMU, heightEMU int64) string {
	return `<w:r><w:drawing><wp:inline><wp:extent cx="` + strconvInt64(widthEMU) + `" cy="` + strconvInt64(heightEMU) + `"/><wp:docPr id="` + strconvInt(docPrID) + `" name="Picture ` + strconvInt(docPrID) + `"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:blipFill><a:blip r:embed="` + relationshipID + `"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`
}

func imageOperationFixture(t *testing.T, documentXML, relationshipsXML, contentTypesXML string, media map[string][]byte) *Document {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	parts := map[string][]byte{
		"[Content_Types].xml":          []byte(contentTypesXML),
		"word/document.xml":            []byte(documentXML),
		"word/_rels/document.xml.rels": []byte(relationshipsXML),
		"custom/untouched.bin":         []byte("keep this package part byte-for-byte"),
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

func imageContentTypes() string {
	return `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`
}

func imageRelationshipRoot(children ...string) string {
	return `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(children, "") + `</Relationships>`
}

func imageDocRoot(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:body>` + body + `</w:body></w:document>`
}

func requireWordImageEditor(t *testing.T, doc *Document) document.WordImageEditor {
	t.Helper()
	editor, ok := any(doc).(document.WordImageEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose inline image editing")
	}
	return editor
}

func projectedWordImage(doc *Document, imageID string) *document.WordImage {
	for _, block := range doc.WordBlocks() {
		if block.Paragraph != nil {
			for _, inline := range block.Paragraph.Inlines {
				if inline.Image != nil && inline.Image.ID == imageID {
					return inline.Image
				}
			}
		}
	}
	return nil
}

// 2000x3000 image

func imageIDsInParagraph(paragraph document.WordParagraph) []string {
	var ids []string
	for _, inline := range paragraph.Inlines {
		if inline.Image != nil {
			ids = append(ids, inline.Image.ID)
		}
	}
	return ids
}

func assertImageUntouchedParts(t *testing.T, original, after []byte, changed map[string]bool) {
	t.Helper()
	beforeParts := readZipParts(t, original)
	afterParts := readZipParts(t, after)
	for name, want := range beforeParts {
		if changed[name] {
			continue
		}
		if !bytes.Equal(afterParts[name], want) {
			t.Errorf("untouched image package part %q changed", name)
		}
	}
}

func strconvInt(value int) string { return strconv.Itoa(value) }

func strconvInt64(value int64) string { return strconv.FormatInt(value, 10) }
