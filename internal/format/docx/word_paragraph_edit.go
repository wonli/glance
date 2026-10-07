package docx

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"

	"glance/internal/document"
)

const word2010Namespace = "http://schemas.microsoft.com/office/word/2010/wordml"
const wordprocessingMLNamespace = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

func (d *Document) JoinParagraphAtBoundary(runID string, direction document.WordParagraphJoinDirection) (document.WordParagraphJoinResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return document.WordParagraphJoinResult{}, err
	}
	if direction != document.WordJoinPrevious && direction != document.WordJoinNext {
		return document.WordParagraphJoinResult{}, fmt.Errorf("invalid Word paragraph join direction %d", direction)
	}
	target, ok := d.wordTextTargets[runID]
	if !ok || target.text == nil || target.run == nil || target.paragraph == nil {
		return document.WordParagraphJoinResult{}, fmt.Errorf("Word text run %q is not editable", runID)
	}
	if err := validateWordParagraphJoinEdge(d.wordTextTargets, runID, target, direction); err != nil {
		return document.WordParagraphJoinResult{}, err
	}

	root, err := parseWordXML(d.parts[d.documentIdx].data)
	if err != nil {
		return document.WordParagraphJoinResult{}, fmt.Errorf("parse DOCX before paragraph join: %w", err)
	}
	currentParagraph := findWordXMLNode(root, target.paragraph)
	if currentParagraph == nil {
		return document.WordParagraphJoinResult{}, errors.New("Word paragraph is no longer present in the DOCX XML")
	}
	parent := findWordXMLParent(root, currentParagraph)
	if parent == nil || parent.name.Space != wordprocessingMLNamespace || (parent.name.Local != "body" && parent.name.Local != "tc") {
		return document.WordParagraphJoinResult{}, errors.New("paragraph join is only supported between direct body or table-cell paragraphs")
	}
	paragraphIndex := -1
	for index, child := range parent.children {
		if child == currentParagraph {
			paragraphIndex = index
			break
		}
	}
	if paragraphIndex < 0 {
		return document.WordParagraphJoinResult{}, errors.New("Word paragraph is not a direct child of its join container")
	}
	adjacentIndex := paragraphIndex - 1
	if direction == document.WordJoinNext {
		adjacentIndex = paragraphIndex + 1
	}
	if adjacentIndex < 0 || adjacentIndex >= len(parent.children) {
		return document.WordParagraphJoinResult{}, errors.New("there is no adjacent paragraph to join")
	}
	adjacent := parent.children[adjacentIndex]
	if adjacent.name.Space != wordprocessingMLNamespace || adjacent.name.Local != "p" || currentParagraph.name.Space != wordprocessingMLNamespace || currentParagraph.name.Local != "p" {
		return document.WordParagraphJoinResult{}, errors.New("paragraph join cannot cross a table, cell, or unsupported block boundary")
	}
	leftParagraph, rightParagraph := adjacent, currentParagraph
	if direction == document.WordJoinNext {
		leftParagraph, rightParagraph = currentParagraph, adjacent
	}
	if paragraphContainsSectionProperties(leftParagraph) || paragraphContainsSectionProperties(rightParagraph) {
		return document.WordParagraphJoinResult{}, errors.New("cannot join a paragraph that contains section properties")
	}
	if gap := d.parts[d.documentIdx].data[leftParagraph.end:rightParagraph.start]; !onlyXMLWhitespace(gap) {
		return document.WordParagraphJoinResult{}, errors.New("cannot safely join paragraphs separated by unsupported XML content")
	}
	if err := validateWordParagraphNamespaceScopes(root, leftParagraph, rightParagraph); err != nil {
		return document.WordParagraphJoinResult{}, err
	}

	leftID := ""
	for id, paragraph := range d.wordParagraphTargets {
		if sameWordXMLNode(paragraph, leftParagraph) {
			leftID = id
			break
		}
	}
	if leftID == "" {
		return document.WordParagraphJoinResult{}, errors.New("left Word paragraph is missing from the document projection")
	}
	leftModel := findWordParagraph(d.wordBlocks, leftID)
	if leftModel == nil {
		return document.WordParagraphJoinResult{}, errors.New("left Word paragraph is missing from the editable projection")
	}

	data := d.parts[d.documentIdx].data
	leftOpening, err := mergedWordParagraphOpening(data, root, leftParagraph, rightParagraph)
	if err != nil {
		return document.WordParagraphJoinResult{}, err
	}
	leftClosing := data[leftParagraph.contentEnd:leftParagraph.end]
	if leftParagraph.selfClosing {
		leftOpening, leftClosing, err = expandSelfClosingWordParagraph(leftOpening, leftParagraph.rawName)
		if err != nil {
			return document.WordParagraphJoinResult{}, err
		}
	}
	leftContent, err := wordParagraphContent(data, leftParagraph, false)
	if err != nil {
		return document.WordParagraphJoinResult{}, err
	}
	rightContent, err := wordParagraphContent(data, rightParagraph, true)
	if err != nil {
		return document.WordParagraphJoinResult{}, err
	}
	seamIndex := len(leftModel.Inlines) - 1
	needsCaretRun := seamIndex < 0 || leftModel.Inlines[seamIndex].Text == nil || !leftModel.Inlines[seamIndex].Text.Editable
	if needsCaretRun {
		seamIndex = len(leftModel.Inlines)
		prefix := elementPrefix(leftParagraph.rawName)
		caretRun := wrapWordElement(prefix, "r", wrapWordElement(prefix, "t", nil))
		leftContent = append(leftContent, caretRun...)
	}
	merged := make([]byte, 0, len(leftOpening)+len(leftContent)+len(rightContent)+len(leftClosing))
	merged = append(merged, leftOpening...)
	merged = append(merged, leftContent...)
	merged = append(merged, rightContent...)
	merged = append(merged, leftClosing...)
	patched, err := applyXMLReplacements(data, []xmlReplacement{{start: leftParagraph.start, end: rightParagraph.end, value: merged}})
	if err != nil {
		return document.WordParagraphJoinResult{}, fmt.Errorf("join DOCX paragraphs: %w", err)
	}
	parts := append([]part(nil), d.parts...)
	parts[d.documentIdx].data = patched
	projection, err := projectWordDocument(parts, d.documentIdx)
	if err != nil {
		return document.WordParagraphJoinResult{}, fmt.Errorf("project DOCX after paragraph join: %w", err)
	}
	projectedParagraph := findWordParagraph(projection.blocks, leftID)
	if projectedParagraph == nil || seamIndex < 0 || seamIndex >= len(projectedParagraph.Inlines) {
		return document.WordParagraphJoinResult{}, errors.New("joined DOCX paragraph has no projected caret seam")
	}
	seam := projectedParagraph.Inlines[seamIndex].Text
	if seam == nil || !seam.Editable {
		return document.WordParagraphJoinResult{}, errors.New("joined DOCX paragraph seam is not editable")
	}
	result := document.WordParagraphJoinResult{ParagraphID: leftID, RunID: seam.ID, Caret: len([]rune(seam.Text))}
	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return document.WordParagraphJoinResult{}, err
	}
	return result, nil
}

