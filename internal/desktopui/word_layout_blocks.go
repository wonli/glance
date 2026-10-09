package desktopui

import (
	"glance/internal/document"
	"image/color"

	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func (a *App) layoutWordList(gtx layout.Context, state *documentSessionState, blocks []document.WordBlock) layout.Dimensions {
	state.wordListState.List.Axis = layout.Vertical
	return state.wordListState.List.Layout(gtx, len(blocks)+1, func(gtx layout.Context, index int) layout.Dimensions {
		if index == len(blocks) {
			return a.layoutWordAppendTarget(gtx, state)
		}
		if blocks[index].Kind == document.WordParagraphBlock && blocks[index].Paragraph != nil {
			border, hasBorder := wordContinuousParagraphBottomBorder(blocks, index)
			return a.layoutWordParagraphWithBottomBorder(gtx, *blocks[index].Paragraph, false, border, hasBorder)
		}
		return a.layoutWordBlock(gtx, blocks[index])
	})
}

func wordContinuousParagraphBottomBorder(blocks []document.WordBlock, index int) (document.WordParagraphBorder, bool) {
	if index < 0 || index >= len(blocks) || blocks[index].Paragraph == nil {
		return document.WordParagraphBorder{}, false
	}
	paragraph := blocks[index].Paragraph
	currentSet := paragraph.Layout.Set&document.WordParagraphBottomBorderSet != 0
	if currentSet && paragraphBottomBorderVisible(paragraph.Layout.BottomBorder) {
		if index+1 < len(blocks) && blocks[index+1].Kind == document.WordParagraphBlock && blocks[index+1].Paragraph != nil && blocks[index+1].Paragraph.Layout.Set&document.WordParagraphBottomBorderSet == 0 {
			return document.WordParagraphBorder{}, false
		}
		return paragraph.Layout.BottomBorder, true
	}
	if !currentSet && index > 0 && blocks[index-1].Kind == document.WordParagraphBlock && blocks[index-1].Paragraph != nil {
		previous := blocks[index-1].Paragraph
		if previous.Layout.Set&document.WordParagraphBottomBorderSet != 0 && paragraphBottomBorderVisible(previous.Layout.BottomBorder) {
			return previous.Layout.BottomBorder, true
		}
	}
	return document.WordParagraphBorder{}, false
}

func paragraphBottomBorderVisible(border document.WordParagraphBorder) bool {
	return border.Style != "none" && border.Style != "nil" && border.SizePT > 0
}

func wordListViewportRange(position layout.Position, elements, viewportSize int) (start, end float32) {
	if position.Length <= 0 || elements <= 0 || viewportSize <= 0 {
		return 0, 1
	}
	length := float32(position.Length)
	elementLength := length / float32(elements)
	start = clampWordFraction((float32(position.First)*elementLength + float32(position.Offset)) / length)
	end = clampWordFraction((float32(position.First+position.Count)*elementLength + float32(position.OffsetLast)) / length)
	viewportFraction := end - start
	visibleFraction := min(1, float32(viewportSize)/length)
	err := visibleFraction - viewportFraction
	if viewportFraction < 1 {
		startShare := start / (1 - viewportFraction)
		endShare := (1 - end) / (1 - viewportFraction)
		start -= startShare * err
		end += endShare * err
	}
	return clampWordFraction(start), clampWordFraction(end)
}

func clampWordFraction(value float32) float32 {
	return max(0, min(1, value))
}

func (a *App) layoutWordAppendTarget(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	if state.wordReadingMode {
		height := gtx.Dp(unit.Dp(560))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	if state.wordAppendTarget.Clicked(gtx) {
		if state.selectedWordImageID != "" {
			clearWordImageSelection(state)
			gtx.Execute(key.FocusCmd{Tag: &state.wordAppendTarget})
		} else if !state.wordInsertionRunning {
			state.wordForceDraftFlush.Store(true)
			state.wordInsertAfterDraftFlush = true
			state.wordFlushRequested.Store(true)
			state.wordFlushAllDrafts.Store(true)
		}
	}
	return state.wordAppendTarget.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(560))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp("单击此处继续输入").Add(gtx.Ops)
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				if !state.wordAppendTarget.Hovered() {
					return layout.Dimensions{Size: gtx.Constraints.Max}
				}
				return layout.Inset{Top: unit.Dp(12), Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Caption(a.theme, "单击此处继续输入").Layout(gtx)
				})
			},
		)
	})
}

