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
			projectWordParagraphLayoutDiagnostics(child, blockID, contentBlock, &sectionStart, &data)
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
					projectWordParagraphLayoutDiagnostics(contentChild, contentBlockID, contentBlock, &sectionStart, &data)
				case "tbl":
					projectWordTableDiagnostics(contentChild, contentBlockID, &data.Unsupported)
				}
				contentBlock++
			}
		case "sectPr":
			data.Sections = append(data.Sections, projectWordSection(sectionStart, child, "section-"+strconv.Itoa(len(data.Sections)), &data.Unsupported))
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
	if sectionPageSetupsDiffer(data.Sections) {
		appendLayoutUnsupported(&data.Unsupported, document.WordLayoutUnsupported{
			SourceID: "section-1", Code: "multiple-sections",
			Description: "This document has multiple sections; section-specific pagination is not supported yet.",
		})
	}
	return data
}

func projectWordParagraphLayoutDiagnostics(paragraph *wordXMLNode, sourceID string, blockIndex int, sectionStart *int, data *document.WordLayoutData) {
	properties := firstChild(paragraph, "pPr")
	if properties == nil {
		return
	}
	if indentation := firstChild(properties, "ind"); hasNonzeroCharacterIndent(indentation) {
		appendLayoutUnsupported(&data.Unsupported, document.WordLayoutUnsupported{SourceID: sourceID, Code: "character-indent", Description: "Nonzero character-based paragraph indents are not supported by page layout yet."})
	}
	if sectionProperties := firstChild(properties, "sectPr"); sectionProperties != nil {
		data.Sections = append(data.Sections, projectWordSection(*sectionStart, sectionProperties, "section-"+strconv.Itoa(len(data.Sections)), &data.Unsupported))
		*sectionStart = blockIndex + 1
	}
}

func hasNonzeroCharacterIndent(indentation *wordXMLNode) bool {
	if indentation == nil {
		return false
	}
	for _, name := range []string{"firstLineChars", "hangingChars"} {
		value, present := attrValuePresent(indentation, name)
		if !present {
			continue
		}
		amount, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil || amount != 0 {
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
		if section.PageSetup != first {
			return true
		}
	}
	return false
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
		if value, present := attrValuePresent(indentation, "firstLineChars"); present {
			if amount, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && amount == 0 {
				layout.FirstLineIndentEMU = 0
				layout.Set |= document.WordParagraphFirstLineIndentSet
			}
		} else if value, present := attrValuePresent(indentation, "hangingChars"); present {
			if amount, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && amount == 0 {
				layout.HangingIndentEMU = 0
				layout.Set |= document.WordParagraphHangingIndentSet
			}
		} else if value := attrValue(indentation, "firstLine"); value != "" {
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
	if tabs := firstChild(properties, "tabs"); tabs != nil {
		layout.Set |= document.WordParagraphTabStopsSet
		for _, tab := range tabs.children {
			if tab.name.Local != "tab" {
				continue
			}
			position, err := strconv.ParseFloat(attrValue(tab, "pos"), 64)
			if err != nil || position < 0 {
				continue
			}
			alignment := strings.ToLower(attrValue(tab, "val"))
			if alignment == "" {
				alignment = "left"
			}
			layout.TabStops = append(layout.TabStops, document.WordTabStop{
				PositionPT: position / 20,
				Alignment:  alignment,
				Leader:     strings.ToLower(attrValue(tab, "leader")),
			})
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

func projectedWordGridSpan(properties *wordXMLNode) int {
	spanNode := firstChild(properties, "gridSpan")
	if spanNode == nil {
		return 0
	}
	value, present := attrValuePresent(spanNode, "val")
	span, err := strconv.Atoi(strings.TrimSpace(value))
	if !present || err != nil || span <= 0 {
		return 0
	}
	return span
}

func projectedWordVerticalMerge(properties *wordXMLNode) document.WordTableVerticalMerge {
	mergeNode := firstChild(properties, "vMerge")
	if mergeNode == nil {
		return document.WordTableVerticalMergeNone
	}
	value, present := attrValuePresent(mergeNode, "val")
	if !present || strings.TrimSpace(value) == "" || strings.EqualFold(strings.TrimSpace(value), "continue") {
		return document.WordTableVerticalMergeContinue
	}
	if strings.EqualFold(strings.TrimSpace(value), "restart") {
		return document.WordTableVerticalMergeRestart
	}
	return document.WordTableVerticalMergeNone
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
	projectWordTableStructureDiagnostics(table, blockID, unsupported)
	projectWordTableMergeDiagnostics(table, blockID, unsupported)
}

func projectWordTableStructureDiagnostics(node *wordXMLNode, blockID string, unsupported *[]document.WordLayoutUnsupported) {
	if node == nil {
		return
	}
	for _, child := range node.children {
		switch child.name.Local {
		case "tbl":
			appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "nested-table", Description: "Nested tables are retained but cannot be paginated faithfully yet."})
			projectWordTableStructureDiagnostics(child, blockID, unsupported)
		case "gridSpan", "vMerge":
			// The table-level validator below accepts valid merge geometry and
			// reports malformed spans and continuations with located errors.
		case "hMerge":
			appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "table-horizontal-merge", Description: "Horizontally merged table cells are retained but are not supported by page layout yet."})
		case "tcW":
			if strings.EqualFold(attrValue(child, "type"), "pct") {
				appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "table-percentage-width", Description: "Percentage-based table cell widths are not supported by page layout yet."})
			}
		case "tblHeader":
			appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "table-repeated-header", Description: "Repeating table header rows across pages is not supported yet."})
		default:
			projectWordTableStructureDiagnostics(child, blockID, unsupported)
		}
	}
}

