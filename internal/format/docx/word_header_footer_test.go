package docx

import (
	"strings"
	"testing"

	"glance/internal/document"
)

func TestWordHeaderFooter_SetAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordHeaderFooterEditor)
	if !ok {
		t.Fatal("doc does not implement WordHeaderFooterEditor")
	}

	// 1. Set Header
	err = editor.SetHeader("机密文档 - 仅供内部传阅")
	if err != nil {
		t.Fatalf("SetHeader failed: %v", err)
	}

	headerPart, found := findPackagePart(doc.parts, "word/header1.xml")
	if !found {
		t.Fatal("word/header1.xml not found")
	}
	if !strings.Contains(string(headerPart.data), "机密文档 - 仅供内部传阅") {
		t.Fatalf("header content missing: %s", string(headerPart.data))
	}

	docXML := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(docXML, "headerReference") {
		t.Fatal("document.xml missing headerReference")
	}

	// 2. Set Footer
	err = editor.SetFooter("第 1 页 · Glance 文档系统")
	if err != nil {
		t.Fatalf("SetFooter failed: %v", err)
	}

	footerPart, found := findPackagePart(doc.parts, "word/footer1.xml")
	if !found {
		t.Fatal("word/footer1.xml not found")
	}
	if !strings.Contains(string(footerPart.data), "第 1 页 · Glance 文档系统") {
		t.Fatalf("footer content missing: %s", string(footerPart.data))
	}

	// 3. Set Page Number
	err = editor.SetPageNumber("center")
	if err != nil {
		t.Fatalf("SetPageNumber failed: %v", err)
	}
	footerPart, _ = findPackagePart(doc.parts, "word/footer1.xml")
	if !strings.Contains(string(footerPart.data), "PAGE") {
		t.Fatalf("page number field missing in footer: %s", string(footerPart.data))
	}

	// 4. Undo
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("doc does not implement WordHistory")
	}
	if !history.CanUndo() {
		t.Fatal("expected CanUndo after edits")
	}
	if _, err := history.Undo(); err != nil {
		t.Fatalf("undo failed: %v", err)
	}
}
