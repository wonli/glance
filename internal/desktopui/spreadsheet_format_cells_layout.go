package desktopui

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (a *App) layoutSpreadsheetFormatCellsDialog(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil {
		return layout.Dimensions{}
	}
	draft := &state.spreadsheetFormatCells
	draft.init()
	for index := range draft.categories {
		if draft.categories[index].Clicked(gtx) {
			draft.category = spreadsheetFormatCellsCategory(index)
		}
	}
	switch draft.category {
	case spreadsheetFormatCellsNumber:
		for index := range draft.numberChoices {
			choice := &draft.numberChoices[index]
			if choice.button.Clicked(gtx) {
				draft.numberFormat, draft.numberTouched = choice.value, true
			}
		}
	case spreadsheetFormatCellsAlignment:
		for index := range draft.alignChoices {
			choice := &draft.alignChoices[index]
			if choice.button.Clicked(gtx) {
				draft.alignment, draft.alignmentTouched = choice.value, true
			}
		}
		for index := range draft.verticalChoices {
			choice := &draft.verticalChoices[index]
			if choice.button.Clicked(gtx) {
				draft.verticalAlignment, draft.verticalTouched = choice.value, true
			}
		}
		if draft.wrapButton.Clicked(gtx) {
			draft.wrapText = !draft.wrapText
			draft.wrapTouched = true
		}
	case spreadsheetFormatCellsFont:
		for index := range draft.familyChoices {
			choice := &draft.familyChoices[index]
			if choice.button.Clicked(gtx) {
				draft.fontFamily, draft.familyTouched = choice.value, true
			}
		}
		for index := range draft.sizeChoices {
			choice := &draft.sizeChoices[index]
			if choice.button.Clicked(gtx) {
				draft.fontSize, draft.sizeTouched = parseSpreadsheetFontSize(choice.value), true
			}
		}
		if draft.boldButton.Clicked(gtx) {
			draft.bold, draft.boldTouched = !draft.bold, true
		}
		if draft.italicButton.Clicked(gtx) {
			draft.italic, draft.italicTouched = !draft.italic, true
		}
		if draft.underlineButton.Clicked(gtx) {
			draft.underline, draft.underlineTouched = !draft.underline, true
		}
		for index := range draft.fontColors {
			choice := &draft.fontColors[index]
			if choice.button.Clicked(gtx) {
				draft.fontColor, draft.fontColorTouched = choice.value, true
			}
		}
	case spreadsheetFormatCellsBorder:
		for index := range draft.borderChoices {
			choice := &draft.borderChoices[index]
			if choice.button.Clicked(gtx) {
				draft.borderPreset, draft.borderTouched = choice.value, true
			}
		}
	case spreadsheetFormatCellsFill:
		for index := range draft.fillColors {
			choice := &draft.fillColors[index]
			if choice.button.Clicked(gtx) {
				draft.fillColor, draft.fillTouched = choice.value, true
			}
		}
	}

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutSpreadsheetFormatCellsContents(gtx, draft)
		}),
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (a *App) layoutSpreadsheetFormatCellsContents(gtx layout.Context, draft *spreadsheetFormatCellsDialogState) layout.Dimensions {
	if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(560)) {
		return a.layoutSpreadsheetFormatCellsCompactContents(gtx, draft)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				width := gtx.Dp(unit.Dp(100))
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutSpreadsheetFormatCellsCategory(gtx, draft, spreadsheetFormatCellsNumber, "数字", 0, width)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutSpreadsheetFormatCellsCategory(gtx, draft, spreadsheetFormatCellsAlignment, "对齐", 1, width)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutSpreadsheetFormatCellsCategory(gtx, draft, spreadsheetFormatCellsFont, "字体", 2, width)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutSpreadsheetFormatCellsCategory(gtx, draft, spreadsheetFormatCellsBorder, "边框", 3, width)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutSpreadsheetFormatCellsCategory(gtx, draft, spreadsheetFormatCellsFill, "填充", 4, width)
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(1), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := gtx.Dp(unit.Dp(220))
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
				return fill(gtx, color.NRGBA{R: 228, G: 232, B: 238, A: 255})
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutSpreadsheetFormatCellsCategoryPage(gtx, draft)
		}),
	)
}

