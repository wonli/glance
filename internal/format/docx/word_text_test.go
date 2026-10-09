package docx

import (
	"archive/zip"
	"bytes"
	"glance/internal/document"
	"io"
	"os"
	"testing"
)

func textEditingFixture(t *testing.T) *Document {
	t.Helper()
	documentXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:keepNext/><w:jc w:val="left"/></w:pPr><w:r><w:rPr><w:b/><w:color w:val="AABBCC"/><w:vanish/><w:rFonts w:ascii="Aptos"/></w:rPr><w:t>bold</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t> normal</w:t></w:r></w:p>
<w:tbl><w:tr><w:tc><w:p><w:r><w:t>cell text</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
</w:body></w:document>`
	return projectionFixture(t, documentXML, "", nil)
}

func requireWordTextEditor(t *testing.T, doc *Document) document.WordTextEditor {
	t.Helper()
	editor, ok := any(doc).(document.WordTextEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose safe Word text editing")
	}
	return editor
}

type wordTextFontSizeEditor interface {
	SetTextRunFontSize(runID string, points float64) error
}

type wordTextFontFamilyEditor interface {
	SetTextRunFontFamily(runID, family string) error
}

type wordTextRangeEditor interface {
	ReplaceTextRange(runID string, start, end int, replacement string) error
}

func requireWordTextRangeEditor(t *testing.T, doc *Document) wordTextRangeEditor {
	t.Helper()
	editor, ok := any(doc).(wordTextRangeEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose safe Word text-range replacement")
	}
	return editor
}

func requireWordTextFontSizeEditor(t *testing.T, doc *Document) wordTextFontSizeEditor {
	t.Helper()
	editor, ok := any(doc).(wordTextFontSizeEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose safe Word font-size editing")
	}
	return editor
}

func requireWordTextFontFamilyEditor(t *testing.T, doc *Document) wordTextFontFamilyEditor {
	t.Helper()
	editor, ok := any(doc).(wordTextFontFamilyEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose safe Word font-family editing")
	}
	return editor
}

func requireWordHistory(t *testing.T, doc *Document) document.WordHistory {
	t.Helper()
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("DOCX adapter does not expose Word undo/redo")
	}
	return history
}

func requireWordBodyEditor(t *testing.T, doc *Document) document.WordBodyEditor {
	t.Helper()
	editor, ok := any(doc).(document.WordBodyEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose body paragraph insertion")
	}
	return editor
}

type wordParagraphSplitCapability interface {
	SplitParagraphAt(runID, value string, selectionStart, selectionEnd int) (paragraphID, newRunID string, err error)
}

func requireWordParagraphSplitter(t *testing.T, doc *Document) wordParagraphSplitCapability {
	t.Helper()
	splitter, ok := any(doc).(wordParagraphSplitCapability)
	if !ok {
		t.Fatal("DOCX adapter does not expose paragraph splitting at a text caret")
	}
	return splitter
}

func requireWordParagraphJoiner(t *testing.T, doc *Document) document.WordParagraphJoiner {
	t.Helper()
	joiner, ok := any(doc).(document.WordParagraphJoiner)
	if !ok {
		t.Fatal("DOCX adapter does not expose paragraph joining at text boundaries")
	}
	return joiner
}

func findRunByText(t *testing.T, blocks []document.WordBlock, value string) (string, string) {
	t.Helper()
	for _, block := range blocks {
		if block.Paragraph != nil {
			for _, inline := range block.Paragraph.Inlines {
				if inline.Text != nil && inline.Text.Text == value {
					return inline.Text.ID, block.Paragraph.ID
				}
			}
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					for _, paragraph := range cell.Paragraphs {
						for _, inline := range paragraph.Inlines {
							if inline.Text != nil && inline.Text.Text == value {
								return inline.Text.ID, paragraph.ID
							}
						}
					}
				}
			}
		}
	}
	t.Fatalf("text run %q not found", value)
	return "", ""
}

type wordParagraphSpacingPatcher interface {
	SetParagraphSpacing(paragraphID string, beforeTwips, afterTwips *int64) error
}

// Apply compact spacing (0 twips before, 80 twips after = 4 pt)

// Verify Undo

func assertPackagePartsEqual(t *testing.T, originalData []byte, savedPath string) {
	t.Helper()
	original := readZipParts(t, originalData)
	savedData, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatal(err)
	}
	saved := readZipParts(t, savedData)
	for name, originalPart := range original {
		if name == "word/document.xml" {
			continue
		}
		if !bytes.Equal(originalPart, saved[name]) {
			t.Errorf("untouched package part %q changed", name)
		}
	}
}

func readZipPart(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	return readZipParts(t, data)[name]
}

func readZipParts(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		part, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		parts[file.Name] = part
	}
	return parts
}

// Initially replace text with "Hello World"

// Set font color and highlight (background color)

// Verify in raw XML that <w:rPr> comes BEFORE <w:t>

// Split paragraph at caret position (e.g. index 5, between "Hello" and "World")

// Verify both paragraphs exist and retain styling

// Test that splitWordRunXML gracefully handles and heals a run where <w:rPr> was after <w:t>

// Both left and right should have <w:rPr> BEFORE <w:t>
