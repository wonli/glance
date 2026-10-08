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
	contentH    float64
	gridColumn  int
	gridSpan    int
	mergeEndRow int
}

type preparedTableRow struct {
	index     int
	cells     []preparedTableCell
	height    float64
	pageIndex int
	column    int
	x         float64
	y         float64
}

type tableCellPageSegment struct {
	pageIndex int
	bounds    Rect
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
				preparedCell.contentH += prepared.totalHeight()
				preparedCell.paragraphs = append(preparedCell.paragraphs, prepared)
			}
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
			if requiredHeight > mergedHeight {
				extraHeight := (requiredHeight - mergedHeight) / float64(cell.mergeEndRow-rowIndex+1)
				for mergeRow := rowIndex; mergeRow <= cell.mergeEndRow; mergeRow++ {
					rows[mergeRow].height += extraHeight
				}
			}
		}
	}
	for rowIndex := range rows {
		row := &rows[rowIndex]
		rowID := tableID + ".r" + itoa(rowIndex)
		if row.height > c.content.HeightPT+epsilonPT {
			c.diagnostic(DiagnosticError, rowID, "table-row-overflow", "This table row is taller than the printable page area and cannot be split safely.")
			return nil
		}
		if !c.ensureSpace(row.height, rowID) {
			// A fresh page cannot fit this row, so do not retry it indefinitely.
			return nil
		}
		row.pageIndex = c.pageIndex
		row.column = c.currentCol
		row.y = c.y
		row.x = area.XPT
		if c.columns > 1 {
			row.x = c.currentColumnRect().XPT
		}
		c.y += row.height
		c.used = true
	}
	for rowIndex, row := range rows {
		for _, cell := range row.cells {
			if cell.cell.VerticalMerge == document.WordTableVerticalMergeContinue {
				continue
			}
			cellWidth, ok := tableGridSpanWidth(widths, cell.gridColumn, cell.gridSpan)
			if !ok {
				c.diagnostic(DiagnosticError, tableCellSourceID(tableID, cell.cell), "table-grid-span", "A table cell span extends beyond the available grid columns.")
				return nil
			}
			cellX := row.x + tableGridColumnOffset(widths, cell.gridColumn)
			if cell.cell.VerticalMerge == document.WordTableVerticalMergeRestart {
				segments := verticalMergePageSegments(rows, rowIndex, cell, widths)
				if err := c.placeVerticalMergeCellText(ctx, tableID, cell, cellWidth, segments); err != nil {
					return err
				}
				continue
			}
			inner := Rect{
				XPT: cellX + tableCellPaddingPT, YPT: row.y + tableCellPaddingPT,
				WidthPT: math.Max(1, cellWidth-tableCellPaddingPT*2), HeightPT: math.Max(0, row.height-tableCellPaddingPT*2),
			}
			cellY := inner.YPT
			for _, paragraph := range cell.paragraphs {
				if err := c.placePreparedParagraph(ctx, paragraph, inner, false, row.pageIndex, cellY); err != nil {
					return err
				}
				cellY += paragraph.totalHeight()
			}
		}
	}
	for rowIndex := range rows {
		row := rows[rowIndex]
		x := area.XPT
		if c.columns > 1 {
			x = row.x
		}
		for _, cell := range row.cells {
			if cell.cell.VerticalMerge == document.WordTableVerticalMergeContinue {
				continue
			}
			cellX := x + tableGridColumnOffset(widths, cell.gridColumn)
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
			c.result.Pages[row.pageIndex].TableCells = append(c.result.Pages[row.pageIndex].TableCells, TableCellPlacement{
				TableID: tableID, RowIndex: row.index, CellID: cell.cell.ID, FillColor: fillColor,
				Bounds:        Rect{XPT: cellX, YPT: row.y, WidthPT: cellWidth, HeightPT: row.height},
				DrawTopBorder: true, DrawBottomBorder: true, DrawLeftBorder: true, DrawRightBorder: true,
			})
		}
	}
	return nil
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

