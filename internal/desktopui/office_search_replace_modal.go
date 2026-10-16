package desktopui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (a *App) layoutOfficeSearchReplaceDialog(gtx layout.Context) layout.Dimensions {
	if a.modal.searchReplaceClose.Clicked(gtx) {
		if a.modal.OnAction != nil {
			a.modal.OnAction("close")
		} else {
			a.modal.Hide()
		}
		if !a.modal.Visible {
			return layout.Dimensions{}
		}
	}
	if a.modal.searchReplaceFindTab.Clicked(gtx) {
		a.modal.SearchReplaceReplaceTab = false
		a.modal.FocusInputRequested = true
	}
	if a.modal.SearchReplaceHasReplace && a.modal.searchReplaceReplaceTab.Clicked(gtx) {
		a.modal.SearchReplaceReplaceTab = true
		a.modal.FocusInputRequested = true
	}
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return a.layoutModalScrim(gtx)
		}),
		layout.Stacked(a.layoutOfficeSearchReplaceCard),
	)
}

func (a *App) layoutOfficeSearchReplaceCard(gtx layout.Context) layout.Dimensions {
	width := gtx.Dp(unit.Dp(560))
	if available := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(32)); available < width {
		width = available
	}
	if width <= 0 {
		return layout.Dimensions{}
	}
	heightDP := 360
	if a.modal.SearchReplaceScope != "" {
		heightDP = 384
	}
	height := gtx.Dp(unit.Dp(float32(heightDP)))
	if available := gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(24)); available < height {
		height = available
	}
	if height <= 0 {
		return layout.Dimensions{}
	}
	compact := height < gtx.Dp(unit.Dp(float32(heightDP)))
	a.modal.SearchReplaceCompact = compact
	topPadding, bottomPadding := unit.Dp(18), unit.Dp(33)
	headerDividerGap, tabGap := unit.Dp(5), unit.Dp(6)
	queryGap, summaryGap, fieldGap, footerGap := unit.Dp(17), unit.Dp(18), unit.Dp(12), unit.Dp(20)
	if compact {
		topPadding, bottomPadding = unit.Dp(6), unit.Dp(6)
		headerDividerGap, tabGap = unit.Dp(4), unit.Dp(4)
		queryGap, summaryGap, fieldGap, footerGap = unit.Dp(6), unit.Dp(6), unit.Dp(6), unit.Dp(6)
	}

	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = gtx.Constraints.Min
	child := op.Record(gtx.Ops)
	dims := layout.Inset{Top: topPadding, Bottom: bottomPadding, Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutOfficeSearchReplaceHeader(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: headerDividerGap}.Layout(gtx, a.layoutOfficeSearchReplaceFullDivider)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: tabGap}.Layout(gtx, a.layoutOfficeSearchReplaceTabs)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutOfficeSearchReplaceFullDivider(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: queryGap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.layoutOfficeSearchReplaceField(gtx, "查找内容", &a.modal.Input, "查找内容")
				})
			}),
		}
		if a.modal.SearchReplaceHasReplace && a.modal.SearchReplaceReplaceTab && a.modal.HasSecondInput {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: fieldGap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.layoutOfficeSearchReplaceField(gtx, "替换为", &a.modal.SecondInput, "替换为")
				})
			}))
		}
		if a.modal.SearchReplaceScope != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: fieldGap}.Layout(gtx, a.layoutOfficeSearchReplaceScope)
			}))
		}
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: summaryGap}.Layout(gtx, a.layoutOfficeSearchReplaceSummary)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutOfficeSearchReplaceDivider(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: footerGap}.Layout(gtx, a.layoutOfficeSearchReplaceActions)
			}),
		)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
	call := child.Stop()

	bounds := image.Rectangle{Max: dims.Size}
	radius := gtx.Dp(unit.Dp(10))
	for _, shadow := range []struct {
		offset int
		alpha  uint8
	}{{10, 10}, {6, 12}, {3, 20}} {
		shadowBounds := bounds.Add(image.Pt(0, gtx.Dp(unit.Dp(shadow.offset))))
		shadowShape := clip.RRect{Rect: shadowBounds, NW: radius, NE: radius, SW: radius, SE: radius}
		paint.FillShape(gtx.Ops, color.NRGBA{R: 31, G: 41, B: 55, A: shadow.alpha}, shadowShape.Op(gtx.Ops))
	}
	card := clip.RRect{Rect: bounds, NW: radius, NE: radius, SW: radius, SE: radius}
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, card.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 216, G: 222, B: 230, A: 255}, clip.Stroke{Path: card.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
	call.Add(gtx.Ops)
	return dims
}

