package docx

import (
	"strconv"
	"strings"

	"glance/internal/document"
)

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
			projectWordTableStructureDiagnostics(child, blockID, unsupported)
		case "gridSpan", "vMerge":
			// The table-level validator below accepts valid merge geometry and
			// reports malformed spans and continuations with located errors.
		case "hMerge":
			appendLayoutUnsupported(unsupported, document.WordLayoutUnsupported{SourceID: blockID, Code: "table-horizontal-merge", Description: "Horizontally merged table cells are retained but are not supported by page layout yet."})
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
		case "drawing", "pict", "object", "tab", "br", "sym", "fldChar", "noBreakHyphen", "softHyphen",
			"footnoteReference", "endnoteReference", "commentReference", "commentRangeStart", "commentRangeEnd":
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
