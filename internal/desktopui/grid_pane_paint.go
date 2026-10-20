package desktopui

import (
	"image"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"glance/internal/document"
)

type gridPaintPane struct {
	window        document.SheetWindow
	windowRange   document.CellRange
	clip          image.Rectangle
	rowHeader     int
	columnHeader  int
	frozenRows    bool
	frozenColumns bool
}

func gridPaintPanes(v *gridViewport, size image.Point, rowHeader, columnHeader int) []gridPaintPane {
	viewport := image.Rect(rowHeader, columnHeader, size.X, size.Y)
	frozenWidth, frozenHeight := v.frozenViewportExtent(float64(viewport.Dx()), float64(viewport.Dy()))
	xDivider, yDivider := rowHeader+int(frozenWidth), columnHeader+int(frozenHeight)
	columnSplit, rowSplit := v.paneSplits()
	panes := []gridPaintPane{
		{window: v.Window, windowRange: v.WindowRange, clip: image.Rect(xDivider, yDivider, size.X, size.Y), rowHeader: rowHeader, columnHeader: columnHeader},
	}
	if rowSplit > 0 {
		panes = append(panes, gridPaintPane{window: v.TopWindow, windowRange: v.TopRange, clip: image.Rect(xDivider, columnHeader, size.X, yDivider), rowHeader: rowHeader, columnHeader: columnHeader, frozenRows: true})
	}
	if columnSplit > 0 {
		panes = append(panes, gridPaintPane{window: v.LeftWindow, windowRange: v.LeftRange, clip: image.Rect(rowHeader, yDivider, xDivider, size.Y), rowHeader: rowHeader, columnHeader: columnHeader, frozenColumns: true})
	}
	if rowSplit > 0 && columnSplit > 0 {
		panes = append(panes, gridPaintPane{window: v.CornerWindow, windowRange: v.CornerRange, clip: image.Rect(rowHeader, columnHeader, xDivider, yDivider), rowHeader: rowHeader, columnHeader: columnHeader, frozenRows: true, frozenColumns: true})
	}
	result := panes[:0]
	for _, pane := range panes {
		if pane.windowRange.StartRow > 0 && !pane.clip.Empty() {
			result = append(result, pane)
		}
	}
	return result
}

