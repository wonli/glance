package docx

import (
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
	clone.Unsupported = append([]document.WordLayoutUnsupported(nil), data.Unsupported...)
	return clone
}

func projectWordLayoutData(parts []part, partIndexes map[string]int, relationships map[string]wordRelationship, root *wordXMLNode) document.WordLayoutData {
	data := document.WordLayoutData{Sections: make([]document.WordSection, 0, 1)}
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

	contentBlock := 0
	sectionStart := 0
	for _, child := range body.children {
		blockID := "b" + strconv.Itoa(contentBlock)
		switch child.name.Local {
		case "p":
			if properties := firstChild(child, "pPr"); properties != nil {
				if indentation := firstChild(properties, "ind"); indentation != nil && (attrValue(indentation, "firstLineChars") != "" || attrValue(indentation, "hangingChars") != "") {
					appendLayoutUnsupported(&data.Unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "character-indent", Description: "Character-based paragraph indents are not supported by page layout yet."})
				}
				if sectionProperties := firstChild(properties, "sectPr"); sectionProperties != nil {
					data.Sections = append(data.Sections, projectWordSection(sectionStart, sectionProperties, "section-"+strconv.Itoa(len(data.Sections)), &data.Unsupported))
					sectionStart = contentBlock + 1
				}
			}
			contentBlock++
		case "tbl":
			projectWordTableDiagnostics(child, blockID, &data.Unsupported)
			contentBlock++
		case "sectPr":
			data.Sections = append(data.Sections, projectWordSection(sectionStart, child, "section-"+strconv.Itoa(len(data.Sections)), &data.Unsupported))
			sectionStart = contentBlock
		default:
			contentBlock++
		}
	}
	if len(data.Sections) == 0 {
		data.Sections = append(data.Sections, document.WordSection{StartBlock: 0})
	}
	if len(data.Sections) > 1 {
		appendLayoutUnsupported(&data.Unsupported, document.WordLayoutUnsupported{
			SourceID: "section-1", Code: "multiple-sections",
			Description: "This document has multiple sections; section-specific pagination is not supported yet.",
		})
	}
	return data
}

func projectWordSection(startBlock int, node *wordXMLNode, sourceID string, unsupported *[]document.WordLayoutUnsupported) document.WordSection {
	section := document.WordSection{StartBlock: startBlock}
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
	}
	if columns := firstChild(node, "cols"); columns != nil {
		count := attrValue(columns, "num")
		if count == "" {
			count = "1"
		}
		if parsed, err := strconv.Atoi(count); err != nil || parsed != 1 {
			appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: sourceID, Code: "columns", Description: "Multi-column section layout is not supported yet."})
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

func parseWordTextStyleSet(properties *wordXMLNode) document.WordTextStyleMask {
	if properties == nil {
		return 0
	}
	var mask document.WordTextStyleMask
	if firstChild(properties, "b") != nil {
		mask |= document.WordTextBoldSet
	}
	if firstChild(properties, "i") != nil {
		mask |= document.WordTextItalicSet
	}
	if firstChild(properties, "u") != nil {
		mask |= document.WordTextUnderlineSet
	}
	if fonts := firstChild(properties, "rFonts"); fonts != nil {
		for _, name := range []string{"eastAsia", "ascii", "hAnsi", "cs"} {
			if attrValue(fonts, name) != "" {
				mask |= document.WordTextFontFamilySet
				break
			}
		}
	}
	if firstChild(properties, "sz") != nil {
		mask |= document.WordTextFontSizeSet
	}
	if firstChild(properties, "color") != nil {
		mask |= document.WordTextFontColorSet
	}
	if firstChild(properties, "highlight") != nil {
		mask |= document.WordTextHighlightSet
	}
	return mask
}

