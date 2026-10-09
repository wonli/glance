package docx

import (
	"glance/internal/document"
	"path/filepath"
	"strings"
	"testing"
)

func TestDrawingShapesCorpusFlattensNestedTableAndFallsBackForPercentWidths(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "office-corpus", "docx", "drawing-shapes.docx")
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	foundNestedText := false
	percentageFallbacks := 0
	var inspectTable func(table *document.WordTable)
	inspectTable = func(table *document.WordTable) {
		for _, row := range table.Rows {
			for _, cell := range row.Cells {
				if cell.WidthFallback {
					percentageFallbacks++
				}
				for _, paragraph := range cell.Paragraphs {
					var text strings.Builder
					for _, inline := range paragraph.Inlines {
						if inline.Text != nil {
							text.WriteString(inline.Text.Text)
						}
						if inline.Unsupported != nil {
							t.Errorf("nested table emitted unsupported marker: %s", inline.Unsupported.Description)
						}
					}
					foundNestedText = foundNestedText || strings.Contains(text.String(), "Ведомости и МАЗАЙ")
					if paragraph.PreviewWarning == "" && strings.Contains(text.String(), "Ведомости") {
						t.Error("flattened nested-table text lacks its read-only preview warning")
					}
				}
			}
		}
	}
	for _, block := range doc.WordBlocks() {
		if block.Table != nil {
			inspectTable(block.Table)
		}
	}
	if !foundNestedText {
		t.Fatal("nested table text was not projected in source order")
	}
	if percentageFallbacks == 0 {
		t.Fatal("percentage-width cell was not marked for grid-based preview")
	}
	for _, unsupported := range doc.WordLayoutData().Unsupported {
		if unsupported.Code == "nested-table" || unsupported.Code == "table-percentage-width" {
			t.Errorf("visible table fallback still blocks the document: %+v", unsupported)
		}
	}
}

func TestLongTableCorpusProjectsAlternateContentTextboxText(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "office-corpus", "docx", "long-table.docx")
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	blocks := doc.WordBlocks()
	if len(blocks) == 0 || blocks[0].Paragraph == nil {
		t.Fatalf("first block = %+v; want the fixture's graphic paragraph", blocks)
	}
	var text strings.Builder
	for _, inline := range blocks[0].Paragraph.Inlines {
		if inline.Text != nil {
			text.WriteString(inline.Text.Text)
		}
		if inline.Unsupported != nil {
			t.Errorf("AlternateContent emitted unsupported inline: %s", inline.Unsupported.Description)
		}
	}
	if !strings.Contains(text.String(), "asdf") || blocks[0].Paragraph.PreviewWarning == "" {
		t.Fatalf("AlternateContent fallback text/warning = %q / %q; want readable text and a warning", text.String(), blocks[0].Paragraph.PreviewWarning)
	}
	for _, inline := range blocks[0].Paragraph.Inlines {
		if inline.Text != nil && strings.Contains(inline.Text.Text, "asdf") && inline.Text.Editable {
			t.Error("AlternateContent fallback text must be read-only")
		}
	}
}
