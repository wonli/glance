package wordlayout

import (
	"context"
	"math"

	"glance/internal/document"
)

const tableCellPaddingPT = 2.0
const maxTableGridColumns = 512

type preparedTableCell struct {
	cell        document.WordTableCell
	paragraphs  []preparedParagraph
	lines       []preparedTableLine
	keepGroups  map[int]tableKeepGroup
	contentH    float64
	gridColumn  int
	gridSpan    int
	mergeEndRow int
}

func (c *layoutCursor) layoutTable(ctx context.Context, shaper TextShaper, styles *styleResolver, tableID string, table document.WordTable, area Rect, editorInsets bool) error {
	if len(table.Rows) == 0 {
		return nil
	}
	if !c.validateTableMergeGeometry(tableID, table) {
		return nil
	}
	widths, inferredWidths := tableColumnWidths(table, area.WidthPT)
	if inferredWidths {
		c.diagnostic(DiagnosticWarning, tableID, "table-grid-column-width-inferred", "One or more table grid column widths are missing; inferred widths require review before output.")
	}
	rows := make([]preparedTableRow, len(table.Rows))
	for rowIndex, sourceRow := range table.Rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		row := preparedTableRow{index: rowIndex, height: tableCellPaddingPT * 2}
		gridColumn := 0
		for _, cell := range sourceRow.Cells {
			if cell.WidthFallback {
				c.diagnostic(DiagnosticWarning, tableCellSourceID(tableID, cell), "table-percentage-width-fallback", "A percentage-based cell width uses the declared table grid and available parent width for preview.")
			}
			span := tableCellGridSpan(cell)
			cellWidth, ok := tableGridSpanWidth(widths, gridColumn, span)
			if !ok {
				c.diagnostic(DiagnosticError, tableCellSourceID(tableID, cell), "table-grid-span", "A table cell span extends beyond the available grid columns.")
				return nil
			}
			preparedCell := preparedTableCell{cell: cell, gridColumn: gridColumn, gridSpan: span, mergeEndRow: rowIndex}
			innerWidth := math.Max(1, cellWidth-tableCellPaddingPT*2)
			for _, paragraph := range cell.Paragraphs {
				prepared, err := prepareParagraph(ctx, shaper, styles, paragraph, paragraph.ID, innerWidth, true, editorInsets)
				if err != nil {
					return err
				}
				for _, line := range prepared.lines {
					if line.pageBreakBefore || line.pageBreakAfter {
						c.diagnostic(DiagnosticError, paragraph.ID, "table-cell-page-break", "Page breaks inside a table row are not supported.")
					}
				}
				preparedCell.paragraphs = append(preparedCell.paragraphs, prepared)
			}
			preparedCell.lines, preparedCell.keepGroups = prepareTableCellLines(preparedCell.paragraphs)
			preparedCell.contentH = preparedParagraphsHeight(preparedCell.paragraphs)
			if cell.VerticalMerge == document.WordTableVerticalMergeNone {
				row.height = math.Max(row.height, preparedCell.contentH+tableCellPaddingPT*2)
			}
			row.cells = append(row.cells, preparedCell)
			gridColumn += span
		}
		rows[rowIndex] = row
	}
	for rowIndex := range rows {
		for cellIndex := range rows[rowIndex].cells {
			cell := &rows[rowIndex].cells[cellIndex]
			if cell.cell.VerticalMerge != document.WordTableVerticalMergeRestart {
				continue
			}
			for nextRow := rowIndex + 1; nextRow < len(rows); nextRow++ {
				continuation := tableMergeContinuation(rows[nextRow], cell.gridColumn, cell.gridSpan)
				if continuation == nil {
					break
				}
				cell.mergeEndRow = nextRow
			}
			mergedHeight := tableRowsHeight(rows, rowIndex, cell.mergeEndRow+1)
			requiredHeight := cell.contentH + tableCellPaddingPT*2
			precedingRowsHeight := tableRowsHeight(rows, rowIndex, cell.mergeEndRow)
			if requiredHeight > mergedHeight {
				extraHeight := (requiredHeight - mergedHeight) / float64(cell.mergeEndRow-rowIndex+1)
				for mergeRow := rowIndex; mergeRow <= cell.mergeEndRow; mergeRow++ {
					rows[mergeRow].height += extraHeight
					rows[mergeRow].mergeExtraHeight += extraHeight
				}
			}
			if cell.mergeEndRow > rowIndex && requiredHeight > precedingRowsHeight+epsilonPT {
				if len(cell.lines) > 0 {
					rows[cell.mergeEndRow].mergeTailReserve = math.Max(rows[cell.mergeEndRow].mergeTailReserve,
						cell.lines[len(cell.lines)-1].height+tableCellPaddingPT)
				}
			}
		}
	}
	for rowIndex := range rows {
		row := &rows[rowIndex]
		rowID := tableID + ".r" + itoa(rowIndex)
		if !c.layoutTableRow(ctx, row, rowID, table.Rows[rowIndex].CantSplit) {
			if err := ctx.Err(); err != nil {
				return err
			}
			return nil
		}
	}
	for rowIndex, row := range rows {
		for cellIndex, cell := range row.cells {
			if cell.cell.VerticalMerge == document.WordTableVerticalMergeContinue {
				continue
			}
			cellWidth, ok := tableGridSpanWidth(widths, cell.gridColumn, cell.gridSpan)
			if !ok {
				c.diagnostic(DiagnosticError, tableCellSourceID(tableID, cell.cell), "table-grid-span", "A table cell span extends beyond the available grid columns.")
				return nil
			}
			if cell.cell.VerticalMerge == document.WordTableVerticalMergeRestart {
				segments := verticalMergePageSegments(rows, rowIndex, cell, widths)
				if err := c.placeVerticalMergeCellText(ctx, tableID, cell, cellWidth, segments); err != nil {
					return err
				}
				continue
			}
			for _, fragment := range row.fragments {
				inner := Rect{
					XPT: fragment.x + tableGridColumnOffset(widths, cell.gridColumn) + tableCellPaddingPT,
					YPT: fragment.y, WidthPT: math.Max(1, cellWidth-tableCellPaddingPT*2),
				}
				for _, placement := range fragment.cells[cellIndex] {
					line := placement.line
					borderSourceID := line.paragraph.borderSourceID
					if borderSourceID == "" {
						borderSourceID = line.paragraph.sourceID
					}
					placeParagraphLine(c, line.paragraph.sourceID, borderSourceID, fragment.pageIndex,
						fragment.y+placement.offset+line.leading, inner, line.paragraph.style.paragraph,
						line.paragraph.lines[line.lineIndex], line.lineIndex == 0,
						line.lineIndex == len(line.paragraph.lines)-1)
				}
			}
		}
	}
	for rowIndex := range rows {
		row := &rows[rowIndex]
		for _, cell := range row.cells {
			if cell.cell.VerticalMerge == document.WordTableVerticalMergeContinue {
				continue
			}
			cellWidth, ok := tableGridSpanWidth(widths, cell.gridColumn, cell.gridSpan)
			if !ok {
				continue
			}
			fillColor := cell.cell.FillColor
			if fillColor == "" {
				fillColor = table.Layout.FillColor
			}
			if cell.cell.VerticalMerge == document.WordTableVerticalMergeRestart {
				c.appendVerticalMergeSegments(tableID, rows, rowIndex, cell, widths, fillColor)
				continue
			}
			for _, fragment := range row.fragments {
				cellX := fragment.x + tableGridColumnOffset(widths, cell.gridColumn)
				c.result.Pages[fragment.pageIndex].TableCells = append(c.result.Pages[fragment.pageIndex].TableCells, TableCellPlacement{
					TableID: tableID, RowIndex: row.index, CellID: cell.cell.ID, FillColor: fillColor,
					Bounds:        Rect{XPT: cellX, YPT: fragment.y, WidthPT: cellWidth, HeightPT: fragment.height},
					DrawTopBorder: true, DrawBottomBorder: true, DrawLeftBorder: true, DrawRightBorder: true,
				})
			}
		}
	}
	return nil
}
