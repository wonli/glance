package docx

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, path, documentXML string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	entry, err := z.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(documentXML)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceSingleRunTextAndSaveCopy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minimal.docx")
	writeFixture(t, path, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Hello</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`)
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	blocks := doc.Blocks()
	if len(blocks) != 1 || blocks[0].Text != "Hello" {
		t.Fatalf("blocks = %#v", blocks)
	}
	if err := doc.ReplaceParagraph(0, "你好 & welcome"); err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(t.TempDir(), "saved.docx")
	if err := doc.SaveAs(copyPath); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.Blocks()[0].Text; got != "你好 & welcome" {
		t.Fatalf("saved paragraph = %q", got)
	}
}

func TestReplaceParagraphRejectsMultipleRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "styled.docx")
	writeFixture(t, path, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>bold</w:t></w:r><w:r><w:t> normal</w:t></w:r></w:p></w:body></w:document>`)
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	if err := doc.ReplaceParagraph(0, "replacement"); err == nil {
		t.Fatal("expected multi-run paragraph replacement to be rejected")
	}
}

func TestReplaceParagraphPreservesBoundarySpaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spaces.docx")
	writeFixture(t, path, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>text</w:t></w:r></w:p></w:body></w:document>`)
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.ReplaceParagraph(0, " spaced "); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "spaces-saved.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	_ = doc.Close()
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.Blocks()[0].Text; got != " spaced " {
		t.Fatalf("saved text = %q, want boundary spaces preserved", got)
	}
}

func TestReplaceParagraphRejectsInlineTab(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tab.docx")
	writeFixture(t, path, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>left</w:t><w:tab/><w:t>right</w:t></w:r></w:p></w:body></w:document>`)
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	if err := doc.ReplaceParagraph(0, "replacement"); err == nil {
		t.Fatal("expected paragraph with inline tab to be read-only")
	}
}

func TestOfficeDocxSamplePreviewEditRoundTrip(t *testing.T) {
	dir := os.Getenv("GLANCE_OFFICE_TEST_DIR")
	if dir == "" {
		t.Skip("set GLANCE_OFFICE_TEST_DIR to the local office sample directory")
	}
	source := filepath.Join(dir, "测试word文档.docx")
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Open(source)
	if err != nil {
		t.Fatal(err)
	}
	blocks := doc.Blocks()
	if len(blocks) == 0 || strings.TrimSpace(blocks[0].Text) == "" {
		t.Fatalf("sample has no visible paragraph: %#v", blocks)
	}
	if err := doc.ReplaceParagraph(0, blocks[0].Text+" 已修改"); err != nil {
		t.Fatalf("edit sample paragraph: %v", err)
	}
	output := filepath.Join(t.TempDir(), "edited.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	if err := doc.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("editing a copy modified the source sample")
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.Blocks()[0].Text; got != blocks[0].Text+" 已修改" {
		t.Fatalf("saved sample paragraph = %q", got)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	assertSameNonDocumentParts(t, source, output)
}

func assertSameNonDocumentParts(t *testing.T, originalPath, savedPath string) {
	t.Helper()
	original, err := zip.OpenReader(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	saved, err := zip.OpenReader(savedPath)
	if err != nil {
		t.Fatal(err)
	}
	defer saved.Close()
	parts := make(map[string][]byte, len(saved.File))
	for _, file := range saved.File {
		r, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(r)
		closeErr := r.Close()
		if readErr != nil && readErr.Error() != "EOF" {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		parts[file.Name] = data
	}
	for _, file := range original.File {
		if file.Name == "word/document.xml" {
			continue
		}
		r, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		want, readErr := io.ReadAll(r)
		closeErr := r.Close()
		if readErr != nil && readErr.Error() != "EOF" {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		if got, ok := parts[file.Name]; !ok || !bytes.Equal(got, want) {
			t.Errorf("DOCX package part %q changed during paragraph edit", file.Name)
		}
	}
}
