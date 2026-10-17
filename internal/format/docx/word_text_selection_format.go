package docx

import (
	"bytes"
	"errors"
	"fmt"
	"glance/internal/document"
	"sort"
)

type wordSelectedTextRange struct {
	start int
	end   int
}

type wordRunSelectionFragments struct {
	prefix   bytes.Buffer
	selected bytes.Buffer
	suffix   bytes.Buffer
	props    [][]byte
}

func (f *wordRunSelectionFragments) bufferFor(start, end, selectionStart, selectionEnd int) *bytes.Buffer {
	switch {
	case end <= selectionStart:
		return &f.prefix
	case start >= selectionEnd:
		return &f.suffix
	default:
		return &f.selected
	}
}

// SetTextSelectionStyle splits only the affected text nodes into safe run
// fragments, preserving all unselected XML and properties in the original run.
func (d *Document) SetTextSelectionStyle(startRunID string, startOffset int, endRunID string, endOffset int, patch document.WordTextFormatPatch) (document.WordTextSelectionFormatResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	selection := document.WordTextSelectionFormatResult{StartRunID: startRunID, StartOffset: startOffset, EndRunID: endRunID, EndOffset: endOffset}
	fail := func(err error) (document.WordTextSelectionFormatResult, error) { return selection, err }
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
	startRunes, endRunes := []rune(start.text.text), []rune(end.text.text)
	if startOffset < 0 || startOffset > len(startRunes) || endOffset < 0 || endOffset > len(endRunes) || start.text.start > end.text.start || (start.text == end.text && startOffset > endOffset) {
		return fail(errors.New("Word selection endpoints are outside their runs or not ordered"))
	}
	if start.text == end.text && startOffset == endOffset {
		return selection, nil
	}
	if start.run.parent != start.paragraph || end.run.parent != end.paragraph {
		return fail(errors.New("Word selection endpoints must belong to direct paragraph runs"))
	}
	updates, order, err := wordStyleUpdates(patch)
	if err != nil {
		return fail(err)
	}
	if len(order) == 0 {
		return selection, nil
	}
	data := d.parts[d.documentIdx].data
	root, err := parseWordXML(data)
	if err != nil {
		return fail(err)
	}
	if err := validateWordTextSelectionForFormatting(data, root, start, end); err != nil {
		return fail(err)
	}

	type selectedTarget struct {
		target wordTextTarget
		range_ wordSelectedTextRange
	}
	selected := make([]selectedTarget, 0)
	for _, target := range d.wordTextTargets {
		if target.text == nil || target.run == nil || target.paragraph == nil || target.text.start < start.text.start || target.text.start > end.text.start {
			continue
		}
		textRunes := []rune(target.text.text)
		from, to := 0, len(textRunes)
		if target.text == start.text {
			from = startOffset
		}
		if target.text == end.text {
			to = endOffset
		}
		if from < 0 || to < from || to > len(textRunes) {
			return fail(errors.New("Word selection range is inconsistent with projected text"))
		}
		if from == to {
			continue
		}
		selected = append(selected, selectedTarget{target: target, range_: wordSelectedTextRange{start: from, end: to}})
	}
	if len(selected) == 0 {
		return selection, nil
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].target.text.start < selected[j].target.text.start })
	selectionStart, selectionEnd := selected[0].target.text.start, selected[len(selected)-1].target.text.end
	rangesByText := make(map[*wordXMLNode]wordSelectedTextRange, len(selected))
	runsByNode := make(map[*wordXMLNode]bool)
	for _, item := range selected {
		if !wordTextStyleMatchesPatch(d.effectiveWordTargetStyle(item.target), patch) {
			runsByNode[item.target.run] = true
		}
		rangesByText[item.target.text] = item.range_
	}
	if len(runsByNode) == 0 {
		return selection, nil
	}
	runs := make([]*wordXMLNode, 0, len(runsByNode))
	for run := range runsByNode {
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].start < runs[j].start })

	replacements := make([]xmlReplacement, 0, len(runs))
	for _, run := range runs {
		fragments, err := splitWordRunForSelection(data, run, rangesByText, selectionStart, selectionEnd)
		if err != nil {
			return fail(err)
		}
		selectedContent := wrapWordRunFragment(data, run, fragments.props, fragments.selected.Bytes())
		selectedRun, err := patchedWordRunFragment(selectedContent, updates, order)
		if err != nil {
			return fail(err)
		}
		var replacement bytes.Buffer
		if fragments.prefix.Len() != 0 {
			replacement.Write(wrapWordRunFragment(data, run, fragments.props, fragments.prefix.Bytes()))
		}
		replacement.Write(selectedRun)
		if fragments.suffix.Len() != 0 {
			replacement.Write(wrapWordRunFragment(data, run, fragments.props, fragments.suffix.Bytes()))
		}
		replacements = append(replacements, xmlReplacement{start: run.start, end: run.end, value: replacement.Bytes()})
	}
	patched, err := applyXMLReplacements(data, replacements)
	if err != nil {
		return fail(err)
	}
	parts := append([]part(nil), d.parts...)
	parts[d.documentIdx].data = patched
	projection, err := projectWordDocument(parts, d.documentIdx)
	if err != nil {
		return fail(fmt.Errorf("validate Word text formatting: %w", err))
	}
	startPosition, err := wordTextTargetParagraphOffset(start, startOffset, d.wordTextTargets)
	if err != nil {
		return fail(err)
	}
	endPosition, err := wordTextTargetParagraphOffset(end, endOffset, d.wordTextTargets)
	if err != nil {
		return fail(err)
	}
	selection.StartRunID, selection.StartOffset, err = wordTextTargetAtParagraphOffset(projection.textTargets, start.paragraphID, startPosition, true)
	if err != nil {
		return fail(err)
	}
	selection.EndRunID, selection.EndOffset, err = wordTextTargetAtParagraphOffset(projection.textTargets, end.paragraphID, endPosition, false)
	if err != nil {
		return fail(err)
	}
	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return fail(err)
	}
	return selection, nil
}

