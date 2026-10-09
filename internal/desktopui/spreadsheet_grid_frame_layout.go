package desktopui

import (
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"image/color"
	"strconv"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func (a *App) layoutColumnHeader(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(36)
	gtx.Constraints.Max.Y = gtx.Dp(36)
	cols := previewColumns(a.grid)
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return gridHeaderCell(gtx, a.theme, "", unit.Dp(44))
	})}
	children = append(children, columnChildren(a, cols, func(gtx layout.Context, label string) layout.Dimensions {
		return gridHeaderCell(gtx, a.theme, label, unit.Dp(110))
	})...)
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func (a *App) layoutGridRow(gtx layout.Context, row int) layout.Dimensions {
	// A List gives each item the remaining viewport as Max.Y. Constrain the
	// item before painting its background, or fill() expands it to that height
	// and the first row hides every later row below a huge blank block.
	gtx.Constraints.Min.Y = gtx.Dp(36)
	gtx.Constraints.Max.Y = gtx.Dp(36)
	cols := previewColumns(a.grid)
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return gridHeaderCell(gtx, a.theme, strconv.Itoa(row+1), unit.Dp(44))
	})}
	for col := 0; col < cols; col++ {
		column := col
		ref := xlsx.CellName(column+1, row+1)
		cell := document.Cell{}
		if row < len(a.grid) && column < len(a.grid[row]) {
			cell = a.grid[row][column]
		}
		button := a.cellButton(ref)
		if button.Clicked(gtx) {
			a.selectedCell = ref
			a.persistViewState()
			a.cellEditor.SetText(cell.Value)
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(unit.Dp(90))
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := material.Body2(a.theme, cell.Value)
				if ref == a.selectedCell {
					label.Color = color.NRGBA{R: 25, G: 80, B: 145, A: 255}
				}
				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return fill(gtx, cellBackground(ref == a.selectedCell, cell.StyleID))
					},
					func(gtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(5)).Layout(gtx, func(gtx layout.Context) layout.Dimensions { return label.Layout(gtx) })
					},
				)
			})
		}))
	}
	return layout.Inset{Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions { return fill(gtx, gridLine) },
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
			},
		)
	})
}

func gridHeaderCell(gtx layout.Context, theme *material.Theme, text string, width unit.Dp) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Dp(width)
	gtx.Constraints.Max.X = gtx.Dp(width)
	gtx.Constraints.Min.Y = gtx.Dp(36)
	gtx.Constraints.Max.Y = gtx.Dp(36)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return fill(gtx, gridHeaderFill)
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.Caption(theme, text).Layout(gtx)
			})
		},
	)
}

func columnChildren(a *App, columns int, widget func(layout.Context, string) layout.Dimensions) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, columns)
	for col := 0; col < columns; col++ {
		label := xlsx.CellName(col+1, 1)
		for i, r := range label {
			if r >= '0' && r <= '9' {
				label = label[:i]
				break
			}
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return widget(gtx, label) }))
	}
	return children
}

func previewColumns(grid [][]document.Cell) int {
	max := 0
	for _, row := range grid {
		if len(row) > max {
			max = len(row)
		}
	}
	return max
}

func cellBackground(selected bool, styleID int) color.NRGBA {
	if selected {
		return selectedFill
	}
	if styleID != 0 {
		return contentSurface
	}
	return contentSurface
}

func tableRows(a *App, rows [][]string) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, len(rows))
	for _, cells := range rows {
		row := append([]string(nil), cells...)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			items := make([]layout.FlexChild, 0, len(row))
			for _, text := range row {
				value := text
				items = append(items, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return material.Body2(a.theme, value).Layout(gtx)
					})
				}))
			}
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return fill(gtx, contentSurface)
				},
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, items...)
				},
			)
		}))
	}
	return children
}

func fill(gtx layout.Context, c color.NRGBA) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, c, clip.Rect{Max: size}.Op())
	return layout.Dimensions{Size: size}
}