func projectWordTableMergeDiagnostics(table *wordXMLNode, blockID string, unsupported *[]document.WordLayoutUnsupported) {
	gridColumns := 0
	if grid := firstChild(table, "tblGrid"); grid != nil {
		for _, child := range grid.children {
			if child.name.Local == "gridCol" {
				gridColumns++
			}
		}
	}

	activeMerges := make(map[int]int)
	for _, row := range table.children {
		if row.name.Local != "tr" {
			continue
		}
		nextMerges := make(map[int]int)
		gridColumn := 0
		for _, cell := range row.children {
			if cell.name.Local != "tc" {
				continue
			}
			properties := firstChild(cell, "tcPr")
			span := 1
			spanNode := firstChild(properties, "gridSpan")
			if spanNode != nil {
				value, present := attrValuePresent(spanNode, "val")
				parsed, err := strconv.Atoi(strings.TrimSpace(value))
				if !present || err != nil || parsed <= 0 {
					appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "table-grid-span", Description: "A table cell has an invalid grid span; page layout cannot place its columns faithfully."})
				} else {
					span = parsed
				}
			}
			if gridColumns > 0 && gridColumn+span > gridColumns {
				appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "table-grid-span", Description: "A table cell span extends beyond the declared table grid; page layout cannot place its columns faithfully."})
			}

			mergeNode := firstChild(properties, "vMerge")
			merge := projectedWordVerticalMerge(properties)
			if mergeNode != nil && merge == document.WordTableVerticalMergeNone {
				appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "table-vertical-merge", Description: "A table cell has an invalid vertical-merge value; page layout cannot place its merge faithfully."})
			}
			switch merge {
			case document.WordTableVerticalMergeRestart:
				nextMerges[gridColumn] = span
			case document.WordTableVerticalMergeContinue:
				if activeSpan, ok := activeMerges[gridColumn]; !ok || activeSpan != span {
					appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "table-vertical-merge", Description: "A vertical-merge continuation has no matching restart at the same grid columns; page layout cannot place the merge faithfully."})
				}
				if wordTableCellHasContent(cell) {
					appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "table-vertical-merge", Description: "A vertical-merge continuation cell contains content; page layout cannot duplicate it faithfully."})
				}
				nextMerges[gridColumn] = span
			}
			gridColumn += span
		}
		activeMerges = nextMerges
	}
}

func wordTableCellHasContent(cell *wordXMLNode) bool {
	if cell == nil {
		return false
	}
	var visit func(*wordXMLNode) bool
	visit = func(node *wordXMLNode) bool {
		switch node.name.Local {
		case "t", "delText", "instrText":
			if node.text != "" {
				return true
			}
		case "drawing", "pict", "object", "tab", "br", "sym", "fldChar", "noBreakHyphen", "softHyphen":
			return true
		}
		for _, child := range node.children {
			if child.name.Local == "tcPr" {
				continue
			}
			if visit(child) {
				return true
			}
		}
		return false
	}
	for _, child := range cell.children {
		if child.name.Local != "tcPr" && visit(child) {
			return true
		}
	}
	return false
}

func appendLayoutUnsupported(items *[]document.WordLayoutUnsupported, item document.WordLayoutUnsupported) {
	for _, current := range *items {
		if current.SourceID == item.SourceID && current.Code == item.Code {
			return
		}
	}
	*items = append(*items, item)
}
