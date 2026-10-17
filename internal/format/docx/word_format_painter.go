package docx

import (
	"bytes"
	"fmt"
	"sort"

	"glance/internal/document"
)

var _ document.WordTextSelectionFormatPainter = (*Document)(nil)

var (
	wordRunFormatPaintProperties       = []string{"rStyle", "rFonts", "b", "bCs", "i", "iCs", "color", "sz", "szCs", "highlight", "u"}
	wordRunPropertyOrder               = []string{"rStyle", "rFonts", "b", "bCs", "i", "iCs", "caps", "smallCaps", "strike", "dstrike", "outline", "shadow", "emboss", "imprint", "noProof", "snapToGrid", "vanish", "webHidden", "color", "spacing", "w", "kern", "position", "sz", "szCs", "highlight", "u", "effect", "bdr", "shd", "fitText", "vertAlign", "rtl", "cs", "em", "lang", "eastAsianLayout", "specVanish", "oMath", "rPrChange"}
	wordParagraphFormatPaintProperties = []string{"pStyle", "numPr", "ind", "jc"}
	wordParagraphPropertyOrder         = []string{"pStyle", "keepNext", "keepLines", "pageBreakBefore", "framePr", "widowControl", "numPr", "suppressLineNumbers", "pBdr", "shd", "tabs", "suppressAutoHyphens", "kinsoku", "wordWrap", "overflowPunct", "topLinePunct", "autoSpaceDE", "autoSpaceDN", "bidi", "adjustRightInd", "snapToGrid", "spacing", "ind", "contextualSpacing", "mirrorIndents", "suppressOverlap", "jc", "textDirection", "textAlignment", "textboxTightWrap", "outlineLvl", "divId", "cnfStyle", "rPr", "sectPr", "pPrChange"}
)

func (d *Document) ApplyWordFormatting(sourceRunID, targetRunID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return false, err
	}
	if sourceRunID == targetRunID {
		return false, nil
	}
	source, sourceOK := d.wordTextTargets[sourceRunID]
	target, targetOK := d.wordTextTargets[targetRunID]
	if !sourceOK || source.run == nil || source.text == nil {
		return false, fmt.Errorf("Word source run %q is not editable", sourceRunID)
	}
	if !targetOK || target.run == nil || target.text == nil {
		return false, fmt.Errorf("Word target run %q is not editable", targetRunID)
	}
	sourceParagraph := d.wordParagraphTargets[source.paragraphID]
	targetParagraph := d.wordParagraphTargets[target.paragraphID]
	if sourceParagraph == nil || targetParagraph == nil {
		return false, fmt.Errorf("Word paragraph for format painting is unavailable")
	}
	if sourceModel, targetModel := findWordParagraph(d.wordBlocks, source.paragraphID), findWordParagraph(d.wordBlocks, target.paragraphID); sourceModel == nil || targetModel == nil || sourceModel.ListKind == document.WordListUnsupported || targetModel.ListKind == document.WordListUnsupported {
		return false, fmt.Errorf("format painting does not modify paragraphs with unsupported numbering")
	}

	data := d.parts[d.documentIdx].data
	var replacements []xmlReplacement
	var err error
	replacements, err = appendWordFormatProperties(data, replacements, source.run, target.run, "rPr", wordRunFormatPaintProperties, wordRunPropertyOrder)
	if err != nil {
		return false, fmt.Errorf("copy Word text formatting: %w", err)
	}
	if source.paragraphID != target.paragraphID {
		replacements, err = appendWordFormatProperties(data, replacements, sourceParagraph, targetParagraph, "pPr", wordParagraphFormatPaintProperties, wordParagraphPropertyOrder)
		if err != nil {
			return false, fmt.Errorf("copy Word paragraph formatting: %w", err)
		}
	}
	patched, err := applyXMLReplacements(data, replacements)
	if err != nil {
		return false, fmt.Errorf("apply Word format painting: %w", err)
	}
	changed := !bytes.Equal(data, patched)
	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return false, err
	}
	return changed, nil
}

