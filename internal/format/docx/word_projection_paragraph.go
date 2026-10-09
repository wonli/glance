package docx

import (
	"fmt"
	"glance/internal/document"
	"strconv"
	"strings"
)

func (p *wordProjector) paragraph(node *wordXMLNode, id string) document.WordParagraph {
	paragraph := document.WordParagraph{ID: id, Alignment: document.WordAlignDefault}
	paragraph.ListKind, paragraph.ListLevel, paragraph.ListMarker = p.paragraphList(node)
	paragraph.ListMaxLevel = p.paragraphListMaxLevel(node, paragraph.ListKind)
	p.paragraphTargets[id] = node
	if properties := firstChild(node, "pPr"); properties != nil {
		paragraph.Layout = parseWordParagraphLayout(properties)
		if style := firstChild(properties, "pStyle"); style != nil {
			paragraph.Style = attrValue(style, "val")
		}
		if alignment := firstChild(properties, "jc"); alignment != nil {
			paragraph.Alignment = parseWordAlignment(attrValue(alignment, "val"))
		}
		if indent := firstChild(properties, "ind"); indent != nil {
			left := attrValue(indent, "left")
			if left == "" {
				left = attrValue(indent, "start")
			}
			right := attrValue(indent, "right")
			if right == "" {
				right = attrValue(indent, "end")
			}
			paragraph.LeftIndentEMU = parseWordIndentEMU(left)
			paragraph.RightIndentEMU = parseWordIndentEMU(right)
		}
	}
	inlineIndex := 0
	p.paragraphInlines[id] = nil
	for _, child := range node.children {
		switch child.name.Local {
		case "pPr", "bookmarkStart", "bookmarkEnd", "proofErr", "commentRangeStart", "commentRangeEnd", "permStart", "permEnd", "lastRenderedPageBreak":
			continue
		case "r":
			p.appendRunInlines(&paragraph, node, child, &inlineIndex)
		case "ins":
			for _, linked := range child.children {
				if linked.name.Local == "r" {
					p.appendRunInlines(&paragraph, node, linked, &inlineIndex)
				}
			}
		case "smartTag", "customXml":
			for _, linked := range child.children {
				if linked.name.Local == "r" {
					p.appendRunInlines(&paragraph, node, linked, &inlineIndex)
				}
			}
		case "del":
			// In standard preview mode, deleted text is omitted.
			continue
		case "hyperlink":
			for _, linked := range child.children {
				if isWordRangeMarker(linked.name.Local) {
					continue
				}
				if linked.name.Local == "r" {
					p.appendRunInlines(&paragraph, node, linked, &inlineIndex)
				} else {
					inlineID := fmt.Sprintf("%s.i%d", id, inlineIndex)
					p.appendProjectedInline(&paragraph, inlineID, unsupportedInline(inlineID, "Unsupported hyperlink content: "+linked.name.Local), linked, nil)
					inlineIndex++
				}
			}
		case "AlternateContent":
			p.appendAlternateContent(&paragraph, child, fmt.Sprintf("%s.i%d", id, inlineIndex))
			inlineIndex++
		default:
			inlineID := fmt.Sprintf("%s.i%d", id, inlineIndex)
			p.appendProjectedInline(&paragraph, inlineID, unsupportedInline(inlineID, "Unsupported paragraph content: "+child.name.Local), child, nil)
			inlineIndex++
		}
	}
	fieldContent := containsNode(node, "fldChar") || containsNode(node, "instrText")
	if fieldContent {
		for index := range paragraph.Inlines {
			text := paragraph.Inlines[index].Text
			if text == nil {
				continue
			}
			text.Editable = false
			delete(p.textTargets, text.ID)
			delete(p.inlineTargets, text.ID)
		}
	}
	return paragraph
}

