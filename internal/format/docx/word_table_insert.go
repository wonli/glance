package docx

import (
	"fmt"
	"strings"

	"glance/internal/document"
)

var _ document.WordTableInsertEditor = (*Document)(nil)

// InsertTable inserts a basic grid table with the given rows and columns into the Word document.
// If afterParagraphID is specified, the table is inserted immediately following that paragraph;
// otherwise it is placed before the body's trailing section properties.
func (d *Document) InsertTable(afterParagraphID string, rows, cols int) (string, error) {
	if rows < 1 || cols < 1 {
		return "", fmt.Errorf("table must have at least 1 row and 1 column, got %dx%d", rows, cols)
	}
	if rows > 100 || cols > 50 {
		return "", fmt.Errorf("table size %dx%d exceeds maximum supported limits (100x50)", rows, cols)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return "", err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return "", fmt.Errorf("DOCX document part is unavailable")
	}

	data := d.parts[d.documentIdx].data
	root, err := parseWordXML(data)
	if err != nil {
		return "", fmt.Errorf("parse DOCX document before table insertion: %w", err)
	}
	body := firstChild(root, "body")
	if body == nil {
		return "", fmt.Errorf("DOCX document has no body element")
	}

	prefix := elementPrefix(root.rawName)
	if prefix == "" {
		prefix = "w"
	}

	// Calculate insertion offset
	insertAt := body.contentEnd
	for _, child := range body.children {
		if child.name.Local == "sectPr" {
			insertAt = child.start
			break
		}
	}
	if afterParagraphID != "" {
		if target, ok := d.wordParagraphTargets[afterParagraphID]; ok && target != nil {
			insertAt = target.end
		}
	}

	// Compute cell and column widths in dxa (~8640 dxa = 6 inches)
	colWidthDXA := 8640 / cols
	if colWidthDXA < 720 {
		colWidthDXA = 720
	}

	var b strings.Builder
	b.WriteString("<" + prefix + ":tbl>")
	b.WriteString("<" + prefix + ":tblPr>")
	b.WriteString("<" + prefix + ":tblW " + prefix + ":w=\"0\" " + prefix + ":type=\"auto\"/>")
	b.WriteString("<" + prefix + ":tblBorders>")
	for _, border := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		b.WriteString(fmt.Sprintf("<%s:%s %s:val=\"single\" %s:sz=\"4\" %s:space=\"0\" %s:color=\"auto\"/>",
			prefix, border, prefix, prefix, prefix, prefix))
	}
	b.WriteString("</" + prefix + ":tblBorders>")
	b.WriteString("</" + prefix + ":tblPr>")

	b.WriteString("<" + prefix + ":tblGrid>")
	for c := 0; c < cols; c++ {
		b.WriteString(fmt.Sprintf("<%s:gridCol %s:w=\"%d\"/>", prefix, prefix, colWidthDXA))
	}
	b.WriteString("</" + prefix + ":tblGrid>")

	for r := 0; r < rows; r++ {
		b.WriteString("<" + prefix + ":tr>")
		for c := 0; c < cols; c++ {
			b.WriteString("<" + prefix + ":tc>")
			b.WriteString(fmt.Sprintf("<%s:tcPr><%s:tcW %s:w=\"%d\" %s:type=\"dxa\"/></%s:tcPr>",
				prefix, prefix, prefix, colWidthDXA, prefix, prefix))
			b.WriteString("<" + prefix + ":p><" + prefix + ":r><" + prefix + ":t></" + prefix + ":t></" + prefix + ":r></" + prefix + ":p>")
			b.WriteString("</" + prefix + ":tc>")
		}
		b.WriteString("</" + prefix + ":tr>")
	}
	b.WriteString("</" + prefix + ":tbl>")

	tableMarkup := []byte(b.String())
	patched, err := applyXMLReplacements(data, []xmlReplacement{{start: insertAt, end: insertAt, value: tableMarkup}})
	if err != nil {
		return "", fmt.Errorf("insert DOCX table XML: %w", err)
	}

	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return "", err
	}

	// Locate the inserted table block in projection
	for _, block := range d.wordBlocks {
		if block.Kind == document.WordTableBlock && block.Table != nil {
			if len(block.Table.Rows) == rows {
				return block.ID, nil
			}
		}
	}

	return "", fmt.Errorf("inserted table block missing in projected document")
}