func (a *App) layoutSpreadsheetFormatCellsCompactContents(gtx layout.Context, draft *spreadsheetFormatCellsDialogState) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutSpreadsheetFormatCellsCategoryGrid(gtx, draft)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := gtx.Dp(unit.Dp(1))
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
				return fill(gtx, color.NRGBA{R: 228, G: 232, B: 238, A: 255})
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutSpreadsheetFormatCellsCategoryPage(gtx, draft)
		}),
	)
}

func (a *App) layoutSpreadsheetFormatCellsCategoryGrid(gtx layout.Context, draft *spreadsheetFormatCellsDialogState) layout.Dimensions {
	const maxColumns = 3
	gap := gtx.Dp(unit.Dp(8))
	columns := fitSpreadsheetFormatCellsColumns(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(88)), gap, maxColumns)
	columnWidth := (gtx.Constraints.Max.X - gap*(columns-1)) / columns
	rows := make([]layout.FlexChild, 0, (len(draft.categories)+columns-1)/columns*2)
	labels := [...]string{"数字", "对齐", "字体", "边框", "填充"}
	for start := 0; start < len(labels); start += columns {
		rowStart := start
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			end := min(rowStart+columns, len(labels))
			buttons := make([]layout.FlexChild, 0, (end-rowStart)*2)
			for index := rowStart; index < end; index++ {
				buttonIndex := index
				buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutSpreadsheetFormatCellsCategory(gtx, draft, spreadsheetFormatCellsCategory(buttonIndex), labels[buttonIndex], buttonIndex, columnWidth)
				}))
				if index+1 < end {
					buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx)
					}))
				}
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, buttons...)
		}))
		if start+columns < len(labels) {
			rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Spacer{Height: unit.Dp(4)}.Layout(gtx)
			}))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

func (a *App) layoutSpreadsheetFormatCellsCategory(gtx layout.Context, draft *spreadsheetFormatCellsDialogState, category spreadsheetFormatCellsCategory, label string, index, widthPX int) layout.Dimensions {
	button := &draft.categories[index]
	return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return a.layoutSpreadsheetFormatCellsChoice(gtx, button, label, draft.category == category, widthPX, 36)
	})
}

func fitSpreadsheetFormatCellsColumns(availableWidth, minColumnWidth, gap, maxColumns int) int {
	if maxColumns < 1 {
		return 1
	}
	if minColumnWidth < 1 {
		minColumnWidth = 1
	}
	if gap < 0 {
		gap = 0
	}
	columns := (availableWidth + gap) / (minColumnWidth + gap)
	if columns < 1 {
		columns = 1
	}
	if columns > maxColumns {
		columns = maxColumns
	}
	return columns
}

func (a *App) layoutSpreadsheetFormatCellsCategoryPage(gtx layout.Context, draft *spreadsheetFormatCellsDialogState) layout.Dimensions {
	children := []layout.FlexChild{}
	switch draft.category {
	case spreadsheetFormatCellsNumber:
		children = append(children, a.formatCellsSection("分类"), a.formatCellsGrid(draft.numberChoices, 3, draft.numberFormat))
	case spreadsheetFormatCellsAlignment:
		children = append(children,
			a.formatCellsSection("水平对齐"),
			a.formatCellsGrid(draft.alignChoices, 4, draft.alignment),
			a.formatCellsSection("垂直对齐"),
			a.formatCellsGrid(draft.verticalChoices, 3, draft.verticalAlignment),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.layoutSpreadsheetFormatCellsButton(gtx, &draft.wrapButton, formatCellsToggleLabel("自动换行", draft.wrapText), draft.wrapText, 144, 36)
				})
			}),
		)
	case spreadsheetFormatCellsFont:
		if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(440)) {
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutSpreadsheetFormatCellsFontOptions(gtx, draft)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						height := gtx.Dp(unit.Dp(1))
						gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
						return fill(gtx, color.NRGBA{R: 228, G: 232, B: 238, A: 255})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutSpreadsheetFormatCellsColorColumn(gtx, draft.fontColors, draft.fontColor, false)
				}),
			)
		} else {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutSpreadsheetFormatCellsFontOptions(gtx, draft)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutSpreadsheetFormatCellsColorColumn(gtx, draft.fontColors, draft.fontColor, false)
						})
					}),
				)
			}))
		}
	case spreadsheetFormatCellsBorder:
		children = append(children,
			a.formatCellsSection("边框预设"),
			a.formatCellsGrid(draft.borderChoices, 2, draft.borderPreset),
		)
	case spreadsheetFormatCellsFill:
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutSpreadsheetFormatCellsColorColumn(gtx, draft.fillColors, draft.fillColor, true)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (a *App) formatCellsSection(label string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		text := material.Caption(a.theme, label)
		text.TextSize = unit.Sp(12)
		text.Color = a.tokens().Colors.Text.Secondary
		text.Font.Weight = font.SemiBold
		return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(8)}.Layout(gtx, text.Layout)
	})
}

