package docx

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"

	"glance/internal/document"
)

const word2010Namespace = "http://schemas.microsoft.com/office/word/2010/wordml"

func (d *Document) SplitParagraphAt(runID, value string, selectionStart, selectionEnd int) (string, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return "", "", err
	}
	target, ok := d.wordTextTargets[runID]
	if !ok || target.text == nil || target.run == nil || target.paragraph == nil {
		return "", "", fmt.Errorf("Word text run %q is not editable", runID)
	}
	runes := []rune(value)
	if selectionStart < 0 || selectionEnd < selectionStart || selectionEnd > len(runes) {
		return "", "", fmt.Errorf("invalid Word text selection %d:%d for %d characters", selectionStart, selectionEnd, len(runes))
	}
	leftText, rightText := string(runes[:selectionStart]), string(runes[selectionEnd:])
	data := d.parts[d.documentIdx].data
	if target.text.text != value {
		updated, err := replaceWordTextNode(data, target.text, value)
		if err != nil {
			return "", "", err
		}
		parts := append([]part(nil), d.parts...)
		parts[d.documentIdx].data = updated
		projection, err := projectWordDocument(parts, d.documentIdx)
		if err != nil {
			return "", "", fmt.Errorf("project DOCX text before paragraph split: %w", err)
		}
		target, ok = projection.textTargets[runID]
		if !ok || target.text == nil || target.run == nil || target.paragraph == nil {
			return "", "", fmt.Errorf("Word text run %q became unavailable before paragraph split", runID)
		}
		data = updated
	}
	leftParagraph, rightParagraph, err := splitWordParagraphXML(data, target.paragraph, target.run, target.text, leftText, rightText)
	if err != nil {
		return "", "", err
	}
	newParagraphStart := target.paragraph.start + len(leftParagraph)
	patched, err := applyXMLReplacements(data, []xmlReplacement{{start: target.paragraph.start, end: target.paragraph.end, value: append(leftParagraph, rightParagraph...)}})
	if err != nil {
		return "", "", fmt.Errorf("split DOCX paragraph: %w", err)
	}
	parts := append([]part(nil), d.parts...)
	parts[d.documentIdx].data = patched
	projection, err := projectWordDocument(parts, d.documentIdx)
	if err != nil {
		return "", "", fmt.Errorf("project DOCX after paragraph split: %w", err)
	}
	paragraphID := ""
	for id, paragraph := range projection.paragraphTargets {
		if paragraph.start == newParagraphStart {
			paragraphID = id
			break
		}
	}
	if paragraphID == "" {
		return "", "", errors.New("split DOCX paragraph is missing from the body projection")
	}
	newRunID, firstTextOffset := "", int(^uint(0)>>1)
	for id, run := range projection.textTargets {
		if run.paragraphID == paragraphID && run.text != nil && run.text.start < firstTextOffset {
			newRunID, firstTextOffset = id, run.text.start
		}
	}
	if newRunID == "" {
		return "", "", errors.New("split DOCX paragraph has no editable text run")
	}
	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return "", "", err
	}
	return paragraphID, newRunID, nil
}

