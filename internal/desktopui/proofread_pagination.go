package desktopui

import (
	"image"
	"image/color"
	"strconv"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/i18n"
)

var proofreadPaginationSurface = color.NRGBA{R: 245, G: 247, B: 249, A: 255}

func (a *App) layoutProofreadingPagination(gtx layout.Context) layout.Dimensions {
	height := min(gtx.Dp(140), gtx.Constraints.Max.Y)
	height = max(0, height)
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, proofreadPaginationSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, proofreadLine, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, max(1, gtx.Dp(1)))}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			if a.pdfDoc == nil {
				return a.layoutProofreadingPaginationWaiting(gtx)
			}
			view := a.ensurePDFView()
			view.thumbList.Axis = layout.Horizontal
			view.thumbnails = true
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutProofreadingPaginationHeading(gtx, view)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return a.layoutProofreadingPaginationPages(gtx, view)
				}),
			)
		},
	)
}

func (a *App) layoutProofreadingPaginationHeading(gtx layout.Context, view *pdfViewState) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(9), Bottom: unit.Dp(8), Left: unit.Dp(22), Right: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Label(a.theme, 13, a.textID("proofread.pagination.title"))
				label.Color = proofreadInk
				label.Font.Weight = font.Bold
				child := gtx
				child.Constraints.Min.X = gtx.Dp(100)
				child.Constraints.Max.X = gtx.Dp(100)
				return layoutProofreadingOpticallyCenteredLabel(child, label)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Label(a.theme, 10, a.textID("proofread.pagination.instruction"))
				label.Color = proofreadMuted
				label.MaxLines = 1
				return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layoutProofreadingOpticallyCenteredLabel(gtx, label)
				})
			}),
		)
	})
}

func (a *App) layoutProofreadingPaginationPages(gtx layout.Context, view *pdfViewState) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(22), Right: unit.Dp(22), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutProofreadingPageArrow(gtx, view, "previous", "chevron-left", view.page > 0)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Dp(13), 0)}
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				view.thumbList.Axis = layout.Horizontal
				return view.thumbList.Layout(gtx, view.info.Pages, func(gtx layout.Context, index int) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutProofreadingPageThumbnail(gtx, view, index)
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Dp(13), 0)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutProofreadingPageArrow(gtx, view, "next", "chevron-right", view.page+1 < view.info.Pages)
			}),
		)
	})
}

func (a *App) layoutProofreadingPageArrow(gtx layout.Context, view *pdfViewState, action, iconName string, enabled bool) layout.Dimensions {
	size := image.Pt(gtx.Dp(34), gtx.Dp(34))
	gtx.Constraints = layout.Exact(size)
	bg := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	stroke := proofreadLine
	iconColor := proofreadMuted
	if !enabled {
		bg = proofreadPaginationSurface
		stroke = color.NRGBA{R: 229, G: 233, B: 238, A: 255}
		iconColor = color.NRGBA{R: 174, G: 181, B: 188, A: 255}
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rect(0, 0, size.X, size.Y), gtx.Dp(5)).Op(gtx.Ops))
			proofreadPaginationBorder(gtx, size, stroke, gtx.Dp(1))
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			return view.button(action).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if img, ok := pdfToolbarIcon(iconName, iconColor); ok {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(16), gtx.Dp(16)))
						return widget.Image{Src: paint.NewImageOp(img), Fit: widget.Contain}.Layout(gtx)
					})
				}
				glyph := "‹"
				if action == "next" {
					glyph = "›"
				}
				label := material.Label(a.theme, 20, glyph)
				label.Color = iconColor
				return layout.Center.Layout(gtx, label.Layout)
			})
		},
	)
}