func (a *App) layoutWordBlock(gtx layout.Context, block document.WordBlock) layout.Dimensions {
	switch block.Kind {
	case document.WordParagraphBlock:
		if block.Paragraph != nil {
			return a.layoutWordParagraph(gtx, *block.Paragraph, false)
		}
	case document.WordTableBlock:
		if block.Table != nil {
			return a.layoutWordTable(gtx, *block.Table)
		}
	case document.WordUnsupportedBlock:
		if block.Unsupported != nil {
			return wordReadOnlyPlaceholder(gtx, a.theme, block.Unsupported.Description)
		}
	}
	return layout.Dimensions{}
}

func (a *App) layoutWordTable(gtx layout.Context, table document.WordTable) layout.Dimensions {
	columnWidths := wordTableColumnWidths(table)
	rows := make([]layout.FlexChild, 0, len(table.Rows))
	for _, row := range table.Rows {
		row := row
		cells := make([]layout.FlexChild, 0, len(row.Cells))
		for cellIndex, cell := range row.Cells {
			cell := cell
			weight := 1.0
			if cellIndex < len(columnWidths) {
				weight = columnWidths[cellIndex]
			}
			cells = append(cells, layout.Flexed(float32(weight), func(gtx layout.Context) layout.Dimensions {
				return a.layoutWordTableCell(gtx, cell)
			}))
		}
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx, cells...)
			})
		}))
	}
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return wordMeasuredBackground(gtx, gridLine, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1), Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
			})
		})
	})
}

func (a *App) layoutWordTableCell(gtx layout.Context, cell document.WordTableCell) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(cell.Paragraphs))
	for _, paragraph := range cell.Paragraphs {
		paragraph := paragraph
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordParagraph(gtx, paragraph, true)
		}))
	}
	fill := contentSurface
	if cellFill, ok := parseWordRunColor(cell.FillColor); ok {
		fill = cellFill
	}
	return wordMeasuredBackground(gtx, fill, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
}

func wordTableColumnWidths(table document.WordTable) []float64 {
	columns := len(table.Layout.GridColumnsEMU)
	for _, row := range table.Rows {
		if len(row.Cells) > columns {
			columns = len(row.Cells)
		}
	}
	if columns == 0 {
		return nil
	}
	widths := make([]float64, columns)
	for index, emu := range table.Layout.GridColumnsEMU {
		if emu > 0 {
			widths[index] = float64(emu)
		}
	}
	for _, row := range table.Rows {
		for index, cell := range row.Cells {
			if widths[index] <= 0 && cell.WidthEMU > 0 {
				widths[index] = float64(cell.WidthEMU)
			}
		}
	}
	knownTotal := 0.0
	knownCount := 0
	for _, width := range widths {
		if width > 0 {
			knownTotal += width
			knownCount++
		}
	}
	if knownCount == 0 {
		for index := range widths {
			widths[index] = 1
		}
		return widths
	}
	average := knownTotal / float64(knownCount)
	for index := range widths {
		if widths[index] <= 0 {
			widths[index] = average
		}
	}
	return widths
}

// wordMeasuredBackground paints a list item's measured bounds without claiming
// the full remaining viewport height. Returning viewport-sized fill dimensions
// here makes a vertical List believe its first paragraph consumes the screen.
func wordMeasuredBackground(gtx layout.Context, fill color.NRGBA, child func(layout.Context) layout.Dimensions) layout.Dimensions {
	recorder := op.Record(gtx.Ops)
	dimensions := child(gtx)
	call := recorder.Stop()
	paint.FillShape(gtx.Ops, fill, clip.Rect{Max: dimensions.Size}.Op())
	call.Add(gtx.Ops)
	return dimensions
}
