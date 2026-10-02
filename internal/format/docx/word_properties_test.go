package docx

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"glance/internal/document"
)

const wordCorePropertiesRelationship = "http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties"

func TestWordDocumentPropertiesEditRoundTripsAndPreservesUnknownMetadata(t *testing.T) {
	fixture := wordPropertiesFixture(t, true, false)
	original, err := fixture.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "metadata.docx")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	initial := getWordDocumentProperties(t, doc)
	assertWordProperty(t, initial, "Title", "Original title")
	assertWordProperty(t, initial, "Subject", "Original subject")
	assertWordProperty(t, initial, "Creator", "Original author")
	assertWordProperty(t, initial, "Keywords", "one; two")
	assertWordProperty(t, initial, "Description", "Original description")

	want := map[string]string{
		"Title":       `新标题 <& "quoted"`,
		"Subject":     "项目主题",
		"Creator":     "作者",
		"Keywords":    "文档, 测试",
		"Description": "说明文字",
	}
	if err := setWordDocumentProperties(t, doc, want); err != nil {
		t.Fatal(err)
	}
	if !doc.Dirty() {
		t.Fatal("editing core properties did not mark the DOCX dirty")
	}

	data, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	if doc.Dirty() {
		t.Fatal("successful DOCX save left document dirty")
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got := getWordDocumentProperties(t, reopened)
	for name, value := range want {
		assertWordProperty(t, got, name, value)
	}
	after := wordPropertiesZipParts(t, data)
	before := wordPropertiesZipParts(t, original)
	for name, originalPart := range before {
		if name == "docProps/core.xml" {
			continue
		}
		if !bytes.Equal(after[name], originalPart) {
			t.Errorf("editing core properties changed unrelated package part %q", name)
		}
	}
	core := string(after["docProps/core.xml"])
	for _, preserved := range []string{`<cp:lastModifiedBy>keep-me</cp:lastModifiedBy>`, `<cp:revision>17</cp:revision>`, `<x:custom flag="keep">retained</x:custom>`} {
		if !strings.Contains(core, preserved) {
			t.Errorf("editing core properties removed unknown metadata %q: %s", preserved, core)
		}
	}
	if !strings.Contains(core, `新标题 &lt;&amp; &#34;quoted&#34;`) {
		t.Fatalf("metadata text was not XML-escaped: %s", core)
	}

	if changed, err := doc.Undo(); err != nil || !changed {
		t.Fatalf("Undo() = %v, %v; want changed", changed, err)
	}
	undoData, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wordPropertiesZipParts(t, undoData)["docProps/core.xml"], before["docProps/core.xml"]) {
		t.Fatal("undo did not restore the original core properties")
	}
	if changed, err := doc.Redo(); err != nil || !changed {
		t.Fatalf("Redo() = %v, %v; want changed", changed, err)
	}
	redoData, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wordPropertiesZipParts(t, redoData)["docProps/core.xml"], after["docProps/core.xml"]) {
		t.Fatal("redo did not restore the edited core properties")
	}
}

func TestWordDocumentPropertiesCreateMissingCorePartAndPackageLinks(t *testing.T) {
	doc := wordPropertiesFixture(t, false, false)
	defer doc.Close()
	original, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := setWordDocumentProperties(t, doc, map[string]string{"Title": "New title", "Creator": "New author"}); err != nil {
		t.Fatal(err)
	}
	data, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	parts := wordPropertiesZipParts(t, data)
	if !strings.Contains(string(parts["_rels/.rels"]), wordCorePropertiesRelationship) || !strings.Contains(string(parts["_rels/.rels"]), `Target="docProps/core.xml"`) {
		t.Fatalf("new core-properties part is not related from package root: %s", parts["_rels/.rels"])
	}
	if !strings.Contains(string(parts["[Content_Types].xml"]), `PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"`) {
		t.Fatalf("new core-properties part has no content type: %s", parts["[Content_Types].xml"])
	}
	if !strings.Contains(string(parts["docProps/core.xml"]), `<dc:title>New title</dc:title>`) || !strings.Contains(string(parts["docProps/core.xml"]), `<dc:creator>New author</dc:creator>`) {
		t.Fatalf("new core-properties part is missing editable values: %s", parts["docProps/core.xml"])
	}
	before := wordPropertiesZipParts(t, original)
	for _, name := range []string{"word/document.xml", "custom/keep.bin"} {
		if !bytes.Equal(parts[name], before[name]) {
			t.Errorf("creating core properties changed unrelated package part %q", name)
		}
	}
	reopened, err := OpenSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got := getWordDocumentProperties(t, reopened)
	assertWordProperty(t, got, "Title", "New title")
	assertWordProperty(t, got, "Creator", "New author")
}

