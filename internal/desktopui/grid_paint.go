package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

type gridPaintStyle struct {
	Fill, Font         color.NRGBA
	Bold, Italic, Wrap bool
	Align              string
	Vertical           string
}

const maxCachedCellPaints = 1024

type cellPaintKey struct {
	Value   string
	Source  document.CellStyle
	Style   gridPaintStyle
	Size    image.Point
	PxPerDp float32
	PxPerSp float32
}

type cellPaintEntry struct {
	ops  op.Ops
	call op.CallOp
}

func paintStyle(style document.CellStyle) gridPaintStyle {
	return gridPaintStyle{Fill: parseGridColor(style.FillColor, color.NRGBA{R: 255, G: 255, B: 255, A: 255}), Font: parseGridColor(style.FontColor, color.NRGBA{R: 35, G: 39, B: 45, A: 255}), Bold: style.Bold, Italic: style.Italic, Wrap: style.WrapText, Align: style.HorizontalAlignment, Vertical: style.VerticalAlignment}
}

func parseGridColor(s string, fallback color.NRGBA) color.NRGBA {
	if len(s) == 7 && s[0] == '#' {
		s = s[1:]
	}
	if len(s) != 6 {
		return fallback
	}
	var r, g, b uint8
	if _, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b); err != nil {
		return fallback
	}
	return color.NRGBA{R: r, G: g, B: b, A: 255}
}

func paintVirtualGrid(gtx layout.Context, a *App, v *gridViewport) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, pageBackground, clip.Rect{Max: size}.Op())
	rowHead := gtx.Dp(unit.Dp(gridRowHeader))
	colHead := gtx.Dp(unit.Dp(gridColumnHeader))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 234, G: 238, B: 244, A: 255}, clip.Rect{Max: image.Pt(size.X, colHead)}.Op())
	paint.FillShape(gtx.Ops, color.NRGBA{R: 234, G: 238, B: 244, A: 255}, clip.Rect{Max: image.Pt(rowHead, size.Y)}.Op())
	paint.FillShape(gtx.Ops, color.NRGBA{R: 190, G: 198, B: 209, A: 255}, clip.Rect{Min: image.Pt(rowHead, 0), Max: image.Pt(rowHead+1, size.Y)}.Op())
	paint.FillShape(gtx.Ops, color.NRGBA{R: 190, G: 198, B: 209, A: 255}, clip.Rect{Min: image.Pt(0, colHead), Max: image.Pt(size.X, colHead+1)}.Op())
	if v.WindowRange.StartRow > 0 {
		paintedMerges := make(map[document.CellRange]bool, len(v.Window.Merges))
		for i, row := range v.Window.Cells {
			rr := v.Window.StartRow + i
			y0f, _ := v.Rows.Boundary(rr - 1)
			y1f, _ := v.Rows.Boundary(rr)
			y0 := int(y0f-v.Y) + colHead
			y1 := int(y1f-v.Y) + colHead
			if y1 <= colHead || y0 >= size.Y {
				continue
			}
			paintTextBox(gtx, a.theme, strconv.Itoa(rr), image.Rect(0, y0, rowHead, y1), color.NRGBA{R: 72, G: 78, B: 88, A: 255}, color.NRGBA{R: 234, G: 238, B: 244, A: 255}, false)
			for j, cell := range row {
				cc := v.Window.StartColumn + j
				mr, mc, merged := mergeAnchorAt(v.Window.Merges, rr, cc)
				cr := document.CellRange{StartRow: rr, StartColumn: cc, EndRow: rr, EndColumn: cc}
				if merged {
					draw := false
					for _, m := range v.Window.Merges {
						if m.Range.StartRow == mr && m.Range.StartColumn == mc {
							if paintedMerges[m.Range] {
								draw = true
								break
							}
							paintedMerges[m.Range] = true
							cr = m.Range
							cell = m.Anchor
							break
						}
					}
					if draw {
						continue
					}
				}
				fullRect := cellRect(v.Rows, v.Columns, cr, v.X, v.Y).Add(image.Pt(rowHead, colHead))
				visibleRect := fullRect.Intersect(image.Rect(rowHead, colHead, size.X, size.Y))
				if visibleRect.Empty() {
					continue
				}
				style := paintStyle(cell.Style)
				if rr == v.SelectedRow && cc == v.SelectedColumn || merged && mr == v.SelectedRow && mc == v.SelectedColumn {
					style.Fill = selectedFill
				}
				paintCachedCell(gtx, v, a.theme, cell, fullRect, visibleRect, style)
			}
		}
	}
	startCol := v.Columns.IndexAt(v.X)
	if startCol < 0 {
		startCol = 0
	}
	endCol := v.Columns.IndexAt(v.X + float64(size.X-rowHead))
	if endCol < startCol {
		endCol = startCol
	}
	for c := startCol; c <= endCol; c++ {
		x0f, _ := v.Columns.Boundary(c)
		x1f, _ := v.Columns.Boundary(c + 1)
		x0 := int(x0f-v.X) + rowHead
		x1 := int(x1f-v.X) + rowHead
		paintTextBox(gtx, a.theme, columnName(c+1), image.Rect(x0, 0, x1, colHead), mutedText, color.NRGBA{R: 234, G: 238, B: 244, A: 255}, false)
	}
	if a.cellEdit.Editing {
		if row, col, ok := parseCellAddress(a.cellEdit.Address); ok {
			rng := document.CellRange{StartRow: row, StartColumn: col, EndRow: row, EndColumn: col}
			for _, m := range v.Window.Merges {
				if row >= m.Range.StartRow && row <= m.Range.EndRow && col >= m.Range.StartColumn && col <= m.Range.EndColumn {
					rng = m.Range
					break
				}
			}
			fullRect := cellRect(v.Rows, v.Columns, rng, v.X, v.Y).Add(image.Pt(rowHead, colHead))
			visibleRect := fullRect.Intersect(image.Rect(rowHead, colHead, size.X, size.Y))
			if !visibleRect.Empty() {
				cell := windowCell(v.Window, row, col)
				style := paintStyle(cell.Style)
				style.Fill = selectedFill
				paintCellEditor(gtx, a.theme, &a.cellEditor, fullRect, visibleRect, cell.Style, style)
			}
		}
	}
	if v.Resize != nil {
		rs := v.Resize.State
		preview := rs.previewEdge()
		line := color.NRGBA{R: 30, G: 105, B: 205, A: 255}
		lineWidth := max(1, gtx.Dp(unit.Dp(resizePreviewLinePx)))
		if rs.Axis == resizeColumns {
			x := int(preview)
			_ = resizePreviewExtent(float64(size.X), preview, float64(lineWidth))
			paint.FillShape(gtx.Ops, line, clip.Rect{Min: image.Pt(x, 0), Max: image.Pt(x+lineWidth, size.Y)}.Op())
		} else {
			y := int(preview)
			_ = resizePreviewExtent(float64(size.Y), preview, float64(lineWidth))
			paint.FillShape(gtx.Ops, line, clip.Rect{Min: image.Pt(0, y), Max: image.Pt(size.X, y+lineWidth)}.Op())
		}
	}
	return layout.Dimensions{Size: size}
}