func (a *App) formatCellsGrid(choices []spreadsheetFormatCellChoice, columns int, current string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if gtx.Constraints.Max.X <= 0 || columns < 1 {
			return layout.Dimensions{}
		}
		gap := gtx.Dp(unit.Dp(8))
		columns = fitSpreadsheetFormatCellsColumns(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(64)), gap, columns)
		columnWidth := (gtx.Constraints.Max.X - gap*max(0, columns-1)) / columns
		if columnWidth < 1 {
			columnWidth = 1
		}
		rows := make([]layout.FlexChild, 0, ((len(choices)+columns-1)/columns)*2)
		for start := 0; start < len(choices); start += columns {
			rowStart := start
			rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				end := min(rowStart+columns, len(choices))
				rowChildren := make([]layout.FlexChild, 0, (end-rowStart)*2)
				for index := rowStart; index < end; index++ {
					choice := &choices[index]
					rowChildren = append(rowChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutSpreadsheetFormatCellsChoice(gtx, &choice.button, choice.label, current == choice.value, columnWidth, 36)
					}))
					if index+1 < end {
						rowChildren = append(rowChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }))
					}
				}
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, rowChildren...)
			}))
			if start+columns < len(choices) {
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(8)}.Layout(gtx) }))
			}
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	})
}

func (a *App) layoutSpreadsheetFormatCellsFontOptions(gtx layout.Context, draft *spreadsheetFormatCellsDialogState) layout.Dimensions {
	children := []layout.FlexChild{
		a.formatCellsSection("字体"),
		a.formatCellsGrid(draft.familyChoices, 3, draft.fontFamily),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutSpreadsheetFormatCellsInlineLabel(gtx, "字号（磅）")
			})
		}),
		a.formatCellsGrid(draft.sizeChoices, 5, formatSpreadsheetFontSize(draft.fontSize)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutSpreadsheetFormatCellsFlexibleButton(gtx, &draft.boldButton, "粗体", draft.bold)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(6)}.Layout(gtx) }),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutSpreadsheetFormatCellsFlexibleButton(gtx, &draft.italicButton, "斜体", draft.italic)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(6)}.Layout(gtx) }),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutSpreadsheetFormatCellsFlexibleButton(gtx, &draft.underlineButton, "下划线", draft.underline)
					}),
				)
			})
		}),
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (a *App) layoutSpreadsheetFormatCellsFlexibleButton(gtx layout.Context, button *widget.Clickable, label string, selected bool) layout.Dimensions {
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(34))
		gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, height)
		gtx.Constraints.Max = gtx.Constraints.Min
		return a.layoutSpreadsheetFormatCellsButtonFace(gtx, label, selected)
	})
}

func (a *App) layoutSpreadsheetFormatCellsInlineLabel(gtx layout.Context, value string) layout.Dimensions {
	label := material.Caption(a.theme, value)
	label.TextSize = unit.Sp(12)
	label.Color = a.tokens().Colors.Text.Secondary
	label.Font.Weight = font.SemiBold
	return label.Layout(gtx)
}