func (a *App) layoutProofreadingPageThumbnail(gtx layout.Context, view *pdfViewState, index int) layout.Dimensions {
	size := image.Pt(gtx.Dp(96), gtx.Dp(96))
	gtx.Constraints = layout.Exact(size)
	selected := index == view.page
	fill := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	border := color.NRGBA{R: 221, G: 227, B: 232, A: 255}
	borderWidth := gtx.Dp(1)
	if selected {
		fill = color.NRGBA{R: 255, G: 245, B: 242, A: 255}
		border = color.NRGBA{R: 198, G: 83, B: 72, A: 255}
		borderWidth = gtx.Dp(2)
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, fill, clip.UniformRRect(image.Rect(0, 0, size.X, size.Y), gtx.Dp(5)).Op(gtx.Ops))
			proofreadPaginationBorder(gtx, size, border, borderWidth)
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			return view.button("page-"+strconv.Itoa(index)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(7), Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							previewSize := image.Pt(gtx.Dp(46), gtx.Dp(61))
							gtx.Constraints = layout.Exact(previewSize)
							if img := view.thumbCache[index]; img.Size().X > 0 {
								return layoutProofreadingPageThumbnailImage(gtx, img, selected)
							}
							return a.layoutProofreadingThumbnailPlaceholder(gtx, index, selected)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							page := a.textArgs("proofread.pagination.page", i18n.Args{
								"Page": proofreadingPageLabelNumber(a, index),
							})
							label := material.Label(a.theme, 10, page)
							label.Color = proofreadMuted
							if selected {
								label.Color = color.NRGBA{R: 166, G: 71, B: 62, A: 255}
								label.Font.Weight = font.Bold
							} else {
								label.Font.Weight = font.SemiBold
							}
							label.MaxLines = 1
							return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = 0
								return label.Layout(gtx)
							})
						}),
					)
				})
			})
		},
	)
}

func proofreadingPageLabelNumber(a *App, pageIndex int) string {
	page := i18n.FormatNumber(a.localeSnapshot().Resolved, float64(pageIndex+1))
	if pageIndex >= 0 && pageIndex < 9 {
		page = "0" + page
	}
	return page
}

func layoutProofreadingPageThumbnailImage(gtx layout.Context, imageOp paint.ImageOp, selected bool) layout.Dimensions {
	size := gtx.Constraints.Max
	border := color.NRGBA{R: 225, G: 230, B: 234, A: 255}
	if selected {
		border = color.NRGBA{R: 233, G: 203, B: 197, A: 255}
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: size}.Op())
			proofreadRoundedBorder(gtx, size, 0, border, max(1, gtx.Dp(1)))
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(size)
			return widget.Image{Src: imageOp, Fit: widget.Contain}.Layout(gtx)
		},
	)
}

func (a *App) layoutProofreadingThumbnailPlaceholder(gtx layout.Context, index int, selected bool) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: size}.Op())
	border := color.NRGBA{R: 225, G: 230, B: 234, A: 255}
	lineColor := color.NRGBA{R: 203, G: 210, B: 216, A: 255}
	if selected {
		border = color.NRGBA{R: 233, G: 203, B: 197, A: 255}
	}
	proofreadRoundedBorder(gtx, size, 0, border, max(1, gtx.Dp(1)))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 166, G: 175, B: 184, A: 255}, clip.Rect{Min: image.Pt(gtx.Dp(6), gtx.Dp(8)), Max: image.Pt(gtx.Dp(35), gtx.Dp(10))}.Op())
	lineWidths := [...]unit.Dp{34, 31, 34, 31, 20}
	for row, lineWidth := range lineWidths {
		y := gtx.Dp(unit.Dp(16 + row*7))
		lineHeight := max(1, gtx.Dp(1.5))
		paint.FillShape(gtx.Ops, lineColor, clip.Rect{Min: image.Pt(gtx.Dp(6), y), Max: image.Pt(gtx.Dp(6+lineWidth), y+lineHeight)}.Op())
	}
	return layout.Dimensions{Size: size}
}

func proofreadPaginationBorder(gtx layout.Context, size image.Point, stroke color.NRGBA, thickness int) {
	proofreadRoundedBorder(gtx, size, 5, stroke, thickness)
}

func proofreadRoundedBorder(gtx layout.Context, size image.Point, radius unit.Dp, stroke color.NRGBA, thickness int) {
	if size.X <= 0 || size.Y <= 0 {
		return
	}
	thickness = max(1, thickness)
	corner := min(gtx.Dp(radius), min(size.X, size.Y)/2)
	shape := clip.UniformRRect(image.Rect(0, 0, size.X, size.Y), corner)
	paint.FillShape(gtx.Ops, stroke, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(thickness)}.Op())
}

func (a *App) layoutProofreadingPaginationWaiting(gtx layout.Context) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		label := material.Label(a.theme, 11, a.textID("proofread.pagination.waiting"))
		label.Color = proofreadMuted
		return label.Layout(gtx)
	})
}