func validateWordParagraphNamespaceScopes(root, left, right *wordXMLNode) error {
	leftBindings := wordNamespaceBindings(root, left)
	rightBindings := wordNamespaceBindings(root, right)
	for prefix, rightBinding := range rightBindings {
		leftBinding, leftHasBinding := leftBindings[prefix]
		if leftHasBinding && leftBinding == rightBinding {
			continue
		}
		if prefix == "" || !leftHasBinding && !wordParagraphDeclaresNamespace(right, prefix) || leftHasBinding {
			return fmt.Errorf("cannot safely join paragraphs with different inherited namespace binding for %q", prefix)
		}
	}
	for prefix, leftBinding := range leftBindings {
		if _, rightHasBinding := rightBindings[prefix]; rightHasBinding {
			continue
		}
		if prefix == "" && leftBinding != "" {
			return errors.New("cannot safely join paragraphs with different inherited default namespace bindings")
		}
	}
	return nil
}

func wordParagraphDeclaresNamespace(paragraph *wordXMLNode, prefix string) bool {
	for _, attribute := range paragraph.attrs {
		_, declaredPrefix, declaration := wordNamespaceDeclaration(attribute)
		if declaration && declaredPrefix == prefix {
			return true
		}
	}
	return false
}

func expandSelfClosingWordParagraph(opening []byte, rawName string) ([]byte, []byte, error) {
	trimmed := bytes.TrimSpace(opening)
	if !bytes.HasSuffix(trimmed, []byte("/>")) {
		return nil, nil, errors.New("cannot expand malformed self-closing Word paragraph")
	}
	expanded := append([]byte(nil), trimmed[:len(trimmed)-2]...)
	expanded = append(expanded, '>')
	closing := []byte("</" + rawName + ">")
	return expanded, closing, nil
}

func validateWordParagraphJoinEdge(targets map[string]wordTextTarget, runID string, target wordTextTarget, direction document.WordParagraphJoinDirection) error {
	wantFirst := direction == document.WordJoinPrevious
	for id, candidate := range targets {
		if id == runID || candidate.paragraph != target.paragraph || candidate.text == nil {
			continue
		}
		if (wantFirst && candidate.text.start < target.text.start) || (!wantFirst && candidate.text.start > target.text.start) {
			return errors.New("paragraph join requires the first or last editable text run at the paragraph boundary")
		}
	}
	return nil
}