func paintCellEditor(gtx layout.Context, theme *material.Theme, editor *widget.Editor, rect, visible image.Rectangle, cellStyle document.CellStyle, style gridPaintStyle) {
	area := clip.Rect(visible).Push(gtx.Ops)
	defer area.Pop()
	t := op.Offset(rect.Min).Push(gtx.Ops)
	defer t.Pop()
	sz := rect.Size()
	paint.FillShape(gtx.Ops, style.Fill, clip.Rect{Max: sz}.Op())
	border := color.NRGBA{R: 35, G: 105, B: 190, A: 255}
	paint.FillShape(gtx.Ops, border, clip.Rect{Max: image.Pt(sz.X, max(1, gtx.Dp(unit.Dp(1))))}.Op())
	paint.FillShape(gtx.Ops, border, clip.Rect{Max: image.Pt(max(1, gtx.Dp(unit.Dp(1))), sz.Y)}.Op())
	gtx.Constraints = layout.Constraints{Min: sz, Max: sz}
	editorStyle := cellEditorStyle(theme, editor, cellStyle, style)
	layout.Inset{Left: unit.Dp(5), Right: unit.Dp(4), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		return layout.Stack{Alignment: cellLayoutAlignment(style.Align, style.Vertical)}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return editorStyle.Layout(gtx)
			}),
		)
	})
	paintCellBorders(gtx, sz, cellStyle.Borders)
}

func cellEditorStyle(theme *material.Theme, editor *widget.Editor, cellStyle document.CellStyle, style gridPaintStyle) material.EditorStyle {
	editor.Alignment = cellTextAlignment(style.Align)
	editor.SingleLine = !cellStyle.WrapText
	result := material.Editor(theme, editor, "")
	result.TextSize = theme.TextSize * 14.0 / 16.0
	result.Color = style.Font
	if style.Bold {
		result.Font.Weight = cellFontWeight(true)
	}
	if style.Italic {
		result.Font.Style = font.Italic
	}
	return result
}

func paintCachedCell(gtx layout.Context, v *gridViewport, theme *material.Theme, cell document.Cell, fullRect, visibleRect image.Rectangle, style gridPaintStyle) {
	key := cellPaintKey{Value: cell.Value, Source: cell.Style, Style: style, Size: fullRect.Size(), PxPerDp: gtx.Metric.PxPerDp, PxPerSp: gtx.Metric.PxPerSp}
	entry := v.cachedCellPaint(key, func(entry *cellPaintEntry) {
		recordGtx := gtx
		recordGtx.Ops = &entry.ops
		record := op.Record(recordGtx.Ops)
		paintStyledCell(recordGtx, theme, cell, image.Rectangle{Max: fullRect.Size()}, style)
		entry.call = record.Stop()
	})
	area := clip.Rect(visibleRect).Push(gtx.Ops)
	transform := op.Offset(fullRect.Min).Push(gtx.Ops)
	entry.call.Add(gtx.Ops)
	transform.Pop()
	area.Pop()
}