func (a *App) layoutOfficeSearchReplaceHeader(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			label := material.Body1(a.theme, a.modal.Title)
			label.TextSize = unit.Sp(16)
			label.Font.Weight = font.SemiBold
			label.Color = color.NRGBA{R: 31, G: 37, B: 45, A: 255}
			return label.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width, height := gtx.Dp(unit.Dp(32)), gtx.Dp(unit.Dp(32))
			if a.modal.SearchReplaceCompact {
				width, height = gtx.Dp(unit.Dp(28)), gtx.Dp(unit.Dp(28))
			}
			gtx.Constraints.Min = image.Pt(width, height)
			gtx.Constraints.Max = gtx.Constraints.Min
			return a.modal.searchReplaceClose.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := material.Body1(a.theme, "×")
				label.TextSize = unit.Sp(22)
				label.Color = color.NRGBA{R: 104, G: 113, B: 124, A: 255}
				return layout.Center.Layout(gtx, label.Layout)
			})
		}),
	)
}

func (a *App) layoutOfficeSearchReplaceTabs(gtx layout.Context) layout.Dimensions {
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutOfficeSearchReplaceTab(gtx, a.theme, &a.modal.searchReplaceFindTab, "查找", a.modal.SearchReplaceCompact, !a.modal.SearchReplaceReplaceTab)
		}),
	}
	if a.modal.SearchReplaceHasReplace {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutOfficeSearchReplaceTab(gtx, a.theme, &a.modal.searchReplaceReplaceTab, "替换", a.modal.SearchReplaceCompact, a.modal.SearchReplaceReplaceTab)
			}),
		)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx, children...)
}

func layoutOfficeSearchReplaceTab(gtx layout.Context, theme *material.Theme, click *widget.Clickable, title string, compact, active bool) layout.Dimensions {
	widthDP, heightDP := unit.Dp(36), unit.Dp(36)
	if compact {
		heightDP = 28
	}
	width, height := gtx.Dp(widthDP), gtx.Dp(heightDP)
	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = gtx.Constraints.Min
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{Alignment: layout.W}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				if active {
					lineHeight := gtx.Dp(unit.Dp(2))
					line := image.Rect(-gtx.Dp(unit.Dp(6)), gtx.Constraints.Max.Y-lineHeight, gtx.Constraints.Max.X-gtx.Dp(unit.Dp(6)), gtx.Constraints.Max.Y)
					paint.FillShape(gtx.Ops, color.NRGBA{R: 52, G: 120, B: 209, A: 255}, clip.Rect{Min: line.Min, Max: line.Max}.Op())
				}
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Body2(theme, title)
						label.TextSize = unit.Sp(12)
						label.Font.Weight = font.SemiBold
						if active {
							label.Color = color.NRGBA{R: 52, G: 120, B: 209, A: 255}
						} else {
							label.Color = color.NRGBA{R: 78, G: 87, B: 98, A: 255}
						}
						return label.Layout(gtx)
					}),
				)
			}),
		)
	})
}

