package docx

import (
	"bytes"
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
	if !strings.Contains(string(footerPart.data), "第 1 页 · Glance 文档系统") {
		t.Fatalf("setting a page number discarded existing footer text: %s", string(footerPart.data))
	}
	if err := editor.SetPageNumber("right"); err != nil {
		t.Fatalf("realign existing page number: %v", err)
	}
	footerPart, _ = findPackagePart(doc.parts, "word/footer1.xml")
	if strings.Count(string(footerPart.data), "instr=\"PAGE\"") != 1 || !strings.Contains(string(footerPart.data), `w:val="right"`) || !strings.Contains(string(footerPart.data), "第 1 页 · Glance 文档系统") {
		t.Fatalf("realigning page number duplicated the field or changed footer content: %s", string(footerPart.data))
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

func TestWordHeaderFooterRepeatedSetDoesNotChangeSnapshotOrHistory(t *testing.T) {
	cases := []struct {
		name string
		set  func(document.WordHeaderFooterEditor) error
	}{
		{"header", func(editor document.WordHeaderFooterEditor) error { return editor.SetHeader("周报标题") }},
		{"footer", func(editor document.WordHeaderFooterEditor) error { return editor.SetFooter("内部使用") }},
		{"page number", func(editor document.WordHeaderFooterEditor) error { return editor.SetPageNumber("center") }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			doc, err := NewBlank()
			if err != nil {
				t.Fatal(err)
			}
			defer doc.Close()
			editor, ok := any(doc).(document.WordHeaderFooterEditor)
			if !ok {
				t.Fatal("doc does not implement WordHeaderFooterEditor")
			}
			history, ok := any(doc).(document.WordHistory)
			if !ok {
				t.Fatal("doc does not implement WordHistory")
			}
			before, err := doc.Snapshot()
			if err != nil {
				t.Fatalf("snapshot initial document: %v", err)
			}
			dirtyBefore := doc.Dirty()
			if err := test.set(editor); err != nil {
				t.Fatalf("initial %s set: %v", test.name, err)
			}
			afterFirst, err := doc.Snapshot()
			if err != nil {
				t.Fatalf("snapshot after first set: %v", err)
			}
			if bytes.Equal(before, afterFirst) || !history.CanUndo() {
				t.Fatal("initial set did not change the package and create one undoable edit")
			}
			if err := test.set(editor); err != nil {
				t.Fatalf("repeated %s set: %v", test.name, err)
			}
			afterRepeat, err := doc.Snapshot()
			if err != nil {
				t.Fatalf("snapshot after repeated set: %v", err)
			}
			if !bytes.Equal(afterFirst, afterRepeat) {
				t.Fatal("repeated set changed DOCX package bytes")
			}
			applied, err := history.Undo()
			if err != nil || !applied {
				t.Fatalf("undo repeated set: applied=%v error=%v", applied, err)
			}
			afterUndo, err := doc.Snapshot()
			if err != nil {
				t.Fatalf("snapshot after undo: %v", err)
			}
			if !bytes.Equal(before, afterUndo) || history.CanUndo() || doc.Dirty() != dirtyBefore {
				t.Fatalf("repeated set added history: restored=%v can_undo=%v dirty=%v want=%v", bytes.Equal(before, afterUndo), history.CanUndo(), doc.Dirty(), dirtyBefore)
			}
		})
	}
}
