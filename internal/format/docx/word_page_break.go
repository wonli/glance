package docx

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"

	"glance/internal/document"
)

func (d *Document) InsertPageBreakAtRange(runID, value string, start, end int) (string, string, error) {
	return d.InsertBreakAtRange(runID, value, start, end, document.WordBreakPage)
}

func (d *Document) InsertBreakAtRange(runID, value string, start, end int, breakType document.WordBreakType) (string, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return "", "", err
	}
	target, ok := d.wordTextTargets[runID]
	if !ok || target.text == nil || target.run == nil || target.paragraph == nil {
		return "", "", fmt.Errorf("Word text run %q is not editable", runID)
	}
	if target.paragraph.parent == nil || target.paragraph.parent.name.Space != wordprocessingMLNamespace || target.paragraph.parent.name.Local != "body" {
		return "", "", errors.New("break insertion is supported only in a direct body paragraph")
	}
	if paragraphContainsSectionProperties(target.paragraph) {
		return "", "", errors.New("cannot insert a break into a section-properties paragraph")
	}
	runes := []rune(value)
	if start < 0 || end < start || end > len(runes) {
		return "", "", errors.New("break range is outside the editable text run")
	}
	if target.text.parent != target.run || target.run.parent != target.paragraph || target.text.name.Space != wordprocessingMLNamespace || target.text.name.Local != "t" || target.run.name.Space != wordprocessingMLNamespace || target.run.name.Local != "r" {
		return "", "", errors.New("break target no longer matches the editable Word text run")
	}
	if len(target.text.children) != 0 {
		return "", "", errors.New("break target text contains nested markup")
	}
	data := d.parts[d.documentIdx].data
	if bytes.Contains(data[target.text.contentStart:target.text.contentEnd], []byte("<")) {
		return "", "", errors.New("break target text contains non-text XML")
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
	breakElement := wordBreakMarkup(target.run.rawName, breakType)
	replacement = append(replacement, makeRun(breakElement)...)
	replacement = append(replacement, makeRun(suffixMarkup)...)
	patched, err := applyXMLReplacements(data, []xmlReplacement{{start: target.run.start, end: target.run.end, value: replacement}})
	if err != nil {
		return "", "", fmt.Errorf("insert DOCX break: %w", err)
	}
	paragraphID := target.paragraphID
	virtualParts := append([]part(nil), d.parts...)
	virtualParts[d.documentIdx].data = patched
	projection, err := projectWordDocument(virtualParts, d.documentIdx)
	if err != nil {
		return "", "", fmt.Errorf("project DOCX after break insertion: %w", err)
	}
	breakSeen := false
	caretRunID := ""
	for _, block := range projection.blocks {
		if block.Paragraph == nil || block.Paragraph.ID != paragraphID {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.PageBreak != nil {
				breakSeen = true
				continue
			}
			if breakSeen && inline.Text != nil && inline.Text.Editable {
				caretRunID = inline.Text.ID
				break
			}
		}
		break
	}
	if caretRunID == "" {
		return "", "", errors.New("inserted DOCX break has no editable caret run after it")
	}
	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return "", "", err
	}
	return paragraphID, caretRunID, nil
}

