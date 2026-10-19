package docx

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestWordRulerIndentsRoundTrip(t *testing.T) {
	for _, delta := range []int64{228600, -152400, 0} {
		d := projectionFixture(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:keepNext/><w:ind w:start="720" w:end="180" w:firstLineChars="200" w:hanging="120"/></w:pPr><w:r><w:t>中文正文</w:t></w:r></w:p><w:p><w:r><w:t>untouched</w:t></w:r></w:p></w:body></w:document>`, "", nil)
		p := d.WordBlocks()[0].Paragraph
		editor, ok := any(d).(interface {
			SetParagraphRulerIndents(string, int64, int64, *int64) error
		})
		if !ok {
			t.Fatal("missing atomic ruler indent editing")
		}
		if err := editor.SetParagraphRulerIndents(p.ID, -91440, 457200, &delta); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "ruler.docx")
		if err := d.SaveAs(path); err != nil {
			t.Fatal(err)
		}
		d.Close()
		reopened, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		got := reopened.WordBlocks()[0].Paragraph
		if got.LeftIndentEMU != -91440 || got.RightIndentEMU != 457200 || got.Layout.FirstLineIndentEMU-got.Layout.HangingIndentEMU != delta {
			t.Fatalf("wrong indents: %+v", got)
		}
		snapshot, _ := reopened.Snapshot()
		xml := readZipPart(t, snapshot, "word/document.xml")
		if bytes.Contains(xml, []byte("w:firstLine=")) && bytes.Contains(xml, []byte("w:hanging=")) {
			t.Fatalf("mutually exclusive OOXML indents emitted together: %s", xml)
		}
		if bytes.Contains(xml, []byte("firstLineChars")) || !bytes.Contains(xml, []byte("keepNext")) || !bytes.Contains(xml, []byte("untouched")) {
			t.Fatalf("preservation failure: %s", xml)
		}
		before := append([]byte(nil), snapshot...)
		if err := editorNoopRuler(reopened, got.ID, -91440, 457200, &delta); err != nil {
			t.Fatal(err)
		}
		after, _ := reopened.Snapshot()
		if !bytes.Equal(before, after) || reopened.Dirty() {
			t.Fatal("no-op ruler edit dirtied document")
		}
		reopened.Close()
	}
}
func editorNoopRuler(d *Document, id string, l, r int64, f *int64) error {
	return any(d).(interface {
		SetParagraphRulerIndents(string, int64, int64, *int64) error
	}).SetParagraphRulerIndents(id, l, r, f)
}
