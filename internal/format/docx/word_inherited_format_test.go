package docx

import (
	"bytes"
	"glance/internal/document"
	"testing"
)

func TestWordFormattingOverridesInheritedBold(t *testing.T) {
	for _, selection := range []bool{false, true} {
		t.Run(map[bool]string{false: "run", true: "selection"}[selection], func(t *testing.T) {
			d := selectionDeleteFixture(t, `<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>AB</w:t></w:r></w:p>`)
			defer d.Close()
			run := d.WordBlocks()[0].Paragraph.Inlines[0].Text
			off := false
			if selection {
				if _, err := d.SetTextSelectionStyle(run.ID, 0, run.ID, 2, document.WordTextFormatPatch{Bold: &off}); err != nil {
					t.Fatal(err)
				}
			} else if err := d.SetTextRunStyle(run.ID, document.WordTextFormatPatch{Bold: &off}); err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(d.parts[d.documentIdx].data, []byte(`w:b w:val="0"`)) {
				t.Fatal("inherited heading bold was not explicitly disabled")
			}
		})
	}
}