func (d *Document) InsertBlankPageAtRange(runID, value string, start, end int) (string, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return "", "", err
	}
	target, ok := d.wordTextTargets[runID]
	if !ok || target.text == nil || target.run == nil || target.paragraph == nil {
		return "", "", fmt.Errorf("Word text run %q is not editable", runID)
	}
	if target.paragraph.parent == nil || target.paragraph.parent.name.Space != wordprocessingMLNamespace || target.paragraph.parent.name.Local != "body" {
		return "", "", errors.New("blank page insertion is supported only in a direct body paragraph")
	}
	if paragraphContainsSectionProperties(target.paragraph) {
		return "", "", errors.New("cannot insert a blank page into a section-properties paragraph")
	}
	runes := []rune(value)
	if start < 0 || end < start || end > len(runes) {
		return "", "", errors.New("blank page range is outside the editable text run")
	}
	if target.text.parent != target.run || target.run.parent != target.paragraph || target.text.name.Space != wordprocessingMLNamespace || target.text.name.Local != "t" || target.run.name.Space != wordprocessingMLNamespace || target.run.name.Local != "r" {
		return "", "", errors.New("blank page target no longer matches the editable Word text run")
	}
	if len(target.text.children) != 0 {
		return "", "", errors.New("blank page target text contains nested markup")
	}
	data := d.parts[d.documentIdx].data
	if bytes.Contains(data[target.text.contentStart:target.text.contentEnd], []byte("<")) {
		return "", "", errors.New("blank page target text contains non-text XML")
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
	middleMarkup, err := wordPageBreakTextMarkup(data, target.text, "")
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
	breakElement := wordPageBreakMarkup(target.run.rawName)
	replacement = append(replacement, makeRun(breakElement)...)
	replacement = append(replacement, makeRun(middleMarkup)...)
	replacement = append(replacement, makeRun(breakElement)...)
	replacement = append(replacement, makeRun(suffixMarkup)...)
	patched, err := applyXMLReplacements(data, []xmlReplacement{{start: target.run.start, end: target.run.end, value: replacement}})
	if err != nil {
		return "", "", fmt.Errorf("insert DOCX blank page: %w", err)
	}
	paragraphID := target.paragraphID
	virtualParts := append([]part(nil), d.parts...)
	virtualParts[d.documentIdx].data = patched
	projection, err := projectWordDocument(virtualParts, d.documentIdx)
	if err != nil {
		return "", "", fmt.Errorf("project DOCX after blank page insertion: %w", err)
	}
	firstBreakSeen := false
	caretRunID := ""
	for _, block := range projection.blocks {
		if block.Paragraph == nil || block.Paragraph.ID != paragraphID {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.PageBreak != nil {
				if !firstBreakSeen {
					firstBreakSeen = true
					continue
				}
			}
			if firstBreakSeen && inline.Text != nil && inline.Text.Editable {
				caretRunID = inline.Text.ID
				break
			}
		}
		break
	}
	if caretRunID == "" {
		return "", "", errors.New("inserted DOCX blank page has no editable caret run in it")
	}
	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return "", "", err
	}
	return paragraphID, caretRunID, nil
}

func validateWordPageBreakRun(data []byte, run, text *wordXMLNode) (*wordXMLNode, error) {
	var properties *wordXMLNode
	textCount := 0
	cursor := run.contentStart
	for index, child := range run.children {
		if !onlyXMLWhitespace(data[cursor:child.start]) {
			return nil, errors.New("break run contains comments or unsupported markup between its children")
		}
		if child == text {
			textCount++
		} else if child.name.Space == wordprocessingMLNamespace && child.name.Local == "rPr" && properties == nil && index == 0 {
			properties = child
		} else {
			return nil, errors.New("break insertion requires a text-only run without unsupported children")
		}
		cursor = child.end
	}
	if !onlyXMLWhitespace(data[cursor:run.contentEnd]) {
		return nil, errors.New("break run contains comments or unsupported trailing markup")
	}
	if textCount != 1 {
		return nil, errors.New("break insertion requires exactly one text element in the run")
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

func wordBreakMarkup(runName string, breakType document.WordBreakType) []byte {
	prefix := rawWordPrefix(runName)
	bt := "page"
	if breakType == document.WordBreakColumn {
		bt = "column"
	}
	if prefix == "" {
		return []byte(fmt.Sprintf(`<w:br xmlns:w="%s" w:type="%s"/>`, wordprocessingMLNamespace, bt))
	}
	return []byte(fmt.Sprintf(`<%s:br %s:type="%s"/>`, prefix, prefix, bt))
}

func wordPageBreakMarkup(runName string) []byte {
	return wordBreakMarkup(runName, document.WordBreakPage)
}

var _ document.WordPageBreakEditor = (*Document)(nil)
