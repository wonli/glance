package docx

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"glance/internal/document"
)

func createRevisionTestDoc(t *testing.T, bodyXML string) *Document {
	t.Helper()
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	ct := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`
	w, err := zw.Create("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte(ct))

	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`
	w, err = zw.Create("_rels/.rels")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte(rels))

	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>` + bodyXML + `</w:body>
</w:document>`
	w, err = zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte(docXML))

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	doc, err := OpenSnapshot(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestWordTrackRevisions_ToggleAndUndo(t *testing.T) {
	doc := createRevisionTestDoc(t, `<w:p><w:r><w:t>Hello world</w:t></w:r></w:p>`)
	defer doc.Close()

	editor, ok := any(doc).(document.WordTrackRevisionsEditor)
	if !ok {
		t.Fatal("doc does not implement WordTrackRevisionsEditor")
	}

	enabled, err := editor.GetTrackRevisions()
	if err != nil {
		t.Fatalf("GetTrackRevisions failed: %v", err)
	}
	if enabled {
		t.Fatal("expected track revisions to be false initially")
	}

	// Enable track revisions
	if err := editor.SetTrackRevisions(true); err != nil {
		t.Fatalf("SetTrackRevisions(true) failed: %v", err)
	}
	enabled, err = editor.GetTrackRevisions()
	if err != nil || !enabled {
		t.Fatalf("expected track revisions to be true, got %v (err: %v)", enabled, err)
	}

	// Undo
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("doc does not implement WordHistory")
	}
	if !history.CanUndo() {
		t.Fatal("expected CanUndo to be true")
	}
	if _, err := history.Undo(); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	enabled, err = editor.GetTrackRevisions()
	if err != nil || enabled {
		t.Fatalf("expected track revisions to be false after undo, got %v", enabled)
	}

	// Redo
	if _, err := history.Redo(); err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	enabled, err = editor.GetTrackRevisions()
	if err != nil || !enabled {
		t.Fatalf("expected track revisions to be true after redo, got %v", enabled)
	}
}

func TestWordTrackRevisions_AcceptAllRevisions(t *testing.T) {
	body := `<w:p>
		<w:r><w:t>Start </w:t></w:r>
		<w:ins w:id="1" w:author="User"><w:r><w:t>Inserted </w:t></w:r></w:ins>
		<w:del w:id="2" w:author="User"><w:r><w:delText>Deleted </w:delText></w:r></w:del>
		<w:r><w:t>End</w:t></w:r>
	</w:p>`
	doc := createRevisionTestDoc(t, body)
	defer doc.Close()

	editor, ok := any(doc).(document.WordTrackRevisionsEditor)
	if !ok {
		t.Fatal("doc does not implement WordTrackRevisionsEditor")
	}

	count, err := editor.AcceptAllRevisions()
	if err != nil {
		t.Fatalf("AcceptAllRevisions failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 revisions accepted, got %d", count)
	}

	blocks := doc.WordBlocks()
	if len(blocks) != 1 || blocks[0].Paragraph == nil {
		t.Fatalf("expected 1 paragraph block, got %d", len(blocks))
	}
	var textBuilder strings.Builder
	for _, inline := range blocks[0].Paragraph.Inlines {
		if inline.Text != nil {
			textBuilder.WriteString(inline.Text.Text)
		}
	}
	fullText := textBuilder.String()
	if !strings.Contains(fullText, "Inserted") {
		t.Errorf("expected accepted text to contain 'Inserted', got: %q", fullText)
	}
	if strings.Contains(fullText, "Deleted") {
		t.Errorf("expected accepted text NOT to contain 'Deleted', got: %q", fullText)
	}

	// Undo should restore revisions
	history := any(doc).(document.WordHistory)
	if _, err := history.Undo(); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
}

func TestWordTrackRevisions_RejectAllRevisions(t *testing.T) {
	body := `<w:p>
		<w:r><w:t>Start </w:t></w:r>
		<w:ins w:id="1" w:author="User"><w:r><w:t>Inserted </w:t></w:r></w:ins>
		<w:del w:id="2" w:author="User"><w:r><w:delText>Deleted </w:delText></w:r></w:del>
		<w:r><w:t>End</w:t></w:r>
	</w:p>`
	doc := createRevisionTestDoc(t, body)
	defer doc.Close()

	editor, ok := any(doc).(document.WordTrackRevisionsEditor)
	if !ok {
		t.Fatal("doc does not implement WordTrackRevisionsEditor")
	}

	count, err := editor.RejectAllRevisions()
	if err != nil {
		t.Fatalf("RejectAllRevisions failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 revisions rejected, got %d", count)
	}

	blocks := doc.WordBlocks()
	if len(blocks) != 1 || blocks[0].Paragraph == nil {
		t.Fatalf("expected 1 paragraph block, got %d", len(blocks))
	}
	var textBuilder strings.Builder
	for _, inline := range blocks[0].Paragraph.Inlines {
		if inline.Text != nil {
			textBuilder.WriteString(inline.Text.Text)
		}
	}
	fullText := textBuilder.String()
	if strings.Contains(fullText, "Inserted") {
		t.Errorf("expected rejected text NOT to contain 'Inserted', got: %q", fullText)
	}
	if !strings.Contains(fullText, "Deleted") {
		t.Errorf("expected rejected text to restore 'Deleted', got: %q", fullText)
	}
}
