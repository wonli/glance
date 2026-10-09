package wordlayout

import (
	"context"
	"glance/internal/document"
	"math"
)

type tableCellPageSegment struct {
	pageIndex int
	column    int
	bounds    Rect
}

func (c *layoutCursor) validateTableMergeGeometry(tableID string, table document.WordTable) bool {
	valid := true
	gridColumns := len(table.Layout.GridColumnsEMU)
	activeMerges := make(map[int]int)
	for _, row := range table.Rows {
		nextMerges := make(map[int]int)
		gridColumn := 0
		for _, cell := range row.Cells {
			span := tableCellGridSpan(cell)
			sourceID := tableCellSourceID(tableID, cell)
			if cell.GridSpan < 0 || span > maxTableGridColumns || (gridColumns > 0 && span > gridColumns-gridColumn) {
				c.diagnostic(DiagnosticError, sourceID, "table-grid-span", "A table cell span is invalid or extends beyond the declared table grid.")
				valid = false
			}
			if cell.VerticalMerge > document.WordTableVerticalMergeContinue {
				c.diagnostic(DiagnosticError, sourceID, "table-vertical-merge", "A table cell has an unknown vertical-merge state.")
				valid = false
			}
			switch cell.VerticalMerge {
			case document.WordTableVerticalMergeRestart:
				nextMerges[gridColumn] = span
			case document.WordTableVerticalMergeContinue:
				if activeSpan, ok := activeMerges[gridColumn]; !ok || activeSpan != span {
					c.diagnostic(DiagnosticError, sourceID, "table-vertical-merge", "A vertical-merge continuation has no matching restart at the same grid columns.")
					valid = false
				}
				if tableCellHasContent(cell) {
					c.diagnostic(DiagnosticError, sourceID, "table-vertical-merge", "A vertical-merge continuation cell contains content.")
					valid = false
				}
				nextMerges[gridColumn] = span
			}
			gridColumn += span
		}
		activeMerges = nextMerges
	}
	return valid
}

func tableCellHasContent(cell document.WordTableCell) bool {
	for _, paragraph := range cell.Paragraphs {
		for _, inline := range paragraph.Inlines {
			if (inline.Text != nil && inline.Text.Text != "") || inline.Image != nil || inline.Unsupported != nil || inline.PageBreak != nil || inline.Tab != nil || inline.FieldCode != nil {
				return true
			}
		}
	}
	return false
}

func tableCellSourceID(tableID string, cell document.WordTableCell) string {
	if cell.ID != "" {
		return cell.ID
	}
	return tableID
}

func tableCellGridSpan(cell document.WordTableCell) int {
	if cell.GridSpan <= 0 {
		return 1
	}
	return cell.GridSpan
}

func tableMergeContinuation(row preparedTableRow, gridColumn, gridSpan int) *preparedTableCell {
	for cellIndex := range row.cells {
		cell := &row.cells[cellIndex]
		if cell.gridColumn == gridColumn && cell.gridSpan == gridSpan && cell.cell.VerticalMerge == document.WordTableVerticalMergeContinue {
			return cell
		}
	}
	return nil
}

func tableGridColumnOffset(widths []float64, gridColumn int) float64 {
	offset := 0.0
	for index := 0; index < gridColumn && index < len(widths); index++ {
		offset += widths[index]
	}
	return offset
}

func tableGridSpanWidth(widths []float64, start, span int) (float64, bool) {
	if start < 0 || span < 1 || start > len(widths) || span > len(widths)-start {
		return 0, false
	}
	width := 0.0
	for index := start; index < start+span; index++ {
		width += widths[index]
	}
	return width, width > 0
}

func verticalMergePageSegments(rows []preparedTableRow, startRow int, cell preparedTableCell, widths []float64) []tableCellPageSegment {
	segments := make([]tableCellPageSegment, 0, 2)
	width, ok := tableGridSpanWidth(widths, cell.gridColumn, cell.gridSpan)
	if !ok {
		return segments
	}
	for rowIndex := startRow; rowIndex <= cell.mergeEndRow && rowIndex < len(rows); rowIndex++ {
		for _, fragment := range rows[rowIndex].fragments {
			bounds := Rect{
				XPT: fragment.x + tableGridColumnOffset(widths, cell.gridColumn), YPT: fragment.y,
				WidthPT: width, HeightPT: fragment.height,
			}
			if len(segments) > 0 {
				last := &segments[len(segments)-1]
				lastBottom := last.bounds.YPT + last.bounds.HeightPT
				if last.pageIndex == fragment.pageIndex && last.column == fragment.column && math.Abs(lastBottom-bounds.YPT) <= epsilonPT {
					last.bounds.HeightPT += bounds.HeightPT
					continue
				}
			}
			segments = append(segments, tableCellPageSegment{pageIndex: fragment.pageIndex, column: fragment.column, bounds: bounds})
		}
	}
	return segments
}

