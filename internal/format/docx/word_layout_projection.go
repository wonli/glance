package docx

import (
	"math"
	"strconv"
	"strings"

	"glance/internal/document"
)

func (d *Document) WordLayoutData() document.WordLayoutData {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return document.WordLayoutData{}
	}
	return cloneWordLayoutData(d.wordLayoutData)
}

func cloneWordLayoutData(data document.WordLayoutData) document.WordLayoutData {
	clone := data
	clone.Sections = append([]document.WordSection(nil), data.Sections...)
	clone.Styles = append([]document.WordStyle(nil), data.Styles...)
	clone.Defaults.Paragraph.TabStops = append([]document.WordTabStop(nil), data.Defaults.Paragraph.TabStops...)
	for index := range clone.Styles {
		clone.Styles[index].Paragraph.TabStops = append([]document.WordTabStop(nil), data.Styles[index].Paragraph.TabStops...)
	}
	clone.Unsupported = append([]document.WordLayoutUnsupported(nil), data.Unsupported...)
	return clone
}

func projectWordLayoutData(parts []part, partIndexes map[string]int, relationships map[string]wordRelationship, root *wordXMLNode) document.WordLayoutData {
	data := document.WordLayoutData{Sections: make([]document.WordSection, 0, 1), DefaultTabStopPT: 36}
	body := firstChild(root, "body")
	if body == nil {
		return data
	}
	stylePart := ""
	for _, relationship := range relationships {
		if relationship.styles && !relationship.external {
			stylePart = relationship.target
			break
		}
	}
	if index, ok := partIndexes[stylePart]; ok && stylePart != "" {
		defaults, styles, err := readWordLayoutStyles(parts[index].data)
		if err != nil {
			appendLayoutUnsupported(&data.Unsupported, document.WordLayoutUnsupported{SourceID: stylePart, Code: "invalid-styles", Description: "The DOCX styles part could not be parsed; page layout cannot resolve style defaults."})
		} else {
			data.Defaults, data.Styles = defaults, styles
		}
	} else if stylePart != "" {
		appendLayoutUnsupported(&data.Unsupported, document.WordLayoutUnsupported{SourceID: stylePart, Code: "missing-styles", Description: "The DOCX package references a missing styles part; page layout cannot resolve style defaults."})
	}
	for _, relationship := range relationships {
		if !relationship.settings || relationship.external {
			continue
		}
		if index, ok := partIndexes[relationship.target]; ok {
			settings, err := parseWordXML(parts[index].data)
			if err == nil {
				if stop := firstChild(settings, "defaultTabStop"); stop != nil {
					if twips, parseErr := strconv.ParseFloat(attrValue(stop, "val"), 64); parseErr == nil && twips > 0 {
						data.DefaultTabStopPT = twips / 20
					}
				}
			}
		}
		break
	}

	contentBlock := 0
	sectionStart := 0
	for _, child := range body.children {
		blockID := "b" + strconv.Itoa(contentBlock)
		switch child.name.Local {
		case "p":
			projectWordParagraphLayoutDiagnostics(parts, partIndexes, relationships, child, blockID, contentBlock, &sectionStart, &data)
			contentBlock++
		case "tbl":
			projectWordTableDiagnostics(child, blockID, &data.Unsupported)
			contentBlock++
		case "sdt":
			content := firstChild(child, "sdtContent")
			if content == nil {
				contentBlock++
				continue
			}
			for contentIndex, contentChild := range content.children {
				if isWordRangeMarker(contentChild.name.Local) {
					continue
				}
				contentBlockID := blockID + ".c" + strconv.Itoa(contentIndex)
				switch contentChild.name.Local {
				case "p":
					projectWordParagraphLayoutDiagnostics(parts, partIndexes, relationships, contentChild, contentBlockID, contentBlock, &sectionStart, &data)
				case "tbl":
					projectWordTableDiagnostics(contentChild, contentBlockID, &data.Unsupported)
				}
				contentBlock++
			}
		case "sectPr":
			data.Sections = append(data.Sections, projectWordSection(parts, partIndexes, relationships, sectionStart, child, "section-"+strconv.Itoa(len(data.Sections)), &data.Unsupported))
			sectionStart = contentBlock
		case "bookmarkStart", "bookmarkEnd", "proofErr", "commentRangeStart", "commentRangeEnd", "permStart", "permEnd":
			// Range markers do not create body blocks in the content projection.
			continue
		default:
			contentBlock++
		}
	}
	if len(data.Sections) == 0 {
		data.Sections = append(data.Sections, document.WordSection{StartBlock: 0})
	}
	for index := 1; index < len(data.Sections); index++ {
		if !data.Sections[index].HeaderSet {
			data.Sections[index].HeaderText = data.Sections[index-1].HeaderText
			data.Sections[index].HeaderDistancePT = data.Sections[index-1].HeaderDistancePT
			data.Sections[index].HeaderSet = data.Sections[index-1].HeaderSet
		}
	}
	if sectionPageSetupsDiffer(data.Sections) {
		appendLayoutUnsupported(&data.Unsupported, document.WordLayoutUnsupported{
			SourceID: "section-1", Code: "multiple-sections",
			Description: "This document has multiple sections; section-specific pagination is not supported yet.",
		})
	}
	return data
}

