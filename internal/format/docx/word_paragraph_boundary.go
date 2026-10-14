package docx

import (
	"errors"
	"fmt"

	"glance/internal/document"
)

func (d *Document) InsertParagraphAtBoundary(runID string, before bool) (string, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return "", "", err
	}
	target, ok := d.wordTextTargets[runID]
	if !ok || target.text == nil || target.run == nil || target.paragraph == nil {
		return "", "", fmt.Errorf("Word text run %q is not editable", runID)
	}
	paragraph := target.paragraph
	properties := firstChild(paragraph, "pPr")
	if properties != nil && containsNode(properties, "sectPr") {
		return "", "", errors.New("cannot insert a paragraph beside a paragraph that contains section properties")
	}

	data := d.parts[d.documentIdx].data
	prefix := elementPrefix(paragraph.rawName)
	paragraphContent := make([]byte, 0, len(data[paragraph.start:paragraph.end]))
	if properties != nil {
		paragraphContent = append(paragraphContent, data[properties.start:properties.end]...)
	}
	runContent := make([]byte, 0, 64)
	if runProperties := firstChild(target.run, "rPr"); runProperties != nil {
		runContent = append(runContent, data[runProperties.start:runProperties.end]...)
	}
	runContent = append(runContent, wrapWordElement(prefix, "t", nil)...)
	paragraphContent = append(paragraphContent, wrapWordElement(prefix, "r", runContent)...)
	newParagraph := wrapWordElement(prefix, "p", paragraphContent)

	insertAt := paragraph.end
	if before {
		insertAt = paragraph.start
	}
	patched, err := applyXMLReplacements(data, []xmlReplacement{{start: insertAt, end: insertAt, value: newParagraph}})
	if err != nil {
		return "", "", fmt.Errorf("insert DOCX paragraph at boundary: %w", err)
	}
	parts := append([]part(nil), d.parts...)
	parts[d.documentIdx].data = patched
	projection, err := projectWordDocument(parts, d.documentIdx)
	if err != nil {
		return "", "", fmt.Errorf("project DOCX after paragraph insertion: %w", err)
	}
	paragraphID := ""
	for id, candidate := range projection.paragraphTargets {
		if candidate.start == insertAt {
			paragraphID = id
			break
		}
	}
	if paragraphID == "" {
		return "", "", errors.New("inserted DOCX paragraph is missing from the body projection")
	}
	newRunID, firstTextOffset := "", int(^uint(0)>>1)
	for id, candidate := range projection.textTargets {
		if candidate.paragraphID == paragraphID && candidate.text != nil && candidate.text.start < firstTextOffset {
			newRunID, firstTextOffset = id, candidate.text.start
		}
	}
	if newRunID == "" {
		return "", "", errors.New("inserted DOCX paragraph has no editable text run")
	}
	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return "", "", err
	}
	return paragraphID, newRunID, nil
}

var _ document.WordParagraphBoundaryEditor = (*Document)(nil)