func splitWordParagraphXML(data []byte, paragraph, targetRun, targetText *wordXMLNode, leftText, rightText string) ([]byte, []byte, error) {
	if paragraph == nil || targetRun == nil || targetText == nil {
		return nil, nil, errors.New("DOCX paragraph split target is incomplete")
	}
	properties := firstChild(paragraph, "pPr")
	if properties != nil && containsNode(properties, "sectPr") {
		return nil, nil, errors.New("cannot split a paragraph that contains section properties")
	}
	var leftContent, rightContent bytes.Buffer
	cursor := paragraph.contentStart
	found := false
	for _, child := range paragraph.children {
		gap := data[cursor:child.start]
		switch {
		case child.name.Local == "pPr":
			if targetRun.start < child.end {
				return nil, nil, errors.New("cannot split inside paragraph properties")
			}
			leftContent.Write(gap)
			leftContent.Write(data[child.start:child.end])
			rightContent.Write(gap)
			rightContent.Write(data[child.start:child.end])
		case nodeContains(child, targetRun):
			leftInline, rightInline, err := splitWordInlineContainer(data, child, targetRun, targetText, leftText, rightText)
			if err != nil {
				return nil, nil, err
			}
			leftContent.Write(gap)
			leftContent.Write(leftInline)
			rightContent.Write(gap)
			rightContent.Write(rightInline)
			found = true
		case child.end <= targetRun.start:
			leftContent.Write(gap)
			leftContent.Write(data[child.start:child.end])
		case child.start >= targetRun.end:
			rightContent.Write(gap)
			rightContent.Write(data[child.start:child.end])
		default:
			return nil, nil, fmt.Errorf("cannot safely split DOCX paragraph around %s", child.name.Local)
		}
		cursor = child.end
	}
	if !found {
		return nil, nil, errors.New("Word text run is not contained in its projected paragraph")
	}
	rightContent.Write(data[cursor:paragraph.contentEnd])

	leftOpening := append([]byte(nil), data[paragraph.start:paragraph.openEnd]...)
	rightOpening, err := removeWordParagraphIdentityAttributes(leftOpening, paragraph)
	if err != nil {
		return nil, nil, err
	}
	closing := data[paragraph.contentEnd:paragraph.end]
	left := make([]byte, 0, len(leftOpening)+leftContent.Len()+len(closing))
	left = append(left, leftOpening...)
	left = append(left, leftContent.Bytes()...)
	left = append(left, closing...)
	right := make([]byte, 0, len(rightOpening)+rightContent.Len()+len(closing))
	right = append(right, rightOpening...)
	right = append(right, rightContent.Bytes()...)
	right = append(right, closing...)
	return left, right, nil
}

func splitWordInlineContainer(data []byte, container, targetRun, targetText *wordXMLNode, leftText, rightText string) ([]byte, []byte, error) {
	if container == targetRun {
		return splitWordRunXML(data, container, targetText, leftText, rightText)
	}
	if container.name.Local != "hyperlink" {
		return nil, nil, fmt.Errorf("cannot safely split text inside DOCX %s", container.name.Local)
	}
	var leftContent, rightContent bytes.Buffer
	cursor := container.contentStart
	found := false
	for _, child := range container.children {
		gap := data[cursor:child.start]
		switch {
		case nodeContains(child, targetRun):
			leftRun, rightRun, err := splitWordInlineContainer(data, child, targetRun, targetText, leftText, rightText)
			if err != nil {
				return nil, nil, err
			}
			leftContent.Write(gap)
			leftContent.Write(leftRun)
			rightContent.Write(gap)
			rightContent.Write(rightRun)
			found = true
		case child.end <= targetRun.start:
			leftContent.Write(gap)
			leftContent.Write(data[child.start:child.end])
		case child.start >= targetRun.end:
			rightContent.Write(gap)
			rightContent.Write(data[child.start:child.end])
		default:
			return nil, nil, fmt.Errorf("cannot safely split DOCX hyperlink around %s", child.name.Local)
		}
		cursor = child.end
	}
	if !found {
		return nil, nil, errors.New("Word text run is not contained in its projected hyperlink")
	}
	rightContent.Write(data[cursor:container.contentEnd])
	opening := data[container.start:container.openEnd]
	closing := data[container.contentEnd:container.end]
	left := append(append(append([]byte(nil), opening...), leftContent.Bytes()...), closing...)
	right := append(append(append([]byte(nil), opening...), rightContent.Bytes()...), closing...)
	return left, right, nil
}