func TestWordDocumentPropertiesRejectAmbiguousFieldWithoutMutation(t *testing.T) {
	doc := wordPropertiesFixture(t, true, true)
	defer doc.Close()
	before, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := setWordDocumentProperties(t, doc, map[string]string{"Title": "Must not replace duplicates"}); err == nil {
		t.Fatal("editing duplicate dc:title properties should be rejected")
	}
	after, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || doc.Dirty() {
		t.Fatal("rejecting ambiguous metadata changed the package or dirty state")
	}
}

func TestWordDocumentPropertiesNoopAndRejectNestedValue(t *testing.T) {
	doc := wordPropertiesFixture(t, true, false)
	defer doc.Close()
	before, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SetWordDocumentProperties(wordProperties("Original title", "Original subject", "Original author", "one; two", "Original description")); err != nil {
		t.Fatal(err)
	}
	after, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if doc.Dirty() || !bytes.Equal(before, after) {
		t.Fatal("applying unchanged metadata should be a no-op")
	}

	index := packagePartIndex(doc.parts, "docProps/core.xml")
	doc.parts[index].data = bytes.Replace(doc.parts[index].data, []byte("<dc:title>Original title</dc:title>"), []byte("<dc:title><x:nested>Original title</x:nested></dc:title>"), 1)
	before, err = doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SetWordDocumentProperties(wordProperties("Changed", "Original subject", "Original author", "one; two", "Original description")); err == nil {
		t.Fatal("editing nested metadata markup should be rejected")
	}
	after, err = doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if doc.Dirty() || !bytes.Equal(before, after) {
		t.Fatal("rejecting nested metadata changed the package or dirty state")
	}
}

func TestWordDocumentPropertiesReuseOrphanedAndSelfClosingCorePart(t *testing.T) {
	t.Run("orphaned part keeps existing and unknown metadata", func(t *testing.T) {
		doc := wordPropertiesFixture(t, false, false)
		defer doc.Close()
		core := `<cp:coreProperties xmlns:cp="` + wordCorePropertiesNS + `" xmlns:dc="` + wordDCElementsNS + `" xmlns:x="urn:glance:test"><dc:title>Keep this title</dc:title><x:custom>keep this too</x:custom></cp:coreProperties>`
		doc.parts = append(doc.parts, part{header: zip.FileHeader{Name: "docProps/core.xml", Method: zip.Deflate}, data: []byte(core)})
		if err := doc.SetWordDocumentProperties(wordProperties("Keep this title", "New subject", "", "", "")); err != nil {
			t.Fatal(err)
		}
		properties, err := doc.GetWordDocumentProperties()
		if err != nil {
			t.Fatal(err)
		}
		if properties.Title != "Keep this title" || properties.Subject != "New subject" {
			t.Fatalf("metadata after linking orphaned part = %+v", properties)
		}
		data, err := doc.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(wordPropertiesZipParts(t, data)["docProps/core.xml"], []byte(`<x:custom>keep this too</x:custom>`)) {
			t.Fatal("linking orphaned core properties removed unknown metadata")
		}
	})

	t.Run("self-closing core root expands safely", func(t *testing.T) {
		doc := wordPropertiesFixture(t, false, false)
		defer doc.Close()
		core := `<cp:coreProperties xmlns:cp="` + wordCorePropertiesNS + `"/>`
		doc.parts = append(doc.parts, part{header: zip.FileHeader{Name: "docProps/core.xml", Method: zip.Deflate}, data: []byte(core)})
		if err := doc.SetWordDocumentProperties(wordProperties("Expanded root", "", "", "", "")); err != nil {
			t.Fatal(err)
		}
		properties, err := doc.GetWordDocumentProperties()
		if err != nil {
			t.Fatal(err)
		}
		if properties.Title != "Expanded root" {
			t.Fatalf("title after expanding core root = %q", properties.Title)
		}
	})
}

