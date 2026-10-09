package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (a *App) layoutSpreadsheetStatusBar(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(50))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height

	if state := a.documentSessionState; state != nil {
		if state.spreadsheetStatusZoomOutButton.Clicked(gtx) {
			a.setSpreadsheetZoom(a.spreadsheetZoom() - 0.1)
		}
		if state.spreadsheetStatusZoomInButton.Clicked(gtx) {
			a.setSpreadsheetZoom(a.spreadsheetZoom() + 0.1)
		}
		if state.spreadsheetStatusZoomResetBtn.Clicked(gtx) {
			a.setSpreadsheetZoom(1.0)
		}
	}

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, chromeSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, gridLine, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			status := a.status
			if status == "" {
				status = "就绪"
			}
			if runes := []rune(status); len(runes) > 18 {
				status = string(runes[:17]) + "…"
			}

			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			gtx.Constraints.Min.Y = height
			gtx.Constraints.Max.Y = height

			return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return spreadsheetStatusText(gtx, a.theme, status)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
					}),
					layout.Rigid(a.layoutSpreadsheetZoomControl),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(22)}.Layout(gtx, a.layoutSpreadsheetZoomValue)
					}),
				)
			})
		},
	)
}

func (a *App) layoutSpreadsheetZoomControl(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	zoom := a.spreadsheetZoom()

	railW := gtx.Dp(unit.Dp(110))
	railH := gtx.Dp(unit.Dp(4))
	trackAreaH := gtx.Dp(unit.Dp(28))

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if state == nil {
				return spreadsheetStatusIcon(gtx, "minus")
			}
			return state.spreadsheetStatusZoomOutButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return spreadsheetStatusIcon(gtx, "minus")
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(railW, trackAreaH)
				gtx.Constraints = layout.Exact(size)

				area := clip.Rect{Max: size}.Push(gtx.Ops)
				if state != nil {
					state.spreadsheetStatusZoomDrag.Add(gtx.Ops)
					for ev, ok := state.spreadsheetStatusZoomDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal); ok; ev, ok = state.spreadsheetStatusZoomDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal) {
						switch ev.Kind {
						case pointer.Press, pointer.Drag, pointer.Release:
							frac := float64(ev.Position.X) / float64(railW)
							a.setSpreadsheetZoom(spreadsheetZoomFromFraction(frac))
							zoom = a.spreadsheetZoom()
						}
					}
				}

				railY := (trackAreaH - railH) / 2
				railRect := image.Rect(0, railY, railW, railY+railH)
				paint.FillShape(gtx.Ops, color.NRGBA{R: 205, G: 211, B: 218, A: 255}, clip.UniformRRect(railRect, railH/2).Op(gtx.Ops))

				thumbDiam := gtx.Dp(unit.Dp(10))
				frac := spreadsheetZoomFraction(zoom)
				thumbCenterX := int(math.Round(frac * float64(railW)))
				thumbLeft := thumbCenterX - thumbDiam/2
				if thumbLeft < 0 {
					thumbLeft = 0
				}
				if thumbLeft+thumbDiam > railW {
					thumbLeft = railW - thumbDiam
				}
				thumbY := (trackAreaH - thumbDiam) / 2
				thumbRect := image.Rect(thumbLeft, thumbY, thumbLeft+thumbDiam, thumbY+thumbDiam)
				paint.FillShape(gtx.Ops, color.NRGBA{R: 105, G: 114, B: 125, A: 255}, clip.UniformRRect(thumbRect, thumbDiam/2).Op(gtx.Ops))

				area.Pop()
				return layout.Dimensions{Size: size}
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if state == nil {
				return spreadsheetStatusIcon(gtx, "plus")
			}
			return state.spreadsheetStatusZoomInButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return spreadsheetStatusIcon(gtx, "plus")
			})
		}),
	)
}

func (a *App) layoutSpreadsheetZoomValue(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	zoom := a.spreadsheetZoom()
	text := fmt.Sprintf("%d%%", int(math.Round(zoom*100)))

	if state == nil {
		return spreadsheetStatusText(gtx, a.theme, text)
	}
	return state.spreadsheetStatusZoomResetBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return spreadsheetStatusText(gtx, a.theme, text)
	})
}

func spreadsheetStatusText(gtx layout.Context, theme *material.Theme, text string) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	style := material.Body2(theme, text)
	style.TextSize = unit.Sp(12)
	style.Color = mutedText
	return layout.Inset{Top: unit.Dp(3)}.Layout(gtx, style.Layout)
}

func spreadsheetStatusIcon(gtx layout.Context, name string) layout.Dimensions {
	icon, _ := tintedToolbarIcon(name, color.NRGBA{R: 105, G: 114, B: 125, A: 255})
	iconSize := gtx.Dp(unit.Dp(18))
	gtx.Constraints.Min = image.Pt(iconSize, iconSize)
	gtx.Constraints.Max = gtx.Constraints.Min
	return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: 18.0 / 96.0}.Layout(gtx)
}