func splitWordRunXML(data []byte, run, targetText *wordXMLNode, leftText, rightText string) ([]byte, []byte, error) {
	var properties, leftContent, rightContent bytes.Buffer
	cursor := run.contentStart
	found := false
	for _, child := range run.children {
		gap := data[cursor:child.start]
		if child.name.Local == "rPr" {
			if found {
				return nil, nil, errors.New("cannot split a DOCX run with text properties after its text")
			}
			properties.Write(gap)
			properties.Write(data[child.start:child.end])
			cursor = child.end
			continue
		}
		switch {
		case child == targetText:
			prefix, suffix, err := splitWordTextNodeXML(data, child, leftText, rightText)
			if err != nil {
				return nil, nil, err
			}
			leftContent.Write(gap)
			leftContent.Write(prefix)
			rightContent.Write(gap)
			rightContent.Write(suffix)
			found = true
		case child.end <= targetText.start:
			leftContent.Write(gap)
			leftContent.Write(data[child.start:child.end])
		case child.start >= targetText.end:
			rightContent.Write(gap)
			rightContent.Write(data[child.start:child.end])
		default:
			return nil, nil, fmt.Errorf("cannot safely split DOCX run around %s", child.name.Local)
		}
		cursor = child.end
	}
	if !found {
		return nil, nil, errors.New("Word text node is not contained in its projected run")
	}
	rightContent.Write(data[cursor:run.contentEnd])
	opening := data[run.start:run.openEnd]
	closing := data[run.contentEnd:run.end]
	left := make([]byte, 0, len(opening)+properties.Len()+leftContent.Len()+len(closing))
	left = append(left, opening...)
	left = append(left, properties.Bytes()...)
	left = append(left, leftContent.Bytes()...)
	left = append(left, closing...)
	right := make([]byte, 0, len(opening)+properties.Len()+rightContent.Len()+len(closing))
	right = append(right, opening...)
	right = append(right, properties.Bytes()...)
	right = append(right, rightContent.Bytes()...)
	right = append(right, closing...)
	return left, right, nil
}

func splitWordTextNodeXML(data []byte, node *wordXMLNode, leftText, rightText string) ([]byte, []byte, error) {
	left, err := wordTextNodeXML(data, node, leftText)
	if err != nil {
		return nil, nil, err
	}
	right, err := wordTextNodeXML(data, node, rightText)
	return left, right, err
}

func wordTextNodeXML(data []byte, node *wordXMLNode, value string) ([]byte, error) {
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
	result := make([]byte, 0, len(opening)+escaped.Len()+len(node.rawName)+3)
	result = append(result, opening...)
	result = append(result, escaped.Bytes()...)
	result = append(result, []byte("</"+node.rawName+">")...)
	return result, nil
}

func removeWordParagraphIdentityAttributes(opening []byte, paragraph *wordXMLNode) ([]byte, error) {
	attributes, _, _, err := scanRawXMLAttributes(opening)
	if err != nil {
		return nil, err
	}
	rawCounts := make(map[string]int)
	knownWordIDs := make(map[string]int)
	for _, attribute := range attributes {
		if attribute.local == "paraId" || attribute.local == "textId" {
			rawCounts[attribute.local]++
		}
	}
	for _, attribute := range paragraph.attrs {
		if attribute.Name.Space == word2010Namespace && (attribute.Name.Local == "paraId" || attribute.Name.Local == "textId") {
			knownWordIDs[attribute.Name.Local]++
		}
	}
	var removals []xmlReplacement
	for _, attribute := range attributes {
		if attribute.local != "paraId" && attribute.local != "textId" {
			continue
		}
		if rawCounts[attribute.local] != 1 || knownWordIDs[attribute.local] != 1 {
			return nil, fmt.Errorf("cannot safely split paragraph with ambiguous %s attribute namespace", attribute.name)
		}
		removals = append(removals, xmlReplacement{start: attribute.start, end: attribute.end})
	}
	return applyXMLReplacements(opening, removals)
}

func nodeContains(parent, child *wordXMLNode) bool {
	return parent != nil && child != nil && parent.start <= child.start && child.end <= parent.end
}

var _ document.WordParagraphEditor = (*Document)(nil)
