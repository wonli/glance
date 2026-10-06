package desktopui

import (
	"fmt"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"glance/internal/document"
	"image"
)

func (a *App) layoutMarkdownTable(gtx layout.Context, b document.MarkdownBlock, block int) layout.Dimensions {
	width := gtx.Constraints.Max.X
	y := 0
	padding := gtx.Dp(8)
	for rowIndex, row := range b.Rows {
		if len(row) == 0 {
			continue
		}
		calls := make([]op.CallOp, len(row))
		dims := make([]layout.Dimensions, len(row))
		rowHeight := 0
		for col, cell := range row {
			cellWidth := width*(col+1)/len(row) - width*col/len(row)
			inner := max(1, cellWidth-2*padding)
			cellGtx := gtx
			cellGtx.Constraints = layout.Constraints{Max: image.Pt(inner, 1<<20)}
			spans := []document.MarkdownSpan{{Text: cell}}
			if rowIndex < len(b.TableSpans) && col < len(b.TableSpans[rowIndex]) {
				spans = b.TableSpans[rowIndex][col]
			}
			record := op.Record(gtx.Ops)
			dims[col] = a.layoutMarkdownSpans(cellGtx, spans, 14, homeText, rowIndex == 0, fmt.Sprintf("table-%d-%d-%d", block, rowIndex, col))
			calls[col] = record.Stop()
			rowHeight = max(rowHeight, dims[col].Size.Y+padding*2)
		}
		for col := range row {
			x := width * col / len(row)
			cellWidth := width*(col+1)/len(row) - x
			push := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			bounds := clip.Rect{Max: image.Pt(cellWidth, rowHeight)}
			paint.FillShape(gtx.Ops, jsonSidebarColor, bounds.Op())
			paint.FillShape(gtx.Ops, jsonBorderColor, clip.Stroke{Path: bounds.Path(), Width: float32(max(1, gtx.Dp(1)))}.Op())
			textX := padding
			extra := max(0, cellWidth-2*padding-dims[col].Size.X)
			if col < len(b.TableAlignments) {
				switch b.TableAlignments[col] {
				case "center":
					textX += extra / 2
				case "right":
					textX += extra
				}
			}
			offset := op.Offset(image.Pt(textX, padding)).Push(gtx.Ops)
			calls[col].Add(gtx.Ops)
			offset.Pop()
			push.Pop()
		}
		y += rowHeight
	}
	return layout.Dimensions{Size: image.Pt(width, y)}
}
