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
	"gioui.org/widget/material"
)

var proofreadEmptyCanvas = color.NRGBA{R: 233, G: 237, B: 241, A: 255}

func (a *App) layoutProofreadingEmptyFilmstrip(gtx layout.Context) layout.Dimensions {
	height := min(gtx.Dp(48), gtx.Constraints.Max.Y)
	size := image.Pt(gtx.Constraints.Max.X, height)
	gtx.Constraints = layout.Exact(size)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 243, G: 245, B: 247, A: 255}, clip.Rect{Max: size}.Op())
			paint.FillShape(gtx.Ops, color.NRGBA{R: 221, G: 226, B: 231, A: 255}, clip.Rect{Max: image.Pt(size.X, max(1, gtx.Dp(1)))}.Op())
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutProofreadingIcon(gtx, "ribbon", "list-filter", 18, color.NRGBA{R: 154, G: 164, B: 175, A: 255})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Dp(10), 0)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Label(a.theme, 12, a.textID("proofread.filmstrip.empty"))
						label.Color = color.NRGBA{R: 138, G: 148, B: 159, A: 255}
						return layoutProofreadingOpticallyCenteredLabel(gtx, label)
					}),
				)
			})
		},
	)
}

func (a *App) layoutProofreadingEmptyObject(gtx layout.Context) layout.Dimensions {
	state := a.proofreading
	if state == nil {
		return layout.Dimensions{}
	}
	if state.chooseDraft.Clicked(gtx) && !state.draftLoading {
		a.startProofreadingSourcePicker()
	}
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, proofreadEmptyCanvas, clip.Rect{Max: size}.Op())
	cardWidth := min(gtx.Dp(464), max(1, size.X-gtx.Dp(32)))
	cardHeight := min(gtx.Dp(520), max(1, size.Y-gtx.Dp(28)))
	cardPoint := image.Pt((size.X-cardWidth)/2, gtx.Dp(14))
	if cardPoint.Y+cardHeight > size.Y {
		cardPoint.Y = max(0, size.Y-cardHeight)
	}
	offset := op.Offset(cardPoint).Push(gtx.Ops)
	defer offset.Pop()
	cardSize := image.Pt(cardWidth, cardHeight)
	shadowOffset := gtx.Dp(2)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 23, G: 33, B: 43, A: 24}, clip.UniformRRect(image.Rect(-shadowOffset, -shadowOffset, cardWidth+shadowOffset, cardHeight+shadowOffset), gtx.Dp(2)).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.UniformRRect(image.Rect(0, 0, cardWidth, cardHeight), gtx.Dp(2)).Op(gtx.Ops))
	proofreadingPageBorder(gtx, cardSize)
	contentWidth := max(1, cardWidth-gtx.Dp(64))
	if contentWidth > gtx.Dp(400) {
		contentWidth = gtx.Dp(400)
	}
	left := (cardWidth - contentWidth) / 2
	a.layoutProofreadingEmptyLabel(gtx, image.Pt(left, gtx.Dp(22)), contentWidth, 10, "proofread.empty.stage", color.NRGBA{R: 132, G: 146, B: 159, A: 255}, font.Normal)
	a.layoutProofreadingEmptyLabel(gtx, image.Pt(left, gtx.Dp(53)), contentWidth, 20, "proofread.empty.title", color.NRGBA{R: 39, G: 50, B: 61, A: 255}, font.SemiBold)
	a.layoutProofreadingEmptyLabel(gtx, image.Pt(left, gtx.Dp(91)), contentWidth, 12, "proofread.empty.body", color.NRGBA{R: 99, G: 112, B: 124, A: 255}, font.Normal)
	zoneTop := gtx.Dp(140)
	zoneHeight := min(gtx.Dp(210), max(gtx.Dp(116), cardHeight-zoneTop-gtx.Dp(70)))
	zoneSize := image.Pt(contentWidth, zoneHeight)
	zoneOffset := op.Offset(image.Pt(left, zoneTop)).Push(gtx.Ops)
	state.chooseDraft.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(zoneSize)
		paint.FillShape(gtx.Ops, color.NRGBA{R: 248, G: 250, B: 253, A: 255}, clip.UniformRRect(image.Rect(0, 0, zoneSize.X, zoneSize.Y), gtx.Dp(6)).Op(gtx.Ops))
		proofreadRoundedBorder(gtx, zoneSize, 6, color.NRGBA{R: 200, G: 216, B: 232, A: 255}, max(1, gtx.Dp(1)))
		if state.draftLoading {
			label := material.Label(a.theme, 15, a.textID("proofread.empty.loading"))
			label.Color = color.NRGBA{R: 52, G: 120, B: 209, A: 255}
			label.Font.Weight = font.SemiBold
			layout.Center.Layout(gtx, label.Layout)
			return layout.Dimensions{Size: zoneSize}
		}
		return a.layoutProofreadingUploadZoneContents(gtx)
	})
	zoneOffset.Pop()
	separatorY := gtx.Dp(378)
	if zoneTop+zoneHeight+gtx.Dp(14) > separatorY {
		separatorY = zoneTop + zoneHeight + gtx.Dp(14)
	}
	if separatorY < cardHeight-gtx.Dp(40) {
		paint.FillShape(gtx.Ops, color.NRGBA{R: 227, G: 231, B: 235, A: 255}, clip.Rect{Min: image.Pt(left, separatorY), Max: image.Pt(left+contentWidth, separatorY+max(1, gtx.Dp(1)))}.Op())
	}
	messageY := max(gtx.Dp(402), separatorY+gtx.Dp(18))
	messageColor := color.NRGBA{R: 104, G: 119, B: 134, A: 255}
	message := "proofread.empty.immutable"
	if state.draftError != "" {
		message = ""
		label := material.Label(a.theme, 11, state.draftError)
		label.Color = color.NRGBA{R: 157, G: 61, B: 54, A: 255}
		label.MaxLines = 2
		child := gtx
		child.Constraints.Max.X = contentWidth
		errOffset := op.Offset(image.Pt(left, messageY)).Push(gtx.Ops)
		label.Layout(child)
		errOffset.Pop()
	} else {
		a.layoutProofreadingEmptyLabel(gtx, image.Pt(left, messageY), contentWidth, 11, message, messageColor, font.Normal)
	}
	return layout.Dimensions{Size: size}
}

