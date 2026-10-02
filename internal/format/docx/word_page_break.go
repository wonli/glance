package docx

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"

	"glance/internal/document"
)

func (d *Document) InsertPageBreakAtRange(runID, value string, start, end int) (string, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return "", "", err
	}
	target, ok := d.wordTextTargets[runID]
	if !ok || target.text == nil || target.run == nil || target.paragraph == nil {
		return "", "", fmt.Errorf("Word text run %q is not editable", runID)
	}
	if !target.text.Editable {
		return "", "", fmt.Errorf("Word text run %q is read-only", runID)
	}
	if target.paragraph.parent == nil || target.paragraph.parent.name.Space != wordprocessingMLNamespace || target.paragraph.parent.name.Local != "body" {
		return "", "", errors.New("page-break insertion is supported only in a direct body paragraph")
	}
	if paragraphContainsSectionProperties(target.paragraph) {
		return "", "", errors.New("cannot insert a page break into a section-properties paragraph")
	}
	runes := []rune(value)
	if start < 0 || end < start || end > len(runes) {
		return "", "", errors.New("page-break range is outside the editable text run")
	}
	if target.text.parent != target.run || target.run.parent != target.paragraph || target.text.name.Space != wordprocessingMLNamespace || target.text.name.Local != "t" || target.run.name.Space != wordprocessingMLNamespace || target.run.name.Local != "r" {
		return "", "", errors.New("page-break target no longer matches the editable Word text run")
	}
	if len(target.text.children) != 0 {
		return "", "", errors.New("page-break target text contains nested markup")
	}
	data := d.parts[d.documentIdx].data
	if bytes.Contains(data[target.text.contentStart:target.text.contentEnd], []byte("<")) {
		return "", "", errors.New("page-break target text contains non-text XML")
	}
	properties, err := validateWordPageBreakRun(data, target.run, target.text)
	if err != nil {
		return "", "", err
	}

	before, after := string(runes[:start]), string(runes[end:])
	runOpening := append([]byte(nil), data[target.run.start:target.run.openEnd]...)
	runClosing := []byte("</" + target.run.rawName + ">")
	propertiesMarkup := []byte(nil)
	if properties != nil {
		propertiesMarkup = append([]byte(nil), data[properties.start:properties.end]...)
	}
	textMarkup, err := wordPageBreakTextMarkup(data, target.text, before)
	if err != nil {
		return "", "", err
	}
	suffixMarkup, err := wordPageBreakTextMarkup(data, target.text, after)
	if err != nil {
		return "", "", err
	}
	makeRun := func(child []byte) []byte {
		result := append([]byte(nil), runOpening...)
		result = append(result, propertiesMarkup...)
		result = append(result, child...)
		result = append(result, runClosing...)
		return result
	}
	var replacement []byte
	if before != "" {
		replacement = append(replacement, makeRun(textMarkup)...)
	}
	pageBreak := wordPageBreakMarkup(target.run.rawName)
	replacement = append(replacement, makeRun(pageBreak)...)
	replacement = append(replacement, makeRun(suffixMarkup)...)
	patched, err := applyXMLReplacements(data, []xmlReplacement{{start: target.run.start, end: target.run.end, value: replacement}})
	if err != nil {
		return "", "", fmt.Errorf("insert DOCX page break: %w", err)
	}
	paragraphID := target.paragraphID
	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return "", "", err
	}

	pageBreakSeen := false
	for _, block := range d.wordBlocks {
		if block.Paragraph == nil || block.Paragraph.ID != paragraphID {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.PageBreak != nil {
				pageBreakSeen = true
				continue
			}
			if pageBreakSeen && inline.Text != nil && inline.Text.Editable {
				return paragraphID, inline.Text.ID, nil
			}
		}
	}
	return "", "", errors.New("inserted DOCX page break has no editable caret run after it")
}

func validateWordPageBreakRun(data []byte, run, text *wordXMLNode) (*wordXMLNode, error) {
	var properties *wordXMLNode
	textCount := 0
	cursor := run.contentStart
	for index, child := range run.children {
		if !onlyXMLWhitespace(data[cursor:child.start]) {
			return nil, errors.New("page-break run contains comments or unsupported markup between its children")
		}
		if child == text {
			textCount++
		} else if child.name.Space == wordprocessingMLNamespace && child.name.Local == "rPr" && properties == nil && index == 0 {
			properties = child
		} else {
			return nil, errors.New("page-break insertion requires a text-only run without unsupported children")
		}
		cursor = child.end
	}
	if !onlyXMLWhitespace(data[cursor:run.contentEnd]) {
		return nil, errors.New("page-break run contains comments or unsupported trailing markup")
	}
	if textCount != 1 {
		return nil, errors.New("page-break insertion requires exactly one text element in the run")
	}
	return properties, nil
}

func wordPageBreakTextMarkup(data []byte, node *wordXMLNode, value string) ([]byte, error) {
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(value)); err != nil {
		return nil, err
	}
	opening := append([]byte(nil), data[node.start:node.openEnd]...)
	if needsWordSpacePreserve(value) && !rawAttributeEquals(opening, "xml", "space", "preserve") {
		var err error
		opening, err = setRawXMLAttribute(opening, "xml", "space", "preserve")
		if err != nil {
			return nil, err
		}
	}
	if node.selfClosing {
		var err error
		opening, err = expandSelfClosingTag(opening)
		if err != nil {
			return nil, err
		}
	}
	result := append([]byte(nil), opening...)
	result = append(result, escaped.Bytes()...)
	result = append(result, []byte("</"+node.rawName+">")...)
	return result, nil
}

func wordPageBreakMarkup(runName string) []byte {
	prefix := rawWordPrefix(runName)
	if prefix == "" {
		return []byte(`<w:br xmlns:w="` + wordprocessingMLNamespace + `" w:type="page"/>`)
	}
	return []byte(fmt.Sprintf(`<%s:br %s:type="page"/>`, prefix, prefix))
}

var _ document.WordPageBreakEditor = (*Document)(nil)