func (d *Document) ApplyWordFormattingToTextSelection(sourceRunID, startRunID string, startOffset int, endRunID string, endOffset int) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return false, err
	}
	source, sourceOK := d.wordTextTargets[sourceRunID]
	start, startOK := d.wordTextTargets[startRunID]
	end, endOK := d.wordTextTargets[endRunID]
	if !sourceOK || source.run == nil || source.text == nil || !wordTextRunEditable(d.wordBlocks, sourceRunID) {
		return false, fmt.Errorf("Word source run %q is not editable", sourceRunID)
	}
	if !startOK || !endOK || start.run == nil || end.run == nil || start.text == nil || end.text == nil || start.paragraph == nil || end.paragraph == nil || !wordTextRunEditable(d.wordBlocks, startRunID) || !wordTextRunEditable(d.wordBlocks, endRunID) {
		return false, fmt.Errorf("Word format-painter selection is not editable")
	}
	if startOffset < 0 || startOffset > len([]rune(start.text.text)) || endOffset < 0 || endOffset > len([]rune(end.text.text)) || start.text.start > end.text.start || (start.text == end.text && startOffset >= endOffset) {
		return false, fmt.Errorf("Word format-painter selection endpoints are invalid")
	}
	root, err := parseWordXML(d.parts[d.documentIdx].data)
	if err != nil {
		return false, err
	}
	data := d.parts[d.documentIdx].data
	if err := validateWordTextSelectionForFormatting(data, root, start, end); err != nil {
		return false, fmt.Errorf("Word format-painter selection is unsafe: %w", err)
	}
	sourceParagraph := d.wordParagraphTargets[source.paragraphID]
	if sourceParagraph == nil {
		return false, fmt.Errorf("Word source paragraph is unavailable")
	}
	if sourceModel := findWordParagraph(d.wordBlocks, source.paragraphID); sourceModel == nil || sourceModel.ListKind == document.WordListUnsupported {
		return false, fmt.Errorf("format painting does not modify paragraphs with unsupported numbering")
	}

	type selectedTarget struct {
		target wordTextTarget
		range_ wordSelectedTextRange
	}
	selected := make([]selectedTarget, 0)
	for runID, target := range d.wordTextTargets {
		if target.text == nil || target.run == nil || target.text.start < start.text.start || target.text.start > end.text.start {
			continue
		}
		if target.run.parent != target.paragraph || target.paragraph == nil || !wordTextRunEditable(d.wordBlocks, runID) {
			return false, fmt.Errorf("Word format-painter selection includes unsupported text")
		}
		from, to := 0, len([]rune(target.text.text))
		if target.text == start.text {
			from = startOffset
		}
		if target.text == end.text {
			to = endOffset
		}
		if from < 0 || to > len([]rune(target.text.text)) || from >= to {
			continue
		}
		selected = append(selected, selectedTarget{target: target, range_: wordSelectedTextRange{start: from, end: to}})
	}
	if len(selected) == 0 {
		return false, fmt.Errorf("Word format-painter selection contains no editable text")
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].target.text.start < selected[j].target.text.start })
	rangesByRun := make(map[*wordXMLNode]map[*wordXMLNode]wordSelectedTextRange)
	paragraphs := make(map[*wordXMLNode]bool)
	runsByNode := make(map[*wordXMLNode]bool)
	for _, item := range selected {
		if rangesByRun[item.target.run] == nil {
			rangesByRun[item.target.run] = make(map[*wordXMLNode]wordSelectedTextRange)
		}
		rangesByRun[item.target.run][item.target.text] = item.range_
		runsByNode[item.target.run] = true
		paragraphs[item.target.paragraph] = true
	}
	runs := make([]*wordXMLNode, 0, len(runsByNode))
	for run := range runsByNode {
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].start < runs[j].start })
	replacements := make([]xmlReplacement, 0, len(runs)+len(paragraphs))
	for _, run := range runs {
		fragments, err := splitWordRunForSelection(data, run, rangesByRun[run], start.text.start, end.text.end)
		if err != nil {
			return false, fmt.Errorf("split Word format-painter selection: %w", err)
		}
		var replacement bytes.Buffer
		if fragments.prefix.Len() > 0 {
			replacement.Write(wrapWordRunFragment(data, run, fragments.props, fragments.prefix.Bytes()))
		}
		selectedRun := wrapWordRunFragment(data, run, fragments.props, fragments.selected.Bytes())
		selectedRun, err = mergeWordFormatPropertiesIntoRunFragment(data, root, source.run, run, selectedRun)
		if err != nil {
			return false, fmt.Errorf("copy source formatting onto selected Word text: %w", err)
		}
		replacement.Write(selectedRun)
		if fragments.suffix.Len() > 0 {
			replacement.Write(wrapWordRunFragment(data, run, fragments.props, fragments.suffix.Bytes()))
		}
		replacements = append(replacements, xmlReplacement{start: run.start, end: run.end, value: replacement.Bytes()})
	}
	if sourceModel := findWordParagraph(d.wordBlocks, source.paragraphID); sourceModel == nil {
		return false, fmt.Errorf("Word source paragraph is unavailable")
	}
	paragraphIDs := make(map[*wordXMLNode]string, len(d.wordParagraphTargets))
	for id, paragraph := range d.wordParagraphTargets {
		paragraphIDs[paragraph] = id
	}
	for paragraph := range paragraphs {
		if paragraph == sourceParagraph {
			continue
		}
		if paragraphModel := findWordParagraph(d.wordBlocks, paragraphIDs[paragraph]); paragraphModel == nil || paragraphModel.ListKind == document.WordListUnsupported {
			return false, fmt.Errorf("format painting does not modify paragraphs with unsupported numbering")
		}
		replacements, err = appendWordFormatProperties(data, replacements, sourceParagraph, paragraph, "pPr", wordParagraphFormatPaintProperties, wordParagraphPropertyOrder)
		if err != nil {
			return false, fmt.Errorf("copy Word paragraph formatting: %w", err)
		}
	}
	patched, err := applyXMLReplacements(data, replacements)
	if err != nil {
		return false, fmt.Errorf("apply Word format painting: %w", err)
	}
	changed := !bytes.Equal(data, patched)
	if !changed {
		return false, nil
	}
	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return false, err
	}
	return true, nil
}

