package docx

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
)

func TestDOCXCorpusFootnoteAndEndnoteReferencesResolveToReadOnlyText(t *testing.T) {
	cases := []struct {
		name        string
		referenceID string
		noteText    string
	}{
		{name: "footnotes", referenceID: "1", noteText: "snoska"},
		{name: "endnotes", referenceID: "2", noteText: "XXX"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("..", "..", "..", "testdata", "office-corpus", "docx", tc.name+".docx")
			doc, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer doc.Close()
			before, err := doc.Snapshot()
			if err != nil {
				t.Fatal(err)
			}

			foundReference, foundNote := false, false
			for _, block := range doc.WordBlocks() {
				if block.Paragraph == nil {
					continue
				}
				for _, inline := range block.Paragraph.Inlines {
					if inline.Unsupported != nil {
						t.Errorf("unsupported inline %q: %s", inline.Unsupported.ID, inline.Unsupported.Description)
					}
					if inline.Text == nil {
						continue
					}
					if inline.Text.Text == tc.referenceID && !inline.Text.Editable {
						foundReference = true
					}
					if strings.Contains(inline.Text.Text, tc.noteText) {
						foundNote = true
						if inline.Text.Editable {
							t.Errorf("note text %q must remain read-only", tc.noteText)
						}
					}
				}
			}
			if !foundReference {
				t.Errorf("note reference %q was not projected as read-only text", tc.referenceID)
			}
			if !foundNote {
				t.Errorf("note text %q was not projected", tc.noteText)
			}
			after, err := doc.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) || doc.Dirty() {
				t.Error("read-only note projection changed the package or dirty state")
			}
		})
	}
}

func TestWordProjectionKeepsUnsupportedInlineKindsForOtherContent(t *testing.T) {
	doc := projectionFixture(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:sym w:char="F041"/></w:r></w:p></w:body></w:document>`, "", nil)
	defer doc.Close()
	inline := doc.WordBlocks()[0].Paragraph.Inlines[0]
	if inline.Kind != document.WordUnsupportedInline || inline.Unsupported == nil {
		t.Fatalf("symbol inline = %#v; want a located unsupported marker", inline)
	}
}
