package docx

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"glance/internal/document"
)

var _ document.WordBodyTextEditor = (*Document)(nil)

// AppendBodyTextParagraph adds one plain-text body paragraph before section
// properties and returns the identities assigned by the neutral projection.
func (d *Document) AppendBodyTextParagraph(text string) (string, string, error) {
	if strings.ContainsAny(text, "\r\n") {
		return "", "", errors.New("Word paragraph text must not contain line breaks")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return "", "", err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return "", "", errors.New("DOCX document part is unavailable")
	}
	data := d.parts[d.documentIdx].data
	root, err := parseWordXML(data)
	if err != nil {
		return "", "", fmt.Errorf("parse DOCX body before paragraph append: %w", err)
	}
	body := firstChild(root, "body")
	if body == nil {
		return "", "", errors.New("DOCX document has no body")
	}
	insertAt := body.contentEnd
	for _, child := range body.children {
		if child.name.Local == "sectPr" {
			insertAt = child.start
			break
		}
	}

	beforeRunIDs := wordTextRunIDs(d.wordBlocks)
	prefix := elementPrefix(body.rawName)
	textMarkup, err := wordTextElementMarkup(prefix, text)
	if err != nil {
		return "", "", fmt.Errorf("encode Word paragraph text: %w", err)
	}
	run := wrapWordElement(prefix, "r", textMarkup)
	paragraph := wrapWordElement(prefix, "p", run)
	patched, err := applyXMLReplacements(data, []xmlReplacement{{start: insertAt, end: insertAt, value: paragraph}})
	if err != nil {
		return "", "", fmt.Errorf("append DOCX body paragraph: %w", err)
	}
	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return "", "", err
	}
	for _, block := range d.wordBlocks {
		if block.Kind != document.WordParagraphBlock || block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text == nil || inline.Text.Text != text {
				continue
			}
			if _, existed := beforeRunIDs[inline.Text.ID]; existed {
				continue
			}
			return block.Paragraph.ID, inline.Text.ID, nil
		}
	}
	return "", "", errors.New("appended DOCX paragraph is missing from the document projection")
}

func wordTextElementMarkup(prefix, text string) ([]byte, error) {
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(text)); err != nil {
		return nil, err
	}
	name := qualifiedWordName(prefix, "t")
	opening := []byte("<" + name + ">")
	if needsWordSpacePreserve(text) {
		var err error
		opening, err = setRawXMLAttribute(opening, "xml", "space", "preserve")
		if err != nil {
			return nil, err
		}
	}
	markup := make([]byte, 0, len(opening)+escaped.Len()+len(name)+3)
	markup = append(markup, opening...)
	markup = append(markup, escaped.Bytes()...)
	markup = append(markup, []byte("</"+name+">")...)
	return markup, nil
}

func wordTextRunIDs(blocks []document.WordBlock) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, block := range blocks {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text != nil {
				ids[inline.Text.ID] = struct{}{}
			}
		}
	}
	return ids
}
