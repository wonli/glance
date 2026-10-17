package docx

import (
	"bytes"
	"errors"
	"fmt"
	"glance/internal/document"
)

// DeleteTextSelection preserves run containers and their properties. Paragraph
// seams are removed only after validating the complete selected XML span.
func (d *Document) DeleteTextSelection(startRunID string, startOffset int, endRunID string, endOffset int) (document.WordTextSelectionDeleteResult, error) {
	return d.editWordTextSelection(startRunID, startOffset, endRunID, endOffset, "")
}

// ReplaceTextSelection inserts replacement text with the formatting of the
// selection's first run and preserves all unselected run content.
func (d *Document) ReplaceTextSelection(startRunID string, startOffset int, endRunID string, endOffset int, replacement string) (document.WordTextSelectionEditResult, error) {
	return d.editWordTextSelection(startRunID, startOffset, endRunID, endOffset, replacement)
}

func (d *Document) editWordTextSelection(startRunID string, startOffset int, endRunID string, endOffset int, replacement string) (document.WordTextSelectionEditResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	fail := func(err error) (document.WordTextSelectionEditResult, error) {
		return document.WordTextSelectionEditResult{}, err
	}
	if err := d.checkOpen(); err != nil {
		return fail(err)
	}
	start, ok := d.wordTextTargets[startRunID]
	if !ok || start.text == nil || start.run == nil || start.paragraph == nil {
		return fail(fmt.Errorf("Word text run %q is not editable", startRunID))
	}
	end, ok := d.wordTextTargets[endRunID]
	if !ok || end.text == nil || end.run == nil || end.paragraph == nil {
		return fail(fmt.Errorf("Word text run %q is not editable", endRunID))
	}
	if startOffset < 0 || startOffset > len([]rune(start.text.text)) || endOffset < 0 || endOffset > len([]rune(end.text.text)) || start.text.start > end.text.start || (start.text == end.text && startOffset > endOffset) {
		return fail(errors.New("Word selection endpoints are outside their runs or not ordered"))
	}
	result := document.WordTextSelectionEditResult{ParagraphID: start.paragraphID, RunID: startRunID, Caret: startOffset}
	if start.text == end.text && startOffset == endOffset && replacement == "" {
		return result, nil
	}
	// Nested content such as fields and hyperlinks requires a separate structural
	// edit policy; do not move a selected endpoint out of its original container.
	if start.run.parent != start.paragraph || end.run.parent != end.paragraph {
		return fail(errors.New("Word selection endpoints must belong to direct paragraph runs"))
	}
	data := d.parts[d.documentIdx].data
	root, err := parseWordXML(data)
	if err != nil {
		return fail(err)
	}
	left, right := findWordXMLNode(root, start.paragraph), findWordXMLNode(root, end.paragraph)
	if left == nil || right == nil {
		return fail(errors.New("Word selection paragraphs are missing"))
	}
	paragraphs := []*wordXMLNode{left}
	if left != right {
		parent := left.parent
		if parent == nil || parent != right.parent || parent.name.Space != wordprocessingMLNamespace || (parent.name.Local != "body" && parent.name.Local != "tc") {
			return fail(errors.New("Word selection cannot cross table cells or unsupported containers"))
		}
		paragraphs = nil
		for _, p := range parent.children {
			if p.start < left.start || p.start > right.start {
				continue
			}
			if p.name.Space != wordprocessingMLNamespace || p.name.Local != "p" {
				return fail(errors.New("Word selection cannot cross non-paragraph blocks"))
			}
			if paragraphContainsSectionProperties(p) {
				return fail(errors.New("Word selection cannot cross section properties"))
			}
			if err := validateWordParagraphNamespaceScopes(root, left, p); err != nil {
				return fail(err)
			}
			if len(paragraphs) > 0 && !onlyXMLWhitespace(data[paragraphs[len(paragraphs)-1].end:p.start]) {
				return fail(errors.New("Word selection crosses unsupported XML between paragraphs"))
			}
			paragraphs = append(paragraphs, p)
		}
	}
	// Validate before producing any edit. Images, fields, tabs, breaks, bookmarks,
	// and unknown XML in the selected span are rejected rather than discarded.
	for _, p := range paragraphs {
		if err := validateWordSelectionTextOnly(data, p, start.text.start, end.text.end); err != nil {
			return fail(err)
		}
	}
	changes := make([]xmlReplacement, 0)
	replacementRunes := []rune(replacement)
	for _, target := range d.wordTextTargets {
		if target.text == nil || target.text.start < start.text.start || target.text.start > end.text.start {
			continue
		}
		value := ""
		if target.text == start.text && target.text == end.text {
			runes := []rune(target.text.text)
			value = string(runes[:startOffset]) + replacement + string(runes[endOffset:])
		} else if target.text == start.text {
			value = string([]rune(target.text.text)[:startOffset]) + replacement
		} else if target.text == end.text {
			value = string([]rune(target.text.text)[endOffset:])
		}
		if value == target.text.text {
			continue
		}
		// Existing text replacement handles escaping and xml:space without modifying
		// any properties or other children of the containing formatted run.
		patched, err := replaceWordTextNode(data, target.text, value)
		if err != nil {
			return fail(err)
		}
		delta := len(patched) - len(data)
		changes = append(changes, xmlReplacement{start: target.text.start, end: target.text.end, value: patched[target.text.start : target.text.end+delta]})
	}
	var patched []byte
	if left == right {
		patched, err = applyXMLReplacements(data, changes)
	} else {
		opening, openErr := mergedWordSelectionParagraphOpening(data, root, paragraphs)
		if openErr != nil {
			return fail(openErr)
		}
		var merged bytes.Buffer
		merged.Write(opening)
		for index, p := range paragraphs {
			contentStart := p.contentStart
			if index > 0 {
				if props := firstChild(p, "pPr"); props != nil {
					if len(p.children) == 0 || p.children[0] != props || !onlyXMLWhitespace(data[contentStart:props.start]) {
						return fail(errors.New("Word paragraph has misplaced properties"))
					}
					contentStart = props.end
				}
			}
			local := make([]xmlReplacement, 0)
			for _, change := range changes {
				if change.start >= contentStart && change.end <= p.contentEnd {
					local = append(local, xmlReplacement{start: change.start - contentStart, end: change.end - contentStart, value: change.value})
				}
			}
			content, contentErr := applyXMLReplacements(data[contentStart:p.contentEnd], local)
			if contentErr != nil {
				return fail(contentErr)
			}
			merged.Write(content)
		}
		merged.Write(data[left.contentEnd:left.end])
		patched, err = applyXMLReplacements(data, []xmlReplacement{{start: left.start, end: right.end, value: merged.Bytes()}})
	}
	if err != nil {
		return fail(err)
	}
	// Resolve the caret in a trial projection before committing a single history
	// record. Earlier unselected inlines keep their order and the start run survives.
	parts := append([]part(nil), d.parts...)
	parts[d.documentIdx].data = patched
	projection, err := projectWordDocument(parts, d.documentIdx)
	if err != nil {
		return fail(err)
	}
	caret, ok := projection.textTargets[startRunID]
	caretOffset := startOffset + len(replacementRunes)
	if !ok || caret.paragraphID != start.paragraphID || caretOffset > len([]rune(caret.text.text)) {
		return fail(errors.New("Word selection edit has no editable caret target"))
	}
	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return fail(err)
	}
	result.Caret = caretOffset
	return result, nil
}