func mergeWordFormatPropertiesIntoRunFragment(data []byte, root, sourceRun, targetRun *wordXMLNode, targetFragment []byte) ([]byte, error) {
	bindings := wordNamespaceBindings(root, targetRun)
	for prefix, namespace := range wordNamespaceBindings(root, sourceRun) {
		if existing, ok := bindings[prefix]; ok && existing != namespace {
			return nil, fmt.Errorf("source and target use conflicting XML namespace %q", prefix)
		}
		bindings[prefix] = namespace
	}
	prefixes := make([]string, 0, len(bindings))
	for prefix := range bindings {
		if prefix != "xml" {
			prefixes = append(prefixes, prefix)
		}
	}
	sort.Strings(prefixes)
	var wrapper bytes.Buffer
	wrapper.WriteString("<root")
	for _, prefix := range prefixes {
		if prefix == "" {
			fmt.Fprintf(&wrapper, ` xmlns="%s"`, escapeXMLString(bindings[prefix]))
			continue
		}
		fmt.Fprintf(&wrapper, ` xmlns:%s="%s"`, prefix, escapeXMLString(bindings[prefix]))
	}
	wrapper.WriteByte('>')
	wrapper.Write(data[sourceRun.start:sourceRun.end])
	wrapper.Write(targetFragment)
	wrapper.WriteString("</root>")
	combined := wrapper.Bytes()
	combinedRoot, err := parseWordXML(combined)
	if err != nil {
		return nil, err
	}
	if len(combinedRoot.children) != 2 || combinedRoot.children[0].name.Local != "r" || combinedRoot.children[1].name.Local != "r" {
		return nil, fmt.Errorf("format-painter run fragments are invalid")
	}
	replacements, err := appendWordFormatProperties(combined, nil, combinedRoot.children[0], combinedRoot.children[1], "rPr", wordRunFormatPaintProperties, wordRunPropertyOrder)
	if err != nil {
		return nil, err
	}
	patched, err := applyXMLReplacements(combined, replacements)
	if err != nil {
		return nil, err
	}
	updatedRoot, err := parseWordXML(patched)
	if err != nil {
		return nil, err
	}
	if len(updatedRoot.children) != 2 {
		return nil, fmt.Errorf("format-painter target run disappeared")
	}
	target := updatedRoot.children[1]
	return append([]byte(nil), patched[target.start:target.end]...), nil
}

func appendWordFormatProperties(data []byte, replacements []xmlReplacement, source, target *wordXMLNode, parentName string, supported, order []string) ([]xmlReplacement, error) {
	sourceProperties := firstChild(source, parentName)
	targetProperties := firstChild(target, parentName)
	if targetProperties == nil {
		if sourceProperties == nil {
			return replacements, nil
		}
		children := make([]byte, 0)
		for _, name := range supported {
			if child := firstChild(sourceProperties, name); child != nil {
				children = append(children, data[child.start:child.end]...)
			}
		}
		if len(children) == 0 {
			return replacements, nil
		}
		markup := wrapWordElement(elementPrefix(target.rawName), parentName, children)
		replacements = append(replacements, xmlReplacement{start: target.openEnd, end: target.openEnd, value: markup})
		return replacements, nil
	}

	ranks := make(map[string]int, len(order))
	for index, name := range order {
		ranks[name] = index
	}
	supportedSet := make(map[string]bool, len(supported))
	for _, name := range supported {
		supportedSet[name] = true
	}
	firstTargets := make(map[string]*wordXMLNode, len(supported))
	duplicateTargets := make(map[string][]*wordXMLNode, len(supported))
	for _, child := range targetProperties.children {
		name := child.name.Local
		if !supportedSet[name] {
			continue
		}
		if firstTargets[name] == nil {
			firstTargets[name] = child
		} else {
			duplicateTargets[name] = append(duplicateTargets[name], child)
		}
	}

	insertions := make(map[int][]byte)
	for _, name := range supported {
		sourceChild := firstChild(sourceProperties, name)
		targetChild := firstTargets[name]
		switch {
		case sourceChild == nil && targetChild != nil:
			replacements = append(replacements, xmlReplacement{start: targetChild.start, end: targetChild.end})
		case sourceChild != nil && targetChild != nil:
			replacements = append(replacements, xmlReplacement{start: targetChild.start, end: targetChild.end, value: data[sourceChild.start:sourceChild.end]})
		case sourceChild != nil:
			insertAt := targetProperties.contentEnd
			sourceRank := ranks[name]
			for _, child := range targetProperties.children {
				if rank, known := ranks[child.name.Local]; known && rank > sourceRank {
					insertAt = child.start
					break
				}
			}
			insertions[insertAt] = append(insertions[insertAt], data[sourceChild.start:sourceChild.end]...)
		}
		for _, duplicate := range duplicateTargets[name] {
			replacements = append(replacements, xmlReplacement{start: duplicate.start, end: duplicate.end})
		}
	}
	for insertAt, value := range insertions {
		replacements = append(replacements, xmlReplacement{start: insertAt, end: insertAt, value: value})
	}
	return replacements, nil
}
