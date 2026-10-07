package docx

import (
	"errors"
	"fmt"
	"strings"

	"glance/internal/document"
)

var _ document.WordTextBoxEditor = (*Document)(nil)

// InsertTextBox inserts a stylized callout text box into the Word document body.
func (d *Document) InsertTextBox(afterParagraphID, text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return errors.New("DOCX document part is unavailable")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		text = "请输入文本框内容..."
	}

	data := d.parts[d.documentIdx].data
	root, err := parseWordXML(data)
	if err != nil {
		return fmt.Errorf("parse DOCX document before text box insertion: %w", err)
	}
	body := firstChild(root, "body")
	if body == nil {
		return errors.New("DOCX document has no body element")
	}

	prefix := elementPrefix(root.rawName)
	if prefix == "" {
		prefix = "w"
	}

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

	var b strings.Builder
	b.WriteString("<" + prefix + ":tbl>")
	b.WriteString("<" + prefix + ":tblPr>")
	b.WriteString("<" + prefix + ":tblW " + prefix + ":w=\"8640\" " + prefix + ":type=\"dxa\"/>")
	b.WriteString("<" + prefix + ":tblBorders>")
	b.WriteString("<" + prefix + ":top " + prefix + ":val=\"none\"/>")
	b.WriteString("<" + prefix + ":left " + prefix + ":val=\"single\" " + prefix + ":sz=\"24\" " + prefix + ":space=\"0\" " + prefix + ":color=\"2B579A\"/>")
	b.WriteString("<" + prefix + ":bottom " + prefix + ":val=\"none\"/>")
	b.WriteString("<" + prefix + ":right " + prefix + ":val=\"none\"/>")
	b.WriteString("</" + prefix + ":tblBorders>")
	b.WriteString("<" + prefix + ":shd " + prefix + ":val=\"clear\" " + prefix + ":color=\"auto\" " + prefix + ":fill=\"F3F6F9\"/>")
	b.WriteString("</" + prefix + ":tblPr>")
	b.WriteString("<" + prefix + ":tblGrid><" + prefix + ":gridCol " + prefix + ":w=\"8640\"/></" + prefix + ":tblGrid>")
	b.WriteString("<" + prefix + ":tr><" + prefix + ":tc>")
	b.WriteString("<" + prefix + ":tcPr><" + prefix + ":tcW " + prefix + ":w=\"8640\" " + prefix + ":type=\"dxa\"/><" + prefix + ":tcMar><" + prefix + ":top " + prefix + ":w=\"180\" " + prefix + ":type=\"dxa\"/><" + prefix + ":left " + prefix + ":w=\"240\" " + prefix + ":type=\"dxa\"/><" + prefix + ":bottom " + prefix + ":w=\"180\" " + prefix + ":type=\"dxa\"/><" + prefix + ":right " + prefix + ":w=\"240\" " + prefix + ":type=\"dxa\"/></" + prefix + ":tcMar></" + prefix + ":tcPr>")
	b.WriteString("<" + prefix + ":p><" + prefix + ":r><" + prefix + ":rPr><" + prefix + ":i/><" + prefix + ":color " + prefix + ":val=\"333333\"/></" + prefix + ":rPr><" + prefix + ":t>" + escapeXMLString(text) + "</" + prefix + ":t></" + prefix + ":r></" + prefix + ":p>")
	b.WriteString("</" + prefix + ":tc></" + prefix + ":tr>")
	b.WriteString("</" + prefix + ":tbl>")
	b.WriteString("<" + prefix + ":p/>")

	boxMarkup := []byte(b.String())
	patched, err := applyXMLReplacements(data, []xmlReplacement{
		{start: insertAt, end: insertAt, value: boxMarkup},
	})
	if err != nil {
		return fmt.Errorf("insert text box XML: %w", err)
	}

	return d.applyWordDocumentXMLLocked(patched)
}
