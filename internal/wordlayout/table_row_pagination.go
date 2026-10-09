package wordlayout

import (
	"context"
	"glance/internal/document"
	"math"
)

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

func tableRowsHeight(rows []preparedTableRow, start, end int) float64 {
	height := 0.0
	for rowIndex := start; rowIndex < end && rowIndex < len(rows); rowIndex++ {
		height += rows[rowIndex].height
	}
	return height
}