func paintGridPaneCells(gtx layout.Context, a *App, v *gridViewport, pane gridPaintPane, selection CellSelection, hasSelection bool, formulaReferences []document.CellRange, gridlines bool) {
	if pane.windowRange.StartRow <= 0 || pane.windowRange.StartColumn <= 0 || pane.windowRange.EndRow < pane.windowRange.StartRow || pane.windowRange.EndColumn < pane.windowRange.StartColumn {
		return
	}
	area := clip.Rect(pane.clip).Push(gtx.Ops)
	defer area.Pop()
	var borders []gridBorderPaint
	var overflowText []gridCellTextOverflow
	paintedMerges := make(map[document.CellRange]bool, len(pane.window.Merges))
	for rowIndex, row := range pane.window.Cells {
		rowNumber := pane.window.StartRow + rowIndex
		if rowNumber < pane.windowRange.StartRow || rowNumber > pane.windowRange.EndRow {
			continue
		}
		for columnIndex, cell := range row {
			columnNumber := pane.window.StartColumn + columnIndex
			if columnNumber < pane.windowRange.StartColumn || columnNumber > pane.windowRange.EndColumn {
				continue
			}
			cellRange := document.CellRange{StartRow: rowNumber, StartColumn: columnNumber, EndRow: rowNumber, EndColumn: columnNumber}
			merged := false
			for _, merge := range pane.window.Merges {
				if rowNumber < merge.Range.StartRow || rowNumber > merge.Range.EndRow || columnNumber < merge.Range.StartColumn || columnNumber > merge.Range.EndColumn {
					continue
				}
				merged = true
				if paintedMerges[merge.Range] {
					cellRange = document.CellRange{}
					break
				}
				paintedMerges[merge.Range] = true
				cellRange = merge.Range
				cell = merge.Anchor
				break
			}
			if cellRange.StartRow <= 0 {
				continue
			}
			if intersectCellRanges(cellRange, pane.windowRange).StartRow <= 0 {
				continue
			}
			fullRect := v.screenCellRect(cellRange, pane.frozenRows, pane.frozenColumns, pane.rowHeader, pane.columnHeader)
			visibleRect := fullRect.Intersect(pane.clip)
			canOverflow := !merged && !fullRect.Empty() && !gridCellIsBeingEdited(a, rowNumber, columnNumber) && gridCellCanOverflowText(cell)
			spillEnd := fullRect.Max.X
			if canOverflow {
				spillEnd = gridCellTextOverflowEnd(v, pane, rowNumber, columnNumber, fullRect)
			}
			if visibleRect.Empty() {
				if canOverflow && fullRect.Max.X <= pane.clip.Min.X && spillEnd > pane.clip.Min.X {
					overflowText = append(overflowText, gridCellTextOverflow{
						cell:  cell,
						rect:  fullRect,
						clip:  image.Rect(fullRect.Min.X, fullRect.Min.Y, spillEnd, fullRect.Max.Y).Intersect(pane.clip),
						style: paintCellStyle(cell),
					})
				}
				continue
			}
			style := paintCellStyle(cell)
			checkPos := CellPosition{Row: rowNumber - 1, Column: columnNumber - 1}
			if merged {
				checkPos = CellPosition{Row: cellRange.StartRow - 1, Column: cellRange.StartColumn - 1}
			}
			if hasSelection && !a.cellEdit.Editing && selection.Contains(checkPos) && (selection.Anchor != selection.Active || selection.Active != checkPos) {
				style.Fill = selectedFill
			}
			if formulaReferenceContains(formulaReferences, rowNumber, columnNumber) || merged && formulaReferenceContains(formulaReferences, cellRange.StartRow, cellRange.StartColumn) {
				style.Fill = selectedFill
			}
			borders = appendGridBorders(borders, fullRect, pane.clip, cell.Style.Borders)
			if merged {
				borders = appendMergedGridBorders(borders, v, pane, cellRange)
			}
			cell.Style.Borders = [4]document.CellBorder{}
			mode := cellPaintModeFull
			if canOverflow && spillEnd > fullRect.Max.X {
				mode = cellPaintModeBackground
				overflowText = append(overflowText, gridCellTextOverflow{
					cell:  cell,
					rect:  fullRect,
					clip:  image.Rect(fullRect.Min.X, fullRect.Min.Y, spillEnd, fullRect.Max.Y).Intersect(pane.clip),
					style: style,
				})
			}
			paintCachedCellMode(gtx, v, a.theme, cell, fullRect, visibleRect, style, gridlines, mode)
		}
	}
	paintGridBorders(gtx, borders)
	for _, text := range overflowText {
		paintCachedCellMode(gtx, v, a.theme, text.cell, text.rect, text.clip, text.style, gridlines, cellPaintModeTextOnly)
	}
}

type gridCellTextOverflow struct {
	cell  document.Cell
	rect  image.Rectangle
	clip  image.Rectangle
	style gridPaintStyle
}