func (v *gridViewport) cachedCellPaint(key cellPaintKey, record func(*cellPaintEntry)) *cellPaintEntry {
	if v.CellPaintCache == nil {
		v.CellPaintCache = make(map[cellPaintKey]*cellPaintEntry)
	}
	entry := v.CellPaintCache[key]
	if entry == nil {
		if len(v.CellPaintCache) >= maxCachedCellPaints {
			oldest := v.CellPaintFIFO[0]
			v.CellPaintFIFO = v.CellPaintFIFO[1:]
			delete(v.CellPaintCache, oldest)
		}
		entry = &cellPaintEntry{}
		record(entry)
		v.CellPaintCache[key] = entry
		v.CellPaintFIFO = append(v.CellPaintFIFO, key)
	}
	return entry
}

func paintTextBox(gtx layout.Context, theme *material.Theme, text string, rect image.Rectangle, fg, bg color.NRGBA, bold bool) {
	if rect.Empty() {
		return
	}
	transform := op.Offset(rect.Min).Push(gtx.Ops)
	defer transform.Pop()
	sz := rect.Size()
	paint.FillShape(gtx.Ops, bg, clip.Rect{Max: sz}.Op())
	paint.FillShape(gtx.Ops, gridLine, clip.Rect{Min: image.Pt(0, sz.Y-1), Max: sz}.Op())
	paint.FillShape(gtx.Ops, gridLine, clip.Rect{Min: image.Pt(sz.X-1, 0), Max: sz}.Op())
	gtx.Constraints = layout.Constraints{Min: sz, Max: sz}
	label := material.Body2(theme, text)
	label.Color = fg
	if bold {
		label.Font.Weight = cellFontWeight(true)
	}
	layout.Inset{Left: unit.Dp(5), Right: unit.Dp(4), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
			layout.Stacked(label.Layout),
		)
	})
}

func paintStyledCell(gtx layout.Context, theme *material.Theme, cell document.Cell, rect image.Rectangle, style gridPaintStyle) {
	paintTextBox(gtx, theme, "", rect, style.Font, style.Fill, false)
	clipOp := clip.Rect(rect).Push(gtx.Ops)
	defer clipOp.Pop()
	transform := op.Offset(rect.Min).Push(gtx.Ops)
	defer transform.Pop()
	size := rect.Size()
	gtx.Constraints = layout.Constraints{Min: size, Max: size}
	label := material.Body2(theme, cell.Value)
	label.Color = style.Font
	if style.Bold {
		label.Font.Weight = cellFontWeight(true)
	}
	if style.Italic {
		label.Font.Style = font.Italic
	}
	label.Alignment = cellTextAlignment(style.Align)
	if style.Wrap {
		label.WrapPolicy = text.WrapWords
	}
	layout.Inset{Left: unit.Dp(5), Right: unit.Dp(4), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		return layout.Stack{Alignment: cellLayoutAlignment(style.Align, style.Vertical)}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
			layout.Stacked(label.Layout),
		)
	})
	paintCellBorders(gtx, size, cell.Style.Borders)
}

func cellFontWeight(bold bool) font.Weight {
	if bold {
		return font.Bold
	}
	return font.Normal
}

func cellLayoutAlignment(horizontal, vertical string) layout.Direction {
	var column int
	switch strings.ToLower(horizontal) {
	case "center":
		column = 1
	case "right":
		column = 2
	}
	var row int
	switch strings.ToLower(vertical) {
	case "top":
		row = -1
	case "bottom":
		row = 1
	}
	grid := [3][3]layout.Direction{
		{layout.NW, layout.N, layout.NE},
		{layout.W, layout.Center, layout.E},
		{layout.SW, layout.S, layout.SE},
	}
	return grid[row+1][column]
}

func paintCellBorders(gtx layout.Context, size image.Point, borders [4]document.CellBorder) {
	for i, b := range borders {
		if b.Style <= 0 {
			continue
		}
		c := parseGridColor(b.Color, gridLine)
		var r image.Rectangle
		switch i {
		case 0:
			r = image.Rect(0, 0, size.X, 1)
		case 1:
			r = image.Rect(size.X-1, 0, size.X, size.Y)
		case 2:
			r = image.Rect(0, size.Y-1, size.X, size.Y)
		case 3:
			r = image.Rect(0, 0, 1, size.Y)
		}
		paint.FillShape(gtx.Ops, c, clip.Rect(r).Op())
	}
}

func cellTextAlignment(alignment string) text.Alignment {
	switch strings.ToLower(alignment) {
	case "center":
		return text.Middle
	case "right":
		return text.End
	default:
		return text.Start
	}
}

func columnName(column int) string { return strings.TrimRight(xlsx.CellName(column, 1), "0123456789") }