func projectWordParagraphLayoutDiagnostics(parts []part, partIndexes map[string]int, relationships map[string]wordRelationship, paragraph *wordXMLNode, sourceID string, blockIndex int, sectionStart *int, data *document.WordLayoutData) {
	properties := firstChild(paragraph, "pPr")
	if properties == nil {
		return
	}
	if indentation := firstChild(properties, "ind"); hasInvalidCharacterIndent(indentation) {
		appendLayoutUnsupported(&data.Unsupported, document.WordLayoutUnsupported{SourceID: sourceID, Code: "invalid-character-indent", Description: "A character-based paragraph indent is malformed and cannot be laid out."})
	}
	if sectionProperties := firstChild(properties, "sectPr"); sectionProperties != nil {
		data.Sections = append(data.Sections, projectWordSection(parts, partIndexes, relationships, *sectionStart, sectionProperties, "section-"+strconv.Itoa(len(data.Sections)), &data.Unsupported))
		*sectionStart = blockIndex + 1
	}
}

func hasInvalidCharacterIndent(indentation *wordXMLNode) bool {
	if indentation == nil {
		return false
	}
	for _, name := range []string{"firstLineChars", "hangingChars"} {
		value, present := attrValuePresent(indentation, name)
		if !present {
			continue
		}
		if _, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err != nil {
			return true
		}
	}
	return false
}

func sectionPageSetupsDiffer(sections []document.WordSection) bool {
	if len(sections) < 2 {
		return false
	}
	first := sections[0].PageSetup
	for _, section := range sections[1:] {
		current := section.PageSetup
		if current.Orientation != first.Orientation || current.Columns != first.Columns ||
			!wordLayoutValuesEquivalentWithinTwip(current.WidthPT, first.WidthPT) ||
			!wordLayoutValuesEquivalentWithinTwip(current.HeightPT, first.HeightPT) ||
			!wordLayoutValuesEquivalentWithinTwip(current.MarginTopPT, first.MarginTopPT) ||
			!wordLayoutValuesEquivalentWithinTwip(current.MarginRightPT, first.MarginRightPT) ||
			!wordLayoutValuesEquivalentWithinTwip(current.MarginBottomPT, first.MarginBottomPT) ||
			!wordLayoutValuesEquivalentWithinTwip(current.MarginLeftPT, first.MarginLeftPT) ||
			!wordLayoutValuesEquivalentWithinTwip(current.ColumnSpacingPT, first.ColumnSpacingPT) {
			return true
		}
	}
	return false
}

func wordLayoutValuesEquivalentWithinTwip(left, right float64) bool {
	return math.Abs(left-right) <= 0.051
}

