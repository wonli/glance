package docx

import (
	"fmt"

	"glance/internal/document"
)

var (
	wordRunFormatPaintProperties       = []string{"rStyle", "rFonts", "b", "bCs", "i", "iCs", "color", "sz", "szCs", "highlight", "u"}
	wordRunPropertyOrder               = []string{"rStyle", "rFonts", "b", "bCs", "i", "iCs", "caps", "smallCaps", "strike", "dstrike", "outline", "shadow", "emboss", "imprint", "noProof", "snapToGrid", "vanish", "webHidden", "color", "spacing", "w", "kern", "position", "sz", "szCs", "highlight", "u", "effect", "bdr", "shd", "fitText", "vertAlign", "rtl", "cs", "em", "lang", "eastAsianLayout", "specVanish", "oMath", "rPrChange"}
	wordParagraphFormatPaintProperties = []string{"pStyle", "numPr", "ind", "jc"}
	wordParagraphPropertyOrder         = []string{"pStyle", "keepNext", "keepLines", "pageBreakBefore", "framePr", "widowControl", "numPr", "suppressLineNumbers", "pBdr", "shd", "tabs", "suppressAutoHyphens", "kinsoku", "wordWrap", "overflowPunct", "topLinePunct", "autoSpaceDE", "autoSpaceDN", "bidi", "adjustRightInd", "snapToGrid", "spacing", "ind", "contextualSpacing", "mirrorIndents", "suppressOverlap", "jc", "textDirection", "textAlignment", "textboxTightWrap", "outlineLvl", "divId", "cnfStyle", "rPr", "sectPr", "pPrChange"}
)

func (d *Document) ApplyWordFormatting(sourceRunID, targetRunID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if sourceRunID == targetRunID {
		return nil
	}
	source, sourceOK := d.wordTextTargets[sourceRunID]
	target, targetOK := d.wordTextTargets[targetRunID]
	if !sourceOK || source.run == nil || source.text == nil {
		return fmt.Errorf("Word source run %q is not editable", sourceRunID)
	}
	if !targetOK || target.run == nil || target.text == nil {
		return fmt.Errorf("Word target run %q is not editable", targetRunID)
	}
	sourceParagraph := d.wordParagraphTargets[source.paragraphID]
	targetParagraph := d.wordParagraphTargets[target.paragraphID]
	if sourceParagraph == nil || targetParagraph == nil {
		return fmt.Errorf("Word paragraph for format painting is unavailable")
	}
	if sourceModel, targetModel := findWordParagraph(d.wordBlocks, source.paragraphID), findWordParagraph(d.wordBlocks, target.paragraphID); sourceModel == nil || targetModel == nil || sourceModel.ListKind == document.WordListUnsupported || targetModel.ListKind == document.WordListUnsupported {
		return fmt.Errorf("format painting does not modify paragraphs with unsupported numbering")
	}

	data := d.parts[d.documentIdx].data
	var replacements []xmlReplacement
	var err error
	replacements, err = appendWordFormatProperties(data, replacements, source.run, target.run, "rPr", wordRunFormatPaintProperties, wordRunPropertyOrder)
	if err != nil {
		return fmt.Errorf("copy Word text formatting: %w", err)
	}
	if source.paragraphID != target.paragraphID {
		replacements, err = appendWordFormatProperties(data, replacements, sourceParagraph, targetParagraph, "pPr", wordParagraphFormatPaintProperties, wordParagraphPropertyOrder)
		if err != nil {
			return fmt.Errorf("copy Word paragraph formatting: %w", err)
		}
	}
	patched, err := applyXMLReplacements(data, replacements)
	if err != nil {
		return fmt.Errorf("apply Word format painting: %w", err)
	}
	return d.applyWordDocumentXMLLocked(patched)
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