func findWordXMLParent(node, target *wordXMLNode) *wordXMLNode {
	if node == nil || target == nil {
		return nil
	}
	for _, child := range node.children {
		if child == target {
			return node
		}
		if parent := findWordXMLParent(child, target); parent != nil {
			return parent
		}
	}
	return nil
}

func findWordXMLNode(root, reference *wordXMLNode) *wordXMLNode {
	if root == nil || reference == nil {
		return nil
	}
	if sameWordXMLNode(root, reference) {
		return root
	}
	for _, child := range root.children {
		if found := findWordXMLNode(child, reference); found != nil {
			return found
		}
	}
	return nil
}

func sameWordXMLNode(first, second *wordXMLNode) bool {
	return first != nil && second != nil && first.start == second.start && first.end == second.end && first.name == second.name
}

func paragraphContainsSectionProperties(paragraph *wordXMLNode) bool {
	properties := firstChild(paragraph, "pPr")
	return properties != nil && containsNode(properties, "sectPr")
}

func onlyXMLWhitespace(value []byte) bool {
	for _, character := range value {
		if character != ' ' && character != '\t' && character != '\r' && character != '\n' {
			return false
		}
	}
	return true
}

func wordParagraphContent(data []byte, paragraph *wordXMLNode, omitProperties bool) ([]byte, error) {
	var content bytes.Buffer
	propertiesSeen := false
	cursor := paragraph.contentStart
	for index, child := range paragraph.children {
		if child.name.Local == "pPr" {
			if propertiesSeen || index != 0 {
				return nil, errors.New("cannot safely join a paragraph with misplaced paragraph properties")
			}
			propertiesSeen = true
			content.Write(data[cursor:child.start])
			if !omitProperties {
				content.Write(data[child.start:child.end])
			}
			cursor = child.end
			continue
		}
		content.Write(data[cursor:child.start])
		content.Write(data[child.start:child.end])
		cursor = child.end
	}
	content.Write(data[cursor:paragraph.contentEnd])
	return content.Bytes(), nil
}

func mergedWordParagraphOpening(data []byte, root, left, right *wordXMLNode) ([]byte, error) {
	opening := append([]byte(nil), data[left.start:left.openEnd]...)
	leftBindings := wordNamespaceBindings(root, left)
	for _, attribute := range right.attrs {
		prefix, local, declaration := wordNamespaceDeclaration(attribute)
		if !declaration {
			continue
		}
		if existing, ok := leftBindings[local]; ok {
			if existing != attribute.Value {
				return nil, fmt.Errorf("cannot safely join paragraphs with conflicting namespace declaration %q", local)
			}
			continue
		}
		if local == "" {
			return nil, errors.New("cannot safely join paragraphs that require a new default namespace")
		}
		var err error
		opening, err = setRawXMLAttribute(opening, prefix, local, attribute.Value)
		if err != nil {
			return nil, fmt.Errorf("preserve paragraph namespace declaration: %w", err)
		}
	}
	return opening, nil
}

func wordNamespaceDeclaration(attribute xml.Attr) (prefix, local string, ok bool) {
	if attribute.Name.Space == "xmlns" {
		return "xmlns", attribute.Name.Local, true
	}
	if attribute.Name.Space == "" && attribute.Name.Local == "xmlns" {
		return "", "", true
	}
	return "", "", false
}

func wordNamespaceBindings(root, target *wordXMLNode) map[string]string {
	var result map[string]string
	var visit func(*wordXMLNode, map[string]string) bool
	visit = func(node *wordXMLNode, inherited map[string]string) bool {
		bindings := make(map[string]string, len(inherited)+len(node.attrs))
		for prefix, namespace := range inherited {
			bindings[prefix] = namespace
		}
		for _, attribute := range node.attrs {
			_, prefix, declaration := wordNamespaceDeclaration(attribute)
			if declaration {
				bindings[prefix] = attribute.Value
			}
		}
		if node == target {
			result = bindings
			return true
		}
		for _, child := range node.children {
			if visit(child, bindings) {
				return true
			}
		}
		return false
	}
	visit(root, nil)
	return result
}

func findWordParagraph(blocks []document.WordBlock, id string) *document.WordParagraph {
	for index := range blocks {
		block := &blocks[index]
		if block.Paragraph != nil && block.Paragraph.ID == id {
			return block.Paragraph
		}
		if block.Table == nil {
			continue
		}
		for rowIndex := range block.Table.Rows {
			for cellIndex := range block.Table.Rows[rowIndex].Cells {
				for paragraphIndex := range block.Table.Rows[rowIndex].Cells[cellIndex].Paragraphs {
					paragraph := &block.Table.Rows[rowIndex].Cells[cellIndex].Paragraphs[paragraphIndex]
					if paragraph.ID == id {
						return paragraph
					}
				}
			}
		}
	}
	return nil
}

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
			if !found {
				properties.Write(gap)
			}
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
var _ document.WordParagraphJoiner = (*Document)(nil)