func projectWordSection(parts []part, partIndexes map[string]int, relationships map[string]wordRelationship, startBlock int, node *wordXMLNode, sourceID string, unsupported *[]document.WordLayoutUnsupported) document.WordSection {
	section := document.WordSection{StartBlock: startBlock, StartType: "nextpage", HeaderDistancePT: 36}
	if sectionType := firstChild(node, "type"); sectionType != nil {
		if value := strings.TrimSpace(attrValue(sectionType, "val")); value != "" {
			section.StartType = strings.ToLower(value)
		}
	}
	pageSize := firstChild(node, "pgSz")
	if pageSize != nil {
		if value := attrValue(pageSize, "w"); value != "" {
			section.PageSetup.WidthPT = wordTwipsToPoints(value)
			section.PageSetup.Set |= document.WordPageWidthSet
		}
		if value := attrValue(pageSize, "h"); value != "" {
			section.PageSetup.HeightPT = wordTwipsToPoints(value)
			section.PageSetup.Set |= document.WordPageHeightSet
		}
		if value, present := attrValuePresent(pageSize, "orient"); present {
			section.PageSetup.Orientation = strings.ToLower(value)
			section.PageSetup.Set |= document.WordPageOrientationSet
		}
		if section.PageSetup.WidthPT <= 0 || section.PageSetup.HeightPT <= 0 {
			appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: sourceID, Code: "invalid-page-size", Description: "The section has an invalid or incomplete page size."})
		}
		if section.PageSetup.Set&document.WordPageOrientationSet != 0 && section.PageSetup.Orientation != "portrait" && section.PageSetup.Orientation != "landscape" {
			appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: sourceID, Code: "unknown-orientation", Description: "The section uses an unsupported page orientation: " + section.PageSetup.Orientation})
		}
	}
	if margins := firstChild(node, "pgMar"); margins != nil {
		projectMargin := func(name string, target *float64, mask document.WordPageSetupMask) {
			if value := attrValue(margins, name); value != "" {
				*target = wordTwipsToPoints(value)
				section.PageSetup.Set |= mask
			}
		}
		projectMargin("top", &section.PageSetup.MarginTopPT, document.WordPageMarginTopSet)
		projectMargin("right", &section.PageSetup.MarginRightPT, document.WordPageMarginRightSet)
		projectMargin("bottom", &section.PageSetup.MarginBottomPT, document.WordPageMarginBottomSet)
		projectMargin("left", &section.PageSetup.MarginLeftPT, document.WordPageMarginLeftSet)
		if value := attrValue(margins, "header"); value != "" {
			section.HeaderDistancePT = wordTwipsToPoints(value)
		} else {
			section.HeaderDistancePT = 36
		}
	}
	for _, child := range node.children {
		if child.name.Local != "headerReference" {
			continue
		}
		headerType := strings.ToLower(strings.TrimSpace(attrValue(child, "type")))
		if headerType != "" && headerType != "default" {
			continue
		}
		section.HeaderSet = true
		relationship := relationships[attrValue(child, "id")]
		if !relationship.header || relationship.external {
			continue
		}
		if index, ok := partIndexes[relationship.target]; ok {
			headerRoot, err := parseWordXML(parts[index].data)
			if err == nil {
				section.HeaderText = strings.Join(wordElementParagraphs(headerRoot), "\n")
			}
		}
	}
	if columns := firstChild(node, "cols"); columns != nil {
		count := attrValue(columns, "num")
		if count == "" {
			count = "1"
		}
		space := attrValue(columns, "space")
		parsed, err := strconv.Atoi(count)
		if err != nil || parsed < 1 || parsed > 4 {
			appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: sourceID, Code: "columns", Description: "Column count must be between 1 and 4."})
		} else {
			section.PageSetup.Columns = parsed
			section.PageSetup.Set |= document.WordPageColumnsSet
			if space != "" {
				section.PageSetup.ColumnSpacingPT = wordTwipsToPoints(space)
				section.PageSetup.Set |= document.WordPageColumnSpacingSet
			} else {
				section.PageSetup.ColumnSpacingPT = 36.0
			}
		}
	}
	return section
}

func readWordLayoutStyles(data []byte) (document.WordStyleDefaults, []document.WordStyle, error) {
	root, err := parseWordXML(data)
	if err != nil {
		return document.WordStyleDefaults{}, nil, err
	}
	var defaults document.WordStyleDefaults
	if node := firstChild(root, "docDefaults"); node != nil {
		defaults.Text = parseWordTextStyle(firstChild(firstChild(node, "rPrDefault"), "rPr"))
		defaults.TextSet = parseWordTextStyleSet(firstChild(firstChild(node, "rPrDefault"), "rPr"))
		defaults.Paragraph = parseWordParagraphLayout(firstChild(firstChild(node, "pPrDefault"), "pPr"))
	}
	styles := make([]document.WordStyle, 0)
	for _, node := range root.children {
		if node.name.Local != "style" {
			continue
		}
		id := attrValue(node, "styleId")
		if id == "" {
			continue
		}
		style := document.WordStyle{
			ID:               id,
			BasedOn:          attrValue(firstChild(node, "basedOn"), "val"),
			DefaultParagraph: strings.EqualFold(attrValue(node, "type"), "paragraph") && isWordTrue(attrValue(node, "default")),
		}
		textProperties := firstChild(node, "rPr")
		style.Text = parseWordTextStyle(textProperties)
		style.TextSet = parseWordTextStyleSet(textProperties)
		style.Paragraph = parseWordParagraphLayout(firstChild(node, "pPr"))
		styles = append(styles, style)
	}
	return defaults, styles, nil
}

func isWordTrue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "on":
		return true
	default:
		return false
	}
}
