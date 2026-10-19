package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/i18n"
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
				label := material.Label(a.theme, unit.Sp(14), a.textID("presentation.ribbon.action.view_sorter"))
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
								title = a.textArgs("presentation.sorter.slide_number", i18n.Args{"Number": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(index+1))})
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

type presentationsorterGoOperationModeArguments struct {
	Operation  string `json:"operation"`
	Mode       string `json:"mode"`
	PageNumber int    `json:"page_number"`
}

func (a *App) selectPresentationSorterSlide(view *presentationViewState, slideID uint32) {
	if view == nil || a.pptxDoc == nil {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_was_closed_reopen_slide_s_aa0078"}, feedbackInfo, notificationToast)
		return
	}
	current := a.pptxDoc.Snapshot()
	for index, slide := range current.Slides {
		if slide.ID != slideID {
			continue
		}
		raw, err := json.Marshal(presentationsorterGoOperationModeArguments{Operation: "set_mode", Mode: "normal", PageNumber: index + 1})
		if err == nil {
			_, err = a.applyPresentationViewIntent("presentation.view.set", raw)
		}
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, err, notificationToast)
		}
		return
	}
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_target_slide_was_deleted_reopen_slide_836f03"}, feedbackInfo, notificationToast)
}

func (view *presentationViewState) sorterButton(slideID uint32) *widget.Clickable {
	if button, ok := view.sorterButtons[slideID]; ok {
		return button
	}
	button := &widget.Clickable{}
	view.sorterButtons[slideID] = button
	return button
}
