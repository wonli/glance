package docx

import (
	"bytes"
	"strings"
	"testing"

	"glance/internal/document"
)

func TestWordBookmark_InsertAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordBookmarkEditor)
	if !ok {
		t.Fatal("doc does not implement WordBookmarkEditor")
	}

	err = editor.InsertBookmark("", "Section_One")
	if err != nil {
		t.Fatalf("InsertBookmark failed: %v", err)
	}

	xml := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, "w:name=\"Section_One\"") {
		t.Fatalf("XML missing bookmark name: %s", xml)
	}
	if !strings.Contains(xml, "w:bookmarkStart") || !strings.Contains(xml, "w:bookmarkEnd") {
		t.Fatalf("XML missing bookmark tags")
	}

	// Undo
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("doc does not implement WordHistory")
	}
	if !history.CanUndo() {
		t.Fatal("expected CanUndo after InsertBookmark")
	}
	if _, err := history.Undo(); err != nil {
		t.Fatalf("undo failed: %v", err)
	}
	xmlAfterUndo := string(doc.parts[doc.documentIdx].data)
	if strings.Contains(xmlAfterUndo, "Section_One") {
		t.Fatal("expected bookmark removed after undo")
	}
}

func TestWordBookmark_UnknownExplicitRunDoesNotFallBackOrMutate(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordBookmarkEditor)
	if !ok {
		t.Fatal("doc does not implement WordBookmarkEditor")
	}
	before := append([]byte(nil), doc.parts[doc.documentIdx].data...)
	err = editor.InsertBookmark("missing-run", "Ghost")
	if err == nil {
		t.Fatal("InsertBookmark accepted an unknown explicit run ID")
	}
	if after := doc.parts[doc.documentIdx].data; !bytes.Equal(after, before) {
		t.Fatalf("unknown bookmark target changed document XML: before=%s after=%s", before, after)
	}
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("doc does not implement WordHistory")
	}
	if history.CanUndo() {
		t.Fatal("unknown bookmark target created an undo record")
	}
}

func TestWordHyperlink_InsertAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordHyperlinkEditor)
	if !ok {
		t.Fatal("doc does not implement WordHyperlinkEditor")
	}

	// 1. External link
	err = editor.InsertHyperlink("", "访问官网", "https://example.com")
	if err != nil {
		t.Fatalf("InsertHyperlink external failed: %v", err)
	}

	xml := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, "访问官网") {
		t.Fatalf("XML missing link text: %s", xml)
	}
	if !strings.Contains(xml, "w:hyperlink") {
		t.Fatalf("XML missing w:hyperlink tag")
	}

	// Check relationship in word/_rels/document.xml.rels
	relsPart, found := findPackagePart(doc.parts, "word/_rels/document.xml.rels")
	if !found {
		t.Fatal("word/_rels/document.xml.rels not found")
	}
	if !strings.Contains(string(relsPart.data), "https://example.com") {
		t.Fatalf("rels missing target URL: %s", string(relsPart.data))
	}

	// Undo
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("doc does not implement WordHistory")
	}
	if !history.CanUndo() {
		t.Fatal("expected CanUndo after InsertHyperlink")
	}
	if _, err := history.Undo(); err != nil {
		t.Fatalf("undo failed: %v", err)
	}

	// 2. Internal anchor link
	err = editor.InsertHyperlink("", "跳转至第一节", "#Section_One")
	if err != nil {
		t.Fatalf("InsertHyperlink internal failed: %v", err)
	}
	xmlInternal := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xmlInternal, "w:anchor=\"Section_One\"") {
		t.Fatalf("XML missing w:anchor: %s", xmlInternal)
	}
}

func TestWordHyperlink_UnknownExplicitRunDoesNotFallBackOrMutate(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordHyperlinkEditor)
	if !ok {
		t.Fatal("doc does not implement WordHyperlinkEditor")
	}
	before := append([]byte(nil), doc.parts[doc.documentIdx].data...)
	err = editor.InsertHyperlink("missing-run", "Ghost", "https://example.com")
	if err == nil {
		t.Fatal("InsertHyperlink accepted an unknown explicit run ID")
	}
	if after := doc.parts[doc.documentIdx].data; !bytes.Equal(after, before) {
		t.Fatalf("unknown hyperlink target changed document XML: before=%s after=%s", before, after)
	}
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("doc does not implement WordHistory")
	}
	if history.CanUndo() {
		t.Fatal("unknown hyperlink target created an undo record")
	}
}

func TestWordTextBox_InsertAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordTextBoxEditor)
	if !ok {
		t.Fatal("doc does not implement WordTextBoxEditor")
	}

	err = editor.InsertTextBox("", "这是一个重要提示文本框")
	if err != nil {
		t.Fatalf("InsertTextBox failed: %v", err)
	}

	xml := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, "这是一个重要提示文本框") {
		t.Fatalf("XML missing textbox text: %s", xml)
	}
	if !strings.Contains(xml, "2B579A") {
		t.Fatalf("XML missing callout border color")
	}

	// Undo
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("doc does not implement WordHistory")
	}
	if !history.CanUndo() {
		t.Fatal("expected CanUndo after InsertTextBox")
	}
	if _, err := history.Undo(); err != nil {
		t.Fatalf("undo failed: %v", err)
	}
	xmlAfterUndo := string(doc.parts[doc.documentIdx].data)
	if strings.Contains(xmlAfterUndo, "这是一个重要提示文本框") {
		t.Fatal("expected textbox text removed after undo")
	}
}

func TestWordTextBox_UnknownExplicitParagraphDoesNotFallBackOrMutate(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordTextBoxEditor)
	if !ok {
		t.Fatal("doc does not implement WordTextBoxEditor")
	}
	before := append([]byte(nil), doc.parts[doc.documentIdx].data...)
	dirtyBefore := doc.Dirty()
	if err := editor.InsertTextBox("missing-paragraph", "不会插入"); err == nil {
		t.Fatal("InsertTextBox accepted an unknown explicit paragraph ID")
	}
	if after := doc.parts[doc.documentIdx].data; !bytes.Equal(after, before) {
		t.Fatalf("unknown text box target changed document XML: before=%s after=%s", before, after)
	}
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("doc does not implement WordHistory")
	}
	if history.CanUndo() {
		t.Fatal("unknown text box target created an undo record")
	}
	if dirtyAfter := doc.Dirty(); dirtyAfter != dirtyBefore {
		t.Fatalf("unknown text box target changed dirty state from %v to %v", dirtyBefore, dirtyAfter)
	}
}