func (c *layoutCursor) placeVerticalMergeCellText(ctx context.Context, tableID string, cell preparedTableCell, width float64, segments []tableCellPageSegment) error {
	if len(cell.paragraphs) == 0 {
		return nil
	}
	if len(segments) == 0 {
		c.diagnostic(DiagnosticError, tableCellSourceID(tableID, cell.cell), "table-cell-content-overflow", "Merged table cell text has no page segment for its content.")
		return nil
	}
	segmentIndex := 0
	y := segments[0].bounds.YPT + tableCellPaddingPT
	segmentTop := func(index int) float64 {
		return segments[index].bounds.YPT
	}
	segmentBottom := func(index int) float64 {
		bottom := segments[index].bounds.YPT + segments[index].bounds.HeightPT
		if index == len(segments)-1 {
			bottom -= tableCellPaddingPT
		}
		return bottom
	}
	advanceSegment := func() bool {
		if segmentIndex+1 >= len(segments) {
			return false
		}
		segmentIndex++
		y = segmentTop(segmentIndex)
		return true
	}
	var previousParagraph preparedParagraph
	previousParagraphCanCollapse := false
	for _, paragraph := range cell.paragraphs {
		if err := ctx.Err(); err != nil {
			return err
		}
		originalBeforePT := paragraph.style.paragraph.BeforePT
		beforePT := originalBeforePT
		canCollapseSpacing := previousParagraphCanCollapse && !preparedHasPageBreak(paragraph)
		if canCollapseSpacing {
			beforePT = paragraphSpacingAdjustment(previousParagraph, paragraph)
		}
		paragraphHeight := paragraph.totalHeight()
		if canCollapseSpacing {
			paragraphHeight += beforePT - originalBeforePT
		}
		keepTogether := paragraph.style.paragraph.KeepTogether || paragraph.style.paragraph.KeepWithNext
		if keepTogether && y+paragraphHeight > segmentBottom(segmentIndex)+epsilonPT {
			for next := segmentIndex + 1; next < len(segments); next++ {
				if paragraph.totalHeight() <= segmentBottom(next)-segmentTop(next)+epsilonPT {
					segmentIndex = next
					y = segmentTop(next)
					beforePT = originalBeforePT
					paragraphHeight = paragraph.totalHeight()
					canCollapseSpacing = false
					break
				}
			}
		}
		for lineIndex, line := range paragraph.lines {
			if err := ctx.Err(); err != nil {
				return err
			}
			leading := 0.0
			if lineIndex == 0 {
				leading = beforePT + paragraph.uiBeforePT
			}
			trailing := 0.0
			if lineIndex == len(paragraph.lines)-1 {
				trailing = paragraph.uiAfterPT + paragraph.style.paragraph.AfterPT + paragraphBottomBorderExtent(paragraph.style.paragraph)
			}
			for y+leading+line.height+trailing > segmentBottom(segmentIndex)+epsilonPT {
				if !advanceSegment() {
					c.diagnostic(DiagnosticError, paragraph.sourceID, "table-cell-content-overflow", "Merged table cell text does not fit within its paginated cell segments.")
					return nil
				}
				if lineIndex == 0 && canCollapseSpacing {
					leading = originalBeforePT + paragraph.uiBeforePT
					canCollapseSpacing = false
				}
			}
			segment := segments[segmentIndex]
			area := Rect{
				XPT: segment.bounds.XPT + tableCellPaddingPT, WidthPT: math.Max(1, width-tableCellPaddingPT*2),
			}
			borderSourceID := paragraph.borderSourceID
			if borderSourceID == "" {
				borderSourceID = paragraph.sourceID
			}
			lineY := y + leading
			placeParagraphLine(c, paragraph.sourceID, borderSourceID, segment.pageIndex, lineY, area, paragraph.style.paragraph, line, lineIndex == 0, lineIndex == len(paragraph.lines)-1)
			y = lineY + line.height + trailing
		}
		previousParagraph = paragraph
		previousParagraphCanCollapse = !preparedHasPageBreak(paragraph)
	}
	return nil
}

func (c *layoutCursor) appendVerticalMergeSegments(tableID string, rows []preparedTableRow, startRow int, cell preparedTableCell, widths []float64, fillColor string) {
	for _, segment := range verticalMergePageSegments(rows, startRow, cell, widths) {
		c.result.Pages[segment.pageIndex].TableCells = append(c.result.Pages[segment.pageIndex].TableCells, TableCellPlacement{
			TableID: tableID, RowIndex: startRow, CellID: cell.cell.ID, FillColor: fillColor,
			Bounds:        segment.bounds,
			DrawTopBorder: true, DrawBottomBorder: true, DrawLeftBorder: true, DrawRightBorder: true,
		})
	}
}
