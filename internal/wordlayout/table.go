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

type preparedTableLine struct {
	paragraph preparedParagraph
	lineIndex int
	height    float64
	leading   float64
}

type tableKeepGroup struct {
	end    int
	height float64
}

type tableCellLinePlacement struct {
	line   preparedTableLine
	offset float64
}

type preparedTableRow struct {
	index            int
	cells            []preparedTableCell
	height           float64
	mergeExtraHeight float64
	mergeTailReserve float64
	fragments        []tableRowFragment
}

type tableRowFragment struct {
	pageIndex int
	column    int
	x         float64
	y         float64
	height    float64
	last      bool
	cells     [][]tableCellLinePlacement
}

type tableCellPageSegment struct {
	pageIndex int
	column    int
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

func prepareTableCellLines(paragraphs []preparedParagraph) ([]preparedTableLine, map[int]tableKeepGroup) {
	lines := make([]preparedTableLine, 0)
	paragraphStarts := make([]int, len(paragraphs)+1)
	var previousParagraph preparedParagraph
	previousParagraphCanCollapse := false
	for paragraphIndex, paragraph := range paragraphs {
		paragraphStarts[paragraphIndex] = len(lines)
		beforePT := paragraph.style.paragraph.BeforePT
		if previousParagraphCanCollapse && !preparedHasPageBreak(paragraph) {
			beforePT = paragraphSpacingAdjustment(previousParagraph, paragraph)
		}
		for lineIndex, line := range paragraph.lines {
			leading, trailing := 0.0, 0.0
			if lineIndex == 0 {
				leading = beforePT + paragraph.uiBeforePT
			}
			if lineIndex == len(paragraph.lines)-1 {
				trailing = paragraph.uiAfterPT + paragraph.style.paragraph.AfterPT + paragraphBottomBorderExtent(paragraph.style.paragraph)
			}
			lines = append(lines, preparedTableLine{
				paragraph: paragraph, lineIndex: lineIndex, leading: leading,
				height: leading + line.height + trailing,
			})
		}
		previousParagraph = paragraph
		previousParagraphCanCollapse = !preparedHasPageBreak(paragraph)
	}
	paragraphStarts[len(paragraphs)] = len(lines)

	groups := make(map[int]tableKeepGroup)
	for paragraphIndex := 0; paragraphIndex < len(paragraphs); {
		paragraph := paragraphs[paragraphIndex]
		layout := paragraph.style.paragraph
		if !layout.KeepTogether && !layout.KeepWithNext {
			paragraphIndex++
			continue
		}
		groupEnd := paragraphIndex
		if layout.KeepWithNext {
			for groupEnd+1 < len(paragraphs) {
				groupEnd++
				if !paragraphs[groupEnd].style.paragraph.KeepWithNext {
					break
				}
			}
		}
		startLine, endLine := paragraphStarts[paragraphIndex], paragraphStarts[groupEnd+1]
		if endLine > startLine {
			height := 0.0
			for index := startLine; index < endLine; index++ {
				height += lines[index].height
			}
			groups[startLine] = tableKeepGroup{end: endLine, height: height}
		}
		paragraphIndex = groupEnd + 1
	}
	return lines, groups
}

func (c *layoutCursor) layoutTableRow(ctx context.Context, row *preparedTableRow, rowID string, cantSplit bool) bool {
	if c.pendingBreak {
		if c.used {
			c.advanceColumn()
		}
		c.pendingBreak = false
	}
	remaining := c.content.YPT + c.content.HeightPT - c.y
	if cantSplit {
		if row.height > c.content.HeightPT+epsilonPT {
			c.diagnostic(DiagnosticError, rowID, "table-row-overflow", "This table row is marked not to split but is taller than the printable page area.")
			return false
		}
		if !c.ensureSpace(row.height, rowID) {
			return false
		}
		c.appendWholeTableRow(row, row.height)
		return true
	}
	if row.height <= remaining+epsilonPT {
		if !c.ensureSpace(row.height, rowID) {
			return false
		}
		c.appendWholeTableRow(row, row.height)
		return true
	}

	positions := make([]int, len(row.cells))
	extraHeight := row.mergeExtraHeight
	firstFragment := true
	for {
		if err := ctx.Err(); err != nil {
			return false
		}
		remaining = c.content.YPT + c.content.HeightPT - c.y
		if remaining <= epsilonPT {
			if c.used {
				c.advanceColumn()
				firstFragment = len(row.fragments) == 0
				continue
			}
			c.diagnostic(DiagnosticError, rowID, "table-row-overflow", "A table row cannot make progress within the printable page area.")
			return false
		}
		topPadding := 0.0
		if firstFragment {
			topPadding = tableCellPaddingPT
		}
		capacity := math.Max(0, remaining-topPadding)
		fragment, nextPositions, cellHeights := c.tableRowLineCandidate(row, positions, capacity, topPadding)
		allLinesPlaced := tableRowCellsComplete(row, nextPositions)
		maxCellHeight := maxFloatSlice(cellHeights)
		extraInFragment := 0.0
		if allLinesPlaced {
			availableTail := math.Max(0, remaining-topPadding-maxCellHeight)
			if extraHeight+tableCellPaddingPT <= availableTail+epsilonPT {
				extraInFragment = extraHeight
				fragment.height = topPadding + maxCellHeight + extraInFragment + tableCellPaddingPT
				fragment.last = true
			} else {
				extraInFragment = math.Min(extraHeight, availableTail)
				rowHeightRemaining := math.Max(0, row.height-tableRowPlacedHeight(row))
				if row.mergeTailReserve > 0 && rowHeightRemaining > remaining+epsilonPT {
					maxExtraBeforeTail := rowHeightRemaining - topPadding - maxCellHeight - row.mergeTailReserve
					if availableTail > maxExtraBeforeTail+epsilonPT {
						extraToPreserveTail := availableTail - maxExtraBeforeTail
						row.height += extraToPreserveTail
						row.mergeExtraHeight += extraToPreserveTail
						extraHeight += extraToPreserveTail
						extraInFragment = math.Min(extraHeight, availableTail)
					} else {
						extraInFragment = math.Min(extraInFragment, math.Max(0, maxExtraBeforeTail))
					}
				}
				fragment.height = topPadding + maxCellHeight + extraInFragment
				if extraHeight-extraInFragment <= epsilonPT && fragment.height+tableCellPaddingPT > remaining+epsilonPT {
					for c.trimTableRowLineCandidate(row, fragment, nextPositions, cellHeights, positions) {
						allLinesPlaced = tableRowCellsComplete(row, nextPositions)
						maxCellHeight = maxFloatSlice(cellHeights)
						if !allLinesPlaced || topPadding+maxCellHeight+extraHeight+tableCellPaddingPT <= remaining+epsilonPT {
							break
						}
					}
					allLinesPlaced = tableRowCellsComplete(row, nextPositions)
					maxCellHeight = maxFloatSlice(cellHeights)
					if allLinesPlaced && topPadding+maxCellHeight+extraHeight+tableCellPaddingPT <= remaining+epsilonPT {
						extraInFragment = extraHeight
						fragment.height = topPadding + maxCellHeight + extraInFragment + tableCellPaddingPT
						fragment.last = true
					} else {
						extraInFragment = 0
						fragment.height = topPadding + maxCellHeight
					}
				}
			}
			extraHeight = math.Max(0, extraHeight-extraInFragment)
		} else {
			fragment.height = topPadding + maxCellHeight
		}
		progress := tableRowHasLineProgress(nextPositions, positions) || extraInFragment > epsilonPT
		if fragment.height <= epsilonPT || !progress {
			if c.used {
				c.advanceColumn()
				firstFragment = len(row.fragments) == 0
				continue
			}
			c.diagnostic(DiagnosticError, rowID, "table-row-overflow", "A table row line is taller than the printable page area.")
			return false
		}
		fragment.pageIndex = c.pageIndex
		fragment.column = c.currentCol
		fragment.x = c.currentColumnRect().XPT
		fragment.y = c.y
		row.fragments = append(row.fragments, fragment)
		c.y += fragment.height
		c.used = true
		positions = nextPositions
		if fragment.last {
			return true
		}
		c.advanceColumn()
		firstFragment = false
	}
}

func (c *layoutCursor) appendWholeTableRow(row *preparedTableRow, height float64) {
	fragment := tableRowFragment{
		pageIndex: c.pageIndex, column: c.currentCol, x: c.currentColumnRect().XPT,
		y: c.y, height: height, last: true,
		cells: make([][]tableCellLinePlacement, len(row.cells)),
	}
	for cellIndex, cell := range row.cells {
		if cell.cell.VerticalMerge != document.WordTableVerticalMergeNone {
			continue
		}
		offset := tableCellPaddingPT
		for _, line := range cell.lines {
			fragment.cells[cellIndex] = append(fragment.cells[cellIndex], tableCellLinePlacement{line: line, offset: offset})
			offset += line.height
		}
	}
	row.fragments = append(row.fragments, fragment)
	c.y += height
	c.used = true
}

func tableRowPlacedHeight(row *preparedTableRow) float64 {
	height := 0.0
	for _, fragment := range row.fragments {
		height += fragment.height
	}
	return height
}

func (c *layoutCursor) tableRowLineCandidate(row *preparedTableRow, positions []int, capacity, topPadding float64) (tableRowFragment, []int, []float64) {
	fragment := tableRowFragment{cells: make([][]tableCellLinePlacement, len(row.cells))}
	nextPositions := append([]int(nil), positions...)
	cellHeights := make([]float64, len(row.cells))
	keepLimit := math.Max(0, c.content.HeightPT-tableCellPaddingPT*2)
	for cellIndex, cell := range row.cells {
		if cell.cell.VerticalMerge != document.WordTableVerticalMergeNone {
			continue
		}
		position := positions[cellIndex]
		usedHeight := 0.0
		for position < len(cell.lines) {
			line := tableCellLineForFragment(cell, position, usedHeight <= epsilonPT && position > 0)
			if group, ok := cell.keepGroups[position]; ok && group.end > position {
				groupLines := make([]preparedTableLine, 0, group.end-position)
				groupHeight := 0.0
				for groupLine := position; groupLine < group.end; groupLine++ {
					preparedLine := tableCellLineForFragment(cell, groupLine, usedHeight <= epsilonPT && groupLine == position)
					groupLines = append(groupLines, preparedLine)
					groupHeight += preparedLine.height
				}
				if groupHeight <= keepLimit+epsilonPT && usedHeight+groupHeight <= capacity+epsilonPT {
					for _, groupLine := range groupLines {
						fragment.cells[cellIndex] = append(fragment.cells[cellIndex], tableCellLinePlacement{line: groupLine, offset: topPadding + usedHeight})
						usedHeight += groupLine.height
					}
					position = group.end
					continue
				}
				if groupHeight <= keepLimit+epsilonPT {
					break
				}
			}
			if usedHeight+line.height > capacity+epsilonPT {
				break
			}
			fragment.cells[cellIndex] = append(fragment.cells[cellIndex], tableCellLinePlacement{line: line, offset: topPadding + usedHeight})
			usedHeight += line.height
			position++
		}
		nextPositions[cellIndex] = position
		cellHeights[cellIndex] = usedHeight
	}
	fragment.height = topPadding + maxFloatSlice(cellHeights)
	return fragment, nextPositions, cellHeights
}

func tableCellLineForFragment(cell preparedTableCell, position int, startsFragment bool) preparedTableLine {
	line := cell.lines[position]
	if startsFragment && line.lineIndex == 0 {
		fullLeading := line.paragraph.style.paragraph.BeforePT + line.paragraph.uiBeforePT
		line.height += fullLeading - line.leading
		line.leading = fullLeading
	}
	return line
}

func tableRowHasLineProgress(nextPositions, positions []int) bool {
	for index := range nextPositions {
		if nextPositions[index] > positions[index] {
			return true
		}
	}
	return false
}

func (c *layoutCursor) trimTableRowLineCandidate(row *preparedTableRow, fragment tableRowFragment, nextPositions []int, cellHeights []float64, positions []int) bool {
	cellIndex := -1
	maximumHeight := -1.0
	for index, placements := range fragment.cells {
		if len(placements) == 0 {
			continue
		}
		if cellHeights[index] >= maximumHeight {
			maximumHeight = cellHeights[index]
			cellIndex = index
		}
	}
	if cellIndex < 0 {
		return false
	}
	cell := row.cells[cellIndex]
	trimStart := nextPositions[cellIndex] - 1
	keepLimit := math.Max(0, c.content.HeightPT-tableCellPaddingPT*2)
	for start, group := range cell.keepGroups {
		if group.end == nextPositions[cellIndex] && start >= positions[cellIndex] && group.height <= keepLimit+epsilonPT {
			trimStart = start
			break
		}
	}
	removeCount := nextPositions[cellIndex] - trimStart
	if removeCount <= 0 || removeCount > len(fragment.cells[cellIndex]) {
		return false
	}
	fragment.cells[cellIndex] = fragment.cells[cellIndex][:len(fragment.cells[cellIndex])-removeCount]
	nextPositions[cellIndex] = trimStart
	cellHeights[cellIndex] = 0
	for _, placement := range fragment.cells[cellIndex] {
		cellHeights[cellIndex] += placement.line.height
	}
	return true
}

func tableRowCellsComplete(row *preparedTableRow, positions []int) bool {
	for cellIndex, cell := range row.cells {
		if cell.cell.VerticalMerge == document.WordTableVerticalMergeNone && positions[cellIndex] < len(cell.lines) {
			return false
		}
	}
	return true
}

func maxFloatSlice(values []float64) float64 {
	maximum := 0.0
	for _, value := range values {
		maximum = math.Max(maximum, value)
	}
	return maximum
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