func validateWordSelectionTextOnly(data []byte, node *wordXMLNode, start, end int) error {
	return validateWordSelectionText(data, node, start, end, false)
}

func validateWordSelectionText(data []byte, node *wordXMLNode, start, end int, preserveBookmarks bool) error {
	if node.end <= start || node.start >= end {
		return nil
	}
	if node.name.Space != wordprocessingMLNamespace {
		return errors.New("Word selection contains unsupported XML")
	}
	switch node.name.Local {
	case "pPr":
		if node.parent == nil || node.parent.name.Local != "p" {
			return errors.New("Word selection contains misplaced paragraph properties")
		}
		return nil
	case "rPr":
		if node.parent == nil || node.parent.name.Local != "r" {
			return errors.New("Word selection contains misplaced run properties")
		}
		return nil
	case "t":
		if len(node.children) != 0 {
			return errors.New("Word text contains nested XML")
		}
		return nil
	case "bookmarkStart", "bookmarkEnd":
		// Formatting replaces runs without moving paragraph-level range markers.
		// Deletion still rejects them because it would remove bookmark endpoints.
		if preserveBookmarks && node.parent != nil && node.parent.name.Local == "p" && len(node.children) == 0 && onlyXMLWhitespace(data[node.contentStart:node.contentEnd]) {
			return nil
		}
		return fmt.Errorf("Word selection contains unsupported content %q", node.name.Local)
	case "p", "r":
	default:
		return fmt.Errorf("Word selection contains unsupported content %q", node.name.Local)
	}
	cursor := node.contentStart
	for _, child := range node.children {
		gapStart, gapEnd := max(cursor, start), min(child.start, end)
		if gapStart < gapEnd && !onlyXMLWhitespace(data[gapStart:gapEnd]) {
			return errors.New("Word selection contains unsupported content between elements")
		}
		if err := validateWordSelectionText(data, child, start, end, preserveBookmarks); err != nil {
			return err
		}
		cursor = child.end
	}
	gapStart, gapEnd := max(cursor, start), min(node.contentEnd, end)
	if gapStart < gapEnd && !onlyXMLWhitespace(data[gapStart:gapEnd]) {
		return errors.New("Word selection contains unsupported trailing XML")
	}
	return nil
}

var _ document.WordTextSelectionEditor = (*Document)(nil)
var _ document.WordTextSelectionReplacer = (*Document)(nil)

// Each retained empty run still carries its original properties, including
// extension XML. Preserve declarations from every merged paragraph and reject
// conflicting bindings before changing the package.
func mergedWordSelectionParagraphOpening(data []byte, root *wordXMLNode, paragraphs []*wordXMLNode) ([]byte, error) {
	left := paragraphs[0]
	opening := append([]byte(nil), data[left.start:left.openEnd]...)
	bindings := wordNamespaceBindings(root, left)
	for _, paragraph := range paragraphs[1:] {
		for _, attribute := range paragraph.attrs {
			prefix, local, declaration := wordNamespaceDeclaration(attribute)
			if !declaration {
				continue
			}
			if existing, ok := bindings[local]; ok {
				if existing != attribute.Value {
					return nil, fmt.Errorf("cannot merge Word selection with conflicting namespace %q", local)
				}
				continue
			}
			if local == "" {
				return nil, errors.New("cannot merge Word selection with a new default namespace")
			}
			var err error
			opening, err = setRawXMLAttribute(opening, prefix, local, attribute.Value)
			if err != nil {
				return nil, err
			}
			bindings[local] = attribute.Value
		}
	}
	return opening, nil
}