func parseWordParagraphLayout(properties *wordXMLNode) document.WordParagraphLayout {
	var layout document.WordParagraphLayout
	if properties == nil {
		return layout
	}
	if spacing := firstChild(properties, "spacing"); spacing != nil {
		if value := attrValue(spacing, "before"); value != "" {
			layout.BeforePT = wordTwipsToPoints(value)
			layout.Set |= document.WordParagraphBeforeSet
		}
		if value := attrValue(spacing, "after"); value != "" {
			layout.AfterPT = wordTwipsToPoints(value)
			layout.Set |= document.WordParagraphAfterSet
		}
		if value := attrValue(spacing, "line"); value != "" {
			layout.LineSpacingRule = strings.ToLower(attrValue(spacing, "lineRule"))
			if layout.LineSpacingRule == "" || layout.LineSpacingRule == "auto" {
				layout.LineSpacingRule = "auto"
				if amount, err := strconv.ParseFloat(value, 64); err == nil && amount >= 0 {
					layout.LineSpacingMultiple = amount / 240
					layout.Set |= document.WordParagraphLineSpacingSet
				}
			} else if layout.LineSpacingRule == "exact" || layout.LineSpacingRule == "atleast" {
				layout.LineSpacingPT = wordTwipsToPoints(value)
				layout.Set |= document.WordParagraphLineSpacingSet
			}
		}
	}
	if indentation := firstChild(properties, "ind"); indentation != nil {
		if value := attrValue(indentation, "firstLine"); value != "" {
			layout.FirstLineIndentEMU = parseWordIndentEMU(value)
			layout.Set |= document.WordParagraphFirstLineIndentSet
		} else if value := attrValue(indentation, "hanging"); value != "" {
			layout.HangingIndentEMU = parseWordIndentEMU(value)
			layout.Set |= document.WordParagraphHangingIndentSet
		}
		left := attrValue(indentation, "left")
		if left == "" {
			left = attrValue(indentation, "start")
		}
		if left != "" {
			layout.LeftIndentEMU = parseWordIndentEMU(left)
			layout.Set |= document.WordParagraphLeftIndentSet
		}
		right := attrValue(indentation, "right")
		if right == "" {
			right = attrValue(indentation, "end")
		}
		if right != "" {
			layout.RightIndentEMU = parseWordIndentEMU(right)
			layout.Set |= document.WordParagraphRightIndentSet
		}
	}
	if alignment := firstChild(properties, "jc"); alignment != nil {
		layout.Alignment = parseWordAlignment(attrValue(alignment, "val"))
		layout.Set |= document.WordParagraphAlignmentSet
	}
	if node := firstChild(properties, "keepNext"); node != nil {
		layout.KeepWithNext = toggleProperty(node)
		layout.Set |= document.WordParagraphKeepWithNextSet
	}
	if node := firstChild(properties, "keepLines"); node != nil {
		layout.KeepTogether = toggleProperty(node)
		layout.Set |= document.WordParagraphKeepTogetherSet
	}
	if node := firstChild(properties, "pageBreakBefore"); node != nil {
		layout.PageBreakBefore = toggleProperty(node)
		layout.Set |= document.WordParagraphPageBreakBeforeSet
	}
	if borders := firstChild(properties, "pBdr"); borders != nil {
		if bottom := firstChild(borders, "bottom"); bottom != nil {
			layout.BottomBorder = parseWordParagraphBorder(bottom)
			layout.Set |= document.WordParagraphBottomBorderSet
		}
	}
	return layout
}

func parseWordParagraphBorder(node *wordXMLNode) document.WordParagraphBorder {
	border := document.WordParagraphBorder{Style: strings.ToLower(attrValue(node, "val")), SizePT: 0.5}
	if border.Style == "" {
		border.Style = "single"
	}
	if value := attrValue(node, "sz"); value != "" {
		if eighths, err := strconv.ParseFloat(value, 64); err == nil && eighths >= 0 {
			border.SizePT = eighths / 8
		}
	}
	if value := attrValue(node, "space"); value != "" {
		if space, err := strconv.ParseFloat(value, 64); err == nil && space >= 0 {
			border.SpacePT = space
		}
	}
	border.Color = strings.ToUpper(attrValue(node, "color"))
	return border
}

func wordCellWidthEMU(width *wordXMLNode) int64 {
	if width == nil || strings.ToLower(attrValue(width, "type")) == "pct" || strings.ToLower(attrValue(width, "type")) == "auto" {
		return 0
	}
	return parseWordIndentEMU(attrValue(width, "w"))
}

func wordTwipsToPoints(value string) float64 {
	amount, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return float64(amount) / 20
}

func projectWordTableDiagnostics(table *wordXMLNode, blockID string, unsupported *[]document.WordLayoutUnsupported) {
	for _, child := range table.children {
		switch child.name.Local {
		case "tbl":
			appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "nested-table", Description: "Nested tables are retained but cannot be paginated faithfully yet."})
			projectWordTableDiagnostics(child, blockID, unsupported)
		case "gridSpan":
			span, err := strconv.Atoi(attrValue(child, "val"))
			if err != nil || span != 1 {
				appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "table-grid-span", Description: "Merged table cells are retained but are not supported by page layout yet."})
			}
		case "hMerge":
			appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "table-horizontal-merge", Description: "Horizontally merged table cells are retained but are not supported by page layout yet."})
		case "vMerge":
			appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "table-vertical-merge", Description: "Vertically merged table cells are retained but are not supported by page layout yet."})
		case "tcW":
			if strings.EqualFold(attrValue(child, "type"), "pct") {
				appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "table-percentage-width", Description: "Percentage-based table cell widths are not supported by page layout yet."})
			}
		case "tblHeader":
			appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "table-repeated-header", Description: "Repeating table header rows across pages is not supported yet."})
		default:
			projectWordTableDiagnostics(child, blockID, unsupported)
		}
	}
}

func appendLayoutUnsupported(items *[]document.WordLayoutUnsupported, item document.WordLayoutUnsupported) {
	for _, current := range *items {
		if current.SourceID == item.SourceID && current.Code == item.Code {
			return
		}
	}
	*items = append(*items, item)
}
