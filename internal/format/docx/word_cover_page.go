package docx

import (
	"fmt"
	"strings"

	"glance/internal/document"
)

var _ document.WordCoverPageEditor = (*Document)(nil)

// InsertCoverPage inserts a standard cover page with title, subtitle, author, and date
// at the very beginning of the document body, separated from subsequent pages by a page break.
func (d *Document) InsertCoverPage(title, subtitle, author string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return fmt.Errorf("DOCX document part is unavailable")
	}

	title = strings.TrimSpace(title)
	if title == "" {
		title = "文档标题"
	}
	subtitle = strings.TrimSpace(subtitle)
	if subtitle == "" {
		subtitle = "文档副标题"
	}
	author = strings.TrimSpace(author)
	if author == "" {
		author = "作者"
	}

	data := d.parts[d.documentIdx].data
	root, err := parseWordXML(data)
	if err != nil {
		return fmt.Errorf("parse DOCX document before cover insertion: %w", err)
	}
	body := firstChild(root, "body")
	if body == nil {
		return fmt.Errorf("DOCX document has no body element")
	}

	prefix := elementPrefix(root.rawName)
	if prefix == "" {
		prefix = "w"
	}

	insertAt := body.contentStart
	if len(body.children) > 0 {
		insertAt = body.children[0].start
	}

	var b strings.Builder
	// Top spacing paragraph
	b.WriteString("<" + prefix + ":p><" + prefix + ":pPr><" + prefix + ":spacing " + prefix + ":before=\"2400\" " + prefix + ":after=\"240\"/></" + prefix + ":pPr></" + prefix + ":p>")
	// Title paragraph (bold, 28pt)
	b.WriteString("<" + prefix + ":p><" + prefix + ":pPr><" + prefix + ":spacing " + prefix + ":before=\"0\" " + prefix + ":after=\"480\"/></" + prefix + ":pPr><" + prefix + ":r><" + prefix + ":rPr><" + prefix + ":b/><" + prefix + ":sz " + prefix + ":val=\"56\"/><" + prefix + ":szCs " + prefix + ":val=\"56\"/></" + prefix + ":rPr><" + prefix + ":t>" + escapeXMLString(title) + "</" + prefix + ":t></" + prefix + ":r></" + prefix + ":p>")
	// Subtitle paragraph (italic, 14pt, color 666666)
	b.WriteString("<" + prefix + ":p><" + prefix + ":pPr><" + prefix + ":spacing " + prefix + ":before=\"0\" " + prefix + ":after=\"1440\"/></" + prefix + ":pPr><" + prefix + ":r><" + prefix + ":rPr><" + prefix + ":i/><" + prefix + ":color " + prefix + ":val=\"666666\"/><" + prefix + ":sz " + prefix + ":val=\"28\"/><" + prefix + ":szCs " + prefix + ":val=\"28\"/></" + prefix + ":rPr><" + prefix + ":t>" + escapeXMLString(subtitle) + "</" + prefix + ":t></" + prefix + ":r></" + prefix + ":p>")
	// Author paragraph (11pt, color 888888)
	b.WriteString("<" + prefix + ":p><" + prefix + ":pPr><" + prefix + ":spacing " + prefix + ":before=\"2880\" " + prefix + ":after=\"240\"/></" + prefix + ":pPr><" + prefix + ":r><" + prefix + ":rPr><" + prefix + ":color " + prefix + ":val=\"888888\"/><" + prefix + ":sz " + prefix + ":val=\"22\"/><" + prefix + ":szCs " + prefix + ":val=\"22\"/></" + prefix + ":rPr><" + prefix + ":t>" + escapeXMLString(author) + "</" + prefix + ":t></" + prefix + ":r></" + prefix + ":p>")
	// Page break paragraph
	b.WriteString("<" + prefix + ":p><" + prefix + ":r><" + prefix + ":br " + prefix + ":type=\"page\"/></" + prefix + ":r></" + prefix + ":p>")

	coverMarkup := []byte(b.String())
	patched, err := applyXMLReplacements(data, []xmlReplacement{{start: insertAt, end: insertAt, value: coverMarkup}})
	if err != nil {
		return fmt.Errorf("insert DOCX cover markup: %w", err)
	}

	return d.applyWordDocumentXMLLocked(patched)
}