func wordTextTargetParagraphOffset(target wordTextTarget, offset int, targets map[string]wordTextTarget) (int, error) {
	ordered := make([]wordTextTarget, 0)
	for _, candidate := range targets {
		if candidate.text != nil && candidate.paragraphID == target.paragraphID {
			ordered = append(ordered, candidate)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].text.start < ordered[j].text.start })
	position := 0
	for _, candidate := range ordered {
		if candidate.text == target.text {
			if offset < 0 || offset > len([]rune(candidate.text.text)) {
				return 0, errors.New("Word selection endpoint is outside its text run")
			}
			return position + offset, nil
		}
		position += len([]rune(candidate.text.text))
	}
	return 0, errors.New("Word selection endpoint is missing from its paragraph")
}

func wordTextTargetAtParagraphOffset(targets map[string]wordTextTarget, paragraphID string, position int, downstream bool) (string, int, error) {
	type identifiedTarget struct {
		id     string
		target wordTextTarget
	}
	ordered := make([]identifiedTarget, 0)
	for id, target := range targets {
		if target.text != nil && target.paragraphID == paragraphID {
			ordered = append(ordered, identifiedTarget{id: id, target: target})
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].target.text.start < ordered[j].target.text.start })
	if position < 0 {
		return "", 0, errors.New("Word selection position is negative")
	}
	cursor := 0
	for _, candidate := range ordered {
		length := len([]rune(candidate.target.text.text))
		if length == 0 {
			continue
		}
		if downstream && position >= cursor && position < cursor+length || !downstream && position > cursor && position <= cursor+length {
			return candidate.id, position - cursor, nil
		}
		cursor += length
	}
	if len(ordered) != 0 && position == cursor {
		last := ordered[len(ordered)-1]
		return last.id, len([]rune(last.target.text.text)), nil
	}
	return "", 0, errors.New("Word selection position cannot be mapped after formatting")
}

func wordTextStyleMatchesPatch(style document.WordTextStyle, patch document.WordTextFormatPatch) bool {
	if patch.Bold != nil && style.Bold != *patch.Bold || patch.Italic != nil && style.Italic != *patch.Italic || patch.Underline != nil && style.Underline != *patch.Underline {
		return false
	}
	if patch.FontColor != nil {
		value, err := normalizeWordColor(*patch.FontColor)
		if err != nil || style.FontColor != value {
			return false
		}
	}
	if patch.Highlight != nil {
		value, err := normalizeWordHighlight(*patch.Highlight)
		if err != nil || style.Highlight != value {
			return false
		}
	}
	if patch.FontSize != nil && style.FontSize != *patch.FontSize {
		return false
	}
	return true
}

