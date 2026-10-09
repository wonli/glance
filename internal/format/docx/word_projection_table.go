package docx

import (
	"fmt"
	"glance/internal/document"
	"strings"
)

func (p *wordProjector) table(node *wordXMLNode, id string) document.WordTable {
	table := document.WordTable{}
	table.Layout.FillColor = wordShadingFill(firstChild(node, "tblPr"))
	rowIndex := 0
	for _, child := range node.children {
		if child.name.Local == "tblGrid" {
			for _, gridColumn := range child.children {
				if gridColumn.name.Local == "gridCol" {
					table.Layout.GridColumnsEMU = append(table.Layout.GridColumnsEMU, parseWordIndentEMU(attrValue(gridColumn, "w")))
				}
			}
			continue
		}
		if child.name.Local != "tr" {
			continue
		}
		row := document.WordTableRow{}
		if rowProperties := firstChild(child, "trPr"); rowProperties != nil {
			if cantSplit := firstChild(rowProperties, "cantSplit"); cantSplit != nil {
				value, present := attrValuePresent(cantSplit, "val")
				row.CantSplit = !present || isWordTrue(value)
			}
		}
		cellIndex := 0
		for _, cellNode := range child.children {
			if cellNode.name.Local != "tc" {
				continue
			}
			cellID := fmt.Sprintf("%s.r%d.c%d", id, rowIndex, cellIndex)
			cellProperties := firstChild(cellNode, "tcPr")
			cell := document.WordTableCell{
				ID:            cellID,
				WidthEMU:      wordCellWidthEMU(firstChild(cellProperties, "tcW")),
				WidthFallback: strings.EqualFold(attrValue(firstChild(cellProperties, "tcW"), "type"), "pct"),
				GridSpan:      projectedWordGridSpan(cellProperties),
				VerticalMerge: projectedWordVerticalMerge(cellProperties),
				FillColor:     wordShadingFill(cellProperties),
			}
			if cell.FillColor == "" {
				cell.FillColor = table.Layout.FillColor
			}
			paragraphIndex := 0
			nestedTableIndex := 0
			for _, content := range cellNode.children {
				switch content.name.Local {
				case "tcPr":
					continue
				case "p":
					cell.Paragraphs = append(cell.Paragraphs, p.paragraph(content, fmt.Sprintf("%s.p%d", cellID, paragraphIndex)))
					paragraphIndex++
				case "tbl":
					nestedID := fmt.Sprintf("%s.n%d", cellID, nestedTableIndex)
					nestedTable := p.table(content, nestedID)
					for _, nestedRow := range nestedTable.Rows {
						for _, nestedCell := range nestedRow.Cells {
							for _, paragraph := range nestedCell.Paragraphs {
								paragraph.PreviewWarning = appendWordPreviewWarning(paragraph.PreviewWarning, "Nested table structure is flattened into the parent cell for preview.")
								p.makeParagraphReadOnly(&paragraph)
								cell.Paragraphs = append(cell.Paragraphs, paragraph)
								paragraphIndex++
							}
						}
					}
					nestedTableIndex++
				default:
					paragraph := document.WordParagraph{ID: fmt.Sprintf("%s.p%d", cellID, paragraphIndex), Alignment: document.WordAlignDefault}
					paragraph.Inlines = append(paragraph.Inlines, unsupportedInline(paragraph.ID+".i0", "Unsupported table-cell content: "+content.name.Local))
					cell.Paragraphs = append(cell.Paragraphs, paragraph)
					paragraphIndex++
				}
			}
			row.Cells = append(row.Cells, cell)
			cellIndex++
		}
		table.Rows = append(table.Rows, row)
		rowIndex++
	}
	return table
}

func wordShadingFill(properties *wordXMLNode) string {
	shading := firstChild(properties, "shd")
	if shading == nil {
		return ""
	}
	if value := strings.ToLower(attrValue(shading, "val")); value == "nil" || value == "none" {
		return ""
	}
	fill := strings.TrimSpace(attrValue(shading, "fill"))
	if (fill == "" || strings.EqualFold(fill, "auto")) && strings.EqualFold(attrValue(shading, "val"), "solid") {
		fill = strings.TrimSpace(attrValue(shading, "color"))
	}
	if len(fill) != 6 {
		return ""
	}
	for _, digit := range fill {
		if !((digit >= '0' && digit <= '9') || (digit >= 'a' && digit <= 'f') || (digit >= 'A' && digit <= 'F')) {
			return ""
		}
	}
	return strings.ToUpper(fill)
}
