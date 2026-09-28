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
				rect := cellRect(v.Rows, v.Columns, cr, v.X, v.Y)
				rect = rect.Add(image.Pt(rowHead, colHead))
				rect = rect.Intersect(image.Rect(rowHead, colHead, size.X, size.Y))
				if rect.Empty() {
					continue
				}
				style := paintStyle(cell.Style)
				if rr == v.SelectedRow && cc == v.SelectedColumn {
					style.Fill = selectedFill
				}
				paintStyledCell(gtx, a.theme, cell, rect, style)
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
	return layout.Dimensions{Size: size}
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
		label.Font.Weight = font.Weight(700)
	}
	layout.Inset{Left: unit.Dp(5), Right: unit.Dp(4), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return label.Layout(gtx) })
}

func paintStyledCell(gtx layout.Context, theme *material.Theme, cell document.Cell, rect image.Rectangle, style gridPaintStyle) {
	paintTextBox(gtx, theme, "", rect, style.Font, style.Fill, false)
	transform := op.Offset(rect.Min).Push(gtx.Ops)
	defer transform.Pop()
	size := rect.Size()
	gtx.Constraints = layout.Constraints{Min: size, Max: size}
	label := material.Body2(theme, cell.Value)
	label.Color = style.Font
	if style.Bold {
		label.Font.Weight = font.Weight(700)
	}
	if style.Italic {
		label.Font.Style = font.Italic
	}
	switch strings.ToLower(style.Align) {
	case "center":
		label.Alignment = text.Middle
	case "right":
		label.Alignment = text.End
	default:
		label.Alignment = text.Start
	}
	if style.Wrap {
		label.WrapPolicy = text.WrapWords
	}
	layout.Inset{Left: unit.Dp(5), Right: unit.Dp(4), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return label.Layout(gtx) })
	for i, b := range cell.Style.Borders {
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
func columnName(column int) string { return strings.TrimRight(xlsx.CellName(column, 1), "0123456789") }