func (p *wordProjector) paragraphList(node *wordXMLNode) (document.WordListKind, int, string) {
	properties := firstChild(node, "pPr")
	numPr := firstChild(properties, "numPr")
	if numPr == nil {
		return document.WordListNone, 0, ""
	}
	numID := attrValue(firstChild(numPr, "numId"), "val")
	if numID == "" {
		return document.WordListUnsupported, 0, ""
	}
	level := 0
	if value := attrValue(firstChild(numPr, "ilvl"), "val"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 || parsed > 8 {
			return document.WordListUnsupported, 0, ""
		}
		level = parsed
	}
	definition, ok := p.numbering[numID]
	if !ok {
		return document.WordListUnsupported, level, ""
	}
	numberingLevel, ok := definition.levels[level]
	if !ok || numberingLevel.kind == document.WordListNone {
		return document.WordListUnsupported, level, ""
	}
	for parentLevel := 0; parentLevel < level; parentLevel++ {
		parentDefinition, exists := definition.levels[parentLevel]
		if !exists || (parentDefinition.kind != document.WordListBullet && parentDefinition.kind != document.WordListNumbered) {
			return document.WordListUnsupported, level, ""
		}
	}
	if numberingLevel.kind == document.WordListBullet {
		marker := strings.TrimSpace(numberingLevel.text)
		if marker == "" || strings.Contains(marker, "%") {
			marker = wordListBulletMarker(level)
		}
		return numberingLevel.kind, level, marker
	}
	if numberingLevel.kind != document.WordListNumbered {
		return document.WordListUnsupported, level, ""
	}
	marker := numberingLevel.text
	for markerLevel := 0; markerLevel <= level; markerLevel++ {
		levelDefinition, exists := definition.levels[markerLevel]
		if !exists {
			continue
		}
		value := wordNumberingValue(node, numID, markerLevel, level, levelDefinition.start)
		formatted := formatWordNumberingValue(value, levelDefinition.format)
		marker = strings.ReplaceAll(marker, "%"+strconv.Itoa(markerLevel+1), formatted)
	}
	if marker == "" {
		value := wordNumberingValue(node, numID, level, level, numberingLevel.start)
		marker = formatWordNumberingValue(value, numberingLevel.format) + "."
	}
	return numberingLevel.kind, level, marker
}

func formatWordNumberingValue(value int, format string) string {
	switch strings.ToLower(format) {
	case "lowerletter":
		return wordNumberingLetters(value, 'a')
	case "upperletter":
		return wordNumberingLetters(value, 'A')
	case "lowerroman":
		return strings.ToLower(wordNumberingRoman(value))
	case "upperroman":
		return wordNumberingRoman(value)
	case "decimalzero":
		return fmt.Sprintf("%02d", value)
	default:
		return strconv.Itoa(value)
	}
}

func wordNumberingLetters(value int, first rune) string {
	if value <= 0 {
		return strconv.Itoa(value)
	}
	var reversed []rune
	for value > 0 {
		value--
		reversed = append(reversed, first+rune(value%26))
		value /= 26
	}
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	return string(reversed)
}

func wordNumberingRoman(value int) string {
	if value <= 0 || value > 3999 {
		return strconv.Itoa(value)
	}
	var result strings.Builder
	for _, part := range []struct {
		value int
		text  string
	}{{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"}, {100, "C"}, {90, "XC"}, {50, "L"}, {40, "XL"}, {10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"}} {
		for value >= part.value {
			result.WriteString(part.text)
			value -= part.value
		}
	}
	return result.String()
}

func (p *wordProjector) paragraphListMaxLevel(node *wordXMLNode, kind document.WordListKind) int {
	if kind != document.WordListBullet && kind != document.WordListNumbered {
		return 0
	}
	numPr := firstChild(firstChild(node, "pPr"), "numPr")
	numID := attrValue(firstChild(numPr, "numId"), "val")
	numberingDefinition, ok := p.numbering[numID]
	if !ok {
		return 0
	}
	maximum := -1
	for level := 0; level <= 8; level++ {
		levelDefinition, exists := numberingDefinition.levels[level]
		if !exists || levelDefinition.kind != kind {
			break
		}
		maximum = level
	}
	return max(0, maximum)
}

func wordListBulletMarker(level int) string {
	markers := [...]string{"•", "◦", "▪"}
	if level < 0 {
		level = 0
	}
	return markers[level%len(markers)]
}

func wordNumberingValue(node *wordXMLNode, numID string, level, currentLevel, start int) int {
	value := start
	if node == nil || node.parent == nil {
		return value
	}
	for _, sibling := range node.parent.children {
		if sibling == node {
			break
		}
		if sibling.name.Local != "p" {
			continue
		}
		siblingNumID, siblingLevel := wordParagraphNumID(sibling)
		if siblingNumID != numID {
			continue
		}
		if siblingLevel == level {
			value++
		} else if siblingLevel >= 0 && siblingLevel < level {
			value = start
		}
	}
	if currentLevel > level && value > start {
		value--
	}
	return value
}

func wordParagraphNumID(paragraph *wordXMLNode) (string, int) {
	numPr := firstChild(firstChild(paragraph, "pPr"), "numPr")
	if numPr == nil {
		return "", -1
	}
	numID := attrValue(firstChild(numPr, "numId"), "val")
	level := 0
	if value := attrValue(firstChild(numPr, "ilvl"), "val"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return numID, -1
		}
		level = parsed
	}
	return numID, level
}

func parseWordIndentEMU(value string) int64 {
	if value == "" {
		return 0
	}
	twips, err := strconv.ParseInt(value, 10, 64)
	const maxTwips = int64(1<<63-1) / 635
	const minTwips = -maxTwips - 1
	if err != nil || twips > maxTwips || twips < minTwips {
		return 0
	}
	return twips * 635
}