func (a *App) layoutProofreadingUploadZoneContents(gtx layout.Context) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Dp(48)
				gtx.Constraints = layout.Exact(image.Pt(size, size))
				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						shape := clip.Ellipse{Max: image.Pt(size, size)}
						paint.FillShape(gtx.Ops, color.NRGBA{R: 234, G: 242, B: 252, A: 255}, shape.Op(gtx.Ops))
						return layout.Dimensions{Size: image.Pt(size, size)}
					},
					func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layoutProofreadingIcon(gtx, "ribbon", "cloud-upload", 23, color.NRGBA{R: 52, G: 120, B: 209, A: 255})
						})
					},
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(0, gtx.Dp(12))} }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Label(a.theme, 15, a.textID("proofread.empty.choose_hint"))
				label.Color = color.NRGBA{R: 39, G: 48, B: 58, A: 255}
				label.Font.Weight = font.SemiBold
				return label.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(0, gtx.Dp(12))} }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Label(a.theme, 12, a.textID("proofread.empty.upload_hint"))
				label.Color = color.NRGBA{R: 122, G: 131, B: 142, A: 255}
				label.MaxLines = 1
				return label.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(0, gtx.Dp(12))} }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Dp(34)
				gtx.Constraints.Max.Y = gtx.Dp(34)
				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						size := gtx.Constraints.Min
						paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.UniformRRect(image.Rect(0, 0, size.X, size.Y), gtx.Dp(7)).Op(gtx.Ops))
						proofreadRoundedBorder(gtx, size, 7, color.NRGBA{R: 185, G: 196, B: 208, A: 255}, max(1, gtx.Dp(1)))
						return layout.Dimensions{Size: size}
					},
					func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							label := material.Label(a.theme, 12, a.textID("proofread.action.choose"))
							label.Color = color.NRGBA{R: 78, G: 87, B: 98, A: 255}
							label.Font.Weight = font.SemiBold
							return layout.Center.Layout(gtx, label.Layout)
						})
					},
				)
			}),
		)
	})
}

func (a *App) layoutProofreadingEmptyLabel(gtx layout.Context, point image.Point, width int, size unit.Sp, key string, ink color.NRGBA, weight font.Weight) {
	offset := op.Offset(point).Push(gtx.Ops)
	defer offset.Pop()
	child := gtx
	child.Constraints.Min.X = 0
	child.Constraints.Max.X = width
	label := material.Label(a.theme, size, a.textID(key))
	label.Color = ink
	label.Font.Weight = weight
	label.MaxLines = 2
	label.Layout(child)
}

func (a *App) layoutProofreadingEmptyCenteredLabel(gtx layout.Context, size image.Point, textSize unit.Sp, key string, ink color.NRGBA, y int) {
	label := material.Label(a.theme, textSize, a.textID(key))
	label.Color = ink
	if textSize > 11 {
		label.Font.Weight = font.SemiBold
	}
	label.MaxLines = 1
	offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	child := gtx
	child.Constraints.Min = image.Pt(size.X, 0)
	child.Constraints.Max = image.Pt(size.X, gtx.Dp(28))
	layout.Center.Layout(child, label.Layout)
	offset.Pop()
}
