package docx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
}
