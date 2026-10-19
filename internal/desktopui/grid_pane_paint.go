package desktopui

import (
	"image"

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
			if visibleRect.Empty() {
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
			paintCachedCellWithGridlines(gtx, v, a.theme, cell, fullRect, visibleRect, style, gridlines)
		}
	}
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
