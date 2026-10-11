package docx

import (
	"archive/zip"
	"errors"
	"fmt"
	"strings"

	"glance/internal/document"
)

var _ document.WordHyperlinkEditor = (*Document)(nil)

// InsertHyperlink inserts or wraps a hyperlink pointing to an external URL or an internal bookmark.
func (d *Document) InsertHyperlink(runID, text, targetURL string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return errors.New("DOCX document part is unavailable")
	}

	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return errors.New("超链接地址不能为空")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		text = targetURL
	}

	data := d.parts[d.documentIdx].data
	root, err := parseWordXML(data)
	if err != nil {
		return fmt.Errorf("parse DOCX document before hyperlink insertion: %w", err)
	}

	prefix := elementPrefix(root.rawName)
	if prefix == "" {
		prefix = "w"
	}

	// Find insertion target run or fallback
	var targetRun *wordXMLNode
	if runID != "" {
		target, ok := d.wordTextTargets[runID]
		if !ok || target.run == nil {
			return fmt.Errorf("hyperlink target text run %q not found", runID)
		}
		targetRun = target.run
	} else {
		for _, t := range d.wordTextTargets {
			if t.run != nil {
				targetRun = t.run
				break
			}
		}
	}

	var startOffset, endOffset int
	if targetRun != nil {
		startOffset = targetRun.start
		endOffset = targetRun.end
	} else {
		body := firstChild(root, "body")
		if body == nil {
			return errors.New("DOCX document has no body element")
		}
		startOffset = body.contentStart
		endOffset = body.contentStart
	}

	isInternal := strings.HasPrefix(targetURL, "#")
	var linkMarkup string
	if isInternal {
		anchor := strings.TrimPrefix(targetURL, "#")
		var b strings.Builder
		b.WriteString("<" + prefix + ":hyperlink " + prefix + ":anchor=\"" + escapeXMLString(anchor) + "\">")
		b.WriteString("<" + prefix + ":r><" + prefix + ":rPr><" + prefix + ":color " + prefix + ":val=\"0563C1\"/><" + prefix + ":u " + prefix + ":val=\"single\"/></" + prefix + ":rPr>")
		b.WriteString("<" + prefix + ":t>" + escapeXMLString(text) + "</" + prefix + ":t></" + prefix + ":r></" + prefix + ":hyperlink>")
		linkMarkup = b.String()

		patched, err := applyXMLReplacements(data, []xmlReplacement{
			{start: startOffset, end: endOffset, value: []byte(linkMarkup)},
		})
		if err != nil {
			return fmt.Errorf("insert internal hyperlink XML: %w", err)
		}
		return d.applyWordDocumentXMLLocked(patched)
	}

	// External hyperlink requires a relationship in word/_rels/document.xml.rels
	relID := d.nextWordRelationshipID()
	relPart, err := d.addWordHyperlinkRelationship(relID, targetURL)
	if err != nil {
		return fmt.Errorf("create external hyperlink relationship: %w", err)
	}

	var b strings.Builder
	b.WriteString("<" + prefix + ":hyperlink r:id=\"" + escapeXMLString(relID) + "\">")
	b.WriteString("<" + prefix + ":r><" + prefix + ":rPr><" + prefix + ":color " + prefix + ":val=\"0563C1\"/><" + prefix + ":u " + prefix + ":val=\"single\"/></" + prefix + ":rPr>")
	b.WriteString("<" + prefix + ":t>" + escapeXMLString(text) + "</" + prefix + ":t></" + prefix + ":r></" + prefix + ":hyperlink>")
	linkMarkup = b.String()

	patched, err := applyXMLReplacements(data, []xmlReplacement{
		{start: startOffset, end: endOffset, value: []byte(linkMarkup)},
	})
	if err != nil {
		return fmt.Errorf("insert external hyperlink XML: %w", err)
	}

	changes := map[string]*part{
		"word/document.xml":            d.partWithData("word/document.xml", patched),
		"word/_rels/document.xml.rels": relPart,
	}
	return d.commitWordPackageChange(changes)
}

func (d *Document) addWordHyperlinkRelationship(id, target string) (*part, error) {
	const name = "word/_rels/document.xml.rels"
	partValue, exists := findPackagePart(d.parts, name)
	if !exists {
		data := []byte(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`)
		partValue = part{header: zip.FileHeader{Name: name, Method: zip.Deflate}, data: data}
	}
	root, err := parseWordXML(partValue.data)
	if err != nil {
		return nil, fmt.Errorf("parse DOCX relationships: %w", err)
	}
	child := fmt.Sprintf(`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="%s" TargetMode="External"/>`, escapeXMLString(id), escapeXMLString(target))
	patched, err := insertElementContent(partValue.data, root, []byte(child))
	if err != nil {
		return nil, err
	}
	partValue.data = patched
	return &partValue, nil
}