func wordProperties(title, subject, creator, keywords, description string) document.WordDocumentProperties {
	return document.WordDocumentProperties{Title: title, Subject: subject, Creator: creator, Keywords: keywords, Description: description}
}

func wordPropertiesFixture(t *testing.T, withCore, duplicateTitle bool) *Document {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	parts := []struct{ name, data string }{
		{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`},
		{"_rels/.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"word/document.xml", `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>fixture</w:t></w:r></w:p></w:body></w:document>`},
	}
	if withCore {
		parts[0].data = strings.Replace(parts[0].data, `</Types>`, `<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/></Types>`, 1)
		parts[1].data = strings.Replace(parts[1].data, `</Relationships>`, `<Relationship Id="rId2" Type="`+wordCorePropertiesRelationship+`" Target="docProps/core.xml"/></Relationships>`, 1)
		core := `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:x="urn:glance:test"><dc:title>Original title</dc:title><dc:subject>Original subject</dc:subject><dc:creator>Original author</dc:creator><cp:keywords>one; two</cp:keywords><dc:description>Original description</dc:description><cp:lastModifiedBy>keep-me</cp:lastModifiedBy><cp:revision>17</cp:revision><x:custom flag="keep">retained</x:custom></cp:coreProperties>`
		if duplicateTitle {
			core = strings.Replace(core, `</dc:title>`, `</dc:title><dc:title>duplicate</dc:title>`, 1)
		}
		parts = append(parts, struct{ name, data string }{"docProps/core.xml", core})
	}
	for _, item := range parts {
		entry, err := writer.Create(item.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, item.data); err != nil {
			t.Fatal(err)
		}
	}
	entry, err := writer.Create("custom/keep.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("preserve this package part")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	doc, err := OpenSnapshot(archive.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func getWordDocumentProperties(t *testing.T, doc *Document) reflect.Value {
	t.Helper()
	method := reflect.ValueOf(doc).MethodByName("GetWordDocumentProperties")
	if !method.IsValid() {
		t.Fatal("DOCX adapter does not expose GetWordDocumentProperties")
	}
	values := method.Call(nil)
	if len(values) != 2 {
		t.Fatalf("GetWordDocumentProperties returned %d values; want properties and error", len(values))
	}
	if !values[1].IsNil() {
		t.Fatalf("GetWordDocumentProperties failed: %v", values[1].Interface())
	}
	return values[0]
}

func setWordDocumentProperties(t *testing.T, doc *Document, properties map[string]string) error {
	t.Helper()
	method := reflect.ValueOf(doc).MethodByName("SetWordDocumentProperties")
	if !method.IsValid() {
		t.Fatal("DOCX adapter does not expose SetWordDocumentProperties")
	}
	value := reflect.New(method.Type().In(0)).Elem()
	for name, text := range properties {
		field := value.FieldByName(name)
		if !field.IsValid() || !field.CanSet() || field.Kind() != reflect.String {
			t.Fatalf("Word properties API has no writable string field %q", name)
		}
		field.SetString(text)
	}
	results := method.Call([]reflect.Value{value})
	if len(results) != 1 {
		t.Fatalf("SetWordDocumentProperties returned %d values; want error", len(results))
	}
	if results[0].IsNil() {
		return nil
	}
	err, ok := results[0].Interface().(error)
	if !ok {
		t.Fatalf("SetWordDocumentProperties returned %T, want error", results[0].Interface())
	}
	return err
}

func assertWordProperty(t *testing.T, properties reflect.Value, name, want string) {
	t.Helper()
	field := properties.FieldByName(name)
	if !field.IsValid() || field.Kind() != reflect.String {
		t.Fatalf("Word properties result has no string field %q", name)
	}
	if got := field.String(); got != want {
		t.Errorf("Word property %s = %q; want %q", name, got, want)
	}
}

func wordPropertiesZipParts(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		parts[file.Name] = content
	}
	return parts
}