func validateWordTextSelectionForFormatting(data []byte, root *wordXMLNode, start, end wordTextTarget) error {
	left, right := findWordXMLNode(root, start.paragraph), findWordXMLNode(root, end.paragraph)
	if left == nil || right == nil {
		return errors.New("Word selection paragraphs are missing")
	}
	paragraphs := []*wordXMLNode{left}
	if left != right {
		parent := left.parent
		if parent == nil || parent != right.parent || parent.name.Space != wordprocessingMLNamespace || (parent.name.Local != "body" && parent.name.Local != "tc") {
			return errors.New("Word selection cannot cross table cells or unsupported containers")
		}
		paragraphs = nil
		for _, paragraph := range parent.children {
			if paragraph.start < left.start || paragraph.start > right.start {
				continue
			}
			if paragraph.name.Space != wordprocessingMLNamespace || paragraph.name.Local != "p" {
				return errors.New("Word selection cannot cross non-paragraph blocks")
			}
			if paragraphContainsSectionProperties(paragraph) {
				return errors.New("Word selection cannot cross section properties")
			}
			if err := validateWordParagraphNamespaceScopes(root, left, paragraph); err != nil {
				return err
			}
			if len(paragraphs) > 0 && !onlyXMLWhitespace(data[paragraphs[len(paragraphs)-1].end:paragraph.start]) {
				return errors.New("Word selection crosses unsupported XML between paragraphs")
			}
			paragraphs = append(paragraphs, paragraph)
		}
	}
	for _, paragraph := range paragraphs {
		if err := validateWordSelectionText(data, paragraph, start.text.start, end.text.end, true); err != nil {
			return err
		}
	}
	return nil
}

func splitWordRunForSelection(data []byte, run *wordXMLNode, ranges map[*wordXMLNode]wordSelectedTextRange, selectionStart, selectionEnd int) (*wordRunSelectionFragments, error) {
	fragments := &wordRunSelectionFragments{}
	cursor := run.contentStart
	for _, child := range run.children {
		if cursor < child.start {
			fragments.bufferFor(cursor, child.start, selectionStart, selectionEnd).Write(data[cursor:child.start])
		}
		if child.name.Space == wordprocessingMLNamespace && child.name.Local == "rPr" {
			fragments.props = append(fragments.props, append([]byte(nil), data[child.start:child.end]...))
			cursor = child.end
			continue
		}
		if selected, ok := ranges[child]; ok {
			runes := []rune(child.text)
			prefix, err := wordTextNodeXML(data, child, string(runes[:selected.start]))
			if err != nil {
				return nil, err
			}
			selectedXML, err := wordTextNodeXML(data, child, string(runes[selected.start:selected.end]))
			if err != nil {
				return nil, err
			}
			suffix, err := wordTextNodeXML(data, child, string(runes[selected.end:]))
			if err != nil {
				return nil, err
			}
			if selected.start > 0 {
				fragments.prefix.Write(prefix)
			}
			fragments.selected.Write(selectedXML)
			if selected.end < len(runes) {
				fragments.suffix.Write(suffix)
			}
		} else {
			fragments.bufferFor(child.start, child.end, selectionStart, selectionEnd).Write(data[child.start:child.end])
		}
		cursor = child.end
	}
	if cursor < run.contentEnd {
		fragments.bufferFor(cursor, run.contentEnd, selectionStart, selectionEnd).Write(data[cursor:run.contentEnd])
	}
	if fragments.selected.Len() == 0 {
		return nil, errors.New("Word selected text is not contained in its formatted run")
	}
	if len(fragments.props) > 1 {
		return nil, errors.New("Word run contains duplicate run-property elements")
	}
	return fragments, nil
}

func wrapWordRunFragment(data []byte, run *wordXMLNode, properties [][]byte, content []byte) []byte {
	result := make([]byte, 0, run.openEnd-run.start+len(content)+run.end-run.contentEnd)
	result = append(result, data[run.start:run.openEnd]...)
	for _, property := range properties {
		result = append(result, property...)
	}
	result = append(result, content...)
	result = append(result, data[run.contentEnd:run.end]...)
	return result
}

func patchedWordRunFragment(fragment []byte, updates map[string]wordPropertyUpdate, order []string) ([]byte, error) {
	run, err := parseWordXML(fragment)
	if err != nil {
		return nil, err
	}
	return patchRunProperties(fragment, run, updates, order)
}

var _ document.WordTextSelectionFormatter = (*Document)(nil)