func (a *App) layoutSpreadsheetFormatCellsColorColumn(gtx layout.Context, choices []spreadsheetFormatCellColorChoice, current string, fill bool) layout.Dimensions {
	section := "字体颜色"
	if fill {
		section = "填充颜色"
	}
	children := []layout.FlexChild{a.formatCellsSection(section)}
	for index := range choices {
		choice := &choices[index]
		selected := strings.EqualFold(current, choice.value)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutSpreadsheetFormatCellsColorChoice(gtx, choice, selected)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (a *App) layoutSpreadsheetFormatCellsColorChoice(gtx layout.Context, choice *spreadsheetFormatCellColorChoice, selected bool) layout.Dimensions {
	return choice.button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width, height := gtx.Dp(unit.Dp(126)), gtx.Dp(unit.Dp(30))
		if width > gtx.Constraints.Max.X {
			width = gtx.Constraints.Max.X
		}
		gtx.Constraints = layout.Exact(image.Pt(width, height))
		bounds := image.Rectangle{Max: gtx.Constraints.Max}
		colors := a.tokens().Colors
		colors = officeAccentTokens(colors, colors.OfficeAccent.Spreadsheet)
		bg := colors.Surface.Surface
		if selected {
			bg = colors.Toolbar.ActiveBackground
		}
		shape := clip.UniformRRect(bounds, gtx.Dp(unit.Dp(6)))
		paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
		border := colors.Surface.Border
		if selected {
			border = colors.Toolbar.ActiveBorder
		}
		paint.FillShape(gtx.Ops, border, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
		swatch := image.Rect(gtx.Dp(unit.Dp(9)), gtx.Dp(unit.Dp(7)), gtx.Dp(unit.Dp(25)), gtx.Dp(unit.Dp(23)))
		if choice.none {
			paint.FillShape(gtx.Ops, colors.Surface.Surface, clip.Rect(swatch).Op())
			paint.FillShape(gtx.Ops, colors.Surface.Border, clip.Stroke{Path: clip.Rect(swatch).Path(), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
		} else {
			paint.FillShape(gtx.Ops, choice.swatch, clip.UniformRRect(swatch, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
		}
		label := material.Caption(a.theme, choice.label)
		label.TextSize = unit.Sp(11)
		label.Color = colors.Text.PureBlack
		return layout.Inset{Left: unit.Dp(31), Right: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return layout.Center.Layout(gtx, label.Layout)
		})
	})
}

func (a *App) layoutSpreadsheetFormatCellsChoice(gtx layout.Context, button *widget.Clickable, label string, selected bool, widthPX, heightDP int) layout.Dimensions {
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(heightDP))
		if widthPX > gtx.Constraints.Max.X {
			widthPX = gtx.Constraints.Max.X
		}
		if widthPX < 0 {
			widthPX = 0
		}
		gtx.Constraints = layout.Exact(image.Pt(widthPX, height))
		return a.layoutSpreadsheetFormatCellsButtonFace(gtx, label, selected)
	})
}

func (a *App) layoutSpreadsheetFormatCellsButton(gtx layout.Context, button *widget.Clickable, label string, selected bool, widthDP, heightDP int) layout.Dimensions {
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := gtx.Dp(unit.Dp(widthDP))
		if width > gtx.Constraints.Max.X {
			width = gtx.Constraints.Max.X
		}
		if width < 0 {
			width = 0
		}
		gtx.Constraints = layout.Exact(image.Pt(width, gtx.Dp(unit.Dp(heightDP))))
		return a.layoutSpreadsheetFormatCellsButtonFace(gtx, label, selected)
	})
}

func (a *App) layoutSpreadsheetFormatCellsButtonFace(gtx layout.Context, label string, selected bool) layout.Dimensions {
	bounds := image.Rectangle{Max: gtx.Constraints.Max}
	colors := a.tokens().Colors
	colors = officeAccentTokens(colors, colors.OfficeAccent.Spreadsheet)
	bg := colors.Surface.Surface
	if selected {
		bg = colors.Toolbar.ActiveBackground
	}
	shape := clip.UniformRRect(bounds, gtx.Dp(unit.Dp(6)))
	paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
	border := colors.Surface.Border
	if selected {
		border = colors.Toolbar.ActiveBorder
	}
	paint.FillShape(gtx.Ops, border, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp(label).Add(gtx.Ops)
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		text := material.Caption(a.theme, label)
		text.TextSize = unit.Sp(11)
		text.Color = colors.Text.PureBlack
		text.MaxLines = 2
		return text.Layout(gtx)
	})
}

func formatCellsToggleLabel(label string, enabled bool) string {
	if enabled {
		return "✓  " + label
	}
	return label
}
