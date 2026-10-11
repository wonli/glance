package docx

import (
	"fmt"

	"glance/internal/document"
)

func (d *Document) InsertParagraphAfter(afterParagraphID string) (string, error) {
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
		return "", fmt.Errorf("parse DOCX body before paragraph insertion: %w", err)
	}
	body := firstChild(root, "body")
	if body == nil {
		return "", fmt.Errorf("DOCX document has no body")
	}
	insertAt := body.contentEnd
	for _, child := range body.children {
		if child.name.Local == "sectPr" {
			insertAt = child.start
			break
		}
	}
	if afterParagraphID != "" {
		target, ok := d.wordParagraphTargets[afterParagraphID]
		if !ok || target == nil {
			return "", fmt.Errorf("Word paragraph %q is unavailable", afterParagraphID)
		}
		insertAt = target.end
	}
	prefix := elementPrefix(body.rawName)
	run := wrapWordElement(prefix, "r", wrapWordElement(prefix, "t", nil))
	paragraph := wrapWordElement(prefix, "p", run)
	patched, err := applyXMLReplacements(data, []xmlReplacement{{start: insertAt, end: insertAt, value: paragraph}})
	if err != nil {
		return "", fmt.Errorf("insert DOCX body paragraph: %w", err)
	}
	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return "", err
	}
	if afterParagraphID != "" {
		for index, block := range d.wordBlocks {
			if block.Kind == document.WordParagraphBlock && block.Paragraph != nil && block.Paragraph.ID == afterParagraphID {
				if index+1 < len(d.wordBlocks) && d.wordBlocks[index+1].Kind == document.WordParagraphBlock && d.wordBlocks[index+1].Paragraph != nil {
					return d.wordBlocks[index+1].Paragraph.ID, nil
				}
			}
		}
	}
	for index := len(d.wordBlocks) - 1; index >= 0; index-- {
		block := d.wordBlocks[index]
		if block.Kind == document.WordParagraphBlock && block.Paragraph != nil {
			return block.Paragraph.ID, nil
		}
	}
	return "", fmt.Errorf("inserted DOCX paragraph is missing from the body projection")
}

func (d *Document) InsertBodyParagraph() (string, error) {
	return d.InsertParagraphAfter("")
}
