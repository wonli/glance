package docx

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

func (d *Document) ReplaceTextRun(runID, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	return d.replaceTextRunLocked(runID, value)
}

func (d *Document) ReplaceTextRange(runID string, start, end int, replacement string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	target, ok := d.wordTextTargets[runID]
	if !ok || target.text == nil || target.run == nil {
		return fmt.Errorf("Word text run %q is not editable", runID)
	}
	current := []rune(target.text.text)
	if start < 0 || end < start || end > len(current) {
		return fmt.Errorf("Word text range [%d,%d) is outside run %q", start, end, runID)
	}
	updated := make([]rune, 0, len(current)-(end-start)+utf8.RuneCountInString(replacement))
	updated = append(updated, current[:start]...)
	updated = append(updated, []rune(replacement)...)
	updated = append(updated, current[end:]...)
	return d.replaceTextRunLocked(runID, string(updated))
}

func (d *Document) replaceTextRunLocked(runID, value string) error {
	target, ok := d.wordTextTargets[runID]
	if !ok || target.text == nil || target.run == nil {
		return fmt.Errorf("Word text run %q is not editable", runID)
	}
	if target.text.text == value {
		return nil
	}
	current := d.parts[d.documentIdx].data
	patched, err := replaceWordTextNode(current, target.text, value)
	if err != nil {
		return err
	}
	return d.applyWordDocumentXMLLocked(patched)
}

func (d *Document) applyWordDocumentXMLLocked(patched []byte) error {
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return errors.New("DOCX document part is unavailable")
	}
	current := d.parts[d.documentIdx].data
	if bytes.Equal(current, patched) {
		return nil
	}
	before := d.captureWordHistoryLocked("word/document.xml")
	d.parts[d.documentIdx].data = patched
	if err := d.reparse(); err != nil {
		d.parts[d.documentIdx].data = current
		if restoreErr := d.reparse(); restoreErr != nil {
			return fmt.Errorf("Word edit failed (%v) and previous content could not be restored (%v)", err, restoreErr)
		}
		return fmt.Errorf("reparse DOCX after Word edit: %w", err)
	}
	d.recordWordHistoryLocked(before)
	return nil
}

func replaceWordTextNode(data []byte, node *wordXMLNode, value string) ([]byte, error) {
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(value)); err != nil {
		return nil, err
	}
	rawOpen := append([]byte(nil), data[node.start:node.openEnd]...)
	if needsWordSpacePreserve(value) && !rawAttributeEquals(rawOpen, "xml", "space", "preserve") {
		var err error
		rawOpen, err = setRawXMLAttribute(rawOpen, "xml", "space", "preserve")
		if err != nil {
			return nil, err
		}
	}
	if node.selfClosing {
		open, err := expandSelfClosingTag(rawOpen)
		if err != nil {
			return nil, err
		}
		replacement := make([]byte, 0, len(open)+escaped.Len()+len(node.rawName)+3)
		replacement = append(replacement, open...)
		replacement = append(replacement, escaped.Bytes()...)
		replacement = append(replacement, []byte("</"+node.rawName+">")...)
		return applyXMLReplacements(data, []xmlReplacement{{start: node.start, end: node.end, value: replacement}})
	}
	changes := []xmlReplacement{{start: node.contentStart, end: node.contentEnd, value: escaped.Bytes()}}
	if !bytes.Equal(rawOpen, data[node.start:node.openEnd]) {
		changes = append(changes, xmlReplacement{start: node.start, end: node.openEnd, value: rawOpen})
	}
	return applyXMLReplacements(data, changes)
}

func needsWordSpacePreserve(value string) bool {
	return value != "" && strings.TrimSpace(value) != value
}
