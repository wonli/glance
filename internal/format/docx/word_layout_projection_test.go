package docx

import (
	"archive/zip"
	"bytes"
	"glance/internal/document"
	"strings"
	"testing"
)

func wordLayoutProjectionFixture(t *testing.T, documentXML, stylesXML string) *Document {
	return wordLayoutProjectionFixtureWithSettings(t, documentXML, stylesXML, "")
}

func wordLayoutProjectionFixtureWithSettings(t *testing.T, documentXML, stylesXML, settingsXML string) *Document {
	t.Helper()
	parts := map[string][]byte{
		"[Content_Types].xml": []byte(`<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`),
		"_rels/.rels":         []byte(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`),
		"word/document.xml":   []byte(documentXML),
	}
	var relationships []string
	if stylesXML != "" {
		relationships = append(relationships, `<Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`)
		parts["word/styles.xml"] = []byte(stylesXML)
	}
	if settingsXML != "" {
		relationships = append(relationships, `<Relationship Id="rIdSettings" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>`)
		parts["word/settings.xml"] = []byte(settingsXML)
	}
	if len(relationships) > 0 {
		parts["word/_rels/document.xml.rels"] = []byte(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(relationships, "") + `</Relationships>`)
	}
	var packageBytes bytes.Buffer
	writer := zip.NewWriter(&packageBytes)
	for name, data := range parts {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	doc, err := OpenSnapshot(packageBytes.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	return doc
}

func requireWordLayoutReader(t *testing.T, doc *Document) document.WordLayoutReader {
	t.Helper()
	reader, ok := any(doc).(document.WordLayoutReader)
	if !ok {
		t.Fatal("DOCX adapter does not expose neutral Word layout metadata")
	}
	return reader
}