func (a *App) layoutOfficeSearchReplaceField(gtx layout.Context, title string, editor *widget.Editor, hint string) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width := gtx.Dp(unit.Dp(92))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			label := material.Body2(a.theme, title)
			label.TextSize = unit.Sp(12)
			label.Font.Weight = font.Medium
			label.Color = color.NRGBA{R: 78, G: 87, B: 98, A: 255}
			return label.Layout(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layoutModalInputField(gtx, a.theme, editor, hint)
		}),
	)
}

func (a *App) layoutOfficeSearchReplaceScope(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width := gtx.Dp(unit.Dp(91))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			label := material.Body2(a.theme, "范围")
			label.TextSize = unit.Sp(11)
			label.Font.Weight = font.Medium
			label.Color = color.NRGBA{R: 104, G: 113, B: 124, A: 255}
			return label.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			heightDP := unit.Dp(32)
			if a.modal.SearchReplaceCompact {
				heightDP = 28
			}
			width, height := gtx.Dp(unit.Dp(190)), gtx.Dp(heightDP)
			gtx.Constraints.Min = image.Pt(width, height)
			gtx.Constraints.Max = gtx.Constraints.Min
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					bounds := image.Rectangle{Max: gtx.Constraints.Max}
					shape := clip.RRect{Rect: bounds, NW: gtx.Dp(unit.Dp(5)), NE: gtx.Dp(unit.Dp(5)), SW: gtx.Dp(unit.Dp(5)), SE: gtx.Dp(unit.Dp(5))}
					paint.FillShape(gtx.Ops, color.NRGBA{R: 245, G: 246, B: 248, A: 255}, shape.Op(gtx.Ops))
					paint.FillShape(gtx.Ops, color.NRGBA{R: 232, G: 235, B: 239, A: 255}, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
					return layout.Dimensions{Size: bounds.Max}
				},
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						label := material.Body2(a.theme, a.modal.SearchReplaceScope)
						label.TextSize = unit.Sp(11)
						label.Color = color.NRGBA{R: 39, G: 48, B: 58, A: 255}
						return layout.Stack{Alignment: layout.W}.Layout(gtx,
							layout.Expanded(func(gtx layout.Context) layout.Dimensions {
								return layout.Dimensions{Size: gtx.Constraints.Max}
							}),
							layout.Stacked(label.Layout),
						)
					})
				},
			)
		}),
	)
}

func (a *App) layoutOfficeSearchReplaceSummary(gtx layout.Context) layout.Dimensions {
	text := a.modal.SearchReplaceSummary
	if text == "" {
		text = "输入查找内容后，点击“查找下一个”"
	}
	label := material.Caption(a.theme, text)
	label.TextSize = unit.Sp(11)
	label.Color = color.NRGBA{R: 104, G: 113, B: 124, A: 255}
	return label.Layout(gtx)
}

func (a *App) layoutOfficeSearchReplaceDivider(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(1))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return fill(gtx, color.NRGBA{R: 232, G: 235, B: 239, A: 255})
}

func (a *App) layoutOfficeSearchReplaceFullDivider(gtx layout.Context) layout.Dimensions {
	left := gtx.Dp(unit.Dp(24))
	height := gtx.Dp(unit.Dp(1))
	size := image.Pt(gtx.Constraints.Max.X+2*left, height)
	offset := op.Offset(image.Pt(-left, 0)).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 232, G: 235, B: 239, A: 255}, clip.Rect{Max: size}.Op())
	offset.Pop()
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
}

func (a *App) layoutOfficeSearchReplaceActions(gtx layout.Context) layout.Dimensions {
	buttons := make([]layout.FlexChild, 0, len(a.modal.Actions))
	for index := range a.modal.Actions {
		action := &a.modal.Actions[index]
		if action.ID == "replace" && !a.modal.SearchReplaceReplaceTab {
			continue
		}
		buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutConfirmDialogAction(gtx, a.theme, action, func() {
				if a.modal.OnAction != nil {
					a.modal.OnAction(action.ID)
				}
			})
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceStart, Gap: gtx.Dp(unit.Dp(8))}.Layout(gtx, buttons...)
}