func gridCellCanOverflowText(cell document.Cell) bool {
	textValue := cell.Value
	if textValue == "" && len(cell.RichText) > 0 {
		var combined strings.Builder
		for _, run := range cell.RichText {
			combined.WriteString(run.Text)
		}
		textValue = combined.String()
	}
	if textValue == "" || cell.Style.WrapText || strings.ContainsAny(textValue, "\r\n") {
		return false
	}
	for _, run := range cell.RichText {
		if strings.ContainsAny(run.Text, "\r\n") {
			return false
		}
	}
	switch strings.ToLower(strings.TrimSpace(cell.Style.HorizontalAlignment)) {
	case "", "general", "left":
	default:
		return false
	}
	if cell.ValueKind != "" {
		return cell.ValueKind == document.CellValueText
	}
	if cell.TypedValue != nil {
		return cell.TypedValue.Kind == document.CellValueText
	}
	trimmed := strings.TrimSpace(textValue)
	if trimmed == "" || strings.EqualFold(trimmed, "true") || strings.EqualFold(trimmed, "false") || strings.HasPrefix(trimmed, "#") {
		return false
	}
	numeric := strings.NewReplacer(",", "", "$", "", "¥", "", "￥", "", "€", "", "£", "", "%", "").Replace(trimmed)
	if strings.HasPrefix(numeric, "(") && strings.HasSuffix(numeric, ")") {
		numeric = "-" + strings.TrimSuffix(strings.TrimPrefix(numeric, "("), ")")
	}
	if _, err := strconv.ParseFloat(numeric, 64); err == nil {
		return false
	}
	return true
}

func gridCellIsBeingEdited(a *App, row, column int) bool {
	if a == nil || !a.cellEdit.Editing {
		return false
	}
	if state := a.documentSessionState; state != nil {
		ref := state.formulaReference
		if ref.Active && ref.InCell && ref.TargetSheet != a.sheet {
			return false
		}
	}
	editRow, editColumn, ok := parseCellAddress(a.cellEdit.Address)
	return ok && editRow == row && editColumn == column
}

func gridCellTextOverflowEnd(v *gridViewport, pane gridPaintPane, row, column int, source image.Rectangle) int {
	if v == nil || pane.windowRange.StartColumn <= 0 || column >= pane.windowRange.EndColumn {
		return source.Max.X
	}
	lastColumn := pane.windowRange.EndColumn
	limit := pane.clip.Max.X
	lastRect := v.screenCellRect(document.CellRange{StartRow: row, StartColumn: lastColumn, EndRow: row, EndColumn: lastColumn}, pane.frozenRows, pane.frozenColumns, pane.rowHeader, pane.columnHeader)
	limit = min(limit, lastRect.Max.X)
	for nextColumn := column + 1; nextColumn <= lastColumn; nextColumn++ {
		nextRect := v.screenCellRect(document.CellRange{StartRow: row, StartColumn: nextColumn, EndRow: row, EndColumn: nextColumn}, pane.frozenRows, pane.frozenColumns, pane.rowHeader, pane.columnHeader)
		if nextRect.Min.X >= limit {
			break
		}
		if gridPaneCellIsMerged(pane, row, nextColumn) || gridCellHasContent(windowCell(pane.window, row, nextColumn)) {
			limit = min(limit, nextRect.Min.X)
			break
		}
	}
	return max(source.Max.X, limit)
}

func gridPaneCellIsMerged(pane gridPaintPane, row, column int) bool {
	for _, merge := range pane.window.Merges {
		if row >= merge.Range.StartRow && row <= merge.Range.EndRow && column >= merge.Range.StartColumn && column <= merge.Range.EndColumn {
			return true
		}
	}
	return false
}

func gridCellHasContent(cell document.Cell) bool {
	return cell.Value != "" || cell.Formula != "" || cell.TypedValue != nil || len(cell.RichText) > 0
}

func intersectCellRanges(a, b document.CellRange) document.CellRange {
	if a.StartRow <= 0 || b.StartRow <= 0 {
		return document.CellRange{}
	}
	rangeOf := document.CellRange{
		StartRow:    max(a.StartRow, b.StartRow),
		StartColumn: max(a.StartColumn, b.StartColumn),
		EndRow:      min(a.EndRow, b.EndRow),
		EndColumn:   min(a.EndColumn, b.EndColumn),
	}
	if rangeOf.EndRow < rangeOf.StartRow || rangeOf.EndColumn < rangeOf.StartColumn {
		return document.CellRange{}
	}
	return rangeOf
}
