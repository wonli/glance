package desktopui

import (
	"image"
	"math"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"glance/internal/document"
)

type gridCellRowPreview struct {
	Sheet      string
	Row        int
	OriginalPx float64
	BasePx     float64
	PreviewPx  float64
}

type gridAutoFitRow struct {
	Sheet string
	Row   int
}

func updateGridCellRowPreview(gtx layout.Context, a *App, v *gridViewport) {
	if a == nil || v == nil {
		return
	}
	if !a.cellEdit.Editing {
		v.cancelCellEditRowPreview()
		return
	}
	row, column, ok := parseCellAddress(a.cellEdit.Address)
	if !ok || v.WindowSheet != a.sheet || !v.cellLoaded(row, column) {
		if v.CellEditRowPreview != nil && v.CellEditRowPreview.Sheet != a.sheet {
			v.cancelCellEditRowPreview()
		}
		return
	}
	if _, _, merged := mergeAnchorAt(v.merges(), row, column); merged {
		v.cancelCellEditRowPreview()
		return
	}
	cell := v.cell(row, column)
	value := a.cellEditor.Text()
	key := gridAutoFitRow{Sheet: a.sheet, Row: row - 1}
	_, managed := a.documentSessionState.spreadsheetAutoFitRowBasePoints[key]
	if !cell.Style.WrapText && !strings.ContainsRune(value, '\n') && !managed {
		if v.CellEditRowPreview != nil && v.CellEditRowPreview.Row == row-1 {
			v.cancelCellEditRowPreview()
		}
		return
	}
	if v.CellEditRowPreview != nil && (v.CellEditRowPreview.Sheet != a.sheet || v.CellEditRowPreview.Row != row-1) {
		v.cancelCellEditRowPreview()
	}
	if v.CellEditRowPreview == nil {
		original := v.Rows.sizeAt(row - 1)
		base, wasManaged := a.documentSessionState.spreadsheetAutoFitRowBasePoints[key]
		basePx := original
		if wasManaged {
			basePx = pointsToGridPixels(base) * v.Scale
		}
		v.CellEditRowPreview = &gridCellRowPreview{Sheet: a.sheet, Row: row - 1, OriginalPx: original, BasePx: basePx}
	}
	preview := v.CellEditRowPreview
	cellRange := document.CellRange{StartRow: row, StartColumn: column, EndRow: row, EndColumn: column}
	width := cellRect(v.Rows, v.Columns, cellRange, 0, 0).Dx()
	height := measureGridCellTextHeight(gtx, a.theme, cell, value, width, v.Scale)
	desired := math.Max(preview.BasePx, math.Min(maxGridRowPx*v.Scale, float64(height)))
	preview.PreviewPx = desired
	setGridRowSizePreservingViewport(v, row-1, desired)
}

func measureGridCellTextHeight(gtx layout.Context, theme *material.Theme, cell document.Cell, value string, width int, scale float64) int {
	if theme == nil || width <= 0 {
		return int(minGridRowPx)
	}
	zoom := 1.0
	if gtx.Metric.PxPerDp > 0 && scale > 0 {
		zoom = scale / float64(gtx.Metric.PxPerDp)
	}
	style := paintStyle(cell.Style)
	label := material.Body2(theme, value)
	label.TextSize = cellTextSize(cell.Style, zoom)
	label.Color = style.Font
	label.Font.Typeface = cellTypeface(style)
	label.Alignment = cellTextAlignment(style.Align)
	if style.Bold {
		label.Font.Weight = cellFontWeight(true)
	}
	if style.Italic {
		label.Font.Style = font.Italic
	}
	if style.Wrap {
		label.WrapPolicy = text.WrapWords
	}
	contentWidth := max(1, width-gtx.Dp(unit.Dp(9)))
	var ops op.Ops
	measureGtx := gtx
	measureGtx.Ops = &ops
	measureGtx.Constraints = layout.Constraints{
		Min: image.Pt(contentWidth, 0),
		Max: image.Pt(contentWidth, max(1, int(maxGridRowPx*scale))),
	}
	dims := label.Layout(measureGtx)
	return int(math.Min(maxGridRowPx*math.Max(1, scale), float64(dims.Size.Y+gtx.Dp(unit.Dp(2)))))
}

func setGridRowSizePreservingViewport(v *gridViewport, row int, size float64) {
	if v == nil || v.Rows == nil || row < 0 || !finite(size) || size <= 0 || v.Rows.sizeAt(row) == size {
		return
	}
	anchor := v.Rows.IndexAt(v.scrollWorldY())
	within := 0.0
	if anchor >= 0 {
		if edge, err := v.Rows.Boundary(anchor); err == nil {
			within = v.scrollWorldY() - edge
		}
	}
	if v.Rows.SetSize(row, size) != nil {
		return
	}
	if anchor >= 0 {
		if edge, err := v.Rows.Boundary(anchor); err == nil {
			v.Y = math.Max(0, edge+within-v.frozenRowBoundary())
		}
	}
	v.GeometryChanged = true
}

func (v *gridViewport) cancelCellEditRowPreview() {
	if v == nil || v.CellEditRowPreview == nil {
		return
	}
	preview := v.CellEditRowPreview
	setGridRowSizePreservingViewport(v, preview.Row, preview.OriginalPx)
	v.CellEditRowPreview = nil
}

func (v *gridViewport) finishCellEditRowPreview(restore bool) *gridCellRowPreview {
	if v == nil || v.CellEditRowPreview == nil {
		return nil
	}
	preview := *v.CellEditRowPreview
	if restore {
		setGridRowSizePreservingViewport(v, preview.Row, preview.OriginalPx)
	}
	v.CellEditRowPreview = nil
	return &preview
}

func (state *documentSessionState) markGridRowHeightManual(sheet string, row int) {
	if state == nil {
		return
	}
	delete(state.spreadsheetAutoFitRowBasePoints, gridAutoFitRow{Sheet: sheet, Row: row})
}