func tableRowsHeight(rows []preparedTableRow, start, end int) float64 {
	height := 0.0
	for rowIndex := start; rowIndex < end && rowIndex < len(rows); rowIndex++ {
		height += rows[rowIndex].height
	}
	return height
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
	segmentStart := startRow
	segmentEnd := startRow
	flush := func() {
		row := rows[segmentStart]
		height := tableRowsHeight(rows, segmentStart, segmentEnd+1)
		width, ok := tableGridSpanWidth(widths, cell.gridColumn, cell.gridSpan)
		if !ok || row.pageIndex < 0 {
			return
		}
		x := row.x + tableGridColumnOffset(widths, cell.gridColumn)
		segments = append(segments, tableCellPageSegment{
			pageIndex: row.pageIndex,
			bounds:    Rect{XPT: x, YPT: row.y, WidthPT: width, HeightPT: height},
		})
	}
	for rowIndex := startRow + 1; rowIndex <= cell.mergeEndRow; rowIndex++ {
		previous := rows[segmentEnd]
		current := rows[rowIndex]
		if current.pageIndex != previous.pageIndex || current.column != previous.column {
			flush()
			segmentStart, segmentEnd = rowIndex, rowIndex
			continue
		}
		segmentEnd = rowIndex
	}
	flush()
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
	for _, paragraph := range cell.paragraphs {
		if err := ctx.Err(); err != nil {
			return err
		}
		paragraphHeight := paragraph.totalHeight()
		keepTogether := paragraph.style.paragraph.KeepTogether || paragraph.style.paragraph.KeepWithNext
		if keepTogether && y+paragraphHeight > segmentBottom(segmentIndex)+epsilonPT {
			for next := segmentIndex + 1; next < len(segments); next++ {
				if paragraphHeight <= segmentBottom(next)-segmentTop(next)+epsilonPT {
					segmentIndex = next
					y = segmentTop(next)
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
				leading = paragraph.style.paragraph.BeforePT + paragraph.uiBeforePT
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

func tableColumnWidths(table document.WordTable, totalWidth float64) ([]float64, bool) {
	columns := len(table.Layout.GridColumnsEMU)
	for _, row := range table.Rows {
		rowColumns := 0
		for _, cell := range row.Cells {
			rowColumns += tableCellGridSpan(cell)
		}
		if rowColumns > columns {
			columns = rowColumns
		}
	}
	if columns == 0 {
		return nil, false
	}
	widths := make([]float64, columns)
	for index, emu := range table.Layout.GridColumnsEMU {
		widths[index] = emuToPoints(emu)
	}
	for _, row := range table.Rows {
		gridColumn := 0
		for _, cell := range row.Cells {
			span := tableCellGridSpan(cell)
			if cell.WidthEMU > 0 && gridColumn < len(widths) && span <= len(widths)-gridColumn {
				knownWidth := 0.0
				missing := make([]int, 0, span)
				for index := gridColumn; index < gridColumn+span; index++ {
					if widths[index] > 0 {
						knownWidth += widths[index]
					} else {
						missing = append(missing, index)
					}
				}
				if len(missing) > 0 {
					remainingWidth := emuToPoints(cell.WidthEMU) - knownWidth
					if remainingWidth > 0 {
						for _, index := range missing {
							widths[index] = remainingWidth / float64(len(missing))
						}
					}
				}
			}
			gridColumn += span
		}
	}
	inferred := false
	for _, width := range widths {
		if width <= 0 {
			inferred = true
			break
		}
	}
	sum := 0.0
	knownCount := 0
	for _, width := range widths {
		if width > 0 {
			sum += width
			knownCount++
		}
	}
	if knownCount == 0 || sum <= 0 {
		for index := range widths {
			widths[index] = totalWidth / float64(columns)
		}
		return widths, inferred
	}
	for index, width := range widths {
		if width <= 0 {
			widths[index] = sum / float64(knownCount)
		}
	}
	sum = 0
	for _, width := range widths {
		sum += width
	}
	scale := totalWidth / sum
	for index := range widths {
		widths[index] *= scale
	}
	return widths, inferred
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	if value < 0 {
		return "-" + itoa(-value)
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
