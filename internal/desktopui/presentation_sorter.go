package desktopui

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
)

func (a *App) layoutPresentationSorter(gtx layout.Context, view *presentationViewState) layout.Dimensions {
	slides := view.snapshot.Slides
	if len(slides) == 0 {
		return layout.Dimensions{}
	}
	view.sorterList.List.Axis = layout.Vertical
	return layout.Inset{Top: unit.Dp(20), Right: unit.Dp(24), Bottom: unit.Dp(16), Left: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Label(a.theme, unit.Sp(14), "幻灯片浏览")
				label.Color = pptxTextSecondary
				label.Font.Weight = font.Medium
				return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, label.Layout)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				gap := gtx.Dp(unit.Dp(16))
				minWidth := max(1, gtx.Dp(unit.Dp(220)))
				columns := max(1, (gtx.Constraints.Max.X+gap)/(minWidth+gap))
				columns = min(columns, len(slides))
				rows := (len(slides) + columns - 1) / columns
				return material.List(a.theme, &view.sorterList).Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
					children := make([]layout.FlexChild, 0, columns)
					for column := 0; column < columns; column++ {
						index := row*columns + column
						if index >= len(slides) {
							children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Dimensions{}
							}))
							continue
						}
						slide := slides[index]
						children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return a.layoutPresentationSorterCard(gtx, view, index, slide)
						}))
					}
					return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start, Gap: gap}.Layout(gtx, children...)
					})
				})
			}),
		)
	})
}

func (a *App) layoutPresentationSorterCard(gtx layout.Context, view *presentationViewState, index int, slide document.PresentationSlide) layout.Dimensions {
	button := view.sorterButton(slide.ID)
	if button.Clicked(gtx) {
		a.selectPresentationSorterSlide(view, slide.ID)
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				bounds := image.Rectangle{Max: gtx.Constraints.Max}
				shape := clip.UniformRRect(bounds, gtx.Dp(unit.Dp(8)))
				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, shape.Op(gtx.Ops))
				border := pptxBorderColor
				width := float32(gtx.Dp(unit.Dp(1)))
				if view.activeSlide == index {
					border = pptxAccentColor
					width = float32(gtx.Dp(unit.Dp(2)))
				}
				paint.FillShape(gtx.Ops, border, clip.Stroke{Path: shape.Path(gtx.Ops), Width: width}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(8), Top: unit.Dp(8), Right: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							width := gtx.Constraints.Max.X
							height := max(gtx.Dp(unit.Dp(100)), int(float64(width)*9/16))
							thumb := gtx
							thumb.Constraints = layout.Exact(image.Pt(width, height))
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return a.layoutPresentationThumbnail(thumb, slide)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							title := slide.Title
							if title == "" {
								title = fmt.Sprintf("幻灯片 %d", index+1)
							}
							label := material.Label(a.theme, unit.Sp(11), fmt.Sprintf("%d. %s", index+1, title))
							label.Color = pptxTextSecondary
							label.MaxLines = 1
							return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, label.Layout)
						}),
					)
				})
			},
		)
	})
}

func (a *App) selectPresentationSorterSlide(view *presentationViewState, slideID uint32) {
	if view == nil || a.pptxDoc == nil {
		a.setStatus("演示文稿已关闭，请重新打开幻灯片浏览")
		return
	}
	current := a.pptxDoc.Snapshot()
	for index, slide := range current.Slides {
		if slide.ID != slideID {
			continue
		}
		raw, err := json.Marshal(map[string]any{
			"operation": "set_mode", "mode": "normal", "page_number": index + 1,
		})
		if err == nil {
			_, err = a.applyPresentationViewIntent("presentation.view.set", raw)
		}
		if err != nil {
			a.setStatus("选择幻灯片失败：" + err.Error())
		}
		return
	}
	a.setStatus("目标幻灯片已删除，请重新打开幻灯片浏览")
}

func (view *presentationViewState) sorterButton(slideID uint32) *widget.Clickable {
	if button, ok := view.sorterButtons[slideID]; ok {
		return button
	}
	button := &widget.Clickable{}
	view.sorterButtons[slideID] = button
	return button
}
